package main

import (
	"errors"
	"os"
	"os/exec"
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

// --- DR-0020 PR-3: Diff tests --------------------------------------------

// TestGitBackend_Diff_NoPaths_HasDiff: with no path filter, `Diff` returns a
// non-empty patch when the workdir differs from REV. The fixture's bump
// commit is HEAD; comparing against HEAD~1 (= initial) gives a VERSION diff.
func TestGitBackend_Diff_NoPaths_HasDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD~1", nil)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) == 0 {
			t.Errorf("Diff = empty, want non-empty patch")
		}
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("Diff should mention VERSION, got: %q", string(out))
		}
	})
}

// TestGitBackend_Diff_NoDiff_EmptyBytes: diffing the worktree against
// HEAD (clean fixture) produces no patch text and no error.
func TestGitBackend_Diff_NoDiff_EmptyBytes(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD", nil)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("Diff = %q, want empty (clean workdir vs HEAD)", string(out))
		}
	})
}

// TestGitBackend_Diff_NonexistentPath_Ignored: a path the user names that
// doesn't exist in the worktree is silently filtered out (kawaz's
// "declarative convergence"). When the path list survives to git, the
// result is whatever exists in that path scope.
func TestGitBackend_Diff_NonexistentPath_Ignored(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		// VERSION exists, doesnotexist.txt does not. We expect the call
		// to succeed and the diff to cover VERSION (vs HEAD~1).
		out, err := b.Diff("HEAD~1", []string{"VERSION", "doesnotexist.txt"})
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("Diff should include VERSION, got: %q", string(out))
		}
	})
}

// TestGitBackend_Diff_AllPathsNonexistent_EmptyVacuous: when every path
// supplied is filtered out (none exist), `Diff` short-circuits to empty
// bytes / nil error. It must NOT call `git diff REV --` with an empty
// path list (which would mean "diff everything").
func TestGitBackend_Diff_AllPathsNonexistent_EmptyVacuous(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD~1", []string{"nope.txt", "alsonope.txt"})
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("Diff = %q, want empty (all paths filtered)", string(out))
		}
	})
}

// TestGitBackend_Diff_BadRev: an unresolvable REV is reported as a VCS-exec
// error (exit code 3 via *exitErr).
func TestGitBackend_Diff_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, err := b.Diff("doesnotexist", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d (vcs exec)", code, exitCodeVCSExec)
		}
	})
}

// TestJjBackend_Diff_NoPaths_HasDiff: jj fixture against @-- (= initial)
// returns the bump-commit diff (VERSION change).
func TestJjBackend_Diff_NoPaths_HasDiff(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", nil)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) == 0 {
			t.Errorf("Diff = empty, want non-empty patch")
		}
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("Diff should mention VERSION, got: %q", string(out))
		}
	})
}

// TestJjBackend_Diff_NoDiff_EmptyBytes: diffing @ against @ yields no
// patch and no error.
func TestJjBackend_Diff_NoDiff_EmptyBytes(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@", nil)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("Diff = %q, want empty (same rev to @)", string(out))
		}
	})
}

// TestJjBackend_Diff_NonexistentPath_Ignored: same declarative-convergence
// rule as git — a nonexistent path doesn't error, and existing paths still
// produce their diff.
func TestJjBackend_Diff_NonexistentPath_Ignored(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", []string{"VERSION", "doesnotexist.txt"})
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("Diff should include VERSION, got: %q", string(out))
		}
	})
}

// TestJjBackend_Diff_AllPathsNonexistent_EmptyVacuous: empty bytes,
// no error, and (critically) we must not call jj with no paths and let
// it return the full diff.
func TestJjBackend_Diff_AllPathsNonexistent_EmptyVacuous(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", []string{"nope.txt", "alsonope.txt"})
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("Diff = %q, want empty (all paths filtered)", string(out))
		}
	})
}

// TestJjBackend_Diff_BadRev: an unresolvable REV is reported as a VCS-exec
// error (exit 3 via *exitErr).
func TestJjBackend_Diff_BadRev(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		_, err := b.Diff("doesnotexist", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d (vcs exec)", code, exitCodeVCSExec)
		}
	})
}

// --- DR-0020 PR-3.1: DiffNameStatus tests --------------------------------

// TestGitBackend_DiffNameStatus_HasChanges: with no path filter, returns
// tab-separated lines like "M\tVERSION" mirroring `git diff --name-status`.
func TestGitBackend_DiffNameStatus_HasChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", nil)
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "VERSION") {
			t.Errorf("DiffNameStatus should mention VERSION, got: %q", s)
		}
		// Must be tab-separated (git's native format) so jj normalization
		// produces uniform output across backends.
		if !strings.Contains(s, "M\tVERSION") {
			t.Errorf("expected tab-separated 'M\\tVERSION', got: %q", s)
		}
	})
}

// TestGitBackend_DiffNameStatus_NoChanges: clean fixture vs HEAD → empty.
func TestGitBackend_DiffNameStatus_NoChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD", nil)
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("DiffNameStatus = %q, want empty", string(out))
		}
	})
}

// TestGitBackend_DiffNameStatus_PathFilter: nonexistent paths are silently
// dropped (same declarative-convergence rule as Diff).
func TestGitBackend_DiffNameStatus_PathFilter(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", []string{"VERSION", "nope.txt"})
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("expected VERSION in output, got: %q", string(out))
		}
	})
}

// TestGitBackend_DiffNameStatus_AllPathsNonexistent: every path filtered →
// empty bytes, no error, must not widen back to all paths.
func TestGitBackend_DiffNameStatus_AllPathsNonexistent(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", []string{"nope.txt"})
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("DiffNameStatus = %q, want empty", string(out))
		}
	})
}

// TestGitBackend_DiffNameStatus_BadRev: unresolvable REV → exit 3.
func TestGitBackend_DiffNameStatus_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, err := b.DiffNameStatus("doesnotexist", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d", code, exitCodeVCSExec)
		}
	})
}

// TestJjBackend_DiffNameStatus_HasChanges_TabNormalized: jj's native
// `jj diff --summary` produces "M VERSION" (space). The backend must
// normalize to git's tab format so cross-backend output is uniform.
func TestJjBackend_DiffNameStatus_HasChanges_TabNormalized(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@--", nil)
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "M\tVERSION") {
			t.Errorf("expected tab-normalized 'M\\tVERSION', got: %q", s)
		}
		// The native jj space-separator must NOT leak through.
		if strings.Contains(s, "M VERSION") {
			t.Errorf("jj space-separated output leaked: %q", s)
		}
	})
}

// TestJjBackend_DiffNameStatus_NoChanges: diff against @ → empty.
func TestJjBackend_DiffNameStatus_NoChanges(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@", nil)
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("DiffNameStatus = %q, want empty", string(out))
		}
	})
}

// TestJjBackend_DiffNameStatus_AllPathsNonexistent: empty result, no widen.
func TestJjBackend_DiffNameStatus_AllPathsNonexistent(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@--", []string{"nope.txt"})
		if err != nil {
			t.Fatalf("DiffNameStatus: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("DiffNameStatus = %q, want empty", string(out))
		}
	})
}

// TestJjBackend_DiffNameStatus_BadRev: unresolvable REV → exit 3.
func TestJjBackend_DiffNameStatus_BadRev(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		_, err := b.DiffNameStatus("doesnotexist", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d", code, exitCodeVCSExec)
		}
	})
}

// --- DR-0020 PR-4: Commit backend tests -----------------------------------
//
// The backend contract for Commit (defined on the interface) is:
//
//   - opts.paths != nil      → snapshot the working-tree content of each
//                              listed path that exists, commit only those.
//                              Nonexistent paths silently dropped
//                              (declarative convergence, same rule as Diff).
//                              All-filtered or no-real-change → no-op, nil.
//   - opts.staged            → commit every dirty/staged change at once
//                              (git: --cached, jj: @ snapshot).
//                              No change at all → no-op, nil.
//   - opts.amend             → fold the current change set into @- (jj) or
//                              the last commit (git --amend). PR-4.1 made
//                              amend fully symmetric with non-amend on
//                              path selectors: bare amend, amend+paths,
//                              and amend+staged are all accepted.
//                              Path-scoped amend follows the same no-op
//                              gate as path mode (all-nonexistent / no-
//                              change → nil). Bare amend bypasses the
//                              gate — message-only amend is a legal
//                              explicit rewrite.
//   - opts.message=="" with !amend → caller-side guarantee (parser rejects
//                              earlier); the backend assumes a message is
//                              present whenever !amend.
//
// These tests build temp fixtures so jj's commit-signing path is fully
// shadowed (HOME=tempdir via runIn + repo-local signing.behavior="drop"
// via setupJjRepo).

// TestGitBackend_Commit_Paths: path-mode commit picks up exactly the
// listed (tracked-modified) files; others remain dirty.
func TestGitBackend_Commit_Paths(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edited\n"); err != nil {
		t.Fatal(err)
	}
	// Make "other.txt" tracked first so we don't conflate this with the
	// untracked-file case (TestGitBackend_Commit_Paths_NewFile covers that).
	runIn(t, dir, "git", "add", "other.txt")
	runIn(t, dir, "git", "-c", "user.name=T", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-qm", "stage other.txt")
	if err := writeFile(filepath.Join(dir, "other.txt"), "edited2\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{paths: []string{"VERSION"}, message: "bump version"}); err != nil {
			t.Fatalf("Commit paths: %v", err)
		}
		// VERSION committed; other.txt still dirty.
		out, err := runBackendCmd("git", "diff", "--name-only")
		if err != nil {
			t.Fatalf("post-commit diff: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != "other.txt" {
			t.Errorf("expected only other.txt dirty after path-commit, got: %q", got)
		}
		msg, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(msg)); got != "bump version" {
			t.Errorf("HEAD message = %q, want 'bump version'", got)
		}
	})
}

// TestGitBackend_Commit_Paths_NewFile: a brand-new (untracked) file
// supplied as PATH must be picked up and committed. Naive
// `git diff --quiet` would skip it (untracked files are ignored by git
// diff) — the backend must `git add -- PATHS` before checking presence.
func TestGitBackend_Commit_Paths_NewFile(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "NEW.txt"), "fresh\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{paths: []string{"NEW.txt"}, message: "add NEW"}); err != nil {
			t.Fatalf("Commit new path: %v", err)
		}
		// HEAD now contains NEW.txt, worktree is clean.
		out, _ := runBackendCmd("git", "log", "-1", "--name-only", "--pretty=")
		if !strings.Contains(string(out), "NEW.txt") {
			t.Errorf("expected NEW.txt in HEAD commit, got: %q", string(out))
		}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if !clean {
			t.Errorf("worktree should be clean after committing untracked file")
		}
	})
}

// TestGitBackend_Commit_Paths_NonexistentOnly: every supplied path is
// nonexistent → no commit, no error (declarative convergence).
func TestGitBackend_Commit_Paths_NonexistentOnly(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// Capture pre-state.
		before, _ := runBackendCmd("git", "rev-parse", "HEAD")
		b := &gitBackend{}
		if err := b.Commit(commitOpts{paths: []string{"no-such.txt"}, message: "ghost"}); err != nil {
			t.Errorf("nonexistent-only Commit should succeed (idempotent), got: %v", err)
		}
		after, _ := runBackendCmd("git", "rev-parse", "HEAD")
		if string(before) != string(after) {
			t.Errorf("expected no new commit, HEAD before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestGitBackend_Commit_Paths_PartialExist: a mix of existing and
// nonexistent paths commits only the existing ones (no error).
func TestGitBackend_Commit_Paths_PartialExist(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{paths: []string{"VERSION", "no-such.txt"}, message: "bump+ghost"}); err != nil {
			t.Fatalf("Commit partial: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--name-only", "--pretty=")
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("expected VERSION in HEAD, got: %q", string(out))
		}
		if strings.Contains(string(out), "no-such.txt") {
			t.Errorf("HEAD should not mention nonexistent path: %q", string(out))
		}
	})
}

// TestGitBackend_Commit_Staged: --staged commits the index (any pending
// `git add`-ed paths in one go), leaves unstaged worktree edits alone.
func TestGitBackend_Commit_Staged(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edited\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{staged: true, message: "staged commit"}); err != nil {
			t.Fatalf("Commit --staged: %v", err)
		}
		// VERSION committed, other.txt remains untracked.
		out, _ := runBackendCmd("git", "log", "-1", "--name-only", "--pretty=")
		if !strings.Contains(string(out), "VERSION") {
			t.Errorf("expected VERSION in HEAD, got: %q", string(out))
		}
		stat, _ := runBackendCmd("git", "status", "--short")
		if !strings.Contains(string(stat), "other.txt") {
			t.Errorf("other.txt should still be untracked after --staged commit, status=%q", string(stat))
		}
	})
}

// TestGitBackend_Commit_Staged_Nothing: --staged with empty index → no-op.
func TestGitBackend_Commit_Staged_Nothing(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("git", "rev-parse", "HEAD")
		b := &gitBackend{}
		if err := b.Commit(commitOpts{staged: true, message: "nothing"}); err != nil {
			t.Errorf("Commit --staged on empty index should succeed (idempotent), got: %v", err)
		}
		after, _ := runBackendCmd("git", "rev-parse", "HEAD")
		if string(before) != string(after) {
			t.Errorf("expected no new commit, HEAD before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestGitBackend_Commit_Amend_NoEdit: --amend without -m folds working
// state into HEAD and preserves the existing commit message.
func TestGitBackend_Commit_Amend_NoEdit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	prevMsg, _ := runBackendCmd("git", "-C", dir, "log", "-1", "--pretty=%s")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, noEdit: true}); err != nil {
			t.Fatalf("Commit --amend: %v", err)
		}
		msg, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if strings.TrimSpace(string(msg)) != strings.TrimSpace(string(prevMsg)) {
			t.Errorf("amend --no-edit should preserve message, got %q want %q",
				strings.TrimSpace(string(msg)), strings.TrimSpace(string(prevMsg)))
		}
	})
}

// TestGitBackend_Commit_Amend_WithMessage: --amend -m rewrites the
// last commit's message.
func TestGitBackend_Commit_Amend_WithMessage(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "rewritten"}); err != nil {
			t.Fatalf("Commit --amend -m: %v", err)
		}
		msg, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(msg)); got != "rewritten" {
			t.Errorf("amend message = %q, want 'rewritten'", got)
		}
	})
}

// --- jj backend Commit tests ----------------------------------------------

// TestJjBackend_Commit_Paths: only the listed path's changes land in the
// committed change, others remain in the next (new) working copy.
func TestJjBackend_Commit_Paths(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edit\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		if err := b.Commit(commitOpts{paths: []string{"VERSION"}, message: "bump"}); err != nil {
			t.Fatalf("Commit paths: %v", err)
		}
		// @- now describes 'bump' and contains only VERSION; @ is the
		// new working copy still carrying other.txt.
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(desc)); got != "bump" {
			t.Errorf("@- description = %q, want 'bump'", got)
		}
		summary, _ := runBackendCmd("jj", "diff", "--summary", "--from", "@--", "--to", "@-")
		if !strings.Contains(string(summary), "VERSION") {
			t.Errorf("@- should include VERSION, got summary=%q", string(summary))
		}
		if strings.Contains(string(summary), "other.txt") {
			t.Errorf("@- should not include other.txt, got summary=%q", string(summary))
		}
	})
}

// TestJjBackend_Commit_Paths_NonexistentOnly: every supplied path is
// nonexistent → no commit, no error (declarative convergence). The @ id
// must stay the same (no new change created).
func TestJjBackend_Commit_Paths_NonexistentOnly(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		b := &jjBackend{}
		if err := b.Commit(commitOpts{paths: []string{"no-such.txt"}, message: "ghost"}); err != nil {
			t.Errorf("nonexistent-only Commit should succeed (idempotent), got: %v", err)
		}
		after, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		if string(before) != string(after) {
			t.Errorf("expected @ unchanged, before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestJjBackend_Commit_Staged: --staged commits all current @ changes.
func TestJjBackend_Commit_Staged(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edit\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		if err := b.Commit(commitOpts{staged: true, message: "all"}); err != nil {
			t.Fatalf("Commit --staged: %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(desc)); got != "all" {
			t.Errorf("@- description = %q, want 'all'", got)
		}
		summary, _ := runBackendCmd("jj", "diff", "--summary", "--from", "@--", "--to", "@-")
		if !strings.Contains(string(summary), "VERSION") || !strings.Contains(string(summary), "other.txt") {
			t.Errorf("@- should include both, got summary=%q", string(summary))
		}
	})
}

// TestJjBackend_Commit_Staged_Nothing: --staged on an empty @ → no-op
// (advisor #1 — DR-0020 explicitly excludes empty commits).
func TestJjBackend_Commit_Staged_Nothing(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		b := &jjBackend{}
		if err := b.Commit(commitOpts{staged: true, message: "nothing"}); err != nil {
			t.Errorf("Commit --staged on empty @ should succeed (idempotent), got: %v", err)
		}
		after, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		if string(before) != string(after) {
			t.Errorf("expected @ unchanged, before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestJjBackend_Commit_Amend_NoEdit: --amend (no -m) folds @ into @-,
// preserving @-'s description.
func TestJjBackend_Commit_Amend_NoEdit(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		// Capture @-'s description from INSIDE the fixture cwd; outside
		// the chdir, runBackendCmd would read from the test binary's
		// original cwd (i.e. the bump-semver repo).
		prevDesc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, noEdit: true}); err != nil {
			t.Fatalf("Commit --amend: %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		if strings.TrimSpace(string(desc)) != strings.TrimSpace(string(prevDesc)) {
			t.Errorf("amend --no-edit should preserve description, got %q want %q",
				strings.TrimSpace(string(desc)), strings.TrimSpace(string(prevDesc)))
		}
	})
}

// TestJjBackend_Commit_Amend_WithMessage: --amend -m rewrites @-'s
// description while absorbing @ into it.
func TestJjBackend_Commit_Amend_WithMessage(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "rewritten"}); err != nil {
			t.Fatalf("Commit --amend -m: %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(desc)); got != "rewritten" {
			t.Errorf("amend description = %q, want 'rewritten'", got)
		}
	})
}

// --- DR-0020 PR-4.1: amend + paths / amend + staged backend tests --------
//
// PR-4 had a parser-level reject for `--amend PATH..` / `--amend
// --staged`. PR-4.1 removes that gate: amend and non-amend modes are
// completely symmetric on which path selectors they accept (the only
// difference is "new commit vs absorb into previous"). These tests pin
// the backend semantics for each accepted combination.

// TestGitBackend_Commit_Amend_Paths: `--amend -m MSG -- PATHS` folds
// ONLY the listed paths' working-tree content into HEAD; unrelated
// dirty / untracked files stay dirty / untracked.
func TestGitBackend_Commit_Amend_Paths(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edit\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "amend+v", paths: []string{"VERSION"}}); err != nil {
			t.Fatalf("Commit amend+paths: %v", err)
		}
		// HEAD contains the bumped VERSION and the new subject.
		msg, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(msg)); got != "amend+v" {
			t.Errorf("HEAD subject = %q, want 'amend+v'", got)
		}
		// other.txt must remain untracked (not folded).
		stat, _ := runBackendCmd("git", "status", "--short")
		if !strings.Contains(string(stat), "other.txt") {
			t.Errorf("other.txt should remain dirty after path-scoped amend, status=%q", string(stat))
		}
	})
}

// TestGitBackend_Commit_Amend_Paths_NoEdit: `--amend -- PATHS` (no -m)
// preserves the previous commit's message while folding the path.
func TestGitBackend_Commit_Amend_Paths_NoEdit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	prevMsg, _ := runBackendCmd("git", "-C", dir, "log", "-1", "--pretty=%s")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, noEdit: true, paths: []string{"VERSION"}}); err != nil {
			t.Fatalf("Commit amend+paths no-edit: %v", err)
		}
		msg, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if strings.TrimSpace(string(msg)) != strings.TrimSpace(string(prevMsg)) {
			t.Errorf("amend --no-edit should preserve message, got %q want %q",
				strings.TrimSpace(string(msg)), strings.TrimSpace(string(prevMsg)))
		}
	})
}

// TestGitBackend_Commit_Amend_Paths_NewFile: an untracked file passed
// as PATH must be picked up (mirroring non-amend path mode — without a
// preceding `git add` the diff gate would miss it).
func TestGitBackend_Commit_Amend_Paths_NewFile(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "NEW.txt"), "fresh\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "amend+new", paths: []string{"NEW.txt"}}); err != nil {
			t.Fatalf("Commit amend+new: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--name-only", "--pretty=")
		if !strings.Contains(string(out), "NEW.txt") {
			t.Errorf("expected NEW.txt in amended HEAD, got: %q", string(out))
		}
	})
}

// TestGitBackend_Commit_Amend_Paths_NonexistentOnly: all-nonexistent
// PATH list during amend → no-op, no HEAD movement (mirrors non-amend
// path-mode declarative convergence; differs from bare `--amend` which
// is an ungated explicit rewrite).
func TestGitBackend_Commit_Amend_Paths_NonexistentOnly(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("git", "rev-parse", "HEAD")
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "ghost", paths: []string{"no-such.txt"}}); err != nil {
			t.Errorf("nonexistent-only amend Commit should succeed (idempotent), got: %v", err)
		}
		after, _ := runBackendCmd("git", "rev-parse", "HEAD")
		if string(before) != string(after) {
			t.Errorf("HEAD should not advance for nonexistent-only amend, before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestGitBackend_Commit_Amend_Staged: amend + staged (no paths) folds
// the index = bare-amend behaviour. Explicit synonym for `--amend` in
// the PR-4.1 commit/amend symmetry.
func TestGitBackend_Commit_Amend_Staged(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		if err := b.Commit(commitOpts{amend: true, staged: true, message: "amend+staged"}); err != nil {
			t.Fatalf("Commit amend+staged: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "amend+staged" {
			t.Errorf("HEAD subject = %q, want 'amend+staged'", got)
		}
	})
}

// TestJjBackend_Commit_Amend_Paths: amend + PATHS squashes only the
// listed paths from @ into @-; other @ changes remain in @.
func TestJjBackend_Commit_Amend_Paths(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edit\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "squashed", paths: []string{"VERSION"}}); err != nil {
			t.Fatalf("Commit amend+paths (jj): %v", err)
		}
		// @- now has 'squashed' as description and includes VERSION.
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(desc)); got != "squashed" {
			t.Errorf("@- description = %q, want 'squashed'", got)
		}
		// other.txt must still be dirty in @ (not folded).
		dirty, _ := runBackendCmd("jj", "diff", "--summary")
		if !strings.Contains(string(dirty), "other.txt") {
			t.Errorf("other.txt should remain dirty in @ after path-scoped amend, summary=%q", string(dirty))
		}
	})
}

// TestJjBackend_Commit_Amend_Paths_NoEdit: amend + PATHS without -m
// preserves @-'s description (using --use-destination-message to avoid
// the editor-prompt-on-combined-description trap that bare jj squash
// hits when both @ and @- have descriptions).
func TestJjBackend_Commit_Amend_Paths_NoEdit(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		// Give @ a description so we can detect any combined-description
		// prompt regression (without -u, squash would prompt here).
		runIn(t, dir, "jj", "describe", "-m", "wip-desc")
		prevDesc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, noEdit: true, paths: []string{"VERSION"}}); err != nil {
			t.Fatalf("Commit amend+paths no-edit (jj): %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		if strings.TrimSpace(string(desc)) != strings.TrimSpace(string(prevDesc)) {
			t.Errorf("amend --no-edit should preserve @- description, got %q want %q",
				strings.TrimSpace(string(desc)), strings.TrimSpace(string(prevDesc)))
		}
	})
}

// TestJjBackend_Commit_Amend_Paths_NonexistentOnly: all-nonexistent
// PATH list during amend → no-op, @- and @ unchanged.
func TestJjBackend_Commit_Amend_Paths_NonexistentOnly(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		beforeParent, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "change_id")
		beforeWc, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, message: "ghost", paths: []string{"no-such.txt"}}); err != nil {
			t.Errorf("nonexistent-only amend Commit should succeed (idempotent), got: %v", err)
		}
		afterParent, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "change_id")
		afterWc, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
		if string(beforeParent) != string(afterParent) || string(beforeWc) != string(afterWc) {
			t.Errorf("expected @ and @- unchanged for nonexistent-only amend (jj)")
		}
	})
}

// TestJjBackend_Commit_Amend_Staged: amend + staged (no paths) folds
// the entire @ change into @- (= same effect as bare amend; explicit
// synonym for PR-4.1 symmetry).
func TestJjBackend_Commit_Amend_Staged(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "other.txt"), "edit\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		b := &jjBackend{}
		if err := b.Commit(commitOpts{amend: true, staged: true, message: "amend+staged"}); err != nil {
			t.Fatalf("Commit amend+staged (jj): %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(desc)); got != "amend+staged" {
			t.Errorf("@- description = %q, want 'amend+staged'", got)
		}
		// Both files folded into @-.
		summary, _ := runBackendCmd("jj", "diff", "--summary", "--from", "@--", "--to", "@-")
		if !strings.Contains(string(summary), "VERSION") || !strings.Contains(string(summary), "other.txt") {
			t.Errorf("@- should include both files, got summary=%q", string(summary))
		}
	})
}

// TestJjBackend_Commit_Amend_NoEdit_BothHaveDesc: regression guard for
// the editor-prompt-on-combined-description trap. When both @ and @-
// carry descriptions and @ is fully absorbed into @-, bare jj squash
// would otherwise pop an editor (Failed to edit description in non-
// interactive callers). PR-4.1 switches the no-edit path to
// --use-destination-message to make this deterministic.
func TestJjBackend_Commit_Amend_NoEdit_BothHaveDesc(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		// Give @ a description so squash's combined-description heuristic
		// would prompt without --use-destination-message.
		runIn(t, dir, "jj", "describe", "-m", "wip-feature")
		prevDesc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		b := &jjBackend{}
		// staged: true → bare-style amend (fold entire @).
		if err := b.Commit(commitOpts{amend: true, noEdit: true, staged: true}); err != nil {
			t.Fatalf("Commit amend no-edit (both have desc): %v", err)
		}
		desc, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description")
		if strings.TrimSpace(string(desc)) != strings.TrimSpace(string(prevDesc)) {
			t.Errorf("no-edit amend should preserve @- description, got %q want %q",
				strings.TrimSpace(string(desc)), strings.TrimSpace(string(prevDesc)))
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

// --- DR-0020 PR-5: Fetch / Push backend tests -----------------------------
//
// Fixtures use a local bare repo as `origin` (file-path remote). This
// satisfies git/jj's protocol expectations without any network and
// without violating the project rule "no real git/jj push outside
// fixtures". The bare lives next to the work directory under the test's
// own t.TempDir tree, so cleanup is automatic.
//
// Tests deliberately exercise behaviour, not exit-code constants on
// success — those are responsibilities of the dispatcher layer
// (runVcsCmdFetch / runVcsCmdPush), tested in main_test.go. Here we
// check that Push surfaces a non-ff condition as the dedicated
// nonFastForwardError sentinel so the dispatcher can map it to exit 5.

// TestGitBackend_Fetch_DefaultRemote: `Fetch("origin")` against a
// pre-loaded bare succeeds with no error and ends "Nothing changed"
// (we don't verify subprocess stderr here — the contract is just
// "no error").
func TestGitBackend_Fetch_DefaultRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.Fetch("origin"); err != nil {
			t.Fatalf("Fetch(origin): %v", err)
		}
	})
}

// TestGitBackend_Fetch_NonexistentRemote: an unknown remote name surfaces
// as an *exitErr with exitCodeVCSExec so the dispatcher exits 3.
func TestGitBackend_Fetch_NonexistentRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.Fetch("nonexistent")
		if err == nil {
			t.Fatal("Fetch(nonexistent) should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// TestJjBackend_Fetch_DefaultRemote: jj fetches via the underlying git
// store (colocated repo). Round-trips through `jj git fetch --remote
// origin`.
func TestJjBackend_Fetch_DefaultRemote(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Fetch("origin"); err != nil {
			t.Fatalf("Fetch(origin): %v", err)
		}
	})
}

// TestJjBackend_Fetch_NonexistentRemote: jj reports "No matching remotes"
// as exit 1 — we wrap it as exitCodeVCSExec.
func TestJjBackend_Fetch_NonexistentRemote(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Fetch("nonexistent")
		if err == nil {
			t.Fatal("Fetch(nonexistent) should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// TestGitBackend_Push_NewBranch: pushing a fresh branch to an empty bare
// is a "new branch" creation; git exits 0. We then verify the bare's
// ref points at the same commit as the local main.
func TestGitBackend_Push_NewBranch(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	})
	// Verify bare now has refs/heads/main pointing to the same SHA.
	localSHA, err := runBackendCmdIn(work, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("local rev-parse: %v", err)
	}
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse: %v", err)
	}
	if strings.TrimSpace(string(localSHA)) != strings.TrimSpace(string(bareSHA)) {
		t.Errorf("bare main = %q, want local main = %q",
			strings.TrimSpace(string(bareSHA)), strings.TrimSpace(string(localSHA)))
	}
}

// TestGitBackend_Push_NothingToPush: when the remote already has the
// same commit, git exits 0 ("Everything up-to-date") and our wrapper
// surfaces that as a clean nil — the DR-0020 idempotency rule.
func TestGitBackend_Push_NothingToPush(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Errorf("Push on up-to-date remote should succeed, got: %v", err)
		}
	})
}

// TestGitBackend_Push_NonFastForward: remote moved on a divergent line;
// our push must be rejected and surface as nonFastForwardError so the
// dispatcher can map to exit 5.
func TestGitBackend_Push_NonFastForward(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	divergeBareViaAttacker(t, bare)
	// Local makes its own commit on top of its old main so we have a
	// divergent push attempt (bare's tip is the attacker's commit).
	if err := writeFile(filepath.Join(work, "local.txt"), "local change\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, work, "git", "add", "local.txt")
	runIn(t, work, "git", "commit", "-qm", "local-only")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin"})
		if err == nil {
			t.Fatal("Push to diverged remote should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exitCodeNonFastForward (5), got: %v", err)
		}
	})
}

// TestGitBackend_Push_BadRemote: unknown remote name → exit 3.
func TestGitBackend_Push_BadRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.Push(pushOpts{name: "main", remote: "nonexistent"})
		if err == nil {
			t.Fatal("Push to nonexistent remote should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// TestJjBackend_Push_NewBookmark: pushing a new bookmark to an empty bare
// succeeds (jj 0.41 handles new bookmarks without --allow-new).
func TestJjBackend_Push_NewBookmark(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	// Create a bookmark named "main" pointing at @- (the second commit).
	// jj's colocated import already brings the git `main` branch in as a
	// bookmark, so we `set` (move/refresh) rather than `create` (which
	// errors on already-existing names).
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	})
	// Verify bare now has refs/heads/main.
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(bareSHA)) == "" {
		t.Errorf("bare should have main after push")
	}
}

// TestJjBackend_Push_NothingToPush: remote already has it → success.
func TestJjBackend_Push_NothingToPush(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	// jj's colocated import already brings the git `main` branch in as a
	// bookmark, so we `set` (move/refresh) rather than `create` (which
	// errors on already-existing names).
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")
	withCwd(t, work, func() {
		b := &jjBackend{}
		// First push gets it onto the remote.
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Fatalf("Push #1: %v", err)
		}
		// Second push is the idempotent no-op.
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Errorf("Push #2 (no-op) should succeed, got: %v", err)
		}
	})
}

// TestJjBackend_Push_NonFastForward: remote moved on a divergent line via
// the attacker fixture; jj's stale-info rejection surfaces as
// exitCodeNonFastForward.
func TestJjBackend_Push_NonFastForward(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	// jj's colocated import already brings the git `main` branch in as a
	// bookmark, so we `set` (move/refresh) rather than `create` (which
	// errors on already-existing names).
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")
	// First push to register the bookmark on the remote.
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Fatalf("setup Push: %v", err)
		}
	})
	// Diverge bare via attacker.
	divergeBareViaAttacker(t, bare)
	// Local advances its bookmark on a divergent line.
	if err := writeFile(filepath.Join(work, "local.txt"), "local change\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, work, "jj", "commit", "-m", "local-only")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")
	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin"})
		if err == nil {
			t.Fatal("Push to diverged remote should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exitCodeNonFastForward (5), got: %v", err)
		}
	})
}

// runBackendCmdIn is a test-only helper that runs name/args in dir and
// returns the trimmed output. Mirrors runBackendCmd (which uses cwd)
// but lets us inspect a bare repo without chdir-ing.
func runBackendCmdIn(dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Output()
}
