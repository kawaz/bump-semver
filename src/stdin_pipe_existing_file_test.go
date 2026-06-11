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

// pipeWith returns a read end of an os.Pipe pre-filled with `data` and with
// the writer closed (EOF after the data). Passing data="" models a
// writer-less pipe that yields 0 bytes (e.g. GitHub Actions wires `run:`
// step stdin to an empty FIFO). The reader must be closed by the caller.
func pipeWith(t *testing.T, data string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		go func() {
			_, _ = w.WriteString(data)
			_ = w.Close()
		}()
	} else {
		_ = w.Close() // immediate EOF: io.ReadAll(stdin) returns 0 bytes
	}
	return r
}

// When the sole input names a file that EXISTS on disk and stdin is an
// EMPTY pipe (GitHub Actions wires `run:` step stdin to a writer-less FIFO),
// the empty pipe must NOT shadow the on-disk file: the shortcut falls back
// to reading the file from disk. Reading the empty pipe instead yielded 0
// bytes → the Cargo.toml confidence-3 rule and the *.toml fallback both
// reported "missing version" (exit 2).
func TestRun_ExistingFile_EmptyPipeFallsBackToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	if err := os.WriteFile(path, []byte(workspaceRootCargo), 0644); err != nil {
		t.Fatal(err)
	}

	r := pipeWith(t, "")
	defer r.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, r, &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "0.3.1" {
		t.Errorf("stdout = %q, want 0.3.1 (empty pipe must fall back to the on-disk file)\nstderr: %s", got, stderr.String())
	}
}

// The same case via the workspace-root fixture committed under tests/, to
// keep an end-to-end path-pinned (basename "Cargo.toml") regression guard.
func TestRun_WorkspaceRootFixture_EmptyPipeFallsBackToDisk(t *testing.T) {
	const fixture = "../tests/fixtures/workspace-root/Cargo.toml"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture missing: %v", err)
	}

	r := pipeWith(t, "")
	defer r.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", fixture, "--no-hint"}, r, &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "0.3.1" {
		t.Errorf("stdout = %q, want 0.3.1\nstderr: %s", got, stderr.String())
	}
}

// DR-0004 §6 contract: single FILE + NON-EMPTY pipe → the pipe wins, the
// FILE is only a name hint. This is the `jj file show v0.1.0 Cargo.toml |
// bump-semver get Cargo.toml` use case: read a *past revision's* content
// from the pipe while the on-disk file holds a *different* (current)
// version. The on-disk file must NOT shadow the piped content.
func TestRun_ExistingFile_NonEmptyPipeWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	// On-disk version differs from the piped version: if the pipe wins we
	// see the piped value, if the disk wins we see the on-disk value.
	onDisk := "[package]\nname = \"x\"\nversion = \"9.9.9\"\n"
	if err := os.WriteFile(path, []byte(onDisk), 0644); err != nil {
		t.Fatal(err)
	}
	piped := "[package]\nname = \"x\"\nversion = \"0.1.0\"\n"

	r := pipeWith(t, piped)
	defer r.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, r, &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "0.1.0" {
		t.Errorf("stdout = %q, want 0.1.0 (non-empty pipe must win over the on-disk file; DR-0004 §6)\nstderr: %s", got, stderr.String())
	}
}

// Empty pipe + a path that does NOT exist on disk → the error must name the
// missing file AND mention the empty pipe so a user who typo'd the path can
// reach the actual cause (rather than a confusing "missing version").
func TestRun_MissingFile_EmptyPipeErrorsWithHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	r := pipeWith(t, "")
	defer r.Close()

	var stdout, stderr bytes.Buffer
	err := run([]string{"get", path, "--no-hint"}, r, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error, got success; stdout=%q", stdout.String())
	}
	msg := err.Error() + stderr.String()
	if !strings.Contains(msg, "does-not-exist.json") || !strings.Contains(strings.ToLower(msg), "empty") {
		t.Errorf("error should name the missing file and the empty pipe; got: %q", msg)
	}
}
