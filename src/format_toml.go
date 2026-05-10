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
//
// VersionPaths semantics: **OR / first-match-wins** (DR-0014).
// Unlike JSON (`package-lock.json`) where every VersionPath must
// extract successfully (AND, used as a self-consistency check), TOML
// rules treat VersionPaths as a list of candidates to try in order.
// The first path that resolves to a non-empty string wins, and that
// is the only Field returned. This is what powers the
// `pyproject.toml` rule's `[project].version` → `[tool.poetry].version`
// fallback (PEP 621 first, Poetry-legacy second). Single-path TOML
// rules (Cargo.toml's `.package.version`, the DR-0011 top-level
// fallback's `.version`) behave identically under both semantics.
//
// NamePaths follow the same OR rule. Names are advisory (a missing
// name does not fail the rule), so the first path that yields a
// non-empty string is reported.
func tomlInspect(rule CandidateRule, content []byte) (Inspection, error) {
	var doc map[string]interface{}
	if err := toml.Unmarshal(content, &doc); err != nil {
		return Inspection{}, fmt.Errorf("parse TOML: %w", err)
	}
	insp := Inspection{}
	var lastVersionErr error
	hit := false
	for _, vp := range rule.VersionPaths {
		val, found, err := jsonPathExtract(doc, vp)
		if err != nil {
			// A non-string at the leaf is a hard error for that
			// path; remember it so a final all-paths-failed report
			// can surface the most informative message.
			lastVersionErr = err
			continue
		}
		if !found {
			lastVersionErr = fmt.Errorf("missing %s", tomlDisplayPath(vp))
			continue
		}
		if val == "" {
			lastVersionErr = fmt.Errorf("empty %s", tomlDisplayPath(vp))
			continue
		}
		insp.Versions = append(insp.Versions, Field{Value: val, Path: tomlDisplayPath(vp)})
		hit = true
		break
	}
	if !hit {
		if lastVersionErr == nil {
			// VersionPaths empty (shouldn't happen for well-formed
			// rules), but be defensive.
			lastVersionErr = fmt.Errorf("no version path configured")
		}
		return Inspection{}, lastVersionErr
	}
	for _, np := range rule.NamePaths {
		val, found, err := jsonPathExtract(doc, np)
		if err != nil {
			continue
		}
		if found && val != "" {
			insp.Names = append(insp.Names, Field{Value: val, Path: tomlDisplayPath(np)})
			break
		}
	}
	return insp, nil
}

// tomlDisplayPath converts an internal jq-style path (`.package.version`)
// into the TOML-native display form (`[package].version`).
//
// The first segment becomes a `[section]` header; remaining segments are
// joined with `.`. Multi-section paths (`.tool.poetry.version`)
// become `[tool.poetry].version`. A single-segment path
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

// tomlSectionPathFromVersionPath converts an internal jq-style version
// path (`.package.version` / `.tool.poetry.version` / `.version`) into
// the dotted section name expected by `tomlReplaceInSection`
// (`"package"` / `"tool.poetry"` / `""`). The leaf segment (the field
// name itself, always `version`) is dropped; the remaining segments
// are joined with `.`. A single-segment path yields the empty string,
// signalling top-level (pre-section) territory.
func tomlSectionPathFromVersionPath(p string) (string, error) {
	segs, err := parseJSONPath(p)
	if err != nil {
		return "", err
	}
	if len(segs) == 0 {
		return "", fmt.Errorf("empty version path")
	}
	if len(segs) == 1 {
		return "", nil
	}
	return joinDot(segs[:len(segs)-1]), nil
}

// --- section-scoped Replace (DR-0014) -----------------------------------
//
// The rewriter is line-anchored regex against a slice of `content` that
// covers exactly the chosen section's body. This preserves comments,
// blank lines, key ordering, and the original quoting style — the same
// trade-off DR-0011 made for the YAML / TOML top-level fallback. A
// proper TOML AST round-trip via BurntSushi/toml would lose every one
// of those properties.
//
// Section detection is intentionally strict (`^\s*\[<path>\]\s*$`):
// only a header on its own line at column-leading whitespace counts.
// Inline tables (`foo = { version = "..." }`) and dotted-key forms
// (`tool.poetry.version = "..."` written without a header) are out of
// scope; if they ever surface as a real-world need, a separate DR can
// extend this regex.
var (
	tomlSectionStartRe = regexp.MustCompile(`(?m)^\s*\[`)
	tomlVersionLineRe  = regexp.MustCompile(`(?m)^(\s*version\s*=\s*)(["'])([^"']*)(["'])`)
)

// tomlSectionHeaderRe builds a regex that matches the section header
// for `sectionPath` (e.g. `"package"`, `"project"`, `"tool.poetry"`).
// The dot inside `tool.poetry` is treated as a TOML path separator,
// so it is escaped in the regex. Anchoring is `(?m)^\s*\[<path>\]\s*$`
// to keep the existing strictness used by DR-0011.
func tomlSectionHeaderRe(sectionPath string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*\[` + regexp.QuoteMeta(sectionPath) + `\]\s*$`)
}

// tomlReplaceInSection rewrites the `version = "..."` line that lives
// inside the section identified by sectionPath. An empty sectionPath
// targets the top-level (pre-section) region — that's how DR-0011's
// `*.toml` fallback case is expressed in this generalised helper.
//
// Behaviour summary:
//
//   - sectionPath == ""    → rewrite version line in the region BEFORE
//     the first `[...]` header. Errors if no top-level version line.
//   - sectionPath != ""    → find `[<sectionPath>]` header on its own
//     line; rewrite version line in the region between that header
//     and the next `[...]` header (or EOF). Errors if the section is
//     missing OR if the section has no version line.
//
// The original quoting style (single vs. double) and any trailing
// inline comment on the version line are preserved verbatim, just like
// the pre-DR-0014 hard-coded rewriters did.
func tomlReplaceInSection(content []byte, sectionPath, newVersion string) ([]byte, error) {
	if sectionPath == "" {
		return tomlReplaceTopLevelRegion(content, newVersion)
	}
	headerRe := tomlSectionHeaderRe(sectionPath)
	hdr := headerRe.FindIndex(content)
	if hdr == nil {
		return nil, fmt.Errorf("missing [%s] section", sectionPath)
	}
	sectionStart := hdr[1]
	sectionEnd := len(content)
	if loc := tomlSectionStartRe.FindIndex(content[sectionStart:]); loc != nil {
		sectionEnd = sectionStart + loc[0]
	}
	section := content[sectionStart:sectionEnd]
	loc := tomlVersionLineRe.FindSubmatchIndex(section)
	if loc == nil {
		return nil, fmt.Errorf("missing [%s].version line", sectionPath)
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

// tomlReplaceTopLevelRegion is the sectionPath == "" branch of
// tomlReplaceInSection (DR-0011 top-level `version = "..."` fallback).
// Kept as its own function so the offset arithmetic mirrors
// the section case structurally without one branch shadowing the
// other.
func tomlReplaceTopLevelRegion(content []byte, newVersion string) ([]byte, error) {
	region := content
	if loc := tomlSectionStartRe.FindIndex(content); loc != nil {
		region = content[:loc[0]]
	}
	loc := tomlVersionLineRe.FindSubmatchIndex(region)
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

// tomlReplace dispatches by re-running Inspect to find which of the
// rule's VersionPaths actually matched the document. The hit path is
// translated into a section path (drop the leaf, dot-join the rest)
// and handed to tomlReplaceInSection.
//
// Re-Inspecting on every Replace looks redundant but is cheap (TOML
// parsing is microseconds for the file sizes we care about) and keeps
// Replace stateless: ruleHandler does not have to thread a "matched
// path index" alongside the Inspection through to Replace.
//
// Files that carry the version in BOTH `[project]` and `[tool.poetry]`
// (theoretical PEP 621 migration mid-state) intentionally have only
// the first match rewritten — DR-0014 § 6 documents the trade-off.
func tomlReplace(rule CandidateRule, content []byte, _ /* current */, newVersion string) ([]byte, error) {
	if len(rule.VersionPaths) == 0 {
		return nil, fmt.Errorf("TOML rule %q has no VersionPath", rule.Name)
	}
	var doc map[string]interface{}
	if err := toml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	for _, vp := range rule.VersionPaths {
		val, found, err := jsonPathExtract(doc, vp)
		if err != nil || !found || val == "" {
			continue
		}
		section, err := tomlSectionPathFromVersionPath(vp)
		if err != nil {
			return nil, err
		}
		return tomlReplaceInSection(content, section, newVersion)
	}
	return nil, fmt.Errorf("TOML rule %q: no VersionPath matched on Replace", rule.Name)
}
