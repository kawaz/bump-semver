package main

import "github.com/spf13/cobra"

// This file holds the per-command help PROSE (Long / Exit codes /
// Examples) and the wiring that attaches it to each cobra command. The
// Options / Global Options sections are NOT here — they are generated from
// the live FlagSet by renderFlagBlock (cobra_help.go), which is the whole
// point of the cobra migration (single source of truth for flags).
//
// Help is English-only and free of user-facing DR numbers (asserted by
// cobra_help_test.go). The wiring funcs (applyXxxHelp) are called from the
// command builders in cobra_bump.go / cobra_compare.go / cobra_vcs.go.

// --- bump family (major / minor / patch / pre / get) -----------------------

const bumpLong = `bump-semver major | minor | patch — bump a SemVer component

Usage:
  bump-semver <major|minor|patch> <INPUT...> [flags]

Action semantics:
  major       Bump the X in X.0.0 (reset Y, Z to 0)
  minor       Bump the y in x.Y.0 (reset Z to 0; x preserved)
  patch       Bump the z in x.y.Z  (x, y preserved)

  Pre-release and build-metadata are dropped by default. Use
  --pre / --build-metadata to re-attach explicit identifiers; --no-pre
  / --no-build-metadata to assert removal.

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string (e.g. 1.2.3, v1.2.3, 1.2.3-rc.1+build.42)
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git (read-only — see --write)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line (read-only)`

const bumpExitCodes = `  0   success
  1   (not used by bump — reserved for compare / vcs is)
  2   usage error (parse failure, version|name mismatch, missing input, ...)`

const bumpExamples = `  bump-semver patch Cargo.toml --write
  bump-semver minor package.json package-lock.json --write
  bump-semver patch v1.2.3                         # v1.2.4 (prefix preserved)
  bump-semver patch 1.2.3-rc.0 --pre rc.0          # 1.2.4-rc.0 (pre re-attached)
  bump-semver minor pyproject.toml --write --json  # JSON output for jq pipelines`

const preLong = `bump-semver pre — manage pre-release identifiers

Usage:
  bump-semver pre <INPUT...> [--write]                  # counter advance (default mode)
  bump-semver pre <INPUT...> --pre PRE [--write]         # set / overwrite
  bump-semver pre <INPUT...> --no-pre [--write]          # remove

Modes:
  (no flag)   Advance the trailing numeric identifier.
              The last identifier must be pure numeric (e.g. rc.0 → rc.1).
              Errors on identifiers like rc1 (mixed letters/digits).

  --pre PRE   Overwrite the pre-release with PRE (also adds one when absent).
              Allows rewinding (e.g. rc.5 → rc.0); rewind is the user's call.

  --no-pre    Strip pre-release entirely. No-op when there is no pre-release.

  Build metadata is preserved across all three modes (use bump
  major/minor/patch to drop it, or pair with --no-build-metadata).

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line (read-only)`

const preExamples = `  bump-semver pre 1.2.3-rc.0                         # 1.2.3-rc.1 (counter)
  bump-semver pre 1.2.3 --pre rc.0                   # 1.2.3-rc.0 (set)
  bump-semver pre 1.2.3-rc.5 --pre rc.0              # 1.2.3-rc.0 (rewind)
  bump-semver pre 1.2.3-rc.0 --no-pre                # 1.2.3 (remove)
  bump-semver pre Cargo.toml --pre rc.0 --write`

const getLong = `bump-semver get — print the current version

Usage:
  bump-semver get <INPUT...> [--json] [--no-pre] [--no-build-metadata]

When multiple INPUTs are given, all sources are treated as equal
peers and must agree; otherwise a "version mismatch:" (or
"name mismatch:" when package names diverge) listing is printed to
stderr and the process exits 1. exit 0 + single-line stdout on
agreement makes the command safe to pipe.

A file-omitted vcs:REV expands across every distinct sibling FILE
path. 'get a b vcs:main' therefore reads four sources: a, b, the
snapshot of a at main, and the snapshot of b at main.

Note: get is read-only; --write / --pre / --build-metadata are
rejected (use --no-pre / --no-build-metadata to strip on output).

Quiet flags never hide get's value: the version is the deliverable,
so -q silences hints only and -qq also silences errors, but the
value is always printed. This keeps 'ref=$(bump-semver get FILE -qq
2>/dev/null)' working instead of capturing an empty string.

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
                             (when FILE is omitted, expands across all
                             sibling FILE paths — see Examples)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
  vcs:latest-tag([REPO])     largest stable SemVer tag (cwd or external repo)
  vcs:latest-release([REPO]) largest stable GitHub Release (gh CLI required)`

const getExitCodes = `  0   every source agrees
  1   sources disagree (per-source listing on stderr)
  2   error (parse failure, missing input, unknown flag, ...)`

const getExamples = `  bump-semver get VERSION
  bump-semver get Cargo.toml package.json package-lock.json   # cross-file agreement check
  bump-semver get a b 'vcs:main@origin'                        # 4-way: a, b, vcs:main:a, vcs:main:b
  bump-semver get package.json --json | jq -r .semver
  bump-semver get 'vcs:latest-tag()'                           # input record (1-liner)
  bump-semver get 'cmd:./bin/mytool --version'                 # run a command, parse its output`

// applyBumpHelp attaches the per-action prose for a bump-family command.
// major / minor / patch share bumpLong; pre and get have their own.
func applyBumpHelp(cmd *cobra.Command, action string) {
	switch action {
	case "pre":
		setHelp(cmd, preLong, bumpExitCodes, preExamples)
	case "get":
		setHelp(cmd, getLong, getExitCodes, getExamples)
	default:
		setHelp(cmd, bumpLong, bumpExitCodes, bumpExamples)
	}
}

// --- compare ---------------------------------------------------------------

const compareLong = `bump-semver compare — compare a base value to one or more others (exit-code-driven)

Usage:
  bump-semver compare <OP> <BASE> <OTHER...>

BASE (the first input) is the reference; every OTHER is compared as
"BASE OP OTHER". The legacy two-input form is the N=1 case. Each
OTHER is evaluated independently — failures are listed on stderr
without short-circuit, so a single invocation surfaces every failing
relation.

Operators (5 base × 4 precision = 20 total):
                full       -major       -minor       -patch
  eq            eq         eq-major     eq-minor     eq-patch
  lt            lt         lt-major     lt-minor     lt-patch
  le            le         le-major     le-minor     le-patch
  gt            gt         gt-major     gt-minor     gt-patch
  ge            ge         ge-major     ge-minor     ge-patch

  base    {eq, lt, le, gt, ge}: pass/fail mapping of the comparison result
  suffix  -major / -minor / -patch: truncate the comparison.
          -major   compares X only
          -minor   compares X.Y (Z and pre-release ignored)
          -patch   compares X.Y.Z (pre-release ignored)
          (omitted) SemVer 2.0.0 § 11 full comparison (includes pre-release)

  Build-metadata is always ignored (SemVer § 10). For numeric
  pre-release identifiers leading zeros are rejected (per spec).

compare is read-only and exit-code-driven: --write / --json / --pre /
--build-metadata are rejected.

Inputs (BASE plus one or more OTHERS):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
  vcs:latest-tag([REPO])     largest stable SemVer tag (cwd or external repo)
  vcs:latest-release([REPO]) largest stable GitHub Release (gh CLI required)

  When an OTHER's vcs: spec has no explicit FILE component, it borrows
  BASE's path. 'compare gt VERSION vcs:main vcs:v1.0.0' therefore reads
  vcs:main:VERSION and vcs:v1.0.0:VERSION.`

const compareExitCodes = `  0   every predicate is true
  1   at least one predicate is false (per-OTHER detail on stderr)
  2   error (parse failure, missing input, unknown OP, etc.)`

const compareExamples = `  bump-semver compare eq 1.2.3 1.2.3
  bump-semver compare lt 1.2.3-rc.1 1.2.3                    # exit 0 (rc.1 < 1.2.3)
  bump-semver compare gt VERSION 'vcs:latest-tag()'          # ready to release? (1-liner)
  bump-semver compare gt VERSION 'vcs:main@origin' 'vcs:v1.0.0'  # ahead of main AND of v1.0.0
  bump-semver compare lt Cargo.toml vcs:origin/main          # stale vs remote main? (pull needed)
  bump-semver compare eq Cargo.toml vcs:HEAD~1               # unchanged since prev commit?
  bump-semver compare eq-major 1.2.3 1.9.7                   # exit 0 (same major)
  bump-semver compare eq-patch 1.2.3 1.2.3-rc.1              # exit 0 (pre-release ignored)
  bump-semver compare eq VERSION 'cmd:./bin/mytool --version'   # built bin matches version file?`

func applyCompareHelp(cmd *cobra.Command) {
	setHelp(cmd, compareLong, compareExitCodes, compareExamples)
}

// --- vcs parent + verbs ----------------------------------------------------

const vcsLong = `bump-semver vcs — VCS helpers (git/jj-agnostic)

Usage:
  bump-semver vcs <command> [args...]
  bump-semver vcs <command> --help     (= per-command detail)

See 'bump-semver vcs <command> --help' for arguments, options, and examples.`

const vcsExitCodes = `  0   success (predicate true)
  1   predicate false (vcs is — silent on stderr, mirrors compare)
  2   usage error (unknown verb / unknown key / wrong number of args)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer (e.g. detached HEAD, multiple bookmarks); also
      'vcs tag push' integrity violation (existing tag at a different rev)
  5   non-fast-forward rejection (vcs push — remote has diverged)`

func applyVcsHelp(cmd *cobra.Command) {
	setHelp(cmd, vcsLong, vcsExitCodes, "")
}

const vcsGetLong = `bump-semver vcs get — read a value from the VCS

Usage:
  bump-semver vcs get <key> [key-specific options...]

Keys:
  root             Absolute path to the repository root
  backend          The detected backend: "git" or "jj"
  current-branch   The unambiguous current branch (git) / bookmark (jj)
                   git:  HEAD's symbolic-ref short name. Detached HEAD → exit 4.
                   jj:   The single bookmark naming heads(::@ & bookmarks()).
                         Zero / multiple bookmarks at the head → exit 4.
  commit-id        40-char git commit SHA of --rev (default: @ for jj / HEAD
                   for git). Accepts any backend-native rev (bookmark, tag,
                   change-id, sha, HEAD~3, etc); cross-backend forms like
                   origin/main ↔ main@origin are normalized.
  latest-tag       Largest SemVer-parseable tag (cwd VCS or via --repository).
                   See 'vcs get latest-tag --help' for options.
  latest-release   Largest SemVer-parseable GitHub Release (gh CLI required).
                   See 'vcs get latest-release --help' for options.`

const vcsGetExitCodes = `  0   success (value printed on stdout, single line)
  2   usage error (key missing / unknown / multiple keys given)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer`

const vcsGetExamples = `  bump-semver vcs get root                    # /path/to/repo
  bump-semver vcs get backend                 # git  (or jj)
  bump-semver vcs get current-branch          # main
  bump-semver vcs get commit-id               # SHA of @ (jj) / HEAD (git)
  bump-semver vcs get commit-id --rev main    # SHA of main
  bump-semver vcs get latest-tag              # largest stable SemVer tag
  bump-semver vcs get latest-release          # largest stable GH Release
  ROOT=$(bump-semver vcs get root) || exit    # capture for further use`

const vcsIsLong = `bump-semver vcs is — test a VCS predicate

Usage:
  bump-semver vcs is <pred>

Predicates:
  clean    Worktree has no uncommitted changes (tracked-only; untracked
           files are ignored on git, snapshotted on jj — see notes).
  dirty    !clean (worktree has uncommitted changes).
  git      The detected backend is git.
  jj       The detected backend is jj.

Notes:
  - clean / dirty (git): runs 'git diff --quiet' (unstaged) AND
    'git diff --cached --quiet' (staged). Untracked files do NOT
    count as dirty.
  - clean / dirty (jj):  the working-copy change '@' is empty. Because
    jj snapshots on read, newly-created files DO render the worktree
    dirty. This asymmetry vs git is by design.
  - git / jj: compare against the auto-probe result. '--vcs git' /
    '--vcs jj' override forces the answer.`

const vcsIsExitCodes = `  0   predicate true
  1   predicate false (silent on stderr, mirrors compare)
  2   usage error (predicate missing / unknown / multiple given)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer (reserved for future predicates)`

const vcsIsExamples = `  bump-semver vcs is clean && bump-semver patch VERSION --write
  if bump-semver vcs is git; then ... fi
  bump-semver vcs is dirty || echo "nothing to commit"`

const vcsDiffLong = `bump-semver vcs diff — print the patch between REV and the working copy

Usage:
  bump-semver vcs diff [-s] REV [PATH..] [--excludes PATTERN]...

Arguments:
  REV          The revision to compare against (git: any rev-spec like
               HEAD~1, origin/main, <sha>; jj: any revset like @-, main@origin).
  PATH..       Optional path filter (= include set). Each entry is a
               literal path, 'glob:<pattern>', or 'file:<path>' (read
               newline-separated path list from <path>; '#' comments and
               blank lines skipped). Nonexistent paths are silently ignored
               (declarative convergence). When every PATH is filtered out,
               stdout is empty and exit is 0 — the verb does NOT widen back
               to "all paths" in that case.

Notes:
  - git: runs 'git diff REV [-- PATH..]' (one-rev form = REV vs working
    copy, including uncommitted changes). With -s, 'git diff --name-status'.
  - jj:  runs 'jj diff --from REV --to @ [-- PATH..]'. With -s, normalizes
    the native '<CODE> <path>' (space) to '<CODE>\t<path>' (tab) so output
    is uniform across backends. M/A/D codes are the supported scope.
  - The patch text is written verbatim to stdout (no re-formatting).
  - On 'vcs diff', -q is overloaded to mirror 'git diff --quiet': suppress
    stdout AND reflect diff presence in the exit code (0 = no diff, 1 =
    diff present). At least one positional PATH is required when --excludes
    is used.`

const vcsDiffExitCodes = `  0   no diff (with -q) OR patch written successfully (default / -s)
  1   diff present (with -q / -qq only)
  2   usage error (REV missing — currently surfaces as the help text)
  3   VCS subprocess error (not a repo, unresolvable REV)`

const vcsDiffExamples = `  bump-semver vcs diff HEAD~1                   # full diff since previous commit
  bump-semver vcs diff main@origin VERSION      # what changed in VERSION vs remote main
  bump-semver vcs diff HEAD~1 src lib           # subtree-scoped diff
  bump-semver vcs diff -s HEAD~1                # M/A/D file list (git --name-status format)
  bump-semver vcs diff -q HEAD~1 -- VERSION && echo "VERSION unchanged"
  bump-semver vcs diff -q HEAD~1 src/ --excludes 'glob:src/**/*_test.go'`

const vcsCommitLong = `bump-semver vcs commit — record changes safely (git/jj-agnostic)

Usage:
  bump-semver vcs commit -m MSG PATH..
  bump-semver vcs commit -m MSG --staged
  bump-semver vcs commit --amend [-m MSG] [PATH.. | --staged]

Modes:
  PATH..        Commit the working-tree content of the listed paths only.
                All paths are forwarded to the VCS as-is: deleted tracked
                files are committed as deletions, and truly unknown paths
                cause a VCS error. No actual change → exit 0, no commit
                (idempotent). Pass --allow-nonexistent-path to restore the
                legacy behaviour of silently dropping missing paths.
  --staged      Commit all staged/dirty changes at once.
                  git: commits the index (anything previously 'git add'-ed).
                  jj:  commits the entire @ snapshot (jj auto-stages).
                Nothing to commit → exit 0, no commit (idempotent).
  --amend       Fold the current change into the previous commit. Bare
                --amend uses the same scope as --staged (index / @ snapshot);
                '--amend PATH..' folds only those paths. With -m: rewrite the
                previous message; without -m: preserve it.

-a / --all is rejected by design (avoids unstaged-grab accidents — use
--staged, or name the PATH list explicitly).`

const vcsCommitExitCodes = `  0   success (commit created, OR idempotent no-op when nothing to do)
  2   usage error (-m missing on non-amend; --staged + PATH; -a; ...)
  3   VCS subprocess error (not a repo, command failed)`

const vcsCommitExamples = `  bump-semver vcs commit -m "bump version" VERSION Cargo.toml package.json
  bump-semver vcs commit --staged -m "release: 1.2.3"
  bump-semver vcs commit --amend                # fold all into previous, keep message
  bump-semver vcs commit --amend -m "release: 1.2.3 (final)"
  bump-semver vcs commit --amend VERSION        # fold ONLY VERSION into previous
  bump-semver vcs commit --allow-nonexistent-path -m "bump" VERSION Cargo.toml  # skip missing`

const vcsFetchLong = `bump-semver vcs fetch — refresh refs from a remote (git/jj-agnostic)

Usage:
  bump-semver vcs fetch [REMOTE]
  bump-semver vcs fetch --remote REMOTE

Arguments:
  REMOTE       The named remote to fetch from. Defaults to "origin" when
               omitted. Pass either as a positional or via --remote (the
               two are mutually exclusive — over-specifying is a usage error).

Notes:
  - git: runs 'git fetch <remote>'.
  - jj:  runs 'jj git fetch --remote <remote>'.
  - Network errors / unknown remote names surface as exit 3 with the
    underlying tool's stderr folded in.`

const vcsFetchExitCodes = `  0   success (refs refreshed; no-op if remote unchanged)
  2   usage error (too many positionals, unknown flag)
  3   VCS subprocess error (unknown remote, network failure, not a repo)`

const vcsFetchExamples = `  bump-semver vcs fetch                 # fetch origin
  bump-semver vcs fetch upstream        # fetch a specific remote
  bump-semver vcs fetch --remote origin # same as the bare form`

const vcsPushLong = `bump-semver vcs push — upload refs to a remote (git/jj-agnostic)

Usage:
  bump-semver vcs push --branch NAME [--remote REMOTE] [--jj-bookmark-auto-advance]

--branch is required (no auto-detection from current-branch); --bookmark
is an accepted synonym for jj users (supplying both at once is an error).
--force / --tags are intentionally NOT provided — bump-semver is a SemVer
helper, not a publishing tool; use the underlying git/jj directly for those.

--jj-bookmark-auto-advance (jj-only, silent no-op on git): move the bookmark
to the publishable commit before pushing — @- when the worktree is clean,
@ when dirty. The move is forward-only and refuses when the bookmark is not
in ancestors(@) (exit 3). Opt-in by design.

Notes:
  - git: runs 'git push <remote> <name>:<name>' (explicit refspec).
  - jj:  runs 'jj git push --bookmark <name> --remote <remote>' followed by
    'jj git export' so colocated .git refs stay in sync.
  - Idempotent: "remote already has it" → exit 0. Non-fast-forward → exit 5
    with git/jj's own stderr passed through verbatim.`

const vcsPushExitCodes = `  0   success (push completed, or idempotent up-to-date)
  2   usage error (--branch/--bookmark missing or specified twice,
                   --force passed, positional args supplied, unknown flag)
  3   VCS subprocess error (unknown remote, network failure, not a repo,
                            jj git export failure, auto-advance refused)
  5   non-fast-forward rejection — read git/jj's stderr for details`

const vcsPushExamples = `  bump-semver vcs push --branch main         # push main to origin
  bump-semver vcs push --branch main --remote upstream
  bump-semver vcs push --branch release-1.2  # push a feature/release branch
  bump-semver vcs push --branch main --jj-bookmark-auto-advance`

const vcsTagLong = `bump-semver vcs tag — manage tags atomically (create+push / delete)

Usage:
  bump-semver vcs tag <command> [args...]

See 'bump-semver vcs tag <command> --help' for arguments and options.
For reading the latest tag/release, use 'vcs get latest-tag' / 'vcs get
latest-release' or input records 'vcs:latest-tag()' / 'vcs:latest-release()'.

Notes:
  - 'tag push' is intentionally NOT separable into "tag locally then push
    later"; the contract is "the tag points to REV on the remote when this
    returns". This keeps tags 1-1 with their remote presence.
  - 'tag delete' removes both halves (local + remote); either half being
    missing is fine (idempotent rm -f semantic).
  - 'tag list' is NOT provided — use 'git tag --list' / 'jj tag list'.`

const vcsTagExitCodes = `  0   success (incl. idempotent same-rev push, absent-tag delete)
  2   usage error (command missing / unknown, NAME shape problem)
  3   VCS subprocess error (unknown remote, bad REV, network failure)
  4   integrity violation: 'tag push' against an existing different-rev tag
      without --allow-move (distinct from 3 so callers can detect drift)`

const vcsTagPushLong = `bump-semver vcs tag push — create or move a tag and push it

Usage:
  bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]

Arguments:
  NAME             Tag name (e.g. "v1.2.3"). Required. Must not be empty,
                   must not contain whitespace, must not start with "refs/"
                   (the "refs/tags/" prefix is added automatically).

Behaviour:
  - absent local tag           → create at REV, push to remote
  - local tag at same REV      → skip local create, still push (one-sided
                                 recovery; clean no-op if remote also matches)
  - local tag at different REV → exit 4 (no side-effect), unless --allow-move
                                 is set, in which case the tag is moved and
                                 force-pushed.
  - bad REV                    → exit 3 (resolution fails before side-effect).

--force / --force-with-lease is not provided (use --allow-move — the precise
opt-in). --tags / --all bulk operations are out of scope.`

const vcsTagPushExitCodes = `  0   success (incl. idempotent same-rev re-push)
  2   usage error (NAME missing / bad shape, --rev missing, --force passed)
  3   VCS subprocess error (unknown remote, bad REV, network failure)
  4   integrity violation: tag exists at a different REV, --allow-move not
      set. No local move, no push attempted.`

const vcsTagPushExamples = `  bump-semver vcs tag push --rev HEAD v1.2.3
  bump-semver vcs tag push --rev main v1.2.3 --remote upstream
  bump-semver vcs tag push --rev HEAD~1 v1.2.3 --allow-move`

const vcsTagDeleteLong = `bump-semver vcs tag delete — remove a tag locally and on a remote

Usage:
  bump-semver vcs tag delete NAME [--remote REMOTE]

Arguments:
  NAME             Tag name to delete. Required.

Behaviour:
  - Removes both the local tag AND the remote tag.
  - Idempotent (rm -f semantic): an absent tag on either side is exit 0,
    not an error.
  - A genuine remote failure (unknown remote, network down) is exit 3;
    the local-half side-effect may have already happened.

--allow-missing is not provided (delete is natively idempotent).`

const vcsTagDeleteExitCodes = `  0   success (tag removed from both sides, OR already absent)
  2   usage error (NAME missing / bad shape)
  3   VCS subprocess error (unknown remote, network failure, not a repo)`

const vcsTagDeleteExamples = `  bump-semver vcs tag delete v0.9.0             # remove from local + origin
  bump-semver vcs tag delete v0.9.0 --remote upstream`

const vcsOutdatedLong = `bump-semver vcs outdated — derived-sync check via FROM→TO mapping

Usage:
  bump-semver vcs outdated FROM TO[..]
  bump-semver vcs outdated -- FROM TO[..] -- FROM TO[..] -- ...

Compare committer timestamps: each TO file must be at least as new as the
FROM that produced it. Exit 1 = stale or missing-mandatory derived.

  FROM   Literal path or 'glob:<pat>'.
  TO     Each may use:  $N / ${N}    capture from FROM ($0 = full match)
                        {a,b,c}      MANDATORY expansion (every option must exist)
                        *, **, []    OPTIONAL fs discovery (no match = silent skip)
                        'glob:<pat>' 2-stage TO discovery (escape-aware)
  --     Pair separator. Single pair: optional. N≥2 pairs: required.

Options (this verb parses its own tokens; flags are documented here):
  --explain                       Print (source → derived) rows + status; exit 0.
  --strict                        Literal FROM no match → exit 1 (CI gate).
  --glob-dotfile=true|false       (default false)  Include dotfile paths in FROM.
  --glob-gitignored=true|false    (default true)   Respect .gitignore in FROM.
  --glob-ignorecase[=true|false]  (default false)  Case-insensitive match.`

const vcsOutdatedExitCodes = `  0   fresh
  1   stale or missing
  2   usage error
  3   VCS subprocess error`

const vcsOutdatedExamples = `  bump-semver vcs outdated 'glob:src/**/*.ts' 'lib/$1/$2.js'
  bump-semver vcs outdated README.md 'README-{ja,en}.md'
  bump-semver vcs outdated --explain 'glob:**/*-ja.md' '$1/$2.md'`

// applyVcsVerbHelp attaches the prose for a vcs child command, keyed by
// its Use name. The two-tier `tag` children are handled separately.
func applyVcsVerbHelp(cmd *cobra.Command) {
	switch cmd.Name() {
	case "get":
		setHelp(cmd, vcsGetLong, vcsGetExitCodes, vcsGetExamples)
	case "is":
		setHelp(cmd, vcsIsLong, vcsIsExitCodes, vcsIsExamples)
	case "diff":
		setHelp(cmd, vcsDiffLong, vcsDiffExitCodes, vcsDiffExamples)
	case "commit":
		setHelp(cmd, vcsCommitLong, vcsCommitExitCodes, vcsCommitExamples)
	case "fetch":
		setHelp(cmd, vcsFetchLong, vcsFetchExitCodes, vcsFetchExamples)
	case "push":
		setHelp(cmd, vcsPushLong, vcsPushExitCodes, vcsPushExamples)
	case "outdated":
		setHelp(cmd, vcsOutdatedLong, vcsOutdatedExitCodes, vcsOutdatedExamples)
	}
}

// --- vcs get latest-tag / latest-release (positional pseudo-commands) -------

// latestHelp holds the prose + the subset of `vcs get` flags shown for the
// latest-tag / latest-release positional keys (they are not separate cobra
// commands; see renderLatestHelp).
type latestHelp struct {
	long        string
	exitCodes   string
	examples    string
	optionFlags []string // long names of `vcs get` flags to show as Options
}

var latestHelpData = map[string]latestHelp{
	"latest-tag": {
		long: `bump-semver vcs get latest-tag — print the SemVer-largest tag

Usage:
  bump-semver vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]

Behaviour:
  Lists tag refs from the cwd VCS (default) or an external repo, drops
  anything that doesn't parse as SemVer 2.0.0, and returns the largest.
  Pre-release tags (v1.2.3-rc.1 etc.) are excluded by default; pass
  --include-prerelease to include them. External path uses
  'git ls-remote --tags' (no gh).`,
		exitCodes: `  0   success (tag found and emitted)
  2   usage error (extra positional arguments)
  3   VCS subprocess error / no semver-compatible tags found`,
		examples: `  bump-semver vcs get latest-tag                     # cwd: largest SemVer tag (bare)
  bump-semver vcs get latest-tag --include-prerelease  # include v1.2.3-rc.1 etc.
  bump-semver vcs get latest-tag --json              # structured (= get --json schema)
  bump-semver vcs get latest-tag --repository kawaz/pkf-tasks
  bump-semver get 'vcs:latest-tag()'                 # input record (= 1-liner ergonomic)`,
		optionFlags: []string{"include-prerelease", "repository", "json"},
	},
	"latest-release": {
		long: `bump-semver vcs get latest-release — print the SemVer-largest GitHub Release

Usage:
  bump-semver vcs get latest-release [--include-prerelease] [--repository REPO] [--json]

Behaviour:
  Reads GitHub Release objects via the gh CLI, drops drafts, drops anything
  that doesn't parse as SemVer 2.0.0, and returns the largest. Pre-releases
  are excluded by default; pass --include-prerelease to include them.

External tool dependency:
  gh CLI (https://cli.github.com/) is required. Missing gh → exit 3 with an
  install hint. gh's own repo detection is independent of --vcs.`,
		exitCodes: `  0   success (release found and emitted)
  2   usage error (extra positional arguments)
  3   gh subprocess error, gh missing, or no semver-compatible releases`,
		examples: `  bump-semver vcs get latest-release                 # cwd repo (gh-detected)
  bump-semver vcs get latest-release --include-prerelease
  bump-semver vcs get latest-release --json          # structured output
  bump-semver vcs get latest-release --repository kawaz/bump-semver
  bump-semver get 'vcs:latest-release(kawaz/pkf-tasks)'  # input record (1-liner)`,
		optionFlags: []string{"include-prerelease", "repository", "json"},
	},
}
