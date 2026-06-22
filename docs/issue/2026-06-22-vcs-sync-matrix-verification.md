---
title: vcs sync の動作マトリクス検証 (= 既に sync 済 / divergent / conflict 各ケース)
status: open
category: task
created: 2026-06-22T22:08:18+09:00
last_read:
open_entered: 2026-06-22T22:08:18+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: 自リポ TODO
---

# vcs sync の動作マトリクス検証 (= 既に sync 済 / divergent / conflict 各ケース)

## 概要

`vcs sync --onto REF` の境界ケース挙動を jj/git 両 backend で実機マトリクス検証し、未検証ケースを埋める。

## 背景

DR-0038 で land した `vcs sync --onto REF` の動作確認は v0.40.0 のテスト (= setup → 実行 → rebase 確認) で行ったが、empirical-verification rule のマトリクス検証観点で **境界ケース**の挙動が未確認:

| 入力状態 | 期待挙動 | 検証状況 |
|---|---|---|
| 通常 divergent (= rebase が forward 移動を起こす) | rebase 実行、@ が onto の子に | ✓ (v0.40.0 test) |
| 既に sync 済 (= @ が onto と同じ commit、または onto の祖先) | no-op で exit 0、または "already up to date" | **未検証** |
| conflict 発生 | exit 3 + 適切な error message | **未検証** |
| onto が存在しない ref | exit 3 | **未検証** |
| onto が backward (= 巻き戻し方向) | jj: `jj rebase` がどう振る舞うか / git: "already up to date" or error | **未検証** |

## 該当性の確認

- 本 issue は dogfood で踏んだ罠ではない (= 4 リポで sync を実利用してない、検証不足の指摘)
- empirical-verification rule の「最低 3 種類の category」を満たすため、上記 5 ケースで実機マトリクス検証を行う
- 必要なら test を vcs_worktree_test.go に追加 (= 既存 TestVcs_Sync_Git / TestVcs_Sync_Jj の隣)

## スコープ

- 含む: 上記 5 ケースの jj/git 両 backend での実機検証 + 結果記録
- 含む: 必要に応じて test 追加 + 挙動修正
- 含まない: `vcs sync` の semantics 変更 (= 別判断、設計に踏み込む場合は別 issue)

## 一次資料

- docs/findings/2026-06-22-dr-0038-dogfood-pitfalls.md (= sync は罠リストに入ってない、ここに追記 or 別 findings)
- empirical-verification rule (= claude-rules-personal for-all/rules/)

## 受け入れ条件

- [ ] 5 ケース × jj/git 両 backend の実機マトリクスが埋まっている
- [ ] 結果が docs/findings/ に記録されている
- [ ] 挙動に問題があった場合は test 追加 + 修正が完了している

## TODO

<!-- wip 時のみ -->

- [ ] 実機マトリクス検証 (jj backend × 5 ケース)
- [ ] 実機マトリクス検証 (git backend × 5 ケース)
- [ ] 結果を docs/findings/ に記録
- [ ] 問題ケースがあれば test 追加 + 修正
