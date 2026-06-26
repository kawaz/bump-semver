package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DR-0013: stripKnownSuffix unit tests ----------------------------------

func TestStripKnownSuffix_LiteralSuffixes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   string
		suffix string
	}{
		{"Cargo.toml.bak", "Cargo.toml", ".bak"},
		{"package.json.backup", "package.json", ".backup"},
		{"Chart.yaml.orig", "Chart.yaml", ".orig"},
		{"manifest.toml.tmp", "manifest.toml", ".tmp"},
		{"VERSION.old", "VERSION", ".old"},
		{"path/to/Cargo.toml.bak", "path/to/Cargo.toml", ".bak"},
		{"sub/package.json.tmp", "sub/package.json", ".tmp"},
	}
	for _, c := range cases {
		got, suffix, ok := stripKnownSuffix(c.in)
		if !ok {
			t.Errorf("stripKnownSuffix(%q): ok=false, want true", c.in)
			continue
		}
		if got != c.want || suffix != c.suffix {
			t.Errorf("stripKnownSuffix(%q) = (%q, %q), want (%q, %q)",
				c.in, got, suffix, c.want, c.suffix)
		}
	}
}

func TestStripKnownSuffix_DateStamps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   string
		suffix string
	}{
		{"Cargo.toml.20260510", "Cargo.toml", ".20260510"},
		{"package.json.20260510_120000", "package.json", ".20260510_120000"},
		{"VERSION.20260101", "VERSION", ".20260101"},
		{"sub/Chart.yaml.20260510_235959", "sub/Chart.yaml", ".20260510_235959"},
	}
	for _, c := range cases {
		got, suffix, ok := stripKnownSuffix(c.in)
		if !ok {
			t.Errorf("stripKnownSuffix(%q): ok=false, want true", c.in)
			continue
		}
		if got != c.want || suffix != c.suffix {
			t.Errorf("stripKnownSuffix(%q) = (%q, %q), want (%q, %q)",
				c.in, got, suffix, c.want, c.suffix)
		}
	}
}

func TestStripKnownSuffix_TildeBackup(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   string
		suffix string
	}{
		{"Cargo.toml~", "Cargo.toml", "~"},
		{"package.json~", "package.json", "~"},
		{"path/to/VERSION~", "path/to/VERSION", "~"},
	}
	for _, c := range cases {
		got, suffix, ok := stripKnownSuffix(c.in)
		if !ok {
			t.Errorf("stripKnownSuffix(%q): ok=false, want true", c.in)
			continue
		}
		if got != c.want || suffix != c.suffix {
			t.Errorf("stripKnownSuffix(%q) = (%q, %q), want (%q, %q)",
				c.in, got, suffix, c.want, c.suffix)
		}
	}
}

// Suffixes that are NOT in the known list (template-style and assorted
// arbitrary extensions) must NOT be stripped.
func TestStripKnownSuffix_UnknownSuffixesNotStripped(t *testing.T) {
	t.Parallel()
	bad := []string{
		"Cargo.toml.template",
		"Cargo.toml.example",
		"Cargo.toml.sample",
		"Cargo.toml.dist",
		"foo.go",
		"README.md",
		"package.json.foobar",
		"Cargo.toml.1234",      // 4 digits, not 8
		"Cargo.toml.123456789", // 9 digits, not 8
	}
	for _, p := range bad {
		got, suffix, ok := stripKnownSuffix(p)
		if ok {
			t.Errorf("stripKnownSuffix(%q): ok=true (got %q, suffix %q), want false",
				p, got, suffix)
		}
		if got != p {
			t.Errorf("stripKnownSuffix(%q): returned path = %q, want unchanged", p, got)
		}
	}
}

// Multi-stage suffixes are stripped ONE level only (DR-0013 § 4).
// `Cargo.toml.bak.20260510` strips to `Cargo.toml.bak` (stripping
// `.20260510`, not the `.bak`).
func TestStripKnownSuffix_OnlyOneLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   string
		suffix string
	}{
		// `.20260510` is the trailing segment, regex matches it first.
		// `.bak` remains in the basename (NOT stripped recursively).
		{"Cargo.toml.bak.20260510", "Cargo.toml.bak", ".20260510"},
		// Tilde wins when it's flush at the end.
		{"Cargo.toml.bak~", "Cargo.toml.bak", "~"},
	}
	for _, c := range cases {
		got, suffix, ok := stripKnownSuffix(c.in)
		if !ok {
			t.Errorf("stripKnownSuffix(%q): ok=false, want true", c.in)
			continue
		}
		if got != c.want || suffix != c.suffix {
			t.Errorf("stripKnownSuffix(%q) = (%q, %q), want (%q, %q)",
				c.in, got, suffix, c.want, c.suffix)
		}
	}
}

// Edge: a name that *is* the suffix (`.bak` alone, `~` alone) must
// not collapse to an empty basename.
func TestStripKnownSuffix_EmptyStemRejected(t *testing.T) {
	t.Parallel()
	bad := []string{".bak", ".backup", ".orig", "~"}
	for _, p := range bad {
		_, _, ok := stripKnownSuffix(p)
		if ok {
			t.Errorf("stripKnownSuffix(%q): ok=true, want false (would leave empty basename)", p)
		}
	}
}

// --- DR-0013: resolveRule integration tests --------------------------------

// All known suffixes resolve through to the right rule for each format.
func TestResolveRule_SuffixStripped_AllFormats(t *testing.T) {
	t.Parallel()
	type tc struct {
		path     string
		content  string
		wantRule string
		wantConf int
		wantSuf  string
	}
	cases := []tc{
		// Cargo.toml (confidence 3 → downgraded to 2)
		{"Cargo.toml.bak", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml", 2, ".bak"},
		{"Cargo.toml.backup", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml", 2, ".backup"},
		{"Cargo.toml.20260510", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml", 2, ".20260510"},
		{"Cargo.toml.20260510_120000", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml", 2, ".20260510_120000"},
		{"Cargo.toml~", "[package]\nname = \"x\"\nversion = \"1.2.3\"\n", "Cargo.toml", 2, "~"},
		// package.json (confidence 3 → 2)
		{"package.json.bak", `{"name":"x","version":"1.2.3"}`, "package.json", 2, ".bak"},
		{"package.json.20260510", `{"name":"x","version":"1.2.3"}`, "package.json", 2, ".20260510"},
		// VERSION (confidence 3 → 2)
		{"VERSION.bak", "1.2.3\n", "VERSION (plain text)", 2, ".bak"},
		{"VERSION~", "1.2.3\n", "VERSION (plain text)", 2, "~"},
		// build.zig.zon (confidence 2 → 1)
		{"build.zig.zon.bak", ".{\n    .version = \"1.2.3\",\n}\n", "build.zig.zon", 1, ".bak"},
		// mix.exs (confidence 2 → 1)
		{"mix.exs.20260510", "    version: \"1.2.3\",\n", "mix.exs", 1, ".20260510"},
		// moon.mod (confidence 3 → 2)
		{"moon.mod.bak", "name = \"kawaz/x\"\nversion = \"1.2.3\"\n", "moon.mod", 2, ".bak"},
	}
	for _, c := range cases {
		rule, insp, err := resolveRule(c.path, []byte(c.content))
		if err != nil {
			t.Errorf("resolveRule(%q) error: %v", c.path, err)
			continue
		}
		if rule.Name != c.wantRule {
			t.Errorf("resolveRule(%q) rule = %q, want %q", c.path, rule.Name, c.wantRule)
		}
		if insp.MatchedConfidence != c.wantConf {
			t.Errorf("resolveRule(%q) MatchedConfidence = %d, want %d",
				c.path, insp.MatchedConfidence, c.wantConf)
		}
		if insp.MatchedSuffixStripped != c.wantSuf {
			t.Errorf("resolveRule(%q) MatchedSuffixStripped = %q, want %q",
				c.path, insp.MatchedSuffixStripped, c.wantSuf)
		}
		if len(insp.Versions) == 0 || insp.Versions[0].Value != "1.2.3" {
			t.Errorf("resolveRule(%q) Versions = %+v, want one 1.2.3", c.path, insp.Versions)
		}
	}
}

// `*.yaml` fallback through suffix stripping. Confidence 1 floored at 1.
func TestResolveRule_SuffixStripped_GlobFallbackFloor(t *testing.T) {
	t.Parallel()
	in := []byte("name: x\nversion: 1.2.3\n")
	rule, insp, err := resolveRule("Chart.yaml.bak", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if rule.Name != "*.yaml (fallback)" {
		t.Errorf("rule = %q, want *.yaml (fallback)", rule.Name)
	}
	if insp.MatchedConfidence != 1 {
		t.Errorf("MatchedConfidence = %d, want 1 (floored)", insp.MatchedConfidence)
	}
	if insp.MatchedSuffixStripped != ".bak" {
		t.Errorf("MatchedSuffixStripped = %q, want .bak", insp.MatchedSuffixStripped)
	}
	if insp.MatchedGlob != "*.yaml" {
		t.Errorf("MatchedGlob = %q, want *.yaml", insp.MatchedGlob)
	}
}

// Multi-stage suffix: `Cargo.toml.bak.20260510` strips ONE level
// (`.20260510`). `Cargo.toml.bak` matches no rule, so the whole
// resolve fails — recursion is intentionally NOT applied.
func TestResolveRule_SuffixStripped_NoRecursion(t *testing.T) {
	t.Parallel()
	in := []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n")
	_, _, err := resolveRule("Cargo.toml.bak.20260510", in)
	if err == nil {
		t.Fatal("expected error (no recursive stripping), got nil")
	}
	var ufe *unsupportedFileError
	if !errors.As(err, &ufe) {
		t.Errorf("expected unsupportedFileError, got %T: %v", err, err)
	}
	// Error must reference the *original* path, not the stripped one.
	if ufe != nil && ufe.path != "Cargo.toml.bak.20260510" {
		t.Errorf("unsupportedFileError.path = %q, want original input", ufe.path)
	}
}

// Template-style suffixes are NOT in the known list and must error.
func TestResolveRule_TemplateSuffix_Unsupported(t *testing.T) {
	t.Parallel()
	in := []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n")
	bad := []string{
		"Cargo.toml.template",
		"Cargo.toml.example",
		"Cargo.toml.sample",
		"Cargo.toml.dist",
	}
	for _, p := range bad {
		_, _, err := resolveRule(p, in)
		if err == nil {
			t.Errorf("resolveRule(%q) expected unsupported error, got nil", p)
			continue
		}
		var ufe *unsupportedFileError
		if !errors.As(err, &ufe) {
			t.Errorf("resolveRule(%q) expected unsupportedFileError, got %T: %v", p, err, err)
		}
	}
}

// Confidence-3 path-pinned rules survive the suffix-stripping change:
// a regular `Cargo.toml` still resolves at confidence 3, no suffix
// metadata leaked.
func TestResolveRule_NoSuffixStripping_RegularPathsUnaffected(t *testing.T) {
	t.Parallel()
	in := []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n")
	_, insp, err := resolveRule("Cargo.toml", in)
	if err != nil {
		t.Fatalf("resolveRule error: %v", err)
	}
	if insp.MatchedConfidence != 3 {
		t.Errorf("MatchedConfidence = %d, want 3", insp.MatchedConfidence)
	}
	if insp.MatchedSuffixStripped != "" {
		t.Errorf("MatchedSuffixStripped = %q, want empty", insp.MatchedSuffixStripped)
	}
}

// detectHandler should accept suffix-stripped paths (so resolveRule
// gets a chance at actually resolving them).
func TestDetectHandler_SuffixStrippedPaths(t *testing.T) {
	t.Parallel()
	good := []string{
		"Cargo.toml.bak",
		"package.json.20260510",
		"VERSION~",
		"path/to/Cargo.toml.backup",
		"sub/Chart.yaml.bak",
	}
	for _, p := range good {
		if _, err := detectHandler(p); err != nil {
			t.Errorf("detectHandler(%q) unexpected error: %v", p, err)
		}
	}
	// Template-style still rejected.
	bad := []string{"Cargo.toml.template", "package.json.example"}
	for _, p := range bad {
		if _, err := detectHandler(p); err == nil {
			t.Errorf("detectHandler(%q) expected error, got nil", p)
		}
	}
}

// --- DR-0013: hint output integration tests ---------------------------------

// Single suffix-stripped FILE → suffix hint emitted.
func TestRun_SuffixHint_SingleFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
	se := stderr.String()
	want := "matched as Cargo.toml rule (suffix .bak stripped)"
	if !strings.Contains(se, want) {
		t.Errorf("stderr should contain %q, got: %q", want, se)
	}
	if !strings.Contains(se, "use --no-hint to suppress") {
		t.Errorf("stderr should mention --no-hint, got: %q", se)
	}
	if !strings.Contains(se, path) {
		t.Errorf("stderr should reference the path %q, got: %q", path, se)
	}
}

// Date stamp variant.
func TestRun_SuffixHint_DateStamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.20260510")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "suffix .20260510 stripped") {
		t.Errorf("expected date-stamp suffix hint, got: %q", stderr.String())
	}
}

// Tilde variant.
func TestRun_SuffixHint_Tilde(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml~")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "suffix ~ stripped") {
		t.Errorf("expected tilde suffix hint, got: %q", stderr.String())
	}
}

// `--no-hint` suppresses the suffix hint.
func TestRun_SuffixHint_NoHintFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("--no-hint must suppress suffix hint, got: %q", stderr.String())
	}
}

// `-q` suppresses the suffix hint.
func TestRun_SuffixHint_QuietSuppresses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("-q must suppress suffix hint, got: %q", stderr.String())
	}
}

// `-qq` suppresses the suffix hint.
func TestRun_SuffixHint_QuietAllSuppresses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "-qq"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("-qq must suppress suffix hint, got: %q", stderr.String())
	}
}

// Coexistence: suffix-stripped basename hits the *.json fallback rule.
// BOTH hints fire (suffix hint first, fallback hint second).
func TestRun_SuffixHint_CoexistsWithFallbackHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// unknown.json.bak: strip .bak → unknown.json → matches *.json fallback (confidence 1).
	path := filepath.Join(dir, "unknown.json.bak")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	se := stderr.String()
	suffixIdx := strings.Index(se, "suffix .bak stripped")
	fallbackIdx := strings.Index(se, "matched as *.json fallback")
	if suffixIdx < 0 {
		t.Errorf("expected suffix hint, got: %q", se)
	}
	if fallbackIdx < 0 {
		t.Errorf("expected *.json fallback hint, got: %q", se)
	}
	if suffixIdx >= 0 && fallbackIdx >= 0 && suffixIdx >= fallbackIdx {
		t.Errorf("suffix hint should precede fallback hint, got: %q", se)
	}
	if got := strings.Count(se, "hint:"); got != 2 {
		t.Errorf("expected 2 hint: lines, got %d: %q", got, se)
	}
}

// `--no-hint` suppresses BOTH the suffix hint and the fallback hint.
func TestRun_SuffixHint_NoHintSuppressesBoth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json.bak")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("--no-hint must suppress both hints, got: %q", stderr.String())
	}
}

// Template-style suffix: unsupported error (the issue-tracker hint
// from DR-0010 still fires; the suffix hint does NOT because the
// suffix was never stripped).
func TestRun_TemplateSuffix_UnsupportedError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.template")
	if err := os.WriteFile(path, []byte("[package]\nversion = \"0.0.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected unsupported file error, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "unsupported file:") {
		t.Errorf("expected 'unsupported file:' message, got: %q", se)
	}
	if !strings.Contains(se, "Open issue") {
		t.Errorf("expected DR-0010 issue-tracker hint, got: %q", se)
	}
	if strings.Contains(se, "suffix") && strings.Contains(se, "stripped") {
		t.Errorf("template suffix must NOT trigger suffix-strip hint, got: %q", se)
	}
}

// Bump action with a suffix-stripped FILE: the suffix hint precedes
// the existing "files not modified" hint (event order).
func TestRun_SuffixHint_OrderedBeforeWriteHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	se := stderr.String()
	suffixIdx := strings.Index(se, "suffix .bak stripped")
	notModifiedIdx := strings.Index(se, "not modified")
	if suffixIdx < 0 {
		t.Errorf("expected suffix hint, got: %q", se)
	}
	if notModifiedIdx < 0 {
		t.Errorf("expected --write hint, got: %q", se)
	}
	if suffixIdx >= 0 && notModifiedIdx >= 0 && suffixIdx >= notModifiedIdx {
		t.Errorf("suffix hint should precede --write hint, got: %q", se)
	}
}

// Compare action also surfaces the suffix hint (it reflects file
// detection, not the action).
func TestRun_SuffixHint_FiresForCompare(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(path, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"compare", "eq", path, "1.2.3"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "suffix .bak stripped") {
		t.Errorf("compare should also emit suffix hint, got: %q", stderr.String())
	}
}

// Suffix-stripped FILE alongside the original FILE in cross-input
// consistency check: both versions must agree, only writable inputs
// get a suffix hint.
func TestRun_SuffixHint_MultipleInputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "Cargo.toml")
	if err := os.WriteFile(a, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "Cargo.toml.bak")
	if err := os.WriteFile(b, []byte("[package]\nname = \"x\"\nversion = \"1.2.3\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", a, b}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
	se := stderr.String()
	if !strings.Contains(se, b+" matched as Cargo.toml rule") {
		t.Errorf("expected suffix hint for %q, got: %q", b, se)
	}
	if strings.Contains(se, a+" matched") {
		t.Errorf("Cargo.toml (no suffix) must not trigger suffix hint, got: %q", se)
	}
	// Only one suffix hint emitted.
	if got := strings.Count(se, "suffix"); got != 1 {
		t.Errorf("expected 1 suffix hint, got %d: %q", got, se)
	}
}

// `--write` works for suffix-stripped FILE inputs (writeback uses the
// resolved rule's Replace, same as a regular FILE).
func TestRun_SuffixStripped_WriteBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml.bak")
	original := "[package]\nname = \"x\"\nversion = \"1.2.3\"\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `version = "1.2.4"`) {
		t.Errorf("file not written back to 1.2.4, got: %q", string(got))
	}
}
