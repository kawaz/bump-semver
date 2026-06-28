---
title: promote ガード追加 (祖先に default branch が居ない時)
status: open
category: request
created: 2026-06-27T01:36:22+09:00
last_read: 2026-06-29T05:01:39+09:00
open_entered: 2026-06-27T01:36:22+09:00
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

# promote ガード追加 (祖先に default branch が居ない時)

## 概要

`vcs promote` 実行前に default branch が現在の commit の祖先にいるかを確認し、居なければ fail してヒントを出す。

## 背景

bump-semver vcs promote は現在の commit に default branch (main / master / trunk) bookmark を forward する。
secondary workspace から呼ぶと、default branch を別 workspace の commit に動かすことになり、
default branch を持つ主 workspace との乖離が生まれる (= 主 ws 側で意図せず ref が変わる)。

kawaz の運用前提:
- 主 ws (= default branch workspace) は常に開けっぱで居れる前提
- secondary ws での promote は **default branch が祖先にいるとき限定** で許可するのが筋
- 祖先にいない時 = 別系統で作業中 = promote すべきでない

## 提案

vcs promote の実行前に「default branch (= 該当 ref) が現在の commit の祖先にいるか」を確認し、
居なければ fail で「先に sync してから promote してください」hint を出す。

- 祖先判定: git は merge-base、jj は revset の non-empty
- 例外パス: --force flag で明示 override

## 受け入れ条件

- [ ] default branch が祖先にいない場合、`vcs promote` が fail してヒントメッセージを出す
- [ ] `--force` flag で override できる
- [ ] git / jj 両方で動作する

## 関連

- DR-0038 (vcs-worktree-promote-support)
