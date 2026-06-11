package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeReplaceHandler is a Handler whose Replace either succeeds with a
// fixed body or fails, so the two-phase write contract can be exercised
// deterministically without depending on a real format's failure mode.
type fakeReplaceHandler struct {
	out     []byte
	failErr error
}

func (h fakeReplaceHandler) Inspect(content []byte) (Inspection, error) {
	return Inspection{Versions: []Field{{Value: "1.2.3", Path: "(fake)"}}}, nil
}

func (h fakeReplaceHandler) Replace(content []byte, current, newVersion string) ([]byte, error) {
	if h.failErr != nil {
		return nil, h.failErr
	}
	return h.out, nil
}

// TestWriteBumpedFiles_AllOrNothing pins the prevention guarantee: when
// the second file's Replace computation fails, the first file must NOT
// be written. Before the two-phase split this was a torn update (first
// file already rewritten on disk).
func TestWriteBumpedFiles_AllOrNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	orig := []byte("1.2.3\n")
	if err := os.WriteFile(a, orig, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, orig, 0644); err != nil {
		t.Fatal(err)
	}

	resolved := []resolvedInput{
		{file: a, content: orig, handler: fakeReplaceHandler{out: []byte("1.2.4\n")}},
		{file: b, content: orig, handler: fakeReplaceHandler{failErr: fmt.Errorf("boom")}},
	}

	err := writeBumpedFiles(resolved, "1.2.3", "1.2.4")
	if err == nil {
		t.Fatal("expected error from second file's Replace, got nil")
	}
	if !strings.Contains(err.Error(), b) {
		t.Errorf("error should name the failing file %q: %v", b, err)
	}

	// First file must be untouched (all-or-nothing).
	got, rErr := os.ReadFile(a)
	if rErr != nil {
		t.Fatal(rErr)
	}
	if !bytes.Equal(got, orig) {
		t.Errorf("first file was modified despite second-file failure: got %q, want %q", got, orig)
	}
}

// TestWriteBumpedFiles_NormalTwoFiles confirms the happy path: both
// files are committed with the computed content.
func TestWriteBumpedFiles_NormalTwoFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	orig := []byte("1.2.3\n")
	if err := os.WriteFile(a, orig, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, orig, 0644); err != nil {
		t.Fatal(err)
	}

	resolved := []resolvedInput{
		{file: a, content: orig, handler: fakeReplaceHandler{out: []byte("1.2.4\n")}},
		{file: b, content: orig, handler: fakeReplaceHandler{out: []byte("1.2.4\n")}},
	}
	if err := writeBumpedFiles(resolved, "1.2.3", "1.2.4"); err != nil {
		t.Fatalf("writeBumpedFiles error: %v", err)
	}
	for _, p := range []string{a, b} {
		got, _ := os.ReadFile(p)
		if string(got) != "1.2.4\n" {
			t.Errorf("%s = %q, want %q", p, got, "1.2.4\n")
		}
	}
}

// TestRun_FileWriteSymlinkPreserved confirms that writing through a
// symlink updates the real file and keeps the symlink as a symlink
// (atomic temp+rename must resolve the link first).
func TestRun_FileWriteSymlinkPreserved(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	// The symlink itself must carry a recognized basename (format
	// detection runs on the path the user named), so the link is the
	// recognized `VERSION` and the real file lives under another name.
	real := filepath.Join(dir, "VERSION.real")
	link := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(real, []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"patch", link, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}

	// The link must still be a symlink.
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was replaced by a regular file: mode=%v", fi.Mode())
	}
	// The real file must hold the bumped value.
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "1.2.4" {
		t.Errorf("real file = %q, want 1.2.4", got)
	}
}

// TestRun_FileWriteSymlinkPreservesTargetMode confirms the real file's
// permission bits survive a write through a symlink.
func TestRun_FileWriteSymlinkPreservesTargetMode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "VERSION.real")
	link := filepath.Join(dir, "VERSION")
	if err := os.WriteFile(real, []byte("1.2.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"patch", link, "--write"}, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run error: %v", err)
	}
	fi, err := os.Stat(real)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("real file mode = %o, want 0600", got)
	}
}
