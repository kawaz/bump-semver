package main

import (
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
	insp, err := (npmLockHandler{}).Inspect([]byte(npmLockSample))
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

func TestNpmLock_Inspect_LockfileV1Error(t *testing.T) {
	t.Parallel()
	in := []byte(`{
  "name": "foo",
  "version": "1.2.3",
  "lockfileVersion": 1,
  "dependencies": {
    "left-pad": {"version": "1.3.0"}
  }
}
`)
	_, err := (npmLockHandler{}).Inspect(in)
	if err == nil || !strings.Contains(err.Error(), "lockfileVersion: 1") {
		t.Errorf("expected lockfileVersion 1 error, got: %v", err)
	}
}

func TestNpmLock_Inspect_NoPackagesField(t *testing.T) {
	t.Parallel()
	// lockfileVersion を欠いていても packages が無ければ v1 扱いでエラー
	in := []byte(`{
  "name": "foo",
  "version": "1.2.3",
  "dependencies": {}
}
`)
	_, err := (npmLockHandler{}).Inspect(in)
	if err == nil {
		t.Error("expected error for missing packages field")
	}
}

func TestNpmLock_Replace_DepsUntouched(t *testing.T) {
	t.Parallel()
	out, err := (npmLockHandler{}).Replace([]byte(npmLockSample), "1.2.3", "2.0.0")
	if err != nil {
		t.Fatalf("Replace error: %v", err)
	}
	s := string(out)
	// top-level + root entry が更新されている
	if !strings.Contains(s, `"version": "2.0.0"`) {
		t.Errorf("expected $.version = 2.0.0:\n%s", s)
	}
	// 依存の version は不変
	if !strings.Contains(s, `"version": "1.3.0"`) {
		t.Errorf("expected node_modules/left-pad version 1.3.0 unchanged:\n%s", s)
	}
	if !strings.Contains(s, `"version": "1.0.0"`) {
		t.Errorf("expected node_modules/right-pad version 1.0.0 unchanged:\n%s", s)
	}
	// 古い 1.2.3 (top-level/root) が完全に消えているか確認
	count := strings.Count(s, `"version": "1.2.3"`)
	if count != 0 {
		t.Errorf("expected 0 occurrences of 1.2.3 (top-level + root) after bump, got %d:\n%s", count, s)
	}
	// 新値は 2 箇所 (top-level + root)
	if got := strings.Count(s, `"version": "2.0.0"`); got != 2 {
		t.Errorf("expected 2 occurrences of 2.0.0, got %d:\n%s", got, s)
	}
}

func TestRun_PackageJsonAndLock_Combined(t *testing.T) {
	t.Parallel()
	// package.json + package-lock.json の整合性チェック + bump
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
	// 依存の 9.9.9 は不変
	got := readFile(t, lockPath)
	if !strings.Contains(got, `"version": "9.9.9"`) {
		t.Errorf("dep version touched:\n%s", got)
	}
	// top-level + root が 1.2.4 に
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
	err := tryRun("get", dir+"/package.json", dir+"/package-lock.json")
	if err == nil || !strings.HasPrefix(err.Error(), "version mismatch:") {
		t.Errorf("expected version mismatch, got: %v", err)
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
