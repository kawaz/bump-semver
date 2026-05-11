# bump-semver 設計書

> [English](./DESIGN.md) | 日本語

## 背景

kawaz/* 各リポジトリのリリースワークフローで、Cargo.toml / package.json / VERSION / .claude-plugin/{plugin,marketplace}.json のバージョン取得・bump・比較を行う必要がある。既存の汎用 `bump` ツール (`kawaz/go/bin/bump`) は `-f <file> -p <regex>` を毎回指定する必要があり、justfile が冗長になる。

例 (claude-cmux-msg の justfile):

```bash
bump {{level}} -w -f .claude-plugin/plugin.json      -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f .claude-plugin/marketplace.json -p '"version":\s*"([^"]+)"'
bump {{level}} -w -f package.json                    -p '"version":\s*"([^"]+)"'
```

3 ファイルに同じ regex を 3 回書く現状を、ファイル名だけで形式判定する CLI に置き換える。さらに v0.5.0 で `compare` サブコマンドが入り、リリース前のドリフト確認等もこの CLI 単体でこなせるようにする (DR-0006)。

## 解決策

ファイル形式判定をツール内部に閉じ込め、CLI 表面は **action + 入力 + 任意フラグ** だけのフラットな構造にする。さらに入力は **FILE / VER / `-`** を位置引数で統一受理し、シェルパイプとの合成性を上げる。

## アーキテクチャ

### CLI 構造

```
bump-semver <ACTION> <INPUT...> [flags]
bump-semver compare <OP> <INPUT> <INPUT>

ACTION = major | minor | patch | pre | get
OP     = eq | lt | le | gt | ge
INPUT  = FILE | VER | -
```

`ACTION` は flat な 5 値 (`major` / `minor` / `patch` / `pre` / `get`)。比較は `compare` という 1 つのネストサブコマンドの下に `eq`/`lt`/`le`/`gt`/`ge` を配置することで、bump/read 系の flat 性を維持しつつ比較系を独立した名前空間に閉じ込めた (DR-0006)。

複数 INPUT は単一の単位として扱う (DR-0004)。検出された全 version は事前に一致している必要があり、name (取れた範囲) も整合性検証される。

### 入力モード (FILE | VER | `-` | `vcs:`)

各位置引数は以下の優先順で解決される (DR-0006 確定論点 B、DR-0008 で `vcs:` を追加):

1. `-` → stdin から VER を 1 行読み込み (1 引数につき stdin 消費は 1 回まで)
2. `vcs:` で始まる → VCS 経由で解決 (DR-0008、後述)
3. ファイルとして存在する → FILE 扱い
4. semver としてパース可能 → VER 扱い
5. それ以外 → エラー

`1.2.3` のようにファイル名と VER 文字列が衝突するケースは `./1.2.3` で明示する (Unix 慣習)。

#### `vcs:` 入力 (DR-0008)

`vcs:REV[:FILE]` は jj/git の `<REV>` 時点の `<FILE>` 内容を取得する。VCS は以下の優先順で自動判定: `--vcs jj|git` フラグ (`auto` / 未指定は次へ) → `.jj` ディレクトリ存在 → `.git` ディレクトリ存在。`.jj` と `.git` が並存する (jj colocate モード、kawaz の git-bare + jj-workspace 構成) 場合は **jj が優先**。`BUMP_SEMVER_VCS` 環境変数がフラグと probe の間に挟まる優先順 2 位だった構造は DR-0016 で廃止されている。

`vcs:latest-tag()` は MVP 唯一の関数: 全 tag を取得し、semver パース不可なものは無視、SemVer 2.0.0 順序で最大を返す。

FILE 省略時は **位置順で最初の FILE 提供 sibling** から借用 (実 FILE 起源 or 他の `vcs:REV:FILE`)。借用源がない場合はエラー。

`bump-semver` は `git fetch` / `jj git fetch` を自動実行しない。古い remote の場合は VCS のエラーがそのまま伝わる。`vcs:` 入力が混ざる invocation での `--write` はエラー (vcs: は read-only)。

### 引数排他ルール

| 組み合わせ | 動作 |
|---|---|
| `--pre` + `--no-pre` | エラー (排他) |
| `--build-metadata` + `--no-build-metadata` | エラー (排他) |
| `--write` + `get` / `compare` | エラー (read-only / 比較に書き戻しは無意味) |
| `--write` 指定時に FILE 入力 0 個 | エラー (`--write requires at least one FILE`) |
| 複数 INPUT で値不一致 | `version mismatch:` でカラム揃え縦列挙 |
| 単一 FILE INPUT + stdin pipe | FILE は名前ヒント、内容は stdin から (legacy) |
| 複数 INPUT 時の stdin pipe | 無視 (cat / sed と同じく明示 INPUT 優先) |
| いずれの違反もない | 正常実行 |

### モジュール構成

Go ソースは `src/` 配下に隔離し、リポジトリ直下にはメタ情報 (README / docs / justfile / VERSION / go.mod 等) のみを置く。`go.mod` 自体はリポジトリ直下のままで、import path / module path は `github.com/kawaz/bump-semver` から変わらない。ビルドは `go build ./src`。

```
.
├── go.mod / go.sum
├── justfile
├── VERSION
├── README{,-ja}.md
├── UPGRADING.md             v0.4.x → v0.5.0 移行ガイド
├── docs/
└── src/
    ├── main.go              entrypoint, argv パース, multi-input 整合性検証
    ├── compare.go           compare サブコマンド (Version.Compare → exit code)
    ├── handler.go           Handler interface (Inspect / Replace) + dispatcher
    ├── handler_*.go         Cargo.toml / *.json / package-lock.json / VERSION
    ├── format_*.go          format-specific Inspect/Replace (JSON / TOML / plain)
    ├── rules.go             path-aware confidence ranked テーブル (DR-0005)
    ├── jsonpath.go          map[string]any ベースの単純 JSONPath
    ├── semver.go            semver 2.0.0 parser + Bump + Compare
    ├── json.go              --json 出力スキーマ (DR-0007)
    ├── vcs.go               vcs: 入力 (jj/git 自動判定 + `latest-tag()`) (DR-0008)
    └── *_test.go            単体 + 統合 + spec_table_test.go (DR-0006 仕様駆動テスト)
```

### 形式判定 — path-aware, confidence-ranked (DR-0005)

判定は `CandidateRule` の **テーブル** で行う。各行が「path-pattern, format, version-paths, name-paths」のタプルで、確度降順に並ぶ。入力 FILE に対する手順:

1. ルールを確度降順 (3 → 2 → 1) に巡回
2. ルールの path-pattern にマッチしたら抽出 (Inspect) を試行
3. 抽出成功 (全 `VersionPaths` が存在し semver パース可能) なら、そのルールが採用される
4. 抽出失敗 → 次にマッチするルールに降りる
5. 全てのマッチルールが失敗したら、最後のエラーを `<path>: <ruleName>: <reason>` で返す

確度レベル:

- **3 — path-pinned**: 相対パス suffix (`.claude-plugin/marketplace.json`) や一意な basename (`Cargo.toml`, `VERSION`, `package.json`, `package-lock.json`)
- **2 — basename only**: 任意ディレクトリの `marketplace.json` / `plugin.json` (Claude plugin の慣習だが `.claude-plugin/` 配下とは限らない)
- **1 — glob fallback**: 上記以外の `*.json` を top-level `.version` で網羅

これにより `.claude-plugin/` 外の `marketplace.json` も Claude plugin としてまず試行され (確度 2)、`.metadata.version` を持たなければ素直に top-level `.version` の汎用 JSON に降格する (確度 1)。新ファイル形式の追加 = **テーブル 1 行追加** (新 format なら新 format-specific Inspect/Replace ペアを 1 つ追加) で済む。CLI 表面には `--pattern` フラグは出さない。

stdin がパイプ **かつ FILE INPUT が 1 個** のときは FILE を「名前ヒント」として上記判定にだけ使い、内容は stdin から読む (legacy ショートカット)。複数 INPUT のときは stdin pipe を無視してファイルから読む (cat / sed と同じく明示 INPUT が優先)。`-` を INPUT として明示すれば新方式の stdin VER 読込として処理される。

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

main は全 INPUT 横断で `Versions` と `Names` を集約し、以下を要求:

- 全 version field が一致 (不一致なら `version mismatch:` でカラム整列の縦列挙、起源ラベル付き)
- 取れた範囲で全 name field が一致 (不一致なら `name mismatch:` ...)。name を持たないファイルはスキップされるので `Cargo.toml` + `VERSION` 混在は問題なく通る

`Replace` は version field のみ書き換え、name は触らない。`package-lock.json` handler は `json.Decoder` で構造を辿るので、依存エントリ (`$.packages["node_modules/..."]`) の version は仮に root version と同値でも書き換わらないことが保証される。

### bump セマンティクス

バージョン文字列は SemVer 2.0.0 構文に kawaz 拡張 prefix/sep を加えた以下を受理する (DR-0003 + DR-0006):

```
本体: (v|ver|version)?[._]?\d+[._]\d+[._]\d+      (sep1 == sep2 を強制)
pre:  -<id>(.<id>)*                                (SemVer 2.0.0 仕様)
meta: +<id>(.<id>)*                                (SemVer 2.0.0 仕様)
```

- 本体セパレータは `.` または `_` のみ。`-` は **不可** (pre-release `-` と衝突するため、DR-0006 で `[._-]` から `[._]` に絞った)
- 数値のみの識別子 (本体・pre 共通) は leading zero 禁止 (SemVer 仕様)
- build metadata は leading zero 許容 (仕様)

prefix と separator は `Bump` / `String` を通して保持される。`pre` と `build metadata` は default では bump 時に **drop** される (DR-0006、npm 流 strip-don't-bump とは異なる単一規則)。

| 入力 | アクション | 出力 |
|---|---|---|
| `1.2.3` | `patch` | `1.2.4` |
| `v1.2.3` | `patch` | `v1.2.4` |
| `version_1_2_3` | `minor` | `version_1_3_0` |
| `1.2.3-rc.0` | `patch` | `1.2.4` (drop) |
| `1.2.3-rc.0` | `pre` | `1.2.3-rc.1` (counter advance) |
| `1.2.3-rc1` | `pre` | error (英数字混在は incremental ではない) |
| `1.2.3` | `pre --pre rc.0` | `1.2.3-rc.0` (上書き) |
| `1.2.3-rc.0` | `pre --no-pre` | `1.2.3` (削除) |
| `1.2.3-rc.0` | `patch --pre rc.0` | `1.2.4-rc.0` (bump + 再付与) |
| `1.2.3-rc.0+build` | `patch` | `1.2.4` (両 drop) |

separator 不一致 (`1.2_3`) はエラー。

`pre` アクションの 3 モード:

- 引数なし: 末尾識別子が pure numeric なら `+1` (`rc.0 → rc.1`)、それ以外エラー
- `--pre PRE`: PRE 値で完全上書き (元 pre 有無問わず、巻き戻りも許容)
- `--no-pre`: pre 削除 (元 pre 不在でも nop)

### 比較セマンティクス (compare サブコマンド)

`compare <OP> <INPUT> <INPUT>` は SemVer 2.0.0 § 11 順序仕様準拠で比較する:

1. MAJOR/MINOR/PATCH 数値比較
2. pre-release あり < 同 base の確定版 (`1.0.0-rc.1 < 1.0.0`)
3. pre-release 同士は識別子比較 (数値 vs 数値は数値順、英数字 vs 英数字は ASCII 順、数値 < 英数字)
4. build metadata は順序比較から完全に除外 (`1.0.0+a == 1.0.0+b`)
5. prefix / sep の違いは正規化 (`v1.2.3` == `1.2.3` == `version_1_2_3`)

各 INPUT は bump 系と同じ FILE/VER/`-` 解決ロジックで解決され、複数 version field を持つ INPUT (例: `package-lock.json`) は内部で整合性検証 → 1 値に集約してから比較に渡す。

終了コード:
- `0` = 真
- `1` = 偽
- `2` = エラー (パース失敗、整合性 NG、未対応ファイル等)

これは `test` / `dpkg --compare-versions` 慣習に揃えた (DR-0006 確定論点 A)。bump 系の旧 exit code 1 (エラー) もこの統一に合わせて 2 に変更されているため、`if [ $? -eq 1 ]` を直接見る古いスクリプトは `if [ $? -ne 0 ]` への書き換えが必要 (UPGRADING.md 参照)。

#### precision suffix (DR-0017)

OP には `-major` / `-minor` / `-patch` のいずれかを suffix で付けられる。比較対象の component を切り詰めて評価する:

- `-major`: X のみで比較 (`eq-major 1.2.3 1.9.7` → true)
- `-minor`: X.Y で比較 (`eq-minor 1.2.3 1.2.9` → true)
- `-patch`: X.Y.Z で比較し pre-release は無視 (`eq-patch 1.2.3 1.2.3-rc.1` → true)
- suffix なし: SemVer 2.0.0 § 11 完全比較 (pre-release を含む)

5 base × 4 precision = 20 OP。build metadata は常に無視 (SemVer § 10)。CI で「メジャー upgrade を検知したい」「pre-release 違いは無視して同じ release version か知りたい」用途を 1 行で表現できる。

### 出力

成功時は **常に新しいバージョンを stdout に1行出力** する (`--write` の有無で変わらない、bump 系)。`compare` は predicate true でも stdout 出力なし (パイプライン汚染回避、結果は exit code で取得)。

エラー時は stderr に `bump-semver: <reason>` を1行 + non-zero exit。エラーメッセージは入力起源 (VER / FILE) で wrap 形式が変わる (DR-0006 確定論点 E):

- VER 起源: 素のエラーをそのまま (例 `rc1 is not incremental, use --pre PRE`)
- FILE 起源: `<file>:<path>=<value>: <semver-error>` で wrap

複数 INPUT 不一致時はカラム整列の縦列挙 (DR-0006 確定論点 F):

```
bump-semver: version mismatch:
  Cargo.toml:[package].version = 1.2.3
  package.json:$.version       = 1.2.4
  <argv>                       = 1.2.3-rc.1
```

起源ラベル: `<file>:<path>` (FILE) / `<argv>` または `<argv:N>` (位置引数の VER) / `<stdin>` (`-`)。

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
