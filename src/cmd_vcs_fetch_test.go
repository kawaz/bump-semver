package main

import (
	"bytes"
	"errors"
	"testing"
)

// TestRun_VcsFetch_DefaultOrigin: `vcs fetch` (no args) targets origin.
func TestRun_VcsFetch_DefaultOrigin(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs fetch: %v", err)
		}
	})
}

// TestRun_VcsFetch_NamedRemote: `vcs fetch <remote>` targets the given
// remote.
func TestRun_VcsFetch_NamedRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "origin"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs fetch origin: %v", err)
		}
	})
}

// TestRun_VcsFetch_NonexistentRemote: bad remote name → exit 3.
func TestRun_VcsFetch_NonexistentRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "nonexistent"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for nonexistent remote")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d, got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsFetch_TooManyArgs: `vcs fetch` accepts at most one positional.
func TestRun_VcsFetch_TooManyArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "origin", "extra"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})
}

// TestRun_VcsFetch_UnknownFlag: `vcs fetch --branch X` is rejected at the
// parser layer (--branch is push-only).
func TestRun_VcsFetch_UnknownFlag(t *testing.T) {
	err := run([]string{"vcs", "fetch", "--branch", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}
