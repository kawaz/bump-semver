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
bump-semver <ACTION> <INPUT...> [flags]
bump-semver compare <OP> <INPUT> <INPUT>
bump-semver --version [--json]
bump-semver --help
```

`<INPUT>` is either a **FILE path**, a **raw VER string**, **`-` (read VER from stdin, single line)**, or **`vcs:REV[:FILE]` / `vcs:<func>(...)`** (read from the VCS, see [vcs: input](#vcs-input)). Multiple inputs of mixed kinds may be given.

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

```bash
bump-semver compare eq Cargo.toml v1.2.3 && echo same
bump-semver compare lt 1.2.3-rc.1 1.2.3                       # exit 0 (rc < release)
bump-semver compare lt Cargo.toml < <(jj file show -r main@origin Cargo.toml)
                                                              # CI check: drifted from main?
```

### Flags

| Flag | Description |
|---|---|
| `--pre PRE`            | Set pre-release identifiers (e.g. `--pre rc.0`) |
| `--no-pre`             | Remove pre-release identifiers |
| `--build-metadata META`| Set build metadata identifiers (e.g. `--build-metadata sha.abc`) |
| `--no-build-metadata`  | Remove build metadata identifiers |
| `--write`              | Write the bumped version back to each FILE input (`major` / `minor` / `patch` / `pre` only) |
| `--vcs jj\|git`         | Force VCS detection for `vcs:` inputs (overrides `BUMP_SEMVER_VCS` env) |
| `--no-hint`            | Suppress all `hint:` lines (fallback match / unsupported file / "files not modified") |
| `-q`, `--quiet`        | Suppress stdout (and all `hint:` lines) |
| `-qq`, `--quiet-all`   | Suppress stdout, hints, and error output (use with caution when debugging) |
| `--json`               | Output structured JSON for `get` / `major` / `minor` / `patch` / `pre` (rejected with `compare`) |
| `--version`, `-V`      | Print the binary version |
| `--help`, `-h`         | Show help |

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
| `vcs:latest-tag()` | Read the largest semver-compatible tag from jj or git |

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
| **3** | `Cargo.toml` | TOML | `[package].version` | `[package].name` |
| **3** | `VERSION` | plain text | (file content) | — |
| **2** (basename) | any `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | any `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | regex | `version := "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |

Unsupported files (e.g. `README.md`, `Cargo.lock`) error out explicitly with `unsupported file: <path>`. Adding a new format = adding one row to the rule table plus, if needed, one new format-specific function (no `--pattern` regex flag, by design).

YAML / TOML fallbacks (DR-0011) only look at top-level keys: a `version` nested inside a section / mapping is intentionally not picked up. For `Cargo.toml` the explicit confidence-3 rule still wins (so the existing `[package].version` behaviour is unchanged). Multi-document YAML (`---` separators) reads only the first document. The same DR-0010 fallback hint fires for these new rules — with `--no-hint` to suppress.

The DR-0012 `regex` format covers eight language manifests whose version is a single line of source code (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt). Only the **first match** is read or rewritten; quote style and trailing comments on the version line are preserved verbatim. Files with multiple version-like lines that need synchronised updates (Xcode `*.pbxproj`, `Info.plist`, etc.) are intentionally out of scope.

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
```

| Form | Meaning |
|---|---|
| `vcs:REV[:FILE]` | Read FILE at `<REV>` from the VCS. The first `:` is consumed by the `vcs:` prefix; the second `:` (if present) splits REV from FILE. Omitted FILE is borrowed from the first sibling input (FILE-origin or another `vcs:REV:FILE`) in argv order |
| `vcs:latest-tag()` | All tags are listed; tags that don't parse as semver are silently ignored; the largest by SemVer 2.0.0 ordering wins. Errors with `no semver-compatible tags found` if the candidate set is empty |

**VCS detection** (in priority order):

1. `--vcs jj|git` flag (highest priority)
2. `BUMP_SEMVER_VCS=jj|git` environment variable
3. `.jj` directory exists in the working dir or any ancestor → jj
4. `.git` directory exists → git
5. Otherwise → error (`not a git or jj repository`)

When both `.jj` and `.git` exist (jj's colocate mode, or kawaz's git-bare + jj-workspace layout), **jj wins** — its revset language is a superset of git's.

**`--write` is incompatible with `vcs:` inputs.** vcs: is read-only by design (writing back into the VCS would require commit/amend semantics, which is out of scope). Combining the two errors out with `--write cannot be used with vcs: inputs (vcs: is read-only)`.

**`bump-semver` does not run `git fetch` / `jj git fetch` automatically.** If `vcs:origin/main` is stale, the underlying VCS error is surfaced verbatim. CI users should fetch explicitly before invoking `bump-semver`.

For CI scripts that need to be VCS-agnostic, prefer revisions that work in both flavours: `origin/main` (auto-translated to `main@origin` for jj), commit hashes, and `latest-tag()`.

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

- `0` — success / compare predicate true
- `1` — compare predicate false
- `2` — error (parse failure, mismatch, unsupported file, exclusivity violation, IO error, etc.)

## Migrating from v0.4.x

v0.5.0 ships three breaking changes. See [UPGRADING.md](./UPGRADING.md) for full details and rewrite examples:

1. **`--value` flag removed** → pass the VER directly as a positional argument (`bump-semver patch 1.2.3`)
2. **Body separator `-` removed** → use `.` or `_` instead (`1-2-3` is no longer accepted)
3. **Bump-path error exit code 1 → 2** (unified with the compare convention)

## Status

v0.9.0 introduces the `regex` format (DR-0012) — a generic line-anchored rewriter that adds eight new file types in one shot (`*.xcconfig`, `*.podspec`, `*.nimble`, `v.mod`, `build.zig.zon`, `*.gemspec`, `mix.exs`, `build.sbt`). v0.8.0 added `*.yaml` / `*.yml` / `*.toml` confidence-1 fallback (DR-0011). v0.7.0 added the `vcs:` input mode (DR-0008) — `vcs:REV[:FILE]` and `vcs:latest-tag()` resolve through jj or git automatically. Earlier highlights: v0.6.0 added `--json` output (DR-0007), v0.5.0 introduced pre-release / build metadata support, the `compare` subcommand, the `pre` action, and the unified FILE/VER positional input (DR-0006). Future formats are added one handler at a time (DR-0001). For design rationale see [docs/decisions/](./docs/decisions/); for upcoming items see [docs/ROADMAP.md](./docs/ROADMAP.md).

## License

[MIT](LICENSE)
