package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestUseCobra_Stage2Routing pins the Stage 2 co-existence router: the
// global short-circuit forms (--version / -V / --help / -h /
// --help-full), the no-argument case AND the whole `vcs` subtree are
// routed to cobra; the remaining real verbs (bump / compare) still flow
// through the legacy parser.
func TestUseCobra_Stage2Routing(t *testing.T) {
	t.Parallel()
	cobraCases := [][]string{
		{},
		{"--version"},
		{"-V"},
		{"--version", "--json"},
		{"--help"},
		{"-h"},
		{"--help-full"},
		{"vcs"},
		{"vcs", "get", "root"},
	}
	for _, argv := range cobraCases {
		if !useCobra(argv) {
			t.Errorf("useCobra(%v) = false, want true (cobra route)", argv)
		}
	}
	legacyCases := [][]string{
		{"major"},
		{"minor", "1.2.3"},
		{"get"},
		{"compare", "eq", "1.0.0", "1.0.0"},
	}
	for _, argv := range legacyCases {
		if useCobra(argv) {
			t.Errorf("useCobra(%v) = true, want false (legacy route)", argv)
		}
	}
}

// TestRunCobra_EmptyArgsShortHelp verifies the cobra path itself (not the
// legacy parser) produces the short help for the no-argument case.
func TestRunCobra_EmptyArgsShortHelp(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := runCobra(nil, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runCobra(nil) error: %v", err)
	}
	if !strings.Contains(stdout.String(), "See 'bump-semver <command> --help'") {
		t.Errorf("runCobra(nil) did not emit short help, got:\n%s", stdout.String())
	}
}

// TestRunCobra_VersionOnlyAcceptsJSON pins the --version usage-error
// wording through the cobra path.
func TestRunCobra_VersionOnlyAcceptsJSON(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := runCobra([]string{"--version", "--bogus"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	var ee *exitErr
	if !asExitErr(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exitErr code=%d, got %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--version only accepts --json") {
		t.Errorf("stderr missing expected wording, got: %q", stderr.String())
	}
}

func asExitErr(err error, target **exitErr) bool {
	for err != nil {
		if ee, ok := err.(*exitErr); ok {
			*target = ee
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
