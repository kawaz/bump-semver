---
title: vcs get current-branch ambiguous の subshell 罠を library 側で吸収できないか
status: open
category: request
created: 2026-06-22T22:06:01+09:00
last_read:
open_entered: 2026-06-22T22:06:01+09:00
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

# vcs get current-branch ambiguous の subshell 罠を library 側で吸収できないか

## 概要

`vcs get current-branch` が ambiguous 時に exit 4 を返す設計は正しいが、bash の subshell `$(...)` 内で使うと `set -euo pipefail` 下でレシピ全体が死ぬ。利用側 boilerplate を library 側で吸収できないか検討する。

## 背景

v0.40.1 dogfood (DR-0038 適用) で踏んだ罠の library 側改善案。

各リポの justfile push gate で以下のように書くのが標準パターン:

```just
[script]
check-on-default-branch:
    if ! bump-semver vcs is on-default-branch; then
        cur=$(bump-semver vcs get current-branch)   # ← ambiguous 時 exit 4
        bn=$(bump-semver vcs get default-branch)
        printf >&2 "..."
        exit 1
    fi
```

「@ の ancestor に bookmark が無い divergent な枝」では `vcs get current-branch` が `exitCodeAmbiguous (4)` を返す (= 設計通り、「現在の branch を答えられない」)。bash の `set -euo pipefail` 下では subshell `$(...)` の exit 非ゼロが recipe 全体を殺し、hint メッセージを出す前に exit 4 で死ぬ。

現状の回避策 (= v0.40.1 で 4 リポに適用):

```bash
cur=$(bump-semver vcs get current-branch 2>/dev/null || echo "(ambiguous)")
```

これは利用側の boilerplate になっていて、library 側で吸収できないかという話。

詳細経緯: docs/findings/2026-06-22-dr-0038-dogfood-pitfalls.md の 「2. vcs get current-branch ambiguous で subshell が exit 4 → gate 自身を殺す」を参照。

## 改善案候補

### 案 A: `--fallback=VALUE` flag

```bash
cur=$(bump-semver vcs get current-branch --fallback='(ambiguous)')
```

- ambiguous 時に fallback 値を stdout に出して exit 0
- pros: 直接的、利用側のフォールバック表現が CLI に寄る
- cons: 「ambiguous = エラー」の semantic が緩む、他の get key にも同じ flag が必要になる可能性

### 案 B: `--no-error`

```bash
cur=$(bump-semver vcs get current-branch --no-error)
[ -z "$cur" ] && cur='(ambiguous)'
```

- ambiguous 時に何も出さず exit 0
- pros: bash 側で `${cur:-(ambiguous)}` の自然な fallback と組合せやすい
- cons: 「empty string vs 失敗」の区別が曖昧

### 案 C: `vcs is on-default-branch` を verbose 化

```bash
result=$(bump-semver vcs is on-default-branch --verbose --json)
# result = {"on_default": false, "current": "feature/x", "default": "main"}
```

- 1 回の呼び出しで current / default / on_default を全部取れる
- pros: subshell 呼び出しが 1 回に減る、ambiguous も「current is null」で表現可能
- cons: `is` verb に出力情報を追加するのは責務逸脱気味 (= 既存 `is` は exit code only)

### 案 D: 現状維持 (= 利用側で 2>/dev/null || echo)

- 各 justfile の boilerplate を docs に記録 + DR-0038 Adoption pattern 節に明示済
- pros: library 側変更なし、現実的に動いている
- cons: 4 リポで同じ boilerplate を保守する必要

## 受け入れ条件

- [ ] 4 案の比較検討が完了し、採用案が決定される
- [ ] 採用案 A/B/C の場合: 実装・テストが完了する
- [ ] 採用案 D の場合: DR-0038 Adoption pattern 節に boilerplate が明示済みであることを確認

## 一次資料

- docs/findings/2026-06-22-dr-0038-dogfood-pitfalls.md (= 罠の発見経緯)
- DR-0038 Adoption pattern 節 (= 現状の利用側 boilerplate)

## 注記

- 本 issue は v0.40.1 dogfood (= 4 リポ適用) で踏んだ罠の上流還元
- 現状 (= 案 D) で機能はしているので緊急性は低い
- 設計判断要素 (= どの案を採るか / そもそも CLI 側で吸収すべきか) は当事者判断に委ねる
- スコープ外: 4 リポの justfile 書き換え (= 採用案に応じて段階移行)
