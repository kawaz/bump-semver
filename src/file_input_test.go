package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `file:LIST` reads a file and expands each line as a path (literal or
// glob:). Blank lines and `#` comments are skipped (DR-0033).
func TestExpandGlobInputs_FileLiteral(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	listPath := filepath.Join(dir, "list.txt")
	body := strings.Join([]string{
		"# comment line",
		"",
		"a.txt",
		"  b.txt  ", // trimmed
		"# another comment",
		"c.txt",
	}, "\n")
	if err := os.WriteFile(listPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := expandGlobInputs([]string{"file:" + listPath}, globOpts{})
	if err != nil {
		t.Fatalf("expandGlobInputs file:literal: %v", err)
	}
	want := []string{"a.txt", "b.txt", "c.txt"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// `file:LIST` lines may use `glob:` prefix, expanded via the shared layer.
func TestExpandGlobInputs_FileWithGlob(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// create fixture files
	for _, name := range []string{"alpha.go", "beta.go", "gamma.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	listPath := filepath.Join(dir, "list.txt")
	body := strings.Join([]string{
		"glob:" + filepath.Join(dir, "*.go"),
		"# explicit literal",
		filepath.Join(dir, "gamma.md"),
	}, "\n")
	if err := os.WriteFile(listPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := expandGlobInputs([]string{"file:" + listPath}, globOpts{})
	if err != nil {
		t.Fatalf("expandGlobInputs file:with-glob: %v", err)
	}
	// got contains alpha.go, beta.go (from glob) + gamma.md (literal).
	wantSubstr := []string{"alpha.go", "beta.go", "gamma.md"}
	for _, w := range wantSubstr {
		found := false
		for _, g := range got {
			if strings.HasSuffix(g, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in expanded output %v", w, got)
		}
	}
}

// Nested `file:` is rejected (= MVP scope-out per DR-0033).
func TestExpandGlobInputs_FileNestedRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	listPath := filepath.Join(dir, "list.txt")
	body := "file:other-list.txt\n"
	if err := os.WriteFile(listPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := expandGlobInputs([]string{"file:" + listPath}, globOpts{})
	if err == nil {
		t.Fatal("expected nested file: to error")
	}
	if !strings.Contains(err.Error(), "nested file:") {
		t.Errorf("error should mention nested file:, got: %v", err)
	}
}

// `file:` with nonexistent path → error.
func TestExpandGlobInputs_FileMissing(t *testing.T) {
	t.Parallel()
	_, err := expandGlobInputs([]string{"file:/this/does/not/exist.txt"}, globOpts{})
	if err == nil {
		t.Fatal("expected missing file: to error")
	}
}

// `file:` with empty path → usage error.
func TestExpandGlobInputs_FileEmptyPath(t *testing.T) {
	t.Parallel()
	_, err := expandGlobInputs([]string{"file:"}, globOpts{})
	if err == nil {
		t.Fatal("expected empty file: path to error")
	}
	if !strings.Contains(err.Error(), "file: path is empty") {
		t.Errorf("error should mention empty path, got: %v", err)
	}
}

// `vcs diff -q REV path --excludes glob:**/*_test.go` post-filters test
// files: when only `*_test.go` changed, exit 0 (no diff after filtering).
func TestRun_VcsDiff_ExcludesPostFilter(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// Bump VERSION AND add a test file in a single commit so the diff
		// from HEAD~1 → working tree shows both changes.
		testPath := filepath.Join(dir, "foo_test.go")
		if err := os.WriteFile(testPath, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.1.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "foo_test.go", "VERSION")
		runIn(t, dir, "git", "commit", "-qm", "bump + add test")
		// Now HEAD..HEAD~1 includes both VERSION change and foo_test.go.
		// With --excludes glob:**/*_test.go, only VERSION should remain.
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "VERSION", "foo_test.go",
			"--excludes", "glob:**/*_test.go"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff with --excludes: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected VERSION in diff after excludes, got: %q", stdout.String())
		}
		if strings.Contains(stdout.String(), "foo_test.go") {
			t.Errorf("test file should be excluded, got: %q", stdout.String())
		}
	})
}

// `--excludes` repeatable + append (DR-0033 原則 1)。
func TestRun_VcsDiff_ExcludesRepeatable(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// Bump VERSION + add two extra files in a single commit.
		for _, name := range []string{"a.gen.go", "b_test.go"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.1.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "a.gen.go", "b_test.go", "VERSION")
		runIn(t, dir, "git", "commit", "-qm", "bump + add gen + test")
		// HEAD..HEAD~1 has VERSION + a.gen.go + b_test.go.
		// Exclude both via two --excludes flags.
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1",
			"VERSION", "a.gen.go", "b_test.go",
			"--excludes", "glob:**/*.gen.go",
			"--excludes", "glob:**/*_test.go"},
			bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff with multiple --excludes: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected VERSION in diff, got: %q", stdout.String())
		}
		for _, banned := range []string{"a.gen.go", "b_test.go"} {
			if strings.Contains(stdout.String(), banned) {
				t.Errorf("%q should be excluded, got: %q", banned, stdout.String())
			}
		}
	})
}

// `--excludes` order-independence (DR-0033 原則 1)。Same arguments in
// different positions produce identical output.
func TestRun_VcsDiff_ExcludesOrderIndependent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "x_test.go")
		runIn(t, dir, "git", "commit", "-qm", "add test")

		var stdoutA, stdoutB bytes.Buffer
		// A: --excludes before positional
		errA := run([]string{"vcs", "diff", "HEAD~1",
			"--excludes", "glob:**/*_test.go",
			"VERSION", "x_test.go"},
			bytes.NewReader(nil), &stdoutA, &bytes.Buffer{})
		// B: --excludes after positional
		errB := run([]string{"vcs", "diff", "HEAD~1",
			"VERSION", "x_test.go",
			"--excludes", "glob:**/*_test.go"},
			bytes.NewReader(nil), &stdoutB, &bytes.Buffer{})
		if errA != nil || errB != nil {
			t.Fatalf("expected both invocations to succeed; got A=%v B=%v", errA, errB)
		}
		if stdoutA.String() != stdoutB.String() {
			t.Errorf("order-dependent output:\nA: %q\nB: %q", stdoutA.String(), stdoutB.String())
		}
	})
}

// `--excludes` without positional PATH → usage error (DR-0033 phase 1
// constraint: bare 'diff everything' minus excludes is not supported)。
func TestRun_VcsDiff_ExcludesRequirePositional(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "--excludes", "glob:**/*_test.go"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error when --excludes given without positional PATH")
		}
		if !strings.Contains(stderr.String(), "--excludes requires") {
			t.Errorf("stderr should mention requirement, got: %q", stderr.String())
		}
	})
}

// DR-0042: `--excludes` placed after `--` reaches the positional PATH
// list instead of being parsed as a flag (POSIX `--` semantics: no flag
// parsing beyond it), so it silently becomes an include pattern instead
// of an exclude — the exact reversal the DR's repro demonstrates. This
// pins the observable half of the fix: stdout/exit code are untouched
// (the parse-level reversal is out of DR-0042's scope, see "不採用" —
// only the stderr warning is new), and the warning names the offending
// token.
func TestRun_VcsDiff_DashDash_FlagLikePositional_Warns(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b_test.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "a.txt", "b_test.txt")
		runIn(t, dir, "git", "commit", "-qm", "add a.txt and b_test.txt")

		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "HEAD~1", "--",
			"glob:*.txt", "--excludes", "glob:*_test.txt"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("vcs diff: %v", err)
		}
		// Reversal still happens (DR-0042 doesn't change parsing): b_test.txt
		// is present because "--excludes" and its pattern became includes.
		if !strings.Contains(stdout.String(), "b_test.txt") {
			t.Errorf("expected the known parse reversal to still occur, got: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), `"--excludes"`) {
			t.Errorf("expected stderr warning naming --excludes, got: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "'--'") {
			t.Errorf("expected stderr warning to mention '--', got: %q", stderr.String())
		}
	})
}

// A normal (non-`-`-leading) PATH never triggers the DR-0042 warning.
func TestRun_VcsDiff_NormalPositional_NoWarning(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "VERSION"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("vcs diff: %v", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("stderr should be empty for a normal PATH, got: %q", got)
		}
	})
}

// DR-0042 Consequences: the warning is a "you're about to get the wrong
// answer" signal, not an error — so `-qq` (which suppresses both stdout
// and stderr error output) must NOT suppress it.
func TestRun_VcsDiff_DashDash_FlagLikePositional_WarnsUnderQuietAll(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		// -q's exit code reflects diff presence (design rationale in
		// runVcsCmdDiff); VERSION always differs in this fixture, so exit
		// is predicate-false (exitCodeFalse), not a real failure — only
		// stdout/stderr content matter for this test.
		_ = run([]string{"vcs", "diff", "-qq", "HEAD~1", "--",
			"VERSION", "--excludes", "glob:**/*_test.go"},
			bytes.NewReader(nil), &stdout, &stderr)
		if got := stdout.String(); got != "" {
			t.Errorf("-qq should suppress stdout, got: %q", got)
		}
		if !strings.Contains(stderr.String(), `"--excludes"`) {
			t.Errorf("expected -qq to still surface the DR-0042 warning, got: %q", stderr.String())
		}
	})
}

// Multiple flag-like positionals each get their own warning line.
func TestRun_VcsDiff_DashDash_MultipleFlagLikePositionals_OneLineEach(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "--",
			"VERSION", "--excludes", "glob:**/*_test.go", "-x"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("vcs diff: %v", err)
		}
		lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected exactly 2 warning lines (one per flag-like token), got %d: %q", len(lines), stderr.String())
		}
		if !strings.Contains(lines[0], `"--excludes"`) {
			t.Errorf("expected first line to name --excludes, got: %q", lines[0])
		}
		if !strings.Contains(lines[1], `"-x"`) {
			t.Errorf("expected second line to name -x, got: %q", lines[1])
		}
	})
}

// helper.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
