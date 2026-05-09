package main

import (
	"fmt"
	"strings"
)

type versionHandler struct{}

func (versionHandler) Get(content []byte) (string, error) {
	s := strings.TrimSpace(string(content))
	if s == "" {
		return "", fmt.Errorf("VERSION: empty")
	}
	return s, nil
}

func (versionHandler) Replace(content []byte, newVersion string) ([]byte, error) {
	// Preserve the trailing newline if the source had one.
	if len(content) > 0 && content[len(content)-1] == '\n' {
		return []byte(newVersion + "\n"), nil
	}
	return []byte(newVersion), nil
}
