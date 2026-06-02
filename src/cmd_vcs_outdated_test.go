package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupOutdatedGitRepo builds a git fixture for `vcs outdated` tests.
// The history is shaped so a source file's last commit is strictly
// newer than its derived file's last commit (= the "stale" branch).
//
// Layout (after init):
//
//	README.md         (committed first, then re-touched at the end)
//	README-ja.md      (committed only at init; never re-touched → stale)
//	README-en.md      (committed only at init; never re-touched → stale)
//	src/foo.ts        (committed at init, re-touched at end → has 2 commits)
//	src/sub/bar.ts    (committed only at init → 1 commit)
//	lib/foo.js        (committed only at init → stale once foo.ts is bumped)
//	lib/sub/bar.js    (committed only at init → fresh — bar.ts wasn't bumped)
//
// Returns the directory.
func setupOutdatedGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")

	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Initial commit: everything present.
	mk("README.md", "en1")
	mk("README-ja.md", "ja1")
	mk("README-en.md", "en-en1")
	mk("src/foo.ts", "foo1")
	mk("src/sub/bar.ts", "bar1")
	mk("lib/foo.js", "foo.js1")
	mk("lib/sub/bar.js", "bar.js1")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	// Sleep so the second commit's ts is strictly greater than the
	// first's. git's %ct is second-granularity, so we need a 1-second
	// gap (or more) for the comparison to register as "newer".
	sleepOneSecond(t)
	// Second commit: bump README.md and src/foo.ts only (= derived
	// translations and lib/foo.js are now stale; lib/sub/bar.js stays
	// fresh because src/sub/bar.ts wasn't re-touched).
	mk("README.md", "en2")
	mk("src/foo.ts", "foo2")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "bump source files")
	return dir
}

// sleepOneSecond pauses for >=1 second so git's second-granularity
// committer timestamps register a different value on the next commit.
// 1.1s gives a safety margin against scheduling jitter.
func sleepOneSecond(t *testing.T) {
	t.Helper()
	time.Sleep(1100 * time.Millisecond)
}

// TestRun_VcsOutdated_T2_Translation: literal-FROM + mandatory `{}` TO.
// README.md was re-touched; README-{ja,en}.md weren't → exit 1.
func TestRun_VcsOutdated_T2_Translation(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "README-{ja,en}.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected stale exit, got nil (stderr=%s)", stderr.String())
		}
		ee, ok := err.(*exitErr)
		if !ok {
			t.Fatalf("expected *exitErr, got %T: %v", err, err)
		}
		if ee.code != exitCodeFalse {
			t.Errorf("exit code = %d, want %d (false/stale)", ee.code, exitCodeFalse)
		}
		// Both ja and en should appear in stderr stale reports.
		stderrS := stderr.String()
		for _, want := range []string{"README-ja.md", "README-en.md", "stale"} {
			if !strings.Contains(stderrS, want) {
				t.Errorf("stderr missing %q:\n%s", want, stderrS)
			}
		}
	})
}

// TestRun_VcsOutdated_T1_Bundle: glob FROM with $1/$2 backrefs.
// src/foo.ts → lib/foo.js (stale: foo.ts re-bumped)
// src/sub/bar.ts → lib/sub/bar.js (fresh: bar.ts unchanged since init)
// At least one stale → exit 1.
func TestRun_VcsOutdated_T1_Bundle(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "glob:src/**/*.ts", "lib/$1/$2.js"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected stale exit, got nil (stderr=%s)", stderr.String())
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
		stderrS := stderr.String()
		// foo.js should report as stale; bar.js fresh (not in stderr).
		if !strings.Contains(stderrS, "lib/foo.js") {
			t.Errorf("expected stale lib/foo.js in stderr:\n%s", stderrS)
		}
		if strings.Contains(stderrS, "lib/sub/bar.js") {
			t.Errorf("did NOT expect lib/sub/bar.js in stderr (should be fresh):\n%s", stderrS)
		}
	})
}

// TestRun_VcsOutdated_T1_AllFresh: when no source has moved beyond its
// derived, exit 0 with no output.
func TestRun_VcsOutdated_T1_AllFresh(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("src/foo.ts", "ts")
	mk("lib/foo.js", "js")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	// Re-touch derived AFTER source so derived is strictly newer.
	sleepOneSecond(t)
	mk("lib/foo.js", "js2")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "regen lib")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "glob:src/**/*.ts", "lib/$1/$2.js"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Errorf("expected fresh exit 0, got %v (stderr=%s)", err, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("expected empty stdout on fresh, got %q", stdout.String())
		}
	})
}

// TestRun_VcsOutdated_Explain: --explain emits the full FROM→TO table
// and exits 0 regardless of stale rows.
func TestRun_VcsOutdated_Explain(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain", "README.md", "README-{ja,en}.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain should exit 0 even with stale rows, got %v (stderr=%s)",
				err, stderr.String())
		}
		out := stdout.String()
		// Both derived rows must appear with status detail.
		for _, want := range []string{"README-ja.md", "README-en.md", "→", "stale"} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout missing %q:\n%s", want, out)
			}
		}
	})
}

// TestRun_VcsOutdated_MultiPair: multiple `--`-separated pairs, exit 1
// if any pair is stale.
func TestRun_VcsOutdated_MultiPair(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		// Pair 1: README → translations (stale)
		// Pair 2: src/sub/bar.ts → lib/sub/bar.js (fresh)
		// Aggregate: at least one stale → exit 1.
		err := run([]string{
			"vcs", "outdated",
			"--",
			"README.md", "README-{ja,en}.md",
			"--",
			"glob:src/sub/*.ts", "lib/sub/$1.js",
		}, bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected stale exit, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
		stderrS := stderr.String()
		if !strings.Contains(stderrS, "README-ja.md") {
			t.Errorf("pair 1 (README) stale row missing:\n%s", stderrS)
		}
	})
}

// TestRun_VcsOutdated_MissingMandatory: TO with `{ja,en}` where one of
// the options is absent → exit 1 with `missing` status.
func TestRun_VcsOutdated_MissingMandatory(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("en"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README-ja.md"), []byte("ja"), 0o644); err != nil {
		t.Fatal(err)
	}
	// README-en.md is missing — `{ja,en}` requires both.
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "README-{ja,en}.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected exit 1 for missing mandatory, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
		stderrS := stderr.String()
		if !strings.Contains(stderrS, "missing") {
			t.Errorf("stderr should mention `missing`:\n%s", stderrS)
		}
		if !strings.Contains(stderrS, "README-en.md") {
			t.Errorf("stderr should name README-en.md:\n%s", stderrS)
		}
	})
}

// TestRun_VcsOutdated_AutoExclude: when FROM `glob:README*.md` matches
// both README.md and README-ja.md and TO is `README-{ja,en}.md`, the
// source path itself should NOT be flagged as its own derived.
func TestRun_VcsOutdated_AutoExclude(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	for _, f := range []string{"README.md", "README-ja.md", "README-en.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain", "README.md", "README-{ja,en}.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain err: %v (stderr=%s)", err, stderr.String())
		}
		out := stdout.String()
		// Source README.md must NOT appear as a derived (= no row
		// "README.md  →  README.md").
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "README.md  →  README.md") {
				t.Errorf("auto-exclude failed: source appears as its own derived:\n%s", out)
			}
		}
	})
}

// TestRun_VcsOutdated_GlobFlagsApply verifies the --glob-* family is
// actually threaded through to FROM expansion. We place a source under
// a .gitignored path and assert it's excluded by default (gitignored
// respected) and included with --glob-gitignored=false.
func TestRun_VcsOutdated_GlobFlagsApply(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// .gitignore excludes 'ignored/' so glob expansion under defaults
	// should skip files under it. The FROM glob is broad enough to
	// match BOTH paths; the gate is the gitignore policy.
	mk(".gitignore", "ignored/\n")
	mk("src/a.ts", "a")
	mk("ignored/b.ts", "b")
	mk("lib/a.js", "ajs")
	mk("lib/b.js", "bjs")
	runIn(t, dir, "git", "add", "src", "lib", ".gitignore")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		// Default (gitignored respected): only src/a.ts is a source;
		// the only derived row is lib/a.js. lib/b.js never appears.
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:**/*.ts", "lib/$2.js"}, bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("default --explain err: %v (stderr=%s)", err, stderr.String())
		}
		if strings.Contains(stdout.String(), "ignored/b.ts") {
			t.Errorf("default should respect .gitignore; ignored/b.ts leaked:\n%s",
				stdout.String())
		}
		// --glob-gitignored=false: ignored/b.ts should now appear.
		var stdout2, stderr2 bytes.Buffer
		err = run([]string{"vcs", "outdated", "--explain",
			"--glob-gitignored=false",
			"glob:**/*.ts", "lib/$2.js"}, bytes.NewReader(nil), &stdout2, &stderr2)
		if err != nil {
			t.Fatalf("gitignored=false err: %v (stderr=%s)", err, stderr2.String())
		}
		if !strings.Contains(stdout2.String(), "ignored/b.ts") {
			t.Errorf("--glob-gitignored=false should include ignored/b.ts:\n%s",
				stdout2.String())
		}
	})
}

// TestRun_VcsOutdated_UsageError: missing args → exit 2.
func TestRun_VcsOutdated_UsageError(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected usage err, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage, got %v", err)
		}
	})
}

// TestRun_VcsOutdated_LeadingSlashDogfood: blocker #1 pin. The
// `**/*-ja.md` → `${1}/${2}.md` mapping must NOT produce a leading-slash
// derived path when the source is at root (= README-ja.md → README.md).
func TestRun_VcsOutdated_LeadingSlashDogfood(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("README.md", "en")
	mk("README-ja.md", "ja")
	mk("docs/guide.md", "g")
	mk("docs/guide-ja.md", "gja")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain", "glob:**/*-ja.md", "${1}/${2}.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain should exit 0, got %v (stderr=%s)", err, stderr.String())
		}
		out := stdout.String()
		// Expected derived: README.md (from README-ja.md) + docs/guide.md
		// (from docs/guide-ja.md). NOT /README.md (leading-slash bug).
		if strings.Contains(out, "→  /README") {
			t.Errorf("leading-slash bug regressed: %s", out)
		}
		if !strings.Contains(out, "→  README.md") {
			t.Errorf("expected `→  README.md` row, got: %s", out)
		}
		if !strings.Contains(out, "docs/guide.md") {
			t.Errorf("expected docs/guide.md row, got: %s", out)
		}
	})
}

// TestRun_VcsOutdated_StrictLiteralFromMissing: blocker #3 pin.
// Default: literal-FROM-not-found warns + exits 0.
// --strict: same case exits 1.
func TestRun_VcsOutdated_StrictLiteralFromMissing(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	// Empty repo: no README.MD-with-wrong-case file.
	if err := os.WriteFile(filepath.Join(dir, "placeholder"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		// Default: literal-FROM typo → warn on stderr, exit 0.
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.MD", "README-ja.MD"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Errorf("default: expected exit 0, got %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stderr.String(), "matched no file") {
			t.Errorf("expected warn message in stderr, got: %s", stderr.String())
		}
		// --strict: same → exit 1.
		var stdout2, stderr2 bytes.Buffer
		err = run([]string{"vcs", "outdated", "--strict", "README.MD", "README-ja.MD"},
			bytes.NewReader(nil), &stdout2, &stderr2)
		if err == nil {
			t.Fatalf("--strict: expected exit 1, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("--strict: expected exitCodeFalse, got %v", err)
		}
		// glob: with no match is NOT a literal — must NOT trigger --strict.
		var stdout3, stderr3 bytes.Buffer
		err = run([]string{"vcs", "outdated", "--strict", "glob:nothing-*.zzz", "lib/$1.out"},
			bytes.NewReader(nil), &stdout3, &stderr3)
		if err != nil {
			t.Errorf("glob: with no match must not fail --strict, got %v (stderr=%s)", err, stderr3.String())
		}
	})
}

// TestRun_VcsOutdated_TOReGlobLiteralEmbed: blocker #4 pin.
// A captured value containing glob meta (`*`) must NOT be re-expanded by
// the TO `glob:` second-stage walk. We exercise the actual verb path with
// a pathological filename on disk (legal on POSIX) and a TO `glob:`
// pattern that, if value escape were missing, would re-glob the `*` and
// match unrelated siblings.
func TestRun_VcsOutdated_TOReGlobLiteralEmbed(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Source with `*` in the basename (= pathological per spec §9, but
	// legal POSIX). FROM uses [*] to match the literal `*` so the capture
	// "a*b" enters the TO substitute.
	mk("src/a*b.ts", "src")
	mk("derived/a*b.js", "ok")     // the legitimate derived
	mk("derived/aXb.js", "wrong")  // would be matched if `*` were re-globbed
	mk("derived/aYb.js", "wrong2") // ditto
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:src/*.ts", "glob:derived/$1.js"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain err: %v (stderr=%s)", err, stderr.String())
		}
		out := stdout.String()
		// The valid derived is derived/a*b.js. derived/aXb.js / aYb.js
		// would appear ONLY if the `*` in the capture got re-globbed by
		// the 2nd-stage walk — i.e. the escape failed.
		if strings.Contains(out, "aXb.js") || strings.Contains(out, "aYb.js") {
			t.Errorf("TO-glob re-globbed captured `*` (blocker #4 regressed):\n%s", out)
		}
		if !strings.Contains(out, "a*b.js") {
			t.Errorf("expected derived/a*b.js row (= literal-embedded capture):\n%s", out)
		}
	})
}

// TestRun_VcsOutdated_VCSErrorPropagates: blocker #2 pin. A file-timestamp
// subprocess error must propagate through fmt.Errorf("... %w", ...) and
// land as exitCodeVCSExec at the top level (= 3), not flatten to exit 2.
func TestRun_VcsOutdated_VCSErrorPropagates(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	// NOT a git repo: any backend probe should fail with exitCodeVCSExec.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README-ja.md"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--vcs", "git",
			"README.md", "README-ja.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected vcs error, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok {
			t.Fatalf("expected *exitErr, got %T: %v", err, err)
		}
		if ee.code != exitCodeVCSExec {
			t.Errorf("exit code = %d, want %d (VCS exec)", ee.code, exitCodeVCSExec)
		}
	})
}

// TestRun_VcsOutdated_FromBraceCapture: spec §4.1 pin — a FROM `{}`
// consumes one `$N` slot. End-to-end via the cmd: brace alt becomes a
// captured literal we can reference in TO.
func TestRun_VcsOutdated_FromBraceCapture(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("README-ja.md", "ja")
	mk("README-en.md", "en")
	mk("README-ja.txt", "")
	mk("README-en.txt", "")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:README-{ja,en}.md", "README-$1.txt"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain err: %v (stderr=%s)", err, stderr.String())
		}
		out := stdout.String()
		// $1 must be the brace literal ("ja" or "en"), so we expect
		// `README-ja.md → README-ja.txt` and `README-en.md → README-en.txt`.
		if !strings.Contains(out, "README-ja.md") || !strings.Contains(out, "README-ja.txt") {
			t.Errorf("expected FROM-brace $1 to bind to alt literal: %s", out)
		}
		if !strings.Contains(out, "README-en.md") || !strings.Contains(out, "README-en.txt") {
			t.Errorf("expected FROM-brace $1 to bind for `en` alt: %s", out)
		}
	})
}

// TestRun_VcsOutdated_RejectQuestionMarkInFrom pins Tier-1 NC-1: a `?`
// anywhere in the FROM pattern must produce a graceful PatternSyntaxError
// (exit 2 = usage), NOT a grammar-drift panic. The original integration
// commit's docs advertised `?` as a feature, but the parser silently
// fell through to a literal branch while doublestar matched it as a
// wildcard — any successful match panicked spec §7's internal-bug path.
func TestRun_VcsOutdated_RejectQuestionMarkInFrom(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "axb.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("`?` in FROM must NOT panic (spec §7 grammar-drift is internal-bug only); got panic: %v", r)
			}
		}()
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "glob:a?b.md", "out/$1.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected usage err for `?` in FROM, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage (2), got %v", err)
		}
		if !strings.Contains(stderr.String(), "MVP scope") {
			t.Errorf("stderr should explain MVP scope, got: %s", stderr.String())
		}
	})
}

// TestRun_VcsOutdated_RejectQuestionMarkInTo pins Tier-1 NC-1 (TO side).
func TestRun_VcsOutdated_RejectQuestionMarkInTo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "out?.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected usage err for `?` in TO, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage (2), got %v", err)
		}
	})
}

// TestRun_VcsOutdated_TOReGlobLiteralEmbed_NonGlobTO pins Tier-2 NC-2:
// when the TO is non-`glob:` (= literal template) but a captured value
// contains glob meta, spec §3.4 mandates the value is embedded as a
// LITERAL — the implementation must not branch into an fs glob walk
// because the post-substitute string happens to contain `*`. The
// existence check is performed against the literal path; pathological
// filenames are user responsibility per spec §9.
func TestRun_VcsOutdated_TOReGlobLiteralEmbed_NonGlobTO(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "Test")
	runIn(t, dir, "git", "config", "user.email", "test@example.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// FROM captures `a*b` via `[*]`. TO is NON-glob (no `glob:` prefix).
	// derived/aXb.md / aYb.md must NOT match — only the literal a*b.md
	// would (spec §9 makes that pathological filename user-responsibility).
	mk("src/a*b.ts", "src")
	mk("derived/aXb.md", "wrong")
	mk("derived/aYb.md", "wrong2")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "initial")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:src/*.ts", "derived/$1.md"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("--explain err: %v (stderr=%s)", err, stderr.String())
		}
		out := stdout.String()
		// derived/aXb.md / aYb.md must NOT appear (spec §3.4 literal embed).
		if strings.Contains(out, "aXb.md") || strings.Contains(out, "aYb.md") {
			t.Errorf("non-glob TO re-globbed captured `*` (spec §3.4 violation):\n%s", out)
		}
		// The literal embed candidate is `derived/a*b.md`; since it doesn't
		// exist on disk, the row's status is "missing".
		if !strings.Contains(out, "derived/a*b.md") {
			t.Errorf("expected literal-embedded derived path `derived/a*b.md`, got:\n%s", out)
		}
	})
}

// TestRun_VcsOutdated_NoArgsHelp: bare `vcs outdated` → help routed.
func TestRun_VcsOutdated_NoArgsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"vcs", "outdated"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("bare verb should route to help, got err: %v", err)
	}
	if !strings.Contains(stdout.String(), "vcs outdated") {
		t.Errorf("expected help text on stdout, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Coverage-matrix cells K1..K26. One-line per-test comments map to cells in
// docs/testing/vcs-outdated-coverage.md §2.2. Open questions and rationale
// live in the matrix doc — these tests pin observed v0.31.0 behavior.
// ---------------------------------------------------------------------------

// K1 (OQ-19): `--strict --explain` → exit 0 even with stale rows.
func TestRun_VcsOutdated_StrictPlusExplain_ExitZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--strict", "--explain",
			"README.md", "README-{ja,en}.md"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("expected exit 0, got %v (stderr=%s)", err, se.String())
		}
		if !strings.Contains(so.String(), "stale") {
			t.Errorf("expected stale rows printed on stdout, got: %s", so.String())
		}
	})
}

// K2 (OQ-19): `--strict --explain` + literal-FROM miss → exit 0.
func TestRun_VcsOutdated_StrictPlusExplain_LitMissExitZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--strict", "--explain",
			"MISSING.md", "out.md"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("expected exit 0 (explain wins over strict), got %v", err)
		}
	})
}

// K3 (OQ-20): `--explain` + mandatory missing → exit 0 but stdout says "will fail".
func TestRun_VcsOutdated_ExplainPlusMissing_TextContradictsExit(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("en"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"README.md", "README-ja.md"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("expected exit 0, got %v", err)
		}
		if !strings.Contains(so.String(), "missing, will fail") {
			t.Errorf("expected `missing, will fail` text on stdout, got: %s", so.String())
		}
	})
}

// K4 (OQ-21): `--strict` + lit-miss in any pair silences other pairs' stale rows.
func TestRun_VcsOutdated_StrictShortCircuitsStaleRow(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--strict", "--",
			"README.md", "README-ja.md", "--",
			"missing.md", "out.md"}, bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Fatalf("expected exitCodeFalse, got %v", err)
		}
		// Characterization: pair 1's stale row is silenced by short-circuit.
		if strings.Contains(se.String(), "stale") {
			t.Errorf("unexpected: stale row reached stderr despite --strict short-circuit: %s", se.String())
		}
		if !strings.Contains(se.String(), "matched no file") {
			t.Errorf("expected lit-miss warning, got: %s", se.String())
		}
	})
}

// K5: FROM `glob:` empty body → exit 2.
func TestRun_VcsOutdated_GlobEmptyBodyRejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "glob:", "out.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage, got %v", err)
		}
	})
}

// K6: TO `glob:` empty body → exit 2.
func TestRun_VcsOutdated_ToGlobEmptyBodyRejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "glob:"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage, got %v", err)
		}
	})
}

// K7 (OQ-22, openConcern): empty TO `""` quietly succeeds as "fresh" against cwd dir.
// Characterization: pins the silent-green gap. NOT a contract — see OQ-22.
func TestRun_VcsOutdated_EmptyToCharacterization(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain", "README.md", ""},
			bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("characterization: expected exit 0, got %v", err)
		}
		// path.Clean("") = "." → derived path is `.` (cwd). Output should
		// mention `.` as derived.
		if !strings.Contains(so.String(), "→  .") {
			t.Errorf("expected derived `.` in --explain output, got: %s", so.String())
		}
	})
}

// K8: trailing `--` (= empty group ignored).
func TestRun_VcsOutdated_TrailingPairSeparator(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "README-ja.md", "--"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse (stale), got %v", err)
		}
	})
}

// K9: leading `--` (= same as no leading sep).
func TestRun_VcsOutdated_LeadingPairSeparator(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--", "README.md", "README-ja.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
	})
}

// K10: only `--` → usage exit 2.
func TestRun_VcsOutdated_OnlyPairSeparator(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage, got %v", err)
		}
	})
}

// K11: multi-pair, same FROM, two TOs — both pairs' rows are emitted independently.
func TestRun_VcsOutdated_MultiPairSameFrom(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--",
			"README.md", "README-ja.md", "--",
			"README.md", "README-en.md"}, bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
		stderrS := se.String()
		if !strings.Contains(stderrS, "README-ja.md") {
			t.Errorf("pair 1 row missing: %s", stderrS)
		}
		if !strings.Contains(stderrS, "README-en.md") {
			t.Errorf("pair 2 row missing: %s", stderrS)
		}
	})
}

// K12 (OQ-23): pair 2 syntax error short-circuits and discards pair 1's results.
func TestRun_VcsOutdated_Pair2ErrorShortCircuits(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--",
			"README.md", "README-ja.md", "--",
			"glob:a?b.md", "out.md"}, bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Fatalf("expected exitCodeUsage (pair 2 syntax err wins), got %v", err)
		}
		// Characterization: pair 1's stale row is NOT emitted.
		if strings.Contains(se.String(), "stale") {
			t.Errorf("pair 1 stale row leaked despite short-circuit: %s", se.String())
		}
		if !strings.Contains(se.String(), "MVP scope") {
			t.Errorf("expected MVP scope explanation, got: %s", se.String())
		}
	})
}

// K13: untracked derived (= on disk, not in VCS) → exit 1 non-explain.
func TestRun_VcsOutdated_UntrackedDerivedExit1(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	// Create derived on disk WITHOUT committing.
	if err := os.WriteFile(filepath.Join(dir, "README-untracked.md"), []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "README-untracked.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Fatalf("expected exitCodeFalse, got %v", err)
		}
		if !strings.Contains(se.String(), "untracked") {
			t.Errorf("expected `untracked` in stderr, got: %s", se.String())
		}
	})
}

// K14: untracked derived under `--explain` → exit 0 with `[untracked: ...]` text.
func TestRun_VcsOutdated_UntrackedDerivedExplain(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README-untracked.md"), []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"README.md", "README-untracked.md"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("expected exit 0, got %v", err)
		}
		if !strings.Contains(so.String(), "untracked") {
			t.Errorf("expected untracked text on stdout, got: %s", so.String())
		}
	})
}

// K15 (OQ-24): cross-source case — source A's derived path equals source B's path.
// Characterization: NOT excluded; A→B row is emitted (fresh if B's ts >= A's).
func TestRun_VcsOutdated_CrossSourceNotExcluded(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("en"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README-ja.md"), []byte("ja"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	sleepOneSecond(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("en2"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "bump")
	withCwd(t, dir, func() {
		// FROM matches README.md + README-ja.md. TO = README{,-ja}.md.
		// Per-source auto-exclude removes README.md→README.md and
		// README-ja.md→README-ja.md. The CROSS rows (README-ja.md→README.md
		// and README.md→README-ja.md) are kept.
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:README{,-ja}.md", "README{,-ja}.md"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Fatalf("--explain err: %v", err)
		}
		out := so.String()
		// Both cross rows should appear (= not auto-excluded across sources).
		if !strings.Contains(out, "README-ja.md  →  README.md") {
			t.Errorf("cross row README-ja.md→README.md missing: %s", out)
		}
		if !strings.Contains(out, "README.md     →  README-ja.md") {
			t.Errorf("cross row README.md→README-ja.md missing: %s", out)
		}
	})
}

// K16: jj backend happy-path — stale derived → exit 1.
func TestRun_VcsOutdated_JjBackendStale(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj required")
	}
	dir := setupOutdatedGitRepo(t)
	// Colocate jj on top.
	runIn(t, dir, "jj", "git", "init", "--git-repo", ".git")
	if err := writeFile(filepath.Join(dir, ".jj/repo/config.toml"),
		"[signing]\nbehavior = \"drop\"\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--vcs", "jj",
			"README.md", "README-ja.md"}, bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Fatalf("expected exitCodeFalse from jj backend, got %v (stderr=%s)", err, se.String())
		}
		if !strings.Contains(se.String(), "stale") {
			t.Errorf("expected stale row from jj backend, got: %s", se.String())
		}
	})
}

// K17: `--vcs jj` in non-jj dir → exit 3.
func TestRun_VcsOutdated_WrongVcsJjExit3(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README-ja.md"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--vcs", "jj",
			"README.md", "README-ja.md"}, bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok {
			t.Fatalf("expected *exitErr, got %T: %v", err, err)
		}
		if ee.code != exitCodeVCSExec {
			t.Errorf("expected exitCodeVCSExec (3), got %d", ee.code)
		}
	})
}

// K18: glob FROM 0-match (NOT literal) → exit 0 even with `--strict`.
func TestRun_VcsOutdated_StrictGlobZeroMatchExit0(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--strict",
			"glob:nothing-*.zzz", "lib/$1.out"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("glob 0-match must not fail --strict, got %v", err)
		}
	})
}

// K19: `$0` in TO yields source's own path → per-source auto-exclude triggers,
// so no rows are emitted (and exit is 0 since there's nothing to fail on).
func TestRun_VcsOutdated_Dollar0InToExcluded(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:src/**/*.ts", "$0"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Fatalf("--explain err: %v", err)
		}
		// All derived rows would equal source → all auto-excluded → empty stdout.
		if strings.TrimSpace(so.String()) != "" {
			t.Errorf("expected empty stdout (all auto-excluded), got: %q", so.String())
		}
	})
}

// K20: `--glob-dotfile=true` reaches sources under dot directories.
func TestRun_VcsOutdated_GlobDotfileFlag(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(".hidden/x.ts", "a")
	mk("lib/x.js", "b")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		// Default Dotfile=false → no source matched.
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:**/*.ts", "lib/$2.js"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(so.String(), ".hidden") {
			t.Errorf("default should exclude .hidden, got: %s", so.String())
		}
		// With --glob-dotfile=true the hidden source is exposed.
		var so2, se2 bytes.Buffer
		err = run([]string{"vcs", "outdated", "--explain",
			"--glob-dotfile=true",
			"glob:**/*.ts", "lib/$2.js"}, bytes.NewReader(nil), &so2, &se2)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(so2.String(), ".hidden") {
			t.Errorf("--glob-dotfile=true should include .hidden source, got: %s", so2.String())
		}
	})
}

// K21 (openConcern: v0.31.0 BUG): `--glob-ignorecase` causes grammar-drift panic
// when the matched path's case differs from the pattern's. doublestar walks
// case-insensitively but the capture regex in buildRawAndRegex is compiled
// case-sensitively, so any case-different match triggers the §3.3 panic.
// Characterization: pins the panic so a future fix flips this test green
// once the capture regex inherits the IgnoreCase flag.
func TestRun_VcsOutdated_GlobIgnorecaseFlag(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("DOCS/README.MD", "x")
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("v0.31.0: --glob-ignorecase + case-different path expected to panic " +
					"(capture regex doesn't inherit IgnoreCase). If this test starts failing, " +
					"verify the fix and flip the assertion to check stdout for `DOCS/README.MD`.")
			}
			s, ok := r.(string)
			if !ok || !strings.Contains(s, "grammar drift") {
				t.Errorf("expected grammar-drift panic, got: %v", r)
			}
		}()
		var so, se bytes.Buffer
		_ = run([]string{"vcs", "outdated", "--explain", "--glob-ignorecase",
			"glob:**/*.md", "out.txt"}, bytes.NewReader(nil), &so, &se)
	})
}

// K22: `--explain` with no FROM matches → exit 0 with empty stdout.
func TestRun_VcsOutdated_ExplainNoMatches(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "--explain",
			"glob:nothing-here-*.ts", "lib/$1.js"}, bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Errorf("expected exit 0, got %v", err)
		}
		if strings.TrimSpace(so.String()) != "" {
			t.Errorf("expected empty stdout, got: %q", so.String())
		}
	})
}

// K23: `$N` out-of-range in TO → empty literal contributes to path.Clean.
func TestRun_VcsOutdated_OutOfRangeBackrefCleaned(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "src.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		// `$5` is out of range (no `$5` slot exists). Should substitute "".
		err := run([]string{"vcs", "outdated", "--explain", "src.ts", "out/${5}/x.js"},
			bytes.NewReader(nil), &so, &se)
		if err != nil {
			t.Fatal(err)
		}
		// path.Clean("out//x.js") → "out/x.js"
		if !strings.Contains(so.String(), "out/x.js") {
			t.Errorf("expected derived `out/x.js`, got: %s", so.String())
		}
	})
}

// K24: bare `$10` in TO (ambiguous) → exit 2.
func TestRun_VcsOutdated_AmbiguousDollar10InTo(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "out-$10.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage for ambiguous $10, got %v", err)
		}
	})
}

// K25: `${abc}` named in TO → exit 2.
func TestRun_VcsOutdated_NamedRefInTo(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := setupOutdatedGitRepo(t)
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "README.md", "out-${abc}.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("expected exitCodeUsage for ${abc}, got %v", err)
		}
	})
}

// K26: mandatory `{,}` brace TO with one alt missing on disk → exit 1.
// Companion to C24 — same semantic but exercises the EMPTY alt branch
// explicitly (alts = "" and "-ja"; only "-ja" exists).
func TestRun_VcsOutdated_EmptyAltBraceTOMissingExit1(t *testing.T) {
	if !gitAvailable() {
		t.Skip()
	}
	dir := t.TempDir()
	runIn(t, dir, "git", "init", "-q", "-b", "main")
	runIn(t, dir, "git", "config", "user.name", "T")
	runIn(t, dir, "git", "config", "user.email", "t@e.com")
	runIn(t, dir, "git", "config", "commit.gpgsign", "false")
	// Source committed; `out.md` (= empty alt) is absent — it's mandatory.
	if err := os.WriteFile(filepath.Join(dir, "source.md"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out-ja.md"), []byte("j"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", ".")
	runIn(t, dir, "git", "commit", "-qm", "init")
	withCwd(t, dir, func() {
		var so, se bytes.Buffer
		err := run([]string{"vcs", "outdated", "source.md", "out{,-ja}.md"},
			bytes.NewReader(nil), &so, &se)
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeFalse {
			t.Errorf("expected exitCodeFalse, got %v", err)
		}
		if !strings.Contains(se.String(), "missing") {
			t.Errorf("expected `missing` for empty-alt absent file, got: %s", se.String())
		}
	})
}

// TestSplitOutdatedPairs_Cases verifies the pair splitter directly.
func TestSplitOutdatedPairs_Cases(t *testing.T) {
	cases := []struct {
		name    string
		argv    []string
		want    []outdatedPair
		wantErr bool
	}{
		{
			name: "single pair no separator",
			argv: []string{"README.md", "README-ja.md"},
			want: []outdatedPair{{From: "README.md", To: []string{"README-ja.md"}}},
		},
		{
			name: "single pair multiple TO",
			argv: []string{"README.md", "README-{ja,en}.md"},
			want: []outdatedPair{{From: "README.md", To: []string{"README-{ja,en}.md"}}},
		},
		{
			name: "two pairs with leading --",
			argv: []string{"--", "F1", "T1", "--", "F2", "T2"},
			want: []outdatedPair{
				{From: "F1", To: []string{"T1"}},
				{From: "F2", To: []string{"T2"}},
			},
		},
		{
			name: "two pairs without leading --",
			argv: []string{"F1", "T1", "--", "F2", "T2"},
			want: []outdatedPair{
				{From: "F1", To: []string{"T1"}},
				{From: "F2", To: []string{"T2"}},
			},
		},
		{
			name:    "single arg fails",
			argv:    []string{"only-one"},
			wantErr: true,
		},
		{
			name: "empty argv → empty pairs (caller handles usage)",
			argv: []string{},
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := splitOutdatedPairs(c.argv)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected err, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d (got=%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i].From != c.want[i].From {
					t.Errorf("pair %d: From = %q, want %q", i, got[i].From, c.want[i].From)
				}
				if strings.Join(got[i].To, ",") != strings.Join(c.want[i].To, ",") {
					t.Errorf("pair %d: To = %v, want %v", i, got[i].To, c.want[i].To)
				}
			}
		})
	}
}
