---
title: vcs get commit-id の jj デフォルト rev が @ (mutable working copy) を指し、git HEAD と概念不一致
status: open
category: bug
created: 2026-07-06T11:19:41+09:00
last_read:
open_entered: 2026-07-06T11:19:41+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: kawaz/claude-gh-monitor
---

# vcs get commit-id の jj デフォルト rev が @ (mutable working copy) を指し、git HEAD と概念不一致

## 概要

`bump-semver vcs get commit-id` のデフォルト rev が help 文言通り「@ for jj / HEAD for git」になっている。これは agnostic API として概念がズレている。

- git `HEAD` = 最後に**固定された**コミット (作業ツリーの未コミット変更を含まない)
- jj `@` = mutable な working-copy コミット (空のことも、未コミット作業を抱えることもある)

git `HEAD` の jj 対応物は `@-` (直前に固定されたコミット) であり `@` ではない。現状は「agnostic API を謳いつつ backend ごとに別概念を返す」設計バグ。help の "@ for jj / HEAD for git" を「両 backend で『最新の固定コミット』を返す」に揃えるべき。

これは gh-monitor の post_tool_use hook (push 後の CI watch 用 SHA pin) を bump-semver vcs に寄せようとした際に露見した。

## 背景

### 実機確認 (bump-semver v0.45.0)

help:
> commit-id … 40-char git commit SHA of --rev (default: @ for jj / HEAD for git)

実測 (jj backend, gh-monitor リポ):
- `default` == `--rev '@'` == working copy `@` の SHA
- `--rev '@-'` == 直前の固定コミット SHA
- @ が空コミットのとき default は空コミット SHA を返す (前セッション journal で `64b7d3d` の実例、実 push 対象は `2c89449`)。この結果 CI run が存在せず watcher が no-match-timeout まで無駄常駐する退行になる。

### 副次観測 (別レイヤ、切り分けて記載)

bump-semver は git backend 経由で .git の export 済みビューを読むため、jj の自動 snapshot に対して git export がラグり、返す SHA が jj の実 @ と一瞬ズレる現象を観測。これは default-rev セマンティクスとは別レイヤ (git-export staleness) だが、default を mutable な @ にしていることでこのラグの実害を受けやすい。default を固定コミットにすれば実害が消える。

### 修正の方向性 (実装判断は当事者に委ねる、両案併記)

デフォルト rev を「最新の固定コミット」に変え、git/jj で同じ概念を返すようにする。実装候補:

**第一推奨**: `latest(::@- & ~empty())` 相当 (@ を除外した最新の固定・非空コミット)
- 空コミット連鎖 / merge / 未コミット @ の 3 罠を全部回避
- git HEAD と概念一致
- push 対象 pin の意図に厳密 (@ が非空でも固定済みの親を返す)
- 注: journal が使っていた `latest(::@ & ~empty())` は @ 非空時に @ 自身 (未コミット working copy) を返してしまうため、`@-` 起点にするのが正しい

**対案**: 素直な `@-`
- シンプル・意図明快
- ただし @- 自体が空コミット / merge のとき別の退行余地 (要追加ガード)

### 影響 / 後方互換

- 影響範囲は kawaz プロジェクト群のみ
- 旧挙動 (@ を取る) が必要なら `--rev @` を明示指定すれば残せる → 後方互換の逃げ道あり
- git backend での revset 挙動 (jj 専用 revset が git で通らない件) は要検証

### 一次資料 (裏取りしてから採否を決めること)

- gh-monitor: `docs/journal/2026-07-06-workflow-absence-check-jj-workspace-handoff.md` (空 @ 実機ケース、「SHA 解決の落とし穴」セクション)
- bump-semver v0.45.0 の `vcs get --help` commit-id 節

## 受け入れ条件

- [ ] jj backend で `vcs get commit-id` のデフォルトが @ ではなく最新の固定コミットを返すことを確認
- [ ] git backend との概念一致 (HEAD 相当) を確認
- [ ] 旧挙動 (@ 取得) を明示指定する経路 (`--rev @`) が残っていることを確認
