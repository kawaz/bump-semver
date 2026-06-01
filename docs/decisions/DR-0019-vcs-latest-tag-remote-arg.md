# DR-0019: `vcs:latest-tag(<arg>)` で他リポの最新 tag を取れるよう拡張する

- Status: **Superseded by DR-0020 PR-Tag-Latest (2026-06-01)** — `vcs:latest-tag([REPO])` 関数入力は v0.29.0 で削除済。代替は `vcs tag latest` サブコマンド (`--source release` で GitHub Release も対象、`--raw` / `--json` / `--include-prerelease` 拡張)。`@`-peel fallback / 信頼境界 (本 DR の内容) は新コマンドに引き継がれている。
- Date: 2026-05-12

## Context

`vcs:latest-tag()` は当初、cwd の VCS から最大の semver-compatible tag を返す機能として導入された (DR-0008 の `vcs:` schema の一機能)。

しかし `kawaz/pkf-tasks` の `migrate:check-pkf-tasks-current` task (利用側 Taskfile.pkl の `pkf-tasks@<version>` import が最新 release より古いか検知する gate) では、**他リポの最新 tag** を参照したい需要が出てきた。pkf-tasks 側で `git ls-remote --tags <url>` を bash pipeline で組み立てる繋ぎ実装 (v0.0.11) を入れたが、VCS-aware ref schema の解釈は bump-semver の責務として吸収するのが筋。

## Decision

`vcs:latest-tag([<arg>])` 形式に拡張する:

```
vcs:latest-tag()                              # 既存: cwd の VCS
vcs:latest-tag(kawaz/pkf-tasks)               # 新規: GitHub 短縮 (owner/repo)
vcs:latest-tag(https://github.com/kawaz/x)    # 新規: フル HTTPS URL
vcs:latest-tag(git@github.com:kawaz/x)        # 新規: SSH URL
vcs:latest-tag(ssh://git@github.com/kawaz/x)  # 新規: SSH URL (scheme form)
```

引数が空 (空白のみ含む) なら従来通り cwd VCS、非空なら remote として `git ls-remote --tags <url>` を実行。

実装:
- `expandRepoArg(arg) -> url`: `owner/repo` を `https://github.com/<owner>/<repo>` に展開、HTTPS/SSH はそのまま、その他は pass-through (`git ls-remote` 側がエラー報告)
- `vcsListTagsRemote(url) -> tags`: `git ls-remote --tags <url>` の出力から `refs/tags/` prefix と annotated tag の `^{}` peel-commit suffix を剥がして tag 名一覧を返す。jj は ls-remote 相当機能を持たないため remote は git で統一
- `vcsLatestTag(vcs, remoteURL)`: `remoteURL != ""` で remote 分岐、空なら従来通り cwd

加えて、SemVer 認識の **`@` peel fallback** を `vcsLatestTag` 内に導入:

```go
v, err := ParseVersion(t)
if err != nil {
    // Fallback: monorepo-style `<name>@<version>` (e.g. `pkf-tasks@0.0.11`)
    if i := strings.LastIndex(t, "@"); i >= 0 {
        v, err = ParseVersion(t[i+1:])
    }
    if err != nil { continue }
}
```

これにより `pkf-tasks@0.0.12` のような monorepo-style tag が semver として認識される。`v0.15.0` の従来 `v` prefix fallback (ParseVersion 内) と並存。

## Rationale

### 不採用案

**1. `vcs:latest-tag:<repo>` の `:` 区切り**

引数受け取りを `:` 区切りで表現する案 (`vcs:latest-tag:kawaz/pkf-tasks` のように)。**不採用**: 既存の `vcs:REV[:FILE]` schema は REV / FILE に `:` を含む可能性があり (jj の revset 等)、`:` を引数区切りとして再利用すると将来の構文衝突リスクがある。`()` 関数記法は明示的に区切られて誤認しない (DR-0008 の `()` 採用と同じ判断)。

**2. 引数を `"..."` でクオートする仕様**

`vcs:latest-tag("kawaz/pkf-tasks")` のように内部ダブルクオートを必須にする案。**不採用**: Pkl の `Task.cmd` を配列形式で書くとき (`["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]`) に内部 quote を含めると JSON/Pkl のエスケープが入って `"vcs:latest-tag(\"kawaz/pkf-tasks\")"` になり可読性が著しく落ちる。markdown link `[text](url)` 記法のように **`()` の中身を raw string として扱う** のが ergonomic。利用者が shell で叩く時の subshell 回避は外側の `'...'` (single quote) で済む。

**3. owner/repo 形式を採用しない**

フル URL のみ受け付ける案。**不採用**: kawaz の運用想定では GitHub に集中しており、`kawaz/pkf-tasks` の typing 短縮は実用価値が高い。owner/repo は GitHub convention (gh CLI 等で広く使われる短縮形)、誤認の余地は小さい。

**4. `@` peel fallback を入れない**

`pkf-tasks@0.0.12` のような monorepo-style tag を semver として認識しない案 (利用者が `:filter` パラメータ等で明示的に渡す)。**不採用**: monorepo の慣習 (Pkl package / npm scoped / Go module subpath 等) で `<name>@<version>` 形式が広く使われており、tag list 経由でだけ使う限り jj `main@origin` revset との衝突はない (revset は tag-list で取得されない)。silently 認識する方が ergonomic。

### 設計上のポイント

#### remote 取得は git で統一

`vcsListTagsRemote` は `--vcs jj|git` の選択に関係なく **常に git** で `ls-remote` を実行。jj は remote tag を一覧する独自コマンドを持たない (`jj git fetch` 後の op log を見るしかなく、ローカルへの副作用が出る)。bump-semver の「no implicit network calls / side effects」原則 (DR-0008) を尊重しつつ remote 機能を出すには git ls-remote しか選択肢がない。

#### 信頼境界

remote URL の正当性は **呼び出し側 (利用者) 責任**。`vcs:latest-tag(<url>)` の `<url>` を第三者書き込み可能な repo に向けると、悪意ある `malicious@99.99.99` tag を push されて「最大の semver tag」として返される攻撃が成立する (`@` peel fallback でも引っかかる)。bump-semver 側で防げる範囲外の信頼境界:

- `bump-semver` の挙動: 与えられた URL の最大 semver tag を返す (これは仕様通り)
- 防御責任: URL の信頼性を呼び出し側が担保する。kawaz の運用では `kawaz/pkf-tasks` 等の自リポを default 固定で、利用者が書き換えない限り別 repo を見ない設計 (pkf-tasks `migrate:check-pkf-tasks-current` の `remoteRepoSpec` default 等)

README / `--help` に「remote URL の信頼性は呼び出し側責任」を明記して利用者に注意喚起。

#### `@` peel fallback の射程

`vcsLatestTag` 内のみで適用。`ParseVersion` 本体は変更せず (`v0.15.0` の従来挙動を維持)。tag-list 経由で来た tag だけ peel 対象。`bump-semver get 0.0.11` のような VER 入力には影響しない。

## Consequences

- 利用例:
  - `bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'` → `0.0.12`
  - `bump-semver get 'vcs:latest-tag()'` → cwd の最大 semver tag
  - `bump-semver compare ge 0.0.12 'vcs:latest-tag(kawaz/pkf-tasks)'` → exit 0 (0.0.12 >= 0.0.12)
- pkf-tasks v0.0.12 で `migrate:check-pkf-tasks-current` / `migrate:update-pkf-tasks` の実装を本機能経由に置換 (繋ぎの `git ls-remote` bash pipeline を撤去)
- pkfire の Pkl Task.cmd 配列形式で書く時に内部 quote 不要 (markdown link 感覚)、ergonomic を確保
- 新規依存はなし (既存 `runVcs("git", ...)` 関数を流用)

## 関連

- 実装: `src/vcs.go` の `expandRepoArg` / `vcsListTagsRemote` / `vcsLatestTag` signature 変更、`vcsLatestTag` 内 `@` peel fallback
- テスト: `src/vcs_test.go` の `TestVcsParseSpec` (新規 cases) + `TestExpandRepoArg` (新規)
- 上位 DR: DR-0008 (`vcs:` schema 導入)
- 利用側: `kawaz/pkf-tasks` v0.0.12 の `tasks/migrate/check-current.pkl` / `update-self.pkl`、v0.0.11 までの繋ぎ実装と置換 (`kawaz/pkf-tasks/docs/journal/2026-05-12-pkf-tasks-v0.0.8-semver-compare-experiment.md` 系の経緯)
- 議論経緯: `docs/journal/2026-05-12-vcs-latest-tag-remote-arg.md`
