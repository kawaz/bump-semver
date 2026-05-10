# Upgrading guide

## v0.8.x → v0.9.0

Pure additive minor release; no breaking changes. See
[`docs/decisions/DR-0012-regex-format.md`](./docs/decisions/DR-0012-regex-format.md).

### New: `regex` format covering eight file types (DR-0012)

v0.9.0 introduces a generic `regex` format that rewrites a single
line of source code, and uses it to add eight new file types:

| basename / glob | Confidence | Example version line |
|---|---|---|
| `v.mod` (V) | 2 | `version: '1.2.3'` |
| `build.zig.zon` (Zig) | 2 | `.version = "1.2.3"` |
| `mix.exs` (Elixir) | 2 | `version: "1.2.3"` |
| `build.sbt` (Scala) | 2 | `version := "1.2.3"` |
| `*.xcconfig` (Xcode) | 1 (fallback) | `MARKETING_VERSION = 1.2.3` |
| `*.podspec` (CocoaPods) | 1 (fallback) | `s.version = '1.2.3'` |
| `*.nimble` (Nim) | 1 (fallback) | `version = "1.2.3"` |
| `*.gemspec` (Ruby) | 1 (fallback) | `s.version = "1.2.3"` |

```bash
$ cat Release.xcconfig
MARKETING_VERSION = 1.2.3

$ bump-semver patch Release.xcconfig --write
hint: Release.xcconfig matched as *.xcconfig fallback. Open issue if explicit support is needed.
1.2.4

$ cat MyPod.podspec
Pod::Spec.new do |s|
  s.name    = 'MyPod'
  s.version = '1.2.3'
end

$ bump-semver get MyPod.podspec
hint: MyPod.podspec matched as *.podspec fallback. Open issue if explicit support is needed.
1.2.3
```

### Single-match semantics

The `regex` format reads / rewrites only the **first** matching line
in the file. Files that need synchronised updates of multiple
version-shaped lines (Xcode `*.pbxproj` build settings, `Info.plist`
with `CFBundleShortVersionString` + `CFBundleVersion`) are
intentionally **out of scope** for v0.9.0. Open an issue if you need
this — a dedicated format (e.g. `format_pbxproj.go`) is the right
answer, not regex extension.

### Quote / comment preservation

The rewriter substitutes only the captured byte range, so quote style
(single, double, or unquoted) and trailing comments on the version
line are preserved verbatim. The same DR-0010 fallback hint fires for
the four glob-based confidence-1 rules (`*.xcconfig` / `*.podspec` /
`*.nimble` / `*.gemspec`); the four basename-based confidence-2 rules
(`v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt`) match without a
hint because they're explicit.

### No new dependencies

`regex` is implemented with the standard library `regexp` package —
no module additions, no binary size increase.

### Suppression

The fallback hint shares the existing `hint:` prefix — `--no-hint` /
`-q` / `-qq` suppress it exactly as they already do for `*.json` /
`*.yaml` / `*.yml` / `*.toml`.

## v0.7.x → v0.8.0

Pure additive minor release; no breaking changes. See
[`docs/decisions/DR-0011-yaml-yml-toml-fallback.md`](./docs/decisions/DR-0011-yaml-yml-toml-fallback.md).

### New: `*.yaml` / `*.yml` / `*.toml` confidence-1 fallback (DR-0011)

`bump-semver` now resolves arbitrary YAML and TOML files through
confidence-1 fallback rules, in addition to the pre-existing
`*.json` fallback. Files that previously errored with
`unsupported file:` are now read / bumped if they expose a
top-level `.version` (`version:` for YAML, `version = "..."` for
TOML).

```bash
$ cat Chart.yaml
apiVersion: v2
name: my-chart
version: 1.2.3

$ bump-semver patch Chart.yaml --write
hint: Chart.yaml matched as *.yaml fallback. Open issue if explicit support is needed.
1.2.4

$ cat manifest.toml
name = "my-pkg"
version = "1.2.3"

$ bump-semver get manifest.toml
hint: manifest.toml matched as *.toml fallback. Open issue if explicit support is needed.
1.2.3
```

### Scope of the fallback

The new rules are **deliberately conservative**:

- They only look at **top-level** keys. A nested `version:` (under
  another mapping) or a section-scoped `version = ...` (e.g.
  `[project] version` inside `pyproject.toml`) is not picked up.
- Multi-document YAML (`---`-separated) is read as the first
  document only.
- The path-pinned `Cargo.toml` rule (confidence 3) still wins for
  files matching that name — the new `*.toml` fallback does not
  affect existing `[package].version` behaviour.

If you need section-scoped or nested coverage (`pyproject.toml`'s
`[project].version`, Helm chart `spec.version`, etc.), open an issue
so a path-pinned confidence-3 rule can be added — exactly as DR-0001
recommends ("add a row only when concretely needed").

### Quote / comment preservation

Both writers preserve the source file's quoting style (double, single,
or unquoted) and any inline `# comment` / `# bumped weekly` on the
version line. We rewrite via line-anchored regex rather than
round-tripping through `yaml.Marshal` / TOML re-serialisation, so
key order and surrounding comments are guaranteed to stay intact.

### Suppression

The fallback hint shares the existing `hint:` prefix — `--no-hint` /
`-q` / `-qq` suppress YAML/TOML hints exactly as they already do for
`*.json`.

### Dependency

Adds `gopkg.in/yaml.v3 v3.0.1` to the build (no Go module path
changes; `go install` and Homebrew users get this transparently).

## v0.7.1 → v0.7.2

Pure additive patch release; no breaking changes. See
[`docs/decisions/DR-0010-fallback-match-hint.md`](./docs/decisions/DR-0010-fallback-match-hint.md).

### New: confidence-1 fallback hint (DR-0010)

When a FILE input matches DR-0005's lowest-confidence `*.json` glob
fallback rule, `bump-semver` now writes a one-line hint to stderr per
such input:

```
$ bump-semver get unknown.json
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
1.2.3
```

This makes it visible when a filename was handled by the generic
fallback rather than an explicit rule, and invites issues for filenames
that should get explicit support.

The hint fires for `get` / `major` / `minor` / `patch` / `pre` /
`compare` alike — it reflects the file detection, not the action.

### New: issue-tracker pointer in `unsupported file:` errors

When a FILE doesn't match any rule at all, the `unsupported file:`
error is followed by a hint pointing at the issue tracker:

```
$ bump-semver get unknown.toml
bump-semver: unsupported file: unknown.toml
hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.
```

### Suppression

Both new hints share the existing `hint:` prefix and are suppressed by
the existing `--no-hint` / `-q` / `-qq` flags — same way as the v0.5.0
"files not modified" hint. CI invocations that don't want the noise can
keep using `--no-hint`. No change to `compare`'s exit-code semantics.

## v0.7.0 → v0.7.1

Pure additive patch release; no breaking changes.

### Highlight: `--version --json`

`bump-semver --version --json` now emits the same structured JSON schema
as `--json` does for `get` / bump actions. Useful for CI scripts:

```bash
bump-semver --version --json | jq -r .semver   # 0.7.1
```

`bump-semver --version` without `--json` still prints the plain `vX.Y.Z`
string as before.

### Other fixes

- Help text Examples cleaned up (positional VER for clarity, removed
  unrealistic `Cargo.toml package.json` cross-compare example, fixed the
  `vcs:origin/main` example to use `compare lt` for the typical
  "stale-vs-remote" check).
- Homebrew formula's `test do` block updated for v0.5.0+ CLI
  (positional VER instead of removed `--value`). Latent bug fix that
  only surfaced when running `brew test bump-semver` manually.

## v0.6.x → v0.7.0

Pure additive release; no breaking changes. See
[`docs/decisions/DR-0008-vcs-input.md`](./docs/decisions/DR-0008-vcs-input.md).

### New feature: `vcs:` input mode

Any positional INPUT may now start with `vcs:`. The argument is then
resolved through jj or git (auto-detected) instead of being read from
disk. This lets CI checks like "ahead of remote main?" or "bumped past
the last release tag?" be written on a single line.

```bash
# Replaces:  jj file show -r main@origin Cargo.toml | bump-semver compare lt Cargo.toml -
bump-semver compare gt Cargo.toml vcs:main@origin

# Take the largest semver-parseable tag and compare against it
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'

# Read a previous revision; FILE is borrowed from the sibling FILE input
bump-semver compare eq Cargo.toml vcs:HEAD~1
```

**VCS detection** (priority order): `--vcs jj|git` flag, then
`BUMP_SEMVER_VCS` env, then probe `.jj` / `.git` in cwd or any
ancestor. When both `.jj` and `.git` exist (jj colocate, or kawaz's
git-bare + jj-workspace layout), jj wins.

**`--write` is incompatible with `vcs:` inputs.** vcs: is read-only by
design — combining the two errors out with a clear message rather than
silently dropping one input.

**`bump-semver` does not run `git fetch` / `jj git fetch` for you.** If
`vcs:origin/main` is stale, the underlying VCS error is surfaced as-is.

See README's "vcs: input" section for the full reference.

## v0.5.x → v0.6.0

Pure additive release; no breaking changes. See
[`docs/decisions/DR-0007-json-output-option.md`](./docs/decisions/DR-0007-json-output-option.md).

### New feature: `--json` output

`get` and the bump actions (`major` / `minor` / `patch` / `pre`) accept
`--json`, producing one line of structured JSON (terminated with a
newline) suitable for `jq` pipelines:

```bash
bump-semver get Cargo.toml --json
# {"name":"my-pkg","version":"1.2.3","semver":"1.2.3","major":1,...}

bump-semver patch Cargo.toml --json
# bumped version, fully decomposed
```

The schema covers `name` / `version` / `semver` / `major` / `minor` /
`patch` / `pre` / `pre_id` / `pre_rest` / `build_metadata` / `build_id`
/ `build_rest`. `compare` does not accept `--json` (its answer is the
exit code, by design). See README's "JSON output" section for the full
field reference.

## v0.4.x → v0.5.0

v0.5.0 introduces pre-release / build-metadata support, the `compare`
subcommand, and a `pre` action, alongside three CLI surface changes that
break compatibility with v0.4.x.

For the design rationale see
[`docs/decisions/DR-0006-pre-release-and-compare.md`](./docs/decisions/DR-0006-pre-release-and-compare.md).

### Breaking changes

#### 1. `--value` is removed

The `--value VER` flag has been removed in favour of unified positional
inputs that accept either a FILE path or a raw VER string.

```diff
- bump-semver patch --value 1.2.3
+ bump-semver patch 1.2.3
- bump-semver get   --value v1.2.3
+ bump-semver get   v1.2.3
```

If you have a local file literally named `1.2.3` (or any string that
parses as a semver) and you mean the file, prefix with `./` to
disambiguate, per Unix convention:

```bash
bump-semver patch ./1.2.3 --write
```

VER and FILE inputs may be mixed in a single invocation; all detected
versions must agree, and only FILE-origin inputs are written back when
`--write` is given.

```bash
# "expected current = 1.2.3" check + write back to two files
bump-semver patch 1.2.3 a.json b.json --write
```

#### 2. Body separator `-` is no longer accepted

DR-0003 originally allowed `1-2-3` style versions (body separator
`[._-]`). Because pre-release identifiers also start with `-`, the two
syntaxes collide once pre-release is introduced. v0.5.0 narrows the body
separator to `[._]` only.

```diff
- bump-semver patch ver-1-2-3
+ bump-semver patch ver_1_2_3
- bump-semver get   1-2-3
+ bump-semver get   1.2.3       # or 1_2_3
```

The prefix-internal separator (between `version` and the digits) still
allows `-` (e.g. `version-1.2.3` is fine), only the digit-to-digit body
separators are restricted.

The chosen prefix and separator are still preserved on output, just
within the new `[._]` set.

#### 3. Bump-path error exit code: 1 → 2

Until v0.4.x the `bump` family exited with code `1` on errors. v0.5.0
introduces `compare`, which uses `1` for "predicate false" per the
`test` / `dpkg --compare-versions` convention. To keep the exit code
semantics consistent across the CLI, **all error paths now exit with
`2`** and `1` is reserved for "compare returned false".

| Outcome | v0.4.x exit | v0.5.0 exit |
|---|---|---|
| bump succeeded | 0 | 0 |
| bump failed (parse / IO / etc.) | 1 | **2** |
| compare predicate true | — | 0 |
| compare predicate false | — | 1 |
| compare encountered an error | — | 2 |

Shell scripts that branch on `$? -eq 1` directly should switch to
the more idiomatic `$? -ne 0`:

```diff
- if ! bump-semver patch Cargo.toml --write; then
-     # error path
-     exit 1
- fi
+ # both forms work, but the new exit code 2 makes "non-zero" the
+ # cleaner test:
+ if ! bump-semver patch Cargo.toml --write; then
+     exit 1
+ fi
```

If you specifically want to distinguish "compare false" from an actual
error, branch on the exit code explicitly:

```bash
if bump-semver compare lt Cargo.toml 1.0.0; then
    echo "still a 0.x release"
elif [ $? -eq 1 ]; then
    echo "already 1.0.0 or newer"
else
    echo "error" >&2
    exit 2
fi
```

### New features in v0.5.0

#### Pre-release / build metadata

The version parser now accepts SemVer 2.0.0 pre-release (`-rc.0`,
`-alpha.1`, etc.) and build metadata (`+sha.5114f85`, `+build.42`,
etc.). They are preserved verbatim on `get`, and dropped by default on
`major` / `minor` / `patch` unless `--pre` / `--build-metadata` is given
explicitly.

```bash
bump-semver get   1.2.3-rc.1+build.42         # 1.2.3-rc.1+build.42
bump-semver patch 1.2.3-rc.0                  # 1.2.4 (drop)
bump-semver patch 1.2.3-rc.0 --pre rc.0       # 1.2.4-rc.0 (re-attach)
bump-semver patch 1.2.3-rc.0 --build-metadata sha.abc
                                              # 1.2.4+sha.abc
```

This **differs from npm-style strip-don't-bump**, which would turn
`patch 1.2.3-rc.0` into `1.2.3` (drop pre, do not bump). DR-0006
explains why bump-semver chose the simpler "always bump, drop unless
explicit" rule.

#### `pre` action

Manage the pre-release portion without touching MAJOR/MINOR/PATCH:

```bash
bump-semver pre 1.2.3-rc.0               # 1.2.3-rc.1   (counter advance)
bump-semver pre 1.2.3      --pre rc.0    # 1.2.3-rc.0   (overwrite)
bump-semver pre 1.2.3-rc.0 --pre alpha   # 1.2.3-alpha  (overwrite, reset)
bump-semver pre 1.2.3-rc.0 --no-pre      # 1.2.3        (release-promotion)
```

Counter advance only succeeds when the trailing identifier is purely
numeric. `1.2.3-rc1` (alphanumeric mixed) errors with
`rc1 is not incremental, use --pre PRE`.

#### `compare` subcommand

Two-input comparison with `eq` / `lt` / `le` / `gt` / `ge` operators.
SemVer 2.0.0 ordering, build metadata excluded from ordering, and
prefix / separator differences are normalised.

```bash
bump-semver compare eq Cargo.toml package.json    # cross-file equality check
bump-semver compare lt 1.2.3-rc.1 1.2.3           # exit 0 (rc < release)
bump-semver compare lt Cargo.toml < <(jj file show -r main@origin Cargo.toml)
                                                  # CI: drifted from main?
```

Exit codes are `0` / `1` / `2` for true / false / error, following the
`test` convention.

#### Unified FILE | VER | `-` positional input

Each positional argument may now be a FILE path, a raw semver VER, or
`-` (read VER from stdin once). They can be mixed freely, and all
detected versions must agree. With `--write`, only FILE-origin inputs
are written back; VER / stdin inputs serve as reference values.

```bash
echo 1.2.3 | bump-semver compare eq Cargo.toml -    # mix file and stdin
bump-semver patch 1.2.3 a.json b.json --write       # check + bump + write
```
