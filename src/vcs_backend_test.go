package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vcsBackend / newVcsBackend tests (DR-0020 PR-1).
//
// These tests build temp-repo fixtures rather than relying on the
// live repo, because the live repo carries multiple bookmarks at HEAD
// which would make `current-branch` ambiguous by design (exit 4).

// TestNewVcsBackend_GitOnly: pure git repo resolves to a git backend.
func TestNewVcsBackend_GitOnly(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsAuto)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		if got := b.Kind(); got != "git" {
			t.Errorf("Kind = %q, want git", got)
		}
	})
}

// TestNewVcsBackend_JjOverGit: a colocated git+jj repo selects jj
// (matches the existing detectVcs precedence in DR-0008).
func TestNewVcsBackend_JjOverGit(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsAuto)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		if got := b.Kind(); got != "jj" {
			t.Errorf("Kind = %q, want jj", got)
		}
	})
}

// TestNewVcsBackend_Override: --vcs git on a colocated repo forces the
// git backend.
func TestNewVcsBackend_Override(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsGit)
		if err != nil {
			t.Fatalf("newVcsBackend(vcsGit): %v", err)
		}
		if got := b.Kind(); got != "git" {
			t.Errorf("Kind = %q, want git (override)", got)
		}
	})
}

// TestNewVcsBackend_NoRepo: no .git / .jj in cwd ancestors is an error.
func TestNewVcsBackend_NoRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		_, err := newVcsBackend(vcsAuto)
		if err == nil {
			t.Fatal("expected error in non-vcs directory")
		}
	})
}

// TestGitBackend_Root: returns the repo root (the directory containing
// .git in our fixture).
func TestGitBackend_Root(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	// Resolve symlinks because /var/folders is a symlink to /private/var
	// on macOS; git reports the canonical path.
	canon, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsGit)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		root, err := b.Root()
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		gotCanon, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatalf("eval symlinks for got: %v", err)
		}
		if gotCanon != canon {
			t.Errorf("Root = %q (canon %q), want %q", root, gotCanon, canon)
		}
	})
}

// TestGitBackend_CurrentBranch: the fixture's `git init -b main` puts
// the working tree on main, so the backend should report "main".
func TestGitBackend_CurrentBranch(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsGit)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		got, err := b.CurrentBranch()
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if got != "main" {
			t.Errorf("CurrentBranch = %q, want main", got)
		}
	})
}

// TestGitBackend_CurrentBranch_Detached: detached HEAD is ambiguous and
// must return exitCodeAmbiguous via *exitErr.
func TestGitBackend_CurrentBranch_Detached(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	// Detach HEAD by checking out the commit sha.
	runIn(t, dir, "git", "checkout", "--detach", "HEAD")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsGit)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		_, err = b.CurrentBranch()
		if err == nil {
			t.Fatal("expected error on detached HEAD")
		}
		if code := exitCodeOf(err); code != exitCodeAmbiguous {
			t.Errorf("exit code = %d, want %d (ambiguous)", code, exitCodeAmbiguous)
		}
	})
}

// TestJjBackend_Root: returns the repo root (jj root with a colocated
// fixture is the same as the git fixture's working dir).
func TestJjBackend_Root(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	canon, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsJj)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		root, err := b.Root()
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		gotCanon, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatalf("eval symlinks: %v", err)
		}
		if gotCanon != canon {
			t.Errorf("Root = %q (canon %q), want %q", root, gotCanon, canon)
		}
	})
}

// TestJjBackend_CurrentBranch_SingleBookmark: a colocated repo with one
// bookmark at the nearest ancestor returns that bookmark name.
func TestJjBackend_CurrentBranch_SingleBookmark(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	// In a colocated `jj git init` setup the git branch is imported as a
	// bookmark named after the branch ("main"). We don't need to create
	// any more.
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsJj)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		got, err := b.CurrentBranch()
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if got != "main" {
			t.Errorf("CurrentBranch = %q, want main", got)
		}
	})
}

// TestJjBackend_CurrentBranch_MultipleBookmarks: more than one bookmark
// at the nearest ancestor commit is ambiguous (exit 4).
func TestJjBackend_CurrentBranch_MultipleBookmarks(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	// Add a second bookmark at the same commit as main (HEAD~0 of jj's
	// view = @-).
	runIn(t, dir, "jj", "bookmark", "create", "feature", "-r", "@-")
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsJj)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		_, err = b.CurrentBranch()
		if err == nil {
			t.Fatal("expected error for multiple bookmarks at head")
		}
		if code := exitCodeOf(err); code != exitCodeAmbiguous {
			t.Errorf("exit code = %d, want %d (ambiguous)", code, exitCodeAmbiguous)
		}
	})
}

// TestJjBackend_CurrentBranch_NoBookmark: zero bookmarks in the
// ancestors of @ is also ambiguous (exit 4).
func TestJjBackend_CurrentBranch_NoBookmark(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := t.TempDir()
	runIn(t, dir, "jj", "git", "init")
	// No commits, no bookmarks.
	withCwd(t, dir, func() {
		b, err := newVcsBackend(vcsJj)
		if err != nil {
			t.Fatalf("newVcsBackend: %v", err)
		}
		_, err = b.CurrentBranch()
		if err == nil {
			t.Fatal("expected error for no bookmark in ancestors")
		}
		if code := exitCodeOf(err); code != exitCodeAmbiguous {
			t.Errorf("exit code = %d, want %d (ambiguous)", code, exitCodeAmbiguous)
		}
	})
}

// --- DR-0020 PR-2: IsClean tests ------------------------------------------

// TestGitBackend_IsClean_Clean: a freshly-committed git fixture is clean
// (tracked files all match HEAD, no staged changes).
func TestGitBackend_IsClean_Clean(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if !clean {
			t.Errorf("IsClean = false, want true (fresh checkout)")
		}
	})
}

// TestGitBackend_IsClean_TrackedDirty: modifying a tracked file (without
// staging) makes the worktree dirty.
func TestGitBackend_IsClean_TrackedDirty(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if clean {
			t.Errorf("IsClean = true, want false (tracked file modified)")
		}
	})
}

// TestGitBackend_IsClean_StagedDirty: a `git add`-ed change makes the
// worktree dirty (even though the workdir matches the index after the add).
func TestGitBackend_IsClean_StagedDirty(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if clean {
			t.Errorf("IsClean = true, want false (staged change)")
		}
	})
}

// TestGitBackend_IsClean_UntrackedIgnored: an untracked file does NOT
// make the worktree dirty (PR-2 contract: untracked excluded; future
// --include-untracked is an additive extension).
func TestGitBackend_IsClean_UntrackedIgnored(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := os.WriteFile(filepath.Join(dir, "NEWFILE.txt"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if !clean {
			t.Errorf("IsClean = false, want true (untracked file ignored)")
		}
	})
}

// TestJjBackend_IsClean_Clean: fresh colocated jj repo has an empty `@`
// (jj puts a new empty change on top of HEAD at init).
func TestJjBackend_IsClean_Clean(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if !clean {
			t.Errorf("IsClean = false, want true (fresh jj @ is empty)")
		}
	})
}

// TestJjBackend_IsClean_TrackedDirty: editing a tracked file is picked up
// by jj's automatic snapshot — `@` becomes non-empty → dirty.
func TestJjBackend_IsClean_TrackedDirty(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if clean {
			t.Errorf("IsClean = true, want false (tracked file modified, jj snapshot)")
		}
	})
}

// TestJjBackend_IsClean_NewFileDirty: jj snapshots new files automatically
// (unlike git, where untracked files are excluded). This is jj's design
// — the contrast vs git is intentional and documented in DR-0020 PR-2.
func TestJjBackend_IsClean_NewFileDirty(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := os.WriteFile(filepath.Join(dir, "NEWFILE.txt"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if clean {
			t.Errorf("IsClean = true, want false (jj snapshots new files)")
		}
	})
}

// exitCodeOf extracts the carried exit code from an *exitErr (or returns
// -1 if err is not an *exitErr). Test-local helper.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	type coder interface{ ExitCode() int }
	if c, ok := err.(coder); ok {
		return c.ExitCode()
	}
	// Try to unwrap.
	if msg := err.Error(); strings.Contains(msg, "exit") {
		return -1
	}
	return -1
}
