# Changelog

All notable changes to bump-semver are recorded here, newest first. Entries are summarised per feature rather than per commit. For the design rationale behind each change see [docs/decisions/](./docs/decisions/).

The format loosely follows [Keep a Changelog](https://keepachangelog.com/); patch-only releases between the milestones listed below are omitted.

## v0.48.0

- Added global `-C, --cwd PATH` ([DR-0043](./docs/decisions/DR-0043-global-cwd-option.md)) — `os.Chdir(PATH)` before anything else runs, collapsing the `(cd PATH && bump-semver ...)` sub-shell pattern into a single invocation (git's `-C` semantics). `-C PATH` / `--cwd PATH` / `--cwd=PATH` are all accepted, may appear anywhere in argv (before or after the subcommand), and are extracted in a pre-cobra-parse pass so every downstream input/vcs/glob resolution observes the new directory. Once-only like every other value flag in the project; a second occurrence or a missing value is an exit-2 usage error.

## v0.47.0

- Added `vcs get repository` / `vcs get repository-url` ([DR-0041](./docs/decisions/DR-0041-vcs-get-repository.md)) — remote 由来のリポジトリ識別子 getter。`root` はローカルパスを返すため linked worktree / named workspace ではディレクトリ名がリポジトリ名として誤用されていた (issue 2026-07-11); remote URL は worktree/workspace 間で共有されるので backend 分岐なしに一貫した値が取れる。`repository` は `owner/repo` slug (GitLab subgroup も全セグメント保持、2 セグメント決め打ちなし)、`repository-url` は https 正規形。新 `--remote NAME` フラグ (デフォルト `origin`、不在時は remote が丁度 1 個ならそれを採用、0 個/複数は exit 4) 付き。scp 風 / `ssh://` / `git://` / `http(s)://` を受理し user info 除去・port 保持・`.git`/末尾 `/` 除去で正規化、ローカルパス remote は exit 3 (`git remote get-url` への誘導メッセージ付き)。

## v0.46.0

- **BREAKING**: `vcs get commit-id` のデフォルト rev (DR-0040) を backend-agnostic 化。jj backend は mutable working copy `@` の SHA を返していたが、git backend の `HEAD` (最後に固定されたコミット) と概念が食い違っていた (agnostic API を謳いながら backend ごとに別概念を返す設計バグ)。jj backend のデフォルトを `heads((::@-) & (~empty() | merges()))` (`@-` を起点に、空コミットを skip しつつ空マージは救済した最新の固定コミット) に変更、git backend は不変。旧挙動 (working copy `@` 自身の SHA) が必要な場合は `--rev @` を明示指定する。gh-monitor の push 後 CI watch (SHA pin) で `@` が空コミットのとき無関係な SHA を watch してしまう退行から発覚。

## v0.45.0

- `vcs promote`: non-fast-forward 拒否時の error message に sync 推奨 hint を追加。jj backend は sideways move (= default が祖先でも子孫でもない) も既に exit 5 で reject していたが、jj 自身の hint (`--allow-backwards`) を bump-semver の canonical recovery path (`vcs sync --onto <default>@origin`) に置き換え。git backend も既存の merge-base ancestor check に同じ hint を組み込み。`vcs promote` の祖先 guard 自体は既存仕様で機能していた (DR-0038 dogfood 観察)、本 release は文言改善に閉じる。
- Test hygiene cleanup: `cmd_vcs_outdated_test.go` の bare `t.Skip()` 25 箇所を file 内 convention に揃えて `t.Skip("git not installed")` / `t.Skip("git+jj fixture requires both binaries")` に統一。`cmd_source_leak_test.go` の darwin polling-with-sleep idiom (= 非子プロセス PID-liveness の検出) に design rationale コメントを追記し、`default-convergence-guard` の polling 安易採用禁則と混同されないよう明示化。test-failure-no-tampering rule 改訂版に対する全 *_test.go audit (= 59 ファイル) の結果、改変パターン (入力書換 / assert 緩和 / 観測根拠なし timeout 延長 / 暗黙 skip / 削除) 該当 0 件、本 release は hygiene drift 限定の正規化に閉じる。

## v0.44.0

- Documented `-qq` as a formal shorthand for `--quiet-all` in the verbosity flag help (= the rewrite from the `-qq` token to `--quiet-all` already existed in `normalizeQuietAll`; only the help text mentioned nothing). Reported by the kawaz/die / kawaz/grapheme.mbt sessions: users were copy-pasting `-qq` from canonical repos without knowing it silences errors too, risking silent CI failures.
- Re-shuffled the `--help-full` "Supported file formats" listing so manifests with bespoke project semantics (`build.zig.zon`, `v.mod`, `mix.exs`, `build.sbt`) appear on their own line with the actual field they extract, instead of being collapsed into a `text + regex (basename)` group line. The collapsed format was reading as "not first-class" to consumers of canonical references (= kawaz/die feedback during dogfood).

## v0.43.0

- Hardened the release.yml semver gate to `latest-release` + `latest-tag` parallel check (DR-0039). `gh release create` does not push the git tag to origin (cli/cli#4357), so `vcs get latest-tag` alone can stale-read and let a downgrade slip through. The new pattern evaluates both axes without short-circuiting and fails the workflow if either gate is not strictly greater than the current VERSION. Verified against kawaz/die's downgrade incident (v0.0.2 climbing over v0.1.x via `--latest=automatic`'s date-priority); same fix template was filed as an issue against 10 other kawaz repos for canonical sync.
- Added per-binary `.sha256` sidecar attachment to the release job (one `<asset>.sha256` per binary, GNU coreutils `sha256sum` format). Users can verify a single platform binary with `sha256sum -c bump-semver-linux-amd64.sha256` without downloading a separate manifest. Convention follows ripgrep / starship / zellij (Rust 系 canonical); justified over a single `SHA256SUMS` because most users only verify the one binary for their platform.

## v0.42.0

- Added `vcs bookmark set NAME [-r/--rev REV] [--allow-backwards]` for the `just push-wip` path: create-or-move a branch (git) / bookmark (jj) explicitly named by the caller. Defaults to HEAD (git) / `@` (jj); fast-forward-only by default, with `--allow-backwards` for recovery / rewind cases. Idempotent same-rev sets are exit 0. Non-FF without `--allow-backwards` is exit 5 (mirrors `vcs promote`). git path bypasses `receive.denyCurrentBranch` via `update-ref` (same trick as `vcs promote`) so it works across linked worktrees.
- Added `vcs get default-branch-path` — returns the absolute path of the worktree (git) / workspace (jj) that currently has the default branch checked out. Tie-break: when multiple worktrees match, the one whose dir basename (git) / workspace name (jj) equals the default branch name wins; otherwise exit 5. No matching worktree → exit 4. Symmetric counterpart to `vcs get default-branch` for callers that need the canonical worktree path (e.g. `cd "$(bump-semver vcs get default-branch-path)" && just bump-version`).

## v0.41.0

- Added MoonBit module manifest auto-detect: `moon.mod` (new DSL format, text+regex) and `moon.mod.json` (legacy JSON, `$.version` / `$.name`). Verified against `moon 0.1.20260618` fresh `moon new` output and against in-tree `~/.moon/lib/core/moon.mod`. The DSL regex picks only top-level `^\s*version = "..."` so the bespoke `import { ... }` / `options( ... )` blocks pass through untouched.

## v0.40.1

- Fix: `vcs promote` の bare 実行 (= 引数なし、`--help` なし) が help を表示するだけで実際の bookmark 移動が走らなかった bug を修正。v0.40.0 dogfood で発覚。
- Docs (DR-0038): justfile push gate の正しい predicate を **`vcs is on-default-branch` の反転** に確定。`vcs is worktree` ベースだと kawaz の jj 運用 (= main workspace は secondary workspace) で main からの push が誤検出される盲点を Adoption pattern セクションに記録。

## v0.40.0

- Added worktree / workspace awareness and default-branch promotion to the `vcs` family:
  - `vcs is worktree` — true inside a linked worktree (git) / secondary workspace (jj).
  - `vcs is on-default-branch` — true when the current branch/bookmark equals the default branch.
  - `vcs get worktree-name` — the linked-worktree / secondary-workspace name; empty on the main worktree.
  - `vcs get default-branch` — the canonical default branch (main / master / trunk), resolved from `refs/remotes/origin/HEAD` with a local fallback.
  - `vcs promote` — move the default branch / bookmark forward to the current commit (no push; non-FF surfaces exit 5). git uses `update-ref` with an ancestor check so the move works even when another worktree has the default branch checked out; jj uses `jj bookmark set -r @-`.
  - `vcs sync --onto REF` — rebase the current worktree / workspace onto REF (git: `git rebase`; jj: `jj rebase -d`).
- Enables justfile `push` gates that detect "still in a worktree" and hint the user toward `vcs sync` → `vcs promote` → `vcs push`.

## v0.39.0

- **BREAKING**: `vcs commit -m MSG PATH..` のデフォルト挙動を反転。削除された tracked path も commit に含まれるようになった (旧: `os.Stat` でフィルタして黙殺、新: `git add -A` / jj fileset 経由で削除を透過)。詳細 [DR-0037](./docs/decisions/DR-0037-vcs-commit-default-include-deletes.md)
- 旧挙動 (複数候補から存在するものを pick して commit) が必要な場合は新フラグ `--allow-nonexistent-path` を追加する。単一 file 固定型の利用 (`Cargo.toml Cargo.lock`、`VERSION` 等、常時存在する file 列挙) は影響なし。

## v0.36.0

- Migrated the hand-written argument parser and help text (~2300 lines) to [spf13/cobra](https://github.com/spf13/cobra). The `Options` section of every help screen is now generated from the cobra FlagSet, and help text is unified in English.
- Added the `completion` subcommand (`bash` / `zsh` / `fish` / `powershell`, generated by cobra).

## v0.35.0

- **Security**: external-command argument-injection hardening ([DR-0034](./docs/decisions/DR-0034-arg-injection-trust-boundary.md)). User-supplied rev / tag NAME / remote / repository values that start with `-` are rejected at the CLI dispatch and input-mode resolver boundary, so they can no longer be reinterpreted as flags by `git` / `jj` / `gh`.
- TOML replacement now verifies the rewrite result, process-group kill on child commands, and `jj` added to CI.

## v0.33.0

- `vcs diff` gained a repeatable `--excludes PATTERN` flag (post-filter, order-independent set subtraction `include ∖ exclude`), forwarded to the backend as a pathspec so deletions are still detected ([DR-0033](./docs/decisions/DR-0033-vcs-excludes-and-file-prefix.md)).
- Added the `file:<path>` input prefix: one path per line, literal or `glob:`, with `#` comments and blank lines skipped. Nested `file:` is rejected. Used by `vcs diff --excludes` and `vcs outdated`.

## v0.32.0

- Split `vcs tag latest` into two source-named verbs, `vcs get latest-tag` and `vcs get latest-release` ([DR-0032](./docs/decisions/DR-0032-vcs-get-latest-by-source-verb.md)). Both share the same `--json` schema as `get --json`.
- Revived the input record `vcs:latest-tag([REPO])` and added `vcs:latest-release([REPO])` (the latter requires `gh`).
- Generalised rev translation into a shared `translateRev` foundation used by every rev entry point (`vcs:` and the `vcs` subcommands), enabling `origin/main` ↔ `main@origin` translation across backends ([DR-0031](./docs/decisions/DR-0031-translate-rev-common-foundation.md)).

## v0.31.0

- **`vcs outdated FROM TO[..]`**: derived-freshness checks using a glob-backref mapping mini-DSL ([DR-0027](./docs/decisions/DR-0027-derived-sync-mini-dsl-and-regex-reject.md), [DR-0028](./docs/decisions/DR-0028-glob-backref-spec-v0.1.0-adoption.md)). FROM is a literal path or `glob:<pat>`; TO supports `$N` / `${N}` backrefs, `{a,b,c}` mandatory expansion, and `*` / `**` / `[]` optional filesystem discovery. `--` separates pairs, `--explain` prints the expansion (always exits 0), `--strict` tightens matching. Adopts the language-independent glob-backref spec v0.1.0 ([docs/specs/glob-backref-v0.1.0.md](./docs/specs/glob-backref-v0.1.0.md)).
- **`--define-rule`**: CLI-defined extraction rules ([DR-0029](./docs/decisions/DR-0029-cli-user-defined-rule-phase1.md)). `--define-rule <PATTERN>` opens a rule block; the following `--format` / `--version-path` / `--version-regex` / `--name-path` / `--name-regex` flags belong to that block until the next `--define-rule`. CLI rules always override builtins, and extraction failure is a hard error (no silent fall-through). Available for `get` / `compare` / bump verbs (including `--write`).
- Unified `format=regex` and `format=plain` into `format=text + VersionRegex`; the public format enum is now `text|json|yaml|toml|xml|xml-element` ([DR-0030](./docs/decisions/DR-0030-format-regex-to-text-unification.md)). Internal refactor only — no CLI surface change.

## v0.30.0

- `--jj-bookmark-auto-advance` now requires the target change to have a description, failing early with a `jj describe` hint instead of looping on a rejected push ([DR-0025](./docs/decisions/DR-0025-auto-advance-description-check.md)).
- `autoAdvanceBookmark` delegates to the official `jj bookmark advance` (jj 0.39+), keeping only the clean-state at-`@` short-circuit and the DR-0025 description check on the outside ([DR-0026](./docs/decisions/DR-0026-auto-advance-delegate-to-jj.md)).

## v0.28.0

- Added the `glob:<pattern>` input mode ([DR-0024](./docs/decisions/DR-0024-glob-prefix.md)) to absorb shell-glob variance when task runners pass multiple arguments. Supports `*` / `**` / `[]` / `{}` / `~` plus three `--glob-*` flags, with no-match silent skip. `glob:` is a SOURCE / PATH-list selector, not a version input prefix.

## v0.21.0

- Cargo workspace support: `[package].version` falls back to `[workspace.package].version` ([DR-0021](./docs/decisions/DR-0021-cargo-workspace-package-version.md), superseding DR-0002).

## v0.20.0

- `get` and `compare` accept N inputs ([DR-0023](./docs/decisions/DR-0023-n-arg-extension.md)). `compare` treats the first positional as BASE and checks every OTHER independently; `get` treats all inputs as equal peers. The legacy two-input `compare` form is the N=1 case.

## v0.17.0

- **`vcs` subcommands**: a git/jj-agnostic helper subtree for release and push operations ([DR-0020](./docs/decisions/DR-0020-vcs-subcommands.md)).
  - `vcs get root|backend|current-branch|commit-id` — single VCS facts on stdout.
  - `vcs is clean|dirty|git|jj` — predicates (the exit code is the answer).
  - `vcs diff REV [PATH..]` — patch vs working copy, with `-s` name-status and `-q` exit-code mode.
  - `vcs commit -m MSG PATH.. | --staged | --amend` — idempotent no-op commits; `-a` / `--all` is rejected as a safety rail.
  - `vcs fetch [REMOTE]` — default `origin`.
  - `vcs push --branch/--bookmark NAME` — non-fast-forward yields exit 5; `--jj-bookmark-auto-advance` is jj-only.
  - `vcs tag push --rev REV NAME` — create + push as one step; same rev is idempotent, a different rev needs `--allow-move`.
  - `vcs tag delete NAME` — removes local + remote, idempotent.
- Re-adopted `justfile` as the canonical task runner (was Taskfile.pkl + pkfire), dogfooding `bump-semver vcs` ([DR-0022](./docs/decisions/DR-0022-justfile-re-adoption.md)).

## v0.16.1

- Hardened the `cmd:` input mode: `--write` + `cmd:` is now rejected by the implementation (previously enforced only for `vcs:`), plus a 30-second hard timeout on the child process and 64 KiB / 4 KiB output caps on stdout / stderr. Whitespace-only commands (`cmd:   `) are rejected by the same non-empty check.

## v0.16.0

- Added the `cmd:<shell-command>` input mode — a read-only input that runs the command via `bash -c`, takes its first non-empty stdout line, strips a leading `v`, and parses the rest as SemVer. The primary use case is gating releases on agreement between version files and a built binary's `--version` output (e.g. `compare eq VERSION 'cmd:./bin/mytool --version'`).

## v0.15.0

- `vcs:latest-tag(<arg>)` reads another repository's latest tag, with a monorepo-style `<name>@<version>` `@`-peel fallback ([DR-0019](./docs/decisions/DR-0019-vcs-latest-tag-remote-arg.md)).

## v0.14.0

- JVM / .NET / Maven / Haskell / RPM support and the new `xml-element` format ([DR-0018](./docs/decisions/DR-0018-jvm-dotnet-haskell-rpm-support.md)) — `pom.xml`, `*.csproj` / `*.fsproj` / `*.vbproj`, `build.gradle` / `build.gradle.kts`, `*.cabal`, `*.spec` become recognised. `pom.xml` uses slash-rooted XML path lookup (`/project/version`) that correctly skips `<parent>/<version>`.

## v0.13.0

- Restructured help into three tiers (`--help` short / `--help-full` complete reference / `bump-semver <action> --help` action-specific).
- Removed the `BUMP_SEMVER_VCS` env var in favour of `--vcs jj|git|auto` ([DR-0016](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md), **BREAKING**).
- `compare` gained 15 precision-suffix operators (`eq-major` / `lt-minor` / `eq-patch` etc., [DR-0017](./docs/decisions/DR-0017-compare-precision-suffix.md)) for a 5 × 4 = 20 total.

## v0.12.0

- Two Xcode-specific path-pinned rules — `project.pbxproj` (multi-match `MARKETING_VERSION` synced across build configurations) and `Info.plist` (XML plist with byte-range value rewriting) — together with the `pbxproj` and `xml` formats ([DR-0015](./docs/decisions/DR-0015-pbxproj-and-info-plist.md)).

## v0.11.0

- Generalised the TOML rewriter into a reusable section-scoped helper and added `pyproject.toml` (PEP 621 with Poetry-legacy fallback) and `mojoproject.toml` ([DR-0014](./docs/decisions/DR-0014-toml-section-scoped.md)).

## v0.10.0

- Suffix-stripped fallback for backup-style filenames (`Cargo.toml.bak`, `package.json.20260510`, `Chart.yaml~`, etc.) ([DR-0013](./docs/decisions/DR-0013-suffix-stripped-format-detection.md)).

## v0.9.0

- Introduced the `regex` format with eight new file types (`*.xcconfig`, `*.podspec`, `*.nimble`, `v.mod`, `build.zig.zon`, `*.gemspec`, `mix.exs`, `build.sbt`) ([DR-0012](./docs/decisions/DR-0012-regex-format.md)). (Later folded into `format=text` in v0.31.0, DR-0030.)

## v0.8.0

- `*.yaml` / `*.yml` / `*.toml` confidence-1 fallback ([DR-0011](./docs/decisions/DR-0011-yaml-yml-toml-fallback.md)).

## v0.7.0

- Added the `vcs:` input mode — `vcs:REV[:FILE]` and `vcs:latest-tag()` resolve through jj or git automatically ([DR-0008](./docs/decisions/DR-0008-vcs-input.md)).

## v0.6.0

- `--json` structured output ([DR-0007](./docs/decisions/DR-0007-json-output-option.md)).

## v0.5.0

- Pre-release / build metadata support, the `compare` subcommand, the `pre` action, and the unified FILE/VER positional input ([DR-0006](./docs/decisions/DR-0006-pre-release-and-compare.md)).

## Earlier

- Flat actions (`major` / `minor` / `patch`) with basename-based format detection; new file formats are added one handler at a time ([DR-0001](./docs/decisions/DR-0001-flat-actions-and-format-detection.md)).
