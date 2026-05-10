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

// --- DR-0014: section-scoped Replace + pyproject.toml / mojoproject.toml -----

// TestTomlInspect_PyProject_PEP621Wins reads `[project].version` and
// `[project].name` from a PEP 621 pyproject.toml (the modern shape).
func TestTomlInspect_PyProject_PEP621Wins(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "my-pkg"
version = "1.2.3"
description = "x"
`)
	insp, err := inspectVia("pyproject.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "[project].version" {
		t.Errorf("Versions = %+v, want one [project].version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Value != "my-pkg" || insp.Names[0].Path != "[project].name" {
		t.Errorf("Names = %+v, want one [project].name=my-pkg", insp.Names)
	}
}

// TestTomlInspect_PyProject_PoetryFallback uses the Poetry-legacy
// `[tool.poetry]` section when `[project]` is absent. The TOML format's
// OR semantics let one rule cover both shapes.
func TestTomlInspect_PyProject_PoetryFallback(t *testing.T) {
	t.Parallel()
	in := []byte(`[tool.poetry]
name = "my-pkg"
version = "1.2.3"
description = "x"
`)
	insp, err := inspectVia("pyproject.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "[tool.poetry].version" {
		t.Errorf("Versions = %+v, want one [tool.poetry].version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Path != "[tool.poetry].name" {
		t.Errorf("Names = %+v, want one [tool.poetry].name", insp.Names)
	}
}

// TestTomlInspect_PyProject_BothPresentPEP621Wins exercises the
// theoretical mid-migration state: when both sections carry a
// version, PEP 621 (the first VersionPath) wins. The Poetry value is
// not surfaced; bump (Replace) likewise touches PEP 621 only — see
// TestTomlReplace_PyProject_BothPresentBumpsPEP621Only below.
func TestTomlInspect_PyProject_BothPresentPEP621Wins(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "my-pkg"
version = "1.2.3"

[tool.poetry]
name = "my-pkg"
version = "9.9.9"
`)
	insp, err := inspectVia("pyproject.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v, want PEP 621 to win", insp.Versions)
	}
}

// TestTomlInspect_PyProject_NeitherPresent fails extraction so the
// dispatcher can fall through to the `*.toml` confidence-1 fallback.
// Here we expect the rule itself (path-pinned) to fail; resolveRule
// will then drop down to the *.toml fallback.
func TestTomlInspect_PyProject_NeitherPresent(t *testing.T) {
	t.Parallel()
	// no [project] / [tool.poetry] / top-level version
	in := []byte(`[tool.black]
line-length = 100
`)
	_, err := inspectVia("pyproject.toml", in)
	if err == nil {
		t.Error("expected extraction failure when neither [project] nor [tool.poetry] has a version")
	}
}

// TestTomlInspect_PyProject_FallbackToTopLevelTOML guards the
// fallthrough behaviour: a pyproject.toml with no `[project]` and no
// `[tool.poetry]` but a top-level `version = "..."` resolves through
// the DR-0011 `*.toml` fallback (confidence 1).
func TestTomlInspect_PyProject_FallbackToTopLevelTOML(t *testing.T) {
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
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

// TestTomlReplace_PyProject_PEP621 rewrites `[project].version` while
// leaving every other section untouched.
func TestTomlReplace_PyProject_PEP621(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "my-pkg"
version = "1.2.3"
description = "x"

[tool.black]
line-length = 100
`)
	out, err := replaceVia("pyproject.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[project]\nname = \"my-pkg\"\nversion = \"1.2.4\"") {
		t.Errorf("[project].version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[tool.black]\nline-length = 100") {
		t.Errorf("unrelated section disturbed:\n%s", s)
	}
}

// TestTomlReplace_PyProject_PoetryFallback rewrites `[tool.poetry].version`
// when `[project]` is absent.
func TestTomlReplace_PyProject_PoetryFallback(t *testing.T) {
	t.Parallel()
	in := []byte(`[tool.poetry]
name = "my-pkg"
version = "1.2.3"

[tool.poetry.dependencies]
python = "^3.10"
`)
	out, err := replaceVia("pyproject.toml", in, "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[tool.poetry]\nname = \"my-pkg\"\nversion = \"2.0.0\"") {
		t.Errorf("[tool.poetry].version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[tool.poetry.dependencies]\npython = \"^3.10\"") {
		t.Errorf("nested section disturbed:\n%s", s)
	}
}

// TestTomlReplace_PyProject_BothPresentBumpsPEP621Only exercises the
// MVP trade-off: when both `[project]` and `[tool.poetry]` carry a
// version (theoretical mid-migration state), only PEP 621 is
// rewritten. DR-0014 § 6 documents this; calling code that needs
// dual-write should add a separate path-pinned rule or open an issue.
func TestTomlReplace_PyProject_BothPresentBumpsPEP621Only(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "my-pkg"
version = "1.2.3"

[tool.poetry]
name = "my-pkg"
version = "1.2.3"
`)
	out, err := replaceVia("pyproject.toml", in, "1.2.3", "1.3.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[project]\nname = \"my-pkg\"\nversion = \"1.3.0\"") {
		t.Errorf("[project].version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[tool.poetry]\nname = \"my-pkg\"\nversion = \"1.2.3\"") {
		t.Errorf("[tool.poetry].version was unexpectedly bumped (MVP only writes the first match):\n%s", s)
	}
}

// TestTomlInspect_MojoProject reads `[workspace].name` / `[workspace].version`.
func TestTomlInspect_MojoProject(t *testing.T) {
	t.Parallel()
	in := []byte(`[workspace]
name = "my-mojo"
version = "1.2.3"

[workspace.dependencies]
foo = "*"
`)
	insp, err := inspectVia("mojoproject.toml", in)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if len(insp.Versions) != 1 || insp.Versions[0].Value != "1.2.3" || insp.Versions[0].Path != "[workspace].version" {
		t.Errorf("Versions = %+v, want one [workspace].version=1.2.3", insp.Versions)
	}
	if len(insp.Names) != 1 || insp.Names[0].Path != "[workspace].name" {
		t.Errorf("Names = %+v", insp.Names)
	}
}

// TestTomlReplace_MojoProject rewrites `[workspace].version` only.
func TestTomlReplace_MojoProject(t *testing.T) {
	t.Parallel()
	in := []byte(`[workspace]
name = "my-mojo"
version = "1.2.3"

[workspace.dependencies]
foo = "1.0.0"
`)
	out, err := replaceVia("mojoproject.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[workspace]\nname = \"my-mojo\"\nversion = \"1.2.4\"") {
		t.Errorf("[workspace].version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[workspace.dependencies]\nfoo = \"1.0.0\"") {
		t.Errorf("nested section disturbed:\n%s", s)
	}
}

// TestTomlReplaceInSection_MissingSection guards the behaviour of the
// section-scoped helper directly (without going through dispatch).
func TestTomlReplaceInSection_MissingSection(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "x"
version = "1.2.3"
`)
	_, err := tomlReplaceInSection(in, "workspace", "1.2.4")
	if err == nil {
		t.Error("expected error when [workspace] section is absent")
	}
}

// TestTomlReplaceInSection_MissingVersion guards the behaviour when
// the section exists but has no version key.
func TestTomlReplaceInSection_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "x"
description = "y"
`)
	_, err := tomlReplaceInSection(in, "project", "1.2.4")
	if err == nil {
		t.Error("expected error when [project] has no version line")
	}
}

// TestTomlReplaceInSection_DottedSection exercises a section path that
// itself contains a dot (`tool.poetry`). The regex must escape the
// dot so e.g. `[toolXpoetry]` does NOT match.
func TestTomlReplaceInSection_DottedSection(t *testing.T) {
	t.Parallel()
	in := []byte(`[tool.poetry]
name = "x"
version = "1.2.3"

[tool.black]
line-length = 100
`)
	out, err := tomlReplaceInSection(in, "tool.poetry", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[tool.poetry]\nname = \"x\"\nversion = \"1.2.4\"") {
		t.Errorf("[tool.poetry].version not bumped:\n%s", s)
	}
	if !strings.Contains(s, "[tool.black]\nline-length = 100") {
		t.Errorf("[tool.black] section disturbed:\n%s", s)
	}
}

// TestTomlReplaceInSection_TopLevelEmptyPath confirms that passing
// an empty sectionPath behaves identically to the DR-0011 top-level
// fallback (this is the documented contract of the helper).
func TestTomlReplaceInSection_TopLevelEmptyPath(t *testing.T) {
	t.Parallel()
	in := []byte(`name = "x"
version = "1.2.3"

[deps]
version = "9.9.9"
`)
	out, err := tomlReplaceInSection(in, "", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `version = "2.0.0"`) {
		t.Errorf("top-level version not bumped via empty sectionPath:\n%s", s)
	}
	if !strings.Contains(s, "[deps]\nversion = \"9.9.9\"") {
		t.Errorf("section version was unexpectedly modified:\n%s", s)
	}
}

// TestTomlReplaceInSection_OnlySectionScoped checks regression
// guard: when two distinct sections both have `version = "1.2.3"`,
// rewriting one must not touch the other.
func TestTomlReplaceInSection_OnlySectionScoped(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "x"
version = "1.2.3"

[tool.poetry]
name = "x"
version = "1.2.3"

[tool.black]
line-length = 100
`)
	out, err := tomlReplaceInSection(in, "tool.poetry", "9.9.9")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "[project]\nname = \"x\"\nversion = \"1.2.3\"") {
		t.Errorf("[project] section was disturbed:\n%s", s)
	}
	if !strings.Contains(s, "[tool.poetry]\nname = \"x\"\nversion = \"9.9.9\"") {
		t.Errorf("[tool.poetry].version not bumped:\n%s", s)
	}
}

// TestTomlInspect_PyProject_PreservesQuoteStyle checks that single-quoted
// version strings round-trip through Replace with the quote style
// preserved (matches the existing top-level fallback's behaviour).
func TestTomlReplace_PyProject_SingleQuoted(t *testing.T) {
	t.Parallel()
	in := []byte(`[project]
name = "x"
version = '1.2.3'
`)
	out, err := replaceVia("pyproject.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), "version = '1.2.4'") {
		t.Errorf("single-quote style not preserved:\n%s", string(out))
	}
}

// TestTomlReplace_PyProject_TrailingComment preserves an inline
// `# comment` after the version assignment.
func TestTomlReplace_PyProject_TrailingComment(t *testing.T) {
	t.Parallel()
	in := []byte("[project]\nname = \"x\"\nversion = \"1.2.3\"  # release tag\n")
	out, err := replaceVia("pyproject.toml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	if !strings.Contains(string(out), `version = "1.2.4"  # release tag`) {
		t.Errorf("trailing comment not preserved:\n%s", string(out))
	}
}
