# idea: src/ パッケージ分割の検討

- Date: 2026-06-10
- Status: idea

## Context

`src/` 配下は現状フラットな 1 パッケージ (約 80 ファイル) で、`vcs_backend.go` が 2184 行に達している。
責務の境界は概念的には明確に分かれており、以下のような分割が候補として考えられる:

| 分割候補 | 代表ファイル群 | 概要 |
|---|---|---|
| `format` 系 | `format_*.go` | 各ファイル形式の Inspect/Replace 実装 |
| `vcs` 系 | `vcs_backend.go`, `vcs_*.go` | Git/Jujutsu/Mercurial などの VCS backend |
| `cli` 系 | `cmd*.go`, cobra 関連 | サブコマンド定義・引数パース |

## 懸念点

- 現状は機能追加・修正の単位が小さく、パッケージ境界を引くと import cycle のリスクがある
- `format` 系と `vcs` 系は `Rule` / `FormatResult` 等の共通型を介して密結合しており、
  分割前に型の所属を整理する必要がある
- テストファイルも同 package に多数あり、分割時の影響範囲が広い

## 進める場合の優先順位案

1. まず `vcs_backend.go` を機能ブロック単位でファイル分割 (同一 package 内) して見通しを改善
2. 分割後に依存グラフを整理し、切り出し可能な境界を確認してから package 分割を判断

実装着手前に DR で設計判断を残すのが望ましい。
