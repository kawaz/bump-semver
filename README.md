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
bump-semver --version
bump-semver --help
```

`<INPUT>` is either a **FILE path**, a **raw VER string**, or **`-` (read VER from stdin, single line)**. Multiple inputs of mixed kinds may be given.

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
| `--no-hint`            | Suppress the "files not modified" hint (bump only) |
| `-q`, `--quiet`        | Suppress stdout (and the hint) |
| `-qq`, `--quiet-all`   | Suppress stdout, hint, and error output (use with caution when debugging) |
| `--version`, `-V`      | Print the binary version |
| `--help`, `-h`         | Show help |

Mutual exclusivity: `--pre` and `--no-pre` cannot both be given; same for the build-metadata pair; `--write` cannot be combined with `get` or `compare`.

`-q` / `-qq` / `--no-hint` are not mutually exclusive: `-qq` is a strict superset of `-q`, which is a strict superset of `--no-hint`, so combinations are silently absorbed. `-q` is a no-op for `compare` (it has no stdout to suppress); `--no-hint` is a no-op for `get` (it never emits a hint) — both are silently accepted for global-flag consistency.

For bump actions (`major` / `minor` / `patch` / `pre`) **with at least one FILE input but no `--write`**, a single line `hint: <N> file(s) not modified; use --write to update or --no-hint to suppress` is written to stderr. VER-only bumps and `get` / `compare` never emit the hint.

### Input (INPUT)

| Form | Meaning |
|---|---|
| FILE | Path to a supported file (auto-detected by basename) |
| VER  | A raw semver string (e.g. `1.2.3`, `v1.2.3`, `1.2.3-rc.1+build.42`) |
| `-`  | Read VER from stdin, one line (used at most once) |

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
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |

Unsupported files (e.g. `README.md`, `Cargo.lock`) error out explicitly with `unsupported file: <path>`. Adding a new format = adding one row to the rule table plus, if needed, one new format-specific function (no `--pattern` regex flag, by design).

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
bump-semver patch Cargo.toml --pre rc.0           # 1.2.4-rc.0 (bump + re-attach pre)
bump-semver patch Cargo.toml --no-pre             # 1.2.4 (release-promotion equivalent)
bump-semver compare lt 1.2.3-rc.1 1.2.3           # exit 0 (rc < release)
bump-semver compare eq Cargo.toml package.json    # cross-file equality
```

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

v0.5.0 introduces pre-release / build metadata support, the `compare` subcommand, the `pre` action, and the unified FILE/VER positional input (DR-0006). Future formats are added one handler at a time (DR-0001). For design rationale see [docs/decisions/](./docs/decisions/); for upcoming items see [docs/ROADMAP.md](./docs/ROADMAP.md).

## License

[MIT](LICENSE)
