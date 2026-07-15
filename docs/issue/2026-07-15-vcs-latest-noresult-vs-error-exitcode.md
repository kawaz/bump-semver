---
title: vcs get latest-release / latest-tag で「該当なし」と subprocess エラーが exit code で区別できない
status: open
category: request
created: 2026-07-15T11:52:26+09:00
last_read:
open_entered: 2026-07-15T11:52:26+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: kuu (依頼元プロジェクト)
---

# vcs get latest-release / latest-tag で「該当なし」と subprocess エラーが exit code で区別できない

## 概要

`vcs get latest-release` / `vcs get latest-tag` は、以下の 2 つの異なる状況を **同じ exit code 3** で返す:

1. 正常な「該当なし」(= semver 互換の tag/release が 1 つも存在しない。初回リリース前の bootstrap 状態など)
2. subprocess / API エラー (= `gh` の認証切れ・network 障害・`gh` 未インストール等)

利用側は exit code だけではこの 2 状況を区別できず、stderr のメッセージ文字列 (`"no semver-compatible ..."`) を grep する回避策を取らざるを得ない。

## 背景

v0.48.1 時点のソースで確認済み:

- `src/cmd_vcs_get_latest_tag.go` の doc comment (L23): `3  VCS subprocess error / no semver-compatible tags found`
- `src/cmd_vcs_get_latest_release.go` の doc comment (L27): `3  gh subprocess error, gh missing, or no semver-compatible releases`
- `src/vcs.go` L505: `return "", Version{}, fmt.Errorf("no semver-compatible tags found")` — 該当なしも通常のエラー値として返り、`fetchLatestTag` / `fetchLatestRelease` 経由でどちらも `emitVcsErr` に流れて exit 3 になる (`cmd_vcs_get_latest_tag.go` L49-51, `cmd_vcs_get_latest_release.go` L45-47)
- `ensureGhAvailable` (`cmd_vcs_get_latest.go` L176-183) の「gh 未インストール」も同じ error 型で同じ exit 3 経路

利用側 (kawaz/kuu の release workflow、`.github/workflows/release.yml`) では、初回リリース前の「本当に空」判定を fail-closed で行うために、stderr を一時ファイルにリダイレクトして `grep -qi "no semver-compatible"` するワークアラウンドを実装済み (同ファイル L87-126 の "semver gate: fail-closed bootstrap" コメント参照)。この回避策は stderr メッセージ文字列の完全一致に依存しており、将来 bump-semver 側でメッセージ文言が変わると kuu 側の判定が壊れる。

## 改善案候補 (当事者判断に委ねる、実装方針はここでは決めない)

### 案 1: no-result と subprocess/API エラーの exit code 分離

- 例: 該当なしは exit 0 + 空 stdout、または専用の exit code (例: exit 3 は subprocess エラー専用、該当なしは別番号)
- 既存の「exit 3 = VCS subprocess error」という意味論との整合性を要検討 (現状は「該当なし」も同じ 3 に同居している)

### 案 2: `--json` 等で機械可読な結果種別を返す口

- 例: `{"found": false, "reason": "no-semver-tags"}` vs `{"found": false, "reason": "subprocess-error", "detail": "..."}`
- 既存 `--json` 出力スキーマ (12-field version schema) との整合性を要検討

いずれの案も、既存の exit code 契約 (DR-0020 family) や `vcs:latest-tag()` / `vcs:latest-release()` input record 経由の呼び出し (`resolveLatestTag` 等) への影響範囲を洗い出してから採否判断が必要。

## 受け入れ条件

- [ ] 改善案の採否 (現状維持を含む) が決定される
- [ ] 採用する場合: exit code / `--json` スキーマの変更内容が確定し、実装・テスト・doc (help text 含む) が追従する
- [ ] 現状維持の場合: 利用側のメッセージ文字列 grep ワークアラウンドを正式な運用パターンとして doc化 (or 明示的に non-goal と記録)

## 一次資料

- `src/cmd_vcs_get_latest_tag.go` (L19-27 exit code doc comment)
- `src/cmd_vcs_get_latest_release.go` (L23-27 exit code doc comment)
- `src/vcs.go` L505 (`"no semver-compatible tags found"` エラー生成箇所)
- `src/cmd_vcs_get_latest.go` (`fetchLatestTag` / `fetchLatestRelease` / `ensureGhAvailable` — 該当なしと subprocess エラーが同じ error 型に合流する箇所)
- 利用側ワークアラウンド実例: kawaz/kuu の `.github/workflows/release.yml` L87-126 (bootstrap 判定コメント + grep ワークアラウンド)

## 注記

- 本 issue は kawaz/kuu プロジェクトでの利用時に気づいた点の上流還元 ([[dogfooding-feedback-upstream]] 運用)
- 部外者 (kuu 側) からの起票のため、具体的な実装方針 (exit code 番号 / JSON スキーマ) はここでは決めない。既存の exit code 契約・input record 経由呼び出しへの影響範囲は当事者 (bump-semver 側) が確認の上で判断すること
- 現状 (= grep ワークアラウンド) で機能はしているため緊急性は低い
