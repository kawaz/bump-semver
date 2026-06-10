# DR-0034: 外部コマンド引数インジェクション対策と入力検証ポリシー (C-1)

- Status: Active
- Date: 2026-06-10
- Related: DR-0019 / DR-0032 (expandRepoArg の URL 設計)、DR-0031 (translateRev、検証と翻訳の責務分離)

## Context

`bump-semver vcs` 系サブコマンドはユーザ由来の rev / tag NAME / remote / repository を
`exec.Command("git", ...)` / `exec.Command("jj", ...)` / `exec.Command("gh", ...)` の
argv に渡す。`exec.Command` は `sh -c` を経由しないためシェルメタ文字は無害だが、
**値が `-` で始まる場合に git/jj/gh がフラグとして解釈する** という問題が残る。

実害として確認された攻撃シナリオ:

```
bump-semver vcs diff -- --output=<path>
```

CLI パーサは `--` でオプション解析を終了させ、直後の `--output=<path>` を rev 引数
として受け取る。これが `git diff --output=<path>` に到達し、任意ファイルへの書き込みが
成立していた (exit 2 で reject し、ファイル非生成を実機で確認済み)。

同様のリスクが rev 以外の入力点にも存在した:

- tag NAME (`git tag -d <name>` で `-d` が削除フラグになる)
- remote 名 (`git fetch --upload-pack=<cmd>` 等のフラグ注入)
- gh repo 引数 (`gh -R <owner/repo>` 形式必須)
- `vcs:REV:FILE` 入力モード (CLI パーサを経由せず rev が直接 backend に渡る)
- `expandRepoArg` の戻り値 (`git ls-remote --tags <url>` の `<url>` 引数)

## Decision

### 検証の配置ポリシー: 入口に集約、backend は薄いまま

検証は **CLI ディスパッチ層と入力モード resolver の入口** に集約し、
backend (git/jj/gh の実行関数) は検証済みの値を受け取るだけとする。

- `vcs diff`: `validateUserRev` を dispatch 入口 (vcs_cmd.go) で実行
- `vcs get commit-id`: 同上
- `vcs tag push`: `validateUserRev` + `validateRemote` を dispatch 入口で実行
- `vcs tag delete`: `validateRemote` を dispatch 入口で実行
- `vcs fetch` / `vcs push`: `validateRemote` を dispatch 入口で実行
- `vcs get latest-tag`: `expandRepoArg` を (string, error) シグネチャに変更し `-` 始まりを reject
- `vcs get latest-release`: `validateGhRepo` を dispatch 入口で実行、`fetchLatestRelease` 内部にも defense-in-depth
- `vcs:REV` 入力モード: `resolveVcsInput` 内で `validateUserRev` を実行 (パーサ非経由経路の漏れ対策)
- `validTagName` (既存): `-` 始まり NAME を追加 reject

### 新規バリデータ

| 関数 | 対象 | 拒否条件 |
|---|---|---|
| `validateUserRev` (vcs.go:371) | rev 引数 | 非空かつ `-` 始まり |
| `validateRemote` (vcs.go:383) | remote 名 | 空 / `-` 始まり / 空白含み |
| `validateGhRepo` (vcs.go:402) | gh -R 用 repo | 非空かつ `owner/repo` 形式でない、`-` 始まり、空白含み |
| `expandRepoArg` (vcs.go:438) | vcs:latest-tag の arg | `-` 始まり (戻り値を `(string, error)` に変更) |

`validTagName` (vcs_cmd.go:690) は既存バリデータに `-` 始まり拒否を追加。

### 許可する値の範囲

```
# validateUserRev が通過させる正当な rev 例
HEAD, HEAD~3, @-, main@origin, origin/main, v1.2.3, abc1234, feature/x
```

空文字列は「引数省略」として各呼び出し元が別途処理するため validateUserRev は
スキップ (二重エラー回避)。

### 全受容点の網羅確認

`backend.CommitID` / `backend.FetchFile` / `backend.Diff` / `backend.DiffNameStatus` /
`backend.TagPush` の実装にある全 rev 受容点を grep で確認し、いずれも上記検証を
通過した後の値のみが到達することを確認した。

### 不採用: URL スキーム allowlist の全面導入

`expandRepoArg` に `http://` / `https://` / `git@` / `ssh://` の allowlist を設け、
それ以外の値を全拒否する案を検討したが **採用しない**。

理由: DR-0019 / DR-0032 が「URL の正当性は呼び出し側責任、`git ls-remote` のパース
エラーがより正確」と明示的に設計判断している。allowlist を導入するとこの設計を覆し、
正当な非 GitHub リモート (GitLab の SSH URL 等) を誤って拒否するリスクが生まれる。
今回の修正目的は `-` 始まりのフラグ注入だけであり、最小限の guard に留めることで
既存 URL 設計との整合を維持する。

### 不採用: backend 層での二重チェック

backend 関数 (runGitCmd / runJjCmd 等) が受け取った引数を再検査する案は、
backend の引数は get-commit-id / fetch / diff 等の subcommand も含み、
どれが「ユーザ由来」かを backend 層で判定するのが困難。また責務が二重になりメンテナンスコストが増す。入口集約 (CLI ディスパッチ + 入力モード resolver) のみで十分。

### translateRev との関係 (DR-0031)

検証は `translateRev` (DR-0031) の**前**に実行する。translateRev はスラッシュ / `@` の
付け替えのみ行い、先頭文字を変更しない (`-` を生成しない) ため、生の user input に
対して一度だけ検証すれば十分。二つの責務 (validate vs translate) を混在させない。

## Consequences

- `-` 始まりの rev / remote / NAME / repository を渡した場合、exit 2 で即拒否
- 正当な rev (`HEAD`, `@-`, `main@origin`, SHA 等) に影響なし
- `vcs:REV:FILE` 入力モードも同じポリシーで保護 (パーサ bypass 経路を閉鎖)
- `expandRepoArg` の呼び出し元 (`cmd_vcs_get_latest.go` 等) はエラーハンドリングが必要になった (シグネチャ変更)
- テスト: `src/vcs_inject_test.go` 新規 (unit + 実害回帰 + gh stub で backend 非到達確認 + 入力モード経路)

## 関連

- 実装: `src/vcs.go` (validateUserRev / validateRemote / validateGhRepo / expandRepoArg)、`src/vcs_cmd.go` (validTagName 拡張 + 各 dispatch 入口)、`src/cmd_vcs_get_latest.go` / `src/cmd_vcs_get_latest_tag.go` / `src/cmd_vcs_get_latest_release.go`
- テスト: `src/vcs_inject_test.go`、`src/vcs_test.go` (TestExpandRepoArg を新シグネチャに追従)
- 設計継承: DR-0019 / DR-0032 (URL 設計の信頼境界)、DR-0031 (translateRev との責務分離)
