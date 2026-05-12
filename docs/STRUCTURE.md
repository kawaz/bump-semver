# bump-semver リポジトリ物理構造

```
.
├── README.md / README-ja.md     ユーザ向け入口 (target API 仕様、英訳ペア)
├── UPGRADING.md                 メジャー版間の移行ガイド (英語のみ)
├── LICENSE                       MIT (kawaz)
├── VERSION                       現バージョン文字列 (release.yml がこれを監視)
├── go.mod / go.sum               Go module 定義 (リポルートに維持)
├── Taskfile.pkl                  pkfire の Taskfile (default/list/run/test/push/bump-version/ci/build/lint)
├── bin/                          ローカルビルド成果物 (gitignore)
├── docs/
│   ├── DESIGN.md / DESIGN-ja.md  アーキテクチャ + module layout (英訳ペア)
│   ├── STRUCTURE.md              本ファイル: 物理構造の説明
│   ├── ROADMAP.md                将来検討項目
│   ├── decisions/                設計判断記録 (DR)
│   │   ├── INDEX.md
│   │   └── DR-NNNN-*.md
│   └── journal/                  日々の生記録 (ハマり所→解決策のペア)
├── src/                          Go ソース全部 (main + handlers + tests)
│   ├── main.go                   entrypoint, argv パース, 排他ルール, multi-input 整合性
│   ├── compare.go                compare サブコマンド (Version.Compare → exit code)
│   ├── handler.go                Handler interface + path-aware 候補解決
│   ├── handler_*.go              Cargo.toml / *.json / package-lock.json / VERSION
│   ├── format_*.go               format-specific Inspect/Replace (JSON / TOML / plain)
│   ├── rules.go                  path-aware confidence ranked テーブル (DR-0005)
│   ├── jsonpath.go               map[string]any ベースの単純 JSONPath
│   ├── semver.go                 SemVer 2.0.0 parser + Bump + Compare
│   ├── json.go                   --json 出力スキーマ (DR-0007)
│   ├── vcs.go                    vcs: 入力 (jj/git 自動判定 + `latest-tag()`) (DR-0008)
│   ├── spec_table_test.go        DR-0006 仕様駆動テーブルテスト
│   └── *_test.go                 各ファイル対応の単体 + 統合テスト
└── .github/
    └── workflows/
        ├── ci.yml                push / PR で `pkf run ci`
        └── release.yml           VERSION 変化検知 → tag + Releases + homebrew-tap
```

## 設計上のポイント

### `src/` 配下に Go ソースを隔離

リポジトリ直下にはメタ情報 (README / docs / Taskfile.pkl / VERSION / go.mod 等) のみを置き、Go ソースは `src/` に集約する。`go.mod` 自体はリポルートに残しているため module / import path は `github.com/kawaz/bump-semver` のまま (パッケージとしての import path は `github.com/kawaz/bump-semver/src`)。

ビルドターゲット指定:
- `go build ./src` (Taskfile.pkl / release.yml ともに `./src`)
- `go test ./...` は go.mod 起点でリポ全体を巡るので `src/` 配下のテストも自動で実行される

### `go.mod` をリポルートに置く理由

`src/go.mod` のように切り出すと module path が `github.com/kawaz/bump-semver/src` となり、リポ名と import 名がずれて気持ち悪い。リポルートに `go.mod` を置きつつビルド対象だけ `./src` に閉じる構成が、`port-peeker` 等の他 kawaz/* リポと統一が取れる方針。

### `bin/` はローカル成果物のみ

`pkf run build` は `bin/bump-semver` を生成するが、CI はリポジトリの `bin/` を使わずに直接 `go build` する。`bin/` は `.gitignore` 対象。

### release.yml が `VERSION` ファイルを監視

`paths: ["VERSION"]` により VERSION 変更コミットが push されたときだけ release ジョブが起動する。`pkf run bump-version <level>` がこのファイルを書き換えて push する責務を持つ。

### `UPGRADING.md` をリポルートに置く理由

破壊変更があるバージョン跨ぎでユーザが最初に開くドキュメント。`README.md` から誘導する性質上、直下にあった方が動線が短い (LICENSE / README と同じ慣習)。kawaz/* の他リポでも UPGRADING.md は直下に置く運用で揃える。英訳ペアは作らず英語のみ (kawaz の OSS 慣習)。

### `spec_table_test.go` の役割

DR-0006 のテーブル (Bump 挙動の表 / Compare の順序例) を Go の table-driven テストとして転記したもの。仕様変更時はまずこのファイルを更新 → 実装が追従する形で TDD を回す。
