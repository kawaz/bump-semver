# DR-0017: compare の precision suffix 拡張 (eq-major / lt-minor 等)

- Status: Active
- Date: 2026-05-11
- Extends: DR-0006 (compare サブコマンド + FILE|VER 統合)

## Context

DR-0006 で `compare {eq|lt|le|gt|ge}` の 5 OP を導入した。SemVer 2.0.0 § 11 の完全順序仕様に従って 2 値を比較する。

しかし CI / リリースフローでは「**どの component まで** 一致 / 大小を見たいか」を切り分けたい局面が多い:

- 「meta release branch を分けるので major が同じかだけ知りたい」
- 「pre-release を回している間は patch まで合っていれば十分（rc.N の数字違いは無視）」
- 「依存 bump が minor 以下に収まっているかチェックしたい」

現状は呼び出し側で `bump-semver get FILE --no-pre --no-build-metadata` で取り出して shell で文字列比較するなどの逃げ道しかなく、`bump-semver` の表現力が CI 用途で頭打ちになっている。

## Decision

### 構文

compare OP に **precision suffix** `-major` / `-minor` / `-patch` を許可する:

```
compare <base>[-<precision>] <INPUT> <INPUT>
base       = eq | lt | le | gt | ge
precision  = major | minor | patch     (省略時は SemVer full = 既存挙動)
```

組み合わせは 5 base × 4 precision (省略含む) = **20 OP**。

### セマンティクス: precision-aware Compare

`v CompareAt(other, precision)` を導入し、precision で指定された component より下位は比較しない。

- `precision = ""` (省略, 既存): MAJOR → MINOR → PATCH → pre-release の順で比較 (DR-0006 / SemVer 2.0.0)
- `precision = "patch"`: MAJOR → MINOR → PATCH で比較、pre-release / build-metadata は無視
- `precision = "minor"`: MAJOR → MINOR で比較
- `precision = "major"`: MAJOR で比較

build-metadata は precision に関わらず常に無視 (SemVer § 10)。

### 比較結果のマッピング

`evalCompareOp(base, cmp)` は既存と同じ:

```
cmp == 0 && base ∈ {eq, le, ge}  → true
cmp <  0 && base ∈ {lt, le}      → true
cmp >  0 && base ∈ {gt, ge}      → true
それ以外                          → false
```

つまり precision は「どの component まで cmp を取るか」だけを変え、base による真偽判定は変えない。

### 期待動作の例

| 呼び出し | 結果 |
|---|---|
| `eq-major 1.2.3 1.9.7` | 0 (true) |
| `eq-major 1.2.3 2.0.0` | 1 (false) |
| `eq-minor 1.2.3 1.2.9` | 0 (true) |
| `eq-minor 1.2.3 1.3.0` | 1 (false) |
| `eq-patch 1.2.3 1.2.3-rc.1` | 0 (true) — pre-release 無視 |
| `eq-patch 1.2.3 1.2.4` | 1 (false) |
| `lt-major 1.9.9 2.0.0-rc.0` | 0 (true) — 2.0.0-rc.0 の pre-release は major 比較では無視 |
| `lt-minor 1.2.9 1.3.0-rc.0` | 0 (true) |
| `gt-patch 1.2.4-rc.0 1.2.3` | 0 (true) — 1.2.4 > 1.2.3 |
| `ge-major 1.0.0 1.99.99` | 0 (true) |
| `ge-patch 1.2.3-rc.0 1.2.3` | 0 (true) — pre-release 無視で 1.2.3 ≥ 1.2.3 |

### CLI 表記

- `--help` (短): `compare` 行を `<eq|lt|le|gt|ge|...>` に圧縮 (網羅しない)
- `bump-semver compare --help`: 全 OP を 5 base × 4 precision の表で列挙
- `--help-full`: 同じ表 + Examples

## Rationale

### 不採用案

**1. `compare --major`, `compare --minor`, `compare --patch` のような precision フラグ**

OP は据え置きで `--precision` flag を別途渡す案。**不採用**: フラグだと OP との位置関係が長くなり (`compare eq --major INPUT INPUT`)、 shell で書くと読みにくい。suffix なら短く、ペア (`-major`/`major`/`-minor` ...) で意味も推測しやすい。

**2. precision を別 OP として完全に新名 (`eqM`, `gtMaj` 等)**

短縮を狙う案。**不採用**: 不規則名は覚えにくく、`base-precision` の規則性が失われる。kebab-case suffix なら CLI 引数の慣習にも合う。

**3. precision に `pre` (pre-release まで含む = full) を明示的に許可**

`eq-pre` で「pre-release も含めて完全一致」を表現する案。**不採用**: precision 省略時の挙動 (既存 5 OP) がすでに pre-release 含む比較。同じ意味の OP を 2 通り提供するのは API 表面を肥大化させるだけで利益がない。

**4. `eq-buildmeta` のような build-metadata 比較 OP**

SemVer § 10 で「build-metadata は順序判定に影響しない」と定義されているので、CLI 側で構造化 JSON 比較に使えば足りる (`bump-semver get FILE --json | jq -r .build_metadata`)。**不採用**: SemVer 仕様外の比較を CLI に持ち込む価値が薄い。

**5. precision を数値で指定 (`eq-0` / `eq-1` / `eq-2`)**

major=0 / minor=1 / patch=2 として数字で精度を表す案。**不採用**: 名前のほうが self-documenting。

### 設計上のポイント

#### precision=patch で pre-release を無視する理由

「同じ patch なら同じバージョン扱いしたい」という意図でこの OP を使うのが想定ユースケース。pre-release を比較対象に含めると `eq-patch 1.2.3 1.2.3-rc.1` が false になってしまい、`-patch` を選ぶ意味が薄れる。CI スクリプトで「rc 系を bump しているがリリース予定バージョンは同じ」というケースを cleanly に扱えるようにする。

pre-release を含めて比較したいなら既存の base OP (`eq` / `lt` / `le` / `gt` / `ge`) をそのまま使う。

#### evalCompareOp の不変

precision の追加で `evalCompareOp` を書き換える必要はない。precision は `CompareAt` の内部で吸収され、cmp を返す段階では既存ロジックと同じ三値 (-1/0/+1)。

#### 後方互換

既存 5 OP の挙動は不変 (precision 省略時 = SemVer full)。テストも `TestEvalCompareOp` などはそのまま pass する。

## Consequences

### 互換性

純粋追加機能。`v0.12.0` 以前のスクリプトは無変更で動く。

### 新規 / 拡張モジュール

- `src/semver.go`: `Version.CompareAt(other, precision string) int` を新規追加 (既存 `Compare` は CompareAt("") への薄いラッパー化)
- `src/main.go`: `parseCompareOp(s) (base, precision string, ok bool)` を新規ヘルパーとして導入。`compareOps` map (既存 5 OP) はそのまま使う
- `src/compare.go`: `runCompare` 内で precision を取り出して `CompareAt` に渡す
- `cliArgs` に `comparePrecision string` フィールドを追加 (`compareOp` は base のみ格納する設計に整理)

### テスト

- `src/spec_table_test.go` (DR-0006 の仕様駆動テスト) に precision 系ケースを追加
- `src/compare_test.go` (or `main_test.go` の `TestRun_Compare_*`) に CLI レイヤでの 20 OP 動作テスト

### help / ドキュメント

- `helpCompare` の Operators セクションに precision 表を追加
- `fullHelpText` の compare 説明にも反映
- README-ja / README の `compare` 説明に 1 段落追加
- DESIGN-ja / DESIGN の 比較セマンティクスに precision 段落追加

### 将来拡張

- `--precision-pre` / `--precision-build` のようなフラグでより細かい精度制御 (現状は不要、必要になれば DR で追加)
- range / interval 比較 (`compare in 1.0.0 2.0.0 X`) — これは別 OP 設計が必要、本 DR の範囲外

## 関連実装

- `src/semver.go` — `CompareAt`
- `src/main.go` — `parseCompareOp` ヘルパー、`cliArgs.comparePrecision`、help 文言
- `src/compare.go` — `runCompare` での precision dispatch
- `src/spec_table_test.go` / `main_test.go` — precision 系テスト追加
