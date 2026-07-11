package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type gitBackend struct{}

func (g *gitBackend) Kind() string { return "git" }

// CommitID resolves rev to its 40-char SHA via git rev-parse, with the
// DR-0031 translateRev applied so jj-style `bookmark@remote` works too.
// Default rev when empty: "HEAD".
func (g *gitBackend) CommitID(rev string) (string, error) {
	if rev == "" {
		rev = "HEAD"
	}
	return resolveGitRev(rev)
}

// Root returns the absolute path to the top-level working tree
// directory via `git rev-parse --show-toplevel`.
func (g *gitBackend) Root() (string, error) {
	out, err := runBackendCmd("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch resolves HEAD via `git symbolic-ref --short HEAD`. A
// detached HEAD (symbolic-ref returns non-zero) is reported as
// exitCodeAmbiguous — there is no single "current branch" to name.
//
// Merge / rebase / cherry-pick / bisect progress detection is deferred
// to later PRs (DR-0020 says these should also be ambiguous, but the
// TDD scope for PR-1 only covers the detached-HEAD path). When those
// scenarios are added we'll layer a `.git/MERGE_HEAD` etc. probe on top.
func (g *gitBackend) CurrentBranch() (string, error) {
	out, err := runBackendCmd("git", "symbolic-ref", "--short", "HEAD")
	if err != nil {
		// symbolic-ref's "fatal: ref HEAD is not a symbolic ref" maps to
		// detached HEAD. Any other error (e.g. corrupted repo) is also
		// reported as ambiguous because we cannot name a branch.
		return "", &exitErr{
			code: exitCodeAmbiguous,
			msg:  fmt.Sprintf("current-branch: %s", strings.TrimPrefix(err.Error(), "git symbolic-ref --short HEAD: ")),
		}
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoteURL resolves `remote` to its raw configured URL (DR-0041). See the
// vcsBackend.RemoteURL interface doc for the full selection contract.
//
// Selection (remote == ""): enumerate via `git remote`, apply
// selectDefaultRemote. Explicit remote: skip straight to `git remote
// get-url <remote>` so an unknown name surfaces git's own "No such
// remote" as exit 3 (not the ambiguous code — the caller named it).
func (g *gitBackend) RemoteURL(remote string) (string, error) {
	if remote == "" {
		out, err := runBackendCmd("git", "remote")
		if err != nil {
			return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		selected, err := selectDefaultRemote(splitAndDedup(string(out)))
		if err != nil {
			return "", err
		}
		remote = selected
	}
	// "--" guards against a remote name that looks like a flag (e.g. one
	// created via `git remote add -- --push URL`). This matters even on
	// the default-selection path above: selectDefaultRemote can hand back
	// such a name without it ever passing through validateRemote (that
	// gate only runs for an explicit --remote), so get-url must defend
	// itself here regardless of how `remote` was chosen.
	out, err := runBackendCmd("git", "remote", "get-url", "--", remote)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(out)), nil
}

// --- jj backend -----------------------------------------------------------

// FetchFile returns `file` at `rev` via `git show <rev>:<file>`.
// `rev` is translated up-front so jj-style refs (`main@origin`) reach
// git as `origin/main` — see translateRev / DR-0031.
func (g *gitBackend) FetchFile(rev, file string) ([]byte, error) {
	rev = translateRev(rev, vcsGit)
	return runBackendCmd("git", "show", rev+":"+file)
}

// ListTags returns every tag known to the local git repo
// (`git tag --list`), deduplicated.
func (g *gitBackend) ListTags() ([]string, error) {
	out, err := runBackendCmd("git", "tag", "--list")
	if err != nil {
		return nil, err
	}
	return splitAndDedup(string(out)), nil
}

// LatestTag picks the SemVer-largest tag from ListTags.
func (g *gitBackend) LatestTag(includePrerelease bool) (string, Version, error) {
	tags, err := g.ListTags()
	if err != nil {
		return "", Version{}, err
	}
	return pickLatestSemverTag(tags, includePrerelease)
}

// IsClean returns true when both `git diff --quiet` (unstaged) and
// `git diff --cached --quiet` (staged) succeed (exit 0). Either check
// reporting exit 1 (= "diff present") flips the answer to dirty.
// Untracked files are NOT considered — `git diff` ignores them by
// design, matching the DR-0020 PR-2 contract.
//
// Both checks are required: editing a file and `git add`-ing it makes
// the workdir match the index, so `--quiet` (no --cached) returns 0 —
// only `--cached` catches the staged-only delta.
func (g *gitBackend) IsClean() (bool, error) {
	for _, args := range [][]string{
		{"diff", "--quiet"},
		{"diff", "--cached", "--quiet"},
	} {
		code, err := runBackendExitCode("git", args...)
		if err != nil {
			return false, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		switch code {
		case 0:
			// this check clean; keep going
		case 1:
			return false, nil
		default:
			return false, &exitErr{
				code: exitCodeVCSExec,
				msg:  fmt.Sprintf("git %s: unexpected exit code %d", strings.Join(args, " "), code),
			}
		}
	}
	return true, nil
}

// --- jj: FetchFile / ListTags / LatestTag ----------------------------------

// Diff returns the patch from `rev` to the current working tree (= the
// one-revision form `git diff <rev>`, which compares REV against the
// worktree including uncommitted changes). When `paths` is supplied,
// we filter to those that exist in the worktree (declarative-convergence)
// and scope the diff to the survivors. All-filtered yields empty bytes
// without invoking git — calling `git diff REV --` with no paths would
// widen back to the full diff.
func (g *gitBackend) Diff(rev string, paths []string, excludes []string) ([]byte, error) {
	rev = translateRev(rev, vcsGit)
	args := []string{"diff", rev}
	if len(paths) > 0 || len(excludes) > 0 {
		pathspec := buildGitPathspec(paths, excludes)
		if pathspec == nil {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err := runBackendCmd("git", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return out, nil
}

// DiffNameStatus on git forwards `git diff --name-status REV [-- PATHS]`.
// git's output is already the contract format (`<CODE>\t<path>\n`), so no
// normalization is needed. Same declarative-convergence path filtering as
// Diff: all-filtered → empty bytes, no git invocation.
func (g *gitBackend) DiffNameStatus(rev string, paths []string, excludes []string) ([]byte, error) {
	rev = translateRev(rev, vcsGit)
	args := []string{"diff", "--name-status", rev}
	if len(paths) > 0 || len(excludes) > 0 {
		pathspec := buildGitPathspec(paths, excludes)
		if pathspec == nil {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, pathspec...)
	}
	out, err := runBackendCmd("git", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return out, nil
}

// buildGitPathspec converts (paths, excludes) into the `--` pathspec args
// passed to `git diff` (DR-0033 phase 2 v2). git's magic pathspec syntax:
//
//   - Include: literal path, glob via :(glob), etc. We pass include patterns
//     unchanged (literal directories, files, or already-prefixed `glob:` after
//     prefix strip via trimGlobPrefix).
//   - Exclude: `:(exclude,glob)<pat>` long form. The short `:!<pat>` does NOT
//     interpret `**` (= fnmatch only); the long form with `glob` magic is
//     required for `**/*_test.go` style patterns (empirically verified on
//     git 2.x).
//
// Return value semantics:
//
//   - nil           = caller should NOT call git (= "all paths filtered" /
//     declarative-convergence rule, mimics the old behavior
//     of filterExistingPaths returning empty).
//   - empty slice   = no pathspec → diff everything.
//   - non-empty     = pass after `--`.
func buildGitPathspec(paths, excludes []string) []string {
	// Include: drop nonexistent literal paths (declarative-convergence).
	var includes []string
	if len(paths) > 0 {
		includes = filterExistingPaths(paths)
		if len(includes) == 0 {
			return nil
		}
		// Strip glob: prefix → pass raw pattern (git pathspec accepts glob
		// natively via :(glob,glob) — but for include we don't need an
		// explicit `glob:` magic; the doublestar pre-expansion (= DR-0024)
		// already converted glob: includes to file lists upstream).
	}
	out := make([]string, 0, len(includes)+len(excludes))
	out = append(out, includes...)
	for _, e := range excludes {
		out = append(out, ":(exclude,glob)"+trimGlobPrefix(e))
	}
	return out
}

// trimGlobPrefix removes a leading `glob:` from the pattern (if present).
// Used by buildGitPathspec when emitting `:(exclude,glob)<pat>` — the
// `glob:` magic name is already implied by the `glob` magic word, so the
// raw pattern is what we want.
func trimGlobPrefix(s string) string {
	return strings.TrimPrefix(s, "glob:")
}

// Commit (git) records the requested change set. See the interface comment
// on `Commit` for the full contract. Implementation notes:
//
//   - paths: `git add -- PATHS` first (handles new files; bare
//     `git diff --quiet HEAD` would miss them — see DR-0020 PR-4 notes),
//     then check `git diff --cached --quiet -- PATHS`; commit only if
//     non-empty.
//   - staged: `git diff --cached --quiet` (no paths) to gate, then
//     `git commit -m MSG` (commits whatever is staged).
//   - amend (PR-4.1): symmetric with non-amend on path selectors. With
//     paths, runs the same `git add -- PATHS` + gate, then
//     `git commit --amend [-m|--no-edit] -- PATHS` (pathspec restricts
//     the rewrite even when the index has unrelated staged content).
//     With staged-only, same as bare `git commit --amend` since the
//     index IS what amend folds. Bare amend is an ungated explicit
//     rewrite (message-only amend is legal).
//
// The no-op rule (no real change → nil) is enforced PRE-commit on every
// non-amend mode AND on amend+paths so DR-0020 "0 targets → exit 0, no
// action" holds. Bare amend bypasses the gate (explicit rewrite).
func (g *gitBackend) Commit(opts commitOpts) error {
	if opts.amend {
		return g.commitAmend(opts)
	}
	if opts.staged {
		// Gate: anything staged at all?
		code, err := runBackendExitCode("git", "diff", "--cached", "--quiet")
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if code == 0 {
			return nil // nothing staged → no-op success
		}
		if _, err := runBackendCmd("git", "commit", "-m", opts.message); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// paths mode.
	// --allow-nonexistent-path: legacy declarative-convergence — filter to
	// filesystem-visible paths first (deleted tracked files are dropped).
	// Default (no flag): forward all paths as-is; git add -A handles
	// modified/new/deleted uniformly and errors on truly unknown paths.
	paths := opts.paths
	if opts.allowNonexistentPath {
		paths = filterExistingPaths(opts.paths)
		if len(paths) == 0 {
			return nil // all-nonexistent → no-op success (legacy behaviour)
		}
	}
	// Stage the paths. -A (= --all) covers modify, add, and delete so that
	// tracked-but-deleted files are staged for removal. With
	// --allow-nonexistent-path we already filtered to existing paths, so -A
	// is harmless there too (no deleted entries remain in the filtered list).
	addArgs := append([]string{"add", "-A", "--"}, paths...)
	if _, err := runBackendCmd("git", addArgs...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	// Now check whether anything actually changed for the given paths.
	gateArgs := append([]string{"diff", "--cached", "--quiet", "--"}, paths...)
	code, err := runBackendExitCode("git", gateArgs...)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if code == 0 {
		return nil // nothing to commit for these paths → no-op
	}
	// `git commit -m MSG -- PATHS` is a partial commit: only PATHS make it
	// into HEAD, even if other paths are staged. Exactly what we want.
	commitArgs := append([]string{"commit", "-m", opts.message, "--"}, paths...)
	if _, err := runBackendCmd("git", commitArgs...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// commitAmend handles git's --amend mode. PR-4.1 made it accept the
// same path selectors as non-amend mode (commit/amend symmetry):
//
//   - bare amend (no paths, no staged): explicit rewrite, ungated.
//     `git commit --amend [-m MSG | --no-edit]`. Message-only amend
//     with a clean index is a legal explicit intent.
//   - amend + staged: same as bare amend — the index is what `--amend`
//     folds. Treated as an explicit synonym for clarity at the verb
//     surface; `staged` does not change the underlying git command.
//   - amend + paths: `git add -- PATHS` (so untracked files become
//     eligible — mirrors non-amend path mode), gate via
//     `git diff --cached --quiet -- PATHS`, then
//     `git commit --amend [-m MSG | --no-edit] -- PATHS`. The pathspec
//     restricts the rewrite to those paths even when the index has
//     unrelated staged content (verified empirically against git 2.x).
func (g *gitBackend) commitAmend(opts commitOpts) error {
	// Path-scoped amend: pre-stage and gate, mirroring non-amend path
	// mode so all-nonexistent / no-change is a no-op.
	if len(opts.paths) > 0 {
		// Apply the same allowNonexistentPath logic as non-amend path mode.
		paths := opts.paths
		if opts.allowNonexistentPath {
			paths = filterExistingPaths(opts.paths)
			if len(paths) == 0 {
				return nil // all-nonexistent → no-op success (legacy behaviour)
			}
		}
		addArgs := append([]string{"add", "-A", "--"}, paths...)
		if _, err := runBackendCmd("git", addArgs...); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		gateArgs := append([]string{"diff", "--cached", "--quiet", "--"}, paths...)
		code, err := runBackendExitCode("git", gateArgs...)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if code == 0 {
			return nil // nothing to fold for these paths → no-op
		}
		args := []string{"commit", "--amend"}
		if opts.noEdit || opts.message == "" {
			args = append(args, "--no-edit")
		} else {
			args = append(args, "-m", opts.message)
		}
		args = append(args, "--")
		args = append(args, paths...)
		if _, err := runBackendCmd("git", args...); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// Bare amend (no paths). `--staged` is an accepted synonym here:
	// git's `--amend` already folds the index, which is exactly what
	// `--staged` names.
	args := []string{"commit", "--amend"}
	if opts.noEdit || opts.message == "" {
		args = append(args, "--no-edit")
	} else {
		args = append(args, "-m", opts.message)
	}
	if _, err := runBackendCmd("git", args...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// Fetch (git) refreshes refs from the named remote via `git fetch <remote>`.
// Network / unknown-remote failures surface as *exitErr{exitCodeVCSExec}.
func (g *gitBackend) Fetch(remote string) error {
	if _, err := runBackendCmd("git", "fetch", remote); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// Push (git) uploads opts.name to opts.remote. We use the explicit
// `<src>:<dst>` refspec form (`name:name`) so the result is unaffected by
// any locally-configured push.default or tracking config — both new
// branches and forward moves go to the same-named ref on the remote.
//
// Non-ff detection: git's rejection stderr matches one of a few well-known
// strings (`(fetch first)`, `(non-fast-forward)`, `[rejected]`). When we
// see any of them on a non-zero exit, we return *exitErr{
// exitCodeNonFastForward}. Anything else is a generic VCS error (exit 3).
// Mirrors CurrentBranch's "unknown failure defaults to a safe code"
// approach.
func (g *gitBackend) Push(opts pushOpts) error {
	// DR-0020 PR-5.2.1 (backend-prefix general rule): opts.jjBookmarkAutoAdvance
	// is a jj-only flag (`--jj-` prefix). On the git backend it is a
	// **silent no-op** — the prefix already tells the user it's a jj-side
	// hook, so git just ignores it and runs a normal push. (PR-5.2
	// originally rejected here at exit 3 as a "should never happen"
	// belt-and-suspenders; PR-5.2.1 drops it since the dispatcher reject
	// is also gone — the new contract is "git ignores --jj-* flags",
	// period.)
	stdout, stderr, code, err := runBackendCapture("git", "push", opts.remote, opts.name+":"+opts.name)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if code == 0 {
		// PR-5.1: forward git's own success-path diagnostic so the user
		// sees "Everything up-to-date" / "* [new branch] main -> main"
		// instead of silent success. On error paths the dispatcher
		// already surfaces stderr via emitVcsErr (formatPushError folds
		// it into ee.msg), so we deliberately skip passthrough there to
		// avoid duplicating the message.
		writePushDiagnostic(opts.stdout, stdout)
		writePushDiagnostic(opts.stderr, stderr)
		return nil
	}
	if isNonFastForward(stderr) {
		return &exitErr{
			code: exitCodeNonFastForward,
			msg:  formatPushError("git", stderr, stdout),
		}
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  formatPushError("git", stderr, stdout),
	}
}

// resolveGitRev returns the commit SHA `rev` resolves to in the cwd git
// repo, or *exitErr{exitCodeVCSExec} on resolution failure.
//
// `^{commit}` peeling ensures annotated tags resolve to their target
// commit (so comparing two refs that one is an annotated tag and the
// other a rev-spec both land on the commit SHA, not the tag-object SHA).
func resolveGitRev(rev string) (string, error) {
	rev = translateRev(rev, vcsGit)
	out, err := runBackendCmd("git", "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(out)), nil
}

// existingGitTagSHA returns the commit SHA that refs/tags/NAME points at,
// or "" when the tag is absent. `-q --verify` makes a missing ref exit 1
// with empty stdout (cleanly distinguished from "weird error"), and the
// `^{commit}` peel keeps annotated tags landing on a commit SHA.
//
// Errors from genuine VCS failures (not "missing", which is the empty-
// string return) bubble up so callers wrap with exitCodeVCSExec.
func existingGitTagSHA(name string) string {
	out, err := runBackendCmd("git", "rev-parse", "-q", "--verify", "refs/tags/"+name+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitTagPushRemote runs the actual `git push` against opts.Remote for a
// freshly-created or freshly-moved local tag. `force` adds `--force` —
// required for the move case because the remote ref already exists at a
// different value (plain push is rejected with `(already exists)`).
//
// When `dir` is non-empty (= the non-colocated jj path's `git -C <bare>`)
// we add `--no-verify` to bypass pre-push hooks. Rationale: pre-push
// hooks are routinely written assuming a worktree (they inspect changed
// files, run linters, etc.), and many `core.hooksPath`-based global hook
// setups fail with "this operation must be run in a work tree" when
// invoked from a bare repo. The tag-push from a bare backing store has
// nothing useful for a worktree-oriented hook to inspect — it's just
// "publish this ref" — so `--no-verify` is the right scope here. The
// colocated path (dir == "") keeps full hook coverage so user
// release-gating hooks still fire there.
//
// Success-path stdout/stderr is forwarded via writePushDiagnostic
// (matches PR-5.1 Push behaviour). Non-zero exit becomes
// *exitErr{exitCodeVCSExec} with the underlying stderr folded in.
func gitTagPushRemote(opts tagPushOpts, force bool, dir string) error {
	args := []string{"push"}
	if dir != "" {
		args = append(args, "--no-verify")
	}
	if force {
		args = append(args, "--force")
	}
	args = append(args, opts.Remote, "refs/tags/"+opts.Name)
	var stdoutBuf, stderrBuf strings.Builder
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	if runErr == nil {
		writePushDiagnostic(opts.Stdout, stdout)
		writePushDiagnostic(opts.Stderr, stderr)
		return nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  formatPushError("git", stderr, stdout),
		}
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  fmt.Sprintf("git %s: %v", strings.Join(args, " "), runErr),
	}
}

// gitTagDeleteRemote runs `git push origin :refs/tags/NAME`. Idempotent by
// virtue of git's own behaviour: a missing remote tag yields "warning:
// deleting a non-existent ref" with exit 0. The only failure path is a
// genuine remote/network error.
//
// Same `--no-verify` rationale as gitTagPushRemote for the bare-context
// (`dir != ""`) path.
func gitTagDeleteRemote(opts tagDeleteOpts, dir string) error {
	args := []string{"push"}
	if dir != "" {
		args = append(args, "--no-verify")
	}
	args = append(args, opts.Remote, ":refs/tags/"+opts.Name)
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return &exitErr{
			code: exitCodeVCSExec,
			msg: formatPushError("git",
				stderrBuf.String(), stdoutBuf.String()),
		}
	}
	writePushDiagnostic(opts.Stdout, stdoutBuf.String())
	writePushDiagnostic(opts.Stderr, stderrBuf.String())
	return nil
}

// TagPush (git): resolve REV → SHA, look up existing tag SHA, decide,
// then create-or-move locally and push.
func (g *gitBackend) TagPush(opts tagPushOpts) error {
	targetSHA, err := resolveGitRev(opts.Rev)
	if err != nil {
		return err
	}
	existingSHA := existingGitTagSHA(opts.Name)
	switch decideTagPush(existingSHA, targetSHA, opts.AllowMove) {
	case tagPushDecisionReject:
		return &exitErr{
			code: exitCodeAmbiguous,
			msg:  formatTagDiffRevError(opts.Name, existingSHA, targetSHA),
		}
	case tagPushDecisionCreate:
		if _, err := runBackendCmd("git", "tag", opts.Name, targetSHA); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return gitTagPushRemote(opts, false, "")
	case tagPushDecisionSkipCreate:
		// Local already has it at the same target — 片落ちリカバリ case.
		// Still issue the push so the remote converges; non-force is
		// safe because we know the local SHA matches what we want.
		return gitTagPushRemote(opts, false, "")
	case tagPushDecisionMove:
		if _, err := runBackendCmd("git", "tag", "-f", opts.Name, targetSHA); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return gitTagPushRemote(opts, true, "")
	default:
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("internal: unhandled tag push decision"),
		}
	}
}

// TagDelete (git): pre-check existence on the local side (git tag -d errors
// on missing), then unconditionally push :refs/tags/NAME to the remote
// (idempotent by git's own behaviour).
func (g *gitBackend) TagDelete(opts tagDeleteOpts) error {
	if existingGitTagSHA(opts.Name) != "" {
		if _, err := runBackendCmd("git", "tag", "-d", opts.Name); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
	}
	return gitTagDeleteRemote(opts, "")
}

// FileTimestamp (git): `git log -1 --format=%ct -- <path>` returns the
// committer timestamp (unix epoch) of the most recent commit touching
// path. Empty output (path untracked / never committed) → 0 (DR-0027
// untracked-as-zero rule, matches the legacy translation-lag pkl
// behaviour).
func (g *gitBackend) FileTimestamp(path string) (int64, error) {
	out, err := runBackendCmd("git", "log", "-1", "--format=%ct", "--", path)
	if err != nil {
		return 0, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, nil
	}
	return parseEpochOrZero(s), nil
}

// IsWorktree reports whether the cwd is inside a linked git worktree
// (= one created via `git worktree add`). The main worktree returns false.
//
// Detection: compare `git rev-parse --git-common-dir` (always the main
// `.git`) against `--git-dir` (per-worktree; the main worktree's `.git`
// is also the common dir, while a linked worktree's `.git/worktrees/<name>`
// differs).
func (g *gitBackend) IsWorktree() (bool, error) {
	common, err := runBackendCmd("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	gitDir, err := runBackendCmd("git", "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return false, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(common)) != strings.TrimSpace(string(gitDir)), nil
}

// WorktreeName returns the current linked worktree's directory basename.
// Returns "" for the main worktree.
//
// Design rationale: git tracks linked worktrees by directory path, not
// by name — there is no `git worktree name` command. The path basename
// is the closest human-readable identifier and matches how the user
// invoked `git worktree add <path>`.
func (g *gitBackend) WorktreeName() (string, error) {
	isWt, err := g.IsWorktree()
	if err != nil {
		return "", err
	}
	if !isWt {
		return "", nil
	}
	root, err := g.Root()
	if err != nil {
		return "", err
	}
	return filepath.Base(root), nil
}

// DefaultBranch resolves the default branch name via `git symbolic-ref
// refs/remotes/origin/HEAD`, with a local-branch fallback (main → master
// → trunk) when the remote HEAD is unset.
func (g *gitBackend) DefaultBranch() (string, error) {
	return resolveDefaultBranchViaGit()
}

// DefaultBranchPath returns the absolute path to the worktree that
// currently has the default branch checked out. See the interface
// comment on DefaultBranchPath for the full contract.
//
// Implementation: parse `git worktree list --porcelain` (one record per
// blank-line-separated paragraph; each record has a `worktree <path>`
// header and a `branch refs/heads/<name>` line when on a branch). Filter
// to records whose branch matches DefaultBranch(), then apply the shared
// tie-break (pickDefaultBranchPath).
func (g *gitBackend) DefaultBranchPath() (string, error) {
	def, err := g.DefaultBranch()
	if err != nil {
		return "", err
	}
	out, err := runBackendCmd("git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	candidates := parseGitWorktreesForBranch(string(out), def)
	return pickDefaultBranchPath(candidates, def)
}

// parseGitWorktreesForBranch extracts worktree entries from `git worktree
// list --porcelain` output that have `defaultBranch` checked out. Each
// porcelain record looks like:
//
//	worktree /abs/path
//	HEAD <40-char-sha>
//	branch refs/heads/<name>
//
// Records are blank-line-separated; the last record may have no trailing
// blank line. Detached HEAD records carry `detached` instead of `branch`;
// bare records carry `bare`. Only `branch refs/heads/<defaultBranch>` is a
// candidate.
func parseGitWorktreesForBranch(out, defaultBranch string) []defaultBranchPathCandidate {
	want := "refs/heads/" + defaultBranch
	var candidates []defaultBranchPathCandidate
	var curPath, curBranch string
	flush := func() {
		if curPath != "" && curBranch == want {
			candidates = append(candidates, defaultBranchPathCandidate{
				name: filepath.Base(curPath),
				path: curPath,
			})
		}
		curPath = ""
		curBranch = ""
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			curPath = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			curBranch = strings.TrimPrefix(line, "branch ")
		}
	}
	flush() // tail record without trailing blank line
	return candidates
}

// IsOnDefaultBranch reports whether CurrentBranch() == DefaultBranch().
// Detached HEAD (ambiguous current) is "not on default" rather than an
// error — the predicate's contract is a boolean.
func (g *gitBackend) IsOnDefaultBranch() (bool, error) {
	return isOnDefaultBranchCommon(g)
}

// Promote advances the default branch to opts.Rev (default: HEAD) via
// `git update-ref` with an explicit fast-forward check.
//
// Design rationale: `git push . <rev>:refs/heads/<default>` is rejected
// by git when another worktree has the default branch checked out
// (`receive.denyCurrentBranch=refuse`, the default) — the very scenario
// promote exists to support (= you are in a linked worktree, default
// branch is checked out by the main worktree). `update-ref` bypasses
// the receive hook because it doesn't go through the push protocol;
// the other worktree's index/workdir are deliberately not touched —
// the user sees "branch is ahead" in `git status` until they pull /
// reset, which is the correct end state. The ancestor check enforces
// fast-forward-only by hand.
func (g *gitBackend) Promote(opts promoteOpts) error {
	def, err := g.DefaultBranch()
	if err != nil {
		return err
	}
	rev := opts.Rev
	if rev == "" {
		rev = "HEAD"
	}
	sha, err := resolveGitRev(rev)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	defRef := "refs/heads/" + def
	// Capture the current default-branch SHA (if the ref exists) so the
	// update-ref call can pin the expected old value — that turns the
	// write into a compare-and-swap, defending against a concurrent
	// move by another worktree between this read and the write.
	defSHAOut, defErr := runBackendCmd("git", "rev-parse", "--verify", defRef)
	defExists := defErr == nil
	defSHA := strings.TrimSpace(string(defSHAOut))
	if defExists && defSHA == sha {
		// Already there — no-op.
		writePushDiagnostic(opts.Stdout, fmt.Sprintf("%s already at %s", def, sha[:12]))
		return nil
	}
	if defExists {
		// Ancestor check: is defSHA an ancestor of sha?
		code, err := runBackendExitCode("git", "merge-base", "--is-ancestor", defSHA, sha)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if code != 0 {
			return &exitErr{
				code: exitCodeNonFastForward,
				msg: fmt.Sprintf("promote: %s (%s) is not an ancestor of %s (%s); would be non-fast-forward (run `bump-semver vcs sync --onto %s@origin` first)",
					def, defSHA[:12], rev, sha[:12], def),
			}
		}
	}
	args := []string{"update-ref", defRef, sha}
	if defExists {
		args = append(args, defSHA)
	}
	if _, err := runBackendCmd("git", args...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	writePushDiagnostic(opts.Stdout, fmt.Sprintf("Moved %s to %s", def, sha[:12]))
	return nil
}

// BookmarkSet writes refs/heads/<Name> so it points at the SHA opts.Rev
// resolves to. Mirrors Promote's update-ref approach (= bypasses
// receive.denyCurrentBranch for cross-worktree updates) but takes Name
// explicitly and accepts opts.AllowBackwards for non-FF moves.
func (g *gitBackend) BookmarkSet(opts bookmarkSetOpts) error {
	rev := opts.Rev
	if rev == "" {
		rev = "HEAD"
	}
	sha, err := resolveGitRev(rev)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	ref := "refs/heads/" + opts.Name
	// Capture the current SHA (if present) so update-ref can pin the
	// expected old value — that turns the write into a compare-and-swap,
	// defending against a concurrent move by another worktree between
	// this read and the write.
	curOut, curErr := runBackendCmd("git", "rev-parse", "--verify", ref)
	exists := curErr == nil
	curSHA := strings.TrimSpace(string(curOut))
	if exists && curSHA == sha {
		// Already there — no-op.
		writePushDiagnostic(opts.Stdout, fmt.Sprintf("%s already at %s", opts.Name, sha[:12]))
		return nil
	}
	if exists && !opts.AllowBackwards {
		// FF check: is curSHA an ancestor of sha?
		code, err := runBackendExitCode("git", "merge-base", "--is-ancestor", curSHA, sha)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if code != 0 {
			return &exitErr{
				code: exitCodeNonFastForward,
				msg: fmt.Sprintf("bookmark set: %s (%s) is not an ancestor of %s (%s); would be non-fast-forward (use --allow-backwards to override)",
					opts.Name, curSHA[:12], rev, sha[:12]),
			}
		}
	}
	args := []string{"update-ref", ref, sha}
	if exists {
		args = append(args, curSHA)
	}
	if _, err := runBackendCmd("git", args...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if exists {
		writePushDiagnostic(opts.Stdout, fmt.Sprintf("Moved %s to %s", opts.Name, sha[:12]))
	} else {
		writePushDiagnostic(opts.Stdout, fmt.Sprintf("Created %s at %s", opts.Name, sha[:12]))
	}
	return nil
}

// Sync rebases the current branch onto opts.Onto via `git rebase <onto>`.
func (g *gitBackend) Sync(opts syncOpts) error {
	if opts.Onto == "" {
		return &exitErr{code: exitCodeUsage, msg: "sync: --onto is required"}
	}
	stdout, stderr, code, runErr := runBackendCapture("git", "rebase", opts.Onto)
	if runErr != nil {
		return &exitErr{code: exitCodeVCSExec, msg: runErr.Error()}
	}
	if code != 0 {
		joined := strings.TrimSpace(stderr)
		if joined == "" {
			joined = strings.TrimSpace(stdout)
		}
		return &exitErr{code: exitCodeVCSExec, msg: "git rebase: " + joined}
	}
	writePushDiagnostic(opts.Stdout, stdout)
	return nil
}

// CountCommitsSince (git): `git rev-list --count HEAD --since=@<ts+1> --
// <path>`. We pass `--since` with a strict +1 second so the boundary
// commit (= the one that established sinceTS) is excluded; only
// strictly-newer source-touching commits contribute.
//
// `@<unix-epoch>` is git's documented epoch literal (parsed by
// approxidate identically to a bare integer in current versions, but
// the `@`-prefixed form is the explicit, version-stable spelling).
//
// `sinceTS == 0` means the derived path is untracked, so we want the
// total count of commits that touched the source — drop `--since`.
func (g *gitBackend) CountCommitsSince(path string, sinceTS int64) (int, error) {
	args := []string{"rev-list", "--count"}
	if sinceTS > 0 {
		args = append(args, fmt.Sprintf("--since=@%d", sinceTS+1))
	}
	args = append(args, "HEAD", "--", path)
	out, err := runBackendCmd("git", args...)
	if err != nil {
		return 0, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, nil
	}
	return int(parseEpochOrZero(s)), nil
}
