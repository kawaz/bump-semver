package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestRun_VcsTagPR6 hosts all PR-6 dispatcher subtests so the file's
// existing structure (one top-level test per concern) is preserved.
// Subtests share the same setup helpers as the PR-5 fetch/push tests.
func TestRun_VcsTagPR6(t *testing.T) {
	t.Parallel()
	// TestRun_VcsTag_NoSubVerb: `vcs tag` alone shows the per-verb help
	// (matches the `vcs commit` / `vcs push` no-args convention).
	t.Run("vcs tag (no sub-verb) shows tag help", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag (no args): %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "push") || !strings.Contains(out, "delete") {
			t.Errorf("vcs tag help should list push/delete sub-verbs, got: %q", out)
		}
	})

	// TestRun_VcsTag_Help: `vcs tag --help` is the same as `vcs tag`.
	t.Run("vcs tag --help shows tag help", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag --help: %v", err)
		}
		if !strings.Contains(stdout.String(), "tag") {
			t.Errorf("vcs tag --help should mention tag, got: %q", stdout.String())
		}
	})

	// TestRun_VcsTagPush_Help: `vcs tag push --help` documents --rev, NAME,
	// --remote, --allow-move.
	t.Run("vcs tag push --help documents flags", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "push", "--help"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag push --help: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{"--rev", "NAME", "--remote", "--allow-move"} {
			if !strings.Contains(out, want) {
				t.Errorf("vcs tag push help should mention %q, got: %q", want, out)
			}
		}
	})

	// TestRun_VcsTagDelete_Help: `vcs tag delete --help` documents NAME and
	// --remote.
	t.Run("vcs tag delete --help documents flags", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "delete", "--help"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag delete --help: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{"NAME", "--remote"} {
			if !strings.Contains(out, want) {
				t.Errorf("vcs tag delete help should mention %q, got: %q", want, out)
			}
		}
	})

	// TestRun_VcsHelp_TagListed: parent `vcs --help` includes `tag` in the
	// verb list.
	t.Run("vcs --help lists tag", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs --help: %v", err)
		}
		if !strings.Contains(stdout.String(), "tag") {
			t.Errorf("vcs --help should list tag verb, got: %q", stdout.String())
		}
	})

	// --- vcs tag push positive paths ----------------------------------------

	t.Run("vcs tag push new tag (git fixture)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "v1.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Errorf("vcs tag push: %v", err)
			}
		})
	})

	t.Run("vcs tag push same rev idempotent (git fixture)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		preloadBareWith(t, work)
		runIn(t, work, "git", "tag", "v1.0.0", "HEAD")
		// Push out-of-band so remote already has it.
		if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
			t.Fatalf("setup push: %v", err)
		}
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "v1.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Errorf("same-rev vcs tag push should succeed, got: %v", err)
			}
		})
	})

	t.Run("vcs tag push diff rev without --allow-move → exit 4", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		preloadBareWith(t, work)
		runIn(t, work, "git", "tag", "v1.0.0", "HEAD~1")
		if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
			t.Fatalf("setup push: %v", err)
		}
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "v1.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected exit 4 for diff rev without --allow-move")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
				t.Errorf("expected exitCodeAmbiguous (4), got: %v", err)
			}
		})
	})

	t.Run("vcs tag push diff rev with --allow-move (git fixture)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		preloadBareWith(t, work)
		runIn(t, work, "git", "tag", "v1.0.0", "HEAD~1")
		if _, err := runBackendCmdIn(work, "git", "push", "origin", "refs/tags/v1.0.0"); err != nil {
			t.Fatalf("setup push: %v", err)
		}
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "--allow-move", "v1.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Errorf("vcs tag push --allow-move: %v", err)
			}
		})
	})

	t.Run("vcs tag push bad rev → exit 3", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "push", "--rev", "nope-rev", "v1.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error for bad REV")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
				t.Errorf("expected exitCodeVCSExec, got: %v", err)
			}
		})
	})

	// --- vcs tag push argument errors --------------------------------------

	t.Run("vcs tag push missing NAME → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for missing NAME")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push missing --rev → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "v1.0.0"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for missing --rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push empty NAME → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", ""},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for empty NAME")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push NAME with refs/ prefix → exit 2", func(t *testing.T) {
		// refs/tags/NAME slipped in as NAME would create refs/tags/refs/tags/...
		// which is almost always a bug. Reject explicitly.
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "refs/tags/v1.0.0"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for refs/ prefix")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push NAME with whitespace → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "v 1.0.0"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for NAME with whitespace")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push extra positional → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "v1.0.0", "extra"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for extra positional")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag push --force is rejected (use --allow-move)", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "push", "--rev", "HEAD", "--force", "v1.0.0"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for --force")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	// --- vcs tag delete positive paths -------------------------------------

	t.Run("vcs tag delete present (git fixture)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		runIn(t, work, "git", "tag", "v0.9.0", "HEAD")
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "delete", "v0.9.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Errorf("vcs tag delete: %v", err)
			}
		})
	})

	t.Run("vcs tag delete absent idempotent (git fixture)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "delete", "never-existed"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Errorf("vcs tag delete (absent) should succeed, got: %v", err)
			}
		})
	})

	t.Run("vcs tag delete bad remote → exit 3", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
		runIn(t, work, "git", "tag", "v9.0.0", "HEAD")
		withCwd(t, work, func() {
			err := run([]string{"vcs", "tag", "delete", "--remote", "nonexistent", "v9.0.0"},
				bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected exit 3 for bad remote")
			}
			var ee *exitErr
			if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
				t.Errorf("expected exitCodeVCSExec, got: %v", err)
			}
		})
	})

	// --- vcs tag delete argument errors ------------------------------------

	// Bare `vcs tag delete` (no NAME, no --remote) shows the per-verb
	// help rather than failing with exit 2 — matches the existing
	// `vcs push` / `vcs commit` "bare verb = help" convention. The
	// usage-error path is covered by the `--remote alone (no NAME)`
	// subtest below.
	t.Run("vcs tag delete (no args) shows delete help", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "delete"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag delete (no args): %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "delete") || !strings.Contains(out, "NAME") {
			t.Errorf("vcs tag delete (no args) should show delete help mentioning NAME, got: %q", out)
		}
	})

	t.Run("vcs tag delete --remote alone (no NAME) → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "delete", "--remote", "origin"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for missing NAME with --remote present")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	t.Run("vcs tag delete extra positional → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "delete", "v1.0.0", "v2.0.0"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for extra positional")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})

	// Unknown sub-verb under tag (e.g. `vcs tag list`) → exit 2.
	t.Run("vcs tag list is unknown sub-verb → exit 2", func(t *testing.T) {
		err := run([]string{"vcs", "tag", "list"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for unknown tag sub-verb")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})
}
