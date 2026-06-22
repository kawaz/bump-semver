package main

import (
	"errors"
	"fmt"
	"io"
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

	// CommitID resolves `rev` to the canonical 40-char git commit SHA.
	// `rev` accepts any backend-native form (jj revset / git rev-spec)
	// and is normalized via translateRev (DR-0031), so cross-backend
	// forms (e.g. `origin/main` ↔ `main@origin`) work uniformly.
	//
	// Ambiguous resolution (revset matching multiple commits / unresolvable
	// rev) returns *exitErr{exitCodeVCSExec}.
	CommitID(rev string) (string, error)

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
	// When includePrerelease is false, pre-release tags are filtered
	// out (default for `vcs tag latest`); pass true to include them.
	// Returns the raw tag string (preserving the source's prefix
	// style, e.g. `v1.2.3` / `release-1.2.3`), the parsed Version,
	// and any error.
	LatestTag(includePrerelease bool) (string, Version, error)

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
	Diff(rev string, paths []string, excludes []string) ([]byte, error)

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
	DiffNameStatus(rev string, paths []string, excludes []string) ([]byte, error)

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

	// FileTimestamp returns the unix-epoch committer timestamp of the
	// most recent commit that touched `path`, or 0 when the path is not
	// tracked (= no committer history). Used by `vcs outdated` (DR-0027)
	// to compare freshness of a derived file against its source(s).
	//
	// The 0-for-untracked normalization mirrors the legacy translation
	// check (pkf-tasks/tasks/docs/translations.pkl): untracked files
	// behave as "infinitely stale", so a derived path that hasn't been
	// committed yet is treated as older than any committed source —
	// which is the right answer when the source moved on but the
	// generator hasn't run yet.
	//
	// Errors from the underlying VCS (subprocess failures, not a repo)
	// surface as *exitErr{exitCodeVCSExec}; "path exists in cwd but is
	// untracked" is NOT an error (= the 0 case above).
	//
	//   - git: `git log -1 --format=%ct -- <path>`
	//   - jj:  `jj log --no-graph -T 'committer.timestamp().format("%s")'
	//          -r 'latest(::@ & files("<path>"))'`
	FileTimestamp(path string) (int64, error)

	// CountCommitsSince returns the number of commits in the source's
	// history that committer-touch `path` AND are strictly newer than
	// `sinceTS` (unix epoch). Used by `vcs outdated --explain` to
	// surface a "N commits behind" diagnostic.
	//
	// The "behind" count is a coarse-but-cheap summary: it answers the
	// question "how many source-touching commits has the derived path
	// fallen behind?" without walking the derived file's own history.
	// This is intentionally informational; the stale/fresh decision is
	// the ts comparison itself (= `tgt_ts < src_ts`), not the count.
	//
	// `sinceTS == 0` (= derived untracked) returns the total count of
	// commits that ever touched `path` — a useful "you've never run
	// the generator" signal.
	//
	//   - git: `git rev-list --count --since=<ts+1> -- <path>` (with the
	//          +1 to exclude the boundary commit, matching strict-newer
	//          semantics)
	//   - jj:  count of `description(...) ~ none() & files("<path>") &
	//          committer_date(after:"@<ts+1>")` revset
	CountCommitsSince(path string, sinceTS int64) (int, error)

	// IsWorktree reports whether the cwd is inside a secondary worktree
	// (git: linked worktree) / non-default workspace (jj: a workspace
	// added via `jj workspace add`). The main worktree / default
	// workspace returns false.
	//
	//   - git: compare `--git-common-dir` and `--git-dir`. Different =
	//     linked worktree.
	//   - jj: the workspace root != the repo root (the default workspace
	//     by convention sits at the repo root).
	//
	// Errors (subprocess failure, not a repo) surface as
	// *exitErr{exitCodeVCSExec}.
	IsWorktree() (bool, error)

	// WorktreeName returns the current worktree / workspace name. The
	// default workspace / main worktree returns "" (empty). Used by hint
	// messages and `just push` gates to surface the worktree the user is
	// currently in.
	//
	//   - git: linked worktree path's basename; "" for the main worktree.
	//   - jj: the workspace name as reported by `jj workspace list` for
	//     the current workspace; "" for the default workspace.
	WorktreeName() (string, error)

	// DefaultBranch returns the canonical default branch / bookmark name
	// (e.g. "main", "master", "trunk"). The resolution is remote-derived
	// when available, falling back to a local heuristic.
	//
	//   - git: `git symbolic-ref refs/remotes/origin/HEAD` (the canonical
	//     answer set by `git clone` / `git remote set-head`).
	//   - jj: same git symbolic-ref lookup against the colocated `.git`
	//     bare repo (jj workspaces always have access to a git view).
	//
	// When the remote HEAD is unset (`fatal: ref refs/remotes/origin/HEAD
	// is not a symbolic ref`), the backend falls back to probing local
	// branches in order: main, master, trunk. Returns *exitErr{exitCodeVCSExec}
	// when none exist.
	DefaultBranch() (string, error)

	// IsOnDefaultBranch reports whether the current branch / bookmark
	// equals the default branch.
	//
	//   - git: CurrentBranch() == DefaultBranch().
	//   - jj: the closest bookmark to @ equals DefaultBranch(). Empty
	//     working copies (`@` with no local bookmark) inherit from `@-`'s
	//     bookmark.
	//
	// Returns (false, nil) when CurrentBranch is ambiguous (detached HEAD,
	// no bookmark) — these are not "on default", but neither are they
	// errors at the predicate layer.
	IsOnDefaultBranch() (bool, error)

	// Promote advances the default branch / bookmark to point at the
	// commit the user is currently building on. Push is NOT performed
	// (the verb is intentionally orthogonal to `vcs push` so the
	// `sync → promote → push` cascade reads as three explicit steps).
	//
	//   - git: `git update-ref refs/heads/<default> <currentSHA>`. The
	//     dispatcher pre-checks that <currentSHA> is a descendant of
	//     <default> (`git merge-base --is-ancestor`) so the move is
	//     fast-forward-only by construction; non-FF cases return
	//     *exitErr{exitCodeNonFastForward}.
	//   - jj: `jj bookmark set <default> -r <rev>` with `--allow-backwards`
	//     omitted so the move is forward-only. The default `<rev>` is
	//     `@-` (the parent of the working copy) — the jj convention is
	//     `@` is the next throw-away change, so `@-` is the confirmed
	//     content the bookmark should sit on. opts.Rev overrides this.
	//
	// Errors (no default branch, non-FF, ambiguous current branch) surface
	// as *exitErr with the corresponding code.
	Promote(opts promoteOpts) error

	// Sync rebases the current worktree / workspace onto opts.Onto. The
	// `--onto` is required (no default-推論 — the caller names it
	// explicitly to keep the side-effect predictable).
	//
	//   - git: `git rebase <onto>` on the current branch.
	//   - jj: `jj rebase -d <onto>` against the entire workspace.
	//
	// Conflict resolution is left to the user — the verb propagates the
	// underlying tool's conflict exit (typically *exitErr{exitCodeVCSExec}).
	Sync(opts syncOpts) error
}

// promoteOpts collects the per-call inputs to Promote. Mirrors the struct
// shape of pushOpts / tagPushOpts so future extensions plug in without
// changing existing callers.
type promoteOpts struct {
	// Rev is the source revision the default branch / bookmark should
	// move to. Empty → backend default (git: HEAD, jj: @-).
	Rev string

	// Stdout / Stderr receive the underlying tool's diagnostic output.
	// nil writers are silently ignored — backend tests that only assert
	// exit semantics can leave them unset.
	Stdout io.Writer
	Stderr io.Writer
}

// syncOpts collects the per-call inputs to Sync.
type syncOpts struct {
	// Onto is the target revision the current worktree should be rebased
	// onto. Required; the dispatcher rejects empty values with exit 2.
	Onto string

	Stdout io.Writer
	Stderr io.Writer
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
	// paths is the list of path arguments the user typed. By default the
	// backend forwards all paths as-is to the VCS (deleted tracked files
	// are committed as deletions; truly unknown paths cause a VCS error).
	// When allowNonexistentPath is set the backend filters out
	// filesystem-absent entries first (legacy declarative-convergence).
	// Empty paths + !staged + !amend is a caller bug; the parser layer
	// rejects that combination with exit 2 before reaching here.
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

	// allowNonexistentPath, when true, silently drops paths that do not
	// exist on the filesystem before handing them to the VCS backend
	// (legacy declarative-convergence / bump behaviour). When false
	// (the default), all supplied paths are forwarded to git/jj as-is;
	// the VCS itself errors on truly unknown paths while naturally
	// handling tracked-but-deleted files.
	allowNonexistentPath bool
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

	// jjBookmarkAutoAdvance (DR-0020 PR-5.2) is the opt-in
	// --jj-bookmark-auto-advance flag. When true on the jj backend, Push
	// first runs a clean → exists → ancestor → forward-move pre-step that
	// advances the named bookmark to @- before the actual push, so the
	// jj-慣習-conformant "bookmark sits on the confirmed commit (@-), @ is
	// the throw-away working copy" layout is reached structurally rather
	// than by remembering a manual `jj bookmark move` step every bump.
	//
	// Always false-in-effect on the git backend — the dispatcher rejects
	// the flag at exit 2 before Push is called when Kind() == "git". The
	// git backend keeps a defensive same-exit reject for the unreachable
	// case (= belt-and-suspenders against future dispatcher refactors).
	//
	// Design rationale: opt-in (no implicit advance) because a bookmark
	// move is a side effect the user should consciously request — silent
	// movement would surprise users who positioned the bookmark
	// intentionally. See DR-0020 PR-5.2 implementation notes for the
	// flag-name selection (`--jj-` prefix names the backend the flag is
	// scoped to, opt-in default).
	jjBookmarkAutoAdvance bool
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

// --- shared promote/sync helpers ------------------------------------------

// resolveDefaultBranchViaGit returns the canonical default branch name by
// asking git directly (both backends route through it). The colocated git
// store is always available — jj workspaces have a backing git repo, and
// the git backend is git itself.
//
// Resolution order:
//  1. `git symbolic-ref --short refs/remotes/origin/HEAD` (canonical answer
//     set by `git clone` / `git remote set-head`). Strip the "origin/" prefix.
//  2. Local branch probe: main → master → trunk. First existing wins.
//
// Returns *exitErr{exitCodeVCSExec} when none of the above resolves.
func resolveDefaultBranchViaGit() (string, error) {
	if out, err := runBackendCmd("git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(string(out))
		if idx := strings.Index(ref, "/"); idx >= 0 {
			return ref[idx+1:], nil
		}
		if ref != "" {
			return ref, nil
		}
	}
	for _, candidate := range []string{"main", "master", "trunk"} {
		code, err := runBackendExitCode("git", "show-ref", "--verify", "--quiet", "refs/heads/"+candidate)
		if err == nil && code == 0 {
			return candidate, nil
		}
	}
	return "", &exitErr{
		code: exitCodeVCSExec,
		msg:  "default-branch: cannot determine (no origin/HEAD; no local main/master/trunk)",
	}
}

// isOnDefaultBranchCommon factors the predicate body shared by git and jj
// backends. Ambiguous CurrentBranch (detached HEAD, no bookmark) collapses
// to (false, nil) — the predicate's contract is a boolean, and ambiguity
// maps cleanly to "definitely not on the uniquely-named default". Any
// other error propagates.
func isOnDefaultBranchCommon(b vcsBackend) (bool, error) {
	def, err := b.DefaultBranch()
	if err != nil {
		return false, err
	}
	cur, err := b.CurrentBranch()
	if err != nil {
		var ee *exitErr
		if errors.As(err, &ee) && ee.code == exitCodeAmbiguous {
			return false, nil
		}
		return false, err
	}
	return cur == def, nil
}

// parseEpochOrZero parses s as a unix-epoch integer. Returns 0 on parse
// failure — defensive against the rare case where a backend changes its
// output format without us noticing (better to surface as "stale"
// signal than crash the verb).
func parseEpochOrZero(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
