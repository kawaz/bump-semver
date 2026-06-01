# vcs outdated MVP 実装ジャーナル

Date: 2026-06-02
Scope: DR-0027 land
Status: Phase A-H 完了、commit 済み、push は親プロセス担当

## 何をやったか

DR-0027 (= 派生 sync check の mini-DSL + `regex:` 不採用) の MVP を 1 セッションで
land。Phase A (glob backref) → B (TO 側 expansion) → C-D (`vcs outdated` 本体 +
`--explain`) → E (dispatcher 接続) → F (テスト) → G-H (README 両言語) の順で
進行、最後に `just ci` で全体検証 + 3 固定題材を手動で確認。

## ハマり所と解決

### 1. doublestar v4 にサブマッチ API がない

`doublestar.FilepathGlob` は `[]string` の path 列を返すだけで、wildcard の
キャプチャを取り出す方法を提供しない。事前の `go doc` で確認:

```
func Match(pattern, name string) (bool, error)
func PathMatch(pattern, name string) (bool, error)
func Glob(...) / FilepathGlob(...) → matches only
```

→ 解決: glob → regex 翻訳器 (`globToCaptureRegex`) を自前実装し、doublestar
で fs マッチ + 列挙し、各 path を regex で再評価して captures を取り出す
2-pass 方式。doublestar の fs セマンティクスは温存しつつ、regex は **純粋に
capture 抽出のためだけ** に使う構造。

### 2. `**` の greedy 問題

最初 `**` を `(.*)` で書いたら `src/**/*.ts` で `src/sub/bar.ts` を見たとき
`**` が `sub/bar` を全部食って `*` がカラになる事故。

→ 解決: `**` を **non-greedy** `(.*?)` に。`$` anchor の引っ張りで最終
セグメントが正しく `*` に届く。`*` 自体は segment-bounded (`[^/]*`) なので
greedy のままで OK。

### 3. `**` がゼロセグメントマッチの時の `$1=""`

DR-0027 の例 `glob:src/**/*.ts` → `src/foo.ts` で `**` が 0 セグメント
マッチすると `$1=""`、`lib/$1/$2.js` 単純展開で `lib//foo.js` になる。

→ 解決: 後処理で連続 `/` を 1 つに縮約 (`collapseSlashes`)。Phase A/B 段階
で advisor の指摘どおりテストに固定 (`TestExpandTOPath_ZeroSegmentDoubleStar`)。
代替案 (= `**/` の `/` を消費するルール) もあるが、`**` 単独 + 別の `/` という
ケースが扱いにくく、純後処理の方が単純。

### 4. jj の `committer_date(after:...)` は unix epoch を受け付けない

最初 `committer_date(after:"@1780339225")` を試したら "Invalid date pattern"。
jj は ISO-8601 タイムスタンプ (`2026-06-02T12:33:46Z` 等) を要求。

→ 解決: `time.Unix(sinceTS+1, 0).UTC().Format("2006-01-02T15:04:05Z")` で
ISO 化してから埋め込み。`+1` は git の `--since=ts+1` と同じ "strict-newer"
セマンティクスを保つため。

### 5. dispatcher の `--` slurp と衝突

既存 `parseVcsArgs` は `case a == "--"` で残り全部を `vcsArgs` に slurp +
ループ終了。`vcs outdated` は `--` を「ペア区切りとして vcsArgs に literal で
残す」必要がある (= `splitOutdatedPairs` がペアに分割する)。

→ 解決: dispatcher に `case a == "--" && vcsVerb == "outdated"` の verb-gate
を追加し、`outdated` のときだけ `--` を literal token としてプッシュ。他の
verb は従来挙動 (slurp + 終了) のまま。

### 6. `vcs outdated` バックエンドメソッド追加

`vcsBackend` interface に `FileTimestamp` + `CountCommitsSince` を追加。
既存パターン (= DR-0020 PR-1..6 で interface を incremental に育てる) に
合わせて末尾に追加。両 backend (git / jj) で同じ contract を満たす実装。

## 検証

- 単体: `glob_backref_test.go` (= 11 ケース、星単独 / `**` / `{}` / `[]` /
  リテラル / no-match / TO substitution / brace expansion / 0-segment ** /
  out-of-range backref / mandatory-vs-optional 判定)
- 統合: `cmd_vcs_outdated_test.go` (= 9 ケース、T1 / T2 / T1 all-fresh /
  --explain / multi-pair / missing mandatory / auto-exclude / usage error /
  bare-verb help routing + splitOutdatedPairs 6 サブケース)
- 手動: T1 / T2 / T3 / 集約 4 パターンを git + jj backend で `--explain`
  動作確認、全て DR-0027 spec どおり

`just ci` は lint + 全テスト 128s + build がパス。

## 次の action

- main push は親プロセスが `pkf run push` でまとめて担当 (= release-flow-awareness
  の標準ループ、tag/release は Claude が打たない)
- scope-out 項目 (DR-0027 §D = named alias / `cmd:` / `.sync.yml` / `{}`
  ネスト / 2 桁 backref / `--json`) は需要発生時に別 DR で別 PR

## 関連

- DR-0027 — 採用判断の正本
- DR-0024 — `glob:` 基盤
- DR-0020 — vcs subcommands ファミリ、本 verb は同ファミリへの追加
- 既存翻訳 check (`pkf-tasks/tasks/docs/translations.pkl`) — ts 比較ロジックの
  起点、`vcs outdated` 移行で代替可能 (今後 task runner 側で切り替え検討)
