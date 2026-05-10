# DR-0007: `--json` 出力オプション

- Status: Active
- Date: 2026-05-10
- Extends: DR-0001 (アクション数), DR-0004 (整合性検証範囲), DR-0006 (compare サブコマンド)

## Context

CI スクリプトで `bump-semver` の出力を扱う際、現在の bare な `1.2.3` 出力だけだと:

- pre-release / build metadata の構造分解を呼び出し側で再実装する必要がある
- prefix 付き入力 (`v1.2.3` 等) と strict semver 形式の両方を欲しい場合に使い分けが面倒
- jq との組み合わせで条件分岐したい場合に grep / sed の文字列処理に依存

`--json` フラグで構造化された JSON を出力できれば、CI の `bump-semver get | jq` パターンが素直に書ける。

## Decision

### スコープ

- **対象アクション**: `get` / `major` / `minor` / `patch` / `pre`
- **対象外**: `compare`

  `compare` は exit code が答えで stdout 出力を持たない設計 (DR-0006)。`--json` を受け取ったらエラー (`compare does not support --json`、exit 2)。

- **`--write` との関係**: 直交。`--json` は stdout 出力の表現形式の切り替えのみで、ファイル書き戻しの成否は exit code と stderr に従う。

### スキーマ

入力例: `v_1.2.3-rc.1+build.42` (prefix=`v_`, 本体 sep=`.`)

```json
{
  "name": "my-pkg",
  "version": "v_1.2.3-rc.1+build.42",
  "semver": "1.2.3-rc.1+build.42",
  "major": 1,
  "minor": 2,
  "patch": 3,
  "pre": "rc.1",
  "pre_id": "rc",
  "pre_rest": "1",
  "build_metadata": "build.42",
  "build_id": "build",
  "build_rest": "42"
}
```

| フィールド | 型 | 説明 |
|---|---|---|
| `name` | string \| null | FILE 起源の name (DR-0004 で集約済の 1 値)。VER / stdin 起源は null |
| `version` | string | 入力フォーマット保持 (prefix + 本体 sep) |
| `semver` | string | strict 形式 (prefix 除去 + 本体 sep を `.` に正規化) |
| `major` / `minor` / `patch` | int | 数値要素 |
| `pre` | string \| null | pre-release 識別子を `.` 結合した文字列 |
| `pre_id` / `pre_rest` | string \| null | `pre` を最初の `.` で分割 |
| `build_metadata` | string \| null | build metadata 識別子を `.` 結合した文字列 |
| `build_id` / `build_rest` | string \| null | 同上「最初の `.` で分割」 |

#### `version` と `semver` の二本立て

入力フォーマットを保持したい用途 (人が読む / 既存ファイルの形式に合わせて書き戻す) と、strict semver が欲しい用途 (npm registry 比較、jq での意味比較等) の両方に対応するため、二つ並べる。

`pre` / `build_metadata` のセパレータ (`.` `+` `-`) は SemVer 仕様で固定なので、`version` と `semver` で差が出るのは prefix と本体 sep だけ。

#### `pre_id` / `pre_rest` の分割定義

最初の `.` で分割。最後の識別子の数値性判定 (advanceable か等) はしない。

| 入力 | pre | pre_id | pre_rest |
|---|---|---|---|
| `1.2.3-rc.1` | `"rc.1"` | `"rc"` | `"1"` |
| `1.2.3-alpha.beta.5` | `"alpha.beta.5"` | `"alpha"` | `"beta.5"` |
| `1.2.3-alpha` | `"alpha"` | `"alpha"` | `null` |
| `1.2.3-rc1` | `"rc1"` | `"rc1"` | `null` |
| `1.2.3-0` | `"0"` | `"0"` | `null` |
| `1.2.3-0.3.7` | `"0.3.7"` | `"0"` | `"3.7"` |

`build_id` / `build_rest` も同じルール。

### 複数 FILE 整合性チェック時

複数 FILE を渡したとき (`bump-semver get a.json b.json --json`):

- 全部一致 → 1 つの JSON を返す (整合済の値を出力)
- 不一致 → 既存通り stderr に整列エラー、exit 2、JSON 出力なし
- `name` も DR-0004 で集約済の 1 値が `name` フィールドに入る

「個別ファイルごとに JSON 配列で返す」案は採用しない。整合性が取れている前提なので冗長。

### 出力フォーマット

- 1 行 + 末尾改行で出力 (改行ありにすると行指向ツール (`while read line`) との相性が良い)
- `-q` / `-qq` で stdout 抑制は引き続き有効 (`--json -q` は何も出力しない)

## Rationale

### 不採用案: 意味判定フィールド (`pre_advanceable: bool` 等)

検討案として「最後の pre 識別子が pure numeric なら counter advance 可能」を `pre_advanceable: true/false` で出すアイデアがあったが、採用しなかった。

- pre / build は SemVer 仕様的に「ユーザ都合で意味付けする領域」であり、CLI が意味判定を背負うとユースケースごとに肥大化する (label 順序、phase 昇格等)
- 「counter advance 可能か知りたい」場合は `bump-semver pre VER` を呼んで exit code (0 = 可、2 = 不可) を見れば良い。CLI は **構造分解だけ**を提供し、意味判定はシェル側で組み立てる方針

### 不採用案: `prefix` / `body_sep` 単独フィールド

`prefix: "v"`, `body_separator: "."` のような分解情報を別フィールドで出す案もあったが、`version` と `semver` の二本立てで実用上は十分。prefix/sep を再構築したい用途は `version` を見ればよく、純粋な数値計算には `semver` または `major.minor.patch` を使えば良い。

### 不採用案: ndjson 形式 (複数 FILE 時)

複数 FILE を行ごとに別 JSON で返す案。整合性が取れているなら値は全部同じであり、複数行返しは冗長。整合性が取れていない場合は既存の stderr エラーで明示する方針が一貫している。

## Consequences

### 互換性

純粋な追加機能。`--json` を渡さない既存の呼び出しは挙動不変。**v0.5.x の任意のスクリプトは v0.6.0 でそのまま動く**。

### compare の制約

`compare` は `--json` を受け付けない。「比較結果を JSON で欲しい」という要望が将来出たら、別の DR で `compare` の出力設計から見直す (現状 stdout を持たない設計と整合性を取る必要がある)。

### スキーマ拡張時のポリシー

将来フィールドを追加する場合は **既存フィールド非破壊** が前提:

- フィールド追加 OK (既存フィールドを読んでいるスクリプトは無影響)
- 既存フィールドの semantics 変更は破壊変更扱い → 別 DR + UPGRADING.md に記載

## 関連実装

- `src/json.go` — `jsonOutput` / `Version.ToJSON` / `Version.strict` / `splitAtFirstDot`
- `src/main.go` — `--json` フラグ、`compare does not support --json` チェック、JSON 経由の stdout 出力
- `src/spec_table_test.go` の `TestSpec_JSONOutput` — DR の例表をそのままなぞる構造分解の網羅テスト
- `src/main_test.go` の `TestRun_JSON_*` — CLI レイヤから見た振る舞い (compare 拒否、`-q` での抑制、name 集約)
