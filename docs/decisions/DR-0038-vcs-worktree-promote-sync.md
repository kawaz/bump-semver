# DR-0038: `vcs` に worktree / promote / sync を追加 — sync→promote→push の 3 段直交

- Status: Accepted (2026-06-22)
- Date: 2026-06-22
- Related: DR-0020 (vcs-subcommands の land 母体), `docs/issue/2026-06-18-vcs-worktree-promote-support.md`, claude-plugin-reference `docs/journal/2026-06-18-worktree-promote-and-marker-lockfile.md` (起票一次資料)

## Context

claude-plugin-reference の lockfile 化作業中に Claude Code の `EnterWorktree` (= background job では必須化) を使った結果、**worktree から main へ反映する手順が AI セッション依存で毎回再構築されている** ことが顕在化した:

1. `worktree.baseRef = fresh` (= 既定) で worktree が `origin/HEAD` ベースに作られ、親 workspace の最近 commit が見えない (= 後で rebase が必要)
2. worktree から main bookmark を進める手順 (`jj bookmark set main -r <change>`) が AI 知識依存
3. divergent (= 共通祖先から 2 line 分岐) の統合に `jj rebase -s <change> -d main` が必要だが AI 判断
4. justfile の `push` task 冒頭にゲートを置きたいが、`bump-semver vcs` に「現在 worktree か / default branch か」を判定する手段が無い

`vcs` には既に `get / is / diff / commit / fetch / push / tag / outdated` が揃っており、`push --jj-bookmark-auto-advance` で bookmark 自動進行も済んでいる。**worktree / promote 概念だけが欠けている** のがボトルネックだった。

## Decision

### サブコマンド 3 種を追加

| 追加 | 用途 | 既存パターン継承 |
|---|---|---|
| `vcs is worktree` / `vcs is on-default-branch` | predicate | `vcs is clean/dirty/git/jj` |
| `vcs get worktree-name` / `vcs get default-branch` | fact 読取 | `vcs get current-branch` |
| `vcs promote` | 副作用: default branch/bookmark を current commit に forward | 新規 verb |
| `vcs sync --onto REF` | 副作用: current worktree を REF に rebase | 新規 verb |

### `vcs promote` は push しない (sync → promote → push の 3 段直交)

`promote` と `push` をくっつけず、利用側で `sync → promote → push` の cascade を明示する設計を採る:

```just
push:
    @if bump-semver vcs is worktree; then
        echo "⚠ worktree ... に居ます。次を順に:"
        echo "  just sync && just promote && just push"
        exit 1
    fi
    bump-semver vcs push --branch main --jj-bookmark-auto-advance
```

3 段は独立した verb で、それぞれ単独で意味を持つ:

- `sync`: worktree を default に揃える (rebase)
- `promote`: default を current に forward (= bookmark/branch 移動だけ)
- `push`: remote へ反映

#### push しない理由

- **既存 `--jj-bookmark-auto-advance` との棲み分け**: 既存 flag は push 内 bookmark 移動。`promote` は push を伴わない bookmark 移動だけを切り出し、単独で再利用可能 (= release cascade 以外の場面、例えば mid-cascade で commit を追加してから push)。
- **副作用の最小化**: 1 verb = 1 副作用。push しない `promote` は失敗時の rollback も発生せず、CI gate でも単独テスト可。
- **既存 `push --jj-bookmark-auto-advance` は廃止しない**: bump cascade の典型ケースには 1 step で済む方が楽。`promote` は cascade 分解が必要な worktree 経路の補完。

### `vcs sync --onto REF` は `--onto` を必須化 (default 推論しない)

`origin/<default-branch>` を勝手に default にしない:

- 副作用 verb は **明示性 > 利便性**。`vcs sync` 単体は意味が曖昧、`vcs sync --onto origin/main` で初めて命令として完結。
- `default-branch` の取得は別 verb として独立 (`vcs get default-branch`) なので、利用側で `--onto $(bump-semver vcs get default-branch)@origin` のようにシェル合成できる (= 分離した primitive の組み合わせ)。
- default を入れると「`--onto` 抜きで動いてしまう」(= サイレントに `origin/main` を rebase 先にしてしまう) のが事故源。

### git の `Promote` は `git update-ref` + 手動 ancestor check (push 経由は使わない)

実装の初期試行で `git push . HEAD:refs/heads/<default>` を採用したが、`receive.denyCurrentBranch=refuse` (git のデフォルト) で **別 worktree が default branch を checkout していると reject** された。これは worktree promote の典型シナリオそのもので、機能が成立しない。

`git update-ref refs/heads/<default> <sha>` に切り替え、ancestor check (`git merge-base --is-ancestor <defSHA> <sha>`) を手動で行うことで forward-only を保証:

```go
// 1. resolveGitRev(rev) で current SHA 解決
// 2. defSHA を rev-parse --verify で取得 (存在しないなら create)
// 3. defSHA == sha なら no-op return
// 4. merge-base --is-ancestor で defSHA が sha の ancestor か確認 (forward only)
//    → 否なら exitErr{exitCodeNonFastForward}
// 5. update-ref defRef sha defSHA (= compare-and-swap で並行更新ガード)
```

`update-ref` は receive hook を経由しないため別 worktree の状態に影響されない。別 worktree の HEAD と ref が乖離する (= `git status` で "branch is ahead") のは正しい結末 — 別 worktree 側で `git pull --ff-only` するなり reset するなりするのは利用者の責任。

### jj の `Promote` は `jj bookmark set <default> -r @-` (forward only)

jj は `jj bookmark set` のデフォルト挙動が forward-only。`@-` (= 直前の non-empty change) を default に進める。

エラー検出: jj 0.42 の "Refusing to move bookmark backwards" メッセージ string match で `exitCodeNonFastForward` に分類。

### `DefaultBranch` の解決順序

1. `git symbolic-ref --short refs/remotes/origin/HEAD` (canonical answer set by `git clone` / `git remote set-head`)
2. Local branch probe: `main` → `master` → `trunk` (`git show-ref --verify`)

jj backend の `DefaultBranch` は git 経由しない (= secondary workspace cwd には `.git` view がなく `git` 呼び出しが失敗する)。代わりに:

1. `jj log -r trunk()` で revset alias の解決を試す (`bookmarks` template)
2. `jj bookmark list -T 'name'` から `main` / `master` / `trunk` を local 確認

git backend と jj backend で同じ resolution order を用意することで、利用者には backend 抽象として同じ結果を返す。

### `IsWorktree` / `WorktreeName` は file-kind / dir basename convention

#### git

`git rev-parse --git-common-dir` と `--git-dir` の比較で linked worktree を判定。linked なら `--git-dir` = `.git/worktrees/<name>` で別 path。

`WorktreeName` は linked worktree の root path basename (= `git worktree add <path>` の `<path>` 末尾)。git 自身に「worktree 名」概念は無いので convention (= dir basename を使う) に従う。

#### jj

`.jj/repo` の file kind で判定:

- default workspace: `.jj/repo/` は **ディレクトリ** (実体)
- secondary workspace: `.jj/repo` は **ファイル** (中身は default `.jj/repo` への相対 path)

`os.Stat` の `IsDir()` 1 回で判定でき、jj CLI の version 差異の影響を受けない。

`WorktreeName` は workspace root の dir basename。jj には「現在の workspace 名を返す API」が無く (`jj workspace list` は全 workspace、`jj workspace root` は path)、convention (= `jj workspace add <name>` の dir name = workspace name) を前提とする。jj-workflow.md の規約と整合。

## Consequences

### 利点

- worktree-aware な justfile push gate が `bump-semver vcs is worktree` ベースで書ける。利用側 (= 各 kawaz リポの justfile) からの dogfooding が即可能。
- `sync` / `promote` / `push` の 3 段直交により、CI / batch job / interactive いずれの場面でも組み立てやすい。
- backend 抽象を維持: git/jj 両方で同じ verb 表面 + 同じ semantic。利用者は `vcs.Kind()` を意識する必要がない。

### コスト

- backend interface に 6 method 追加 — 既存 `vcs_backend.go` の interface サイズが少し膨らむ。DR-0036 で「パッケージ分割は不採用、ファイル分割で対応」とした路線の延長で、依然許容範囲内 (合計 ~30 method)。
- `Promote` が backend で大きく違う実装 (git: update-ref + ancestor check vs jj: bookmark set) を持ち、共通抽出は浅い。これは VCS abstraction の本質的な non-DRY (= jj/git の forward-move の概念位置がそもそも違う) であり、無理に共通化しない方が正しい。
- `WorktreeName` は dir basename convention 依存。jj/git 双方とも CLI に「現在の worktree/workspace 名を返す API」がない以上、別の選択肢 (= 内部 metadata 解析 / jj template parsing) は version 依存が大きく、convention 採用が合理的。

## Not chosen

### `--push-after` flag を `promote` に持たせる

`vcs promote --push-after` で push まで一気に — を検討。**不採用**:

- 副作用が 2 つの verb は名前と挙動が乖離する (`promote` の名は bookmark/branch 移動を示唆、push まで含むなら別名)。
- 既存 `vcs push --jj-bookmark-auto-advance` が 1 step push の経路を担っており、`promote` を増強する利益が薄い。
- cascade のうち push だけ別途やりたい場面 (例: promote の結果を確認してから push) を素直に表現できる方が利益高い。

### `Sync` に `--onto` のデフォルト推論

`--onto` を省略時に `origin/<default-branch>` 推論を入れる案。**不採用** (上記 Decision の通り)。

### git Promote を `git push . <rev>:refs/heads/<default>` で実装

初期試行で採用したが `receive.denyCurrentBranch` で別 worktree が default checkout 時に reject されるため断念。`update-ref` 経路に切り替えた (上記 Decision の通り)。

### jj Promote の rev default に `@` を取る

`@-` ではなく `@` を default にする案。**不採用**: jj の慣習で `@` は throw-away working copy、`@-` が confirmed content。default を `@-` にする方が「`jj commit` 直後の典型状態 (= @ 空、@- が新規 commit)」と整合。利用者は必要なら `opts.Rev` で override 可能。

## Notes

- 実装: src/vcs_backend.go (interface 拡張 + 共通 helper), src/vcs_backend_jj.go, src/vcs_backend_git.go, src/vcs_cmd.go, src/cobra_vcs.go, src/cobra_help_text.go, src/cli_types.go, src/vcs_worktree_test.go
- 14 fixture tests (git linked worktree / jj secondary workspace の両方で IsWorktree / WorktreeName / DefaultBranch / IsOnDefaultBranch / Promote (FF + non-FF) / Sync を網羅)
- Release v0.40.0 で land
- 後続フォローアップ (本 DR の範囲外):
  - claude-rules-personal の `worktree-workflow-runbook-template.md` 起票 — `just push` ゲートを各 kawaz リポに展開する標準テンプレ
  - Claude Code `EnterWorktree` の `worktree.baseRef = head` 設定変更可否 (harness 側の話)
  - jj-worktree plugin への bookmark 自動セット提案
