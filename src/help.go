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
  bump-semver <command> [args...]
  bump-semver <command> --help     (= per-command detail)
  bump-semver --version
  bump-semver --help | --help-full

Commands:
  <major|minor|patch|pre>   Bump version (FILE / VER 入力、pre は counter advance / set / remove)
  get                       Read the current version
  compare                   Compare SemVer values via <eq|lt|le|gt|ge|...>
  vcs                       VCS helpers (git/jj-agnostic; sub-tree: vcs --help)

See 'bump-semver <command> --help' for arguments / options / examples,
or 'bump-semver --help-full' for the complete reference.

Inputs are positional: FILE / VER / - / vcs:REV[:FILE] / cmd:CMD.
(Latest tag / release lookups also available as 'vcs:latest-tag([REPO])'
/ 'vcs:latest-release([REPO])' input records or 'vcs get latest-{tag,release}'
subcommands. See DR-0032.)
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
  bump-semver compare <OP> <BASE> <OTHER...> [flags]
  bump-semver --version
  bump-semver --help | --help-full

Commands (bump/read):
  major   Bump major (X.0.0); pre-release / build-metadata dropped by default
  minor   Bump minor (x.Y.0); pre-release / build-metadata dropped by default
  patch   Bump patch (x.y.Z); pre-release / build-metadata dropped by default
  pre     Pre-release counter advance / set / remove (see --pre / --no-pre)
  get     Print the current version (with optional --no-pre / --no-build-metadata)

Compare (nested subcommand, DR-0023: BASE plus one or more OTHERS):
  compare eq  BASE OTHER...   true if BASE equals every OTHER (SemVer 2.0.0 ordering, build metadata ignored)
  compare lt  BASE OTHER...   true if BASE <  every OTHER
  compare le  BASE OTHER...   true if BASE <= every OTHER
  compare gt  BASE OTHER...   true if BASE >  every OTHER
  compare ge  BASE OTHER...   true if BASE >= every OTHER
  Optional -major / -minor / -patch suffix (DR-0017) truncates the comparison
  (e.g. eq-major 1.2.3 1.9.7 -> true). See: bump-semver compare --help

Inputs:
  FILE                       path to a supported file (auto-detected by basename)
  VER                        a raw semver string (e.g. 1.2.3, v1.2.3, 1.2.3-rc.1+build.42)
  -                          read VER from stdin (single line, used at most once)
  vcs:REV[:FILE]             read FILE at <REV> from the VCS (jj or git, auto-detected)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
                             (read-only, strips a leading 'v'; e.g. cmd:mytool --version)
  vcs:latest-tag([REPO])     largest stable SemVer tag (cwd or external repo)
  vcs:latest-release([REPO]) largest stable GitHub Release (gh CLI required)
                             (= revived in v0.32.0 per DR-0032; richer option
                              set in 'vcs get latest-{tag,release}' subcommands)

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
  v.mod / build.zig.zon / mix.exs / build.sbt        text + regex (basename) [DR-0012 / DR-0030]
  build.gradle / build.gradle.kts                    text + regex (basename) [DR-0018 / DR-0030]
  *.xcconfig / *.podspec / *.nimble / *.gemspec      text + regex (fallback) [DR-0012 / DR-0030]
  *.cabal / *.spec                                   text + regex (fallback) [DR-0018 / DR-0030]
  *.csproj / *.fsproj / *.vbproj                     XML element, /Project/PropertyGroup/Version [DR-0018]
  VERSION            text (whole file = version, no regex)

  Backup-style suffix fallback (DR-0013): Cargo.toml.bak / package.json.20260510 /
  Chart.yaml~ etc. strip one trailing suffix and retry against the table above.
  Suffixes: .bak / .backup / .orig / .tmp / .old / .YYYYMMDD / .YYYYMMDD_HHMMSS / ~

Multiple inputs (FILE / VER / - / vcs: / cmd:) may be mixed. All extracted
versions must agree; otherwise a "version mismatch:" error lists each origin
and value. With --write, only FILE-origin inputs are written back (vcs: and
cmd: are read-only).

Not in the table? Define your own rule with --define-rule (DR-0029):

  --define-rule <PATTERN>    Open a rule block for SOURCES matching <PATTERN>
                             (= absolute path, relative path, basename, or
                              glob:<pattern>). Subsequent --format / --version-*
                              / --name-* flags belong to this block until the
                              next --define-rule.
  --format <FMT>             text|json|yaml|toml|xml. xml resolves the final
                             path segment against both a child element and an
                             attribute (same value = ok, differing = ambiguous).
  --version-path <DOTPATH>   For json/yaml/toml/xml: where the version field is
                             (e.g. $.version, plugin.version, deps[0].version).
  --version-regex <PATTERN>  For text: regex with one capture group
                             (exact one match required, 0/2+ matches = error).
  --name-path / --name-regex Optional package-name extraction (symmetric).

  Rule-definition flags placed BEFORE the first --define-rule act as the
  global default (= applies to every SOURCE not covered by a named block).
  CLI rules always override builtin rules; an extraction failure on a CLI
  rule is a hard error (no silent fall-through to builtin).

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
  1   predicate false (compare: per-OTHER detail on stderr; vcs is / get version|name mismatch: source listing on stderr; suppress with -qq)
  2   usage error (parse failure, bump-time version|name mismatch, missing input, unknown verb/key, etc.)
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
  LATEST=$(bump-semver vcs get latest-tag); bump-semver compare gt Cargo.toml "$LATEST"
                                                         # ready to release? (CI, capture-then-compare)
  bump-semver compare gt Cargo.toml 'vcs:latest-tag()'   # ready to release? (1-liner input record)
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

When multiple INPUTs are given, all sources are treated as equal
peers and must agree; otherwise a "version mismatch:" (or
"name mismatch:" when package names diverge) listing is printed to
stderr and the process exits 1 (DR-0023). exit 0 + single-line
stdout on agreement makes the command safe to pipe.

A file-omitted vcs:REV expands across every distinct sibling FILE
path. 'get a b vcs:main' therefore reads four sources: a, b, the
snapshot of a at main, and the snapshot of b at main (DR-0008).

Inputs (multiple, must agree):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
                             (when FILE is omitted, expands across all
                             sibling FILE paths — see Examples)
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
                             (strips a leading 'v'; e.g. cmd:./bin/mytool --version)
  vcs:latest-tag([REPO])     largest stable SemVer tag (cwd or external repo)
  vcs:latest-release([REPO]) largest stable GitHub Release (gh CLI required)
                             (= revived in v0.32.0 per DR-0032; richer option
                              set in 'vcs get latest-{tag,release}' subcommands)

Options:
  --json                     Structured JSON output (.name, .version, .semver,
                             .major / .minor / .patch / .pre / .build_metadata, ...)
  --no-pre                   Strip pre-release from the printed value
  --no-build-metadata        Strip build metadata from the printed value
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / -qq / --no-hint       Output suppression (see --help-full)

Exit codes:
  0   every source agrees
  1   sources disagree (per-source listing on stderr)
  2   error (parse failure, missing input, unknown flag, ...)

Examples:
  bump-semver get VERSION
  bump-semver get Cargo.toml package.json package-lock.json   # cross-file agreement check
  bump-semver get a b 'vcs:main@origin'                        # 4-way: a, b, vcs:main:a, vcs:main:b
  bump-semver get package.json --json | jq -r .semver
  bump-semver vcs get latest-tag                               # largest semver tag (cwd) — bare
  bump-semver vcs get latest-tag --json | jq -r .version       # raw tag string (e.g. v1.2.3)
  bump-semver vcs get latest-tag --repository kawaz/pkf-tasks  # remote (GitHub short)
  bump-semver get 'vcs:latest-tag()'                           # input record (1-liner)
  bump-semver get 'cmd:./bin/mytool --version'                 # run a command, parse its output
`

// helpCompare documents the compare subcommand. The OP list is the
// authoritative reference for which operators are supported — the
// shortHelpText only shows it as `<eq|lt|le|gt|ge|...>` to stay short.
const helpCompare = `bump-semver compare — compare a base value to one or more others (exit-code-driven)

Usage:
  bump-semver compare <OP> <BASE> <OTHER...>

BASE (the first input) is the reference; every OTHER is compared as
"BASE OP OTHER". The legacy two-input form is the N=1 case. Each
OTHER is evaluated independently — failures are listed on stderr
without short-circuit, so a single invocation surfaces every failing
relation (DR-0023).

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

Inputs (BASE plus one or more OTHERS):
  FILE                       supported file (basename auto-detected)
  VER                        raw semver string
  -                          read VER from stdin
  vcs:REV[:FILE]             read FILE at <REV> from jj or git
  cmd:CMD                    run CMD via bash -c, take first non-empty stdout line as VER
  vcs:latest-tag([REPO])     largest stable SemVer tag (cwd or external repo)
  vcs:latest-release([REPO]) largest stable GitHub Release (gh CLI required)
                             (= revived in v0.32.0 per DR-0032)

  When an OTHER's vcs: spec has no explicit FILE component, it
  borrows BASE's path (DR-0008 / DR-0023). 'compare gt VERSION
  vcs:main vcs:v1.0.0' therefore reads vcs:main:VERSION and
  vcs:v1.0.0:VERSION.

Options:
  --vcs jj|git|auto          Force VCS detection for vcs: inputs (default: auto)
  -q / --no-hint             Suppress DR-0010 hints (per-OTHER failure list is preserved)
  -qq                        Also suppress the per-OTHER failure list

  --write / --json / --pre / --build-metadata: rejected (compare is
  read-only and exit-code-driven, not value-output-driven).

Exit codes:
  0   every predicate is true
  1   at least one predicate is false (per-OTHER detail on stderr)
  2   error (parse failure, missing input, unknown OP, etc.)

Examples:
  bump-semver compare eq 1.2.3 1.2.3
  bump-semver compare lt 1.2.3-rc.1 1.2.3                    # exit 0 (rc.1 < 1.2.3)
  bump-semver compare gt VERSION 'vcs:latest-tag()'          # ready to release? (1-liner)
  LATEST=$(bump-semver vcs get latest-tag); bump-semver compare gt Cargo.toml "$LATEST"
                                                             # same, capture-then-compare (CI)
  bump-semver compare gt VERSION 'vcs:main@origin' 'vcs:v1.0.0'  # ahead of main AND of v1.0.0
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
  bump-semver vcs <command> [args...]
  bump-semver vcs <command> --help     (= per-command detail)
  bump-semver vcs --help               (= this list)

Commands:
  get        Read a value from the VCS (root / backend / current-branch)
  is         Test a predicate (clean / dirty / git / jj)
  diff       Print the patch between a rev and the working copy
  commit     Commit working-tree content (paths or --staged), incl. --amend
  fetch      Fetch refs from a remote
  push       Push a branch / bookmark to a remote
  tag        Manage tags atomically (push / delete / latest)
  outdated   Derived-sync check via FROM→TO mapping (DR-0027 / DR-0028)

See 'bump-semver vcs <command> --help' for arguments, options, and examples.

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
  4   ambiguous answer (e.g. detached HEAD, multiple bookmarks) —
      also used by 'vcs tag push' when an existing tag points at a
      different rev and --allow-move was not passed (integrity violation)
  5   non-fast-forward rejection (vcs push — remote has diverged)
`

// helpVcsGet documents `vcs get <key>`.
//
// The three keys are intentionally minimal — anything more elaborate
// belongs in a dedicated verb (e.g. tag listing in a future `vcs tag
// list`). Keep the set tight so callers can rely on every key being
// equally cheap and equally well-defined.
const helpVcsGet = `bump-semver vcs get — read a value from the VCS [DR-0020 / DR-0032]

Usage:
  bump-semver vcs get <key> [key-specific options...]

Keys:
  root             Absolute path to the repository root
  backend          The detected backend: "git" or "jj"
  current-branch   The unambiguous current branch (git) / bookmark (jj)
                   git:  HEAD's symbolic-ref short name. Detached HEAD → exit 4.
                   jj:   The single bookmark naming heads(::@ & bookmarks()).
                         Zero / multiple bookmarks at the head → exit 4.
  latest-tag       Largest SemVer-parseable tag (cwd VCS or via --repository).
                   See 'vcs get latest-tag --help' for options (DR-0032).
  latest-release   Largest SemVer-parseable GitHub Release (gh CLI required).
                   See 'vcs get latest-release --help' for options (DR-0032).

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
  bump-semver vcs get latest-tag              # largest stable SemVer tag
  bump-semver vcs get latest-release          # largest stable GH Release
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
const helpVcsDiff = `bump-semver vcs diff — print the patch between REV and the working copy [DR-0020 / DR-0033]

Usage:
  bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..] [--excludes PATTERN]...

Arguments:
  REV          The revision to compare against (git: any rev-spec like
               HEAD~1, origin/main, <sha>; jj: any revset like @-, main@origin).
  PATH..       Optional path filter (= include set). Each entry is a
               literal path, 'glob:<pattern>' (DR-0024), or
               'file:<path>' (DR-0033 — read newline-separated path list
               from <path>; '#' comments and blank lines skipped, lines
               accept literal or 'glob:' shapes). Nonexistent paths are
               silently ignored (declarative convergence). When every
               PATH is filtered out, stdout is empty and exit is 0 — the
               verb does NOT widen back to "all paths" in that case.

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

Options:
  -s, --name-status      Emit one '<CODE>\\t<path>' line per changed
                         file (M/A/D) instead of the raw patch.
                         Mirrors 'git diff --name-status'.
  --excludes PATTERN     Drop paths matching PATTERN from the diff.
                         Repeatable (= append, each --excludes adds one
                         pattern). Order-independent: final set = include
                         ∖ ⋃(excludes). PATTERN accepts the same shape
                         as positional PATH (literal / glob: / file:).
                         Constraint: at least one positional PATH must
                         be given when --excludes is used.
                         Forwarded to the backend as native pathspec
                         (git: ':(exclude,glob)<pat>' magic pathspec;
                         jj: single fileset expression '(includes) ~ pat'),
                         so literal directory includes (e.g. 'src/') and
                         deletions (files present in REV but removed in
                         the working copy) are handled by git/jj natively.
                         'file:' excludes are expanded locally first into
                         a flat list of patterns before forwarding.

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
  bump-semver vcs diff -q HEAD~1 src/ --excludes 'glob:src/**/*_test.go'
                                                # is non-test src/ unchanged? (DR-0033)
  bump-semver vcs diff HEAD~1 file:.bump-targets --excludes file:.bump-excludes
                                                # include / exclude via external lists
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
                       [--jj-bookmark-auto-advance]

Arguments:
  --branch NAME    Branch to push (jj users: bookmark). Required — no
                   auto-detection. --bookmark accepted as a synonym.
  --remote REMOTE  Target remote name. Defaults to "origin" when omitted.

jj-specific options (silent no-op on git — backend-prefix general rule):
  --jj-bookmark-auto-advance   Move the bookmark to the publishable
                               commit before pushing. Target depends on
                               the working copy:
                                 - clean (@ empty)     → bookmark → @-
                                 - dirty (@ non-empty) → bookmark → @
                               Preconditions enforced before any move:
                                 (a) the bookmark exists (otherwise we
                                     fall through to the normal push and
                                     let jj surface its own error);
                                 (b) the bookmark is in ancestors(@)
                                     (sideways/divergent → exit 3 with a
                                     hint, no move). The move itself is
                                     forward-only — jj's default refuses
                                     backwards/sideways moves and we do
                                     NOT pass --allow-backwards.

                               Why: jj 慣習 places bookmarks on the
                               confirmed parent commit (@-), not on the
                               throw-away working copy (@). Manually
                               running 'jj bookmark move' every bump
                               is friction; this flag automates it
                               while keeping the safety checks explicit.
                               Opt-in by design — silent advancement
                               would surprise users who positioned the
                               bookmark intentionally.

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
                            jj git export failure persisted across retry,
                            --jj-bookmark-auto-advance refused: bookmark
                            not in ancestors(@))
  5   non-fast-forward rejection — read git/jj's stderr for details

Examples:
  bump-semver vcs push --branch main         # push main to origin
  bump-semver vcs push --branch main --remote upstream
                                             # custom remote
  bump-semver vcs push --branch release-1.2  # push a feature/release branch
  bump-semver vcs push --branch main --jj-bookmark-auto-advance
                                             # jj: auto-move bookmark to
                                             # @- (clean) or @ (dirty)
                                             # before pushing
`

// helpVcsTag documents the `vcs tag` parent verb (DR-0020 PR-6).
//
// `vcs tag` is the first two-tier verb in the family; the parent help
// just enumerates the sub-verbs and points at their dedicated help.
// Per kawaz CLI design preferences: sections in order (sub-verb list,
// global options, exit codes), long options only.
const helpVcsTag = `bump-semver vcs tag — manage tags atomically (create+push / delete) [DR-0020]

Usage:
  bump-semver vcs tag <command> [args...]
  bump-semver vcs tag --help

Commands:
  push       Create / move tag at a rev and push to a remote
  delete     Remove tag both locally and on the remote (idempotent)

See 'bump-semver vcs tag <command> --help' for arguments and options.
For reading the latest tag/release, use 'vcs get latest-tag' / 'vcs get
latest-release' or input records 'vcs:latest-tag()' / 'vcs:latest-release()'
(DR-0032).

Notes:
  - 'tag push' is intentionally NOT separable into "tag locally then push later".
    The verb's contract is "the tag points to REV on the remote when this returns";
    the local create is the means, not the deliverable. This keeps tags
    1-1 with their remote presence (DR-0020 design — no orphan local tags
    that didn't make it out).
  - 'tag delete' removes both halves (local + remote) — pair with 'tag push'
    so the lifecycle stays symmetric. Either half being missing is fine
    (idempotent rm -f semantic).
  - 'tag list' is NOT provided — use 'git tag --list' / 'jj tag list'
    directly; the underlying tools' filters / templates are richer than
    anything a bump-semver shim would expose.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)
  --help, -h             Show this help

Exit codes:
  0   success (incl. idempotent same-rev push, absent-tag delete)
  2   usage error (command missing / unknown, NAME shape problem)
  3   VCS subprocess error (unknown remote, bad REV, network failure)
  4   integrity violation: 'tag push' against an existing different-rev tag
      without --allow-move (distinct from 3 so callers can detect
      "tag drifted" vs "git/jj broke")
`

// helpVcsTagPush documents `vcs tag push --rev REV NAME [--remote REMOTE]
// [--allow-move]` (DR-0020 PR-6).
const helpVcsTagPush = `bump-semver vcs tag push — create or move a tag and push it [DR-0020]

Usage:
  bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]

Arguments:
  NAME             Tag name (e.g. "v1.2.3"). Required. Must not be empty,
                   must not contain whitespace, must not start with "refs/"
                   (the "refs/tags/" prefix is added automatically).

Options:
  --rev REV        Target revision (any git rev-spec / jj revset). Required.
  --remote REMOTE  Target remote. Defaults to "origin".
  --allow-move     Permit moving an existing tag to a different REV.
                   Without this flag, a different-rev tag is exit 4
                   (integrity violation). Same-rev re-push is always OK.

Behaviour:
  - absent local tag           → create at REV, push to remote
  - local tag at same REV      → skip local create, still push
                                 (片落ちリカバリ: remote may be missing it
                                 even when local has it; the push is a
                                 clean no-op if remote also matches)
  - local tag at different REV → exit 4 (no side-effect), unless
                                 --allow-move is set, in which case the
                                 tag moves and is force-pushed.
  - bad REV                    → exit 3 (resolution failure surfaces
                                 before any side-effect).

Not provided by design:
  --force / --force-with-lease   Use --allow-move. Force is too broad —
                                 it conflates "same-rev idempotent push"
                                 with "different-rev rewrite"; --allow-move
                                 is the precise opt-in (DR-0020 line 71/91).
  --tags / --all                 Bulk operations are out of scope.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (incl. idempotent same-rev re-push)
  2   usage error (NAME missing / bad shape, --rev missing, --force passed)
  3   VCS subprocess error (unknown remote, bad REV, network failure)
  4   integrity violation: tag exists at a different REV, --allow-move
      not set. No local move, no push attempted.

Examples:
  bump-semver vcs tag push --rev HEAD v1.2.3
                                                # tag HEAD as v1.2.3, push to origin
  bump-semver vcs tag push --rev "$(bump-semver get VERSION)" v1.2.3
                                                # not useful; use a rev-spec, not a version
  bump-semver vcs tag push --rev main v1.2.3 --remote upstream
                                                # tag main, push to upstream
  bump-semver vcs tag push --rev HEAD~1 v1.2.3 --allow-move
                                                # move existing v1.2.3 back one commit
`

// helpVcsTagDelete documents `vcs tag delete NAME [--remote REMOTE]`
// (DR-0020 PR-6).
const helpVcsTagDelete = `bump-semver vcs tag delete — remove a tag locally and on a remote [DR-0020]

Usage:
  bump-semver vcs tag delete NAME [--remote REMOTE]

Arguments:
  NAME             Tag name to delete. Required.
  --remote REMOTE  Target remote. Defaults to "origin".

Behaviour:
  - Removes both the local tag AND the remote tag.
  - Idempotent (rm -f semantic): an absent tag on either side is exit 0,
    not an error. The verb's intent is the end-state "NAME has no tag",
    which an already-absent tag already satisfies.
  - A genuine remote failure (unknown remote, network down) is exit 3;
    the local-half side-effect may have already happened (we accept that
    asymmetry — the common case is "remote is fine, just clean up").

Not provided by design:
  --allow-missing  Delete is natively idempotent; the flag would be no-op
                   in every case (DR-0020 line 74 / 92).

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (tag removed from both sides, OR already absent — same result)
  2   usage error (NAME missing / bad shape)
  3   VCS subprocess error (unknown remote, network failure, not a repo)

Examples:
  bump-semver vcs tag delete v0.9.0             # remove from local + origin
  bump-semver vcs tag delete v0.9.0 --remote upstream
                                                # delete from a non-default remote
`

// helpVcsGetLatestTag / helpVcsGetLatestRelease document the v0.32.0
// successors to `vcs tag latest` (DR-0032). The source axis (tag vs
// release) is folded into the verb name so each verb has a single,
// honest responsibility.
const helpVcsGetLatestTag = `bump-semver vcs get latest-tag — print the SemVer-largest tag [DR-0032]

Usage:
  bump-semver vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]

Behaviour:
  Lists tag refs from the cwd VCS (default) or an external repo, drops
  anything that doesn't parse as SemVer 2.0.0, and returns the largest.
  Pre-release tags (v1.2.3-rc.1 etc.) are excluded by default; pass
  --include-prerelease to include them.

Options:
  --repository REPO      External repo target: owner/repo (GitHub short)
                         or full HTTPS/SSH URL. Default: cwd VCS.
                         External path uses git ls-remote --tags (no gh).
  --include-prerelease   Include pre-release tags (default: excluded).
  --json                 Structured JSON output (= same 12-field version
                         schema as 'get --json'). .version preserves the
                         raw tag string (e.g. "v1.2.3"); .semver is the
                         canonical bare form. .name surfaces the prefix
                         from monorepo-style tags (pkf-tasks@0.0.13 →
                         "pkf-tasks").

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto, .jj wins over .git)
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (tag found and emitted)
  2   usage error (extra positional arguments)
  3   VCS subprocess error / no semver-compatible tags found

Examples:
  bump-semver vcs get latest-tag                     # cwd: largest SemVer tag (bare)
  bump-semver vcs get latest-tag --include-prerelease  # include v1.2.3-rc.1 etc.
  bump-semver vcs get latest-tag --json              # structured (= get --json schema)
  bump-semver vcs get latest-tag --repository kawaz/pkf-tasks
                                                     # remote (git ls-remote --tags)
  bump-semver get 'vcs:latest-tag()'                 # input record (= 1-liner ergonomic)
  bump-semver compare gt VERSION 'vcs:latest-tag()'  # 1-liner CI 比較
`

const helpVcsGetLatestRelease = `bump-semver vcs get latest-release — print the SemVer-largest GitHub Release [DR-0032]

Usage:
  bump-semver vcs get latest-release [--include-prerelease] [--repository REPO] [--json]

Behaviour:
  Reads GitHub Release objects via the gh CLI, drops drafts, drops anything
  that doesn't parse as SemVer 2.0.0, and returns the largest. Pre-releases
  are excluded by default; pass --include-prerelease to include them.

Options:
  --repository REPO      External repo target: owner/repo (GitHub short)
                         or full HTTPS/SSH URL. Default: cwd repo
                         (gh auto-detects).
  --include-prerelease   Include pre-release tags (default: excluded).
  --json                 Structured JSON output (= same 12-field version
                         schema as 'get --json'). .version preserves the
                         raw release name; .semver is the canonical
                         bare form.

External Tool Dependencies:
  gh CLI                 Required (https://cli.github.com/). Missing gh →
                         exit 3 with an install hint.

Global Options:
  --vcs jj|git|auto      Force VCS detection (default: auto). gh's own
                         repo detection is independent of this flag.
  -q, --quiet            Suppress stdout (errors still printed)
  -qq, --quiet-all       Suppress stdout, hint, and error output (use with caution)

Exit codes:
  0   success (release found and emitted)
  2   usage error (extra positional arguments)
  3   gh subprocess error, gh missing, or no semver-compatible releases

Examples:
  bump-semver vcs get latest-release                 # cwd repo (gh-detected): largest release
  bump-semver vcs get latest-release --include-prerelease
                                                     # include rc.1 etc.
  bump-semver vcs get latest-release --json          # structured output
  bump-semver vcs get latest-release --repository kawaz/bump-semver
                                                     # external GitHub repo
  bump-semver get 'vcs:latest-release(kawaz/pkf-tasks)'
                                                     # input record (1-liner)
`

// helpVcsOutdated — see DR-0027 / DR-0028 + `docs/specs/glob-backref-v0.1.0.md`.
const helpVcsOutdated = `bump-semver vcs outdated — derived-sync check via FROM→TO mapping [DR-0027 / DR-0028]

Usage:
  bump-semver vcs outdated FROM TO[..]
  bump-semver vcs outdated -- FROM TO[..] -- FROM TO[..] -- ...
  bump-semver vcs outdated [--explain] [--strict] ...

Compare committer timestamps: each TO file must be at least as new as
the FROM that produced it. Exit 1 = stale or missing-mandatory derived.

  FROM   Literal path or 'glob:<pat>'.
  TO     Each may use:  $N / ${N}    capture from FROM ($0 = full match)
                        {a,b,c}      MANDATORY expansion (every option must exist)
                        *, **, []    OPTIONAL fs discovery (no match = silent skip)
                        'glob:<pat>' 2-stage TO discovery (escape-aware)
  --     Pair separator. Single pair: optional. N≥2 pairs: required.

Options:
  --explain                       Print (source → derived) rows + status; exit 0.
  --strict                        Literal FROM no match → exit 1 (CI gate).
  --glob-dotfile=true|false       (default false)  Include dotfile paths in FROM expansion.
  --glob-gitignored=true|false    (default true)   Respect .gitignore in FROM expansion.
  --glob-ignorecase[=true|false]  (default false)  Case-insensitive match.

Examples (always single-quote; bash eats $1, {a,b}, etc.):
  bump-semver vcs outdated 'glob:src/**/*.ts' 'lib/$1/$2.js'
  bump-semver vcs outdated README.md 'README-{ja,en}.md'
  bump-semver vcs outdated --explain 'glob:**/*-ja.md' '$1/$2.md'

Exit codes:  0 fresh  /  1 stale or missing  /  2 usage  /  3 vcs

Reference (grammar, backref numbering, edge cases, design rationale):
  https://github.com/kawaz/bump-semver
`

// actionHelpTexts dispatches per-action help. Keys are CLI action
// names. major/minor/patch share helpBump because the action name
// itself disambiguates which component is bumped.
//
// Two-tier verbs use space-separated keys ("vcs get"). The parent verb
// ("vcs") gets the parent help; per-verb keys map to the per-verb help.
// Three-tier paths ("vcs tag push") are introduced by PR-6.
var actionHelpTexts = map[string]string{
	"major":                  helpBump,
	"minor":                  helpBump,
	"patch":                  helpBump,
	"pre":                    helpPre,
	"get":                    helpGet,
	"compare":                helpCompare,
	"vcs":                    helpVcs,
	"vcs get":                helpVcsGet,
	"vcs is":                 helpVcsIs,
	"vcs diff":               helpVcsDiff,
	"vcs commit":             helpVcsCommit,
	"vcs fetch":              helpVcsFetch,
	"vcs push":               helpVcsPush,
	"vcs tag":                helpVcsTag,
	"vcs tag push":           helpVcsTagPush,
	"vcs tag delete":         helpVcsTagDelete,
	"vcs get latest-tag":     helpVcsGetLatestTag,
	"vcs get latest-release": helpVcsGetLatestRelease,
	"vcs outdated":           helpVcsOutdated,
}
