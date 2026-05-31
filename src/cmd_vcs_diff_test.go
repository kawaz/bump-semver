package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestRun_VcsDiff_NoArgs: `vcs diff` with no REV shows the vcs-diff help
// (matches the no-args convention used by `vcs get` and `vcs is`).
func TestRun_VcsDiff_NoArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff (no args): %v", err)
		}
		if !strings.Contains(stdout.String(), "REV") {
			t.Errorf("expected vcs diff help mentioning REV, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_HasDiff: `vcs diff HEAD~1` on the fixture prints a
// patch covering VERSION on stdout and exits 0.
func TestRun_VcsDiff_Git_HasDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected stdout to include VERSION patch, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_NoDiff: `vcs diff HEAD` on a clean fixture produces
// no stdout, exits 0.
func TestRun_VcsDiff_Git_NoDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD: %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (no diff), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Git_PathFilter: `vcs diff REV VERSION nope.txt` returns
// the VERSION diff and silently ignores the nonexistent path.
func TestRun_VcsDiff_Git_PathFilter(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "VERSION", "nope.txt"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD~1 VERSION nope.txt: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected VERSION in diff, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_AllPathsNonexistent: every path filtered out → empty
// stdout, exit 0. Must NOT fall through to "diff everything".
func TestRun_VcsDiff_Git_AllPathsNonexistent(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "nope.txt", "alsonope.txt"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff (all-nonexistent): %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (all paths filtered), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Git_BadRev: unresolvable REV → exit 3 (VCS exec).
func TestRun_VcsDiff_Git_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "doesnotexist"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsDiff_Jj_HasDiff: `vcs diff @--` on a jj fixture prints the
// bump diff (VERSION) and exits 0.
func TestRun_VcsDiff_Jj_HasDiff(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "@--"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff @--: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected stdout to include VERSION patch, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NoRepo: outside a vcs repo → exit 3.
func TestRun_VcsDiff_NoRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// --- DR-0020 PR-3.1: vcs diff -s / -q tests ------------------------------

// TestRun_VcsDiff_NameStatus_Git: `vcs diff -s HEAD~1` prints
// tab-separated M/A/D lines (git-native format) and exits 0.
func TestRun_VcsDiff_NameStatus_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -s HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected 'M\\tVERSION' in stdout, got: %q", stdout.String())
		}
		// Crucially: stdout must NOT contain raw patch text (unified diff hunks).
		if strings.Contains(stdout.String(), "@@") {
			t.Errorf("name-status output should not contain patch hunks, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NameStatus_LongOption: --name-status equivalent to -s.
func TestRun_VcsDiff_NameStatus_LongOption(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "--name-status", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff --name-status HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected 'M\\tVERSION', got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NameStatus_Jj: jj backend produces tab-normalized output.
func TestRun_VcsDiff_NameStatus_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "@--"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -s @--: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected tab-normalized 'M\\tVERSION', got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Quiet_HasChanges_ExitsFalse: -q with diff present →
// stdout empty, exit code 1 (predicate-false), no error message.
func TestRun_VcsDiff_Quiet_HasChanges_ExitsFalse(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "HEAD~1"}, bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatal("expected exitCodeFalse error for diff present")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -q, got: %q", got)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("stderr should be empty (silent predicate), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Quiet_NoChanges_ExitsZero: -q with no diff → exit 0.
func TestRun_VcsDiff_Quiet_NoChanges_ExitsZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "HEAD"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -q HEAD (no diff): expected nil, got %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty, got: %q", got)
		}
	})
}

// TestRun_VcsDiff_QuietLong_HasChanges: --quiet alias works the same way.
func TestRun_VcsDiff_QuietLong_HasChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "--quiet", "HEAD~1"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsDiff_QuietAll_HasChanges: -qq also reflects presence via
// exit code. stderr is suppressed even for error paths.
func TestRun_VcsDiff_QuietAll_HasChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-qq", "HEAD~1"}, bytes.NewReader(nil), &stdout, &stderr)
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -qq, got: %q", got)
		}
	})
}

// TestRun_VcsDiff_NameStatusAndQuiet_QuietWins: `-s -q` → -q wins;
// stdout empty, exit reflects presence (1 = has diff).
func TestRun_VcsDiff_NameStatusAndQuiet_QuietWins(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "-q", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (-q overrides -s), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Quiet_BadRev: -q + bad REV → exit 3 (VCS exec), not 1.
// Distinguishing exec failure from predicate-false is required by DR-0020.
func TestRun_VcsDiff_Quiet_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "doesnotexist"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for bad rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsDiff_Quiet_Jj_HasChanges: jj backend also surfaces diff
// presence via exit 1.
func TestRun_VcsDiff_Quiet_Jj_HasChanges(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "-q", "@--"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (jj has diff), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsDiff_Quiet_AllPathsNonexistent_ExitsZero: every path
// filtered → empty diff → exit 0 (matches "no diff" branch).
func TestRun_VcsDiff_Quiet_AllPathsNonexistent_ExitsZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "-q", "HEAD~1", "nope.txt"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("expected nil (all paths filtered → no diff), got: %v", err)
		}
	})
}

// --- v0.20.2 bugfix: verb-aware flag rejection ---------------------------
//
// PR-3.1 (v0.20.1) introduced `-s/--name-status` for `vcs diff` but the
// shared parser also silently accepted it for `vcs get` / `vcs is` (no-op).
// This violates kawaz CLI design (rules/cli-design-preferences.md: unknown
// flags must exit 2 with a usage hint, so typos are caught). The fix gates
// `-s/--name-status` to the `diff` verb; other verbs hit the generic
// unknown-flag rejection.
