---
title: vcs commit が削除された path を黙殺する (nonexistent-path silently drop の考慮漏れ)
status: open
category: bug
created: 2026-06-21T12:15:49+09:00
last_read:
open_entered: 2026-06-21T12:15:49+09:00
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

# vcs commit が削除された path を黙殺する (nonexistent-path silently drop の考慮漏れ)

## 概要

`vcs commit -m "msg" <files...>` でパス指定する際、そのパスが削除済み（git で言う "deleted" 状態）の場合、コマンドが黙殺してそのパスを commit に含めないことがある。

## 背景

`push-workflow.md` ルール等で「自分が修正したファイルだけをパス指定して固定する」運用を推奨している。削除操作を含む change を commit しようとした際に、削除済みのパスを指定しても commit に含まれずサイレントに無視される挙動が発生する可能性がある。

これにより以下の問題が起きる:

- 削除したつもりのファイルが commit に含まれず、次の commit や push で思わぬ状態になる
- エラーが出ないため問題に気づけない
- パス指定を信頼する運用のもとでは、特に見落としやすい

関連 issue: [vcs-commit-path-include-deletes](./2026-06-20-vcs-commit-path-include-deletes.md) (削除を含めるオプション追加の要望)

## 受け入れ条件

- [ ] 存在しないパス（削除済みを含む）を `vcs commit <path>` に指定したとき、警告またはエラーを出す
- [ ] または、削除済みパスも commit に含める動作に変更する（`vcs-commit-path-include-deletes` issue と連携）
- [ ] いずれの場合も黙殺（silent drop）しない

## TODO

<!-- wip 時のみ -->

- [ ] 実際の挙動を実機で確認（削除済みパス指定時に何が起きるか）
- [ ] git / jj それぞれの経路での挙動差を確認
- [ ] 修正方針を決定（警告出す or 削除も含める）
