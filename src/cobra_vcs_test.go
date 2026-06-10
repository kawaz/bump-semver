package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestCobraVcs_UnknownVerbWording pins the unknown-vcs-verb usage error
// (exit 2) routed through the cobra `vcs` parent RunE: cobra calls the
// parent RunE for any token that is not a registered child, which hands
// it to the dispatcher's "unknown vcs verb" branch.
func TestCobraVcs_UnknownVerbWording(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"vcs", "bogus"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for unknown vcs verb")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "unknown vcs verb: bogus") {
		t.Errorf("stderr should name the unknown verb, got: %q", stderr.String())
	}
}

// TestParseOutdatedTokens_PairSeparatorLiteral is a focused unit test on
// the DisableFlagParsing path: `--` must survive as a literal token in
// vcsArgs (the pair separator splitOutdatedPairs scans for), and flags
// before the positionals must be consumed.
func TestParseOutdatedTokens_PairSeparatorLiteral(t *testing.T) {
	t.Parallel()
	raw := []string{
		"--strict",
		"--", "F1", "T1",
		"--", "F2", "T2",
	}
	got, err := parseOutdatedTokens(cliArgs{kind: "vcs"}, raw)
	if err != nil {
		t.Fatalf("parseOutdatedTokens: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil cliArgs (not the help short-circuit)")
	}
	if !got.vcsOutdated.Strict {
		t.Error("--strict not recorded")
	}
	want := []string{"--", "F1", "T1", "--", "F2", "T2"}
	if !reflect.DeepEqual(got.vcsArgs, want) {
		t.Errorf("vcsArgs = %v, want %v (literal `--` preserved)", got.vcsArgs, want)
	}
}

// TestParseOutdatedTokens_HelpShortCircuit: a bare token stream or a
// leading --help returns (nil, nil) = "show outdated help".
func TestParseOutdatedTokens_HelpShortCircuit(t *testing.T) {
	t.Parallel()
	for _, raw := range [][]string{nil, {"--help"}, {"-h"}} {
		got, err := parseOutdatedTokens(cliArgs{kind: "vcs"}, raw)
		if err != nil {
			t.Errorf("parseOutdatedTokens(%v): unexpected err %v", raw, err)
		}
		if got != nil {
			t.Errorf("parseOutdatedTokens(%v) = %+v, want nil (help)", raw, got)
		}
	}
}

// TestParseOutdatedTokens_QuietAll: both `-qq` and the normalised
// `--quiet-all` raise verbosity to outputQuietAll (the outdated path is
// reachable by both because runCobra rewrites `-qq` before SetArgs).
func TestParseOutdatedTokens_QuietAll(t *testing.T) {
	t.Parallel()
	for _, tok := range []string{"-qq", "--quiet-all"} {
		got, err := parseOutdatedTokens(cliArgs{kind: "vcs"}, []string{tok, "F", "T"})
		if err != nil {
			t.Fatalf("parseOutdatedTokens(%s): %v", tok, err)
		}
		if got.output.Verbosity != outputQuietAll {
			t.Errorf("token %s: verbosity = %v, want outputQuietAll", tok, got.output.Verbosity)
		}
	}
}

// TestNormalizeQuietAll covers the `-qq` → `--quiet-all` rewrite and the
// `--` boundary that stops it (post-separator positionals are untouched).
func TestNormalizeQuietAll(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"diff", "-qq", "HEAD"}, []string{"diff", "--quiet-all", "HEAD"}},
		{[]string{"diff", "-q", "HEAD"}, []string{"diff", "-q", "HEAD"}},
		{[]string{"outdated", "--", "-qq", "T"}, []string{"outdated", "--", "-qq", "T"}},
	}
	for _, c := range cases {
		got := normalizeQuietAll(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("normalizeQuietAll(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
