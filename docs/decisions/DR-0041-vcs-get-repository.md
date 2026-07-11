# DR-0041: `vcs get repository` / `repository-url` — remote 由来のリポジトリ識別子 getter

- Status: Accepted (2026-07-11)
- Date: 2026-07-11
- Related: DR-0020 (vcs-subcommands, exit code 規約), DR-0032 (`--repository` フラグの受理形式), `docs/issue/2026-07-11-vcs-get-repo-root-getter-missing.md`

## Context

`vcs get` にはリポジトリの「同一性」(どのリポジトリか) を返す getter が無い。`root` はローカルパスを返すため、git linked worktree / jj named workspace ではディレクトリ名がリポジトリ名として誤用される (issue 2026-07-11)。一方 remote URL は worktree/workspace 間で共有されるため、remote 由来の識別子なら backend 分岐なしに一貫した値が取れる。

利用側の一般的なニーズは 2 種:

1. **識別子として使う**: `gh -R`、forge API、CI 環境変数 (`GITHUB_REPOSITORY`)、ログ・表示。エコシステムで最も流通しているのは `owner/repo` slug
2. **参照として使う**: ブラウザで開く、ドキュメント/リリースノートへのリンク埋め込み、clone

## Decision

### 2 key 構成

- **`vcs get repository`** → slug (例: `kawaz/bump-semver`)。定義は「remote URL から host を除いたパス全体」(`.git` 除去)。GitHub なら常に 2 セグメント、GitLab subgroup なら `group/sub/repo` — セグメント数を 2 に決め打ちしない
- **`vcs get repository-url`** → https 正規形 URL (例: `https://github.com/kawaz/bump-semver`)。remote が ssh 形式でも https に正規化する — リンクとして使える形が最有用で、生 URL は `git remote get-url` で足りる

host 単体や host 付き slug の key は増やさない (URL から導出可能、必要になったら後付けできる)。

`vcs get repository` の出力は `vcs get latest-tag --repository <R>` が受ける形式 (owner/repo) と一致する — 出力をそのまま `--repository` に食わせられる対称性。

### remote 選択

- デフォルト `origin`、`--remote NAME` で上書き (vcs fetch/push と同じ規約)
- origin 不在時: remote が丁度 1 個ならそれを採用。0 個、または origin 無しで複数は exit 4 (ambiguous) — `current-branch` の ambiguous 規約 (DR-0020) と同じ扱い
- `--remote NAME` 明示で該当 remote が無い場合は exit 3 (VCS subprocess エラーがそのまま表面化)
- `--remote` は repository / repository-url 以外の key では usage エラー (exit 2) — `--rev` の gating と同型

### URL 正規化仕様

受理する remote URL 形式と変換:

- scp 風 `[user@]host:path` (scheme 無し、`:` の後が `//` でない) → host + path
- scheme 付き `ssh://` / `git://` / `http://` / `https://` → user info を除去、scheme は `https` に置換。host の port は保持 (self-hosted forge の非標準 port を壊さない)
- 末尾 `.git` と末尾 `/` を除去
- slug = path から先頭 `/` を除いた全体
- ローカルパス remote (`/path/to/repo`, `file://...`) は forge URL に変換できないため exit 3 (原因と `git remote get-url` での代替をメッセージで示す)

### 仕様の限界 (許容)

ssh alias (`~/.ssh/config` の `Host gh-work` 等) を使った remote は正規化しても実 host に戻せない (`https://gh-work/...` になる)。alias 解決まで踏み込むのは過剰と判断し、remote URL の host をそのまま使う。

## Consequences

- claude-ccmsg 等の利用側は backend 別分岐 (`jj なら dirname(root)`) を書かずに repo 名を取得できる
- issue の path ベース getter (`repo-root`) は本 DR のスコープ外 — remote 由来で用途が満たせるため見送り、path 版が必要になったら別 DR
