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

コマンドツリーは [spf13/cobra](https://github.com/spf13/cobra) で構築している。bump/read 系は top-level で flat に並べ、`compare` / `vcs` / `completion` をネストした名前空間とする。help とシェル補完は cobra が生成する (`--help` 短縮 / `--help-full` 完全リファレンス / `<command> --help` 各コマンド別、`completion <shell>` で bash/zsh/fish/powershell)。各 help の Options セクションは cobra の `FlagSet` からレンダリングされるので、フラグ一覧が実装からずれない。

```
bump-semver
├── major | minor | patch          bump (FILE / VER 入力、pre / build は default drop)
├── pre                            pre-release counter advance / set (--pre) / remove (--no-pre)
├── get                            現バージョン読み取り (read-only, multi-input 整合チェック)
├── compare <OP> BASE OTHER...     exit-code 駆動の比較 (20 OP = 5 base × 4 precision)
├── vcs                            git/jj 抽象 helper (DR-0020)
│   ├── commit                     -m MSG PATH.. | --staged | --amend  (-a/--all は reject)
│   ├── diff                       [-s|--name-status] [-q] REV [PATH..] [--excludes ...]
│   ├── fetch                      [REMOTE] | --remote NAME  (default origin)
│   ├── get                        root | backend | current-branch | commit-id |
│   │                              latest-tag | latest-release
│   ├── is                         clean | dirty | git | jj   (答えは exit code)
│   ├── outdated                   FROM TO[..] | -- 区切り複数ペア  [--explain --strict --glob-*]
│   ├── push                       --branch/--bookmark NAME [--remote] [--jj-bookmark-auto-advance]
│   └── tag
│       ├── push                   --rev REV NAME [--remote] [--allow-move]
│       └── delete                 NAME [--remote]   (idempotent rm -f semantics)
└── completion                     <bash|zsh|fish|powershell>   (cobra 自動生成)

OP    = (eq|lt|le|gt|ge)[-major|-minor|-patch]
INPUT = FILE | VER | - | vcs:… | cmd:…
```

bump/read 系 (`major` / `minor` / `patch` / `pre` / `get`) は flat。比較・VCS helper・シェル補完はそれぞれ独立したネスト名前空間に閉じ込め、よく使う操作を 1 トークンの深さに保つ (DR-0006, DR-0020)。

`compare` は単一の BASE と 1 つ以上の OTHER を取り、各 OTHER を BASE に対して独立に評価する (失敗した OTHER を全部報告、short-circuit しない)。

bump/read アクションへの複数 INPUT は単一の単位として扱う (DR-0004)。検出された全 version は事前に一致している必要があり、name (取れた範囲) も整合性検証される。

`vcs` 名前空間は git/jj の事実取得と変更操作を 1 つの安定した surface で提供する。backend は自動判定 (`.jj` と `.git` 並存時は jj 優先) か `--vcs jj|git|auto` で強制。`vcs tag` に `list` verb はない (`git tag --list` / `jj tag list` を使う)。`vcs get commit-id` は `--rev` (default `@`/`HEAD`) 時点の 40-char SHA を返す。

### 入力モード (FILE | VER | `-` | `vcs:` | `cmd:`)

**version INPUT** (bump/read/compare アクションの位置引数) は以下の優先順で解決される (DR-0006 確定論点 B、`vcs:` は DR-0008、`cmd:` は DR-0014):

1. `-` → stdin から VER を 1 行読み込み (1 引数につき stdin 消費は 1 回まで)
2. `vcs:` で始まる → VCS 経由で解決 (DR-0008、後述)
3. `cmd:` で始まる → shell コマンドを実行して stdout から VER を取得 (後述)
4. ファイルとして存在する → FILE 扱い
5. semver としてパース可能 → VER 扱い
6. それ以外 → エラー

`1.2.3` のようにファイル名と VER 文字列が衝突するケースは `./1.2.3` で明示する (Unix 慣習)。

`glob:<pattern>` と `file:<path>` は version INPUT prefix では **ない**。これらは別レイヤ — SOURCE パターン (`--define-rule` 用) と PATH-list セレクタ (`vcs diff` / `vcs commit` / `vcs outdated` / `--excludes` 用) である:

- `glob:<pattern>` (doublestar, DR-0024) はソースファイル集合にマッチする。glob 挙動は `--glob-dotfile` / `--glob-gitignored` / `--glob-ignorecase` で調整可能。
- `file:<path>` (DR-0033) は newline 区切り path list を読む (各行は literal path か `glob:`)。`#` コメント・空行はスキップ、nested `file:` は reject。

#### `vcs:` 入力 (DR-0008)

`vcs:REV[:FILE]` は jj/git の `<REV>` 時点の `<FILE>` 内容を取得する。VCS は以下の優先順で自動判定: `--vcs jj|git` フラグ (`auto` / 未指定は次へ) → `.jj` ディレクトリ存在 → `.git` ディレクトリ存在。`.jj` と `.git` が並存する (jj colocate モード、kawaz の git-bare + jj-workspace 構成) 場合は **jj が優先** (DR-0016)。

> **latest-tag / latest-release の取得** は入力 record と subcommand の両経路を提供する (DR-0032):
>
> - 入力 record (`vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])`): スカラ返却、1-liner ergonomic 向け。prerelease は除外固定 (stable only)。`compare gt VERSION 'vcs:latest-tag()'` のような 1 行記述に最適。
> - Subcommand (`vcs get latest-tag` / `vcs get latest-release`): richer option (`--include-prerelease`、`--json` で構造化 version schema、`--repository REPO`) を備える。CI / release workflow の shell pipeline 向け。`latest-release` は `gh` 経由で GitHub Releases を読むため PATH に `gh` が必要。
>
> source 軸 (tag list か GitHub Release か) は **verb 名に畳んで** flag にしない設計 (DR-0032)。monorepo-style tag (`pkf-tasks@0.0.12`) の `@`-peel fallback と第三者書き込み可能 remote の信頼境界 (DR-0019) は両経路に引き継がれている。

FILE 省略時は **位置順で最初の FILE 提供 sibling** から借用 (実 FILE 起源 or 他の `vcs:REV:FILE`)。借用源がない場合はエラー。

`bump-semver` は `git fetch` / `jj git fetch` を自動実行しない。古い remote の場合は VCS のエラーがそのまま伝わる。`vcs:` 入力が混ざる invocation での `--write` はエラー (vcs: は read-only)。

#### `cmd:` 入力

`cmd:<shell-command>` は `<shell-command>` を `bash -c` で実行し、stdout の最初の非空行を VER として取得する。leading `v` は strip され、SemVer 2.0.0 として parse される。`vcs:` と同じく **read-only** (`--write` には FILE が最低 1 つ必要)。child は専用 process group で実行され、30 秒の hard timeout で group 全体を kill、stdout / stderr は cap (64 KiB / 4 KiB) されて暴走コマンドを防ぐ。

主な用途は「ビルド済みバイナリの `--version` 出力」と「version files」を 1 つの `bump-semver` 呼び出しで横断比較するケース。例えば `bump-semver compare eq VERSION 'cmd:./bin/mytool --version'` で「VERSION の値とバイナリに焼かれた version 文字列が一致しているか」を release gate にできる (= `bump-version` 後の `go build` 忘れを検出)。

エラー伝播: コマンドの exit code 非 0 → child の stderr を含めてエラー、stdout 空 → `command produced no output` エラー、stdout の最初の行が SemVer として parse 不可 → `cmd:<command>: "<line>" is not a valid version` エラー。command 文字列の責任分担は [信頼境界](#信頼境界-dr-0034) を参照。

### カスタムルール (`--define-rule`, DR-0023)

形式判定器は built-in ルールテーブル (後述) を持つが、1 回の invocation で `--define-rule <PATTERN>` により独自ルールを宣言できる。PATTERN は絶対/相対 path、basename、`glob:<pattern>` のいずれかで、ルールブロックを開く。後続の rule-body フラグは次の `--define-rule` までそのブロックに属する:

- `--format` `text|json|yaml|toml|xml` — そのブロックの抽出方式
- `--version-path DOTPATH` / `--version-regex REGEX` — version の所在
- `--name-path DOTPATH` / `--name-regex REGEX` — optional な package-name field

最初の `--define-rule` より前に置いた rule-body フラグは global default block (named block 非カバーの全 SOURCE をカバー) になる。CLI 定義ルールは built-in テーブルを常に override し、CLI ルールの抽出失敗は **hard error** — built-in への silent fall-through はない。

### 引数排他ルール

| 組み合わせ | 動作 |
|---|---|
| `--pre` + `--no-pre` | エラー (排他) |
| `--build-metadata` + `--no-build-metadata` | エラー (排他) |
| `--write` + `get` / `compare` | エラー (read-only / 比較に書き戻しは無意味) |
| `--write` + `vcs:` / `cmd:` 入力 | エラー (これらは read-only 入力) |
| `--write` 指定時に FILE 入力 0 個 | エラー (`--write requires at least one FILE`) |
| `vcs commit -a` / `--all` | エラー (DR-0020 safety: 明示 stage せよ) |
| `vcs push` で `--branch` と `--bookmark` 併用 | エラー (同義語、片方だけ渡す) |
| 複数 INPUT で値不一致 | `version mismatch:` でカラム揃え縦列挙 |
| 単一 FILE INPUT + stdin pipe | FILE は名前ヒント、内容は stdin から (legacy)。空パイプならディスク上の FILE にフォールバック |
| 複数 INPUT 時の stdin pipe | 無視 (cat / sed と同じく明示 INPUT 優先) |
| いずれの違反もない | 正常実行 |

### モジュール構成

Go ソースは `src/` 配下に隔離し、リポジトリ直下にはメタ情報 (README / docs / justfile / VERSION / go.mod 等) のみを置く。`go.mod` 自体はリポジトリ直下のままで、import path / module path は `github.com/kawaz/bump-semver` から変わらない。ビルドは `go build ./src`。

```
.
├── go.mod / go.sum
├── justfile                 build / lint / test / bump-version / push レシピ
├── VERSION
├── README{,-ja}.md
├── CHANGELOG.md
├── docs/
├── tests/
└── src/
```

`src/` のファイルは責務でグループ化される (各グループは数個の `*.go` + 対応する `*_test.go`):

| グループ | ファイル (代表) | 責務 |
|---|---|---|
| CLI / cobra | `cobra_root.go`, `cobra_bump.go`, `cobra_compare.go`, `cobra_vcs.go`, `cobra_values.go`, `cobra_buildargs`, `cobra_help.go`, `cobra_help_text.go`, `cobra_errors.go` | cobra コマンドツリー、フラグ定義、help / `--help-full`、エラー → exit-code マッピング |
| カスタムルール | `cli_define_rule.go`, `cli_dispatch.go`, `cli_types.go`, `rule_resolver.go`, `rule_apply.go` | `--define-rule` パース、CLI ルールの built-in 上書き、解決済みルールの適用 (DR-0023) |
| 形式判定 | `rules.go`, `resolve.go`, `suffix.go` | confidence-ranked ルールテーブル (DR-0005)、入力解決 (FILE/VER/`-`/`vcs:`/`cmd:`)、backup-suffix fallback |
| 形式ハンドラ | `handler.go`, `format_json.go`, `format_toml.go`, `format_text.go`, `format_yaml.go`, `format_xml.go`, `format_xml_element.go`, `format_xml_dotpath.go`, `format_pbxproj.go`, `jsonpath.go` | format 別 Inspect / Replace |
| SemVer コア | `semver.go`, `compare.go`, `json.go` | SemVer 2.0.0 parse / Bump / Compare、`compare` exit code、`--json` 出力スキーマ (DR-0007) |
| VCS レイヤ | `vcs.go`, `vcs_backend.go`, `vcs_cmd.go`, `cmd_vcs_outdated.go`, `cmd_vcs_get_latest*.go` | `vcs` サブコマンド、git/jj backend 抽象、入力バリデータ (DR-0034)、`outdated` / `latest-tag` / `latest-release` (DR-0027/0028/0032) |
| Glob / path list | `glob.go`, `glob_backref.go`, `file_input.go` | `glob:` マッチ + backref (DR-0024)、`file:` path list (DR-0033) |
| `cmd:` 入力 | `cmd_source.go`, `cmd_source_unix.go`, `cmd_source_other.go` | process-group kill / timeout / 出力 cap 付き shell コマンド入力 |
| エントリ / misc | `main.go`, `help.go`, `exit.go` | entrypoint、multi-input 整合性、exit-code 定数 |

テストファイル (`*_test.go`) は各グループの隣に置く。`spec_table_test.go` が SemVer 仕様テーブルを駆動する (DR-0006)。

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

現在サポートしている format は `json`, `toml`, `yaml`, `text` (`format_text.go`: `VERSION` のような全文プレーン内容に加え、DR-0012 の `*.cabal` / `*.spec` / `build.gradle` / `*.xcconfig` 等 1 行 manifest 向け line-anchored 書き換え), `pbxproj` (DR-0015、Xcode の multi-match 同期), `xml` (DR-0015、Apple plist の `<key>/<string>` ペア専用), `xml-element` (DR-0018、slash-rooted XML path lookup。`pom.xml` / `*.csproj` 等で使用。`format_xml_element.go` / `format_xml_dotpath.go`)。`xml` と `xml-element` は意図的に別 format として並列に dispatch する: plist の flat key-value と Maven/.NET の入れ子 element では評価規則が違うため、責務を分離している。

stdin がパイプ **かつ FILE INPUT が 1 個** のときは FILE を「名前ヒント」として上記判定にだけ使い、内容は stdin から読む (legacy ショートカット)。パイプが空 (例: CI step の stdin に配線された writer 不在の FIFO) ならディスク上の FILE 読みにフォールバックし、`--write` も通常通り効く。複数 INPUT のときは stdin pipe を無視してファイルから読む (cat / sed と同じく明示 INPUT が優先)。`-` を INPUT として明示すれば新方式の stdin VER 読込として処理される。

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

`compare <OP> <BASE> <OTHER...>` は SemVer 2.0.0 § 11 順序仕様準拠で、各 OTHER を BASE に対して独立に比較する:

1. MAJOR/MINOR/PATCH 数値比較
2. pre-release あり < 同 base の確定版 (`1.0.0-rc.1 < 1.0.0`)
3. pre-release 同士は識別子比較 (数値 vs 数値は数値順、英数字 vs 英数字は ASCII 順、数値 < 英数字)
4. build metadata は順序比較から完全に除外 (`1.0.0+a == 1.0.0+b`)
5. prefix / sep の違いは正規化 (`v1.2.3` == `1.2.3` == `version_1_2_3`)

各 INPUT は bump 系と同じ FILE/VER/`-`/`vcs:`/`cmd:` 解決ロジックで解決され、複数 version field を持つ INPUT (例: `package-lock.json`) は内部で整合性検証 → 1 値に集約してから比較に渡す。

終了コード:
- `0` = 真 (全 OTHER について)
- `1` = 偽 (失敗した OTHER を全部 stderr に列挙、short-circuit しない)
- `2` = エラー (パース失敗、整合性 NG、未対応ファイル等)

これは `test` / `dpkg --compare-versions` 慣習に揃えた (DR-0006 確定論点 A)。bump も compare もエラーは exit `2` 扱いなので、スクリプトは `$? -eq 1` でなく `$? -ne 0` で分岐する。

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

## 信頼境界 (DR-0034)

`bump-semver` は 2 通りの方法で外部プログラムを実行し、責任分担も 2 通り異なる:

- **`cmd:` 入力** は任意 shell コマンドを `bash -c` で実行する。command 文字列は呼び出し側の責任 — 自動化は信頼できない入力を連結してはならない。child は専用 process group で動き timeout で group 全体を kill、出力も cap されるが、コマンドの *内容* は設計上信頼される。
- **`vcs` サブコマンドと `vcs:` 入力** はユーザ由来の rev / tag NAME / remote / repository 値を `git` / `jj` / `gh` の argv に渡す。これらは `exec.Command` (no `sh -c`) を経由するためシェルメタ文字は無害だが、`-` 始まりの値はフラグとして解釈されうる。入力は **dispatch 入口と `vcs:` resolver** という単一の choke point で検証され、backend 関数は検証済みの値だけを受け取る:

  | バリデータ | 対象 | 拒否 |
  |---|---|---|
  | `validateUserRev` | rev 引数 (`vcs diff` / `vcs tag push` / `vcs get commit-id` / `vcs:REV`) | `-` 始まり |
  | `validateRemote` | remote 名 (`fetch` / `push` / `tag *`) | 空 / `-` 始まり / 空白含み |
  | `validateGhRepo` | `gh -R` 用 repo | `owner/repo` 形式でない、`-` 始まり、空白含み |
  | `validTagName` | tag NAME | `-` 始まり (既存規則 + 追加) |
  | `expandRepoArg` | `vcs:latest-tag` の repo arg | `-` 始まり |

  guard は意図的に最小限 (`-` 始まりのフラグ注入だけ block)。URL スキーム allowlist の全面導入は **採用しない** — remote URL の正当性は呼び出し側責任、`git ls-remote` のパースエラーがより正確、という DR-0019 / DR-0032 の設計判断を維持するため (allowlist は正当な非 GitHub remote を誤拒否しうる)。

## 配布

### リリースフロー

```
just bump-version [patch|minor|major]
  ↓ ensure-clean、VERSION 書き換え (bump-semver --write)、vcs commit
just push
  ↓ ci + check-outdated-translations + check-version-bumped ゲート → vcs push --branch main
GitHub Actions (.github/workflows/release.yml) が VERSION 変化を検出 (on: push, paths: [VERSION])
  ↓
Linux / macOS / Windows × amd64 / arm64 の 6 ターゲットでビルド
  ↓
gh release create v<VERSION> --target <sha> --generate-notes でタグ + Releases ノートを自動作成
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
