package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Field is one detected version-like or name-like value inside a file.
// Path is a human-readable JSONPath / TOML-path string used in error
// messages (e.g. `$.version`, `[package].version`,
// `$.packages[""].version`, `(file content)`). It does not have to follow
// any single grammar; it just needs to disambiguate which spot inside the
// file the value came from.
type Field struct {
	Value string
	Path  string
}

// Inspection is everything a handler can extract from a file.
//
//   - Versions: 1 or more version strings detected in the file. Required —
//     a handler must return at least one or it should error out.
//   - Names: 0 or more package-name-like strings detected in the file.
//     Optional. Used solely for cross-file consistency validation; never
//     written back.
type Inspection struct {
	Versions []Field
	Names    []Field
}

// Handler reads / writes the version string of a single file format.
//
// Replace receives both the current and new version explicitly. current is
// the version string the handler itself returned via Inspect for the same
// content (callers are expected to thread through one of the values from
// Inspection.Versions). Threading the value through saves handlers from
// parsing the content a second time, and lets handlers like the JSON one
// anchor their regex on the current literal to avoid touching nested
// "version" keys with different values.
type Handler interface {
	Inspect(content []byte) (Inspection, error)
	Replace(content []byte, current, newVersion string) ([]byte, error)
}

// detectHandler picks a Handler by basename. Detection is intentionally
// strict (no regex fallback). Adding a new format = adding a new Handler.
//
// Order matters: package-lock.json must be checked before the generic
// "*.json" branch, since it is also a *.json but needs special treatment.
func detectHandler(path string) (Handler, error) {
	base := filepath.Base(path)
	switch {
	case base == "Cargo.toml":
		return cargoHandler{}, nil
	case base == "VERSION":
		return versionHandler{}, nil
	case base == "package-lock.json":
		return npmLockHandler{}, nil
	case strings.HasSuffix(base, ".json"):
		return jsonHandler{}, nil
	default:
		return nil, fmt.Errorf("unsupported file: %s", path)
	}
}
