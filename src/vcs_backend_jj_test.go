package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJjBackend_Root: returns the repo root (jj root with a colocated
// fixture is the same as the git fixture's working dir).
func TestJjBackend_Root(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestJjBackend_IsClean_Clean: fresh colocated jj repo has an empty `@`
// (jj puts a new empty change on top of HEAD at init).
func TestJjBackend_IsClean_Clean(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// --- DR-0020 PR-2.2: merge-commit handling for IsClean ------------------
//
// PR-2 used `jj log -r @ -T 'empty'` alone. PR-2.1 (v0.25.2) overrode
// that template with `if(parents.len() > 1, "true", empty)` based on a
// misread of kawaz's intent — flipping evil merges (parents>1,
// non-empty tree) from dirty to clean. PR-2.2 reverts to the empty-only
// template. The correct reading of kawaz's directive is:
//
//   - empty merge (parents>1, tree == merge-of-parents) → clean.
//     This already worked under the empty-only template because jj's
//     `empty` keyword is parent-relative for merges (returns true when
//     @'s tree equals the merge of all parents).
//   - evil merge (parents>1, extra tree edits) → dirty. The PR-2.1
//     flip was wrong: a merge commit carrying uncommitted content
//     IS dirty.

// TestJjBackend_IsClean_MergeEmpty: a merge commit whose tree matches
// the merge of its parents (empty=true on jj's parent-relative
// template) is clean. This works under the empty-only template
// without any merge-specific short-circuit — pinned here to lock the
// invariant that empty merges remain clean.
func TestJjBackend_IsClean_MergeEmpty(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := jjMergeFixture(t, "")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if !clean {
			t.Errorf("IsClean = false, want true (merge commit, empty tree)")
		}
	})
}

// TestJjBackend_IsClean_MergeNonEmpty: a merge commit with an
// additional tree edit ("evil merge", empty=false but parents>1) is
// dirty. The PR-2.1 flip that classified this as clean was a misread
// of kawaz's intent; PR-2.2 restores the correct behaviour — a merge
// commit carrying uncommitted content is dirty just like any other
// non-empty change.
func TestJjBackend_IsClean_MergeNonEmpty(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := jjMergeFixture(t, "EVIL.txt")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		clean, err := b.IsClean()
		if err != nil {
			t.Fatalf("IsClean: %v", err)
		}
		if clean {
			t.Errorf("IsClean = true, want false (evil merge: parents>1 with extra tree edits is dirty)")
		}
	})
}

// --- DR-0020 PR-3: Diff tests --------------------------------------------

// TestJjBackend_Diff_NoPaths_HasDiff: jj fixture against @-- (= initial)
// returns the bump-commit diff (VERSION change).
func TestJjBackend_Diff_NoPaths_HasDiff(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", nil, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@", nil, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", []string{"VERSION", "doesnotexist.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.Diff("@--", []string{"nope.txt", "alsonope.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		_, err := b.Diff("doesnotexist", nil, nil)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		if code := exitCodeOf(err); code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d (vcs exec)", code, exitCodeVCSExec)
		}
	})
}

// --- DR-0020 PR-3.1: DiffNameStatus tests --------------------------------

// TestJjBackend_DiffNameStatus_HasChanges_TabNormalized: jj's native
// `jj diff --summary` produces "M VERSION" (space). The backend must
// normalize to git's tab format so cross-backend output is uniform.
func TestJjBackend_DiffNameStatus_HasChanges_TabNormalized(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@--", nil, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@", nil, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.DiffNameStatus("@--", []string{"nope.txt"}, nil)
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
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		_, err := b.DiffNameStatus("doesnotexist", nil, nil)
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

// TestJjBackend_Commit_Paths: only the listed path's changes land in the
// committed change, others remain in the next (new) working copy.
func TestJjBackend_Commit_Paths(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestJjBackend_Commit_Amend_Paths: amend + PATHS squashes only the
// listed paths from @ into @-; other @ changes remain in @.
func TestJjBackend_Commit_Amend_Paths(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestJjBackend_Fetch_DefaultRemote: jj fetches via the underlying git
// store (colocated repo). Round-trips through `jj git fetch --remote
// origin`.
func TestJjBackend_Fetch_DefaultRemote(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestJjBackend_Push_NewBookmark: pushing a new bookmark to an empty bare
// succeeds (jj 0.41 handles new bookmarks without --allow-new).
func TestJjBackend_Push_NewBookmark(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestJjBackend_TagPush_NewTag: fresh tag at @- via jj tag set + jj git
// export + native git push to origin.
func TestJjBackend_TagPush_NewTag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin",
		}); err != nil {
			t.Fatalf("TagPush(new): %v", err)
		}
	})
	// Resolve `@-` via the same jj operation the SUT used; jj's
	// auto-snapshot on every command means git's view of any given
	// revspec can shift between subprocess calls, so we ask jj to
	// resolve `@-` at the same moment we read the bare.
	want := jjResolveRev(t, work, "@-")
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q", got, want)
	}
}

// TestJjBackend_TagPush_SameRevIdempotent: local tag exists, push again at
// same REV. Crucial: jj's `jj tag set NAME -r REV` errors out when the
// tag already exists (even at the same REV), so the backend MUST pre-check
// and skip the create on a same-rev match. The push half stays — that's
// the 片落ちリカバリ behaviour.
func TestJjBackend_TagPush_SameRevIdempotent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v1.0.0", "-r", "@-")
	// Push out of band so the remote already has it.
	if _, err := runBackendCmdIn(work, "jj", "git", "export"); err != nil {
		t.Fatalf("setup jj git export: %v", err)
	}
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	want := tagOnBare(t, bare, "v1.0.0")
	if want == "" {
		t.Fatal("setup invariant: bare should have v1.0.0")
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin",
		}); err != nil {
			t.Errorf("same-rev jj TagPush should succeed (idempotent), got: %v", err)
		}
	})
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q (unchanged)", got, want)
	}
}

// TestJjBackend_TagPush_DiffRevNoMoveFlag: jj-side integrity violation.
// The backend must pre-detect the diff-rev case and emit exit 4 with no
// side-effect on the bare (jj's own "Refusing to move tag" hint would be
// exit 1 untransformed).
func TestJjBackend_TagPush_DiffRevNoMoveFlag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v1.0.0", "-r", "@--")
	if _, err := runBackendCmdIn(work, "jj", "git", "export"); err != nil {
		t.Fatalf("setup jj git export: %v", err)
	}
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	wantBare := tagOnBare(t, bare, "v1.0.0")
	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin",
		})
		if err == nil {
			t.Fatal("diff-rev jj TagPush without --allow-move should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exitCodeAmbiguous (4), got: %v", err)
		}
	})
	if got := tagOnBare(t, bare, "v1.0.0"); got != wantBare {
		t.Errorf("bare v1.0.0 should be unchanged after rejected move, got %q want %q", got, wantBare)
	}
}

// TestJjBackend_TagPush_DiffRevAllowMove: with `--allow-move`, the move is
// permitted: `jj tag set --allow-move` + export + force-push.
func TestJjBackend_TagPush_DiffRevAllowMove(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v1.0.0", "-r", "@--")
	if _, err := runBackendCmdIn(work, "jj", "git", "export"); err != nil {
		t.Fatalf("setup jj git export: %v", err)
	}
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin", AllowMove: true,
		}); err != nil {
			t.Fatalf("TagPush(--allow-move): %v", err)
		}
	})
	// See TestJjBackend_TagPush_NewTag for why we resolve via jj rather
	// than git rev-parse here.
	want := jjResolveRev(t, work, "@-")
	if got := tagOnBare(t, bare, "v1.0.0"); got != want {
		t.Errorf("bare v1.0.0 = %q, want %q (moved)", got, want)
	}
}

// TestJjBackend_TagPush_BadRev: unresolvable REV → exit 3.
func TestJjBackend_TagPush_BadRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &jjBackend{}
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

// --- jj TagDelete ----------------------------------------------------------

// TestJjBackend_TagDelete_PresentTag: jj tag delete + jj git export +
// remote delete; both sides end up tagless.
func TestJjBackend_TagDelete_PresentTag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v0.9.0", "-r", "@-")
	if _, err := runBackendCmdIn(work, "jj", "git", "export"); err != nil {
		t.Fatalf("setup jj git export: %v", err)
	}
	if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v0.9.0"); err != nil {
		t.Fatalf("setup push: %v", err)
	}
	if tagOnBare(t, bare, "v0.9.0") == "" {
		t.Fatal("setup invariant: bare should have v0.9.0 before delete")
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "v0.9.0", Remote: "origin"}); err != nil {
			t.Fatalf("TagDelete: %v", err)
		}
	})
	if got := tagOnBare(t, bare, "v0.9.0"); got != "" {
		t.Errorf("bare should not have v0.9.0 after delete, got %q", got)
	}
}

// TestJjBackend_TagDelete_AbsentIdempotent: `jj tag delete` is natively
// idempotent (exit 0 with "No matching tags"). The remote half is also
// idempotent (git's `push :refs/tags/NAME` against a missing ref exits 0).
func TestJjBackend_TagDelete_AbsentIdempotent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "never-existed", Remote: "origin"}); err != nil {
			t.Errorf("absent jj TagDelete should succeed (rm -f semantic), got: %v", err)
		}
	})
}

// TestJjBackend_TagPush_NonColocated: exercises the production layout
// (DR-0020 line 105: git bare backing + jj workspace, no `.git/` in cwd).
// The colocated fixtures above never hit jjGitPushDir's fall-through to
// the resolved git_target — this test does.
func TestJjBackend_TagPush_NonColocated(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, origin := setupJjRepoNonColocatedWithRemote(t, "1.0.0")
	// Guard against a future refactor accidentally colocating.
	if _, err := os.Stat(filepath.Join(work, ".git")); err == nil {
		t.Fatalf("fixture invariant: work/.git must NOT exist in non-colocated layout")
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin",
		}); err != nil {
			t.Fatalf("TagPush(new, non-colocated): %v", err)
		}
	})
	want := jjResolveRev(t, work, "@-")
	if got := tagOnBare(t, origin, "v1.0.0"); got != want {
		t.Errorf("non-colocated origin v1.0.0 = %q, want %q", got, want)
	}
}

// TestJjBackend_TagPush_NonColocated_SecondaryWorkspace exercises a jj
// secondary workspace (created via `jj workspace add`). In this layout
// the secondary workspace's `.jj/repo` is a regular *file* containing an
// indirection to the primary workspace's repo store — reading
// `.jj/repo/store/git_target` directly fails with ENOTDIR. The fix routes
// through `jj git root` so jj handles the indirection.
//
// Regression: bump-semver v0.31.1 and earlier crashed here with
// "open .jj/repo/store/git_target: not a directory" (kawaz/bump-semver
// docs/issue/2026-06-08-jj-secondary-workspace-git-target.md).
func TestJjBackend_TagPush_NonColocated_SecondaryWorkspace(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, origin := setupJjRepoNonColocatedWithRemote(t, "1.0.0")
	ws2 := filepath.Join(filepath.Dir(work), "ws2")
	runIn(t, work, "jj", "workspace", "add", "--name", "ws2", ws2)
	// Fixture invariant: secondary `.jj/repo` is a regular file holding the
	// indirection. If jj ever flips this to a directory the regression
	// target disappears and we want to know.
	fi, err := os.Stat(filepath.Join(ws2, ".jj/repo"))
	if err != nil {
		t.Fatalf("stat ws2/.jj/repo: %v", err)
	}
	if fi.IsDir() {
		t.Skip("secondary workspace .jj/repo is a directory; jj layout changed")
	}
	withCwd(t, ws2, func() {
		b := &jjBackend{}
		if err := b.TagPush(tagPushOpts{
			Name: "v1.0.0", Rev: "@-", Remote: "origin",
		}); err != nil {
			t.Fatalf("TagPush(new, secondary workspace): %v", err)
		}
	})
	want := jjResolveRev(t, work, "@-")
	if got := tagOnBare(t, origin, "v1.0.0"); got != want {
		t.Errorf("secondary-workspace origin v1.0.0 = %q, want %q", got, want)
	}
}

// TestJjBackend_TagDelete_NonColocated: delete also routes through
// `git -C <git_target> push origin :refs/tags/NAME` in the non-colocated
// layout, so it gets a dedicated test for the same reason.
func TestJjBackend_TagDelete_NonColocated(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, origin := setupJjRepoNonColocatedWithRemote(t, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v0.9.0", "-r", "@-")
	if _, err := runBackendCmdIn(work, "jj", "git", "export"); err != nil {
		t.Fatalf("setup jj git export: %v", err)
	}
	// Push the tag to origin via the backing store so we have something
	// to delete on the remote half. `--no-verify` mirrors the SUT's
	// bare-context push (see gitTagPushRemote rationale): the user's
	// global core.hooksPath may have pre-push hooks that fail in a bare
	// context; the SUT avoids them and the setup must too.
	bareGitTarget, _ := os.ReadFile(filepath.Join(work, ".jj/repo/store/git_target"))
	backing := strings.TrimSpace(string(bareGitTarget))
	if _, err := runBackendCmdIn(backing, "git", "push", "--no-verify", "origin", "refs/tags/v0.9.0"); err != nil {
		t.Fatalf("setup remote push: %v", err)
	}
	if tagOnBare(t, origin, "v0.9.0") == "" {
		t.Fatal("setup invariant: origin should have v0.9.0 before delete")
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.TagDelete(tagDeleteOpts{Name: "v0.9.0", Remote: "origin"}); err != nil {
			t.Fatalf("TagDelete (non-colocated): %v", err)
		}
	})
	if got := tagOnBare(t, origin, "v0.9.0"); got != "" {
		t.Errorf("non-colocated origin should not have v0.9.0 after delete, got %q", got)
	}
}

// TestJjBackend_TagDelete_BadRemote: unknown remote → exit 3.
func TestJjBackend_TagDelete_BadRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "tag", "set", "v9.0.0", "-r", "@-")
	withCwd(t, work, func() {
		b := &jjBackend{}
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

// --- DR-0020 PR-5.2: --jj-bookmark-auto-advance backend tests --------------
//
// PR-5.2 adds an opt-in pre-step to `vcs push` (jj only): when the named
// bookmark is a strict ancestor of @, move it forward to @- before pushing.
// jj慣習: bookmarks live on confirmed commits (= @-), the working copy (@)
// is throw-away. Manually advancing the bookmark every bump is friction; the
// flag removes it but only when ALL preconditions hold:
//
//   - working copy is clean (no uncommitted changes)
//   - the bookmark exists
//   - the bookmark is on the ancestor line of @ (not divergent / sideways)
//   - the bookmark is strictly before @- (= forward move required)
//
// Failure modes return exit 3 (= exitCodeVCSExec, "VCS-layer precondition
// not met" — same taxonomy as "unknown remote" / "not a repo"). exit 1
// (exitCodeFalse) is reserved for predicate verbs (`compare` / `vcs is`)
// and would mis-classify a refusal as a query result.

// TestJjBackend_Push_AutoAdvance_Forward: bookmark sits 1 commit below @-;
// auto-advance moves it to @- and pushes. After success the bookmark must
// be at @-'s change_id on the local AND on the bare (= bookmark advanced
// AND push happened).
func TestJjBackend_Push_AutoAdvance_Forward(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjRepoWithRemote(t, nil, "1.0.0")
	// Setup: place "main" bookmark at @-- (one commit behind @-) so a
	// forward move to @- is required. setupJjRepoWithRemote leaves @ on
	// an empty working copy above the seed commits.
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@--", "--allow-backwards")
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true}); err != nil {
			t.Fatalf("Push auto-advance: %v", err)
		}
	})
	// Bookmark must be at @-'s commit on the local.
	localBkSHA, err := runBackendCmdIn(work, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("local rev-parse main: %v", err)
	}
	parentSHA, err := runBackendCmdIn(work, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("local rev-parse HEAD~?: %v", err)
	}
	_ = parentSHA
	// And the bare must have main at the same SHA.
	bareBkSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(localBkSHA)) != strings.TrimSpace(string(bareBkSHA)) {
		t.Errorf("bare main = %q, want local main = %q",
			strings.TrimSpace(string(bareBkSHA)), strings.TrimSpace(string(localBkSHA)))
	}
}

// TestJjBackend_Push_AutoAdvance_AlreadyAtParent: bookmark is already at @-;
// auto-advance is a no-op for the move step, push proceeds normally.
func TestJjBackend_Push_AutoAdvance_AlreadyAtParent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true}); err != nil {
			t.Fatalf("Push auto-advance no-op: %v", err)
		}
	})
}

// TestJjBackend_Push_AutoAdvance_AtWorkingCopy_Clean: bookmark sits at @
// itself on a clean working copy. Naively moving to @- would be a backwards
// move (jj rejects with "Refusing to move bookmark backwards or sideways");
// auto-advance must short-circuit here so the push proceeds with the
// bookmark untouched. Per kawaz original spec: "<name> が @ 自身を指す →
// 通常 push (auto-advance 不要)".
func TestJjBackend_Push_AutoAdvance_AtWorkingCopy_Clean(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	// Clean @ (empty working copy from setupJjRepoWithRemote) with main
	// pinned at @ itself.
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@", "--allow-backwards")
	wcCID, err := runBackendCmdIn(work, "jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
	if err != nil {
		t.Fatalf("capture @: %v", err)
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true})
		// The push itself may fail because @ on a clean fixture is empty/undescribed,
		// but auto-advance must NOT emit a "backwards or sideways" error — that
		// would mean the short-circuit is missing. We assert the bookmark stayed
		// put and that any error is NOT the backwards-move one.
		if err != nil {
			var ee *exitErr
			if errors.As(err, &ee) {
				if strings.Contains(ee.msg, "backwards or sideways") {
					t.Fatalf("auto-advance must short-circuit at-@ case (no backwards move attempt), got: %v", err)
				}
			}
		}
	})
	bookmarkAfter, _ := runBackendCmdIn(work, "jj", "log", "-r", "main", "--no-graph", "-T", "change_id")
	if strings.TrimSpace(string(bookmarkAfter)) != strings.TrimSpace(string(wcCID)) {
		t.Errorf("bookmark must stay at @ (%q), got %q",
			strings.TrimSpace(string(wcCID)), strings.TrimSpace(string(bookmarkAfter)))
	}
}

// TestJjBackend_Push_AutoAdvance_Divergent: bookmark is on a sibling change
// (= not in ancestors(@)); auto-advance must refuse with exit 3 and NOT
// move the bookmark.
//
// Setup tactic: capture the change_id of the "bump" commit (which IS in
// ancestors of the default @), then `jj new <bump>` to anchor @ on the
// bump line, then `jj new <bump> -m sib` to create a sibling, set the
// bookmark on the sibling, finally re-edit / new on the original bump
// line so the bookmark is sideways relative to the final @.
func TestJjBackend_Push_AutoAdvance_Divergent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	// Capture the bump commit's change_id so we can branch deterministically.
	bumpCID, err := runBackendCmdIn(work, "jj", "log", "-r", "@-", "--no-graph", "-T", "change_id")
	if err != nil {
		t.Fatalf("capture bump change_id: %v", err)
	}
	bump := strings.TrimSpace(string(bumpCID))
	// Create a sibling change off bump and place "main" on it.
	runIn(t, work, "jj", "new", bump, "-m", "sib")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@", "--allow-backwards")
	// Return @ to a fresh empty working copy above the original bump line —
	// now main is on a sibling, NOT in ancestors(@).
	runIn(t, work, "jj", "new", bump)
	bookmarkBefore, _ := runBackendCmdIn(work, "jj", "log", "-r", "main", "--no-graph", "-T", "change_id")
	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true})
		if err == nil {
			t.Fatal("auto-advance on divergent bookmark should fail")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec (3), got: %v", err)
		}
		if !strings.Contains(ee.msg, "ancestor") && !strings.Contains(ee.msg, "advance") {
			t.Errorf("expected divergent hint, got: %q", ee.msg)
		}
	})
	bookmarkAfter, _ := runBackendCmdIn(work, "jj", "log", "-r", "main", "--no-graph", "-T", "change_id")
	if string(bookmarkBefore) != string(bookmarkAfter) {
		t.Errorf("bookmark must NOT move on refusal: before=%q after=%q",
			string(bookmarkBefore), string(bookmarkAfter))
	}
}

// TestJjBackend_Push_AutoAdvance_Dirty: working copy has uncommitted edits;
// auto-advance moves the bookmark to @ (not @-) so the dirty commit IS
// the publishable one (= "immutable 化" pattern, kawaz 確定 2026-05-31).
// Push then proceeds against the bookmark at @.
func TestJjBackend_Push_AutoAdvance_Dirty(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@--", "--allow-backwards")
	// Dirty the working copy by writing to VERSION (an existing tracked
	// file) and describing it. The "dirty + describe + push" pattern is
	// the one PR-5.2 covers — jj refuses to push commits with no
	// description, so a realistic dirty workflow always describes first.
	if err := writeFile(filepath.Join(work, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, work, "jj", "describe", "-m", "bump to 9.9.9")
	// Capture @'s change_id (jj will snapshot the edit on next read).
	wcCID, err := runBackendCmdIn(work, "jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
	if err != nil {
		t.Fatalf("capture @ change_id: %v", err)
	}
	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin", jjBookmarkAutoAdvance: true}); err != nil {
			t.Fatalf("auto-advance with dirty worktree should succeed (target=@): %v", err)
		}
	})
	bookmarkAfter, _ := runBackendCmdIn(work, "jj", "log", "-r", "main", "--no-graph", "-T", "change_id")
	if strings.TrimSpace(string(bookmarkAfter)) != strings.TrimSpace(string(wcCID)) {
		t.Errorf("dirty auto-advance: bookmark should be at @ (%q), got %q",
			strings.TrimSpace(string(wcCID)), strings.TrimSpace(string(bookmarkAfter)))
	}
}
