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

// excludeInputs basic set-subtraction (literal exact match).
func TestExcludeInputs_Literal(t *testing.T) {
	t.Parallel()
	got, err := excludeInputs(
		[]string{"a.go", "a_test.go", "b.go"},
		[]string{"a_test.go"},
		globOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go", "b.go"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// excludeInputs with empty excludes → unchanged.
func TestExcludeInputs_NoExcludes(t *testing.T) {
	t.Parallel()
	got, err := excludeInputs([]string{"a", "b", "c"}, nil, globOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v, want [a b c]", got)
	}
}

// excludeInputs with glob: pattern.
func TestExcludeInputs_GlobPattern(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	withCwd(t, dir, func() {
		for _, name := range []string{"a.go", "a_test.go", "b.go", "b_test.go", "main.go"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		includes, err := expandGlobInputs([]string{"glob:*.go"}, globOpts{})
		if err != nil {
			t.Fatalf("expand includes: %v", err)
		}
		// includes now has all 5 .go files.
		got, err := excludeInputs(includes, []string{"glob:*_test.go"}, globOpts{})
		if err != nil {
			t.Fatalf("excludeInputs: %v", err)
		}
		// Expect a.go, b.go, main.go (test files removed).
		wantSubset := []string{"a.go", "b.go", "main.go"}
		for _, w := range wantSubset {
			found := false
			for _, g := range got {
				if g == w {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in remaining %v", w, got)
			}
		}
		// Negative check: no _test.go entries.
		for _, g := range got {
			if strings.HasSuffix(g, "_test.go") {
				t.Errorf("did not expect %q (test file) in remaining %v", g, got)
			}
		}
	})
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
