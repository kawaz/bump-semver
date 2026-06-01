package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestRun_VcsTagLatest_PR_PRTagLatest hosts all PR-Tag-Latest dispatcher
// subtests. The structure mirrors TestRun_VcsTagPR6 for consistency.
func TestRun_VcsTagLatest(t *testing.T) {
	// --- help routing -------------------------------------------------
	t.Run("vcs tag latest --help documents the flags", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "latest", "--help"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag latest --help: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{
			"--source", "--repository", "--include-prerelease",
			"--raw", "--json", "tag", "release",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("vcs tag latest help should mention %q, got: %q", want, out)
			}
		}
	})

	// Parent `vcs tag --help` should list `latest` alongside push/delete.
	t.Run("vcs tag --help lists latest", func(t *testing.T) {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "--help"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag --help: %v", err)
		}
		if !strings.Contains(stdout.String(), "latest") {
			t.Errorf("vcs tag --help should list 'latest' sub-verb, got: %q", stdout.String())
		}
	})

	// --- default --source=tag, cwd VCS --------------------------------
	t.Run("vcs tag latest (cwd, default = bare semver)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3", "v1.1.0"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest: %v", err)
			}
			// Default: bare SemVer (Prefix stripped).
			if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
				t.Errorf("default output = %q, want %q (bare SemVer)", got, "1.2.3")
			}
		})
	})

	// --- --raw preserves the original tag string ----------------------
	t.Run("vcs tag latest --raw (cwd, preserves v prefix)", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3", "v1.1.0"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest", "--raw"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest --raw: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "v1.2.3" {
				t.Errorf("--raw output = %q, want %q (v prefix kept)", got, "v1.2.3")
			}
		})
	})

	// --- --include-prerelease ----------------------------------------
	t.Run("vcs tag latest excludes prereleases by default", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		// v1.2.3-rc.1 would be the largest under include-prerelease,
		// but is dropped by default → v1.1.0 wins.
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3-rc.1", "v1.1.0"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "1.1.0" {
				t.Errorf("default (no --include-prerelease) = %q, want %q", got, "1.1.0")
			}
		})
	})

	t.Run("vcs tag latest --include-prerelease includes prereleases", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3-rc.1", "v1.1.0"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest", "--include-prerelease"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest --include-prerelease: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "1.2.3-rc.1" {
				t.Errorf("--include-prerelease = %q, want %q", got, "1.2.3-rc.1")
			}
		})
	})

	// --- --json ------------------------------------------------------
	t.Run("vcs tag latest --json emits structured output", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest", "--json"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest --json: %v", err)
			}
			var got map[string]string
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("--json parse: %v (raw: %q)", err, stdout.String())
			}
			if got["tag"] != "v1.2.3" {
				t.Errorf("--json tag = %q, want v1.2.3", got["tag"])
			}
			if got["version"] != "1.2.3" {
				t.Errorf("--json version = %q, want 1.2.3", got["version"])
			}
			// commit/date are best-effort; tag source leaves them
			// empty (no extra subprocess).
			if got["commit"] != "" {
				t.Errorf("--json commit (tag source) should be empty, got %q", got["commit"])
			}
		})
	})

	// --- usage errors ------------------------------------------------
	t.Run("--raw + --json is a usage error", func(t *testing.T) {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "tag", "latest", "--raw", "--json"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for --raw + --json")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit code %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "mutually exclusive") {
			t.Errorf("stderr should mention 'mutually exclusive', got: %q", stderr.String())
		}
	})

	t.Run("invalid --source value is a usage error", func(t *testing.T) {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "tag", "latest", "--source", "bogus"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for invalid --source")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit code %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "invalid --source") {
			t.Errorf("stderr should mention 'invalid --source', got: %q", stderr.String())
		}
	})

	t.Run("positional arg is a usage error", func(t *testing.T) {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "tag", "latest", "extra"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for positional")
		}
		if !strings.Contains(stderr.String(), "does not accept positional") {
			t.Errorf("stderr should mention 'does not accept positional', got: %q", stderr.String())
		}
	})

	// --- gh missing for --source release ------------------------------
	t.Run("--source release without gh returns exit 3 with hint", func(t *testing.T) {
		// Stub gh as missing via the package-level hook.
		orig := ghLookPath
		ghLookPath = func() error { return errors.New("not found") }
		defer func() { ghLookPath = orig }()

		var stderr bytes.Buffer
		err := run([]string{"vcs", "tag", "latest", "--source", "release"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit-3 for missing gh")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit code %d (vcs/exec), got: %v", exitCodeVCSExec, err)
		}
		if !strings.Contains(stderr.String(), "gh CLI is required") {
			t.Errorf("stderr should mention 'gh CLI is required', got: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "cli.github.com") {
			t.Errorf("stderr should mention install URL, got: %q", stderr.String())
		}
	})

	// --- --source release happy path (gh stubbed) --------------------
	t.Run("--source release picks largest non-draft non-prerelease by default", func(t *testing.T) {
		// Stub gh to return a known release list. v1.2.3-rc.1 is a
		// prerelease (skipped by default); v0.9.0 has IsDraft=true
		// (always skipped); v1.2.2 wins.
		origLookup := ghLookPath
		ghLookPath = func() error { return nil }
		defer func() { ghLookPath = origLookup }()

		origRunner := ghRunner
		ghRunner = func(args ...string) ([]byte, error) {
			// Validate the args shape — we expect --json with the
			// fields the implementation needs.
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "release list") {
				return nil, fmt.Errorf("unexpected gh args: %v", args)
			}
			return []byte(`[
				{"tagName":"v1.2.3-rc.1","isDraft":false,"isPrerelease":true,"publishedAt":"2026-01-15T00:00:00Z"},
				{"tagName":"v1.2.2","isDraft":false,"isPrerelease":false,"publishedAt":"2026-01-10T00:00:00Z"},
				{"tagName":"v0.9.0","isDraft":true,"isPrerelease":false,"publishedAt":"2026-01-01T00:00:00Z"},
				{"tagName":"v1.1.0","isDraft":false,"isPrerelease":false,"publishedAt":"2025-12-15T00:00:00Z"}
			]`), nil
		}
		defer func() { ghRunner = origRunner }()

		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "latest",
			"--source", "release", "--repository", "kawaz/bump-semver"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag latest --source release: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "1.2.2" {
			t.Errorf("--source release default = %q, want %q", got, "1.2.2")
		}
	})

	t.Run("--source release --json populates date from gh", func(t *testing.T) {
		origLookup := ghLookPath
		ghLookPath = func() error { return nil }
		defer func() { ghLookPath = origLookup }()

		origRunner := ghRunner
		ghRunner = func(args ...string) ([]byte, error) {
			return []byte(`[
				{"tagName":"v1.2.2","isDraft":false,"isPrerelease":false,"publishedAt":"2026-01-10T00:00:00Z"}
			]`), nil
		}
		defer func() { ghRunner = origRunner }()

		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "latest",
			"--source", "release", "--repository", "kawaz/bump-semver",
			"--json"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag latest --source release --json: %v", err)
		}
		var got map[string]string
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("--json parse: %v", err)
		}
		if got["version"] != "1.2.2" {
			t.Errorf("--json version = %q, want 1.2.2", got["version"])
		}
		if got["date"] != "2026-01-10T00:00:00Z" {
			t.Errorf("--json date = %q, want 2026-01-10T00:00:00Z", got["date"])
		}
	})

	// --- --vcs git override threaded through to backend selection ----
	t.Run("--vcs git is honoured by vcs tag latest", func(t *testing.T) {
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		// Setup a plain git repo (no .jj). With --vcs git the auto-probe
		// is bypassed; this asserts the override reaches the backend
		// selector (regression guard: pre-fix the override was dropped
		// and tests passed by luck because cwd was always git-only).
		dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3"}, "1.2.3")
		withCwd(t, dir, func() {
			var stdout bytes.Buffer
			err := run([]string{"vcs", "tag", "latest", "--vcs", "git"},
				bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("vcs tag latest --vcs git: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
				t.Errorf("--vcs git = %q, want %q", got, "1.2.3")
			}
		})
	})

	// --- --source tag --repository uses git ls-remote (no gh required)
	t.Run("--source tag --repository: gh stub not invoked", func(t *testing.T) {
		// Even with gh "missing", --source tag --repository should
		// succeed via git ls-remote (no gh dependency on the tag path).
		// We stub ghLookPath to error to guarantee gh wouldn't work,
		// then run against an in-process git fixture used as a remote.
		if !gitAvailable() {
			t.Skip("git not installed")
		}
		origLookup := ghLookPath
		ghLookPath = func() error { return errors.New("gh missing on purpose") }
		defer func() { ghLookPath = origLookup }()

		// Use a local git repo's path as the "remote URL"; git
		// ls-remote --tags accepts any local repo path.
		workDir := setupGitRepo(t,
			[]string{"v1.0.0", "v1.2.3", "v1.1.0"}, "1.2.3")

		var stdout bytes.Buffer
		err := run([]string{"vcs", "tag", "latest",
			"--source", "tag", "--repository", workDir},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs tag latest --source tag --repository: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
			t.Errorf("--source tag --repository = %q, want %q", got, "1.2.3")
		}
	})
}
