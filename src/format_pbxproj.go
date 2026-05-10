package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// format_pbxproj.go implements the DR-0015 Xcode `project.pbxproj`
// format. The file is OpenStep plist (Apple's pre-XML key=value
// notation) and lives inside `<project>.xcodeproj/project.pbxproj`.
//
// `MARKETING_VERSION` is the user-visible version string (vs.
// `CURRENT_PROJECT_VERSION`, which is the build number — out of scope
// here). It appears once **per build configuration per target**, so a
// minimal app project with Debug + Release has at least two matches,
// and a multi-target / multi-flavor project can have many more. Xcode
// requires every occurrence to carry the **same** value; otherwise
// build configurations diverge in marketing version, which App Store
// Connect rejects.
//
// Design rationale (DR-0015):
//
//   - Dedicated format rather than extending the DR-0012 `regex`
//     format with a `MultiMatch` flag: keeping the regex format at
//     "first match only" preserves a clean responsibility boundary,
//     while the multi-match-with-consistency-check semantics live in
//     their own file.
//   - Inspect emits one `Field` per match with `Path = "line:N"`. When
//     values disagree, the dispatcher hands the slice to main.go's
//     existing `formatMismatchError`, which renders a column-aligned
//     `version mismatch:` block (`<file>:line:N` labels). No bespoke
//     error path is required.
//   - Replace looks for matches whose value equals the inspected
//     `current` and rewrites every one to `newVersion`, so the
//     synchronisation property is preserved by construction. Other
//     keys that happen to share the value (a `CURRENT_PROJECT_VERSION
//     = 1.2.3` set by mistake, etc.) are not touched because the
//     regex anchors on `MARKETING_VERSION`.

// pbxprojMarketingVersionRe matches one `MARKETING_VERSION = <value>;`
// line inside an OpenStep plist. The value may be unquoted
// (`1.2.3`) or double-quoted (`"1.2.3"`); the captured group is the
// raw value with quotes stripped.
//
// The regex is line-anchored (`(?m)^`) so we don't accidentally match
// occurrences inside trailing block comments, and the trailing `;` is
// required (the OpenStep plist statement terminator) so we don't trip
// on `MARKETING_VERSION` mentioned in a `/* ... */` comment block.
var pbxprojMarketingVersionRe = regexp.MustCompile(
	`(?m)^[ \t]*MARKETING_VERSION[ \t]*=[ \t]*"?([^";\s]+)"?[ \t]*;`)

// pbxprojInspect extracts every `MARKETING_VERSION` value, annotates it
// with the source line, and returns one `Field` per occurrence. When
// the values agree, main.go consumes them as a single `current`
// version (DR-0004 `allSameValue`); when they disagree, main.go's
// existing mismatch renderer surfaces a column-aligned diagnostic.
func pbxprojInspect(_ CandidateRule, content []byte) (Inspection, error) {
	matches := pbxprojMarketingVersionRe.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return Inspection{}, fmt.Errorf("missing MARKETING_VERSION in pbxproj")
	}
	insp := Inspection{}
	for _, m := range matches {
		// m layout: [0,1]=full match, [2,3]=group 1 (the value).
		if m[2] < 0 || m[3] < 0 {
			continue
		}
		val := string(content[m[2]:m[3]])
		if val == "" {
			continue
		}
		line := byteOffsetLine(content, m[2])
		insp.Versions = append(insp.Versions, Field{
			Value: val,
			Path:  fmt.Sprintf("line:%d", line),
		})
	}
	if len(insp.Versions) == 0 {
		return Inspection{}, fmt.Errorf("missing MARKETING_VERSION in pbxproj")
	}
	return insp, nil
}

// pbxprojReplace rewrites every match of the same regex with
// `newVersion`. We deliberately ignore `current` for the rewrite step
// itself — Inspect already certified that all `MARKETING_VERSION`
// values agree (otherwise main.go would have rejected the bump with
// `version mismatch:`), so the safe move is to overwrite **all**
// matches uniformly. That preserves the "every build configuration
// shares one marketing version" invariant the file's consumer (Xcode
// + App Store Connect) requires.
//
// Quote style is preserved match-by-match: an unquoted value stays
// unquoted, a `"..."` value keeps its double quotes. Replacements are
// applied tail-to-head so earlier byte offsets stay valid.
func pbxprojReplace(_ CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	matches := pbxprojMarketingVersionRe.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("missing MARKETING_VERSION in pbxproj")
	}
	out := append([]byte{}, content...)
	repl := []byte(newVersion)
	// Walk in reverse so earlier offsets keep their meaning after each
	// splice.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		if m[2] < 0 || m[3] < 0 {
			continue
		}
		head := append([]byte{}, out[:m[2]]...)
		tail := append([]byte{}, out[m[3]:]...)
		out = append(head, repl...)
		out = append(out, tail...)
	}
	return out, nil
}

// byteOffsetLine returns the 1-based line number that contains
// `content[off]`. Used to label individual `MARKETING_VERSION`
// matches in mismatch diagnostics (`<file>:line:N`).
func byteOffsetLine(content []byte, off int) int {
	if off < 0 || off > len(content) {
		return 0
	}
	return bytes.Count(content[:off], []byte{'\n'}) + 1
}
