package main

// Tests for the N-arg extension (DR-0023):
//   - get: N inputs, all-source equality, exit 1 + stderr listing on mismatch.
//   - compare: F1 + OTHERS[..], full-eval, exit 1 + per-OTHER stderr.
//   - borrowing: get/bump peer-expand across all sibling FILE paths;
//     compare borrows F1 only (existing behavior preserved).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- get: N-arg + verb-specific stderr -------------------------------------

// get with N>=2 and all sources agree: exit 0, stdout = single value.
func TestRun_Get_NArg_AllAgree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"get", a, b, "1.2.3"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("expected success, got: %v (stderr=%q)", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
		t.Errorf("stdout = %q, want 1.2.3", got)
	}
}

// get with N>=2 and a mismatch: exit code 1 (predicate-false-like),
// stderr lists all sources. This flips the legacy behavior of "exit 2
// on get mismatch"; it is intentional per DR-0023 (get treats all
// sources as equal peers, like a cross-source equality assertion).
func TestRun_Get_NArg_Mismatch_Exit1AndStderr(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"version":"1.2.4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", a, b}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != exitCodeFalse {
		t.Errorf("get mismatch exit code = %d, want %d (DR-0023: peer-equality)", ee.code, exitCodeFalse)
	}
	// Stderr lists every source + value.
	s := stderr.String()
	if !strings.Contains(s, "version mismatch:") {
		t.Errorf("stderr missing 'version mismatch:' header: %q", s)
	}
	if !strings.Contains(s, "1.2.3") || !strings.Contains(s, "1.2.4") {
		t.Errorf("stderr should list both values, got: %q", s)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty on mismatch, got: %q", stdout.String())
	}
}

// --- compare: N-arg + per-OTHER stderr -------------------------------------

// compare with N=1 OTHER (== existing 2-input form) still passes.
func TestRun_Compare_LegacyTwoInputs_StillWorks(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"compare", "gt", "1.2.4", "1.2.3"},
		bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// compare with multiple OTHERS, all predicates hold: exit 0.
func TestRun_Compare_NOthers_AllHold(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	// 2.0.0 > 1.0.0 AND 2.0.0 > 1.5.0 AND 2.0.0 > 1.9.9 → all true.
	if err := run([]string{"compare", "gt", "2.0.0", "1.0.0", "1.5.0", "1.9.9"},
		bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("expected exit 0, got: %v (stderr=%q)", err, stderr.String())
	}
}

// compare with N OTHERS, one fails: exit 1, stderr lists that failure.
func TestRun_Compare_NOthers_OneFails_Exit1AndStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	// 2.0.0 > 1.0.0 OK, 2.0.0 > 3.0.0 FAIL.
	err := run([]string{"compare", "gt", "2.0.0", "1.0.0", "3.0.0"},
		bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected predicate-false")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeFalse {
		t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
	}
	s := stderr.String()
	if !strings.Contains(s, "compare gt") || !strings.Contains(s, "2.0.0") || !strings.Contains(s, "3.0.0") {
		t.Errorf("stderr should describe failing pair, got: %q", s)
	}
	if !strings.Contains(s, "not greater than") {
		t.Errorf("stderr should use the operator phrase 'not greater than', got: %q", s)
	}
}

// compare with N OTHERS, all fail: exit 1, stderr lists all failures
// (full-evaluation, not short-circuit).
func TestRun_Compare_NOthers_AllFail_FullEval(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	// 1.0.0 > 2.0.0 FAIL, 1.0.0 > 3.0.0 FAIL, 1.0.0 > 4.0.0 FAIL.
	err := run([]string{"compare", "gt", "1.0.0", "2.0.0", "3.0.0", "4.0.0"},
		bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected predicate-false")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeFalse {
		t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
	}
	s := stderr.String()
	for _, val := range []string{"2.0.0", "3.0.0", "4.0.0"} {
		if !strings.Contains(s, val) {
			t.Errorf("stderr missing %q (full-eval should list every failing OTHER): %q", val, s)
		}
	}
}

// -qq on compare with predicate-false suppresses the per-OTHER stderr
// listing (consistent with the "-qq suppresses diagnostics" contract).
func TestRun_Compare_NOthers_QuietAllSuppressesStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"compare", "gt", "1.0.0", "2.0.0", "-qq"},
		bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected predicate-false")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeFalse {
		t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq must suppress per-OTHER stderr, got: %q", stderr.String())
	}
}

// compare with zero OTHERS (single input) is a usage error.
func TestRun_Compare_SingleInput_UsageError(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := run([]string{"compare", "gt", "1.2.3"},
		bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected usage error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeUsage {
		t.Errorf("expected exit %d, got: %v", exitCodeUsage, err)
	}
}

// --- get: peer-expand borrowing across N sibling FILE paths ----------------

// `get a b vcs:HEAD` expands the file-omitted vcs: to one entry per
// sibling FILE path → 4 resolved inputs in total. All read the same
// version, so the predicate holds.
func TestRun_Get_VcsBorrow_PeerExpand_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	// Add a second file with the SAME version at HEAD.
	withCwd(t, dir, func() {
		b := filepath.Join(dir, "b.json")
		if err := os.WriteFile(b, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "b.json")
		runIn(t, dir, "git", "commit", "-qm", "add b.json")

		var stdout, stderr bytes.Buffer
		// 4 effective sources: VERSION, b.json, vcs:HEAD:VERSION, vcs:HEAD:b.json
		err := run([]string{"get", "VERSION", "b.json", "vcs:HEAD"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("peer-expand all-agree should succeed, got: %v (stderr=%q)", err, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != "1.2.3" {
			t.Errorf("stdout = %q, want 1.2.3", got)
		}
	})
}

// Peer-expand detects mismatch when the borrowed vcs: snapshot for a
// sibling FILE disagrees with the working-tree value.
func TestRun_Get_VcsBorrow_PeerExpand_DetectsMismatch_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// b.json: commit at 1.2.3, then mutate working tree to 9.9.9.
		b := filepath.Join(dir, "b.json")
		if err := os.WriteFile(b, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "b.json")
		runIn(t, dir, "git", "commit", "-qm", "add b.json")
		if err := os.WriteFile(b, []byte(`{"version":"9.9.9"}`), 0644); err != nil {
			t.Fatal(err)
		}
		// vcs:HEAD must expand into BOTH VERSION (1.2.3) and b.json
		// (1.2.3 in HEAD). Working-tree b.json is 9.9.9 → mismatch.
		var stdout, stderr bytes.Buffer
		err := run([]string{"get", "VERSION", "b.json", "vcs:HEAD"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatal("expected mismatch (working b.json=9.9.9 vs HEAD/VERSION=1.2.3)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		// stderr should mention both values.
		if !strings.Contains(stderr.String(), "9.9.9") {
			t.Errorf("stderr should mention 9.9.9, got: %q", stderr.String())
		}
	})
}

// --- compare: F1-borrow with N>=2 OTHERS path-omitted vcs ------------------

// `compare gt VERSION vcs:HEAD vcs:HEAD~1` — both OTHERS borrow F1's
// path (VERSION). After the test bump, VERSION > HEAD~1's snapshot.
// vcs:HEAD is the post-commit state, so vs that we test eq instead.
func TestRun_Compare_NOthers_BorrowFromF1_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// 2 OTHERS, both path-omitted; both borrow VERSION.
		// 1.2.3 == HEAD's VERSION (1.2.3) AND 1.2.3 > HEAD~1's VERSION (0.0.1).
		// Use ge so both succeed.
		var stdout, stderr bytes.Buffer
		err := run([]string{"compare", "ge", "VERSION", "vcs:HEAD", "vcs:HEAD~1"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err != nil {
			t.Fatalf("compare ge should succeed for both, got: %v (stderr=%q)", err, stderr.String())
		}
	})
}

// --- follow-up #35: name mismatch on get → exit 1 + source listing ---------

// `get` with two FILE inputs whose `name` fields disagree must behave
// the same as a `version mismatch`: exit 1, stderr lists every source
// + name value, stdout empty. Bump verbs (patch/minor/major) keep their
// existing exit-2 behavior (see TestRun_MultiFile_NameMismatch in
// main_test.go) because writing back inconsistent inputs is destructive.
func TestRun_Get_NameMismatch_Exit1AndStderr(t *testing.T) {
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
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", a, b}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != exitCodeFalse {
		t.Errorf("get name mismatch exit code = %d, want %d (follow-up #35: peer-equality)", ee.code, exitCodeFalse)
	}
	s := stderr.String()
	if !strings.Contains(s, "name mismatch:") {
		t.Errorf("stderr missing 'name mismatch:' header: %q", s)
	}
	if !strings.Contains(s, "foo") || !strings.Contains(s, "bar") {
		t.Errorf("stderr should list both name values, got: %q", s)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty on mismatch, got: %q", stdout.String())
	}
}

// -qq on get name mismatch suppresses the per-source stderr listing
// (consistent with the "-qq suppresses diagnostics" contract and the
// existing version-mismatch behavior).
func TestRun_Get_NameMismatch_QuietAllSuppressesStderr(t *testing.T) {
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
	var stdout, stderr bytes.Buffer
	err := run([]string{"get", a, b, "-qq"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected name mismatch error")
	}
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != exitCodeFalse {
		t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
	}
	if stderr.Len() != 0 {
		t.Errorf("-qq must suppress name-mismatch stderr, got: %q", stderr.String())
	}
}

// --- follow-up #35: peer-expand stderr labels are file-specific ------------

// When `vcs:HEAD` peer-expands across multiple sibling FILE paths and
// the snapshots disagree, the stderr `version mismatch:` listing must
// label each expanded source with its borrowed in-file path so users
// can tell them apart — `vcs:HEAD:VERSION` and `vcs:HEAD:b.json`, not
// two indistinguishable `vcs:HEAD` lines.
func TestRun_Get_PeerExpand_StderrLabelsAreFileSpecific_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := setupGitRepo(t, nil, "1.2.3")
	withCwd(t, dir, func() {
		// Commit b.json at 1.2.3, then mutate working-tree b.json to 9.9.9.
		// VERSION (working) = 1.2.3, b.json (working) = 9.9.9.
		// vcs:HEAD peer-expands to vcs:HEAD:VERSION (1.2.3) and
		// vcs:HEAD:b.json (1.2.3). The mismatch (9.9.9) drives the
		// stderr listing.
		b := filepath.Join(dir, "b.json")
		if err := os.WriteFile(b, []byte(`{"version":"1.2.3"}`), 0644); err != nil {
			t.Fatal(err)
		}
		runIn(t, dir, "git", "add", "b.json")
		runIn(t, dir, "git", "commit", "-qm", "add b.json")
		if err := os.WriteFile(b, []byte(`{"version":"9.9.9"}`), 0644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		err := run([]string{"get", "VERSION", "b.json", "vcs:HEAD"},
			bytes.NewReader(nil), &stdout, &stderr)
		if err == nil {
			t.Fatal("expected mismatch (working b.json=9.9.9 vs HEAD/VERSION=1.2.3)")
		}
		var ee *exitErr
		if !errors.As(err, &ee) || ee.code != exitCodeFalse {
			t.Errorf("expected exit %d, got: %v", exitCodeFalse, err)
		}
		s := stderr.String()
		// Both peer-expanded vcs sources must appear with their
		// borrowed in-file path appended, so they're distinguishable
		// from each other in the column-aligned listing.
		if !strings.Contains(s, "vcs:HEAD:VERSION") {
			t.Errorf("stderr should label the VERSION-borrowed peer as 'vcs:HEAD:VERSION', got: %q", s)
		}
		if !strings.Contains(s, "vcs:HEAD:b.json") {
			t.Errorf("stderr should label the b.json-borrowed peer as 'vcs:HEAD:b.json', got: %q", s)
		}
	})
}
