package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// JSON-like path support for the path-aware confidence-ranked dispatcher
// (DR-0005). The grammar is a deliberately small jq-style subset:
//
//   path    := segment+
//   segment := '.' identifier | '[' '"' string '"' ']'
//
// e.g.
//   .version
//   .metadata.version
//   .packages[""].version
//   .packages["node_modules/foo"].version
//
// Bracketed quoted-string segments are required for keys that contain dots,
// slashes, or are the empty string (the root entry of `package-lock.json`).

// parseJSONPath turns a path string into the sequence of object keys it
// addresses. The path must start with '.' (or '[' for the rare top-level
// quoted-key entry).
func parseJSONPath(p string) ([]string, error) {
	if p == "" || (p[0] != '.' && p[0] != '[') {
		return nil, fmt.Errorf("path must start with '.' or '[': %q", p)
	}
	var segs []string
	for i := 0; i < len(p); {
		switch p[i] {
		case '.':
			i++ // consume the dot
			j := i
			for j < len(p) && p[j] != '.' && p[j] != '[' {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("empty identifier segment in %q", p)
			}
			segs = append(segs, p[i:j])
			i = j
		case '[':
			i++
			if i >= len(p) || p[i] != '"' {
				return nil, fmt.Errorf(`expected '"' after '[' in %q`, p)
			}
			i++
			var sb strings.Builder
			for i < len(p) && p[i] != '"' {
				if p[i] == '\\' && i+1 < len(p) {
					sb.WriteByte(p[i+1])
					i += 2
					continue
				}
				sb.WriteByte(p[i])
				i++
			}
			if i >= len(p) {
				return nil, fmt.Errorf(`unterminated string in %q`, p)
			}
			i++ // closing "
			if i >= len(p) || p[i] != ']' {
				return nil, fmt.Errorf(`expected ']' in %q`, p)
			}
			i++ // closing ]
			segs = append(segs, sb.String())
		default:
			return nil, fmt.Errorf("unexpected character %q at offset %d in %q", p[i], i, p)
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty path: %q", p)
	}
	return segs, nil
}

// jsonPathExtract walks an already-decoded JSON document and returns the
// string value at path. found=false means the path did not resolve (a
// missing key on the way is a clean miss, not an error). If a non-string
// value sits at the leaf, that's an error — handlers don't try to coerce.
func jsonPathExtract(doc interface{}, path string) (value string, found bool, err error) {
	segs, err := parseJSONPath(path)
	if err != nil {
		return "", false, err
	}
	cur := doc
	for _, seg := range segs {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		v, exists := m[seg]
		if !exists {
			return "", false, nil
		}
		cur = v
	}
	s, ok := cur.(string)
	if !ok {
		return "", false, fmt.Errorf("value at %s is not a string", path)
	}
	return s, true, nil
}

// jsonOffset is the byte range of one located JSON string value, *between*
// (and excluding) its surrounding double quotes.
type jsonOffset struct {
	start, end int
	path       string
}

// locateJSONPath streams `content` with a json.Decoder, descending into the
// path's segments, and returns the byte range of the leaf string value.
// found=false means the path did not resolve (clean miss, not an error).
func locateJSONPath(content []byte, path string) (jsonOffset, bool, error) {
	segs, err := parseJSONPath(path)
	if err != nil {
		return jsonOffset{}, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(content))
	tok, err := dec.Token()
	if err != nil {
		return jsonOffset{}, false, fmt.Errorf("parse JSON: %w", err)
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return jsonOffset{}, false, fmt.Errorf("top-level JSON value must be an object")
	}
	off, found, err := descend(dec, content, segs, 0)
	if err != nil {
		return jsonOffset{}, false, err
	}
	if found {
		off.path = path
	}
	return off, found, nil
}

// descend walks the decoder until it either hits the leaf string described
// by segs[depth:] or runs out of options at the current level.
func descend(dec *json.Decoder, content []byte, segs []string, depth int) (jsonOffset, bool, error) {
	target := segs[depth]
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return jsonOffset{}, false, err
		}
		key, _ := keyTok.(string)
		if key != target {
			if err := skipValue(dec); err != nil {
				return jsonOffset{}, false, err
			}
			continue
		}
		// The key matched.
		if depth+1 == len(segs) {
			before := dec.InputOffset()
			valTok, err := dec.Token()
			if err != nil {
				return jsonOffset{}, false, err
			}
			if _, ok := valTok.(string); !ok {
				return jsonOffset{}, false, fmt.Errorf("value at %s is not a string", joinSegs(segs))
			}
			after := dec.InputOffset()
			s, e, ok := findQuotedRange(content, int(before), int(after))
			if !ok {
				return jsonOffset{}, false, fmt.Errorf("cannot locate string literal at %s", joinSegs(segs))
			}
			return jsonOffset{start: s, end: e}, true, nil
		}
		// More to descend — value must be an object.
		valTok, err := dec.Token()
		if err != nil {
			return jsonOffset{}, false, err
		}
		d, ok := valTok.(json.Delim)
		if !ok || d != '{' {
			return jsonOffset{}, false, fmt.Errorf("value at %s is not an object", joinSegs(segs[:depth+1]))
		}
		off, found, err := descend(dec, content, segs, depth+1)
		if err != nil {
			return jsonOffset{}, false, err
		}
		if found {
			return off, true, nil
		}
		// We descended into the right object but the next segment was not
		// found. Drain the rest of this object and bail.
		// (skipValue would re-consume the closing '}' on top of what we
		// already partially consumed; instead read until the matching '}'.)
		if err := drainObject(dec); err != nil {
			return jsonOffset{}, false, err
		}
		return jsonOffset{}, false, nil
	}
	return jsonOffset{}, false, nil
}

// drainObject reads tokens until the current object is closed.
func drainObject(dec *json.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

func joinSegs(segs []string) string {
	var sb strings.Builder
	for _, s := range segs {
		if isSimpleIdent(s) {
			sb.WriteByte('.')
			sb.WriteString(s)
		} else {
			sb.WriteString(`["`)
			sb.WriteString(s)
			sb.WriteString(`"]`)
		}
	}
	return sb.String()
}

func isSimpleIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		case c == '_' && i > 0:
		default:
			return false
		}
	}
	return true
}

// skipValue consumes one complete JSON value (token, object, or array)
// from the decoder. Used by descend() and by handlers that walk a JSON
// document with a streaming decoder.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if d != '{' && d != '[' {
		return nil
	}
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d2, ok := tok.(json.Delim); ok {
			switch d2 {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

// findQuotedRange finds the byte range of the first JSON-style string
// literal `"..."` in content[lo:hi]. The returned range is between the
// quotes (excluding both quotes), suitable for in-place replacement.
func findQuotedRange(content []byte, lo, hi int) (start, end int, ok bool) {
	if lo < 0 {
		lo = 0
	}
	if hi > len(content) {
		hi = len(content)
	}
	startQ := -1
	for i := lo; i < hi; i++ {
		if content[i] == '"' {
			startQ = i
			break
		}
	}
	if startQ < 0 {
		return 0, 0, false
	}
	for i := startQ + 1; i < hi; i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] == '"' {
			return startQ + 1, i, true
		}
	}
	return 0, 0, false
}
