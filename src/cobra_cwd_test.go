package main

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runCwdLocked runs argv through run() while holding cwdMu (shared with
// the withCwd helper in vcs_test.go): -C/--cwd makes run() itself call
// os.Chdir, which is process-wide state, so it must be serialised
// against every other test in this package that touches cwd — and the
// original directory must be restored afterward (DR-0043 Consequences).
func runCwdLocked(t *testing.T, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	t.Helper()
	cwdMu.Lock()
	defer cwdMu.Unlock()
	orig, err := getCwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := chdir(orig); err != nil {
			t.Fatalf("chdir back %s: %v", orig, err)
		}
	}()
	return run(argv, stdin, stdout, stderr)
}

// TestExtractCwdOption is a focused unit test on the pre-cobra-parse scan
// (mirrors TestNormalizeQuietAll's style): all five accepted spellings
// (-C V / --cwd V / --cwd=V / -CV / -C=V), the once-only rule, the
// missing/empty-value errors, and the `--` boundary that makes a literal
// -C token positional again (never reinterpreted as a flag).
//
// The attached-shorthand forms (-CV / -C=V) were a 2026-07-11 audit
// finding: pflag itself accepts them for any value-taking shorthand, so
// omitting them here let the token slip past this pre-scan and reach
// the persistent flag's cwdLeakGuardValue — see
// TestRun_CwdOption_LeakGuardFailsClosed for that backstop's own test.
func TestExtractCwdOption(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		in       []string
		wantRest []string
		wantPath string
		wantOK   bool
		wantErr  string // substring; "" = no error expected
	}{
		{
			name:     "short-form-leading",
			in:       []string{"-C", "/tmp/repo", "vcs", "get", "root"},
			wantRest: []string{"vcs", "get", "root"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			name:     "long-form-separate-value",
			in:       []string{"--cwd", "/tmp/repo", "get", "VERSION"},
			wantRest: []string{"get", "VERSION"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			name:     "long-form-inline-value",
			in:       []string{"--cwd=/tmp/repo", "get", "VERSION"},
			wantRest: []string{"get", "VERSION"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			// -C is a global option: it must be removed from wherever it
			// sits in argv, not just a leading position.
			name:     "trailing-position-free",
			in:       []string{"vcs", "get", "root", "-C", "/tmp/repo"},
			wantRest: []string{"vcs", "get", "root"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			name:     "absent",
			in:       []string{"vcs", "get", "root"},
			wantRest: []string{"vcs", "get", "root"},
			wantPath: "",
			wantOK:   false,
		},
		{
			// A `--` separator freezes everything after it as positional;
			// a literal -C token there is left alone (matches
			// normalizeQuietAll's identical guard for -qq).
			name:     "after-separator-untouched",
			in:       []string{"get", "--", "-C", "value"},
			wantRest: []string{"get", "--", "-C", "value"},
			wantPath: "",
			wantOK:   false,
		},
		{
			// pflag's attached-shorthand form: -C/tmp/repo (no space, no
			// '='). This is the exact spelling the audit finding showed
			// slipping through unmatched before this case existed.
			name:     "attached-shorthand",
			in:       []string{"-C/tmp/repo", "vcs", "get", "root"},
			wantRest: []string{"vcs", "get", "root"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			// The git-style explicit-separator variant of the attached
			// form: -C=/tmp/repo. The leading '=' is stripped, not kept
			// as part of the path.
			name:     "attached-shorthand-with-equals",
			in:       []string{"-C=/tmp/repo", "vcs", "get", "root"},
			wantRest: []string{"vcs", "get", "root"},
			wantPath: "/tmp/repo",
			wantOK:   true,
		},
		{
			name:    "missing-value-at-end",
			in:      []string{"vcs", "get", "root", "-C"},
			wantErr: "-C/--cwd requires a value",
		},
		{
			// A bare "-C=" (attached form, nothing after '=') must not
			// silently resolve to an empty path and reach os.Chdir("")
			// (audit finding item 4: that outcome is platform-dependent,
			// not a clear error). It folds into the same "requires a
			// value" wording as a genuinely absent value.
			name:    "empty-value-attached-with-equals",
			in:      []string{"-C=", "vcs", "get", "root"},
			wantErr: "-C/--cwd requires a value",
		},
		{
			// Same guarantee for the long inline form.
			name:    "empty-value-long-inline",
			in:      []string{"--cwd=", "vcs", "get", "root"},
			wantErr: "-C/--cwd requires a value",
		},
		{
			// And for an explicit empty string passed as the separate
			// next token.
			name:    "empty-value-separate-token",
			in:      []string{"-C", "", "vcs", "get", "root"},
			wantErr: "-C/--cwd requires a value",
		},
		{
			name:    "specified-twice-same-spelling",
			in:      []string{"-C", "a", "-C", "b", "get", "VERSION"},
			wantErr: "-C/--cwd specified twice",
		},
		{
			name:    "specified-twice-mixed-spelling",
			in:      []string{"--cwd", "a", "--cwd=b", "get", "VERSION"},
			wantErr: "-C/--cwd specified twice",
		},
		{
			// The once-only check must also catch the attached spelling
			// against an earlier separate-token occurrence, not just
			// identical spellings against each other.
			name:    "specified-twice-attached-vs-separate",
			in:      []string{"-C", "a", "-Cb", "get", "VERSION"},
			wantErr: "-C/--cwd specified twice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rest, path, ok, err := extractCwdOption(c.in)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("extractCwdOption(%v) err = %v, want substring %q", c.in, err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractCwdOption(%v) unexpected err: %v", c.in, err)
			}
			if ok != c.wantOK {
				t.Errorf("extractCwdOption(%v) found = %v, want %v", c.in, ok, c.wantOK)
			}
			if path != c.wantPath {
				t.Errorf("extractCwdOption(%v) path = %q, want %q", c.in, path, c.wantPath)
			}
			if !reflect.DeepEqual(rest, c.wantRest) {
				t.Errorf("extractCwdOption(%v) rest = %v, want %v", c.in, rest, c.wantRest)
			}
		})
	}
}

// TestRun_CwdOption_VcsGetRoot_Git: `-C <repo> vcs get root` reports the
// repo root without the caller having chdir'd there first (DR-0043's
// core promise — the sub-shell `(cd <path> && bump-semver ...)` pattern
// collapses into a single invocation).
func TestRun_CwdOption_VcsGetRoot_Git(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")

	var stdout, stderr bytes.Buffer
	err := runCwdLocked(t, []string{"-C", dir, "vcs", "get", "root"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run -C %s vcs get root: %v (stderr: %s)", dir, err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	gotCanon, _ := filepath.EvalSymlinks(got)
	wantCanon, _ := filepath.EvalSymlinks(dir)
	if gotCanon != wantCanon {
		t.Errorf("root = %q (canon %q), want %q", got, gotCanon, wantCanon)
	}
}

// TestRun_CwdOption_ThreeSpellingsEquivalent pins `-C V` / `--cwd V` /
// `--cwd=V` / the attached shorthand `-CV` / and its `-C=V` variant as
// producing identical behavior (DR-0043 Decision + the 2026-07-11 audit
// finding: the attached forms must actually chdir, not silently no-op
// through the never-Set() persistent flag).
func TestRun_CwdOption_ThreeSpellingsEquivalent(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	wantCanon, _ := filepath.EvalSymlinks(dir)

	forms := [][]string{
		{"-C", dir, "vcs", "get", "root"},
		{"--cwd", dir, "vcs", "get", "root"},
		{"--cwd=" + dir, "vcs", "get", "root"},
		{"-C" + dir, "vcs", "get", "root"},
		{"-C=" + dir, "vcs", "get", "root"},
	}
	for _, argv := range forms {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := runCwdLocked(t, argv, bytes.NewReader(nil), &stdout, &stderr); err != nil {
				t.Fatalf("run %v: %v (stderr: %s)", argv, err, stderr.String())
			}
			gotCanon, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
			if gotCanon != wantCanon {
				t.Errorf("run %v: root = %q, want %q", argv, gotCanon, wantCanon)
			}
		})
	}
}

// TestRun_CwdOption_LeakGuardFailsClosed is a unit test on
// cwdLeakGuardValue.Set() directly (the defence-in-depth backstop
// registered by registerCwdFlag): if some future -C/--cwd spelling ever
// slips past extractCwdOption unrecognised and reaches cobra, it must
// fail with a usage error rather than being silently absorbed by the
// dummy flag while the process keeps running in the wrong directory
// (the exact failure mode of the 2026-07-11 audit finding, before
// extractCwdOption was taught the attached-shorthand spellings).
func TestRun_CwdOption_LeakGuardFailsClosed(t *testing.T) {
	t.Parallel()
	err := (cwdLeakGuardValue{}).Set("/some/path")
	if err == nil {
		t.Fatal("cwdLeakGuardValue.Set must fail, not silently absorb the value")
	}
	if !strings.Contains(err.Error(), "-C/--cwd could not be applied") {
		t.Errorf("err = %v, want substring %q", err, "-C/--cwd could not be applied")
	}
}

// TestRun_CwdOption_FileSubcommand: -C also drives the plain FILE-input
// verbs (not just `vcs`), since the chdir happens before any input
// resolution regardless of which subcommand follows.
func TestRun_CwdOption_FileSubcommand(t *testing.T) {
	t.Parallel()
	dir := tempWriteFiles(t, map[string]string{"VERSION": "1.2.3\n"})

	var stdout, stderr bytes.Buffer
	err := runCwdLocked(t, []string{"-C", dir, "get", "VERSION"}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run -C %s get VERSION: %v (stderr: %s)", dir, err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("get VERSION = %q, want %q", got, "1.2.3")
	}
}

// TestRun_CwdOption_PositionFree: -C works trailing a fully-formed
// subcommand invocation, not just as the leading token (DR-0043 —
// pre-cobra-parse extraction, not a persistent-flag position dependent
// on cobra's own traversal).
func TestRun_CwdOption_PositionFree(t *testing.T) {
	t.Parallel()
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	wantCanon, _ := filepath.EvalSymlinks(dir)

	var stdout, stderr bytes.Buffer
	argv := []string{"vcs", "get", "root", "-C", dir}
	if err := runCwdLocked(t, argv, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run %v: %v (stderr: %s)", argv, err, stderr.String())
	}
	gotCanon, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
	if gotCanon != wantCanon {
		t.Errorf("root = %q, want %q", gotCanon, wantCanon)
	}
}

// TestRun_CwdOption_SpecifiedTwiceExit2 pins the once-only rule (DR-0043
// Decision: no relative chaining like git's -C, matching the project's
// newOnceString convention).
func TestRun_CwdOption_SpecifiedTwiceExit2(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"-C", "a", "-C", "b", "get", "VERSION"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for repeated -C")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "-C/--cwd specified twice") {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), "-C/--cwd specified twice")
	}
}

// TestRun_CwdOption_MissingValueExit2 pins the exit-2 usage error for a
// trailing `-C` with no following value.
func TestRun_CwdOption_MissingValueExit2(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"get", "VERSION", "-C"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for trailing -C with no value")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "-C/--cwd requires a value") {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), "-C/--cwd requires a value")
	}
}

// TestRun_CwdOption_NonexistentPathExit2 pins the exit-2 usage error for
// a chdir failure, requiring the message to name the offending path and
// carry the underlying cause (interface-wording: cause + what happened).
func TestRun_CwdOption_NonexistentPathExit2(t *testing.T) {
	t.Parallel()
	bad := filepath.Join(t.TempDir(), "does-not-exist")

	var stdout, stderr bytes.Buffer
	err := runCwdLocked(t, []string{"-C", bad, "get", "VERSION"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected usage error for a nonexistent -C path")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), bad) {
		t.Errorf("stderr = %q, want it to name the path %q", stderr.String(), bad)
	}
	if !strings.Contains(stderr.String(), "cannot change directory") {
		t.Errorf("stderr = %q, want the cause phrase %q", stderr.String(), "cannot change directory")
	}
}
