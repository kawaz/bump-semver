# `vcs:latest-tag(<repo>)` で他リポの最新 tag を参照可能にする

- Date: 2026-05-11
- Type: Feature
- Priority: Mid (pkf-tasks v0.0.12+ で利用予定)

## 動機

`kawaz/pkf-tasks` で `migrate:check-pkf-tasks-current` という gate task を作る予定 (利用側 Taskfile.pkl の `pkf-tasks@<version>` import が最新 release より古いと push 時に fail させる)。

このとき「kawaz/pkf-tasks の最新 release tag」を取得する必要があるが、現在の `bump-semver` の `vcs:latest-tag()` は **cwd の VCS** しか見ない。利用者の Taskfile.pkl から実行する文脈では cwd = 利用者リポなので、対象外。

繋ぎとして pkf-tasks 側に `git ls-remote --tags <url>` の Pkl helper を入れて先に動かす方針 (v0.0.11)、ただし VCS-aware ref schema の解釈は本来 bump-semver の責務なので、本機能として bump-semver に統合したい。

## スキーマ案

bump-semver の既存 `vcs:` schema 設計判断 (`:` 区切りは REV / FILE / tag 名に `:` を含む可能性とコンフリクトするため、引数取りには **関数記法 `()` を採用** している) を踏襲する。本機能も同流儀:

```
vcs:latest-tag()                       # 既存: cwd の VCS、互換維持
vcs:latest-tag(<owner>/<repo>)         # 新規: GitHub 短縮形式
vcs:latest-tag(https://...)            # 新規: フル URL
vcs:latest-tag(git@github.com:...)     # 新規: SSH URL (任意)
```

CLI quote: shell の `()` が subshell 記法のため引数渡し時は quote 必須 (`bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'`)。これは現状の `vcs:latest-tag()` と同じ取り扱い、help / docs にも明記済の前提。

利用例:

```bash
bump-semver get vcs:latest-tag(kawaz/pkf-tasks)
# => pkf-tasks@0.0.9 (or version 部分のみ?)

bump-semver compare gt VERSION vcs:latest-tag(kawaz/bump-semver)
# => 現リポの VERSION が kawaz/bump-semver の最新 release より大きいか

# pkf-tasks 用の利用例 (migrate:check-pkf-tasks-current 内)
expected=$(bump-semver get vcs:latest-tag(kawaz/pkf-tasks))
grep -q "$expected" Taskfile.pkl || { echo "Taskfile.pkl is behind"; exit 1; }
```

## 仕様詳細

### 引数解決ロジック

1. **`owner/repo` 形式** (例: `kawaz/pkf-tasks`): `https://github.com/<owner>/<repo>` に展開
2. **`https://...` / `http://...`**: そのまま使用
3. **`git@<host>:<owner>/<repo>(.git)?`**: SSH URL、そのまま `git ls-remote` に渡す
4. **引数なし**: 現状互換 (cwd の VCS)

### tag フィルタ

- pkf-tasks 流儀 (`pkf-tasks@<version>`) と bump-semver 流儀 (`v<version>`) の両方で動くようにする
- SemVer-compatible なものだけ返す (現状の `vcs:latest-tag()` と同じ挙動)
- 出力形式は **tag 名そのまま** (例: `pkf-tasks@0.0.9` / `v0.14.2`)。version 部分だけ欲しい時は呼び出し側で `${result#prefix}` する

### 実装方針

- `git ls-remote --tags <url>` で remote refs 取得
- jj は ls-remote 相当機能を持たないため、bump-semver は内部で git を呼ぶ
- `--vcs jj|git` flag は cwd の VCS detection 用、remote 取得には影響しない
- ネットワーク失敗時のエラーメッセージ: "vcs: failed to ls-remote `<url>`" (URL 表示)

### CLI quote / 引数仕様

**内部ダブルクオートは不要 (`<arg>` は raw string)**:

```bash
# OK (推奨、canonical)
bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'

# NG (内部 quote 不要、解釈エラー or 文字列として読まれる)
bump-semver get 'vcs:latest-tag("kawaz/pkf-tasks")'
```

設計判断 (重要): Pkl の Task `cmd` を **配列形式** で書く習慣 (`["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]`) との相性を優先する。内部にダブルクオートを含めると Pkl / JSON でエスケープが必要になり (`"vcs:latest-tag(\"user/repo\")"`) 可読性が著しく落ちる。これを回避するため bump-semver の parser は `(...)` 内の中身を raw string として受ける。

shell quote (`'...'`) は shell の `()` subshell 記法を回避するために必要だが、これは外側の wrapper であり引数の一部ではない。Pkl 配列形式では shell を介さないため wrapper も不要 (`["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]` で動く)。

help の例示:
```
bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'      # shell から (single quote で () 回避)
bump-semver get vcs:latest-tag\(kawaz/pkf-tasks\)      # shell から (バックスラッシュ escape)
# Pkl Task.cmd 配列形式: ["bump-semver", "get", "vcs:latest-tag(kawaz/pkf-tasks)"]
```

## 互換性

- 既存 `vcs:latest-tag()` (引数なし) の挙動は変えない
- 新規引数受付のみ追加 = 後方互換

## 利用想定

- pkf-tasks v0.0.12 で `vcs.latestRemoteTag(repo)` Pkl helper の内部実装を bump-semver 呼び出しに置換 (現繋ぎ実装の git ls-remote bash を撤去)
- kawaz の他リポでも「他リポの最新 tag」を参照する場面で再利用可能

## 関連

- pkf-tasks 側の繋ぎ実装: `kawaz/pkf-tasks` v0.0.11 (`tasks/vcs/auto.pkl` の `latestRemoteTag` Pkl function)
- pkf-tasks 側の DR-0006 (vcs/* を VCS knowledge 集積場として位置付け) と整合的に bump-semver/pkf-tasks の責務分担を明確化
