# DR-0030: format=regex 概念廃止 → format=text + version-regex 統合

- Status: Active
- Date: 2026-06-03
- Partially supersedes: DR-0012 (regex format builtin) — 「`regex` という format 名」と「`format == "regex"` の dispatcher 経路」を廃止。**regex 抽出を builtin に持つ機能自体は維持** (= 既存の `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt` / `*.xcconfig` / `*.gemspec` / `*.cabal` / `*.spec` / `build.gradle` / `build.gradle.kts` の挙動は不変)
- Related: DR-0001 (basename 自動判定 + format 列), DR-0005 (confidence ranked candidates + format 列), DR-0018 (xml-element format)
- Prerequisite for: DR-0029 (CLI から rule 指定。`--format` enum が本 DR の statement に依存)

## Context

bump-semver の internal `CandidateRule.Format` は現在 `"plain"` / `"json"` / `"toml"` /
`"yaml"` / `"xml"` / `"xml-element"` / **`"regex"`** の混在状態。DR-0012 で
`"regex"` を導入したが、これは概念的に **次元が違う値** が同じ列挙に混ざっている:

- **format** = 本来「ファイル構造の分類」(= どの parser を使うか) のラベル
- **regex** = 「抽出手段」(= 構造化 parser を使わず regex で 1 行抜く)

実態として `Format: "regex"` の builtin (= podspec / nimble / v.mod / build.zig.zon /
mix.exs / build.sbt / xcconfig / gemspec / cabal / spec / build.gradle 系) は全て
**「format=text の file から regex で 1 行抽出」** している。「format=text +
VersionRegex」 と 1:1 で書ける。

DR-0029 (CLI から rule 指定) で `--format` flag を CLI 表面に露出するにあたり、
enum に `regex` を残すと:

- 「`--format text` (= text 全文を regex で抽出) と `--format regex` (= 旧 builtin の
  regex format) で何が違うのか」が user-facing の二重定義になる
- help / docs の builtin 表で `regex` という format 列が出るが、それは内部実装ラベル
  であって user が選ぶ format ではない
- 将来 `--format` enum を拡張するときに「format = 構造分類」の axis が崩れている

→ DR-0029 を確定する前に internal 表現を統合し、format enum を「構造分類」だけに
絞る。これは DR-0012 の機能を残したまま、命名と概念を整理する refactor。

## Decision

### 概念モデル

| 軸 | 値 | 役割 |
|---|---|---|
| **format** | `text` / `json` / `yaml` / `toml` / `xml` / `xml-element` | ファイルの構造分類 (= parser 選択) |
| **VersionRegex** | regex string (optional) | text format で **必須**、構造化 format で **path 値への 2 段抽出 or 全文 regex** として併用可 |
| **VersionPaths** | path string list (optional) | 構造化 format で値の場所を指定 |
| **NameRegex** / **NamePaths** | 上記と対称 (optional) | name 抽出用 |

`format=text` の subtype:

- `VersionRegex` 無し → 全文を version 文字列として読む (= 旧 `format="plain"` 相当、
  VERSION ファイル handler)
- `VersionRegex` 有り → regex で抽出 (= 旧 `format="regex"` 相当、podspec 等)

これにより `plain` も `text` に統合され、enum 値は **6 → 5** に整理される
(text / json / yaml / toml / xml / xml-element、内 xml-element は DR-0018 由来で残存)。

### Internal refactor 範囲

`src/rules.go` の以下を書き換える:

1. **`CandidateRule.Format` field の comment** (現状 `"json", "toml", "plain"` 記述
   + `VersionRegex` field comment が `Format == "regex"` 前提)
   - 新 comment: `"text" | "json" | "yaml" | "toml" | "xml" | "xml-element"` に統一
   - `VersionRegex` / `NameRegex` の説明を「text format で抽出する場合 + 構造化 format
     で path 値への 2 段抽出として併用可」に書き換え

2. **builtin rule literals**:
   - `Format: "regex"` のリテラル 12 箇所 (= L250, L262, L272, L282, L296, L307, L325,
     L335, L346, L356, L368, L380) → `Format: "text"` に置換
   - `Format: "plain"` (= VERSION handler) も `Format: "text"` に統合 (= `VersionRegex`
     無しなら全文読み)

3. **dispatcher の switch case**:
   - `case "regex":` (= L567, L590 の 2 箇所) を削除
   - `case "text":` で **`VersionRegex` の有無で分岐** する経路に統合
     - 有り → 旧 regex 経路 (= first-match-only / line-anchored 等の DR-0012 規約を維持)
     - 無し → 旧 plain 経路 (= 全文 = version)
   - `case "plain":` も `case "text":` に統合

4. **ファイル整理**:
   - `format_regex.go` の処理を `format_text.go` (新規 or rename) に移動、
     `VersionRegex` 有無で分岐
   - `format_plain.go` の処理も同 `format_text.go` に統合
   - 既存 test (= `format_regex_test.go` 等) は新名称に追従

### 機能不変性

機能挙動は **完全不変**:

- 同 file への同 input で同 output (= 全 builtin rule の挙動が同じ)
- CLI 表面 (positional / option) は本 DR で変更なし
- `--json` 出力の version 値・name 値は不変
- DR-0012 で確立した「1 ファイル 1 マッチ / line-anchored / first match only」規約は
  text + version-regex 経路でそのまま維持

ユーザ目視で変わるのは:

- `--help-full` の "Supported file formats" 表記 (= `regex` 表記が `text +
  version-regex` 形式になる、ただし表記方針は DR-0029 で「組み込みルール一覧」表に
  整理し直す)
- README / README-ja の format 表記 (= 同上)
- CHANGELOG / UPGRADING に「内部 format 名の整理 (= 機能不変)」として記録

### Implementation order

1. `src/rules.go` の field comment + 12 literal + dispatcher switch case を一括書き換え
2. `format_text.go` を新規作成 (= `format_regex.go` + `format_plain.go` 統合、
   VersionRegex 有無で分岐)
3. 旧 `format_regex.go` / `format_plain.go` を削除、test を新名称に rename
4. 既存 test 全 pass を確認 (= 機能不変なので全 green が期待)
5. `--help-full` / README の format 表記更新 (= DR-0029 の help 方針と並行更新)
6. CHANGELOG / UPGRADING に記録

DR-0029 (CLI rule) は本 DR 完了後に実装着手する。

## Consequences

### Positive

- format enum の axis が「構造分類のみ」に整理され、DR-0029 の `--format` flag が
  二重定義なく公開できる
- `plain` / `regex` の二重命名問題が解消
- 将来 format 追加時 (= 例: ini / properties 等) の判断基準が明確 (= 「構造分類か否か」
  だけで決まる)

### Negative

- DR-0012 で「regex format」と呼んでいた概念が消える (= 過去 doc / commit 参照時に
  名前 mismatch)。CHANGELOG / UPGRADING で「rename only、機能不変」を明示
- internal refactor で `format_regex.go` / `format_plain.go` の git blame が一段
  noisy になる (= 統合先 `format_text.go` への移動が記録される)

### Neutral

- ユーザ目視の挙動変更なし、上記 Negative は doc 上の話のみ

## Related DR / Issue

- DR-0001: flat 4-action + basename ベース判定 → format 列の意味を再整理
- DR-0005: path-aware confidence ranked candidates → format 列の値整理
- DR-0012: regex format 抽象 → 本 DR で partial supersede (= 命名と概念のみ、機能維持)
- DR-0018: JVM / .NET / Haskell / RPM 対応で追加した `xml-element` format → そのまま
  維持 (= 構造分類として有意義、本 DR の対象外)
- DR-0029 (= 並行起票): CLI から rule 指定。本 DR の format enum 5 値 (xml-element を
  CLI 外として扱う場合は 4 値) を前提に CLI 表面を設計
