package main

import (
	"bytes"
	"strings"
	"testing"
)

const npmLockSample = `{
  "name": "foo",
  "version": "1.2.3",
  "lockfileVersion": 3,
  "requires": true,
  "packages": {
    "": {
      "name": "foo",
      "version": "1.2.3",
      "license": "MIT",
      "dependencies": {
        "left-pad": "^1.0.0"
      }
    },
    "node_modules/left-pad": {
      "version": "1.3.0",
      "resolved": "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz",
      "integrity": "sha512-aaaaaaa==",
      "license": "MIT"
    },
    "node_modules/right-pad": {
      "name": "right-pad",
      "version": "1.0.0"
    }
  }
}
`

func TestNpmLock_Inspect(t *testing.T) {
	t.Parallel()
	insp, err := inspectVia("package-lock.json", []byte(npmLockSample))
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	wantVersionPaths := []string{`$.version`, `$.packages[""].version`}
	if len(insp.Versions) != 2 {
		t.Fatalf("Versions = %+v, want 2 entries", insp.Versions)
	}
	for i, want := range wantVersionPaths {
		if insp.Versions[i].Path != want {
			t.Errorf("Versions[%d].Path = %q, want %q", i, insp.Versions[i].Path, want)
		}
		if insp.Versions[i].Value != "1.2.3" {
			t.Errorf("Versions[%d].Value = %q, want 1.2.3", i, insp.Versions[i].Value)
		}
	}
	wantNamePaths := []string{`$.name`, `$.packages[""].name`}
	if len(insp.Names) != 2 {
		t.Fatalf("Names = %+v, want 2 entries", insp.Names)
	}
	for i, want := range wantNamePaths {
		if insp.Names[i].Path != want {
			t.Errorf("Names[%d].Path = %q, want %q", i, insp.Names[i].Path, want)
		}
		if insp.Names[i].Value != "foo" {
			t.Errorf("Names[%d].Value = %q, want foo", i, insp.Names[i].Value)
		}
	}
}

func TestNpmLock_Replace_DepsUntouched(t *testing.T) {
	t.Parallel()
	out, err := replaceVia("package-lock.json", []byte(npmLockSample), "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"version": "2.0.0"`) {
		t.Errorf("expected $.version = 2.0.0:\n%s", s)
	}
	if !strings.Contains(s, `"version": "1.3.0"`) {
		t.Errorf("expected node_modules/left-pad version 1.3.0 unchanged:\n%s", s)
	}
	if !strings.Contains(s, `"version": "1.0.0"`) {
		t.Errorf("expected node_modules/right-pad version 1.0.0 unchanged:\n%s", s)
	}
	count := strings.Count(s, `"version": "1.2.3"`)
	if count != 0 {
		t.Errorf("expected 0 occurrences of 1.2.3 (top-level + root) after bump, got %d:\n%s", count, s)
	}
	if got := strings.Count(s, `"version": "2.0.0"`); got != 2 {
		t.Errorf("expected 2 occurrences of 2.0.0, got %d:\n%s", got, s)
	}
}

func TestRun_PackageJsonAndLock_Combined(t *testing.T) {
	t.Parallel()
	pkg := `{"name":"foo","version":"1.2.3"}`
	lock := `{
  "name": "foo",
  "version": "1.2.3",
  "lockfileVersion": 3,
  "packages": {
    "": {"name": "foo", "version": "1.2.3"},
    "node_modules/dep": {"version": "9.9.9"}
  }
}
`
	dir := tempWriteFiles(t, map[string]string{"package.json": pkg, "package-lock.json": lock})
	pkgPath := dir + "/package.json"
	lockPath := dir + "/package-lock.json"
	out := mustRun(t, "patch", pkgPath, lockPath, "--write")
	if strings.TrimSpace(out) != "1.2.4" {
		t.Fatalf("stdout = %q, want 1.2.4", out)
	}
	got := readFile(t, lockPath)
	if !strings.Contains(got, `"version": "9.9.9"`) {
		t.Errorf("dep version touched:\n%s", got)
	}
	if got := strings.Count(got, `"version": "1.2.4"`); got != 2 {
		t.Errorf("expected 2 occurrences of 1.2.4 in lock, got %d", got)
	}
}

func TestRun_PackageJsonAndLock_VersionMismatch(t *testing.T) {
	t.Parallel()
	pkg := `{"name":"foo","version":"1.2.3"}`
	lock := `{
  "name": "foo",
  "version": "1.2.3",
  "lockfileVersion": 3,
  "packages": {
    "": {"name": "foo", "version": "1.2.4"}
  }
}
`
	dir := tempWriteFiles(t, map[string]string{"package.json": pkg, "package-lock.json": lock})
	// DR-0023: get mismatch is exit 1 (predicate-false) with the
	// per-source listing on stderr — the returned err carries the
	// exit code, not the message text.
	var stderr bytes.Buffer
	err := run([]string{"get", dir + "/package.json", dir + "/package-lock.json"},
		bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected version mismatch")
	}
	if !strings.HasPrefix(stderr.String(), "version mismatch:") {
		t.Errorf("expected 'version mismatch:' on stderr, got: %q", stderr.String())
	}
}

func TestRun_PackageLock_InternalNameMismatch(t *testing.T) {
	t.Parallel()
	// top-level $.name と $.packages[""].name が違う壊れた lockfile は
	// main の name 整合性チェックで検出される
	lock := `{
  "name": "foo",
  "version": "1.2.3",
  "lockfileVersion": 3,
  "packages": {
    "": {"name": "bar", "version": "1.2.3"}
  }
}
`
	dir := tempWriteFiles(t, map[string]string{"package-lock.json": lock})
	err := tryRun("get", dir+"/package-lock.json")
	if err == nil || !strings.HasPrefix(err.Error(), "name mismatch:") {
		t.Errorf("expected name mismatch, got: %v", err)
	}
}
