# Decision Records (DR) Index

bump-semver の設計判断記録一覧。ファイル名は `DR-NNNN-title.md` (4 桁ゼロパディング)。`docs-structure.md` ルールに従い `## Active` / `## Archived` / `## Moved to research/` で区分する。

## Active

- [DR-0001](./DR-0001-flat-actions-and-format-detection.md) — flat 4-action CLI + basename ベースのファイル形式判定
- [DR-0002](./DR-0002-cargo-workspace-not-supported.md) — Cargo workspace の `[workspace.package].version` を MVP では扱わない
- [DR-0003](./DR-0003-prefix-and-flexible-separator.md) — prefix (`v`/`ver`/`version`) と柔軟 separator (`. _ -`) を許容する
- [DR-0004](./DR-0004-multi-file-and-name-consistency.md) — 複数 FILE 一括 bump + name 整合性検証 + package-lock.json 特殊化
- [DR-0005](./DR-0005-path-aware-confidence-ranked-candidates.md) — basename 決め打ちから path-aware confidence ranked candidates へ
- [DR-0006](./DR-0006-pre-release-and-compare.md) — pre-release/build-metadata 対応 + compare サブコマンド + FILE\|VER 統合
- [DR-0007](./DR-0007-json-output-option.md) — `--json` 出力オプション (構造化 JSON、`get` / bump 系のみ)
- [DR-0008](./DR-0008-vcs-input.md) — `vcs:` 入力モード (jj/git の他リビジョン・最新 tag を入力として受け付け)
- [DR-0009](./DR-0009-lockfile-support-scope.md) — lock files の対応対象判断 (npm 以外は対象外、bun/pnpm/yarn/Cargo)
- [DR-0010](./DR-0010-fallback-match-hint.md) — confidence 1 fallback マッチ時の hint 出力 + unsupported file エラーの誘導文言
- [DR-0011](./DR-0011-yaml-yml-toml-fallback.md) — `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback 追加

## Archived

(なし)

## Moved to research/

(なし)
