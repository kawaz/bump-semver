package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `bump` actions emit the bumped version through ToJSON, with all
// non-pre/build fields populated.
func TestRun_JSON_Bump(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"patch", "1.2.3", "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	m := decodeJSON(t, stdout.String())
	if v := jsonField(t, m, "version"); v != "1.2.4" {
		t.Errorf("version = %v, want 1.2.4", v)
	}
	if v := jsonField(t, m, "semver"); v != "1.2.4" {
		t.Errorf("semver = %v, want 1.2.4", v)
	}
	if v := jsonField(t, m, "major"); v != float64(1) {
		t.Errorf("major = %v, want 1", v)
	}
	if v := jsonField(t, m, "minor"); v != float64(2) {
		t.Errorf("minor = %v, want 2", v)
	}
	if v := jsonField(t, m, "patch"); v != float64(4) {
		t.Errorf("patch = %v, want 4", v)
	}
	// VER origin: name is null.
	if v := jsonField(t, m, "name"); v != nil {
		t.Errorf("name = %v, want null", v)
	}
}

// `get` is a read-only action: --json renders the current value
// through the same path as bump, populating name from FILE origin.
func TestRun_JSON_Get(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"my-pkg","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"get", pkg, "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	m := decodeJSON(t, stdout.String())
	if v := jsonField(t, m, "version"); v != "1.2.3" {
		t.Errorf("version = %v, want 1.2.3", v)
	}
	if v := jsonField(t, m, "name"); v != "my-pkg" {
		t.Errorf("name = %v, want my-pkg", v)
	}
}

// FILE+VER mix: name comes from the FILE origin (DR-0004 already
// validates name consistency across multiple FILEs).
func TestRun_JSON_FileVerMix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"patch", pkg, "1.2.3", "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	m := decodeJSON(t, stdout.String())
	if v := jsonField(t, m, "name"); v != "foo" {
		t.Errorf("name = %v, want foo (from FILE)", v)
	}
	if v := jsonField(t, m, "version"); v != "1.2.4" {
		t.Errorf("version = %v, want 1.2.4", v)
	}
}

// pre/build fields render as JSON null when absent (not empty string).
func TestRun_JSON_NullPre(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"get", "1.2.3", "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	m := decodeJSON(t, stdout.String())
	for _, key := range []string{"pre", "pre_id", "pre_rest", "build_metadata", "build_id", "build_rest", "name"} {
		v, ok := m[key]
		if !ok {
			t.Errorf("missing key %q (must be present even when null)", key)
		}
		if v != nil {
			t.Errorf("%s = %v, want null", key, v)
		}
	}
}

// pre_id / pre_rest split at the first '.': DR-0007 example table.
func TestRun_JSON_PreRest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantPre  string
		wantID   string
		wantRest any // string or nil
	}{
		{"1.2.3-rc.1", "rc.1", "rc", "1"},
		{"1.2.3-alpha.beta.5", "alpha.beta.5", "alpha", "beta.5"},
		{"1.2.3-alpha", "alpha", "alpha", nil},
		{"1.2.3-rc1", "rc1", "rc1", nil},
		{"1.2.3-0", "0", "0", nil},
		{"1.2.3-0.3.7", "0.3.7", "0", "3.7"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			if err := run([]string{"get", c.in, "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
				t.Fatalf("run error: %v", err)
			}
			m := decodeJSON(t, stdout.String())
			if v := jsonField(t, m, "pre"); v != c.wantPre {
				t.Errorf("%s: pre = %v, want %q", c.in, v, c.wantPre)
			}
			if v := jsonField(t, m, "pre_id"); v != c.wantID {
				t.Errorf("%s: pre_id = %v, want %q", c.in, v, c.wantID)
			}
			if v := jsonField(t, m, "pre_rest"); v != c.wantRest {
				t.Errorf("%s: pre_rest = %v, want %v", c.in, v, c.wantRest)
			}
		})
	}
}

// compare must reject --json (DR-0007: predicate-only, no stdout).
func TestRun_JSON_Compare_Rejected(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"compare", "eq", "1.2.3", "1.2.3", "--json"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected --json with compare to error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected *exitErr code=2, got %v (%T)", err, err)
	}
	if !strings.Contains(stderr.String(), "compare does not support --json") {
		t.Errorf("stderr should mention the rejection reason, got: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on rejection, got: %q", stdout.String())
	}
}

// -q (and -qq) suppress the JSON output too. The exit code path is
// unchanged; only stdout is muted.
func TestRun_JSON_QuietSuppresses(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", "1.2.3", "--json", "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("-q must suppress JSON stdout, got: %q", stdout.String())
	}
}

// version + semver diverge when prefix / body sep is present.
func TestRun_JSON_PrefixSemverDivergence(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := run([]string{"get", "v_1_2_3-rc.1+build.42", "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	m := decodeJSON(t, stdout.String())
	if v := jsonField(t, m, "version"); v != "v_1_2_3-rc.1+build.42" {
		t.Errorf("version = %v, want v_1_2_3-rc.1+build.42 (preserved)", v)
	}
	if v := jsonField(t, m, "semver"); v != "1.2.3-rc.1+build.42" {
		t.Errorf("semver = %v, want 1.2.3-rc.1+build.42 (strict)", v)
	}
}
