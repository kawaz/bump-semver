package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		argv []string
		want cliArgs
	}{
		{[]string{"patch", "Cargo.toml"}, cliArgs{action: "patch", file: "Cargo.toml"}},
		{[]string{"patch", "Cargo.toml", "--write"}, cliArgs{action: "patch", file: "Cargo.toml", write: true}},
		{[]string{"patch", "--write", "Cargo.toml"}, cliArgs{action: "patch", file: "Cargo.toml", write: true}},
		{[]string{"get", "VERSION"}, cliArgs{action: "get", file: "VERSION"}},
		{[]string{"minor", "--value", "1.2.3"}, cliArgs{action: "minor", value: "1.2.3"}},
		{[]string{"minor", "--value=1.2.3"}, cliArgs{action: "minor", value: "1.2.3"}},
		{[]string{"--version"}, cliArgs{special: "version"}},
		{[]string{"-V"}, cliArgs{special: "version"}},
		{[]string{"--help"}, cliArgs{special: "help"}},
		{[]string{"-h"}, cliArgs{special: "help"}},
		{[]string{}, cliArgs{special: "help"}},
		{[]string{"patch", "--", "--weird-file.json"}, cliArgs{action: "patch", file: "--weird-file.json"}},
	}
	for _, tc := range cases {
		got, err := parseArgs(tc.argv)
		if err != nil {
			t.Errorf("parseArgs(%v) error: %v", tc.argv, err)
			continue
		}
		if got != tc.want {
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
		{"patch", "Cargo.toml", "Other"},              // multiple file
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
	if err := run([]string{"patch", "--value", "1.2.3-alpha"}, bytes.NewReader(nil), &bytes.Buffer{}); err == nil {
		t.Error("expected error for pre-release input")
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
