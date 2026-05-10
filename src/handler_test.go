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

// TestResolveRule_FallbackYAMLAndYML pins the DR-0011 confidence-1
// fallback for both YAML extensions: a previously-unsupported
// `Chart.yaml` / `manifest.yml` resolves through the new rules.
func TestResolveRule_FallbackYAMLAndYML(t *testing.T) {
	t.Parallel()
	type tc struct {
		path    string
		want    string
		wantGlb string
	}
	cases := []tc{
		{"Chart.yaml", "*.yaml (fallback)", "*.yaml"},
		{"helm/Chart.yaml", "*.yaml (fallback)", "*.yaml"},
		{"manifest.yml", "*.yml (fallback)", "*.yml"},
		{"sub/dir/manifest.yml", "*.yml (fallback)", "*.yml"},
	}
	in := []byte("name: x\nversion: 1.2.3\n")
	for _, c := range cases {
		rule, insp, err := resolveRule(c.path, in)
		if err != nil {
			t.Errorf("resolveRule(%q) error: %v", c.path, err)
			continue
		}
		if rule.Name != c.want {
			t.Errorf("resolveRule(%q) = %q, want %q", c.path, rule.Name, c.want)
		}
		if insp.MatchedConfidence != 1 {
			t.Errorf("resolveRule(%q) MatchedConfidence = %d, want 1", c.path, insp.MatchedConfidence)
		}
		if insp.MatchedGlob != c.wantGlb {
			t.Errorf("resolveRule(%q) MatchedGlob = %q, want %q", c.path, insp.MatchedGlob, c.wantGlb)
		}
	}
}

// TestResolveRule_FallbackTOMLTopLevel checks that an arbitrary `.toml`
// file with a top-level `version = "..."` is rescued by the new
// `*.toml` fallback when no path-pinned rule applies.
func TestResolveRule_FallbackTOMLTopLevel(t *testing.T) {
	t.Parallel()
	in := []byte("name = \"x\"\nversion = \"1.2.3\"\n")
	rule, insp, err := resolveRule("manifest.toml", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "*.toml (fallback)" {
		t.Errorf("rule = %q, want fallback", rule.Name)
	}
	if insp.MatchedConfidence != 1 || insp.MatchedGlob != "*.toml" {
		t.Errorf("matched-rule metadata wrong: confidence=%d glob=%q", insp.MatchedConfidence, insp.MatchedGlob)
	}
}

// TestDetectHandler_NewFallbackExtensions extends the recognised-paths
// list to cover the DR-0011 additions (`*.yaml`, `*.yml`, `*.toml`).
func TestDetectHandler_NewFallbackExtensions(t *testing.T) {
	t.Parallel()
	good := []string{
		"Chart.yaml",
		"helm/Chart.yaml",
		"manifest.yml",
		"any.toml",
		"sub/any.toml",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
	// Still unsupported: pom.xml etc.
	bad := []string{"pom.xml"}
	for _, p := range bad {
		if _, err := detectHandler(p); err == nil {
			t.Errorf("detectHandler(%q) expected error, got nil", p)
		}
	}
}

// TestDetectHandler_RegexFormatPaths covers the DR-0012 additions:
// regex format rules attached to either a fixed basename
// (`v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt`, confidence 2)
// or a glob (`*.xcconfig` / `*.podspec` / `*.nimble` / `*.gemspec`,
// confidence 1).
func TestDetectHandler_RegexFormatPaths(t *testing.T) {
	t.Parallel()
	good := []string{
		// basename rules (confidence 2)
		"v.mod",
		"sub/v.mod",
		"build.zig.zon",
		"app/build.zig.zon",
		"mix.exs",
		"deps/mix.exs",
		"build.sbt",
		"sub/build.sbt",
		// glob rules (confidence 1)
		"Release.xcconfig",
		"configs/Debug.xcconfig",
		"MyPod.podspec",
		"Pods/MyPod.podspec",
		"foo.nimble",
		"sub/foo.nimble",
		"mygem.gemspec",
		"sub/mygem.gemspec",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
}

// TestDetectHandler_PyProjectAndMojoProject covers the DR-0014 path-pinned
// confidence-3 rules. Like Cargo.toml, both basenames register at any
// depth in the tree.
func TestDetectHandler_PyProjectAndMojoProject(t *testing.T) {
	t.Parallel()
	good := []string{
		"pyproject.toml",
		"sub/pyproject.toml",
		"deeply/nested/pyproject.toml",
		"mojoproject.toml",
		"app/mojoproject.toml",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
}

// TestResolveRule_PyProjectPrecedence pins the rule selection: the
// path-pinned `pyproject.toml` rule (confidence 3) wins for any
// pyproject.toml that has a parseable `[project].version` or
// `[tool.poetry].version`.
func TestResolveRule_PyProjectPrecedence(t *testing.T) {
	t.Parallel()
	type tc struct {
		path    string
		content string
		want    string // CandidateRule.Name
	}
	cases := []tc{
		// PEP 621
		{"pyproject.toml", "[project]\nname = \"x\"\nversion = \"1.2.3\"\n", "pyproject.toml"},
		// Poetry legacy
		{"pyproject.toml", "[tool.poetry]\nname = \"x\"\nversion = \"1.2.3\"\n", "pyproject.toml"},
		// Both present (PEP 621 wins inside the rule, but the rule itself is the same)
		{"sub/pyproject.toml", "[project]\nname=\"x\"\nversion=\"1.2.3\"\n[tool.poetry]\nname=\"x\"\nversion=\"1.2.3\"\n", "pyproject.toml"},
		// mojoproject
		{"mojoproject.toml", "[workspace]\nname = \"m\"\nversion = \"1.2.3\"\n", "mojoproject.toml"},
		{"app/mojoproject.toml", "[workspace]\nname = \"m\"\nversion = \"1.2.3\"\n", "mojoproject.toml"},
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
		if rule.Confidence != 3 {
			t.Errorf("resolveRule(%q).Confidence = %d, want 3", c.path, rule.Confidence)
		}
	}
}

// TestResolveRule_PyProjectFallthroughToTopLevel guards the fallback
// chain: a pyproject.toml whose version is NOT in `[project]` /
// `[tool.poetry]` (e.g. the user has it at top level for whatever
// reason) falls through to the DR-0011 `*.toml` fallback rule.
func TestResolveRule_PyProjectFallthroughToTopLevel(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "x"
version = "1.2.3"
`)
	rule, insp, err := resolveRule("pyproject.toml", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "*.toml (fallback)" {
		t.Errorf("rule = %q, want *.toml fallback", rule.Name)
	}
	if insp.MatchedConfidence != 1 {
		t.Errorf("MatchedConfidence = %d, want 1", insp.MatchedConfidence)
	}
}

// TestDetectHandler_PbxprojAndInfoPlist covers the DR-0015 path-pinned
// confidence-3 rules. Both basenames register at any depth in the
// tree (Xcode bundles them inside `<project>.xcodeproj/`).
func TestDetectHandler_PbxprojAndInfoPlist(t *testing.T) {
	t.Parallel()
	good := []string{
		"project.pbxproj",
		"App.xcodeproj/project.pbxproj",
		"deeply/nested/App.xcodeproj/project.pbxproj",
		"Info.plist",
		"App/Info.plist",
		"deeply/nested/Info.plist",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
}

// TestResolveRule_PbxprojAndInfoPlistPrecedence pins the rule
// selection: both basenames resolve to their dedicated confidence-3
// rule rather than falling through to a generic format.
func TestResolveRule_PbxprojAndInfoPlistPrecedence(t *testing.T) {
	t.Parallel()
	type tc struct {
		path    string
		content string
		want    string // CandidateRule.Name
	}
	cases := []tc{
		{
			"project.pbxproj",
			"\t\tABC = {\n\t\t\tMARKETING_VERSION = 1.2.3;\n\t\t};\n",
			"project.pbxproj",
		},
		{
			"App.xcodeproj/project.pbxproj",
			"MARKETING_VERSION = 1.2.3;\n",
			"project.pbxproj",
		},
		{
			"Info.plist",
			`<?xml version="1.0"?><plist><dict><key>CFBundleShortVersionString</key><string>1.2.3</string></dict></plist>`,
			"Info.plist",
		},
		{
			"App/Info.plist",
			`<plist><dict><key>CFBundleShortVersionString</key><string>1.2.3</string></dict></plist>`,
			"Info.plist",
		},
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
		if rule.Confidence != 3 {
			t.Errorf("resolveRule(%q).Confidence = %d, want 3", c.path, rule.Confidence)
		}
	}
}

// TestResolveRule_PbxprojMismatchSurfaces: when an inconsistent
// pbxproj is read end-to-end through `tryRun`, the dispatcher emits
// a `version mismatch:` error sourced from main.go's existing
// column-aligned formatter. The mismatch is detected at the
// per-input layer (inside resolveFile) before any cross-file logic
// kicks in.
func TestResolveRule_PbxprojMismatchSurfaces(t *testing.T) {
	t.Parallel()
	dir := tempWriteFiles(t, map[string]string{
		"project.pbxproj": "MARKETING_VERSION = 1.2.3;\nMARKETING_VERSION = 1.2.4;\n",
	})
	err := tryRun("get", dir+"/project.pbxproj")
	if err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "version mismatch:") {
		t.Errorf("error does not mention 'version mismatch:': %v", err)
	}
	if !strings.Contains(err.Error(), "line:") {
		t.Errorf("mismatch labels missing 'line:' annotation: %v", err)
	}
}

// TestResolveRule_RegexFormatConfidence pins the confidence band of
// every DR-0012 rule so the dispatcher's hint emission picks the
// right glob name.
func TestResolveRule_RegexFormatConfidence(t *testing.T) {
	t.Parallel()
	type tc struct {
		path    string
		content string
		want    string // CandidateRule.Name
		conf    int
	}
	cases := []tc{
		{"v.mod", "Module {\n\tversion: '1.2.3'\n}\n", "v.mod", 2},
		{"build.zig.zon", ".{\n    .version = \"1.2.3\",\n}\n", "build.zig.zon", 2},
		{"mix.exs", "    version: \"1.2.3\",\n", "mix.exs", 2},
		{"build.sbt", "version := \"1.2.3\"\n", "build.sbt", 2},
		{"Release.xcconfig", "MARKETING_VERSION = 1.2.3\n", "*.xcconfig (fallback)", 1},
		{"MyPod.podspec", "s.version = '1.2.3'\n", "*.podspec (fallback)", 1},
		{"foo.nimble", "version = \"1.2.3\"\n", "*.nimble (fallback)", 1},
		{"mygem.gemspec", "s.version = \"1.2.3\"\n", "*.gemspec (fallback)", 1},
	}
	for _, c := range cases {
		rule, insp, err := resolveRule(c.path, []byte(c.content))
		if err != nil {
			t.Errorf("resolveRule(%q) error: %v", c.path, err)
			continue
		}
		if rule.Name != c.want {
			t.Errorf("resolveRule(%q) = %q, want %q", c.path, rule.Name, c.want)
		}
		if rule.Confidence != c.conf {
			t.Errorf("resolveRule(%q).Confidence = %d, want %d", c.path, rule.Confidence, c.conf)
		}
		if c.conf == 1 {
			if insp.MatchedConfidence != 1 || insp.MatchedGlob == "" {
				t.Errorf("resolveRule(%q) confidence-1 metadata missing: %+v", c.path, insp)
			}
		}
	}
}
