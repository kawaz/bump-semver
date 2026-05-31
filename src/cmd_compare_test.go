package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Compare_Eq_True(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"compare", "eq", "1.2.3", "1.2.3"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("compare should not print on success, got: %q", stdout.String())
	}
}

func TestRun_Compare_Eq_False(t *testing.T) {
	t.Parallel()
	err := run([]string{"compare", "eq", "1.2.3", "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected predicate-false (exit 1) error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != 1 {
		t.Errorf("exit code = %d, want 1", ee.code)
	}
}

func TestRun_Compare_AllOps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op       string
		a, b     string
		wantTrue bool
	}{
		{"eq", "1.2.3", "1.2.3", true},
		{"eq", "v1.2.3", "1.2.3", true},    // prefix ignored
		{"eq", "1.2.3+a", "1.2.3+b", true}, // build metadata ignored
		{"eq", "1.2.3", "1.2.4", false},
		{"lt", "1.2.3", "1.2.4", true},
		{"lt", "1.2.3-rc.1", "1.2.3", true},
		{"lt", "1.2.3", "1.2.3", false},
		{"le", "1.2.3", "1.2.3", true},
		{"le", "1.2.3", "1.2.4", true},
		{"le", "1.2.4", "1.2.3", false},
		{"gt", "2.0.0", "1.0.0", true},
		{"gt", "1.0.0", "2.0.0", false},
		{"gt", "1.0.0", "1.0.0", false},
		{"ge", "1.0.0", "1.0.0", true},
		{"ge", "1.0.0", "0.9.9", true},
		{"ge", "1.0.0", "2.0.0", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.op+"_"+c.a+"_"+c.b, func(t *testing.T) {
			t.Parallel()
			err := run([]string{"compare", c.op, c.a, c.b}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if c.wantTrue {
				if err != nil {
					t.Errorf("expected success (true), got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected predicate-false (exit 1), got success")
			}
			var ee *exitErr
			if !errors.As(err, &ee) {
				t.Fatalf("expected *exitErr, got %T: %v", err, err)
			}
			if ee.code != 1 {
				t.Errorf("exit code = %d, want 1", ee.code)
			}
		})
	}
}

// TestRun_Compare_PrecisionOps pins DR-0017 precision-suffix OPs at
// the CLI layer. TestVersion_CompareAt covers the underlying math;
// this test ensures parseArgs → runCompare → exit-code mapping all
// agree for the suffix form.
func TestRun_Compare_PrecisionOps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op       string
		a, b     string
		wantTrue bool
	}{
		// -major: only X matters.
		{"eq-major", "1.2.3", "1.9.7", true},
		{"eq-major", "1.2.3", "2.0.0", false},
		{"lt-major", "1.9.9", "2.0.0-rc.0", true}, // pre-release on bigger is ignored
		{"ge-major", "1.0.0", "1.99.99", true},

		// -minor: X.Y only.
		{"eq-minor", "1.2.3", "1.2.9", true},
		{"eq-minor", "1.2.3", "1.3.0", false},
		{"lt-minor", "1.2.9", "1.3.0-rc.0", true},

		// -patch: X.Y.Z; pre-release ignored.
		{"eq-patch", "1.2.3", "1.2.3-rc.1", true},
		{"eq-patch", "1.2.3-rc.1", "1.2.3-rc.99", true},
		{"eq-patch", "1.2.3", "1.2.4", false},
		{"gt-patch", "1.2.4-rc.0", "1.2.3", true},
		{"ge-patch", "1.2.3-rc.0", "1.2.3", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.op+"_"+c.a+"_"+c.b, func(t *testing.T) {
			t.Parallel()
			err := run([]string{"compare", c.op, c.a, c.b}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if c.wantTrue {
				if err != nil {
					t.Errorf("expected success (true), got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected predicate-false (exit 1), got success")
			}
			var ee *exitErr
			if !errors.As(err, &ee) {
				t.Fatalf("expected *exitErr, got %T: %v", err, err)
			}
			if ee.code != 1 {
				t.Errorf("exit code = %d, want 1", ee.code)
			}
		})
	}
}

// TestRun_Compare_PrecisionInvalidOps pins error paths for malformed
// precision suffixes — they must exit 2 (error), not 1 (predicate
// false).
func TestRun_Compare_PrecisionInvalidOps(t *testing.T) {
	t.Parallel()
	cases := []string{
		"eq-foo",         // unknown precision
		"neq-major",      // unknown base
		"eq-",            // empty precision
		"eq-major-minor", // double suffix
		"-major",         // empty base
	}
	for _, op := range cases {
		op := op
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			err := run([]string{"compare", op, "1.2.3", "1.2.3"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("expected error for op %q", op)
			}
			var ee *exitErr
			if !errors.As(err, &ee) {
				t.Fatalf("expected *exitErr, got %T: %v", err, err)
			}
			if ee.code != 2 {
				t.Errorf("invalid OP should map to exit 2, got %d: %v", ee.code, err)
			}
		})
	}
}

func TestRun_Compare_ParseError(t *testing.T) {
	t.Parallel()
	err := run([]string{"compare", "eq", "1.2.3", "not-a-version"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected parse error")
	}
	// Phase 5: run() always returns *exitErr. A parse error must carry
	// exit code 2, not 1 (which is reserved for predicate-false).
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != 2 {
		t.Errorf("parse errors should map to exit 2 (got %d): %v", ee.code, err)
	}
}

func TestRun_Compare_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"name":"foo","version":"1.2.4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"compare", "lt", a, b}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Errorf("expected lt true, got: %v", err)
	}
}

// --- exit code semantics for main ------------------------------------------

// compare with -qq suppresses the parse-error stderr line. Predicate-
// false (exit 1) has no stderr output to suppress in the first place.
func TestRun_Compare_QuietAllSuppressesError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"compare", "eq", "garbage", "garbage", "-qq"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq must suppress stderr, got: %q", stderr.String())
	}
}

// DR-0023: predicate-false compare emits a per-OTHER failure listing
// on stderr (so users see *why* a multi-OTHER assertion failed). The
// listing is preserved under -q (only DR-0010 hints are quiet there)
// and suppressed under -qq (consistent with quiet-all suppressing
// every diagnostic).
func TestRun_Compare_PredicateFalse_StderrUnderQuiet(t *testing.T) {
	t.Parallel()
	// -q: per-OTHER listing still appears.
	var stdout, stderr bytes.Buffer
	err := run([]string{"compare", "eq", "1.2.3", "1.2.4", "-q"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected predicate-false (exit 1)")
	}
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T", err)
	}
	if ee.code != 1 {
		t.Errorf("exit code = %d, want 1", ee.code)
	}
	if stdout.Len() != 0 {
		t.Errorf("compare stdout must be empty, got: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "compare eq") || !strings.Contains(stderr.String(), "not equal to") {
		t.Errorf("-q must preserve per-OTHER stderr, got: %q", stderr.String())
	}
}
