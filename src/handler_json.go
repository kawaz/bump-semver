package main

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type jsonHandler struct{}

func (jsonHandler) Get(content []byte) (string, error) {
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return "", fmt.Errorf("parse JSON: %w", err)
	}
	if doc.Version == "" {
		return "", fmt.Errorf("JSON: missing top-level \"version\"")
	}
	return doc.Version, nil
}

func (jsonHandler) Replace(content []byte, newVersion string) ([]byte, error) {
	cur, err := (jsonHandler{}).Get(content)
	if err != nil {
		return nil, err
	}
	// Anchor on the literal "version": "<cur>" pattern. Using the current
	// value as part of the regex disambiguates the top-level version from
	// nested "version" keys whose value happens to differ.
	re := regexp.MustCompile(`("version"\s*:\s*)"` + regexp.QuoteMeta(cur) + `"`)
	matches := re.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("JSON: cannot locate \"version\" line in source")
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("JSON: multiple \"version\": %q occurrences; cannot disambiguate top-level version", cur)
	}
	m := matches[0]
	out := make([]byte, 0, len(content)+len(newVersion))
	out = append(out, content[:m[2]]...)
	out = append(out, content[m[2]:m[3]]...) // "version" + spaces + : + spaces
	out = append(out, '"')
	out = append(out, newVersion...)
	out = append(out, '"')
	out = append(out, content[m[1]:]...)
	return out, nil
}
