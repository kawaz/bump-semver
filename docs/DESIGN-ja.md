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
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value VER

ACTION = major | minor | patch | get
```

`ACTION` は flat な 4 値。`get` も他と同じ階層に置くことで、サブコマンド分岐や引数順不同問題を構造的に消す。

複数 FILE は単一の単位として一括 bump する (DR-0004)。検出された全 version は事前に一致している必要があり、name (取れた範囲) も整合性検証される。

### 引数排他ルール

| 組み合わせ | 動作 |
|---|---|
| `FILE...` + `--value` | エラー (どちらか一方必須・両方は不可) |
| `--write` + `--value` | エラー (書き戻し対象なし) |
| `--write` + `get` | エラー (取得操作に書き戻しは無意味) |
| stdin pipe + 複数 `FILE...` | stdin pipe を無視、ファイルから読む |
| stdin pipe + 単一 `FILE` + `--write` | エラー (stdin が入力源、書き戻し先と矛盾) |
| いずれの違反もない | 正常実行 |

### モジュール構成

Go ソースは `src/` 配下に隔離し、リポジトリ直下にはメタ情報 (README / docs / justfile / VERSION / go.mod 等) のみを置く。`go.mod` 自体はリポジトリ直下のままで、import path / module path は `github.com/kawaz/bump-semver` から変わらない。ビルドは `go build ./src`。

```
.
├── go.mod / go.sum
├── justfile
├── VERSION
├── README{,-ja}.md
├── docs/
└── src/
    ├── main.go              # entrypoint, argv パース, multi-file 整合性検証
    ├── handler.go           # Handler interface (Inspect / Replace) + dispatcher
    ├── handler_cargo.go     # Cargo.toml (TOML, [package].version + .name)
    ├── handler_json.go      # *.json ($.version + optional $.name)
    ├── handler_npm_lock.go  # package-lock.json (npm 7+, $.version + $.packages[""].version)
    ├── handler_version.go   # VERSION (plain text)
    ├── semver.go            # X.Y.Z parsing + bump (v / ver / version prefix と . _ - separator)
    └── *_test.go            # 単体 + 統合テスト
```

### 形式判定 (basename)

| 判定キー | Handler |
|---|---|
| `basename(path) == "Cargo.toml"` | cargo |
| `basename(path) == "VERSION"` | version |
| `basename(path) == "package-lock.json"` | npm-lock (`*.json` 一般枝より先に判定) |
| `path` が `*.json` で終わる | json |
| 上記以外 | エラー (`unsupported file: <path>`) |

stdin がパイプ **かつ FILE が 1 個** のときは FILE を「名前ヒント」として上記判定にだけ使い、内容は stdin から読む。複数 FILE のときは stdin pipe を無視してファイルから読む (cat / sed と同じく明示 FILE が優先)。

### Handler interface と整合性検証 (DR-0004)

各 handler はファイル中の version-like / name-like 値を全部記録した `Inspection` を返す:

```go
type Field struct {
    Value string
    Path  string  // エラー表示用: "$.version", "[package].version", "(file content)" 等
}

type Inspection struct {
    Versions []Field  // 1+
    Names    []Field  // 0+ (optional)
}

type Handler interface {
    Inspect(content []byte) (Inspection, error)
    Replace(content []byte, current, newVersion string) ([]byte, error)
}
```

main は全 FILE 横断で `Versions` と `Names` を集約し、以下を要求:

- 全 version field が一致 (不一致なら `version mismatch:` で file:path = value を列挙)
- 取れた範囲で全 name field が一致 (不一致なら `name mismatch:` ...)。name を持たないファイルはスキップされるので `Cargo.toml` + `VERSION` 混在は問題なく通る。

`Replace` は version field のみ書き換え、name は触らない。`package-lock.json` handler は `json.Decoder` で構造を辿るので、依存エントリ (`$.packages["node_modules/..."]`) の version は仮に root version と同値でも書き換わらないことが保証される。

### bump セマンティクス

バージョン文字列は `[v|ver|version][_.-]?X<sep>Y<sep>Z` を受理する (`<sep>` は `.` / `_` / `-` のいずれか、両側で一致が必要、DR-0003)。オプションの prefix と選ばれた separator は `Bump` / `String` を通して保持される:

| 入力 | アクション | 出力 |
|---|---|---|
| `1.2.3` | `patch` | `1.2.4` |
| `v1.2.3` | `patch` | `v1.2.4` |
| `version_1_2_3` | `minor` | `version_1_3_0` |
| `ver-1-2-3` | `major` | `ver-2-0-0` |
| `1-2-3` | `get` | `1-2-3` |

separator 不一致 (`1.2-3`) はエラー。pre-release / build metadata (`-alpha.1`, `+build.42` 等) は MVP では非対応 — 含まれていたらエラー。必要が出たら semver モジュールに対応を追加する。

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
