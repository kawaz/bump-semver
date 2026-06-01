# 2026-06-02 `autoAdvanceBookmark` を `jj bookmark advance` に委譲

DR-0026 着地。`src/vcs_backend.go` の自前 chain (existence / ancestor / at-@ /
at-target / `jj bookmark move`) を `jj bookmark advance` 1 コマンドに圧縮した。

## 経緯

DR-0025 (description check 追加) を実装中に kawaz が jj 公式 `jj bookmark advance` を
発見。DR-0020 PR-5.2 で書いた ~75 行の自前 chain は実質 reimplementation だった。
DR-0025 は bug fix のみ land し、refactor は `docs/issue/auto-advance-delegate-to-jj-bookmark-advance.md`
に切り出し → 本作業で消化。

## 委譲前の empirical matrix (jj 0.41.0、2026-06-02 検証)

`jj bookmark advance` の挙動を 5 ケースで実測:

| ケース | jj exit | jj message | 旧自前 chain |
|---|---|---|---|
| bookmark 存在せず | 0 | `Warning: No matching bookmarks ... No bookmarks to update.` | nil (silent skip) |
| ancestor 上で forward 可 | 0 | `Advanced 1 bookmarks to ...` | OK |
| 既に target | 0 | `No bookmarks to update.` | nil |
| sideways / 後退 | 1 | `Error: Refusing to advance bookmark backwards or sideways: NAME` | 自前 exit 3 + ancestor hint |
| bookmark が @、target=`@-` | 1 | 上と同じ | **nil (short-circuit 2b)** |

→ 最後の行 (= bookmark at @、clean target) は naive 委譲だと **挙動変化**。
旧 chain は no-op で push に進む、委譲後は jj が exit 1 で reject する。
ここだけ guard を残す判断 (= clean 分岐内の `present(NAME) & @` revset)。

## 設計判断 (= A / B / C)

| 案 | 内容 | 採否 |
|---|---|---|
| A | `jj bookmark advance` 委譲 + clean 時 at-@ short-circuit + DR-0025 check は外側に残す | **採用** |
| B | `jj bookmark move` 委譲 (existence/ancestor は自前) | 不採用 (削減効果半減、`advance` で十分) |
| C | description check も委譲 = DR-0025 撤回 | 不採用 (push reject ループ再発、DR-0025 の本質を破壊) |

## 実装変更

`src/vcs_backend.go autoAdvanceBookmark`:

- 削除: `present()` chain、`NAME & ::@` ancestor chain、`NAME & target` at-target chain、`jj bookmark move`
- 残存: `IsClean()` 分岐 / clean 時 `present(NAME) & @` short-circuit / DR-0025 description check / `jj bookmark advance NAME --to <target>`
- エラー wrap: `runBackendCmd` が既に jj stderr を folding するので、`exitErr` の msg に `vcs push --jj-bookmark-auto-advance:` prefix を付けるだけ

### 行数

- before: 75 行 (function body 74 行 + コメント) ※実装本体は ~50 行
- after: ~40 行 (本体 ~35 行)
- 削減: 自前 chain 約 50 行 → 5 行 (= 委譲 1 行 + wrap 4 行)

### テスト assertion の更新

事前想定では「失敗 path の error message assertion を jj 仕様に合わせて update」だったが、
実際は **既存テストすべて変更不要で pass**:

- `TestJjBackend_Push_AutoAdvance_Divergent` の assertion は `Contains(msg, "ancestor") || Contains(msg, "advance")` で OR 化済 (= DR-0025 land 時点で既に防御的に書かれていた)。委譲後の jj message に "advance" 主語 + bookmark 名が入るので、`Contains("advance")` 側で hit する
- `TestJjBackend_Push_AutoAdvance_AtWorkingCopy_Clean` は guard を残したことで挙動が変わらず pass
- description check 系 2 個も advance 委譲 **前** に description check が走る位置を守ったので pass
- happy path (Forward / AlreadyAtParent / Dirty / DirtyWhitespaceDescription) は jj 側の挙動と完全一致するため pass

→ assertion 更新工程はゼロ。code-review で発覚した「事前想定が overshoot だった」典型例。

### docs

- DR-0026 起票
- INDEX.md に追記
- README.md / README-ja.md の `--jj-bookmark-auto-advance` 行に「jj 0.39+ 必須」追記、
  ついでに「git で usage error」の古い記述を「silent no-op」に修正 (= PR-5.2.1 land 時に
  本文修正漏れだった)

## ハマり所 → 解決策

### ハマり 1: 旧 issue の素案 (Go コード) の description check 位置が dirty-only だった

issue doc 46-60 行の素案では `checkDescriptionForPush(name)` が `if !clean` 内にあり、
DR-0025 の hoist (= 両 branch 共通) と矛盾していた。advisor が指摘、task 文に従って
target 確定後 / 両 branch 共通の位置に置き直し。`TestJjBackend_Push_AutoAdvance_JjCleanTargetNoDescription`
が回帰検出 guard として機能した。

### ハマり 2: jj 最低 version を 0.16 と書きそうになった

issue doc が「jj 0.16+」と推測 (= 要確認 とも書いていた)。jj 公式 CHANGELOG を grep
して **0.39.0** で `jj bookmark advance` が導入されたと確定 → README に 0.39+ で記載。

### ハマり 3: at-@ clean 短絡を委譲時に消す誘惑

issue doc の素案では at-@ short-circuit が削除されていた。実機で `bookmark @、target=@-`
を試すと jj が exit 1 で reject → guard を残す判断に変更。`TestJjBackend_Push_AutoAdvance_AtWorkingCopy_Clean`
を読まずに進めていたら land 後に CI 落ちで気づくところだった。

## 関連

- DR-0026 (本作業の正本)
- DR-0025 (description check、本作業後も維持)
- DR-0020 PR-5.2 (元設計、本作業で chain ~50 行を消化)
- jj 0.39.0 CHANGELOG: `jj bookmark advance` 導入
