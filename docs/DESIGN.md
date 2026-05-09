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
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value VER

ACTION = major | minor | patch | get
```

`ACTION` is a flat 4-value enum. Keeping `get` at the same level as the bump actions structurally eliminates subcommand branching and argument-order ambiguity.

Multiple FILEs are bumped together as a single unit (DR-0004). Their detected versions must agree; their detected names are also cross-checked when available.

### Mutual exclusivity rules

| Combination | Result |
|---|---|
| `FILE...` + `--value` | Error (exactly one form is required) |
| `--write` + `--value` | Error (no file to write back to) |
| `--write` + `get` | Error (no meaning for a read-only operation) |
| stdin pipe + multiple `FILE...` | stdin pipe is ignored, files are read from disk |
| stdin pipe + single `FILE` + `--write` | Error (stdin is the source, conflicts with writing back) |
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
    ├── main.go              # entrypoint, argv parsing, multi-file orchestration
    ├── handler.go           # Handler interface (Inspect / Replace) + dispatcher
    ├── handler_cargo.go     # Cargo.toml (TOML, [package].version + .name)
    ├── handler_json.go      # *.json ($.version + optional $.name)
    ├── handler_npm_lock.go  # package-lock.json (npm 7+, $.version + $.packages[""].version)
    ├── handler_version.go   # VERSION (plain text)
    ├── semver.go            # X.Y.Z parsing + bump (with v/ver/version prefix and . _ - separators)
    └── *_test.go            # unit + integration tests
```

### Format detection — path-aware, confidence-ranked (DR-0005)

The detector is a **table of `CandidateRule` rows**, each describing a `(path-pattern, format, version-paths, name-paths)` tuple, ordered by descending confidence. For an input FILE:

1. Walk rules in confidence order (3 → 2 → 1)
2. If the rule's path-pattern matches, attempt extraction (Inspect)
3. If extraction succeeds (every `VersionPaths` entry is found and parses as semver), the rule is the resolved one
4. If extraction fails, fall through to the next matching rule
5. If every matching rule fails, the deepest error is returned with `<path>: <ruleName>: <reason>`

Confidence levels:

- **3 — path-pinned**: relative path-suffix anchors (`.claude-plugin/marketplace.json`) or unique basename (`Cargo.toml`, `VERSION`, `package.json`, `package-lock.json`)
- **2 — basename only**: any directory's `marketplace.json` / `plugin.json` (Claude plugin convention, but not necessarily under `.claude-plugin/`)
- **1 — glob fallback**: `*.json` with top-level `.version` for everything else

This lets `marketplace.json` outside `.claude-plugin/` still get tried as a Claude-plugin marketplace first (confidence 2), and gracefully fall back to a plain `.version` JSON (confidence 1) if `.metadata.version` isn't present. Adding a new file format means **adding one row to the table** (and, if it's a brand new file format, one new format-specific Inspect/Replace pair). No `--pattern` flag is exposed at the CLI level.

When stdin is a pipe and exactly one FILE is given, FILE is used **only** as a name hint for the dispatch above; the content is read from stdin. With multiple FILEs the stdin pipe is ignored — the explicit files take precedence (cat / sed convention).

### Handler interface and consistency checks (DR-0004)

Each handler returns an `Inspection` describing every detected version-like and name-like value in the file:

```go
type Field struct {
    Value string
    Path  string  // human-readable: "$.version", "[package].version", "(file content)" 等
}

type Inspection struct {
    Versions []Field  // 1+
    Names    []Field  // 0+ (optional)
}

type Handler interface {
    Inspect(content []byte) (Inspection, error)
    Replace(content []byte, current, newVersion string) ([]byte, error)
}
```

main aggregates `Versions` and `Names` across all FILEs and requires:

- All version fields agree (otherwise `version mismatch:` with file:path = value lines)
- All name fields agree where available (otherwise `name mismatch:` ...). Files without a name are skipped, so `Cargo.toml` + `VERSION` works fine.

`Replace` writes only the version field(s); names are never touched. The `package-lock.json` handler streams the JSON document with a decoder so dependency entries (`$.packages["node_modules/..."]`) are guaranteed not to be rewritten even when their version happens to equal the current root version.

### Bump semantics

The version parser accepts `[v|ver|version][_.-]?X<sep>Y<sep>Z`, where `<sep>` is one of `.` / `_` / `-` and is required to be the same on both sides (DR-0003). The optional prefix and the chosen separator are preserved through `Bump` and `String`:

| Input | Action | Output |
|---|---|---|
| `1.2.3` | `patch` | `1.2.4` |
| `v1.2.3` | `patch` | `v1.2.4` |
| `version_1_2_3` | `minor` | `version_1_3_0` |
| `ver-1-2-3` | `major` | `ver-2-0-0` |
| `1-2-3` | `get` | `1-2-3` |

Inconsistent separators (`1.2-3`) are rejected. Pre-release / build metadata (`-alpha.1`, `+build.42`, etc.) is **not** supported in the MVP — encountering one is an error. Add support to the semver module when concretely needed.

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
