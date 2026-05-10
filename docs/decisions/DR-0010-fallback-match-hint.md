# DR-0010: confidence 1 fallback マッチ時の hint 出力 + unsupported file エラーの誘導文言

- Status: Active
- Date: 2026-05-11
- Closes: docs/issue/2026-05-10-fallback-match-hint.md
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」哲学), DR-0005 (path-aware confidence ranked candidates), v0.5.0 の `--no-hint` / `-q` / `-qq` 抑制機構

## Context

DR-0005 で導入した confidence ranked rules は、`unknown.json` のような未知のファイル名でも `*.json` glob fallback (confidence 1) で動いてしまう。これは設計通りだが、利用者から見ると:

- 「動いた、OK」で済ませてしまい、本来は別ルールで処理されるべきファイルだったかもしれないことに気づきにくい
- 明示的に対応してほしいファイル形式があっても、issue を立てるトリガーが発生しない (DR-0001 の「必要が出たら 1 行追加」フィードバックループが回らない)

したがって confidence 1 fallback で動いたとき、その事実を stderr に hint として残し、必要なら issue を立ててもらう動線を作る。

完全に未対応のファイル (`unknown.toml` 等) も同様で、`unsupported file:` エラー本体に「明示対応が必要なら issue を立てて」誘導文言を 1 行 append する (発信源は同じ issue だが、こちらはエラー扱い）。

## Decision

### 1. confidence 1 fallback マッチ時に hint を stderr に出す

bump 系・get 共通で、confidence 1 でマッチした FILE 入力 1 つにつき stderr に 1 行:

```
hint: <path> matched as <glob> fallback. Open issue if explicit support is needed.
```

例:
```
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
```

### 2. 発動条件

- DR-0005 confidence ranked candidates で **confidence 1 のルールが採用された** とき
- confidence 2 (basename-only) / confidence 3 (path-pinned) は明示対応済みなので hint なし

#### confidence 2 を hint 対象に含めない理由

confidence 2 (任意 dir の `marketplace.json` / `plugin.json`) は **basename で明示対応されている**。任意ディレクトリで偶然同名のファイルが意図せず confidence 2 にヒットした場合は、現状すでに「抽出失敗 → 次ルール (confidence 1) へ降りる」動作で、最終的に confidence 1 hint がきちんと出る (本 DR の仕組みで吸収される)。したがって confidence 2 は静かに通すのが正しい。

### 3. 複数 FILE 指定時は該当ファイルだけ列挙

```bash
bump-semver get a.json b.json unknown.json
```

`unknown.json` のみ confidence 1 にヒット、`a.json` / `b.json` は明示対応済みのとき:

```
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
```

複数 FILE が同時に confidence 1 にヒットしたら **1 行ずつ列挙**。集約 (`hint: 3 files matched as *.json fallback`) は採用しない (どのファイルが該当なのか追えなくなり hint 価値が下がる)。

### 4. 抑制フラグ

v0.5.0 の `--no-hint` / `-q` / `-qq` で抑制可能。既存の「files not modified」hint と同じ抑制機構に乗る:

- `--no-hint`: hint のみ抑制
- `-q` / `--quiet`: stdout + hint
- `-qq` / `--quiet-all`: stdout + hint + error

CI で hint 不要なら `--no-hint` を付ける運用が自然。

### 5. v0.5.0 既存 hint との共存

`bump-semver patch unknown.json` で両方発火するケース:

```
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
hint: 1 file not modified; use --write to update or --no-hint to suppress
```

順序は **fallback hint が先、`--write` hint が後** (発生時系列順: ルール解決 → 書き戻し未実行)。両方とも同じ抑制フラグで一括抑制される。

`hint:` prefix で揃えることで、CI の grep フィルタが両方を一括捕捉できる。

### 6. unsupported file エラーへの誘導文言

完全に未対応のファイル (`unknown.toml` のような fallback も無いケース) は引き続き `unsupported file: <path>` でエラーするが、文言を拡張して issue 誘導を追加する:

```
unsupported file: unknown.toml
hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.
```

`bump-semver: ` prefix は run() の `emitErr` で付くので、最終的な stderr 出力は:

```
bump-semver: unsupported file: unknown.toml
hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.
```

`hint:` 行は `--no-hint` / `-q` / `-qq` で抑制可能 (発信源は同じ「明示対応されていないファイルを使った」 → 誘導目的も同じだから一貫させる)。

## 不採用案

### A. confidence 2 でも hint を出す

「basename 一致だが path-pinned に届かないルール」も「ユーザに明示対応してほしいケース」と捉える案。検討したが、DR-0005 の confidence 2 は **任意 dir の `marketplace.json` を `.metadata.version` として扱う** ような明示的な basename サポートのために設計されており、ここに hint を出すと正常系で常に hint が出続ける (うるさい)。

confidence 2 で「本当は対応してほしいファイル」だった場合は、現状すでに confidence 2 → confidence 1 にフォールスルーする動作になっており、その時点で confidence 1 の hint が出る。設計上の隙間は無い。

### B. ファイル名を集約 (`hint: 3 files matched as *.json fallback`)

複数 FILE が同時に confidence 1 にヒットしたとき、行数を抑えるための集約案。検討したが:

- どのファイルが該当か追えなくなる → hint の本来目的 (issue を立てる時のファイル特定) が損なわれる
- 1 ファイルあたり 1 行は他の v0.5.0 hint と一貫した粒度

集約は採用せず、1 ファイル 1 行で出す。

### C. hint 文言に GitHub URL を含める

`Open an issue at https://github.com/kawaz/bump-semver/issues if explicit support is needed.` と URL を含める案。fallback hint は **頻発する可能性がある** (`*.json` fallback はわりと普通に発生) ため、CI ログの 1 行が長くなりすぎる。短縮形 `Open issue if explicit support is needed.` を採用 (URL は help text や README で案内すれば十分)。

一方、unsupported file エラーは **発生したら作業が止まる** ため、ログ 1 行の長さよりも次のアクション (issue を立てる) を即座に示す方が価値が高い。こちらは URL 込みで明記する。

### D. Inspection 構造体に `MatchedRuleConfidence` を追加せず、Handler interface に method を追加

`Handler` interface に `MatchedRule() *CandidateRule` のような method を追加する案。検討したが、`Handler` は format 抽象化のための interface であり、ruleHandler の概念をそこに混ぜると interface が肥大する。`Inspection` は「Inspect の結果」であり、どのルールでマッチしたかは結果情報の一部と見なせる (matched confidence/glob は Inspect の副産物)。`Inspection` に `MatchedConfidence` / `MatchedGlob` を持たせる方が責務が自然。

### E. fallback hint を warning prefix (`warning:`) で出す

「fallback で動いた」のは正常系で、警告ではない (書ける限り書く + 必要なら明示対応してね、という案内)。`hint:` prefix で v0.5.0 既存 hint と一貫させる方が、抑制フラグ (`--no-hint`) の意味とも合う。

## Consequences

### 実装変更

- `src/handler.go`: `Inspection` 構造体に `MatchedConfidence int` / `MatchedGlob string` を追加。`Handler.Inspect` の戻り値で main 側に伝える経路を確保
- `src/rules.go`: `resolveRule` が成功した時、選ばれた rule の `Confidence` / `Glob` を `Inspection` に詰める。エラー文言を `unsupported file: <path>\nhint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.` に拡張
- `src/handler.go`: `detectHandler` の `unsupported file` エラーも同じ拡張文言に揃える
- `src/main.go`: `runBump` / `runGet` (実体は `runBump` の get 分岐) で confidence==1 の resolved input を列挙して fallback hint を stderr に出力。既存の「files not modified」hint より前に出す。`shouldShowHint` と同じ抑制フラグで判定
- `src/main_test.go`: fallback hint テスト + `--no-hint` / `-q` / `-qq` 抑制テスト + unsupported file エラー文言テスト

### 後方互換

- CLI 表面は完全に維持
- stderr に追加で hint が出るようになるが、stdout は不変 (CI の stdout pipe は影響を受けない)
- 既存の hint 抑制フラグでまとめて抑制可能

### v0.7.2 (本 DR の実装) 後の状態

- Phase: confidence 1 fallback hint が動作 (本 DR)
- 次 Phase: suffix 吸収 (`docs/issue/2026-05-10-suffix-stripped-format-detection.md`) — 同じ hint 機構を再利用予定 (suffix 吸収で動いた時は `hint: <path> matched as <basename> via suffix stripping...` のように共通の `hint:` prefix で報告)

### ROADMAP

`docs/ROADMAP.md` には特に追加なし (本 DR は機能追加だが小粒)。

## 関連

- DR-0001: basename 自動判定 + 「必要が出たら 1 行追加」哲学 (本 DR は哲学のフィードバックループ装置)
- DR-0005: path-aware confidence ranked candidates (本 DR の前提機構)
- v0.5.0: `--no-hint` / `-q` / `-qq` 抑制機構 (本 DR は同機構に相乗り)
- 関連 issue: [`docs/issue/2026-05-10-suffix-stripped-format-detection.md`](../issue/2026-05-10-suffix-stripped-format-detection.md) — suffix 吸収時に同じ hint 機構を共通化予定
