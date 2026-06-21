---
title: vcs commit PATH.. で delete も commit に含めるオプション
status: discarded
category: request
created: 2026-06-20T00:16:45+09:00
last_read:
open_entered:
wip_entered:
blocked_entered:
pending_entered:
discarded_entered: 2026-06-21T12:21:08+09:00
resolved_entered:
discard_reason: ["2026-06-21 issue (vcs-commit-nonexistent-path-silently-drop) に統合。新 issue は同じ事象を扱うが、対応方針として「git 文脈に合わせデフォルト反転、bump 用途固有の silently-drop は明示フラグ側へ追い出す」を確定済み。本 issue は (a)(b)(c) 3 案のフラグ止まりで部外者起票スタンスだったが、ユーザ判断 (kawaz, 2026-06-21) で (b) デフォルト反転を採用したため役目を終えた。"]
pending_reason:
close_reason:
blocked_by:
origin: claude-local-issue plugin 開発時の dogfood で発見 (= 部外者起票、フラグ止まりスタンス)
---

# vcs commit PATH.. で delete も commit に含めるオプション (フラグ起票)

claude-local-issue plugin 開発過程で `bump-semver vcs commit PATH..` の spec **「Nonexistent paths are silently dropped」** が原因で、ファイル移動 (= `mv old new`) を伴う操作の close commit に旧 path の delete が含まれず後続 commit に漏れる事象が発生した。当事者判断に委ねるフラグ起票。

## 観察した事象 (= 一次資料)

claude-local-issue の `/local-issue:update --status=resolved` (close フロー) で:

1. `mv docs/issue/<slug>.md docs/issue/archive/<slug>.md` で archive 移動
2. `bump-semver vcs commit -m "issue(close): <slug> -> archive" docs/issue/<old> docs/issue/archive/<new> docs/issue/INDEX.md` で commit
3. 結果: commit には `M docs/issue/INDEX.md` + `A docs/issue/archive/<new>` のみ、**`D docs/issue/<old>` が含まれない**

3 回再現確認 (claude-local-issue commit `8c873e3`, `61579a0`, `94b3a18`):

```bash
git show --name-status 94b3a18
# M docs/issue/INDEX.md
# A docs/issue/archive/2026-06-19-update-input-format-improvements-from-9cell-trial.md
# (元 path の D が無い)
```

`bump-semver vcs commit --help` の spec:

> PATH..        Commit the working-tree content of the listed paths only.
>               Nonexistent paths are silently dropped. All-nonexistent OR
>               no actual change → exit 0, no commit (idempotent).

= **silently dropped** が起因。

## 影響

- 後続 commit に「unstaged delete」が漏れ込む (= 履歴の責務分離が崩れる)
- 利用側 (= claude-local-issue close フロー) が暫定 workaround として `--staged` モードに切り替えざるを得ない (= 巻き込み防止のため dirty 確認 step を追加で必要、複雑化)

## 検討の余地があるかもしれない案 (= 断定しない、当事者判断)

### (a) `--include-deletes` オプションを追加

```
bump-semver vcs commit --include-deletes -m MSG PATH..
```

= nonexistent path を「delete として stage」する。silently drop ではなく vcs に明示的に delete を伝える。

- 利点: 既存 spec「nonexistent silently dropped」を維持しつつ opt-in で delete を扱える
- 利用側: close フローのような「mv + commit」パターンで自然に書ける

### (b) PATH.. で nonexistent path を自動検出して delete として扱う (デフォルト挙動変更)

= silently drop をやめて、nonexistent は「delete」として stage する。

- 利点: オプション不要、より自然
- 欠点: spec 変更は破壊的、既存利用箇所で「typo の path を意図せず delete として stage する」リスク

### (c) ドキュメントに「mv パターンの正しい使い方 (= --staged)」を明記するだけ (= API 変更なし)

- 利点: コストゼロ
- 欠点: 利用側で毎回 dirty 確認 step が必要、UX 劣化

## 当事者判断に委ねる点

- (a)(b)(c) のいずれを採るか / 別案を考えるか
- そもそも「mv パターンは --staged で扱う」が正規ルートなら、その明示で十分かもしれない
- claude-local-issue 側は **(c) 想定の workaround** (= cp + rm + --staged + dirty 事前確認) を v0.2.3 で実装済み (5ac4dbe..ddd2874 範囲)。bump-semver 側で改善が入れば追従可能

## 起票元の経緯 (補足)

claude-local-issue v0.2.0-v0.2.2 で 3 回の close で同パターン再現。元の close フローは「mv + bump-semver vcs commit old new INDEX.md」だったが、`old` が silently dropped されて delete commit が抜けた。v0.2.3 で「cp + rm + bump-semver vcs commit --staged」に変更して回避。

実際の close commit:
- `8c873e3 issue(close): initial-open-items -> archive`
- `61579a0 issue(close): cmux-msg-dogfood-migration-feedback -> archive`
- `94b3a18 issue(close): update-input-format-improvements-from-9cell-trial -> archive`

これらの D が次の feature commit に紛れ込んでいた。

## 関連

- 起票元 plugin: claude-local-issue v0.2.3
- 起票元 issue (= 同じ事象を local-issue 側で起票): `docs/issue/2026-06-20-update-close-leaves-unstaged-delete.md`
- bump-semver vcs commit --help の spec 抜粋 (本起票時点)
