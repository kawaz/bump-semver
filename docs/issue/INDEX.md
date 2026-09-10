# Issue INDEX

active な issue の一覧。close 済みは archive/ にあり、ここには載せない。

| date | category | status | slug | 概要 |
|---|---|---|---|---|
| 2026-09-10 | task | open | [ecosystem-review-2026-09](./2026-09-10-ecosystem-review-2026-09.md) | エコシステム外部レビュー (2026-09) の指摘への対応検討 |
| 2026-09-10 | bug | open | [check-version-bumped-fails-when-origin-lacks-version-file](./2026-09-10-check-version-bumped-fails-when-origin-lacks-version-file.md) | check-version-bumped が origin に version file が無い初回 push で常に失敗する |
| 2026-07-29 | bug | open | [justfile-on-success-release-list-help-broken](./2026-07-29-justfile-on-success-release-list-help-broken.md) | justfile の on-success-release の --list 説明が崩れている (llm-gateway からのフラグ、[doc()] 対処例あり) |
| 2026-07-28 | bug | open | [check-on-default-branch-gate-false-no-hint](./2026-07-28-check-on-default-branch-gate-false-no-hint.md) | check-on-default-branch の gate false 時に hint が出ない (DR-0038 未追従疑い) |
| 2026-07-15 | request | open | [vcs-latest-noresult-vs-error-exitcode](./2026-07-15-vcs-latest-noresult-vs-error-exitcode.md) | vcs get latest-release / latest-tag で「該当なし」と subprocess エラーが exit code で区別できない |
| 2026-06-22 | request | open | [vcs-get-current-branch-ambiguous-fallback](./2026-06-22-vcs-get-current-branch-ambiguous-fallback.md) | vcs get current-branch ambiguous の subshell 罠を library 側で吸収できないか |
| 2026-06-22 | task | open | [vcs-sync-matrix-verification](./2026-06-22-vcs-sync-matrix-verification.md) | vcs sync の動作マトリクス検証 (= 既に sync 済 / divergent / conflict 各ケース) |

<!--
雛形メモ (migrate sub-command 用):

- 列構成は固定 (= 上記 5 列、列名と順序を変えない)
- 行の {{rows}} は migrate が走査後の active issue から生成 (= 全件再生成)
- ソート規約:
  1. status 優先順: idea → open → wip → blocked → pending-sublimation
  2. 同 status 内は date 降順 (= 新しい起票が上)
- 各行: `| YYYY-MM-DD | <category> | <status> | [<slug>](./YYYY-MM-DD-<slug>.md) | <本文 1 行目から 80 文字以内> |`
- 概要は 80 文字を超えたら末尾を「…」で省略
-->
