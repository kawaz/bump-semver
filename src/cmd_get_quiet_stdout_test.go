package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_GetQuietKeepsStdoutValue pins the get-only rule that the
// version value (primary output) is always printed, even under -q /
// --quiet / -qq / --quiet-all. For get the stdout value IS the
// deliverable, so quiet flags only silence hints (-q) and errors (-qq),
// never the value itself. Regression for the machine-use idiom
// `ref=$(bump-semver get ... -qq 2>/dev/null)` returning empty.
func TestRun_GetQuietKeepsStdoutValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
	}{
		{"get-q", []string{"get", "1.2.3", "-q"}},
		{"get-quiet-long", []string{"get", "1.2.3", "--quiet"}},
		{"get-qq", []string{"get", "1.2.3", "-qq"}},
		{"get-quiet-all-long", []string{"get", "1.2.3", "--quiet-all"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			err := run(tc.argv, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("run(%v): %v", tc.argv, err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
				t.Errorf("stdout = %q, want %q", got, "1.2.3")
			}
		})
	}
}

// TestRun_GetQuietJSONKeepsStdout pins that the --json value output is
// also retained under quiet flags (get's value path, JSON variant).
func TestRun_GetQuietJSONKeepsStdout(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := run([]string{"get", "1.2.3", "--json", "-qq"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run get --json -qq: %v", err)
	}
	if !strings.Contains(stdout.String(), "1.2.3") {
		t.Errorf("stdout = %q, want it to contain %q", stdout.String(), "1.2.3")
	}
}

// TestRun_BumpQuietStillSuppressesStdout pins the unchanged behaviour:
// for bump verbs (major/minor/patch/pre) the stdout value IS still
// suppressed by -q / -qq (the value is a side effect of the bump, not
// the primary deliverable). Only get is exempt.
func TestRun_BumpQuietStillSuppressesStdout(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
	}{
		{"major-q", []string{"major", "1.2.3", "-q"}},
		{"minor-qq", []string{"minor", "1.2.3", "-qq"}},
		{"patch-q", []string{"patch", "1.2.3", "-q"}},
		{"pre-qq", []string{"pre", "1.2.3-rc.0", "-qq"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			err := run(tc.argv, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("run(%v): %v", tc.argv, err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "" {
				t.Errorf("stdout = %q, want empty (bump value suppressed by quiet)", got)
			}
		})
	}
}
