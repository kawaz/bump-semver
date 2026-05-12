# bump-semver Design Document

> English | [日本語](./DESIGN-ja.md)

## Background

The release workflows across `kawaz/*` repositories need to read, bump, and compare the semver string in `Cargo.toml`, `package.json`, `VERSION`, and `.claude-plugin/{plugin,marketplace}.json`. The existing generic `bump` tool (`kawaz/go/bin/bump`) requires `-f <file> -p <regex>` on every invocation, which makes justfiles verbose.

Example (current `claude-cmux-msg` justfile):

```bash
bump {{level}} -w -f .claude-plugin/plugin.json      -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f .claude-plugin/marketplace.json -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f package.json                    -p '"version":\s*"([^"]+)"'
```

Replacing this — three files, the same regex repeated three times — with a CLI that detects the format by filename is the goal. v0.5.0 additionally folds in a `compare` subcommand so pre-release drift checks etc. can be done with the same tool (DR-0006).

## Approach

Hide format detection inside the tool, and keep the CLI surface to **action + input + optional flag** only. Inputs are unified positional **FILE / VER / `-`**, which composes well with shell pipelines.

## Architecture

### CLI surface

```
bump-semver <ACTION> <INPUT...> [flags]
bump-semver compare <OP> <INPUT> <INPUT>

ACTION = major | minor | patch | pre | get
OP     = eq | lt | le | gt | ge
INPUT  = FILE | VER | -
```

`ACTION` is a flat 5-value enum (`major` / `minor` / `patch` / `pre` / `get`). Comparison operators are placed under one nested subcommand (`compare`) so the bump/read surface stays flat while comparison gets its own namespace (DR-0006).

Multiple INPUTs are operated on as a single unit (DR-0004). Their detected versions must agree; their detected names are also cross-checked when available.

### Input modes (FILE | VER | `-` | `vcs:`)

Each positional argument is resolved in this priority order (DR-0006 確定論点 B; DR-0008 added the `vcs:` rule):

1. `-` → read VER from stdin, one line (stdin can be consumed at most once across all `-` arguments)
2. Starts with `vcs:` → resolve through the VCS (DR-0008, see below)
3. Exists as a file → FILE
4. Parses as semver → VER
5. Otherwise → error

When a filename collides with a valid semver string (e.g. a local file literally named `1.2.3`), prefix with `./` to disambiguate, per Unix convention.

#### `vcs:` input (DR-0008)

`vcs:REV[:FILE]` reads `<FILE>` at `<REV>` from jj or git. The VCS is detected in this priority order: `--vcs jj|git` flag (`auto` and the unset case fall through), `.jj` directory probe, `.git` directory probe. When both `.jj` and `.git` exist (jj's colocate mode, or kawaz's git-bare + jj-workspace layout), jj wins. See DR-0016 for the rationale behind removing the `BUMP_SEMVER_VCS` env var that used to sit between the flag and the probes.

`vcs:latest-tag([<arg>])` lists every tag, drops the ones that don't parse as semver, and returns the largest by SemVer 2.0.0 ordering. v0.15.0 (DR-0019) extended it to take a remote-repo argument:

- `vcs:latest-tag()` — no argument: query the cwd VCS (original behaviour)
- `vcs:latest-tag(kawaz/pkf-tasks)` — `owner/repo` short form: expanded to `https://github.com/<owner>/<repo>` and queried with `git ls-remote --tags`
- `vcs:latest-tag(https://...)` / `vcs:latest-tag(git@...)` — full HTTPS / SSH URLs pass through

In addition, a **`@` peel fallback** is applied during the semver parse of each candidate tag: monorepo-style names like `pkf-tasks@0.0.12` (Pkl package, npm scoped, Go module subpath conventions) fail the regular semver parser, but the substring after the last `@` is retried so they are recognised. The fallback only fires inside the tag-list path so jj revsets like `main@origin` (which are never tag-list output) are unaffected.

**Argument syntax design** (DR-0019): the `<arg>` portion is treated as a raw string — no inner double quotes required (think markdown link `[text](url)`). This keeps Pkl `Task.cmd` array forms readable (`["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]`) without JSON escape hell. Shells need an outer single-quote wrapper to keep `()` from being interpreted as a subshell (`'vcs:latest-tag(kawaz/pkf-tasks)'`).

**Trust boundary** (DR-0019): the validity of the remote URL is the **caller's responsibility**. Pointing at a repo where a third party can push tags allows an attacker to publish `malicious@99.99.99` and have it returned as the largest tag. This is outside the scope of what `bump-semver` itself can defend; consumers are expected to hard-code the repo argument and not let user input flow into it (e.g. `kawaz/pkf-tasks`'s `migrate:check-pkf-tasks-current` defaults `remoteRepoSpec` to a fixed repo).

When the FILE component is omitted, it is borrowed from the first FILE-providing sibling argument in **position order** (a real FILE-origin input, or another `vcs:REV:FILE`). Errors out when no sibling can supply a FILE.

`bump-semver` does not run `git fetch` / `jj git fetch`; stale-remote errors surface verbatim from the underlying VCS. `--write` is rejected when any input starts with `vcs:` (vcs: is read-only by design).

### Mutual exclusivity rules

| Combination | Result |
|---|---|
| `--pre` + `--no-pre` | Error (mutually exclusive) |
| `--build-metadata` + `--no-build-metadata` | Error (mutually exclusive) |
| `--write` + `get` / `compare` | Error (read-only / comparison has no writable target) |
| `--write` with zero FILE inputs | Error (`--write requires at least one FILE`) |
| Multiple INPUTs disagree | `version mismatch:` with column-aligned origin labels |
| Single FILE INPUT + stdin pipe | FILE is a name hint, content read from stdin (legacy) |
| Multiple INPUTs + stdin pipe | stdin pipe is ignored (explicit INPUTs win, cat / sed convention) |
| Otherwise | Proceed |

### Module layout

Go sources live under `src/`, leaving only metadata (README / docs / Taskfile.pkl / VERSION / go.mod, etc.) at the repository root. `go.mod` itself stays at the root, so the module / import path remains `github.com/kawaz/bump-semver`. Build with `go build ./src`.

```
.
├── go.mod / go.sum
├── Taskfile.pkl
├── VERSION
├── README{,-ja}.md
├── UPGRADING.md             v0.4.x → v0.5.0 migration guide
├── docs/
└── src/
    ├── main.go              entrypoint, argv parsing, multi-input consistency
    ├── compare.go           compare subcommand (Version.Compare → exit code)
    ├── handler.go           Handler interface (Inspect / Replace) + dispatcher
    ├── handler_*.go         Cargo.toml / *.json / package-lock.json / VERSION
    ├── format_*.go          format-specific Inspect/Replace (JSON / TOML / plain)
    ├── rules.go             path-aware confidence-ranked rule table (DR-0005)
    ├── jsonpath.go          map[string]any-based simple JSONPath
    ├── semver.go            SemVer 2.0.0 parser + Bump + Compare
    ├── json.go              --json output schema (DR-0007)
    ├── vcs.go               vcs: input (jj/git auto-detect + `latest-tag()`) (DR-0008)
    └── *_test.go            unit + integration + spec_table_test.go (DR-0006 spec-driven)
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

The currently supported formats are `json`, `toml`, `yaml`, `plain`, `regex` (DR-0012, line-anchored rewriter for single-line manifests like `*.cabal` / `*.spec` / `build.gradle` / `*.xcconfig`), `pbxproj` (DR-0015, Xcode multi-match), `xml` (DR-0015, Apple plist `<key>/<string>` pairs), and `xml-element` (DR-0018, slash-rooted XML path lookup used by `pom.xml` / `*.csproj`). The `xml` and `xml-element` formats are intentionally separate: plist's flat key-value shape and Maven/.NET's nested-element shape have different evaluation rules, so each gets its own dispatcher case (`rules.go::tryRule` / `formatReplace`).

When stdin is a pipe and exactly one FILE INPUT is given, FILE is used **only** as a name hint for the dispatch above; the content is read from stdin (legacy shortcut). With multiple INPUTs the stdin pipe is ignored (explicit INPUTs take precedence, cat / sed convention). Passing `-` as an INPUT explicitly invokes the new "read VER from stdin" path.

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

main aggregates `Versions` and `Names` across all INPUTs and requires:

- All version fields agree (otherwise `version mismatch:` with column-aligned origin labels)
- All name fields agree where available (otherwise `name mismatch:` ...). Files without a name are skipped, so `Cargo.toml` + `VERSION` works fine.

`Replace` writes only the version field(s); names are never touched. The `package-lock.json` handler streams the JSON document with a decoder so dependency entries (`$.packages["node_modules/..."]`) are guaranteed not to be rewritten even when their version happens to equal the current root version.

### Bump semantics

The version parser accepts SemVer 2.0.0 syntax with the kawaz prefix/sep extension (DR-0003 + DR-0006):

```
body:  (v|ver|version)?[._]?\d+[._]\d+[._]\d+      (sep1 == sep2 enforced)
pre:   -<id>(.<id>)*                                (per SemVer 2.0.0)
meta:  +<id>(.<id>)*                                (per SemVer 2.0.0)
```

- Body separator is `.` or `_` only. `-` is **not allowed** (would collide with the pre-release `-`; DR-0006 narrowed `[._-]` down to `[._]`)
- Numeric-only identifiers (in body and pre-release) must not have leading zeros (per SemVer)
- Build metadata identifiers may have leading zeros (per SemVer)

The optional prefix and the chosen separator are preserved through `Bump` and `String`. Pre-release and build metadata are **dropped** by default on bumps (DR-0006 — a single rule, distinct from the npm-style strip-don't-bump behaviour).

| Input | Action | Output |
|---|---|---|
| `1.2.3` | `patch` | `1.2.4` |
| `v1.2.3` | `patch` | `v1.2.4` |
| `version_1_2_3` | `minor` | `version_1_3_0` |
| `1.2.3-rc.0` | `patch` | `1.2.4` (drop) |
| `1.2.3-rc.0` | `pre` | `1.2.3-rc.1` (counter advance) |
| `1.2.3-rc1` | `pre` | error (alphanumeric-mixed is not incremental) |
| `1.2.3` | `pre --pre rc.0` | `1.2.3-rc.0` (overwrite) |
| `1.2.3-rc.0` | `pre --no-pre` | `1.2.3` (remove) |
| `1.2.3-rc.0` | `patch --pre rc.0` | `1.2.4-rc.0` (bump + re-attach) |
| `1.2.3-rc.0+build` | `patch` | `1.2.4` (both dropped) |

Inconsistent separators (`1.2_3`) are rejected.

The `pre` action has three modes:

- No flag: counter-advance only when the trailing identifier is purely numeric (`rc.0 → rc.1`); otherwise error
- `--pre PRE`: overwrite with PRE entirely (regardless of prior pre, going backwards is allowed)
- `--no-pre`: remove pre-release (no-op if there was none)

### Comparison semantics (compare subcommand)

`compare <OP> <INPUT> <INPUT>` follows SemVer 2.0.0 § 11 ordering:

1. MAJOR/MINOR/PATCH numerically
2. Pre-release version is "less than" the corresponding release (`1.0.0-rc.1 < 1.0.0`)
3. Pre-release identifiers are compared field-by-field — numeric vs numeric numerically, alphanumeric vs alphanumeric by ASCII, numeric < alphanumeric
4. Build metadata is **completely excluded** from ordering (`1.0.0+a == 1.0.0+b`)
5. Prefix / separator differences are normalised away (`v1.2.3` == `1.2.3` == `version_1_2_3`)

Each INPUT is resolved by the same FILE/VER/`-` logic as the bump path. INPUTs that contribute multiple version fields (e.g. `package-lock.json` exposing `$.version` and `$.packages[""].version`) are checked for internal agreement and collapsed to one value before comparison.

Exit codes:
- `0` — predicate true
- `1` — predicate false
- `2` — error (parse failure, mismatch, unsupported file, etc.)

This follows the `test` / `dpkg --compare-versions` convention (DR-0006 確定論点 A). The bump path's old "error = exit 1" behaviour was also unified to `2` here; shell scripts that previously branched on `$? -eq 1` for errors should switch to `$? -ne 0` (see UPGRADING.md).

#### precision suffix (DR-0017)

Each OP optionally accepts a `-major` / `-minor` / `-patch` suffix that truncates the comparison:

- `-major`: compare X only (`eq-major 1.2.3 1.9.7` → true)
- `-minor`: compare X.Y (`eq-minor 1.2.3 1.2.9` → true)
- `-patch`: compare X.Y.Z, ignoring pre-release (`eq-patch 1.2.3 1.2.3-rc.1` → true)
- (no suffix): full SemVer 2.0.0 § 11 comparison (pre-release included)

5 bases × 4 precisions = 20 operators. Build metadata is always ignored (SemVer § 10). Lets CI scripts express "I only care about the major bump" or "ignore pre-release noise — did the release version change?" in one line.

### Output

The new version is **always written to stdout on a single line** on success (regardless of `--write`, for bump actions). `compare` writes nothing to stdout even on a true predicate (avoids pipeline pollution; the result is signalled via exit code only).

Errors print `bump-semver: <reason>` to stderr and exit non-zero. The error message format depends on the input origin (DR-0006 確定論点 E):

- VER origin: the raw error message is passed through verbatim (e.g. `rc1 is not incremental, use --pre PRE`)
- FILE origin: wrapped as `<file>:<path>=<value>: <semver-error>`

When multiple INPUTs disagree, the values are listed column-aligned (DR-0006 確定論点 F):

```
bump-semver: version mismatch:
  Cargo.toml:[package].version = 1.2.3
  package.json:$.version       = 1.2.4
  <argv>                       = 1.2.3-rc.1
```

Origin labels: `<file>:<path>` (FILE) / `<argv>` or `<argv:N>` (positional VER) / `<stdin>` (`-`).

## Distribution

### Release flow

```
pkf run bump-version [patch|minor|major]
  ↓
ensure-clean → test → build → rewrite VERSION → jj describe + new → pkf run push
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
