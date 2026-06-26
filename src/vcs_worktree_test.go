package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixtures --------------------------------------------------------------

// setupGitRepoWithWorktree builds on setupGitRepo by adding a linked
// worktree at `<main>/../<wtName>` on a new branch `<branchName>`. Returns
// the main worktree path and the linked worktree path.
func setupGitRepoWithWorktree(t *testing.T, fileVersion, wtName, branchName string) (mainDir, wtDir string) {
	t.Helper()
	mainDir = setupGitRepo(t, nil, fileVersion)
	wtDir = filepath.Join(filepath.Dir(mainDir), wtName)
	runIn(t, mainDir, "git", "worktree", "add", "-b", branchName, wtDir)
	return mainDir, wtDir
}

// setupJjRepoWithWorkspace builds on setupJjRepo by adding a secondary
// workspace at `<default>/../<wsName>`. Returns the default workspace path
// and the secondary workspace path.
func setupJjRepoWithWorkspace(t *testing.T, fileVersion, wsName string) (defaultDir, wsDir string) {
	t.Helper()
	defaultDir = setupJjRepo(t, nil, fileVersion)
	wsDir = filepath.Join(filepath.Dir(defaultDir), wsName)
	runIn(t, defaultDir, "jj", "workspace", "add", wsDir)
	return defaultDir, wsDir
}

// --- IsWorktree ------------------------------------------------------------

func TestVcs_IsWorktree_Git_Main(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.IsWorktree()
		if err != nil {
			t.Fatalf("IsWorktree: %v", err)
		}
		if got {
			t.Errorf("main worktree: IsWorktree = true, want false")
		}
	})
}

func TestVcs_IsWorktree_Git_Linked(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	_, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "linked-wt", "feature/x")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		got, err := b.IsWorktree()
		if err != nil {
			t.Fatalf("IsWorktree: %v", err)
		}
		if !got {
			t.Errorf("linked worktree: IsWorktree = false, want true")
		}
	})
}

func TestVcs_IsWorktree_Jj_Default(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		got, err := b.IsWorktree()
		if err != nil {
			t.Fatalf("IsWorktree: %v", err)
		}
		if got {
			t.Errorf("default workspace: IsWorktree = true, want false")
		}
	})
}

func TestVcs_IsWorktree_Jj_Secondary(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	_, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		got, err := b.IsWorktree()
		if err != nil {
			t.Fatalf("IsWorktree: %v", err)
		}
		if !got {
			t.Errorf("secondary workspace: IsWorktree = false, want true")
		}
	})
}

// --- WorktreeName ----------------------------------------------------------

func TestVcs_WorktreeName_Git_Main(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.WorktreeName()
		if err != nil {
			t.Fatalf("WorktreeName: %v", err)
		}
		if got != "" {
			t.Errorf("main worktree: WorktreeName = %q, want \"\"", got)
		}
	})
}

func TestVcs_WorktreeName_Git_Linked(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	_, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "linked-wt", "feature/x")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		got, err := b.WorktreeName()
		if err != nil {
			t.Fatalf("WorktreeName: %v", err)
		}
		if got != "linked-wt" {
			t.Errorf("WorktreeName = %q, want \"linked-wt\"", got)
		}
	})
}

func TestVcs_WorktreeName_Jj_Default(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		got, err := b.WorktreeName()
		if err != nil {
			t.Fatalf("WorktreeName: %v", err)
		}
		if got != "" {
			t.Errorf("default workspace: WorktreeName = %q, want \"\"", got)
		}
	})
}

func TestVcs_WorktreeName_Jj_Secondary(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	_, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		got, err := b.WorktreeName()
		if err != nil {
			t.Fatalf("WorktreeName: %v", err)
		}
		if got != "feature-ws" {
			t.Errorf("WorktreeName = %q, want \"feature-ws\"", got)
		}
	})
}

// --- DefaultBranch ---------------------------------------------------------

func TestVcs_DefaultBranch_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.DefaultBranch()
		if err != nil {
			t.Fatalf("DefaultBranch: %v", err)
		}
		if got != "main" {
			t.Errorf("DefaultBranch = %q, want \"main\"", got)
		}
	})
}

func TestVcs_DefaultBranch_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		got, err := b.DefaultBranch()
		if err != nil {
			t.Fatalf("DefaultBranch: %v", err)
		}
		if got != "main" {
			t.Errorf("DefaultBranch = %q, want \"main\"", got)
		}
	})
}

// --- IsOnDefaultBranch -----------------------------------------------------

func TestVcs_IsOnDefaultBranch_Git_True(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.IsOnDefaultBranch()
		if err != nil {
			t.Fatalf("IsOnDefaultBranch: %v", err)
		}
		if !got {
			t.Errorf("on main: IsOnDefaultBranch = false, want true")
		}
	})
}

func TestVcs_IsOnDefaultBranch_Git_False(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	_, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "wt", "feature/x")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		got, err := b.IsOnDefaultBranch()
		if err != nil {
			t.Fatalf("IsOnDefaultBranch: %v", err)
		}
		if got {
			t.Errorf("on feature/x: IsOnDefaultBranch = true, want false")
		}
	})
}

// --- Promote ---------------------------------------------------------------

func TestVcs_Promote_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	mainDir, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "wt", "feature/x")
	// Make a new commit on feature/x.
	if err := writeFile(filepath.Join(wtDir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, wtDir, "git", "add", "VERSION")
	runIn(t, wtDir, "git", "commit", "-qm", "bump to 2.0.0")
	// Capture feature/x SHA.
	featureSHA := strings.TrimSpace(runInOut(t, wtDir, "git", "rev-parse", "HEAD"))
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		if err := b.Promote(promoteOpts{}); err != nil {
			t.Fatalf("Promote: %v", err)
		}
	})
	// main branch should now point at feature/x's HEAD.
	mainSHA := strings.TrimSpace(runInOut(t, mainDir, "git", "rev-parse", "refs/heads/main"))
	if mainSHA != featureSHA {
		t.Errorf("after Promote: main = %s, want %s (feature/x)", mainSHA, featureSHA)
	}
}

func TestVcs_Promote_Git_NonFastForward(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	mainDir, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "wt", "feature/x")
	// Diverge main from feature/x: add a commit on main while feature/x stays put.
	if err := writeFile(filepath.Join(mainDir, "VERSION"), "1.5.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, mainDir, "git", "add", "VERSION")
	runIn(t, mainDir, "git", "commit", "-qm", "diverge main")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		err := b.Promote(promoteOpts{})
		if err == nil {
			t.Fatalf("Promote: want non-FF error, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeNonFastForward {
			t.Errorf("Promote err = %v, want exitErr{nonFastForward}", err)
		}
	})
}

func TestVcs_Promote_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	defaultDir, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	// Make a new commit on the secondary workspace.
	if err := writeFile(filepath.Join(wsDir, "VERSION"), "2.0.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, wsDir, "jj", "commit", "-m", "bump to 2.0.0", "VERSION")
	// Capture the just-committed change's commit_id (= @-).
	committed := strings.TrimSpace(runInOut(t, wsDir, "jj", "log", "-r", "@-", "--no-graph", "-T", `commit_id ++ "\n"`))
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		if err := b.Promote(promoteOpts{}); err != nil {
			t.Fatalf("Promote: %v", err)
		}
	})
	// main bookmark should now point at @- (the bump commit).
	mainCommit := strings.TrimSpace(runInOut(t, defaultDir, "jj", "log", "-r", "main", "--no-graph", "-T", `commit_id ++ "\n"`))
	if mainCommit != committed {
		t.Errorf("after Promote: main = %s, want %s", mainCommit, committed)
	}
}

// --- Sync ------------------------------------------------------------------

func TestVcs_Sync_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	mainDir, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "wt", "feature/x")
	// Advance main beyond feature/x's base.
	if err := writeFile(filepath.Join(mainDir, "VERSION"), "1.5.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, mainDir, "git", "add", "VERSION")
	runIn(t, mainDir, "git", "commit", "-qm", "advance main")
	newMainSHA := strings.TrimSpace(runInOut(t, mainDir, "git", "rev-parse", "HEAD"))
	// Add a commit on feature/x.
	if err := writeFile(filepath.Join(wtDir, "NEWFILE"), "x\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, wtDir, "git", "add", "NEWFILE")
	runIn(t, wtDir, "git", "commit", "-qm", "feature work")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		if err := b.Sync(syncOpts{Onto: "main"}); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	})
	// After rebase, HEAD~1 should be newMainSHA.
	parentSHA := strings.TrimSpace(runInOut(t, wtDir, "git", "rev-parse", "HEAD~1"))
	if parentSHA != newMainSHA {
		t.Errorf("after Sync: HEAD~1 = %s, want %s (= new main)", parentSHA, newMainSHA)
	}
}

func TestVcs_Sync_Jj(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	defaultDir, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	// Advance main in the default workspace.
	if err := writeFile(filepath.Join(defaultDir, "VERSION"), "1.5.0\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, defaultDir, "jj", "commit", "-m", "advance main", "VERSION")
	runIn(t, defaultDir, "jj", "bookmark", "set", "main", "-r", "@-")
	newMainCommit := strings.TrimSpace(runInOut(t, defaultDir, "jj", "log", "-r", "main", "--no-graph", "-T", `commit_id ++ "\n"`))
	// Add a commit on the secondary workspace.
	if err := writeFile(filepath.Join(wsDir, "NEWFILE"), "x\n"); err != nil {
		t.Fatal(err)
	}
	runIn(t, wsDir, "jj", "commit", "-m", "feature work", "NEWFILE")
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		if err := b.Sync(syncOpts{Onto: "main"}); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	})
	// After rebase, the bump commit's parent should be newMainCommit.
	parent := strings.TrimSpace(runInOut(t, wsDir, "jj", "log", "-r", "@--", "--no-graph", "-T", `commit_id ++ "\n"`))
	if parent != newMainCommit {
		t.Errorf("after Sync: @-- = %s, want %s (= new main)", parent, newMainCommit)
	}
}

func TestVcs_Sync_MissingOnto_Git(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		err := b.Sync(syncOpts{})
		if err == nil {
			t.Fatalf("Sync without --onto: want error, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeUsage {
			t.Errorf("Sync err = %v, want exitErr{usage}", err)
		}
	})
}

// runInOut runs `name args...` in dir with the same hermetic env runIn
// uses, returning stdout. Helper for tests that need a subprocess's
// output (e.g. a resolved commit SHA) for assertion.
func runInOut(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{},
		"PATH="+t.TempDir()+":"+pathEnv(),
		"HOME="+t.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"JJ_USER=Test",
		"JJ_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v in %s: %v\n%s", name, args, dir, err, out)
	}
	return string(out)
}

// --- DefaultBranchPath -----------------------------------------------------

// canonPath returns filepath.EvalSymlinks(p) or p when EvalSymlinks fails
// (= the test fixture's tempdir may sit under /var → /private/var on
// macOS; the worktree paths git emits go through the symlink, so we
// canonicalise both sides before comparing).
func canonPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func TestVcs_DefaultBranchPath_Git_MainOnly(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.DefaultBranchPath()
		if err != nil {
			t.Fatalf("DefaultBranchPath: %v", err)
		}
		if canonPath(got) != canonPath(dir) {
			t.Errorf("DefaultBranchPath = %q, want %q", got, dir)
		}
	})
}

// TestVcs_DefaultBranchPath_Git_FromLinkedWorktree: the main worktree
// (= the one carrying refs/heads/main) is returned regardless of where
// the command is invoked from.
func TestVcs_DefaultBranchPath_Git_FromLinkedWorktree(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	mainDir, wtDir := setupGitRepoWithWorktree(t, "1.0.0", "feature-wt", "feature/x")
	withCwd(t, wtDir, func() {
		b := &gitBackend{}
		got, err := b.DefaultBranchPath()
		if err != nil {
			t.Fatalf("DefaultBranchPath: %v", err)
		}
		if canonPath(got) != canonPath(mainDir) {
			t.Errorf("DefaultBranchPath = %q, want %q (main worktree)", got, mainDir)
		}
	})
}

// TestVcs_DefaultBranchPath_Git_NoMatch: when no worktree has the
// default branch checked out, exit 4.
func TestVcs_DefaultBranchPath_Git_NoMatch(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := setupGitRepo(t, nil, "1.0.0")
	// Move the only worktree off main onto a feature branch — no worktree
	// will have main checked out anymore.
	runIn(t, dir, "git", "checkout", "-b", "feature/x")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		_, err := b.DefaultBranchPath()
		if err == nil {
			t.Fatalf("DefaultBranchPath: want error, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeAmbiguous {
			t.Errorf("DefaultBranchPath err = %v, want exitErr{ambiguous}", err)
		}
	})
}

// TestVcs_DefaultBranchPath_Git_TieBreak: when two worktrees both have
// the default branch checked out, the one whose dir basename matches
// the default branch name wins.
//
// Setup: rename the main worktree so its basename ≠ "main", then add a
// linked worktree at `main` checking out a different branch first, then
// `git checkout main` in that linked worktree. (Git refuses two
// worktrees on the same branch by default; we force it via the second
// worktree being a "detached then checkout" sequence with
// `--ignore-other-worktrees`.)
//
// Skipped (= the corner case is hard to set up portably): tied to a
// future regression test if needed. The pickDefaultBranchPath unit test
// (TestPickDefaultBranchPath_TieBreak) already exercises the tie-break
// logic in isolation.
func TestVcs_DefaultBranchPath_Git_TieBreak(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	// We exercise the tie-break via the table-driven unit test below;
	// reproducing the "two worktrees both on main" state via the git CLI
	// requires `--ignore-other-worktrees` semantics that vary across git
	// versions. This integration test asserts the no-tie happy path; the
	// tie-break code path is covered by TestPickDefaultBranchPath_TieBreak.
	dir := setupGitRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &gitBackend{}
		got, err := b.DefaultBranchPath()
		if err != nil {
			t.Fatalf("DefaultBranchPath: %v", err)
		}
		if canonPath(got) != canonPath(dir) {
			t.Errorf("DefaultBranchPath = %q, want %q", got, dir)
		}
	})
}

func TestVcs_DefaultBranchPath_Jj_DefaultOnly(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	dir := setupJjRepo(t, nil, "1.0.0")
	withCwd(t, dir, func() {
		b := &jjBackend{}
		got, err := b.DefaultBranchPath()
		if err != nil {
			t.Fatalf("DefaultBranchPath: %v", err)
		}
		if canonPath(got) != canonPath(dir) {
			t.Errorf("DefaultBranchPath = %q, want %q", got, dir)
		}
	})
}

// TestVcs_DefaultBranchPath_Jj_FromSecondary: invoked from a secondary
// workspace, the default workspace (with the `main` bookmark on @-) is
// returned. The setupJjRepoWithWorkspace fixture leaves both workspaces
// sharing the same @- (= main bookmark), so this scenario hits the
// tie-break path: only the workspace named "main" → wait, the default
// workspace is named "default" in jj's `workspace add` convention. The
// tie-break key would be the default workspace dir basename (= the
// tempdir name), so for fresh setupJjRepo with an unrelated name, both
// candidates match but neither is named "main" → exit 5.
//
// This makes the realistic post-`workspace add` scenario surface the
// ambiguity error, which is the intended behaviour: kawaz's workflow
// requires the main workspace's directory to be named after the default
// branch for unambiguous tie-break.
func TestVcs_DefaultBranchPath_Jj_FromSecondary_Ambiguous(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	_, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		_, err := b.DefaultBranchPath()
		// Two workspaces both have `main` as their nearest bookmark, and
		// neither is named "main" → tie-break fails with exit 5.
		if err == nil {
			t.Fatalf("DefaultBranchPath: want ambiguous tie-break error, got nil")
		}
		ee, ok := err.(*exitErr)
		if !ok || ee.code != exitCodeNonFastForward {
			t.Errorf("DefaultBranchPath err = %v, want exitErr{nonFastForward}", err)
		}
	})
}

// TestVcs_DefaultBranchPath_Jj_FromSecondary_Named: same fixture as
// TestVcs_DefaultBranchPath_Jj_FromSecondary_Ambiguous but the default
// workspace is renamed to "main" via `jj workspace rename` so the
// tie-break picks it deterministically. Mirrors the kawaz convention
// where the workspace named after the default branch holds it.
func TestVcs_DefaultBranchPath_Jj_FromSecondary_Named(t *testing.T) {
	if !gitAvailable() || !jjAvailable() {
		t.Skip("git+jj fixture requires both binaries")
	}
	defaultDir, wsDir := setupJjRepoWithWorkspace(t, "1.0.0", "feature-ws")
	// Rename the default workspace from its initial "default" name to
	// "main" so the tie-break key matches DefaultBranch() == "main".
	runIn(t, defaultDir, "jj", "workspace", "rename", "main")
	withCwd(t, wsDir, func() {
		b := &jjBackend{}
		got, err := b.DefaultBranchPath()
		if err != nil {
			t.Fatalf("DefaultBranchPath: %v", err)
		}
		if canonPath(got) != canonPath(defaultDir) {
			t.Errorf("DefaultBranchPath = %q, want %q (main-renamed default workspace)", got, defaultDir)
		}
	})
}

// --- pickDefaultBranchPath unit tests --------------------------------------
//
// Table-driven unit tests for the tie-break helper. Isolated from the
// VCS fixtures so the matrix is cheap to extend.
func TestPickDefaultBranchPath_TieBreak(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		def        string
		candidates []defaultBranchPathCandidate
		wantPath   string
		wantCode   int // 0 = success
	}{
		{
			name:     "zero candidates → exit 4",
			def:      "main",
			wantCode: exitCodeAmbiguous,
		},
		{
			name: "single candidate → that path",
			def:  "main",
			candidates: []defaultBranchPathCandidate{
				{name: "anything", path: "/abs/anything"},
			},
			wantPath: "/abs/anything",
		},
		{
			name: "two candidates, one named after branch → that one wins",
			def:  "main",
			candidates: []defaultBranchPathCandidate{
				{name: "feature-wt", path: "/abs/feature-wt"},
				{name: "main", path: "/abs/main"},
			},
			wantPath: "/abs/main",
		},
		{
			name: "two candidates, none named after branch → exit 5",
			def:  "main",
			candidates: []defaultBranchPathCandidate{
				{name: "a", path: "/abs/a"},
				{name: "b", path: "/abs/b"},
			},
			wantCode: exitCodeNonFastForward,
		},
		{
			name: "two candidates, both named after branch → exit 5",
			def:  "main",
			candidates: []defaultBranchPathCandidate{
				{name: "main", path: "/abs/main-1"},
				{name: "main", path: "/abs/main-2"},
			},
			wantCode: exitCodeNonFastForward,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickDefaultBranchPath(tc.candidates, tc.def)
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				if got != tc.wantPath {
					t.Errorf("path = %q, want %q", got, tc.wantPath)
				}
				return
			}
			if err == nil {
				t.Fatalf("want exit %d, got success path %q", tc.wantCode, got)
			}
			ee, ok := err.(*exitErr)
			if !ok || ee.code != tc.wantCode {
				t.Errorf("err = %v, want exitErr{code=%d}", err, tc.wantCode)
			}
		})
	}
}

// TestParseGitWorktreesForBranch exercises the porcelain parser against
// the documented shape (worktree / HEAD / branch lines, blank-line
// separators) and a couple of edge cases (detached HEAD, bare, missing
// trailing blank line).
func TestParseGitWorktreesForBranch(t *testing.T) {
	t.Parallel()
	out := `worktree /abs/main
HEAD aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
branch refs/heads/main

worktree /abs/feature-wt
HEAD bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
branch refs/heads/feature/x

worktree /abs/detached-wt
HEAD cccccccccccccccccccccccccccccccccccccccc
detached

worktree /abs/main-dup
HEAD dddddddddddddddddddddddddddddddddddddddd
branch refs/heads/main`
	got := parseGitWorktreesForBranch(out, "main")
	want := []defaultBranchPathCandidate{
		{name: "main", path: "/abs/main"},
		{name: "main-dup", path: "/abs/main-dup"},
	}
	if len(got) != len(want) {
		t.Fatalf("parseGitWorktreesForBranch len = %d (%+v), want %d (%+v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
