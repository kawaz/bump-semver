# `pnpm-lock.yaml` のサポート

## 背景

pnpm が生成する `pnpm-lock.yaml` (YAML 形式)。

## 仕様調査が必要

1. **自パッケージの version を含むか?**
   - `importers."."` (root) のエントリに自身の version 情報があるか?
   - 仮説: **依存のみ** (root の version は `package.json` 側で管理、yarn.lock と同じパターン)
2. workspace の場合、各 importer のエントリに version があるかも要確認

## 想定される結論 (調査前の仮説)

`yarn.lock` と同じく **対応不要** の可能性が高い。`package.json` だけ bump すれば pnpm が自動同期する。

ただし pnpm workspace で `importers["packages/foo"]` のような構造を持つ場合、何らかの version 情報を持つ可能性は残る。要調査。

## 想定される実装 (仕様確認後)

もし対応するなら:

- basename `pnpm-lock.yaml` で専用 handler
- YAML パーサが必要 (`gopkg.in/yaml.v3` 等)

## 関連

- 親 issue: `2026-05-09-multi-file-mode-with-version-consistency.md`
- 兄弟 issue: `2026-05-09-yarn-lock-support.md` (同じく「依存のみ」で対応不要の可能性)

## 優先度

最低。`yarn-lock-support` と同じく、調査して「対応不要」なら issue を delete。

報告者: kawaz/jj-worktree main の親 CC — 2026-05-09 (kawaz の指示に基づき起票)
