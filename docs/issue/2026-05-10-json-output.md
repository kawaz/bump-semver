# `--json` 出力オプション

v0.6.0 候補。get / bump 系アクションで `--json` フラグを追加し、構造化された JSON を stdout に出力する。

## スコープ

**対象**: `get` / `major` / `minor` / `patch` / `pre`

**対象外**:
- `compare`: exit code 主役で stdout 出力なしの設計のため不要
- `--write` の書き戻し結果報告: stdout の bumped version 出力を JSON に置き換えるだけ。書き戻しの成否は exit code と stderr エラーで十分

## 設計方針 (確定)

### 1. 構造分解だけ提供、意味的判定はしない

pre/build は SemVer 仕様的に「ユーザ都合で意味付けする領域」。CLI は **構造分解だけ**を提供し、advance 可否やラベル昇格などの意味的判定は CLI 側で背負わない。CI スクリプトで必要なら jq + bash で判定すれば良い。

例: 「counter advance 可能か知りたい」→ `bump-semver pre VER` を呼んで exit code を見る (exit 0 なら可、2 なら不可)。`pre_advanceable: bool` のようなフィールドは追加しない。

### 2. `version` (入力フォーマット保持) と `semver` (strict 版) の二本立て

- `version`: 入力の `prefix` / 拡張本体 sep をそのまま保持 (例: `v_1_2_3-rc.1+build.42`)
- `semver`: SemVer 2.0.0 strict 形式に正規化 (例: `1.2.3-rc.1+build.42`、prefix 除去 + 本体 sep を `.` に正規化)

CI で「strict semver が欲しい」「フォーマット保持で欲しい」のどちらにも対応。

注: pre-release の `.` と build metadata の `.` `+` セパレータは SemVer 仕様準拠で固定なので、`version` と `semver` で差が出るのは **prefix と本体セパレータだけ**。

### 3. `pre_id` / `pre_rest` の分割定義

「最初の `.`」で分割。最後の識別子の数値性判定はしない (構造分解のみ)。

| 入力 | pre | pre_id | pre_rest |
|---|---|---|---|
| `1.2.3-rc.1` | `"rc.1"` | `"rc"` | `"1"` |
| `1.2.3-alpha.beta.5` | `"alpha.beta.5"` | `"alpha"` | `"beta.5"` |
| `1.2.3-alpha` | `"alpha"` | `"alpha"` | `null` |
| `1.2.3-rc1` | `"rc1"` | `"rc1"` | `null` |
| `1.2.3-0` | `"0"` | `"0"` | `null` |
| `1.2.3-0.3.7` | `"0.3.7"` | `"0"` | `"3.7"` |

`build_id` / `build_rest` も同じ規則 (最初の `.` で分割)。

## 確定スキーマ

入力例: `v_1_2_3-rc.1+build.42` (prefix=`v`、本体 sep=`_`)

```json
{
  "name": "my-pkg",                            // string|null (VER 起源 / name 欠落 FILE は null)
  "version": "v_1_2_3-rc.1+build.42",          // string (入力フォーマット保持: prefix と本体 sep)
  "semver": "1.2.3-rc.1+build.42",             // string (strict semver: prefix 除去 + 本体 sep を . に正規化)
  "major": 1,                                   // int
  "minor": 2,                                   // int
  "patch": 3,                                   // int
  "pre": "rc.1",                                // string|null
  "pre_id": "rc",                               // string|null
  "pre_rest": "1",                              // string|null
  "build_metadata": "build.42",                 // string|null
  "build_id": "build",                          // string|null
  "build_rest": "42"                            // string|null
}
```

入力が strict semver の場合 (`1.2.3-rc.1+build.42`、prefix 無し、本体 sep `.`)、`version` と `semver` は同じ値になる。

## 複数 FILE 整合性チェック時の挙動

`bump-semver get a.json b.json --json` のような複数 FILE 指定:

- 全部一致なら **1 つの JSON** を返す
- 不一致なら現状通り stderr に整列エラー (exit 2)
- `name` も全 FILE で一致が確認されている (DR-0004)

「個別ファイルごとの JSON 配列を返す」は **採用しない** (整合性が取れている前提なので冗長)。

## 実装スケッチ

### 新フラグ

`cliArgs` に `json bool` フィールドを追加。`--json` フラグでパース。

排他ルール:
- `compare` と `--json` 同時指定 → error: `compare does not support --json`

### 出力経路

`run()` の bump/get 経路で、現状 `fmt.Fprintln(stdout, newV.String())` の代わりに、`--json` 時は構造化 JSON を出力。

```go
type jsonOutput struct {
    Name          *string `json:"name"`
    Version       string  `json:"version"`
    Semver        string  `json:"semver"`
    Major         int     `json:"major"`
    Minor         int     `json:"minor"`
    Patch         int     `json:"patch"`
    Pre           *string `json:"pre"`
    PreID         *string `json:"pre_id"`
    PreRest       *string `json:"pre_rest"`
    BuildMetadata *string `json:"build_metadata"`
    BuildID       *string `json:"build_id"`
    BuildRest     *string `json:"build_rest"`
}
```

`Version.ToJSON(name *string) jsonOutput` のようなメソッドを `semver.go` に追加。`semver` フィールドは `prefix=""`, `sep="."` で再構築した String() を返す内部関数で。

### name の取得

- FILE 起源: `Inspection.Names` から取得 (整合性検証で 1 値に集約済)
- VER / stdin 起源: `null`

## 関連 DR

実装時に **DR-0007** として起票:
- `--json` フラグ追加の経緯と設計判断
- スキーマ確定 (本 issue を内容として転記)
- 不採用案 (`pre_advanceable` 等の意味判定フィールド) と理由

## テスト方針

- `spec_table_test.go` の各バージョン文字列について、JSON 出力の構造を確認する `TestSpec_JSONOutput` を追加
- エッジケース: `null` フィールド (VER 起源で name なし、pre/build なし、pre_rest なしなど)
- 複数 FILE 整合性 OK 時の name 取得確認

## ROADMAP 反映

`docs/ROADMAP.md` の「機能候補」セクションに `--json` を追加 (本 issue へのリンク付き)。

実装着手時にこの issue を `docs-knowledge-flow.md` に従って削除し、決定事項は DR-0007 へ。
