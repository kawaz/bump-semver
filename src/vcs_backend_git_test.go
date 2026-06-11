package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitBackend_Root: returns the repo root (the directory containing
// .git in our fixture).
func TestGitBackend_Root(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_IsClean_Clean: a freshly-committed git fixture is clean
// (tracked files all match HEAD, no staged changes).
func TestGitBackend_IsClean_Clean(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_Diff_NoPaths_HasDiff: with no path filter, `Diff` returns a
// non-empty patch when the workdir differs from REV. The fixture's bump
// commit is HEAD; comparing against HEAD~1 (= initial) gives a VERSION diff.
func TestGitBackend_Diff_NoPaths_HasDiff(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD~1", nil, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD", nil, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		// VERSION exists, doesnotexist.txt does not. We expect the call
		// to succeed and the diff to cover VERSION (vs HEAD~1).
		out, err := b.Diff("HEAD~1", []string{"VERSION", "doesnotexist.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.Diff("HEAD~1", []string{"nope.txt", "alsonope.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, err := b.Diff("doesnotexist", nil, nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d (vcs exec)", code, exitCodeVCSExec)
		}
	})
}

// TestGitBackend_DiffNameStatus_HasChanges: with no path filter, returns
// tab-separated lines like "M\tVERSION" mirroring `git diff --name-status`.
func TestGitBackend_DiffNameStatus_HasChanges(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", nil, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD", nil, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", []string{"VERSION", "nope.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.DiffNameStatus("HEAD~1", []string{"nope.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, err := b.DiffNameStatus("doesnotexist", nil, nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d", code, exitCodeVCSExec)
		}
	})
}

// TestGitBackend_Commit_Paths: path-mode commit picks up exactly the
// listed (tracked-modified) files; others remain dirty.
func TestGitBackend_Commit_Paths(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_Commit_Amend_Paths: `--amend -m MSG -- PATHS` folds
// ONLY the listed paths' working-tree content into HEAD; unrelated
// dirty / untracked files stay dirty / untracked.
func TestGitBackend_Commit_Amend_Paths(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_Fetch_DefaultRemote: `Fetch("origin")` against a
// pre-loaded bare succeeds with no error and ends "Nothing changed"
// (we don't verify subprocess stderr here — the contract is just
// "no error").
func TestGitBackend_Fetch_DefaultRemote(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_Push_NewBranch: pushing a fresh branch to an empty bare
// is a "new branch" creation; git exits 0. We then verify the bare's
// ref points at the same commit as the local main.
func TestGitBackend_Push_NewBranch(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGitBackend_TagPush_NewTag: a fresh NAME at HEAD is created locally
// and pushed; the bare ends up holding it.
func TestGitBackend_TagPush_NewTag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "HEAD", Remote: "origin",
		}); err != nil {
			t.Fatalf("TagPush(new): %v", err)
		}
	})
	want := localHeadSHA(t, work)
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q", got, want)
	}
}

// TestGitBackend_TagPush_SameRevIdempotent: the 片落ちリカバリ case.
// Locally we already have the tag; running again with the same REV must
// succeed (the operation's intent is "tag points to REV on remote",
// which is already true). This isolates the local-create-skip branch
// because the remote already has it too (preloaded).
func TestGitBackend_TagPush_SameRevIdempotent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	// Tag locally then push (round 1) so we're in the "exists on both
	// sides" state at the same REV.
	runIn(t, work, "git", "tag", "v1.0.0", "HEAD")
	want := localHeadSHA(t, work)
	withCwd(t, work, func() {
		b := &gitBackend{}
		// Manually push the local tag via runBackendCmdIn (out of band,
		// not via TagPush) — we want round 1 to set up the remote without
		// touching our SUT.
		if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
			t.Fatalf("setup push: %v", err)
		}
		// Round 2: same REV, must be a no-op success.
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "HEAD", Remote: "origin",
		}); err != nil {
			t.Errorf("same-rev TagPush should succeed (idempotent), got: %v", err)
		}
	})
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q (unchanged)", got, want)
	}
}

// TestGitBackend_TagPush_DiffRevNoMoveFlag: same NAME at a different REV
// without `--allow-move` is the integrity violation case (exit 4). The
// bare must remain pointing at the original REV (no side-effect).
func TestGitBackend_TagPush_DiffRevNoMoveFlag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	// Round 1: tag at HEAD~1, push it.
	parentSHA := localParentSHA(t, work)
	runIn(t, work, "git", "tag", "v1.0.0", "HEAD~1")
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	withCwd(t, work, func() {
		b := &gitBackend{}
		// Attempt move to HEAD without flag → exit 4.
		err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "HEAD", Remote: "origin",
		})
		if err == nil {
			t.Fatal("diff-rev TagPush without --allow-move should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exitCodeAmbiguous (4), got: %v", err)
		}
	})
	// Bare must still point at the original REV.
	if got := tagOnBare(t, bare, "v1.0.0"); got != parentSHA {
		t.Errorf("bare v1.0.0 = %q, want %q (unchanged)", got, parentSHA)
	}
}

// TestGitBackend_TagPush_DiffRevAllowMove: with `--allow-move=true`, the
// move is permitted; bare ends up pointing at the new REV.
func TestGitBackend_TagPush_DiffRevAllowMove(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	runIn(t, work, "git", "tag", "v1.0.0", "HEAD~1")
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "HEAD", Remote: "origin", AllowMove: true,
		}); err != nil {
			t.Fatalf("TagPush(--allow-move): %v", err)
		}
	})
	want := localHeadSHA(t, work)
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q (moved to HEAD)", got, want)
	}
}

// TestGitBackend_TagPush_BadRev: unresolvable REV surfaces as exitCodeVCSExec
// (3) — distinct from the integrity-violation exit 4 so callers can branch.
func TestGitBackend_TagPush_BadRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "definitely-not-a-rev", Remote: "origin",
		})
		if err == nil {
			t.Fatal("TagPush with bad REV should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// TestGitBackend_TagPush_BadRemote: unknown remote → exit 3.
func TestGitBackend_TagPush_BadRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "HEAD", Remote: "nonexistent",
		})
		if err == nil {
			t.Fatal("TagPush to nonexistent remote should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// --- git TagDelete ---------------------------------------------------------

// TestGitBackend_TagDelete_PresentTag: a tag present locally and on the
// bare is removed from both.
func TestGitBackend_TagDelete_PresentTag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	runIn(t, work, "git", "tag", "v0.9.0", "HEAD")
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v0.9.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	if tagOnBare(t, bare, "v0.9.0") == "" {
		t.Fatal("setup invariant: bare should have v0.9.0 before delete")
	}
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "v0.9.0", Remote: "origin"}); err != nil {
			t.Fatalf("TagDelete: %v", err)
		}
	})
	if got := tagOnBare(t, bare, "v0.9.0"); got != "" {
		t.Errorf("bare should not have v0.9.0 after delete, got %q", got)
	}
	// Local should also be gone.
	out, _ := runBackendCmdIn(work, "git", "tag", "--list", "v0.9.0")
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("local should not have v0.9.0 after delete, got %q", string(out))
	}
}

// TestGitBackend_TagDelete_AbsentIdempotent: deleting an absent tag is a
// no-op success (rm -f semantic). Critical: git's bare `git tag -d NAME`
// errors when the tag is missing, so the backend MUST pre-check existence.
func TestGitBackend_TagDelete_AbsentIdempotent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "never-existed", Remote: "origin"}); err != nil {
			t.Errorf("absent TagDelete should succeed (rm -f semantic), got: %v", err)
		}
	})
}

// TestGitBackend_TagDelete_LocalOnly: local has the tag, bare doesn't.
// Both halves of the delete must short-circuit cleanly: the remote push
// of `:refs/tags/NAME` reports "deleting a non-existent ref" but exits 0,
// so the backend can run it unconditionally without breaking idempotence.
func TestGitBackend_TagDelete_LocalOnly(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "git", "tag", "local-only", "HEAD")
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "local-only", Remote: "origin"}); err != nil {
			t.Errorf("TagDelete (local-only) should succeed, got: %v", err)
		}
	})
	out, _ := runBackendCmdIn(work, "git", "tag", "--list", "local-only")
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("local should not have local-only after delete, got %q", string(out))
	}
	if got := tagOnBare(t, bare, "local-only"); got != "" {
		t.Errorf("bare should not have local-only, got %q", got)
	}
}

// TestGitBackend_TagDelete_BadRemote: unknown remote on the remote-delete
// half → exit 3. The local half already ran (idempotent) but the remote
// failure surfaces.
func TestGitBackend_TagDelete_BadRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "git", "tag", "v9.0.0", "HEAD")
	withCwd(t, work, func() {
		b := &gitBackend{}
		err := b.TagDelete(tagDeleteOpts{Name: "v9.0.0", Remote: "nonexistent"})
		if err == nil {
			t.Fatal("TagDelete to nonexistent remote should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec, got: %v", err)
		}
	})
}

// --- jj TagPush ------------------------------------------------------------

// TestGitBackend_Push_AutoAdvance_SilentNoOp: PR-5.2.1 (backend-prefix
// general rule) — when --jj-bookmark-auto-advance reaches the git backend
// it is a **silent no-op** (the `--jj-` prefix structurally tells the
// user it's jj-only, so git simply ignores it and runs a normal push).
// PR-5.2 previously rejected here at exit 3 as a defensive guard; the new
// contract is "git ignores jj-prefixed flags", verified by this test.
func TestGitBackend_Push_AutoAdvance_SilentNoOp(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &gitBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true}); err != nil {
			t.Fatalf("gitBackend.Push with jjBookmarkAutoAdvance must silently no-op + push, got: %v", err)
		}
	})
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(bareSHA)) == "" {
		t.Errorf("bare should have main after silent no-op push")
	}
}
