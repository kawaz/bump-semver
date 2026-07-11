package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VcsGet_NoArgs: `vcs get` with no key shows the vcs-get help.
func TestRun_VcsGet_NoArgs(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"vcs", "get"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs get (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "root") || !strings.Contains(stdout.String(), "backend") {
		t.Errorf("expected vcs get help mentioning root/backend, got: %q", stdout.String())
	}
}

// TestRun_VcsGet_UnknownKey: an unknown key is a usage error (exit 2)
// and the error names the available keys.
func TestRun_VcsGet_UnknownKey(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown key")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "root") {
			t.Errorf("error should mention available keys, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsGet_Backend_Git: prints "git" on a git-only repo.
func TestRun_VcsGet_Backend_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Backend_Jj: prints "jj" on a colocated git+jj repo
// (jj wins over git per DR-0008 precedence).
func TestRun_VcsGet_Backend_Jj(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "jj" {
			t.Errorf("backend = %q, want jj", got)
		}
	})
}

// TestRun_VcsGet_Root_Git: prints the repo root path.
func TestRun_VcsGet_Root_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "root"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get root: %v", err)
		}
		got := strings.TrimSpace(stdout.String())
		if got == "" {
			t.Errorf("Root should be non-empty, got empty string")
		}
		// Compare via EvalSymlinks because macOS /var/folders symlinks
		// through to /private/var.
		gotCanon, _ := filepath.EvalSymlinks(got)
		wantCanon, _ := filepath.EvalSymlinks(dir)
		if gotCanon != wantCanon {
			t.Errorf("root = %q (canon %q), want %q", got, gotCanon, wantCanon)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Git: prints "main" for the fixture.
func TestRun_VcsGet_CurrentBranch_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get current-branch: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "main" {
			t.Errorf("current-branch = %q, want main", got)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Detached: detached HEAD returns exit 4
// (exitCodeAmbiguous), not the standard exit 2 (usage).
func TestRun_VcsGet_CurrentBranch_Detached(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "checkout", "--detach", "HEAD")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error on detached HEAD")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exit %d (ambiguous), got: %v", exitCodeAmbiguous, err)
		}
	})
}

// TestRun_VcsGet_Backend_VcsOverride: --vcs git on a colocated repo
// forces the git backend (was jj otherwise).
func TestRun_VcsGet_Backend_VcsOverride(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "--vcs", "git"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend --vcs git: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend (--vcs git) = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Quiet: -q suppresses the stdout value but the command
// still exits 0.
func TestRun_VcsGet_Quiet(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "-q"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend -q: %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -q, got: %q", got)
		}
	})
}

// TestRun_VcsGet_NoRepo: outside a vcs repo, `vcs get backend` should
// report exit 3 (VCS exec / not-a-repo) — distinct from the get's own
// usage errors.
func TestRun_VcsGet_NoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// --- DR-0020 PR-2: `vcs is` integration tests -----------------------------

// TestRun_VcsGet_RejectNameStatusShort: `vcs get -s root` must exit 2
// (verb-local flag for diff, not valid on get).
func TestRun_VcsGet_RejectNameStatusShort(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "-s", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get -s`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("expected stderr to mention 'unknown flag', got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "-s") {
		t.Errorf("expected stderr to name the offending flag '-s', got: %q", stderr.String())
	}
}

// TestRun_VcsGet_RejectNameStatusLong: long form must also exit 2.
func TestRun_VcsGet_RejectNameStatusLong(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--name-status", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --name-status`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--name-status") {
		t.Errorf("expected stderr to name '--name-status', got: %q", stderr.String())
	}
}

// TestRun_VcsGet_RejectUnknownFlag: a completely unknown flag is also
// rejected (covers the generic catch-all, not just -s/--name-status).
func TestRun_VcsGet_RejectUnknownFlag(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--foobar", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --foobar`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--foobar") {
		t.Errorf("expected stderr to name '--foobar', got: %q", stderr.String())
	}
}

// --- DR-0020 PR-4: `vcs commit` integration tests ------------------------
//
// The verb has three modes — `-m MSG PATH..` (path-scoped), `--staged -m
// MSG` (commit-all), and `--amend [-m MSG]` (fold into previous). Each
// has its own correctness story; the run-level tests below pin the
// usage-error matrix (-a rejection, mode exclusivity, dynamic hint), and
// the happy path on a real fixture per backend. Backend-level commit
// semantics are exercised in vcs_backend_test.go.

// TestRun_VcsGet_GlobalQuietStillAccepted: regression guard — global
// `-q` must still work for `vcs get` (it's a global flag, not verb-local).
func TestRun_VcsGet_GlobalQuietStillAccepted(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "-q", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get -q backend: %v", err)
		}
	})
}

// TestRun_VcsGet_DefaultBranchPath_Git: end-to-end dispatch from CLI
// argv through the git backend — verifies the new key registers in
// vcsGetKeys, dispatches to backend.DefaultBranchPath(), and prints the
// resolved absolute path to stdout.
func TestRun_VcsGet_DefaultBranchPath_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "default-branch-path"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get default-branch-path: %v", err)
		}
		got, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
		want, _ := filepath.EvalSymlinks(dir)
		if got != want {
			t.Errorf("default-branch-path = %q, want %q", got, want)
		}
	})
}

// TestRun_VcsGet_CommitID_ExplicitAtRev_Jj: DR-0040 changed the jj
// backend's *default* rev away from the mutable working copy `@`, but
// `--rev @` must still resolve to it explicitly — the old behaviour
// stays reachable, just no longer the default.
func TestRun_VcsGet_CommitID_ExplicitAtRev_Jj(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "UNCOMMITTED.txt"), "wip\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var atOut, defaultOut bytes.Buffer
		if err := run([]string{"vcs", "get", "commit-id", "--rev", "@"}, bytes.NewReader(nil), &atOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get commit-id --rev @: %v", err)
		}
		if err := run([]string{"vcs", "get", "commit-id"}, bytes.NewReader(nil), &defaultOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get commit-id: %v", err)
		}
		at := strings.TrimSpace(atOut.String())
		def := strings.TrimSpace(defaultOut.String())
		if at == def {
			t.Errorf("--rev @ (%q) should differ from the new default (%q) — @ carries an uncommitted edit", at, def)
		}
	})
}

// --- DR-0041: `vcs get repository` / `repository-url` ---------------------

// TestRun_VcsGet_Repository_UnknownKeyMentionsNewKeys: the unknown-key
// error text names every recognised key — a regression guard that
// DR-0041's "repository" / "repository-url" additions to vcsGetKeys are
// actually wired into the error message (not just appended to a list that
// some other code path re-derives).
func TestRun_VcsGet_Repository_UnknownKeyMentionsNewKeys(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown key")
		}
		for _, want := range []string{"repository", "repository-url"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("unknown-key error should mention %q, got: %q", want, stderr.String())
			}
		}
	})
}

// TestRun_VcsGet_Repository_Git_HTTPS: origin configured as a plain https
// URL — repository / repository-url should pass it through with just the
// .git suffix stripped (DR-0041 idempotent-on-https case).
func TestRun_VcsGet_Repository_Git_HTTPS(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var repoOut, urlOut bytes.Buffer
		if err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &repoOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository: %v", err)
		}
		if err := run([]string{"vcs", "get", "repository-url"}, bytes.NewReader(nil), &urlOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository-url: %v", err)
		}
		if got := strings.TrimSpace(repoOut.String()); got != "kawaz/bump-semver" {
			t.Errorf("repository = %q, want kawaz/bump-semver", got)
		}
		if got := strings.TrimSpace(urlOut.String()); got != "https://github.com/kawaz/bump-semver" {
			t.Errorf("repository-url = %q, want https://github.com/kawaz/bump-semver", got)
		}
	})
}

// TestRun_VcsGet_Repository_Git_SSH: origin configured as an scp-style ssh
// remote (the most common `git clone git@github.com:...` shape) — must
// normalize to the same slug/https-URL as the https fixture above, proving
// the two source forms converge.
func TestRun_VcsGet_Repository_Git_SSH(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "git@github.com:kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var repoOut, urlOut bytes.Buffer
		if err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &repoOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository: %v", err)
		}
		if err := run([]string{"vcs", "get", "repository-url"}, bytes.NewReader(nil), &urlOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository-url: %v", err)
		}
		if got := strings.TrimSpace(repoOut.String()); got != "kawaz/bump-semver" {
			t.Errorf("repository = %q, want kawaz/bump-semver", got)
		}
		if got := strings.TrimSpace(urlOut.String()); got != "https://github.com/kawaz/bump-semver" {
			t.Errorf("repository-url = %q, want https://github.com/kawaz/bump-semver", got)
		}
	})
}

// TestRun_VcsGet_Repository_ExplicitRemote: --remote NAME picks a
// non-origin remote by name, bypassing the default-remote selection rule
// entirely (DR-0041: explicit --remote always wins, no ambiguity check).
func TestRun_VcsGet_Repository_ExplicitRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/origin-repo.git")
	runIn(t, dir, "git", "remote", "add", "upstream", "https://github.com/kawaz/upstream-repo.git")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "repository", "--remote", "upstream"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get repository --remote upstream: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "kawaz/upstream-repo" {
			t.Errorf("repository --remote upstream = %q, want kawaz/upstream-repo", got)
		}
	})
}

// TestRun_VcsGet_Repository_NoRemotes_Ambiguous: zero configured remotes
// has no candidate to fall back on — DR-0041 maps this to exit 4
// (ambiguous), the same code family used for detached-HEAD / multi-
// bookmark current-branch.
func TestRun_VcsGet_Repository_NoRemotes_Ambiguous(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0") // no `git remote add` at all
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error with zero remotes configured")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exit %d (ambiguous), got: %v", exitCodeAmbiguous, err)
		}
	})
}

// TestRun_VcsGet_Repository_MultipleNoOrigin_Ambiguous: 2+ remotes with no
// "origin" among them is also ambiguous — there's no DR-0041 tie-break
// rule beyond the origin-name / sole-remote cases.
func TestRun_VcsGet_Repository_MultipleNoOrigin_Ambiguous(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "fork-a", "https://github.com/kawaz/fork-a.git")
	runIn(t, dir, "git", "remote", "add", "fork-b", "https://github.com/kawaz/fork-b.git")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error with 2 remotes and no origin")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exit %d (ambiguous), got: %v", exitCodeAmbiguous, err)
		}
	})
}

// TestRun_VcsGet_Repository_SingleNoOrigin_Adopted: exactly one remote
// configured (not named "origin") is adopted without needing --remote —
// DR-0041's "sole remote wins" fallback.
func TestRun_VcsGet_Repository_SingleNoOrigin_Adopted(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "upstream", "https://github.com/kawaz/solo-remote.git")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get repository (sole non-origin remote): %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "kawaz/solo-remote" {
			t.Errorf("repository (sole remote adopted) = %q, want kawaz/solo-remote", got)
		}
	})
}

// TestRun_VcsGet_Repository_Jj: same origin-https case as the git test,
// exercised through the jj backend (colocated repo, origin wired on the
// underlying git store per DR-0041's "jj sees remotes via the colocated
// git store" implementation note).
func TestRun_VcsGet_Repository_Jj(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "git@github.com:kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var repoOut, urlOut bytes.Buffer
		if err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &repoOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository (jj): %v", err)
		}
		if err := run([]string{"vcs", "get", "repository-url"}, bytes.NewReader(nil), &urlOut, &bytes.Buffer{}); err != nil {
			t.Fatalf("vcs get repository-url (jj): %v", err)
		}
		if got := strings.TrimSpace(repoOut.String()); got != "kawaz/bump-semver" {
			t.Errorf("repository (jj) = %q, want kawaz/bump-semver", got)
		}
		if got := strings.TrimSpace(urlOut.String()); got != "https://github.com/kawaz/bump-semver" {
			t.Errorf("repository-url (jj) = %q, want https://github.com/kawaz/bump-semver", got)
		}
	})
}

// TestRun_VcsGet_Repository_Jj_ExplicitRemote: --remote NAME on the jj
// backend — exercises the `jj git remote list` parse + lookup path
// (distinct code path from git's `git remote get-url`).
func TestRun_VcsGet_Repository_Jj_ExplicitRemote(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/origin-repo.git")
	runIn(t, dir, "git", "remote", "add", "upstream", "https://github.com/kawaz/upstream-repo.git")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "repository", "--remote", "upstream"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get repository --remote upstream (jj): %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "kawaz/upstream-repo" {
			t.Errorf("repository --remote upstream (jj) = %q, want kawaz/upstream-repo", got)
		}
	})
}

// TestRun_VcsGet_Repository_LocalRemote_VCSExec: origin configured as a
// local filesystem path (the shape `setupGitRepoWithRemote` fixtures use,
// and also the shape a colocated jj repo's bare backing store would have)
// must surface as exit 3 through the full `run()` path — this is the
// design-impl B-direction check the DESIGN.md validator table claims but
// the unit-level `TestNormalizeRemoteURL/absolute_local_path` case (which
// only checks normalizeRemoteURL's own error return) never exercised: it
// pins the emitVcsErr non-*exitErr → exitCodeVCSExec mapping so a future
// refactor that drops that mapping (or wraps the error differently) shows
// up as a broken test instead of a silent exit-code regression.
func TestRun_VcsGet_Repository_LocalRemote_VCSExec(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for a local-filesystem-path remote")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs-exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsGet_Repository_ExplicitRemote_Unresolved_VCSExec: `--remote
// NAME` naming a remote that doesn't exist is exit 3 on both backends
// (README/DESIGN's documented contract) but the two implementations reach
// it via different code paths — git lets `git remote get-url <name>`
// fail and passes its subprocess error through, jj's RemoteURL synthesizes
// a "no such remote" *exitErr by hand since `jj git remote list` doesn't
// error on an absent name. Both need their own regression test since
// either implementation could independently drift off exit 3.
func TestRun_VcsGet_Repository_ExplicitRemote_Unresolved_VCSExec(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository", "--remote", "no-such-remote"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for an unresolved --remote name")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs-exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsGet_Repository_Jj_ExplicitRemote_Unresolved_VCSExec: jj
// counterpart of the above — exercises jjBackend.RemoteURL's hand-
// synthesized "no such remote" *exitErr path (distinct from git's
// subprocess-error passthrough).
func TestRun_VcsGet_Repository_Jj_ExplicitRemote_Unresolved_VCSExec(t *testing.T) {
	t.Parallel()
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository", "--remote", "no-such-remote"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for an unresolved --remote name (jj)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs-exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsGet_Repository_RemoteFlag_RejectDash: `vcs get repository
// --remote -X` must be rejected as a usage error (exit 2) before ever
// reaching the backend — vcs_cmd.go's C-1 validateRemote gate on the
// explicit-remote path. TestValidateRemote_LeadingDash already pins
// validateRemote() in isolation; this test goes through the full `run()`
// argv-parsing + dispatch path so a future refactor that stops calling
// validateRemote on this branch (or a flag-parsing change that mangles a
// leading-dash value before it reaches the gate) shows up as a failing
// test instead of `git remote get-url -X` silently reaching the backend.
func TestRun_VcsGet_Repository_RemoteFlag_RejectDash(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "remote", "add", "origin", "https://github.com/kawaz/bump-semver.git")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "repository", "--remote", "-X"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for a leading-dash --remote value")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "-X") {
			t.Errorf("expected stderr to name the rejected value, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsGet_RemoteFlag_RejectedOnOtherKeys: --remote is scoped to
// repository / repository-url (DR-0041 gating, same shape as --rev's
// commit-id-only gate). Using it with an unrelated key (`root`) is a
// usage error, not silently ignored.
func TestRun_VcsGet_RemoteFlag_RejectedOnOtherKeys(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "root", "--remote", "origin"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get root --remote`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--remote") {
		t.Errorf("expected stderr to name '--remote', got: %q", stderr.String())
	}
}

// --- DR-0020 PR-5: vcs fetch / vcs push dispatcher tests ------------------
