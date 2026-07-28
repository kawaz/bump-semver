---
title: justfile の on-success-release の --list 説明が崩れている
status: open
category: bug
created: 2026-07-29T04:14:38+09:00
last_read:
open_entered: 2026-07-29T04:14:38+09:00
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

# justfile の on-success-release の --list 説明が崩れている

## 概要

`just --list` の `on-success-release` 行の説明文が、文の途中から始まり
末尾に不要な閉じ括弧が残る形で表示されている。

```
on-success-release         # 通知 event に `[ACTION:release.yml] just on-success-release` が emit される)
```

## 背景

justfile では `on-success-release:` の直前に 3 行のコメントがある:

```just
# release.yml workflow が success になった時に AI が実行する action
# (watch-workflow の `--on-success release.yml 'just on-success-release'` 経由で
# 通知 event に `[ACTION:release.yml] just on-success-release` が emit される)
on-success-release:
```

`just --list` は recipe 直前の**最後の 1 行のコメントのみ**を doc として
表示する仕様のため、複数行コメントの 2 行目以降で括弧を開いて改行した
文章が、閉じ括弧だけ残った状態で 1 行として表示されてしまっている。

## 受け入れ条件

- [ ] `just --list` の `on-success-release` 行が、それ単体で意味の通る
      説明文になっている (文の途中から始まらない、不要な括弧が残らない)
- [ ] 同様の複数行コメント + `--list` 表示崩れが他の recipe に無いか確認
