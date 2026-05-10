# Upgrading guide

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
