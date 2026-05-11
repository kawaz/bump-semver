package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// format_xml_element.go implements a path-driven XML element value
// extractor / rewriter. This is the sibling of `format_xml.go` (which
// is plist-specific, working on `<key>NAME</key><string>VALUE</string>`
// pairs) — `xml-element` instead resolves a slash-separated path like
// `/project/version` or `/Project/PropertyGroup/Version` against the
// element tree and rewrites the inner text of the first match.
//
// Design rationale:
//
//   - Used for Maven `pom.xml` (`/project/version`, root version only;
//     `<parent>/<version>` is intentionally skipped because it
//     references a different artefact). Also used for .NET MSBuild
//     project files (`/Project/PropertyGroup/Version`).
//   - XML namespaces are ignored — match by local name only. Maven's
//     `<project xmlns="http://maven.apache.org/POM/4.0.0">` and similar
//     do not need their xmlns spelled out in path strings.
//   - Like format_xml.go, we walk tokens via `encoding/xml.Decoder` to
//     keep byte offsets accurate, then splice the new value into the
//     original byte stream rather than re-serialising. This preserves
//     DOCTYPE, attribute order, indentation, declaration, comments.
//   - Path syntax: leading slash `/` is mandatory. `/` separates
//     element names. The first segment must equal the root element's
//     local name. Subsequent segments are matched against immediate
//     children (depth-first, first match wins per level). Wildcards,
//     predicates, attribute selectors are out of scope.
//   - On multiple-match ambiguity (e.g. a pom.xml with `<project>` →
//     `<version>` and `<project>` → `<parent>` → `<version>`), the
//     **first leftmost match by document order** at the requested
//     depth wins. Maven's `<parent>` sits before the root `<version>`
//     in idiomatic pom.xml, but `<parent>/<version>` is not the same
//     path as `/project/version` (different depth), so the path query
//     keeps them apart.

// xmlElementInspect resolves each VersionPath / NamePath as an element
// path and captures the element's inner text.
func xmlElementInspect(rule CandidateRule, content []byte) (Inspection, error) {
	if len(rule.VersionPaths) == 0 {
		return Inspection{}, fmt.Errorf("xml-element rule %q has no VersionPaths", rule.Name)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		hit, err := findXMLElement(content, vp)
		if err != nil {
			return Inspection{}, fmt.Errorf("%s: %w", vp, err)
		}
		if strings.TrimSpace(hit.value) == "" {
			return Inspection{}, fmt.Errorf("%s: empty element", vp)
		}
		insp.Versions = append(insp.Versions, Field{Value: hit.value, Path: vp})
	}
	for _, np := range rule.NamePaths {
		hit, err := findXMLElement(content, np)
		if err == nil && strings.TrimSpace(hit.value) != "" {
			insp.Names = append(insp.Names, Field{Value: hit.value, Path: np})
		}
	}
	return insp, nil
}

// xmlElementReplace rewrites the byte range covered by each version
// path's inner text with newVersion. Multiple paths are spliced in
// tail-to-head order so earlier spans keep their offsets valid.
func xmlElementReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) == 0 {
		return nil, fmt.Errorf("xml-element rule %q has no VersionPaths", rule.Name)
	}
	type span struct{ start, end int }
	var spans []span
	for _, vp := range rule.VersionPaths {
		hit, err := findXMLElement(content, vp)
		if err != nil {
			return nil, fmt.Errorf("cannot locate %s in source: %w", vp, err)
		}
		spans = append(spans, span{start: hit.valueStart, end: hit.valueEnd})
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].start < spans[j].start {
				spans[i], spans[j] = spans[j], spans[i]
			}
		}
	}
	out := append([]byte{}, content...)
	repl := []byte(newVersion)
	for _, s := range spans {
		head := append([]byte{}, out[:s.start]...)
		tail := append([]byte{}, out[s.end:]...)
		out = append(head, repl...)
		out = append(out, tail...)
	}
	return out, nil
}

// xmlElementHit is the inner-text span of one resolved element.
type xmlElementHit struct {
	value                string
	valueStart, valueEnd int
}

// findXMLElement walks the document depth-first and returns the inner
// text + byte offsets of the first element matching `path`.
//
// `path` is a slash-rooted element path:
//
//	"/project/version"             — Maven root version
//	"/Project/PropertyGroup/Version" — .NET MSBuild project version
//	"/project/parent/version"      — Maven parent version (if ever needed)
//
// Matching is by local name only (XML namespaces are stripped). The
// path must consume at least one segment after the leading slash;
// otherwise an error is returned.
func findXMLElement(content []byte, path string) (xmlElementHit, error) {
	segs, err := parseElementPath(path)
	if err != nil {
		return xmlElementHit{}, err
	}
	dec := xml.NewDecoder(bytes.NewReader(content))
	// stack tracks the chain of currently-open element names so we can
	// know whether the cursor sits at the requested path. depth() ==
	// len(segs) means the next token is inside the target element.
	var stack []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlElementHit{}, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			stack = append(stack, v.Name.Local)
			if pathMatches(stack, segs) {
				// We are now inside the target element. Take inner text.
				return readInnerText(dec)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return xmlElementHit{}, fmt.Errorf("not found")
}

// parseElementPath splits "/a/b/c" into ["a", "b", "c"]. Rejects
// missing leading slash or empty / repeated separators.
func parseElementPath(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("element path %q must start with '/'", path)
	}
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segs) == 0 {
		return nil, fmt.Errorf("element path %q is empty", path)
	}
	for _, s := range segs {
		if s == "" {
			return nil, fmt.Errorf("element path %q has an empty segment", path)
		}
	}
	return segs, nil
}

// pathMatches checks whether the current element-name stack matches
// the path segments exactly.
func pathMatches(stack []string, segs []string) bool {
	if len(stack) != len(segs) {
		return false
	}
	for i := range stack {
		if stack[i] != segs[i] {
			return false
		}
	}
	return true
}

// readInnerText reads the CharData immediately following a
// StartElement and returns its value + byte offsets. The decoder's
// InputOffset right after the StartElement gives the start; after the
// CharData token it gives the end (i.e. the byte position of the
// closing tag's `<`). Empty / self-closing / nested-child elements
// surface as an empty value with start == end.
func readInnerText(dec *xml.Decoder) (xmlElementHit, error) {
	start := int(dec.InputOffset())
	tok, err := dec.Token()
	if err != nil {
		return xmlElementHit{}, err
	}
	switch t := tok.(type) {
	case xml.CharData:
		end := int(dec.InputOffset())
		return xmlElementHit{
			value:      string(t),
			valueStart: start,
			valueEnd:   end,
		}, nil
	case xml.EndElement:
		// Empty or self-closing element.
		return xmlElementHit{value: "", valueStart: start, valueEnd: start}, nil
	default:
		return xmlElementHit{}, fmt.Errorf("expected character data, got %T", tok)
	}
}
