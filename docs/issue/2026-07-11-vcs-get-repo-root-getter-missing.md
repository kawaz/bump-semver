---
title: vcs get に「本体リポの root/名前」を返す getter が無く、git linked worktree で repo 名が worktree 名に化ける
status: open
category: design
created: 2026-07-11T11:59:12+09:00
last_read:
open_entered: 2026-07-11T11:59:12+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: claude-ccmsg
---

# vcs get に「本体リポの root/名前」を返す getter が無く、git linked worktree で repo 名が worktree 名に化ける

## 概要

`vcs get root` は jj workspace / git linked worktree のいずれでも「自分自身の root」を返すが、
「本体リポの root/名前」に遡る getter が無い。jj 側は利用側で `dirname(root)` すれば遡れるが、
git の linked worktree では遡る手段が無く、worktree 名が誤って repo 名として扱われる。

## 背景

claude-ccmsg の hook が `bump-semver vcs get root/backend/worktree-name/current-branch` で
repo 名 / workspace 名を導出する際に判明 (2026-07-11、実測)。

事実:

1. **jj**: named workspace では `root` が workspace 自身の root を返す。`dirname(root)` で
   本体リポの root (= 複数 workspace の親) に遡れる。
2. **git**: linked worktree (実測例: `mermaid-aa-pr1`) では `root` が worktree 自身の root を
   返し、そこから本体リポへ遡る getter が無いため、`basename(root)` = worktree 名が
   誤って repo 名として扱われてしまう。

利用側 (claude-ccmsg) のワークアラウンド: `hooks/session-start.ts` の `deriveRepoWs` が
`backend=jj` のときだけ `dirname(root)` する分岐を持ち、git linked worktree は既知の制約として
コメント化してある。

## 提案 (フラグ止まり、採否は当事者判断)

`vcs get repo-root` (worktree/workspace から本体リポの root に遡る) や `vcs get repo-name`
のような getter があると、利用側が backend 別の分岐 (jj なら `dirname` 等) を自前で書かずに済む。

git 側の一次情報候補: `git rev-parse --git-common-dir` から本体側の `.git` ディレクトリに
遡れるはず (未検証、裏取りしてから採否を決めてほしい)。

## 受け入れ条件

- [ ] `git rev-parse --git-common-dir` (または同等の一次情報) で本体リポ root へ遡れるか裏取りする
- [ ] jj 側の `dirname(root)` 遡り方式と揃えられるか検討する
- [ ] 採用するなら `vcs get repo-root` / `vcs get repo-name` 等の getter を実装、見送るなら理由を記録する

## 注記

- 本 issue は claude-ccmsg 側セッションからの部外者観測に基づく起票。具体的な実装方針は
  当事者 (bump-semver) 側の判断に委ねる
- 利用側ワークアラウンドの詳細: claude-ccmsg リポ `hooks/session-start.ts` の `deriveRepoWs`
