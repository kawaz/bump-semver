# `yarn.lock` のサポート

## 背景

Yarn classic (v1) / Yarn Berry (v2+) で使われる `yarn.lock` の独自テキスト形式 (YAML 風だが厳密には独自)。

## 仕様調査が必要 (実装より先)

`yarn.lock` は **通常、依存パッケージの version しか書かない** (自パッケージ entry はない、root の version は `package.json` のみで管理する設計)。よって:

1. **そもそも自パッケージの version を含むケースがあるか?**
   - workspace root: 含まないはず
   - workspace member: 含まないはず
   - 結論: **基本的に対応不要** の可能性が高い
2. もし含むパターンがある場合 (Berry の独自設定等) はそのケースを特定
3. 含まないなら、`yarn.lock` は bump-semver の対応対象外として明記する (basename `yarn.lock` を「対応外」リストに追加し、誤って渡された時に「`yarn.lock` does not contain the project's own version; nothing to do」と stderr に出して exit 0 する選択肢もあり)

## 想定される結論 (調査前の仮説)

- **対応しない**: `yarn.lock` には自身の version 情報がないため。`package.json` だけ bump すれば yarn 側が自動同期する
- 誤って `bump-semver patch yarn.lock --write` を呼ばれた時のための **明示的なノーオプ + 警告** はあっても良い

## 関連

- 親 issue: `2026-05-09-multi-file-mode-with-version-consistency.md`
- npm 系: `package-lock.json` は逆に自身の version を 2 箇所に持つ (親 issue 参照)

## 優先度

最低。仕様調査して「対応不要」の結論なら issue を delete するだけで済む可能性。

報告者: kawaz/jj-worktree main の親 CC — 2026-05-09 (kawaz の指示に基づき起票)
