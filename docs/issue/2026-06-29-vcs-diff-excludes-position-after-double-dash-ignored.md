---
title: vcs diff の --excludes を -- の後ろに置くと positional path 扱いされ exclude が無効化される
status: open
category: bug
created: 2026-06-29T12:29:29+09:00
last_read:
open_entered: 2026-06-29T12:29:29+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: kuu.mbt (dogfooding 起点)
---

# vcs diff の --excludes を -- の後ろに置くと positional path 扱いされ exclude が無効化される

## 概要

`bump-semver vcs diff` の `--help` に `[PATH..] [--excludes PATTERN]...` と
示されている使用例通りに `--` の後ろへ `--excludes` を置くと、exclude が完全に
無視され、引数値 (glob pattern) が positional path として include set に取り込まれる。

## 背景

kawaz リポ群の justfile (kuu.mbt / timespec.mbt / cache-warden / jj-worktree /
bump-semver 自身) で以下の書き方を採用していた:

```make
bump-semver vcs diff -q main@origin -- "$@" --excludes 'glob:src/**/*_wbtest.mbt'
```

bump-semver 0.45.0 + jj 管理リポで実機検証すると `--excludes` が認識されず、
`glob:src/**/*_wbtest.mbt` が positional path として expand された file 群が
include set に加わる挙動を観測した。

### jj 呼び出し trace で確認した実際の引数

```
jj diff --summary --from main@origin --to @ -- src/ src/core/advanced_wbtest.mbt src/core/cmd_wbtest.mbt ...
```

`--excludes` も fileset 式 `(src/) ~ glob:...` も現れない。

### workaround

`--excludes` を `--` の**前**に移動すると動く:

```make
# 動く:
bump-semver vcs diff -q main@origin --excludes 'glob:src/**/*_wbtest.mbt' -- "$@"
```

kuu.mbt の justfile は commit `fbf991ff` で対症療法済み。

## 受け入れ条件

以下のいずれか:

- [ ] `--` の後ろに `--excludes` を置いても exclude として認識される (= help 例と整合)
- [ ] `--` 以降の `--flag` 形式引数を検出して error にする (= 誤用を明示エラー化)
- [ ] help の usage 例を「`--excludes` は `--` の前に置く」と明示し、影響を受ける
  既存リポ (timespec.mbt / cache-warden / jj-worktree 等) の justfile も修正済み

## 推奨修正案

**選択肢**:

1. `--excludes` を `--` の後ろでも認識するように parser を変更 (= help と整合)
2. `--help` を「`--excludes` は `--` の前に置く」明示に変更 + 影響リポの justfile 修正
3. `--` 以降に `--flag` 形式の引数が現れたら error にして誤用を検出可能にする

(2) が最小変更。影響リポは kawaz 管理で 5 件以上存在する見込み (kuu.mbt は対症療法済み)。
(1) は `--` の POSIX 意味論 (= 以降は positional のみ) から外れるため parser 設計判断が要る。
(3) は strict 化であり、誤用の可視化に有用だが既存利用側を全部直す必要がある。

## 関連実装箇所 (参考)

- `vcs_backend_jj.go:115-131` (Diff) と `buildJjPathspec (181-213)` — fileset 式を
  組む実装自体は正しい。問題は `--excludes` slice が空のまま backend に渡るため
  dispatcher 層の引数解析が原因と推測。裏取りは bump-semver 側で要確認。

## 影響範囲

test-only commit (= `*_wbtest.mbt` / `*_test.go` 等) が bump trigger に混入する可能性。
観測上、kawaz リポでは該当ファイルのみ変更する commit が少なかったため長期間
顕在化しなかった。今後 test-only commit を増やすと踏む機会が増える見込み。

観測環境: bump-semver 0.45.0、jj 管理リポ、macOS Darwin 25.5.0
