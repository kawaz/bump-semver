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

Action-specific help: bump-semver <action> --help
Full reference:       bump-semver --help-full

Inputs are positional: FILE / VER / - / vcs:REV[:FILE] / vcs:latest-tag([REPO]).
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
  Cargo.toml         TOML, [package].version (and [package].name for cross-input checks)
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

Multiple inputs (FILE / VER / -) may be mixed. All extracted versions must
agree; otherwise a "version mismatch:" error lists each origin and value.
With --write, only FILE-origin inputs are written back.

Exit codes:
  0   success (or compare predicate true)
  1   compare predicate false
  2   error (parse failure, mismatch, missing input, etc.)

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

Options:
  --write                    Write the bumped version back to each FILE input
                             (vcs:/VER/- inputs are reference-only; --write requires ≥ 1 FILE)
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
`

// actionHelpTexts dispatches per-action help. Keys are CLI action
// names. major/minor/patch share helpBump because the action name
// itself disambiguates which component is bumped.
var actionHelpTexts = map[string]string{
	"major":   helpBump,
	"minor":   helpBump,
	"patch":   helpBump,
	"pre":     helpPre,
	"get":     helpGet,
	"compare": helpCompare,
}
