package main

import (
	"fmt"
	"strings"
)

type versionHandler struct{}

func (versionHandler) Inspect(content []byte) (Inspection, error) {
	s := strings.TrimSpace(string(content))
	if s == "" {
		return Inspection{}, fmt.Errorf("VERSION: empty")
	}
	return Inspection{
		Versions: []Field{{Value: s, Path: "(file content)"}},
	}, nil
}

func (versionHandler) Replace(content []byte, _ /* current */, newVersion string) ([]byte, error) {
	// Preserve the trailing newline if the source had one.
	if len(content) > 0 && content[len(content)-1] == '\n' {
		return []byte(newVersion + "\n"), nil
	}
	return []byte(newVersion), nil
}
