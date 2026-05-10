package main

import (
	"fmt"
	"regexp"
)

// format_regex.go implements the DR-0012 generic "line-anchored regex"
// format. It is the workhorse for languages whose version is a single
// expression in the source file rather than a structured manifest:
// xcconfig, CocoaPods podspec, Nim nimble, V `v.mod`, Zig
// `build.zig.zon`, Ruby gemspec, Elixir `mix.exs`, Scala `build.sbt`.
//
// A `regex` rule is described by two fields on `CandidateRule`:
//
//   - VersionRegex (required): a regular expression with exactly one
//     capture group `(...)`. The captured byte range is the version
//     value; everything else (including any quote characters around
//     the value) is treated as fixed prefix/suffix and preserved
//     verbatim during Replace.
//   - NameRegex (optional): same shape, used to extract a package
//     name when present. A failed extraction here does not fail the
//     rule — the JSON/TOML/YAML formats treat name as optional too.
//
// Only the **first** match in the file is examined. Multi-match files
// (project.pbxproj-style synchronised settings) are intentionally out
// of scope; if the need arises, a dedicated format is the right
// answer (see DR-0012 § "1 ファイル 1 マッチ").

// regexInspect extracts the version (and optionally the name) from
// `content` using the rule's regex fields. The captured value of the
// first sub-match is reported as the version; an empty or absent
// match yields a clear extraction error so the dispatcher can fall
// through (or surface a meaningful "missing version" diagnostic when
// the rule is the last candidate).
func regexInspect(rule CandidateRule, content []byte) (Inspection, error) {
	if rule.VersionRegex == "" {
		return Inspection{}, fmt.Errorf("regex rule %q has no VersionRegex", rule.Name)
	}
	vre, err := regexp.Compile(rule.VersionRegex)
	if err != nil {
		return Inspection{}, fmt.Errorf("regex rule %q: invalid VersionRegex: %w", rule.Name, err)
	}
	if vre.NumSubexp() < 1 {
		return Inspection{}, fmt.Errorf("regex rule %q: VersionRegex has no capture group", rule.Name)
	}
	loc := vre.FindSubmatchIndex(content)
	if loc == nil {
		return Inspection{}, fmt.Errorf("missing version (regex %q did not match)", rule.VersionRegex)
	}
	// loc layout: [0,1]=full match, [2,3]=group 1, [4,5]=group 2, ...
	// We always use group 1 as the version value.
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
	// Name is optional. NameRegex absence or extraction failure is
	// silently swallowed — the rule still wins on the strength of its
	// version match alone.
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

// regexReplace rewrites the byte range covered by the first capture
// group of the rule's VersionRegex with `newVersion`, leaving every
// other byte (quotes, surrounding identifier text, trailing comments,
// neighbouring lines) verbatim. Only the first match is touched, by
// design.
func regexReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if rule.VersionRegex == "" {
		return nil, fmt.Errorf("regex rule %q has no VersionRegex", rule.Name)
	}
	vre, err := regexp.Compile(rule.VersionRegex)
	if err != nil {
		return nil, fmt.Errorf("regex rule %q: invalid VersionRegex: %w", rule.Name, err)
	}
	if vre.NumSubexp() < 1 {
		return nil, fmt.Errorf("regex rule %q: VersionRegex has no capture group", rule.Name)
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
