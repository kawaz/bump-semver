# DR-0018: JVM / .NET / Haskell / RPM 系の対応ファイル追加

- Status: Active
- Date: 2026-05-11
- Extends: DR-0012 (regex format), DR-0015 (xml format / Apple plist)

## Context

v0.13 までで対応していた言語エコシステムは Rust / Node.js / Python / Go / Ruby / Elixir / Scala / Nim / Zig / V / Swift (Xcode plist + xcconfig + pbxproj) / Claude plugins。JVM 系 (Gradle / Maven) と .NET (`*.csproj` 等) は基本的な package manifest としてカバーすべき対象だったが未対応だった。加えて Haskell (`*.cabal`) と RPM spec (`*.spec`) も「regex 1 行追加で足りるのに対応が薄かった」群。

これらをまとめて v0.14.0 で追加する。

## Decision

### regex format (DR-0012) の拡張

新規 rule を 4 つ追加 (regex format):

| Rule | パターン | confidence | 備考 |
|---|---|---|---|
| `build.gradle` | `^version\s*=?\s*['"]...['"]` | 2 (basename) | Groovy DSL。`version = '1.2.3'` / `version "1.2.3"` (method-call shorthand) / `version = "1.2.3"` の三形を 1 regex でカバー |
| `build.gradle.kts` | `^version\s*=\s*['"]...['"]` | 2 (basename) | Kotlin DSL。method-call shorthand なしの厳しい形 |
| `*.cabal` | `^version\s*:\s*(...)` | 1 (glob) | Haskell。`cabal-version:` と混同しないよう line-anchored で `version:` だけ拾う |
| `*.spec` | `^Version\s*:\s*(...)` | 1 (glob) | RPM。capital V で `Name:` / `Release:` と区別 |

Name の抽出は cabal と spec で実装 (`^name:` / `^Name:`)。Gradle は `rootProject.name = ...` が settings.gradle 側にあって build.gradle には現れないため省略。

### xml format の一般化 (新 format `xml-element`)

DR-0015 で導入した `xml` format は Apple plist の `<key>NAME</key><string>VALUE</string>` ペア専用。Maven の `pom.xml` や .NET の `*.csproj` は別構造 (「element の path で値を指定」) なので、既存 `xml` を拡張するのではなく新 format `xml-element` を導入する:

- path 構文: `/project/version`, `/Project/PropertyGroup/Version` のようなスラッシュ階層
- XML 名前空間は **local name で比較** (Maven の `xmlns="http://maven.apache.org/POM/4.0.0"` も無視)
- 同一 path に複数マッチがある場合は **document order の最初** が勝つ
- 値の抽出と書き換えは `encoding/xml.Decoder` でトークンを歩きながら byte offset を取得し、原文に直接 splice する (xml.Marshal を使わない、DOCTYPE / 属性順序 / インデント保持のため)

新 rule:

| Rule | path | confidence |
|---|---|---|
| `pom.xml` | `/project/version` + name `/project/artifactId` | 3 (basename) |
| `*.csproj` | `/Project/PropertyGroup/Version` | 1 (glob) |
| `*.fsproj` | 同上 | 1 (glob) |
| `*.vbproj` | 同上 | 1 (glob) |

pom.xml の `<parent>/<version>` は path `/project/parent/version` (depth 3) で `/project/version` (depth 2) と区別される。誤って parent の version を bump してしまう事故を path 構文が構造的に防ぐ。

### 既存 `xml` format との関係

| format | 用途 | path 構文 |
|---|---|---|
| `xml` (DR-0015) | Apple plist (`Info.plist`) | bare key name (`CFBundleShortVersionString`) — plist 専用の `<key>/<string>` ペアを探す |
| `xml-element` (DR-0018) | Maven / .NET / 一般 XML | slash-rooted element path (`/project/version`) — depth-first 探索で最初の一致 |

両者は完全に別パスで dispatch される (`rule.Format` で switch)。共存。

## Rationale

### 不採用案

**1. plist (DR-0015) を一般化して `xml-element` を兼ねさせる**

`<key>/<string>` ペア専用ロジックに path 構文を後付けする案。**不採用**: plist 構造は「key-value ペアが flat に並ぶ」一方、Maven / .NET は「element の入れ子で path 階層がある」。前提が違うので両者を 1 つの format に押し込めると分岐だらけになる。format 名で責務を分けたほうが読み手にとって明快。

**2. Gradle multi-module project の sub-project version を扱う**

`settings.gradle` で `include 'subA', 'subB'` 等で複数 module を持つプロジェクトに対応する案。**不採用**: 各 subproject の version は独立して bump したいケースがほとんどなので、それぞれの `subA/build.gradle` を直接 bump-semver にかけるのが筋。`bump-semver patch subA/build.gradle subB/build.gradle --write` のような複数 FILE 呼び出しで対応できる (DR-0004 の name 整合性検査は副次的に効く)。

**3. Maven `<modules>` を再帰的に追う**

`pom.xml` 内の `<modules>` 要素を解釈して子 pom を巡回する案。**不採用**: bump-semver の責務は「1 ファイル / 1 グループのファイル群を bump」。再帰巡回は build tool 側 (Maven 自身) の仕事。

**4. cabal の `cabal-version:` を捕まえる**

`cabal-version: 2.4` も version-shaped なので捕まえる案。**不採用**: これは Cabal 仕様の version であって package の version ではない。regex を line-anchored にして `version:` (cabal-version とは別の field) だけ拾うことで構造的に区別する。

**5. .NET の `<AssemblyVersion>` / `<FileVersion>` も対応**

MSBuild project には Version 以外に AssemblyVersion / FileVersion / InformationalVersion など複数 version 系 element がある。**不採用 (MVP として)**: 1 ファイル内で複数 version field を bump する設計は今の Handler interface の Versions / Names モデルではややぎこちない (全部一致を要求するため)。`<Version>` が代表的かつ NuGet パッケージのデフォルト動作で他 field の base になるので、まずこれだけ対応。AssemblyVersion 等は将来要望あれば別 DR で追加。

### 設計上のポイント

#### xml-element の path 構文

XPath は強力すぎて実装コストが高い。bump-semver の用途では「root から特定 element までの一意な path で値を取る」だけで足りるので、最小構文 `/<elem>/<elem>/<elem>` に絞った。これは XPath のサブセット (predicates / wildcards / attributes なし) として理解しやすい。

#### 名前空間の local-name マッチ

Maven の `<project xmlns="http://maven.apache.org/POM/4.0.0">` のようにデフォルト名前空間が宣言されていても、利用者が path を `{http://maven.apache.org/POM/4.0.0}project/...` のように書くのは煩雑すぎる。CLI 引数として書く path は qualified name ではなく local name のほうが実用的なので、namespace は無視する。

#### Gradle の version 構文揺れ

Groovy DSL は `version = '1.2.3'` / `version "1.2.3"` (method call) / `version = "1.2.3"` の 3 形が混在する。1 regex `^version\s*=?\s*['"]([^'"]+)['"]` で 3 形を吸収。`=?` で `=` の有無を許容、quote は `['"]` で single/double を許容。Kotlin DSL は method-call shorthand が使えないので別 rule で `=` を必須にする。

## Consequences

### 互換性

純粋追加機能。既存ファイル形式の挙動は変わらず、新規拡張 / basename だけが追加で認識される。v0.13.x の任意のスクリプトは v0.14.0 でそのまま動く。

### 新規モジュール

- `src/format_xml_element.go`: xml-element format の Inspect / Replace + element path 解析 + 名前空間無視のトークン巡回
- `src/format_xml_element_test.go`: 単体テスト (pom / parent ignored / csproj / fsproj / multiple PropertyGroup / missing version)
- `src/format_regex_test.go` に Gradle / cabal / spec 系テスト追加

### 既存テストの更新

- `src/handler_test.go::TestDetectHandler_NewFallbackExtensions`: 旧来 "pom.xml は unsupported" を assert していた行を撤去 (v0.14 で supported になったため)

### バージョン

v0.14.0 (minor) として release。新規 file format 追加は本ツールでは minor バージョンに相当。

### help / ドキュメント反映

- `help.go` の `fullHelpText` の Supported file formats 表に build.gradle / *.cabal / *.spec / pom.xml / *.csproj 系を追記
- README-ja / README の対応形式説明にも反映
- DESIGN-ja / DESIGN の path-aware confidence ranked candidates 説明に xml-element format の存在を追記
- UPGRADING.md に v0.13.x → v0.14.0 セクション

## 関連実装

- `src/rules.go` — Cabal / Gradle / RPM spec の regex rule + pom.xml / *.csproj / *.fsproj / *.vbproj の xml-element rule、tryRule / formatReplace dispatcher に `xml-element` case 追加
- `src/format_xml_element.go` — 新規 format 本体
- `src/format_xml_element_test.go` — 新規 format テスト
- `src/format_regex_test.go` — Cabal / Gradle / Spec テスト追加
- `src/handler_test.go` — pom.xml unsupported assert の撤去
- `help.go` — Supported file formats 表 + Examples 更新
