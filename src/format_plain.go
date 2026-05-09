package main

import (
	"fmt"
	"strings"
)

// plainInspect treats the entire file content as a single trimmed version
// string. NamePath is ignored (plain text has nowhere to put a name).
func plainInspect(_ CandidateRule, content []byte) (Inspection, error) {
	s := strings.TrimSpace(string(content))
	if s == "" {
		return Inspection{}, fmt.Errorf("empty file")
	}
	return Inspection{
		Versions: []Field{{Value: s, Path: "(file content)"}},
	}, nil
}

// plainReplace rewrites the whole content with newVersion, preserving the
// trailing newline (if any) of the source.
func plainReplace(_ CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(content) > 0 && content[len(content)-1] == '\n' {
		return []byte(newVersion + "\n"), nil
	}
	return []byte(newVersion), nil
}
