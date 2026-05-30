package main

// Help text constants live in this file so main.go can focus on argv
// parsing and dispatch. Each `--help` variant has its own constant; the
// short / full pair is selected by argv (--help vs --help-full) and the
// per-action texts are dispatched through actionHelpTexts.

// shortHelpText is the default --help output: a one-screen overview
// that points at the per-action help (and at --help-full as the
// authoritative reference). Aim for ~25 lines; anything longer
// belongs in helpBump / helpPre / helpGet / helpCompare or in
// fullHelpText.
const shortHelpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <action> [args...]
  bump-semver --version
  bump-semver --help | --help-full

Actions:
  get        Read the current version
  major      Bump major (X.0.0)
  minor      Bump minor (x.Y.0)
  patch      Bump patch (x.y.Z)
  pre        Pre-release identifiers (counter advance / set / remove)
  compare    Compare two SemVer values via <eq|lt|le|gt|ge|...>
  vcs        VCS helpers (git/jj-agnostic; e.g. vcs get root, vcs is clean)

Action-specific help: bump-semver <action> --help
Full reference:       bump-semver --help-full

Inputs are positional: FILE / VER / - / vcs:REV[:FILE] / vcs:latest-tag([REPO]) / cmd:CMD.
Files are auto-detected by basename (Cargo.toml, package.json,
pyproject.toml, VERSION, ...). See --help-full for the table.
`

// fullHelpText is the complete reference, printed by --help-full.
// It includes the supported-format table, the full Examples block,
// exit codes, and the multi-input semantics — anything a user might
// reach for when scripting or debugging an unusual file.
const fullHelpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <ACTION> <INPUT...> [flags]
  bump-semver compare <OP> <INPUT> <INPUT> [flags]
  bump-semver --version
  bump-semver --help | --help-full

Actions (bump/read):
  major   Bump major (X.0.0); pre-release / build-metadata dropped by default
  minor   Bump minor (x.Y.0); pre-release / build-metadata dropped by default
  patch   Bump patch (x.y.Z); pre-release / build-metadata dropped by default
  pre     Pre-release counter advance / set / remove (see --pre / --no-pre)
  get     Print the current version (with optional --no-pre / --no-build-metadata)

Compare (nested subcommand):
  compare eq  INPUT INPUT     true if equal (SemVer 2.0.0 ordering, build metadata ignored)
  compare lt  INPUT INPUT     true if first <  second
  compare le  INPUT INPUT     true if first <= second
  compare gt  INPUT INPUT     true if first >  second
  compare ge  INPUT INPUT     true if first >= second
  Optional -major / -minor / -patch suffix (DR-0017) truncates the comparison
  (e.g. eq-major 1.2.3 1.9.7 -> true). See: bump-semver compare --help

Inputs:
  FILE                       path to a supported file (auto-detected by basename)
  VER                        a raw semver string (e.g. 1.2.3, v1.2.3, 1.2.3-rc.1+build.42)
  -                          read VER from stdin (single line, used at most once)
  vcs:REV[:FILE]             read FILE at <REV> from the VCS (jj or git, auto-detected)
  vcs:latest-tag([REPO])     largest semver tag; REPO = owner/repo or full URL (default: cwd VCS)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
                             (read-only, strips a leading 'v'; e.g. cmd:mytool --version)

Options:
  --write                Write the new version back to each FILE input (bump only)
  --pre PRE              Set pre-release identifiers (e.g. --pre rc.0)
  --build-metadata META  Set build metadata identifiers (e.g. --build-metadata sha.abc)
  --no-pre               Remove pre-release identifiers
  --no-build-metadata    Remove build metadata identifiers

Global Options:
  --vcs jj|git|auto      Force VCS detection for vcs: inputs (default: auto)
  --no-hint              Suppress hints (fallback / unsupported / "files not modified")
  -q, --quiet            Suppress stdout (and the hint)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)
  --json                 Output structured JSON (get / bump only, not for compare)
  --version, -V          Print the binary version
  --help, -h             Show the short help (Usage + common options)
  --help-full            Show this full reference

Supported file formats (auto-detected by basename):
  Cargo.toml         TOML, [package].version (try) -> [workspace.package].version (fallback) [DR-0021]
  pyproject.toml     TOML, [project].version (try) -> [tool.poetry].version (fallback) [DR-0014]
  mojoproject.toml   TOML, [workspace].version (and [workspace].name) [DR-0014]
  package-lock.json  npm 7+ lockfile, $.version + $.packages[""].version (deps untouched)
  pom.xml            XML element, /project/version (and /project/artifactId) [DR-0018]
  *.json             JSON, $.version (and optional $.name)
  *.yaml / *.yml     YAML, top-level .version (and optional .name) [DR-0011 fallback]
  *.toml             TOML, top-level version  (and optional name)  [DR-0011 fallback]
  v.mod / build.zig.zon / mix.exs / build.sbt        regex (basename) [DR-0012]
  build.gradle / build.gradle.kts                    regex (basename) [DR-0018]
  *.xcconfig / *.podspec / *.nimble / *.gemspec      regex (fallback) [DR-0012]
  *.cabal / *.spec                                   regex (fallback) [DR-0018]
  *.csproj / *.fsproj / *.vbproj                     XML element, /Project/PropertyGroup/Version [DR-0018]
  VERSION            plain text

  Backup-style suffix fallback (DR-0013): Cargo.toml.bak / package.json.20260510 /
  Chart.yaml~ etc. strip one trailing suffix and retry against the table above.
  Suffixes: .bak / .backup / .orig / .tmp / .old / .YYYYMMDD / .YYYYMMDD_HHMMSS / ~

Multiple inputs (FILE / VER / - / vcs: / cmd:) may be mixed. All extracted
versions must agree; otherwise a "version mismatch:" error lists each origin
and value. With --write, only FILE-origin inputs are written back (vcs: and
cmd: are read-only).

VCS helpers (DR-0020):
  vcs get root              Print the repository root
  vcs get backend           Print "git" or "jj"
  vcs get current-branch    Print the unambiguous branch (git) / bookmark (jj)
  vcs is clean|dirty        Test worktree cleanliness (untracked ignored on git)
  vcs is git|jj             Test the detected backend
  vcs diff REV [PATH..]     Print the patch between REV and the working copy
  vcs commit -m MSG ...     Commit (PATH.. | --staged | --amend); -a is rejected
  vcs fetch [REMOTE]        Refresh refs from a remote (default origin)
  vcs push --branch NAME    Push to remote (jj users: bookmark; --force not provided)
  (See: bump-semver vcs --help)

Exit codes:
  0   success (or predicate true)
  1   predicate false (compare / vcs is — silent on stderr)
  2   usage error (parse failure, mismatch, missing input, unknown verb/key, etc.)
  3   VCS subprocess error (vcs subcommands only — e.g. not in a repo)
  4   ambiguous answer (vcs subcommands only — e.g. detached HEAD)
  5   non-fast-forward push (vcs push — remote has diverged)

Examples:
  bump-semver get VERSION
  bump-semver patch VERSION --write
  bump-semver patch Cargo.toml --write
  bump-semver patch package.json --write
  bump-semver minor pyproject.toml --write
  bump-semver minor package.json package-lock.json --write
  bump-semver patch 1.2.3
  bump-semver patch v1.2.3                       # v1.2.4 (prefix preserved)
  bump-semver minor version_1_2_3                # version_1_3_0 (prefix + body sep '_' preserved)
  bump-semver pre 1.2.3-rc.0                     # 1.2.3-rc.1
  bump-semver pre 1.2.3 --pre rc.0               # 1.2.3-rc.0
  bump-semver patch 1.2.3-rc.0 --pre rc.0        # 1.2.4-rc.0 (pre re-attached)
  bump-semver compare lt 1.2.3-rc.1 1.2.3        # exit 0
  bump-semver compare eq .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
  bump-semver get package.json --json            # structured output for jq
  bump-semver --version --json                   # decompose own version into the same JSON schema
  bump-semver compare gt Cargo.toml 'vcs:latest-tag()'   # ready to release? (CI)
  bump-semver compare lt Cargo.toml vcs:origin/main      # stale vs remote main? (pull needed)
  bump-semver compare eq Cargo.toml vcs:HEAD~1           # unchanged since prev commit?
  bump-semver compare eq VERSION 'cmd:./bin/mytool --version'   # built bin matches version file?
`

// helpBump documents `major` / `minor` / `patch`. The three share the
// same shape so we keep a single help text and let the user infer the
// specific component being bumped from the action name.
const helpBump = `bump-semver major | minor | patch — bump a SemVer component

Usage:
  bump-semver <major|minor|patch> <INPUT...> [--write] [--pre PRE] [--build-metadata META] [--no-pre] [--no-build-metadata]

Action semantics:
  major       Bump the X in X.0.0 (reset Y, Z to 0)
  minor       Bump the y in x.Y.0 (reset Z to 0; x preserved)
  patch       Bump the z in x.y.Z  (x, y preserved)

  Pre-release and build-metadata are dropped by default (DR-0006). Use
  --pre / --build-metadata to re-attach explicit identifiers; --no-pre
  / --no-build-metadata to assert removal (errors if the user later
  also passes the matching set form).

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string (e.g. 1.2.3, v1.2.3, 1.2.3-rc.1+build.42)
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git (read-only — see --write)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line (read-only)

Options:
  --write                    Write the bumped version back to each FILE input
                             (vcs:/cmd:/VER/- inputs are reference-only; --write requires ≥ 1 FILE)
  --pre PRE                  Set pre-release identifiers on the result (e.g. --pre rc.0)
  --build-metadata META      Set build metadata on the result (e.g. --build-metadata sha.abc)
  --no-pre                   Assert removal of pre-release
  --no-build-metadata        Assert removal of build metadata
  --json                     Structured JSON output
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / -qq / --no-hint       Output suppression (see --help-full)

Examples:
  bump-semver patch Cargo.toml --write
  bump-semver minor package.json package-lock.json --write
  bump-semver patch v1.2.3                         # v1.2.4 (prefix preserved)
  bump-semver patch 1.2.3-rc.0 --pre rc.0          # 1.2.4-rc.0 (pre re-attached)
  bump-semver minor pyproject.toml --write --json  # JSON output for jq pipelines
`

// helpPre documents the three modes of `pre` separately because they
// behave quite differently from each other (counter advance vs set vs
// remove). Getting these confused is the most common pre-related
// foot-gun, so the action help is explicit about them.
const helpPre = `bump-semver pre — manage pre-release identifiers

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
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line (read-only)

Options:
  --write                    Write the result back to each FILE input
  --pre PRE                  Set / overwrite (mode 2)
  --no-pre                   Remove (mode 3) — mutually exclusive with --pre
  --build-metadata META      Set build metadata on the result
  --no-build-metadata        Strip build metadata on the result
  --json                     Structured JSON output
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / -qq / --no-hint       Output suppression (see --help-full)

Examples:
  bump-semver pre 1.2.3-rc.0                         # 1.2.3-rc.1 (counter)
  bump-semver pre 1.2.3 --pre rc.0                   # 1.2.3-rc.0 (set)
  bump-semver pre 1.2.3-rc.5 --pre rc.0              # 1.2.3-rc.0 (rewind)
  bump-semver pre 1.2.3-rc.0 --no-pre                # 1.2.3 (remove)
  bump-semver pre Cargo.toml --pre rc.0 --write
`

// helpGet documents the read-only `get` action. Short and focused —
// most callers just want to print or pipe the current version.
const helpGet = `bump-semver get — print the current version

Usage:
  bump-semver get <INPUT...> [--json] [--no-pre] [--no-build-metadata]

When multiple INPUTs are given, all extracted versions must agree;
otherwise a "version mismatch:" error lists each origin and value.
This is the read-side mirror of the bump actions and is safe to call
with --json for piping into jq.

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
  vcs:latest-tag([REPO])     largest semver tag (cwd VCS or remote: owner/repo / URL)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
                             (strips a leading 'v'; e.g. cmd:./bin/mytool --version)

Options:
  --json                     Structured JSON output (.name, .version, .semver,
                             .major / .minor / .patch / .pre / .build_metadata, ...)
  --no-pre                   Strip pre-release from the printed value
  --no-build-metadata        Strip build metadata from the printed value
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / -qq / --no-hint       Output suppression (see --help-full)

Examples:
  bump-semver get VERSION
  bump-semver get Cargo.toml package.json package-lock.json   # cross-file agreement check
  bump-semver get package.json --json | jq -r .semver
  bump-semver get 'vcs:latest-tag()'                          # largest semver tag (cwd)
  bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'            # remote (GitHub short)
  bump-semver get 'vcs:latest-tag(https://github.com/x/y)'     # remote (full URL)
  bump-semver get 'cmd:./bin/mytool --version'                 # run a command, parse its output
`

// helpCompare documents the compare subcommand. The OP list is the
// authoritative reference for which operators are supported — the
// shortHelpText only shows it as `<eq|lt|le|gt|ge|...>` to stay short.
const helpCompare = `bump-semver compare — compare two SemVer values (exit-code-driven)

Usage:
  bump-semver compare <OP> <INPUT> <INPUT>

Operators (5 base × 4 precision = 20 total):
                full       -major       -minor       -patch
  eq            eq         eq-major     eq-minor     eq-patch
  lt            lt         lt-major     lt-minor     lt-patch
  le            le         le-major     le-minor     le-patch
  gt            gt         gt-major     gt-minor     gt-patch
  ge            ge         ge-major     ge-minor     ge-patch

  base    {eq, lt, le, gt, ge}: pass/fail mapping of the comparison result
  suffix  -major / -minor / -patch (DR-0017): truncate the comparison.
          -major   compares X only
          -minor   compares X.Y (Z and pre-release ignored)
          -patch   compares X.Y.Z (pre-release ignored)
          (omitted) SemVer 2.0.0 § 11 full comparison (includes pre-release)

  Build-metadata is always ignored (SemVer § 10). For numeric
  pre-release identifiers leading zeros are rejected (per spec).

Inputs (exactly two):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
  vcs:latest-tag([REPO])     largest semver tag (cwd VCS or remote: owner/repo / URL)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER

  When a vcs: input has no explicit FILE component, the FILE is
  borrowed from the first sibling input that provides one (DR-0008).

Options:
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / -qq / --no-hint       Output suppression (see --help-full)

  --write / --json / --pre / --build-metadata: rejected (compare is
  read-only and exit-code-driven, not value-output-driven).

Exit codes:
  0   predicate is true
  1   predicate is false
  2   error (parse failure, missing input, unknown OP, etc.)

Examples:
  bump-semver compare eq 1.2.3 1.2.3
  bump-semver compare lt 1.2.3-rc.1 1.2.3                    # exit 0 (rc.1 < 1.2.3)
  bump-semver compare gt Cargo.toml 'vcs:latest-tag()'       # is the local bump ahead of release? (CI)
  bump-semver compare lt Cargo.toml vcs:origin/main          # stale vs remote main? (pull needed)
  bump-semver compare eq Cargo.toml vcs:HEAD~1               # unchanged since prev commit?
  bump-semver compare eq .claude-plugin/plugin.json .claude-plugin/marketplace.json
  bump-semver compare eq-major 1.2.3 1.9.7                   # exit 0 (same major)
  bump-semver compare eq-patch 1.2.3 1.2.3-rc.1              # exit 0 (pre-release ignored)
  bump-semver compare lt-minor Cargo.toml vcs:origin/main    # only minor bumps since main?
  bump-semver compare eq VERSION 'cmd:./bin/mytool --version'   # built bin matches version file?
`

// helpVcs documents the `vcs` parent subcommand (DR-0020). PR-1 only
// implements `vcs get`; other verbs (is / diff / commit / push / tag)
// are listed in the design doc and will appear here as they land.
//
// kawaz CLI design preferences: sections in order (subcommand list,
// options, global options, env), long options only, --help is the
// no-args default.
const helpVcs = `bump-semver vcs — VCS helpers (git/jj-agnostic) [DR-0020]

Usage:
  bump-semver vcs <verb> [args...]
  bump-semver vcs --help

Verbs:
  get <key>           Read a value from the VCS. Keys: root | backend | current-branch.
  is  <pred>          Test a predicate. Predicates: clean | dirty | git | jj. Exit 0=true, 1=false.
  diff REV [PATH..]   Print the patch between REV and the working copy (git/jj-agnostic).
  commit -m MSG PATH..        Commit listed paths' working-tree content (safe default).
  commit -m MSG --staged      Commit all staged/dirty changes at once.
  commit --amend [-m MSG] [PATH.. | --staged]
                              Fold current changes into the previous commit
                              (symmetric with -m PATH.. / -m --staged above).
  fetch [REMOTE]              Fetch refs from REMOTE (default: origin).
  push --branch NAME [--remote REMOTE]
                              Push NAME to REMOTE (default: origin).
                              (jj users: "branch" = bookmark; --bookmark also accepted.)

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (and the hint)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)
  --help, -h             Show this help

Exit codes:
  0   success (predicate true)
  1   predicate false (vcs is — silent on stderr, mirrors compare)
  2   usage error (unknown verb / unknown key / wrong number of args)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer (e.g. detached HEAD, multiple bookmarks)
  5   non-fast-forward rejection (vcs push — remote has diverged)
`

// helpVcsGet documents `vcs get <key>`.
//
// The three keys are intentionally minimal — anything more elaborate
// belongs in a dedicated verb (e.g. tag listing in a future `vcs tag
// list`). Keep the set tight so callers can rely on every key being
// equally cheap and equally well-defined.
const helpVcsGet = `bump-semver vcs get — read a value from the VCS [DR-0020]

Usage:
  bump-semver vcs get <key>

Keys:
  root             Absolute path to the repository root
  backend          The detected backend: "git" or "jj"
  current-branch   The unambiguous current branch (git) / bookmark (jj)
                   git:  HEAD's symbolic-ref short name. Detached HEAD → exit 4.
                   jj:   The single bookmark naming heads(::@ & bookmarks()).
                         Zero / multiple bookmarks at the head → exit 4.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (value printed on stdout, single line, no trailing newline beyond '\n')
  2   usage error (key missing / unknown / multiple keys given)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer

Examples:
  bump-semver vcs get root                    # /path/to/repo
  bump-semver vcs get backend                 # git  (or jj)
  bump-semver vcs get current-branch          # main
  ROOT=$(bump-semver vcs get root) || exit    # capture for further use
`

// helpVcsIs documents `vcs is <pred>` (DR-0020 PR-2).
//
// The four predicates are intentionally minimal: `clean` / `dirty` cover
// "is the worktree ready to commit?" — the question Taskfile/justfile
// authors actually ask before bumping a version. `git` / `jj` cover
// "which backend was selected?" so shell scripts can branch without
// re-running the probe themselves.
//
// Future predicates (`ahead` / `behind` / …) plug in here as the
// design rolls out; backend-specific concepts (e.g. jj's empty `@`)
// are intentionally excluded for portability.
const helpVcsIs = `bump-semver vcs is — test a VCS predicate [DR-0020]

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
    count as dirty (PR-2 contract; a future opt-in would change this).
  - clean / dirty (jj):  the working-copy change '@' is empty (template
    keyword 'empty'). Because jj snapshots on read, newly-created files
    DO render the worktree dirty. This asymmetry vs git is by design.
  - git / jj: compare against the auto-probe result. '--vcs git' /
    '--vcs jj' override forces the answer.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            (no-op for is — there is no stdout payload)
  -qq, --quiet-all       Suppress error output (use with caution)

Exit codes:
  0   predicate true
  1   predicate false (silent on stderr, mirrors compare)
  2   usage error (predicate missing / unknown / multiple given)
  3   VCS subprocess error (not a repo, command failed)
  4   ambiguous answer (reserved for future predicates)

Examples:
  bump-semver vcs is clean && bump-semver patch VERSION --write
  if bump-semver vcs is git; then ... fi
  bump-semver vcs is dirty || echo "nothing to commit"
`

// helpVcsDiff documents `vcs diff REV [PATH..]` (DR-0020 PR-3).
//
// The verb is a thin wrapper around the backend's native `diff` (one-rev
// form: REV vs working copy, including uncommitted changes). The user
// gets identical output whether the cwd is git or jj — git via
// `git diff REV`, jj via `jj diff --from REV --to @`.
//
// PATH filtering is declarative-convergent: nonexistent paths are
// silently dropped (kawaz's design decision). When every supplied path
// is filtered out the command exits 0 with empty stdout — it explicitly
// does NOT widen back to "diff everything".
const helpVcsDiff = `bump-semver vcs diff — print the patch between REV and the working copy [DR-0020]

Usage:
  bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]

Arguments:
  REV          The revision to compare against (git: any rev-spec like
               HEAD~1, origin/main, <sha>; jj: any revset like @-, main@origin).
  PATH..       Optional path filter. Nonexistent paths are silently
               ignored (declarative convergence). When every PATH is
               filtered out, stdout is empty and exit is 0 — the verb
               does NOT widen back to "all paths" in that case.

Notes:
  - git: runs 'git diff REV [-- PATH..]' (one-rev form = REV vs working
    copy, including uncommitted changes). With -s, runs 'git diff
    --name-status REV [-- PATH..]'.
  - jj:  runs 'jj diff --from REV --to @ [-- PATH..]'. With -s, runs
    'jj diff --summary' and normalizes the native '<CODE> <path>'
    (space) to '<CODE>\\t<path>' (tab) so output is uniform across
    backends. M/A/D codes are the supported scope.
  - The patch text is written verbatim to stdout (no re-formatting).
  - A path present in REV but deleted in @ is NOT shown when named
    explicitly (os.Stat filters it). The full diff (no PATH) still
    shows the deletion.

Verb Options:
  -s, --name-status      Emit one '<CODE>\\t<path>' line per changed
                         file (M/A/D) instead of the raw patch.
                         Mirrors 'git diff --name-status'.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            On 'vcs diff', overloaded to mirror 'git diff
                         --quiet': suppress stdout AND reflect diff
                         presence in the exit code (0 = no diff, 1 = diff
                         present). With -s, -q wins (stdout empty, exit
                         still reflects presence). Use 'vcs diff -q REV
                         -- VERSION && echo unchanged' to script "no
                         changes since REV?".
  -qq, --quiet-all       Same exit-code semantics as -q, plus suppress
                         error output.

Exit codes:
  0   no diff (with -q) OR patch written successfully (default / -s)
  1   diff present (with -q / -qq only)
  2   usage error (REV missing — currently surfaces as the help text)
  3   VCS subprocess error (not a repo, unresolvable REV)

Examples:
  bump-semver vcs diff HEAD~1                   # full diff since previous commit
  bump-semver vcs diff main@origin VERSION      # what changed in VERSION vs remote main
  bump-semver vcs diff HEAD~1 src lib           # subtree-scoped diff
  bump-semver vcs diff @-                       # jj: diff since @- (parent change)
  bump-semver vcs diff -s HEAD~1                # M/A/D file list (git --name-status format)
  bump-semver vcs diff -q HEAD~1 -- VERSION && echo "VERSION unchanged"
                                                # exit 0 ⇔ no diff in VERSION
`

// helpVcsCommit documents `vcs commit` (DR-0020 PR-4).
//
// Three modes — path (default safety), --staged (commit-all), --amend
// (rewrite). `-a` is intentionally NOT provided (kawaz CLI safety: jj's
// auto-staged worldview makes `-a`-style unstaged grabs too easy to
// trip on; use --staged or pass a PATH explicitly).
//
// The empty-no-op rule (DR-0020) means a path list with no real
// change — including all-nonexistent — exits 0 without creating a
// commit. This makes `vcs commit -m "..." VERSION Cargo.toml
// package.json` a useful "commit whatever bumped" snippet across
// languages without needing per-project presets.
const helpVcsCommit = `bump-semver vcs commit — record changes safely (git/jj-agnostic) [DR-0020]

Usage:
  bump-semver vcs commit -m MSG PATH..
  bump-semver vcs commit -m MSG --staged
  bump-semver vcs commit --amend [-m MSG] [PATH.. | --staged]

Modes:
  PATH..        Commit the working-tree content of the listed paths only.
                Nonexistent paths are silently dropped (declarative
                convergence). All-nonexistent OR no actual change for any
                surviving path → exit 0, no commit (idempotent).
  --staged      Commit all staged/dirty changes at once.
                  git: commits the index (anything previously 'git add'-ed).
                  jj:  commits the entire @ snapshot (= all current changes,
                       since jj auto-stages).
                No staged/dirty content → exit 0, no commit (idempotent).
  --amend       Fold the current change into the previous commit (instead
                of creating a new one). Fully symmetric with non-amend:
                  --amend                  bare amend (no path selector):
                                             git: folds the staged index
                                                  into HEAD (unstaged
                                                  worktree changes are NOT
                                                  included — same scope as
                                                  --staged).
                                             jj:  folds the entire @
                                                  snapshot into @- (jj
                                                  auto-stages, so this IS
                                                  every current change).
                  --amend PATH..           fold only those paths.
                  --amend --staged         explicit synonym for bare amend
                                           (the index / @ snapshot IS the
                                           absorption source).
                With -m: rewrite the previous commit's message.
                Without -m: preserve the previous commit's message.
                Equivalences:
                  git: git add -- PATHS; git commit --amend [-m|--no-edit] -- PATHS
                  jj:  jj squash --from @ --into @- [-m MSG | -u] [-- PATHS]
                Path-scoped amend follows the same no-op rule as path mode
                (all-nonexistent / no-change → exit 0). Bare amend bypasses
                the gate — message-only rewrite is a legal explicit intent.

Arguments:
  -m, --message MSG    Commit message. Required UNLESS --amend.

Not provided by design:
  -a / --all           Use --staged (avoids unstaged-grab accidents — see
                       DR-0020). For jj users the equivalent is naming the
                       PATH list explicitly.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (commit created, OR idempotent no-op when there was nothing to do)
  2   usage error (-m missing on non-amend; --staged + PATH; -a; other parse errors)
  3   VCS subprocess error (not a repo, command failed)

Examples:
  bump-semver vcs commit -m "bump version" VERSION Cargo.toml package.json
                                                # commit whichever exist & changed
  bump-semver vcs commit --staged -m "release: 1.2.3"
                                                # commit everything in one shot
  bump-semver vcs commit --amend                # fold all into previous, keep message
  bump-semver vcs commit --amend -m "release: 1.2.3 (final)"
                                                # rewrite previous message
  bump-semver vcs commit --amend VERSION        # fold ONLY VERSION into previous
  bump-semver vcs commit --amend --staged -m "fixup"
                                                # fold all staged into previous
`

// helpVcsFetch documents `vcs fetch [REMOTE]` (DR-0020 PR-5).
//
// The verb is a thin, opinionated wrapper over each backend's native
// fetch:
//
//   - git: `git fetch <remote>`
//   - jj:  `jj git fetch --remote <remote>`
//
// Single positional arg for REMOTE keeps the surface minimal — refspec
// scoping, prune flags, and tag controls intentionally pass through the
// underlying tool unchanged (= use plain `git fetch ...` / `jj git fetch
// ...` for those).
const helpVcsFetch = `bump-semver vcs fetch — refresh refs from a remote (git/jj-agnostic) [DR-0020]

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
    underlying tool's stderr folded in.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (refs refreshed; no-op if remote unchanged)
  2   usage error (too many positionals, unknown flag)
  3   VCS subprocess error (unknown remote, network failure, not a repo)

Examples:
  bump-semver vcs fetch                 # fetch origin
  bump-semver vcs fetch upstream        # fetch a specific remote
  bump-semver vcs fetch --remote origin # same as the bare form
`

// helpVcsPush documents `vcs push --branch|--bookmark NAME [--remote
// REMOTE]` (DR-0020 PR-5).
//
// The verb deliberately requires NAME (no auto-detection from
// current-branch / heads(::@)) and does NOT expose --force or --tags.
// Both restrictions are kawaz CLI safety stances:
//
//   - Auto-NAME would be silently wrong when the user is on the wrong
//     branch / bookmark. Better to fail with "what would you like to
//     push?" than to push the wrong ref.
//   - Force push is a rewrite of remote history. bump-semver is a SemVer
//     helper, not a publishing tool — the user has the underlying
//     `git push --force-with-lease` / `jj git push --force-with-lease`
//     available when they actually need that capability.
//
// --branch is the canonical name (matches the cross-VCS vocabulary
// already used by `vcs get current-branch`). --bookmark is an alias for
// jj users who think in bookmarks; the flags are interchangeable but
// supplying both at once is a usage error.
const helpVcsPush = `bump-semver vcs push — upload refs to a remote (git/jj-agnostic) [DR-0020]

Usage:
  bump-semver vcs push --branch NAME [--remote REMOTE]

Arguments:
  --branch NAME    Branch to push (jj users: bookmark). Required — no
                   auto-detection. --bookmark accepted as a synonym.
  --remote REMOTE  Target remote name. Defaults to "origin" when omitted.

Notes:
  - git: runs 'git push <remote> <name>:<name>'. The explicit refspec
    avoids surprises from local push.default / tracking config.
  - jj:  runs 'jj git push --bookmark <name> --remote <remote>' followed
    by 'jj git export' so colocated .git refs stay in sync. Export is
    retried once on failure; persistent failures surface as exit 3 with
    a recovery hint (see jj-vcs/jj issues #493, #6098, #6203).
  - Idempotent: "remote already has it" → exit 0; git/jj's own
    "Everything up-to-date" / "Nothing changed" line is forwarded to
    stderr so the user can see the convergence happened.
  - Non-fast-forward (remote rejected the push) → exit 5; the underlying
    git/jj stderr is passed through verbatim. Recovery (fetch + reconcile,
    or force push if you really mean it) is your call — bump-semver does
    not paraphrase the tool's diagnostic.

Not provided by design:
  --force / --force-with-lease   Rewriting remote history is out of
                                 scope. Use the underlying git/jj tool
                                 directly when you genuinely need it.
  --tags / --all                 Tag and bulk pushes are out of scope.
                                 The bump-semver release flow puts each
                                 tag/ref through CI/CD, not a manual push.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (push completed, or idempotent up-to-date)
  2   usage error (--branch/--bookmark missing or specified twice,
                   --force passed, positional args supplied, unknown flag)
  3   VCS subprocess error (unknown remote, network failure, not a repo,
                            jj git export failure persisted across retry)
  5   non-fast-forward rejection — read git/jj's stderr for details

Examples:
  bump-semver vcs push --branch main         # push main to origin
  bump-semver vcs push --branch main --remote upstream
                                             # custom remote
  bump-semver vcs push --branch release-1.2  # push a feature/release branch
`

// actionHelpTexts dispatches per-action help. Keys are CLI action
// names. major/minor/patch share helpBump because the action name
// itself disambiguates which component is bumped.
//
// Two-tier verbs use space-separated keys ("vcs get"). The parent verb
// ("vcs") gets the parent help; per-verb keys map to the per-verb help.
var actionHelpTexts = map[string]string{
	"major":      helpBump,
	"minor":      helpBump,
	"patch":      helpBump,
	"pre":        helpPre,
	"get":        helpGet,
	"compare":    helpCompare,
	"vcs":        helpVcs,
	"vcs get":    helpVcsGet,
	"vcs is":     helpVcsIs,
	"vcs diff":   helpVcsDiff,
	"vcs commit": helpVcsCommit,
	"vcs fetch":  helpVcsFetch,
	"vcs push":   helpVcsPush,
}
