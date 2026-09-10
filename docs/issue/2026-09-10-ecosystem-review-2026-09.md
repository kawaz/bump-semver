---
title: エコシステム外部レビュー(2026-09)の指摘への対応検討
status: open
category: task
created: 2026-09-10T14:53:00+09:00
last_read:
open_entered: 2026-09-10T14:53:00+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: kawaz依頼(2026-09-10、claude-rules-personalセッション経由)
---

# エコシステム外部レビュー(2026-09)の指摘への対応検討

## 概要

外部レビューで本リポ (bump-semver) 向けの指摘が出た。個別ファイル
`claude-rules-personal` の `docs/research/2026-09-10-ecosystem-review/bump-semver.md`、
共通ファイル同 `common.md` (P-20〜P-24 が bump-semver 発の横断パターン) を参照し、
対応を検討する。

温度感: レビューは初版の指摘から個別プロジェクトの精読を進めるたびに認識が改まり、
指摘が覆されたケースが多い。全面的に鵜呑みにせず実物と照合してから採否を決めること。

## 背景

単体評価 (9/9、depth 350)。350 コミット、DR 43 本、テスト関数 861、Go 約 40k 行
(`src/` 単一 `package main` 110 ファイル)。指摘は 5 件 (B-1〜B-5)、優先度
★3 (次の作業で) / ★2 (近いうちに) / ★1 (気づいた時に)。

## 各項目の指摘と実物照合の結果

### B-1 (削除・却下不要): 既知バグは解決済み — 確認済み・対応不要

指摘: 「`vcs commit` が存在しないパスを黙って落とす」。

照合: `docs/decisions/DR-0037-vcs-commit-default-include-deletes.md` が存在し、
デフォルト反転で解決済み。レビュー側も「取り下げ」と明記。**現状維持、対応不要。**

### B-2 ★1 名前と実態の乖離 — 裁定待ち (kawaz 判断要)

指摘: README 1 行目「semver 文字列を取得・bump・比較するための、絞り込まれた CLI」が
実態 (`vcs` / `glob` / `outdated` / worktree / repository slug を含む 43 DR) の
一部しか説明していない。

照合: 実機確認済み。README.md / README-ja.md の 1 行目は現在もレビュー指摘どおりの
文言のまま (未修正)。

裁定候補 (レビュー側の裁定は (b) だが、実行前に kawaz 確認):

- (a) 改名 (crates.io/npm/PyPI/GitHub/homebrew 名の衝突確認とセットで別途検討)
- (b) 改名せず README 1 行目を実態に合わせて書き換え、名前は歴史として受け入れる

**裁定待ち。kawaz の判断が要る。**

### B-3 ★2 justfile canonical の記述が 3 箇所に分散 — 妥当、対応推奨

指摘: canonical 記述が (1) 本リポ `docs/runbooks/justfile-pattern-audit.md`、
(2) rules-personal `reference/justfile/recipes.md`、(3) 各リポ justfile 冒頭コメント、
の 3 箇所に分かれ、(1) が「`docs-structure` rule で定められている」と書くが
その rule は移設済みで dead reference。

照合: 実機確認済み。`docs/runbooks/justfile-pattern-audit.md` 冒頭に
「`docs-structure` rule で justfile の canonical owner と定められている」の記述が
現存 (dead reference のまま)。手順本体 (`ls -lt` で mtime 順、diff、対応表) は
他リポ監査手順であり本リポの runbook というより rules-personal 側の
fleet-audit 手順に近い、という指摘も妥当。

**対応方針を採用**: dead reference を除去し、「canonical justfile は本リポの
`justfile`。監査手順は rules-personal の fleet-audit」の 1 行に整理する。
rules-personal 側 (`docs/runbooks/fleet-audit.md` への移設) は越境作業のため
claude-rules-personal 側の issue と対で進める。★2、次に本リポを触る時に着手可。

### B-4 ★1 他リポ運用に直結する open issue 2 本 — 妥当、既存 issue として追跡中

指摘: `2026-07-28-check-on-default-branch-gate-false-no-hint.md` と
`2026-07-29-justfile-on-success-release-list-help-broken.md` が全リポの
push/watch recipe に影響し、8/1 から止まっている。

照合: 実機確認済み。両 issue とも `docs/issue/` に現存し status: open のまま
(INDEX.md にも掲載済み)。レビューの指摘は正確。

**採用 (新規起票は不要、既存 issue の消化を優先度付けとして確認)**: 次に本リポを
触る時にこの 2 本をまとめて着手する。hint の件は rules-personal 側
`check-on-default-branch` が自前 printf hint を持つ実例があり、本リポ側で
出せば各リポの `[script]` recipe が消える、という指摘も参考にする。

### B-5 (現状維持、B-2 に従属)

指摘: DR-0036 で `internal/` 分割を見送り、再検討トリガを「`vcs` 系分離の需要」
「ビルド/テスト時間」と定めている。B-2 で改名や `vcs` 分離を選ぶならトリガに当たる。

照合: `docs/decisions/DR-0036-package-split-deferred.md` 現存、src は実際に
`package main` 110 ファイル (実機カウント済み)。

**B-2 の裁定に従属**: (a) なら DR-0036 の再検討を同時に、(b) なら現状維持。
B-2 が決まるまで単独では動かさない。

## 横断パターン (common.md より、本リポ発の P-20〜P-24)

本リポの設計判断がバックポート候補として common.md に載っている
(P-20: 述語コマンドの exit code 規約、P-21: 定型レイヤは opinionated でよい、
P-22: `glob:` prefix での shell 展開吸収、P-23: 外部コマンド呼び出しの信頼境界
(DR-0034)、P-24: all-or-nothing + atomic write (DR-0035))。これらは
rules-personal 側の reference へのバックポートが対象であり、本リポ側の
追加対応は不要 (現状の実装がそのまま採用元)。

## 受け入れ条件

- [x] bump-semver.md / common.md を読み、各指摘を実物と照合した
- [x] B-1: 解決済みと確認、対応不要
- [ ] B-2: kawaz の裁定を得る (改名 or README 修正)
- [ ] B-3: runbook の dead reference 除去 + canonical 記述の 2 層整理
- [ ] B-4: 既存の 2 issue (2026-07-28 / 2026-07-29) を消化する
- [x] B-5: B-2 裁定待ちとして保留 (単独では動かさない)

対応タイミングは担当セッションまたは kawaz に任せる。
