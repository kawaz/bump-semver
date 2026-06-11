package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workspaceRootCargo is a workspace-root Cargo.toml: it carries
// [workspace.package].version but no top-level [package] (DR-0021 fallback
// target). Matches tests/fixtures/workspace-root/Cargo.toml.
const workspaceRootCargo = `[workspace]
members = ["crates/*"]
resolver = "2"

[workspace.package]
version = "0.3.1"
edition = "2021"
`

// When the sole input names a file that EXISTS on disk and stdin happens to
// be a pipe (even an empty one, as GitHub Actions wires `run:` step stdin to
// a writer-less FIFO), the legacy stdin-pipe shortcut must NOT shadow the
// on-disk file. Reading the empty pipe instead yielded 0 bytes → the
// Cargo.toml confidence-3 rule and the *.toml fallback both reported
// "missing version" (exit 2). The file on disk is the source of truth here.
func TestRun_ExistingFile_NotShadowedByEmptyStdinPipe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	if err := os.WriteFile(path, []byte(workspaceRootCargo), 0644); err != nil {
		t.Fatal(err)
	}

	// os.Pipe with the writer closed immediately models the GHA step stdin:
	// a pipe/FIFO that yields EOF (0 bytes) with no writer.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close() // immediate EOF: io.ReadAll(stdin) returns 0 bytes
	defer r.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, r, &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "0.3.1" {
		t.Errorf("stdout = %q, want 0.3.1 (file must be read, not the empty pipe)\nstderr: %s", got, stderr.String())
	}
}

// The same case via the workspace-root fixture committed under tests/, to
// keep an end-to-end path-pinned (basename "Cargo.toml") regression guard.
func TestRun_WorkspaceRootFixture_NotShadowedByEmptyStdinPipe(t *testing.T) {
	const fixture = "../tests/fixtures/workspace-root/Cargo.toml"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture missing: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	defer r.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", fixture, "--no-hint"}, r, &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "0.3.1" {
		t.Errorf("stdout = %q, want 0.3.1\nstderr: %s", got, stderr.String())
	}
}
