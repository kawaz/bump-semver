# DR-0026: `autoAdvanceBookmark` を jj 公式 `jj bookmark advance` に委譲

- Status: Active
- Date: 2026-06-02
- Extends: DR-0020 PR-5.2 (`--jj-bookmark-auto-advance`)
- Relates to: DR-0025 (description check は委譲後も維持)

## Context

DR-0020 PR-5.2 で導入した `autoAdvanceBookmark` (~75 行) は、jj 公式
`jj bookmark advance` (jj 0.39.0 で導入) と本質的に同じ機能を独自実装している。
DR-0025 実装中に kawaz が公式 command を発見、reimplementation を委譲に置き換える
refactor を切り出した (= 旧 issue `docs/issue/auto-advance-delegate-to-jj-bookmark-advance.md`)。

### jj bookmark 操作の primitive (抽象度昇順)

| コマンド | 役割 | new bookmark? | forward-only? |
|---|---|---|---|
| `jj bookmark set NAME -r REV` | 作成 or 移動 | yes | no (`--allow-backwards` 要) |
| `jj bookmark move NAME --to REV` | **既存** 移動のみ | no | jj が backwards/sideways を refuse |
| `jj bookmark advance NAME --to REV` | 移動の forward-only 特殊化 + 不在 silent skip | no | yes (= 後退/横移動は reject) |

現実装は `move` ベース + 自前の existence/ancestor/at-@/at-target chain (= ~50 行)。
`advance` 委譲はこの chain を 1 コマンドに圧縮する。

## Decision

### A 案採用: `jj bookmark advance` に委譲、clean/dirty 分岐と description check のみ外側に残す

`autoAdvanceBookmark` を以下の構造に書き換える:

1. `IsClean()` で target 決定 (clean → `@-`, dirty → `@`)
2. **clean かつ bookmark が `@` にある**場合は短絡 return (= 後退移動を回避)
3. DR-0025 description check (`-r <target>` で query)
4. `jj bookmark advance NAME --to <target>` に委譲
5. エラーは `vcs push --jj-bookmark-auto-advance: ...` prefix で wrap (jj stderr は folding)

削減行数: 約 50 行 (= existence / ancestor / at-target chain が消える)。

### `jj bookmark advance` の実測挙動 (jj 0.41.0、2026-06-02 検証)

| 入力 | 結果 | 旧自前 chain | 委譲後 |
|---|---|---|---|
| 存在しない bookmark | `Warning: No matching bookmarks ... No bookmarks to update.` exit 0 | nil (fall through) | 同等 (= 旧と同じ silent skip) |
| ancestor 上で forward 可 | bookmark 移動、exit 0 | OK | OK |
| 既に target にある | `No bookmarks to update.` exit 0 | nil | 同等 |
| sideways / 後退 | `Error: Refusing to advance bookmark backwards or sideways: NAME` exit 1 | 自前 exit 3 + ancestor hint | exit 3 + jj msg を folding (prefix で wrap) |
| **bookmark が @、clean (target=`@-`)** | 後退 → exit 1 error | **nil (short-circuit 2b)** | **要 guard** |

最後の行が **行動変化**: 旧 chain は「bookmark が @ にあるなら、clean モードでも no-op
(後退移動を回避)」と短絡していた。これを残さないと
`TestJjBackend_Push_AutoAdvance_AtWorkingCopy_Clean` が回帰する。
→ **clean 分岐内に `name & @` チェックを 1 つだけ残す** (= 削減対象は他 3 つの chain)。

### description check は委譲後も維持 (DR-0025)

`jj bookmark advance` は description 有無を check しない (= push reject まで surface
しない)。DR-0025 が指摘する push reject 無限ループは依然として起こり得るため、
**advance 直前**に description check を行う。位置は target 確定後 / advance 委譲前、
両 branch (clean / dirty) 共通。

### エラーメッセージ仕様変更

旧:
```
vcs push --jj-bookmark-auto-advance: bookmark "main" is not an ancestor of @
(auto-advance can only move forward, not sideways); rebase or move the bookmark
manually then retry
```

新 (jj の文面を folding):
```
vcs push --jj-bookmark-auto-advance: jj bookmark advance main --to @-: Refusing
to advance bookmark backwards or sideways: main
```

trade-off:
- pros: jj 側の改善 (= 説明文の更新、新 case の追加) に自動追従、~50 行の保守責任が消える
- cons: 「rebase or move the bookmark manually then retry」の recovery hint が消える
  → prefix `vcs push --jj-bookmark-auto-advance:` で「どのフラグが原因か」は残る。
  jj 公式 message は "advance" 主語 + bookmark 名を含むので、ユーザは jj docs に
  辿り着ける

→ 採用。fixed string で hint を持つことの保守コスト > 文面追従の恩恵。

## 不採用案

### B 案: `jj bookmark move` 委譲 (= existence/ancestor は自前のまま)

`advance` の代わりに `move` に委譲する案。`move` は jj 内蔵で backwards/sideways
refuse があるが、existence チェック (= `present()`) と「不在 bookmark の場合は
fall-through」の挙動は自前で書く必要がある。`advance` の "no matching bookmarks =
silent skip" を再現するため。

却下: 削減効果が半分以下 (= existence chain が残る)。`advance` で十分。

### C 案: description check も委譲 (= autoAdvanceBookmark 廃止)

DR-0025 を撤回して description check を捨てる案。

却下: DR-0025 の本質は「bump-semver が advance target に bookmark を置くなら、
その target の describe 責任を bump-semver が負う」。push reject 無限ループは
DR-0025 の Decision 通り pre-step で防ぐ。`jj bookmark advance` 自身は push 段階の
reject まで surface しないので、ループは委譲後も再発する。

## 影響範囲

### 既存変更

- `src/vcs_backend.go` — `autoAdvanceBookmark` (現 ~75 行) を ~25 行に圧縮:
  - 削除: existence chain (`present(NAME)`)、ancestor chain (`NAME & ::@`)、at-target chain (`NAME & target`)、`jj bookmark move` 呼び出し
  - 残存: `IsClean()` / target 決定 / clean 時の at-@ short-circuit / DR-0025 description check / `jj bookmark advance` 委譲
  - エラー wrap: `runBackendCmd` が既に jj stderr を folding するので、`exitErr` の msg に prefix を付けるだけで十分

### テスト変更

- `src/vcs_backend_test.go`:
  - `TestJjBackend_Push_AutoAdvance_Divergent` — assertion 緩和: 旧 `"ancestor"` 限定から jj message ("backwards or sideways" / "advance") にも match するよう緩める (現状 `Contains(ee.msg, "ancestor") || Contains(ee.msg, "advance")` で既に OR 化済 → そのまま動く)
  - 他 5 個 (Forward / AlreadyAtParent / AtWorkingCopy_Clean / Dirty / GitSilentNoOp) は **挙動変わらず、そのまま pass する想定**
- `src/cmd_vcs_push_test.go`:
  - happy path 系 3 個 (Forward / Dirty / DirtyWhitespaceDescription) はそのまま pass
  - description check 系 2 個 (JjCleanTargetNoDescription / JjDirtyNoDescription) は description check が advance 委譲前に走る位置を保つので pass
  - GitSilentNoOp / ParserAcceptsFlag は dispatcher 層、影響なし

### docs / version 要件

- `README.md` の `--jj-bookmark-auto-advance` 行に「**Requires jj 0.39+** (for the underlying `jj bookmark advance` command)」を追記
- 旧 issue `docs/issue/auto-advance-delegate-to-jj-bookmark-advance.md` を delete (resolved)
- journal `docs/journal/2026-06-02-advance-delegate-to-jj.md` に経緯と行数削減記録

## 関連

- DR-0020 PR-5.2: `--jj-bookmark-auto-advance` 元設計 (= 自前実装の正本)
- DR-0020 PR-5.2.1: git で silent no-op 化 (= backend-prefix general rule)
- DR-0025: description check 追加 (= 本 refactor 後も pre-step として残る)
- jj 公式 docs: https://docs.jj-vcs.dev/latest/cli-reference/#jj-bookmark-advance
- jj CHANGELOG 0.39.0 (= `jj bookmark advance` 導入): https://github.com/jj-vcs/jj/blob/main/CHANGELOG.md
