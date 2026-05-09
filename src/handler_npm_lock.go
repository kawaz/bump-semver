package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// npmLockHandler handles npm 7+ `package-lock.json` (lockfileVersion 2 / 3).
//
// It is split out from the generic JSON handler because a lockfile contains
// many `"version"` keys (one per dependency under
// `$.packages["node_modules/<name>"]`) and only the project's own version —
// `$.version` and `$.packages[""].version` — should be bumped. Replace uses
// the JSON parser to walk the structure and only rewrite those two paths,
// so dependency entries are never touched.
//
// lockfileVersion 1 (npm 5/6, dependencies tree without `packages`) is
// rejected — users are asked to regenerate with npm 7+.
type npmLockHandler struct{}

func (npmLockHandler) Inspect(content []byte) (Inspection, error) {
	var doc struct {
		Name            string                     `json:"name"`
		Version         string                     `json:"version"`
		LockfileVersion int                        `json:"lockfileVersion"`
		Packages        map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse package-lock.json: %w", err)
	}
	if doc.LockfileVersion == 1 || doc.Packages == nil {
		return Inspection{}, fmt.Errorf("unsupported lockfileVersion: 1, please regenerate with npm 7+")
	}

	insp := Inspection{}
	if doc.Version != "" {
		insp.Versions = append(insp.Versions, Field{Value: doc.Version, Path: "$.version"})
	}
	if doc.Name != "" {
		insp.Names = append(insp.Names, Field{Value: doc.Name, Path: "$.name"})
	}

	if rawRoot, ok := doc.Packages[""]; ok {
		var rootEntry struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(rawRoot, &rootEntry); err != nil {
			return Inspection{}, fmt.Errorf(`parse package-lock.json $.packages[""]: %w`, err)
		}
		if rootEntry.Version != "" {
			insp.Versions = append(insp.Versions, Field{Value: rootEntry.Version, Path: `$.packages[""].version`})
		}
		if rootEntry.Name != "" {
			insp.Names = append(insp.Names, Field{Value: rootEntry.Name, Path: `$.packages[""].name`})
		}
	}

	if len(insp.Versions) == 0 {
		return Inspection{}, fmt.Errorf(`package-lock.json: no version field found at $.version or $.packages[""].version`)
	}
	return insp, nil
}

// npmLockOffset is the inclusive byte range of one version *value*
// (the bytes between the surrounding quotes).
type npmLockOffset struct {
	start, end int
	path       string
}

func (npmLockHandler) Replace(content []byte, _ /* current */, newVersion string) ([]byte, error) {
	offsets, err := findNpmLockVersionOffsets(content)
	if err != nil {
		return nil, err
	}
	if len(offsets) == 0 {
		return nil, fmt.Errorf("package-lock.json: no version field to replace")
	}
	// Replace from tail to head so earlier offsets stay valid.
	sort.Slice(offsets, func(i, j int) bool { return offsets[i].start > offsets[j].start })
	out := append([]byte{}, content...)
	repl := []byte(newVersion)
	for _, t := range offsets {
		head := append([]byte{}, out[:t.start]...)
		tail := append([]byte{}, out[t.end:]...)
		out = append(head, repl...)
		out = append(out, tail...)
	}
	return out, nil
}

// findNpmLockVersionOffsets walks the JSON document with a streaming
// decoder and records the byte ranges of the two version *values*:
//
//   - top-level `$.version`
//   - `$.packages[""].version`
//
// `$.packages["node_modules/<name>"]` entries are explicitly skipped so
// dependency versions are never touched.
func findNpmLockVersionOffsets(content []byte) ([]npmLockOffset, error) {
	dec := json.NewDecoder(bytes.NewReader(content))

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parse package-lock.json: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("package-lock.json: top-level value must be an object")
	}

	var offsets []npmLockOffset

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse package-lock.json: %w", err)
		}
		key, _ := keyTok.(string)

		switch key {
		case "version":
			before := dec.InputOffset()
			valTok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("parse package-lock.json: %w", err)
			}
			if _, ok := valTok.(string); !ok {
				return nil, fmt.Errorf(`package-lock.json: $.version is not a string`)
			}
			after := dec.InputOffset()
			s, e, ok := findQuotedRange(content, int(before), int(after))
			if !ok {
				return nil, fmt.Errorf(`package-lock.json: cannot locate $.version literal`)
			}
			offsets = append(offsets, npmLockOffset{start: s, end: e, path: "$.version"})

		case "packages":
			pkgsOffsets, err := scanPackagesObject(dec, content)
			if err != nil {
				return nil, err
			}
			offsets = append(offsets, pkgsOffsets...)

		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("parse package-lock.json: %w", err)
			}
		}
	}
	// closing `}`
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("parse package-lock.json: %w", err)
	}
	return offsets, nil
}

// scanPackagesObject reads the value of the `"packages"` field, expected
// to be an object mapping each path to its entry. Only the entry with key
// "" (the root package) contributes a version offset; the rest are skipped
// so dependency versions are never touched.
func scanPackagesObject(dec *json.Decoder, content []byte) ([]npmLockOffset, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf(`parse package-lock.json: %w`, err)
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return nil, fmt.Errorf(`package-lock.json: $.packages must be an object`)
	}

	var offsets []npmLockOffset

	for dec.More() {
		pkgKeyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse package-lock.json: %w", err)
		}
		pkgKey, _ := pkgKeyTok.(string)
		if pkgKey != "" {
			// Dependency entry — never bump.
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("parse package-lock.json: %w", err)
			}
			continue
		}

		// pkgKey == "" → root package entry. Walk its fields and record the
		// version offset.
		rTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse package-lock.json: %w", err)
		}
		rD, ok := rTok.(json.Delim)
		if !ok || rD != '{' {
			return nil, fmt.Errorf(`package-lock.json: $.packages[""] must be an object`)
		}
		for dec.More() {
			innerKeyTok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("parse package-lock.json: %w", err)
			}
			innerKey, _ := innerKeyTok.(string)
			if innerKey != "version" {
				if err := skipValue(dec); err != nil {
					return nil, fmt.Errorf("parse package-lock.json: %w", err)
				}
				continue
			}
			before := dec.InputOffset()
			valTok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("parse package-lock.json: %w", err)
			}
			if _, ok := valTok.(string); !ok {
				return nil, fmt.Errorf(`package-lock.json: $.packages[""].version is not a string`)
			}
			after := dec.InputOffset()
			s, e, ok := findQuotedRange(content, int(before), int(after))
			if !ok {
				return nil, fmt.Errorf(`package-lock.json: cannot locate $.packages[""].version literal`)
			}
			offsets = append(offsets, npmLockOffset{start: s, end: e, path: `$.packages[""].version`})
		}
		// closing `}` of the root entry
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("parse package-lock.json: %w", err)
		}
	}
	// closing `}` of $.packages
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("parse package-lock.json: %w", err)
	}
	return offsets, nil
}

// skipValue consumes one complete JSON value (token, object, or array)
// from the decoder.
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
// literal `"..."` in content[lo:hi]. The returned range is the contents
// between the quotes, i.e. excluding both quotes.
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
			i++ // skip the next escaped char
			continue
		}
		if content[i] == '"' {
			return startQ + 1, i, true
		}
	}
	return 0, 0, false
}
