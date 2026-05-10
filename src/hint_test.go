package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DR-0010: confidence-1 fallback hint ------------------------------------

// fallback hint fires for an unknown JSON filename that resolves through
// the *.json glob fallback (confidence 1).
func TestRun_FallbackHint_UnknownJSONFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
	want := "matched as *.json fallback"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr should contain %q, got: %q", want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Open issue") {
		t.Errorf("stderr should mention 'Open issue', got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), path) {
		t.Errorf("stderr should reference the file path %q, got: %q", path, stderr.String())
	}
}

// confidence-3 (path-pinned) match must NOT trigger the fallback hint.
func TestRun_FallbackHint_NotShownForPackageJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "fallback") {
		t.Errorf("package.json (confidence 3) must not trigger fallback hint, got: %q", stderr.String())
	}
}

// confidence-2 (basename-only marketplace.json that uses .metadata.version)
// should not trigger the fallback hint either, even though it's not a
// path-pinned rule.
func TestRun_FallbackHint_NotShownForBasenameMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// marketplace.json in a generic directory hits confidence 2
	// (basename-only) and uses .metadata.version. Confidence 2 is an
	// explicit basename rule — no fallback hint.
	path := filepath.Join(dir, "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","metadata":{"version":"1.2.3"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "fallback") {
		t.Errorf("confidence-2 basename match must not trigger fallback hint, got: %q", stderr.String())
	}
}

// confidence-2 rule fails extraction → falls through to confidence-1
// glob → fallback hint should fire (this is the case the rule table
// explicitly accommodates: an unrelated marketplace.json with a top-
// level .version).
func TestRun_FallbackHint_ConfidenceFallthrough(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// marketplace.json in a generic directory but lacks .metadata.version
	// → confidence-2 rule fails → falls back to confidence-1 (*.json).
	path := filepath.Join(dir, "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "matched as *.json fallback") {
		t.Errorf("expected fallback hint when confidence-2 rule falls through, got: %q", stderr.String())
	}
}

// Multiple FILE inputs: only the confidence-1 ones get a hint line, one
// hint per file (no aggregation).
func TestRun_FallbackHint_MultipleFilesListedSeparately(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// package.json → confidence 3 (no hint)
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// unknown1.json → confidence 1 (hint expected)
	a := filepath.Join(dir, "unknown1.json")
	if err := os.WriteFile(a, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// unknown2.json → confidence 1 (hint expected)
	b := filepath.Join(dir, "unknown2.json")
	if err := os.WriteFile(b, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", pkg, a, b}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	se := stderr.String()
	if !strings.Contains(se, a+" matched as *.json fallback") {
		t.Errorf("stderr should mention %s as fallback, got: %q", a, se)
	}
	if !strings.Contains(se, b+" matched as *.json fallback") {
		t.Errorf("stderr should mention %s as fallback, got: %q", b, se)
	}
	if strings.Contains(se, pkg+" matched") {
		t.Errorf("package.json (confidence 3) must not appear in fallback hint, got: %q", se)
	}
	if got := strings.Count(se, "matched as *.json fallback"); got != 2 {
		t.Errorf("expected 2 fallback hint lines, got %d: %q", got, se)
	}
}

// --no-hint suppresses the fallback hint.
func TestRun_FallbackHint_NoHintFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("--no-hint must suppress fallback hint, got: %q", stderr.String())
	}
}

// -q suppresses the fallback hint (and stdout).
func TestRun_FallbackHint_QuietSuppresses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("-q must suppress fallback hint, got: %q", stderr.String())
	}
}

// -qq suppresses the fallback hint.
func TestRun_FallbackHint_QuietAllSuppresses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "-qq"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("-qq must suppress fallback hint, got: %q", stderr.String())
	}
}

// Coexistence with the v0.5.0 "files not modified" hint: a confidence-1
// file bumped without --write should produce both, in order (fallback
// hint first, --write hint second).
func TestRun_FallbackHint_CoexistsWithWriteHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	se := stderr.String()
	fallbackIdx := strings.Index(se, "matched as *.json fallback")
	notModifiedIdx := strings.Index(se, "not modified")
	if fallbackIdx < 0 {
		t.Errorf("expected fallback hint, got: %q", se)
	}
	if notModifiedIdx < 0 {
		t.Errorf("expected --write hint, got: %q", se)
	}
	if fallbackIdx >= notModifiedIdx {
		t.Errorf("fallback hint should precede --write hint, got: %q", se)
	}
	// And both share the `hint:` prefix so a single grep can capture both.
	if got := strings.Count(se, "hint:"); got != 2 {
		t.Errorf("expected 2 hint: lines, got %d: %q", got, se)
	}
}

// Coexistence: --no-hint suppresses BOTH hints (common abstraction).
func TestRun_FallbackHint_NoHintSuppressesBoth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("--no-hint must suppress both fallback and --write hints, got: %q", stderr.String())
	}
}

// fallback hint fires for compare too (the action is irrelevant; the
// hint reflects the file detection).
func TestRun_FallbackHint_FiresForCompare(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"compare", "eq", path, "1.2.3"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "matched as *.json fallback") {
		t.Errorf("compare should also emit fallback hint, got: %q", stderr.String())
	}
}

// --- DR-0010: unsupported-file hint -----------------------------------------

// unsupported file (real on-disk file but no rule matches) emits the
// "Open issue at https://..." hint after the error.
func TestRun_UnsupportedFile_HintWithIssueURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.toml")
	if err := os.WriteFile(path, []byte("nothing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unsupported file, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "unsupported file:") {
		t.Errorf("expected 'unsupported file:' message, got: %q", se)
	}
	if !strings.Contains(se, "https://github.com/kawaz/bump-semver/issues") {
		t.Errorf("expected issue URL in hint, got: %q", se)
	}
	if !strings.Contains(se, path) {
		t.Errorf("expected the file path in the error, got: %q", se)
	}
}

// --no-hint suppresses the unsupported-file hint but still prints the
// "bump-semver: unsupported file:" error itself.
func TestRun_UnsupportedFile_NoHintSuppressesHintOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.toml")
	if err := os.WriteFile(path, []byte("nothing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unsupported file, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "unsupported file:") {
		t.Errorf("error message must remain when --no-hint is set, got: %q", se)
	}
	if strings.Contains(se, "Open issue") {
		t.Errorf("--no-hint must suppress the issue-tracker hint, got: %q", se)
	}
}

// -qq suppresses both error and hint (existing behavior preserved).
func TestRun_UnsupportedFile_QuietAllSuppressesAll(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.toml")
	if err := os.WriteFile(path, []byte("nothing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path, "-qq"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unsupported file, got nil")
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq must silence stderr, got: %q", stderr.String())
	}
}

// -q suppresses the issue-tracker hint but keeps the canonical error
// line (matches v0.5.0 -q semantics: error line stays, hints go).
func TestRun_UnsupportedFile_QuietSuppressesHintOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.toml")
	if err := os.WriteFile(path, []byte("nothing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path, "-q"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unsupported file, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "unsupported file:") {
		t.Errorf("error must remain under -q, got: %q", se)
	}
	if strings.Contains(se, "Open issue") {
		t.Errorf("-q must suppress the issue-tracker hint, got: %q", se)
	}
}
