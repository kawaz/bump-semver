package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// format_xml.go implements the DR-0015 Apple `Info.plist` format
// (XML plist). It is intentionally narrow: only the
// `<key>NAME</key><string>VALUE</string>` pair shape that plist files
// use is supported. Maven `pom.xml`, Android Gradle XML, generic XML
// configuration files etc. are out of scope and will get their own
// format file (e.g. `format_xml_pom.go`) when the need surfaces.
//
// Design rationale (DR-0015):
//
//   - `encoding/xml.Decoder` is used to walk tokens (so we get accurate
//     byte offsets and don't have to invent an XML parser), but the
//     resulting tree is **not** re-serialized via `xml.Marshal`. That
//     would lose the DOCTYPE, indentation, attribute order, and
//     declaration which Xcode is very particular about. Instead we
//     locate the byte range of the target value and splice the new
//     value into the original content.
//   - Path syntax is just the bare plist key name (e.g.
//     `CFBundleShortVersionString`). No XPath. The narrow shape of
//     plist documents makes that adequate.
//   - Placeholder values like `$(MARKETING_VERSION)` (Xcode 11+) are
//     extracted verbatim — `ParseVersion` further up the pipeline
//     rejects them, the rule fall-through path then turns the file
//     into an `unsupported file:` outcome with a helpful hint. We do
//     not special-case placeholders here.

// xmlInspect walks the plist looking for `<key>NAME</key><string>VAL</string>`
// pairs. Each `rule.VersionPaths[N]` / `rule.NamePaths[N]` entry is a
// bare plist key name; the corresponding `<string>` value is captured
// into `Versions` / `Names`.
//
// Behaviour notes:
//
//   - Multiple keys with the same name take the **first** occurrence
//     only (plist files typically don't repeat keys; if they do, the
//     later one wins at runtime, but bump-semver treats the file as
//     malformed and only reports the first hit).
//   - Whitespace-only `<string>   </string>` is treated as empty and
//     surfaces as a "missing" hit (rule fall-through).
//   - `VersionPaths` is mandatory; missing version keys yield an
//     extraction error so the dispatcher can fall through.
//     `NamePaths` is optional, matching the JSON / TOML / regex
//     conventions.
func xmlInspect(rule CandidateRule, content []byte) (Inspection, error) {
	if len(rule.VersionPaths) == 0 {
		return Inspection{}, fmt.Errorf("xml rule %q has no VersionPaths", rule.Name)
	}
	matches, err := scanPlistKeyValues(content)
	if err != nil {
		return Inspection{}, fmt.Errorf("parse XML plist: %w", err)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		key := strings.TrimPrefix(vp, ".")
		hit, ok := pickFirstByKey(matches, key)
		if !ok {
			return Inspection{}, fmt.Errorf("missing <key>%s</key>", key)
		}
		if strings.TrimSpace(hit.value) == "" {
			return Inspection{}, fmt.Errorf("empty <key>%s</key>", key)
		}
		insp.Versions = append(insp.Versions, Field{Value: hit.value, Path: key})
	}
	for _, np := range rule.NamePaths {
		key := strings.TrimPrefix(np, ".")
		if hit, ok := pickFirstByKey(matches, key); ok && strings.TrimSpace(hit.value) != "" {
			insp.Names = append(insp.Names, Field{Value: hit.value, Path: key})
		}
	}
	return insp, nil
}

// xmlReplace rewrites the byte range covered by each version key's
// `<string>...</string>` value with `newVersion`. The original file's
// indentation, DOCTYPE, attribute order, and trailing newline are
// preserved bit-for-bit because we splice into the original byte
// stream rather than serialising via `encoding/xml`.
//
// Multiple version keys (e.g. an Info.plist that ever lists
// `CFBundleShortVersionString` twice) are written in tail-to-head
// order so each splice keeps later offsets valid.
func xmlReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) == 0 {
		return nil, fmt.Errorf("xml rule %q has no VersionPaths", rule.Name)
	}
	matches, err := scanPlistKeyValues(content)
	if err != nil {
		return nil, fmt.Errorf("parse XML plist: %w", err)
	}
	type span struct{ start, end int }
	var spans []span
	for _, vp := range rule.VersionPaths {
		key := strings.TrimPrefix(vp, ".")
		hit, ok := pickFirstByKey(matches, key)
		if !ok {
			return nil, fmt.Errorf("cannot locate <key>%s</key> in source", key)
		}
		spans = append(spans, span{start: hit.valueStart, end: hit.valueEnd})
	}
	// Apply tail-to-head so prior spans stay valid.
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

// plistKeyValue is one `<key>NAME</key><string>VALUE</string>` pair
// that scanPlistKeyValues extracted from the source bytes, with
// enough offset information to splice a new value in place of `value`.
type plistKeyValue struct {
	key string
	// value is the textual content (CharData) of the matching
	// `<string>` element, with leading/trailing whitespace preserved.
	value string
	// valueStart / valueEnd are the byte offsets of the `<string>`
	// element's CharData inside the source content. They cover
	// exactly the bytes between `<string>` and `</string>`, so
	// `content[valueStart:valueEnd]` equals the raw inner text
	// (entities still in their `&amp;` form).
	valueStart, valueEnd int
}

// scanPlistKeyValues walks the document and returns every
// `<key>NAME</key>` immediately followed (after whitespace) by a
// `<string>VALUE</string>` element. Other element types (`<true/>`,
// `<integer>`, `<array>`, etc.) are recognised as non-matching and
// don't pair up with the preceding `<key>`.
//
// The byte offsets returned are taken from the underlying io.Reader
// position, which `encoding/xml.Decoder.InputOffset` exposes after
// each token. We also peek into the raw content to find the exact
// boundaries of the `<string>` element's CharData (which the Decoder
// gives us as the offset *after* the token, not the offsets of the
// inner text).
func scanPlistKeyValues(content []byte) ([]plistKeyValue, error) {
	dec := xml.NewDecoder(bytes.NewReader(content))
	// The decoder defaults to strict mode; that's what we want — a
	// malformed plist should error rather than silently misbehave.
	var out []plistKeyValue
	for {
		// Capture the offset of the *next* token's start before we
		// advance.
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "key" {
			continue
		}
		// Read the CharData inside <key>...</key>.
		var key string
		if next, err := dec.Token(); err != nil {
			return nil, err
		} else if cd, ok := next.(xml.CharData); ok {
			key = string(cd)
		}
		// Skip the matching </key>.
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		// The very next StartElement (after possible CharData
		// whitespace) is the value element. Anything else (a stray
		// `<key>` again, EOF, etc.) means this `<key>` has no value
		// pair and we silently drop it — the caller will report
		// "missing <key>X</key>" if it cared about that key.
		var valueStart, valueEnd int
		var valueText string
		var sawString bool
	consumeValue:
		for {
			tok2, err := dec.Token()
			if err == io.EOF {
				break consumeValue
			}
			if err != nil {
				return nil, err
			}
			switch v := tok2.(type) {
			case xml.CharData:
				// inter-element whitespace; keep going
			case xml.StartElement:
				if v.Name.Local != "string" {
					// `<true/>`, `<integer>`, `<array>`, ... — not a
					// rewriteable string value. Bail out without
					// recording.
					break consumeValue
				}
				// Take the offset *after* the `<string>` start tag.
				valueStart = int(dec.InputOffset())
				// Read the CharData; if the element is `<string/>`
				// (self-closing) or `<string></string>` the next
				// token is EndElement and value remains empty.
				inner, err := dec.Token()
				if err != nil {
					return nil, err
				}
				switch ic := inner.(type) {
				case xml.CharData:
					valueText = string(ic)
					// End offset is offset *before* `</string>`,
					// which equals the InputOffset right after the
					// CharData token (the decoder reads up to but not
					// including the `<` of the end tag... actually
					// it reads through it; we recover by subtracting
					// the length of the text plus the length of
					// trailing whitespace... see helper below).
					valueEnd = int(dec.InputOffset())
					// `dec.InputOffset()` after CharData is the byte
					// position of the first character past the
					// CharData run (i.e. the `<` of the end tag), so
					// it's exactly the value-end offset we want.
					// Skip the </string> end token.
					if _, err := dec.Token(); err != nil {
						return nil, err
					}
				case xml.EndElement:
					// `<string></string>` empty.
					valueText = ""
					valueEnd = valueStart
				default:
					// Unexpected; drop this pair.
					break consumeValue
				}
				sawString = true
				break consumeValue
			case xml.EndElement:
				// Closing of an enclosing element before we found
				// the value.
				break consumeValue
			}
		}
		if sawString {
			out = append(out, plistKeyValue{
				key:        key,
				value:      valueText,
				valueStart: valueStart,
				valueEnd:   valueEnd,
			})
		}
	}
	return out, nil
}

// pickFirstByKey returns the first plistKeyValue with the given key
// name. plist documents that repeat keys are degenerate; we simply
// take the first occurrence (Xcode does the same in its own parsers).
func pickFirstByKey(items []plistKeyValue, key string) (plistKeyValue, bool) {
	for _, kv := range items {
		if kv.key == key {
			return kv, true
		}
	}
	return plistKeyValue{}, false
}
