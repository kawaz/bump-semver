---
title: vcs get default-branch-path 追加
status: resolved
category: request
created: 2026-06-27T01:36:18+09:00
last_read:
open_entered: 2026-06-27T01:36:18+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered: 2026-06-29T04:59:47+09:00
discard_reason:
pending_reason:
close_reason: ["implemented"]
blocked_by:
origin: 自リポ TODO
---

# vcs get default-branch-path 追加

## 概要

`vcs get default-branch` で default branch 名 (main / master / trunk 等) は取れるが、
その branch を持つ workspace の絶対 path を取る経路がない。
`vcs get default-branch-path` を追加して workspace の絶対 path を返す。

## 背景

機械操作の主用途:
- secondary workspace から release flow を自動化 (= main ws に cd して sync / promote / push)
- justfile の push 系経路で default branch を持つ ws はどこ? を解決

## 受け入れ条件

- [ ] `vcs get default-branch-path` が default branch を持つ workspace の絶対 path を stdout に出す
- [ ] exit 0 = 取れた、exit 4 = 該当 ws なし、exit 5 = ambiguous
- [ ] 複数 ws 該当時の優先: WS 名と default branch 名が完全一致するものがあればそれを返す、無ければ ambiguous fail
- [ ] jj: `jj workspace list` + bookmark 解決経路で実装
- [ ] git: `git worktree list --porcelain` + branch 解決経路で実装

## TODO

<!-- wip 時のみ -->

- [ ] 実装設計・仕様確定
- [ ] jj / git 各経路実装
- [ ] テスト追加
- [ ] CHANGELOG 更新

## 関連

- DR-0038
- CHANGELOG v0.40.0 で `vcs get default-branch` 追加済
