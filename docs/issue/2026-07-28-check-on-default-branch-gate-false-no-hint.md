---
title: check-on-default-branch の gate false 時に hint が出ない
status: open
category: bug
created: 2026-07-28T22:13:43+09:00
last_read:
open_entered: 2026-07-28T22:13:43+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: 依頼元プロジェクト (claude-rules-personal からの越境起票)
---

# check-on-default-branch の gate false 時に hint が出ない

## 概要

canonical (bump-semver 自身) の justfile にある `check-on-default-branch` recipe は
`bump-semver vcs is on-default-branch` の 1 行のみで構成されている。`vcs is` は
predicate-false 時に stderr へ何も出さない仕様 (compare と同じ semantics、
src/vcs_cmd.go:308 付近) のため、worktree から `just push` を実行すると recipe 名と
exit code しか出ず、次に何をすればよいか (sync → promote → push 等) が分からない。

## 背景

一方、DR-0038 の Adoption pattern、および利用側リポ (claude-rules-personal の
justfile、同リポ docs-structure skill の `templates/runbooks/worktree-workflow.template.md`)
では printf >&2 で sync→promote→push の hint を出す形が採用されている。

canonical の justfile がこの Adoption pattern に追従していない乖離があるように見える
(部外者からの観測)。

裏取り先 (一次資料):
- 自リポ justfile の `check-on-default-branch` recipe
- `docs/decisions/DR-0038` の Adoption pattern 節
- claude-rules-personal の `worktree-workflow.template.md`

本 issue は部外者観測のフラグ提起にとどまる。該当性の裏取りと、hint の文言・持たせ方
(justfile 側で printf するか / vcs is 側で何か返すか等) の実装判断は当事者側に委ねる。

## 受け入れ条件

- [ ] 上記一次資料を確認し、DR-0038 Adoption pattern からの乖離が実際に存在するか判定する
- [ ] 乖離が存在する場合、canonical justfile の hint 出力方針を決定し反映する
- [ ] 乖離が無い/意図的な場合、その理由を記録して close する
