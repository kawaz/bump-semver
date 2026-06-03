package main

import (
	"fmt"
	"regexp"
	"strings"
)

// format_text.go implements the unified "text" format that subsumes the
// legacy "plain" and "regex" formats (DR-0030).
//
// A `text` rule is described by two optional fields on `CandidateRule`:
//
//   - VersionRegex (optional): a regular expression with exactly one
//     capture group `(...)`. When set, the captured byte range of the
//     first match is the version value; everything else (quotes,
//     surrounding identifier text, trailing comments) is preserved
//     verbatim during Replace.
//     When unset, the entire file content (whitespace-trimmed for
//     Inspect, preserved newline for Replace) is treated as the
//     version string — the legacy "plain" semantics for VERSION-style
//     files.
//   - NameRegex (optional): same shape as VersionRegex, used to
//     extract a package name when present. A failed extraction here
//     does not fail the rule (names are advisory across all formats).
//     Ignored entirely when VersionRegex is unset (whole-file content
//     has nowhere to put a name).
//
// When VersionRegex is set, only the **first** match in the file is
// examined. Multi-match files (project.pbxproj-style synchronised
// settings) are intentionally out of scope; if the need arises, a
// dedicated format is the right answer (see DR-0012 § "1 ファイル 1
// マッチ").

// textInspect extracts the version (and optionally the name) from
// `content`. Dispatches between the legacy plain (no regex) and regex
// (with regex) behaviour based on VersionRegex.
func textInspect(rule CandidateRule, content []byte) (Inspection, error) {
	if rule.VersionRegex == "" {
		s := strings.TrimSpace(string(content))
		if s == "" {
			return Inspection{}, fmt.Errorf("empty file")
		}
		return Inspection{
			Versions: []Field{{Value: s, Path: "(file content)"}},
		}, nil
	}
	vre, err := regexp.Compile(rule.VersionRegex)
	if err != nil {
		return Inspection{}, fmt.Errorf("text rule %q: invalid VersionRegex: %w", rule.Name, err)
	}
	if vre.NumSubexp() < 1 {
		return Inspection{}, fmt.Errorf("text rule %q: VersionRegex has no capture group", rule.Name)
	}
	loc := vre.FindSubmatchIndex(content)
	if loc == nil {
		return Inspection{}, fmt.Errorf("missing version (regex %q did not match)", rule.VersionRegex)
	}
	if loc[2] < 0 || loc[3] < 0 {
		return Inspection{}, fmt.Errorf("missing version (capture group did not participate)")
	}
	val := string(content[loc[2]:loc[3]])
	if val == "" {
		return Inspection{}, fmt.Errorf("empty version capture")
	}
	insp := Inspection{
		Versions: []Field{{Value: val, Path: "(regex)"}},
	}
	if rule.NameRegex != "" {
		if nre, err := regexp.Compile(rule.NameRegex); err == nil && nre.NumSubexp() >= 1 {
			if nloc := nre.FindSubmatchIndex(content); nloc != nil {
				if nloc[2] >= 0 && nloc[3] >= 0 {
					if nval := string(content[nloc[2]:nloc[3]]); nval != "" {
						insp.Names = append(insp.Names, Field{Value: nval, Path: "(regex)"})
					}
				}
			}
		}
	}
	return insp, nil
}

// textReplace rewrites `content`. When VersionRegex is set, replaces
// only the first capture group's byte range with newVersion (DR-0012
// legacy regex behaviour). When unset, rewrites the whole file with
// newVersion, preserving any trailing newline (legacy plain behaviour
// for VERSION-style files).
func textReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if rule.VersionRegex == "" {
		if len(content) > 0 && content[len(content)-1] == '\n' {
			return []byte(newVersion + "\n"), nil
		}
		return []byte(newVersion), nil
	}
	vre, err := regexp.Compile(rule.VersionRegex)
	if err != nil {
		return nil, fmt.Errorf("text rule %q: invalid VersionRegex: %w", rule.Name, err)
	}
	if vre.NumSubexp() < 1 {
		return nil, fmt.Errorf("text rule %q: VersionRegex has no capture group", rule.Name)
	}
	loc := vre.FindSubmatchIndex(content)
	if loc == nil {
		return nil, fmt.Errorf("missing version (regex %q did not match)", rule.VersionRegex)
	}
	if loc[2] < 0 || loc[3] < 0 {
		return nil, fmt.Errorf("missing version (capture group did not participate)")
	}
	out := make([]byte, 0, len(content)-(loc[3]-loc[2])+len(newVersion))
	out = append(out, content[:loc[2]]...)
	out = append(out, newVersion...)
	out = append(out, content[loc[3]:]...)
	return out, nil
}
