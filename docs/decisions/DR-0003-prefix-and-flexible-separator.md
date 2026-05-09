# DR-0003: prefix (`v` / `ver` / `version`) と柔軟 separator (`. _ -`) を許容する

- ステータス: Accepted
- 日付: 2026-05-09
- 関連: DR-0001 (flat 4-action + basename 形式判定) の semver 仕様を拡張

## 文脈

DR-0001 は version 文字列を **strict X.Y.Z (separator は `.` のみ、prefix なし)** に限定していた。これは MVP の最小範囲としては正しかったが、実運用で扱いたい入力が以下のように広がっている:

- Git タグ風: `v1.2.3` (gh release タグ、CHANGELOG の見出し、`Cargo.toml` の `version = "v1.2.3"` と書く流派)
- レガシー系 / 別スタイル: `1_2_3` / `1-2-3` (アンダースコア命名規則のリポ、ファイル名・ブランチ名から版番号を取りたいケース)
- 接頭詞付き: `ver-1.2.3` / `version_1.2.3` (リリースノート見出し、自動生成タグ)

これらすべて「数字 3 つを `.` `_` `-` のいずれかで区切り、先頭に `v` / `ver` / `version` の任意接頭詞」というパターンに収まる。`bump-semver` の handler 層 (Cargo / JSON / VERSION) はあくまで文字列を出し入れするだけなので、semver パーサーをここまで広げても他層への影響は出ない。

## 決定

### 1. 受理可能な書式を拡張

正規表現 (anchored):

```
^(v(?:er(?:sion)?)?[_.\-]?)?(\d+)([_.\-])(\d+)([_.\-])(\d+)$
```

- prefix: `v` / `ver` / `version` のいずれか + 任意の `_` / `.` / `-` 1 文字 (全部省略可能)
- 各コンポーネント: `\d+`
- separator: `.` / `_` / `-` のいずれか
- 末尾: 余計な文字を許さない (`-alpha` `+build` は依然 reject)

### 2. separator 不一致は reject

`1.2-3` のような sep1 != sep2 はエラー。実用上は誤入力の可能性が高く、出力時にもどちらを採用すべきか曖昧になるため。

### 3. prefix と separator は **保持**

`Version` 構造体に `Prefix` / `Sep` フィールドを持ち、`Bump` と `String` で保持して書き出す:

| 入力 | action | 出力 |
|---|---|---|
| `v1.2.3` | patch | `v1.2.4` |
| `version_1_2_3` | minor | `version_1_3_0` |
| `ver-1-2-3` | major | `ver-2-0-0` |
| `1_2_3` | patch | `1_2_4` |
| `1.2.3` | patch | `1.2.4` |

### 4. handler 層は無変更

- `cargoHandler` の正規表現は `[^"']*` で値全体をキャプチャするので `"v1.2.3"` でも問題ない
- `jsonHandler` は現在値で値特定する仕様 (DR-0002 関連) なので prefix の有無に関わらず動作する
- `versionHandler` は trim だけで内容を返すので無関係

つまり拡張は `src/semver.go` 内に閉じる。

## 不採用案

### A. prefix を `v` のみに限定

`ver` / `version` まで許容すると正規表現が冗長になる。一方で `version_1_2_3` 形式を fix で書き戻したい実需 (タグ自動生成 / リリースノート向け) があり、ここで切るとあとで再拡張になるので最初から包括する。

### B. 大文字 `V` も許容 (`[vV]`)

将来必要なら追加するが、現状は小文字 `v` のみ。Git タグの慣習も小文字優位、CHANGELOG の見出しも `## v1.2.3` が多数派。

### C. separator 不一致を許容して書き出し時は最初の sep を採用

挙動の予測しにくさ (`1.2-3` patch -> `1.2.4` か `1-2-4` か) がデバッグ事故の温床になる。strict reject で予測可能性を優先。

### D. pre-release / build metadata 対応 (`-alpha.1`, `+build.42`)

DR-0001 不採用案 D と同じ理由で MVP 範囲外を維持。本 DR は「prefix と separator」の話に閉じる。

## 影響

- `Version{Major:1, Minor:2, Patch:3}` を直接生成して `String()` を呼ぶコードがあった場合、`Sep` 未設定で `"."` にフォールバックするので zero-value 互換は維持
- 既存 VERSION ファイル (`0.0.0`) や `*.json` / `Cargo.toml` の `1.2.3` 形式の挙動は不変

## 関連

- DR-0001: flat 4-action + basename 形式判定 (この DR が拡張する上位設計)
- `src/semver.go` / `src/semver_test.go` (TestParseVersion / TestBump_PreservesPrefixAndSep)
- `src/main_test.go` (TestRun_ValueBumps の v prefix ケース、TestRun_FileWriteVPrefixPreserved)
- `docs/ROADMAP.md` の関連項目 (`v` prefix 言及があれば本 DR で解消)
