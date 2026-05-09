# bump-semver

> [English](./README.md) | 日本語

バージョン管理用ファイル中の semver 文字列を取得・bump するための、絞り込まれた CLI。ファイル形式は basename で自動判定 (`--pattern` regex フラグ不要)、4 つの flat なアクション (`major` / `minor` / `patch` / `get`) を持ち、新しいバージョンは常に stdout に出力するのでシェルパイプラインに合成しやすい。

## なぜ作ったか

既存のバージョン bump CLI は「汎用すぎて毎回 regex を指定する必要がある」「特定のファイル形式しか扱えない」のどちらかに偏っている。`bump-semver` は真逆の立場を取り、kawaz が実際に使うファイル形式だけを正確にサポートし、新しい形式は具体的な必要が出たときに追加する。結果として「小さい・断定的・予測可能」な kawaz スタイルのツールになる。

## インストール

```bash
brew install kawaz/tap/bump-semver
```

`kawaz/tap` は [`kawaz/homebrew-tap`](https://github.com/kawaz/homebrew-tap) のこと。2ステップ等価形式: `brew tap kawaz/tap && brew install bump-semver`。

Linux / macOS / Windows (amd64, arm64) のビルド済みバイナリも GitHub Releases に公開。

## 使い方

```
bump-semver <ACTION> <FILE | --value VER> [--write]
```

### アクション

| アクション | 効果 |
|---|---|
| `major` | major を bump (`X.0.0`) |
| `minor` | minor を bump (`x.Y.0`) |
| `patch` | patch を bump (`x.y.Z`) |
| `get`   | 現在のバージョンを出力 |

### オプション

| オプション | 説明 |
|---|---|
| `--value VER` | FILE の代わりに VER を入力として使う (FILE と排他) |
| `--write` | 新しいバージョンを FILE に書き戻す (`major` / `minor` / `patch` のみ、`--value` と排他) |

### サポートするファイル形式

basename で自動判定:

| パターン | 形式 |
|---|---|
| `Cargo.toml` | TOML、`[package].version` |
| `*.json` | JSON、`.version` (`package.json`、`.claude-plugin/plugin.json`、`.claude-plugin/marketplace.json`、`moon.mod.json` を網羅) |
| `VERSION` | プレーンテキスト |

未対応ファイルは明示的なエラー (regex フォールバックは設計上持たない)。

### stdin パイプ

stdin がパイプの場合、FILE は名前ヒントとして扱われ、内容は stdin から読み込まれる。ファイルをチェックアウトせずにリビジョン間で比較したい時に有用:

```bash
jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml
```

### 使用例

```bash
bump-semver patch Cargo.toml --write          # bump + 書き戻し、新バージョンを出力
bump-semver minor package.json                # メモリ上で bump、新バージョン出力 (ファイル不変)
bump-semver get .claude-plugin/plugin.json    # 現在のバージョン
bump-semver patch --value 1.2.3               # 1.2.4
bump-semver get --value 1.2.3                 # パース検証 (1.2.3) かエラー
```

### 終了コード

- `0` — 成功
- 非ゼロ — エラー (未対応ファイル、排他オプション違反、パース失敗、IO エラー等)

## 開発状況

この README は目標 API の仕様。実装は進行中。設計判断は [docs/decisions/](./docs/decisions/) を参照。

## ライセンス

[MIT](LICENSE)
