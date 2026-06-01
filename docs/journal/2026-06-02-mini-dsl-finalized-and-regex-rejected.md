# 2026-06-02 派生 sync check mini-DSL 採用確定 / `regex:` 不採用

要件発散 issue (`docs/issue/derived-sync-check-cli-requirements.md`) を **DR-0027 で結論化、delete**。`vcs outdated` verb + mini-DSL MVP + `--explain` 必須 + `regex:` 不採用方針を pin。

## 経緯

| 時系列 | 出来事 |
|---|---|
| 2026-05-30 (e4f74abe) | kawaz の元アイデア「A → A' リスト、コミット鮮度比較」 |
| 2026-05-30 (6a0a262f, ee1ebc05) | 翻訳例で説明 |
| 2026-05-31 (6a0a262f, 0c769a9b) | follow-up #32 スコープ確定 (要件発散) |
| 2026-06-01 (6a0a262f, U6ab8caf2) | **本質汎化訂正**: 翻訳特化思考解除、タスクランナー cache 差分文脈 |
| 2026-06-01 (6a0a262f, U736e5751) | kawaz mini-DSL 起案 + 自己評価「自分でも驚くほどなんかこの mini-DSL いけてそうでびっくり」 |
| 2026-06-02 | 要件発散 doc に 10.9 検討漏れ candidates A-O / 10.10 UX 観点を追記 |
| 2026-06-02 | マルチペルソナ adversarial レビュー起動 (agent `ad8476c30f590f0b5`、12 候補案 × 6 ペルソナ) |
| 2026-06-02 | codex adversarial review 起動 (job `task-mpvfchbn-m6f9rj`) |
| 2026-06-02 | kawaz × Claude 議論 — `()` capture 案 / `regex:` 検討の却下理由を詰める |
| 2026-06-02 | **DR-0027 起票 + 要件発散 issue delete (本 commit)** |

## 採用案サマリー

- **mapping DSL**: kawaz 起案 mini-DSL (= `glob:` 拡張、可変パーツが順に backref 登録、`{}` 必須 / `*`/`**`/`[]` 任意 + `--` ペア区切り + 自動除外)
- **subcommand**: `vcs outdated` (= make/`outdated` 慣習、日本人にも馴染みやすい、`sync-check`/`check-derived` 候補を凌駕)
- **必須 UX**: `--explain` mode (= `$N` 位置 backref 順序 ambiguity の運用吸収、empirical-verification 哲学整合)
- **鮮度比較**: ts ベース (= 現行翻訳 check と同方式継承)
- **派生不在**: mini-DSL 構文で自動決定 (= `{}` 必須 / `*`/`**`/`[]` 任意)

## 主要却下案 + 理由

### `regex:` プレフィックス (= 本 DR 補強根拠の主要却下対象)

kawaz: 「regex は当然マッピングを考えた時に最初に思いつくんだけど、glob のファイルシステム上の発見有無との相性が悪い。特に `**` を regex と fs 解決に落とし込みにくい。」

詰めた論点:
- regex `**` 相当の `.*` は文字列 semantics、glob `**` は fs 上の path separator 跨ぎ + 実存再帰探索 = 意味階層が違う
- regex で fs マッチを書く 2 経路 (= 全 file enumerate→regex filter / regex に fs 再帰意味論) は両方不健全
- 採用案 (a) の 3 つの驚きポイント — **fs 実存マッチング** / **{}必須 vs */**/[] 任意の二分** / **発見 or 新規の自然カバー** — **全て regex 移行で消える**
- 「ぱっと見の印象が regex」は機構の話ではなく見た目、本質利点を捨てる取引は劣化

### `()` capture (= zsh `zmv` 系譜の明示 capture)

却下理由: glob の filename-representability (= `(foo)/(bar)-ja.md` のような括弧含む path) を犠牲、escape 強制で DR-0024 §10.7 と矛盾。

その他棄却 (lambda script / `--map` regex / named group / anchored label / shell expansion 風) は DR-0027 不採用案 section 参照。

## prior art 整理 (= kawaz 「MS-DOS の `mv *.txt *.bak`」 発想元)

ワイルドカード後方参照は新発明ではなく、確立された 2 系統:
- **系統 1**: 位置 backref (MS-DOS COMMAND.COM `copy *.txt *.bak` / Unix `mmv '*.txt' '#1.bak'`) = 採用案 (a) と同系譜
- **系統 2**: 明示 capture + backref (zsh `zmv '(*).txt' '$1.bak'`) = 却下案 `()` capture と同系譜

採用案 (a) は系統 1 (MS-DOS / mmv 系譜) を **glob: portable subset + fs 実存マッチング + {}必須 vs * 任意の二分** という現代的拡張として組み合わせたもの。kawaz の感覚「MS-DOS の `*.txt *.bak` みたいなやつ、発想の元としては原始の記憶」(2026-06-02) と整合。

## 「思いつき一発で最善そう」自己警戒の結末

kawaz: 「なんとなくでパターン例作って考察してみたけど思いつきが一発ですんなり最善そうってパターンが不安になるだけで悪くないよねやっぱ？」(2026-06-02)

→ マルチペルソナ adversarial レビュー (12 候補案 × 6 ペルソナ) + codex adversarial review + kawaz × Claude 議論を経て、元案 (a) を超える対抗案なしと確認。`regex:` も含めて却下理由を明文化、本 DR で補強完了。

## 次のアクション (= Phase 2 実装)

1. DR-0027 影響範囲に従い `vcs outdated` 実装
2. `glob:` の backref 拡張 (= マッチした可変パーツを `[]string` で保持)
3. `--explain` mode 実装 (= 展開後派生 path + 鮮度判定の構造化表示)
4. 既存翻訳 check (= `pkf-tasks/tasks/docs/translations.pkl`) を `vcs outdated` で代替可能か検証 → 可能なら Taskfile.pkl shim も削減 (= DR-0022 の方向性に整合)
5. README に Examples + shell escape 注意 + `--explain` 用法

## 関連

- DR-0027: 採用判断の正本
- DR-0024: `glob:` prefix 基盤
- DR-0020 PR-x: `vcs` ファミリ
- マルチペルソナレビュー: agent `ad8476c30f590f0b5` (= レポートをこの journal に転記検討)
- codex adversarial review: job `task-mpvfchbn-m6f9rj` (= 結果を kawaz 確認後この journal に転記検討)
