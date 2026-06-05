package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestVcsParseSpec covers DR-0008's spec parsing rules: rev mode with
// and without a borrowed file, function mode, and the `:` boundary
// disambiguation.
func TestVcsParseSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		spec       string
		wantRev    string
		wantFile   string
		wantIsFunc bool
		wantFunc   string
	}{
		{"rev-and-file", "vcs:HEAD~1:Cargo.toml", "HEAD~1", "Cargo.toml", false, ""},
		{"rev-only", "vcs:HEAD~1", "HEAD~1", "", false, ""},
		{"jj-bookmark-at-remote", "vcs:main@origin", "main@origin", "", false, ""},
		{"jj-remote-slash", "vcs:origin/main", "origin/main", "", false, ""},
		{"function-no-args", "vcs:latest-tag()", "", "", true, "latest-tag"},
		{"function-with-spaces-empty", "vcs:latest-tag( )", " ", "", true, "latest-tag"},
		{"function-with-owner-repo", "vcs:latest-tag(kawaz/pkf-tasks)", "kawaz/pkf-tasks", "", true, "latest-tag"},
		{"function-with-https-url", "vcs:latest-tag(https://github.com/kawaz/pkf-tasks)", "https://github.com/kawaz/pkf-tasks", "", true, "latest-tag"},
		{"unknown-function", "vcs:current-branch()", "", "", true, "current-branch"},
		// Path with subdirectories shouldn't trip up the `:` split.
		{"rev-and-nested-file", "vcs:HEAD:src/Cargo.toml", "HEAD", "src/Cargo.toml", false, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rev, file, isFunc, funcName := vcsParseSpec(tc.spec)
			if rev != tc.wantRev || file != tc.wantFile || isFunc != tc.wantIsFunc || funcName != tc.wantFunc {
				t.Errorf("vcsParseSpec(%q) = (rev=%q, file=%q, isFunc=%v, func=%q)\n  want = (rev=%q, file=%q, isFunc=%v, func=%q)",
					tc.spec, rev, file, isFunc, funcName,
					tc.wantRev, tc.wantFile, tc.wantIsFunc, tc.wantFunc)
			}
		})
	}
}

// TestExpandRepoArg covers the remote-arg expansion used by
// `vcs:latest-tag(<arg>)`. Short GitHub-style `owner/repo` is expanded
// to a full HTTPS URL; full URLs and SSH URLs pass through unchanged;
// the empty string represents "no arg — use cwd VCS".
func TestExpandRepoArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"kawaz/pkf-tasks", "https://github.com/kawaz/pkf-tasks"},
		{"  kawaz/pkf-tasks  ", "https://github.com/kawaz/pkf-tasks"}, // whitespace trim
		{"https://github.com/kawaz/pkf-tasks", "https://github.com/kawaz/pkf-tasks"},
		{"http://example.com/x/y", "http://example.com/x/y"},
		{"git@github.com:kawaz/pkf-tasks.git", "git@github.com:kawaz/pkf-tasks.git"},
		{"ssh://git@github.com/kawaz/pkf-tasks.git", "ssh://git@github.com/kawaz/pkf-tasks.git"},
		{"too/many/slashes", "too/many/slashes"}, // unknown shape, pass-through (ls-remote will fail)
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := expandRepoArg(tc.in)
			if got != tc.want {
				t.Errorf("expandRepoArg(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseVcsOverride covers the --vcs value validation done at the
// CLI layer. Both "" (flag not passed) and "auto" resolve to vcsAuto so
// the probe runs.
func TestParseVcsOverride(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    vcsKind
		wantErr bool
	}{
		{"", vcsAuto, false},
		{"auto", vcsAuto, false},
		{"jj", vcsJj, false},
		{"git", vcsGit, false},
		{"foo", vcsAuto, true},
		{"JJ", vcsAuto, true},   // case-sensitive on purpose; users see help
		{"AUTO", vcsAuto, true}, // case-sensitive
	}
	for _, tc := range cases {
		got, err := parseVcsOverride(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseVcsOverride(%q) err=%v want err=%v", tc.in, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseVcsOverride(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestAltJjRev pins the `origin/main` → `main@origin` fallback rule.
// Pure jj-syntax (`main@origin`) has no `/` so it stays as-is; nested
// slashes (`feature/foo/bar`) are explicitly NOT remapped because the
// remote-name boundary is ambiguous.
func TestAltJjRev(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantOut string
		wantOk  bool
	}{
		{"origin/main", "main@origin", true},
		{"upstream/feature", "feature@upstream", true},
		{"main@origin", "", false},
		{"HEAD~1", "", false},
		{"main", "", false},
		{"", "", false},
		{"a/b/c", "", false}, // multiple slashes: ambiguous
		{"/leading", "", false},
	}
	for _, tc := range cases {
		got, ok := altJjRev(tc.in)
		if got != tc.wantOut || ok != tc.wantOk {
			t.Errorf("altJjRev(%q) = (%q, %v), want (%q, %v)",
				tc.in, got, ok, tc.wantOut, tc.wantOk)
		}
	}
}

// TestSplitAndDedup covers the deduplicating line-splitter used to
// normalise jj/git tag listings (jj's `log -r tags()` template can
// duplicate a tag if it shows up on multiple changes).
func TestSplitAndDedup(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n\n\n", nil},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\na\nb\na", []string{"a", "b"}},
		{"  v1.0.0  \nv2.0.0\n", []string{"v1.0.0", "v2.0.0"}},
	}
	for _, tc := range cases {
		got := splitAndDedup(tc.in)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitAndDedup(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- fixture-driven tests --------------------------------------------------
//
// The remaining tests need a real git or jj repository. We build them
// in t.TempDir() so they run hermetically in CI. jj availability is
// probed and tests are t.Skip()-ed when jj isn't installed; git is
// assumed (every dev environment we ship to has it).

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func jjAvailable() bool {
	_, err := exec.LookPath("jj")
	return err == nil
}

// runIn runs `name args...` in dir with a clean environment so the
// test isn't influenced by user gitconfig / jjconfig. We keep PATH
// (so jj can find git) and HOME pointed at a tempdir to suppress any
// signing / template defaults.
func runIn(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	runInEnv(t, dir, nil, name, args...)
}

// runInEnv is like runIn but appends extra env vars on top of the
// hermetic base. Use it to pin commit timestamps via
// GIT_AUTHOR_DATE / GIT_COMMITTER_DATE so tests don't need real-time
// sleeps to create a >=1s gap between commits.
//
// extraEnv is appended last so duplicate keys override the hermetic
// defaults (relies on exec.Cmd.Env's documented "last value wins"
// for repeated keys — see os/exec docs).
func runInEnv(t *testing.T, dir string, extraEnv []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{},
		"PATH="+t.TempDir()+":"+pathEnv(),
		"HOME="+t.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		// Disable signing on jj so tests don't talk to ssh-agent / 1Password.
		"JJ_USER=Test",
		"JJ_EMAIL=test@example.com",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v in %s: %v\n%s", name, args, dir, err, out)
	}
}

func pathEnv() string {
	out, err := exec.Command("sh", "-c", "echo $PATH").Output()
	if err != nil {
		return "/usr/bin:/bin"
	}
	return strings.TrimSpace(string(out))
}

// setupGitRepo creates a git repo with two commits and the requested
// tags pointing at HEAD. fileVersion is the version string written
// into a top-level VERSION file at HEAD; HEAD~1 has version "0.0.1".
func setupGitRepo(t *testing.T, tags []string, fileVersion string) string {
	t.Helper()
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := writeFile(filepath.Join(dir, "VERSION"), "0.0.1\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	if err := writeFile(filepath.Join(dir, "VERSION"), fileVersion+"\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	runIn(t, dir, "git", "commit", "-qm", "bump")
	for _, tag := range tags {
		runIn(t, dir, "git", "tag", tag)
	}
	return dir
}

// withCwd runs fn with the working directory temporarily switched to
// dir. The vcs detection probes cwd, so tests need a way to point it
// at a fixture without polluting the rest of the test suite.
func withCwd(t *testing.T, dir string, fn func()) {
	t.Helper()
	orig, err := getCwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := chdir(orig); err != nil {
			t.Fatalf("chdir back %s: %v", orig, err)
		}
	}()
	fn()
}

// Indirection through small helpers keeps the test bodies readable;
// the actual filesystem call lives in os to avoid surprises.
func getCwd() (string, error) { return os.Getwd() }
func chdir(p string) error    { return os.Chdir(p) }

// writeFile is a small wrapper that does the right thing for the test
// fixtures we set up here (mode 0644, full content).
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// TestVcsListTags_Git verifies tag enumeration on a real git fixture.
//
// These VCS-fixture tests cannot run with t.Parallel() because they
// chdir(2) the whole process to point at the fixture directory.
// Cwd is shared across goroutines, so racing tests would clobber
// each other's view of the filesystem.
func TestVcsListTags_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0", "v1.1.0", "not-a-version"}, "1.1.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.ListTags()
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		want := map[string]bool{"v1.0.0": true, "v1.1.0": true, "not-a-version": true}
		for _, g := range got {
			delete(want, g)
		}
		if len(want) != 0 {
			t.Errorf("missing tags from output: %v (got %v)", want, got)
		}
	})
}

// TestVcsLatestTag_Git: parse-failed tags are silently ignored, the
// largest semver-parseable tag wins. Default excludes prereleases.
func TestVcsLatestTag_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3", "v1.1.0", "build-2025"}, "1.2.3")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		raw, v, err := b.LatestTag(false)
		if err != nil {
			t.Fatalf("LatestTag: %v", err)
		}
		// Prefix preserved (DR-0006), so the tag came back with `v`.
		if got := v.String(); got != "v1.2.3" {
			t.Errorf("LatestTag version = %q, want v1.2.3", got)
		}
		if raw != "v1.2.3" {
			t.Errorf("LatestTag raw = %q, want v1.2.3", raw)
		}
	})
}

// TestVcsLatestTag_Git_NoSemver: when nothing parses we error out.
func TestVcsLatestTag_Git_NoSemver(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"build-2025", "rolling"}, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, _, err := b.LatestTag(false)
		if err == nil {
			t.Fatal("expected error for no-semver-tags repo")
		}
		if !strings.Contains(err.Error(), "no semver-compatible tags") {
			t.Errorf("error should mention 'no semver-compatible tags', got: %v", err)
		}
	})
}

// TestVcsLatestTag_Git_Prerelease: default filters out prereleases;
// --include-prerelease (= true) brings them back into the candidate set.
func TestVcsLatestTag_Git_Prerelease(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3-rc.1", "v1.1.0"}, "1.2.3")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		// Default: prereleases excluded → v1.1.0 wins (v1.2.3-rc.1 dropped).
		_, v, err := b.LatestTag(false)
		if err != nil {
			t.Fatalf("LatestTag(false): %v", err)
		}
		if got := v.String(); got != "v1.1.0" {
			t.Errorf("LatestTag(false) = %q, want v1.1.0 (prerelease filtered)", got)
		}
		// includePrerelease=true → v1.2.3-rc.1 wins (largest by SemVer order).
		_, v2, err := b.LatestTag(true)
		if err != nil {
			t.Fatalf("LatestTag(true): %v", err)
		}
		if got := v2.String(); got != "v1.2.3-rc.1" {
			t.Errorf("LatestTag(true) = %q, want v1.2.3-rc.1", got)
		}
	})
}

// TestVcsFetchFile_Git: read VERSION at a previous commit (HEAD~1).
func TestVcsFetchFile_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		out, err := b.FetchFile("HEAD~1", "VERSION")
		if err != nil {
			t.Fatalf("FetchFile: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != "0.0.1" {
			t.Errorf("VERSION at HEAD~1 = %q, want 0.0.1", got)
		}
	})
}

// TestDetectVcs_Git: a directory containing only `.git` resolves to git.
func TestDetectVcs_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		got, err := detectVcs(vcsAuto)
		if err != nil {
			t.Fatalf("detectVcs: %v", err)
		}
		if got != vcsGit {
			t.Errorf("detectVcs = %v, want vcsGit", got)
		}
	})
}

// TestDetectVcs_NoRepo: no .git / .jj anywhere in the chain is an error.
func TestDetectVcs_NoRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		_, err := detectVcs(vcsAuto)
		if err == nil {
			t.Fatal("expected error in non-vcs directory")
		}
		if !strings.Contains(err.Error(), "not a git or jj repository") {
			t.Errorf("error should mention 'not a git or jj repository', got: %v", err)
		}
	})
}

// TestDetectVcs_Override: --vcs flag bypasses the probe.
func TestDetectVcs_Override(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		got, err := detectVcs(vcsGit)
		if err != nil {
			t.Fatalf("detectVcs(git override): %v", err)
		}
		if got != vcsGit {
			t.Errorf("override should win, got %v", got)
		}
	})
}

// --- jj-flavoured fixture tests --------------------------------------------
//
// jj coexists with git (kawaz's git-bare + jj-workspace layout has both),
// so these tests build a colocated git+jj fixture: `git init` first,
// then `jj git init --git-repo`. Tag/commit creation is done through git
// because that lets us avoid jj's identity / signing requirements (which
// would normally need an ssh-agent / 1Password connection in real
// environments). After the git side is set up we run `jj git import`
// implicitly by invoking jj — the import happens lazily on first read.

// setupJjRepo creates a colocated git+jj repo with two commits and the
// requested tags. Since jj reads from the underlying git store, this
// gives us a realistic-looking jj repo without ever invoking jj's
// commit signing path.
func setupJjRepo(t *testing.T, tags []string, fileVersion string) string {
	t.Helper()
	dir := setupGitRepo(t, tags, fileVersion)
	// Initialise jj on top of the existing git directory. We use
	// `--git-repo .git` so jj reads the colocated git data without
	// trying to take ownership of the working copy.
	runIn(t, dir, "jj", "git", "init", "--git-repo", ".git")
	// DR-0020 PR-4: backend Commit goes through `runBackendCmd` which
	// inherits the test process env (= user's $HOME), so a user with
	// `signing.key` set globally would have every `jj commit` attempt to
	// sign through ssh-agent / 1Password. Drop signing at the repo-config
	// layer (`.jj/repo/config.toml`); jj's config-precedence has repo-local
	// override the user file, so `signing.behavior = "drop"` here wins for
	// THIS repo regardless of the host's global signing.key.
	if err := writeFile(filepath.Join(dir, ".jj/repo/config.toml"),
		"[signing]\nbehavior = \"drop\"\n"); err != nil {
		t.Fatalf("write jj repo-local config: %v", err)
	}
	return dir
}

// TestDetectVcs_JjOverGit: when both .jj and .git exist, jj wins.
// This is the kawaz git-bare + jj-workspace layout we ship to.
func TestDetectVcs_JjOverGit(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		got, err := detectVcs(vcsAuto)
		if err != nil {
			t.Fatalf("detectVcs: %v", err)
		}
		if got != vcsJj {
			t.Errorf("jj should win over git, got %v", got)
		}
	})
}

// TestVcsListTags_Jj: jj's `log -r tags()` template produces the same
// tags as git, modulo dedup.
func TestVcsListTags_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, []string{"v1.0.0", "v2.0.0", "build-x"}, "2.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		got, err := b.ListTags()
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		want := map[string]bool{"v1.0.0": true, "v2.0.0": true, "build-x": true}
		for _, g := range got {
			delete(want, g)
		}
		if len(want) != 0 {
			t.Errorf("missing tags from output: %v (got %v)", want, got)
		}
	})
}

// TestVcsFetchFile_Jj: read VERSION at the previous commit via jj.
//
// `jj git init --git-repo` puts @ on a fresh empty change above HEAD,
// so `@-` resolves to the bump commit (1.2.3) and `@--` to the initial
// commit (0.0.1). We probe the second-back reference here.
func TestVcsFetchFile_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		out, err := b.FetchFile("@--", "VERSION")
		if err != nil {
			t.Fatalf("FetchFile: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != "0.0.1" {
			t.Errorf("VERSION at @-- = %q, want 0.0.1", got)
		}
	})
}

// setupGitRepoWithRemote wraps setupGitRepo by also creating a sibling
// `bare.git` directory next to the workdir and adding it as `origin`.
// The bare starts empty; tests that want a pre-populated remote should
// push the main branch as part of their arrange phase. Returns the work
// directory (matching setupGitRepo's contract) and the bare path so tests
// can introspect or mutate the remote.
//
// PR-5 needs an actual remote to exercise fetch/push end-to-end. A bare
// repo at a local filesystem path satisfies git/jj's protocol expectations
// without going over the network and without violating the "no real
// git/jj push outside fixtures" constraint.
func setupGitRepoWithRemote(t *testing.T, tags []string, fileVersion string) (workDir, bareDir string) {
	t.Helper()
	workDir = setupGitRepo(t, tags, fileVersion)
	// Place the bare repo as a sibling to the workDir under its own
	// parent (t.TempDir creates a per-test root for setupGitRepo; we add
	// the bare under that same parent so cleanup is automatic).
	bareDir = filepath.Join(filepath.Dir(workDir), "bare.git")
	// Use -b main so the bare's HEAD matches the work-dir's default
	// branch — otherwise the bare defaults to master under runIn's
	// GIT_CONFIG_GLOBAL=/dev/null env, and `git clone bare.git` would
	// fail to check out main when an attacker fixture tries to diverge.
	runIn(t, filepath.Dir(workDir), "git", "init", "--bare", "-q", "-b", "main", "bare.git")
	runIn(t, workDir, "git", "remote", "add", "origin", bareDir)
	return workDir, bareDir
}

// setupJjRepoWithRemote is the jj-flavoured counterpart to
// setupGitRepoWithRemote — same layout (colocated git+jj workdir +
// sibling bare.git) with origin pre-wired on the git side. jj sees the
// remote via the colocated git store, so `jj git fetch --remote origin`
// and `jj git push --remote origin` Just Work.
func setupJjRepoWithRemote(t *testing.T, tags []string, fileVersion string) (workDir, bareDir string) {
	t.Helper()
	workDir = setupJjRepo(t, tags, fileVersion)
	bareDir = filepath.Join(filepath.Dir(workDir), "bare.git")
	// Use -b main so the bare's HEAD matches the work-dir's default
	// branch — otherwise the bare defaults to master under runIn's
	// GIT_CONFIG_GLOBAL=/dev/null env, and `git clone bare.git` would
	// fail to check out main when an attacker fixture tries to diverge.
	runIn(t, filepath.Dir(workDir), "git", "init", "--bare", "-q", "-b", "main", "bare.git")
	runIn(t, workDir, "git", "remote", "add", "origin", bareDir)
	return workDir, bareDir
}

// setupJjRepoNonColocatedWithRemote mirrors setupJjRepoWithRemote but for
// the **non-colocated** layout (= jj's git store is a separate bare repo,
// not a `.git/` directory inside the work tree). This matches kawaz's
// production "git bare + jj workspace" setup (DR-0020 line 105) which the
// default `setupJjRepoWithRemote` (colocated) doesn't exercise.
//
// Layout:
//
//	root/
//	  stage/            transient seeding tree (deleted before return)
//	  backing.git/      bare repo jj uses as its git store
//	  origin.git/       bare repo configured as `origin` on backing.git
//	  work/             jj workspace (no `.git/` inside)
//
// Two commits ("init" + "bump") are seeded into backing.git via the
// stage worktree, then stage is removed so the only worktree-shaped
// directory tests see is `work/`. `git_target` ends up an absolute path
// to backing.git, exercising the `jjGitPushDir()` branch that returns
// that path for the `git -C <bare> push` step.
//
// Returns the work directory and origin bare path (= the remote tests
// assert against). backing.git is internal plumbing — tests don't need
// to know about it.
func setupJjRepoNonColocatedWithRemote(t *testing.T, fileVersion string) (workDir, originBare string) {
	t.Helper()
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	backing := filepath.Join(root, "backing.git")
	originBare = filepath.Join(root, "origin.git")
	workDir = filepath.Join(root, "work")

	// Seed two commits via a stage worktree (jj non-colocated init takes
	// an EXISTING git repo as its store, so we need history first).
	runIn(t, root, "git", "init", "-q", "-b", "main", "stage")
	runIn(t, stage, "git", "config", "user.name", "Test")
	runIn(t, stage, "git", "config", "user.email", "test@example.com")
	runIn(t, stage, "git", "config", "commit.gpgsign", "false")
	if err := writeFile(filepath.Join(stage, "VERSION"), "0.0.1\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, stage, "git", "add", "VERSION")
	runIn(t, stage, "git", "commit", "-qm", "initial")
	if err := writeFile(filepath.Join(stage, "VERSION"), fileVersion+"\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, stage, "git", "add", "VERSION")
	runIn(t, stage, "git", "commit", "-qm", "bump")

	// Create backing.git and push history into it.
	runIn(t, root, "git", "init", "--bare", "-q", "-b", "main", "backing.git")
	runIn(t, stage, "git", "remote", "add", "backing", backing)
	runIn(t, stage, "git", "push", "-q", "backing", "main")

	// Create origin.git and wire it on backing.git (not on work/, since
	// non-colocated work doesn't have a `.git` to hold remote config —
	// the remote lives on the backing store).
	runIn(t, root, "git", "init", "--bare", "-q", "-b", "main", "origin.git")
	runIn(t, backing, "git", "remote", "add", "origin", originBare)

	// jj init non-colocated.
	runIn(t, root, "jj", "git", "init", "--git-repo", backing, "work")
	if err := writeFile(filepath.Join(workDir, ".jj/repo/config.toml"),
		"[signing]\nbehavior = \"drop\"\n"); err != nil {
		t.Fatalf("write jj repo-local config: %v", err)
	}

	// Drop the stage worktree so tests see only the non-colocated layout
	// (no risk of a test running git from stage by accident).
	if err := os.RemoveAll(stage); err != nil {
		t.Fatalf("rm stage: %v", err)
	}
	return workDir, originBare
}

// preloadBareWith pushes the work-dir's main branch to the bare so the
// bare starts in a "remote already has this history" state. Useful for
// fetch tests (where we want some refs to fetch) and forward-push tests
// (so the next push is a forward, not a new-branch creation).
func preloadBareWith(t *testing.T, workDir string) {
	t.Helper()
	runIn(t, workDir, "git", "push", "-q", "origin", "main:main")
}

// divergeBareViaAttacker simulates a "remote was pushed to by someone
// else" scenario: it clones the bare, adds a divergent commit, and force-
// pushes back. After this the local workDir's main is one commit behind
// the bare on a divergent line — exactly the non-ff setup `vcs push`
// must reject with exit 5.
func divergeBareViaAttacker(t *testing.T, bareDir string) {
	t.Helper()
	parent := filepath.Dir(bareDir)
	attacker := filepath.Join(parent, "attacker")
	runIn(t, parent, "git", "clone", "-q", bareDir, attacker)
	runIn(t, attacker, "git", "config", "user.name", "Attacker")
	runIn(t, attacker, "git", "config", "user.email", "att@example.com")
	runIn(t, attacker, "git", "config", "commit.gpgsign", "false")
	if err := writeFile(filepath.Join(attacker, "diverge.txt"), "diverged on remote\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, attacker, "git", "add", "diverge.txt")
	runIn(t, attacker, "git", "commit", "-qm", "divergent")
	// Force-push so the bare's main now points at the attacker's tip.
	runIn(t, attacker, "git", "push", "-q", "--force", "origin", "main:main")
}
