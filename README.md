# bump-semver

> English | [日本語](./README-ja.md)

A focused CLI for reading and bumping the semver string in version-tracking files. Detects the file format by basename (no `--pattern` regex flag), supports four flat actions (`major` / `minor` / `patch` / `get`), and writes the new version to stdout always so it composes well in shell pipelines.

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
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value VER
```

### Actions

| Action | Effect |
|---|---|
| `major` | Bump major (`X.0.0`) |
| `minor` | Bump minor (`x.Y.0`) |
| `patch` | Bump patch (`x.y.Z`) |
| `get`   | Print the current version |

### Options

| Option | Description |
|---|---|
| `--value VER` | Use VER as input instead of reading from FILE(s) (mutually exclusive with FILE) |
| `--write` | Write the new version back to each FILE (only valid with `major` / `minor` / `patch`, mutually exclusive with `--value`) |

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

### Multiple files: cross-file consistency

Pass multiple FILEs to bump them as a single unit. Versions across files must already agree (otherwise: `version mismatch:` with file:path = value lines). Detected package names are also cross-checked when available, to guard against accidentally bumping files from a different project together; names are never written back.

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
```

`get` with multiple FILEs works as a CI-friendly consistency check (no `--write` needed, just verifies that all detected version fields agree).

### stdin pipe

When stdin is a pipe **and exactly one FILE is given**, FILE is treated as a name hint and content is read from stdin. With multiple FILEs the stdin pipe is ignored (files override stdin, matching the cat/sed convention). Useful for comparing across revisions without checking out the file:

```bash
jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml
```

### Examples

```bash
bump-semver patch Cargo.toml --write          # bump + write back, prints new version
bump-semver minor package.json                # bump in memory, prints new version (file untouched)
bump-semver get .claude-plugin/plugin.json    # current version
bump-semver patch --value 1.2.3               # 1.2.4
bump-semver get --value 1.2.3                 # parse-validate (1.2.3) or error
bump-semver patch --value v1.2.3              # v1.2.4 (prefix preserved)
bump-semver minor --value version_1_2_3       # version_1_3_0 (prefix + separator preserved)
```

The version parser also accepts an optional `v` / `ver` / `version` prefix and `.` / `_` / `-` separators (e.g. `v1.2.3`, `ver-1-2-3`, `version_1_2_3`); the chosen prefix and separator are preserved on output. Pre-release / build metadata (`-alpha.1`, `+build.42`) is not supported.

### Exit codes

- `0` — success
- non-zero — error (unsupported file, exclusive option violation, parse failure, IO error, etc.)

## Status

v0.1.0 has been released — the MVP supports `Cargo.toml`, `*.json`, and `VERSION`. Further formats are added one handler at a time as concrete needs arise (see DR-0001). For design rationale see [docs/decisions/](./docs/decisions/); for upcoming items see [docs/ROADMAP.md](./docs/ROADMAP.md).

## License

[MIT](LICENSE)
