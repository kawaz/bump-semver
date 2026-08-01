---
title: vcs commit の --allow-nonexistent-path ヒントに従うと削除 path が黙って捨てられる
status: resolved
category: bug
created: 2026-07-31T23:16:26+09:00
last_read: 2026-08-01T16:10:12+09:00
open_entered: 2026-07-31T23:16:26+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered: 2026-08-01T21:15:04+09:00
discard_reason:
pending_reason:
close_reason: ["ANP-Q1 a 裁定: --allow-nonexistent-path のヒント文言 (jj backend) と help/flag description に「tracked-but-deleted path も drop 対象に含まれる」旨を明示。実装挙動は据え置き。デフォルト (フラグ無し) は削除を正しく commit するため利用側の 3 path 指定は変更不要。関連 commit は本 close と同一チェンジ (src/vcs_backend_jj.go, src/cobra_help_text.go, src/cobra_vcs.go)。"]
blocked_by:
origin: 依頼元プロジェクト (claude-local-issue からの越境起票)
---

# vcs commit の --allow-nonexistent-path ヒントに従うと削除 path が黙って捨てられる

## 概要

`bump-semver vcs commit` に渡した path が 1 つでも解決できないと、エラーメッセージが
`(use --allow-nonexistent-path to silently drop)` というヒントを出す。このヒントに
従うと **存在しない path を黙って捨てる**ため、「移動元ファイルの削除を commit に
含める」という正当な用途が壊れる。

## 再現の文脈 (利用側)

claude-local-issue の close 処理は、issue ファイルを `docs/issue/<f>.md` から
`docs/issue/archive/<f>.md` へ `mv` した後、

```
bump-semver vcs commit -m "..." docs/issue/<f>.md docs/issue/archive/<f>.md docs/issue/INDEX.md
```

の 3 path を指定して、削除と追加を同一 commit に入れる。このとき旧 path は
filesystem 上に既に存在しないため、まさに「存在しない path」に該当する。

## 実機確認 (bump-semver v0.48.1 / jj 0.43.0)

- 3 path 指定は git (R100) / jj (R) の両方で正しく rename commit になり、残留ゼロ。
  ツールの基本動作は正しい。
- `--allow-nonexistent-path` を付けると、正しい 3 path を渡していても削除 path が
  黙って捨てられ、`D docs/issue/<f>.md` が working copy に残留する (再現済み)。

## 部外者としての所感 (実装判断は当事者に委ねる)

- ヒント文言が「エラーを消す方法」として提示されるため、削除 path を含む正当な
  ケースでも安易に採用されやすいのではないか。
- 「意図的に削除を commit したい path」と「タイポ等で実在しない path」を、現状の
  フラグ 1 つで区別できているかは確認が要る。
- 該当性の裏取りと、そもそも対処すべき問題かの判断は bump-semver 側でお願いします。
  具体的な実装案は出しません。category (bug か request か) の判定も当事者側で。
- なお、このフラグが実際の失敗事例で使われた直接証拠はなく、状況証拠にとどまる。

## 関連

- claude-local-issue の
  `docs/issue/2026-07-29-close-archive-move-drops-file-deletion-from-commit.md` に、
  利用側の観測と対策 (commit 後に `bump-semver vcs is clean` で検証、
  `--allow-nonexistent-path` を使わない invariant を明文化) を記録済み。

## 受け入れ条件

- [ ] `--allow-nonexistent-path` 指定時に削除 path が drop される挙動を自リポで再現確認する
- [ ] ヒント文言が削除 path のケースで誤誘導になっていないか判定する
- [ ] 対処する場合は方針 (文言変更 / フラグの semantics 見直し / 何もしない) を決めて反映し、
      対処しない場合はその理由を記録して close する
