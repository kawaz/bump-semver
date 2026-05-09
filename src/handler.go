package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Handler reads / writes the version string of a single file format.
type Handler interface {
	Get(content []byte) (string, error)
	Replace(content []byte, newVersion string) ([]byte, error)
}

// detectHandler picks a Handler by basename. Detection is intentionally
// strict (no regex fallback). Adding a new format = adding a new Handler.
func detectHandler(path string) (Handler, error) {
	base := filepath.Base(path)
	switch {
	case base == "Cargo.toml":
		return cargoHandler{}, nil
	case base == "VERSION":
		return versionHandler{}, nil
	case strings.HasSuffix(base, ".json"):
		return jsonHandler{}, nil
	default:
		return nil, fmt.Errorf("unsupported file: %s", path)
	}
}
