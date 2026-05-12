# 2026-05-12 — `vcs:latest-tag(<arg>)` 実装 (v0.15.0)

## 動機

`kawaz/pkf-tasks` の `migrate:check-pkf-tasks-current` task が「利用側 Taskfile.pkl の `pkf-tasks@<version>` import が最新 release より古いか」を判定するために、**他リポの最新 tag を取得** したかった。pkf-tasks 側で `git ls-remote` の bash ベタ書きで繋ぎ実装していたが、VCS-aware ref schema の解釈は bump-semver の責務なので統合した。

`docs/issue/2026-05-11-vcs-latest-tag-remote-repo.md` で起票していた feature を v0.15.0 として実装。issue は解決済として削除 (この journal で経緯を追える)。

## 実装

### スキーマ拡張

```
vcs:latest-tag()                      # 既存: cwd の VCS (互換維持)
vcs:latest-tag(kawaz/pkf-tasks)       # 新規: GitHub short (owner/repo)
vcs:latest-tag(https://...)           # 新規: フル URL
vcs:latest-tag(git@github.com:x/y)    # 新規: SSH URL
```

引数は raw string で渡す (内部ダブルクオート不要 = markdown link `[]()` と同じ感覚)。Pkl の `Task.cmd` 配列形式 (`["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]`) で書く時の二重エスケープ地獄を回避する設計判断。

### 主要変更 (`src/vcs.go`)

1. **`expandRepoArg(arg string) string`** (新規):
   - HTTPS / SSH / `ssh://` はそのまま
   - `owner/repo` (slash 1 つ + no whitespace) → `https://github.com/<owner>/<repo>` に展開
   - 空文字列 → 空文字列 (cwd VCS の signal)
   - whitespace trim 込み

2. **`vcsListTagsRemote(url string) ([]string, error)`** (新規):
   - `git ls-remote --tags <url>` で remote refs 取得
   - `refs/tags/` prefix 除去、annotated tag の `^{}` peel commit suffix 除去
   - jj は ls-remote 相当機能を持たないため git で統一

3. **`vcsLatestTag(vcs vcsKind, remoteURL string)`** (signature 変更):
   - `remoteURL != ""` なら `vcsListTagsRemote`、空なら従来通り `vcsListTags`
   - 既存 caller は `""` を渡す

4. **`resolveVcsFunc`** の `latest-tag` 分岐:
   - `args` (parser が `rev` フィールドに格納している) を `expandRepoArg` に通して remote URL に解決
   - empty arg = cwd 動作で後方互換

5. **`vcsLatestTag` 内の SemVer parse fallback**:
   - `ParseVersion(tag)` が失敗した場合、最後の `@` 以降を再 parse (`pkf-tasks@0.0.11` → `0.0.11` を semver として認識)
   - monorepo-style tag (Pkl package, npm scoped, Go module subpath 等の `<name>@<version>` 慣習) に対応
   - tag list 経由のみ。jj 流儀の `main@origin` revset とは衝突しない (revset は tag-list 経由で来ない)

## 動作確認

```
$ bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'
0.0.11

$ bump-semver get 'vcs:latest-tag(https://github.com/kawaz/pkf-tasks)'
0.0.11

$ bump-semver get 'vcs:latest-tag()'   # cwd (bump-semver 自身)
v0.14.2
```

## v0.15.0 後の pkf-tasks 側更新 (将来作業)

pkf-tasks v0.0.12+ で `tasks/migrate/check-current.pkl` / `update-self.pkl` の `git ls-remote` 実装を `bump-semver get vcs:latest-tag(kawaz/pkf-tasks)` 呼び出しに置換予定。これにより:

- VCS knowledge の集約 (DR-0006 pkf-tasks 側) と整合
- pkf-tasks 側の bash 重複コード削減
- bump-semver の SemVer parse logic を再利用 (peel fallback 含む)

## 学び

### TDD 流儀

`vcs_test.go` の既存 `TestVcsParseSpec` cases に `function-with-owner-repo` / `function-with-https-url` を追加 (parser は既存実装で args を `rev` に格納する設計だったので test pass)。新規 `TestExpandRepoArg` で URL 展開ロジック 8 ケース確認。

### `@` peel fallback の射程

`<name>@<version>` 形式は Pkl package (`pkf-tasks@0.0.11`) / npm scoped (`@scope/pkg@1.0.0`) / Go module subpath (`v2/cmd/foo@v2.1.0`) など複数 ecosystem で慣習化されている。tag list 経由でだけ有効にする限り、jj `main@origin` revset との衝突はない (revset 経由で取得されない)。

### release.yml の自動 trigger

bump-semver は VERSION ファイル変更を `on: push` の paths で検出して release.yml を自動 trigger する設計。CHANGELOG.md は持たず、journal / release commit message で経緯を追う流儀。

## 関連

- 旧 issue: `docs/issue/2026-05-11-vcs-latest-tag-remote-repo.md` (本 release で解決、delete 済)
- pkf-tasks 側の繋ぎ実装: `kawaz/pkf-tasks` v0.0.11 の `tasks/migrate/check-current.pkl` / `update-self.pkl` (git ls-remote ベース)
- 設計議論: kawaz/pkf-tasks DR-0006 (vcs/* を VCS knowledge 集積場として位置付け) と整合
