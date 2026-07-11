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
- [DR-0019](./DR-0019-vcs-latest-tag-remote-arg.md) — `vcs:latest-tag(<arg>)` で他リポの最新 tag 取得対応 + monorepo-style tag (`<name>@<version>`) の `@` peel fallback。Superseded by DR-0020 (= 一旦 subcommand 化で削除)、DR-0032 (= 入力 record 復活で部分再生)
- [DR-0020](./DR-0020-vcs-subcommands.md) — `vcs` サブコマンド群 (git/jj 吸収のリリース/push 定型操作: get/is/diff/commit/push/tag)。PR-Tag-Latest section は Superseded by DR-0032 (= `vcs tag latest` → `vcs get latest-{tag,release}` に再整理 + JSON schema 統一 + 入力 record 復活)
- [DR-0021](./DR-0021-cargo-workspace-package-version.md) — Cargo workspace の `[workspace.package].version` 対応 (`[package]` → `[workspace.package]` の OR フォールバック)。DR-0002 を supersede
- [DR-0022](./DR-0022-justfile-re-adoption.md) — Justfile 回帰 (Taskfile.pkl + pkfire → Justfile + `bump-semver vcs` ドッグフード)。Taskfile.pkl は翻訳 check の shim としてのみ残存
- [DR-0023](./DR-0023-n-arg-extension.md) — `get` / `compare` の N 引数化 + `vcs:` borrowing の N 個展開 (verb 別責務分離: get 対等ピア / compare F1 基準)
- [DR-0024](./DR-0024-glob-prefix.md) — `glob:<pattern>` 入力モード (タスクランナー多段引数渡しでの shell glob ブレ吸収。`*` / `**` / `[]` / `{}` / `~` + `--glob-*` 三種フラグ + no-match silent-skip)
- [DR-0025](./DR-0025-auto-advance-description-check.md) — `--jj-bookmark-auto-advance` の description 必須 check (dirty branch で undescribed @ を target にする push reject ループを早期 fail + `jj describe` hint で抑止、判定は jj template engine 経由)
- [DR-0026](./DR-0026-auto-advance-delegate-to-jj.md) — `autoAdvanceBookmark` を jj 公式 `jj bookmark advance` (jj 0.39+) に委譲。existence/ancestor/at-target chain ~50 行を削除、clean 時の at-@ short-circuit と DR-0025 description check のみ外側に残す
- [DR-0027](./DR-0027-derived-sync-mini-dsl-and-regex-reject.md) — 派生 sync check の mapping DSL 採用 (`vcs outdated`) / `regex:` 不採用。`glob:` 拡張で 可変パーツが順に backref 登録 + `{}` 必須 / `*`/`**`/`[]` 任意 + `--` ペア区切り + 自動除外 + `--explain` 必須。MS-DOS COMMAND.COM / Unix mmv 系譜の現代的整理 (matching semantics / TO escape は DR-0028 で再設計、その他は維持)
- [DR-0028](./DR-0028-glob-backref-spec-v0.1.0-adoption.md) — glob-backref **言語非依存 spec v0.1.0** (`docs/specs/glob-backref-v0.1.0.md`) を `vcs outdated` の正本に採用。旧 v1 実装の 4 構造的問題 (leading-slash false-missing / `*`空文字 silent skip / TO 再 glob 解釈 / literal-FROM 不在 silent green) を根本対応。`{}` 直積全展開 + `**` 0-segment = `.` + `path.Clean` + char-class wrap escape + grammar drift panic + `--strict` flag。Extends DR-0024 / DR-0027 (matching semantics 部のみ partial supersede)
- [DR-0029](./DR-0029-cli-user-defined-rule-phase1.md) — CLI から「自分のファイルにこの rule」を指定する口 (Phase 1)。`--define-rule <PATTERN>` ブロック方式 + match strength scoring (5: 絶対 / 3: 相対 / 2: basename / 1: glob / builtin fallback) + `--format <text|json|yaml|toml>` / `--version-path` / `--version-regex` / `--name-path` / `--name-regex`。get / compare / bump 全 verb で write も解禁、CLI rule は builtin より優先、extraction failure は hard error、`--version-regex` は exact one match (= builtin より厳格)、bump --write は atomic + path/regex 併用書き戻しアルゴリズム pin、name safety rail は warning hint で silent downgrade 防止。Prerequisite: DR-0030
- [DR-0030](./DR-0030-format-regex-to-text-unification.md) — `format=regex` 概念廃止 → `format=text + VersionRegex` 統合 (`format=plain` も text に統合、enum を `text|json|yaml|toml|xml|xml-element` に整理)。internal refactor のみで機能不変、CLI 表面の変更なし。DR-0012 partial supersede。DR-0029 の `--format` enum 公開の前提
- [DR-0031](./DR-0031-translate-rev-common-foundation.md) — rev 翻訳を共通基盤化 (`vcs:` と `vcs` サブコマンドの全 rev 受け口で利く `translateRev`)。altJjRev (FetchFile 専用フォールバック) を廃止して全 rev 受け口 (FetchFile / Diff / DiffNameStatus / resolveJjRev / resolveGitRev) の入口で 1 度通す形に統合。`origin/main` ↔ `main@origin` の双方向翻訳、backend 固有 syntax は pass-through。対称翻訳 (`@-`↔`HEAD^` 等) は v2 で再検討。default rev 節は Superseded by DR-0040
- [DR-0032](./DR-0032-vcs-get-latest-by-source-verb.md) — `vcs tag latest [--source <tag|release>]` を `vcs get latest-tag` / `vcs get latest-release` の 2 verb に分割 (= source 軸を verb 名に畳む、`--source` / `--raw` 廃止)。`--json` schema を `get --json` と同一の version schema に統一。入力 record `vcs:latest-tag([REPO])` 復活 + `vcs:latest-release([REPO])` 新設。DR-0020 PR-Tag-Latest を re-supersede、DR-0019 の入力 record を部分再生、DR-0031 (translateRev) とは独立 (= `expandRepoArg` / `translateRev` は別経路維持)
- [DR-0033](./DR-0033-vcs-excludes-and-file-prefix.md) — `vcs diff` (phase 1) に `--excludes PATTERN` flag を追加 (= repeatable + append、post-filter で順序非依存、include ∖ exclude の集合演算)。`file:<path>` 入力 prefix も land (= 1 行 1 path、literal / `glob:` 受け入れ、`#` コメント / 空行スキップ)。`!`-prefix shorthand は不採用 (= gitignore セマンティック反転 / bash history expansion 回避)。DR-0024 で scope-out した「exclude pattern」「file:LIST 将来案」をともに本 DR で land、release.yml dogfood で `_test.go` 除外を実現
- [DR-0034](./DR-0034-arg-injection-trust-boundary.md) — 外部コマンド引数インジェクション対策 (C-1)。ユーザ由来の rev / tag NAME / remote / repository が `-` 始まりだと git/jj/gh にフラグ解釈される問題を、CLI ディスパッチ + 入力モード resolver の入口に検証を集約して解消。URL スキーム allowlist は DR-0019/DR-0032 の設計継承で不採用、最小の `-` 始まり拒否に限定
- [DR-0035](./DR-0035-atomic-write-and-all-or-nothing.md) — `--write` の二相化 (全 file の Replace を先に計算 → all-or-nothing) + アトミック書き込み (同一ディレクトリ temp + rename) による manifest 破損防止 (C-2)。symlink は実体解決してから rename (symlink 維持 + 実体更新)、mode 保持。rollback 不採用 (DR-0004 §7 維持) はそのままに prevention を追加、§7 を部分 supersede。phase 2 後段失敗の部分書き込み窓は残余リスクとして許容
- [DR-0036](./DR-0036-package-split-deferred.md) — パッケージ分割 (`internal/` 化) は見送り、`vcs_backend.go` (2184 行) を同一パッケージ内でファイル 3 分割 (`_git` / `_jj` / 本体) するファイルレベル整理を採用。rules ⇄ format / resolve ⇄ vcs の双方向結合 + `exitErr` 全層染み出しにより別パッケージ化は即 import cycle + 共有型パッケージ新設 + 100 識別子超の export 改名 + テスト約 20 本書き換えが必要で、単一バイナリ CLI の規模に対し設計コストが便益を上回る。宣言の cut&paste のみ (コード変更ゼロ)。再検討トリガは vcs の別ツール切り出し需要等
- [DR-0037](./DR-0037-vcs-commit-default-include-deletes.md) — `vcs commit` path mode のデフォルト反転。削除された tracked path を `filterExistingPaths` で黙殺する旧挙動 (declarative-convergence) をやめ、指定 path を git/jj に素直に渡す。`git add -A` により削除も含めて stage される。旧挙動は `--allow-nonexistent-path` でオプトイン。DR-0020 PR-4 の Commit path mode 規定を本 DR が反転 (Diff 等他 mode は維持)
- [DR-0039](./DR-0039-release-yml-semver-gate-pattern.md) — release.yml semver gate canonical pattern (= `latest-release` + `latest-tag` 並列 check + 最終 `gh release view` 重複 guard、`gh release create` の git tag 非 push 仕様 + `--latest=automatic` の date-priority 罠への対策)
- [DR-0038](./DR-0038-vcs-worktree-promote-sync.md) — `vcs` に worktree 検出 + default branch 移動を追加 (= `vcs is worktree/on-default-branch` / `vcs get worktree-name/default-branch` / `vcs promote` / `vcs sync --onto`)。promote は push しない (sync → promote → push の 3 段直交)、sync --onto は必須化 (default 推論しない)、git promote は `update-ref` + 手動 ancestor check で `receive.denyCurrentBranch` を回避、jj DefaultBranch は git に依存せず jj-native で解決 (secondary workspace 対応)。worktree-aware な justfile push gate を可能に
- [DR-0040](./DR-0040-vcs-get-commit-id-default-rev-agnostic.md) — `vcs get commit-id` のデフォルト rev を backend-agnostic 化。jj backend を mutable working copy `@` から `heads((::@-) & (~empty() | merges()))` (最新の固定コミット、git `HEAD` 相当) に変更、git backend は不変。`@-` 自体が空コミット/空マージのケースを実機検証で確認して構造的に解消。DR-0031 の default rev 節を supersede

- [DR-0041](./DR-0041-vcs-get-repository.md) — `vcs get repository` (owner/repo slug) / `repository-url` (https 正規形) を新設。remote URL 由来なので worktree/workspace 間で一貫、slug は「host を除いたパス全体」(GitLab subgroup 対応)。remote 選択は origin デフォルト + `--remote NAME`、origin 不在で単一 remote なら採用・それ以外は exit 4。ssh alias の実 host 解決はスコープ外

- [DR-0042](./DR-0042-vcs-diff-flag-after-dashdash-hint.md) — `vcs diff` で `--` 以後に置かれた flag 風 positional (例: `-- ... --excludes`) への stderr 警告。`--` の POSIX 契約 (以後 positional 固定) は維持し、flag 解釈 (案 1) も hard error (案 3) も不採用。警告は stdout / exit code 不変で `-qq` でも抑制しない (誤答の予告であってエラー出力とは責務が別)。無音で exclude が include に反転する footgun の可視化

- [DR-0043](./DR-0043-global-cwd-option.md) — グローバル `-C/--cwd <path>` を新設 (git の `-C` 相当)。short は make/tar/git の UNIX 慣習、long は意味論最短の `--cwd` (bun/yarn 先例、`--directory`/`--cd` は不採用理由付き)。累積なし (once-only)、cobra 解析前の `os.Chdir` 一発で全経路が自動追従。chdir 失敗は exit 2

## Archived

- [DR-0002](./DR-0002-cargo-workspace-not-supported.md) — Cargo workspace の `[workspace.package].version` を MVP では扱わない (Superseded by DR-0021)

## Moved to research/

(なし)
