package main

import (
	"fmt"
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
