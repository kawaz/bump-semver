# DR-0027: 派生 sync check の mapping DSL 採用 (`vcs outdated`) / `regex:` 不採用

- Status: Active
- Date: 2026-06-02
- Extends: DR-0024 (`glob:` prefix), DR-0020 (vcs subcommands)
- Supersedes: `docs/issue/derived-sync-check-cli-requirements.md` (= 要件発散 issue を本 DR で結論化、delete)

## Context

「source が更新されたら、一緒に更新されるべき derived が古いままじゃないか」を確認する派生 sync check CLI の設計議論。出発点は翻訳ペアだが、bundle / 生成コード / lock / schema migration 等の同構造ケースに **本質汎化** (kawaz、2026-06-01) — 翻訳特化思考を解除、タスクランナーの cache 差分 / 再実行文脈に近い汎用 check として再定義。

検討プロセス:

1. 要件発散 (`docs/issue/derived-sync-check-cli-requirements.md` 1-9 章) — マッピング軸 7 種 / 派生不在の扱い 7 種 / 鮮度比較機構 7 種 / 副次 check / 引数案 A-I / 実装場所候補 5 種を並列展示
2. kawaz 起案 mini-DSL (同 10 章) — `glob:` 拡張で **可変パーツが後方参照レジスタに順に登録** + `{}` 必須 / `*`/`**`/`[]` 任意 の二分 + `--` ペア区切り + 自動除外。kawaz 自己評価: 「自分でも驚くほどなんかこの mini-DSL いけてそうでびっくり」(2026-06-01)
3. 内部 self-review (同 10.9 検討漏れ candidates A-O / 10.10 UX 観点) — 弱点候補を洗い出し
4. マルチペルソナ adversarial レビュー (agent `ad8476c30f590f0b5`) — 12 候補案 × 6 ペルソナで評価、元案を超える対抗案なしと判定
5. codex adversarial review (job `task-mpvfchbn-m6f9rj`) — 検討漏れ網羅性 + UX 受容性 + 5 年負債観点
6. kawaz × Claude 議論 (2026-06-02) — `()` capture 案 / `regex:` 検討の却下理由を詰める

採用判断に到達。本 DR が結論を pin する。

## Decision

### A. mapping DSL = mini-DSL (= kawaz 起案、`glob:` 拡張)

MVP 仕様:

| 要素 | 仕様 |
|---|---|
| 基盤 | `glob:<pattern>` プレフィックス (DR-0024) |
| 可変パーツ | `*` / `**` / `{a,b,c}` / `[abc0-9]` の 4 種 (DR-0024 と同じ) |
| 後方参照 | 各可変パーツが **登場順に backref レジスタへ追加**、`$N` / `${N}` で TO 側 path に embed |
| `{}` 必須 | `{a,b,c}` の全展開 path は **必須存在** (不在 → fail) |
| `*`/`**`/`[]` 任意 | wildcard マッチ集合は **任意** (不在 silent skip、マッチしたものだけ check) |
| ペア区切り | `vcs outdated FROM TO[..]` (1 ペアは `--` 省略可) / `vcs outdated -- F1 T1[..] -- F2 T2[..]` (複数ペアは `--` 必須) |
| 自動除外 | FROM が TO の glob にマッチしても、起点 FROM 自身は派生集合から除外 |
| 鮮度比較 | committer timestamp (jj/git)、`tgt_ts < src_ts` で lag (= 現行翻訳 check と同方式) |
| 未追跡 | ts=0 正規化 (= 現行継承) |

サンプル (= 3 固定題材):

```
# T1: bundle
vcs outdated 'glob:src/**/*.ts' 'lib/$1/$2.js'

# T2: 翻訳 (複数 derived 必須)
vcs outdated README.md 'README-{ja,en}.md'

# T3: 生成コード
vcs outdated 'glob:proto/**/*.proto' 'generated/$1/$2.pb.go'

# 集約 (3 ペア 1 コマンド)
vcs outdated \
  -- 'glob:src/**/*.ts' 'lib/$1/$2.js' \
  -- README.md 'README-{ja,en}.md' \
  -- 'glob:proto/**/*.proto' 'generated/$1/$2.pb.go'
```

### B. subcommand verb = `vcs outdated`

採用理由:
- persona 集約で `outdated` が短く意図明確 (= make 慣習に乗れる、release gate 文脈で明確)
- `sync-check` は file sync 連想で曖昧 (kawaz feedback 2026-06-02 「微妙にしっくり来てない」)
- `check-derived` は「check って何をチェック?」「OK/NG は?」の曖昧さ残
- 日本人にも `outdated` は馴染みやすい (kawaz feedback 2026-06-02)

### C. `--explain` mode 必須

- `vcs outdated --explain ...` で **展開後の派生 path 列 + 鮮度判定** を一覧表示
- `$N` 位置 backref の順序 ambiguity (= 元案最大の失敗モード) を運用で吸収
- `dry-run` ではなく `explain` を採用: dry-run は「実行しないだけ、シャドー実行と同じ」ニュアンス、`explain` は「内部状態確認」ニュアンスで本機能の意図に合致

出力イメージ:

```
$ vcs outdated --explain 'glob:src/**/*.ts' 'lib/$1/$2.js'
src/foo.ts        → lib/foo.js          [fresh: derived ts >= source ts]
src/bar/baz.ts    → lib/bar/baz.js      [missing, will fail]
src/qux/zap.ts    → lib/qux/zap.js      [stale: derived 3 commits behind source]
```

### D. scope-out 項目 (= MVP 外、需要ベースで将来別 DR)

- **named alias** (`${name:pattern}` 形式) — P3 snakemake / P4 可読性 persona 駆動で論拠あるが、kawaz は「named は別経路」と pre-bucket 済。`$N` 一本 + `--explain` で MVP は十分
- **`cmd:` GENERATOR** (= 外部コマンド scheme) — kawaz 反論 #4 の「別経路」、Phase 3+ で別 DR
- **`.sync.yml` / `file:LIST`** (= 明示 pair config) — 同上、補完経路として将来併存
- **mini-DSL のエッジケース詰め** (= `{}` ネスト、`$N` 桁拡張等) — 需要発生時に extend

## 不採用案

### `regex:` プレフィックス (= 主要却下対象、本 DR の補強根拠)

却下理由:

| 観点 | glob: (採用) | regex: (却下) |
|---|---|---|
| マッチ semantics | filesystem 上の実存ファイル | 文字列パターン |
| `**` の意味階層 | path separator 跨ぎ + fs 再帰探索 | `.*` で文字列レベル任意 (= fs 意味なし) |
| `{}` 必須 / `*`/`**`/`[]` 任意の二分 | 自然な独自意味付け (= 「必須 vs 発見」の自然なカバー) | regex に必須/任意の自然な区別なし |
| 発見 or 新規の表現 | 上記二分の副産物として自然カバー | 別 flag で表現する羽目 |
| portable subset | OS 横断 (= doublestar v4 / mmv / shell glob で共通) | PCRE / POSIX BRE / ERE / Go regex で dialect 差大 |
| capture group | 採用案 (a) は不要 (= 可変パーツが自動 backref) | `()` 必須、DR-0024 §10.7 (escape 不採用) と矛盾 |
| 計算コスト | doublestar 経由で fs walk + match を一気に | 「全 file enumerate → regex filter」は爆発、「regex 側に fs 意味論」は doublestar 再実装 |

「regex は当然マッピングを考えた時に最初に思いつく」(kawaz、2026-06-02) のは事実。ただし **`**` を regex と fs 解決に落とし込めない** (kawaz 同) のが本質。regex で fs マッチを書く 2 経路 — (1) 全 file enumerate → regex filter で計算爆発、(2) regex に fs 再帰意味論を持たせる (= それはもう glob、独自実装) — どちらも不健全。

採用案 (a) が達成した 3 つの kawaz「驚きポイント」 — **fs 実存マッチング** / **{}必須・*任意 の二分** / **発見・新規の自然カバー** — **全て regex 移行で消える**。「ぱっと見の印象が regex っぽい」(kawaz、2026-06-02) ためだけに 3 つの本質利点を捨てる取引 = 明確に劣化。

AXIS 3 (= DR-0024 で defined した依存コスト / 網羅責務) 違反でもある。

### `()` capture (= zsh `zmv` 系譜の明示 capture)

却下理由:
- glob 本来の filename-representability を犠牲: `(foo)/(bar)-ja.md` のような括弧を含む path / ファイル名を glob で扱えなくなる
- escape (`\(`/`\)`) の導入を強制 → **DR-0024 §10.7 「クオート / エスケープ仕様は導入しない」と真っ向矛盾**
- P2 (regex/sed persona) の好感は高いが、hard 制約 (filename 表現性 + DR-0024) が勝つ

### lambda script (= JS / Lua engine 内蔵で translator)

却下理由:
- 依存爆発: goja / otto / lua engine 内蔵でバイナリサイズ激増、sandbox 設計コスト
- AXIS 3 (依存コスト) 真っ向違反
- スクリプト bug が release gate 全体を破壊しうる

### `--map regex-replace` flag (= sed 風)

却下理由:
- regex を新規責務として bump-semver が抱える (= AXIS 3 違反、`regex:` プレフィックス案と同じ問題)
- silent zero-match の温床 (= P5 CI persona が最も嫌う)
- 複数 derived 表現が awkward (= 空白区切り? 配列?)

### named group (= `{from:**}` 等、`{}` 文法衝突)

却下理由 (= 全置換として):
- `{}` が「必須展開」と「named capture」の二重意味化 = 文法衝突
- 別記法 (`<name>` / `(?P<name>...)`) は事実上 `()` 案 + alias で escape 問題が残る
- kawaz 反論 #4 「named は別経路」と整合
- (将来 optional alias として scope 内追加するかは別 DR で要望ベース)

### anchored label / shell-expansion 風 (= bash parameter expansion 風) / その他

棄却。マルチペルソナレビュー (agent `ad8476c30f590f0b5`) で全候補評価済、`(a) mini-DSL` を超える対抗案なし。

## prior art (= 「後方参照できる glob」は新発明ではない)

ワイルドカード後方参照には確立された 2 系統:

| 系統 | 代表例 | 機構 | 本 DR との関係 |
|---|---|---|---|
| 位置 backref | MS-DOS COMMAND.COM (`copy *.txt *.bak`) / Unix `mmv '*.txt' '#1.bak'` | 可変パーツが順に backref レジスタに足される | **採用案 (a) と同系譜** |
| 明示 capture + backref | zsh `zmv '(*).txt' '$1.bak'` | `()` で明示 capture、`$N` で参照 | **却下案 `()` capture と同系譜** |

本 DR は **位置 backref 系統** (= MS-DOS / mmv 系譜) を `glob:` の portable subset + fs 実存マッチング + `{}` 必須 / `*`/`**`/`[]` 任意の二分という現代的拡張として組み合わせる。

「glob にキャプチャ概念が無かった」(kawaz、2026-06-02) は厳密には「glob は参照展開のみで使われてきた歴史」が正確。後方参照付き glob は「人類が思いつかなかったわけでなく、確立された 2 系統が個別に存在」している領域。本 DR は系統 1 を選び、kawaz の `glob:` portable subset (DR-0024) と二分仕様で **派生 sync check という用途に最適化** する。

## 採用案の弱点と mitigation (= 採用後の運用ガイド)

| 弱点 | mitigation |
|---|---|
| `$N` 位置 backref の順序 ambiguity (= `**`/`*`/`{}`/`[]` の登場順を user が誤読しうる) | **`--explain` mode 必須化** で展開後 backref + 派生 path を可視化 |
| `{}` ネスト禁止 (= MVP 1 階層のみ) | help / error メッセージで明示 |
| shell escape (`$N`/`{}`/`--` が shell 特殊文字と衝突) | **single quote 必須** を help 冒頭で明示、Examples セクションで強調 |
| `{}` 必須 vs `*` 任意 の二分は doc-or-die | help の Examples に「`{ja,en}` は両方必要 / `[a-z][a-z]` はマッチ集合任意」の対比サンプル |
| 鮮度比較 timestamp は rebase / cherry-pick で再生成される | 既知の仕様限界 (= 3.1 既述)、ts 比較の特性として README に明記 |
| 未追跡 / 未 fetch の ts=0 正規化 | 現行翻訳 check と同等、`--strict` で fail 化は将来検討 |

## 影響範囲

### 新規

- `src/cmd_vcs_outdated.go` — `vcs outdated` 実装 (= dispatcher + verb)
- `src/cmd_vcs_outdated_test.go` — 3 固定題材 + `$N` 順序 + `{}` 必須 + 自動除外 + `--explain`
- `src/glob_backref.go` (or 既存 `glob.go` 拡張) — 可変パーツの backref レジスタ + TO 側展開
- `src/glob_backref_test.go`
- `docs/journal/2026-06-02-mini-dsl-finalized-and-regex-rejected.md` — land 経緯

### 既存変更

- `src/cli_parse.go` — `vcs outdated` dispatcher、`--explain` flag、`--` 複数ペア解釈
- `src/glob.go` — 既存 glob expansion を「マッチした可変パーツを `[]string` で保持」拡張
- README / README-ja — `vcs outdated` 仕様 + Examples + shell escape 注意 + `--explain` 用法

### 削除

- `docs/issue/derived-sync-check-cli-requirements.md` — 本 DR で結論化、delete (= docs-knowledge-flow rule)

## Phase 分離

- **Phase 1**: `glob:` prefix 単体 — land 済 (DR-0024、v0.30.0)
- **Phase 2** (= 本 DR): `vcs outdated` + mini-DSL + `--explain` MVP land
- **Phase 3+**: scope-out 項目 (= named alias / `cmd:` GENERATOR / `.sync.yml`) を要望ベースで別 DR

## 検証

- mapping パターン matrix (T1 bundle / T2 翻訳 / T3 生成コード) で fixture 検証
- `$N` 位置 backref 順序 (`**` / `*` / `{}` / `[]` 単独 + 組み合わせ)
- `{}` 必須 vs `*`/`**`/`[]` 任意の境界 (= 不在時 fail / silent skip)
- `--explain` 出力 (= 展開後派生 path + 鮮度判定の構造化表示、`--json` 検討)
- multi-pair (= `--` 区切り) の独立性 (= ペア間で名前空間分離、`$N` は各ペア独立)
- 自動除外 (= FROM が TO に含まれた場合の起点除外)
- 鮮度比較 (= ts ベース、ts=0 正規化、現行翻訳 check との同等性)
- shell escape (= single quote / double quote / unquoted で `$N`/`{}` がどう動くかの動作確認)

## 関連

- DR-0020 PR-x (= vcs subcommands): `vcs outdated` は本ファミリの追加 verb
- DR-0024: `glob:` prefix 基盤 (= 本 DR の MVP は DR-0024 拡張として実装)
- 既存翻訳 check (= `pkf-tasks/tasks/docs/translations.pkl`): ts ベース比較の起点、`vcs outdated` 移行後は本 verb で代替可能
- ペルソナレビュー結果: agent `ad8476c30f590f0b5` のレポート (= journal に転記推奨)
- codex review: job `task-mpvfchbn-m6f9rj` (= journal に転記推奨)
- prior art: MS-DOS COMMAND.COM `copy *.txt *.bak`, Unix `mmv`, zsh `zmv`
