---
title: just watch recipe が watch-workflow.sh を PATH 前提で呼んでいる
status: open
category: bug
created: 2026-09-19T18:36:31+09:00
last_read:
open_entered: 2026-09-19T18:36:31+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: sandbox-jev
---

# just watch recipe が watch-workflow.sh を PATH 前提で呼んでいる

## 概要

`just watch` recipe が `watch-workflow.sh` を PATH 前提で呼んでいるため、PATH に無い環境 (Claude Code の Monitor tool から起動した shell) では `command not found` で exit 127 になる (2026-09-19 実測、sandbox-jev セッション)。

## 背景

gh-monitor plugin の実体 `~/.claude-personal/plugins/cache/gh-monitor/gh-monitor/<ver>/scripts/watch-workflow.sh` をフルパスで呼べば動いた。

提案 (フラグ止まり、裏取りしてから採否を決めてほしい): recipe 内で `command -v watch-workflow.sh` が無ければ plugin cache の最新版を探すフォールバックを持つ、または push 後の hint と同様に絶対パスを出す。

## 受け入れ条件

- [ ] `just watch` が PATH に `watch-workflow.sh` が無い環境でも動くか、動かない場合は分かりやすいエラー・代替手段を提示する

## TODO

<!-- wip 時のみ -->
