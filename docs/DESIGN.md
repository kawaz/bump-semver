# bump-semver Design Document

> English | [日本語](./DESIGN-ja.md)

## Background

The release workflows across `kawaz/*` repositories need to read and bump the semver string in `Cargo.toml`, `package.json`, `VERSION`, and `.claude-plugin/{plugin,marketplace}.json`. The existing generic `bump` tool (`kawaz/go/bin/bump`) requires `-f <file> -p <regex>` on every invocation, which makes justfiles verbose.

Example (current `claude-cmux-msg` justfile):

```bash
bump {{level}} -w -f .claude-plugin/plugin.json      -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f .claude-plugin/marketplace.json -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f package.json                    -p '"version":\s*"([^"]+)"'
```

Replacing this — three files, the same regex repeated three times — with a CLI that detects the format by filename is the goal.

## Approach

Hide format detection inside the tool, and keep the CLI surface to **action + input + optional flag** only.

## Architecture

### CLI surface

```
bump-semver <ACTION> <FILE | --value VER> [--write]

ACTION = major | minor | patch | get
```

`ACTION` is a flat 4-value enum. Keeping `get` at the same level as the bump actions structurally eliminates subcommand branching and argument-order ambiguity.

### Mutual exclusivity rules

| Combination | Result |
|---|---|
| `FILE` + `--value` | Error (exactly one is required) |
| `--write` + `--value` | Error (no file to write back to) |
| `--write` + `get` | Error (no meaning for a read-only operation) |
| Otherwise | Proceed |

### Module layout

Go sources live under `src/`, leaving only metadata (README / docs / justfile / VERSION / go.mod, etc.) at the repository root. `go.mod` itself stays at the root, so the module / import path remains `github.com/kawaz/bump-semver`. Build with `go build ./src`.

```
.
├── go.mod / go.sum
├── justfile
├── VERSION
├── README{,-ja}.md
├── docs/
└── src/
    ├── main.go             # entrypoint, argv parsing, exclusivity checks
    ├── handler.go          # Handler interface (Get / Replace) + dispatcher
    ├── handler_cargo.go    # Cargo.toml (TOML, [package].version)
    ├── handler_json.go     # *.json (.version)
    ├── handler_version.go  # VERSION (plain text)
    ├── semver.go           # X.Y.Z parsing + bump
    └── *_test.go           # unit tests per handler / semver / main
```

### Format detection (by basename)

| Match | Handler |
|---|---|
| `basename(path) == "Cargo.toml"` | cargo |
| `basename(path) == "VERSION"` | version |
| `path` ends with `.json` | json |
| Otherwise | Error (`unsupported file: <path>`) |

When stdin is a pipe, FILE is used **only** as a name hint for the dispatch above; the content is read from stdin.

### Bump semantics

For input `X.Y.Z`:

- `major` → `(X+1).0.0`
- `minor` → `X.(Y+1).0`
- `patch` → `X.Y.(Z+1)`
- `get`   → `X.Y.Z` (identity)

Pre-release / build metadata (`-alpha.1`, `+build.42`, etc.) is **not** supported in the MVP — encountering one is an error. Add support to the handler / semver module when concretely needed.

### Output

The new version is **always written to stdout on a single line** on success, regardless of `--write`. That makes `NEW=$(bump-semver patch Cargo.toml --write)` an easy shell idiom.

Errors print `bump-semver: <reason>` to stderr and exit non-zero.

## Distribution

### Release flow

```
just bump-version [patch|minor|major]
  ↓
ensure-clean → test → build → rewrite VERSION → jj describe + new → just push
  ↓
GitHub Actions (.github/workflows/release.yml) detects the VERSION change
  ↓
Build for 6 targets: Linux / macOS / Windows × amd64 / arm64
  ↓
gh release create --target <sha> --generate-notes (auto-tag + Release notes)
  ↓
update-homebrew job updates the Formula in kawaz/homebrew-tap
```

This pattern is established in kawaz/port-peeker / kawaz/jj-worktree / kawaz/authsock-warden (see jj-worktree/main/docs/decisions/DR-0003 for the full rationale). Because `bump-semver` itself can bump the VERSION file, the project is self-hosting from day one.

### Windows support

The tool only does file I/O and string manipulation, with no OS-specific calls, so cross-build from Linux runners is straightforward. Homebrew is not used for Windows — binaries are published to GitHub Releases only.

## Related repositories

- kawaz/jj-worktree (Rust): reference implementation for release workflows, DRs, and doc pair organisation
- kawaz/port-peeker (Go): minimal skeleton for VERSION-file-driven releases
- kawaz/claude-cmux-msg: primary consumer (three-file plugin version sync)
