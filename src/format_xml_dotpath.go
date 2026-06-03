package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// format_xml_dotpath.go implements the `xml` format for DR-0029
// user-defined rules (--format xml --version-path ...). Unlike the
// builtin `xml-element` format (format_xml_element.go), which uses
// slash-rooted paths (`/project/version`) and matches child elements
// only, this format uses the SAME dot-path language as json / yaml /
// toml (DR-0029 § "パス言語統一"):
//
//	$.project.version    (leading $ optional, JSONPath-style)
//	project.version
//	.project.version
//
// The unifying intent is "one path language for every structured
// format". XML's structural difference from JSON/YAML/TOML is that a
// node carries both child elements AND attributes; the final path
// segment is therefore resolved against BOTH:
//
//   - child element: the inner text of <last>...</last> under the
//     parent path.
//   - attribute:     the value of last="..." on the element named by
//     the path prefix.
//
// Resolution rule (DR-0029, kawaz 2026-06-03):
//   - exactly one of {child, attribute} has a value → use it.
//   - both have a value AND the values are EQUAL → accept (the peer-
//     consistency spirit: several spots holding the same value is fine).
//     On write, BOTH byte ranges are rewritten.
//   - both have a value but DIFFER → ambiguous error (do not guess).
//   - neither → not found error.
//
// textContent is whitespace-trimmed on read (XML pretty-printing often
// pads inner text). On write, the surrounding whitespace is preserved
// and ONLY the trimmed value's byte range is spliced — the same
// pinpoint-rewrite discipline used by the path+regex combination
// (DR-0029): never reformat, only replace the value bytes.

// xmlDotInspect resolves each VersionPath / NamePath as a dot-path and
// captures the resolved value (child inner text or attribute value).
func xmlDotInspect(rule CandidateRule, content []byte) (Inspection, error) {
	if len(rule.VersionPaths) == 0 {
		return Inspection{}, fmt.Errorf("xml rule %q has no VersionPaths", rule.Name)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		res, err := xmlDotResolve(content, vp)
		if err != nil {
			return Inspection{}, fmt.Errorf("%s: %w", vp, err)
		}
		insp.Versions = append(insp.Versions, Field{Value: res.value, Path: vp})
	}
	for _, np := range rule.NamePaths {
		res, err := xmlDotResolve(content, np)
		if err == nil && res.value != "" {
			insp.Names = append(insp.Names, Field{Value: res.value, Path: np})
		}
	}
	return insp, nil
}

// xmlDotReplace rewrites the trimmed-value byte range(s) of each version
// path with newVersion, preserving surrounding whitespace. A single
// path may yield multiple spans (child + attribute holding the same
// value); all are rewritten. Spans are spliced tail-to-head so earlier
// spans keep their offsets.
func xmlDotReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) == 0 {
		return nil, fmt.Errorf("xml rule %q has no VersionPaths", rule.Name)
	}
	var spans []xmlSpan
	for _, vp := range rule.VersionPaths {
		res, err := xmlDotResolve(content, vp)
		if err != nil {
			return nil, fmt.Errorf("cannot locate %s in source: %w", vp, err)
		}
		spans = append(spans, res.spans...)
	}
	// Sort descending by start so later splices don't shift earlier ones.
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

// xmlSpan is one byte range to rewrite.
type xmlSpan struct{ start, end int }

// xmlDotHit is one resolved value with its trimmed byte range.
type xmlDotHit struct {
	value                string
	valueStart, valueEnd int
}

// xmlDotResult is the resolution of one dot-path: the agreed value plus
// every byte range that holds it (1 for child-only / attr-only, 2 when
// child and attribute carry the same value).
type xmlDotResult struct {
	value string
	spans []xmlSpan
}

// parseXMLDotPath splits a dot-path into element/attribute name
// segments. Accepts an optional leading `$` and/or `.` (JSONPath
// style). Rejects `[N]` index syntax (reserved for a future sibling-
// index extension) and empty segments.
func parseXMLDotPath(path string) ([]string, error) {
	p := path
	if strings.HasPrefix(p, "$") {
		p = p[1:]
	}
	p = strings.TrimPrefix(p, ".")
	if p == "" {
		return nil, fmt.Errorf("empty path")
	}
	if strings.ContainsAny(p, "[]") {
		return nil, fmt.Errorf("xml dot-path %q: '[N]' index syntax is not supported (reserved for a future extension)", path)
	}
	segs := strings.Split(p, ".")
	for _, s := range segs {
		if s == "" {
			return nil, fmt.Errorf("xml dot-path %q has an empty segment", path)
		}
	}
	return segs, nil
}

// xmlDotResolve walks the document and resolves `path`, checking both
// the child-element interpretation and the attribute interpretation of
// the final segment. Both-present-and-equal is accepted (returns both
// spans); both-present-and-different is an ambiguous error.
func xmlDotResolve(content []byte, path string) (xmlDotResult, error) {
	segs, err := parseXMLDotPath(path)
	if err != nil {
		return xmlDotResult{}, err
	}

	var childHit *xmlDotHit
	var attrHit *xmlDotHit

	dec := xml.NewDecoder(bytes.NewReader(content))
	var stack []string
	prevOffset := 0
	for {
		startOff := prevOffset
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlDotResult{}, err
		}
		curOff := int(dec.InputOffset())
		prevOffset = curOff

		switch v := tok.(type) {
		case xml.StartElement:
			stack = append(stack, v.Name.Local)
			// Child interpretation: stack == segs exactly.
			if childHit == nil && stackEquals(stack, segs) {
				h, perr := readTrimmedInnerText(dec, &prevOffset)
				if perr != nil {
					return xmlDotResult{}, perr
				}
				if h.value != "" {
					hh := h
					childHit = &hh
				}
				// readTrimmedInnerText advanced the decoder past the
				// CharData; the matching EndElement token will pop the
				// stack on a later iteration.
				continue
			}
			// Attribute interpretation: stack == segs[:-1] and the last
			// segment names an attribute on this element.
			if attrHit == nil && len(segs) >= 2 && stackEquals(stack, segs[:len(segs)-1]) {
				attrName := segs[len(segs)-1]
				if h, ok := xmlAttrSpan(content, startOff, curOff, v, attrName); ok && h.value != "" {
					hh := h
					attrHit = &hh
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	switch {
	case childHit != nil && attrHit != nil:
		if childHit.value != attrHit.value {
			return xmlDotResult{}, fmt.Errorf("path %q is ambiguous: a child element and an attribute both match but hold different values (child=%q, attr=%q); disambiguate the document or path", path, childHit.value, attrHit.value)
		}
		// Same value in both spots — accept, rewrite both on write.
		return xmlDotResult{
			value: childHit.value,
			spans: []xmlSpan{
				{start: childHit.valueStart, end: childHit.valueEnd},
				{start: attrHit.valueStart, end: attrHit.valueEnd},
			},
		}, nil
	case childHit != nil:
		return xmlDotResult{value: childHit.value, spans: []xmlSpan{{start: childHit.valueStart, end: childHit.valueEnd}}}, nil
	case attrHit != nil:
		return xmlDotResult{value: attrHit.value, spans: []xmlSpan{{start: attrHit.valueStart, end: attrHit.valueEnd}}}, nil
	default:
		return xmlDotResult{}, fmt.Errorf("not found (neither child element nor attribute)")
	}
}

// stackEquals reports whether the element-name stack equals segs.
func stackEquals(stack, segs []string) bool {
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

// readTrimmedInnerText reads the CharData after a StartElement and
// returns the whitespace-trimmed value plus the byte range of the
// trimmed portion (surrounding whitespace excluded). prevOffset is
// updated so the caller's scan loop stays in sync.
func readTrimmedInnerText(dec *xml.Decoder, prevOffset *int) (xmlDotHit, error) {
	rawStart := int(dec.InputOffset())
	tok, err := dec.Token()
	if err != nil {
		return xmlDotHit{}, err
	}
	rawEnd := int(dec.InputOffset())
	*prevOffset = rawEnd
	switch t := tok.(type) {
	case xml.CharData:
		raw := string(t)
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return xmlDotHit{value: "", valueStart: rawStart, valueEnd: rawStart}, nil
		}
		// Compute the byte offsets of the trimmed value within the raw
		// CharData span. Leading/trailing whitespace stays untouched on
		// write (DR-0029 pinpoint-rewrite discipline).
		leadWS := len(raw) - len(strings.TrimLeft(raw, " \t\r\n"))
		trailWS := len(raw) - len(strings.TrimRight(raw, " \t\r\n"))
		return xmlDotHit{
			value:      trimmed,
			valueStart: rawStart + leadWS,
			valueEnd:   rawEnd - trailWS,
		}, nil
	case xml.EndElement:
		// Empty / self-closing element.
		return xmlDotHit{value: "", valueStart: rawStart, valueEnd: rawStart}, nil
	default:
		return xmlDotHit{value: "", valueStart: rawStart, valueEnd: rawStart}, nil
	}
}

// xmlAttrSpan locates the value byte range of attribute `attrName` on
// the start-element whose raw text spans content[tagStart:tagEnd]. The
// attribute value's bytes (inside the quotes) are returned so write can
// splice just the value. Returns ok=false if the attribute is absent.
//
// encoding/xml does not expose attribute byte offsets, so we re-scan
// the raw start-tag text. The match is anchored on a word boundary
// before the attribute name to avoid matching a substring of a longer
// attribute name (e.g. "version" inside "appversion").
func xmlAttrSpan(content []byte, tagStart, tagEnd int, se xml.StartElement, attrName string) (xmlDotHit, bool) {
	// Quick membership check via the decoded attrs (namespace-agnostic:
	// match by local name).
	found := false
	for _, a := range se.Attr {
		if a.Name.Local == attrName {
			found = true
			break
		}
	}
	if !found {
		return xmlDotHit{}, false
	}
	if tagStart < 0 || tagEnd > len(content) || tagStart >= tagEnd {
		return xmlDotHit{}, false
	}
	raw := content[tagStart:tagEnd]
	// Match: (boundary)attrName\s*=\s*("value"|'value')
	// Escape attrName for regex safety.
	re := regexp.MustCompile(`(?:^|[\s])` + regexp.QuoteMeta(attrName) + `\s*=\s*("([^"]*)"|'([^']*)')`)
	loc := re.FindSubmatchIndex(raw)
	if loc == nil {
		return xmlDotHit{}, false
	}
	// Group 2 = double-quoted value, group 3 = single-quoted value.
	var vStart, vEnd int
	if loc[4] >= 0 { // group 2 (double-quoted)
		vStart, vEnd = loc[4], loc[5]
	} else if loc[6] >= 0 { // group 3 (single-quoted)
		vStart, vEnd = loc[6], loc[7]
	} else {
		return xmlDotHit{}, false
	}
	value := string(raw[vStart:vEnd])
	return xmlDotHit{
		value:      value,
		valueStart: tagStart + vStart,
		valueEnd:   tagStart + vEnd,
	}, true
}
