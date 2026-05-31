package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VcsCommit_NoArgs: `vcs commit` with no args shows the help.
func TestRun_VcsCommit_NoArgs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "commit"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs commit (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "commit") || !strings.Contains(stdout.String(), "--staged") {
		t.Errorf("expected vcs commit help mentioning commit/--staged, got: %q", stdout.String())
	}
}

// TestRun_VcsCommit_NoMessage_NoAmend: `-m` is required unless --amend.
// Missing message (and no --amend) → exit 2.
func TestRun_VcsCommit_NoMessage_NoAmend(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "VERSION"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error when -m is missing")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_DashA_Rejected: `-a` is intentionally not supported.
// DR-0020 makes this an opinionated safety rejection — exit 2 with a
// hint that names the supported modes (--staged / PATH).
func TestRun_VcsCommit_DashA_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-a", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected -a to be rejected")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "--staged") && !strings.Contains(stderr.String(), "PATH") {
			t.Errorf("error should hint at --staged / PATH, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_PathAndStaged_Rejected: path + --staged is ambiguous,
// must reject with exit 2.
func TestRun_VcsCommit_PathAndStaged_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "--staged", "-m", "x", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --staged + PATH to reject")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_NoMode_DynamicHint_Git: when no PATH and no --staged
// and no --amend, the error hint must come from backend.Kind(); git tells
// the user to use --staged or pass a PATH.
func TestRun_VcsCommit_NoMode_DynamicHint_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for no mode")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "--staged") {
			t.Errorf("git hint should mention --staged, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_NoMode_DynamicHint_Jj: jj's hint must explicitly say
// that `-a` is not supported (kawaz CLI design safety).
func TestRun_VcsCommit_NoMode_DynamicHint_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for no mode")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		// jj users typically reach for `-a`; the hint should explicitly
		// say it's not supported and tell them to name a PATH.
		if !strings.Contains(stderr.String(), "PATH") {
			t.Errorf("jj hint should mention PATH, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_Paths_Git: end-to-end happy path on git — modify
// VERSION, run `vcs commit -m MSG VERSION`, HEAD advances and worktree
// is clean.
func TestRun_VcsCommit_Paths_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "-m", "bump", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit -m bump VERSION: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "bump" {
			t.Errorf("HEAD subject = %q, want 'bump'", got)
		}
	})
}

// TestRun_VcsCommit_Paths_NonexistentOnly_Idempotent: all-nonexistent
// PATH list → exit 0, no HEAD movement (declarative convergence).
func TestRun_VcsCommit_Paths_NonexistentOnly_Idempotent(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("git", "rev-parse", "HEAD")
		err := run([]string{"vcs", "commit", "-m", "ghost", "no-such.txt"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for nonexistent-only, got: %v", err)
		}
		after, _ := runBackendCmd("git", "rev-parse", "HEAD")
		if string(before) != string(after) {
			t.Errorf("HEAD should not advance, before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestRun_VcsCommit_Staged_Git: --staged commits the index in one shot.
func TestRun_VcsCommit_Staged_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "--staged", "-m", "bump-all"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --staged: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "bump-all" {
			t.Errorf("HEAD subject = %q, want 'bump-all'", got)
		}
	})
}

// TestRun_VcsCommit_Amend_Git: --amend with -m updates the last commit.
func TestRun_VcsCommit_Amend_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "--amend", "-m", "amended"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --amend: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "amended" {
			t.Errorf("HEAD subject after amend = %q, want 'amended'", got)
		}
	})
}

// TestRun_VcsCommit_Paths_Jj: jj end-to-end happy path mirrors the git
// test — modify VERSION, commit, @- now describes the commit.
func TestRun_VcsCommit_Paths_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "-m", "bump", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit -m bump VERSION (jj): %v", err)
		}
		out, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(out)); got != "bump" {
			t.Errorf("@- description = %q, want 'bump'", got)
		}
	})
}

// TestRun_VcsCommit_Amend_WithPath_Git: `--amend PATH..` folds only
// the listed paths into the previous commit (DR-0020 PR-4.1 — commit /
// amend symmetry: the only difference is "new commit vs absorb into
// previous", not which path modes are accepted).
func TestRun_VcsCommit_Amend_WithPath_Git(t *testing.T) {
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
		err := run([]string{"vcs", "commit", "--amend", "-m", "amended+path", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --amend PATH: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "amended+path" {
			t.Errorf("HEAD subject after amend = %q, want 'amended+path'", got)
		}
		// other.txt must remain untracked (not folded into HEAD).
		stat, _ := runBackendCmd("git", "status", "--short")
		if !strings.Contains(string(stat), "other.txt") {
			t.Errorf("other.txt should remain dirty after path-scoped amend, status=%q", string(stat))
		}
	})
}

// TestRun_VcsCommit_Amend_WithStaged_Git: `--amend --staged` folds the
// entire current change (= bare amend behaviour, since the staged index
// IS the fold-into-previous source for git) and is accepted as an
// explicit synonym for bare `--amend` (DR-0020 PR-4.1 symmetry).
func TestRun_VcsCommit_Amend_WithStaged_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "--amend", "--staged", "-m", "amended+staged"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --amend --staged: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "amended+staged" {
			t.Errorf("HEAD subject after amend --staged = %q, want 'amended+staged'", got)
		}
	})
}

// TestRun_VcsCommit_Amend_PathAndStaged_Rejected: `--amend PATH..
// --staged` is still rejected (step 2 / path+staged exclusivity is
// amend-agnostic — only step 3.5 was removed for PR-4.1).
func TestRun_VcsCommit_Amend_PathAndStaged_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "--amend", "--staged", "-m", "x", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --amend --staged PATH triple-combo to reject")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_NotARepo: outside any vcs repo, `vcs commit` should
// surface exit 3 (newVcsBackend failure), consistent with get/is/diff.
func TestRun_VcsCommit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestHelpVcsCommit_BareAmendIsBackendSplit: PR-4.1 advisor noted that
// "fold ALL current changes" is wrong for git (the index is what amend
// folds, not unstaged tree state). PR-5.1 splits the bare `--amend`
// line into git/jj-specific phrasing, mirroring the `--staged` block
// just above it.
func TestHelpVcsCommit_BareAmendIsBackendSplit(t *testing.T) {
	body := helpVcsCommit
	if strings.Contains(body, "fold ALL current changes") {
		t.Errorf("PR-5.1 replaces 'fold ALL current changes' with backend-split phrasing, "+
			"but the old wording is still present: %q", body)
	}
	// The new wording should explicitly call out git's index scope (the
	// kawaz/advisor correctness fix) and jj's @-snapshot scope.
	if !strings.Contains(body, "git: ") || !strings.Contains(body, "jj: ") {
		t.Errorf("helpVcsCommit bare-amend block should split into git/jj rows like --staged, got: %q", body)
	}
}

// --- C: jj git export retry seam (unit-level) ----------------------------
//
// Real jj cannot be coerced into a "fail once, succeed second" pattern
// on demand, so we expose the export call as a package-level function
// variable (`jjGitExportFunc`) that tests can override. Two cases:
//   - first call fails, second succeeds → Push returns nil (retry worked)
//   - both calls fail               → Push returns exit 3 + recovery hint
