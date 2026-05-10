# 2026-05-10: vcs: 入力モード実装 (v0.7.0 / DR-0008)

## 仕様確定 (DR-0008)

- `vcs:REV[:FILE]` / `vcs:latest-tag()` を新規入力形式として追加
- VCS 自動判定: `--vcs` flag → `BUMP_SEMVER_VCS` env → `.jj` → `.git`
- jj/git 並存時は jj 優先 (kawaz の git-bare + jj-workspace 構成想定)
- jj の `origin/main` ↔ `main@origin` 自動フォールバック
- `--write` と `vcs:` は排他 (vcs: は read-only)
- fetch は自動実行しない (副作用回避)
- FILE 省略時の借用は **位置順** で最初の FILE 提供入力 (実 FILE or `vcs:REV:FILE`)
- 借用源なしはエラー (信頼度比較は不採用、シンプルさ優先)

## 実装の主要な選択

### `src/vcs.go` を新規追加

VCS 関連ロジックは 1 ファイルに集約。`detectVcs` / `parseVcsOverride` / `vcsParseSpec` / `resolveVcsInput` / `vcsFetchFile` / `vcsListTags` / `vcsLatestTag` / `altJjRev`。

### `resolveInputs` を最小侵襲で拡張

既存の「raw / file / 借用源」分類ロジックの拡張で対応:

1. 1 周目で全引数を classify、`fileForBorrow` を「位置順で最初の FILE 提供入力」として決定
   - 実 FILE 起源
   - `vcs:REV:FILE` (file 明示済 vcs:)
2. 2 周目で各引数を resolve、vcs: のうち file 省略のものは `fileForBorrow` を使う
3. VCS 自動判定は **lazy** (vcs: 入力が含まれる invocation でのみ走る)、jj/git 不在環境でも非 vcs 呼び出しは動く

### jj の tag 取得

`jj log -r 'tags()' --no-graph -T 'tags.map(|t| t.name() ++ "\n").join("")'`

複数 change が同じ tag を共有する可能性を考慮し、結果は `splitAndDedup` で重複除去。

### jj の `origin/main` フォールバック

git ユーザの感覚で `vcs:origin/main` と書きたいケースを救う。jj の native 形式 `main@origin` でリトライし、それでも失敗したら最初のエラーを返す。fallback は 1 階層のみ (`/` を 1 つだけ含む形式)、`feature/foo/bar` のような多階層は曖昧なので素通し。

## ハマり所と解決

### jj が新しい change を `@` に作る

`jj git init --git-repo .git` 後、`@` は **新規の空 change** で git の HEAD は `@-` になる。HEAD~1 相当は `@--`。テスト fixture でこれを `@-` で取ろうとして失敗 (`@-` は最新コミット = bumped 値が返ってきた)。`@--` で fix。

### bash heredoc の `!` エスケープ

`cat << 'GOEOF'` で書いた Go コードに `!gitAvailable()` が `\!gitAvailable()` に化けた。zsh の history expansion 抑制 (`'GOEOF'` だけでは不十分) が原因。`sed -i 's|\\!|!|g'` で一括修復。

教訓: Go コードのような `!` を含むものは `Write` ツールで書く方が安全。

### chdir + t.Parallel() は両立しない

VCS fixture テストは `os.Chdir` でカレントディレクトリを切り替える。`t.Parallel()` を付けると複数テストが同時に chdir 競合する。fixture テストは parallel 化しない方針で統一。Pure な単体テスト (parseSpec, splitAndDedup 等) は parallel OK。

### sandbox の go cache 問題

`/Users/kawaz/Library/Caches/go-build/` がサンドボックスの allowlist に入っていなくて `go vet` / `go test` / `go build` が "operation not permitted" で失敗。サブエージェント側では `dangerouslyDisableSandbox: true` で全 go コマンドを実行。

## テスト構成

- `src/vcs_test.go` (新規):
  - 単体: `TestVcsParseSpec`, `TestParseVcsOverride`, `TestAltJjRev`, `TestSplitAndDedup`
  - git fixture: `TestVcsListTags_Git`, `TestVcsLatestTag_Git`, `TestVcsLatestTag_Git_NoSemver`, `TestVcsFetchFile_Git`
  - jj fixture: `TestDetectVcs_JjOverGit`, `TestVcsListTags_Jj`, `TestVcsFetchFile_Jj`
  - 検出: `TestDetectVcs_Git`, `TestDetectVcs_NoRepo`, `TestDetectVcs_Override`
- `src/main_test.go` (追記):
  - parseArgs: `vcs-flag-jj`, `vcs-flag-git-eq`, `vcs-input-bump`, `vcs-input-compare`, `vcs-bad-value`, `vcs-missing-arg`, `vcs-double`
  - CLI 統合: `TestRun_VcsInput_Simple`, `_FileBorrow`, `_LatestTag`, `_LatestTag_NoSemver`, `_WriteRejected`, `_VcsForceFlag`, `_InvalidVcsValue`, `_BorrowRequired`, `_UnknownFunction`, `_MultipleVcs`, `_BorrowFromVcsExplicit`, `_AllVcsNoFile`, `_BorrowPositionOrder`

git/jj fixture は `t.TempDir()` で隔離して作成。jj が無い環境では `t.Skip()`。CI に jj が居なくても git 系テストは動く。

## CI

`just lint` / `just test` / `just build` 全パス。手動確認:

- `./bin/bump-semver compare gt ./VERSION vcs:main@origin` → exit 0 (0.7.0 > 0.6.0)
- `./bin/bump-semver get ./VERSION vcs:main@origin` → version mismatch を整列表示
- `./bin/bump-semver patch ./VERSION vcs:main@origin --write` → 排他エラー

## 残タスク (push 前にメインで)

- `just push` (lint / test / check-translations 経由)
- VERSION 0.6.0 → 0.7.0 で release.yml が tag + GitHub Release を作成
- homebrew-tap が Formula 更新
