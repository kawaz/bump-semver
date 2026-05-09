# bump-semver 設計書

> [English](./DESIGN.md) | 日本語

## 背景

kawaz/* 各リポジトリのリリースワークフローで、Cargo.toml / package.json / VERSION / .claude-plugin/{plugin,marketplace}.json のバージョン取得・bump を行う必要がある。既存の汎用 `bump` ツール (`kawaz/go/bin/bump`) は `-f <file> -p <regex>` を毎回指定する必要があり、justfile が冗長になる。

例 (claude-cmux-msg の justfile):

```bash
bump {{level}} -w -f .claude-plugin/plugin.json      -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f .claude-plugin/marketplace.json -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f package.json                    -p '"version":\s*"([^"]+)"'
```

3 ファイルに同じ regex を 3 回書く現状を、ファイル名だけで形式判定する CLI に置き換える。

## 解決策

ファイル形式判定をツール内部に閉じ込め、CLI 表面は **action + 入力 + 任意フラグ** だけのフラットな構造にする。

## アーキテクチャ

### CLI 構造

```
bump-semver <ACTION> <FILE | --value VER> [--write]

ACTION = major | minor | patch | get
```

`ACTION` は flat な 4 値。`get` も他と同じ階層に置くことで、サブコマンド分岐や引数順不同問題を構造的に消す。

### 引数排他ルール

| 組み合わせ | 動作 |
|---|---|
| `FILE` + `--value` | エラー (どちらか一方必須・両方は不可) |
| `--write` + `--value` | エラー (書き戻し対象なし) |
| `--write` + `get` | エラー (取得操作に書き戻しは無意味) |
| いずれの違反もない | 正常実行 |

### モジュール構成 (予定)

```
.
├── main.go             # entrypoint, argv parsing, exclusivity checks
├── handler.go          # Handler interface (Match / Get / Bump)
├── handler_cargo.go    # Cargo.toml (TOML, [package].version)
├── handler_json.go     # *.json (.version)
├── handler_version.go  # VERSION (plain text)
├── semver.go           # x.y.z parsing + bump
└── handler_test.go etc.
```

### 形式判定 (basename)

| 判定キー | Handler |
|---|---|
| `basename(path) == "Cargo.toml"` | cargo |
| `basename(path) == "VERSION"` | version |
| `path` が `*.json` で終わる | json |
| 上記以外 | エラー (`unsupported file: <path>`) |

stdin がパイプの場合は FILE を「名前ヒント」として上記判定にだけ使い、内容は stdin から読む。

### bump セマンティクス

入力 `X.Y.Z` に対して:

- `major` → `(X+1).0.0`
- `minor` → `X.(Y+1).0`
- `patch` → `X.Y.(Z+1)`
- `get`   → `X.Y.Z` (恒等)

pre-release / build metadata (`-alpha.1` `+build.42` 等) は MVP では非対応 (含まれていたらエラー)。必要になったら handler / semver に追加。

### 出力

成功時は **常に新しいバージョンを stdout に1行出力** する (`--write` の有無で変わらない)。これにより `NEW=$(bump-semver patch Cargo.toml --write)` の形でシェル変数に取りやすい。

エラー時は stderr に "bump-semver: <reason>" を1行 + non-zero exit。

## 配布

### リリースフロー

```
just bump-version [patch|minor|major]
  ↓
ensure-clean → test → build → VERSION 書き換え → jj describe + new → just push
  ↓
GitHub Actions (.github/workflows/release.yml) が VERSION 変化を検出
  ↓
Linux / macOS / Windows × amd64 / arm64 の 6 ターゲットでビルド
  ↓
gh release create --target <sha> --generate-notes でタグ + Releases ノートを自動作成
  ↓
update-homebrew job が kawaz/homebrew-tap の Formula を更新
```

このパターンは kawaz/port-peeker / kawaz/jj-worktree / kawaz/authsock-warden で確立済 (詳細は jj-worktree/main/docs/decisions/DR-0003)。`bump-semver` 自身が VERSION ファイルを bump できるので、ドッグフーディングが成立する。

### Windows サポート

ファイル I/O と文字列操作のみで OS 依存処理がないため Linux クロスビルドで完結する。Homebrew は対象外で、GitHub Releases にバイナリのみ配布。

## 関連リポジトリ

- kawaz/jj-worktree (Rust): リリースワークフロー / DR / docs ペア整備の参考実装
- kawaz/port-peeker (Go): VERSION ファイル駆動リリースの最小骨格
- kawaz/claude-cmux-msg: bump-semver の主要ユースケース (Claude プラグインの3ファイル version 同期)
