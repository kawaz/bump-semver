package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// jsonInspect parses content as a JSON document and extracts the version /
// name fields described by the rule. Returns an Inspection on success;
// returns an error on the first missing / non-string version field so the
// dispatcher can fall back to the next rule.
func jsonInspect(rule CandidateRule, content []byte) (Inspection, error) {
	var doc interface{}
	if err := json.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse JSON: %w", err)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		val, found, err := jsonPathExtract(doc, vp)
		if err != nil {
			return Inspection{}, err
		}
		if !found {
			return Inspection{}, fmt.Errorf("missing %s", "$"+vp)
		}
		if val == "" {
			return Inspection{}, fmt.Errorf("empty %s", "$"+vp)
		}
		insp.Versions = append(insp.Versions, Field{Value: val, Path: "$" + vp})
	}
	for _, np := range rule.NamePaths {
		val, found, _ := jsonPathExtract(doc, np)
		if found && val != "" {
			insp.Names = append(insp.Names, Field{Value: val, Path: "$" + np})
		}
	}
	return insp, nil
}

// jsonReplace rewrites every version path described by rule to newVersion.
// Locations are resolved by streaming the document with a json.Decoder, so
// e.g. `package-lock.json`'s `$.packages["node_modules/..."]` entries are
// never touched even when their version happens to equal `current`.
func jsonReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	var offsets []jsonOffset
	for _, vp := range rule.VersionPaths {
		off, found, err := locateJSONPath(content, vp)
		if err != nil {
			return nil, fmt.Errorf("locate %s: %w", "$"+vp, err)
		}
		if !found {
			return nil, fmt.Errorf("cannot locate %s in source", "$"+vp)
		}
		offsets = append(offsets, off)
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
