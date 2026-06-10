package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cargoWorkspaceHintSubstr is a stable fragment of the workspace hint so
// tests don't pin the exact wording.
const cargoWorkspaceHintSubstr = "version.workspace = true"

// writeCargo writes a minimal single-crate Cargo.toml under dir/<sub>/Cargo.toml
// and returns the full path. The basename stays "Cargo.toml" so the
// path-pinned rule (confidence 3) matches.
func writeCargo(t *testing.T, dir, sub, name string) string {
	t.Helper()
	d := filepath.Join(dir, sub)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, "Cargo.toml")
	body := "[package]\nname = \"" + name + "\"\nversion = \"1.2.3\"\n"
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Two Cargo.toml with diverging package names → name mismatch (exit 2 on
// bump) and the workspace hint is emitted on stderr.
func TestRun_CargoNameMismatch_EmitsWorkspaceHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeCargo(t, dir, "a", "cache-warden")
	b := writeCargo(t, dir, "b", "cache-warden-cli")

	var stderr bytes.Buffer
	err := run([]string{"patch", a, b}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "name mismatch:") {
		t.Errorf("stderr missing 'name mismatch:' header: %q", se)
	}
	if !strings.Contains(se, "hint:") || !strings.Contains(se, cargoWorkspaceHintSubstr) {
		t.Errorf("stderr missing Cargo workspace hint (%q): %q", cargoWorkspaceHintSubstr, se)
	}
}

// A package.json name mismatch must NOT surface the Rust workspace hint.
func TestRun_JSONNameMismatch_NoWorkspaceHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"name":"bar","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	err := run([]string{"patch", a, b}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "name mismatch:") {
		t.Errorf("stderr missing 'name mismatch:' header: %q", se)
	}
	if strings.Contains(se, cargoWorkspaceHintSubstr) {
		t.Errorf("package.json mismatch must not emit Rust workspace hint: %q", se)
	}
}

// --no-hint suppresses the Cargo workspace hint (but not the error body).
func TestRun_CargoNameMismatch_NoHintSuppressesWorkspaceHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeCargo(t, dir, "a", "cache-warden")
	b := writeCargo(t, dir, "b", "cache-warden-cli")

	var stderr bytes.Buffer
	err := run([]string{"patch", a, b, "--no-hint"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "name mismatch:") {
		t.Errorf("--no-hint must keep the error body, got: %q", se)
	}
	if strings.Contains(se, "hint:") || strings.Contains(se, cargoWorkspaceHintSubstr) {
		t.Errorf("--no-hint must suppress the Cargo workspace hint, got: %q", se)
	}
}

// get-path name mismatch (exit 1) also surfaces the workspace hint when a
// Cargo.toml is involved.
func TestRun_CargoNameMismatch_Get_EmitsWorkspaceHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeCargo(t, dir, "a", "cache-warden")
	b := writeCargo(t, dir, "b", "cache-warden-cli")

	var stderr bytes.Buffer
	err := run([]string{"get", a, b}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch (predicate-false), got nil")
	}
	se := stderr.String()
	if !strings.Contains(se, "name mismatch:") {
		t.Errorf("stderr missing 'name mismatch:' header: %q", se)
	}
	if !strings.Contains(se, cargoWorkspaceHintSubstr) {
		t.Errorf("get name mismatch with Cargo.toml should emit workspace hint: %q", se)
	}
}
