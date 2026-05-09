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
