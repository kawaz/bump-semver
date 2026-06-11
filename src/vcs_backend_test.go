package main

import (
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	withCwd(t, dir, func() {
		_, err := newVcsBackend(vcsAuto)
		if err == nil {
			t.Fatal("expected error in non-vcs directory")
		}
	})
}

// jjMergeFixture builds on setupJjRepo to produce a colocated jj repo
// whose @ is a real merge commit. It creates two side changes (with
// description "branchA" / "branchB") off the bookmarked HEAD, then
// `jj new` over both. If extraFile is non-empty, that file is written
// into @ AFTER the merge so the resulting @ is non-empty (= evil
// merge); otherwise @ stays empty (= clean merge).
func jjMergeFixture(t *testing.T, extraFile string) string {
	t.Helper()
	dir := setupJjRepo(t, nil, "1.0.0")
	// Two side changes off the current @-.
	runIn(t, dir, "jj", "new", "-m", "branchA", "@-")
	runIn(t, dir, "jj", "bookmark", "create", "side-a", "-r", "@")
	runIn(t, dir, "jj", "new", "-m", "branchB", "@-")
	runIn(t, dir, "jj", "bookmark", "create", "side-b", "-r", "@")
	// Merge them; @ becomes a commit with parents=2.
	runIn(t, dir, "jj", "new", "-m", "merge AB", "side-a", "side-b")
	if extraFile != "" {
		if err := os.WriteFile(filepath.Join(dir, extraFile), []byte("evil\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
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

// runBackendCmdIn is a test-only helper that runs name/args in dir and
// returns the trimmed output. Mirrors runBackendCmd (which uses cwd)
// but lets us inspect a bare repo without chdir-ing.
func runBackendCmdIn(dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

// --- DR-0020 PR-6: TagPush / TagDelete backend tests ----------------------
//
// Fixtures reuse the bare-as-origin pattern from PR-5: a local bare repo
// at a sibling path of the workdir, set as `origin`. Tags push end-to-end
// without any network. Like PR-5, success-path exit codes are covered at
// the dispatcher level (main_test.go); these tests focus on the four
// behavioural matrices that span both backends:
//
//   - new tag                       → exit 0, tag visible on bare
//   - same NAME @ same REV          → exit 0 (idempotent reconciliation),
//                                     the 片落ちリカバリ case in DR-0020 line 71:
//                                     local has the tag, remote may not, so
//                                     create-skip but still push
//   - same NAME @ diff REV no flag  → exit 4 (integrity violation, distinct
//                                     from generic "VCS failed" so callers
//                                     can branch on it)
//   - same NAME @ diff REV w/ flag  → exit 0 (move + force-push to remote)
//   - unresolvable REV              → exit 3 (caller bug surfaces, not a
//                                     "tag already there" red herring)
//   - delete present tag            → exit 0, bare loses the tag
//   - delete absent tag             → exit 0 (rm -f semantic per DR-0020
//                                     line 74; the verb's intent is the
//                                     end-state, not the transition)

// jjResolveRev runs `jj log --no-graph -r REV -T commit_id` in dir and
// returns the SHA jj currently sees for REV. This matters in test
// assertions because jj auto-snapshots on every command, so git's view
// of any given revspec can drift between subprocess calls — asking jj
// for the SHA at the same moment as the assertion keeps the comparison
// honest.
func jjResolveRev(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := runBackendCmdIn(dir, "jj", "log", "--no-graph",
		"-r", rev, "-T", `commit_id ++ "\n"`)
	if err != nil {
		t.Fatalf("jj log -r %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

// tagOnBare returns the commit SHA the bare repo's NAME tag points at, or
// "" when the bare has no such tag. We use `git -C bare show-ref` rather
// than `rev-parse` so a missing ref returns empty rather than erroring —
// that lets the assertion side stay declarative.
func tagOnBare(t *testing.T, bare, name string) string {
	t.Helper()
	out, err := runBackendCmdIn(bare, "git", "show-ref", "--tags", "-s", name)
	if err != nil {
		// `show-ref` exits 1 when the ref is absent; treat as "no tag".
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localHeadSHA returns the work-dir HEAD's commit SHA, for cross-checking
// that a pushed tag actually points where we expect.
func localHeadSHA(t *testing.T, work string) string {
	t.Helper()
	out, err := runBackendCmdIn(work, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("local rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// localParentSHA returns HEAD~1's SHA for tests that want to move a tag
// to a different rev.
func localParentSHA(t *testing.T, work string) string {
	t.Helper()
	out, err := runBackendCmdIn(work, "git", "rev-parse", "HEAD~1")
	if err != nil {
		t.Fatalf("local rev-parse HEAD~1: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// --- git TagPush -----------------------------------------------------------
