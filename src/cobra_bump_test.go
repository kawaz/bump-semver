package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestBuildBump_GetRejectsWriteFlags pins the get-only read-only
// rejections (legacy parseBumpArgs tail) reach the build stage with the
// exact wording.
func TestBuildBump_GetRejectsWriteFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"get-write", []string{"get", "VERSION", "--write"}, "--write is not valid with get"},
		{"get-pre", []string{"get", "VERSION", "--pre", "rc.0"}, "--pre is not valid with get (use --no-pre to strip)"},
		{"get-build-metadata", []string{"get", "VERSION", "--build-metadata", "x"}, "--build-metadata is not valid with get (use --no-build-metadata to strip)"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildArgsForTest(t, tc.argv)
			if err == nil {
				t.Fatalf("buildArgsForTest(%v) expected error, got nil", tc.argv)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestBuildBump_NoInput pins the "at least one input" wording for an empty
// bump invocation.
func TestBuildBump_NoInput(t *testing.T) {
	t.Parallel()
	_, err := buildArgsForTest(t, []string{"patch"})
	if err == nil {
		t.Fatalf("buildArgsForTest([patch]) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "at least one input (FILE | VER | -) is required") {
		t.Errorf("error = %q", err.Error())
	}
}

// TestBuildBump_DefineRuleArgvOrder verifies the DR-0029 recorder replay
// preserves argv order across interleaved --define-rule / rule flags on
// the bump path (plan §3.6 verification point): the cobra custom Value
// Set() order is the argv order, so two named blocks each receive the
// flags written between them.
func TestBuildBump_DefineRuleArgvOrder(t *testing.T) {
	t.Parallel()
	got, err := buildArgsForTest(t, []string{
		"get", "a.txt", "b.json",
		"--define-rule", "a.txt", "--version-regex", "v(.+)",
		"--define-rule", "b.json", "--format", "json",
	})
	if err != nil {
		t.Fatalf("buildArgsForTest error: %v", err)
	}
	// blocks: [global, a.txt, b.json]
	if len(got.ruleBlocks) != 3 {
		t.Fatalf("ruleBlocks: got %d, want 3", len(got.ruleBlocks))
	}
	if got.ruleBlocks[1].Pattern != "a.txt" || got.ruleBlocks[1].Opts.VersionRegex == nil || *got.ruleBlocks[1].Opts.VersionRegex != "v(.+)" {
		t.Errorf("block[1] = %+v", got.ruleBlocks[1])
	}
	if got.ruleBlocks[2].Pattern != "b.json" || got.ruleBlocks[2].Opts.Format == nil || *got.ruleBlocks[2].Opts.Format != "json" {
		t.Errorf("block[2] = %+v", got.ruleBlocks[2])
	}
}

// TestRun_PreValueDashQQ pins the value-position guard end-to-end: a
// `-qq` passed as the value of --pre must reach the bump as the literal
// pre-release "-qq" (not get rewritten to --quiet-all by the verbosity
// normalisation). Regression for normalizeQuietAll clobbering value tokens.
func TestRun_PreValueDashQQ(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"major", "1.2.3", "--pre", "-qq"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run major 1.2.3 --pre -qq: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "2.0.0--qq" {
		t.Errorf("stdout = %q, want %q", got, "2.0.0--qq")
	}
}

// TestRun_DoubleBoolFlagRejected pins that --write / --no-pre /
// --no-build-metadata reject a second occurrence with the legacy
// "specified twice" wording (onceBoolValue), routed through run() so the
// flagErrorFunc shaping is exercised.
func TestRun_DoubleBoolFlagRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"double-write", []string{"patch", "1.2.3", "--write", "--write"}, "--write specified twice"},
		{"double-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre", "--no-pre"}, "--no-pre specified twice"},
		{"double-no-build", []string{"patch", "1.2.3", "--no-build-metadata", "--no-build-metadata"}, "--no-build-metadata specified twice"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			err := run(tc.argv, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
			if err == nil {
				t.Fatalf("run(%v) expected error, got nil", tc.argv)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tc.want)
			}
		})
	}
}
