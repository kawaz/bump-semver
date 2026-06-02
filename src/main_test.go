package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		// short help: コンパクトな overview。Commands 一覧 + --help-full 案内
		{"short-help-flag", []string{"--help"}, "See 'bump-semver <command> --help'", "Supported file formats"},
		{"short-help-h", []string{"-h"}, "'bump-semver --help-full' for the complete reference", ""},
		{"short-help-empty", []string{}, "See 'bump-semver <command> --help'", ""},
		// full help: Supported file formats 表 + 全 Examples
		{"full-help", []string{"--help-full"}, "Supported file formats (auto-detected by basename)", "Action-specific help: bump-semver <action> --help"},
		// per-action helps: それぞれ固有の見出しを持つ
		{"action-help-major", []string{"major", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-minor", []string{"minor", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-patch", []string{"patch", "--help"}, "bump-semver major | minor | patch — bump a SemVer component", ""},
		{"action-help-pre", []string{"pre", "--help"}, "bump-semver pre — manage pre-release identifiers", ""},
		{"action-help-get", []string{"get", "--help"}, "bump-semver get — print the current version", ""},
		{"action-help-compare", []string{"compare", "--help"}, "bump-semver compare — compare a base value to one or more others", ""},
		{"action-help-compare-with-op", []string{"compare", "eq", "--help"}, "bump-semver compare — compare a base value to one or more others", ""},
		{"action-help-compare-with-precision-op", []string{"compare", "eq-major", "--help"}, "bump-semver compare — compare a base value to one or more others", ""},
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
