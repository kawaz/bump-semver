# bump-semver

> [English](./README.md) | 日本語

バージョン管理用ファイル中の semver 文字列を取得・bump・比較するための、絞り込まれた CLI。ファイル形式は basename で自動判定 (`--pattern` regex フラグ不要)、5 つの flat なアクション (`major` / `minor` / `patch` / `pre` / `get`) と 3 つのネスト名前空間 (`compare` / `vcs` / `completion`) を持つ。新しいバージョンは常に stdout に出力するのでシェルパイプラインに合成しやすい。

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
bump-semver vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]
bump-semver vcs get latest-release [--include-prerelease] [--repository REPO] [--json]
bump-semver vcs outdated FROM TO[..]
bump-semver completion <bash|zsh|fish|powershell>
bump-semver --version [--json]
bump-semver --help | --help-full
```

`<INPUT>` は **FILE パス** / **生の VER 文字列** / **`-` (stdin から VER 1 行読込)** / **`vcs:REV[:FILE]` または `vcs:<関数>(...)`** (VCS 経由で取得、[vcs: 入力](#vcs-入力) 参照) / **`cmd:<shell-command>`** (shell コマンド経由で取得、[cmd: 入力](#cmd-入力) 参照) のいずれかで、複数指定時は混在可能。

ヘルプは 3 段構成:

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
bump-semver vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]
bump-semver vcs get latest-release [--include-prerelease] [--repository REPO] [--json]
bump-semver vcs outdated FROM TO[..]                   # 派生ファイル sync check (単一ペア)
bump-semver vcs outdated -- FROM TO[..] -- FROM TO[..] [-- ...]   # 複数ペア
bump-semver vcs outdated [--explain] FROM TO[..]       # 診断表示 (常に exit 0)
bump-semver vcs outdated [--strict] FROM TO[..]        # リテラル FROM が 0 件 → exit 1
```

git/jj を抽象化した小さなヘルパー群 ([DR-0020](./docs/decisions/DR-0020-vcs-subcommands.md)): `vcs get` (read-only な事実取得)、`vcs is` (述語)、`vcs diff` (patch 出力。`-s/--name-status` で M/A/D サマリ、`-q/--quiet` で差分有無を終了コードで返す `git diff --quiet` 相当)、`vcs commit` (path 必須を基本としつつ `--staged` / `--amend` を持つ安全な commit)、`vcs fetch` / `vcs push` (ネットワーク側の counterpart。`--force` は意図的に非提供、non-ff は exit 5 で検出)、`vcs tag push` / `vcs tag delete` (create+push をアトミックに / delete は冪等。`--allow-move` を tag 移動の精密な opt-in として用意し、別 rev の整合性違反は exit 4 で表面化)。最新 version の取得は `vcs get latest-tag` / `vcs get latest-release` 配下 ([DR-0032](./docs/decisions/DR-0032-vcs-get-latest-by-source-verb.md)、source 軸を verb 名に畳む) で、`vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])` 入力 record が 1-liner ergonomic な等価形。動機: Taskfile / justfile で git と jj を毎回手書き分岐する板挟みの解消。`bump-semver` は既に `vcs:` で VCS read を吸収しているので、その自然な拡張として `vcs` サブコマンド群を同居させる。

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
| `--jj-bookmark-auto-advance` | **jj 専用の opt-in**。push 前に bookmark を「公開すべき commit」に自動で進める。clean な `@` (空 working copy) → bookmark を `@-` に。dirty な `@` (非空、通常は describe 済) → bookmark を `@` に。bookmark が存在しない場合は何もせず通常 push に委ね、`ancestors(@)` に居ない (sideways / divergent) 場合は exit 3 + hint で停止する (移動しない)。移動自体は forward-only (`--allow-backwards` は付けない)。git リポで指定した場合は silent no-op (= `--jj-` prefix が「jj 専用」を構造的に示すため git は無視)。**jj 0.39 以上が必要** — DR-0026 で bookmark の移動を jj 公式 `jj bookmark advance` (jj 0.39.0 で導入) に委譲し、bump-semver 側には clean/dirty target 選択と DR-0025 description check のみを残す。**Why**: jj 慣習では bookmark は確定 commit (`@-`) に置き、`@` は使い捨ての working copy。bump のたびに `jj bookmark move` を手で打つ摩擦を構造的に解消するためのフラグ |

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

**`vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]`** および **`vcs get latest-release [--include-prerelease] [--repository REPO] [--json]`** ([DR-0032](./docs/decisions/DR-0032-vcs-get-latest-by-source-verb.md)) — SemVer 最大の tag / GitHub Release を出力する。source 軸 (tag list か Release オブジェクトか) は verb 名に畳まれ、各 verb は単一の責務を持つ。1-liner ergonomic な等価形 `vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])` も入力 record として使用可能 ([vcs: 入力](#vcs-入力) 参照)。

| フラグ | デフォルト | 意味 |
|---|---|---|
| `--repository REPO` | cwd VCS / repo | 外部対象: `owner/repo` (GitHub 短縮、`https://github.com/...` に展開) or HTTPS/SSH フル URL。`latest-tag` は `git ls-remote --tags` (gh 不要)、`latest-release` は `gh release list -R` (gh 必須) |
| `--include-prerelease` | 除外 | pre-release tag (`v1.2.3-rc.1` 等) を含める |
| `--json` | bare SemVer | `get --json` と同一の 12-field schema (`{"name":..., "version":..., "semver":..., "major":..., ...}`)。`.version` は raw tag 文字列を保持、`.semver` は canonical bare 形、`.name` は `pkf-tasks@0.0.13` 形式の monorepo prefix を抽出 |

```bash
bump-semver vcs get latest-tag                       # cwd: 1.2.3 (bare SemVer)
bump-semver vcs get latest-tag --json | jq -r .version  # raw tag 文字列 (e.g. v1.2.3)
bump-semver vcs get latest-tag --include-prerelease  # v1.2.3-rc.1 も対象
bump-semver vcs get latest-tag --repository kawaz/pkf-tasks
                                                     # 外部 repo (git ls-remote --tags)
bump-semver vcs get latest-release                   # cwd repo: 最大 GitHub Release (gh 必須)
bump-semver vcs get latest-release --repository kawaz/bump-semver --json
                                                     # 外部 GitHub Release、structured output

# 入力 record 経路 (DR-0032 の 1-liner):
bump-semver compare gt VERSION 'vcs:latest-tag()'    # release 準備済? (1-liner)
bump-semver get 'vcs:latest-release(kawaz/pkf-tasks)' # 外部 repo の最新 Release
```

終了コード: `0` 成功; `2` usage (余分な positional); `3` VCS / gh subprocess エラー OR `latest-release` で `gh` 未インストール。

`--vcs jj|git|auto` は引き続き有効。colocated 構成で git 側を見たい場合は `bump-semver vcs get backend --vcs git` (または `vcs is git --vcs git`) で強制できる。

**`vcs outdated FROM TO[..]`** ([DR-0027](./docs/decisions/DR-0027-derived-sync-mini-dsl-and-regex-reject.md) / [DR-0028](./docs/decisions/DR-0028-glob-backref-spec-v0.1.0-adoption.md)、仕様 [glob-backref v0.1.0](./docs/specs/glob-backref-v0.1.0.md)) — predicate: 派生ファイル `TO` が正本 `FROM` 以上に新しいかを committer-timestamp で比較する (= 既存の翻訳 lag チェックを verb 化した形)。stale → exit 1。DR-0024 の `glob:` を拡張した mini-DSL: FROM 側の可変パーツ (`*` / `**` / `{a,b,c}` / `[abc]`) が **登場順** にキャプチャされ、TO 側で `$N` / `${N}` で参照できる。TO の `{a,b,c}` は **必須展開** (全展開先が存在しないと fail); TO の `*` / `**` / `[]` は **任意の filesystem 検出** (マッチ無しは silent skip)。`?` は MVP scope 外 (仕様 §2.1、v0.3+ で検討予定) で、pattern syntax error として reject される。`--explain` で `(source → derived)` の完全展開 + 鮮度ステータスを表示できる (= 診断モード、stale でも exit 0)。`--strict` で リテラル FROM が 0 件マッチ時に exit 1 (= デフォルトは警告のみで exit 0)。

| 観点 | 挙動 |
|---|---|
| FROM の形 | リテラルパス (`README.md`) または `glob:<pattern>` (複数 source)。キャプチャは source ごと |
| TO の形 | 1 ペアにつき複数指定可。各 TO は `$N` / `{}` / `glob:` / リテラル混在 OK。FROM 自身は自身の派生集合から自動除外 (per-source) |
| ペア区切り | ペア間は `--`。単一ペアなら `--` 省略可。N≥2 ペアでは各ペアの前に `--` 必須 |
| backref 番号 | `*` / `**` / `{}` / `[]` 各 1 つが `$N` の 1 番を消費 (登場順)。`$0` / `${0}` は match path 全体。N≥10 は `${N}` 必須、`$10` は ambiguous で reject。範囲外 N は空文字 |
| 鮮度判定 | `derived_ts < source_ts` で stale (既存翻訳 check と同方式)。未追跡は ts=0 → 「無限に古い」扱い |
| `**` のゼロセグメント | `**` は 0 セグメントマッチを許容、その場合 `$N` の値は `.`。`path.Clean` と組合せで `${1}/foo` が root マッチ時に `/foo` (絶対 path) に化けるのを防ぐ |
| TO `glob:` エスケープ | TO が `glob:` で始まる場合、キャプチャ値の glob meta は char-class で wrap される (`a*b` → `a[*]b`) ので 2 段目 walk で literal として扱われる。template 自体の `*` / `**` / `{}` / `[]` は glob として生きる |
| `--explain` | `source → derived [status]` 行を出力。status は `fresh` / `stale: N commit(s) behind` / `missing, will fail` / `untracked: derived has no commit ts` |
| `--strict` | リテラル FROM が 0 件 → exit 1 (= release-gate での typo 検知用)。デフォルトは警告だけ出して exit 0 (= 後方互換) |
| shell エスケープ | `$N` / `{}` / `--` は shell の特殊文字。FROM/TO は **必ず single quote** すること (bump-semver 側はエスケープ解釈しない、DR-0024 §10.7) |

```bash
# T1 bundle (TypeScript src/ → コンパイル先 lib/)。$1 = ** segment、$2 = *。
bump-semver vcs outdated 'glob:src/**/*.ts' 'lib/$1/$2.js'

# T2 翻訳 (1 正本に対し複数の必須派生)
bump-semver vcs outdated README.md 'README-{ja,en}.md'

# T3 codegen (proto/ → generated/、深いパスを保持)
bump-semver vcs outdated 'glob:proto/**/*.proto' 'generated/$1/$2.pb.go'

# 集約: 3 ペアを 1 コマンドで
bump-semver vcs outdated \
  -- 'glob:src/**/*.ts'       'lib/$1/$2.js' \
  -- README.md                 'README-{ja,en}.md' \
  -- 'glob:proto/**/*.proto'   'generated/$1/$2.pb.go'

# 診断: 完全展開 + 派生ごとの鮮度状況
bump-semver vcs outdated --explain 'glob:src/**/*.ts' 'lib/$1/$2.js'
# →
# src/foo.ts      →  lib/foo.js      [fresh: derived ts >= source ts]
# src/sub/bar.ts  →  lib/sub/bar.js  [missing, will fail]

# Release-gate: リテラル FROM の typo (= `README-ja.MD` 等) を exit 1 で検知
bump-semver vcs outdated --strict README.md 'README-{ja,en}.md'
```

`vcs outdated` の終了コード: `0` 全派生が fresh (または `--explain` モードで status に関わらず。引数なし `vcs outdated` は help を表示して exit 0); `1` 1 つ以上の派生が stale / missing / untracked、または `--strict` 指定でリテラル FROM が 0 件; `2` usage error (ペア形式不正、`$10` ambiguous、backref 形式不正); `3` VCS subprocess error (リポ外、等)。MVP 範囲外 (= spec v0.1.0、需要ベースで別 DR): `regex:` プレフィックス (= 明示的に却下、根拠は DR-0027 参照)、`{}` ネスト、`[^...]` complement char class、named capture `${name:pattern}`、`cmd:` GENERATOR スキーム、cross-source 自動除外、特殊文字を含む病的ファイル名。

### フラグ

| フラグ | 説明 |
|---|---|
| `--pre PRE`            | pre-release 識別子を設定 (例 `--pre rc.0`) |
| `--no-pre`             | pre-release を削除 |
| `--build-metadata META`| build metadata を設定 (例 `--build-metadata sha.abc`) |
| `--no-build-metadata`  | build metadata を削除 |
| `--write`              | bump 結果を各 FILE 入力に書き戻す (`major` / `minor` / `patch` / `pre` のみ) |
| `--vcs jj\|git\|auto`    | `vcs:` 入力の VCS を強制指定 (default: `auto`) |
| `--define-rule PATTERN` | `PATTERN` (絶対 / 相対 path、basename、`glob:<pattern>`) にマッチする SOURCE 用のカスタム抽出ルールを開く。[カスタムルール](#カスタムルール---define-rule) 参照 |
| `--format FMT`         | rule body: source の形式 `text\|json\|yaml\|toml\|xml` |
| `--version-path DOTPATH` | rule body: `json\|yaml\|toml\|xml` の version field path (例 `$.version`) |
| `--version-regex REGEX` | rule body: `text` 用の version regex (capture group ちょうど 1 個) |
| `--name-path DOTPATH`  | rule body: optional なパッケージ名 path |
| `--name-regex REGEX`   | rule body: optional なパッケージ名 regex |
| `--glob-dotfile`       | `glob:` で dotfile を含む (`=true` / `=false` 必須) |
| `--glob-gitignored`    | `glob:` で `.gitignore` を尊重 (`=true` / `=false` 必須、default true) |
| `--glob-ignorecase`    | `glob:` で大文字小文字を無視 (bare = true) |
| `--no-hint`            | 全 `hint:` 行を抑制 (fallback match / unsupported file / 「files not modified」) |
| `-q`, `--quiet`        | stdout と全 `hint:` 行を抑制 |
| `--quiet-all`          | stdout / hint / エラー出力をすべて抑制 (debug 時注意。`-qq` は `-q` の重ね指定として機能) |
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
| `cmd:<shell-command>` | shell コマンドを `bash -c` で実行し、stdout の最初の非空行を VER として取得 (read-only、[cmd: 入力](#cmd-入力) 参照) |
| `glob:<pattern>` | doublestar による glob 展開でマッチした path 群を展開 ([DR-0024](./docs/decisions/DR-0024-glob-prefix.md)、`vcs diff` / `vcs commit` / `vcs outdated` で利用可) |
| `file:<path>` | `<path>` の改行区切り path list を読み込み、`#` コメント / 空行スキップ、各行は literal or `glob:` shape ([DR-0033](./docs/decisions/DR-0033-vcs-excludes-and-file-prefix.md)、`vcs diff` で利用可) |

> **最新 tag / 最新 release の取得** ([DR-0032](./docs/decisions/DR-0032-vcs-get-latest-by-source-verb.md)): 入力 record と subcommand の両経路を提供。入力 record `vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])` は 1-liner ergonomic 向け (`compare gt VERSION 'vcs:latest-tag()'`、stable only)。subcommand [`vcs get latest-tag`](#vcs-サブコマンド) / [`vcs get latest-release`](#vcs-サブコマンド) は richer option (`--include-prerelease`、`--json` で 12 field version schema、`--repository REPO`) を備える。

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
| **3** | `VERSION` | text (regex なし) | (ファイル内容) | — |
| **2** (basename) | 任意 dir の `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | 任意 dir の `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | text + regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | text + regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | text + regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | text + regex | `version := "..."` | — |
| **2** | `build.gradle` (Gradle Groovy) [DR-0018] | text + regex | `version = '...'` / `version "..."` | — |
| **2** | `build.gradle.kts` (Gradle Kotlin DSL) [DR-0018] | text + regex | `version = "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | text + regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | text + regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | text + regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | text + regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.cabal` (Haskell) [DR-0018] | text + regex | `version: ...` (line-anchored) | `name: ...` |
| **1** (fallback) | `*.spec` (RPM) [DR-0018] | text + regex | `Version: ...` (capital V) | `Name: ...` |
| **1** (fallback) | `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) [DR-0018] | xml-element | `/Project/PropertyGroup/Version` | — |

未対応ファイル (例: `README.md`, `Cargo.lock`) は `unsupported file: <path>` で明示エラー。新フォーマット追加 = テーブル 1 行追加 (+ 必要なら新 format-specific 関数 1 つ) で済む構造 (`--pattern` regex フラグは設計上持たない)。

YAML / TOML fallback (DR-0011) は **top-level キーだけ**を見る。section 配下 / nested mapping 配下の `version` は意図的に対象外。`Cargo.toml` / `pyproject.toml` / `mojoproject.toml` は引き続き confidence-3 ルールが優先されるので、それぞれの section-scoped 挙動は不変。multi-document YAML (`---` 区切り) は最初の document のみ。これらの新ルールでも DR-0010 の fallback hint が出る (`--no-hint` で抑制可能)。

`pyproject.toml` ルール (DR-0014) は PEP 621 の `[project].version` を優先し、無ければ Poetry 旧形式の `[tool.poetry].version` を試行する (TOML format の OR semantics)。両方を持つ pyproject.toml (PEP 621 移行中の理論的中間状態) では最初の hit (PEP 621) のみ書き換えられる。`mojoproject.toml` ルール (DR-0014) は `[workspace].version` を直接読み書きする。両ルールとも共通の TOML section-scoped Replace を経由するので quote style と前後セクション・コメントは保持される。

`Cargo.toml` ルール (DR-0021) も同じ try-fallback 形を使う。シングルクレートの `[package].version` を先に試し、`[package]` を持たない workspace-root では `[workspace.package].version` (メンバー crate が `version.workspace = true` で継承する正本) にフォールバックする。両方を宣言するメンバー crate では crate 自身の `[package].version` が優先。マッチした path (`[package].version` か `[workspace.package].version`) は `get` / `--json` 出力に出るので、何の version を bump しているか常に確認できる。

`text + VersionRegex` ルール ([DR-0030](./docs/decisions/DR-0030-format-regex-to-text-unification.md)) は「version が 1 行のソースコード式で書かれる」8+ 言語マニフェスト (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt / build.gradle / build.gradle.kts / cabal / spec) をカバーする。**最初のマッチ 1 個** だけが読み書きされ、quote style と version 行末尾のコメントは保持される。

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

### カスタムルール (`--define-rule`)

builtin の表に無い SOURCE は、コマンドラインで抽出ルールを定義できる ([DR-0029](./docs/decisions/DR-0029-cli-user-defined-rule-phase1.md))。`--define-rule <PATTERN>` で rule block を開き、続く rule body フラグが次の `--define-rule` までその block に属する:

| フラグ | 意味 |
|---|---|
| `--format <FMT>` | `text` / `json` / `yaml` / `toml` / `xml`。`xml` は path 末尾セグメントを子要素と属性の両方に対して解決する (値一致 = ok、相違 = ambiguous) |
| `--version-path <DOTPATH>` | `json` / `yaml` / `toml` / `xml` 用: version field の場所 (例 `$.version`、`plugin.version`、`deps[0].version`) |
| `--version-regex <PATTERN>` | `text` 用: capture group ちょうど 1 個の regex (0 個 / 2 個以上はエラー) |
| `--name-path` / `--name-regex` | optional なパッケージ名抽出 (version 系と対称) |

`PATTERN` は絶対 path / 相対 path / basename / `glob:<pattern>`。match strength がスコア化され (絶対 5 / 相対 3 / basename 2 / glob 1)、最も具体的なルールが勝つ。最初の `--define-rule` より**前**に置いた rule body フラグは、named block 非カバーの全 SOURCE への global default になる。CLI ルールは常に builtin を上書きし、CLI ルールの抽出失敗は hard error (builtin への silent fall-through なし)。`get` / `compare` / bump 系 (`--write` 含む) で利用可能。

```bash
# tool 固有の JSON field から version を読む
bump-semver get plugin.json --define-rule plugin.json --format json --version-path '$.meta.version'

# 独自 prefix の後ろに version がある text file
bump-semver patch app.conf --write --define-rule app.conf --format text --version-regex 'VERSION=([0-9.]+)'
```

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

stdin がパイプ **かつ INPUT が単一の FILE のとき**、その FILE は名前ヒントとして扱われ、内容は stdin から読み込まれる (legacy ショートカット、後方互換)。パイプが空 (例: CI ランナーが step の stdin に writer 不在の FIFO を配線するケース) ならディスク上の FILE が読まれ、`--write` も通常通り効く。複数 INPUT のときは stdin pipe は無視される。ファイルをチェックアウトせずにリビジョン間で比較したい時に有用:

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
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'  # release 準備済? (1-liner 入力 record)
LATEST=$(bump-semver vcs get latest-tag); bump-semver compare gt Cargo.toml "$LATEST"  # 同じ、capture-then-compare (CI)
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
# 直近のリリースタグより上げてるか (1-liner、入力 record 経路)
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'

# 自分が main から遅れてないか (stale チェック)
bump-semver compare lt Cargo.toml vcs:origin/main

# 前コミットからバージョン変わってる?
bump-semver compare eq Cargo.toml vcs:HEAD~1            # FILE は相手から借用
bump-semver compare eq Cargo.toml vcs:HEAD~1:Cargo.toml # 明示形式

# 他リポの最新 release tag を取得
bump-semver compare ge 0.0.13 'vcs:latest-tag(kawaz/pkf-tasks)'  # 現 pin は最新追従済?
# または: bump-semver get 'vcs:latest-release(kawaz/pkf-tasks)' (GitHub Releases、gh 必須)
```

| 形式 | 解釈 |
|---|---|
| `vcs:REV[:FILE]` | `<REV>` 時点の `<FILE>` を VCS から読み出す。最初の `:` は `vcs:` プレフィックス、2 つ目の `:` で REV と FILE を分割。FILE 省略時は位置順で最初の sibling (FILE 起源 or `vcs:REV:FILE` 形式) から借用 |
| `vcs:latest-tag([REPO])` | cwd VCS (`REPO` 省略) または外部リポ (`owner/repo` 短縮 / フル URL) の最大 stable SemVer tag。prerelease は除外固定 (入力 record subset)。prerelease 含めたい / JSON 欲しい場合は [`vcs get latest-tag`](#vcs-サブコマンド) subcommand を使う |
| `vcs:latest-release([REPO])` | 最大 stable GitHub Release (draft は除外)。gh CLI 必須。`vcs:latest-tag()` と同じ subset 制約 |

> latest-tag / latest-release は上記の symmetric な 2 経路で提供される ([DR-0032](./docs/decisions/DR-0032-vcs-get-latest-by-source-verb.md)): 1-liner ergonomic 向けの scalar-returning 入力 record + 詳細 option 向けの [`vcs get latest-{tag,release}`](#vcs-サブコマンド) subcommand (source 軸を verb 名に畳む)。

**VCS 自動判定** (優先順):

1. `--vcs jj|git` フラグ (`auto` / 未指定は次へ)
2. cwd または親に `.jj` ディレクトリ → jj
3. `.git` ディレクトリ → git
4. それ以外 → エラー (`not a git or jj repository`)

`.jj` と `.git` が並存している場合 (jj colocate モード、kawaz の git-bare + jj-workspace 構成) は **jj が優先**。jj の revset 言語は git ref のスーパーセットなので。

> 旧バージョン (v0.12 以前) では `BUMP_SEMVER_VCS=jj|git` 環境変数がフラグの次の優先位にあったが、v0.13 で廃止された ([DR-0016](./docs/decisions/DR-0016-remove-bump-semver-vcs-env.md))。CI / 開発環境で env を設定していた場合は `--vcs jj|git` フラグへ置き換える。

**`--write` と `vcs:` は排他**。VCS の中身に書き戻す機能は持たない (commit/amend が必要になりスコープ外)。混在させると `--write cannot be used with vcs: inputs (vcs: is read-only)` エラー。

**`bump-semver` は `git fetch` / `jj git fetch` を自動実行しない**。`vcs:origin/main` が古い場合は VCS 側のエラーがそのまま伝わる。CI では明示的に fetch してから bump-semver を呼ぶ運用にする。

CI スクリプトを VCS 中立にしたい場合は jj/git どちらでも通る形式 (`origin/main` (jj 側で `main@origin` に自動フォールバック) / commit hash) を推奨。「最新リリースタグ」は [`vcs:latest-tag()`](#vcs-入力) (入力 record、1-liner) または [`vcs get latest-tag`](#vcs-サブコマンド) (subcommand、詳細 option) を使う — jj/git 自動判定は `vcs:` 入力と同じ。

### cmd: 入力

`cmd:<shell-command>` は `<shell-command>` を `bash -c` で実行し、stdout の最初の非空行を VER として取得する read-only 入力。leading `v` は strip され、SemVer 2.0.0 として parse される。

```bash
# ビルド済みバイナリの --version 出力と VERSION ファイルが一致してるか?
bump-semver compare eq VERSION 'cmd:./bin/mytool --version'

# 外部ツールの version を取得 (VCS tag list の届かない場所)
bump-semver get 'cmd:brew info --json mytool | jq -r .[0].installed[0].version'

# 別の bump-semver 呼び出し結果との比較も可
LATEST=$(bump-semver vcs get latest-tag)
bump-semver compare gt 'cmd:bump-semver get Cargo.toml' "$LATEST"
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

## シェル補完

```bash
bump-semver completion <bash|zsh|fish|powershell>
```

指定したシェル用の補完スクリプトを生成する (インストール手順は `bump-semver completion <shell> --help` を参照)。例: `bump-semver completion zsh > "${fpath[1]}/_bump-semver"`。

## 機能

現状の機能一覧:

- **bump / read / compare**: `major` / `minor` / `patch` / `pre` / `get`、および precision grid 5 × 4 = 20 OP の `compare`。
- **basename による形式自動判定**: TOML / JSON / YAML / XML マニフェスト + 多数の text+regex 形式 (Cargo / npm / PEP 621 / Maven / Gradle / .NET / Xcode 等) に対応し、backup suffix の fallback も持つ。
- **カスタムルール**: builtin の表に無い SOURCE 用に `--define-rule`。
- **柔軟な入力**: FILE / 生 VER / `-` (stdin) / `vcs:` (jj or git から読む) / `cmd:` (shell コマンドから読む) / `glob:` / `file:`。
- **`vcs` サブコマンド**: git/jj を抽象化したヘルパー群 (`get` / `is` / `diff` / `commit` / `fetch` / `push` / `tag` / `get latest-tag` / `get latest-release` / `outdated`)。
- **構造化出力** (`--json`) と bash / zsh / fish / powershell のシェル補完。

version ごとの完全な履歴は [CHANGELOG.md](./CHANGELOG.md)。設計判断は [docs/decisions/](./docs/decisions/)、将来検討項目は [docs/ROADMAP.md](./docs/ROADMAP.md) を参照。

## builtin に無い形式が必要なら

経路は 2 つ:

- **待たない** — [`--define-rule`](#カスタムルール---define-rule) で自分で抽出を定義する。即座に動く。
- **みんなのために builtin 化** — [Built-in format request](https://github.com/kawaz/bump-semver/issues/new?template=format-request.yml) を起票する。スコープと PR ガイドラインは [CONTRIBUTING.md](./CONTRIBUTING.md) を参照。

## ライセンス

[MIT](LICENSE)
