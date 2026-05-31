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
bump-semver get <INPUT...>
bump-semver <major|minor|patch|pre> <INPUT...> [--write]
bump-semver compare <eq|lt|le|gt|ge|...> <BASE> <OTHER...>
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit [--amend] [-m MSG] <PATH..|--staged>     # or: vcs commit --amend [-m MSG]
bump-semver vcs fetch [REMOTE]
bump-semver vcs push --branch NAME [--remote REMOTE]
bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]
bump-semver vcs tag delete NAME [--remote REMOTE]
bump-semver --version [--json]
bump-semver --help | --help-full
```

`<INPUT>` is either a **FILE path**, a **raw VER string**, **`-` (read VER from stdin, single line)**, **`vcs:REV[:FILE]` / `vcs:<func>(...)`** (read from the VCS, see [vcs: input](#vcs-input)), or **`cmd:<shell-command>`** (read from a shell command, see [cmd: input](#cmd-input)). Multiple inputs of mixed kinds may be given.

Help comes in three tiers (v0.13.0+):

- `bump-semver --help` / `-h`: short overview (actions + main navigation) fitting in one screen
- `bump-semver --help-full`: complete reference (supported file formats / full Examples / exit codes / etc.)
- `bump-semver <action> --help`: action-specific reference. `bump-semver patch --help` shows the bump help (shared by major/minor/patch), `bump-semver pre --help` documents the three pre modes, `bump-semver compare --help` lists the full 20-operator grid including precision suffixes.

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
bump-semver compare <OP> <BASE> <OTHER...>
```

`<OP>` is one of `eq` / `lt` / `le` / `gt` / `ge`. Comparison follows SemVer 2.0.0 ordering (build metadata excluded from ordering, prefix / sep differences normalised). `<BASE>` is the reference; every `<OTHER>` is checked independently as "BASE OP OTHER" ([DR-0023](./docs/decisions/DR-0023-n-arg-extension.md)). The legacy two-input form is the N=1 case.

| OP | True when |
|---|---|
| `eq` | BASE == every OTHER |
| `lt` | BASE <  every OTHER |
| `le` | BASE <= every OTHER |
| `gt` | BASE >  every OTHER |
| `ge` | BASE >= every OTHER |

Exit codes: `0` = true / `1` = false / `2` = error (`test` / `dpkg --compare-versions` convention). On `1` each failing pair is described on stderr (e.g. `compare gt: VERSION (0.26.3) is not greater than O1=vcs:main@origin (0.27.0)`). Use `-qq` to suppress the per-OTHER listing.

Each OP may carry a `-major` / `-minor` / `-patch` suffix that truncates the comparison ([DR-0017](./docs/decisions/DR-0017-compare-precision-suffix.md)). 5 bases × 4 precisions = 20 operators.

When an OTHER's `vcs:REV` spec has no explicit FILE, BASE's path is borrowed: `compare gt VERSION vcs:main vcs:v1.0.0` reads `vcs:main:VERSION` and `vcs:v1.0.0:VERSION`.

```bash
bump-semver compare eq Cargo.toml v1.2.3 && echo same
bump-semver compare lt 1.2.3-rc.1 1.2.3                       # exit 0 (rc < release)
bump-semver compare eq-major 1.2.3 1.9.7                      # exit 0 (same major)
bump-semver compare eq-patch 1.2.3 1.2.3-rc.1                 # exit 0 (pre-release ignored)
bump-semver compare lt-minor Cargo.toml vcs:origin/main       # only minor-or-below bumps?
bump-semver compare gt VERSION 'vcs:main@origin' 'vcs:v1.0.0' # ahead of both main and v1.0.0
```

### vcs subcommand

```
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit -m MSG PATH..
bump-semver vcs commit -m MSG --staged
bump-semver vcs commit --amend [-m MSG] [PATH.. | --staged]
bump-semver vcs fetch [REMOTE]
bump-semver vcs push --branch NAME [--remote REMOTE]   # jj users: --bookmark also accepted
bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]
bump-semver vcs tag delete NAME [--remote REMOTE]      # idempotent (rm -f semantic)
```

A small family of git/jj-agnostic helpers ([DR-0020](./docs/decisions/DR-0020-vcs-subcommands.md)). PR-1 shipped `vcs get` (read-only); PR-2 adds `vcs is` (predicate); PR-3 adds `vcs diff` (patch printer); PR-3.1 extends `vcs diff` with `-s/--name-status` (M/A/D summary) and `-q/--quiet` (exit-code reflects diff presence, mirroring `git diff --quiet`); PR-4 adds `vcs commit` (path-required commit with safety defaults); PR-5 adds `vcs fetch` / `vcs push` (the network counterparts, with `--force` intentionally absent and non-ff detection mapped to exit 5); PR-6 adds `vcs tag push` / `vcs tag delete` (atomic create+push / idempotent delete, with `--allow-move` as the precise opt-in for tag relocation and exit 4 surfacing different-rev integrity violations). The motivation is the recurring `Taskfile / justfile` pain of branching on git vs jj — `bump-semver` already abstracts version reads via `vcs:`, so the `vcs` verb is the natural place for these helpers.

**`vcs get <key>`** — emit a value on stdout:

| Key | Output |
|---|---|
| `root` | Absolute path to the repository root |
| `backend` | `git` or `jj` (jj wins on a colocated repo) |
| `current-branch` | The unambiguous current branch (git) / bookmark (jj). Detached HEAD or multiple bookmarks at the same head → exit 4 |

**`vcs is <pred>`** — exit code is the answer (0=true, 1=false, silent on stderr):

| Predicate | Meaning |
|---|---|
| `clean` | Worktree has no uncommitted changes. **git**: `git diff --quiet` AND `git diff --cached --quiet` (untracked files ignored). **jj**: the working-copy change `@` is empty (template `empty`). jj snapshots on read, so newly-created files DO render dirty — this asymmetry vs git is intentional |
| `dirty` | `!clean` |
| `git` / `jj` | The detected (or `--vcs`-forced) backend matches |

**`vcs diff REV [PATH..]`** — print the patch between `REV` and the working copy on stdout. Backend-uniform: git runs `git diff REV [-- PATH..]`, jj runs `jj diff --from REV --to @ [-- PATH..]`. Both forms compare REV against the worktree, including uncommitted changes.

`-s` / `--name-status` switches the output to one `<CODE>\t<path>` line per changed file (M/A/D — modify / add / delete). git native; jj's `--summary` output is normalized to tab-separated form so the result is uniform across backends.

`-q` / `--quiet` on `vcs diff` overloads the global "suppress stdout" meaning to also mirror `git diff --quiet`'s `--exit-code` semantic: **exit 0 = no diff, exit 1 = diff present**. Stdout is empty; stderr is preserved unless `-qq` is used. With `-s -q`, `-q` wins (stdout empty, exit reflects presence). This is the predicate form for scripting "has anything changed since REV?" — particularly useful for `check-version-bumped`-style gates. Other vcs verbs (`get`/`is`) keep the pure stdout-suppression meaning; the overload is justified by the diff verb being the only one whose "is there anything?" question is well-posed.

Path filter rule (**declarative convergence**): nonexistent `PATH` arguments are silently ignored. When every supplied `PATH` is filtered out the command exits `0` with empty stdout — it does **NOT** widen back to "diff everything". A path present in `REV` but deleted in the worktree is not shown when named explicitly (the full diff with no `PATH` still shows the deletion). Under `-q`, all-filtered yields exit 0 (= "no diff to report").

Exit codes (also see below): `0` success / predicate true (incl. `vcs diff -q` with no diff); `1` predicate false (`vcs is`, and `vcs diff -q` when diff is present); `2` usage error; `3` VCS subprocess error (incl. "not a repo", unresolvable REV); `4` ambiguous answer; `5` non-fast-forward push (`vcs push` only).

```bash
bump-semver vcs get backend                  # git
bump-semver vcs get root                     # /path/to/repo
bump-semver vcs get current-branch           # main
ROOT=$(bump-semver vcs get root) || exit

bump-semver vcs is clean && bump-semver patch VERSION --write
if bump-semver vcs is git; then ... fi
bump-semver vcs is dirty || echo "nothing to commit"

bump-semver vcs diff HEAD~1                   # full diff since previous commit
bump-semver vcs diff main@origin VERSION      # what changed in VERSION vs remote main
bump-semver vcs diff HEAD~1 src lib           # subtree-scoped diff
bump-semver vcs diff -s HEAD~1                # M/A/D file list (git --name-status format)
bump-semver vcs diff -q HEAD~1 -- VERSION && echo "VERSION unchanged"
                                              # exit 0 ⇔ no diff in VERSION
```

**`vcs commit`** — three commit modes with opinionated safety defaults.

| Mode | Behaviour |
|---|---|
| `-m MSG PATH..` | Stage + commit each existing path's working-tree content. Nonexistent paths silently dropped (declarative convergence). All-nonexistent / no real change → exit 0 with no commit (idempotent) |
| `-m MSG --staged` | Commit every staged/dirty change in one shot. **git**: commits the index. **jj**: commits the whole `@` snapshot (jj auto-stages). No content → exit 0, idempotent |
| `--amend [-m MSG] [PATH.. \| --staged]` | Fold the current change into the previous commit instead of creating a new one. Fully symmetric with the two modes above — `--amend` accepts the same `PATH..` / `--staged` selectors. Bare `--amend` is an explicit rewrite (ungated; message-only amend with no change is legal); the absorbed scope follows the backend — **git**: folds the staged index into HEAD (unstaged worktree changes are NOT included); **jj**: folds the entire `@` snapshot into `@-` (jj auto-stages, so this IS every current change). `--amend PATH..` folds only those paths (same all-nonexistent / no-change → no-op rule as plain path mode). `--amend --staged` is an explicit synonym for bare amend (the index / `@` snapshot IS amend's absorption source). With `-m`: rewrite the previous commit's message; without: preserve it. Equivalences: git → `git add -- PATHS; git commit --amend [-m\|--no-edit] -- PATHS`; jj → `jj squash --from @ --into @- [-m MSG \| -u] [-- PATHS]` |

**`-a` / `--all` is intentionally not provided** (DR-0020 safety). jj's auto-staged worldview makes `-a`'s unstaged-grab semantic too easy to trip on; use `--staged` (commit all current changes) or pass `PATH..` explicitly. Calling `-a` exits 2 with a hint pointing at `--staged` / `PATH..`.

The empty-no-op rule for path / `--staged` modes makes this snippet portable across languages:

```bash
# Commit whatever version-bearing files exist & changed; safe if some don't apply
bump-semver vcs commit -m "bump version" VERSION Cargo.toml package.json pyproject.toml
```

Exit codes for `vcs commit`: `0` success or idempotent no-op; `2` usage error (missing `-m`, `-a` rejected, `--staged + PATH`, no-mode); `3` VCS subprocess error (not a repo, commit failed).

```bash
bump-semver vcs commit -m "bump 1.2.3" VERSION         # commit just VERSION
bump-semver vcs commit --staged -m "release: 1.2.3"     # commit everything staged
bump-semver vcs commit --amend                          # absorb (git: index; jj: @) into previous, keep msg
bump-semver vcs commit --amend -m "release: 1.2.3 (final)"  # rewrite previous msg
bump-semver vcs commit --amend VERSION                  # fold ONLY VERSION into previous
bump-semver vcs commit --amend --staged -m "fixup"      # fold all staged into previous
```

**`vcs fetch [REMOTE]`** — refresh refs from the named remote (default `origin`).

- **git**: `git fetch <remote>`
- **jj**: `jj git fetch --remote <remote>`

REMOTE may be passed as a positional or via `--remote NAME` — supplying both at once is a usage error to avoid silent precedence surprises. Refspec scoping, prune, and tag flags intentionally pass through the underlying tool (= drop down to plain `git fetch ...` / `jj git fetch ...` for those).

**`vcs push --branch NAME [--remote REMOTE] [--jj-bookmark-auto-advance]`** — upload `NAME` to `REMOTE` (default `origin`). `--branch` is canonical; jj users may also write `--bookmark` (= the jj-native term for the same thing). The two spellings share one slot, so supplying both is a usage error.

| Aspect | Behaviour |
|---|---|
| Mode | **git**: `git push <remote> <name>:<name>` (explicit refspec avoids `push.default` surprises). **jj**: `jj git push --bookmark <name> --remote <remote>` followed by `jj git export` (colocated `.git` stays in sync). Export failure is retried once (transient packed-refs / HEAD races usually clear); persistent failure → exit 3 with a recovery hint pointing at the matching [jj-vcs/jj issue](https://github.com/jj-vcs/jj/issues) (`#493` ref-hierarchy, `#6098` HEAD race, `#6203` packed-refs) |
| Name required | No auto-detection from current branch / bookmark. Naming it explicitly removes the "wait, which ref did I just push?" surprise — typos and stale state can't lead to the wrong ref going out |
| Idempotency | "Remote already has it" → exit 0; git/jj's own `Everything up-to-date` / `Nothing changed` line is forwarded to stderr so the user can confirm the convergence happened. DR-0020's 0-targets-no-op rule applies |
| Non-fast-forward | Remote rejected the push → **exit 5**; the underlying git/jj stderr is passed through verbatim (no editorial `remote has diverged` paraphrase). Recovery is the user's call — `git fetch` + reconcile, or `git push --force-with-lease` directly if you genuinely mean to rewrite remote history. `--force` is intentionally not exposed (exits 2) |
| `--force` / `--tags` | Not provided. Force push rewrites remote history (out of scope for a SemVer helper); tag pushing belongs to release automation (`gh release create`), not to this verb |
| `--jj-bookmark-auto-advance` | **jj-only opt-in (PR-5.2)**. Before pushing, move the bookmark to the publishable commit: clean `@` (empty working copy) → bookmark to `@-`; dirty `@` (non-empty, typically described) → bookmark to `@`. The bookmark must exist (otherwise the normal push reports it) AND must be in `ancestors(@)` — sideways/divergent positioning → exit 3 with a hint, no move. The move itself is forward-only (no `--allow-backwards`). Running this on a git repo is a usage error (exit 2). Why: jj 慣習 places bookmarks on confirmed commits (`@-`), not on the throw-away working copy (`@`). Manually running `jj bookmark move` every bump is friction — this flag automates the move while keeping the safety checks explicit |

```bash
bump-semver vcs fetch                      # fetch origin
bump-semver vcs fetch upstream             # fetch a specific remote

bump-semver vcs push --branch main         # push main to origin
bump-semver vcs push --branch main --remote upstream

# Common pre-release gate (Taskfile pattern):
bump-semver vcs is clean \
  && bump-semver vcs fetch \
  && bump-semver vcs push --branch main

# jj: auto-advance the bookmark before pushing (no manual `jj bookmark move`)
bump-semver vcs push --branch main --jj-bookmark-auto-advance
```

Exit codes for `vcs push`: `0` success / no-op; `2` usage (`--branch`/`--bookmark` missing, both supplied, `--force` passed, positional args, unknown flag, `--jj-bookmark-auto-advance` on a git repo); `3` VCS subprocess error (unknown remote, network, jj export failure that persisted across the retry, `--jj-bookmark-auto-advance` refused because the bookmark is not in `ancestors(@)`); `5` non-fast-forward — read git/jj's stderr for the recovery path.

**`vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]`** — create / move the tag `NAME` at `REV` and push it to `REMOTE` (default `origin`) in a single atomic intent. The verb's contract is "the tag points to `REV` on the remote when this returns" — the local create is the means, not the deliverable, so the tag lifecycle stays 1-1 with its remote presence (no orphan local tags).

| Aspect | Behaviour |
|---|---|
| Mode | **git**: `git tag NAME REV` (or `git tag -f` for `--allow-move`) followed by `git push origin refs/tags/NAME` (`--force` only when `--allow-move`). **jj**: `jj tag set NAME -r REV` (with `--allow-move` if moving) followed by `jj git export` then `git -C <git_target> push ...` — native git push because jj 0.41 has no per-tag push primitive (DR-0020 line 70 commits to "create via jj tag set, push via native git" so jj retains tag awareness while we get fine-grained remote control) |
| Same-rev re-push | Local already at the same target → skip local create, still push. This is the 片落ちリカバリ case: local has the tag but the previous push may have failed before reaching the remote. Same-rev push is a clean no-op when the remote also matches |
| Different-rev no flag | **Exit 4** with no side-effect (no local move, no push attempt). Distinct from generic `3` so callers can branch on integrity violations |
| Different-rev `--allow-move` | Move locally + force-push to remote. `--force-with-lease` is not used: tag refs have no remote-tracking ref, so a bare lease can't establish anything and is no safer than `--force`; the move is already gated behind explicit `--allow-move` + the diff-rev pre-check, so we know what we're overwriting |
| Bad REV | Resolution failure → **exit 3** before any side-effect — distinguishable from "your tag has drifted" (4) and "git/jj broke" (also 3 but with the tool's stderr folded in) |
| `--force` / `--tags` / `--all` | Not provided. `--force` is too broad — it conflates same-rev idempotent reconciliation with different-rev rewrites; `--allow-move` is the precise opt-in. Bulk operations are out of scope (DR-0020 line 91) |

**`vcs tag delete NAME [--remote REMOTE]`** — remove the tag from both local and remote, idempotent. Per DR-0020 line 74 (`rm -f` semantic): the verb's intent is the end-state "no tag at NAME", which an already-absent tag already satisfies, so absent on either side is exit 0, not an error.

- **git**: pre-checks local existence via `git rev-parse -q --verify refs/tags/NAME` (bare `git tag -d NAME` errors on missing) then `git push origin :refs/tags/NAME` (git's own "deleting a non-existent ref" returns exit 0 — naturally idempotent at the remote layer)
- **jj**: `jj tag delete NAME` is natively idempotent ("No matching tags" → exit 0) so we just run it; then `jj git export` and the same `git push origin :refs/tags/NAME` for the remote half
- A genuine remote failure (unknown remote, network down) is exit 3; the local-half side-effect may have already happened. We accept that asymmetry — the common case is "remote is fine, just clean up old local tags" and the alternative ("only delete locally if remote ack'd") would trade rare clean retries for frequent friction
- `--allow-missing` is **not provided** — delete is already idempotent so the flag would be a no-op (DR-0020 line 92)

```bash
bump-semver vcs tag push --rev HEAD v1.2.3
                                                # tag HEAD as v1.2.3, push to origin
bump-semver vcs tag push --rev HEAD~1 v1.2.3 --allow-move
                                                # move v1.2.3 back one commit (force-push)
bump-semver vcs tag push --rev main v1.2.3 --remote upstream
                                                # tag main, push to a non-default remote
bump-semver vcs tag delete v0.9.0               # remove from local + origin (idempotent)
```

Exit codes for `vcs tag push`: `0` success (incl. idempotent same-rev re-push); `2` usage (NAME / `--rev` missing, NAME with bad shape, `--force` passed, extra positional); `3` VCS subprocess error (bad REV, unknown remote, network); `4` integrity violation (existing tag at different REV without `--allow-move`). For `vcs tag delete`: `0` success or already-absent; `2` usage; `3` VCS error.

`--vcs jj|git|auto` still applies, so `bump-semver vcs get backend --vcs git` (or `vcs is git --vcs git`) forces the git branch on a colocated repo.

### Flags

| Flag | Description |
|---|---|
| `--pre PRE`            | Set pre-release identifiers (e.g. `--pre rc.0`) |
| `--no-pre`             | Remove pre-release identifiers |
| `--build-metadata META`| Set build metadata identifiers (e.g. `--build-metadata sha.abc`) |
| `--no-build-metadata`  | Remove build metadata identifiers |
| `--write`              | Write the bumped version back to each FILE input (`major` / `minor` / `patch` / `pre` only) |
| `--vcs jj\|git\|auto`    | Force VCS detection for `vcs:` inputs (default: `auto`) |
| `--no-hint`            | Suppress all `hint:` lines (fallback match / unsupported file / "files not modified") |
| `-q`, `--quiet`        | Suppress stdout (and all `hint:` lines) |
| `-qq`, `--quiet-all`   | Suppress stdout, hints, and error output (use with caution when debugging) |
| `--json`               | Output structured JSON for `get` / `major` / `minor` / `patch` / `pre` (rejected with `compare`) |
| `--version`, `-V`      | Print the binary version |
| `--help`, `-h`         | Short help (one screen) |
| `--help-full`          | Full reference (Supported file formats / all Examples / Exit codes / etc.) |
| `<action> --help`      | Action-specific reference (`bump-semver patch --help` / `compare --help` / etc.) |

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
| `vcs:latest-tag([REPO])` | Read the largest semver-compatible tag. `REPO` = `owner/repo` short form or full URL queries a remote (`git ls-remote --tags`); omit for cwd VCS |
| `cmd:<shell-command>` | Run `<shell-command>` via `bash -c`, take the first non-empty stdout line as VER (read-only, see [cmd: input](#cmd-input)) |

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
| **3** | `Cargo.toml` | TOML | `[package].version` (try) → `[workspace.package].version` | `[package].name` (try) → `[workspace.package].name` |
| **3** | `pyproject.toml` | TOML | `[project].version` (try) → `[tool.poetry].version` | `[project].name` (try) → `[tool.poetry].name` |
| **3** | `mojoproject.toml` | TOML | `[workspace].version` | `[workspace].name` |
| **3** | `project.pbxproj` (Xcode) | pbxproj | every `MARKETING_VERSION = ...;` (synced) | — |
| **3** | `Info.plist` (Apple plist) | xml | `<key>CFBundleShortVersionString</key>` | — |
| **3** | `pom.xml` (Maven) [DR-0018] | xml-element | `/project/version` | `/project/artifactId` |
| **3** | `VERSION` | plain text | (file content) | — |
| **2** (basename) | any `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | any `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | regex | `version := "..."` | — |
| **2** | `build.gradle` (Gradle Groovy) [DR-0018] | regex | `version = '...'` / `version "..."` | — |
| **2** | `build.gradle.kts` (Gradle Kotlin DSL) [DR-0018] | regex | `version = "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.cabal` (Haskell) [DR-0018] | regex | `version: ...` (line-anchored) | `name: ...` |
| **1** (fallback) | `*.spec` (RPM) [DR-0018] | regex | `Version: ...` (capital V) | `Name: ...` |
| **1** (fallback) | `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) [DR-0018] | xml-element | `/Project/PropertyGroup/Version` | — |

Unsupported files (e.g. `README.md`, `Cargo.lock`) error out explicitly with `unsupported file: <path>`. Adding a new format = adding one row to the rule table plus, if needed, one new format-specific function (no `--pattern` regex flag, by design).

YAML / TOML fallbacks (DR-0011) only look at top-level keys: a `version` nested inside a section / mapping is intentionally not picked up. For `Cargo.toml` / `pyproject.toml` / `mojoproject.toml` the explicit confidence-3 rules still win (so their existing section-scoped behaviour is unchanged). Multi-document YAML (`---` separators) reads only the first document. The same DR-0010 fallback hint fires for these new rules — with `--no-hint` to suppress.

The `pyproject.toml` rule (DR-0014) tries PEP 621's `[project].version` first and falls back to Poetry-legacy `[tool.poetry].version` so a single rule covers both ecosystems mid-migration. When a file carries both sections (theoretical mid-migration state), only the first match (PEP 621) is rewritten. The `mojoproject.toml` rule (DR-0014) reads / writes `[workspace].version` directly. Both rules use the same TOML section-scoped rewriter, so quote style and surrounding sections / comments stay intact.

The `Cargo.toml` rule (DR-0021) uses the same try-fallback shape: a single-crate manifest's `[package].version` is tried first, and a workspace-root manifest (no `[package]`) falls back to `[workspace.package].version` — the value member crates inherit via `version.workspace = true`. When a member crate declares both, its own `[package].version` wins. The matched path (`[package].version` or `[workspace.package].version`) is shown in `get` / `--json` output so you always see which version you are bumping.

The DR-0012 `regex` format covers eight language manifests whose version is a single line of source code (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt). Only the **first match** is read or rewritten; quote style and trailing comments on the version line are preserved verbatim.

The DR-0015 rules add the two Xcode-specific files where multiple version strings need synchronised updates: `project.pbxproj` (Xcode iOS / macOS project bundle, OpenStep plist) reads / rewrites **every** `MARKETING_VERSION = ...;` line at once and verifies they agree (a mismatched file surfaces a column-aligned `version mismatch:` block with `<file>:line:N` labels), and `Info.plist` (XML plist) reads / rewrites the `<key>CFBundleShortVersionString</key><string>...</string>` pair while preserving DOCTYPE, indentation, attribute order, and sibling keys byte-for-byte. Files using the Xcode 11+ `<string>$(MARKETING_VERSION)</string>` placeholder produce an `unsupported file:` outcome — the placeholder isn't a parseable version, which is the cue to add `project.pbxproj` to the invocation where the real value lives. `CFBundleVersion` (build number) is intentionally out of scope (build numbers aren't SemVer; CI typically writes them).

#### Suffix-stripped fallback (DR-0013)

When a path doesn't match any rule directly, `bump-semver` strips one trailing **backup-style suffix** from the basename and retries the rule table. The chosen rule's reported confidence is downgraded one band (3→2, 2→1, 1→1) and a `hint:` line is emitted to stderr so the resolution stays transparent. Multi-stage suffixes (`Cargo.toml.bak.20260510`) strip **only the trailing segment**; recursion is intentionally not applied.

| Suffix | Example | Resolved as |
|---|---|---|
| `.bak` / `.backup` / `.orig` / `.tmp` / `.old` | `Cargo.toml.bak` | `Cargo.toml` rule (confidence 2) |
| `.YYYYMMDD` (8 digits) | `package.json.20260510` | `package.json` rule (confidence 2) |
| `.YYYYMMDD_HHMMSS` (8+`_`+6 digits) | `Chart.yaml.20260510_120000` | `*.yaml` fallback (confidence 1) |
| trailing `~` (Emacs / vi) | `Cargo.toml~` | `Cargo.toml` rule (confidence 2) |

```bash
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

Template-style suffixes (`.template` / `.example` / `.sample` / `.dist`) are **not** stripped — their content is usually a placeholder, so silently treating them as real manifests would be more dangerous than the existing `unsupported file:` error. If you need to read a template, copy it under a backup-style name (`cp Cargo.toml.template Cargo.toml.tmp`).

The suffix hint shares the existing `hint:` prefix and is suppressed by `--no-hint` / `-q` / `-qq` exactly like the DR-0010 fallback hint. When both fire (e.g. `unknown.json.bak` → strip `.bak` → `*.json` glob), the suffix hint is emitted first.

For npm `package-lock.json` specifically, lockfile v1 (npm 5/6) is rejected with `unsupported lockfileVersion: 1, please regenerate with npm 7+`. Dependency entries (`$.packages["node_modules/..."]`) are never rewritten even if their version happens to equal the project's own.

### Multiple INPUTs: cross-input consistency

Pass multiple INPUTs to operate on them as a single unit. Versions across all INPUTs must already agree; otherwise a `version mismatch:` listing of every origin and value (column-aligned) is printed and the command fails. For `get` that failure is exit 1 with the listing on stderr (predicate-false semantics, [DR-0023](./docs/decisions/DR-0023-n-arg-extension.md)); for bump actions (`major` / `minor` / `patch` / `pre`) it is exit 2 because the input set is internally inconsistent. Detected package names are also cross-checked when available, to guard against accidentally bumping files from a different project together; names are never written back.

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
bump-semver patch 1.2.3 a.json b.json --write   # use VER as the "expected current value" for consistency, write bumped result to a/b
```

`get` with multiple INPUTs works as a CI-friendly consistency check (no `--write` needed, just verifies that all detected version fields agree). A file-omitted `vcs:REV` peer-expands across every sibling FILE path, so `get a b vcs:main@origin` is a four-way check (`a`, `b`, `vcs:main@origin:a`, `vcs:main@origin:b`).

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

# v0.15.0+ — Query a different repo's latest release tag
bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'        # owner/repo short form
bump-semver get 'vcs:latest-tag(https://github.com/x/y)' # full URL
bump-semver compare ge 0.0.13 'vcs:latest-tag(kawaz/pkf-tasks)'  # current pin up-to-date?
```

| Form | Meaning |
|---|---|
| `vcs:REV[:FILE]` | Read FILE at `<REV>` from the VCS. The first `:` is consumed by the `vcs:` prefix; the second `:` (if present) splits REV from FILE. Omitted FILE is borrowed from the first sibling input (FILE-origin or another `vcs:REV:FILE`) in argv order |
| `vcs:latest-tag()` | All tags from the cwd VCS; semver-incompatible tags are silently dropped; the largest by SemVer 2.0.0 ordering wins. Errors with `no semver-compatible tags found` if the candidate set is empty |
| `vcs:latest-tag(<arg>)` | v0.15.0+. `<arg>` = `owner/repo` (GitHub short, expanded to `https://github.com/...`) or full HTTPS/SSH URL. Queries the remote via `git ls-remote --tags`; jj/git auto-detection is irrelevant for remote queries. Monorepo-style tags like `pkf-tasks@0.0.13` are recognised (`@` peel fallback) so the same call works against multi-package repos. The argument is a **raw string** — no inner quotes needed (think markdown link `[]()`). **Trust boundary**: the validity of the URL is the caller's responsibility; pointing at an untrusted repo lets attackers publish `malicious@99.99.99` and have it returned as the largest tag (DR-0019) |

**VCS detection** (in priority order):

1. `--vcs jj|git` flag (`auto` and the unset case fall through)
2. `.jj` directory exists in the working dir or any ancestor → jj
3. `.git` directory exists → git
4. Otherwise → error (`not a git or jj repository`)

When both `.jj` and `.git` exist (jj's colocate mode, or kawaz's git-bare + jj-workspace layout), **jj wins** — its revset language is a superset of git's.

> Earlier versions (≤ v0.12) inserted a `BUMP_SEMVER_VCS=jj|git` environment variable between the flag and the probes; that knob was removed in v0.13 ([DR-0016](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md)). If a CI / dev environment previously relied on the env var, replace it with the `--vcs jj|git` flag.

**`--write` is incompatible with `vcs:` inputs.** vcs: is read-only by design (writing back into the VCS would require commit/amend semantics, which is out of scope). Combining the two errors out with `--write cannot be used with vcs: inputs (vcs: is read-only)`.

**`bump-semver` does not run `git fetch` / `jj git fetch` automatically.** If `vcs:origin/main` is stale, the underlying VCS error is surfaced verbatim. CI users should fetch explicitly before invoking `bump-semver`.

For CI scripts that need to be VCS-agnostic, prefer revisions that work in both flavours: `origin/main` (auto-translated to `main@origin` for jj), commit hashes, and `latest-tag()`.

### cmd: input

`cmd:<shell-command>` runs `<shell-command>` via `bash -c` and takes the first non-empty stdout line as VER (read-only, v0.16.0+). A leading `v` is stripped, and the value is parsed as SemVer 2.0.0.

```bash
# Does the built binary's --version match the VERSION file?
bump-semver compare eq VERSION 'cmd:./bin/mytool --version'

# Read a version that lives outside vcs:latest-tag's reach
bump-semver get 'cmd:brew info --json mytool | jq -r .[0].installed[0].version'

# Compare against another bump-semver invocation
bump-semver compare gt 'cmd:bump-semver get Cargo.toml' 'vcs:latest-tag()'
```

| Form | Interpretation |
|---|---|
| `cmd:<shell-command>` | Executes `<shell-command>` via `bash -c`. The first non-empty stdout line is taken as VER (leading `v` stripped, parsed as SemVer). Non-zero exit, empty stdout, or parse failure surface as errors (child stderr is included). **Read-only**: rejected by `--write` (same as `vcs:`) |

**`--write` and `cmd:` are mutually exclusive** (same as `vcs:`). There is no notion of writing back to a command's output.

**Trust boundary**: an arbitrary shell command is executed. Callers in CI / automation must assemble the command string safely — never `concat` untrusted input (env vars, argv) into a `cmd:` value.

The primary driver is kawaz/pkf-tasks v3.0's `semver/versions.pkl` (release-time gate), where version files and the built binary's `--version` output need to be cross-checked through a single `bump-semver get` invocation.

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

- `0` — success / predicate true (`compare`, `vcs is`)
- `1` — predicate false (`compare`, `vcs is` — silent on stderr)
- `2` — error (parse failure, mismatch, unsupported file, exclusivity violation, IO error, unknown verb/key for `vcs`, etc.)
- `3` — VCS subprocess error (`vcs` subcommands only: not in a repo, git/jj invocation failed)
- `4` — ambiguous answer (`vcs` subcommands only: detached HEAD, multiple bookmarks at the same head)
- `5` — non-fast-forward push (`vcs push` only; remote has diverged — fetch + reconcile, then retry)

## Migrating from v0.4.x

v0.5.0 ships three breaking changes. See [UPGRADING.md](./UPGRADING.md) for full details and rewrite examples:

1. **`--value` flag removed** → pass the VER directly as a positional argument (`bump-semver patch 1.2.3`)
2. **Body separator `-` removed** → use `.` or `_` instead (`1-2-3` is no longer accepted)
3. **Bump-path error exit code 1 → 2** (unified with the compare convention)

## Status

v0.16.1 hardens the `cmd:` input mode — `--write` + `cmd:` is now rejected by the implementation (the README already documented this, but v0.16.0 only enforced the rule for `vcs:` and silently let `cmd:` slip through), plus a 30-second hard timeout on the child process and 64 KiB / 4 KiB output caps on stdout / stderr to defend against runaway commands. Whitespace-only commands (`cmd:   `) are now rejected by the same non-empty check as `cmd:`. v0.16.0 adds the `cmd:<shell-command>` input mode — a read-only input that runs the command via `bash -c`, takes its first non-empty stdout line, strips a leading `v`, and parses the rest as SemVer. The primary use case is gating releases on agreement between version files and the built binary's `--version` output (e.g. `compare eq VERSION 'cmd:./bin/mytool --version'`). It also underpins kawaz/pkf-tasks v3.0's `semver/versions.pkl`. v0.14.0 adds JVM / .NET / Maven / Haskell / RPM support and a new `xml-element` format (DR-0018) — `pom.xml`, `*.csproj` / `*.fsproj` / `*.vbproj`, `build.gradle` / `build.gradle.kts`, `*.cabal`, `*.spec` all become recognised. `pom.xml` uses slash-rooted XML path lookup (`/project/version`) that correctly skips `<parent>/<version>`. v0.13.0 brings three changes: the help system is restructured into three tiers (`--help` short / `--help-full` complete reference / `bump-semver <action> --help` action-specific), the `BUMP_SEMVER_VCS` env var is removed in favour of `--vcs jj|git|auto` (DR-0016, BREAKING — see UPGRADING.md), and `compare` gains 15 precision-suffix operators (`eq-major` / `lt-minor` / `eq-patch` etc., DR-0017) for a 5×4 = 20 total. v0.12.0 added two Xcode-specific path-pinned rules — `project.pbxproj` (multi-match `MARKETING_VERSION` synced across build configurations) and `Info.plist` (XML plist with byte-range value rewriting) — together with `pbxproj` and `xml` formats (DR-0015). v0.11.0 generalised the TOML rewriter into a reusable section-scoped helper and added `pyproject.toml` (PEP 621 with Poetry-legacy fallback) and `mojoproject.toml` (DR-0014). v0.10.0 added the suffix-stripped fallback for backup-style filenames (DR-0013). v0.9.0 introduced the `regex` format with eight new file types (`*.xcconfig`, `*.podspec`, `*.nimble`, `v.mod`, `build.zig.zon`, `*.gemspec`, `mix.exs`, `build.sbt`) (DR-0012). v0.8.0 added `*.yaml` / `*.yml` / `*.toml` confidence-1 fallback (DR-0011). v0.7.0 added the `vcs:` input mode — `vcs:REV[:FILE]` and `vcs:latest-tag()` resolve through jj or git automatically (DR-0008). Earlier: v0.6.0 added `--json` output (DR-0007); v0.5.0 introduced pre-release / build metadata support, the `compare` subcommand, the `pre` action, and the unified FILE/VER positional input (DR-0006). Future formats are added one handler at a time (DR-0001). For design rationale see [docs/decisions/](./docs/decisions/); for upcoming items see [docs/ROADMAP.md](./docs/ROADMAP.md).

## License

[MIT](LICENSE)
