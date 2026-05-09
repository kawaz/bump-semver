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
bump-semver <ACTION> <FILE | --value VER> [--write]
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
| `--value VER` | Use VER as input instead of reading from FILE (mutually exclusive with FILE) |
| `--write` | Write the new version back to FILE (only valid with `major` / `minor` / `patch`, mutually exclusive with `--value`) |

### Supported file formats

Auto-detected by basename:

| Pattern | Format |
|---|---|
| `Cargo.toml` | TOML, `[package].version` |
| `*.json` | JSON, `.version` (covers `package.json`, `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `moon.mod.json`) |
| `VERSION` | plain text |

Unsupported files cause an explicit error (no regex fallback by design).

### stdin pipe

When stdin is a pipe, FILE is treated as a name hint and the content is read from stdin. Useful for comparing across revisions without checking out the file:

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
```

### Exit codes

- `0` — success
- non-zero — error (unsupported file, exclusive option violation, parse failure, IO error, etc.)

## Status

This README documents the target API. Implementation is in progress; see [docs/decisions/](./docs/decisions/) for design rationale.

## License

[MIT](LICENSE)
