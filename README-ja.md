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
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value VER
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
| `--write` | 新しいバージョンを各 FILE に書き戻す (`major` / `minor` / `patch` のみ、`--value` と排他) |

### サポートするファイル形式

basename で自動判定:

| パターン | 形式 |
|---|---|
| `Cargo.toml` | TOML、`[package].version` (整合性検証用に `[package].name` も読む) |
| `package-lock.json` | npm 7+ の lockfile、`$.version` + `$.packages[""].version` (依存は触らない)。lockfile v1 はエラー (`npm 7+` で再生成) |
| `*.json` | JSON、`$.version` (オプションで `$.name`)。`package.json` / `.claude-plugin/plugin.json` / `.claude-plugin/marketplace.json` / `moon.mod.json` を網羅 |
| `VERSION` | プレーンテキスト |

未対応ファイルは明示的なエラー (regex フォールバックは設計上持たない)。

### 複数ファイル: 整合性検証

複数の FILE を渡すと 1 つの単位として bump される。全 file 間で version は事前に一致している必要がある (不一致なら `version mismatch:` で file:path = value 列挙)。検出された package name も取れた範囲で整合性検証され、別プロジェクトのファイルを誤って一括 bump する事故を構造的に防ぐ。name は書き戻し対象ではない。

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
```

複数 FILE 指定時の `get` は CI 用の整合性チェックとして機能する (`--write` 不要、全 version が一致しているかだけ検証)。

### stdin パイプ

stdin がパイプ **かつ FILE が 1 個のとき**、FILE は名前ヒントとして扱われ、内容は stdin から読み込まれる。複数 FILE のときは stdin pipe は無視される (cat / sed と同じく「明示 FILE が stdin より優先」)。ファイルをチェックアウトせずにリビジョン間で比較したい時に有用:

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
bump-semver patch --value v1.2.3              # v1.2.4 (prefix を保持)
bump-semver minor --value version_1_2_3       # version_1_3_0 (prefix + separator を保持)
```

バージョン文字列は `v` / `ver` / `version` の任意プレフィックスと `.` / `_` / `-` のセパレータを受理する (例: `v1.2.3`, `ver-1-2-3`, `version_1_2_3`)。プレフィックスとセパレータは bump 後の出力でも保持される。pre-release / build metadata (`-alpha.1`, `+build.42`) は非対応。

### 終了コード

- `0` — 成功
- 非ゼロ — エラー (未対応ファイル、排他オプション違反、パース失敗、IO エラー等)

## 開発状況

v0.1.0 をリリース済 (Cargo.toml / `*.json` / VERSION の 3 形式に対応した MVP)。今後は「必要が出たら handler を 1 つ追加」(DR-0001) 方針で拡張する。設計判断は [docs/decisions/](./docs/decisions/)、将来検討項目は [docs/ROADMAP.md](./docs/ROADMAP.md) を参照。

## ライセンス

[MIT](LICENSE)
