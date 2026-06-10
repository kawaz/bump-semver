package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VcsPush_Branch: `vcs push --branch main` pushes to origin.
func TestRun_VcsPush_Branch(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --branch main: %v", err)
		}
	})
}

// TestRun_VcsPush_BookmarkAlias: `vcs push --bookmark main` is an alias of
// `--branch main`.
func TestRun_VcsPush_BookmarkAlias(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--bookmark", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --bookmark main: %v", err)
		}
	})
}

// TestRun_VcsPush_NoArgs: `vcs push` with no args shows the per-verb help
// (matches the existing `vcs commit` / `vcs diff` convention — bare verb
// = help, partial verb = error).
func TestRun_VcsPush_NoArgs(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "push"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs push (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "push") || !strings.Contains(stdout.String(), "--branch") {
		t.Errorf("expected vcs push help mentioning push/--branch, got: %q", stdout.String())
	}
}

// TestRun_VcsPush_MissingName: `vcs push --remote origin` (no
// --branch/--bookmark) is a usage error — NAME is required (no auto-
// detection by design).
func TestRun_VcsPush_MissingName(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--remote", "origin"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for missing --branch/--bookmark")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})
}

// TestRun_VcsPush_BranchAndBookmarkBothSet: setting both --branch and
// --bookmark on the same invocation is a usage error (they're aliases of
// one field, double-set rejected).
func TestRun_VcsPush_BranchAndBookmarkBothSet(t *testing.T) {
	t.Parallel()
	err := run([]string{"vcs", "push", "--branch", "main", "--bookmark", "main"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --branch + --bookmark")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsPush_RemoteFlag: `--remote NAME` overrides the default origin.
func TestRun_VcsPush_RemoteFlag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--remote", "origin"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --branch main --remote origin: %v", err)
		}
	})
}

// TestRun_VcsPush_BadRemote: nonexistent remote → exit 3.
func TestRun_VcsPush_BadRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--remote", "nonexistent"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for nonexistent remote")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d, got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsPush_NonFastForward: divergent remote → exit 5. PR-5.1
// removes the bump-semver editorial hint ("remote has diverged..."), so
// the assertion now verifies that (a) git's own native rejection marker
// (`(fetch first)` or `(non-fast-forward)`) reaches stderr unmolested and
// (b) the old bump-semver hint phrase is GONE — kawaz confirmed users
// should read the underlying tool's message directly rather than an
// editorial paraphrase.
func TestRun_VcsPush_NonFastForward(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	divergeBareViaAttacker(t, bare)
	if err := writeFile(filepath.Join(work, "local.txt"), "local\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, work, "git", "add", "local.txt")
	runIn(t, work, "git", "commit", "-qm", "local-only")
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected non-ff failure")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exit %d, got: %v", exitCodeNonFastForward, err)
		}
		s := stderr.String()
		// Git's own native rejection marker must reach the user.
		if !strings.Contains(s, "(fetch first)") && !strings.Contains(s, "(non-fast-forward)") {
			t.Errorf("expected git's native rejection marker in stderr, got: %q", s)
		}
		// The old editorial hint must NOT appear (PR-5.1 removed it).
		if strings.Contains(s, "remote has diverged") ||
			strings.Contains(s, "force push is intentionally not supported") {
			t.Errorf("PR-5.1 removed the editorial non-ff hint, but it is still present: %q", s)
		}
	})
}

// TestRun_VcsPush_NothingToPush: idempotent success when remote already
// has it. PR-5.1 additionally requires that git's own "Everything
// up-to-date" diagnostic reaches the user (stdout OR stderr — git puts
// it on stderr but we don't lock the channel here) so the user can see
// the convergence happened rather than a silent no-op.
func TestRun_VcsPush_NothingToPush(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Errorf("idempotent push should succeed, got: %v", err)
		}
		combined := stdout.String() + stderr.String()
		if !strings.Contains(combined, "Everything up-to-date") {
			t.Errorf("expected git's 'Everything up-to-date' to reach the user, got: %q", combined)
		}
	})
}

// TestRun_VcsPush_NothingToPush_Quiet: with -q, the success-path
// diagnostic is informational (not error-class) and gets suppressed on
// BOTH stdout and stderr. This matches the bump-semver --quiet contract
// where hint-class output goes away under -q.
func TestRun_VcsPush_NothingToPush_Quiet(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main", "-q"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Errorf("idempotent push with -q should succeed, got: %v", err)
		}
		combined := stdout.String() + stderr.String()
		if strings.Contains(combined, "Everything up-to-date") {
			t.Errorf("-q should suppress success diagnostic on both channels, got: %q", combined)
		}
	})
}

// TestRun_VcsPush_RejectForce: `--force` is intentionally not provided —
// any attempt is a usage error.
func TestRun_VcsPush_RejectForce(t *testing.T) {
	t.Parallel()
	err := run([]string{"vcs", "push", "--branch", "main", "--force"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --force")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsPush_UnknownVerbFlag: a verb-local flag on the wrong verb
// (e.g. --tags) is rejected at the parser layer.
func TestRun_VcsPush_UnknownFlag(t *testing.T) {
	t.Parallel()
	err := run([]string{"vcs", "push", "--branch", "main", "--tags"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --tags")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsHelp_FetchPush: `vcs --help` includes fetch / push in the
// verb list.
func TestRun_VcsHelp_FetchPush(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs --help: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "fetch") {
		t.Errorf("vcs help should mention 'fetch', got: %q", out)
	}
	if !strings.Contains(out, "push") {
		t.Errorf("vcs help should mention 'push', got: %q", out)
	}
}

// TestRun_VcsFetchHelp / TestRun_VcsPushHelp: per-verb help works.
func TestRun_VcsFetchHelp(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "fetch", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs fetch --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "fetch") {
		t.Errorf("vcs fetch help should mention 'fetch', got: %q", stdout.String())
	}
}

// TestRun_VcsPushHelp: PR-5.1 simplifies the help so `--branch` is the
// canonical surface and the bookmark vocabulary appears as a single
// brief parenthetical for jj users. Both flag spellings still parse —
// kawaz's confirmation was "branch 一本化で help に jj では bookmark の
// 意と簡潔記載て", i.e. terminology bridge, not alias advertising.
func TestRun_VcsPushHelp(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "push", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs push --help: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "--branch") {
		t.Errorf("vcs push help should mention --branch, got: %q", out)
	}
	if !strings.Contains(out, "bookmark") {
		t.Errorf("vcs push help should mention bookmark (jj vocabulary bridge), got: %q", out)
	}
	// The PR-5 verbose "alias" phrasing must be gone (PR-5.1 simplification).
	if strings.Contains(out, "Alias of --branch") || strings.Contains(out, "alias of --branch") {
		t.Errorf("PR-5.1 dropped the 'Alias of --branch' line, but it is still present: %q", out)
	}
}

// --- DR-0020 PR-5.1: regression-locking tests ----------------------------
//
// These tests pin the four behaviour changes from PR-5.1:
//   - help simplification (A, E)
//   - non-ff hint removal (B, covered above + here for parent help)
//   - jj git export retry-once + recovery hint (C)
//   - nothing-to-push diagnostic forwarding (D, covered above)

// TestHelpVcsPush_BookmarkIsBrief: the help body must NOT carry the
// verbose `--bookmark NAME  Alias of --branch...` line introduced by
// PR-5; PR-5.1 keeps `--branch` canonical and reduces the bookmark
// mention to a single inline parenthetical for jj users. kawaz's
// directive: 一行注釈に圧縮 — the test also caps how many bookmark
// occurrences may appear (= one inline mention, not a re-explanation
// of the mutual-exclusion rule).
func TestHelpVcsPush_BookmarkIsBrief(t *testing.T) {
	t.Parallel()
	body := mustRun(t, "vcs", "push", "--help")
	// Old verbose lines that PR-5.1 deletes.
	for _, banned := range []string{
		"--bookmark NAME  Alias of --branch",
		"--bookmark NAME  Alias of `--branch`",
		"# alias",
		"may appear per invocation",
		"mutually exclusive",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("PR-5.1 should remove %q from vcs push --help, but it is still present", banned)
		}
	}
	// The terminology bridge must remain (in whatever exact wording the
	// implementation picks — we just require "bookmark" appears, since
	// jj users need to recognise the verbose).
	if !strings.Contains(body, "bookmark") {
		t.Errorf("vcs push --help should still mention 'bookmark' for jj users, got: %q", body)
	}
}

// TestHelpVcsPush_NoEditorialHint: the per-verb help body must NOT
// promise the deleted `remote has diverged` editorial hint — keep the
// text honest about what bump-semver actually prints (= git/jj raw).
func TestHelpVcsPush_NoEditorialHint(t *testing.T) {
	t.Parallel()
	body := mustRun(t, "vcs", "push", "--help")
	if strings.Contains(body, "remote has diverged") {
		t.Errorf("vcs push --help should not promise a hint that PR-5.1 removed, got: %q", body)
	}
	// Help should now point users at the underlying tool's message
	// instead. We assert the user-facing concept rather than a fixed
	// phrasing (the implementer picks the exact wording).
	if !strings.Contains(body, "git/jj") && !strings.Contains(body, "underlying") {
		t.Errorf("vcs push --help should redirect non-ff users to the underlying tool's stderr, got: %q", body)
	}
}

// TestJjBackend_Push_ExportRetrySucceeds: PR-5.1 retry-once on jj git
// export failure. The first attempt fails with a transient-looking
// stderr; the second succeeds; Push returns nil.
func TestJjBackend_Push_ExportRetrySucceeds(t *testing.T) {
	// Mutates package-level `jjGitExportFunc` seam; cannot be parallel.
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")

	// Install a stub export func: fail #1, succeed #2.
	calls := 0
	orig := jjGitExportFunc
	jjGitExportFunc = func() (stderr string, code int, err error) {
		calls++
		if calls == 1 {
			return "Internal error: cannot lock ref 'refs/heads/test'", 1, nil
		}
		return "", 0, nil
	}
	t.Cleanup(func() { jjGitExportFunc = orig })

	withCwd(t, work, func() {
		b := &jjBackend{}
		if err := b.Push(pushOpts{name: "main", remote: "origin"}); err != nil {
			t.Errorf("Push with first-attempt export failure + retry success should return nil, got: %v", err)
		}
	})
	if calls != 2 {
		t.Errorf("expected exactly 2 export attempts (retry-once), got: %d", calls)
	}
}

// TestJjBackend_Push_ExportRetryFailsTwice: PR-5.1 — both attempts fail
// → exit 3 + recovery-hint message containing the matched-pattern
// guidance and a jj-vcs issue link. We assert the substring-matched
// case (ref-hierarchy conflict, issue #493) which is the most common
// in practice.
func TestJjBackend_Push_ExportRetryFailsTwice(t *testing.T) {
	// Mutates package-level `jjGitExportFunc` seam; cannot be parallel.
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")

	calls := 0
	orig := jjGitExportFunc
	// Use a ref-hierarchy conflict signature (jj issue #493).
	jjGitExportFunc = func() (stderr string, code int, err error) {
		calls++
		return "Internal error: Failed to export refs to underlying Git repo: " +
			"cannot lock ref 'refs/heads/test', there are refs beneath that folder", 1, nil
	}
	t.Cleanup(func() { jjGitExportFunc = orig })

	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin"})
		if err == nil {
			t.Fatal("Push with persistent export failure should error")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec (3), got: %v", err)
		}
		// Original jj stderr must be folded in (not paraphrased).
		if !strings.Contains(ee.msg, "cannot lock ref") {
			t.Errorf("recovery message should fold in the jj stderr, got: %q", ee.msg)
		}
		// Recovery guidance should mention the recognised pattern's
		// concrete remedy (git for-each-ref or rename/delete) AND the
		// jj-vcs issue link for cross-reference.
		if !strings.Contains(ee.msg, "for-each-ref") && !strings.Contains(ee.msg, "rename or delete") {
			t.Errorf("recovery message for ref-hierarchy clash should advise inspect+rename/delete, got: %q", ee.msg)
		}
		if !strings.Contains(ee.msg, "jj-vcs/jj") {
			t.Errorf("recovery message should link to jj-vcs/jj for further reading, got: %q", ee.msg)
		}
	})
	if calls != 2 {
		t.Errorf("expected exactly 2 export attempts before giving up, got: %d", calls)
	}
}

// TestJjBackend_Push_ExportRetryGenericFallback: PR-5.1 — when the
// stderr doesn't match a known pattern, the generic fallback still
// triggers (retry + exit 3 + issue link).
func TestJjBackend_Push_ExportRetryGenericFallback(t *testing.T) {
	// Mutates package-level `jjGitExportFunc` seam; cannot be parallel.
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@-")

	orig := jjGitExportFunc
	jjGitExportFunc = func() (stderr string, code int, err error) {
		return "Internal error: something totally novel we have not catalogued", 1, nil
	}
	t.Cleanup(func() { jjGitExportFunc = orig })

	withCwd(t, work, func() {
		b := &jjBackend{}
		err := b.Push(pushOpts{name: "main", remote: "origin"})
		if err == nil {
			t.Fatal("expected error for unrecognised export failure")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec (3), got: %v", err)
		}
		// Raw stderr passthrough (no paraphrase) + issue link.
		if !strings.Contains(ee.msg, "something totally novel") {
			t.Errorf("recovery message should fold in the raw jj stderr, got: %q", ee.msg)
		}
		if !strings.Contains(ee.msg, "jj-vcs/jj") {
			t.Errorf("recovery message should link to jj-vcs/jj, got: %q", ee.msg)
		}
	})
}

// setupJjForAutoAdvance builds the common jj fixture shared by every
// AutoAdvance jj test: a colocated jj repo with origin/main wired up via
// setupJjRepoWithRemote, and the local `main` bookmark planted at @--
// so auto-advance has somewhere forward to move. Per-test specifics
// (dirty working copy, undescribed parent, describe message, etc.) stay
// inline in the test since they're what each variant is actually
// pinning.
func setupJjForAutoAdvance(t *testing.T) (work, bare string) {
	t.Helper()
	work, bare = setupJjRepoWithRemote(t, nil, "1.0.0")
	runIn(t, work, "jj", "bookmark", "set", "main", "-r", "@--", "--allow-backwards")
	return work, bare
}

// --- DR-0020 PR-5.2 / PR-5.2.1: --jj-bookmark-auto-advance dispatcher tests
//
// PR-5.2 adds `vcs push --jj-bookmark-auto-advance`. PR-5.2.1 reframes the
// flag under the **backend-prefix general rule** (kawaz 2026-06-01 確定):
// `--jj-*` / `--git-*` flags route by name to their backend; the other
// backend silently ignores them. The dispatcher tests below pin:
//
//   - parser accepts the flag (no false-positive "unknown flag" rejection)
//   - on a git repo, the flag is a silent no-op — the push proceeds
//     normally, no "jj-specific" diagnostic in stderr
//   - on a jj repo, the flag reaches the backend (= forward-move case
//     succeeds, mirroring TestRun_VcsPush_Branch but with the flag set)
//
// Quiet rules and stdout/stderr passthrough are unchanged from PR-5/5.1;
// re-asserting them here would be redundant.

// TestRun_VcsPush_AutoAdvance_GitSilentNoOp: passing the jj-prefixed flag
// to a git repo is a **silent no-op** (PR-5.2.1, backend-prefix general
// rule). The push completes normally, exit 0, and no auto-advance /
// jj-specific text leaks into stderr. The same script can therefore run
// against both jj and git backends without conditional branching.
func TestRun_VcsPush_AutoAdvance_GitSilentNoOp(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err != nil {
			t.Fatalf("--jj-bookmark-auto-advance on git should be silent no-op + normal push, got: %v (stderr=%q)", err, stderr.String())
		}
		s := stderr.String()
		// No backend-prefix diagnostic — the flag is structurally a
		// jj-side hook; git just ignores it. (The normal git push
		// success diagnostic is fine; we only forbid auto-advance
		// or "jj-specific" wording, which would indicate the old
		// reject path is still active.)
		if strings.Contains(s, "jj-specific") || strings.Contains(s, "auto-advance") {
			t.Errorf("git push should not surface --jj-bookmark-auto-advance in stderr (silent no-op), got: %q", s)
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

// TestRun_VcsPush_AutoAdvance_JjForward: the happy path — clean jj working
// copy, bookmark sitting before @-, flag set; auto-advance runs and the
// push succeeds. End-to-end mirror of TestRun_VcsPush_Branch.
func TestRun_VcsPush_AutoAdvance_JjForward(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjForAutoAdvance(t)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs push --jj-bookmark-auto-advance (jj forward): %v", err)
		}
	})
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(bareSHA)) == "" {
		t.Errorf("bare should have main after auto-advance push")
	}
}

// TestRun_VcsPush_AutoAdvance_JjDirty: dirty working copy → bookmark goes
// to @ (not @-, kawaz 確定 2026-05-31 — both clean and dirty workflows
// are first-class; users wanting strict clean-only gate with `vcs is
// clean` themselves). Push succeeds and the bare receives the @ commit.
func TestRun_VcsPush_AutoAdvance_JjDirty(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjForAutoAdvance(t)
	if err := writeFile(filepath.Join(work, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	// The realistic dirty workflow always describes before push (jj
	// refuses to push undescribed commits). PR-5.2 leaves the describe
	// step to the user — auto-advance covers the bookmark move, nothing
	// else.
	runIn(t, work, "jj", "describe", "-m", "bump to 9.9.9")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("auto-advance on dirty worktree should succeed (bookmark → @): %v", err)
		}
	})
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(bareSHA)) == "" {
		t.Errorf("bare should have main after dirty auto-advance push")
	}
}

// TestRun_VcsPush_AutoAdvance_JjCleanTargetNoDescription: clean working
// copy whose @- (the clean-branch advance target) has no description →
// auto-advance must fail fast with exit 3 and a hint pointing at
// `jj describe -r @-` (DR-0025). Pins the contract that the description
// check applies symmetrically to both clean (target=@-) and dirty
// (target=@) paths — the same push-reject trap exists on either side.
func TestRun_VcsPush_AutoAdvance_JjCleanTargetNoDescription(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjForAutoAdvance(t)
	// Build an undescribed @-: jj new on top of the current @ (which is
	// itself an undescribed auto-created change) so the new @- inherits
	// the no-description state, and @ stays clean+empty above it.
	runIn(t, work, "jj", "new")
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for clean @ with undescribed @- (advance target)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Fatalf("expected exitCodeVCSExec (3), got: %v", err)
		}
		msg := ee.msg + stderr.String()
		if !strings.Contains(msg, "jj describe") {
			t.Errorf("error message should hint at `jj describe`, got: %q", msg)
		}
		if !strings.Contains(msg, "@-") {
			t.Errorf("error message should name @- as the missing-description target, got: %q", msg)
		}
	})
}

// TestRun_VcsPush_AutoAdvance_JjDirtyNoDescription: dirty working copy
// whose @ has no description → auto-advance must fail fast with exit 3
// and a hint pointing at `jj describe` (DR-0025). Without this guard
// the dirty branch (target=@) advances the bookmark onto an undescribed
// commit, jj refuses the push ("Won't push commit XXX since it has no
// description"), and the user hits a retry loop because nothing in the
// flow describes the @ for them.
func TestRun_VcsPush_AutoAdvance_JjDirtyNoDescription(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, _ := setupJjForAutoAdvance(t)
	if err := writeFile(filepath.Join(work, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	// Intentionally NOT calling `jj describe` — the auto-created @
	// stays in the "no description" state. auto-advance should detect
	// this before moving the bookmark and surface the hint instead of
	// letting jj's push-reject percolate up unactionable.
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for dirty @ with no description")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Fatalf("expected exitCodeVCSExec (3), got: %v", err)
		}
		msg := ee.msg + stderr.String()
		if !strings.Contains(msg, "jj describe") {
			t.Errorf("error message should hint at `jj describe`, got: %q", msg)
		}
		if !strings.Contains(msg, "description") {
			t.Errorf("error message should name the missing description, got: %q", msg)
		}
	})
}

// TestRun_VcsPush_AutoAdvance_JjDirtyWhitespaceDescription: jj accepts
// whitespace-only descriptions on push (verified 2026-06-01) — auto-
// advance must accept them too (DR-0025). This pins the contract that
// the description check delegates to jj's `if(description, ...)` truth
// rather than a Go-side TrimSpace == "" check; the latter would over-
// reject relative to jj's actual push gate.
func TestRun_VcsPush_AutoAdvance_JjDirtyWhitespaceDescription(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	work, bare := setupJjForAutoAdvance(t)
	if err := writeFile(filepath.Join(work, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	// Whitespace-only description: jj treats this as "has description"
	// (the template engine's `if(description, ...)` is truthy for any
	// non-empty string), and the push proceeds without rejection.
	runIn(t, work, "jj", "describe", "-m", "   ")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("auto-advance with whitespace-only description should succeed (jj accepts it), got: %v", err)
		}
	})
	bareSHA, err := runBackendCmdIn(bare, "git", "rev-parse", "main")
	if err != nil {
		t.Fatalf("bare rev-parse main: %v", err)
	}
	if strings.TrimSpace(string(bareSHA)) == "" {
		t.Errorf("bare should have main after whitespace-description auto-advance push")
	}
}

// TestRun_VcsPush_AutoAdvance_ParserAcceptsFlag: the parser must accept
// `--jj-bookmark-auto-advance` as a verb-local boolean flag (no false
// "unknown flag" rejection at the parser layer). Specifying it without
// --branch is still a usage error for the same reason as a bare
// `vcs push --remote origin` (NAME required) — this test pins the
// "flag is parsed cleanly, semantic checks happen downstream" boundary.
func TestRun_VcsPush_AutoAdvance_ParserAcceptsFlag(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	// No --branch: should hit the "name required" error, not "unknown flag".
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--jj-bookmark-auto-advance"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for missing --branch")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
		}
		s := stderr.String() + ee.msg
		if strings.Contains(s, "unknown flag") {
			t.Errorf("parser must accept --jj-bookmark-auto-advance, but got 'unknown flag': %q", s)
		}
	})
}

// --- DR-0020 PR-6: vcs tag push / vcs tag delete dispatcher tests -------
