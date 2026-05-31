package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

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
