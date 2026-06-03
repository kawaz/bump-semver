# Issue: format=regex 概念廃止 → format=text + version-regex 統合

- Status: Draft (議論段階 / Decision 未確定)
- Date: 2026-06-03
- Related: DR-0012 (regex format builtin、本 issue で partial supersede 予定), DR-0001 (basename 自動判定 / format 列), DR-0005 (confidence ranked candidates / format 列)
- Sister issue: `2026-06-03-cli-user-defined-rule.md` (= 別 DR、CLI から rule 指定する口。本 issue 実装後の format enum を前提に書かれる)

## Context

bump-semver の builtin format 列 (DR-0001/0005 confidence の `format` フィールド) は現在
`text` / `json` / `yaml` / `toml` / `xml` / **`regex`** の 6 値を持つ (DR-0012 で `regex` 追加)。
だが `regex` は **format 名として概念的に間違い** (kawaz 指摘、2026-06-03):

- `format` は本来 **「ファイル構造の分類」** を表すラベル (text / json / yaml / toml / xml の 5 値)
- `regex` は **「抽出手段」** であって、構造分類ではない
- 既存 `format: regex` の builtin (= podspec / nimble / v.mod / build.zig.zon / mix.exs /
  build.sbt / xcconfig / gemspec / cabal / spec / build.gradle / build.gradle.kts) は
  実態として **「format=text の file から regex で 1 行抽出」** している。「format=text +
  VersionRegex」 と 1:1 で書ける

姉妹 issue (`2026-06-03-cli-user-defined-rule.md`) で CLI に `--format` flag を露出する
ことが議論されており、その flag enum が `<text|json|yaml|toml|xml|regex>` だと
**「regex は format か? 抽出手段か?」** の二重定義が外部 API に固定化される。本 issue
ではその前に **internal 表現を統合** し、format enum を 5 値に整理する。

### 概念モデル (= 統合後)

| 軸 | 値 | 役割 |
|---|---|---|
| **format** | `text` / `json` / `yaml` / `toml` / `xml` | ファイルの構造分類 (parser がどれを使うか) |
| **抽出手段 1: version-path** | dot-path string (optional) | 構造化 file (json/yaml/toml/xml) で値の場所を指定 |
| **抽出手段 2: version-regex** | regex string (optional) | text format で必須、構造化 format では path 値への 2 段抽出として併用可 |
| **name 抽出** | name-path / name-regex | 上記と対称 (optional) |

組合せの可否:

| format | path 必須? | regex 必須? | 備考 |
|---|---|---|---|
| `text` | × (= 使えない) | ○ | path 概念なし、regex 必須 |
| `json` / `yaml` / `toml` / `xml` | ○ (default の抽出経路) | × (= path 値への 2 段適用 or 全文 regex として併用可) | DR-0029 § "Path / Regex 併記時の挙動" の表に従う |

## Goal

1. **internal CandidateRule の `Format = "regex"` を `Format = "text" + VersionRegex = "..."` に
   書き換え** (= 機能変更なし、refactor のみ)
2. **format enum を 5 値** (`text` / `json` / `yaml` / `toml` / `xml`) **に縮約**
3. `--help-full` / README の **「Supported file formats」** 表記を `text + version-regex`
   形式に書き換え (= 「`*.podspec   regex   ...`」を「`*.podspec   text   version-regex=...`」
   等に)
4. DR-0012 を **partial supersede** (= 「regex format 抽象」という概念は廃止、ただし
   builtin に regex 抽出 file 群を持つこと自体は残る、機能は維持)

## 影響範囲

### 内部コード (= 2026-06-03 実機確認、bump-semver 0.x 時点)

- `src/rules.go`:
  - `CandidateRule.Format` field の comment 更新 (現状 `"json", "toml", "plain"`
    と記述、`"regex"` 言及なし → DR-0030 後は `"text" | "json" | "yaml" | "toml" |
    "xml"` に統一)
  - `CandidateRule.VersionRegex` / `NameRegex` field comment 更新
    (`Format == "regex"` 前提の記述 → `Format == "text"` ベース + json/yaml/toml/xml
    で path 値への 2 段抽出として併用可、に書き換え)
  - `Format: "regex"` のリテラル 12 箇所 (L250, 262, 272, 282, 296, 307, 325, 335,
    346, 356, 368, 380) を `Format: "text"` に置換
- format dispatcher: `src/rules.go` 内の `case "regex":` (= L567, L590 の 2 箇所) を
  削除し、`case "text":` のロジックで VersionRegex を見るように統合
- 既存の `format_regex.go` / `format_plain.go` 内の処理は **どちらかに統合 or 改名**:
  - 名前としては `format_text.go` が新概念 (= 「format=text かつ VersionRegex あり/なし」)
  - `format_plain.go` は現状の `Format: "plain"` 専用 (VERSION ファイル)、
    `format_regex.go` は `Format: "regex"` 専用 → 統合先は実装時判断
- 影響 file 群 (DR-0012 / DR-0018 で `Format: "regex"` で登録されたもの):
  - `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt`
  - `*.xcconfig` / `*.gemspec` / `*.cabal` / `*.spec`
  - `build.gradle` / `build.gradle.kts`

### 外部 API / ユーザ目視

- `--help-full` の "Supported file formats" 表記 (= ユーザは format 名が変わる)
- README / README-ja の format 表記
- `--json` 出力に format 名を含む箇所があれば変更 (= 要確認)
- CHANGELOG / UPGRADING で記録 (= 表記変更を案内、機能は不変)

### テスト

- 既存 fixture テスト (= 各 file の get / bump の挙動) は変更不要 (= 機能不変)
- 内部表現を assert しているテストがあれば書き換え

## 実装時の論点: `format=plain` との関係 (= 2026-06-03 実機確認で浮上)

src/ には既存 format として **`plain`** も存在 (= VERSION ファイル用、全文 = version)。
DR-0030 の対象 `regex` と並んで「構造化なし」系の format で、概念的に近い:

- `format=plain` = VersionRegex 無し、全文を version として読む
- `format=regex` (廃止対象) = VersionRegex 必須、regex で抽出

新世代の `format=text` への統合方針 2 案:

- **案 A** (= 概念整理最大): `plain` も `text` に統合、`format=text` のみ。VersionRegex の
  有無で挙動分岐 (= 無しなら全文、有りなら regex)。format enum は 5 値 (`text|json|yaml|
  toml|xml`)
- **案 B** (= DR-0030 スコープ最小): `plain` は残す、`regex` のみ廃止。format enum は
  6 値 (`text|plain|json|yaml|toml|xml`)。「`text` は regex 必須、`plain` は全文」と
  概念分離

姉妹 issue (DR-0029) の `--format` enum 5 値 (kawaz 指示) は **案 A** に沿う。
**実装時推奨は案 A** (= 概念整合性最大、CLI で見せる format 数を最小化)。ただし案 A は
`format=plain` の builtin (= VERSION ファイル handler) も text に書き換え必要 (= 影響範囲
やや増)。Phase 1 実装着手前に案 A/B を確定する。

## 非影響範囲

- 機能挙動は完全に不変 (= 同 file への同 input で同 output)
- `bump-semver get` / `compare` / `bump` の CLI 表面 (positional / option) は不変
- `--json` 出力の **version 値・name 値** は不変 (= format 名 field だけ表記変更)
- DR-0012 で確立した「1 ファイル 1 マッチ / line-anchored / first match only」規約は
  そのまま維持 (= text format でも同規約が適用される、概念整理だけ)

## Phase 分割

### Phase 1 (= 本 DR で確定 + 実装)

1. internal CandidateRule の format 列挙を 5 値に
2. 既存 `format: regex` builtin を `format: text + VersionRegex` に書き換え
3. format dispatcher を text + version-regex 経路に統合
4. help / README 表記更新
5. CHANGELOG / UPGRADING に記録
6. 既存テスト全 pass を確認

### Phase 2 (= 別 DR、本 issue 範囲外)

- (本 DR で扱わない) — DR-0029 (CLI から rule 指定) の前提として **format enum 5 値**
  が確定したことを宣言するだけ

## 関連 DR / Issue

- **DR-0012**: regex format 抽象 → 本 issue で partial supersede (= 「format=regex」
  概念の廃止、ただし regex 抽出 builtin file 群は維持)
- **DR-0001**: flat 4-action + basename ベース判定 → format 列の意味を再整理
- **DR-0005**: path-aware confidence ranked candidates → format 列の値 5 値化
- **DR-0018**: JVM / .NET / Haskell / RPM 対応で追加した `xml-element` / regex format
  → xml-element はそのまま、regex 系は本 DR で text に統合
- **姉妹 issue** `2026-06-03-cli-user-defined-rule.md` (DR-0029 候補): CLI に `--format`
  flag を露出するため、本 DR の format enum 5 値化が前提

## Implementation order

`DR-0030 (本 issue 確定後)` → `DR-0029 (CLI から rule 指定)` の順で実装。理由:

- DR-0029 の `--format` enum が「regex 含まない 5 値」で確定的に書ける
- 「format=regex 残骸」を意識せずに DR-0029 の help / docs を書ける
- DR-0029 実装中に「format=text + version-regex」を扱う共通経路が既に
  internal に整備されている (= CLI rule もその経路に乗るだけ)

## Next action

- 本 issue の方針 (= format=regex 廃止) が確定したら `DR-0030-format-regex-to-text-unification.md`
  として確定 DR を起票
- 実装 1 PR で完了想定 (= 機能変更なしの refactor、テスト不変)
- 完了後、姉妹 issue (DR-0029 候補) に進む
