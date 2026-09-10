---
title: check-version-bumped が origin に version file が無い初回 push で常に失敗する
status: open
category: bug
created: 2026-09-10T09:51:16+09:00
last_read:
open_entered: 2026-09-10T09:51:16+09:00
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

# check-version-bumped が origin に version file が無い初回 push で常に失敗する

## 概要

初回 push (origin の main が GitHub 生成の Initial commit だけで `package.json` が無い状態) で、justfile の `check-version-bumped` (canonical の recipe、`bump-semver vcs …` を使う) が

```
bump-semver: jj file show -r main@origin package.json: Error: No such path: package.json
```

を出したあと「bump-trigger-paths が変わってるが version 未 bump」で常に失敗する。version を bump しても (0.1.0 → 0.1.1) 同じ結果になる。

## 背景

再現手順: `gh repo create --license MIT` で作った remote に、ローカルで育てた jj リポを rebase して `just push`。

観測: kawaz/ccmsg-webui の初回 push (2026-09-10)。

ワークアラウンド: push recipe の他の gate (ci / check-translations) を手で回し、`bump-semver vcs push --branch main --jj-bookmark-auto-advance` を直接実行して push した。

論点: origin 側に version file が無い場合、「origin の version = なし」として bump 済み扱いにするのが妥当か (= 初回 push は必ず bump されているとみなす)。裏取りしてから採否を決める必要がある (= `jj file show` の No such path エラーを検出して early-return するのが妥当か、それとも別の判定ロジックが必要かは要調査)。

## 受け入れ条件

- [ ] `jj file show -r main@origin <version-file>` が `No such path` を返すケースの扱いを決定する (裏取り後)
- [ ] 初回 push (origin に version file が存在しない) で `check-version-bumped` が誤って失敗しなくなる
- [ ] 既存の「origin に version file がある通常ケース」の bump 検出動作を壊さない

## TODO

<!-- wip 時のみ -->
