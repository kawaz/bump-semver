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

// TestParseVcsOverride covers the --vcs / BUMP_SEMVER_VCS value
// validation done at the CLI layer.
func TestParseVcsOverride(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    vcsKind
		wantErr bool
	}{
		{"", vcsAuto, false},
		{"jj", vcsJj, false},
		{"git", vcsGit, false},
		{"foo", vcsAuto, true},
		{"JJ", vcsAuto, true}, // case-sensitive on purpose; users see help
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
		got, err := vcsListTags(vcsGit)
		if err != nil {
			t.Fatalf("vcsListTags: %v", err)
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
// largest semver-parseable tag wins.
func TestVcsLatestTag_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0", "v1.2.3", "v1.1.0", "build-2025"}, "1.2.3")
	withCwd(t, dir, func() {
		v, err := vcsLatestTag(vcsGit)
		if err != nil {
			t.Fatalf("vcsLatestTag: %v", err)
		}
		// Prefix preserved (DR-0006), so the tag came back with `v`.
		if got := v.String(); got != "v1.2.3" {
			t.Errorf("vcsLatestTag = %q, want v1.2.3", got)
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
		_, err := vcsLatestTag(vcsGit)
		if err == nil {
			t.Fatal("expected error for no-semver-tags repo")
		}
		if !strings.Contains(err.Error(), "no semver-compatible tags") {
			t.Errorf("error should mention 'no semver-compatible tags', got: %v", err)
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
		out, err := vcsFetchFile(vcsGit, "HEAD~1", "VERSION")
		if err != nil {
			t.Fatalf("vcsFetchFile: %v", err)
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
		got, err := vcsListTags(vcsJj)
		if err != nil {
			t.Fatalf("vcsListTags: %v", err)
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
		out, err := vcsFetchFile(vcsJj, "@--", "VERSION")
		if err != nil {
			t.Fatalf("vcsFetchFile: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != "0.0.1" {
			t.Errorf("VERSION at @-- = %q, want 0.0.1", got)
		}
	})
}
