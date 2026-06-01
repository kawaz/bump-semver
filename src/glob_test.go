package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// fakeHomeFn returns a closure that always reports the given path as the
// user's home directory. Used to test `~` expansion without HOME mutation.
func fakeHomeFn(home string) func() (string, error) {
	return func() (string, error) { return home, nil }
}

// TestParseGlobSpec covers the `glob:<pattern>` prefix splitter.
func TestParseGlobSpec(t *testing.T) {
	cases := []struct {
		spec   string
		want   string
		errSub string // substring expected in err, "" = no err
	}{
		{"glob:src/**/*.ts", "src/**/*.ts", ""},
		{"glob:VERSION", "VERSION", ""},
		{"glob:", "", "empty"},
		{"vcs:HEAD", "", "not a glob"},
	}
	for _, c := range cases {
		got, err := parseGlobSpec(c.spec)
		if c.errSub != "" {
			if err == nil || !strings.Contains(err.Error(), c.errSub) {
				t.Errorf("parseGlobSpec(%q): expected err containing %q, got %v", c.spec, c.errSub, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseGlobSpec(%q): unexpected err: %v", c.spec, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseGlobSpec(%q) = %q, want %q", c.spec, got, c.want)
		}
	}
}

// TestExpandTilde covers the `~` / `~/...` expansion (and `~user/...`
// pass-through, which we don't support in MVP).
func TestExpandTilde(t *testing.T) {
	home := "/fake/home"
	hfn := fakeHomeFn(home)
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"~user/foo", "~user/foo"}, // pass-through (not supported)
		{"plain/path", "plain/path"},
		{"/abs/path", "/abs/path"},
	}
	for _, c := range cases {
		got, err := expandTilde(c.in, hfn)
		if err != nil {
			t.Errorf("expandTilde(%q): err: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// globFixture sets up a directory tree with files for the matrix tests
// and returns its absolute path. Layout:
//
//	src/a.ts          src/b.tsx          src/c.go
//	src/sub/d.ts      src/sub/e.tsx
//	.hidden/x.ts      docs/README.md
//	.gitignore        (ignores ignored.txt and gen/)
//	ignored.txt       gen/g.ts
func globFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("src/a.ts", "a")
	mk("src/b.tsx", "b")
	mk("src/c.go", "c")
	mk("src/sub/d.ts", "d")
	mk("src/sub/e.tsx", "e")
	mk(".hidden/x.ts", "x")
	mk("docs/README.md", "doc")
	mk(".gitignore", "ignored.txt\ngen/\n")
	mk("ignored.txt", "i")
	mk("gen/g.ts", "g")
	return dir
}

// TestExpandGlob_Patterns walks the supported pattern matrix from
// DR-0024: `*` / `**` / `[]` / `{}` against the fixture above. Default
// flags (Dotfile=false, Gitignored=true, IgnoreCase=false).
func TestExpandGlob_Patterns(t *testing.T) {
	dir := globFixture(t)
	withCwd(t, dir, func() {
		cases := []struct {
			pat  string
			want []string
		}{
			{"src/*.ts", []string{"src/a.ts"}},                    // single-segment *
			{"src/**/*.ts", []string{"src/a.ts", "src/sub/d.ts"}}, // ** recursive
			{"src/**/*.{ts,tsx}", []string{"src/a.ts", "src/b.tsx", "src/sub/d.ts", "src/sub/e.tsx"}},
			{"src/[ab].*", []string{"src/a.ts", "src/b.tsx"}}, // [] char class
			{"missing/**/*.ts", []string{}},                   // no-match → empty (silent)
			{"VERSION-nope", []string{}},                      // literal no-match
		}
		for _, c := range cases {
			got, err := expandGlob(c.pat, globOpts{}, defaultHomeFn)
			if err != nil {
				t.Errorf("expandGlob(%q): err: %v", c.pat, err)
				continue
			}
			sort.Strings(got)
			want := append([]string(nil), c.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("expandGlob(%q) = %v, want %v", c.pat, got, want)
			}
		}
	})
}

// TestExpandGlob_Dotfile: default excludes dotfiles; --glob-dotfile=true
// includes them.
func TestExpandGlob_Dotfile(t *testing.T) {
	dir := globFixture(t)
	withCwd(t, dir, func() {
		// Default: no .hidden/x.ts.
		got, err := expandGlob("**/*.ts", globOpts{}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range got {
			if strings.HasPrefix(g, ".hidden") {
				t.Errorf("default dotfile=false leaked %q", g)
			}
		}
		// Dotfile=true: now .hidden/x.ts shows up.
		got2, err := expandGlob("**/*.ts", globOpts{Dotfile: true}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		var sawHidden bool
		for _, g := range got2 {
			if g == filepath.Join(".hidden", "x.ts") {
				sawHidden = true
			}
		}
		if !sawHidden {
			t.Errorf("dotfile=true did NOT include .hidden/x.ts: %v", got2)
		}
	})
}

// TestExpandGlob_Gitignored: default respects .gitignore; gitignored=false
// includes paths the .gitignore would have filtered.
func TestExpandGlob_Gitignored(t *testing.T) {
	dir := globFixture(t)
	withCwd(t, dir, func() {
		// Default (Gitignored=nil → true): ignored.txt suppressed.
		got, err := expandGlob("*.txt", globOpts{}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range got {
			if g == "ignored.txt" {
				t.Errorf("default gitignored=true leaked %q", g)
			}
		}
		// Explicit gitignored=false: ignored.txt is included.
		off := false
		got2, err := expandGlob("*.txt", globOpts{Gitignored: &off}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		var saw bool
		for _, g := range got2 {
			if g == "ignored.txt" {
				saw = true
			}
		}
		if !saw {
			t.Errorf("gitignored=false did NOT include ignored.txt: %v", got2)
		}
	})
}

// TestExpandGlob_IgnoreCase verifies WithCaseInsensitive plumbing.
func TestExpandGlob_IgnoreCase(t *testing.T) {
	dir := globFixture(t)
	withCwd(t, dir, func() {
		// Case-sensitive: no match.
		got, err := expandGlob("SRC/A.TS", globOpts{}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		// On case-sensitive filesystems this is empty; on macOS APFS it may
		// match by FS coincidence. Either way ignorecase=true must not error.
		_ = got
		got2, err := expandGlob("**/A.TS", globOpts{IgnoreCase: true}, defaultHomeFn)
		if err != nil {
			t.Fatal(err)
		}
		var saw bool
		for _, g := range got2 {
			if strings.EqualFold(filepath.Base(g), "a.ts") {
				saw = true
			}
		}
		if !saw {
			t.Errorf("ignorecase=true should have matched a.ts: %v", got2)
		}
	})
}

// TestExpandGlob_TildeRoundtrip: `~/foo` expands via homeFn.
func TestExpandGlob_TildeRoundtrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hfn := fakeHomeFn(dir)
	got, err := expandGlob("~/VERSION", globOpts{}, hfn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasSuffix(got[0], "VERSION") {
		t.Errorf("expected one ~-expanded match for VERSION, got %v", got)
	}
}

// TestRun_Get_GlobPrefix: end-to-end via `get glob:src/**/*` against a
// fixture where every file holds the same version → exit 0.
func TestRun_Get_GlobPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/a.json", "src/b.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"version":"1.2.3"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		err := run([]string{"get", "glob:src/*.json"}, bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("get glob:src/*.json: %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "1.2.3") {
			t.Errorf("expected 1.2.3 on stdout, got: %q", stdout.String())
		}
	})
}

// TestRun_Get_GlobPrefix_NoMatch: 0-match glob: + no other inputs →
// the "at least one input is required" gate fires with exit 2 (we
// silently skipped the glob: selector; the rest is plain dispatcher
// behavior).
func TestRun_Get_GlobPrefix_NoMatch(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		err := run([]string{"get", "glob:nonexistent/**/*.json"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		// Parser sees 1 input; resolveInputs collapses to 0; downstream may
		// produce an exit-2 or other error. We just want NOT-nil here so we
		// don't accidentally pass an empty inputs list to the bump backend.
		if err == nil {
			t.Errorf("expected an error for 0-match glob with no other inputs, got nil")
		}
	})
}

// TestRun_VcsDiff_GlobExpansion: `vcs diff REV -- glob:src/*.ts` short-
// circuits when expansion is empty (does NOT widen to "diff everything").
func TestRun_VcsDiff_GlobExpansion(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		// glob: that matches NO files → expansion empty → exit 0, no stdout.
		// Critical anti-regression: must NOT call `git diff REV` (no
		// pathspec), which would widen back to the full diff.
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "--", "glob:nonexistent/**/*.ts"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff glob: %v", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("expected empty stdout for 0-match glob, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_GlobMatch: `vcs diff -- glob:VERSION` matches the
// VERSION file and shows the diff.
func TestRun_VcsDiff_GlobMatch(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "--", "glob:VERSION"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff glob:VERSION: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected VERSION in diff output, got: %q", stdout.String())
		}
	})
}

// TestParseArgs_GlobFlags covers the parser-side acceptance / rejection of
// --glob-* flags.
func TestParseArgs_GlobFlags(t *testing.T) {
	cases := []struct {
		argv []string
		err  string // substring; "" = expect no err
		// post-parse predicate (only run if no err)
		check func(t *testing.T, a cliArgs)
	}{
		{
			argv: []string{"get", "--glob-dotfile=true", "glob:**/*.ts"},
			check: func(t *testing.T, a cliArgs) {
				if !a.glob.Dotfile {
					t.Errorf("expected glob.Dotfile=true")
				}
			},
		},
		{
			argv: []string{"get", "--glob-dotfile", "glob:**/*.ts"},
			err:  "requires =true or =false",
		},
		{
			argv: []string{"get", "--glob-dotfile=maybe", "glob:**/*.ts"},
			err:  "requires true or false",
		},
		{
			argv: []string{"get", "--glob-gitignored=false", "glob:**/*.ts"},
			check: func(t *testing.T, a cliArgs) {
				if a.glob.Gitignored == nil || *a.glob.Gitignored {
					t.Errorf("expected glob.Gitignored=&false")
				}
			},
		},
		{
			argv: []string{"get", "--glob-ignorecase", "glob:**/*.ts"},
			check: func(t *testing.T, a cliArgs) {
				if !a.glob.IgnoreCase {
					t.Errorf("expected glob.IgnoreCase=true (bare flag)")
				}
			},
		},
		{
			argv: []string{"get", "--glob-ignorecase=false", "glob:**/*.ts"},
			check: func(t *testing.T, a cliArgs) {
				if a.glob.IgnoreCase {
					t.Errorf("expected glob.IgnoreCase=false")
				}
			},
		},
		// vcs verb-aware: diff/commit accept, get rejects (unknown flag).
		{
			argv: []string{"vcs", "get", "root", "--glob-dotfile=true"},
			err:  "unknown flag for 'vcs get'",
		},
		{
			argv: []string{"vcs", "diff", "HEAD", "--glob-dotfile=true", "--", "glob:**/*"},
			check: func(t *testing.T, a cliArgs) {
				if !a.glob.Dotfile {
					t.Errorf("expected glob.Dotfile=true on vcs diff")
				}
			},
		},
	}
	for _, c := range cases {
		got, err := parseArgs(c.argv)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("parseArgs(%v): expected err containing %q, got %v", c.argv, c.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseArgs(%v): unexpected err: %v", c.argv, err)
			continue
		}
		if c.check != nil {
			c.check(t, got)
		}
	}
}
