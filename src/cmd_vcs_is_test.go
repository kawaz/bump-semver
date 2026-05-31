package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VcsIs_NoArgs: `vcs is` with no predicate shows the vcs-is help
// (matches the no-args == --help convention used by `vcs get`).
func TestRun_VcsIs_NoArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "is"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs is (no args): %v", err)
		}
		if !strings.Contains(stdout.String(), "clean") || !strings.Contains(stdout.String(), "dirty") {
			t.Errorf("expected vcs is help mentioning clean/dirty, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsIs_UnknownPred: an unknown predicate is a usage error (exit 2)
// — DR-0020 explicitly forbids silent-false on typos to prevent misroutes.
func TestRun_VcsIs_UnknownPred(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown predicate")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "clean") {
			t.Errorf("error should mention available predicates, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsIs_Clean_True: a fresh git fixture is clean → exit 0.
func TestRun_VcsIs_Clean_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for clean repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Clean_False: a tracked-modification renders the repo
// dirty → `vcs is clean` exits 1, with NO stderr (predicate-false is
// silent, matching compare).
func TestRun_VcsIs_Clean_False(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit 1 on dirty repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("predicate-false should be silent on stderr, got: %q", got)
		}
	})
}

// TestRun_VcsIs_Dirty_True: dirty repo → `vcs is dirty` exits 0.
func TestRun_VcsIs_Dirty_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is dirty` on dirty repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Dirty_False: clean repo → `vcs is dirty` exits 1.
func TestRun_VcsIs_Dirty_False(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is dirty` on clean repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Git_True: a git-only fixture → `vcs is git` exits 0.
func TestRun_VcsIs_Git_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is git` on git repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Jj_False_OnGit: a git-only fixture → `vcs is jj` exits 1.
func TestRun_VcsIs_Jj_False_OnGit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "jj"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is jj` on git-only repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Jj_True: colocated git+jj → `vcs is jj` exits 0 (jj wins
// over git in the auto-probe, matching DR-0008 precedence).
func TestRun_VcsIs_Jj_True(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "jj"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is jj` on colocated repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Git_False_OnColocated: a colocated repo resolves to jj
// in auto-probe, so `vcs is git` exits 1 (matches `vcs get backend`).
func TestRun_VcsIs_Git_False_OnColocated(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is git` on colocated repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Git_True_WithOverride: --vcs git on a colocated repo
// forces the git branch, so `vcs is git` exits 0.
func TestRun_VcsIs_Git_True_WithOverride(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git", "--vcs", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is git --vcs git` on colocated repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_NoRepo_Clean: outside a vcs repo `vcs is clean` reports
// exit 3 (can't tell the answer — distinct from "answer is false").
func TestRun_VcsIs_NoRepo_Clean(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsIs_NoRepo_Git: outside a vcs repo `vcs is git` reports exit
// 3 (can't tell), NOT exit 1 — distinguishes "not git" from "no answer".
// DR-0020: "曖昧・期待外はエラー".
func TestRun_VcsIs_NoRepo_Git(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsIs_TooManyArgs: `vcs is clean dirty` → usage error (exit 2).
func TestRun_VcsIs_TooManyArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for multiple predicates")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// --- DR-0020 PR-3: `vcs diff` integration tests ---------------------------

// TestRun_VcsIs_RejectNameStatusShort: same for `vcs is`.
func TestRun_VcsIs_RejectNameStatusShort(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "is", "-s", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs is -s`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
}

// TestRun_VcsIs_RejectNameStatusLong: long form for `vcs is`.
func TestRun_VcsIs_RejectNameStatusLong(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "is", "--name-status", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs is --name-status`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
}
