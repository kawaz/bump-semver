package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want cliArgs
	}{
		{"bump-file", []string{"patch", "Cargo.toml"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}}},
		{"bump-file-write", []string{"patch", "Cargo.toml", "--write"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}, write: true}},
		{"write-before-input", []string{"patch", "--write", "Cargo.toml"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"Cargo.toml"}, write: true}},
		{"get-file", []string{"get", "VERSION"}, cliArgs{kind: "bump", action: "get", inputs: []string{"VERSION"}}},
		{"bump-ver", []string{"minor", "1.2.3"}, cliArgs{kind: "bump", action: "minor", inputs: []string{"1.2.3"}}},
		{"version-flag", []string{"--version"}, cliArgs{kind: "version"}},
		{"version-short", []string{"-V"}, cliArgs{kind: "version"}},
		{"version-json", []string{"--version", "--json"}, cliArgs{kind: "version", json: true}},
		{"version-json-short", []string{"-V", "--json"}, cliArgs{kind: "version", json: true}},
		{"help-flag", []string{"--help"}, cliArgs{kind: "help"}},
		{"help-short", []string{"-h"}, cliArgs{kind: "help"}},
		{"help-full", []string{"--help-full"}, cliArgs{kind: "helpFull"}},
		{"empty", []string{}, cliArgs{kind: "help"}},
		// v0.13.0 subcommand --help dispatch (DR-0017 周辺)
		{"action-help-major", []string{"major", "--help"}, cliArgs{kind: "helpAction", action: "major"}},
		{"action-help-minor", []string{"minor", "--help"}, cliArgs{kind: "helpAction", action: "minor"}},
		{"action-help-patch", []string{"patch", "--help"}, cliArgs{kind: "helpAction", action: "patch"}},
		{"action-help-patch-short", []string{"patch", "-h"}, cliArgs{kind: "helpAction", action: "patch"}},
		{"action-help-pre", []string{"pre", "--help"}, cliArgs{kind: "helpAction", action: "pre"}},
		{"action-help-get", []string{"get", "--help"}, cliArgs{kind: "helpAction", action: "get"}},
		{"action-help-compare-no-op", []string{"compare", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		{"action-help-compare-op-then-help", []string{"compare", "eq", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		{"action-help-compare-precision-then-help", []string{"compare", "eq-major", "--help"}, cliArgs{kind: "helpAction", action: "compare"}},
		// --vcs auto (DR-0016) happy path
		{"vcs-flag-auto", []string{"patch", "1.2.3", "--vcs", "auto"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcs: "auto", vcsSet: true}},
		{"dash-dash-passthrough", []string{"patch", "--", "--weird-file.json"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"--weird-file.json"}}},
		{"multi-file", []string{"get", "package.json", "package-lock.json"}, cliArgs{kind: "bump", action: "get", inputs: []string{"package.json", "package-lock.json"}}},
		{"multi-file-write", []string{"patch", "a.json", "b.json", "c.json", "--write"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"a.json", "b.json", "c.json"}, write: true}},
		// pre action with cross-cutting flags
		{"pre-with-pre", []string{"pre", "1.2.3", "--pre", "rc.0"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3"}, pre: "rc.0", preSet: true}},
		{"pre-with-pre-eq", []string{"pre", "1.2.3", "--pre=rc.0"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3"}, pre: "rc.0", preSet: true}},
		{"pre-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre"}, cliArgs{kind: "bump", action: "pre", inputs: []string{"1.2.3-rc.0"}, noPre: true}},
		{"patch-build-meta", []string{"patch", "1.2.3", "--build-metadata", "sha.abc"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, buildMetadata: "sha.abc", buildMetadataSet: true}},
		{"patch-build-meta-eq", []string{"patch", "1.2.3", "--build-metadata=sha.abc"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, buildMetadata: "sha.abc", buildMetadataSet: true}},
		{"patch-no-build-meta", []string{"patch", "1.2.3+x", "--no-build-metadata"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3+x"}, noBuildMetadata: true}},
		// stdin marker
		{"stdin-marker", []string{"patch", "-"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"-"}}},
		// compare
		{"compare-eq", []string{"compare", "eq", "1.2.3", "1.2.3"}, cliArgs{kind: "compare", compareOp: "eq", inputs: []string{"1.2.3", "1.2.3"}}},
		{"compare-lt-files", []string{"compare", "lt", "a.json", "b.json"}, cliArgs{kind: "compare", compareOp: "lt", inputs: []string{"a.json", "b.json"}}},
		{"compare-ge-stdin", []string{"compare", "ge", "1.2.3", "-"}, cliArgs{kind: "compare", compareOp: "ge", inputs: []string{"1.2.3", "-"}}},
		// DR-0017: compare precision suffix split into base + precision
		{"compare-eq-major", []string{"compare", "eq-major", "1.2.3", "1.9.7"}, cliArgs{kind: "compare", compareOp: "eq", comparePrecision: "major", inputs: []string{"1.2.3", "1.9.7"}}},
		{"compare-lt-minor", []string{"compare", "lt-minor", "1.2.9", "1.3.0"}, cliArgs{kind: "compare", compareOp: "lt", comparePrecision: "minor", inputs: []string{"1.2.9", "1.3.0"}}},
		{"compare-ge-patch", []string{"compare", "ge-patch", "1.2.3", "1.2.3-rc.0"}, cliArgs{kind: "compare", compareOp: "ge", comparePrecision: "patch", inputs: []string{"1.2.3", "1.2.3-rc.0"}}},
		// DR-0008: vcs flag and vcs: inputs survive parseArgs intact
		{"vcs-flag-jj", []string{"patch", "1.2.3", "--vcs", "jj"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcs: "jj", vcsSet: true}},
		{"vcs-flag-git-eq", []string{"patch", "1.2.3", "--vcs=git"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, vcs: "git", vcsSet: true}},
		{"vcs-input-bump", []string{"patch", "vcs:HEAD"}, cliArgs{kind: "bump", action: "patch", inputs: []string{"vcs:HEAD"}}},
		{"vcs-input-compare", []string{"compare", "gt", "Cargo.toml", "vcs:latest-tag()"}, cliArgs{kind: "compare", compareOp: "gt", inputs: []string{"Cargo.toml", "vcs:latest-tag()"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(tc.argv)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", tc.argv, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseArgs(%v)\n  got = %+v\n  want= %+v", tc.argv, got, tc.want)
			}
		})
	}
}

func TestParseArgs_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
	}{
		{"unknown-action", []string{"foo", "Cargo.toml"}},
		{"version-with-other-flag", []string{"--version", "--quiet"}},
		{"version-short-with-other-flag", []string{"-V", "--no-hint"}},
		{"version-with-positional", []string{"--version", "Cargo.toml"}},
		{"missing-input", []string{"patch"}},
		{"unknown-flag", []string{"patch", "Cargo.toml", "--unknown"}},
		{"double-write", []string{"patch", "Cargo.toml", "--write", "--write"}},
		{"get-with-write", []string{"get", "VERSION", "--write"}},
		{"get-with-pre", []string{"get", "VERSION", "--pre", "rc.0"}},
		{"get-with-build-metadata", []string{"get", "VERSION", "--build-metadata", "sha.x"}},
		{"compare-with-write", []string{"compare", "eq", "1.2.3", "1.2.3", "--write"}},
		{"compare-with-pre", []string{"compare", "eq", "1.2.3", "1.2.3", "--pre", "rc.0"}},
		{"compare-with-build-meta", []string{"compare", "eq", "1.2.3", "1.2.3", "--build-metadata", "sha"}},
		{"compare-too-few", []string{"compare", "eq", "1.2.3"}},
		{"compare-too-many", []string{"compare", "eq", "1.2.3", "1.2.3", "1.2.4"}},
		{"compare-no-op", []string{"compare"}},
		{"compare-bad-op", []string{"compare", "neq", "1.2.3", "1.2.3"}},
		// DR-0017: precision suffix validation
		{"compare-bad-precision", []string{"compare", "eq-foo", "1.2.3", "1.2.3"}},
		{"compare-bad-base-with-precision", []string{"compare", "neq-major", "1.2.3", "1.2.3"}},
		{"compare-empty-precision", []string{"compare", "eq-", "1.2.3", "1.2.3"}},
		{"compare-double-precision", []string{"compare", "eq-major-minor", "1.2.3", "1.2.3"}},
		{"pre-and-no-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--no-pre"}},
		{"build-and-no-build", []string{"patch", "1.2.3", "--build-metadata", "x", "--no-build-metadata"}},
		{"empty-pre", []string{"pre", "1.2.3", "--pre", ""}},
		{"empty-build-metadata", []string{"patch", "1.2.3", "--build-metadata", ""}},
		{"pre-missing-arg", []string{"pre", "1.2.3", "--pre"}},
		{"build-missing-arg", []string{"patch", "1.2.3", "--build-metadata"}},
		{"double-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--pre", "rc.1"}},
		{"double-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre", "--no-pre"}},
		// DR-0008: --vcs validation
		{"vcs-bad-value", []string{"patch", "1.2.3", "--vcs", "hg"}},
		{"vcs-missing-arg", []string{"patch", "1.2.3", "--vcs"}},
		{"vcs-double", []string{"patch", "1.2.3", "--vcs", "git", "--vcs", "jj"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseArgs(tc.argv); err == nil {
				t.Errorf("parseArgs(%v) expected error, got nil", tc.argv)
			}
		})
	}
}

func TestRun_VerBumps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"patch", "1.2.3"}, "1.2.4\n"},
		{[]string{"minor", "1.2.3"}, "1.3.0\n"},
		{[]string{"major", "1.2.3"}, "2.0.0\n"},
		{[]string{"get", "1.2.3"}, "1.2.3\n"},
		// v prefix / 柔軟 separator も最終的に同じ経路を通る
		{[]string{"patch", "v1.2.3"}, "v1.2.4\n"},
		{[]string{"minor", "version_1_2_3"}, "version_1_3_0\n"},
		// DR-0006: body sep `-` removed; "ver-1.2.3" still works because
		// the `-` is part of the prefix.
		{[]string{"major", "ver-1.2.3"}, "ver-2.0.0\n"},
		{[]string{"get", "v1.2.3"}, "v1.2.3\n"},
		// pre action
		{[]string{"pre", "1.2.3-rc.0"}, "1.2.3-rc.1\n"},
		{[]string{"pre", "1.2.3", "--pre", "rc.0"}, "1.2.3-rc.0\n"},
		{[]string{"pre", "1.2.3-rc.0", "--no-pre"}, "1.2.3\n"},
		// patch with --pre re-attaches pre after bump
		{[]string{"patch", "1.2.3", "--pre", "rc.0"}, "1.2.4-rc.0\n"},
		// patch with --build-metadata
		{[]string{"patch", "1.2.3", "--build-metadata", "sha.abc"}, "1.2.4+sha.abc\n"},
		// get with --no-pre / --no-build-metadata
		{[]string{"get", "1.2.3-rc.0", "--no-pre"}, "1.2.3\n"},
		{[]string{"get", "1.2.3+build", "--no-build-metadata"}, "1.2.3\n"},
	}
	for _, tc := range cases {
		var stdout bytes.Buffer
		if err := run(tc.argv, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
			t.Errorf("run(%v) error: %v", tc.argv, err)
			continue
		}
		if stdout.String() != tc.want {
			t.Errorf("run(%v) stdout = %q, want %q", tc.argv, stdout.String(), tc.want)
		}
	}
}

func TestRun_RejectsBadVer(t *testing.T) {
	t.Parallel()
	// Truly malformed input (1.2.3-alpha is now valid, see DR-0006).
	if err := run([]string{"patch", "not-a-version"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Error("expected error for malformed input")
	}
}

func TestRun_PreActionErrorOriginContext(t *testing.T) {
	t.Parallel()
	// 確定論点 E: VER-origin pass-through (no file context).
	err := run([]string{"pre", "1.2.3-rc1"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "rc1 is not incremental") {
		t.Errorf("error should mention 'rc1 is not incremental': %q", msg)
	}
	// VER-origin: should NOT be wrapped with file path.
	if strings.Contains(msg, "<argv>") {
		t.Errorf("VER-origin error should be passed through, got: %q", msg)
	}

	// FILE-origin: wrap with file:path=value.
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3-rc1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"pre", path}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg = err.Error()
	if !strings.Contains(msg, path) {
		t.Errorf("FILE-origin error should contain file path %q, got: %q", path, msg)
	}
	if !strings.Contains(msg, "rc1 is not incremental") {
		t.Errorf("error should preserve semver-layer message: %q", msg)
	}
}

func TestRun_FileGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.String() != "1.2.3\n" {
		t.Errorf("stdout = %q, want 1.2.3\\n", stdout.String())
	}
}

func TestRun_FileWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	src := []byte(`{
  "name": "foo",
  "version": "1.2.3"
}
`)
	if err := os.WriteFile(path, src, 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"version": "1.2.4"`) {
		t.Errorf("file not updated:\n%s", string(got))
	}
}

func TestRun_UnsupportedFile(t *testing.T) {
	t.Parallel()
	// README.md is neither a supported file nor a parseable VER. We
	// expect a clear error.
	err := run([]string{"get", "README.md"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for unsupported input, got nil")
	}
}

func TestRun_FileGetWithVPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("v1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.String() != "v1.2.3\n" {
		t.Errorf("stdout = %q, want v1.2.3\\n", stdout.String())
	}
}

func TestRun_FileWriteVPrefixPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	src := []byte(`{"name":"foo","version":"v1.2.3"}`)
	if err := os.WriteFile(path, src, 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := stdout.String(); got != "v1.2.4\n" {
		t.Errorf("stdout = %q, want v1.2.4\\n", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"version":"v1.2.4"`) {
		t.Errorf("file not updated:\n%s", string(got))
	}
}

func TestRun_FileWritePreservesMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	src := []byte(`{"name":"foo","version":"1.2.3"}`)
	if err := os.WriteFile(path, src, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("permission = %o, want 0600", got)
	}
}

func TestRun_StdinPipe(t *testing.T) {
	t.Parallel()
	// Use os.Pipe() so stdin is detected as a pipe (not char device).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.Write([]byte(`{"version": "1.2.3"}`))
		_ = w.Close()
	}()
	var stdout bytes.Buffer
	if err := run([]string{"patch", "package.json"}, r, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", stdout.String())
	}
	_ = r.Close()
}

func TestRun_StdinPipeWriteRejected(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.Write([]byte(`{"version": "1.2.3"}`))
		_ = w.Close()
	}()
	err = run([]string{"patch", "package.json", "--write"}, r, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected error for stdin pipe + --write")
	}
	_ = r.Close()
}

// `-` marker reads VER from stdin (single line).
func TestRun_StdinDashMarker(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.Write([]byte("1.2.3\n"))
		_ = w.Close()
	}()
	var stdout bytes.Buffer
	if err := run([]string{"patch", "-"}, r, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", got)
	}
	_ = r.Close()
}

// --- multi-file tests --------------------------------------------------------

func TestRun_MultiFile_AllSame(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	plug := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plug, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"patch", pkg, plug, "--write"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", got)
	}
	for _, p := range []string{pkg, plug} {
		got, _ := os.ReadFile(p)
		if !strings.Contains(string(got), `"version":"1.2.4"`) {
			t.Errorf("%s not updated:\n%s", p, string(got))
		}
	}
}

func TestRun_MultiFile_VersionMismatch(t *testing.T) {
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
	err := run([]string{"patch", a, b, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "version mismatch:") {
		t.Errorf("error does not start with 'version mismatch:': %q", msg)
	}
	if !strings.Contains(msg, "1.2.3") || !strings.Contains(msg, "1.2.4") {
		t.Errorf("error should list both values, got: %q", msg)
	}
}

func TestRun_MultiFile_NameMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"name":"bar","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"patch", a, b, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected name mismatch error, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "name mismatch:") {
		t.Errorf("error does not start with 'name mismatch:': %q", msg)
	}
	if !strings.Contains(msg, "foo") || !strings.Contains(msg, "bar") {
		t.Errorf("error should list both names, got: %q", msg)
	}
}

func TestRun_MultiFile_NameOptional(t *testing.T) {
	t.Parallel()
	// VERSION (no name) + package.json (name=foo) — version match is enough.
	dir := t.TempDir()
	v := filepath.Join(dir, "VERSION")
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(v, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkg, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"minor", v, pkg, "--write"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.3.0" {
		t.Errorf("stdout = %q, want 1.3.0", got)
	}
}

func TestRun_MultiFile_GetForVerification(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"name":"x","version":"1.2.4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"get", a, b}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected version mismatch on get, got nil")
	}
}

func TestRun_StdinPipeIgnoredWithMultiFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.Write([]byte(`garbage to be ignored`))
		_ = w.Close()
	}()
	var stdout bytes.Buffer
	if err := run([]string{"patch", a, b}, r, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4 (read from files, not stdin)", got)
	}
	_ = r.Close()
}

// --- FILE / VER mix ----------------------------------------------------------

func TestRun_FileVerMix_Consistent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	// FILE + VER both at 1.2.3 — passes consistency, bumps to 1.2.4.
	if err := run([]string{"patch", pkg, "1.2.3"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", got)
	}
}

func TestRun_FileVerMix_Mismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"foo","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"patch", pkg, "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected version mismatch")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "version mismatch:") {
		t.Errorf("got: %q", msg)
	}
	if !strings.Contains(msg, "<argv>") {
		t.Errorf("VER-origin entry should be labeled <argv>: %q", msg)
	}
}

func TestRun_WriteRequiresFile(t *testing.T) {
	t.Parallel()
	// --write with only VER inputs is rejected. Validation happens
	// before stdout is touched so error path is side-effect free.
	var stdout bytes.Buffer
	err := run([]string{"patch", "1.2.3", "--write"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for --write without FILE")
	}
	if !strings.Contains(err.Error(), "FILE") {
		t.Errorf("error should mention FILE: %q", err.Error())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on validation failure, got: %q", stdout.String())
	}
}

func TestRun_MismatchErrorAlignment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json") // uses $.version path
	b := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(a, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("1.2.5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"patch", a, b, "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	msg := err.Error()
	// All three values must appear.
	for _, v := range []string{"1.2.3", "1.2.4", "1.2.5"} {
		if !strings.Contains(msg, v) {
			t.Errorf("missing value %q in error: %q", v, msg)
		}
	}
	// The `=` should be column-aligned: every line containing `=` must
	// have the `=` at the same column. Lines other than the header
	// start with two spaces.
	lines := strings.Split(msg, "\n")
	col := -1
	for i, ln := range lines {
		if i == 0 {
			continue // header line
		}
		idx := strings.Index(ln, " = ")
		if idx < 0 {
			t.Errorf("line missing ' = ': %q", ln)
			continue
		}
		if col < 0 {
			col = idx
		} else if col != idx {
			t.Errorf("alignment broken: expected '=' at col %d, got %d in line %q", col, idx, ln)
		}
	}
}

// --- compare ----------------------------------------------------------------

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

// TestRun_HelpDispatch covers v0.13.0 の 3 段 help 体系 (short / full /
// per-action) が run() 経由で正しく異なる出力を返すことを確認。文字列
// 完全一致ではなく「各 help が固有の sentinel 句を含むか」で識別する
// (将来の文言変更で fragile にならないように)。
func TestRun_HelpDispatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		argv        []string
		mustContain string // 出力にこの substring が含まれていれば pass
		mustExclude string // 出力にこの substring が含まれていれば fail (空文字なら無視)
	}{
		// short help: コンパクトな overview。Actions 一覧 + --help-full 案内
		{"short-help-flag", []string{"--help"}, "Action-specific help: bump-semver <action> --help", "Supported file formats"},
		{"short-help-h", []string{"-h"}, "Full reference:       bump-semver --help-full", ""},
		{"short-help-empty", []string{}, "Action-specific help: bump-semver <action> --help", ""},
		// full help: Supported file formats 表 + 全 Examples
		{"full-help", []string{"--help-full"}, "Supported file formats (auto-detected by basename)", "Action-specific help: bump-semver <action> --help"},
		// per-action helps: それぞれ固有の見出しを持つ
		{"action-help-major", []string{"major", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-minor", []string{"minor", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-patch", []string{"patch", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-pre", []string{"pre", "--help"}, "bump-semver pre — manage pre-release identifiers", ""},
		{"action-help-get", []string{"get", "--help"}, "bump-semver get — print the current version", ""},
		{"action-help-compare", []string{"compare", "--help"}, "bump-semver compare — compare two SemVer values", ""},
		{"action-help-compare-with-op", []string{"compare", "eq", "--help"}, "bump-semver compare — compare two SemVer values", ""},
		{"action-help-compare-with-precision-op", []string{"compare", "eq-major", "--help"}, "bump-semver compare — compare two SemVer values", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			if err := run(tc.argv, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
				t.Fatalf("run(%v) error: %v", tc.argv, err)
			}
			out := stdout.String()
			if !strings.Contains(out, tc.mustContain) {
				t.Errorf("run(%v) output does not contain %q\ngot:\n%s", tc.argv, tc.mustContain, out)
			}
			if tc.mustExclude != "" && strings.Contains(out, tc.mustExclude) {
				t.Errorf("run(%v) output should not contain %q (wrong help variant)", tc.argv, tc.mustExclude)
			}
		})
	}
}

// TestRun_BumpSemverVcsEnvIgnored は DR-0016 で削除された
// BUMP_SEMVER_VCS 環境変数が誤って再導入された場合の regression を
// 防ぐ。env を設定しても detectVcs / parseVcsOverride が一切影響を
// 受けないことを assert する。
func TestRun_BumpSemverVcsEnvIgnored(t *testing.T) {
	// t.Setenv は test cleanup で復元される
	t.Setenv("BUMP_SEMVER_VCS", "git")
	// parseVcsOverride: 空文字 / "auto" は env を見ずに vcsAuto を返す
	if got, err := parseVcsOverride(""); err != nil || got != vcsAuto {
		t.Errorf("parseVcsOverride(\"\") = (%v, %v), want (vcsAuto, nil)", got, err)
	}
	if got, err := parseVcsOverride("auto"); err != nil || got != vcsAuto {
		t.Errorf("parseVcsOverride(\"auto\") = (%v, %v), want (vcsAuto, nil)", got, err)
	}
	// 明示的 jj / git は env と無関係に解決される
	if got, err := parseVcsOverride("jj"); err != nil || got != vcsJj {
		t.Errorf("parseVcsOverride(\"jj\") = (%v, %v), want (vcsJj, nil)", got, err)
	}
	if got, err := parseVcsOverride("git"); err != nil || got != vcsGit {
		t.Errorf("parseVcsOverride(\"git\") = (%v, %v), want (vcsGit, nil)", got, err)
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

// Sanity check: a plain semver-layer error returned from run() carries
// exit code 2 (not 1, which is reserved for compare predicate-false).
// Phase 5 changed run() to always return *exitErr; this test now
// verifies the carried code rather than the absence of *exitErr.
func TestRun_BumpError_ExitCodeIs2(t *testing.T) {
	t.Parallel()
	err := run([]string{"patch", "garbage"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != 2 {
		t.Errorf("bump errors should map to exit 2 (got %d): %v", ee.code, err)
	}
}

// --- Phase 5: hint + quiet flags --------------------------------------------

// hint is printed to stderr when bumping at least one FILE without --write.
func TestRun_Hint_DefaultBumpWithFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", got)
	}
	if !strings.Contains(stderr.String(), "hint: 1 file not modified") {
		t.Errorf("stderr should contain hint, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--write") || !strings.Contains(stderr.String(), "--no-hint") {
		t.Errorf("hint should mention --write and --no-hint, got: %q", stderr.String())
	}
}

// --no-hint suppresses only the hint; stdout is unaffected.
func TestRun_Hint_NoHintFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.4" {
		t.Errorf("stdout = %q, want 1.2.4", got)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("stderr should be hint-free, got: %q", stderr.String())
	}
}

// With --write the action actually modifies files, so no "not modified"
// hint is appropriate.
func TestRun_Hint_NotShownWithWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("stderr should not contain a hint when --write is given, got: %q", stderr.String())
	}
}

// VER-only inputs never had a file to modify; no hint.
func TestRun_Hint_NotShownForVerOnly(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", "1.2.3"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("VER-only bump must not print hint, got: %q", stderr.String())
	}
}

// `get` is read-only; never has a "not modified" outcome.
func TestRun_Hint_NotShownForGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("get must not print hint, got: %q", stderr.String())
	}
}

// Singular "1 file" vs plural "N files".
func TestRun_Hint_FileCountSingular(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "1 file not modified") {
		t.Errorf("expected '1 file not modified' (singular), got: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "1 files") {
		t.Errorf("singular case should not say '1 files', got: %q", stderr.String())
	}
}

func TestRun_Hint_FileCountPlural(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	c := filepath.Join(dir, "c.json")
	for _, p := range []string{a, b, c} {
		if err := os.WriteFile(p, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", a, b, c}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(stderr.String(), "3 files not modified") {
		t.Errorf("expected '3 files not modified' (plural), got: %q", stderr.String())
	}
}

// VER inputs interleaved with FILE inputs are not counted in the hint.
func TestRun_Hint_FileCountIgnoresVerArgs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	if err := os.WriteFile(a, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", a, "1.2.3"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	// Two inputs, but only one of them is a FILE — hint says "1 file".
	if !strings.Contains(stderr.String(), "1 file not modified") {
		t.Errorf("expected '1 file not modified' (FILE count only), got: %q", stderr.String())
	}
}

// -q suppresses stdout.
func TestRun_Quiet_SuppressesStdout(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", "1.2.3", "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected stdout suppressed, got: %q", stdout.String())
	}
}

// -q also suppresses the hint.
func TestRun_Quiet_SuppressesHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("-q must suppress stdout, got: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "hint:") {
		t.Errorf("-q must suppress the hint, got: %q", stderr.String())
	}
}

// --quiet long form behaves identically to -q.
func TestRun_Quiet_LongFlag(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", "1.2.3", "--quiet"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("--quiet must suppress stdout, got: %q", stdout.String())
	}
}

// -qq suppresses error output (e.g. mismatch errors) on stderr but
// still propagates the non-zero exit code.
func TestRun_QuietAll_SuppressesError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"name":"x","version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"name":"x","version":"1.2.4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"patch", a, b, "-qq"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Fatalf("expected *exitErr with code=2, got %v (code=%d)", err, ee.code)
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq must suppress stderr, got: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("-qq must suppress stdout, got: %q", stdout.String())
	}
}

// --quiet-all long form behaves identically to -qq.
func TestRun_QuietAll_LongFlag(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"patch", "garbage", "--quiet-all"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if stderr.Len() != 0 {
		t.Errorf("--quiet-all must suppress stderr, got: %q", stderr.String())
	}
}

// -q with `get` suppresses stdout — the documented batch-validation use
// case (exit code is meaningful, value is not).
func TestRun_GetQuiet_BatchValidation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("get -q must suppress stdout, got: %q", stdout.String())
	}
}

// `get -qq` on an invalid file: error suppressed, exit code preserved.
func TestRun_GetQuiet_InvalidFile(t *testing.T) {
	t.Parallel()
	// Without -qq: error message reaches stderr.
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", "nonexistent-file-zzz"}, bytes.NewReader(nil), &stdout, &stderr); err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if stderr.Len() == 0 {
		t.Errorf("default get on bad input should print to stderr")
	}

	// With -qq: stderr stays empty, exit code is 2.
	var stdout2, stderr2 bytes.Buffer
	err := run([]string{"get", "nonexistent-file-zzz", "-qq"}, bytes.NewReader(nil), &stdout2, &stderr2)
	if err == nil {
		t.Fatal("expected error for nonexistent file with -qq")
	}
	if stderr2.Len() != 0 {
		t.Errorf("-qq must suppress stderr, got: %q", stderr2.String())
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exit code 2, got %v", err)
	}
}

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

// Predicate-false compare keeps exit-1 behavior (no stderr regardless).
func TestRun_Compare_QuietFlagsNoOpForPredicateFalse(t *testing.T) {
	t.Parallel()
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
	if stderr.Len() != 0 || stdout.Len() != 0 {
		t.Errorf("compare predicate-false should be silent, got stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}

// --no-hint with `get` is silently accepted (no-op, since get never
// emits a hint).
func TestRun_NoHint_NoOpForGet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", path, "--no-hint"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
}

// -q and -qq together collapse to -qq (silently accepted, no error).
func TestRun_QuietAndQuietAll_CollapseToQuietAll(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"patch", "garbage", "-q", "-qq"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq dominates: stderr should be suppressed, got: %q", stderr.String())
	}
}

// --no-hint with -q is silently accepted (-q already suppresses hint).
func TestRun_NoHint_AndQuiet_NoConflict(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(path, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"patch", path, "--no-hint", "-q"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("expected silence, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// --- DR-0007: --json output -------------------------------------------------

// decodeJSON is a tiny helper so each --json test reads as
// "run, decode, assert" without re-stating the Unmarshal call.
func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("JSON output must end with a single newline, got: %q", s)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(s, "\n")), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v (input=%q)", err, s)
	}
	return got
}

// jsonField returns the JSON field as expected by the schema. A test
// that calls this with a missing key fails — every DR-0007 field
// (including the nullable ones) is required to be present.
func jsonField(t *testing.T, m map[string]any, key string) any {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("missing key %q in JSON output: %v", key, m)
	}
	return v
}

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

// parseArgs recognizes all the new quiet/no-hint flag spellings.
func TestParseArgs_QuietFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		argv []string
		want cliArgs
	}{
		{
			"no-hint",
			[]string{"patch", "1.2.3", "--no-hint"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, noHint: true},
		},
		{
			"quiet-short",
			[]string{"patch", "1.2.3", "-q"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, quiet: true},
		},
		{
			"quiet-long",
			[]string{"patch", "1.2.3", "--quiet"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, quiet: true},
		},
		{
			"quiet-all-short",
			[]string{"patch", "1.2.3", "-qq"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, quietAll: true},
		},
		{
			"quiet-all-long",
			[]string{"patch", "1.2.3", "--quiet-all"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, quietAll: true},
		},
		{
			"compare-with-quiet",
			[]string{"compare", "eq", "1.2.3", "1.2.3", "-qq"},
			cliArgs{kind: "compare", compareOp: "eq", inputs: []string{"1.2.3", "1.2.3"}, quietAll: true},
		},
		{
			"get-with-quiet",
			[]string{"get", "VERSION", "-q"},
			cliArgs{kind: "bump", action: "get", inputs: []string{"VERSION"}, quiet: true},
		},
		{
			"q-and-qq-coexist",
			[]string{"patch", "1.2.3", "-q", "-qq"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, quiet: true, quietAll: true},
		},
		{
			"no-hint-and-quiet-coexist",
			[]string{"patch", "1.2.3", "--no-hint", "-q"},
			cliArgs{kind: "bump", action: "patch", inputs: []string{"1.2.3"}, noHint: true, quiet: true},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(tc.argv)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", tc.argv, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseArgs(%v)\n  got = %+v\n  want= %+v", tc.argv, got, tc.want)
			}
		})
	}
}

// --- DR-0008: vcs: input mode ----------------------------------------------
//
// These tests exercise the CLI from end to end against a real git
// fixture. They cannot run with t.Parallel() because they chdir(2) the
// process. jj-flavoured CLI tests would need ssh-agent / signing
// disabled to run hermetically; we cover jj at the unit-test layer
// (vcs_test.go) and stick with git here for CLI round-tripping.

// Compare against vcs:HEAD~1 (which holds the pre-bump 0.0.1) — current
// version 1.2.3 should be greater.
func TestRun_VcsInput_Simple(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Working tree (1.2.3) > HEAD~1 (0.0.1).
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1:VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true, got: %v", err)
		}
	})
}

// File borrowing: the second argument has no FILE component, so the
// path is taken from the first FILE-origin sibling (`VERSION`).
func TestRun_VcsInput_FileBorrow(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true (borrow VERSION), got: %v", err)
		}
	})
}

// `vcs:latest-tag()` resolves through the function path; v1.0.0 < 1.2.3.
func TestRun_VcsInput_LatestTag(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0"}, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:latest-tag()"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected current > latest tag, got: %v", err)
		}
	})
}

// `vcs:latest-tag()` errors when no tag parses — actionable error.
func TestRun_VcsInput_LatestTag_NoSemver(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"build-stamp"}, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "VERSION", "vcs:latest-tag()"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error when no semver tags")
		}
		if !strings.Contains(stderr.String(), "no semver-compatible tags") {
			t.Errorf("stderr should mention 'no semver-compatible tags', got: %q", stderr.String())
		}
	})
}

// --write with vcs: input is rejected before any side effects.
func TestRun_VcsInput_WriteRejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"patch", "VERSION", "vcs:HEAD~1", "--write"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --write + vcs: rejection")
		}
		if !strings.Contains(stderr.String(), "--write cannot be used with vcs: inputs") {
			t.Errorf("stderr should mention the rejection reason, got: %q", stderr.String())
		}
	})
}

// --write with cmd: input is rejected (same policy as vcs:, both are
// read-only schemas resolved from external sources without a writable
// backing file).
func TestRun_CmdInput_WriteRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"patch", "VERSION", "cmd:echo 1.2.3", "--write"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --write + cmd: rejection")
		}
		if !strings.Contains(stderr.String(), "--write cannot be used with cmd: inputs") {
			t.Errorf("stderr should mention the rejection reason, got: %q", stderr.String())
		}
	})
}

// --vcs git forces the override even though the directory is also
// suitable for jj. We only have a git fixture here, so this primarily
// tests the flag parsing + override propagation path.
func TestRun_VcsInput_VcsForceFlag(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1", "--vcs", "git"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true with --vcs git, got: %v", err)
		}
	})
}

// --vcs with an invalid value is a parse-time error.
func TestRun_VcsInput_InvalidVcsValue(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"compare", "gt", "1.2.3", "1.2.4", "--vcs", "hg"},
		bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected invalid --vcs value to error")
	}
	if !strings.Contains(stderr.String(), "invalid --vcs value") {
		t.Errorf("stderr should mention the invalid value, got: %q", stderr.String())
	}
}

// `vcs:HEAD~1` (no FILE) with no FILE-origin sibling to borrow from is
// an error — the user has to supply `vcs:HEAD~1:path` or pair it with
// a real file.
func TestRun_VcsInput_BorrowRequired(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "1.2.3", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error when no FILE to borrow from")
		}
		if !strings.Contains(stderr.String(), "file is required") {
			t.Errorf("stderr should mention borrow failure, got: %q", stderr.String())
		}
	})
}

// Unknown function names produce a clean error rather than reaching the
// VCS subprocess.
func TestRun_VcsInput_UnknownFunction(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"compare", "eq", "1.2.3", "vcs:current-branch()", "--vcs", "git"},
		bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected unknown function error")
	}
	if !strings.Contains(stderr.String(), "unknown vcs function") {
		t.Errorf("stderr should mention unknown function, got: %q", stderr.String())
	}
}

// Multiple vcs: inputs are allowed and pass through allSameValue. We
// only have one commit at HEAD~1 (0.0.1) and another at HEAD (1.2.3),
// so two `vcs:HEAD~1:VERSION` references should agree with each other
// and with a same-valued VER input.
func TestRun_VcsInput_MultipleVcs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Three inputs all evaluating to 0.0.1: get should succeed.
		err := run([]string{"get", "0.0.1", "vcs:HEAD~1:VERSION", "vcs:HEAD~1:VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("multiple vcs: inputs should agree, got: %v", err)
		}
	})
}

// Borrow source picked from `vcs:REV:FILE`: when only vcs: inputs are
// present and one of them names a file explicitly, downstream
// file-omitted vcs: inputs borrow from it.
func TestRun_VcsInput_BorrowFromVcsExplicit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Both args resolve via vcs:; the second borrows VERSION from
		// the first. Both refer to HEAD so they should equal.
		err := run([]string{"compare", "eq", "vcs:HEAD:VERSION", "vcs:HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected eq true (vcs:HEAD borrowed VERSION), got: %v", err)
		}
	})
}

// All-vcs-with-no-file is an error: nothing to borrow from.
func TestRun_VcsInput_AllVcsNoFile(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "vcs:HEAD", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error: no file to borrow")
		}
		if !strings.Contains(stderr.String(), "file is required") {
			t.Errorf("stderr should mention borrow failure, got: %q", stderr.String())
		}
	})
}

// Position-order: the *first* FILE-origin input wins as borrow target,
// even when later args also have FILEs. We verify by building a repo
// with two siblings (`a.json` agrees with HEAD, `b.json` doesn't) and
// passing them in different orders.
func TestRun_VcsInput_BorrowPositionOrder(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Add a second file (a.json) at HEAD with a *different*
		// version. The vcs:HEAD argument with no file should borrow
		// from VERSION (the leftmost FILE-origin), so the comparison
		// against the literal 1.2.3 should pass.
		if err := os.WriteFile(filepath.Join(dir, "a.json"),
			[]byte(`{"version":"9.9.9"}`), 0644); err != nil {
			t.Fatal(err)
		}
		// VERSION (=1.2.3) is the leftmost FILE; vcs:HEAD borrows it.
		// Order: VERSION, a.json, vcs:HEAD — but compare takes only 2
		// inputs. Use `get` (which is variadic) to test the borrow.
		err := run([]string{"get", "VERSION", "1.2.3", "vcs:HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected agree on 1.2.3 (borrow VERSION), got: %v", err)
		}
	})
}

// TestRun_VersionPlain は --version で生の文字列が stdout に出ることを確認。
// version グローバルは ldflags 注入で、テスト時は default "dev"。
func TestRun_VersionPlain(t *testing.T) {
	// version グローバルを差し替えるため t.Parallel() しない
	orig := version
	t.Cleanup(func() { version = orig })
	version = "v1.2.3"

	var stdout bytes.Buffer
	if err := run([]string{"--version"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := stdout.String(), "v1.2.3\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRun_VersionJSON は --version --json で構造化 JSON が出ることを確認。
func TestRun_VersionJSON(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })
	version = "v1.2.3-rc.1+build.42"

	var stdout bytes.Buffer
	if err := run([]string{"--version", "--json"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	if got == "" || got[len(got)-1] != '\n' {
		t.Errorf("expected JSON line with trailing newline, got %q", got)
	}
	var out jsonOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, got)
	}
	if out.Version != "v1.2.3-rc.1+build.42" {
		t.Errorf("Version: got %q, want %q", out.Version, "v1.2.3-rc.1+build.42")
	}
	if out.Semver != "1.2.3-rc.1+build.42" {
		t.Errorf("Semver: got %q, want %q", out.Semver, "1.2.3-rc.1+build.42")
	}
	if out.Major != 1 || out.Minor != 2 || out.Patch != 3 {
		t.Errorf("Major/Minor/Patch: got %d/%d/%d, want 1/2/3", out.Major, out.Minor, out.Patch)
	}
	if out.Pre == nil || *out.Pre != "rc.1" {
		t.Errorf("Pre: got %v, want %q", out.Pre, "rc.1")
	}
	if out.BuildMetadata == nil || *out.BuildMetadata != "build.42" {
		t.Errorf("BuildMetadata: got %v, want %q", out.BuildMetadata, "build.42")
	}
}

// TestRun_VersionJSON_InvalidVersion: ldflags 無しでビルドされたバイナリ
// (version = "dev") が --version --json を呼ばれた場合、エラーで exit 2。
func TestRun_VersionJSON_InvalidVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })
	version = "dev"

	var stderr bytes.Buffer
	err := run([]string{"--version", "--json"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatalf("expected error for invalid version 'dev', got nil")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitErr code=2, got %v", err)
	}
}

// --- DR-0020 `vcs` subcommand integration tests --------------------------

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

// TestRun_VcsGet_NoArgs: `vcs get` with no key shows the vcs-get help.
func TestRun_VcsGet_NoArgs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "get"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs get (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "root") || !strings.Contains(stdout.String(), "backend") {
		t.Errorf("expected vcs get help mentioning root/backend, got: %q", stdout.String())
	}
}

// TestRun_VcsGet_UnknownKey: an unknown key is a usage error (exit 2)
// and the error names the available keys.
func TestRun_VcsGet_UnknownKey(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown key")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "root") {
			t.Errorf("error should mention available keys, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsGet_Backend_Git: prints "git" on a git-only repo.
func TestRun_VcsGet_Backend_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Backend_Jj: prints "jj" on a colocated git+jj repo
// (jj wins over git per DR-0008 precedence).
func TestRun_VcsGet_Backend_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "jj" {
			t.Errorf("backend = %q, want jj", got)
		}
	})
}

// TestRun_VcsGet_Root_Git: prints the repo root path.
func TestRun_VcsGet_Root_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "root"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get root: %v", err)
		}
		got := strings.TrimSpace(stdout.String())
		if got == "" {
			t.Errorf("Root should be non-empty, got empty string")
		}
		// Compare via EvalSymlinks because macOS /var/folders symlinks
		// through to /private/var.
		gotCanon, _ := filepath.EvalSymlinks(got)
		wantCanon, _ := filepath.EvalSymlinks(dir)
		if gotCanon != wantCanon {
			t.Errorf("root = %q (canon %q), want %q", got, gotCanon, wantCanon)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Git: prints "main" for the fixture.
func TestRun_VcsGet_CurrentBranch_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get current-branch: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "main" {
			t.Errorf("current-branch = %q, want main", got)
		}
	})
}

// TestRun_VcsGet_CurrentBranch_Detached: detached HEAD returns exit 4
// (exitCodeAmbiguous), not the standard exit 2 (usage).
func TestRun_VcsGet_CurrentBranch_Detached(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	runIn(t, dir, "git", "checkout", "--detach", "HEAD")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "current-branch"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error on detached HEAD")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeAmbiguous {
			t.Errorf("expected exit %d (ambiguous), got: %v", exitCodeAmbiguous, err)
		}
	})
}

// TestRun_VcsGet_Backend_VcsOverride: --vcs git on a colocated repo
// forces the git backend (was jj otherwise).
func TestRun_VcsGet_Backend_VcsOverride(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "--vcs", "git"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend --vcs git: %v", err)
		}
		if got := strings.TrimSpace(stdout.String()); got != "git" {
			t.Errorf("backend (--vcs git) = %q, want git", got)
		}
	})
}

// TestRun_VcsGet_Quiet: -q suppresses the stdout value but the command
// still exits 0.
func TestRun_VcsGet_Quiet(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "backend", "-q"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get backend -q: %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -q, got: %q", got)
		}
	})
}

// TestRun_VcsGet_NoRepo: outside a vcs repo, `vcs get backend` should
// report exit 3 (VCS exec / not-a-repo) — distinct from the get's own
// usage errors.
func TestRun_VcsGet_NoRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "get", "backend"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// --- DR-0020 PR-2: `vcs is` integration tests -----------------------------

// TestRun_VcsIs_NoArgs: `vcs is` with no predicate shows the vcs-is help
// (matches the no-args == --help convention used by `vcs get`).
func TestRun_VcsIs_NoArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "is"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs is (no args): %v", err)
		}
		if !strings.Contains(stdout.String(), "clean") || !strings.Contains(stdout.String(), "dirty") {
			t.Errorf("expected vcs is help mentioning clean/dirty, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsIs_UnknownPred: an unknown predicate is a usage error (exit 2)
// — DR-0020 explicitly forbids silent-false on typos to prevent misroutes.
func TestRun_VcsIs_UnknownPred(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "wibble"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for unknown predicate")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "clean") {
			t.Errorf("error should mention available predicates, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsIs_Clean_True: a fresh git fixture is clean → exit 0.
func TestRun_VcsIs_Clean_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for clean repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Clean_False: a tracked-modification renders the repo
// dirty → `vcs is clean` exits 1, with NO stderr (predicate-false is
// silent, matching compare).
func TestRun_VcsIs_Clean_False(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected exit 1 on dirty repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("predicate-false should be silent on stderr, got: %q", got)
		}
	})
}

// TestRun_VcsIs_Dirty_True: dirty repo → `vcs is dirty` exits 0.
func TestRun_VcsIs_Dirty_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "9.9.9\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is dirty` on dirty repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Dirty_False: clean repo → `vcs is dirty` exits 1.
func TestRun_VcsIs_Dirty_False(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is dirty` on clean repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Git_True: a git-only fixture → `vcs is git` exits 0.
func TestRun_VcsIs_Git_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is git` on git repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Jj_False_OnGit: a git-only fixture → `vcs is jj` exits 1.
func TestRun_VcsIs_Jj_False_OnGit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "jj"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is jj` on git-only repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Jj_True: colocated git+jj → `vcs is jj` exits 0 (jj wins
// over git in the auto-probe, matching DR-0008 precedence).
func TestRun_VcsIs_Jj_True(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "jj"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is jj` on colocated repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_Git_False_OnColocated: a colocated repo resolves to jj
// in auto-probe, so `vcs is git` exits 1 (matches `vcs get backend`).
func TestRun_VcsIs_Git_False_OnColocated(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected exit 1 for `is git` on colocated repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsIs_Git_True_WithOverride: --vcs git on a colocated repo
// forces the git branch, so `vcs is git` exits 0.
func TestRun_VcsIs_Git_True_WithOverride(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "is", "git", "--vcs", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for `is git --vcs git` on colocated repo, got: %v", err)
		}
	})
}

// TestRun_VcsIs_NoRepo_Clean: outside a vcs repo `vcs is clean` reports
// exit 3 (can't tell the answer — distinct from "answer is false").
func TestRun_VcsIs_NoRepo_Clean(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsIs_NoRepo_Git: outside a vcs repo `vcs is git` reports exit
// 3 (can't tell), NOT exit 1 — distinguishes "not git" from "no answer".
// DR-0020: "曖昧・期待外はエラー".
func TestRun_VcsIs_NoRepo_Git(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "git"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsIs_TooManyArgs: `vcs is clean dirty` → usage error (exit 2).
func TestRun_VcsIs_TooManyArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "is", "clean", "dirty"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for multiple predicates")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// --- DR-0020 PR-3: `vcs diff` integration tests ---------------------------

// TestRun_VcsDiff_NoArgs: `vcs diff` with no REV shows the vcs-diff help
// (matches the no-args convention used by `vcs get` and `vcs is`).
func TestRun_VcsDiff_NoArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff (no args): %v", err)
		}
		if !strings.Contains(stdout.String(), "REV") {
			t.Errorf("expected vcs diff help mentioning REV, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_HasDiff: `vcs diff HEAD~1` on the fixture prints a
// patch covering VERSION on stdout and exits 0.
func TestRun_VcsDiff_Git_HasDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected stdout to include VERSION patch, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_NoDiff: `vcs diff HEAD` on a clean fixture produces
// no stdout, exits 0.
func TestRun_VcsDiff_Git_NoDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD: %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (no diff), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Git_PathFilter: `vcs diff REV VERSION nope.txt` returns
// the VERSION diff and silently ignores the nonexistent path.
func TestRun_VcsDiff_Git_PathFilter(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "VERSION", "nope.txt"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff HEAD~1 VERSION nope.txt: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected VERSION in diff, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Git_AllPathsNonexistent: every path filtered out → empty
// stdout, exit 0. Must NOT fall through to "diff everything".
func TestRun_VcsDiff_Git_AllPathsNonexistent(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD~1", "nope.txt", "alsonope.txt"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff (all-nonexistent): %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (all paths filtered), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Git_BadRev: unresolvable REV → exit 3 (VCS exec).
func TestRun_VcsDiff_Git_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "doesnotexist"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for nonexistent rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsDiff_Jj_HasDiff: `vcs diff @--` on a jj fixture prints the
// bump diff (VERSION) and exits 0.
func TestRun_VcsDiff_Jj_HasDiff(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "@--"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff @--: %v", err)
		}
		if !strings.Contains(stdout.String(), "VERSION") {
			t.Errorf("expected stdout to include VERSION patch, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NoRepo: outside a vcs repo → exit 3.
func TestRun_VcsDiff_NoRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "HEAD"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside a vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// --- DR-0020 PR-3.1: vcs diff -s / -q tests ------------------------------

// TestRun_VcsDiff_NameStatus_Git: `vcs diff -s HEAD~1` prints
// tab-separated M/A/D lines (git-native format) and exits 0.
func TestRun_VcsDiff_NameStatus_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -s HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected 'M\\tVERSION' in stdout, got: %q", stdout.String())
		}
		// Crucially: stdout must NOT contain raw patch text (unified diff hunks).
		if strings.Contains(stdout.String(), "@@") {
			t.Errorf("name-status output should not contain patch hunks, got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NameStatus_LongOption: --name-status equivalent to -s.
func TestRun_VcsDiff_NameStatus_LongOption(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "--name-status", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff --name-status HEAD~1: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected 'M\\tVERSION', got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_NameStatus_Jj: jj backend produces tab-normalized output.
func TestRun_VcsDiff_NameStatus_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "@--"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -s @--: %v", err)
		}
		if !strings.Contains(stdout.String(), "M\tVERSION") {
			t.Errorf("expected tab-normalized 'M\\tVERSION', got: %q", stdout.String())
		}
	})
}

// TestRun_VcsDiff_Quiet_HasChanges_ExitsFalse: -q with diff present →
// stdout empty, exit code 1 (predicate-false), no error message.
func TestRun_VcsDiff_Quiet_HasChanges_ExitsFalse(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "HEAD~1"}, bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatal("expected exitCodeFalse error for diff present")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (false), got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -q, got: %q", got)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("stderr should be empty (silent predicate), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Quiet_NoChanges_ExitsZero: -q with no diff → exit 0.
func TestRun_VcsDiff_Quiet_NoChanges_ExitsZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "HEAD"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs diff -q HEAD (no diff): expected nil, got %v", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty, got: %q", got)
		}
	})
}

// TestRun_VcsDiff_QuietLong_HasChanges: --quiet alias works the same way.
func TestRun_VcsDiff_QuietLong_HasChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "--quiet", "HEAD~1"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsDiff_QuietAll_HasChanges: -qq also reflects presence via
// exit code. stderr is suppressed even for error paths.
func TestRun_VcsDiff_QuietAll_HasChanges(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout, stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-qq", "HEAD~1"}, bytes.NewReader(nil), &stdout, &stderr)
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty with -qq, got: %q", got)
		}
	})
}

// TestRun_VcsDiff_NameStatusAndQuiet_QuietWins: `-s -q` → -q wins;
// stdout empty, exit reflects presence (1 = has diff).
func TestRun_VcsDiff_NameStatusAndQuiet_QuietWins(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "diff", "-s", "-q", "HEAD~1"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("stdout should be empty (-q overrides -s), got: %q", got)
		}
	})
}

// TestRun_VcsDiff_Quiet_BadRev: -q + bad REV → exit 3 (VCS exec), not 1.
// Distinguishing exec failure from predicate-false is required by DR-0020.
func TestRun_VcsDiff_Quiet_BadRev(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "diff", "-q", "doesnotexist"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error for bad rev")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsDiff_Quiet_Jj_HasChanges: jj backend also surfaces diff
// presence via exit 1.
func TestRun_VcsDiff_Quiet_Jj_HasChanges(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "-q", "@--"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d (jj has diff), got: %v", exitCodeFalse, err)
		}
	})
}

// TestRun_VcsDiff_Quiet_AllPathsNonexistent_ExitsZero: every path
// filtered → empty diff → exit 0 (matches "no diff" branch).
func TestRun_VcsDiff_Quiet_AllPathsNonexistent_ExitsZero(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "diff", "-q", "HEAD~1", "nope.txt"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("expected nil (all paths filtered → no diff), got: %v", err)
		}
	})
}

// --- v0.20.2 bugfix: verb-aware flag rejection ---------------------------
//
// PR-3.1 (v0.20.1) introduced `-s/--name-status` for `vcs diff` but the
// shared parser also silently accepted it for `vcs get` / `vcs is` (no-op).
// This violates kawaz CLI design (rules/cli-design-preferences.md: unknown
// flags must exit 2 with a usage hint, so typos are caught). The fix gates
// `-s/--name-status` to the `diff` verb; other verbs hit the generic
// unknown-flag rejection.

// TestRun_VcsGet_RejectNameStatusShort: `vcs get -s root` must exit 2
// (verb-local flag for diff, not valid on get).
func TestRun_VcsGet_RejectNameStatusShort(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "-s", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get -s`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("expected stderr to mention 'unknown flag', got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "-s") {
		t.Errorf("expected stderr to name the offending flag '-s', got: %q", stderr.String())
	}
}

// TestRun_VcsGet_RejectNameStatusLong: long form must also exit 2.
func TestRun_VcsGet_RejectNameStatusLong(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--name-status", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --name-status`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--name-status") {
		t.Errorf("expected stderr to name '--name-status', got: %q", stderr.String())
	}
}

// TestRun_VcsIs_RejectNameStatusShort: same for `vcs is`.
func TestRun_VcsIs_RejectNameStatusShort(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "is", "-s", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs is -s`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
}

// TestRun_VcsIs_RejectNameStatusLong: long form for `vcs is`.
func TestRun_VcsIs_RejectNameStatusLong(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "is", "--name-status", "clean"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs is --name-status`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
}

// TestRun_VcsGet_RejectUnknownFlag: a completely unknown flag is also
// rejected (covers the generic catch-all, not just -s/--name-status).
func TestRun_VcsGet_RejectUnknownFlag(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"vcs", "get", "--foobar", "root"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected usage error for `vcs get --foobar`")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
	}
	if !strings.Contains(stderr.String(), "--foobar") {
		t.Errorf("expected stderr to name '--foobar', got: %q", stderr.String())
	}
}

// --- DR-0020 PR-4: `vcs commit` integration tests ------------------------
//
// The verb has three modes — `-m MSG PATH..` (path-scoped), `--staged -m
// MSG` (commit-all), and `--amend [-m MSG]` (fold into previous). Each
// has its own correctness story; the run-level tests below pin the
// usage-error matrix (-a rejection, mode exclusivity, dynamic hint), and
// the happy path on a real fixture per backend. Backend-level commit
// semantics are exercised in vcs_backend_test.go.

// TestRun_VcsCommit_NoArgs: `vcs commit` with no args shows the help.
func TestRun_VcsCommit_NoArgs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "commit"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs commit (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "commit") || !strings.Contains(stdout.String(), "--staged") {
		t.Errorf("expected vcs commit help mentioning commit/--staged, got: %q", stdout.String())
	}
}

// TestRun_VcsCommit_NoMessage_NoAmend: `-m` is required unless --amend.
// Missing message (and no --amend) → exit 2.
func TestRun_VcsCommit_NoMessage_NoAmend(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "VERSION"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error when -m is missing")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_DashA_Rejected: `-a` is intentionally not supported.
// DR-0020 makes this an opinionated safety rejection — exit 2 with a
// hint that names the supported modes (--staged / PATH).
func TestRun_VcsCommit_DashA_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-a", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected -a to be rejected")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "--staged") && !strings.Contains(stderr.String(), "PATH") {
			t.Errorf("error should hint at --staged / PATH, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_PathAndStaged_Rejected: path + --staged is ambiguous,
// must reject with exit 2.
func TestRun_VcsCommit_PathAndStaged_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "--staged", "-m", "x", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --staged + PATH to reject")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_NoMode_DynamicHint_Git: when no PATH and no --staged
// and no --amend, the error hint must come from backend.Kind(); git tells
// the user to use --staged or pass a PATH.
func TestRun_VcsCommit_NoMode_DynamicHint_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for no mode")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "--staged") {
			t.Errorf("git hint should mention --staged, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_NoMode_DynamicHint_Jj: jj's hint must explicitly say
// that `-a` is not supported (kawaz CLI design safety).
func TestRun_VcsCommit_NoMode_DynamicHint_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected usage error for no mode")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		// jj users typically reach for `-a`; the hint should explicitly
		// say it's not supported and tell them to name a PATH.
		if !strings.Contains(stderr.String(), "PATH") {
			t.Errorf("jj hint should mention PATH, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_Paths_Git: end-to-end happy path on git — modify
// VERSION, run `vcs commit -m MSG VERSION`, HEAD advances and worktree
// is clean.
func TestRun_VcsCommit_Paths_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "-m", "bump", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit -m bump VERSION: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "bump" {
			t.Errorf("HEAD subject = %q, want 'bump'", got)
		}
	})
}

// TestRun_VcsCommit_Paths_NonexistentOnly_Idempotent: all-nonexistent
// PATH list → exit 0, no HEAD movement (declarative convergence).
func TestRun_VcsCommit_Paths_NonexistentOnly_Idempotent(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		before, _ := runBackendCmd("git", "rev-parse", "HEAD")
		err := run([]string{"vcs", "commit", "-m", "ghost", "no-such.txt"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected exit 0 for nonexistent-only, got: %v", err)
		}
		after, _ := runBackendCmd("git", "rev-parse", "HEAD")
		if string(before) != string(after) {
			t.Errorf("HEAD should not advance, before=%s after=%s",
				strings.TrimSpace(string(before)), strings.TrimSpace(string(after)))
		}
	})
}

// TestRun_VcsCommit_Staged_Git: --staged commits the index in one shot.
func TestRun_VcsCommit_Staged_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "--staged", "-m", "bump-all"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --staged: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "bump-all" {
			t.Errorf("HEAD subject = %q, want 'bump-all'", got)
		}
	})
}

// TestRun_VcsCommit_Amend_Git: --amend with -m updates the last commit.
func TestRun_VcsCommit_Amend_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, dir, "git", "add", "VERSION")
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "--amend", "-m", "amended"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit --amend: %v", err)
		}
		out, _ := runBackendCmd("git", "log", "-1", "--pretty=%s")
		if got := strings.TrimSpace(string(out)); got != "amended" {
			t.Errorf("HEAD subject after amend = %q, want 'amended'", got)
		}
	})
}

// TestRun_VcsCommit_Paths_Jj: jj end-to-end happy path mirrors the git
// test — modify VERSION, commit, @- now describes the commit.
func TestRun_VcsCommit_Paths_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	if err := writeFile(filepath.Join(dir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		err := run([]string{"vcs", "commit", "-m", "bump", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs commit -m bump VERSION (jj): %v", err)
		}
		out, _ := runBackendCmd("jj", "log", "-r", "@-", "--no-graph", "-T", "description.first_line()")
		if got := strings.TrimSpace(string(out)); got != "bump" {
			t.Errorf("@- description = %q, want 'bump'", got)
		}
	})
}

// TestRun_VcsCommit_Amend_WithPath_Rejected: `--amend PATH..` must
// reject with exit 2 (DR-0020 safety: silent path-ignore would be the
// 巻き込み事故 the "path 必須" philosophy guards against).
func TestRun_VcsCommit_Amend_WithPath_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "--amend", "VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --amend PATH to reject")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
		if !strings.Contains(stderr.String(), "--amend") {
			t.Errorf("error should explain the --amend grammar, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsCommit_Amend_WithStaged_Rejected: `--amend --staged` is
// rejected for symmetry (silently-accepted no-op flag would be a UX
// trap; the MVP amend grammar is `--amend [-m MSG]` only).
func TestRun_VcsCommit_Amend_WithStaged_Rejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "--amend", "--staged"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --amend --staged to reject")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit %d (usage), got: %v", exitCodeUsage, err)
		}
	})
}

// TestRun_VcsCommit_NotARepo: outside any vcs repo, `vcs commit` should
// surface exit 3 (newVcsBackend failure), consistent with get/is/diff.
func TestRun_VcsCommit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "commit", "-m", "x"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error outside vcs repo")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d (vcs exec), got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsGet_GlobalQuietStillAccepted: regression guard — global
// `-q` must still work for `vcs get` (it's a global flag, not verb-local).
func TestRun_VcsGet_GlobalQuietStillAccepted(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		var stdout bytes.Buffer
		err := run([]string{"vcs", "get", "-q", "backend"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("vcs get -q backend: %v", err)
		}
	})
}

// --- DR-0020 PR-5: vcs fetch / vcs push dispatcher tests ------------------

// TestRun_VcsFetch_DefaultOrigin: `vcs fetch` (no args) targets origin.
func TestRun_VcsFetch_DefaultOrigin(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs fetch: %v", err)
		}
	})
}

// TestRun_VcsFetch_NamedRemote: `vcs fetch <remote>` targets the given
// remote.
func TestRun_VcsFetch_NamedRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "origin"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs fetch origin: %v", err)
		}
	})
}

// TestRun_VcsFetch_NonexistentRemote: bad remote name → exit 3.
func TestRun_VcsFetch_NonexistentRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "nonexistent"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for nonexistent remote")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d, got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsFetch_TooManyArgs: `vcs fetch` accepts at most one positional.
func TestRun_VcsFetch_TooManyArgs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "fetch", "origin", "extra"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})
}

// TestRun_VcsFetch_UnknownFlag: `vcs fetch --branch X` is rejected at the
// parser layer (--branch is push-only).
func TestRun_VcsFetch_UnknownFlag(t *testing.T) {
	err := run([]string{"vcs", "fetch", "--branch", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsPush_Branch: `vcs push --branch main` pushes to origin.
func TestRun_VcsPush_Branch(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --branch main: %v", err)
		}
	})
}

// TestRun_VcsPush_BookmarkAlias: `vcs push --bookmark main` is an alias of
// `--branch main`.
func TestRun_VcsPush_BookmarkAlias(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--bookmark", "main"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --bookmark main: %v", err)
		}
	})
}

// TestRun_VcsPush_NoArgs: `vcs push` with no args shows the per-verb help
// (matches the existing `vcs commit` / `vcs diff` convention — bare verb
// = help, partial verb = error).
func TestRun_VcsPush_NoArgs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "push"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs push (no args): %v", err)
	}
	if !strings.Contains(stdout.String(), "push") || !strings.Contains(stdout.String(), "--branch") {
		t.Errorf("expected vcs push help mentioning push/--branch, got: %q", stdout.String())
	}
}

// TestRun_VcsPush_MissingName: `vcs push --remote origin` (no
// --branch/--bookmark) is a usage error — NAME is required (no auto-
// detection by design).
func TestRun_VcsPush_MissingName(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--remote", "origin"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected usage error for missing --branch/--bookmark")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeUsage {
			t.Errorf("expected exit 2, got: %v", err)
		}
	})
}

// TestRun_VcsPush_BranchAndBookmarkBothSet: setting both --branch and
// --bookmark on the same invocation is a usage error (they're aliases of
// one field, double-set rejected).
func TestRun_VcsPush_BranchAndBookmarkBothSet(t *testing.T) {
	err := run([]string{"vcs", "push", "--branch", "main", "--bookmark", "main"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --branch + --bookmark")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsPush_RemoteFlag: `--remote NAME` overrides the default origin.
func TestRun_VcsPush_RemoteFlag(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--remote", "origin"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("vcs push --branch main --remote origin: %v", err)
		}
	})
}

// TestRun_VcsPush_BadRemote: nonexistent remote → exit 3.
func TestRun_VcsPush_BadRemote(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main", "--remote", "nonexistent"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for nonexistent remote")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeVCSExec {
			t.Errorf("expected exit %d, got: %v", exitCodeVCSExec, err)
		}
	})
}

// TestRun_VcsPush_NonFastForward: divergent remote → exit 5 + hint mentions
// "diverged".
func TestRun_VcsPush_NonFastForward(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, bare := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	divergeBareViaAttacker(t, bare)
	if err := writeFile(filepath.Join(work, "local.txt"), "local\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, work, "git", "add", "local.txt")
	runIn(t, work, "git", "commit", "-qm", "local-only")
	withCwd(t, work, func() {
		var stderr bytes.Buffer
		err := run([]string{"vcs", "push", "--branch", "main"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected non-ff failure")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeNonFastForward {
			t.Errorf("expected exit %d, got: %v", exitCodeNonFastForward, err)
		}
		if !strings.Contains(stderr.String(), "diverged") {
			t.Errorf("expected 'diverged' hint in stderr, got: %q", stderr.String())
		}
	})
}

// TestRun_VcsPush_NothingToPush: idempotent success when remote already
// has it.
func TestRun_VcsPush_NothingToPush(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	work, _ := setupGitRepoWithRemote(t, nil, "1.0.0")
	preloadBareWith(t, work)
	withCwd(t, work, func() {
		err := run([]string{"vcs", "push", "--branch", "main"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("idempotent push should succeed, got: %v", err)
		}
	})
}

// TestRun_VcsPush_RejectForce: `--force` is intentionally not provided —
// any attempt is a usage error.
func TestRun_VcsPush_RejectForce(t *testing.T) {
	err := run([]string{"vcs", "push", "--branch", "main", "--force"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --force")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsPush_UnknownVerbFlag: a verb-local flag on the wrong verb
// (e.g. --tags) is rejected at the parser layer.
func TestRun_VcsPush_UnknownFlag(t *testing.T) {
	err := run([]string{"vcs", "push", "--branch", "main", "--tags"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected usage error for --tags")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit 2, got: %v", err)
	}
}

// TestRun_VcsHelp_FetchPush: `vcs --help` includes fetch / push in the
// verb list.
func TestRun_VcsHelp_FetchPush(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs --help: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "fetch") {
		t.Errorf("vcs help should mention 'fetch', got: %q", out)
	}
	if !strings.Contains(out, "push") {
		t.Errorf("vcs help should mention 'push', got: %q", out)
	}
}

// TestRun_VcsFetchHelp / TestRun_VcsPushHelp: per-verb help works.
func TestRun_VcsFetchHelp(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "fetch", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs fetch --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "fetch") {
		t.Errorf("vcs fetch help should mention 'fetch', got: %q", stdout.String())
	}
}

func TestRun_VcsPushHelp(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"vcs", "push", "--help"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("vcs push --help: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "--branch") {
		t.Errorf("vcs push help should mention --branch, got: %q", out)
	}
	if !strings.Contains(out, "--bookmark") {
		t.Errorf("vcs push help should mention --bookmark (alias), got: %q", out)
	}
}
