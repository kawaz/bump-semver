# DR-0043: グローバル `-C/--cwd <path>` — 起動ディレクトリの付け替え

- Status: Accepted (2026-07-11)
- Date: 2026-07-11
- Related: DR-0020 (vcs-subcommands, exit code 規約), DR-0041 (`vcs get repository` — 他リポを対象にする利用面の同根ニーズ)

## Context

bump-semver は cwd 前提のコマンド (VERSION 系ファイル引数、`vcs` サブツリー、rule 解決) が
大半で、別ディレクトリのリポを対象にするには利用側がサブシェル `(cd <path> && bump-semver ...)`
を書く必要がある。git の `-C <path>` (= "run as if started in <path>") 相当が欲しい。

## Decision

### 名前: `-C, --cwd <path>`

- short `-C` は make / tar / git の UNIX 慣習 ("Change directory") をそのまま採用
- long は `--cwd` (bun / yarn 先例)。意味論が「このプロセスの cwd をこれにして実行」
  そのものであり最短で伝わる。GNU 正統の `--directory` (tar / make) は、`--repository`
  (DR-0032) のような「対象を指す引数」と並んだとき「cwd を変える」ことが読み取りにくい
  ため不採用。`--cd` は先例が希薄で既存公開語彙優先の規約 (interface-wording) により不採用

### 累積なし (once-only)

git の `-C` は繰り返し指定で前の `-C` からの相対解釈になるが、bump-semver では既存の
once-only flag 流儀 (newOnceString) に合わせて 1 回のみ。2 回目の指定は usage エラー。
相対チェーンが必要なユースケースは観測されておらず、必要ならパスを 1 つに合成して渡せる。

### 実装: 起動直後の `os.Chdir` 一発

cobra 解析より前 (既存の `-qq` 前処理と同層) に argv から `-C/--cwd` を取り出して
`os.Chdir` する。以降の全経路 (ファイル引数・vcs backend・rule 解決・glob) が cwd 経由で
自動追従するため、個別のパス解決改修が不要。

- chdir 失敗 (存在しない / 権限なし) は exit 2 (usage) + cause を含むメッセージ
- 値なし (`-C` が末尾) も exit 2

## Consequences

- 利用側のサブシェル定型 `(cd <path> && bump-semver ...)` が `bump-semver -C <path> ...` に縮む
- `os.Chdir` はプロセス全体に効くため、run() を直接呼ぶテストは cwd を復元する
  (t.Chdir / defer で担保)
- help の global options・README synopsis に同期が必要
