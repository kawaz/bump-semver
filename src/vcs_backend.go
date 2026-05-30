package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	// DiffNameStatus returns one line per changed file between `rev` and
	// the working copy, formatted as `<CODE>\t<path>\n` (git's native
	// `--name-status` format). CODE is M/A/D (modify / add / delete).
	//
	// Cross-backend uniformity: the git backend forwards
	// `git diff --name-status REV`. The jj backend runs
	// `jj diff --summary --from REV --to @`, whose native format is
	// `<CODE> <path>` (space separator) — the implementation normalizes
	// the first space to a tab so callers get the git form regardless
	// of the underlying backend.
	//
	// Same path-filtering rule as Diff: nonexistent paths are silently
	// dropped, and an all-filtered list returns empty bytes (must not
	// widen back to "all paths").
	//
	// Used by `vcs diff -s/--name-status` for display and by
	// `vcs diff -q/--quiet` to derive presence (len > 0 → diff present).
	// Returning a single shape lets both options share one code path.
	DiffNameStatus(rev string, paths []string) ([]byte, error)

	// Fetch refreshes refs from the named remote (DR-0020 PR-5). Empty
	// remote is a caller bug — the dispatcher always supplies a value
	// (defaulting to "origin" when the user omits one).
	//
	//   - git: `git fetch <remote>`
	//   - jj:  `jj git fetch --remote <remote>`
	//
	// Errors (unknown remote, network failure) are wrapped as
	// *exitErr{code: exitCodeVCSExec} so the dispatcher preserves the
	// exit-code contract.
	Fetch(remote string) error

	// Push uploads opts.name to opts.remote (DR-0020 PR-5). Both fields
	// are required; the dispatcher validates the user supplied a NAME
	// before reaching here (no auto-detection, by design: the human
	// always names the branch / bookmark explicitly so a forgotten
	// `--branch` doesn't push an unintended ref).
	//
	//   - git: `git push <remote> <name>:<name>`. New branches and forward
	//     moves both succeed; divergence yields *exitErr{code:
	//     exitCodeNonFastForward}.
	//   - jj:  `jj git push --bookmark <name> --remote <remote>`. jj 0.41
	//     handles new bookmarks without `--allow-new`. Followed by `jj git
	//     export` so the colocated `.git` refs reflect the push (and the
	//     export's own exit code is propagated, not swallowed). Divergence
	//     ("stale info" / "Failed to push some bookmarks") yields
	//     *exitErr{code: exitCodeNonFastForward}.
	//
	// Force push is intentionally NOT exposed (DR-0020 PR-5 safety):
	// rewriting the remote ref is a separate authoring concern that
	// belongs in the underlying tool, not in a SemVer release helper.
	// The non-ff hint surfaced to the user spells this out.
	//
	// Idempotency: "remote already has it" → nil (DR-0020 0-targets-no-op
	// rule). git emits "Everything up-to-date" with exit 0; jj emits
	// "Nothing changed" with exit 0; both pass through as success.
	Push(opts pushOpts) error

	// Commit records the requested change set into the VCS (DR-0020 PR-4).
	// Modes are encoded in commitOpts; the contract is:
	//
	//   - paths mode (len(paths) > 0): stage + commit exactly those paths'
	//     working-tree contents. Nonexistent paths are silently dropped
	//     (declarative-convergence, same rule as Diff). When the survivor
	//     set produces no actual change vs HEAD/@-, this is a no-op
	//     (returns nil without creating an empty commit).
	//   - staged mode (opts.staged): commit every dirty/staged file at
	//     once (git: --cached, jj: the entire @ snapshot). No real change
	//     → no-op nil. Mirrors DR-0020 "0 targets → exit 0, no action".
	//   - amend mode (opts.amend): fold the current change into the
	//     previous commit (git: --amend; jj: squash @ → @-). With
	//     opts.noEdit (-m absent), the previous commit's message is
	//     preserved verbatim; with opts.message non-empty, the previous
	//     commit's message is replaced. amend is NOT subject to the
	//     empty-no-op rule — message-only amend is a legal explicit
	//     rewrite.
	//
	// The caller is responsible for the message-required check (parser
	// already enforces it for !amend modes). The mode-flags themselves
	// are mutually exclusive at the parser layer; this method assumes a
	// resolved combination (paths XOR staged, with amend orthogonal).
	//
	// Errors are wrapped as *exitErr{code: exitCodeVCSExec} when the
	// underlying subprocess fails so callers preserve the exit-code
	// contract (exit 3 outside a vcs operation context).
	Commit(opts commitOpts) error

	// TagPush creates / moves a tag locally AND pushes it to opts.Remote
	// in a single atomic intent (DR-0020 PR-6). The verb's meaning is "the
	// tag should point to REV on the remote when this returns" — the local
	// create is the means, not the deliverable.
	//
	// Branch logic (shared across backends via resolveTagPushPlan):
	//   - existing tag SHA == REV's SHA   → skip local create, still push
	//     (片落ちリカバリ: remote may be missing it even though local has
	//     it; same-rev push is a clean no-op on the remote when already
	//     there, and a forward-push when only local had it)
	//   - existing tag SHA != REV's SHA, !AllowMove → *exitErr{exitCodeAmbiguous (4)}
	//     with NO side-effect (no local move, no push attempt). Distinct
	//     from generic exitCodeVCSExec so callers can branch on integrity
	//     vs. infrastructure failures.
	//   - existing tag SHA != REV's SHA, AllowMove → force-move locally,
	//     `--force`-push to remote.
	//   - no existing tag                 → create + push.
	//
	// REV-resolution failure → *exitErr{exitCodeVCSExec (3)}, before any
	// side-effect. This keeps "you typed the rev wrong" distinguishable
	// from "your tag has drifted" (4) and "git/jj broke" (also 3 but with
	// the underlying tool's stderr folded in).
	//
	// Force-push choice (DR-0020 PR-6 implementation notes): plain `--force`
	// rather than `--force-with-lease`. The move is already gated behind
	// AllowMove (user-confirmed) and the diff-rev pre-check (we know what
	// we're overwriting), so the lease adds no real protection for tag refs
	// (tags don't have remote-tracking refs, so a bare `--force-with-lease`
	// can't establish a lease and is no safer than `--force`). An explicit
	// lease value `--force-with-lease=refs/tags/NAME:<remote-sha>` would
	// require an extra ls-remote round trip without strengthening the
	// safety story.
	TagPush(opts tagPushOpts) error

	// TagDelete removes a tag from both the local VCS and opts.Remote
	// (DR-0020 PR-6). Delete is natively idempotent (`rm -f` semantic per
	// DR line 74: the verb's intent is the end-state "no tag", and an
	// already-absent tag is an end-state already achieved):
	//
	//   - local half:  `git tag -d` errors on missing tags, so the git
	//                  backend MUST pre-check with `git rev-parse -q
	//                  --verify`. jj's `jj tag delete` is natively
	//                  idempotent.
	//   - remote half: `git push origin :refs/tags/NAME` against a missing
	//                  remote ref reports "deleting a non-existent ref"
	//                  but EXITS 0, so both backends can run it
	//                  unconditionally without breaking idempotence.
	//
	// A genuine remote-side failure (unknown remote name, network down)
	// surfaces as *exitErr{exitCodeVCSExec (3)} — the local half may
	// already have succeeded by then; we accept that asymmetry because
	// the alternative ("only delete locally if remote ack'd") would
	// trade rare clean retries for the common "remote is offline, I just
	// want to clean up my local tags" use case.
	TagDelete(opts tagDeleteOpts) error
}

// tagPushOpts collects the arguments for TagPush. Kept as a struct (vs.
// positional args) so future extensions (e.g. a `--remote-only` flag for
// "I already have the tag locally, only push") plug in without breaking
// existing callers.
type tagPushOpts struct {
	// Name is the tag name (e.g. "v1.2.3"). Required; the dispatcher
	// validates non-empty and screens for surface bugs (refs/-prefix,
	// whitespace) before reaching here.
	Name string
	// Rev is the revision the tag should point at (any git rev-spec / jj
	// revset). Required. Resolution failure → *exitErr{exitCodeVCSExec}.
	Rev string
	// Remote is the named remote to push to. Required; dispatcher
	// defaults to "origin" when the user omits --remote.
	Remote string
	// AllowMove permits moving an existing tag to a different rev. The
	// same-rev idempotent case does NOT need AllowMove (DR-0020 line 71).
	AllowMove bool
	// Stdout / Stderr receive the underlying tool's success-path
	// diagnostic output (mirrors PR-5.1 Push). Backend may use io.Discard
	// when not interested; the dispatcher wires the user's stdout/stderr
	// when -q/-qq are not set.
	Stdout io.Writer
	Stderr io.Writer
}

// tagDeleteOpts collects the arguments for TagDelete. Same justification
// for the struct shape as tagPushOpts.
type tagDeleteOpts struct {
	// Name is the tag name to delete. Required.
	Name string
	// Remote is the named remote to delete from. Required; dispatcher
	// defaults to "origin".
	Remote string
	Stdout io.Writer
	Stderr io.Writer
}

// commitOpts collects the modal arguments for `Commit`. Kept as a
// single struct (rather than method-per-mode) so the interface stays
// compact and future flags (e.g. PR-5's `--allow-empty` if ever needed)
// extend without breaking implementations.
type commitOpts struct {
	// paths is the list of path arguments the user typed. The backend
	// filters out nonexistent entries (declarative-convergence) before
	// committing. Empty paths + !staged + !amend is a caller bug; the
	// parser layer rejects that combination with exit 2 before reaching
	// here.
	paths []string

	// message is the commit message. Required when !amend; with amend it
	// is optional — empty + opts.noEdit means "keep the previous
	// message".
	message string

	// staged switches to "commit everything dirty/staged at once" mode
	// (git: --cached; jj: the full @ snapshot). Mutually exclusive with
	// paths at the parser layer.
	staged bool

	// amend folds the current change into the previous commit
	// (git: --amend; jj: squash --from @ --into @-). PR-4.1 made amend
	// fully symmetric with non-amend in the path selectors it accepts:
	//
	//   - amend + paths           → fold only the listed paths into prev
	//   - amend + staged          → fold the entire staged/dirty set
	//                              (synonym for bare amend in both
	//                              backends since the index / @ snapshot
	//                              IS the absorption source)
	//   - amend, no paths/staged  → bare amend (ungated explicit rewrite)
	//
	// Path-scoped amend follows the same declarative-convergence rule as
	// non-amend path mode (all-nonexistent or no-change → no-op). Bare
	// amend bypasses that gate — message-only rewrite is a legal
	// explicit intent.
	amend bool

	// noEdit applies only with amend: keep the previous commit's
	// message unchanged. Mutually exclusive with a non-empty message at
	// the parser layer.
	noEdit bool
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

// DiffNameStatus on git forwards `git diff --name-status REV [-- PATHS]`.
// git's output is already the contract format (`<CODE>\t<path>\n`), so no
// normalization is needed. Same declarative-convergence path filtering as
// Diff: all-filtered → empty bytes, no git invocation.
func (g *gitBackend) DiffNameStatus(rev string, paths []string) ([]byte, error) {
	args := []string{"diff", "--name-status", rev}
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

// DiffNameStatus on jj runs `jj diff --summary --from REV --to @
// [-- PATHS]` and normalizes the native space separator to a tab so the
// output matches git's `--name-status` shape exactly.
//
// jj summary format: `<CODE> <path>` (single space). We split on the FIRST
// space only — paths with embedded spaces stay intact in the right half.
// Lines that don't match the `<CODE> <path>` shape are passed through
// unchanged (defensive: jj could introduce new prefix forms; we don't want
// to silently mangle them).
//
// Rename / copy codes (R/C) are best-effort: jj and git may differ in how
// they render them, but M/A/D — the cases that matter for the kawaz
// "version bumped?" check — are identical.
func (j *jjBackend) DiffNameStatus(rev string, paths []string) ([]byte, error) {
	args := []string{"diff", "--summary", "--from", rev, "--to", "@"}
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
	return normalizeJjNameStatus(out), nil
}

// normalizeJjNameStatus converts jj's `<CODE> <path>\n` lines into git's
// `<CODE>\t<path>\n` form. The first space on each line becomes a tab; the
// rest of the line is left untouched so paths-with-spaces survive intact.
// Trailing newlines are preserved.
func normalizeJjNameStatus(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	lines := strings.Split(string(in), "\n")
	for i, line := range lines {
		// SplitN with n=2 takes only the first space — paths-with-spaces stay whole.
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && len(parts[0]) > 0 {
			lines[i] = parts[0] + "\t" + parts[1]
		}
	}
	return []byte(strings.Join(lines, "\n"))
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

// --- Commit implementations (DR-0020 PR-4) --------------------------------

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
	existing := filterExistingPaths(opts.paths)
	if len(existing) == 0 {
		return nil // all-nonexistent → no-op success
	}
	// Stage the surviving paths so new (untracked) files become eligible.
	addArgs := append([]string{"add", "--"}, existing...)
	if _, err := runBackendCmd("git", addArgs...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	// Now check whether anything actually changed for the given paths.
	gateArgs := append([]string{"diff", "--cached", "--quiet", "--"}, existing...)
	code, err := runBackendExitCode("git", gateArgs...)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if code == 0 {
		return nil // nothing to commit for these paths → no-op
	}
	// `git commit -m MSG -- PATHS` is a partial commit: only PATHS make it
	// into HEAD, even if other paths are staged. Exactly what we want.
	commitArgs := append([]string{"commit", "-m", opts.message, "--"}, existing...)
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
		existing := filterExistingPaths(opts.paths)
		if len(existing) == 0 {
			return nil // all-nonexistent → no-op success
		}
		addArgs := append([]string{"add", "--"}, existing...)
		if _, err := runBackendCmd("git", addArgs...); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		gateArgs := append([]string{"diff", "--cached", "--quiet", "--"}, existing...)
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
		args = append(args, existing...)
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

// Commit (jj) records the requested change set. See the interface comment
// on `Commit` for the full contract. Implementation notes:
//
//   - paths: `jj commit [FILESETS]... -m MSG` puts only those paths' @
//     changes into a new commit (the rest stays in the new working copy).
//     Pre-gated by `jj diff --summary --from @- --to @ -- PATHS` so an
//     all-nonexistent or no-change set is a no-op (DR-0020 explicitly
//     wants no empty commits — jj would otherwise happily create one).
//   - staged: `jj commit -m MSG` (no paths) commits the entire @ snapshot.
//     Pre-gated by the `empty` template (same predicate as IsClean).
//   - amend (PR-4.1): symmetric with non-amend. With paths,
//     `jj squash --from @ --into @- [-m MSG | -u] -- PATHS` folds only
//     those paths from @ into @-, leaving the rest in @. With staged or
//     bare, drops the `-- PATHS` tail and folds all of @. The no-edit
//     path uses `--use-destination-message` rather than the squash
//     default (which would prompt for a combined description when @ and
//     @- both carry descriptions — observed on jj 0.41 in non-
//     interactive callers and confirmed as the cause of editor-spawn
//     hangs).
func (j *jjBackend) Commit(opts commitOpts) error {
	if opts.amend {
		return j.commitAmend(opts)
	}
	if opts.staged {
		// Gate: is @ empty?
		out, err := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", "empty")
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if strings.TrimSpace(string(out)) == "true" {
			return nil // empty @ → no-op success
		}
		if _, err := runBackendCmd("jj", "commit", "-m", opts.message); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// paths mode.
	existing := filterExistingPaths(opts.paths)
	if len(existing) == 0 {
		return nil
	}
	// Gate via `jj diff --summary` over the same paths: if it produces no
	// output, there is nothing to commit even after path filtering.
	gateArgs := append([]string{"diff", "--summary", "--from", "@-", "--to", "@", "--"}, existing...)
	gateOut, err := runBackendCmd("jj", gateArgs...)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if strings.TrimSpace(string(gateOut)) == "" {
		return nil
	}
	commitArgs := append([]string{"commit", "-m", opts.message}, existing...)
	if _, err := runBackendCmd("jj", commitArgs...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// commitAmend handles jj's amend mode by squashing @ into @-. PR-4.1
// added path / staged symmetry:
//
//   - bare amend (no paths, no staged): explicit rewrite, ungated.
//     `jj squash --from @ --into @- [-m MSG | -u]`. Safe on empty @
//     (message-only amend = description update on @-).
//   - amend + staged: same as bare amend — jj has no separate staging
//     area, the entire @ snapshot IS the absorption source. Accepted
//     as an explicit synonym.
//   - amend + paths: gate via `jj diff --summary --from @- --to @ --
//     PATHS` (same predicate as non-amend path mode), then `jj squash
//     --from @ --into @- [-m MSG | -u] -- PATHS` folds only those
//     paths.
//
// Design rationale (no-edit ⇒ --use-destination-message): when @ has a
// description and ends up empty after squash, bare `jj squash` writes a
// combined description and opens an editor for confirmation. In non-
// interactive callers (bump-semver scripted use) this surfaces as
// "Failed to edit description / Editor 'false' exited with exit
// status: 1" (verified on jj 0.41). `-u` keeps @-'s description
// verbatim — exactly the no-edit semantic — and removes the prompt
// path entirely.
func (j *jjBackend) commitAmend(opts commitOpts) error {
	// Path-scoped amend: gate first so all-nonexistent / no-change is
	// a no-op (declarative convergence, mirrors non-amend path mode).
	if len(opts.paths) > 0 {
		existing := filterExistingPaths(opts.paths)
		if len(existing) == 0 {
			return nil
		}
		gateArgs := append([]string{"diff", "--summary", "--from", "@-", "--to", "@", "--"}, existing...)
		gateOut, err := runBackendCmd("jj", gateArgs...)
		if err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if strings.TrimSpace(string(gateOut)) == "" {
			return nil
		}
		args := []string{"squash", "--from", "@", "--into", "@-"}
		if opts.noEdit || opts.message == "" {
			args = append(args, "--use-destination-message")
		} else {
			args = append(args, "-m", opts.message)
		}
		args = append(args, "--")
		args = append(args, existing...)
		if _, err := runBackendCmd("jj", args...); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		return nil
	}
	// Bare amend (and amend + staged, which is the same operation in
	// jj's auto-staged model).
	args := []string{"squash", "--from", "@", "--into", "@-"}
	if opts.noEdit || opts.message == "" {
		args = append(args, "--use-destination-message")
	} else {
		args = append(args, "-m", opts.message)
	}
	if _, err := runBackendCmd("jj", args...); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// --- Fetch / Push implementations (DR-0020 PR-5) --------------------------

// pushOpts collects the per-call inputs to Push. Kept as a struct (vs a
// pair of positional strings) so future extensions (e.g. a `--push-options
// KEY=VALUE` pass-through) plug in without changing existing callers.
//
// Force push is intentionally not modelled here: the verb does not expose
// `--force` and adding the field invites accidental wiring. See DR-0020
// PR-5 implementation notes for the rationale.
type pushOpts struct {
	// name is the branch (git) / bookmark (jj) name to push. Required;
	// the dispatcher rejects empty names with exit 2.
	name string

	// remote is the named remote to push to. Defaults to "origin" at the
	// dispatcher layer; required to be non-empty here.
	remote string

	// stdout / stderr receive the underlying tool's success-path
	// diagnostic output (DR-0020 PR-5.1). When non-nil and the push
	// succeeds, the backend writes git/jj's own stdout / stderr to
	// these writers so the caller can surface them (e.g. "Everything
	// up-to-date" / "Nothing changed"). nil writers are silently
	// ignored — backend-level tests that only care about exit semantics
	// can leave them unset without leaking diagnostic state across
	// tests (the package previously kept the diagnostic on globals,
	// which made test ordering load-bearing — see DR-0020 PR-5.1
	// implementation notes for the discarded approach).
	//
	// Design rationale: threading io.Writer through pushOpts is the
	// minimal surface change that lets backends emit diagnostic output
	// without growing the vcsBackend method signatures. Push is the
	// only verb today where the underlying tool's success-path output
	// is informational rather than purely structural; if a future verb
	// needs the same treatment, copy this field rather than promoting
	// to the interface.
	stdout io.Writer
	stderr io.Writer
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

// Fetch (jj) refreshes refs from the named remote via `jj git fetch
// --remote <remote>`. Same wrapping as the git variant.
func (j *jjBackend) Fetch(remote string) error {
	if _, err := runBackendCmd("jj", "git", "fetch", "--remote", remote); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	return nil
}

// jjGitExportFunc is the seam tests use to inject deterministic `jj git
// export` outcomes (PR-5.1). Real callers get the default implementation,
// which shells out to `jj git export`. Tests override it to exercise the
// retry-once + recovery-hint paths without needing a fixture that can
// produce a transient-then-clearing failure on demand.
var jjGitExportFunc = func() (stderr string, code int, err error) {
	_, stderrOut, exitCode, runErr := runBackendCapture("jj", "git", "export")
	return stderrOut, exitCode, runErr
}

// Push (jj) uploads opts.name to opts.remote via `jj git push --bookmark
// <name> --remote <remote>`. After a successful push we run `jj git
// export` and propagate its exit code — this keeps the colocated `.git`
// refs in sync and surfaces edge cases (ref-hierarchy conflicts, HEAD
// races) the DR explicitly asks us NOT to swallow.
//
// `--allow-new` is intentionally omitted: jj 0.41 deprecated it in favour
// of remote auto-track configuration, and new bookmarks push fine without
// it in our default config. Future jj versions may flip the default; if
// new-bookmark push starts erroring on a supported version, the fix is
// to switch to `--allow-new` (kept simple here — see DR-0020 PR-5 notes).
//
// Non-ff detection: jj's rejection markers ("stale info", "Failed to push
// some bookmarks") are matched in isNonFastForward. Anything else on
// non-zero exit is a generic VCS error (exit 3).
func (j *jjBackend) Push(opts pushOpts) error {
	stdout, stderr, code, err := runBackendCapture("jj", "git", "push",
		"--bookmark", opts.name, "--remote", opts.remote)
	if err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if code == 0 {
		// PR-5.1: forward jj's success-path diagnostic (e.g. "Nothing
		// changed" / "Changes to push to <remote>" / bookmark moves) so
		// the user sees what jj actually said. Error paths skip
		// passthrough — emitVcsErr already folds stderr into ee.msg.
		writePushDiagnostic(opts.stdout, stdout)
		writePushDiagnostic(opts.stderr, stderr)
	}
	if code != 0 {
		if isNonFastForward(stderr) {
			return &exitErr{
				code: exitCodeNonFastForward,
				msg:  formatPushError("jj", stderr, stdout),
			}
		}
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  formatPushError("jj", stderr, stdout),
		}
	}
	// Push succeeded; sync colocated git refs via `jj git export`.
	// PR-5.1: retry once on failure (the common cases — transient
	// packed-refs lock, HEAD races — are cleared on the second attempt),
	// then escalate to exit 3 with a recovery hint pointing at jj's
	// upstream issues. We don't swallow the underlying jj stderr —
	// kawaz's directive is "公式は無視するなで終わりってことないよね"
	// = give the user an actionable next step instead of a bare wrap.
	exStderr1, exCode1, exErr1 := jjGitExportFunc()
	if exErr1 == nil && exCode1 == 0 {
		return nil
	}
	// First attempt failed; try once more (covers the
	// transient-lock-clears class).
	exStderr2, exCode2, exErr2 := jjGitExportFunc()
	if exErr2 == nil && exCode2 == 0 {
		return nil
	}
	// Both attempts failed — pick the most informative stderr (prefer
	// the second attempt's, which reflects the persistent state) and
	// build a recovery hint.
	finalStderr := strings.TrimSpace(exStderr2)
	if finalStderr == "" {
		finalStderr = strings.TrimSpace(exStderr1)
	}
	if exErr2 != nil {
		finalStderr = strings.TrimSpace(finalStderr + "\n" + exErr2.Error())
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  jjGitExportRecoveryMessage(finalStderr),
	}
}

// writePushDiagnostic emits the underlying tool's success-path output to
// the caller-supplied writer, normalising trailing whitespace and adding
// a single newline. Nil writer is a silent no-op so unit tests at the
// backend layer (which only assert exit semantics) need not wire stub
// writers. Empty input is also a no-op — git/jj sometimes succeed with
// no stderr (e.g. `--quiet` push), and a bare newline would be noise.
func writePushDiagnostic(w io.Writer, s string) {
	if w == nil {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	_, _ = fmt.Fprintln(w, s)
}

// jjGitExportRecoveryMessage builds an actionable error message for a
// persistent `jj git export` failure (PR-5.1). The message folds in the
// raw jj stderr (no paraphrase) and appends pattern-specific remedies
// derived from jj-vcs/jj upstream issues. Unmatched stderr falls back to
// a generic "raw stderr + upstream issue list" body so the user always
// gets a starting point, never a bare wrap.
func jjGitExportRecoveryMessage(jjStderr string) string {
	var hint string
	switch {
	// Ref-hierarchy clash (jj-vcs/jj #493): git's filesystem refs can't
	// hold both `refs/heads/foo` and `refs/heads/foo/bar`.
	case strings.Contains(jjStderr, "there are refs beneath that folder"),
		strings.Contains(jjStderr, "cannot lock ref"):
		hint = "ref-hierarchy clash (jj-vcs/jj #493): " +
			"inspect with 'git for-each-ref refs/heads/', then rename or delete " +
			"the conflicting refs and retry."
	// packed-refs lock not released (jj-vcs/jj #6203).
	case strings.Contains(jjStderr, "packed-refs"):
		hint = "packed-refs lock contention (jj-vcs/jj #6203): " +
			"ensure no other git/jj process is running, remove " +
			"'.git/packed-refs.lock' if stale, then retry."
	// HEAD reference race (jj-vcs/jj #6098).
	case strings.Contains(jjStderr, `reference "HEAD"`),
		strings.Contains(jjStderr, "HEAD\" should have content"):
		hint = "HEAD reference race (jj-vcs/jj #6098): " +
			"run 'jj git import' to resync the working copy with the underlying " +
			"git store, then retry."
	default:
		hint = "see https://github.com/jj-vcs/jj/issues " +
			"(known patterns: #493 ref-hierarchy, #6098 HEAD race, #6203 packed-refs)."
	}
	return fmt.Sprintf("jj git export failed twice after push: %s\nrecovery: %s", jjStderr, hint)
}

// runBackendCapture is the variant of runBackendCmd we need for push:
// the child's exit code is a real signal (= success / non-ff / other), so
// we must read stdout AND stderr AND the code without treating non-zero
// as a Go error. Real exec failures (binary missing, signal) are still
// surfaced via the err return.
func runBackendCapture(name string, args ...string) (stdout, stderr string, code int, err error) {
	cmd := exec.Command(name, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			return stdout, stderr, ee.ExitCode(), nil
		}
		return stdout, stderr, -1,
			fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), runErr)
	}
	return stdout, stderr, 0, nil
}

// nonFfMarkers are substrings observed in real git / jj rejection output
// that uniquely identify a non-fast-forward refusal. Locale-fragile (git's
// hint text can be translated), but the parenthesized markers
// `(fetch first)` / `(non-fast-forward)` and jj's `stale info` /
// `Failed to push some bookmarks` are stable across locales.
//
// Conservative match: anything that doesn't hit one of these falls back
// to exit 3 (generic VCS error). Better to mis-classify a non-ff as a
// generic failure than to mis-classify some other failure (a bad URL, a
// signing error) as non-ff and steer the user toward the wrong remedy.
var nonFfMarkers = []string{
	"(fetch first)",                 // git
	"(non-fast-forward)",            // git
	"stale info",                    // jj
	"Failed to push some bookmarks", // jj
}

// isNonFastForward returns true when stderr contains any of the known
// non-ff markers. Case-sensitive — every marker observed in fixtures
// preserves the original casing.
func isNonFastForward(stderr string) bool {
	for _, m := range nonFfMarkers {
		if strings.Contains(stderr, m) {
			return true
		}
	}
	return false
}

// formatPushError builds the error message body for a failed push, folding
// the underlying tool's stderr in. The caller wraps it in an *exitErr with
// the right code; this helper just keeps the formatting uniform.
func formatPushError(tool, stderr, stdout string) string {
	s := strings.TrimSpace(stderr)
	if s == "" {
		s = strings.TrimSpace(stdout)
	}
	if s == "" {
		return fmt.Sprintf("%s push failed", tool)
	}
	return fmt.Sprintf("%s push failed: %s", tool, s)
}

// --- TagPush / TagDelete implementations (DR-0020 PR-6) -------------------

// jjGitPushDir returns the directory to pass to `git -C` for the
// tag-push step in jj backend operations.
//
// Two layouts are supported (DR-0020 line 105):
//   - colocated:  `.git` is a real directory inside cwd. Push from cwd
//     itself — pushing from inside `.git/` would lose the worktree
//     context that pre-push hooks expect.
//   - non-colocated: `.jj/repo/store/git_target` points to the backing
//     bare repo (typically an absolute path under `~/.local/share/.../`).
//     Bare repos push fine without a worktree, so `git -C <bare>` is
//     correct here.
//
// Errors wrap as *exitErr{exitCodeVCSExec}.
func jjGitPushDir() (string, error) {
	// Colocated check: a `.git` entry in cwd that is a directory wins
	// regardless of what git_target says. Saves us from "git_target's
	// relative path resolved to the same .git but the bare config doesn't
	// reach our hooks" cases.
	if fi, err := os.Stat(".git"); err == nil && fi.IsDir() {
		// Empty dir-arg means "use cwd" downstream — avoids special-casing
		// the worktree/git-dir split.
		return "", nil
	}
	const rel = ".jj/repo/store/git_target"
	raw, err := os.ReadFile(rel)
	if err != nil {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("read .jj/repo/store/git_target: %v", err),
		}
	}
	target := strings.TrimSpace(string(raw))
	if target == "" {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  ".jj/repo/store/git_target is empty",
		}
	}
	if !filepath.IsAbs(target) {
		base := ".jj/repo/store"
		joined := filepath.Join(base, target)
		abs, absErr := filepath.Abs(joined)
		if absErr != nil {
			return "", &exitErr{
				code: exitCodeVCSExec,
				msg:  fmt.Sprintf("resolve %s: %v", joined, absErr),
			}
		}
		return abs, nil
	}
	return target, nil
}

// resolveGitRev returns the commit SHA `rev` resolves to in the cwd git
// repo, or *exitErr{exitCodeVCSExec} on resolution failure.
//
// `^{commit}` peeling ensures annotated tags resolve to their target
// commit (so comparing two refs that one is an annotated tag and the
// other a rev-spec both land on the commit SHA, not the tag-object SHA).
func resolveGitRev(rev string) (string, error) {
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

// resolveJjRev returns the commit SHA `rev` resolves to in the cwd jj
// repo, or *exitErr{exitCodeVCSExec} on resolution failure.
//
// We use `jj log --no-graph -r REV -T commit_id` which (a) prints exactly
// one line per resolved change and (b) emits the canonical 40-char commit
// SHA — same format `git rev-parse` returns so cross-backend SHA
// comparisons stay trivial.
func resolveJjRev(rev string) (string, error) {
	out, err := runBackendCmd("jj", "log", "--no-graph", "-r", rev, "-T", `commit_id ++ "\n"`)
	if err != nil {
		return "", &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	// jj prints one line per matched change. A multi-line result means
	// the revset matched more than one change — treat as ambiguous-like
	// VCS error (the caller wrote a revset that doesn't yield a single
	// commit; jj's own error message would be similar).
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj rev %q resolved to nothing", rev),
		}
	}
	if len(lines) > 1 {
		return "", &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("jj rev %q matched multiple changes", rev),
		}
	}
	return lines[0], nil
}

// existingJjTagSHA returns the commit SHA that NAME points at in jj, or
// "" when the tag is absent. Uses `jj tag list NAME -T` with the
// `self.normal_target().commit_id()` keyword (verified in PR-6 probing on
// jj 0.41). Multi-line output is treated as "not present" so the caller
// proceeds with the create path; a downstream `jj tag set` will surface
// any actual error.
func existingJjTagSHA(name string) string {
	out, err := runBackendCmd("jj", "tag", "list", name,
		"-T", `self.normal_target().commit_id() ++ "\n"`)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) != 1 {
		return ""
	}
	return lines[0]
}

// tagPushDecision captures the shared-logic outcome before any backend
// command runs. The four states map 1-1 to the DR-0020 PR-6 contract
// (lines 71): absent → create+push; same → skip-create+push; diff+!move
// → error 4; diff+move → move+force-push.
type tagPushDecision int

const (
	tagPushDecisionCreate     tagPushDecision = iota // absent locally
	tagPushDecisionSkipCreate                        // exists locally at same REV
	tagPushDecisionMove                              // exists locally at different REV, AllowMove set
	tagPushDecisionReject                            // exists locally at different REV, AllowMove not set
)

// decideTagPush implements the shared-logic decision matrix above. Pure
// function over (existingSHA, targetSHA, allowMove); the backend feeds it
// its own resolved values and acts on the returned decision.
func decideTagPush(existingSHA, targetSHA string, allowMove bool) tagPushDecision {
	switch {
	case existingSHA == "":
		return tagPushDecisionCreate
	case existingSHA == targetSHA:
		return tagPushDecisionSkipCreate
	case allowMove:
		return tagPushDecisionMove
	default:
		return tagPushDecisionReject
	}
}

// formatTagDiffRevError builds the exit-4 message for the rejected
// diff-rev case. The wording calls out --allow-move as the override so
// the user can act on the hint without consulting the help text.
func formatTagDiffRevError(name, existingSHA, targetSHA string) string {
	return fmt.Sprintf(
		"tag %q already points to %s, want %s; pass --allow-move to move it",
		name, shortSHA(existingSHA), shortSHA(targetSHA))
}

// shortSHA truncates a 40-char commit SHA to 12 chars for human-readable
// error messages. Empty or short inputs pass through unchanged.
func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// gitTagPushRemote runs the actual `git push` against opts.Remote for a
// freshly-created or freshly-moved local tag. `force` adds `--force` —
// required for the move case because the remote ref already exists at a
// different value (plain push is rejected with `(already exists)`).
//
// Success-path stdout/stderr is forwarded via writePushDiagnostic
// (matches PR-5.1 Push behaviour). Non-zero exit becomes
// *exitErr{exitCodeVCSExec} with the underlying stderr folded in.
func gitTagPushRemote(opts tagPushOpts, force bool, dir string) error {
	args := []string{"push"}
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
func gitTagDeleteRemote(opts tagDeleteOpts, dir string) error {
	cmd := exec.Command("git", "push", opts.Remote, ":refs/tags/"+opts.Name)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  formatPushError("git",
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

// TagPush (jj): same logic as the git backend but routes the local create
// through `jj tag set`, runs `jj git export` to materialise the tag in
// the underlying git store, then issues a native `git -C <git_target>
// push` against the remote.
//
// Why native git push for the remote half: jj 0.41 has no native
// tag-push command (`jj git push --bookmark` only handles bookmarks,
// and the push-everything `jj git push` is too broad — DR-0020 requires
// per-tag intent). DR-0020 line 70 commits to "create via jj tag set,
// push via native git" so jj retains tag awareness while we get fine-
// grained remote control.
func (j *jjBackend) TagPush(opts tagPushOpts) error {
	targetSHA, err := resolveJjRev(opts.Rev)
	if err != nil {
		return err
	}
	existingSHA := existingJjTagSHA(opts.Name)
	gitTarget, gtErr := jjGitPushDir()
	if gtErr != nil {
		return gtErr
	}
	switch decideTagPush(existingSHA, targetSHA, opts.AllowMove) {
	case tagPushDecisionReject:
		return &exitErr{
			code: exitCodeAmbiguous,
			msg:  formatTagDiffRevError(opts.Name, existingSHA, targetSHA),
		}
	case tagPushDecisionCreate:
		if _, err := runBackendCmd("jj", "tag", "set", opts.Name, "-r", opts.Rev); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, false, gitTarget)
	case tagPushDecisionSkipCreate:
		// Local already has it at the same target — ensure git store has
		// it (export is a no-op if it's already there), then push.
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, false, gitTarget)
	case tagPushDecisionMove:
		if _, err := runBackendCmd("jj", "tag", "set", opts.Name,
			"-r", opts.Rev, "--allow-move"); err != nil {
			return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
		}
		if err := jjGitExportOrWrap(); err != nil {
			return err
		}
		return gitTagPushRemote(opts, true, gitTarget)
	default:
		return &exitErr{
			code: exitCodeVCSExec,
			msg:  fmt.Sprintf("internal: unhandled tag push decision"),
		}
	}
}

// TagDelete (jj): `jj tag delete` is natively idempotent (PR-6 probing on
// jj 0.41 confirms missing-NAME yields "No matching tags" with exit 0),
// so we can run it unconditionally. Export so the git store loses the
// ref, then push the delete to the remote (also idempotent at the git
// layer).
func (j *jjBackend) TagDelete(opts tagDeleteOpts) error {
	if _, err := runBackendCmd("jj", "tag", "delete", opts.Name); err != nil {
		return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
	}
	if err := jjGitExportOrWrap(); err != nil {
		return err
	}
	gitTarget, gtErr := jjGitPushDir()
	if gtErr != nil {
		return gtErr
	}
	return gitTagDeleteRemote(opts, gitTarget)
}

// jjGitExportOrWrap runs `jj git export` via the same seam Push uses, so
// the PR-5.1 retry-once + recovery-hint hardening is shared. The retry
// covers transient packed-refs locks and HEAD races that surfaced in
// PR-5 testing (jj-vcs/jj #493, #6098, #6203). PR-6 reuses the seam
// rather than introducing a parallel export path.
func jjGitExportOrWrap() error {
	exStderr1, exCode1, exErr1 := jjGitExportFunc()
	if exErr1 == nil && exCode1 == 0 {
		return nil
	}
	exStderr2, exCode2, exErr2 := jjGitExportFunc()
	if exErr2 == nil && exCode2 == 0 {
		return nil
	}
	finalStderr := strings.TrimSpace(exStderr2)
	if finalStderr == "" {
		finalStderr = strings.TrimSpace(exStderr1)
	}
	if exErr2 != nil {
		finalStderr = strings.TrimSpace(finalStderr + "\n" + exErr2.Error())
	}
	return &exitErr{
		code: exitCodeVCSExec,
		msg:  jjGitExportRecoveryMessage(finalStderr),
	}
}
