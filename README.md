# bump-semver

> English | [日本語](./README-ja.md)

A focused CLI for reading, bumping, and comparing the semver string in version-tracking files. Detects the file format by basename (no `--pattern` regex flag), supports five flat actions (`major` / `minor` / `patch` / `pre` / `get`) plus one nested subcommand (`compare`). The new version is always written to stdout so it composes well in shell pipelines.

## Why

Existing version-bump CLIs are either too generic (require a regex / pattern flag for every invocation) or limited to a single file format. `bump-semver` takes the opposite stance: it covers exactly the formats kawaz actually uses and adds new ones only when concretely needed. The result is a `kawaz-grade` tool — small, opinionated, predictable.

## Install

```bash
brew install kawaz/tap/bump-semver
```

`kawaz/tap` is [`kawaz/homebrew-tap`](https://github.com/kawaz/homebrew-tap). Two-step equivalent: `brew tap kawaz/tap && brew install bump-semver`.

Pre-built binaries for Linux / macOS / Windows (amd64, arm64) are also published to GitHub Releases.

## Usage

```
bump-semver get <INPUT...>
bump-semver <major|minor|patch|pre> <INPUT...> [--write]
bump-semver compare <eq|lt|le|gt|ge|...> <INPUT> <INPUT>
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit -m MSG <PATH..|--staged> | --amend [-m MSG]
bump-semver --version [--json]
bump-semver --help | --help-full
```

`<INPUT>` is either a **FILE path**, a **raw VER string**, **`-` (read VER from stdin, single line)**, **`vcs:REV[:FILE]` / `vcs:<func>(...)`** (read from the VCS, see [vcs: input](#vcs-input)), or **`cmd:<shell-command>`** (read from a shell command, see [cmd: input](#cmd-input)). Multiple inputs of mixed kinds may be given.

Help comes in three tiers (v0.13.0+):

- `bump-semver --help` / `-h`: short overview (actions + main navigation) fitting in one screen
- `bump-semver --help-full`: complete reference (supported file formats / full Examples / exit codes / etc.)
- `bump-semver <action> --help`: action-specific reference. `bump-semver patch --help` shows the bump help (shared by major/minor/patch), `bump-semver pre --help` documents the three pre modes, `bump-semver compare --help` lists the full 20-operator grid including precision suffixes.

### Actions

| Action | Effect |
|---|---|
| `major` | Bump major (`X.0.0`); pre-release / build metadata dropped by default |
| `minor` | Bump minor (`x.Y.0`); same drop default |
| `patch` | Bump patch (`x.y.Z`); same drop default |
| `pre`   | Pre-release counter advance / overwrite / remove (three modes, see below) |
| `get`   | Print the current version (also doubles as a consistency check) |

### compare subcommand

```
bump-semver compare <OP> <INPUT> <INPUT>
```

`<OP>` is one of `eq` / `lt` / `le` / `gt` / `ge`. Comparison follows SemVer 2.0.0 ordering (build metadata excluded from ordering, prefix / sep differences normalised).

| OP | True when |
|---|---|
| `eq` | first == second |
| `lt` | first <  second |
| `le` | first <= second |
| `gt` | first >  second |
| `ge` | first >= second |

Exit codes: `0` = true / `1` = false / `2` = error (`test` / `dpkg --compare-versions` convention).

Each OP may carry a `-major` / `-minor` / `-patch` suffix that truncates the comparison ([DR-0017](./docs/decisions/DR-0017-compare-precision-suffix.md)). 5 bases × 4 precisions = 20 operators.

```bash
bump-semver compare eq Cargo.toml v1.2.3 && echo same
bump-semver compare lt 1.2.3-rc.1 1.2.3                       # exit 0 (rc < release)
bump-semver compare eq-major 1.2.3 1.9.7                      # exit 0 (same major)
bump-semver compare eq-patch 1.2.3 1.2.3-rc.1                 # exit 0 (pre-release ignored)
bump-semver compare lt-minor Cargo.toml vcs:origin/main       # only minor-or-below bumps?
```

### vcs subcommand

```
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit -m MSG PATH..
bump-semver vcs commit -m MSG --staged
bump-semver vcs commit --amend [-m MSG]
```

A small family of git/jj-agnostic helpers ([DR-0020](./docs/decisions/DR-0020-vcs-subcommands.md)). PR-1 shipped `vcs get` (read-only); PR-2 adds `vcs is` (predicate); PR-3 adds `vcs diff` (patch printer); PR-3.1 extends `vcs diff` with `-s/--name-status` (M/A/D summary) and `-q/--quiet` (exit-code reflects diff presence, mirroring `git diff --quiet`); PR-4 adds `vcs commit` (path-required commit with safety defaults). Further verbs (`vcs push`, `vcs tag`) will follow as the design rolls out. The motivation is the recurring `Taskfile / justfile` pain of branching on git vs jj — `bump-semver` already abstracts version reads via `vcs:`, so the `vcs` verb is the natural place for these helpers.

**`vcs get <key>`** — emit a value on stdout:

| Key | Output |
|---|---|
| `root` | Absolute path to the repository root |
| `backend` | `git` or `jj` (jj wins on a colocated repo) |
| `current-branch` | The unambiguous current branch (git) / bookmark (jj). Detached HEAD or multiple bookmarks at the same head → exit 4 |

**`vcs is <pred>`** — exit code is the answer (0=true, 1=false, silent on stderr):

| Predicate | Meaning |
|---|---|
| `clean` | Worktree has no uncommitted changes. **git**: `git diff --quiet` AND `git diff --cached --quiet` (untracked files ignored). **jj**: the working-copy change `@` is empty (template `empty`). jj snapshots on read, so newly-created files DO render dirty — this asymmetry vs git is intentional |
| `dirty` | `!clean` |
| `git` / `jj` | The detected (or `--vcs`-forced) backend matches |

**`vcs diff REV [PATH..]`** — print the patch between `REV` and the working copy on stdout. Backend-uniform: git runs `git diff REV [-- PATH..]`, jj runs `jj diff --from REV --to @ [-- PATH..]`. Both forms compare REV against the worktree, including uncommitted changes.

`-s` / `--name-status` switches the output to one `<CODE>\t<path>` line per changed file (M/A/D — modify / add / delete). git native; jj's `--summary` output is normalized to tab-separated form so the result is uniform across backends.

`-q` / `--quiet` on `vcs diff` overloads the global "suppress stdout" meaning to also mirror `git diff --quiet`'s `--exit-code` semantic: **exit 0 = no diff, exit 1 = diff present**. Stdout is empty; stderr is preserved unless `-qq` is used. With `-s -q`, `-q` wins (stdout empty, exit reflects presence). This is the predicate form for scripting "has anything changed since REV?" — particularly useful for `check-version-bumped`-style gates. Other vcs verbs (`get`/`is`) keep the pure stdout-suppression meaning; the overload is justified by the diff verb being the only one whose "is there anything?" question is well-posed.

Path filter rule (**declarative convergence**): nonexistent `PATH` arguments are silently ignored. When every supplied `PATH` is filtered out the command exits `0` with empty stdout — it does **NOT** widen back to "diff everything". A path present in `REV` but deleted in the worktree is not shown when named explicitly (the full diff with no `PATH` still shows the deletion). Under `-q`, all-filtered yields exit 0 (= "no diff to report").

Exit codes (also see below): `0` success / predicate true (incl. `vcs diff -q` with no diff); `1` predicate false (`vcs is`, and `vcs diff -q` when diff is present); `2` usage error; `3` VCS subprocess error (incl. "not a repo", unresolvable REV); `4` ambiguous answer (`5` is reserved for `vcs push` non-ff in a future PR).

```bash
bump-semver vcs get backend                  # git
bump-semver vcs get root                     # /path/to/repo
bump-semver vcs get current-branch           # main
ROOT=$(bump-semver vcs get root) || exit

bump-semver vcs is clean && bump-semver patch VERSION --write
if bump-semver vcs is git; then ... fi
bump-semver vcs is dirty || echo "nothing to commit"

bump-semver vcs diff HEAD~1                   # full diff since previous commit
bump-semver vcs diff main@origin VERSION      # what changed in VERSION vs remote main
bump-semver vcs diff HEAD~1 src lib           # subtree-scoped diff
bump-semver vcs diff -s HEAD~1                # M/A/D file list (git --name-status format)
bump-semver vcs diff -q HEAD~1 -- VERSION && echo "VERSION unchanged"
                                              # exit 0 ⇔ no diff in VERSION
```

**`vcs commit`** — three commit modes with opinionated safety defaults.

| Mode | Behaviour |
|---|---|
| `-m MSG PATH..` | Stage + commit each existing path's working-tree content. Nonexistent paths silently dropped (declarative convergence). All-nonexistent / no real change → exit 0 with no commit (idempotent) |
| `-m MSG --staged` | Commit every staged/dirty change in one shot. **git**: commits the index. **jj**: commits the whole `@` snapshot (jj auto-stages). No content → exit 0, idempotent |
| `--amend [-m MSG]` | Fold the current change into the previous commit. With `-m`: rewrite the message; without: preserve it (no-edit). Message-only amend with no current change is a legal explicit rewrite |

**`-a` / `--all` is intentionally not provided** (DR-0020 safety). jj's auto-staged worldview makes `-a`'s unstaged-grab semantic too easy to trip on; use `--staged` (commit all current changes) or pass `PATH..` explicitly. Calling `-a` exits 2 with a hint pointing at `--staged` / `PATH..`.

The empty-no-op rule for path / `--staged` modes makes this snippet portable across languages:

```bash
# Commit whatever version-bearing files exist & changed; safe if some don't apply
bump-semver vcs commit -m "bump version" VERSION Cargo.toml package.json pyproject.toml
```

Exit codes for `vcs commit`: `0` success or idempotent no-op; `2` usage error (missing `-m`, `-a` rejected, `--staged + PATH`, no-mode); `3` VCS subprocess error (not a repo, commit failed).

```bash
bump-semver vcs commit -m "bump 1.2.3" VERSION         # commit just VERSION
bump-semver vcs commit --staged -m "release: 1.2.3"     # commit everything staged
bump-semver vcs commit --amend                          # absorb into previous, keep msg
bump-semver vcs commit --amend -m "release: 1.2.3 (final)"  # rewrite previous msg
```

`--vcs jj|git|auto` still applies, so `bump-semver vcs get backend --vcs git` (or `vcs is git --vcs git`) forces the git branch on a colocated repo.

### Flags

| Flag | Description |
|---|---|
| `--pre PRE`            | Set pre-release identifiers (e.g. `--pre rc.0`) |
| `--no-pre`             | Remove pre-release identifiers |
| `--build-metadata META`| Set build metadata identifiers (e.g. `--build-metadata sha.abc`) |
| `--no-build-metadata`  | Remove build metadata identifiers |
| `--write`              | Write the bumped version back to each FILE input (`major` / `minor` / `patch` / `pre` only) |
| `--vcs jj\|git\|auto`    | Force VCS detection for `vcs:` inputs (default: `auto`) |
| `--no-hint`            | Suppress all `hint:` lines (fallback match / unsupported file / "files not modified") |
| `-q`, `--quiet`        | Suppress stdout (and all `hint:` lines) |
| `-qq`, `--quiet-all`   | Suppress stdout, hints, and error output (use with caution when debugging) |
| `--json`               | Output structured JSON for `get` / `major` / `minor` / `patch` / `pre` (rejected with `compare`) |
| `--version`, `-V`      | Print the binary version |
| `--help`, `-h`         | Short help (one screen) |
| `--help-full`          | Full reference (Supported file formats / all Examples / Exit codes / etc.) |
| `<action> --help`      | Action-specific reference (`bump-semver patch --help` / `compare --help` / etc.) |

Mutual exclusivity: `--pre` and `--no-pre` cannot both be given; same for the build-metadata pair; `--write` cannot be combined with `get` or `compare`.

`-q` / `-qq` / `--no-hint` are not mutually exclusive: `-qq` is a strict superset of `-q`, which is a strict superset of `--no-hint`, so combinations are silently absorbed. `-q` is a no-op for `compare` (it has no stdout to suppress).

`bump-semver` may emit one or more `hint:` lines on stderr alongside the normal stdout output. All hints share the `hint:` prefix and are suppressed together by `--no-hint` / `-q` / `-qq`. The hints currently in use:

| Hint | Trigger | Action / `-q` |
|---|---|---|
| `hint: <path> matched as *.<ext> fallback. Open issue if explicit support is needed.` | A FILE input matched a confidence-1 fallback rule (`*.json` from DR-0010, `*.yaml` / `*.yml` / `*.toml` from DR-0011). One line per such input. | Anywhere a FILE is resolved (`get` / bump / `compare`). |
| `hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.` | A FILE path doesn't match any rule, so `unsupported file:` is reported. The hint follows the error line. | Same as above. |
| `hint: <N> file(s) not modified; use --write to update or --no-hint to suppress` | A bump action (`major` / `minor` / `patch` / `pre`) had at least one FILE input but no `--write`. | Bump actions only. VER-only bumps and `get` / `compare` never emit it. |

### Input (INPUT)

| Form | Meaning |
|---|---|
| FILE | Path to a supported file (auto-detected by basename) |
| VER  | A raw semver string (e.g. `1.2.3`, `v1.2.3`, `1.2.3-rc.1+build.42`) |
| `-`  | Read VER from stdin, one line (used at most once) |
| `vcs:REV[:FILE]` | Read FILE at `<REV>` from jj or git (auto-detected, see [vcs: input](#vcs-input)) |
| `vcs:latest-tag([REPO])` | Read the largest semver-compatible tag. `REPO` = `owner/repo` short form or full URL queries a remote (`git ls-remote --tags`); omit for cwd VCS |
| `cmd:<shell-command>` | Run `<shell-command>` via `bash -c`, take the first non-empty stdout line as VER (read-only, see [cmd: input](#cmd-input)) |

If a local file is literally named `1.2.3` and you mean the file, write `./1.2.3` (Unix convention).

### Supported version syntax

```
body:  (v|ver|version)?[._]?\d+[._]\d+[._]\d+      (sep1 == sep2 enforced)
pre:   -<id>(.<id>)*                                (SemVer 2.0.0 compliant)
meta:  +<id>(.<id>)*                                (SemVer 2.0.0 compliant)
```

- The `v` / `ver` / `version` prefix is optional (e.g. `v1.2.3`, `version_1_2_3`)
- Body separator is `.` or `_`, and must match on both sides (DR-0003 + DR-0006)
- Body separator `-` is **not allowed** (would collide with the pre-release `-`)
- Pre-release: `rc.0`, `alpha`, `beta.1`, etc. Numeric-only identifiers must not have leading zeros
- Build metadata: `build.42`, `sha.5114f85`, etc. Leading zeros are allowed (per SemVer)

The chosen prefix and separator are **preserved** on output.

### Bump behavior (drop default)

On bump, unless `--pre` / `--build-metadata` is explicitly given, any existing pre-release / build metadata is **dropped** (DR-0006).

| Input | `patch` | `pre` | `pre --pre alpha` | `pre --no-pre` |
|---|---|---|---|---|
| `1.2.3`            | `1.2.4` | error: no pre-release | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0`       | `1.2.4` (drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |
| `1.2.3-rc1`        | `1.2.4` | error: not incremental | `1.2.3-alpha` | `1.2.3` |
| `1.2.3+build`      | `1.2.4` (drop) | error: no pre-release | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0+build` | `1.2.4` (both dropped) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |

This **differs from the npm-style strip-don't-bump (`patch 1.2.3-rc.0 → 1.2.3`)**. We use a single rule — `patch` always advances the patch number; pre-release / build metadata are dropped unless explicitly carried over with `--pre` / `--build-metadata` — for internal consistency.

### `pre` action: three modes

- **No flag (`pre INPUT`)**: counter-advance only when the existing pre-release's last identifier is purely numeric (e.g. `rc.0` → `rc.1`). Otherwise (e.g. `rc1`, `alpha`) error
- **`--pre PRE`**: overwrite the pre-release with PRE entirely (regardless of prior pre, including going backwards)
- **`--no-pre`**: remove pre-release (no-op if there was none)

### Supported file formats

Detection is **path-aware and confidence-ranked** (DR-0005). For each input FILE, rules are tried in confidence order; if a high-confidence rule's path-pattern matches but extraction fails (e.g. a `marketplace.json` without `.metadata.version`), the next rule is tried. The lowest-confidence fallback covers any `*.json` with a top-level `.version`.

| Confidence | Pattern | Format | Version path(s) | Name path(s) |
|---|---|---|---|---|
| **3** (path-pinned) | `.claude-plugin/marketplace.json` | JSON | `$.metadata.version` | `$.name` |
| **3** | `.claude-plugin/plugin.json` | JSON | `$.version` | `$.name` |
| **3** | `package.json` | JSON | `$.version` | `$.name` |
| **3** | `package-lock.json` | JSON | `$.version`, `$.packages[""].version` | `$.name`, `$.packages[""].name` |
| **3** | `Cargo.toml` | TOML | `[package].version` (try) → `[workspace.package].version` | `[package].name` (try) → `[workspace.package].name` |
| **3** | `pyproject.toml` | TOML | `[project].version` (try) → `[tool.poetry].version` | `[project].name` (try) → `[tool.poetry].name` |
| **3** | `mojoproject.toml` | TOML | `[workspace].version` | `[workspace].name` |
| **3** | `project.pbxproj` (Xcode) | pbxproj | every `MARKETING_VERSION = ...;` (synced) | — |
| **3** | `Info.plist` (Apple plist) | xml | `<key>CFBundleShortVersionString</key>` | — |
| **3** | `pom.xml` (Maven) [DR-0018] | xml-element | `/project/version` | `/project/artifactId` |
| **3** | `VERSION` | plain text | (file content) | — |
| **2** (basename) | any `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | any `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | regex | `version := "..."` | — |
| **2** | `build.gradle` (Gradle Groovy) [DR-0018] | regex | `version = '...'` / `version "..."` | — |
| **2** | `build.gradle.kts` (Gradle Kotlin DSL) [DR-0018] | regex | `version = "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.cabal` (Haskell) [DR-0018] | regex | `version: ...` (line-anchored) | `name: ...` |
| **1** (fallback) | `*.spec` (RPM) [DR-0018] | regex | `Version: ...` (capital V) | `Name: ...` |
| **1** (fallback) | `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) [DR-0018] | xml-element | `/Project/PropertyGroup/Version` | — |

Unsupported files (e.g. `README.md`, `Cargo.lock`) error out explicitly with `unsupported file: <path>`. Adding a new format = adding one row to the rule table plus, if needed, one new format-specific function (no `--pattern` regex flag, by design).

YAML / TOML fallbacks (DR-0011) only look at top-level keys: a `version` nested inside a section / mapping is intentionally not picked up. For `Cargo.toml` / `pyproject.toml` / `mojoproject.toml` the explicit confidence-3 rules still win (so their existing section-scoped behaviour is unchanged). Multi-document YAML (`---` separators) reads only the first document. The same DR-0010 fallback hint fires for these new rules — with `--no-hint` to suppress.

The `pyproject.toml` rule (DR-0014) tries PEP 621's `[project].version` first and falls back to Poetry-legacy `[tool.poetry].version` so a single rule covers both ecosystems mid-migration. When a file carries both sections (theoretical mid-migration state), only the first match (PEP 621) is rewritten. The `mojoproject.toml` rule (DR-0014) reads / writes `[workspace].version` directly. Both rules use the same TOML section-scoped rewriter, so quote style and surrounding sections / comments stay intact.

The `Cargo.toml` rule (DR-0021) uses the same try-fallback shape: a single-crate manifest's `[package].version` is tried first, and a workspace-root manifest (no `[package]`) falls back to `[workspace.package].version` — the value member crates inherit via `version.workspace = true`. When a member crate declares both, its own `[package].version` wins. The matched path (`[package].version` or `[workspace.package].version`) is shown in `get` / `--json` output so you always see which version you are bumping.

The DR-0012 `regex` format covers eight language manifests whose version is a single line of source code (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt). Only the **first match** is read or rewritten; quote style and trailing comments on the version line are preserved verbatim.

The DR-0015 rules add the two Xcode-specific files where multiple version strings need synchronised updates: `project.pbxproj` (Xcode iOS / macOS project bundle, OpenStep plist) reads / rewrites **every** `MARKETING_VERSION = ...;` line at once and verifies they agree (a mismatched file surfaces a column-aligned `version mismatch:` block with `<file>:line:N` labels), and `Info.plist` (XML plist) reads / rewrites the `<key>CFBundleShortVersionString</key><string>...</string>` pair while preserving DOCTYPE, indentation, attribute order, and sibling keys byte-for-byte. Files using the Xcode 11+ `<string>$(MARKETING_VERSION)</string>` placeholder produce an `unsupported file:` outcome — the placeholder isn't a parseable version, which is the cue to add `project.pbxproj` to the invocation where the real value lives. `CFBundleVersion` (build number) is intentionally out of scope (build numbers aren't SemVer; CI typically writes them).

#### Suffix-stripped fallback (DR-0013)

When a path doesn't match any rule directly, `bump-semver` strips one trailing **backup-style suffix** from the basename and retries the rule table. The chosen rule's reported confidence is downgraded one band (3→2, 2→1, 1→1) and a `hint:` line is emitted to stderr so the resolution stays transparent. Multi-stage suffixes (`Cargo.toml.bak.20260510`) strip **only the trailing segment**; recursion is intentionally not applied.

| Suffix | Example | Resolved as |
|---|---|---|
| `.bak` / `.backup` / `.orig` / `.tmp` / `.old` | `Cargo.toml.bak` | `Cargo.toml` rule (confidence 2) |
| `.YYYYMMDD` (8 digits) | `package.json.20260510` | `package.json` rule (confidence 2) |
| `.YYYYMMDD_HHMMSS` (8+`_`+6 digits) | `Chart.yaml.20260510_120000` | `*.yaml` fallback (confidence 1) |
| trailing `~` (Emacs / vi) | `Cargo.toml~` | `Cargo.toml` rule (confidence 2) |

```bash
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

Template-style suffixes (`.template` / `.example` / `.sample` / `.dist`) are **not** stripped — their content is usually a placeholder, so silently treating them as real manifests would be more dangerous than the existing `unsupported file:` error. If you need to read a template, copy it under a backup-style name (`cp Cargo.toml.template Cargo.toml.tmp`).

The suffix hint shares the existing `hint:` prefix and is suppressed by `--no-hint` / `-q` / `-qq` exactly like the DR-0010 fallback hint. When both fire (e.g. `unknown.json.bak` → strip `.bak` → `*.json` glob), the suffix hint is emitted first.

For npm `package-lock.json` specifically, lockfile v1 (npm 5/6) is rejected with `unsupported lockfileVersion: 1, please regenerate with npm 7+`. Dependency entries (`$.packages["node_modules/..."]`) are never rewritten even if their version happens to equal the project's own.

### Multiple INPUTs: cross-input consistency

Pass multiple INPUTs to operate on them as a single unit. Versions across all INPUTs must already agree (otherwise: a `version mismatch:` error lists each origin and value, column-aligned). Detected package names are also cross-checked when available, to guard against accidentally bumping files from a different project together; names are never written back.

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
bump-semver patch 1.2.3 a.json b.json --write   # use VER as the "expected current value" for consistency, write bumped result to a/b
```

`get` with multiple INPUTs works as a CI-friendly consistency check (no `--write` needed, just verifies that all detected version fields agree).

With `--write`, only **FILE-origin inputs** are written back; VER and stdin inputs are reference values used for consistency checking only. Specifying `--write` without any FILE input is an error (`--write requires at least one FILE`).

### stdin pipe

When stdin is a pipe **and exactly one FILE INPUT is given**, that FILE is treated as a name hint and content is read from stdin (legacy shortcut, kept for backward compatibility). With multiple INPUTs the stdin pipe is ignored. Useful for comparing across revisions without checking out the file:

```bash
jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml
```

To explicitly read VER from stdin (the new unified form), pass `-` as an INPUT — it can be mixed with FILE INPUTs:

```bash
echo 1.2.3 | bump-semver compare eq Cargo.toml -
```

### Examples

```bash
bump-semver patch Cargo.toml --write              # bump + write back, prints new version
bump-semver minor package.json                    # bump in memory, prints new version (file untouched)
bump-semver get .claude-plugin/plugin.json        # current version
bump-semver patch 1.2.3                           # 1.2.4 (raw VER)
bump-semver patch v1.2.3                          # v1.2.4 (prefix preserved)
bump-semver minor version_1_2_3                   # version_1_3_0 (prefix + separator preserved)
bump-semver pre 1.2.3-rc.0                        # 1.2.3-rc.1 (counter advance)
bump-semver pre 1.2.3 --pre rc.0                  # 1.2.3-rc.0 (overwrite)
bump-semver patch 1.2.3-rc.0 --pre rc.0           # 1.2.4-rc.0 (bump + re-attach pre)
bump-semver patch 1.2.3-rc.0 --no-pre             # 1.2.4 (drop and bump, release-promotion equivalent)
bump-semver compare lt 1.2.3-rc.1 1.2.3           # exit 0 (rc < release)
bump-semver compare eq .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json   # 3-file consistency check
bump-semver get   Cargo.toml --json               # structured output for jq
bump-semver patch Cargo.toml --json               # bumped version, fully decomposed
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'   # ready to release? (CI: bumped past last tag)
bump-semver compare lt Cargo.toml vcs:origin/main      # stale vs remote main? (pull needed)
```

### JSON output (`--json`)

`get` and the bump actions (`major` / `minor` / `patch` / `pre`) accept `--json`. The result is a single line of JSON terminated by a newline (DR-0007), suitable for piping into `jq`. `compare` does not accept `--json` — its answer is the exit code, by design.

```bash
bump-semver get Cargo.toml --json
# {"name":"my-pkg","version":"1.2.3","semver":"1.2.3","major":1,"minor":2,"patch":3,"pre":null,"pre_id":null,"pre_rest":null,"build_metadata":null,"build_id":null,"build_rest":null}

bump-semver patch v_1.2.3-rc.1+build.42 --json
# {"name":null,"version":"v_1.2.4","semver":"1.2.4","major":1,"minor":2,"patch":4,"pre":null,...}
```

| Field | Type | Notes |
|---|---|---|
| `name` | string \| null | From the FILE-origin name field (e.g. `package.json $.name`); null for VER / stdin origin |
| `version` | string | Input format preserved (prefix + body separator kept) |
| `semver` | string | Strict SemVer 2.0.0 form (prefix removed, body sep normalised to `.`) |
| `major` / `minor` / `patch` | int | Numeric components |
| `pre` | string \| null | Joined pre-release identifiers (e.g. `"rc.1"`); null when absent |
| `pre_id` / `pre_rest` | string \| null | `pre` split at the first `.` (`pre_rest` is null when there's no `.`) |
| `build_metadata` | string \| null | Joined build metadata (e.g. `"build.42"`); null when absent |
| `build_id` / `build_rest` | string \| null | Same first-`.` split rule as pre |

The CLI provides **structural decomposition only**. It does not encode semantic judgements such as "is this counter advanceable" — for that kind of check, run `bump-semver pre VER` and look at the exit code.

### vcs: input

Any positional INPUT that starts with `vcs:` is resolved through the version-control system (jj or git). This lets you compare against another revision, the remote main branch, or the latest release tag without an extra `jj file show | bump-semver compare lt - ...` shell pipeline (DR-0008).

```bash
# Has the working-tree version been bumped past the last release tag?
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'

# Are we stale vs remote main? (pull needed before push)
bump-semver compare lt Cargo.toml vcs:origin/main

# Did Cargo.toml's version change since the previous commit?
bump-semver compare eq Cargo.toml vcs:HEAD~1            # FILE borrowed from the sibling
bump-semver compare eq Cargo.toml vcs:HEAD~1:Cargo.toml # explicit form

# v0.15.0+ — Query a different repo's latest release tag
bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'        # owner/repo short form
bump-semver get 'vcs:latest-tag(https://github.com/x/y)' # full URL
bump-semver compare ge 0.0.13 'vcs:latest-tag(kawaz/pkf-tasks)'  # current pin up-to-date?
```

| Form | Meaning |
|---|---|
| `vcs:REV[:FILE]` | Read FILE at `<REV>` from the VCS. The first `:` is consumed by the `vcs:` prefix; the second `:` (if present) splits REV from FILE. Omitted FILE is borrowed from the first sibling input (FILE-origin or another `vcs:REV:FILE`) in argv order |
| `vcs:latest-tag()` | All tags from the cwd VCS; semver-incompatible tags are silently dropped; the largest by SemVer 2.0.0 ordering wins. Errors with `no semver-compatible tags found` if the candidate set is empty |
| `vcs:latest-tag(<arg>)` | v0.15.0+. `<arg>` = `owner/repo` (GitHub short, expanded to `https://github.com/...`) or full HTTPS/SSH URL. Queries the remote via `git ls-remote --tags`; jj/git auto-detection is irrelevant for remote queries. Monorepo-style tags like `pkf-tasks@0.0.13` are recognised (`@` peel fallback) so the same call works against multi-package repos. The argument is a **raw string** — no inner quotes needed (think markdown link `[]()`). **Trust boundary**: the validity of the URL is the caller's responsibility; pointing at an untrusted repo lets attackers publish `malicious@99.99.99` and have it returned as the largest tag (DR-0019) |

**VCS detection** (in priority order):

1. `--vcs jj|git` flag (`auto` and the unset case fall through)
2. `.jj` directory exists in the working dir or any ancestor → jj
3. `.git` directory exists → git
4. Otherwise → error (`not a git or jj repository`)

When both `.jj` and `.git` exist (jj's colocate mode, or kawaz's git-bare + jj-workspace layout), **jj wins** — its revset language is a superset of git's.

> Earlier versions (≤ v0.12) inserted a `BUMP_SEMVER_VCS=jj|git` environment variable between the flag and the probes; that knob was removed in v0.13 ([DR-0016](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md)). If a CI / dev environment previously relied on the env var, replace it with the `--vcs jj|git` flag.

**`--write` is incompatible with `vcs:` inputs.** vcs: is read-only by design (writing back into the VCS would require commit/amend semantics, which is out of scope). Combining the two errors out with `--write cannot be used with vcs: inputs (vcs: is read-only)`.

**`bump-semver` does not run `git fetch` / `jj git fetch` automatically.** If `vcs:origin/main` is stale, the underlying VCS error is surfaced verbatim. CI users should fetch explicitly before invoking `bump-semver`.

For CI scripts that need to be VCS-agnostic, prefer revisions that work in both flavours: `origin/main` (auto-translated to `main@origin` for jj), commit hashes, and `latest-tag()`.

### cmd: input

`cmd:<shell-command>` runs `<shell-command>` via `bash -c` and takes the first non-empty stdout line as VER (read-only, v0.16.0+). A leading `v` is stripped, and the value is parsed as SemVer 2.0.0.

```bash
# Does the built binary's --version match the VERSION file?
bump-semver compare eq VERSION 'cmd:./bin/mytool --version'

# Read a version that lives outside vcs:latest-tag's reach
bump-semver get 'cmd:brew info --json mytool | jq -r .[0].installed[0].version'

# Compare against another bump-semver invocation
bump-semver compare gt 'cmd:bump-semver get Cargo.toml' 'vcs:latest-tag()'
```

| Form | Interpretation |
|---|---|
| `cmd:<shell-command>` | Executes `<shell-command>` via `bash -c`. The first non-empty stdout line is taken as VER (leading `v` stripped, parsed as SemVer). Non-zero exit, empty stdout, or parse failure surface as errors (child stderr is included). **Read-only**: rejected by `--write` (same as `vcs:`) |

**`--write` and `cmd:` are mutually exclusive** (same as `vcs:`). There is no notion of writing back to a command's output.

**Trust boundary**: an arbitrary shell command is executed. Callers in CI / automation must assemble the command string safely — never `concat` untrusted input (env vars, argv) into a `cmd:` value.

The primary driver is kawaz/pkf-tasks v3.0's `semver/versions.pkl` (release-time gate), where version files and the built binary's `--version` output need to be cross-checked through a single `bump-semver get` invocation.

### Error message format

Errors print `bump-semver: <reason>` to stderr on a single line. The format depends on the input origin (VER vs FILE), so callers can grep for known substrings.

**VER origin** (positional argument or stdin-supplied raw semver):

```
bump-semver: rc1 is not incremental, use --pre PRE
bump-semver: 1.2.3 does not have a pre-release, use --pre PRE
```

**FILE origin** (version read from a file): wrapped with the file path and in-file version field path.

```
bump-semver: Cargo.toml:[package].version=1.2.3-rc1: rc1 is not incremental, use --pre PRE
bump-semver: package.json:$.version=1.2.3: 1.2.3 does not have a pre-release, use --pre PRE
```

**Mismatch errors** (multiple INPUTs disagree): printed column-aligned, vertically.

```
bump-semver: version mismatch:
  Cargo.toml:[package].version = 1.2.3
  package.json:$.version       = 1.2.4
  <argv>                       = 1.2.3-rc.1
```

Origin labels: `<file>:<path>` (FILE origin) / `<argv>` or `<argv:N>` (positional VER) / `<stdin>` (`-`).

### Exit codes

- `0` — success / predicate true (`compare`, `vcs is`)
- `1` — predicate false (`compare`, `vcs is` — silent on stderr)
- `2` — error (parse failure, mismatch, unsupported file, exclusivity violation, IO error, unknown verb/key for `vcs`, etc.)
- `3` — VCS subprocess error (`vcs` subcommands only: not in a repo, git/jj invocation failed)
- `4` — ambiguous answer (`vcs` subcommands only: detached HEAD, multiple bookmarks at the same head)
- `5` — non-fast-forward push (reserved for `vcs push` in a future PR)

## Migrating from v0.4.x

v0.5.0 ships three breaking changes. See [UPGRADING.md](./UPGRADING.md) for full details and rewrite examples:

1. **`--value` flag removed** → pass the VER directly as a positional argument (`bump-semver patch 1.2.3`)
2. **Body separator `-` removed** → use `.` or `_` instead (`1-2-3` is no longer accepted)
3. **Bump-path error exit code 1 → 2** (unified with the compare convention)

## Status

v0.16.1 hardens the `cmd:` input mode — `--write` + `cmd:` is now rejected by the implementation (the README already documented this, but v0.16.0 only enforced the rule for `vcs:` and silently let `cmd:` slip through), plus a 30-second hard timeout on the child process and 64 KiB / 4 KiB output caps on stdout / stderr to defend against runaway commands. Whitespace-only commands (`cmd:   `) are now rejected by the same non-empty check as `cmd:`. v0.16.0 adds the `cmd:<shell-command>` input mode — a read-only input that runs the command via `bash -c`, takes its first non-empty stdout line, strips a leading `v`, and parses the rest as SemVer. The primary use case is gating releases on agreement between version files and the built binary's `--version` output (e.g. `compare eq VERSION 'cmd:./bin/mytool --version'`). It also underpins kawaz/pkf-tasks v3.0's `semver/versions.pkl`. v0.14.0 adds JVM / .NET / Maven / Haskell / RPM support and a new `xml-element` format (DR-0018) — `pom.xml`, `*.csproj` / `*.fsproj` / `*.vbproj`, `build.gradle` / `build.gradle.kts`, `*.cabal`, `*.spec` all become recognised. `pom.xml` uses slash-rooted XML path lookup (`/project/version`) that correctly skips `<parent>/<version>`. v0.13.0 brings three changes: the help system is restructured into three tiers (`--help` short / `--help-full` complete reference / `bump-semver <action> --help` action-specific), the `BUMP_SEMVER_VCS` env var is removed in favour of `--vcs jj|git|auto` (DR-0016, BREAKING — see UPGRADING.md), and `compare` gains 15 precision-suffix operators (`eq-major` / `lt-minor` / `eq-patch` etc., DR-0017) for a 5×4 = 20 total. v0.12.0 added two Xcode-specific path-pinned rules — `project.pbxproj` (multi-match `MARKETING_VERSION` synced across build configurations) and `Info.plist` (XML plist with byte-range value rewriting) — together with `pbxproj` and `xml` formats (DR-0015). v0.11.0 generalised the TOML rewriter into a reusable section-scoped helper and added `pyproject.toml` (PEP 621 with Poetry-legacy fallback) and `mojoproject.toml` (DR-0014). v0.10.0 added the suffix-stripped fallback for backup-style filenames (DR-0013). v0.9.0 introduced the `regex` format with eight new file types (`*.xcconfig`, `*.podspec`, `*.nimble`, `v.mod`, `build.zig.zon`, `*.gemspec`, `mix.exs`, `build.sbt`) (DR-0012). v0.8.0 added `*.yaml` / `*.yml` / `*.toml` confidence-1 fallback (DR-0011). v0.7.0 added the `vcs:` input mode — `vcs:REV[:FILE]` and `vcs:latest-tag()` resolve through jj or git automatically (DR-0008). Earlier: v0.6.0 added `--json` output (DR-0007); v0.5.0 introduced pre-release / build metadata support, the `compare` subcommand, the `pre` action, and the unified FILE/VER positional input (DR-0006). Future formats are added one handler at a time (DR-0001). For design rationale see [docs/decisions/](./docs/decisions/); for upcoming items see [docs/ROADMAP.md](./docs/ROADMAP.md).

## License

[MIT](LICENSE)
