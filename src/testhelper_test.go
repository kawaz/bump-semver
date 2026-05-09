package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func tempWriteFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return dir
}

func mustRun(t *testing.T, argv ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(argv, bytes.NewReader(nil), &stdout); err != nil {
		t.Fatalf("run %v: %v", argv, err)
	}
	return stdout.String()
}

func tryRun(argv ...string) error {
	return run(argv, bytes.NewReader(nil), &bytes.Buffer{})
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(got)
}

// inspectVia routes Inspect through detectHandler so tests exercise the
// real dispatcher (DR-0005 path-aware confidence-ranked candidates).
func inspectVia(path string, content []byte) (Inspection, error) {
	h, err := detectHandler(path)
	if err != nil {
		return Inspection{}, err
	}
	return h.Inspect(content)
}

// replaceVia is the symmetric helper for Replace. It runs Inspect first to
// pin the resolved rule on the handler, then delegates to Replace.
func replaceVia(path string, content []byte, current, newVersion string) ([]byte, error) {
	h, err := detectHandler(path)
	if err != nil {
		return nil, err
	}
	if _, err := h.Inspect(content); err != nil {
		return nil, err
	}
	return h.Replace(content, current, newVersion)
}
