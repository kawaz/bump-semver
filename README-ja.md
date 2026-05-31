# bump-semver

> [English](./README.md) | 日本語

バージョン管理用ファイル中の semver 文字列を取得・bump・比較するための、絞り込まれた CLI。ファイル形式は basename で自動判定 (`--pattern` regex フラグ不要)、5 つの flat なアクション (`major` / `minor` / `patch` / `pre` / `get`) と 1 つのネストサブコマンド (`compare`) を持つ。新しいバージョンは常に stdout に出力するのでシェルパイプラインに合成しやすい。

## なぜ作ったか

既存のバージョン bump CLI は「汎用すぎて毎回 regex を指定する必要がある」「特定のファイル形式しか扱えない」のどちらかに偏っている。`bump-semver` は真逆の立場を取り、kawaz が実際に使うファイル形式だけを正確にサポートし、新しい形式は具体的な必要が出たときに追加する。結果として「小さい・断定的・予測可能」な kawaz スタイルのツールになる。

## インストール

```bash
brew install kawaz/tap/bump-semver
```

`kawaz/tap` は [`kawaz/homebrew-tap`](https://github.com/kawaz/homebrew-tap) のこと。2ステップ等価形式: `brew tap kawaz/tap && brew install bump-semver`。

Linux / macOS / Windows (amd64, arm64) のビルド済みバイナリも GitHub Releases に公開。

## 使い方

```
bump-semver get <INPUT...>
bump-semver <major|minor|patch|pre> <INPUT...> [--write]
bump-semver compare <eq|lt|le|gt|ge|...> <BASE> <OTHER...>
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit [--amend] [-m MSG] <PATH..|--staged>     # または: vcs commit --amend [-m MSG]
bump-semver vcs fetch [REMOTE]
bump-semver vcs push --branch NAME [--remote REMOTE]
bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]
bump-semver vcs tag delete NAME [--remote REMOTE]
bump-semver --version [--json]
bump-semver --help | --help-full
```

`<INPUT>` は **FILE パス** / **生の VER 文字列** / **`-` (stdin から VER 1 行読込)** / **`vcs:REV[:FILE]` または `vcs:<関数>(...)`** (VCS 経由で取得、[vcs: 入力](#vcs-入力) 参照) / **`cmd:<shell-command>`** (shell コマンド経由で取得、[cmd: 入力](#cmd-入力) 参照) のいずれかで、複数指定時は混在可能。

ヘルプは 3 段構成 (v0.13.0+):

- `bump-semver --help` / `-h`: 1 画面に収まる短い概要 (アクション一覧 + 主要動線)
- `bump-semver --help-full`: 完全リファレンス (Supported file formats 表 / 全 Examples / Exit codes 等)
- `bump-semver <action> --help`: アクション固有の詳細 help。`bump-semver patch --help` で bump 共通 help (major/minor/patch)、`bump-semver pre --help` で pre の 3 モード、`bump-semver compare --help` で precision suffix 含む全 20 OP のリファレンス

### アクション

| アクション | 効果 |
|---|---|
| `major` | major を bump (`X.0.0`)。pre-release / build metadata はデフォルトで drop |
| `minor` | minor を bump (`x.Y.0`)。同上 |
| `patch` | patch を bump (`x.y.Z`)。同上 |
| `pre`   | pre-release の counter advance / 上書き / 削除 (3 モード、後述) |
| `get`   | 現在のバージョンを出力 (整合性チェック兼用) |

### compare サブコマンド

```
bump-semver compare <OP> <BASE> <OTHER...>
```

`<OP>` は `eq` / `lt` / `le` / `gt` / `ge` のいずれか。SemVer 2.0.0 順序仕様準拠で比較する (build metadata は順序比較から除外、prefix / sep の違いは正規化)。`<BASE>` を基準に各 `<OTHER>` を個別に "BASE OP OTHER" として評価する ([DR-0023](./docs/decisions/DR-0023-n-arg-extension.md))。従来の 2 入力形式は N=1 ケースとして互換維持。

| OP | 真となる条件 |
|---|---|
| `eq` | BASE が全 OTHER と等しい |
| `lt` | BASE が全 OTHER より小さい |
| `le` | BASE が全 OTHER 以下 |
| `gt` | BASE が全 OTHER より大きい |
| `ge` | BASE が全 OTHER 以上 |

終了コード: `0` = 真 / `1` = 偽 / `2` = エラー (`test` / `dpkg --compare-versions` 慣習)。`1` のときは失敗したペアごとに stderr へ詳細を 1 行ずつ出力する (`compare gt: VERSION (0.26.3) is not greater than O1=vcs:main@origin (0.27.0)` 形式、全 OTHER を評価しきってからまとめて出す)。`-qq` で抑制可能。

OP には `-major` / `-minor` / `-patch` の suffix を付けて比較精度を切り詰められる ([DR-0017](./docs/decisions/DR-0017-compare-precision-suffix.md))。5 base × 4 precision = 20 OP。

OTHER 側の `vcs:REV` で FILE を省略すると BASE の path を借用する: `compare gt VERSION vcs:main vcs:v1.0.0` は `vcs:main:VERSION` と `vcs:v1.0.0:VERSION` を読む。

```bash
bump-semver compare eq Cargo.toml v1.2.3 && echo same
bump-semver compare lt 1.2.3-rc.1 1.2.3                       # exit 0 (rc < 確定版)
bump-semver compare eq-major 1.2.3 1.9.7                      # exit 0 (同じ major)
bump-semver compare eq-patch 1.2.3 1.2.3-rc.1                 # exit 0 (pre-release 無視)
bump-semver compare lt-minor Cargo.toml vcs:origin/main       # minor 以下しか動いてない?
bump-semver compare gt VERSION 'vcs:main@origin' 'vcs:v1.0.0' # main と v1.0.0 のどちらより上か
```

### vcs サブコマンド

```
bump-semver vcs get <root|backend|current-branch>
bump-semver vcs is  <clean|dirty|git|jj>
bump-semver vcs diff [-s|--name-status] [-q|--quiet] REV [PATH..]
bump-semver vcs commit -m MSG PATH..
bump-semver vcs commit -m MSG --staged
bump-semver vcs commit --amend [-m MSG] [PATH.. | --staged]
bump-semver vcs fetch [REMOTE]
bump-semver vcs push --branch NAME [--remote REMOTE]   # jj users: --bookmark も同義で受理
bump-semver vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]
bump-semver vcs tag delete NAME [--remote REMOTE]      # 冪等 (rm -f セマンティクス)
```

git/jj を抽象化した小さなヘルパー群 ([DR-0020](./docs/decisions/DR-0020-vcs-subcommands.md))。PR-1 で `vcs get` (read-only) が land、PR-2 で `vcs is` (述語) が加わり、PR-3 で `vcs diff` (patch 出力) が追加され、PR-3.1 で `vcs diff` に `-s/--name-status` (M/A/D サマリ) と `-q/--quiet` (差分有無を終了コードで返す、`git diff --quiet` 相当) が拡張、PR-4 で `vcs commit` (path 必須を基本としつつ `--staged` / `--amend` を持つ安全な commit) が land、PR-5 で `vcs fetch` / `vcs push` (ネットワーク側の counterpart。`--force` は意図的に非提供、non-ff は exit 5 で検出) が land、PR-6 で `vcs tag push` / `vcs tag delete` (create+push をアトミックに / delete は冪等。`--allow-move` を tag 移動の精密な opt-in として用意し、別 rev の整合性違反は exit 4 で表面化) が追加。動機: Taskfile / justfile で git と jj を毎回手書き分岐する板挟みの解消。`bump-semver` は既に `vcs:` で VCS read を吸収しているので、その自然な拡張として `vcs` サブコマンド群を同居させる。

**`vcs get <key>`** — 値を stdout に出力:

| key | 出力 |
|---|---|
| `root` | リポジトリルートの絶対パス |
| `backend` | `git` または `jj` (colocated 構成では jj が勝つ) |
| `current-branch` | 一意に決まる現在の branch (git) / bookmark (jj)。DETACHED HEAD や同じ head に bookmark が複数 → exit 4 |

**`vcs is <pred>`** — 終了コードが答え (0=真、1=偽、stderr は silent):

| 述語 | 意味 |
|---|---|
| `clean` | コミット必要な変更なし。**git**: `git diff --quiet` AND `git diff --cached --quiet` (untracked は無視)。**jj**: `@` が empty (template `empty`)。jj は read で自動 snapshot するので新規ファイルも dirty 扱いになる — git との非対称は意図的 |
| `dirty` | `!clean` |
| `git` / `jj` | 自動判定 (または `--vcs` 強制) の backend が一致 |

**`vcs diff REV [PATH..]`** — `REV` と working copy の patch を stdout に出力。backend 統一: git は `git diff REV [-- PATH..]`、jj は `jj diff --from REV --to @ [-- PATH..]` を実行。どちらも REV と working copy (未コミット変更を含む) を比較する。

`-s` / `--name-status` を付けると 1 ファイル 1 行の `<CODE>\t<path>` 形式 (M/A/D — modify / add / delete) を出力する。git は native、jj は `--summary` の native 出力 (space 区切り) をタブ区切りに正規化して、backend 間で同一形式に揃える。

`-q` / `--quiet` は `vcs diff` ではグローバルの「stdout 抑制」の意味を **`git diff --quiet` の `--exit-code` セマンティクス相当に拡張する**: **exit 0 = 差分なし、exit 1 = 差分あり**。stdout は空、stderr は `-qq` でない限り保持される。`-s -q` を併用した場合は `-q` が勝つ (stdout 空、exit は差分有無)。`check-version-bumped` 系の「REV から何か変わったか?」判定 (scripting) に直接使える。他の vcs verb (`get` / `is`) は従来通り「stdout 抑制」のみで差分有無は持たない — `diff` だけがこの意味付けに対して well-posed なので、ここだけ overload している。

PATH フィルタ規則 (**宣言的収束**): 存在しない `PATH` 引数は黙ってスキップされる。全 `PATH` が filter で 0 件になった場合は exit `0` + 空 stdout で終わる — `git diff REV --` のような「全ファイル diff」への broaden は **しない**。`REV` には存在するが working copy で削除済みのファイルを明示的に `PATH` 指定した場合、その削除は表示されない (PATH 無指定の full diff なら表示される)。`-q` 時に全 PATH が filter で消えた場合は exit 0 (= 「報告すべき差分なし」)。

終了コード (詳細は後述): `0` 成功 / 述語が真 (`vcs diff -q` で差分なしも含む); `1` 述語が偽 (`vcs is`、および `vcs diff -q` で差分ありの場合); `2` usage エラー; `3` VCS 実行エラー (= 「リポではない」「REV 解決不能」を含む); `4` 曖昧; `5` non-fast-forward push (`vcs push` のみ)。

```bash
bump-semver vcs get backend                  # git
bump-semver vcs get root                     # /path/to/repo
bump-semver vcs get current-branch           # main
ROOT=$(bump-semver vcs get root) || exit

bump-semver vcs is clean && bump-semver patch VERSION --write
if bump-semver vcs is git; then ... fi
bump-semver vcs is dirty || echo "コミットすべき変更なし"

bump-semver vcs diff HEAD~1                   # 直前コミットからの全 diff
bump-semver vcs diff main@origin VERSION      # VERSION の remote main との差分
bump-semver vcs diff HEAD~1 src lib           # subtree スコープの diff
bump-semver vcs diff -s HEAD~1                # M/A/D ファイル一覧 (git --name-status 形式)
bump-semver vcs diff -q HEAD~1 -- VERSION && echo "VERSION 変更なし"
                                              # exit 0 ⇔ VERSION に差分なし
```

**`vcs commit`** — 安全側に倒した 3 モードの commit:

| モード | 動作 |
|---|---|
| `-m MSG PATH..` | 指定 path の working-tree 内容だけを stage + commit。存在しない path は黙ってスキップ (宣言的収束)。全 path 不在 / 全部変更なし → exit 0 (commit せず冪等成功) |
| `-m MSG --staged` | staged / dirty な変更を一括 commit。**git**: index を commit。**jj**: `@` 全体を commit (jj は自動 stage)。内容なし → exit 0 (冪等) |
| `--amend [-m MSG] [PATH.. \| --staged]` | 新規 commit を作る代わりに直前 commit へ吸収する。上 2 モードと**完全対称** — `--amend` でも `PATH..` / `--staged` 同じ selector を受理する。素の `--amend` は明示 rewrite (gate なし、message-only amend も合法)。吸収範囲は backend で異なる — **git**: staged の index を HEAD に吸収 (未 stage の worktree 変更は **含まない**、`--staged` と同じスコープ)。**jj**: `@` snapshot 全体を `@-` に吸収 (jj は自動 stage なので全変更が吸収対象)。`--amend PATH..` は指定 path のみを吸収 (path モードと同じ「全 path 不在 / 全変更なし → no-op」ルールが効く)。`--amend --staged` は素の `--amend` の明示的シノニム (= 吸収元は index / `@` snapshot そのもの)。`-m` ありで直前メッセージを上書き、なしで保持。等価コマンド: git → `git add -- PATHS; git commit --amend [-m\|--no-edit] -- PATHS`、jj → `jj squash --from @ --into @- [-m MSG \| -u] [-- PATHS]` |

**`-a` / `--all` は意図的に非提供** (DR-0020 安全設計)。jj の「カレントコミット=自動 stage」世界観だと `-a` の unstaged 巻き込み挙動は事故を招きやすいため、`--staged` (全変更を commit) または `PATH..` を明示する形に絞っている。`-a` を渡すと exit 2 + `--staged` / `PATH..` への誘導 hint を返す。

path / `--staged` モードの empty-no-op ルールにより、以下のような言語横断スニペットが安全に書ける:

```bash
# version 関連ファイルが「あれば commit、無ければスルー」(プロジェクトに依存しない)
bump-semver vcs commit -m "bump version" VERSION Cargo.toml package.json pyproject.toml
```

`vcs commit` の終了コード: `0` 成功 or 冪等 no-op; `2` usage エラー (`-m` 欠如 / `-a` 拒否 / `--staged + PATH` / mode 未指定); `3` VCS 実行エラー (リポではない、commit 失敗)。

```bash
bump-semver vcs commit -m "bump 1.2.3" VERSION         # VERSION のみ commit
bump-semver vcs commit --staged -m "release: 1.2.3"     # staged を一括 commit
bump-semver vcs commit --amend                          # 直前に吸収 (git: index、jj: @)、メッセージ維持
bump-semver vcs commit --amend -m "release: 1.2.3 (final)"  # 直前のメッセージを更新
bump-semver vcs commit --amend VERSION                  # VERSION だけを直前に吸収
bump-semver vcs commit --amend --staged -m "fixup"      # staged を一括で直前に吸収
```

**`vcs fetch [REMOTE]`** — 指定 remote (省略時 `origin`) から refs を refresh する。

- **git**: `git fetch <remote>`
- **jj**: `jj git fetch --remote <remote>`

`REMOTE` は positional または `--remote NAME` のいずれかで渡す — 両方同時に指定すると usage error (暗黙の優先順位サプライズを避けるため)。Refspec scope / prune / tag 制御は意図的にラップしていない (= 必要なら素の `git fetch ...` / `jj git fetch ...` を直接使う)。

**`vcs push --branch NAME [--remote REMOTE] [--jj-bookmark-auto-advance]`** — `NAME` を `REMOTE` (省略時 `origin`) に push。`--branch` が canonical。jj users は `--bookmark` でも書ける (jj のネイティブ用語で同義)。同じスロットを共有するため両方同時指定は usage error。

| 観点 | 動作 |
|---|---|
| 実行 | **git**: `git push <remote> <name>:<name>` (明示 refspec で `push.default` の副作用を排除)。**jj**: `jj git push --bookmark <name> --remote <remote>` の後に `jj git export` を実行 (colocated `.git` の ref 同期)。export 失敗時は 1 回だけ再試行 (一時的な packed-refs lock / HEAD race などはこれで解消する)。再試行も失敗したら exit 3 + 該当 [jj-vcs/jj issue](https://github.com/jj-vcs/jj/issues) (`#493` ref 階層衝突、`#6098` HEAD race、`#6203` packed-refs) を示す復旧 hint を出す |
| NAME 必須 | 現在の branch / bookmark からの自動推測はしない。明示することで「あれ、結局どの ref を push した?」という事故を構造的に防ぐ |
| 冪等 | 「remote が既に最新」→ exit 0。git/jj 自身の `Everything up-to-date` / `Nothing changed` 行は stderr に素通しするので、収束が起きたことをユーザが確認できる。DR-0020 の 0-targets-no-op ルール |
| non-ff | remote が拒否 → **exit 5**。bump-semver は editorial な hint を被せず、git/jj の素 stderr をそのまま流す (kawaz 確定: 復旧 = ユーザ責務)。`fetch` + reconcile で進めるか、本気で remote history を rewrite するなら素の `git push --force-with-lease` を直接叩く。`--force` は非提供 (指定すると exit 2) |
| `--force` / `--tags` | 非提供。force push は remote history の rewrite で SemVer ヘルパの責務外、tag push は release 自動化 (`gh release create`) 側の仕事 |
| `--jj-bookmark-auto-advance` | **jj 専用の opt-in (PR-5.2)**。push 前に bookmark を「公開すべき commit」に自動で進める。clean な `@` (空 working copy) → bookmark を `@-` に。dirty な `@` (非空、通常は describe 済) → bookmark を `@` に。bookmark が存在しない場合は何もせず通常 push に委ね、`ancestors(@)` に居ない (sideways / divergent) 場合は exit 3 + hint で停止する (移動しない)。移動自体は forward-only (`--allow-backwards` は付けない)。git リポで指定すると usage error (exit 2)。**Why**: jj 慣習では bookmark は確定 commit (`@-`) に置き、`@` は使い捨ての working copy。bump のたびに `jj bookmark move` を手で打つ摩擦を構造的に解消するためのフラグ |

```bash
bump-semver vcs fetch                      # origin を fetch
bump-semver vcs fetch upstream             # 特定の remote を fetch

bump-semver vcs push --branch main         # main を origin へ
bump-semver vcs push --branch main --remote upstream

# よく使う release 前 gate (Taskfile パターン):
bump-semver vcs is clean \
  && bump-semver vcs fetch \
  && bump-semver vcs push --branch main

# jj: bookmark を自動で進めてから push (手動の `jj bookmark move` 不要)
bump-semver vcs push --branch main --jj-bookmark-auto-advance
```

`vcs push` の終了コード: `0` 成功 / no-op; `2` usage (`--branch` / `--bookmark` 欠如・両指定、`--force` 指定、positional 引数、未知フラグ、git リポでの `--jj-bookmark-auto-advance`); `3` VCS 実行エラー (unknown remote / network / 再試行しても解消しない jj export 失敗 / `--jj-bookmark-auto-advance` が bookmark を `ancestors(@)` に見出せず拒否); `5` non-fast-forward — 復旧経路は git/jj の stderr を参照。

**`vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]`** — `NAME` を `REV` で create / move し、`REMOTE` (省略時 `origin`) に push するまでをアトミックな 1 ステップで実行する。動詞の契約は「return 時点で tag が remote 上で `REV` を指している」。local 作成は手段であって成果物ではないため、tag の lifecycle は remote 上の存在と 1-1 になる (orphan local tag を作らない)。

| 観点 | 動作 |
|---|---|
| 実行 | **git**: `git tag NAME REV` (`--allow-move` の場合は `git tag -f`) の後に `git push origin refs/tags/NAME` (`--allow-move` 時のみ `--force`)。**jj**: `jj tag set NAME -r REV` (移動時は `--allow-move`) → `jj git export` → `git -C <git_target> push ...`。jj 0.41 が tag 単位の push primitive を持たないため native git push を使う (DR-0020 line 70 で「create は jj tag set / push は native git」と確定) |
| 同 rev 再 push | local が既に同じ REV を指している → local create はスキップして push は実行。**片落ちリカバリ**: local にはあるが前回の push が remote に届く前に落ちた可能性を救う。remote も同じなら push は素の no-op |
| 別 rev でフラグなし | **exit 4** で side-effect なし (local の move も push も行わない)。一般 VCS エラー (`3`) と分離してあるので呼び側で整合性違反を別扱いできる |
| 別 rev で `--allow-move` | local を `--force` 相当で移動 + remote に force-push。`--force-with-lease` は tag ref に remote-tracking ref が無いため lease が成立せず `--force` と安全性は変わらない。すでに `--allow-move` (明示 opt-in) + 別 rev 検知 (= 何を上書きするか分かっている) でガード済み |
| 不正 REV | 解決失敗 → **exit 3** で side-effect 前。「tag が drift した」(4) / 「git/jj 故障」(3 + 素 stderr) と区別可能 |
| `--force` / `--tags` / `--all` | 非提供。`--force` は粗すぎる (同 rev 冪等な reconcile と別 rev rewrite を区別できない) ため `--allow-move` が精密な opt-in。bulk 操作はスコープ外 (DR-0020 line 91) |

**`vcs tag delete NAME [--remote REMOTE]`** — local と remote 両方から tag を削除する。DR-0020 line 74 の `rm -f` セマンティクスに従って冪等: 動詞の意図は「NAME に tag が無い状態」への収束であり、既に無ければ目的達成済みなので片側 / 両側不在は exit 0。

- **git**: local 存在を `git rev-parse -q --verify refs/tags/NAME` で事前判定 (素の `git tag -d NAME` は不在で失敗するため) → `git push origin :refs/tags/NAME` (git 自身の「deleting a non-existent ref」は exit 0 — remote layer は構造的に冪等)
- **jj**: `jj tag delete NAME` は native で冪等 ("No matching tags" → exit 0) なのでそのまま実行 → `jj git export` → 同じ `git push origin :refs/tags/NAME`
- 真の remote 失敗 (unknown remote / network down) は exit 3。local 側の side-effect は既に走っている可能性があるが、許容する非対称: 典型的なユースケースは「remote は健全で、ただ古い local tag を片付けたい」であり、「remote ack 後にしか local も消さない」案は稀なクリーンリトライのために頻繁な摩擦を生む
- `--allow-missing` は**非提供** — 既に冪等なので存在しても no-op になる (DR-0020 line 92)

```bash
bump-semver vcs tag push --rev HEAD v1.2.3
                                                # HEAD を v1.2.3 として tag、origin に push
bump-semver vcs tag push --rev HEAD~1 v1.2.3 --allow-move
                                                # v1.2.3 を 1 commit 後退させる (force-push)
bump-semver vcs tag push --rev main v1.2.3 --remote upstream
                                                # main を tag、非デフォルト remote へ
bump-semver vcs tag delete v0.9.0               # local + origin から削除 (冪等)
```

`vcs tag push` の終了コード: `0` 成功 (同 rev 再 push の冪等成功を含む); `2` usage (NAME / `--rev` 欠如、NAME の形式不正、`--force` 指定、余分な positional); `3` VCS 実行エラー (REV 不正 / unknown remote / network); `4` 整合性違反 (`--allow-move` 無しで既存 tag が別 rev)。`vcs tag delete` は `0` 成功 or 既に不在; `2` usage; `3` VCS エラー。

`--vcs jj|git|auto` は引き続き有効。colocated 構成で git 側を見たい場合は `bump-semver vcs get backend --vcs git` (または `vcs is git --vcs git`) で強制できる。

### フラグ

| フラグ | 説明 |
|---|---|
| `--pre PRE`            | pre-release 識別子を設定 (例 `--pre rc.0`) |
| `--no-pre`             | pre-release を削除 |
| `--build-metadata META`| build metadata を設定 (例 `--build-metadata sha.abc`) |
| `--no-build-metadata`  | build metadata を削除 |
| `--write`              | bump 結果を各 FILE 入力に書き戻す (`major` / `minor` / `patch` / `pre` のみ) |
| `--vcs jj\|git\|auto`    | `vcs:` 入力の VCS を強制指定 (default: `auto`) |
| `--no-hint`            | 全 `hint:` 行を抑制 (fallback match / unsupported file / 「files not modified」) |
| `-q`, `--quiet`        | stdout と全 `hint:` 行を抑制 |
| `-qq`, `--quiet-all`   | stdout / hint / エラー出力をすべて抑制 (debug 時注意) |
| `--json`               | `get` / `major` / `minor` / `patch` / `pre` の出力を構造化 JSON にする (`compare` では不可) |
| `--version`, `-V`      | バイナリのバージョン |
| `--help`, `-h`         | 短いヘルプ (1 画面) |
| `--help-full`          | 完全リファレンス (Supported file formats / 全 Examples / Exit codes 等) |
| `<action> --help`      | アクション固有の詳細 help (`bump-semver patch --help` / `compare --help` 等) |

排他: `--pre` と `--no-pre` 同時指定はエラー、`--build-metadata` と `--no-build-metadata` 同時指定はエラー、`--write` と `get` / `compare` の組み合わせはエラー。

`-q` / `-qq` / `--no-hint` は排他チェックなし: `-qq` は `-q` の上位互換、`-q` は `--no-hint` の上位互換 (両方指定でも黙って吸収)。`compare` は元々 stdout を持たないので `-q` は stdout 抑制部分は no-op。

`bump-semver` は通常の stdout に加えて状況に応じて stderr に `hint:` 行を 1 つ以上出力する。すべての hint は共通の `hint:` prefix を持ち、`--no-hint` / `-q` / `-qq` で一括抑制される。現状の hint 一覧:

| Hint | 発火条件 | 対象 / 抑制 |
|---|---|---|
| `hint: <path> matched as *.<ext> fallback. Open issue if explicit support is needed.` | FILE 入力が confidence 1 fallback で解決された (`*.json` は DR-0010、`*.yaml` / `*.yml` / `*.toml` は DR-0011)。該当ファイルごとに 1 行 | FILE を resolve するすべての場面 (`get` / bump 系 / `compare`) |
| `hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.` | FILE が `unsupported file:` でエラーになった時、その直後の hint 行 | 上に同じ |
| `hint: <N> file(s) not modified; use --write to update or --no-hint to suppress` | bump 系 (`major` / `minor` / `patch` / `pre`) で FILE 入力があり `--write` 未指定 | bump 系のみ。VER のみの bump や `get` / `compare` では出ない |

### 入力 (INPUT)

| 形式 | 解釈 |
|---|---|
| FILE | サポート形式のファイルパス (basename で自動判定) |
| VER  | semver 文字列を直接 (`1.2.3` / `v1.2.3` / `1.2.3-rc.1+build.42` 等) |
| `-`  | stdin から VER を 1 行読込 (1 回のみ使用可) |
| `vcs:REV[:FILE]` | jj/git の `<REV>` 時点のファイル内容から取得 (自動判定、[vcs: 入力](#vcs-入力) 参照) |
| `vcs:latest-tag()` | jj/git のタグ一覧から最大の semver-compat 値を取得 |
| `cmd:<shell-command>` | shell コマンドを `bash -c` で実行し、stdout の最初の非空行を VER として取得 (read-only、[cmd: 入力](#cmd-入力) 参照) |

`1.2.3` という名前のローカルファイルを明示したいときは `./1.2.3` で曖昧さを回避 (Unix 慣習)。

### サポートする version 形式

```
本体: (v|ver|version)?[._]?\d+[._]\d+[._]\d+      (sep1 == sep2 を強制)
pre:  -<id>(.<id>)*                                (SemVer 2.0.0 仕様準拠)
meta: +<id>(.<id>)*                                (SemVer 2.0.0 仕様準拠)
```

- prefix `v` / `ver` / `version` は省略可 (例 `v1.2.3`, `version_1_2_3`)
- 本体セパレータは `.` または `_` のいずれか、両側で一致が必要 (DR-0003 + DR-0006)
- 本体に `-` セパレータは **不可** (pre-release の `-` と衝突するため)
- pre-release: `rc.0`, `alpha`, `beta.1` 等。数値のみ識別子は leading zero 禁止
- build metadata: `build.42`, `sha.5114f85` 等。leading zero 許容 (SemVer 仕様)

入力にあった prefix と sep は出力で**保持される**。

### bump 挙動 (drop デフォルト)

bump 時、`--pre` / `--build-metadata` を明示しない限り、既存の pre-release / build metadata は **drop** する (DR-0006)。

| 入力 | `patch` | `pre` | `pre --pre alpha` | `pre --no-pre` |
|---|---|---|---|---|
| `1.2.3`            | `1.2.4` | error: pre-release 不在 | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0`       | `1.2.4` (drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |
| `1.2.3-rc1`        | `1.2.4` | error: not incremental | `1.2.3-alpha` | `1.2.3` |
| `1.2.3+build`      | `1.2.4` (drop) | error: pre-release 不在 | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0+build` | `1.2.4` (両 drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |

これは **npm 流の strip-don't-bump (`patch 1.2.3-rc.0 → 1.2.3`) とは異なる**。`patch` は常に patch を上げる、`--pre` / `--build-metadata` を明示しなければ drop、という単一規則を採用 (内部一貫性優先)。

### `pre` アクションの 3 モード

- **引数なし (`pre INPUT`)**: 既存 pre が `rc.N` のように末尾識別子が pure numeric のときのみ counter advance。それ以外 (`rc1` 等の英数字混在) はエラー
- **`--pre PRE`**: PRE 値で完全上書き (元 pre 有無問わず、巻き戻りも許容)
- **`--no-pre`**: pre 削除 (元 pre 不在でも nop)

### サポートするファイル形式

判定は **path-aware confidence ranked** (DR-0005)。各 FILE に対して確度順にルールを試行し、高確度ルールの path-pattern にマッチしても抽出失敗 (例: `.metadata.version` を持たない `marketplace.json`) なら次ルールへ降りる。最低確度の fallback ルールが top-level `.version` を持つ任意 `*.json` を網羅する。

| 確度 | パターン | 形式 | version パス | name パス |
|---|---|---|---|---|
| **3** (path-pinned) | `.claude-plugin/marketplace.json` | JSON | `$.metadata.version` | `$.name` |
| **3** | `.claude-plugin/plugin.json` | JSON | `$.version` | `$.name` |
| **3** | `package.json` | JSON | `$.version` | `$.name` |
| **3** | `package-lock.json` | JSON | `$.version`, `$.packages[""].version` | `$.name`, `$.packages[""].name` |
| **3** | `Cargo.toml` | TOML | `[package].version` (try) → `[workspace.package].version` | `[package].name` (try) → `[workspace.package].name` |
| **3** | `pyproject.toml` | TOML | `[project].version` (try) → `[tool.poetry].version` | `[project].name` (try) → `[tool.poetry].name` |
| **3** | `mojoproject.toml` | TOML | `[workspace].version` | `[workspace].name` |
| **3** | `project.pbxproj` (Xcode) | pbxproj | 全 `MARKETING_VERSION = ...;` (同期更新) | — |
| **3** | `Info.plist` (Apple plist) | xml | `<key>CFBundleShortVersionString</key>` | — |
| **3** | `pom.xml` (Maven) [DR-0018] | xml-element | `/project/version` | `/project/artifactId` |
| **3** | `VERSION` | plain text | (ファイル内容) | — |
| **2** (basename) | 任意 dir の `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | 任意 dir の `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | regex | `version := "..."` | — |
| **2** | `build.gradle` (Gradle Groovy) [DR-0018] | regex | `version = '...'` / `version "..."` | — |
| **2** | `build.gradle.kts` (Gradle Kotlin DSL) [DR-0018] | regex | `version = "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.cabal` (Haskell) [DR-0018] | regex | `version: ...` (line-anchored) | `name: ...` |
| **1** (fallback) | `*.spec` (RPM) [DR-0018] | regex | `Version: ...` (capital V) | `Name: ...` |
| **1** (fallback) | `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) [DR-0018] | xml-element | `/Project/PropertyGroup/Version` | — |

未対応ファイル (例: `README.md`, `Cargo.lock`) は `unsupported file: <path>` で明示エラー。新フォーマット追加 = テーブル 1 行追加 (+ 必要なら新 format-specific 関数 1 つ) で済む構造 (`--pattern` regex フラグは設計上持たない)。

YAML / TOML fallback (DR-0011) は **top-level キーだけ**を見る。section 配下 / nested mapping 配下の `version` は意図的に対象外。`Cargo.toml` / `pyproject.toml` / `mojoproject.toml` は引き続き confidence-3 ルールが優先されるので、それぞれの section-scoped 挙動は不変。multi-document YAML (`---` 区切り) は最初の document のみ。これらの新ルールでも DR-0010 の fallback hint が出る (`--no-hint` で抑制可能)。

`pyproject.toml` ルール (DR-0014) は PEP 621 の `[project].version` を優先し、無ければ Poetry 旧形式の `[tool.poetry].version` を試行する (TOML format の OR semantics)。両方を持つ pyproject.toml (PEP 621 移行中の理論的中間状態) では最初の hit (PEP 621) のみ書き換えられる。`mojoproject.toml` ルール (DR-0014) は `[workspace].version` を直接読み書きする。両ルールとも共通の TOML section-scoped Replace を経由するので quote style と前後セクション・コメントは保持される。

`Cargo.toml` ルール (DR-0021) も同じ try-fallback 形を使う。シングルクレートの `[package].version` を先に試し、`[package]` を持たない workspace-root では `[workspace.package].version` (メンバー crate が `version.workspace = true` で継承する正本) にフォールバックする。両方を宣言するメンバー crate では crate 自身の `[package].version` が優先。マッチした path (`[package].version` か `[workspace.package].version`) は `get` / `--json` 出力に出るので、何の version を bump しているか常に確認できる。

DR-0012 の `regex` フォーマットは「version が 1 行のソースコード式で書かれる」8 つの言語マニフェスト (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt) をカバーする。**最初のマッチ 1 個** だけが読み書きされ、quote style と version 行末尾のコメントは保持される。

DR-0015 で追加された 2 ルールは、Xcode iOS / macOS プロジェクト固有の「同一ファイル内で複数 version を同期更新する」ケースを扱う。`project.pbxproj` (Xcode の OpenStep plist 形式) は **全 `MARKETING_VERSION = ...;` 行** を一括で読み書きし、不一致があれば `<file>:line:N` 形式のラベル付き column-aligned `version mismatch:` 出力で報告する。`Info.plist` (XML plist) は `<key>CFBundleShortVersionString</key><string>...</string>` ペアを読み書きし、DOCTYPE / インデント / 属性順序 / 兄弟 key を byte 単位で保持する (encoding/xml の Marshal は経由しない)。Xcode 11+ default の `<string>$(MARKETING_VERSION)</string>` placeholder は SemVer としてパース不能なので `unsupported file:` で落ちる — これは利用者に「`project.pbxproj` を追加で渡せ」というシグナルとして機能する。`CFBundleVersion` (build number) は SemVer ではないのでスコープ外 (CI で別途埋めるのが慣例)。

#### Suffix-stripped fallback (DR-0013)

どのルールにも直接マッチしないパスは、basename 末尾の **backup 系 suffix を 1 段だけ剥がして** 既存ルール表で再試行される。採用されたルールの confidence は 1 段下げて報告 (3→2, 2→1, 1→1) され、`hint:` 行が stderr に出るので解決経路が透明に保たれる。多段 suffix (`Cargo.toml.bak.20260510`) は **末尾 1 段のみ** 剥がす (再帰しない)。

| Suffix | 例 | 解決先 |
|---|---|---|
| `.bak` / `.backup` / `.orig` / `.tmp` / `.old` | `Cargo.toml.bak` | `Cargo.toml` ルール (confidence 2) |
| `.YYYYMMDD` (8 桁数字) | `package.json.20260510` | `package.json` ルール (confidence 2) |
| `.YYYYMMDD_HHMMSS` (8+`_`+6 桁数字) | `Chart.yaml.20260510_120000` | `*.yaml` fallback (confidence 1) |
| 末尾 `~` (Emacs / vi 系) | `Cargo.toml~` | `Cargo.toml` ルール (confidence 2) |

```bash
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

Template 系 suffix (`.template` / `.example` / `.sample` / `.dist`) は **意図的に剥がさない**。中身が placeholder のことが多く、本物の manifest として静かに扱うのは現状の `unsupported file:` エラーよりも危険なため。template から抽出したい場合は backup 系 suffix にコピーすればよい (`cp Cargo.toml.template Cargo.toml.tmp`)。

suffix hint は既存の `hint:` prefix を共有し、`--no-hint` / `-q` / `-qq` で DR-0010 fallback hint と同じように抑制できる。両方発火するケース (`unknown.json.bak` → `.bak` 剥がし → `*.json` glob) では suffix hint が先に出る。

npm `package-lock.json` のみ特別扱い: lockfile v1 (npm 5/6) は `unsupported lockfileVersion: 1, please regenerate with npm 7+` エラー。依存エントリ (`$.packages["node_modules/..."]`) は仮に値が同じでも書き換わらない。

### 複数 INPUT: 整合性検証

複数 INPUT を渡すと 1 つの単位として処理される。全 INPUT 間で version は事前に一致している必要がある。不一致時の挙動は verb で異なる ([DR-0023](./docs/decisions/DR-0023-n-arg-extension.md)): `get` は exit 1 + stderr に `version mismatch:` (package name が割れている場合は `name mismatch:`) カラム整列リスト (述語偽の意味付け、全 source 対等。version / name どちらの不一致でも `get` は同じ exit 1 規約); bump 系 (`major` / `minor` / `patch` / `pre`) は exit 2 + stderr の `bump-semver: version mismatch:` / `bump-semver: name mismatch:` (内部不整合で動作拒否)。検出された package name も version と同じく整合性検証され、別プロジェクトのファイルを誤って一括 bump する事故を構造的に防ぐ。name は書き戻し対象ではない。

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
bump-semver patch 1.2.3 a.json b.json --write   # VER 引数で「期待値」を指定して整合性確認、結果は a/b に書き戻す
```

複数 INPUT 指定時の `get` は CI 用の整合性チェックとして機能する (`--write` 不要、全 version が一致しているかだけ検証)。FILE を省略した `vcs:REV` は兄弟 FILE 全 path にピア展開されるので、`get a b vcs:main@origin` は (`a`, `b`, `vcs:main@origin:a`, `vcs:main@origin:b`) の 4-way チェックになる。

`--write` 時、書き戻し対象は **FILE 入力のみ**。VER / stdin 入力は参照値として整合性検証だけに使われる。`--write` 指定時に FILE 入力が 1 つもない場合はエラー (`--write requires at least one FILE`)。

### stdin パイプ

stdin がパイプ **かつ INPUT が単一の FILE のとき**、その FILE は名前ヒントとして扱われ、内容は stdin から読み込まれる (legacy ショートカット、後方互換)。複数 INPUT のときは stdin pipe は無視される。ファイルをチェックアウトせずにリビジョン間で比較したい時に有用:

```bash
jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml
```

`-` を INPUT として明示すれば、stdin から VER 1 行を読み込む新方式 (FILE 入力と混在可能):

```bash
echo 1.2.3 | bump-semver compare eq Cargo.toml -
```

### 使用例

```bash
bump-semver patch Cargo.toml --write              # bump + 書き戻し、新バージョンを出力
bump-semver minor package.json                    # メモリ上で bump、新バージョン出力 (ファイル不変)
bump-semver get .claude-plugin/plugin.json        # 現在のバージョン
bump-semver patch 1.2.3                           # 1.2.4 (VER 直接指定)
bump-semver patch v1.2.3                          # v1.2.4 (prefix を保持)
bump-semver minor version_1_2_3                   # version_1_3_0 (prefix + separator を保持)
bump-semver pre 1.2.3-rc.0                        # 1.2.3-rc.1 (counter advance)
bump-semver pre 1.2.3 --pre rc.0                  # 1.2.3-rc.0 (上書き)
bump-semver patch 1.2.3-rc.0 --pre rc.0           # 1.2.4-rc.0 (bump + pre 再付与)
bump-semver patch 1.2.3-rc.0 --no-pre             # 1.2.4 (drop して bump、確定昇格相当)
bump-semver compare lt 1.2.3-rc.1 1.2.3           # exit 0 (rc < 確定)
bump-semver compare eq .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json   # 3 ファイル整合性チェック
bump-semver get   Cargo.toml --json               # jq 連携向け構造化出力
bump-semver patch Cargo.toml --json               # bump 後のバージョンを完全分解
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'   # ready to release? (CI: bump 済か確認)
bump-semver compare lt Cargo.toml vcs:origin/main      # stale vs remote main? (pull 必要か)
```

### JSON 出力 (`--json`)

`get` と bump 系 (`major` / `minor` / `patch` / `pre`) は `--json` を受け付ける。出力は末尾改行付きの JSON 1 行 (DR-0007)、`jq` にそのまま渡せる。`compare` は exit code が答えなので `--json` 非対応。

```bash
bump-semver get Cargo.toml --json
# {"name":"my-pkg","version":"1.2.3","semver":"1.2.3","major":1,"minor":2,"patch":3,"pre":null,"pre_id":null,"pre_rest":null,"build_metadata":null,"build_id":null,"build_rest":null}

bump-semver patch v_1.2.3-rc.1+build.42 --json
# {"name":null,"version":"v_1.2.4","semver":"1.2.4","major":1,"minor":2,"patch":4,"pre":null,...}
```

| フィールド | 型 | 内容 |
|---|---|---|
| `name` | string \| null | FILE 起源の name (例 `package.json $.name`) を集約。VER / stdin 起源では null |
| `version` | string | 入力フォーマット保持 (prefix + 本体 sep を維持) |
| `semver` | string | strict SemVer 2.0.0 形式 (prefix 除去 + 本体 sep を `.` に正規化) |
| `major` / `minor` / `patch` | int | 数値要素 |
| `pre` | string \| null | pre-release 識別子を `.` で結合した文字列 (例 `"rc.1"`)、不在なら null |
| `pre_id` / `pre_rest` | string \| null | `pre` を最初の `.` で分割。 `.` がなければ `pre_rest` は null |
| `build_metadata` | string \| null | build metadata 識別子の結合文字列 (例 `"build.42"`)、不在なら null |
| `build_id` / `build_rest` | string \| null | `pre` と同じ「最初の `.` で分割」ルール |

CLI が提供するのは **構造分解のみ**。「counter advance 可能か」のような意味判定はしない (必要なら `bump-semver pre VER` を実行して exit code を見る運用)。

### vcs: 入力

`vcs:` で始まる位置引数は VCS (jj または git) 経由で解決される。リリース前のドリフトチェックや「最新 tag より上げているか」の比較を、`jj file show | bump-semver compare lt - ...` のような shell パイプを書かずに 1 行で実現できる (DR-0008)。

```bash
# 直近のリリースタグより上げてるか (CI で push 前にチェック)
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'

# 自分が main から遅れてないか (stale チェック)
bump-semver compare lt Cargo.toml vcs:origin/main

# 前コミットからバージョン変わってる?
bump-semver compare eq Cargo.toml vcs:HEAD~1            # FILE は相手から借用
bump-semver compare eq Cargo.toml vcs:HEAD~1:Cargo.toml # 明示形式

# v0.15.0+: 他リポの最新 release tag を取得
bump-semver get 'vcs:latest-tag(kawaz/pkf-tasks)'        # owner/repo 短縮
bump-semver get 'vcs:latest-tag(https://github.com/x/y)' # フル URL
bump-semver compare ge 0.0.13 'vcs:latest-tag(kawaz/pkf-tasks)'  # 現 pin は最新追従済?
```

| 形式 | 解釈 |
|---|---|
| `vcs:REV[:FILE]` | `<REV>` 時点の `<FILE>` を VCS から読み出す。最初の `:` は `vcs:` プレフィックス、2 つ目の `:` で REV と FILE を分割。FILE 省略時は位置順で最初の sibling (FILE 起源 or `vcs:REV:FILE` 形式) から借用 |
| `vcs:latest-tag()` | cwd VCS の全 tag を取得し、semver パース不可なものは無視、SemVer 2.0.0 順序で最大を返す。0 件なら `no semver-compatible tags found` エラー |
| `vcs:latest-tag(<arg>)` | v0.15.0+。`<arg>` = `owner/repo` (GitHub 短縮、`https://github.com/...` に展開) or HTTPS/SSH フル URL。`git ls-remote --tags` でリモートを照会するため jj/git 自動判定はリモート時に無関係。`pkf-tasks@0.0.13` のような monorepo-style tag は `@` peel fallback で認識される (multi-package repo にも同じ呼び出しで対応)。引数は **raw string** で内部 quote 不要 (markdown link `[]()` 感覚)。**信頼境界**: URL の正当性は呼び出し側責任。第三者書き込み可能 repo を指せば悪意ある `malicious@99.99.99` tag が最大として返る攻撃が成立する (DR-0019) |

**VCS 自動判定** (優先順):

1. `--vcs jj|git` フラグ (`auto` / 未指定は次へ)
2. cwd または親に `.jj` ディレクトリ → jj
3. `.git` ディレクトリ → git
4. それ以外 → エラー (`not a git or jj repository`)

`.jj` と `.git` が並存している場合 (jj colocate モード、kawaz の git-bare + jj-workspace 構成) は **jj が優先**。jj の revset 言語は git ref のスーパーセットなので。

> 旧バージョン (v0.12 以前) では `BUMP_SEMVER_VCS=jj|git` 環境変数がフラグの次の優先位にあったが、v0.13 で廃止された ([DR-0016](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md))。CI / 開発環境で env を設定していた場合は `--vcs jj|git` フラグへ置き換える。

**`--write` と `vcs:` は排他**。VCS の中身に書き戻す機能は持たない (commit/amend が必要になりスコープ外)。混在させると `--write cannot be used with vcs: inputs (vcs: is read-only)` エラー。

**`bump-semver` は `git fetch` / `jj git fetch` を自動実行しない**。`vcs:origin/main` が古い場合は VCS 側のエラーがそのまま伝わる。CI では明示的に fetch してから bump-semver を呼ぶ運用にする。

CI スクリプトを VCS 中立にしたい場合は jj/git どちらでも通る形式 (`origin/main` (jj 側で `main@origin` に自動フォールバック) / commit hash / `latest-tag()`) を推奨。

### cmd: 入力

`cmd:<shell-command>` は `<shell-command>` を `bash -c` で実行し、stdout の最初の非空行を VER として取得する read-only 入力 (v0.16.0+)。leading `v` は strip され、SemVer 2.0.0 として parse される。

```bash
# ビルド済みバイナリの --version 出力と VERSION ファイルが一致してるか?
bump-semver compare eq VERSION 'cmd:./bin/mytool --version'

# 外部ツールの version を取得 (vcs:latest-tag の届かない場所)
bump-semver get 'cmd:brew info --json mytool | jq -r .[0].installed[0].version'

# 別の bump-semver 呼び出し結果との比較も可
bump-semver compare gt 'cmd:bump-semver get Cargo.toml' 'vcs:latest-tag()'
```

| 形式 | 解釈 |
|---|---|
| `cmd:<shell-command>` | `bash -c <shell-command>` で実行し、stdout の最初の非空行を取得。leading `v` を strip し SemVer parse。exit code 非 0 / stdout 空 / parse 失敗はエラー伝播 (child の stderr を含む)。**read-only** (`--write` と排他、`vcs:` と同様) |

**`--write` と `cmd:` は排他** (`vcs:` と同じ)。`cmd:` の出力先に書き戻す概念がないため。

**信頼境界**: 任意 shell コマンドが実行されるため、CI / 自動化スクリプトで使う場合は呼び出し側の責任で安全な command 文字列を組み立てること。外部入力 (環境変数 / argv 等) を `cmd:` の文字列に concat しない。

主な動線は kawaz/pkf-tasks v3.0 の `semver/versions.pkl` (リリース前 gate) で「version files + bin --version 出力」を 1 つの `bump-semver get` 呼び出しで横断比較するための基礎機能として導入された。

### エラーメッセージの形式

エラーは stderr に `bump-semver: <reason>` として 1 行出力される。grep でフィルタする運用も想定し、起源 (VER / FILE) によってフォーマットを使い分けている。

**VER 起源** (位置引数または stdin 経由の生 semver 文字列):

```
bump-semver: rc1 is not incremental, use --pre PRE
bump-semver: 1.2.3 does not have a pre-release, use --pre PRE
```

**FILE 起源** (ファイルから読まれた version): file path + version field path で wrap される。

```
bump-semver: Cargo.toml:[package].version=1.2.3-rc1: rc1 is not incremental, use --pre PRE
bump-semver: package.json:$.version=1.2.3: 1.2.3 does not have a pre-release, use --pre PRE
```

**不一致エラー** (複数 INPUT で値がズレている場合): カラム整列で縦列挙される。

```
bump-semver: version mismatch:
  Cargo.toml:[package].version = 1.2.3
  package.json:$.version       = 1.2.4
  <argv>                       = 1.2.3-rc.1
```

起源ラベル: `<file>:<path>` (FILE 起源) / `<argv>` または `<argv:N>` (位置引数の VER) / `<stdin>` (`-` 経由)。

### 終了コード

- `0` — 成功 / 述語が真 (`compare`、`vcs is`)
- `1` — 述語が偽 (`compare`、`vcs is` — stderr は silent)
- `2` — エラー (パース失敗、整合性 NG、未対応ファイル、排他オプション違反、IO エラー、`vcs` の未知 verb/key 等)
- `3` — VCS 実行エラー (`vcs` サブコマンドのみ: リポ外、git/jj 実行失敗)
- `4` — 曖昧 (`vcs` サブコマンドのみ: DETACHED HEAD、同じ head に bookmark が複数)
- `5` — non-fast-forward push (`vcs push` のみ; remote が divergent — fetch + reconcile してから retry)

## v0.4.x からの移行

v0.5.0 で 3 つの破壊変更が入っている。詳細とサンプルは [UPGRADING.md](./UPGRADING.md) を参照:

1. **`--value` フラグ廃止** → 位置引数で直接 VER を渡す (`bump-semver patch 1.2.3`)
2. **本体セパレータ `-` 廃止** → `.` または `_` を使う (`1-2-3` は不可)
3. **bump 系のエラー時 exit code 1 → 2** (compare 規約に合わせて統一)

## 開発状況

v0.16.1 で `cmd:` 入力モードを堅牢化 — `--write` + `cmd:` の組み合わせを実装側で明示拒否 (v0.16.0 で README には書いていたが実装が `vcs:` のみで `cmd:` を素通ししていた致命 bug、DoS 軽減として子プロセスに 30 秒 timeout + stdout 64 KiB / stderr 4 KiB の出力上限を追加、`cmd:` の空白のみコマンドも非空コマンド要件として reject)。v0.16.0 で `cmd:<shell-command>` 入力モード — shell コマンドの stdout 最初の非空行を VER として取得する read-only 入力。`bash -c` で実行し leading `v` を strip して SemVer parse。`compare eq VERSION 'cmd:./bin/mytool --version'` でビルド済みバイナリと version files の整合 gate にできる。kawaz/pkf-tasks v3.0 の `semver/versions.pkl` 設計を支える前提機能。v0.14.0 で JVM / .NET / Maven / Haskell / RPM 対応 + 新 format `xml-element` (DR-0018) — `pom.xml` / `*.csproj` / `*.fsproj` / `*.vbproj` / `build.gradle` / `build.gradle.kts` / `*.cabal` / `*.spec` を一括追加。`pom.xml` は slash-rooted XML path lookup (`/project/version`) で `<parent>/<version>` を構造的に避ける。v0.13.0 で 3 つの変更: help を 3 段化 (`--help` 短 / `--help-full` 完全リファレンス / `bump-semver <action> --help` action 固有)、`BUMP_SEMVER_VCS` 環境変数廃止 + `--vcs jj|git|auto` (DR-0016、BREAKING、UPGRADING.md 参照)、`compare` に precision suffix 15 OP 追加 (`eq-major` / `lt-minor` / `eq-patch` 等、DR-0017) で 5×4 = 20 OP に拡張。v0.12.0 で Xcode 固有の 2 ルール — `project.pbxproj` (build configuration ごとの `MARKETING_VERSION` を全行同期更新、不一致時は `<file>:line:N` ラベル付き column-aligned mismatch を出力) と `Info.plist` (XML plist の `<key>CFBundleShortVersionString</key>` を byte-range 書き換えで DOCTYPE / インデント保持) — を path-pinned confidence 3 ルールとして追加し、専用 format `pbxproj` / `xml` を新設 (DR-0015)。v0.11.0 で TOML rewriter を section-scoped に一般化し、`pyproject.toml` (PEP 621 + Poetry 旧形式 fallback) と `mojoproject.toml` (`[workspace]`) を path-pinned confidence 3 ルールとして追加 (DR-0014)。v0.10.0 で backup 系 suffix のための suffix-stripped fallback (DR-0013) を追加。v0.9.0 で `regex` フォーマット (DR-0012) を導入。1 行 regex で書き換える汎用 format により 8 種類のファイル (`*.xcconfig` / `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `*.gemspec` / `mix.exs` / `build.sbt`) を一括追加した。v0.8.0 で `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback (DR-0011)、v0.7.0 で `vcs:` 入力モード (DR-0008) — `vcs:REV[:FILE]` / `vcs:latest-tag()` で jj/git の他リビジョン・最新 tag を自動判定で取得。直前: v0.6.0 で `--json` 出力 (DR-0007)、v0.5.0 で pre-release / build metadata 対応 + `compare` サブコマンド + `pre` アクション + FILE/VER 統合 (DR-0006)。今後も「必要が出たら handler を 1 つ追加」(DR-0001) 方針で拡張する。設計判断は [docs/decisions/](./docs/decisions/)、将来検討項目は [docs/ROADMAP.md](./docs/ROADMAP.md) を参照。

## ライセンス

[MIT](LICENSE)
