package main

import (
	"fmt"
	"regexp"

	"github.com/BurntSushi/toml"
)

type cargoHandler struct{}

func (cargoHandler) Get(content []byte) (string, error) {
	var doc struct {
		Package struct {
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(content, &doc); err != nil {
		return "", fmt.Errorf("parse Cargo.toml: %w", err)
	}
	if doc.Package.Version == "" {
		return "", fmt.Errorf("Cargo.toml: missing [package].version")
	}
	return doc.Package.Version, nil
}

var (
	cargoPackageHeaderRe = regexp.MustCompile(`(?m)^\s*\[package\]\s*$`)
	cargoSectionStartRe  = regexp.MustCompile(`(?m)^\s*\[`)
	// Match: version = "x.y.z" within the [package] section. The value can
	// be quoted with " or ' (TOML allows both); we keep the same quote on output.
	cargoVersionLineRe = regexp.MustCompile(`(?m)^(\s*version\s*=\s*)(["'])([^"']*)(["'])`)
)

func (cargoHandler) Replace(content []byte, newVersion string) ([]byte, error) {
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
	// loc indices: [matchS, matchE, g1S, g1E, g2S, g2E, g3S, g3E, g4S, g4E]
	out := make([]byte, 0, len(content)+len(newVersion))
	out = append(out, content[:sectionStart]...)
	out = append(out, section[:loc[2]]...)       // before group1 (line up to "version =")
	out = append(out, section[loc[2]:loc[3]]...) // group1 verbatim
	out = append(out, section[loc[4]:loc[5]]...) // opening quote
	out = append(out, newVersion...)
	out = append(out, section[loc[8]:loc[9]]...) // closing quote
	out = append(out, section[loc[1]:]...)
	out = append(out, content[sectionEnd:]...)
	return out, nil
}
