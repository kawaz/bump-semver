package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VcsGet_NoArgs: `vcs get` with no key shows the vcs-get help.
func TestRun_VcsGet_NoArgs(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "get"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs get (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "root") || !strings.Contains(stdout.String(), "backend") {
		t.Errorf("expected vcs get help mentioning root/backend, got: %q", stdout.String())
	}
}

// TestRun_VcsGet_UnknownKey: an unknown key is a usage error (exit 2)
// and the error names the available keys.
func TestRun_VcsGet_UnknownKey(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown key")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "root") {
			t.Errorf("error should mention available keys, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsGet_Backend_Git: prints "git" on a git-only repo.
func TestRun_VcsGet_Backend_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Backend_Jj: prints "jj" on a colocated git+jj repo
// (jj wins over git per DR-0008 precedence).
func TestRun_VcsGet_Backend_Jj(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "jj" {
			t.Errorf("backend = %q, want jj", got)
		}
	})
}

// TestRun_VcsGet_Root_Git: prints the repo root path.
func TestRun_VcsGet_Root_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "root"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get root: %v", err)
		}
		got := strings.TrimSpace(stdout.String())
		if got == "" {
			t.Errorf("Root should be non-empty, got empty string")
		}
		// Compare via EvalSymlinks because macOS /var/folders symlinks
		// through to /private/var.
		gotCanon, _ := filepath.EvalSymlinks(got)
		wantCanon, _ := filepath.EvalSymlinks(dir)
		if gotCanon != wantCanon {
			t.Errorf("root = %q (canon %q), want %q", got, gotCanon, wantCanon)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Git: prints "main" for the fixture.
func TestRun_VcsGet_CurrentBranch_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get current-branch: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "main" {
			t.Errorf("current-branch = %q, want main", got)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Detached: detached HEAD returns exit 4
// (exitCodeAmbiguous), not the standard exit 2 (usage).
func TestRun_VcsGet_CurrentBranch_Detached(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "checkout", "--detach", "HEAD")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error on detached HEAD")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exit %d (ambiguous), got: %v", exitCodeAmbiguous, err)
		}
	})
}

// TestRun_VcsGet_Backend_VcsOverride: --vcs git on a colocated repo
// forces the git backend (was jj otherwise).
func TestRun_VcsGet_Backend_VcsOverride(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "--vcs", "git"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend --vcs git: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend (--vcs git) = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Quiet: -q suppresses the stdout value but the command
// still exits 0.
func TestRun_VcsGet_Quiet(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "-q"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend -q: %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -q, got: %q", got)
		}
	})
}

// TestRun_VcsGet_NoRepo: outside a vcs repo, `vcs get backend` should
// report exit 3 (VCS exec / not-a-repo) — distinct from the get's own
// usage errors.
func TestRun_VcsGet_NoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// --- DR-0020 PR-2: `vcs is` integration tests -----------------------------

// TestRun_VcsGet_RejectNameStatusShort: `vcs get -s root` must exit 2
// (verb-local flag for diff, not valid on get).
func TestRun_VcsGet_RejectNameStatusShort(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "-s", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get -s`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("expected stderr to mention 'unknown flag', got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "-s") {
		t.Errorf("expected stderr to name the offending flag '-s', got: %q", stderr.String())
	}
}

// TestRun_VcsGet_RejectNameStatusLong: long form must also exit 2.
func TestRun_VcsGet_RejectNameStatusLong(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--name-status", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --name-status`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--name-status") {
		t.Errorf("expected stderr to name '--name-status', got: %q", stderr.String())
	}
}

// TestRun_VcsGet_RejectUnknownFlag: a completely unknown flag is also
// rejected (covers the generic catch-all, not just -s/--name-status).
func TestRun_VcsGet_RejectUnknownFlag(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--foobar", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --foobar`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--foobar") {
		t.Errorf("expected stderr to name '--foobar', got: %q", stderr.String())
	}
}

// --- DR-0020 PR-4: `vcs commit` integration tests ------------------------
//
// The verb has three modes — `-m MSG PATH..` (path-scoped), `--staged -m
// MSG` (commit-all), and `--amend [-m MSG]` (fold into previous). Each
// has its own correctness story; the run-level tests below pin the
// usage-error matrix (-a rejection, mode exclusivity, dynamic hint), and
// the happy path on a real fixture per backend. Backend-level commit
// semantics are exercised in vcs_backend_test.go.

// TestRun_VcsGet_GlobalQuietStillAccepted: regression guard — global
// `-q` must still work for `vcs get` (it's a global flag, not verb-local).
func TestRun_VcsGet_GlobalQuietStillAccepted(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "-q", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get -q backend: %v", err)
		}
	})
}

// TestRun_VcsGet_DefaultBranchPath_Git: end-to-end dispatch from CLI
// argv through the git backend — verifies the new key registers in
// vcsGetKeys, dispatches to backend.DefaultBranchPath(), and prints the
// resolved absolute path to stdout.
func TestRun_VcsGet_DefaultBranchPath_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "default-branch-path"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get default-branch-path: %v", err)
		}
		got, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
		want, _ := filepath.EvalSymlinks(dir)
		if got != want {
			t.Errorf("default-branch-path = %q, want %q", got, want)
		}
	})
}

// TestRun_VcsGet_CommitID_ExplicitAtRev_Jj: DR-0040 changed the jj
// backend's *default* rev away from the mutable working copy `@`, but
// `--rev @` must still resolve to it explicitly — the old behaviour
// stays reachable, just no longer the default.
func TestRun_VcsGet_CommitID_ExplicitAtRev_Jj(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "UNCOMMITTED.txt"), "wip\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var atOut, defaultOut bytes.Buffer
		if err := run([]string{"vcs", "get", "commit-id", "--rev", "@"}, bytes.NewReader(nil), &atOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get commit-id --rev @: %v", err)
		}
		if err := run([]string{"vcs", "get", "commit-id"}, bytes.NewReader(nil), &defaultOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get commit-id: %v", err)
		}
		at := strings.TrimSpace(atOut.String())
		def := strings.TrimSpace(defaultOut.String())
		if at == def {
			t.Errorf("--rev @ (%q) should differ from the new default (%q) — @ carries an uncommitted edit", at, def)
		}
	})
}

// --- DR-0020 PR-5: vcs fetch / vcs push dispatcher tests ------------------
