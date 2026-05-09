package main

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type jsonHandler struct{}

func (jsonHandler) Inspect(content []byte) (Inspection, error) {
	var doc struct {
		Version string `json:"version"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse JSON: %w", err)
	}
	if doc.Version == "" {
		return Inspection{}, fmt.Errorf("JSON: missing top-level \"version\"")
	}
	insp := Inspection{
		Versions: []Field{{Value: doc.Version, Path: "$.version"}},
	}
	if doc.Name != "" {
		insp.Names = append(insp.Names, Field{Value: doc.Name, Path: "$.name"})
	}
	return insp, nil
}

func (jsonHandler) Replace(content []byte, current, newVersion string) ([]byte, error) {
	// Anchor on the literal "version": "<current>" pattern. Using the current
	// value as part of the regex disambiguates the top-level version from
	// nested "version" keys whose value happens to differ.
	re := regexp.MustCompile(`("version"\s*:\s*)"` + regexp.QuoteMeta(current) + `"`)
	matches := re.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("JSON: cannot locate \"version\" line in source")
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("JSON: multiple \"version\": %q occurrences; cannot disambiguate top-level version", current)
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
