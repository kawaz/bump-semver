package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestRun_Vcs_NoArgs: `bump-semver vcs` with no verb shows help on stdout
// and exits 0 (= kawaz CLI design: no-args == --help).
func TestRun_Vcs_NoArgs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "vcs") {
		t.Errorf("expected help on stdout, got: %q", stdout.String())
	}
}

// TestRun_Vcs_UnknownVerb: an unknown vcs verb is a usage error (exit 2).
func TestRun_Vcs_UnknownVerb(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for unknown verb")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
}
