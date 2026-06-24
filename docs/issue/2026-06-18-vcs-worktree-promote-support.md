---
title: vcs に worktree/workspace 検出 + default branch への promote サブコマンドを追加
status: open
category: request
created: 2026-06-18T11:29:21+09:00
last_read:
open_entered: 2026-06-18T11:29:21+09:00
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

# vcs に worktree/workspace 検出 + default branch への promote サブコマンドを追加

## 背景

claude-plugin-reference v0.2.17 でバージョンマーカーをロックファイル化する作業中、Claude Code の `EnterWorktree` (= background job では必須化される) 経由で worktree 内編集 → main 統合 → push という手順を踏んだ。

その過程で、**worktree → main 反映の手順が AI セッション依存で毎回再構築されている** ことが顕在化した。具体的な詰まり所:

1. `worktree.baseRef = fresh` (= 既定) で worktree が origin/HEAD ベースに作られるため、親 workspace で進めた最近 commit (= まだ main bookmark 未進行) が worktree から見えず、後で rebase が必要になる
2. worktree から main bookmark を進める手順が AI セッション側の jj 知識に依存 (= `jj bookmark set main -r <change>` の判断)
3. divergent な状態 (= 共通祖先から 2 line 分岐) からの統合に `jj rebase -s <change> -d main` が必要だが、これも AI 判断
4. justfile の `push` task 冒頭にゲートを置きたいが、bump-semver vcs に「現在 worktree か」「default branch か」を判定する手段が無い

詳細な実践記録 (一次資料):
https://github.com/kawaz/claude-plugin-reference/blob/main/docs/journal/2026-06-18-worktree-promote-and-marker-lockfile.md

## 既存 vcs サブコマンドの拡張提案

`vcs` には既に `get / is / diff / commit / fetch / push / tag / outdated` が揃っていて、`push --jj-bookmark-auto-advance` で bookmark 自動進行も実装済み。**worktree/promote 概念だけが欠けている**。

| 追加サブコマンド | signature | 用途 | 既存パターンとの関係 |
|---|---|---|---|
| `vcs is worktree` | `→ bool (exit 0/1)` | 現在 worktree/workspace 内か | `vcs is clean/dirty/git/jj` パターン |
| `vcs get worktree-name` | `→ string` | hint メッセージ用。default なら空 | `vcs get current-branch` パターン |
| `vcs get default-branch` | `→ string` | main/master/trunk 抽象化 | 新規。jj: `trunk()` / git: `git symbolic-ref refs/remotes/origin/HEAD` |
| `vcs is on-default-branch` | `→ bool` | 現 branch/bookmark が default か | 既存 `get current-branch` の比較を 1 コマンド化 |
| `vcs promote` | (副作用あり) | 現 change を default branch に合流 | `push --jj-bookmark-auto-advance` を push と切り離した版。jj: `jj bookmark set <default> -r @-`、git: `git checkout <default> && git merge --ff-only <branch>` |
| `vcs sync --onto <ref>` | (副作用あり) | worktree のベースを更新 | 新規。jj: `jj rebase -d <ref>`、git: `git rebase <ref>` |

## ユースケース: `just push` 冒頭ゲート

各 kawaz リポの justfile 標準テンプレに以下のゲートを入れることで、worktree から push しようとした AI を hint で誘導できる:

```just
push:
    @if bump-semver vcs is worktree; then \
        wt=$(bump-semver vcs get worktree-name); \
        bn=$(bump-semver vcs get default-branch); \
        echo "⚠ worktree '$wt' にいます。${bn} に合流が必要です。"; \
        echo ""; \
        echo "  1. ベースを最新に同期:   just sync"; \
        echo "  2. ${bn} に合流:          just promote"; \
        echo "  3. push:                  just push"; \
        exit 1; \
    fi
    # 既存の検証ゲート + vcs push
```

このゲート (+ `just promote` / `just sync` task) は docs-structure runbook の標準テンプレに入れる予定 (関連 issue: claude-rules-personal docs/issue/2026-06-18-worktree-workflow-runbook-template.md)。

## 該当性の確認

- 既存 `vcs is clean/dirty/git/jj` / `vcs get current-branch / commit-id` パターンに乗る命名で、本リポの抽象化設計と整合する
- `--jj-bookmark-auto-advance` flag が既に bookmark 移動を扱っているので、`promote` (= bookmark 移動だけ切り出し) は同じ層の追加
- jj/git 両 backend での実装余地は確認済 (= journal の解決方向セクション)

## スコープ

含む:
- 上記 6 サブコマンドの jj/git 両 backend 実装
- ヘルプ + 補完
- runbook (docs-structure 側で起票) からの参照リンク

含まない (= 別判断 / 別 issue):
- `EnterWorktree` 側の `worktree.baseRef = head` 設定変更可否 (= Claude Code 本体 / harness 側の話)
- jj-worktree plugin への bookmark 自動セット提案 (= jj-worktree plugin 側で別途検討、journal 参照)
- 既存 kawaz リポ群への一斉 migration (= 段階的に各リポで判断)

## 実装方針は当事者判断に委ねる

具体的なサブコマンド名 / option 形式 / backend 内部実装は本リポの設計思想に従う。当該 issue は **必要性のフラグ + 一次資料 (= 実践 journal) の提示** であり、実装の細部は委ねる。
