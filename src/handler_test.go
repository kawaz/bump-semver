package main

import (
	"strings"
	"testing"
)

// pathHasAnyRule is the cheap "this path is supported by *some* rule"
// check. detectHandler defers content-driven decisions to Inspect, so the
// gate at this layer is just "no rule could ever match" (e.g. README.md).
func TestDetectHandler_RecognisedPaths(t *testing.T) {
	t.Parallel()
	good := []string{
		"Cargo.toml",
		"path/to/Cargo.toml",
		"VERSION",
		"sub/VERSION",
		"package.json",
		".claude-plugin/plugin.json",
		".claude-plugin/marketplace.json",
		"sub/.claude-plugin/plugin.json",
		"some/dir/marketplace.json",
		"some/dir/plugin.json",
		"any.json",
		"package-lock.json",
		"moon.mod.json",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
	bad := []string{"README.md", "src/main.go", "Version", "version"}
	for _, p := range bad {
		if _, err := detectHandler(p); err == nil {
			t.Errorf("detectHandler(%q) expected error, got nil", p)
		}
	}
}

// TestResolveRule_Precedence pins the confidence ordering: a path that
// matches a confidence-3 path-suffix rule resolves to that rule, not to
// the generic basename rule below it.
func TestResolveRule_Precedence(t *testing.T) {
	t.Parallel()
	type tc struct {
		path    string
		content string
		want    string // CandidateRule.Name
	}
	cases := []tc{
		{".claude-plugin/marketplace.json", `{"name":"x","version":"1.0.0","metadata":{"version":"1.2.3"}}`, "Claude plugin marketplace.json"},
		{".claude-plugin/plugin.json", `{"name":"x","version":"1.2.3"}`, "Claude plugin plugin.json"},
		{"any/dir/marketplace.json", `{"name":"x","metadata":{"version":"1.2.3"}}`, "marketplace.json (any directory)"},
		{"any/dir/plugin.json", `{"name":"x","version":"1.2.3"}`, "plugin.json (any directory)"},
		{"package.json", `{"name":"x","version":"1.2.3"}`, "package.json"},
		{"package-lock.json", `{"name":"x","version":"1.2.3","lockfileVersion":3,"packages":{"":{"name":"x","version":"1.2.3"}}}`, "package-lock.json (npm 7+)"},
		{"Cargo.toml", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml"},
		{"VERSION", "1.2.3\n", "VERSION (plain text)"},
		{"some-other.json", `{"version":"1.2.3"}`, "*.json (fallback)"},
	}
	for _, c := range cases {
		rule, _, err := resolveRule(c.path, []byte(c.content))
		if err != nil {
			t.Errorf("resolveRule(%q) error: %v", c.path, err)
			continue
		}
		if rule.Name != c.want {
			t.Errorf("resolveRule(%q) = %q, want %q", c.path, rule.Name, c.want)
		}
	}
}

// TestResolveRule_FallbackOnMissingMetadata exercises the try → fallback
// path: a `marketplace.json` (basename match, confidence 2) that lacks
// `.metadata.version` should still resolve to the `*.json` fallback rule
// when only `.version` is present.
func TestResolveRule_FallbackOnMissingMetadata(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name":"x","version":"1.2.3"}`)
	rule, insp, err := resolveRule("any/dir/marketplace.json", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "*.json (fallback)" {
		t.Errorf("rule = %q, want fallback", rule.Name)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want one $.version=1.2.3", insp.Versions)
	}
}

// TestResolveRule_AllCandidatesFail returns the deepest extraction error
// (with rule name + path context) when no rule produces a hit.
func TestResolveRule_AllCandidatesFail(t *testing.T) {
	t.Parallel()
	in := []byte(`{"name":"x"}`) // no version anywhere
	_, _, err := resolveRule("dir/marketplace.json", in)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "marketplace.json") {
		t.Errorf("error should mention path: %v", err)
	}
}
