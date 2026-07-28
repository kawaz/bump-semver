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
origin: llm-gateway
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

## 部外者からのフラグ (llm-gateway より、実測 2026-07-29)

llm-gateway 側で bump-semver の justfile canonical 実装を移植した際、同じ
recipe で同じ崩れを実測し、`[doc("...")]` アノテーションで解決した実例が
llm-gateway の justfile にある (他の複数行コメント recipe `ci` / `test` /
`sign` / `bump-version` も同様に `[doc(...)]` で対処済み)。

対処方針・採否の裁定は本リポ側に委ねる。参照情報として提示するのみで、
実装の指示ではない。bump-semver 側で同じ構造 (複数行コメント + 最終行が
継続文) の recipe が他にもあるかは未確認。

### ついでの気づき (別対処でも可、任意)

同 justfile の `push` recipe 内の通知コマンドが `cmux-msg notify --self ...`
(justfile:117) になっている。`cmux-msg` は現在も PATH に存在し動作するため
実害はないが、現行コマンド名は `ccmsg`。rename への追従要否は本リポの判断
に委ねる。

## 受け入れ条件

- [ ] `just --list` の `on-success-release` 行が、それ単体で意味の通る
      説明文になっている (文の途中から始まらない、不要な括弧が残らない)
- [ ] 同様の複数行コメント + `--list` 表示崩れが他の recipe に無いか確認
