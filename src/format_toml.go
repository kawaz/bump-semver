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
// become `[workspace.package].version`. A single-segment path
// (`.version`, used by the DR-0011 top-level TOML fallback) renders as
// just `version` — TOML's natural form for a top-level key.
func tomlDisplayPath(p string) string {
	segs, err := parseJSONPath(p)
	if err != nil {
		return p
	}
	if len(segs) == 1 {
		return segs[0]
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

// tomlReplace dispatches on the rule's VersionPath: the path-pinned
// Cargo rule (`.package.version`) drives the section-aware rewriter
// below; the DR-0011 top-level fallback (`.version`) drives the
// section-less rewriter that touches the file's pre-section region.
//
// In both branches the original quoting style (single or double
// quotes) and any trailing comment on the version line are preserved.
var (
	cargoPackageHeaderRe = regexp.MustCompile(`(?m)^\s*\[package\]\s*$`)
	cargoSectionStartRe  = regexp.MustCompile(`(?m)^\s*\[`)
	cargoVersionLineRe   = regexp.MustCompile(`(?m)^(\s*version\s*=\s*)(["'])([^"']*)(["'])`)
)

func tomlReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) != 1 {
		return nil, fmt.Errorf("TOML format currently supports a single version path per rule (got %v)", rule.VersionPaths)
	}
	switch rule.VersionPaths[0] {
	case ".package.version":
		return tomlReplaceCargoPackage(content, newVersion)
	case ".version":
		return tomlReplaceTopLevel(content, newVersion)
	default:
		return nil, fmt.Errorf("TOML format does not yet support version path %q", rule.VersionPaths[0])
	}
}

// tomlReplaceCargoPackage rewrites `[package].version` while leaving
// every other section (including `[dependencies]` entries that carry
// their own `version = ...`) untouched.
func tomlReplaceCargoPackage(content []byte, newVersion string) ([]byte, error) {
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

// tomlReplaceTopLevel rewrites the top-level `version = "..."` line
// (DR-0011). The "top-level region" is everything before the first
// `[section]` header — keys that come after a section header belong
// to that section in TOML semantics, so the regex must not stray
// into them.
func tomlReplaceTopLevel(content []byte, newVersion string) ([]byte, error) {
	region := content
	if loc := cargoSectionStartRe.FindIndex(content); loc != nil {
		region = content[:loc[0]]
	}
	loc := cargoVersionLineRe.FindSubmatchIndex(region)
	if loc == nil {
		return nil, fmt.Errorf("missing top-level version line")
	}
	out := make([]byte, 0, len(content)+len(newVersion))
	out = append(out, region[:loc[2]]...)       // before "version ="
	out = append(out, region[loc[2]:loc[3]]...) // "version = " verbatim
	out = append(out, region[loc[4]:loc[5]]...) // opening quote
	out = append(out, newVersion...)
	out = append(out, region[loc[8]:loc[9]]...) // closing quote
	out = append(out, region[loc[1]:]...)
	out = append(out, content[len(region):]...)
	return out, nil
}
