# Decision Records (DR) Index

bump-semver の設計判断記録一覧。ファイル名は `DR-NNNN-title.md` (4 桁ゼロパディング)。`docs-structure.md` ルールに従い `## Active` / `## Archived` / `## Moved to research/` で区分する。

## Active

- [DR-0001](./DR-0001-flat-actions-and-format-detection.md) — flat 4-action CLI + basename ベースのファイル形式判定
- [DR-0003](./DR-0003-prefix-and-flexible-separator.md) — prefix (`v`/`ver`/`version`) と柔軟 separator (`. _ -`) を許容する
- [DR-0004](./DR-0004-multi-file-and-name-consistency.md) — 複数 FILE 一括 bump + name 整合性検証 + package-lock.json 特殊化
- [DR-0005](./DR-0005-path-aware-confidence-ranked-candidates.md) — basename 決め打ちから path-aware confidence ranked candidates へ
- [DR-0006](./DR-0006-pre-release-and-compare.md) — pre-release/build-metadata 対応 + compare サブコマンド + FILE\|VER 統合
- [DR-0007](./DR-0007-json-output-option.md) — `--json` 出力オプション (構造化 JSON、`get` / bump 系のみ)
- [DR-0008](./DR-0008-vcs-input.md) — `vcs:` 入力モード (jj/git の他リビジョン・最新 tag を入力として受け付け)
- [DR-0009](./DR-0009-lockfile-support-scope.md) — lock files の対応対象判断 (npm 以外は対象外、bun/pnpm/yarn/Cargo)
- [DR-0010](./DR-0010-fallback-match-hint.md) — confidence 1 fallback マッチ時の hint 出力 + unsupported file エラーの誘導文言
- [DR-0011](./DR-0011-yaml-yml-toml-fallback.md) — `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback 追加
- [DR-0012](./DR-0012-regex-format.md) — regex format 抽象 + xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt 対応
- [DR-0013](./DR-0013-suffix-stripped-format-detection.md) — 既知 suffix を剥がして既存ルールで再判定する fallback (`Cargo.toml.bak` / `package.json.20260510` / `Chart.yaml~` 等)
- [DR-0014](./DR-0014-toml-section-scoped.md) — TOML section-scoped Replace の一般化 + `pyproject.toml` (`[project]` / `[tool.poetry]` try-fallback) と `mojoproject.toml` (`[workspace]`) 対応
- [DR-0015](./DR-0015-pbxproj-and-info-plist.md) — Xcode `project.pbxproj` (multi-match 同期更新) と `Info.plist` (XML plist) 対応
- [DR-0016](./DR-0016-remove-bump-semver-vcs-env.md) — `BUMP_SEMVER_VCS` 環境変数廃止 + `--vcs auto` 一本化 (DR-0008 の env 部分を supersede)
- [DR-0017](./DR-0017-compare-precision-suffix.md) — `compare` の precision suffix 拡張 (`eq-major` / `lt-minor` 等、5 base × 4 precision = 20 OP)
- [DR-0018](./DR-0018-jvm-dotnet-haskell-rpm-support.md) — JVM (Gradle Groovy / Kotlin DSL) + .NET (`*.csproj` / `*.fsproj` / `*.vbproj`) + Maven (`pom.xml`) + Haskell (`*.cabal`) + RPM (`*.spec`) 対応。新 format `xml-element` 導入
- [DR-0019](./DR-0019-vcs-latest-tag-remote-arg.md) — `vcs:latest-tag(<arg>)` で他リポの最新 tag 取得対応 + monorepo-style tag (`<name>@<version>`) の `@` peel fallback
- [DR-0020](./DR-0020-vcs-subcommands.md) — `vcs` サブコマンド群 (git/jj 吸収のリリース/push 定型操作: get/is/diff/commit/push/tag)。未実装、ROADMAP 参照
- [DR-0021](./DR-0021-cargo-workspace-package-version.md) — Cargo workspace の `[workspace.package].version` 対応 (`[package]` → `[workspace.package]` の OR フォールバック)。DR-0002 を supersede
- [DR-0022](./DR-0022-justfile-re-adoption.md) — Justfile 回帰 (Taskfile.pkl + pkfire → Justfile + `bump-semver vcs` ドッグフード)。Taskfile.pkl は翻訳 check の shim としてのみ残存
- [DR-0023](./DR-0023-n-arg-extension.md) — `get` / `compare` の N 引数化 + `vcs:` borrowing の N 個展開 (verb 別責務分離: get 対等ピア / compare F1 基準)
- [DR-0024](./DR-0024-glob-prefix.md) — `glob:<pattern>` 入力モード (タスクランナー多段引数渡しでの shell glob ブレ吸収。`*` / `**` / `[]` / `{}` / `~` + `--glob-*` 三種フラグ + no-match silent-skip)
- [DR-0025](./DR-0025-auto-advance-description-check.md) — `--jj-bookmark-auto-advance` の description 必須 check (dirty branch で undescribed @ を target にする push reject ループを早期 fail + `jj describe` hint で抑止、判定は jj template engine 経由)
- [DR-0026](./DR-0026-auto-advance-delegate-to-jj.md) — `autoAdvanceBookmark` を jj 公式 `jj bookmark advance` (jj 0.39+) に委譲。existence/ancestor/at-target chain ~50 行を削除、clean 時の at-@ short-circuit と DR-0025 description check のみ外側に残す
- [DR-0027](./DR-0027-derived-sync-mini-dsl-and-regex-reject.md) — 派生 sync check の mapping DSL 採用 (`vcs outdated`) / `regex:` 不採用。`glob:` 拡張で 可変パーツが順に backref 登録 + `{}` 必須 / `*`/`**`/`[]` 任意 + `--` ペア区切り + 自動除外 + `--explain` 必須。MS-DOS COMMAND.COM / Unix mmv 系譜の現代的整理 (matching semantics / TO escape は DR-0028 で再設計、その他は維持)
- [DR-0028](./DR-0028-glob-backref-spec-v0.1.0-adoption.md) — glob-backref **言語非依存 spec v0.1.0** (`docs/specs/glob-backref-v0.1.0.md`) を `vcs outdated` の正本に採用。旧 v1 実装の 4 構造的問題 (leading-slash false-missing / `*`空文字 silent skip / TO 再 glob 解釈 / literal-FROM 不在 silent green) を根本対応。`{}` 直積全展開 + `**` 0-segment = `.` + `path.Clean` + char-class wrap escape + grammar drift panic + `--strict` flag。Extends DR-0024 / DR-0027 (matching semantics 部のみ partial supersede)

## Archived

- [DR-0002](./DR-0002-cargo-workspace-not-supported.md) — Cargo workspace の `[workspace.package].version` を MVP では扱わない (Superseded by DR-0021)

## Moved to research/

(なし)
