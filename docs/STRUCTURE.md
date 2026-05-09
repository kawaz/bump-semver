# bump-semver リポジトリ物理構造

```
.
├── README.md / README-ja.md     ユーザ向け入口 (target API 仕様、英訳ペア)
├── LICENSE                       MIT (kawaz)
├── VERSION                       現バージョン文字列 (release.yml がこれを監視)
├── go.mod / go.sum               Go module 定義 (リポルートに維持)
├── justfile                      開発タスク (lint / test / build / push / bump-version)
├── bin/                          ローカルビルド成果物 (gitignore)
├── docs/
│   ├── DESIGN.md / DESIGN-ja.md  アーキテクチャ + module layout (英訳ペア)
│   ├── STRUCTURE.md              本ファイル: 物理構造の説明
│   ├── ROADMAP.md                将来検討項目
│   └── decisions/                設計判断記録 (DR)
│       ├── INDEX.md
│       └── DR-NNNN-*.md
├── src/                          Go ソース全部 (main + handlers + tests)
│   ├── main.go                   entrypoint, argv パース, 排他ルール, stdin pipe
│   ├── handler.go                Handler interface + basename ベースの dispatcher
│   ├── handler_cargo.go          Cargo.toml ([package].version)
│   ├── handler_json.go           *.json (.version)
│   ├── handler_version.go        VERSION (plain text)
│   ├── semver.go                 X.Y.Z parser + Bump
│   └── *_test.go                 各ファイル対応の単体 + 統合テスト
└── .github/
    └── workflows/
        ├── ci.yml                push / PR で `just ci`
        └── release.yml           VERSION 変化検知 → tag + Releases + homebrew-tap
```

## 設計上のポイント

### `src/` 配下に Go ソースを隔離

リポジトリ直下にはメタ情報 (README / docs / justfile / VERSION / go.mod 等) のみを置き、Go ソースは `src/` に集約する。`go.mod` 自体はリポルートに残しているため module / import path は `github.com/kawaz/bump-semver` のまま (パッケージとしての import path は `github.com/kawaz/bump-semver/src`)。

ビルドターゲット指定:
- `go build ./src` (justfile / release.yml ともに `./src`)
- `go test ./...` は go.mod 起点でリポ全体を巡るので `src/` 配下のテストも自動で実行される

### `go.mod` をリポルートに置く理由

`src/go.mod` のように切り出すと module path が `github.com/kawaz/bump-semver/src` となり、リポ名と import 名がずれて気持ち悪い。リポルートに `go.mod` を置きつつビルド対象だけ `./src` に閉じる構成が、`port-peeker` 等の他 kawaz/* リポと統一が取れる方針。

### `bin/` はローカル成果物のみ

`just build` は `bin/bump-semver` を生成するが、CI はリポジトリの `bin/` を使わずに直接 `go build` する。`bin/` は `.gitignore` 対象。

### release.yml が `VERSION` ファイルを監視

`paths: ["VERSION"]` により VERSION 変更コミットが push されたときだけ release ジョブが起動する。`just bump-version <level>` がこのファイルを書き換えて push する責務を持つ。
