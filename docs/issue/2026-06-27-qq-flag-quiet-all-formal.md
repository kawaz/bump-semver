---
title: -qq の挙動を help / docs に formal 化 (現状 undocumented alias)
status: open
category: request
created: 2026-06-27T01:39:37+09:00
last_read:
open_entered: 2026-06-27T01:39:37+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: 依頼元プロジェクト (grapheme.mbt / timespec.mbt セッション)
---

# -qq の挙動を help / docs に formal 化 (現状 undocumented alias)

## 概要

`-qq` が `--quiet-all` 相当として振る舞うことが文書化されていない。利用側各リポで `-qq` が慣習として定着しているが正式仕様として未文書化であるため、help / docs に明示する。

## 背景 (= dogfood 観察 from grapheme.mbt / timespec.mbt セッション)

`bump-semver get` の quiet flag は以下の挙動が観測される (v0.41.0 実機検証):

正常系: `-q` / `-qq` / `-qqq` / `--quiet` / `--quiet-all` いずれも get の戻り値は保持
エラー系 (`bump-semver get /nonexistent`):
- flag なし / `-q`: stderr に error message
- `-qq` / `--quiet-all`: stderr も空

つまり **`-qq` は `--quiet-all` 相当として振る舞う**。しかし `bump-semver get --help` には:
```
-q, --quiet         suppress stdout and hints
    --quiet-all     suppress stdout, hints, and errors
```
としか書かれておらず、**`-qq` が `--quiet-all` の alias として動くことが文書化されていない**。

## 問題

kawaz 各リポ (claude-cmux-msg, claude-plugin-reference, claude-gh-monitor, timespec.mbt, grapheme.mbt 等) で `-qq` が定着して多用されている。**慣習として定着しているが正式仕様としては未文書化**。

懸念:
1. **意味の取り違え**: 利用側は「-qq = -q を強めに」程度の認識で書いている可能性。実は error message まで殺すモードだと知らずに使っているケースは CI で error が握りつぶされる構造リスク
2. **将来の意味変更リスク**: 文書化されてない alias は upstream が将来 `-qq` を別解釈に変えると silent breaking change
3. **discoverability**: 利用者が `-qq` を見て help を引いても由来が分からない

## 提案 (どれか / 全部)

a. `bump-semver get --help` の `-q, --quiet` 行に alias 関係を追記 (例: `(use -qq for --quiet-all shorthand)`)
b. `--help-full` or README の Quiet semantics section に「`-q` 重ね指定で `--quiet-all` 相当」と明示
c. もし `-q -q` = `--quiet-all` が cobra の counting flag 機能由来で意図しない副作用なら、`-qq` を formal alias として定義し直す (= 偶発挙動から仕様化)

## 関連

- 上流還元 part-outsider フラグ from grapheme.mbt (session acad9f77...) + timespec.mbt (session 0ab6ca63...)
- v0.41.0 の moon.mod auto-detect 機能の dogfood で発覚

## 実装方針 (= 当事者セッションへ委ねる)

該当 cobra 定義箇所を読んで、`-q` が `CountFlag` 由来か明示 alias か確認。状態確定後に上記 a/b/c から選ぶ。実装コストは a (= help 文 1 行追記) が最小、c (= 仕様化) が最大。

## 受け入れ条件

- [ ] `-qq` の挙動 (= `--quiet-all` 相当) が help または docs に明示されている
- [ ] 利用者が `-qq` を見て help を引いたとき由来が分かる

## 追記 (2026-06-27, from grapheme.mbt session acad9f77...)

grapheme.mbt 側で justfile + publish.yml の `bump-semver` 呼び出し 7 箇所を意図単位でレビューした結果が来た:

- **5 箇所は `--quiet` で十分** (value 取得経路、error は早期 fail させたい)
- **2 箇所は意図的に `-qq`** (exit code で分岐する用途、自前で error 出すので bump-semver の stderr 不要)
- 機械的コピペで `-qq` を撒いていたのが「意図に応じて使い分け」に綺麗に整理された

## 結論 (= 当事者推奨の優先順)

**提案 a (help 文 1 行追記) で十分**:
- 「-qq = -q を 2 つ重ねた強い quiet」は Unix 慣習 (git rev-parse / grep -qq) で素直に通じる
- 提案 c (formal alias 化 via cobra) はメンテコストの割に得るものが少ない (既に動いてる)
- help に「-qq は --quiet-all 相当 = error も silence」を 1 行入れるだけで利用側の意図レビュー判断材料になる、安価で効く
- 提案 b (README quiet section) は a があれば不要、好みで追加

## 具体的な help 文案 (grapheme.mbt 提案、そのまま採用候補)

```
-q, --quiet         suppress stdout and hints (get keeps its value);
                    repeat (-qq) for --quiet-all behavior
    --quiet-all     suppress stdout, hints, and errors (get keeps its value; use with caution)
```

## 実装範囲

- src/help.go or cobra help text の該当 flag definition で help string を更新
- 全 sub-command で `-q` を持つ箇所 (get / compare / 他) すべてに同じ文言追加するか検討 (= 一貫性)
- README / docs/ には optional で追記

## 関連

- v0.42.0 同梱候補 (= push-wip / default-branch-path 機能群と一緒に release)
