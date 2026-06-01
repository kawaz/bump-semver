# 2026-06-01 auto-advance description check 追加

v0.30.0 リリース中に踏んだ `--jj-bookmark-auto-advance` の no-description push reject
ループを fix。DR-0025 起票。

## 経緯

v0.30.0 (glob: prefix) land 中に以下のフローでハマった:

1. `just push` 走る
2. 内部 `lint-go` が `gofmt -w .` で未整形 file を整形 → working copy が dirty 化
3. `--jj-bookmark-auto-advance` の dirty 判定 → @ に bookmark advance
4. @ は auto-create された empty change + gofmt 差分 = **no description**
5. `jj git push` が「Won't push commit XXX since it has no description」で exit 3
6. もう一度 `just push` してもループ (= 同じ状態に戻る)

復旧: `bump-semver vcs push --branch main` (= auto-advance なし) を直叩き。

## 実機検証 (= jj description の真の仕様)

`/tmp/jj-desc-probe` で確認:

| Case | `-T description` 出力 | `if(description, "T", "F")` | push reject? |
|---|---|---|---|
| 未設定 (auto-create) | `` (空) | `F` | yes |
| `jj describe -m ""` | (no-op、`Nothing changed`) | `F` | yes |
| `jj describe -m "real"` | `real` | `T` | no |
| `jj describe -m "   "` (whitespace only) | `   ` | `T` | **no** |

**ハマり所**: 当初 Go 側で `TrimSpace == ""` 判定にしていたが、whitespace-only は
jj が push accept する仕様。over-reject になる → jj template engine 経由 (= jj 自身
の truthy 判定) に変更。

## 設計判断 (= 案 A vs B vs C)

| 案 | 内容 | 採否 |
|---|---|---|
| A | description 必須 check を auto-advance pre-step に追加、早期 fail + `jj describe` hint | **採用** (元仕様維持 + 早期 fail) |
| B | auto-advance を `@-` のみに set (= dirty branch を消す) | 不採用 (dirty + describe 済 workflow が壊れる、kawaz 確定 2026-05-31 を撤回することになる) |
| C | `lint-go` の `gofmt -w` を pre-check 化 (= 整形漏れで fail) | 不採用 (lint-go 責務拡張、`gofmt` は trigger の一つに過ぎず根本原因は auto-advance 側) |

## 実装

`src/vcs_backend.go` autoAdvanceBookmark の dirty branch (target=@ 確定後) に
description check 追加。判定は jj template:

```go
descOut, _ := runBackendCmd("jj", "log", "-r", "@", "--no-graph", "-T", `if(description, "T", "F")`)
if strings.TrimSpace(string(descOut)) != "T" {
    return error...
}
```

Why jj template:
- jj の push reject 判定 (= description == "") と完全一致
- whitespace-only を accept (= over-reject 防止)
- Go 側がシンプル

## code-review で発覚した追加 trap (clean branch、hoist 修正)

初版 (dirty branch のみに check 追加) を land 後、`/code-review` (altitude angle) で
**clean branch (target=@-) でも同型の trap が再現する** と指摘。

再現:
- bookmark C → `jj new` (= 新 empty change D = `@-`、describe してない) → `jj new`
  (= さらに新 empty change E = `@`、clean)
- `--jj-bookmark-auto-advance`: clean → target=`@-`=D
- D は undescribed → bookmark move 成功 → `jj git push` が D を push しようとして
  reject、hint なしの opaque error

→ fix: description check を `clean/dirty 分岐の後` (= target 確定後) に hoist。
`-r <target>` で check、target が `@` でも `@-` でも一律で動く。テスト追加:
`TestRun_VcsPush_AutoAdvance_JjCleanTargetNoDescription`。

DR-0025 本文にも「clean branch 再現条件」と「hoist 採用」を反映。

## テスト

- `TestRun_VcsPush_AutoAdvance_JjCleanTargetNoDescription` — clean branch 同型 trap
  の pin (hoist で両 branch 共通化を担保)
- `TestRun_VcsPush_AutoAdvance_JjDirtyNoDescription` — dirty branch bug 再現 → fix
  後 exit 3 + `jj describe` hint を pin
- `TestRun_VcsPush_AutoAdvance_JjDirtyWhitespaceDescription` — whitespace-only は
  accept (= jj 仕様準拠) を pin

## 関連

- DR-0025: 設計判断の正本
- DR-0020 PR-5.2: `--jj-bookmark-auto-advance` 元設計
- DR-0022: Justfile 回帰 (= 現行 `just push` フロー、gofmt -w 経由で本 bug が露呈)
