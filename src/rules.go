package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// CandidateRule describes one (path-pattern, format, version-paths) tuple
// that the dispatcher can try against an input file.
//
// The dispatcher considers rules in **descending Confidence order**. When
// the rule's path pattern matches but extraction (Inspect) fails on the
// given content, the dispatcher falls through to the next matching rule.
// That's how a generic `marketplace.json` (anywhere in the tree) gets a
// chance at `.metadata.version`, but eventually falls back to `.version`
// for unrelated JSON files that happen to share the basename.
type CandidateRule struct {
	// Name is a human-readable label shown in errors / debug output.
	Name string
	// PathSuffix is matched as a clean path-suffix (slash-aware) against
	// the input path. An empty string means "match by basename only".
	PathSuffix string
	// Basename, if non-empty, requires filepath.Base(path) to equal it.
	// Used for confidence-2 rules that don't pin a directory.
	Basename string
	// Glob, if non-empty, is a basename glob like "*.json" matched as a
	// suffix. Used for the lowest-confidence fallback.
	Glob string
	// Confidence: 3 = path-pinned, 2 = basename-only, 1 = glob fallback.
	Confidence int
	// Format selects the parser/serializer pair: "json", "toml", "plain".
	Format string
	// NamePaths lists every place the rule should look for a package
	// name. Names are optional — a missing name does not cause the rule
	// to fail (unlike a missing version). Multiple paths are useful for
	// formats like `package-lock.json` that record the same name in two
	// places, where a discrepancy is itself a useful diagnostic
	// (DR-0004 cross-file name consistency picks it up).
	NamePaths []string
	// VersionPaths lists every place the rule expects a version string;
	// all of them must extract successfully for the rule to count as a hit.
	VersionPaths []string
}

// rules is the master table. Order is irrelevant for matching (the
// dispatcher sorts by Confidence), but readers should still see the
// high-confidence path-pinned rules first.
var rules = []CandidateRule{
	{
		Name:         "Claude plugin marketplace.json",
		PathSuffix:   ".claude-plugin/marketplace.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".metadata.version"},
	},
	{
		Name:         "Claude plugin plugin.json",
		PathSuffix:   ".claude-plugin/plugin.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		Name:         "package.json",
		Basename:     "package.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		Name:         "package-lock.json (npm 7+)",
		Basename:     "package-lock.json",
		Confidence:   3,
		Format:       "json",
		NamePaths:    []string{".name", `.packages[""].name`},
		VersionPaths: []string{".version", `.packages[""].version`},
	},
	{
		Name:         "marketplace.json (any directory)",
		Basename:     "marketplace.json",
		Confidence:   2,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".metadata.version"},
	},
	{
		Name:         "plugin.json (any directory)",
		Basename:     "plugin.json",
		Confidence:   2,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		Name:         "Cargo.toml",
		Basename:     "Cargo.toml",
		Confidence:   3,
		Format:       "toml",
		NamePaths:    []string{".package.name"},
		VersionPaths: []string{".package.version"},
	},
	{
		Name:       "VERSION (plain text)",
		Basename:   "VERSION",
		Confidence: 3,
		Format:     "plain",
	},
	{
		Name:         "*.json (fallback)",
		Glob:         "*.json",
		Confidence:   1,
		Format:       "json",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: top-level `.version` fallback for arbitrary YAML
		// (Helm Chart.yaml, GitHub Actions workflow metadata, etc.).
		// Multi-document YAML (`---` separators) is intentionally
		// out of scope — only the first document is examined.
		Name:         "*.yaml (fallback)",
		Glob:         "*.yaml",
		Confidence:   1,
		Format:       "yaml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: same as `*.yaml` but for the `.yml` extension
		// (carried separately because the rule table doesn't model
		// alternation in glob patterns).
		Name:         "*.yml (fallback)",
		Glob:         "*.yml",
		Confidence:   1,
		Format:       "yaml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
	{
		// DR-0011: top-level `version = "..."` fallback for arbitrary
		// TOML files. Cargo.toml's `[package].version` is handled by
		// the confidence-3 rule above; this one only catches files
		// that put `version` at top level (e.g. `pyproject.toml` with
		// the version outside `[project]`, custom manifest TOMLs).
		Name:         "*.toml (fallback)",
		Glob:         "*.toml",
		Confidence:   1,
		Format:       "toml",
		NamePaths:    []string{".name"},
		VersionPaths: []string{".version"},
	},
}

// pathMatches checks whether the rule could apply to path on its own (no
// content inspection yet).
func (r CandidateRule) pathMatches(path string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	if r.PathSuffix != "" {
		want := filepath.ToSlash(r.PathSuffix)
		return cleanPath == want || strings.HasSuffix(cleanPath, "/"+want)
	}
	if r.Basename != "" {
		return filepath.Base(path) == r.Basename
	}
	if r.Glob != "" {
		// We only use globs of the form "*.ext".
		if strings.HasPrefix(r.Glob, "*.") {
			return strings.HasSuffix(filepath.Base(path), r.Glob[1:])
		}
		matched, err := filepath.Match(r.Glob, filepath.Base(path))
		return err == nil && matched
	}
	return false
}

// rulesByConfidenceDesc returns the rules sorted by Confidence descending,
// preserving the original table order within the same confidence band.
func rulesByConfidenceDesc() []CandidateRule {
	out := make([]CandidateRule, len(rules))
	copy(out, rules)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out
}

// resolveRule walks every rule whose path-pattern matches `path`, tries
// extraction with each, and returns the first hit (highest confidence).
// If every matching rule fails extraction, the last error is wrapped and
// returned to the caller.
//
// On a successful match, the chosen rule's Confidence and (for confidence
// 1) Glob are stamped on the returned Inspection so the caller can render
// a DR-0010 fallback hint without re-resolving the rule.
func resolveRule(path string, content []byte) (CandidateRule, Inspection, error) {
	var lastErr error
	var lastRule CandidateRule
	matched := false
	for _, rule := range rulesByConfidenceDesc() {
		if !rule.pathMatches(path) {
			continue
		}
		matched = true
		insp, err := tryRule(rule, content)
		if err == nil {
			insp.MatchedConfidence = rule.Confidence
			if rule.Confidence == 1 {
				insp.MatchedGlob = rule.Glob
			}
			return rule, insp, nil
		}
		lastErr = err
		lastRule = rule
	}
	if !matched {
		return CandidateRule{}, Inspection{}, &unsupportedFileError{path: path}
	}
	return CandidateRule{}, Inspection{}, fmt.Errorf("%s: %s: %w", path, lastRule.Name, lastErr)
}

// tryRule dispatches to the format-specific Inspect implementation.
func tryRule(rule CandidateRule, content []byte) (Inspection, error) {
	switch rule.Format {
	case "json":
		return jsonInspect(rule, content)
	case "toml":
		return tomlInspect(rule, content)
	case "yaml":
		return yamlInspect(rule, content)
	case "plain":
		return plainInspect(rule, content)
	default:
		return Inspection{}, fmt.Errorf("unknown format %q in rule %q", rule.Format, rule.Name)
	}
}

func formatReplace(rule CandidateRule, content []byte, current, newVersion string) ([]byte, error) {
	switch rule.Format {
	case "json":
		return jsonReplace(rule, content, current, newVersion)
	case "toml":
		return tomlReplace(rule, content, current, newVersion)
	case "yaml":
		return yamlReplace(rule, content, current, newVersion)
	case "plain":
		return plainReplace(rule, content, current, newVersion)
	default:
		return nil, fmt.Errorf("unknown format %q in rule %q", rule.Format, rule.Name)
	}
}

// pathHasAnyRule reports whether at least one rule's path-pattern matches.
// Used by detectHandler to fail fast on unsupported file names.
func pathHasAnyRule(path string) bool {
	for _, r := range rules {
		if r.pathMatches(path) {
			return true
		}
	}
	return false
}
