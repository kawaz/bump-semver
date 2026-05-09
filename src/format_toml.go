package main

import (
	"fmt"
	"regexp"

	"github.com/BurntSushi/toml"
)

// tomlInspect parses content as TOML and extracts the version / name
// fields described by the rule. Path syntax is the same jq-style dot path
// as the JSON handler — `.package.version` for what TOML displays as
// `[package].version`. Display strings flip the syntax back on output so
// users see TOML-native paths (e.g. `[package].version`) in errors.
func tomlInspect(rule CandidateRule, content []byte) (Inspection, error) {
	var doc map[string]interface{}
	if err := toml.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse TOML: %w", err)
	}
	insp := Inspection{}
	for _, vp := range rule.VersionPaths {
		val, found, err := jsonPathExtract(doc, vp)
		if err != nil {
			return Inspection{}, err
		}
		if !found {
			return Inspection{}, fmt.Errorf("missing %s", tomlDisplayPath(vp))
		}
		if val == "" {
			return Inspection{}, fmt.Errorf("empty %s", tomlDisplayPath(vp))
		}
		insp.Versions = append(insp.Versions, Field{Value: val, Path: tomlDisplayPath(vp)})
	}
	for _, np := range rule.NamePaths {
		val, found, _ := jsonPathExtract(doc, np)
		if found && val != "" {
			insp.Names = append(insp.Names, Field{Value: val, Path: tomlDisplayPath(np)})
		}
	}
	return insp, nil
}

// tomlDisplayPath converts an internal jq-style path (`.package.version`)
// into the TOML-native display form (`[package].version`).
//
// The first segment becomes a `[section]` header; remaining segments are
// joined with `.`. Multi-section paths (`.workspace.package.version`)
// become `[workspace.package].version`.
func tomlDisplayPath(p string) string {
	segs, err := parseJSONPath(p)
	if err != nil || len(segs) < 2 {
		return p
	}
	sectionEnd := len(segs) - 1
	return "[" + joinDot(segs[:sectionEnd]) + "]." + segs[sectionEnd]
}

func joinDot(segs []string) string {
	out := ""
	for i, s := range segs {
		if i > 0 {
			out += "."
		}
		out += s
	}
	return out
}

// tomlReplace handles only the Cargo-style `[package].version` rewrite.
// More general TOML path rewriting can be added when concretely needed.
//
// The implementation preserves the original quoting style (single or
// double quotes) and any trailing comment on the version line.
var (
	cargoPackageHeaderRe = regexp.MustCompile(`(?m)^\s*\[package\]\s*$`)
	cargoSectionStartRe  = regexp.MustCompile(`(?m)^\s*\[`)
	cargoVersionLineRe   = regexp.MustCompile(`(?m)^(\s*version\s*=\s*)(["'])([^"']*)(["'])`)
)

func tomlReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) != 1 || rule.VersionPaths[0] != ".package.version" {
		return nil, fmt.Errorf("TOML format currently supports only [package].version (got %v)", rule.VersionPaths)
	}
	hdr := cargoPackageHeaderRe.FindIndex(content)
	if hdr == nil {
		return nil, fmt.Errorf("Cargo.toml: missing [package] section")
	}
	sectionStart := hdr[1]
	sectionEnd := len(content)
	if loc := cargoSectionStartRe.FindIndex(content[sectionStart:]); loc != nil {
		sectionEnd = sectionStart + loc[0]
	}
	section := content[sectionStart:sectionEnd]
	loc := cargoVersionLineRe.FindSubmatchIndex(section)
	if loc == nil {
		return nil, fmt.Errorf("Cargo.toml: missing [package].version line")
	}
	out := make([]byte, 0, len(content)+len(newVersion))
	out = append(out, content[:sectionStart]...)
	out = append(out, section[:loc[2]]...)       // before "version ="
	out = append(out, section[loc[2]:loc[3]]...) // "version = " verbatim
	out = append(out, section[loc[4]:loc[5]]...) // opening quote
	out = append(out, newVersion...)
	out = append(out, section[loc[8]:loc[9]]...) // closing quote
	out = append(out, section[loc[1]:]...)
	out = append(out, content[sectionEnd:]...)
	return out, nil
}
