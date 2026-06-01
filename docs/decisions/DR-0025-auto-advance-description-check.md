# DR-0025: `--jj-bookmark-auto-advance` の description 必須 check

- Status: Active
- Date: 2026-06-01
- Extends: DR-0020 PR-5.2 (`--jj-bookmark-auto-advance`)

## Context

DR-0020 PR-5.2 で導入した `vcs push --jj-bookmark-auto-advance` が、no-description な
advance target に bookmark を移動 → 直後の `jj git push` が **push reject 無限ループ**
を起こすバグが v0.30.0 リリース中に観測された (削除済 issue
`docs/issue/auto-advance-no-description-push-fail.md`、内容は本 DR と
[journal 2026-06-01-auto-advance-description-fix](../journal/2026-06-01-auto-advance-description-fix.md)
に統合)。

### 再現条件 (dirty branch / target=@)

1. `just push` 実行
2. 内部 `lint-go` (= `gofmt -w .`) で未整形 file が working copy に書き戻され dirty 化
3. `--jj-bookmark-auto-advance` の dirty 判定 → @ に bookmark advance
4. @ は jj が auto-create した new empty change (= **no description**) + gofmt 差分
5. `jj git push` が `Won't push commit XXX since it has no description` で exit 3
6. 再度 `just push` してもループ (= 同じ状態が再生成される)

### 再現条件 (clean branch / target=@-、code-review で発覚)

dirty branch だけの対策では不十分。clean 側にも同型の trap がある:

1. bookmark C → `jj new` (= 新 empty change D = `@-` だが describe してない) → `jj new`
   (= さらに新 empty change E = `@`、clean)
2. `--jj-bookmark-auto-advance`: clean → target=`@-`=D
3. D は undescribed → `jj bookmark move main --to @-` 成功 → `jj git push` が D を push
   しようとして reject、hint なしの opaque error

→ 「bump-semver が advance target に bookmark を移動するなら、その target の describe
責任を bump-semver が負う」が DR の本質。clean / dirty いずれの分岐でも target 確定後に
describe check を行う。

### 観測された jj の description 仕様 (実機検証 2026-06-01)

| Case | `-T description` 出力 | `if(description, "T", "F")` | push reject? |
|---|---|---|---|
| 未設定 (auto-create) | `` (空) | `F` | yes |
| `jj describe -m ""` | (no-op、`Nothing changed`) | `F` | yes |
| `jj describe -m "real"` | `real` | `T` | no |
| `jj describe -m "   "` (whitespace only) | `   ` | `T` | **no** |

→ jj 自身は「**空文字列でなければ description あり**」と判定。whitespace-only も
受け入れる。push reject の判定は `description == ""` と等価。

## Decision

### A 案採用: description 必須 check を auto-advance pre-step に追加 (両 branch)

`autoAdvanceBookmark` の **clean/dirty 分岐後** (= target 確定後) に、bookmark を target
に move する **前** に target の description を確認。空なら exit 3 +
`jj describe -r <target>` hint で早期 fail。

- 初版 (= initial DR-0025、code-review 前) は dirty branch (target=@) だけに check を
  追加していたが、code-review (altitude angle) で「clean branch (target=@-) でも同型の
  trap が再現する」と指摘 → check を target に hoist して両 branch 共通化。

### 判定主体は jj 自身 (= template engine)

実装は `jj log -r @ --no-graph -T 'if(description, "T", "F")'` で問い合わせ、
出力が `"T"` でなければ fail。これは:

- **jj の push reject 判定と同じロジック** (= jj template の `if` は空文字を falsy、
  非空文字を truthy として扱う = jj 内部の description 有無判定と一致)
- whitespace-only description は jj が accept → bump-semver も accept (= over-reject
  防止)
- Go 側のコードがシンプル (= `TrimSpace == ""` ではなく `"T"` 文字列比較)

### 失敗時の hint

```
vcs push --jj-bookmark-auto-advance: advance target <TARGET> for bookmark "<NAME>"
has no description; jj would refuse to push it. Run `jj describe -r <TARGET>` to
set a description, then retry (or move bookmark "<NAME>" manually if <TARGET>
should not be the target)
```

- `<TARGET>` は `@` (= dirty branch) または `@-` (= clean branch) のいずれか、実際の
  target 確定後の値が入る
- 「`jj describe -r <TARGET>`」を明示 (= ユーザが次に打つコマンドそのもの)
- 「bookmark を手動で move する代替案」も提示 (= advance target にしたくないケース)

## 不採用案

### B 案: auto-advance を `@-` のみに set (= 元仕様変更)

DR-0020 PR-5.2 で **clean なら @-、dirty なら @** という分岐を確定済 (= kawaz 確定
2026-05-31「clean 前提と dirty + describe して push の両運用」)。dirty 運用 (=
describe 済 + push) は実在のワークフローで、**常に @-** に変えると「dirty + describe
済」運用が壊れる。撤回は workflow 削減になり不可。

### C 案: `lint-go` の `gofmt -w` を pre-check 化 (= `gofmt -d` で diff 検出)

```just
lint-go:
    gofmt -d . > /dev/null || (echo "gofmt diff detected; commit fmt first"; exit 1)
    go vet ./...
```

却下理由:
- `lint-go` の責務拡張 (= 「整形 + vet」から「整形検査 + vet」へ)
- 「整形漏れがあったら自動で整形して続行」という現状の便利さを失う (= ユーザに「先に
  fmt commit しろ」を強いる)
- 根本原因は「auto-advance が undescribed commit を target にすること」であり、`gofmt`
  はトリガの一つに過ぎない。手動編集中の dirty + auto-advance でも同じ罠は再現する
  → fix はもっと根本側 (= auto-advance 自身) で打つべき

## 影響範囲

### 既存変更

- `src/vcs_backend.go` — `autoAdvanceBookmark` の clean/dirty 分岐後 (target 確定後)
  に description check を hoist。jj template engine 経由で empty 判定、空なら exit 3
  + hint。両 branch で同じ check が走る

### テスト追加

- `src/cmd_vcs_push_test.go`:
  - `TestRun_VcsPush_AutoAdvance_JjCleanTargetNoDescription` — clean branch
    (target=@-) で @- が undescribed なら exit 3 + `jj describe -r @-` hint
  - `TestRun_VcsPush_AutoAdvance_JjDirtyNoDescription` — dirty branch (target=@) で
    @ が undescribed なら exit 3 + `jj describe -r @` hint
  - `TestRun_VcsPush_AutoAdvance_JjDirtyWhitespaceDescription` — whitespace-only
    description は jj が accept する仕様を pin (= bump-semver も accept、over-reject
    しない)

### docs

- `docs/issue/auto-advance-no-description-push-fail.md` — 本 DR で resolve、delete
  (内容は本 DR + journal に統合)
- `docs/journal/2026-06-01-auto-advance-description-fix.md` — 経緯と実機検証結果、
  code-review で発覚した clean branch の同型 trap、hoist による修正

## 将来の refactor 候補

実装作業中に kawaz が jj 公式 `jj bookmark advance` を発見 (= DR-0020 PR-5.2 設計時には
未認知だった可能性)。本 DR の description check は **`jj bookmark advance` を使った
としても引き続き必要** (= bookmark advance 自体は description を check しない、push
reject で別レイヤ捕捉)。

`autoAdvanceBookmark` の existence/ancestor/forward-only/at-target chain ~60 行を
`jj bookmark advance --to` 委譲に置き換える refactor は **別 issue** で扱う:

- [docs/issue/auto-advance-delegate-to-jj-bookmark-advance.md](../issue/auto-advance-delegate-to-jj-bookmark-advance.md)

本 DR は scope を「bug fix のみ」に絞り、refactor を独立した review サイクルに分離。

## 関連

- DR-0020 PR-5.2: `--jj-bookmark-auto-advance` 元設計
- DR-0020 PR-5.2.1: git で silent no-op 化 (= backend-prefix general rule)
- DR-0022: Justfile 回帰 (= 現行 `just push` フロー)
- 将来 refactor: [docs/issue/auto-advance-delegate-to-jj-bookmark-advance.md](../issue/auto-advance-delegate-to-jj-bookmark-advance.md)
