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
