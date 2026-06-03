# 2026-06-03 — DR-0029 / DR-0030 候補の draft 起票 + multi-persona review

## 概要

CLI から「自分のファイルにこの rule」を指定する口 (= `--define-rule` ブロック方式) の
Phase 1 案を docs/issue/ に起票し、nitpick × 2 (設計 / 振る舞い) + codex (= 5 年負債観点、
完了通知待ち) のマルチペルソナレビューを並列実施。1 周目反映 → 2 周目 nitpick → 反映の
2 周目までこなした状態で commit。

並行して **format=regex 廃止 + format=text + version-regex 統合** の DR-0030 候補も
姉妹 issue として起票 (= kawaz 指示で DR-0029 から分離)。

## 起票した issue ファイル

- `docs/issue/2026-06-03-cli-user-defined-rule.md` (= DR-0029 candidate、本日 morning に
  別セッション [claude-session-analysis] で起票したもの。引き継ぎ後本セッションで改訂)
- `docs/issue/2026-06-03-format-regex-to-text-unification.md` (= DR-0030 candidate、
  本セッション中に新規起票)
- `docs/issue/2026-06-03-format-request-window.md` (= memo、別セッションで起票。
  DR 起票不要の運用整備)

## ハマり所 → 解決策

### ハマり 1: `--format` enum に `regex` を含めるかで二重命名問題

- 設計 nitpick 1 周目で「`--format text` と builtin `format: regex` (DR-0012) の二重命名が
  破綻」と指摘
- kawaz から「そもそも format=regex が概念的に間違い、regex は format ではない」との
  本質指摘
- → DR-0029 とは別建ての DR-0030 として「format=regex 廃止 → format=text + version-regex
  統合」を分離合意 (= 関心事が違うので別 DR)。実装順は DR-0030 → DR-0029 (= internal
  統合してから CLI 露出)
- DR-0029 の `--format` enum は `<text|json|yaml|toml|xml>` の 5 値で確定

### ハマり 2: 「ブロック外位置の rule 系 flag は error」の矛盾

- 2 周目 nitpick で致命矛盾を指摘: line 587 のスコープ規約と line 613-617 のブロック
  終了規則が両立しない (= positional はブロック終了シグナルではないので「ブロック外」
  状態が論理的に発生しない)
- 解決策: positional はブロックスコープに **透過** と明示し、「ブロック外」状態は
  **`--define-rule` の typo / 抜けでしか発生しない** と書く (= 案 b 採用)
- typo 防御は argparse 層 (= unknown option を error) + 0a 補強規約 (= 「最初の
  `--define-rule` より前のみ global flag を許す」厳密順序) の 2 層で実現

### ハマり 3: tier 4 (ドット相対) が cwd 依存で再現性なし

- 設計 nitpick 1 周目で「tier 4 を削除、tier 3 と統合」と指摘
- 解決策: tier scoring を 5/3/2/1/0 の **4 段に整理**。`./X` は **tier 3 の単なる書き方**
  (= `filepath.Clean` で `X` に正規化されてから比較)、別 tier を作らない
- 振る舞い nitpick の path 正規化規約 (= `filepath.Clean` + symlink resolve しない) と
  整合

### ハマり 4: dead block / dead global の検出基準

- 2 周目 nitpick で「全 SOURCE が block でカバーされた時の global が dead code」「どの
  SOURCE にもマッチしない `--define-rule` が dead block」の 2 新規穴を指摘
- 解決策:
  - **dead block** → **error** (= ユーザが書いた `--define-rule` が silent に無視
    されると debug 困難)
  - **dead global** → **warning** (= override されない SOURCE を想定して書いた意図と
    区別困難なため、error にしない)。`--no-hint` / `-q` で抑制可能

### ハマり 5: codex の結果取得経路

- `codex:codex-rescue` subagent は内部で codex job (= task-mpxf1xje-66lpzr) を forward
  するだけで job 完了通知は kawaz の console に push される
- AI からは `/codex:result <job-id>` が `disable-model-invocation: true` で叩けない
- → 本セッションでは codex 結果待ちで止めず、nitpick × 2 + 2 周目 nitpick だけで 1 周目
  確定。kawaz が console から貼ってくれた時点で 2 ラウンド目として反映 → 完了
- 注: subagent が forward 時に表示した「job ID: b2xg9czae」は **不正確** で、`/codex:status`
  で見える正しい job ID は `task-mpxf1xje-66lpzr` だった。kawaz が `/codex:status` で
  正しい ID を確認 → `/codex:result task-mpxf1xje-66lpzr` で取得
- nonstop モードで AskUserQuestion 禁止下のため、待ちで止まる選択肢を取らず、出来る
  ところまで進める判断

### ハマり 6: codex 指摘 Critical C-1 (= write 書き戻し仕様の盲点)

- 初心者ペルソナまでで 4 観点反映済としていたが、codex の Critical C-1 で「`info.json`
  の `$.name` から `"myapp v1.0.5"` を path で取得、regex で `1.0.5` を抜く例」の **bump
  --write 時の書き戻しアルゴリズムが未定義** という致命穴を指摘される
- 解決策: Phase 1 で書き戻しアルゴリズムを pin (= path で scalar string を取得、regex
  group 1 の byte range だけ置換、元の path に scalar として戻す)。非 string scalar /
  array / object は error。これにより「path 値全体を上書きする」のか「regex 部分だけ
  置換する」のかの曖昧さが解消

### ハマり 7: codex 指摘 Critical C-2 (= 1 match 制約)

- DR-0012 builtin は first-match-only (= 線形検索で最初のマッチを採用、line-anchored
  推奨)。CLI rule もこれを継承すると書いていた (= 旧論点 7)
- codex 指摘: CLI rule は **builtin より厳格** にすべき (= exact one match、0 / 2+ は
  error)。理由は「ユーザが明示指定したのに silent な first-match 採用は debug 困難 +
  誤書換え事故温床」
- 解決策: CLI `--version-regex` / `--name-regex` は exact-one-match で error。builtin は
  従来通り first-match-only。help / docs で「user-defined は builtin より厳格」と対比
  明示

### ハマり 8: codex 指摘 Critical C-3 (= `--format xml` の path 契約未定義)

- draft 初版は `--format <text|json|yaml|toml|xml>` の 5 値で公開していた
- codex 指摘: XML は JSON/YAML/TOML と木の semantics が違う (= 要素繰り返し / 属性 /
  テキストノード / 名前空間 / root anchoring) ため、共通 dot-path で扱う契約に無理。
  enum に xml を入れたまま path 契約を後決めにすると、後から slash-path に変える /
  属性記法を足す / 配列規則を変える のどれも互換破壊
- 解決策: **Phase 1 では `--format` enum から xml を外す** (= 4 値: text/json/yaml/toml)。
  xml は Phase 2+ で別 path language (= XPath subset / slash-rooted) を別 DR で設計
  してから解禁。これにより Phase 1 で曖昧な XML 契約を pin する事故を回避

### ハマり 9: codex 指摘 High H-3 (= name safety rail の薄まり)

- 現行 builtin は multi-input 時に name も cross-check して「別 project を一緒に
  bump しない」guard を提供 (README L476-478)
- user-defined rule で `--name-*` を optional とすると、name source なしの source は
  cross-check 対象から外れる = safety rail が薄まる
- 解決策: user-defined で `--name-*` を **書いた** source は name-check 対象。書かな
  かった source は除外するが、その際 **stderr に warning hint** を出す (= silent
  downgrade を避ける)。`--no-hint` / `-q` で抑制可能

## 反映済の主要 Decision (= DR-0029 候補)

- **`--define-rule <PATTERN>` ブロック方式** (= 案 1 採用、対抗案 2/3 は不採用)
- **PATTERN tier 5/3/2/1/0 の 4 段** (= 旧 tier 4 削除、`filepath.Clean` 正規化 +
  symlink resolve しない)
- **`glob:` prefix のみ受け付け** (= bare PATTERN は完全 literal、glob meta 文字含む
  なら error)
- **`vcs:` 借用形式 = 兄弟 FILE 独立継承 / 単独形式 = VCS root 相対**
- **Block scope の rule 完全宣言規約** (= block で 1 flag でも書いたら部分継承禁止)
- **0a 補強 (= 最初の `--define-rule` より前のみ global flag を許す)** + dead block error
  + dead global warning
- **論点 4 確定: bump --write も CLI rule で許可** + atomicity (= 全 SOURCE validate
  後にまとめて write)
- **論点 6 確定: name flag も Phase 1 に含める** (= 対称性原則)
- **論点 7 確定: CLI `--version-regex` は DR-0012 builtin と同規約** (= first match,
  capture group 1、line-anchored は推奨)
- **Help 配置: 各 verb `--help` は summary 行、root `--help-full` に "User-defined
  rules" 専用セクション**
- **Design rationale コメント必須** (= 位置依存パーサ = kawaz CLI 規約の意図的例外を
  コードに明記)
- **codex Critical 反映**: `--format` enum は **Phase 1 で 4 値** (text/json/yaml/toml、
  xml は Phase 2+) / CLI `--version-regex` は **exact one match** (= builtin の
  first-match-only より厳格) / `--version-path + --version-regex` 併用時の
  **bump 書き戻しアルゴリズム** を pin (= path で scalar string 取得 → regex group 1
  の byte range だけ置換 → 元の path に戻す)
- **codex High 反映**: CLI rule の extraction 失敗は **hard error** (= builtin
  fallback なし) / Phase 2 config schema は CLI block の direct serialization では
  なく **internal rule object に近い form** (= `version_paths[]` / `match_mode` /
  `rewrite_mode`) を採用、Phase 1 syntax は CLI 体験の seed として残る / name
  safety rail は user-defined でも維持 (= name source なしは warning hint、silent
  downgrade なし)

## 反映済の主要 Decision (= DR-0030 候補)

- **`format=regex` 廃止 → `format=text + VersionRegex` に統合** (= internal refactor、
  機能変更なし)
- **format enum は 5 値**: `text` / `json` / `yaml` / `toml` / `xml`
- **影響範囲**: `src/rules.go` の `Format: "regex"` リテラル 12 箇所 + dispatcher
  `case "regex":` 2 箇所 + format_regex.go / format_plain.go の名称統合
- **DR-0012 partial supersede**
- **Open question**: `format=plain` も text に統合するか (案 A) / `plain` は残すか
  (案 B)。実装時推奨は案 A (= 概念整合性最大)

## DR rename の前提 (= 次セッション or codex 結果反映後)

issue file → DR rename 時に必要な作業 (= Next action セクションに記録):

1. status を `Draft` → `Accepted` に変更
2. 「(= 2026-06-03 nitpick 反映)」等の出自注記を **一括 strip**
   (= `feedback_no_process_noise_in_final_docs` ルール)
3. 取り消し線 (= 軸 2 表 (iv) / ~~論点 4: ...~~) を削除
4. 議論プロセスの章 (= 「軸 1: 入口の形」「軸 2: 指定構文」「案 1/2/3 比較」) を削除し、
   採用案の説明に集約
5. INDEX.md に DR 番号で追記
6. `docs/issue/2026-06-03-cli-user-defined-rule.md` → `docs/decisions/DR-0029-cli-user-defined-rule-phase1.md`
   (DR-0030 候補も同様に rename)
7. `docs/issue/2026-06-03-format-request-window.md` は **memo のまま残す** (= 実装着手
   時に delete)

## 実機調査メモ (= help 階層の現状)

- `bump-semver --help-full` は **root のみ存在**、subcommand では unknown option エラー
- `bump-semver vcs tag --help` 等のネスト help も未整備 (= `vcs tag` を 1 verb として
  認識せず、tag が action として解釈される構造的問題)
- kawaz CLI 規約「子・孫のネストした subcommand を持てること」「各レベルで help 階層」
  と現状実装に乖離あり
- DR-0029 の help 配置は **root `--help-full` に詳細を集約**する暫定運用で Phase 1 を
  動かす (= 各 verb の `--help-full` 実装 / vcs ネスト help 整理は Phase 2+ 別 issue)

## 関連

- `docs/issue/2026-06-03-cli-user-defined-rule.md` — DR-0029 candidate (本日の主要
  起票物、本セッション中に 2 周分のレビュー反映)
- `docs/issue/2026-06-03-format-regex-to-text-unification.md` — DR-0030 candidate
  (本セッションで新規起票)
- `docs/issue/2026-06-03-format-request-window.md` — memo (別セッションで起票)
- 既存 DR: DR-0001 / 0005 / 0010 / 0012 / 0023 / 0024 / 0027 / 0028 (= related)
