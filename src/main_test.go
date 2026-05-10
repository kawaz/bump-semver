package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		argv []string
		want cliArgs
	}{
		{[]string{"patch", "Cargo.toml"}, cliArgs{action: "patch", files: []string{"Cargo.toml"}}},
		{[]string{"patch", "Cargo.toml", "--write"}, cliArgs{action: "patch", files: []string{"Cargo.toml"}, write: true}},
		{[]string{"patch", "--write", "Cargo.toml"}, cliArgs{action: "patch", files: []string{"Cargo.toml"}, write: true}},
		{[]string{"get", "VERSION"}, cliArgs{action: "get", files: []string{"VERSION"}}},
		{[]string{"minor", "--value", "1.2.3"}, cliArgs{action: "minor", value: "1.2.3"}},
		{[]string{"minor", "--value=1.2.3"}, cliArgs{action: "minor", value: "1.2.3"}},
		{[]string{"--version"}, cliArgs{special: "version"}},
		{[]string{"-V"}, cliArgs{special: "version"}},
		{[]string{"--help"}, cliArgs{special: "help"}},
		{[]string{"-h"}, cliArgs{special: "help"}},
		{[]string{}, cliArgs{special: "help"}},
		{[]string{"patch", "--", "--weird-file.json"}, cliArgs{action: "patch", files: []string{"--weird-file.json"}}},
		// 複数 FILE は valid に
		{[]string{"get", "package.json", "package-lock.json"}, cliArgs{action: "get", files: []string{"package.json", "package-lock.json"}}},
		{[]string{"patch", "a.json", "b.json", "c.json", "--write"}, cliArgs{action: "patch", files: []string{"a.json", "b.json", "c.json"}, write: true}},
	}
	for _, tc := range cases {
		got, err := parseArgs(tc.argv)
		if err != nil {
			t.Errorf("parseArgs(%v) error: %v", tc.argv, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseArgs(%v) = %+v, want %+v", tc.argv, got, tc.want)
		}
	}
}

func TestParseArgs_Errors(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		{"foo", "Cargo.toml"}, // unknown action
		{"patch"},             // missing file/value
		{"patch", "Cargo.toml", "--value", "1.0"},     // file + value
		{"patch", "--value", "1.0", "--write"},        // value + write
		{"get", "VERSION", "--write"},                 // get + write
		{"patch", "--value"},                          // --value missing arg
		{"patch", "--value", "1.0", "--value", "1.1"}, // double --value
		{"patch", "Cargo.toml", "--unknown"},          // unknown option
		{"patch", "Cargo.toml", "--write", "--write"}, // double write
	}
	for _, argv := range cases {
		if _, err := parseArgs(argv); err == nil {
			t.Errorf("parseArgs(%v) expected error, got nil", argv)
		}
	}
}

func TestRun_ValueBumps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"patch", "--value", "1.2.3"}, "1.2.4\n"},
		{[]string{"minor", "--value", "1.2.3"}, "1.3.0\n"},
		{[]string{"major", "--value", "1.2.3"}, "2.0.0\n"},
		{[]string{"get", "--value", "1.2.3"}, "1.2.3\n"},
		// v prefix / 柔軟 separator も最終的に同じ経路を通る
		{[]string{"patch", "--value", "v1.2.3"}, "v1.2.4\n"},
		{[]string{"minor", "--value", "version_1_2_3"}, "version_1_3_0\n"},
		// DR-0006: body sep `-` removed. `ver-1.2.3` is still allowed
		// (the `-` is part of the prefix, body sep is `.`).
		{[]string{"major", "--value", "ver-1.2.3"}, "ver-2.0.0\n"},
		{[]string{"get", "--value", "v1.2.3"}, "v1.2.3\n"},
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

func TestRun_ValueRejectsBadInput(t *testing.T) {
	t.Parallel()
	// DR-0006: pre-release / build metadata are now VALID. Use a truly
	// malformed input here.
	if err := run([]string{"patch", "--value", "not-a-version"}, bytes.NewReader(nil), &bytes.Buffer{}); err == nil {
		t.Error("expected error for malformed input")
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
	err := run([]string{"get", "README.md"}, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unsupported file") {
		t.Errorf("expected unsupported-file error, got %v", err)
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
	// VERSION (name 取れない) + package.json (name=foo) は version 一致してれば OK
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
	// `get` モード単独でも整合性検証として機能する
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
	// stdin pipe があっても複数 FILE 指定時はファイルから読む (cat 慣例)
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
		// stdin pipe には無関係なゴミを流す。multi-file path なので
		// 読まれずに済むこと (= 結果が file 内容で決まる) を確認する。
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
