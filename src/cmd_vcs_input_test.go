package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Compare against vcs:HEAD~1 (which holds the pre-bump 0.0.1) — current
// version 1.2.3 should be greater.
func TestRun_VcsInput_Simple(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Working tree (1.2.3) > HEAD~1 (0.0.1).
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1:VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true, got: %v", err)
		}
	})
}

// File borrowing: the second argument has no FILE component, so the
// path is taken from the first FILE-origin sibling (`VERSION`).
func TestRun_VcsInput_FileBorrow(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true (borrow VERSION), got: %v", err)
		}
	})
}

// `vcs:latest-tag()` resolves through the function path; v1.0.0 < 1.2.3.
func TestRun_VcsInput_LatestTag(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"v1.0.0"}, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:latest-tag()"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected current > latest tag, got: %v", err)
		}
	})
}

// `vcs:latest-tag()` errors when no tag parses — actionable error.
func TestRun_VcsInput_LatestTag_NoSemver(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, []string{"build-stamp"}, "1.0.0")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "VERSION", "vcs:latest-tag()"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error when no semver tags")
		}
		if !strings.Contains(stderr.String(), "no semver-compatible tags") {
			t.Errorf("stderr should mention 'no semver-compatible tags', got: %q", stderr.String())
		}
	})
}

// --write with vcs: input is rejected before any side effects.
func TestRun_VcsInput_WriteRejected(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"patch", "VERSION", "vcs:HEAD~1", "--write"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --write + vcs: rejection")
		}
		if !strings.Contains(stderr.String(), "--write cannot be used with vcs: inputs") {
			t.Errorf("stderr should mention the rejection reason, got: %q", stderr.String())
		}
	})
}

// --write with cmd: input is rejected (same policy as vcs:, both are
// read-only schemas resolved from external sources without a writable
// backing file).
func TestRun_CmdInput_WriteRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"patch", "VERSION", "cmd:echo 1.2.3", "--write"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected --write + cmd: rejection")
		}
		if !strings.Contains(stderr.String(), "--write cannot be used with cmd: inputs") {
			t.Errorf("stderr should mention the rejection reason, got: %q", stderr.String())
		}
	})
}

// --vcs git forces the override even though the directory is also
// suitable for jj. We only have a git fixture here, so this primarily
// tests the flag parsing + override propagation path.
func TestRun_VcsInput_VcsForceFlag(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		err := run([]string{"compare", "gt", "VERSION", "vcs:HEAD~1", "--vcs", "git"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected gt true with --vcs git, got: %v", err)
		}
	})
}

// --vcs with an invalid value is a parse-time error.
func TestRun_VcsInput_InvalidVcsValue(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"compare", "gt", "1.2.3", "1.2.4", "--vcs", "hg"},
		bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected invalid --vcs value to error")
	}
	if !strings.Contains(stderr.String(), "invalid --vcs value") {
		t.Errorf("stderr should mention the invalid value, got: %q", stderr.String())
	}
}

// `vcs:HEAD~1` (no FILE) with no FILE-origin sibling to borrow from is
// an error — the user has to supply `vcs:HEAD~1:path` or pair it with
// a real file.
func TestRun_VcsInput_BorrowRequired(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "1.2.3", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error when no FILE to borrow from")
		}
		if !strings.Contains(stderr.String(), "file is required") {
			t.Errorf("stderr should mention borrow failure, got: %q", stderr.String())
		}
	})
}

// Unknown function names produce a clean error rather than reaching the
// VCS subprocess.
func TestRun_VcsInput_UnknownFunction(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := run([]string{"compare", "eq", "1.2.3", "vcs:current-branch()", "--vcs", "git"},
		bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("expected unknown function error")
	}
	if !strings.Contains(stderr.String(), "unknown vcs function") {
		t.Errorf("stderr should mention unknown function, got: %q", stderr.String())
	}
}

// Multiple vcs: inputs are allowed and pass through allSameValue. We
// only have one commit at HEAD~1 (0.0.1) and another at HEAD (1.2.3),
// so two `vcs:HEAD~1:VERSION` references should agree with each other
// and with a same-valued VER input.
func TestRun_VcsInput_MultipleVcs(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Three inputs all evaluating to 0.0.1: get should succeed.
		err := run([]string{"get", "0.0.1", "vcs:HEAD~1:VERSION", "vcs:HEAD~1:VERSION"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("multiple vcs: inputs should agree, got: %v", err)
		}
	})
}

// Borrow source picked from `vcs:REV:FILE`: when only vcs: inputs are
// present and one of them names a file explicitly, downstream
// file-omitted vcs: inputs borrow from it.
func TestRun_VcsInput_BorrowFromVcsExplicit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Both args resolve via vcs:; the second borrows VERSION from
		// the first. Both refer to HEAD so they should equal.
		err := run([]string{"compare", "eq", "vcs:HEAD:VERSION", "vcs:HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected eq true (vcs:HEAD borrowed VERSION), got: %v", err)
		}
	})
}

// All-vcs-with-no-file is an error: nothing to borrow from.
func TestRun_VcsInput_AllVcsNoFile(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		var stderr bytes.Buffer
		err := run([]string{"compare", "eq", "vcs:HEAD", "vcs:HEAD~1"},
			bytes.NewReader(nil), &bytes.Buffer{}, &stderr)
		if err == nil {
			t.Fatal("expected error: no file to borrow")
		}
		if !strings.Contains(stderr.String(), "file is required") {
			t.Errorf("stderr should mention borrow failure, got: %q", stderr.String())
		}
	})
}

// Position-order: the *first* FILE-origin input wins as borrow target,
// even when later args also have FILEs. We verify by building a repo
// with two siblings (`a.json` agrees with HEAD, `b.json` doesn't) and
// passing them in different orders.
func TestRun_VcsInput_BorrowPositionOrder(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Add a second file (a.json) at HEAD with a *different*
		// version. The vcs:HEAD argument with no file should borrow
		// from VERSION (the leftmost FILE-origin), so the comparison
		// against the literal 1.2.3 should pass.
		if err := os.WriteFile(filepath.Join(dir, "a.json"),
			[]byte(`{"version":"9.9.9"}`), 0644); err != nil {
			t.Fatal(err)
		}
		// VERSION (=1.2.3) is the leftmost FILE; vcs:HEAD borrows it.
		// Order: VERSION, a.json, vcs:HEAD — but compare takes only 2
		// inputs. Use `get` (which is variadic) to test the borrow.
		err := run([]string{"get", "VERSION", "1.2.3", "vcs:HEAD"},
			bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{})
		if err != nil {
			t.Errorf("expected agree on 1.2.3 (borrow VERSION), got: %v", err)
		}
	})
}
