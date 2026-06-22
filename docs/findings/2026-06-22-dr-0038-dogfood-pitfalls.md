# DR-0038 (vcs worktree/promote/sync) dogfood で踏んだ罠

- Date: 2026-06-22
- Related: [DR-0038](../decisions/DR-0038-vcs-worktree-promote-sync.md), [CHANGELOG v0.40.0 / v0.40.1](../../CHANGELOG.md)

## 判明した事実

### 1. `vcs is worktree` ベースの justfile gate は kawaz の jj 運用と合わない

- kawaz の git bare + jj workspace 方式では default workspace は repo root 直下 (`.jj/` と同階層)、`main/` は `jj workspace add` で作った **secondary workspace**。
- `IsWorktree()` は実装通り「secondary workspace = true」を返す (= 設計正しい、`.jj/repo` の file kind で判定)。
- そのため `if bump-semver vcs is worktree` ベースの push gate は **main workspace からの push も block** する誤検出に。
- 正しい predicate は **`vcs is on-default-branch` の反転** (= 「default branch 上にいなければ警告」)。これは `is worktree` (= 場所) と `is on-default-branch` (= bookmark/branch) の question 差を直接表現している。
- `vcs is worktree` 自体は git linked-worktree シナリオで意義があるので削除しない。push gate には不向き、というだけ。

### 2. `vcs get current-branch` ambiguous で subshell が exit 4 → gate 自身を殺す

```just
[script]
check-on-default-branch:
    if ! bump-semver vcs is on-default-branch; then
        cur=$(bump-semver vcs get current-branch)   # ← ambiguous 時 exit 4
        bn=$(bump-semver vcs get default-branch)
        printf >&2 "..."
        exit 1
    fi
```

- 「`@` の ancestor に bookmark が無い divergent な枝」では `vcs get current-branch` が `exitCodeAmbiguous (4)` を返す (= 設計通り、「現在の branch を答えられない」)。
- bash の `set -euo pipefail` 下では subshell `$(...)` の exit 非ゼロが recipe 全体を殺し、hint メッセージを出す前に exit 4 で死ぬ。
- 回避策: `cur=$(bump-semver vcs get current-branch 2>/dev/null || echo "(ambiguous)")` でフォールバック。
- 4 リポ統一の justfile gate にこのフォールバックを組込み済 (v0.40.1)。
- 上流側 (`bump-semver`) の改善候補: `vcs get current-branch --fallback=VALUE` flag や `vcs is on-default-branch --verbose` で current 値も併せて返す等。設計判断要 (= 別 issue で起票予定)。

### 3. `git push . <rev>:refs/heads/<default>` は `receive.denyCurrentBranch=refuse` で reject される

- 初期試行で採用した `git push . HEAD:refs/heads/main` は、別 worktree が main branch を checkout している (= まさに worktree promote の典型シナリオ) と **git のデフォルト設定で reject** される:
  ```
  remote: error: refusing to update checked out branch: refs/heads/main
  ```
- `receive.denyCurrentBranch` のデフォルト `refuse` がこの動作を起こす。`updateInstead` に変えれば動くが、worktree 側の index/workdir も更新してしまい意図と乖離。
- 解決: `git push` 経路を捨て、`git update-ref` + 手動 ancestor check (`git merge-base --is-ancestor`) に切替。`update-ref` は receive hook を経由しないため別 worktree の状態に影響されない。別 worktree の HEAD と ref が乖離する (= `git status` で "branch is ahead") のは正しい結末。

### 4. v0.40.0 の `vcs promote` に bareVerb 短絡 bug が混入していた

- `cobra_vcs.go` の `newVcsPromoteCmd` の `RunE` 冒頭で `if bareVerb(cmd, posArgs) { return cmd.Help() }` を入れていた。
- `vcs promote` は引数も flag も持たない設計 (= bare 実行が唯一の正常動作) なので、help 短絡が全 promote 呼び出しを潰していた。
- v0.40.1 で `bareVerb` 短絡を削除して fix (= stray positional args は dispatcher が exit 2 で reject、usage 安全性は維持)。
- 教訓: bareVerb 短絡は **「引数必須」な verb にだけ** 入れる。「引数も flag も無いのが正常」な verb には絶対入れない。

### 5. jj の「現在の workspace 名」を返す CLI API が不在

- `jj workspace list` は全 workspace 名 (= 現在のは特定できない)、`jj workspace root` は path のみ。template も「現在 workspace」用の field は無い (jj 0.42 で確認)。
- 回避策: workspace root の dir basename を workspace 名とみなす (= `jj workspace add <name>` で `<name>/` が作られる convention に依存)。kawaz の jj-workflow rule にこの convention が明記されているので運用上問題なし。
- `WorktreeName` の実装はこの convention に基づく (= DR-0038 にも明記済)。

## 実用的な示唆

### 各リポの justfile push gate テンプレ (= 4 リポで採用済)

```just
[private]
[script]
check-on-default-branch:
    if ! bump-semver vcs is on-default-branch; then
        cur=$(bump-semver vcs get current-branch 2>/dev/null || echo "(ambiguous)")
        bn=$(bump-semver vcs get default-branch)
        printf >&2 "⚠ 現在 '%s' bookmark/branch にいます。%s に合流してから push してください\n  1. bump-semver vcs sync --onto %s@origin\n  2. bump-semver vcs promote\n  3. %s ワークスペースに移動して just push\n" "$cur" "$bn" "$bn" "$bn"
        exit 1
    fi

push: check-on-default-branch ...
```

### dogfood の段取り

新規 verb を land した直後は **release 当日に実利用** で踏ませる。「実装テスト全部 green」では拾えない罠 (= 1, 2, 3, 4 は全て release 後の実利用で初めて顕在化) が多い。

## 検証の詳細

| 罠 | 発見タイミング | 修正 |
|---|---|---|
| `vcs is worktree` 誤検出 | v0.40.0 main workspace で `just push` → check-not-worktree fire | v0.40.1 で gate を `is on-default-branch` に切替、DR-0038 Adoption pattern 節追記 |
| `current-branch` ambiguous の subshell 死 | claude-rules-personal の divergent 枝で gate test | 4 リポの justfile に `2>/dev/null || echo "(ambiguous)"` フォールバック追加 |
| `git push .` denyCurrentBranch | v0.40.0 開発中の test 段階 | `update-ref` + ancestor check に切替 (DR-0038 git Promote 節に記録済) |
| `promote` bareVerb 短絡 | v0.40.0 dogfood の `bump-semver vcs promote` 呼び出し | v0.40.1 で削除 |
| jj workspace 名取得 API 不在 | 実装段階の調査 | dir basename convention 採用 (DR-0038 既述) |
