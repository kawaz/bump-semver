# DR-0015: Xcode `project.pbxproj` (multi-match 同期) と `Info.plist` (XML plist) 対応

- Status: Accepted
- Date: 2026-05-11
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」)、DR-0005 (path-aware confidence ranked candidates)、DR-0012 (regex format / 「1 ファイル 1 マッチ」制限の根拠)、DR-0014 (TOML section-scoped Replace、直前リリース)

## Context

DR-0012 で `regex` フォーマットを導入したとき、**意図的にスコープ外**としたケースが 2 つあった:

1. **Xcode `project.pbxproj`**: 同一ファイル内で `MARKETING_VERSION = 1.2.3;` が **複数行** (Debug / Release / 各 target の build settings) に登場し、それらを **同期更新** する必要がある。DR-0012 の regex format は `FindSubmatchIndex` (= 最初の 1 マッチだけ) で動くので構造的に対応できない。
2. **`Info.plist`**: Apple の XML plist 形式。`<key>CFBundleShortVersionString</key><string>1.2.3</string>` のような **2 要素ペア構造** を持つ XML で、行ベース regex で扱うのは脆い (改行・属性挿入・名前空間で簡単に壊れる)。

Xcode iOS / macOS プロジェクトでは `*.xcconfig` (DR-0012 で対応済) + `project.pbxproj` + `Info.plist` の 3 点セットでバージョン管理されるのが一般的で、bump-semver の `--write` で 3 ファイル一括 bump できないと実用にならない。DR-0012 § "不採用案 D" で明示的に「実需が出た時点で別 DR」と予約していたので、本 DR で消化する。

### Xcode `project.pbxproj` の構造

OpenStep plist 形式 (Apple 独自、JSON でも XML でもない、シェルスクリプト風):

```
/* Begin XCBuildConfiguration section */
		ABC123 /* Debug */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				MARKETING_VERSION = 1.2.3;
				CURRENT_PROJECT_VERSION = 42;
			};
		};
		DEF456 /* Release */ = {
			isa = XCBuildConfiguration;
			buildSettings = {
				MARKETING_VERSION = 1.2.3;
			};
		};
/* End XCBuildConfiguration section */
```

`MARKETING_VERSION` 行は build configuration ごと (Debug / Release / 任意のカスタム configuration) × target ごとに増える (1 つの app target に Debug / Release で 2 行、複数 target なら掛け算)。**全行が同一値** であることが Xcode 側の前提 (片方だけ古い値が残ると Xcode が build configuration ごとに別バージョンとして扱い、App Store Connect への submit でリジェクトされる)。

引用符は省略可: `MARKETING_VERSION = 1.2.3;` も `MARKETING_VERSION = "1.2.3";` も両方有効。`CURRENT_PROJECT_VERSION` (build number) は別概念 (整数 / build hash) で本 DR のスコープ外。

### `Info.plist` の構造

XML plist (Apple の標準フォーマット、`<plist>` ルート + `<dict>` 子要素):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>1.2.3</string>
	<key>CFBundleIdentifier</key>
	<string>com.example.app</string>
	<key>CFBundleVersion</key>
	<string>42</string>
</dict>
</plist>
```

`CFBundleShortVersionString` が marketing version、`CFBundleVersion` が build number (本 DR スコープ外、別概念)。

Xcode 11+ では値が **placeholder** になっているケースがある:

```xml
<key>CFBundleShortVersionString</key>
<string>$(MARKETING_VERSION)</string>
```

この場合は `project.pbxproj` 側の `MARKETING_VERSION` が真値で、`Info.plist` は実体を持たない。bump-semver としては**抽出失敗** (placeholder は SemVer としてパース不能) で rule fallthrough → unsupported error にするのが自然。

## Decision

### 1. `format_pbxproj.go` 専用 format を新設

OpenStep plist は XML でも JSON でもない固有形式。汎用 format には載せず、専用 file で扱う。実装は行ベース regex で十分:

```go
// pbxprojInspect: 全 MARKETING_VERSION マッチを抽出し、
// 値が全て一致するか整合性検証。不一致なら mismatch エラー。
func pbxprojInspect(_ CandidateRule, content []byte) (Inspection, error)

// pbxprojReplace: 全マッチを同一新値で同期書き換え。
// (Inspect が整合性を保証している前提で current ↔ newVersion の単純置換)
func pbxprojReplace(_ CandidateRule, content []byte, current, newVersion string) ([]byte, error)
```

regex: `(?m)^\s*MARKETING_VERSION\s*=\s*"?([^";\s]+)"?\s*;` (引用符の有無両対応)。1 ファイル内整合性検証は **`Inspection.Versions` に全マッチを `line:N` Path 付きで詰めて返し、main.go の既存 mismatch 検出器に通す** ことで実現する (新エラーパスを足さない)。

### 2. `format_xml.go` 新設 (plist 形式専用、汎用 XML には広げない)

`encoding/xml` を使うが、抽出 / 書き換えの対象は **`<key>NAME</key><string>VALUE</string>` の隣接ペア** に限定する。`encoding/xml.Decoder` で structured token stream を読み、`StartElement{key}` 直後に value を取る形式探知を行う。

Replace は **元文書の byte offset 範囲を直接書き換える** (encoding/xml の Marshal は属性順序・インデント・DOCTYPE を保持しないため round-trip 不可)。Decoder の `InputOffset()` で各 token の byte 位置を取れるので、value `<string>...</string>` の中身の byte range を覚えておいて元バイト列を切り貼りする。

```go
// xmlInspect: <key>NAME</key><string>VALUE</string> ペアを順次走査し、
// rule.VersionPaths[0] に一致する key の value を Versions に、
// rule.NamePaths[0] に一致する key の value を Names に詰める。
func xmlInspect(rule CandidateRule, content []byte) (Inspection, error)

// xmlReplace: 同様に value 範囲を特定し、その byte range だけ書き換え。
func xmlReplace(rule CandidateRule, content []byte, current, newVersion string) ([]byte, error)
```

`VersionPaths` の path 形式は `CFBundleShortVersionString` のような plist key 名そのまま (XPath 風の構文は導入しない、シンプルに保つ)。

placeholder 値 (`$(MARKETING_VERSION)` 等) は `ParseVersion` 失敗を main.go 側のレイヤで検出し、自然に rule fallthrough → `unsupported file:` で落ちる (Inspect 側で特別扱いはしない)。実際には xmlInspect が値を返す → main.go が ParseVersion で fail という二段。

### 3. 新規ルール 2 個

| confidence | パターン | format | version key | name key |
|---|---|---|---|---|
| **3** | `project.pbxproj` (basename) | pbxproj | `MARKETING_VERSION` | (なし) |
| **3** | `Info.plist` (basename) | xml | `CFBundleShortVersionString` | (なし) |

basename 一致なので glob 不要。`*.pbxproj` glob にしない理由: 拡張子 `.pbxproj` を持つファイルは実質 `project.pbxproj` 1 種類しかない (Xcode の bundle 内固定名)。glob にすると confidence 1 になり、誤検出リスクと hint 出力ノイズが増える。

`name` 抽出は両方とも MVP では skip (DR-0001「必要が出たら追加」)。理由:
- pbxproj の `PRODUCT_NAME` は build setting 階層内で複数定義され同期検証用途には過剰。
- Info.plist の `CFBundleIdentifier` (`com.example.app`) は plist key としては取れるが、cross-file name consistency (DR-0004) の比較対象として意味があるかは別議論。`Cargo.toml` の `[package].name` と `com.example.app` を照合してもズレるだけ。実需が出たら別 issue で追加する。

### 4. handler.go / rules.go の switch 拡張

`tryRule` / `formatReplace` の switch に `"pbxproj"` / `"xml"` arm を追加。format 数: 5 (json/toml/yaml/plain/regex) → 7。

DR-0012 § "不採用案 C" で言及した「format 数 10+ 時の dispatch 構造再評価」はまだ余裕がある (現状 7、ROADMAP 候補の jsonc を入れて 8)。switch 維持で続行。

### 5. 整合性エラーは v0.5.0 mismatch 整列スタイルに揃える

pbxproj で `MARKETING_VERSION` の値が複数行で異なる場合、専用エラーパスは作らず **`Inspection.Versions` に全マッチを詰めて返す** だけにする。main.go の既存 `formatMismatchError` (column-aligned) がそのまま動き、ラベル形式は `<file>:line:N` になる:

```
version mismatch:
  project.pbxproj:line:23  = 1.2.3
  project.pbxproj:line:31  = 1.2.4
  project.pbxproj:line:45  = 1.2.3
```

各 Field の Path に `"line:N"` を入れることで `formatMismatchError` の column alignment が自動的に効く。専用整列ロジックは書かない。

### 6. 不採用案

#### A. `regex` format に `MultiMatch bool` フラグ追加

DR-0012 で確立した「regex format は 1 ファイル 1 マッチ」を意図的に守る (DR-0012 § "1 ファイル 1 マッチ" の設計判断)。MultiMatch フラグを後付けすると、(a) 整合性検証ロジックが regex format に紛れ込む、(b) `*.podspec` 等の既存 regex rule が「素朴 regex」と「整合性検証付き同期更新」の二態を持つことになり責務が曖昧になる、(c) コメント等に同名キーがあった場合の誤マッチリスクを拾う構造を持たないので結局専用化が必要、という 3 つの問題が出る。専用 format のほうが clean。

#### B. `format_pbxproj.go` を `format_openstep_plist.go` のような汎用 OpenStep plist パーサにする

OpenStep plist の完全パーサは過剰投資。`MARKETING_VERSION` は build settings の階層下の単純な key=value 行で、行ベース regex で十分かつ堅牢。OpenStep plist は他に Xcode の workspace 設定 (`contents.xcworkspacedata`) 等もあるが、それらは XML で別形式。専用 format の cycle-time 短縮 > 汎用パーサの再利用性。

#### C. `format_xml.go` を汎用 XML format (Maven `pom.xml` / Android Gradle `build.gradle` 等含む) として最初から広く設計する

XML 全般を扱う format は必然的に「XPath 風の path 構文」「namespace 処理」「mixed content の扱い」「CDATA セクション」等に踏み込まざるをえず、複雑度が plist 専用より一桁上がる。`pom.xml` 対応の実需が出た時点で format を別建てする (`format_xml.go` を `format_xml_plist.go` にリネーム + `format_xml_pom.go` 追加 等) のが筋。本 DR では plist 構造に絞る。

#### D. Info.plist を `encoding/xml` で round-trip (Unmarshal → 構造体書き換え → Marshal)

却下理由:
- `encoding/xml.Marshal` は属性順序・インデント・DOCTYPE を保持しない (Apple の plist は DOCTYPE 必須、Xcode はインデント崩れに敏感)
- 既存の TOML / YAML 各 format も同じ理由で round-trip を避け、行ベースで byte range 書き換えしている (DR-0011, DR-0014 と整合)
- 元文書の byte offset での書き換えなら DOCTYPE / 改行 / インデント / 属性順序が完全保持される

#### E. placeholder (`$(MARKETING_VERSION)`) を Info.plist 側で特別扱いし、`project.pbxproj` を再帰的に読みに行く

却下理由 (本 DR スコープ外):
- ファイル間参照を解決する責務は bump-semver の役割を超える (build system の領分)
- 利用者が `bump-semver patch project.pbxproj Info.plist --write` のように両ファイルを並べれば、cross-file consistency (DR-0004) で値合わせが効く。`Info.plist` だけ単独 bump する需要は低い
- `Info.plist` で placeholder を見たら ParseVersion 失敗 → unsupported error は自然な振る舞い (利用者は `project.pbxproj` を入力に追加する解決策に気付ける)

#### F. `CFBundleVersion` (build number) も同 DR で対応

却下理由:
- build number は SemVer ではない (整数 / build hash / commit count etc.)
- bump-semver は「SemVer の major/minor/patch を bump する」CLI で、build number bump は別ドメイン (CI で `$GITHUB_RUN_NUMBER` を埋めるのが一般的)
- 必要が出たら別 CLI / 別フラグで扱う (`bump-semver` のスコープ外)

## Consequences

### 実装変更

- `src/format_pbxproj.go` 新規: pbxprojInspect (全 MARKETING_VERSION マッチ + 整合性検証) + pbxprojReplace (全マッチ同期更新)
- `src/format_xml.go` 新規: xmlInspect / xmlReplace (`encoding/xml` Decoder + byte offset 書き換え、plist 構造専用)
- `src/rules.go`: pbxproj / Info.plist 用の confidence-3 ルール 2 個追加
- `src/handler.go` (実体は `src/rules.go`) の `tryRule` / `formatReplace` switch に `"pbxproj"` / `"xml"` arm 追加
- `src/format_pbxproj_test.go` 新規: 単一 / 複数 build settings / 不一致エラー / placeholder スキップ
- `src/format_xml_test.go` 新規: Info.plist 抽出 / 書き換え / placeholder で extraction 結果が parseError 経由で握りつぶされる経路
- `src/handler_test.go` 拡張: pbxproj / Info.plist の rule resolution + detect テスト
- README.md / README-ja.md: 対応形式テーブルに 2 行追加 + Xcode 系の placeholder ケース注意
- UPGRADING.md: v0.11.x → v0.12.0 セクション追加 (純粋追加)
- ROADMAP.md: pbxproj / Info.plist を Done セクションに移動

### 後方互換

- CLI 表面は完全に維持
- 既存の json/toml/yaml/plain/regex ルールは無変更
- これまで `unsupported file:` だった `project.pbxproj` / `Info.plist` が新たに動く (純粋追加)

### v0.11.x → v0.12.0

新ルール 2 個 (path-pinned confidence 3) + 新 format 2 個 (`pbxproj` / `xml`) なので minor bump。`UPGRADING.md` に v0.11.x → v0.12.0 セクションを追加。

### 次のフェーズ

- Maven `pom.xml` / Android Gradle 系 → `format_xml.go` を `format_xml_plist.go` にリネームし、`format_xml_pom.go` を新規追加するのが筋 (汎用 XML format に広げない)
- jsonc (Bun bun.lock / VS Code settings.json) → 別 DR
- `CFBundleVersion` (build number) → bump-semver スコープ外、別 CLI / 別フラグで検討

## 関連

- DR-0001: 「必要が出たら 1 行追加」(本 DR は 2 ルール追加 + 専用 format 2 個 = 「必要なものを最小で」)
- DR-0005: confidence ranked dispatcher (本 DR の前提機構、両ルールとも path-pinned confidence 3)
- DR-0012: regex format / 「1 ファイル 1 マッチ」制限 (本 DR が予約した次フェーズを消化、§ 不採用案 D で名指しされていた)
- DR-0014: TOML section-scoped Replace (直前リリース、byte range 書き換え方針が本 DR と整合)
- ROADMAP: `*.pbxproj` / `Info.plist` 行を本 DR で Done に移動
