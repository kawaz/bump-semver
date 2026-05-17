# DR-0020: `vcs` サブコマンド群 (git/jj を吸収するリリース/push 定型操作)

- Status: Active (未実装、ROADMAP 参照)
- Date: 2026-05-30

## Context

bump-semver は version bump/比較に加え、`vcs:` 入力モード (DR-0008 / DR-0016 / DR-0019) で **VCS からの read** を既に責務に持つ (`vcs:main@origin:file`, `vcs:latest-tag()` 等)。

リリースや push 周りの定型処理を Taskfile / justfile に書くとき、毎回「jj/git を判定して分岐する長いスクリプトを堅牢に書く」か「妥協して雑にシンプルに書く」かの板挟みが頻発する。bump-semver の開発動機はまさに **「複数プロジェクトで共通する、複雑になりがちだが頻出するパターンを、意図通りに堅牢かつシンプルに行う」** ことであり、その対象には version 操作だけでなく **VCS 判定・使い分け** も含まれる。

そこで、git/jj の差を吸収する `vcs` サブコマンド群を導入する。これは独立ツールでも pkf 側でもなく bump-semver に置く (動機が「1 ツールへの集約による板挟み解消」であり、既に `vcs:` で VCS read を持つため自然な拡張)。

## Decision

### スコープと設計哲学

`vcs` サブコマンド群は以下に厳格に留める:

1. **git/jj に共通する最小サブセット ∩ ユーザ意図との齟齬が生まれない範囲**。タスクランナー等の定型処理パターン用。複雑なことは各自が git/jj を直接使う
2. **安全のための意図的制約は、素の git/jj より厳しくしてよい** (例: commit の path 必須＝巻き込み事故防止)。単なる薄いラッパーではなく「定型処理で事故らないよう安全側に倒した opinionated な共通レイヤ」
3. **採用は網羅基準でなくフロー駆動**。リリース/push フローで必要になったら足す。先回りで網羅 API を作らない (YAGNI)

### サブコマンド一覧 (確定形)

```
vcs get <key>                         # 値を stdout。key: root | backend | current-branch
vcs is <pred>                         # 述語。exit 0=真 / 1=偽 / 2+=エラー。共通述語のみ (clean|dirty|git|jj…)
vcs diff REV [PATH..]                  # 差分無しは exit0 補償、存在しない PATH は無視 (エラーにしない)
vcs commit -m MSG PATH..               # 指定 path の現内容を記録 (git の add は自動・透明)
vcs commit -m MSG --staged             # staged 全部 (git: staged / jj: カレント全部=自動staged)
vcs commit --amend [-m MSG] [PATH..|--staged]   # 直前単位に吸収 (-m 無し=no-edit)
vcs fetch [REMOTE]
vcs push --branch|--bookmark NAME [--remote origin]        # force 無し
vcs tag push --rev REV NAME [--remote origin] [--allow-move]
vcs tag delete NAME [--remote origin]  # 冪等 (不在でも成功)
```

### 各サブコマンドの仕様

**`get`** — 値取得 (stdout):
- `root`: VCS リポジトリのルートパス
- `backend`: `git` / `jj` / `none`
- `current-branch`: git=現在の branch、jj=`ancestors(@) ∩ bookmarks()` の最近接。**git/jj とも「一意に定まる時のみ返す。曖昧・不在は全てエラー」**。git は DETACHED HEAD / merge・rebase・cherry-pick・bisect 進行中を拒否。jj は最近接 bookmark が 0 個・同率複数・半順序で比較不能なら拒否 (merge による親分岐自体はエラー要因でなく、候補の一意性が基準)

**`is`** — 述語 (exit code):
- exit `0`=真 / `1`=偽 / `2+`=エラー。**未知述語 (typo) は silent に偽でなく必ず exit2+stderr** (誤分岐防止)
- 述語は **両 VCS ユーザーが共通理解できるものに限定** (clean / dirty / git / jj / 将来 ahead / behind 等)。jj 固有概念 (`@` empty 等) は入れない (可搬性維持)
- `clean` の定義は「コミットすべき未記録変更があるか」を意図ベースで固定 (untracked の扱いを明文化)
- ガード系 (`assert-clean` 等) は提供しない。`if ! vcs is clean; then …` で利用者が組む

**`diff REV [PATH..]`**: REV→現在の PATH 変更。差分無しでも exit0、存在しない PATH は無視 (エラーにしない)。`check-version-bumped` 等での pathspec エラーを根本回避

**`commit`**:
- **path 必須が基本** (＝巻き込み事故防止のあえての制限。jj はカレントコミット≒working tree で並列作業時に他ファイルを巻き込みやすいため)。指定 path の **working tree 内容を自動 stage して commit、指定外は一切触らない** (git の `add` は透明)
- `--staged`: staged 集合を全部 commit (git: staged / jj: 全変更=自動 staged)。`commit -a` 相当 (unstaged 巻き込み) は提供しない (unstaged は jj に無くスコープ外)
- `--amend`: 直前単位に吸収。`-m` 無し=no-edit (元メッセージ保持)、`-m` あり=差し替え。メッセージのみ amend (`--amend -m MSG`、path/--staged 無し) は巻き込みが無いので path 不要で許容
- **不在 PATH は無視** (エラーにしない＝`diff` と同じ規律)。指定 path のうち **存在し変更があるものだけ** commit。これにより「version 更新で触りがちな well-known ファイル (`src package.json Cargo.toml VERSION` 等) を言語を問わず利用者が自分で羅列し、あればコミット・なければスルー」という怠惰パターンが成立する (個人の再利用スニペット用途。プリセット内蔵はしない＝不採用案 11)
- **commit 対象が結果 0 個** (path 全不在 or 全部変更なし) → **冪等成功 exit0** (git の "nothing to commit" exit1 には倣わない＝「コミットすべき状態に既に収束済み」)。typo 等で miss commit しても変更は working tree に残り、下流の `is clean` / ensure-clean で別途検出されるため、ここを成功にしても見逃しは生じない
- 引数なし commit (path も `--staged` も無い＝使用法エラー) → エラー＋**環境適応 hint** (git なら `--staged`、jj なら path を案内)

**`push --branch|--bookmark NAME`**:
- NAME **必須** (自動推測しない＝jj の branch レス性で「現在 branch を推測する」場面を作らない、曖昧さ排除)
- **force 無し** (ff/安全 push のみ。non-ff は失敗、複雑な push は素の git/jj)。non-ff エラー文面で「複雑な push は git/jj を直接」と誘導
- 内部で **push → `jj git export` をセット実行** (非 colocated は export が自動で走らないため backing git の branch を最新化)。**export は失敗しうる (jj #493/#6098/#6203) ので exit code を確認し失敗を握りつぶさず報告**

**`tag push --rev REV NAME`**:
- **必ず push とセット** (ローカル単独 tag は作らない＝「tag=リモートに在る」を不変条件化)
- `--rev` 必須。REV は push 済み ref (branch 名可) を指定。**immutability 判定は「REV が remote に在る commit に解決されるか」で git/jj 統一** (push 後なら immutable)。mutable なら エラー＋hint
- 作成は **jj では `jj tag set` (jj v0.35+) を使い、push は native git** (jj に tag-push ネイティブ手段が無いため)。git では `git tag` + `git push`。jj tag set 経由で **jj が tag を把握し ref state 乖離を抑える** (B 案)
- **宣言的収束**: 同名 & 同 rev → `--allow-move` 不要で冪等成功 (片落ちリカバリが摩擦なく動く)。同名 & 別 rev → エラー、`--allow-move` で移動許可

**`tag delete NAME`**:
- ローカル+リモート両方から削除 (push とワンセット)。**デフォルト冪等** (不在でも成功＝`rm -f` 類推。delete の意図は「無い状態への収束」で不在は目的達成済み)。`--allow-missing` 等の flag は不要

### 横断する設計原則

- **get/is の二分**: 値が欲しい→`get` (stdout)、真偽で分岐→`is` (exit code)
- **命名規律**: ① 両 VCS で共通理解できる語彙は単一語 (`--staged`) ② VCS 固有語彙は canonical + alias + 注釈 (`--branch`/`--bookmark`、help に「jj では bookmark」注釈) ③ 許可フラグは `--allow-X` (例 `--allow-move`)
- **help は環境非依存 (契約・共有・補完の対象なので常に同一) / エラー・hint は文脈適応 (実行時ガイドなので cwd の VCS に寄せてよい)**
- **宣言的収束 + 曖昧はエラー**: 望む最終状態への収束を冪等に。曖昧・期待外は基本エラー、解消はユーザ責務。**ただし「対象の不在」が目的と矛盾しない操作は冪等成功** — delete の対象不在 (無い状態への収束)、diff/commit の PATH 不在 (その path に用が無いだけ) は無視する。一方 `current-branch` 取得等「不在＝曖昧で危険」な read 系は従来通りエラー

## Rationale

### 不採用案

1. **独立 `vcs` ツール / pkf 側へ分割** — bump-semver の動機が「1 ツールへの集約で板挟み解消」であり、既に `vcs:` で VCS read を持つ。分割は動機に反する
2. **`--all` (commit 全部モード)** — git の `commit -a`/`--all` と紛らわしく、unstaged も巻き込むと誤読される。`--staged` は誤読しない
3. **`--all-in-staged` / `--all-in-commit` 併記** — 冗長かつ help 汚染。jj ユーザーは「カレントコミット=自動ステージ」を理解しているので `--staged` 単一語で共通理解でき、併記不要
4. **`squash` 独立サブコマンド** — commit とオプション体系が完全同一で、差は「新規打つ/直前吸収」の 1 点。別コマンドは丸ごと重複。`commit --amend` フラグが DRY
5. **`tag push --force`** — 意図が曖昧 (冪等リカバリと別 rev 移動を無差別上書き)。`--allow-move` で「同 rev 冪等は無条件 / 別 rev 移動のみ明示許可」に分離でき、より安全
6. **`tag delete --allow-missing`** — delete は本来冪等であるべき (`rm -f`)。flag を足すのは過剰
7. **`push` の force / `--force-with-lease`** — 履歴破壊 (branch force) はスコープ外。non-ff は失敗させ、複雑な push は素の git/jj に委ねる。なお tag force を排除しないのは「tag=ラベル移動で commit 履歴は無傷」と質が違うため (`--allow-move` として限定提供)
8. **動的 help visibility (cwd の VCS で `--branch`/`--bookmark` を出し分け)** — 環境で help が変わると共有時に齟齬、completion が環境依存になる。help は環境非依存併記＋注釈、エラー/hint のみ文脈適応
9. **tag を backing git に直接 `git tag` (A 案)** — jj v0.35+ の `jj tag set` (B 案) なら jj が tag を把握し、直接 git 操作による ref state 乖離・HEAD 競合 (jj #6098 系) を抑えられる。「新しいツールを使うなら、そのツール出現以前の時代はサポート不要」として jj v0.35+ を前提に B 案採用
10. **採用基準を網羅的に先決め** — over-engineering。リリース/push フローで必要になったら足すフロー駆動が正しい門番
11. **commit のデフォルト version-path プリセット内蔵** — 「言語問わず well-known な version ファイル群を bump-semver 側が内蔵し `vcs commit --defaults` で一発」案。だが (a) 言語/プロジェクト構成は多様で「正しいデフォルト集合」を bump-semver が責務として抱えるのは過剰、(b) 不在 PATH スルーがあれば利用者は自分のプロジェクトに合うリストを羅列するだけで同じ怠惰運用が成立する。プリセットは利用者の Taskfile/justfile 側の関心事 (YAGNI)。「個人の再利用スニペットとしては有用だが、ツール組み込みとして提供する筋ではない」と判断

### 設計上のポイント

- **path 必須 = 安全装置**: jj はカレントコミット≒working tree で、並列作業 (複数エージェント等) 時に他ファイルを同一コミットに巻き込みやすい。path 必須は両 VCS 共通の巻き込み事故防止であり、素の git/jj より厳しくする意図的制約
- **immutability = 「remote に在るか」で統一**: git の remote-tracking も jj の `immutable_heads()` も結局「push 済みか」に帰着する。tag 対象を「push 後 = immutable」で判定することで git/jj の immutable 定義差を吸収
- **tag を打つと jj 上 immutable 化**: jj は lightweight tag 1 つで対象 commit を immutable 扱いにする。確定リリース commit に打つ前提なら、むしろ「以後 amend させない」安全装置として機能する
- **`jj git export` は失敗しうる**: colocated が毎コマンド自動 export している＝頻繁な自動 export 自体は安全側だが、ref 階層衝突・HEAD 競合・packed-refs 不具合で失敗する。push 後 export をセットにする実装では exit code チェック必須
- **bare の扱い**: bare git は backing store として正規サポート (`jj git init --git-repo=<bare>`)、colocated 形態だけが不可 (working copy 共有が定義なので bare と両立しない)。kawaz の git bare + jj workspace は非 colocated backing として正規サポート

## Consequences

- 利用側 (Taskfile / justfile) は `is-jj`/`is-git` 分岐や jj/git の dirty 判定・diff の差を手書きする必要がなくなる。`check-version-bumped` の pathspec エラーや origin 比較の fetch 漏れも `vcs` 側で吸収できる
- 未実装。実装着手時は `vcs get` / `vcs is` / `vcs diff` (read 系、leaky 少) から、続いて commit/push、最後に tag (jj export・immutability 連動) の順が無難
- jj は **v0.35+ を最小サポートバージョン**とする (`jj tag set` 依存)
- **実機検証推奨マトリクス** (empirical-verification 方針): `jj git init --git-repo=<bare>` → `jj tag set` → native `git push` tag → `jj git import`/`export` の挙動を、対象 jj バージョンで確認

## 関連

- 上位/関連 DR: DR-0008 (`vcs:` schema 導入)、DR-0016 (`--vcs auto` 一本化)、DR-0019 (`vcs:latest-tag(<arg>)`)
- 設計議論の経緯 + jj 一次情報調査: `docs/journal/2026-05-30-vcs-subcommands-design.md`
- ROADMAP: `docs/ROADMAP.md` (vcs サブコマンド群の実装項目)
