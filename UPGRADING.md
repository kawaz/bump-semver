# Upgrading guide

## v0.13.x → v0.14.0

Pure additive minor release adding several new package-manifest
formats. No breaking changes. See [`docs/decisions/DR-0018-jvm-dotnet-haskell-rpm-support.md`](./docs/decisions/DR-0018-jvm-dotnet-haskell-rpm-support.md).

### New: JVM / .NET / Haskell / RPM support (DR-0018)

v0.14.0 extends the rule table to cover five new ecosystems:

| New file type | Confidence | Format | Notes |
|---|---|---|---|
| `pom.xml` (Maven) | 3 (basename) | xml-element | `/project/version` + `/project/artifactId`. `<parent>/<version>` is correctly skipped via the path-based query |
| `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) | 1 (glob) | xml-element | `/Project/PropertyGroup/Version` (first match wins on multi-PropertyGroup files) |
| `build.gradle` (Gradle Groovy DSL) | 2 (basename) | regex | accepts `version = '...'` / `version "..."` (method-call shorthand) / `version = "..."` |
| `build.gradle.kts` (Gradle Kotlin DSL) | 2 (basename) | regex | accepts `version = "..."` |
| `*.cabal` (Haskell) | 1 (glob) | regex | `^version: ...` (line-anchored, distinct from `cabal-version:`); `^name: ...` for cross-input checks |
| `*.spec` (RPM) | 1 (glob) | regex | `^Version: ...` (capital V); `^Name: ...` for cross-input checks |

### New format: `xml-element`

DR-0015 introduced an `xml` format that was Apple-plist-specific
(`<key>NAME</key><string>VALUE</string>` pair shape). v0.14.0 adds a
sibling `xml-element` format for path-rooted XML lookups:

- Path syntax: `/project/version`, `/Project/PropertyGroup/Version`
- XML namespaces are matched by local name only (Maven's
  `xmlns="http://maven.apache.org/POM/4.0.0"` does not need to be
  spelled out)
- Document-order first match wins on ambiguity
- Inner text is spliced into the original byte stream; DOCTYPE /
  attribute order / indentation / declaration are preserved

```bash
bump-semver get pom.xml
bump-semver patch pom.xml --write
bump-semver get MyApp.csproj
bump-semver patch MyApp.csproj --write

bump-semver get build.gradle
bump-semver patch build.gradle.kts --write
```

### No migration needed

All v0.13.x invocations work unchanged. The new file types simply
become recognised; previously-unsupported invocations like
`bump-semver get pom.xml` now succeed instead of failing with
`unsupported file:`.

## v0.12.x → v0.13.0

One BREAKING removal scoped to the `vcs:` input path plus several
additive features. The breaking change is easy to migrate (see below).

### BREAKING: `BUMP_SEMVER_VCS` env var removed (DR-0016)

The `BUMP_SEMVER_VCS=jj|git` environment variable, which used to sit
between `--vcs` and the `.jj` / `.git` probes in the VCS-detection
priority order, has been removed. The CLI now has:

```
1. --vcs jj|git           (--vcs auto / no flag → fall through)
2. .jj directory present  → jj
3. .git directory present → git
4. (otherwise)            → error
```

If your CI / dev environment set `BUMP_SEMVER_VCS=...`, replace it
with the `--vcs jj|git` flag:

```bash
# Old
export BUMP_SEMVER_VCS=jj
bump-semver compare gt Cargo.toml vcs:main@origin

# New
bump-semver --vcs jj compare gt Cargo.toml vcs:main@origin
```

See [`docs/decisions/DR-0016-remove-bump-semver-vcs-env.md`](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md).

### New: `--vcs auto` as an explicit default value (DR-0016)

`--vcs` now accepts `auto` in addition to `jj` / `git`. `auto` is the
default and is equivalent to omitting the flag. Useful as a
self-documenting CI command:

```bash
bump-semver --vcs auto compare gt Cargo.toml vcs:main@origin
```

### Changed: `--help` is now a short overview; `--help-full` is the reference

`bump-semver --help` (and a bare `bump-semver`) now prints a ~21-line
overview pointing at per-action help. The previous full content was
moved behind `--help-full`. Scripts that grep `bump-semver --help`
output may need to switch to `--help-full`.

```
bump-semver --help            # short overview (Usage + actions + pointers)
bump-semver --help-full       # complete reference (every format, every example)
bump-semver <action> --help   # action-specific reference (NEW, see below)
```

### New: subcommand `--help`

Each action accepts `--help` for an action-specific reference:

```bash
bump-semver patch --help      # bump help (shared by major/minor/patch)
bump-semver pre --help        # pre help (3 modes documented)
bump-semver get --help        # get help (--json, --no-pre, ...)
bump-semver compare --help    # compare help (every OP including DR-0017 precision)
```

### New: `compare` precision suffix (DR-0017)

Every compare operator optionally accepts a `-major` / `-minor` /
`-patch` suffix that truncates the comparison:

```bash
bump-semver compare eq-major 1.2.3 1.9.7        # exit 0 (same major)
bump-semver compare eq-patch 1.2.3 1.2.3-rc.1   # exit 0 (pre-release ignored)
bump-semver compare lt-minor Cargo.toml vcs:origin/main   # only minor-or-below changes since main?
```

5 bases × 4 precisions = 20 operators. Suffix-less operators are
unchanged (full SemVer 2.0.0 comparison including pre-release).

See [`docs/decisions/DR-0017-compare-precision-suffix.md`](./docs/decisions/DR-0017-compare-precision-suffix.md).

## v0.11.x → v0.12.0

Pure additive minor release; no breaking changes. See
[`docs/decisions/DR-0015-pbxproj-and-info-plist.md`](./docs/decisions/DR-0015-pbxproj-and-info-plist.md).

### New: `project.pbxproj` and `Info.plist` as path-pinned rules (DR-0015)

v0.12.0 adds two Xcode-specific path-pinned (confidence 3) rules and
two dedicated formats so iOS / macOS projects can bump every place
the marketing version lives in one invocation. v0.9.0 already covered
`*.xcconfig` (DR-0012); v0.12.0 closes the gap with `project.pbxproj`
(multi-match `MARKETING_VERSION` synchronisation) and `Info.plist`
(XML plist).

| Path | Format | Version key | Notes |
|---|---|---|---|
| `project.pbxproj` | pbxproj | every `MARKETING_VERSION = ...;` (synced) | Xcode reject submission if values diverge across build configurations; the format reads / writes them as one |
| `Info.plist` | xml | `<key>CFBundleShortVersionString</key><string>...</string>` | `<string>$(MARKETING_VERSION)</string>` (Xcode 11+ default) is treated as a placeholder and yields `unsupported file:` |

```bash
$ bump-semver get App.xcodeproj/project.pbxproj
1.2.3

$ bump-semver patch \
    App.xcodeproj/project.pbxproj \
    App/Info.plist \
    Configs/Release.xcconfig \
    --write
1.2.4
```

### `project.pbxproj`: multi-match consistency check

The new `pbxproj` format reads **every** `MARKETING_VERSION = ...;`
line in the file. When all values agree, the bump rewrites all of
them to the new version uniformly — preserving the "every build
configuration shares one marketing version" invariant Xcode and
App Store Connect require.

When values disagree (a previous bump that wrote only some of the
configurations, an unmerged branch, etc.), the same column-aligned
`version mismatch:` block introduced in v0.5.0 surfaces, this time
with `<file>:line:N` labels so the offending lines are obvious:

```
$ bump-semver get App.xcodeproj/project.pbxproj
bump-semver: version mismatch:
  App.xcodeproj/project.pbxproj:line:23 = 1.2.3
  App.xcodeproj/project.pbxproj:line:31 = 1.2.4
  App.xcodeproj/project.pbxproj:line:45 = 1.2.3
```

Both quote styles are accepted (`MARKETING_VERSION = 1.2.3;` and
`MARKETING_VERSION = "1.2.3";`); the rewriter preserves whichever
each match used.

### `Info.plist`: byte-range rewriting (no XML round-trip)

The new `xml` format walks the document with `encoding/xml.Decoder`
to locate the byte range of the target `<string>` element's value,
then splices the new value into the original content. The rewriter
**does not** call `xml.Marshal`, so DOCTYPE, the XML declaration,
attribute order, indentation (tabs vs spaces), and every sibling
key / value survive byte-for-byte:

```bash
$ cat Info.plist
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>1.2.3</string>
	<key>CFBundleIdentifier</key>
	<string>com.example.app</string>
</dict>
</plist>

$ bump-semver patch Info.plist --write
1.2.4

$ cat Info.plist
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>1.2.4</string>
	<key>CFBundleIdentifier</key>
	<string>com.example.app</string>
</dict>
</plist>
```

`CFBundleVersion` (build number) is intentionally not touched — build
numbers are not SemVer (commit counts, build hashes, monotonic
integers), and CI is the natural place to populate them.

### Placeholder Info.plist: `unsupported file:` outcome

Xcode 11+ projects default to letting `Info.plist` reference the
build setting via a `$(MARKETING_VERSION)` placeholder, with the
real value living in `project.pbxproj`. When `bump-semver` reads
such a placeholder it extracts the literal text, which then fails
the SemVer parser upstream and surfaces as the same
`unsupported file:` outcome users see for any other unparseable
input:

```
$ bump-semver get Info.plist
bump-semver: Info.plist:CFBundleShortVersionString=$(MARKETING_VERSION):
invalid version "$(MARKETING_VERSION)": expected [v|ver|version][_.-]?X[._]Y[._]Z[-PRE][+BUILD]
```

This is intentional. The fix is to add `project.pbxproj` to the
invocation (where the actual `MARKETING_VERSION = 1.2.3;` lines
live), not to teach `bump-semver` to walk `$(MARKETING_VERSION)`
references — that would put a build-system resolver inside a
version-bumper, well outside the tool's scope.

### Multi-file synchronisation across the Xcode triple

Combined with the v0.9.0 `*.xcconfig` rule (DR-0012), the typical
Xcode bump now reads / writes all three places at once:

```bash
$ bump-semver patch \
    App.xcodeproj/project.pbxproj \
    App/Info.plist \
    Configs/Release.xcconfig \
    --write
```

The cross-file consistency check (DR-0004) requires every input to
agree before any file is touched, so a half-bumped repository surfaces
the mismatch up front rather than producing partially synced output.

### No new dependencies

DR-0015 ships entirely as `src/format_pbxproj.go`,
`src/format_xml.go`, two `rules.go` rows, and two switch arms in
`tryRule` / `formatReplace`. No module additions, no binary size
increase. `encoding/xml` is part of the standard library; the
existing `regexp` import covers the OpenStep plist line matcher.

## v0.10.x → v0.11.0

Pure additive minor release; no breaking changes. See
[`docs/decisions/DR-0014-toml-section-scoped.md`](./docs/decisions/DR-0014-toml-section-scoped.md).

### New: `pyproject.toml` and `mojoproject.toml` as path-pinned rules (DR-0014)

v0.11.0 generalises the TOML rewriter into a single section-scoped
helper and uses it to register two new confidence-3 rules:

| Path | Format | Version path | Name path |
|---|---|---|---|
| `pyproject.toml` | TOML | `[project].version` (try) → `[tool.poetry].version` | `[project].name` (try) → `[tool.poetry].name` |
| `mojoproject.toml` | TOML | `[workspace].version` | `[workspace].name` |

```bash
$ cat pyproject.toml
[project]
name = "my-pkg"
version = "1.2.3"

$ bump-semver get pyproject.toml
1.2.3

$ bump-semver patch pyproject.toml --write
1.2.4

$ cat pyproject.toml
[project]
name = "my-pkg"
version = "1.2.4"
```

### `pyproject.toml`: PEP 621 first, Poetry-legacy second

The TOML format now treats `VersionPaths` as **OR** (first match
wins). For `pyproject.toml` the rule lists PEP 621's `[project]`
section first and the Poetry-legacy `[tool.poetry]` section second,
so a single rule covers both ecosystems. Files mid-migration that
carry both sections have **only the first match (PEP 621)**
rewritten — DR-0014 § 6 documents the trade-off.

```bash
$ cat pyproject.toml
[tool.poetry]
name = "my-pkg"
version = "1.2.3"

$ bump-semver patch pyproject.toml --write
1.2.4
# [tool.poetry].version is rewritten because [project] is absent
```

### TOML format: VersionPaths semantics changed to OR

Previously the TOML format iterated `VersionPaths` and required every
path to extract successfully (mirroring JSON / `package-lock.json`).
v0.11.0 switches TOML to first-match-wins so the new try-fallback
shape works. The change is invisible to existing rules — both
`Cargo.toml` (`.package.version`) and the DR-0011 `*.toml` fallback
(`.version`) carry a single VersionPath, so AND vs OR is identical
for them. JSON's AND semantics is unchanged (still drives
`package-lock.json` cross-field consistency).

### Existing TOML rules preserved

The DR-0011 `*.toml` confidence-1 fallback still reads / writes
top-level `version = "..."` for any TOML file that doesn't match a
higher-confidence rule. `Cargo.toml`'s `[package].version` still wins
over `*.toml` for files named `Cargo.toml`. A `pyproject.toml`
without `[project]` or `[tool.poetry]` (but with a top-level
`version`) cleanly falls through to the `*.toml` fallback and emits
the usual DR-0010 hint.

### Quote / comment preservation

The new section-scoped rewriter substitutes only the captured byte
range, so the quote style (single, double) and any trailing inline
`# comment` on the version line are preserved verbatim. Sections
above and below the bumped one are untouched bit-for-bit, including
nested sections like `[tool.poetry.dependencies]` whose own
`version = "..."` lines stay frozen.

### No new dependencies

DR-0014 is implemented entirely in `src/format_toml.go` and
`src/rules.go`. No module additions, no binary size increase.

## v0.9.x → v0.10.0

Pure additive minor release; no breaking changes. See
[`docs/decisions/DR-0013-suffix-stripped-format-detection.md`](./docs/decisions/DR-0013-suffix-stripped-format-detection.md).

### New: suffix-stripped fallback for backup-style filenames (DR-0013)

`bump-semver` now resolves files with a trailing **backup-style
suffix** by stripping one segment from the basename and retrying the
DR-0005 rule table. Previously these errored with `unsupported file:`;
v0.10.0 reads them as if the suffix weren't there, and emits a
`hint:` line to stderr so the resolution stays transparent.

| Suffix | Example | Resolved as |
|---|---|---|
| `.bak` / `.backup` / `.orig` / `.tmp` / `.old` | `Cargo.toml.bak` | `Cargo.toml` rule (confidence 2) |
| `.YYYYMMDD` (8 digits) | `package.json.20260510` | `package.json` rule (confidence 2) |
| `.YYYYMMDD_HHMMSS` (8+`_`+6 digits) | `Chart.yaml.20260510_120000` | `*.yaml` fallback (confidence 1) |
| trailing `~` (Emacs / vi) | `Cargo.toml~` | `Cargo.toml` rule (confidence 2) |

```bash
$ cp Cargo.toml Cargo.toml.bak
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3

$ bump-semver compare gt Cargo.toml Cargo.toml.20260510
hint: Cargo.toml.20260510 matched as Cargo.toml rule (suffix .20260510 stripped); use --no-hint to suppress
# (exit 0 if the live file is newer than the dated backup)
```

### Confidence downgrade

The chosen rule's reported confidence is downgraded one band so
callers can see the rule was reached via the suffix-stripped
fallback:

- confidence 3 (path-pinned, e.g. `Cargo.toml`) → reported as 2
- confidence 2 (basename-only, e.g. `mix.exs`) → reported as 1
- confidence 1 (glob fallback, e.g. `*.json`) → still 1 (floor)

When the suffix-stripped form lands on a confidence-1 glob rule
(`unknown.json.bak` → strip `.bak` → `*.json`), **both** the suffix
hint and the existing DR-0010 fallback hint fire. The suffix hint is
emitted first (filename-level) and the fallback hint second
(content-level):

```
hint: unknown.json.bak matched as unknown.json rule (suffix .bak stripped); use --no-hint to suppress
hint: unknown.json.bak matched as *.json fallback. Open issue if explicit support is needed.
1.2.3
```

### Single-level stripping (no recursion)

Multi-stage suffixes (`Cargo.toml.bak.20260510`) strip **only the
trailing segment**. The intermediate form (`Cargo.toml.bak`) is
retried once; if it fails to resolve, the whole call fails with the
original `unsupported file: Cargo.toml.bak.20260510` error.
Recursive stripping is intentionally deferred — single-suffix files
are the 95% case, and multi-stage chains can be opted in via a
future DR if real-world need surfaces.

### Template-style suffixes are NOT stripped

`.template` / `.example` / `.sample` / `.dist` are intentionally left
out of the known-suffix list. Their content is usually a placeholder
(`__VERSION__`, `0.0.0`) and silently treating them as real
manifests would be more dangerous than the current `unsupported
file:` behaviour. To bump-read a template, copy it under a
backup-style name (`cp Cargo.toml.template Cargo.toml.tmp`).

### Suppression

The suffix hint shares the existing `hint:` prefix — `--no-hint` /
`-q` / `-qq` suppress it exactly as they already do for `*.json` /
`*.yaml` / `*.yml` / `*.toml` / etc. CI invocations that don't want
the noise can keep using `--no-hint`.

### `--write` works for suffix-stripped files

`bump-semver patch Cargo.toml.bak --write` rewrites the **backup
file** itself, not the live `Cargo.toml`. This is intentional — the
input path determines the output path, just like every other FILE
input. Whether bumping a backup makes sense is left to the caller.

### No new dependencies

DR-0013 is a pure dispatcher change in `src/rules.go`,
`src/handler.go`, `src/main.go` plus a new `src/suffix.go` helper.
No module additions, no binary size increase.

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
