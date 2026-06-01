# Refactor: `autoAdvanceBookmark` を jj 公式 `jj bookmark advance` に委譲

Status: 未着手 (2026-06-01)

## 背景

DR-0025 (auto-advance description check) を実装中に kawaz が発見:

- jj 公式 `jj bookmark advance` が DR-0020 PR-5.2 で自作した `autoAdvanceBookmark`
  と本質的に同じ機能を提供している
- bump-semver の `src/vcs_backend.go autoAdvanceBookmark` (~60 行) は jj 公式機能の
  reimplementation
- DR-0020 PR-5.2 設計時に `jj bookmark advance` 未認知だった可能性

## jj bookmark 操作の階層

3 つの primitive がある (= 抽象度の昇順):

| コマンド | 役割 | new bookmark を作る? | forward-only? |
|---|---|---|---|
| `jj bookmark set NAME -r REV` | 作成 or 移動 (どちらも可) | yes | no (= backwards/sideways も可、`--allow-backwards` 要) |
| `jj bookmark move NAME --to REV` | **既存** bookmark の移動のみ | no (= 不在ならエラー) | jj が backwards/sideways を refuse (= `--allow-backwards` 要) |
| `jj bookmark advance NAME --to REV` | 移動の forward-only 特殊化 + auto-from | no | yes (= 後退は引数 reject) |

現実装 `autoAdvanceBookmark` は **`move`** を使用 (= forward-only 制約は自前の ancestor
check で担保、`--allow-backwards` は deliberately omit)。`advance` 委譲は「自前の
ancestor check + move」を「`advance` 1 コマンド」に圧縮する refactor。

## 機能対応表

| 観点 | bump-semver `autoAdvanceBookmark` | `jj bookmark advance` |
|---|---|---|
| 対象 bookmark | 引数 (= `--branch NAME`) | 位置引数 or `revsets.bookmark-advance-from` (default: `heads(::to & bookmarks())`) |
| target | clean→`@-`, dirty→`@` (内部分岐) | `--to <REVSET>` (default: `revsets.bookmark-advance-to` = `@`) |
| 既存性 check | 自前 (`present(NAME)`) | jj 内蔵 (= 不在 bookmark は対象に含まれない) |
| 祖先 check | 自前 (`NAME & ::@`) | jj 内蔵 (= `heads(::to & bookmarks())` で ancestor のみ) |
| forward-only | 自前 (`--allow-backwards` を deliberately omit) | jj 内蔵 (= forward-move のみ) |
| 既に target | 自前 short-circuit (`NAME & target` で判定) | jj 内蔵 (= no-op) |
| description check | DR-0025 で追加 | **なし** (= push reject で別レイヤ捕捉) |

## 委譲後の素案

clean/dirty 分岐は外側に残し、各分岐で `jj bookmark advance` を呼ぶ:

```go
func (j *jjBackend) autoAdvanceBookmark(name string) error {
    clean, err := j.IsClean()
    if err != nil { return err }
    target := "@-"
    if !clean {
        target = "@"
        // DR-0025: description check は引き続き必要
        // (= jj bookmark advance では check されない、push reject で検出)
        if err := j.checkDescriptionForPush(name); err != nil { return err }
    }
    if _, err := runBackendCmd("jj", "bookmark", "advance", name, "--to", target); err != nil {
        return &exitErr{code: exitCodeVCSExec, msg: err.Error()}
    }
    return nil
}
```

削減できる行数: 約 50 行 (= existence/ancestor/at-@/at-target chain を全部 jj 側に委譲)。

## 検討事項

### 1. エラーメッセージの変化

現状の自前 chain は「`auto-advance can only move forward, not sideways`」のような
**bump-semver 文脈の固有メッセージ**を返す。`jj bookmark advance` 委譲後は jj 自身の
エラーメッセージが surface する (= "Refusing to move bookmark backwards or sideways"
等)。

trade-off:
- pros: jj の進化に追従、保守コスト減
- cons: ユーザに「`--jj-bookmark-auto-advance` のせいで止まった」のヒントが薄れる

→ 委譲後は `jj bookmark advance` の stderr に bump-semver 側の prefix (= 「vcs push
--jj-bookmark-auto-advance:」) を加えて wrap する設計が現実的か。

### 2. 既存テストへの影響

`src/cmd_vcs_push_test.go` の AutoAdvance 系テスト (= ~10 個) は、自前 chain の
固有挙動 (= 順序、stderr 文字列) を pin している。委譲後は:

- 成功 path のテストはそのまま動く (= 結果が同じ)
- 失敗 path のエラーメッセージ assert を `jj bookmark advance` 仕様に合わせて更新

更新範囲は約 30-50 行の test 修正。

### 3. jj version の最低要求

`jj bookmark advance` は jj 0.16+ で導入。bump-semver の README / docs に「jj 0.16 以上」と
明記する必要がある (現状の README 確認要)。

### 4. description check の位置

- 案 a: `autoAdvanceBookmark` 内で advance 直前に check (= 上記素案)
- 案 b: `Push` 全体の pre-step として独立 (= auto-advance flag に関わらず描述 check)

→ 案 a が DR-0025 の "auto-advance だけが踏むバグ" という scope に整合。bare push
(= no auto-advance) は dirty を bookmark に乗せない (= 既に bookmark がある場所を push)
ので、description は元々ユーザ責任。

## 関連 follow-up を merge (= 本 refactor で一気に消化)

- **jj log 5 calls 統合 (5→1 複合 template)**: code-review (reuse) で指摘された
  efficiency cleanup。`present(NAME)` / `NAME & ::@` / `NAME & @` / target description /
  `NAME & target` を 1 jj log + 複合 template で取る案。**本 refactor で `autoAdvanceBookmark`
  ごと消える**ので単独 land せず、ここで一括処理する (= 委譲後は call 数自体が 1-2 個
  まで激減、複合 template 案は moot)

## 次のアクション

1. DR 起票 (新 DR-0026 等): `jj bookmark advance` 委譲 refactor
2. jj 最低 version を README / docs に明記
3. `autoAdvanceBookmark` を委譲版に書き換え (= 自前 chain を `jj bookmark advance --to`
   1 コマンドに圧縮、上記「5→1 統合」案は自動的に解消)
4. test 更新 (= エラーメッセージ assert を jj 仕様に合わせる)
5. journal 記録

## 関連

- DR-0020 PR-5.2: `--jj-bookmark-auto-advance` 元設計 (= 自前実装の正本)
- DR-0025: description check 追加 (= 本 refactor 後も残る)
- jj 公式 docs: https://docs.jj-vcs.dev/latest/cli-reference/#jj-bookmark-advance
