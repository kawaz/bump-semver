# DR-0001: flat 4-action CLI + basename ベースのファイル形式判定

- ステータス: Accepted
- 日付: 2026-05-09
- 関連: kawaz/jj-worktree/main/docs/findings/2026-05-09-justfile-template-research.md (kawaz リポ群の justfile 横断調査)

## 文脈

kawaz/* の justfile で複数ファイルのバージョン管理が必要 (Cargo.toml / package.json / .claude-plugin/{plugin,marketplace}.json / moon.mod.json / VERSION)。既存の汎用 `bump` ツール (`kawaz/go/bin/bump`) は `-f <file> -p <regex>` を毎回指定する設計で、claude-cmux-msg の justfile では同じ regex を 3 ファイルに 3 回書いている状態。

複数の version-bump CLI が世にあるが、kawaz のニーズに合うものがない:

- 対応ファイル種類が限られている (Cargo.toml 専用 / npm 専用 等)
- 取得 (`get`) または bump のどちらかしかない
- 汎用すぎて引数体系が複雑 (regex 指定必須)

## 決定

### 1. CLI を flat な 4 アクション + 排他フラグに統一

```
bump-semver <major|minor|patch|get> <FILE|--value VER> [--write]
```

- アクションは `major` / `minor` / `patch` / `get` の **flat な 4 値**。`get` をサブコマンドではなくアクションの 1 つとして同階層に並べることで、引数順不同問題が構造的に消える (アクションは必ず先頭、第 2 引数は FILE か `--value` の二択)
- `FILE` と `--value VER` は **排他**。どちらか一方必須
- `--write` は `major` / `minor` / `patch` のみ、`--value` とは排他 (書き戻し対象なし)
- 排他違反は **エラーで弾く**

### 2. ファイル形式は basename で自動判定 (regex フラグなし)

| basename パターン | Handler | 取得経路 |
|---|---|---|
| `Cargo.toml` | cargo | TOML `[package].version` |
| `*.json` | json | JSON `.version` |
| `VERSION` | version | 全文 trim |
| その他 | エラー | `unsupported file: <path>` |

`*.json` を一律 `.version` キーで扱うことで、`package.json` / `.claude-plugin/plugin.json` / `marketplace.json` / `moon.mod.json` を **追加コードなしで網羅**できる。

未対応ファイルは regex フォールバックなしで即エラー。「網羅」は捨て、必要が出たら handler を 1 つ追加するだけで対応する方針。

### 3. stdin がパイプの場合は FILE を名前ヒントに

stdin が pipe (`!isatty(0)`) のとき、FILE はファイル形式判定の名前ヒントとしてのみ使い、内容は stdin から読む。これは `jj file show <rev> Cargo.toml | bump-semver get Cargo.toml` のように、ワークスペースに展開していないリビジョンのバージョンを取りたいときに使う。

### 4. 成功時は常に stdout に新バージョン

`--write` してもしなくても、新バージョンを stdout に 1 行出力する。`NEW=$(bump-semver patch Cargo.toml --write)` のシェル合成を素直に動かすため。

## 不採用案

### A. `bump` / `get` の 2 サブコマンド体系

```
bump-semver get FILE
bump-semver bump FILE [level]
```

問題:

- `bump FILE level` の引数順不同を実装する必要 (アクション enum を後ろにも置けるロジック)
- ヘルプが「bump は level 省略可」「get は level 取らない」と一貫しない説明になる
- 4-action flat 案では `bump-semver minor Cargo.toml` が常に 3 トークン以下、`bump <FILE> <level>` 案より短く読みやすい

### B. 既存 `bump` ツールに subcommand を追加して進化

`kawaz/go/bin/bump` (実体は外部の Go 製ツール、kawaz の自作ではない) に subcommand を足す改造案。kawaz のレポジトリではなく外部ツールであるため不可。仮に自作だとしても、引数体系が大きく変わる破壊的変更になり、既存ユーザ (claude-cmux-msg 等) が壊れる。

### C. regex フォールバック (`--pattern`)

未対応ファイルに対して `--pattern '...'` で fallback できる案。「汎用すぎる引数体系を避ける」という本 DR の核心と矛盾するため不採用。必要な形式は handler を追加して対応する。

### D. pre-release / build metadata 対応

semver の `1.2.3-alpha.1+build.42` を MVP で扱う案。実装複雑度が増すうえ、kawaz の現用途では使う場面がない。「都度対応」方針に従い、必要が出たら handler / semver に追加する。

## 実装方針

- Go 実装 (`go.mod` 自身が version フィールドを持たないため、自身のバージョンは VERSION ファイルで管理)
- 配布: kawaz/homebrew-tap + GitHub Releases (Linux / macOS / Windows × amd64 / arm64 の 6 ターゲット)
- リリースフロー: kawaz/port-peeker と同じ「VERSION 変化検知 → tag 自動生成 + `--generate-notes`」パターン (kawaz/jj-worktree/main/docs/decisions/DR-0003 と整合)

## 関連

- 主要消費者: kawaz/claude-cmux-msg, kawaz/jj-worktree, kawaz/port-peeker, kawaz/authsock-warden の justfile から呼ばれる
- 参考実装: kawaz/port-peeker (Go + VERSION 駆動 release.yml)
- 設計思想の背景: kawaz/jj-worktree/main/docs/findings/2026-05-09-justfile-template-research.md
