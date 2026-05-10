# DR-0012: regex format 抽象 + xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt 対応

- Status: Accepted
- Date: 2026-05-11
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」)、DR-0005 (path-aware confidence ranked candidates)、DR-0010 (confidence 1 fallback hint)、DR-0011 (yaml/yml/toml fallback で同じ手法を先行採用)

## Context

DR-0011 までで JSON / TOML / YAML / plain の 4 形式に対応した。残る ROADMAP の「未対応フォーマット候補」(README が指している `*.gemspec` / `mix.exs` / `build.sbt` 等) は、いずれも **「ある決まった行に regex で 1 箇所だけ書かれた version 文字列を読み書きする」** という性質を持つ:

| 言語 / ツール | 典型的な version 行 |
|---|---|
| Xcode build config (`*.xcconfig`) | `MARKETING_VERSION = 1.2.3` |
| CocoaPods (`*.podspec`) | `s.version = "1.2.3"` |
| Nim (`*.nimble`) | `version = "1.2.3"` |
| V (`v.mod`) | `version: '1.2.3'` |
| Zig (`build.zig.zon`) | `.version = "1.2.3"` |
| Ruby gem (`*.gemspec`) | `s.version = "1.2.3"` |
| Elixir (`mix.exs`) | `version: "1.2.3"` |
| Scala SBT (`build.sbt`) | `version := "1.2.3"` |

これらは単純な構造化フォーマットではない (Ruby DSL / Scala DSL / Nim NimScript / Zig ZON 等の **言語ソース**)。完全パーサを書くのは過剰投資で、**1 行 regex** で十分。一方、形式ごとに `format_xcconfig.go` / `format_podspec.go` / `format_nimble.go` ... を 8 個並べるのは DR-0005 の「ルール 1 行追加で済ませる」精神に反する (handler 乱立、`/simplify` 候補)。

JSON / TOML / YAML 各 format はそれぞれ専用 parser に強く依存しているので分離してきたが、上の 8 形式は **「言語非依存・regex 1 個・行ベース 1 箇所書き換え」** という共通骨格を 100% 共有する。であれば抽象は format ではなく rule のフィールドで持たせるのが筋。

## Decision

### 1. 新 format `regex` を導入

`CandidateRule` に regex 系のフィールドを 2 つ追加し、Format `"regex"` のとき採用される汎用「行ベース regex で 1 箇所書き換える」フォーマットを新設する。

```go
type CandidateRule struct {
    // ... (既存フィールド)

    // regex format 専用 (Format == "regex" のとき)。version 抽出 +
    // 書き換え用 regex。最初の `(...)` capture group が version 値の
    // 範囲となり、その範囲を新 version 文字列で置換する。
    VersionRegex string
    // regex format 専用 (optional)。1 capture group が package name 値。
    // 不在でもルール失敗にはならない (JSON/TOML/YAML format と同じ扱い)。
    NameRegex string
}
```

実装は `src/format_regex.go` 新規 1 ファイルで完結:

- `regexInspect`: `regexp.Compile(rule.VersionRegex)` → `FindSubmatchIndex` で最初のマッチを取り、submatch[2:4] (= 1 番目の capture group) を version とする
- `regexReplace`: 同様に最初のマッチの 1 番目の capture group の byte range だけを新 version で書き換え (前後の引用符 / コメント / 行末は保持)
- name は optional で `NameRegex` が非空なら同様に抽出 (extraction 失敗はルール失敗ではなく「name 不在」として握りつぶす)

`tryRule` / `formatReplace` の switch に `"regex"` arm を 1 つ追加。

### 2. 新規対応ファイル (本リリース)

| confidence | パターン | format | VersionRegex | NameRegex | 備考 |
|---|---|---|---|---|---|
| **2** (basename) | `v.mod` | regex | `(?m)^\s*version\s*:\s*'([^']+)'` | `(?m)^\s*name\s*:\s*'([^']+)'` | V 言語 |
| **2** | `build.zig.zon` | regex | `(?m)\.version\s*=\s*"([^"]+)"` | `(?m)\.name\s*=\s*\.?(?:@?"([^"]+)"\|([A-Za-z_][A-Za-z0-9_]*))` (※未採用、下記参照) | Zig ZON |
| **2** | `mix.exs` | regex | `(?m)version:\s*"([^"]+)"` | (なし、Elixir DSL は app: で名前を分離) | Elixir |
| **2** | `build.sbt` | regex | `(?m)^\s*version\s*:?=\s*"([^"]+)"` | (なし、SBT DSL は name := で別管理) | Scala SBT |
| **1** (glob) | `*.xcconfig` | regex | `(?m)^\s*MARKETING_VERSION\s*=\s*([^\s;/]+)` | (なし) | Xcode build configs |
| **1** | `*.podspec` | regex | `(?m)^\s*(?:s\|spec)\.version\s*=\s*['"]([^'"]+)['"]` | `(?m)^\s*(?:s\|spec)\.name\s*=\s*['"]([^'"]+)['"]` | CocoaPods |
| **1** | `*.nimble` | regex | `(?m)^\s*version\s*=\s*"([^"]+)"` | (なし、Nim は dir 名から推測) | Nim |
| **1** | `*.gemspec` | regex | `(?m)^\s*(?:s\|spec)\.version\s*=\s*['"]([^'"]+)['"]` | `(?m)^\s*(?:s\|spec)\.name\s*=\s*['"]([^'"]+)['"]` | Ruby gem |

`build.zig.zon` の name 抽出は形式のバリエーションが大きすぎる (識別子型 / 文字列型 / `@"..."` / enum literal `.foo`) ため本 DR では NameRegex 未採用。version だけ抽出する。

basename 一致 (`v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt`) は **confidence 2** で扱う。glob しか持たない 4 つ (`*.xcconfig` / `*.podspec` / `*.nimble` / `*.gemspec`) は **confidence 1** で fallback hint も出る。

### 3. 1 ファイル 1 マッチ

regex format は **「最初のマッチ 1 個」だけ** を扱う。`project.pbxproj` の build settings のように同一ファイル内で複数の version 行を **同期更新** する必要があるケースはスコープ外。実需が出た時点で `format_pbxproj.go` のような専用 format を別途追加する (DR-0005 の精神: ファイル種別ごとに必要なら新 format 追加)。

これは設計上の制限ではなく**意図的なシンプルさ**: `*.podspec` で `s.version = ...` が複数登場する書き方は実質存在せず、xcconfig も `MARKETING_VERSION` は 1 ファイルに 1 個が普通。複数同期が必要な実例 (Xcode の `*.pbxproj` の `MARKETING_VERSION = ` 群、`Info.plist` の `CFBundleShortVersionString` + `CFBundleVersion` の二重管理) は本 DR で扱わない。

### 4. Mojo (`mojoproject.toml`) は ROADMAP 行きで未対応

Mojo の `mojoproject.toml` は TOML の `[workspace] version = "..."` 形式。これは TOML の section-scoped version で、DR-0011 の top-level fallback では拾えない。`*.toml` fallback のスコープ外 (top-level のみ)、かつ section-scoped TOML の Replace は v0.9.0 では未実装。

実需が薄く (Mojo は 2026 時点でも early stage)、`pyproject.toml` の `[project].version` 等と一括で「TOML section-scoped 対応」を別 DR で扱うのが筋。本 DR では ROADMAP の「未対応フォーマット候補」に追記するだけにとどめる。

### 5. 既存 format との precedence

`*.xcconfig` / `*.podspec` 等の glob は既存 fallback (`*.json` / `*.yaml` / `*.yml` / `*.toml`) と拡張子が **被らない**。`v.mod` / `build.zig.zon` / `mix.exs` / `build.sbt` は basename 一致なので拡張子衝突の問題なし。

`build.zig.zon` は ZON 拡張子だが、`*.zon` glob は本 DR で導入しない (basename `build.zig.zon` で十分)。

### 6. テスト戦略

- `src/format_regex_test.go` 新規: 各形式の Inspect / Replace を inline 文字列で網羅
  - 典型例 (素直な `version = "1.2.3"`)
  - quote style 違い (single/double/unquoted)
  - コメント付き行
  - 複数 version-like 行 (最初のマッチが採用される)
- `handler_test.go` の `TestDetectHandler_*` 系列を拡張し、新規 8 形式が detectHandler を通ることを確認
- DR-0010 fallback hint は confidence 1 (`*.xcconfig` / `*.podspec` / `*.nimble` / `*.gemspec`) で発火する。既存の hint インフラを再利用 (`emitFallbackHints` の MatchedGlob ロジックは format に依存しない)

## 不採用案

### A. 既存 plain format の拡張

`format_plain.go` に regex オプションを後付けする案。plain は「ファイル全体 = version」というシンプルな responsibility を持つ format で、regex を混ぜると plain の責務が二重化する。新 format `regex` で分離するほうが clean。

### B. format ごとに `format_xcconfig.go` / `format_podspec.go` ... を分割

ファイル数が 8 個増えるだけで、コード本体は regex 1 個 + 行ベース置換ロジックのコピペになる。`/simplify` で必ず統合される将来が見える。最初から `format_regex.go` 1 個でやる。

### C. `CandidateRule` に Inspect / Replace の closure を持たせる (function dispatcher 化)

DR-0005 の「現在は宣言的テーブルを維持、format 数 10+ 時に再評価」の判断 (ROADMAP 「dispatch 構造の再評価」) に従い、本 DR ではまだ早い。format 数は本 DR 後に 5 (json/toml/yaml/plain/regex) で、regex format が 1 個で 8 ファイル形式を吸収するため switch 拡張は最小限。

### D. project.pbxproj / Info.plist の複数同期更新を本 DR で扱う

「同一ファイル内で複数行を一括書き換え」という別軸の責務。regex format の単純さを壊す。実需 (Xcode iOS アプリで `bump-semver patch *.xcconfig project.pbxproj Info.plist --write`) が出た時点で:

- `*.pbxproj` 用に専用 format (複数 build settings 同期更新)
- `Info.plist` 用に XML/PLIST format

をそれぞれ別 DR で立てる。本 DR の `regex` format は「1 ファイル 1 マッチ」で固定する。

### E. 言語ごとに完全パーサ (Ruby AST / Scala AST / Nim AST 等)

ROI に見合わない。1 行 regex で実需 95% カバーできる。エッジケースは利用者が手で直す or issue 経由で path-pinned ルール昇格。

### F. NameRegex を全形式で必須化

mix.exs / build.sbt の name はそれぞれ `app: :foo` (atom) / `name := "foo"` という別行にあり、`version` 行とは構造が異なる。version 抽出と同じ regex 体系で扱うとルール定義が肥大化。`NameRegex` は **optional** とし、省略形式 (NameRegex なし) を許容する。これは JSON/TOML/YAML format で `NamePaths` が 0 個でも許容されるのと整合する。

## Consequences

### 実装変更

- `src/rules.go`: `CandidateRule` に `VersionRegex` / `NameRegex` 追加 + 新規 8 ルール
- `src/format_regex.go` 新規: regexInspect / regexReplace
- `src/handler.go` の switch (`tryRule` / `formatReplace`) に `"regex"` arm 追加
- `src/format_regex_test.go` 新規: inline 文字列テスト
- `src/handler_test.go` 拡張: 新規 8 形式の detectHandler テスト
- README / README-ja / UPGRADING の更新

### 後方互換

- CLI 表面は完全維持
- これまで `unsupported file:` だった 8 形式が新たに動く (純粋追加)
- 既存の json/toml/yaml/plain ルールは無変更

### v0.8.x → v0.9.0

新フォーマット追加なので minor bump。`UPGRADING.md` に v0.8.x → v0.9.0 セクションを追加。

### ROADMAP

- 「未対応フォーマット候補」から `*.gemspec` / `mix.exs` / `build.sbt` を削除 (本 DR で対応)
- 「未対応フォーマット候補」に `mojoproject.toml` (TOML section-scoped) / `pyproject.toml` (`[project].version`) / `*.pbxproj` (複数同期) / `Info.plist` (XML) を追加または明示

### 次のフェーズ

- TOML section-scoped 対応 (`pyproject.toml` の `[project].version`、`mojoproject.toml` の `[workspace].version`) → 別 DR
- 複数同期更新 format (`*.pbxproj` / `Info.plist`) → 別 DR
- suffix 吸収 (`Cargo.toml.bak` 等、issue: 2026-05-10-suffix-stripped-format-detection.md) → 別 DR

## 関連

- DR-0001: 「必要が出たら 1 行追加」哲学
- DR-0005: confidence ranked dispatcher (本 DR の前提機構)
- DR-0010: confidence 1 fallback hint (本 DR の `*.xcconfig` 等で再利用)
- DR-0011: yaml/yml/toml fallback で同じ「行ベース 1 箇所書き換え」手法を先行採用
- ROADMAP: gemspec / mix.exs / build.sbt を本 DR で消化
