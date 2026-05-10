package main

import (
	"strings"
	"testing"
)

// --- DR-0011: top-level `*.toml` confidence-1 fallback --------------------

// TestTomlInspect_TopLevel_PyProjectStyle exercises the top-level
// fallback against a pyproject.toml-shaped file whose version sits
// outside `[project]` (e.g. tooling that pins it at root rather than
// under `[project]`). Files with version inside `[project]` are out
// of scope for v0.8.0; see DR-0011 § 4.
func TestTomlInspect_TopLevel_PyProjectStyle(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "my-pkg"
version = "1.2.3"

[tool.something]
foo = "bar"
`)
	insp, err := inspectVia("manifest.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "version" {
		t.Errorf("Versions = %+v, want one version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "my-pkg" || insp.Names[0].Path != "name" {
		t.Errorf("Names = %+v, want one name=my-pkg", insp.Names)
	}
}

// TestTomlInspect_TopLevel_CargoStillUsesPathPinned guards against
// regression: the path-pinned `Cargo.toml` rule (confidence 3) must
// continue to win, never the new top-level fallback.
func TestTomlInspect_TopLevel_CargoStillUsesPathPinned(t *testing.T) {
	t.Parallel()
	in := []byte(`[package]
name = "foo"
version = "1.2.3"
`)
	rule, _, err := resolveRule("Cargo.toml", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "Cargo.toml" {
		t.Errorf("path-pinned Cargo.toml rule must win, got %q", rule.Name)
	}
}

// TestTomlInspect_TopLevel_FallsBackForCargoWorkspaceWithTopLevel
// covers a case where Cargo.toml has no `[package]` but does have a
// stray top-level version: the path-pinned rule fails, but the
// confidence-1 fallback still rescues the read. This isn't a real
// Cargo layout — it's just exercising the fallthrough mechanics.
func TestTomlInspect_TopLevel_FallsBackForFileWithoutPackage(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "foo"
version = "9.9.9"
`)
	rule, insp, err := resolveRule("manifest.toml", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "*.toml (fallback)" {
		t.Errorf("rule = %q, want fallback", rule.Name)
	}
	if rule.Confidence != 1 {
		t.Errorf("Confidence = %d, want 1", rule.Confidence)
	}
	if rule.Glob != "*.toml" {
		t.Errorf("Glob = %q, want *.toml", rule.Glob)
	}
	if insp.MatchedConfidence != 1 || insp.MatchedGlob != "*.toml" {
		t.Errorf("Inspection didn't carry matched-rule metadata: %+v", insp)
	}
	if insp.Versions[0].Value != "9.9.9" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

// TestTomlInspect_TopLevel_RejectsSectionedVersion verifies the
// fallback does NOT reach into `[project] version = ...`. That kind of
// section-scoped version is the responsibility of a future
// `pyproject.toml` path-pinned rule (DR-0011 § 4).
func TestTomlInspect_TopLevel_RejectsSectionedVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "x"
version = "1.2.3"
`)
	if _, err := inspectVia("manifest.toml", in); err == nil {
		t.Error("top-level fallback must not match [project].version")
	}
}

// TestTomlReplace_TopLevel_Unquoted bumps a top-level version preserving
// the surrounding lines.
func TestTomlReplace_TopLevel_DoubleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "my-pkg"
version = "1.2.3"

[tool.x]
foo = "bar"
`)
	out, err := replaceVia("manifest.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "1.2.4"`) {
		t.Errorf("top-level version not bumped:\n%s", s)
	}
	if !strings.Contains(s, `name = "my-pkg"`) {
		t.Errorf("name line lost:\n%s", s)
	}
	if !strings.Contains(s, `[tool.x]`) {
		t.Errorf("section header lost:\n%s", s)
	}
}

// TestTomlReplace_TopLevel_SingleQuoted preserves single quotes.
func TestTomlReplace_TopLevel_SingleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte("version = '1.2.3'\nname = \"foo\"\n")
	out, err := replaceVia("manifest.toml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version = '2.0.0'") {
		t.Errorf("single-quote style not preserved:\n%s", string(out))
	}
}

// TestTomlReplace_TopLevel_PreservesTrailingComment keeps an inline
// `# comment` after the version assignment.
func TestTomlReplace_TopLevel_PreservesTrailingComment(t *testing.T) {
	t.Parallel()
	in := []byte("version = \"1.2.3\"  # bumped weekly\nname = \"foo\"\n")
	out, err := replaceVia("manifest.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version = "1.2.4"  # bumped weekly`) {
		t.Errorf("trailing comment not preserved:\n%s", string(out))
	}
}

// TestTomlReplace_TopLevel_StaysOutOfSections is the regression guard:
// a `version = ...` line under a section must stay untouched, since
// the fallback only owns the pre-section region.
func TestTomlReplace_TopLevel_StaysOutOfSections(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "my-pkg"
version = "1.2.3"

[deps]
version = "9.9.9"
`)
	out, err := replaceVia("manifest.toml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "2.0.0"`) {
		t.Errorf("top-level version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[deps]\nversion = \"9.9.9\"") {
		t.Errorf("section-scoped version was modified:\n%s", s)
	}
}
