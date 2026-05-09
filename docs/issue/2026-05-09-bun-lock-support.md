# `bun.lock` (Bun 1.2+ テキスト形式) のサポート

## 背景

Bun 1.2 以降は lockfile が **テキスト形式の `bun.lock`** に変わった (旧 `bun.lockb` バイナリ形式は廃止予定)。Bun 公式によると `bun.lock` は JSONC 風 (コメント許容、trailing comma 許容)。

Bun プロジェクトで `bump-semver` を使う場合、`package.json` と `bun.lock` の version を同期する必要がある。

## 仕様調査が必要

実装に着手する前に確認すべき点:

1. **`bun.lock` 内部に自パッケージの version が含まれているか?**
   - npm の `package-lock.json` のように `top-level.version` + `packages[""].version` を持つ?
   - それとも依存のみで自パッケージ version は持たない?
2. ファイル形式の正確な仕様 (Bun 公式ドキュメント or 実ファイル例で確認)
3. lockfileVersion 互換性 (Bun 1.2 / 1.3 などでフォーマット差異)

## 想定される実装

(仕様確認後)

- basename `bun.lock` で専用 handler に dispatch
- JSONC パーサが必要 (標準 `encoding/json` ではコメント / trailing comma で失敗)
  - 軽量な JSONC パーサを依存追加 (`tailscale.com/util/jsonc` 等) or 独自実装で前処理
- 抽出パス: 仕様調査後に決定 (現時点では未確定)

## 関連

- 親 issue: `2026-05-09-multi-file-mode-with-version-consistency.md` (複数 FILE + version 整合性、これが先行)
- 旧バイナリ形式 `bun.lockb` は対応しない (バイナリ解析の責務は bump-semver の範囲外)

## 優先度

低。Bun を本格使用する kawaz リポが現れたら着手。現状 Bun 系プロジェクト (claude-statusline 等) でも `package.json` の version だけ管理して `bun.lock` は git 管理外 or 自動同期 (= 触らない) のパターンが多い。

報告者: kawaz/jj-worktree main の親 CC — 2026-05-09 (kawaz の指示に基づき起票)
