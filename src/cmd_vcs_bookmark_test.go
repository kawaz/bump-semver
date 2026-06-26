package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Tests for `vcs bookmark set` (T11). Same shape as cmd_vcs_tag_test.go's
// PR-6 dispatcher subtests: one TestRun_VcsBookmark host with sub-tests for
// each grammar / exit-code concern, mirroring the existing two-tier `vcs
// tag` verb pattern.

func TestRun_VcsBookmark(t *testing.T) {
	t.Parallel()

	// `vcs bookmark` alone shows the per-verb help (mirrors `vcs tag`).
	t.Run("vcs bookmark (no sub-verb) shows bookmark help", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "bookmark"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs bookmark (no args): %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "set") {
			t.Errorf("vcs bookmark help should list the set sub-verb, got: %q", out)
		}
	})

	// `vcs bookmark --help` is the same as `vcs bookmark`.
	t.Run("vcs bookmark --help shows bookmark help", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "bookmark", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs bookmark --help: %v", err)
		}
		if !strings.Contains(stdout.String(), "bookmark") {
			t.Errorf("vcs bookmark --help should mention bookmark, got: %q", stdout.String())
		}
	})

	// `vcs bookmark set --help` documents NAME, --rev / -r, --allow-backwards.
	t.Run("vcs bookmark set --help documents flags", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "bookmark", "set", "--help"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs bookmark set --help: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{"NAME", "--rev", "--allow-backwards"} {
			if !strings.Contains(out, want) {
				t.Errorf("vcs bookmark set help should mention %q, got: %q", want, out)
			}
		}
	})

	// `vcs bookmark <unknown>` → exit-2 usage error with sub-verb hint.
	t.Run("unknown sub-verb is exit 2", func(t *testing.T) {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "bookmark", "wibble"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown sub-verb")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "set") {
			t.Errorf("error should mention available sub-verbs, got: %q", stderr.String())
		}
	})

	// `vcs bookmark set` (no NAME) → exit-2 usage error.
	t.Run("missing NAME is exit 2", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, nil, "1.0.0")
		withCwd(t, dir, func() {
			var stderr bytes.Buffer
			err := run([]string{"vcs", "bookmark", "set", "--rev", "HEAD"},
				bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatal("expected usage error for missing NAME")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeUsage {
				t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
			}
		})
	})

	// `vcs bookmark set foo bar` (two NAMEs) → exit-2.
	t.Run("too many positionals is exit 2", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, nil, "1.0.0")
		withCwd(t, dir, func() {
			var stderr bytes.Buffer
			err := run([]string{"vcs", "bookmark", "set", "foo", "bar"},
				bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatal("expected usage error for multiple NAMEs")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeUsage {
				t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
			}
		})
	})

	// `vcs bookmark set <bad-name>` → exit-2.
	t.Run("refs/-prefixed NAME is exit 2", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, nil, "1.0.0")
		withCwd(t, dir, func() {
			var stderr bytes.Buffer
			err := run([]string{"vcs", "bookmark", "set", "refs/heads/foo"},
				bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatal("expected usage error for refs/-prefixed NAME")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeUsage {
				t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
			}
		})
	})

	// `vcs bookmark set --rev=""` → exit-2.
	t.Run("empty --rev value is exit 2", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, nil, "1.0.0")
		withCwd(t, dir, func() {
			var stderr bytes.Buffer
			err := run([]string{"vcs", "bookmark", "set", "feat", "--rev", ""},
				bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatal("expected usage error for empty --rev")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeUsage {
				t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
			}
		})
	})
}

// TestRun_VcsBookmarkSet_Git_Create: `vcs bookmark set` on an absent branch
// creates refs/heads/<NAME> at the resolved REV.
func TestRun_VcsBookmarkSet_Git_Create(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "bookmark", "set", "feature-x", "-r", "HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs bookmark set feature-x -r HEAD: %v", err)
		}
		// Verify the ref exists and points at HEAD.
		got := strings.TrimSpace(runInOut(t, dir, "git", "rev-parse", "refs/heads/feature-x"))
		want := strings.TrimSpace(runInOut(t, dir, "git", "rev-parse", "HEAD"))
		if got != want {
			t.Errorf("refs/heads/feature-x = %s, want HEAD = %s", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_DefaultRev: --rev defaults to HEAD when omitted.
// This is the justfile push-wip's `bump-semver vcs bookmark set "$ws" -r @`
// path with -r missing — should still succeed at HEAD.
func TestRun_VcsBookmarkSet_Git_DefaultRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "bookmark", "set", "feature-y"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs bookmark set feature-y (default rev): %v", err)
		}
		got := runInOut(t, dir, "git", "rev-parse", "refs/heads/feature-y")
		want := runInOut(t, dir, "git", "rev-parse", "HEAD")
		if got != want {
			t.Errorf("refs/heads/feature-y = %s, want HEAD = %s", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_Idempotent: setting an existing ref to the
// same REV is a no-op (exit 0).
func TestRun_VcsBookmarkSet_Git_Idempotent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// First set creates.
		if err := run([]string{"vcs", "bookmark", "set", "stable", "-r", "HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("first set: %v", err)
		}
		// Second set is no-op.
		if err := run([]string{"vcs", "bookmark", "set", "stable", "-r", "HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Errorf("idempotent set should succeed: %v", err)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_FFForward: a ref at HEAD~1 moves forward to
// HEAD without --allow-backwards.
func TestRun_VcsBookmarkSet_Git_FFForward(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// Create the ref at HEAD~1.
		runIn(t, dir, "git", "branch", "trailing", "HEAD~1")
		// Move it forward to HEAD.
		if err := run([]string{"vcs", "bookmark", "set", "trailing", "-r", "HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs bookmark set trailing -r HEAD (FF): %v", err)
		}
		got := runInOut(t, dir, "git", "rev-parse", "refs/heads/trailing")
		want := runInOut(t, dir, "git", "rev-parse", "HEAD")
		if got != want {
			t.Errorf("after FF: refs/heads/trailing = %s, want HEAD = %s", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_NonFF: a ref at HEAD cannot move back to HEAD~1
// without --allow-backwards → exit 5.
func TestRun_VcsBookmarkSet_Git_NonFF(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		runIn(t, dir, "git", "branch", "leader", "HEAD")
		var stderr bytes.Buffer
		err := run([]string{"vcs", "bookmark", "set", "leader", "-r", "HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit 5 (non-ff)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exit %d (non-ff), got: %v", exitCodeNonFastForward, err)
		}
		if !strings.Contains(stderr.String(), "allow-backwards") {
			t.Errorf("non-ff message should hint at --allow-backwards, got: %q", stderr.String())
		}
		// Ref must not have moved.
		got := runInOut(t, dir, "git", "rev-parse", "refs/heads/leader")
		want := runInOut(t, dir, "git", "rev-parse", "HEAD")
		if got != want {
			t.Errorf("after non-ff reject: leader = %s, want unchanged HEAD = %s", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_AllowBackwards: --allow-backwards lets the ref
// move to an earlier commit.
func TestRun_VcsBookmarkSet_Git_AllowBackwards(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		runIn(t, dir, "git", "branch", "wanderer", "HEAD")
		if err := run([]string{"vcs", "bookmark", "set", "wanderer", "-r", "HEAD~1", "--allow-backwards"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs bookmark set wanderer -r HEAD~1 --allow-backwards: %v", err)
		}
		got := runInOut(t, dir, "git", "rev-parse", "refs/heads/wanderer")
		want := runInOut(t, dir, "git", "rev-parse", "HEAD~1")
		if got != want {
			t.Errorf("after backwards: wanderer = %s, want HEAD~1 = %s", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_BadRev: an unresolvable REV is exit 3.
func TestRun_VcsBookmarkSet_Git_BadRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "bookmark", "set", "feat", "-r", "no-such-rev-zzz"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit 3 for unresolvable rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsBookmarkSet_Git_NoRepo: outside a vcs repo → exit 3.
func TestRun_VcsBookmarkSet_NoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "bookmark", "set", "feat"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsBookmarkSet_Jj_Create: jj backend creates a new bookmark via
// `jj bookmark set`.
func TestRun_VcsBookmarkSet_Jj_Create(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		if err := run([]string{"vcs", "bookmark", "set", "feat-jj", "-r", "@"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs bookmark set feat-jj -r @: %v", err)
		}
		// Verify the bookmark exists.
		out := runInOut(t, dir, "jj", "bookmark", "list", "feat-jj")
		if !strings.Contains(out, "feat-jj") {
			t.Errorf("expected feat-jj in `jj bookmark list`, got: %q", out)
		}
	})
}

// TestRun_VcsBookmarkSet_Jj_DefaultRev: --rev defaults to @ (the current
// change), matching the documented `(default: @ / HEAD)` contract. Promote
// uses @- because it tags a finished commit; bookmark set tags an explicit
// caller intent, which is normally the working copy itself.
func TestRun_VcsBookmarkSet_Jj_DefaultRev(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		if err := run([]string{"vcs", "bookmark", "set", "feat-default-jj"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs bookmark set feat-default-jj (default rev): %v", err)
		}
		want := strings.TrimSpace(runInOut(t, dir, "jj", "log", "-r", "@", "--no-graph", "-T", "commit_id"))
		got := strings.TrimSpace(runInOut(t, dir, "jj", "log", "-r", "feat-default-jj", "--no-graph", "-T", "commit_id"))
		if got != want || got == "" {
			t.Errorf("feat-default-jj points at %q, want commit at @ = %q", got, want)
		}
	})
}

// TestRun_VcsBookmarkSet_Jj_NonFF: jj rejects a backwards move without
// --allow-backwards → exit 5.
func TestRun_VcsBookmarkSet_Jj_NonFF(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// Create bookmark at @.
		runIn(t, dir, "jj", "bookmark", "set", "stuck", "-r", "@")
		// Try to move it backwards to @-.
		var stderr bytes.Buffer
		err := run([]string{"vcs", "bookmark", "set", "stuck", "-r", "@-"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit 5 (non-ff)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exit %d (non-ff), got: %v", exitCodeNonFastForward, err)
		}
	})
}

// TestRun_VcsBookmarkSet_Jj_AllowBackwards: --allow-backwards lets the
// bookmark move backwards in jj too.
func TestRun_VcsBookmarkSet_Jj_AllowBackwards(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		runIn(t, dir, "jj", "bookmark", "set", "wanderer-jj", "-r", "@")
		if err := run([]string{"vcs", "bookmark", "set", "wanderer-jj", "-r", "@-", "--allow-backwards"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs bookmark set wanderer-jj -r @- --allow-backwards: %v", err)
		}
	})
}
