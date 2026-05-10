package main

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// yamlInspect parses content as a YAML document and extracts the
// version / name fields described by the rule. This is the DR-0011
// `*.yaml` / `*.yml` confidence-1 fallback handler.
//
// Multi-document YAML (`---` separators) is intentionally out of
// scope: only the first document is examined. The vast majority of
// version-tracking YAML in the wild (Helm `Chart.yaml`, GitHub
// Actions metadata, custom manifest files) is single-document, and
// the multi-document case has no obvious "which doc wins" rule.
//
// The `jsonpath.go` extractor already handles `map[string]interface{}`
// trees, and yaml.v3 unmarshals a YAML mapping into exactly that
// shape, so the same path engine works for both formats. Display
// strings drop the leading `.` to match YAML convention (`version`
// rather than `$.version`).
func yamlInspect(rule CandidateRule, content []byte) (Inspection, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse YAML: %w", err)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		val, found, err := jsonPathExtract(doc, vp)
		if err != nil {
			return Inspection{}, err
		}
		if !found {
			return Inspection{}, fmt.Errorf("missing %s", yamlDisplayPath(vp))
		}
		if val == "" {
			return Inspection{}, fmt.Errorf("empty %s", yamlDisplayPath(vp))
		}
		insp.Versions = append(insp.Versions, Field{Value: val, Path: yamlDisplayPath(vp)})
	}
	for _, np := range rule.NamePaths {
		val, found, _ := jsonPathExtract(doc, np)
		if found && val != "" {
			insp.Names = append(insp.Names, Field{Value: val, Path: yamlDisplayPath(np)})
		}
	}
	return insp, nil
}

// yamlDisplayPath converts an internal jq-style path (`.version`)
// into the YAML-native display form used in error messages
// (`version` for top-level, `parent.child` for nested). Since the
// fallback only addresses top-level paths today, callers will almost
// always see the single-segment form.
func yamlDisplayPath(p string) string {
	segs, err := parseJSONPath(p)
	if err != nil || len(segs) == 0 {
		return p
	}
	return joinDot(segs)
}

// yamlTopLevelVersionLineRe matches a top-level mapping entry whose
// key is `version`, anchored at column 0 (start of line).
//
// "Top-level" here means **column 0** — no leading whitespace —
// because YAML uses indentation for nesting. A `version:` at column
// 0 belongs to the document root regardless of where else
// `version:` appears under nested keys.
//
// Submatch shape: prefix up to and including the colon + spaces, then
// the entire value tail (everything from the value's first character
// to the line terminator). The rewriter inspects the tail to decide
// whether the value is double-quoted, single-quoted, or unquoted, and
// substitutes only the value portion — preserving quote style and any
// trailing comment.
var yamlTopLevelVersionLineRe = regexp.MustCompile(
	`(?m)^(version[ \t]*:[ \t]*)([^\n]*)$`,
)

// yamlReplace rewrites the first top-level `version: ...` line found
// at column 0 in the document. The original quoting style and
// trailing comment (if any) are preserved verbatim — this is why we
// cannot round-trip through `yaml.Marshal`, which strips comments and
// reorders keys.
//
// Multi-document YAML is unsupported (DR-0011): only the first
// matching `version:` line wins. If callers need per-document
// rewriting they can split on `---` and process each document
// independently before concatenating.
func yamlReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) != 1 || rule.VersionPaths[0] != ".version" {
		return nil, fmt.Errorf("YAML format currently supports only top-level .version (got %v)", rule.VersionPaths)
	}
	loc := yamlTopLevelVersionLineRe.FindSubmatchIndex(content)
	if loc == nil {
		return nil, fmt.Errorf("missing top-level version line")
	}
	// loc layout: [0,1]=full match, [2,3]=prefix, [4,5]=value-tail.
	tailStart, tailEnd := loc[4], loc[5]
	tail := content[tailStart:tailEnd]
	valStart, valEnd, ok := yamlValueRange(tail, tailStart)
	if !ok {
		return nil, fmt.Errorf("malformed top-level version value")
	}
	out := make([]byte, 0, len(content)+len(newVersion))
	out = append(out, content[:valStart]...)
	out = append(out, newVersion...)
	out = append(out, content[valEnd:]...)
	return out, nil
}

// yamlValueRange locates the byte range of the actual scalar value
// inside the value-tail of a YAML mapping line (everything that
// follows `version:` up to the line terminator). Returned offsets are
// already shifted by `base` so they are absolute within the original
// buffer.
//
// The function understands three shapes:
//
//   - `"..."` — double-quoted: range is between the quotes.
//   - `'...'` — single-quoted: range is between the quotes.
//   - bare scalar — unquoted: range stops at the first inline comment
//     (`  #`) or trailing whitespace before EOL.
//
// Empty / whitespace-only / comment-only tails are reported as not
// ok, signalling a malformed value to the caller.
func yamlValueRange(tail []byte, base int) (start, end int, ok bool) {
	// Skip leading spaces/tabs (the regex already trimmed them, but
	// guard against future edits).
	i := 0
	for i < len(tail) && (tail[i] == ' ' || tail[i] == '\t') {
		i++
	}
	if i >= len(tail) {
		return 0, 0, false
	}
	switch tail[i] {
	case '"':
		// double-quoted: find the closing unescaped `"`.
		for j := i + 1; j < len(tail); j++ {
			if tail[j] == '\\' && j+1 < len(tail) {
				j++
				continue
			}
			if tail[j] == '"' {
				return base + i + 1, base + j, true
			}
		}
		return 0, 0, false
	case '\'':
		// single-quoted: YAML uses doubled `''` for an embedded
		// single quote; treat that literally so the rewriter doesn't
		// stop short on the first one.
		for j := i + 1; j < len(tail); j++ {
			if tail[j] == '\'' {
				if j+1 < len(tail) && tail[j+1] == '\'' {
					j++
					continue
				}
				return base + i + 1, base + j, true
			}
		}
		return 0, 0, false
	case '#':
		// comment-only — no value present.
		return 0, 0, false
	default:
		// unquoted bare scalar. End at the first `  #` (comment with
		// at least one space before `#`, per YAML rules) or at the
		// last non-whitespace byte before EOL.
		valStart := i
		valEnd := len(tail)
		for j := i; j < len(tail); j++ {
			if tail[j] == '#' && j > i && (tail[j-1] == ' ' || tail[j-1] == '\t') {
				valEnd = j
				break
			}
		}
		// Trim trailing whitespace.
		for valEnd > valStart && (tail[valEnd-1] == ' ' || tail[valEnd-1] == '\t') {
			valEnd--
		}
		if valEnd <= valStart {
			return 0, 0, false
		}
		return base + valStart, base + valEnd, true
	}
}
