package main

import (
	"bytes"
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
		{"help-flag", []string{"--help"}, cliArgs{kind: "help"}},
		{"help-short", []string{"-h"}, cliArgs{kind: "help"}},
		{"empty", []string{}, cliArgs{kind: "help"}},
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
		{"pre-and-no-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--no-pre"}},
		{"build-and-no-build", []string{"patch", "1.2.3", "--build-metadata", "x", "--no-build-metadata"}},
		{"empty-pre", []string{"pre", "1.2.3", "--pre", ""}},
		{"empty-build-metadata", []string{"patch", "1.2.3", "--build-metadata", ""}},
		{"pre-missing-arg", []string{"pre", "1.2.3", "--pre"}},
		{"build-missing-arg", []string{"patch", "1.2.3", "--build-metadata"}},
		{"double-pre", []string{"pre", "1.2.3", "--pre", "rc.0", "--pre", "rc.1"}},
		{"double-no-pre", []string{"pre", "1.2.3-rc.0", "--no-pre", "--no-pre"}},
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
		if err := run(tc.argv, bytes.NewReader(nil), &stdout); err != nil {
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
	if err := run([]string{"patch", "not-a-version"}, bytes.NewReader(nil), &bytes.Buffer{}); err == nil {
		t.Error("expected error for malformed input")
	}
}

func TestRun_PreActionErrorOriginContext(t *testing.T) {
	t.Parallel()
	// 確定論点 E: VER-origin pass-through (no file context).
	err := run([]string{"pre", "1.2.3-rc1"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	err = run([]string{"pre", path}, bytes.NewReader(nil), &bytes.Buffer{})
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
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout); err != nil {
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
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout); err != nil {
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
	err := run([]string{"get", "README.md"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	if err := run([]string{"get", path}, bytes.NewReader(nil), &stdout); err != nil {
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
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &stdout); err != nil {
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
	if err := run([]string{"patch", path, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}); err != nil {
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
	if err := run([]string{"patch", "package.json"}, r, &stdout); err != nil {
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
	err = run([]string{"patch", "package.json", "--write"}, r, &bytes.Buffer{})
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
	if err := run([]string{"patch", "-"}, r, &stdout); err != nil {
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
	if err := run([]string{"patch", pkg, plug, "--write"}, bytes.NewReader(nil), &stdout); err != nil {
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
	err := run([]string{"patch", a, b, "--write"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	err := run([]string{"patch", a, b, "--write"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	if err := run([]string{"minor", v, pkg, "--write"}, bytes.NewReader(nil), &stdout); err != nil {
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
	err := run([]string{"get", a, b}, bytes.NewReader(nil), &bytes.Buffer{})
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
	if err := run([]string{"patch", a, b}, r, &stdout); err != nil {
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
	if err := run([]string{"patch", pkg, "1.2.3"}, bytes.NewReader(nil), &stdout); err != nil {
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
	err := run([]string{"patch", pkg, "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	err := run([]string{"patch", "1.2.3", "--write"}, bytes.NewReader(nil), &stdout)
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
	err := run([]string{"patch", a, b, "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{})
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
	if err := run([]string{"compare", "eq", "1.2.3", "1.2.3"}, bytes.NewReader(nil), &stdout); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("compare should not print on success, got: %q", stdout.String())
	}
}

func TestRun_Compare_Eq_False(t *testing.T) {
	t.Parallel()
	err := run([]string{"compare", "eq", "1.2.3", "1.2.4"}, bytes.NewReader(nil), &bytes.Buffer{})
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
			err := run([]string{"compare", c.op, c.a, c.b}, bytes.NewReader(nil), &bytes.Buffer{})
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

func TestRun_Compare_ParseError(t *testing.T) {
	t.Parallel()
	err := run([]string{"compare", "eq", "1.2.3", "not-a-version"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected parse error")
	}
	// Should NOT be *exitErr (which would mean "predicate false"); main
	// will turn this into exit 2.
	var ee *exitErr
	if errors.As(err, &ee) {
		t.Errorf("parse errors should not be *exitErr (would map to exit 1, want 2): %v", err)
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
	if err := run([]string{"compare", "lt", a, b}, bytes.NewReader(nil), &bytes.Buffer{}); err != nil {
		t.Errorf("expected lt true, got: %v", err)
	}
}

// --- exit code semantics for main ------------------------------------------

// Sanity check: a plain semver-layer error returned from run() is NOT
// an *exitErr (so main converts it to exit 2).
func TestRun_BumpError_NotExitErr(t *testing.T) {
	t.Parallel()
	err := run([]string{"patch", "garbage"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		t.Errorf("bump errors should not be *exitErr; main maps plain errors to exit 2: %v", err)
	}
}
