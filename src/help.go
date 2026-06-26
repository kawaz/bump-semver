package main

// This file holds only the two ROOT-level help screens: the short
// overview (--help / no args) and the full reference (--help-full). They
// are bespoke (not derived from the command tree) because the root has no
// flags of its own to enumerate and the overview is a curated index.
//
// Per-command help (major / pre / get / compare / vcs ...) lives in
// cobra_help_text.go (prose) + cobra_help.go (rendering): its Options
// sections are generated from the live FlagSet, the single source of
// truth for flags.

// shortHelpText is the default --help output: a one-screen overview that
// points at the per-command help and at --help-full as the authoritative
// reference.
const shortHelpText = `bump-semver — focused semver bump CLI

Usage:
  bump-semver <command> [args...]
  bump-semver <command> --help     (= per-command detail)
  bump-semver --version
  bump-semver --help | --help-full

Commands:
  <major|minor|patch|pre>   Bump version (FILE / VER input; pre = counter advance / set / remove)
  get                       Read the current version
  compare                   Compare SemVer values via <eq|lt|le|gt|ge|...>
  vcs                       VCS helpers (git/jj-agnostic; sub-tree: vcs --help)

See 'bump-semver <command> --help' for arguments / options / examples,
or 'bump-semver --help-full' for the complete reference.

Inputs are positional: FILE / VER / - / vcs:REV[:FILE] / cmd:CMD.
(Latest tag / release lookups also available as 'vcs:latest-tag([REPO])'
/ 'vcs:latest-release([REPO])' input records or 'vcs get latest-{tag,release}'
subcommands.)
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

Compare (nested subcommand, BASE plus one or more OTHERS):
  compare eq  BASE OTHER...   true if BASE equals every OTHER (SemVer 2.0.0 ordering, build metadata ignored)
  compare lt  BASE OTHER...   true if BASE <  every OTHER
  compare le  BASE OTHER...   true if BASE <= every OTHER
  compare gt  BASE OTHER...   true if BASE >  every OTHER
  compare ge  BASE OTHER...   true if BASE >= every OTHER
  Optional -major / -minor / -patch suffix truncates the comparison
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
                             (= revived in v0.32.0; richer option
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
  Cargo.toml         TOML, [package].version (try) -> [workspace.package].version (fallback)
  pyproject.toml     TOML, [project].version (try) -> [tool.poetry].version (fallback)
  mojoproject.toml   TOML, [workspace].version (and [workspace].name)
  package-lock.json  npm 7+ lockfile, $.version + $.packages[""].version (deps untouched)
  pom.xml            XML element, /project/version (and /project/artifactId)
  *.json             JSON, $.version (and optional $.name)
  *.yaml / *.yml     YAML, top-level .version (and optional .name)
  *.toml             TOML, top-level version  (and optional name) 
  v.mod / build.zig.zon / mix.exs / build.sbt        text + regex (basename)
  build.gradle / build.gradle.kts                    text + regex (basename)
  moon.mod           MoonBit module (DSL), top-level version = "X" (and name)
  moon.mod.json      MoonBit module (legacy JSON), $.version (and $.name)
  *.xcconfig / *.podspec / *.nimble / *.gemspec      text + regex (fallback)
  *.cabal / *.spec                                   text + regex (fallback)
  *.csproj / *.fsproj / *.vbproj                     XML element, /Project/PropertyGroup/Version
  VERSION            text (whole file = version, no regex)

  Backup-style suffix fallback: Cargo.toml.bak / package.json.20260510 /
  Chart.yaml~ etc. strip one trailing suffix and retry against the table above.
  Suffixes: .bak / .backup / .orig / .tmp / .old / .YYYYMMDD / .YYYYMMDD_HHMMSS / ~

Multiple inputs (FILE / VER / - / vcs: / cmd:) may be mixed. All extracted
versions must agree; otherwise a "version mismatch:" error lists each origin
and value. With --write, only FILE-origin inputs are written back (vcs: and
cmd: are read-only).

Not in the table? Define your own rule with --define-rule:

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

VCS helpers:
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
