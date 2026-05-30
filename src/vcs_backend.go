package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// vcsBackend is the unified interface used by the `vcs` subcommand
// family (DR-0020). It abstracts the few VCS operations every verb
// needs, with git and jj as the two concrete implementations.
//
// Design philosophy:
//   - The interface is grown incrementally as PRs land verbs. PR-1
//     adds only what `vcs get` needs (Root / Kind / CurrentBranch).
//   - Errors from CurrentBranch (and future ambiguity-prone reads)
//     carry an exit code via *exitErr so the caller doesn't have to
//     pattern-match on error strings.
//   - The factory `newVcsBackend` accepts a `vcsKind` override (= the
//     parsed `--vcs` flag value, see DR-0008). Empty / vcsAuto means
//     "probe cwd ancestors", mirroring the long-standing behaviour
//     in detectVcs.
type vcsBackend interface {
	// Kind returns "git" or "jj" — the canonical name surfaced by
	// `vcs get backend`.
	Kind() string

	// Root returns the repository root as an absolute path.
	Root() (string, error)

	// CurrentBranch returns the unambiguous current branch / bookmark.
	// Ambiguity (detached HEAD, multiple bookmarks at @, zero
	// bookmarks in ancestors) returns *exitErr{code: exitCodeAmbiguous}
	// so callers can preserve the exit-code contract.
	CurrentBranch() (string, error)

	// FetchFile reads the contents of `file` at revision `rev` from
	// the underlying VCS. Replaces the free function vcsFetchFile.
	FetchFile(rev, file string) ([]byte, error)

	// ListTags returns every tag known to the local VCS, in whatever
	// order the VCS reports them. Caller filters / sorts.
	// Use latestTagFromRemote for remote queries — those are always
	// git ls-remote regardless of the local backend.
	ListTags() ([]string, error)

	// LatestTag returns the SemVer-largest tag known to the local VCS.
	// Non-semver tag names are silently skipped (mirrors DR-0008).
	LatestTag() (Version, error)

	// IsClean reports whether the worktree has no uncommitted changes
	// requiring action (DR-0020 PR-2). Definitions per backend:
	//
	//   - git: `git diff --quiet` (unstaged) AND `git diff --cached
	//     --quiet` (staged). **Untracked files are intentionally
	//     ignored** (mirrors `git diff`'s default; the future
	//     `--include-untracked` option would be opt-in).
	//   - jj: the working-copy change `@` is empty (`jj log -r @ -T
	//     'empty'` == "true"). Because jj snapshots automatically,
	//     new files become part of `@` and DO render the worktree
	//     dirty. This asymmetry vs git is intrinsic to jj's design
	//     (see DR-0020 PR-2 implementation notes).
	//
	// Returns (true, nil) for clean, (false, nil) for dirty. Errors are
	// wrapped as *exitErr{code: exitCodeVCSExec} so unexpected backend
	// failures don't degrade into "dirty".
	IsClean() (bool, error)

	// Diff returns the patch between `rev` and the working copy. When
	// `paths` is non-empty, callers should pass the original (user-named)
	// list; the backend is responsible for filtering out paths that don't
	// exist in the worktree (declarative-convergence rule, DR-0020 PR-3):
	//
	//   - paths == nil / len 0           → full diff (rev .. working copy)
	//   - some paths exist after filter  → diff scoped to the survivors
	//   - all paths filter to zero       → empty []byte, nil error
	//     (must NOT fall through to "all paths" — that would silently
	//     widen the user's request)
	//
	// Errors (VCS subprocess failure, bad REV) are returned as
	// *exitErr{code: exitCodeVCSExec} so callers preserve the exit-code
	// contract.
	Diff(rev string, paths []string) ([]byte, error)
}

// newVcsBackend resolves the `--vcs` override (or auto-probe) into a
// concrete backend. The probe walks cwd's ancestors looking for `.jj`
// (priority) or `.git`, matching DR-0008's precedence.
//
// On failure (no override, no marker found) we return an *exitErr with
// exitCodeVCSExec — "not a VCS repo" is a VCS-layer condition, not a
// usage error, and we want shells to be able to distinguish.
func newVcsBackend(override vcsKind) (vcsBackend, error) {
	kind, err := detectVcs(override)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	switch kind {
	case vcsJj:
		return &jjBackend{}, nil
	case vcsGit:
		return &gitBackend{}, nil
	default:
		// Defensive — detectVcs never returns vcsAuto on success.
		return nil, &exitErr{code: exitCodeVCSExec, msg: "vcs not detected"}
	}
}

// --- git backend ----------------------------------------------------------

type gitBackend struct{}

func (g *gitBackend) Kind() string { return "git" }

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

// --- jj backend -----------------------------------------------------------

type jjBackend struct{}

func (j *jjBackend) Kind() string { return "jj" }

// Root returns the absolute path to the jj working copy via `jj root`.
func (j *jjBackend) Root() (string, error) {
	out, err := runBackendCmd("jj", "root")
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the bookmark that uniquely names the current
// commit's nearest ancestor head (DR-0020):
//
//	jj log -r 'heads(::@ & bookmarks())' --no-graph \
//	  -T 'bookmarks.map(|b| b.name()).join("\n") ++ "\n"'
//
// The template gives one bookmark name per line per head. Behaviour:
//
//   - 1 unique name on 1 head → success
//   - 0 lines (no bookmark in ancestors)         → exitCodeAmbiguous
//   - >1 lines (multiple bookmarks at the head)  → exitCodeAmbiguous
//   - >1 heads (parallel branches in ancestors)  → exitCodeAmbiguous
//
// We deliberately collapse "multiple heads" into the same exit code as
// the other ambiguity cases: the contract is "single name or error",
// and the caller doesn't need to disambiguate the failure mode.
func (j *jjBackend) CurrentBranch() (string, error) {
	out, err := runBackendCmd("jj", "log", "-r", "heads(::@ & bookmarks())",
		"--no-graph", "-T", `bookmarks.map(|b| b.name()).join("\n") ++ "\n"`)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	names := make([]string, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		names = append(names, s)
	}
	switch len(names) {
	case 0:
		return "", &exitErr{
			code: exitCodeAmbiguous,
			msg:  "current-branch: no bookmark found in ancestors of @",
		}
	case 1:
		return names[0], nil
	default:
		return "", &exitErr{
			code: exitCodeAmbiguous,
			msg:  fmt.Sprintf("current-branch: ambiguous (multiple bookmarks at head: %s)", strings.Join(names, ", ")),
		}
	}
}

// runBackendCmd is the shared subprocess helper for backend methods.
// Output() + folded stderr keeps subprocess diagnostics intact (the
// jj/git native messages are almost always more accurate than anything
// we could rephrase).
func runBackendCmd(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			return nil, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errOut)
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// runBackendExitCode is a runBackendCmd variant that returns the child's
// exit code instead of treating non-zero as an error. Required for
// commands whose exit code is a normal signal (e.g. `git diff --quiet`
// uses 0/1 for clean/dirty; routing those through runBackendCmd would
// misclassify "dirty" as a VCS error).
//
// A real exec failure (binary missing, signal, etc.) is still returned
// as an error with stderr folded in for diagnostics.
func runBackendExitCode(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			return -1, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errOut)
		}
		return -1, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return 0, nil
}

// --- git: FetchFile / ListTags / LatestTag ---------------------------------

// FetchFile returns `file` at `rev` via `git show <rev>:<file>`.
func (g *gitBackend) FetchFile(rev, file string) ([]byte, error) {
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
func (g *gitBackend) LatestTag() (Version, error) {
	tags, err := g.ListTags()
	if err != nil {
		return Version{}, err
	}
	return pickLatestSemverTag(tags)
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

// FetchFile returns `file` at `rev` via `jj file show`. When `rev`
// looks like `<remote>/<bookmark>` (a git-style remote ref) we
// transparently retry as jj's native `<bookmark>@<remote>` form on
// failure — git users habitually write `origin/main` and the fallback
// keeps that ergonomic. See altJjRev for the mapping.
func (j *jjBackend) FetchFile(rev, file string) ([]byte, error) {
	out, err := runBackendCmd("jj", "file", "show", "-r", rev, file)
	if err == nil {
		return out, nil
	}
	if alt, ok := altJjRev(rev); ok {
		if out2, err2 := runBackendCmd("jj", "file", "show", "-r", alt, file); err2 == nil {
			return out2, nil
		}
	}
	return nil, err
}

// ListTags returns every tag known to the local jj repo. The template
// emits one tag name per line per change with tags; the dedup pass
// collapses duplicates from changes that share a tag.
//
// We do not run `jj git fetch` here — DR-0008 makes "no implicit
// network calls" an explicit decision.
func (j *jjBackend) ListTags() ([]string, error) {
	out, err := runBackendCmd("jj", "log", "-r", "tags()", "--no-graph",
		"-T", `tags.map(|t| t.name() ++ "\n").join("")`)
	if err != nil {
		return nil, err
	}
	return splitAndDedup(string(out)), nil
}

// LatestTag picks the SemVer-largest tag from ListTags.
func (j *jjBackend) LatestTag() (Version, error) {
	tags, err := j.ListTags()
	if err != nil {
		return Version{}, err
	}
	return pickLatestSemverTag(tags)
}

// Diff returns the patch from `rev` to the current working tree (= the
// one-revision form `git diff <rev>`, which compares REV against the
// worktree including uncommitted changes). When `paths` is supplied,
// we filter to those that exist in the worktree (declarative-convergence)
// and scope the diff to the survivors. All-filtered yields empty bytes
// without invoking git — calling `git diff REV --` with no paths would
// widen back to the full diff.
func (g *gitBackend) Diff(rev string, paths []string) ([]byte, error) {
	args := []string{"diff", rev}
	if len(paths) > 0 {
		existing := filterExistingPaths(paths)
		if len(existing) == 0 {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, existing...)
	}
	out, err := runBackendCmd("git", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return out, nil
}

// Diff returns the patch between `rev` and `@` (jj's working copy). Same
// declarative-convergence path filter as the git backend — see the
// gitBackend.Diff comment for the contract.
func (j *jjBackend) Diff(rev string, paths []string) ([]byte, error) {
	args := []string{"diff", "--from", rev, "--to", "@"}
	if len(paths) > 0 {
		existing := filterExistingPaths(paths)
		if len(existing) == 0 {
			return nil, nil
		}
		args = append(args, "--")
		args = append(args, existing...)
	}
	out, err := runBackendCmd("jj", args...)
	if err != nil {
		return nil, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return out, nil
}

// filterExistingPaths returns the subset of `paths` that os.Stat resolves.
// kawaz's declarative-convergence rule: nonexistent paths are silently
// dropped rather than erroring. This means `vcs diff REV a b c` succeeds
// when only some of `a b c` exist, and converges to "empty" only when none
// do (the caller is responsible for the no-op short-circuit).
//
// Caveat: a path present in REV but deleted in @ is not surfaced when the
// user names it explicitly (os.Stat misses it). The full diff (no paths
// supplied) still shows the deletion. See DR-0020 PR-3 implementation
// notes — this is the intentional scope of declarative-convergence.
func filterExistingPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// IsClean returns true when the working-copy change `@` is empty.
//
// jj's `empty` template keyword renders the literal string "true" or
// "false" — no diff text parsing needed. Reading `@` is also what
// triggers jj's automatic snapshot, so this implicitly reflects any
// just-edited (or just-created) files in the worktree.
//
// Contrast with git: jj treats new files as worktree state by design,
// so an untracked-new-file makes `IsClean` return false (intentional
// asymmetry, documented in DR-0020 PR-2).
func (j *jjBackend) IsClean() (bool, error) {
	out, err := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "empty")
	if err != nil {
		return false, &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	switch strings.TrimSpace(string(out)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj log -r @ -T empty: unexpected output %q", strings.TrimSpace(string(out))),
		}
	}
}
