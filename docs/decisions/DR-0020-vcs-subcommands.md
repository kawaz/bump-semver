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
- PR-1〜PR-6 + PR-4.1 + PR-5.1 + PR-5.2 + PR-5.2.1 + PR-2.2 land 済 (`vcs get` / `vcs is` (canonical empty 判定 = PR-2.2) / `vcs diff` (+ `-s`/`-q`) / `vcs commit` (+ amend symmetry refinements) / `vcs fetch` + `vcs push` (+ hint simplification / jj export retry / passthrough / `--jj-bookmark-auto-advance` opt-in による jj bookmark 自動移動 + backend-prefix general rule = `--jj-*`/`--git-*` flag は他 backend で silent no-op) / `vcs tag push` + `vcs tag delete` (atomic create+push / 冪等 delete、`--allow-move` opt-in、別 rev integrity 違反は exit 4))。残りは PR-7 (移行+docs)
- jj は **v0.35+ を最小サポートバージョン**とする (`jj tag set` 依存)
- **実機検証推奨マトリクス** (empirical-verification 方針): `jj git init --git-repo=<bare>` → `jj tag set` → native `git push` tag → `jj git import`/`export` の挙動を、対象 jj バージョンで確認

## 実装ノート (PR-1 着手時に確定、2026-05-30)

設計確定後、実装着手時に詰めた運用ガードレール:

- **PR 分割**: 7 PR (PR-1 基盤+`vcs get` / PR-2 `vcs is` / PR-3 `vcs diff` / PR-4 `vcs commit` / PR-5 fetch+push / PR-6 tag / PR-7 移行+docs)。PR-1 で `vcsBackend` interface (`Kind` / `Root` / `CurrentBranch`) と git/jj backend を導入し、後続 PR は同じ pattern を踏襲する
- **exit code 規約**: `0` 成功 / `1` 偽 (`compare` + 将来の `vcs is`) / `2` usage / `3` VCS 実行エラー / `4` 曖昧 / `5` non-fast-forward push (`vcs push` 用に予約)。`src/exit.go` の定数で 1 箇所管理
- **`vcs is clean` の untracked 扱い** (PR-2 で実装): **除外** (tracked のみで判定)。`--include-untracked` は当初予約案だったが、PR-2 で **interface 引数を持たず `IsClean() (bool, error)` 形** (no-param) に確定。YAGNI: 「将来仮の要件のために dead な引数を生やさない」(`design-thinking.md` ルール準拠)。将来 untracked を含めたい場面が出たら別メソッド (例: `IsCleanIncludingUntracked()`) を追加する形で対応する
- **jj 対応範囲**: **v0.35+** をサポート (`jj tag set` 依存)。CI matrix は `0.35` / `0.41` / `latest` の 3 種を目指す (v0.41 が手元の primary バージョン)
- **`vcs get current-branch` 一意性**:
  - git: `git symbolic-ref --short HEAD`。DETACHED HEAD は exit 4 (merge / rebase / cherry-pick 進行中の追加判定は次 PR で `.git/MERGE_HEAD` 等を probe して足す)
  - jj: `heads(::@ & bookmarks())` の template で名前を集める。0 件 / 複数件 はいずれも exit 4
- **`vcsKind` (DR-0008/0016) との一本化**: PR-1 で実施済。`vcsBackend` interface に `Kind` / `Root` / `CurrentBranch` (新規) に加えて `FetchFile` / `ListTags` / `LatestTag` を載せ、`vcsFetchFile` / `vcsListTags` / `vcsLatestTag` 等の free function は廃止 (= 中身は backend メソッドに移管)。`resolveVcsInput` / `resolveVcsFunc` は `vcsBackend` を受け取る形に書き換え、`resolveInputs` も `detectVcs` ではなく `newVcsBackend` で backend を取得する。`vcsKind` は `--vcs jj|git|auto` の override-spec 型として残し、`parseVcsOverride` と `detectVcs` (probe-only) も継続。`vcsListTagsRemote` は `latestTagFromRemote` (常に `git ls-remote` を叩く free function) に名称変更 + 統合。journal の「移行は PR-7」は本 DR 確定方針 (一本化) で上書き済 — PR-1 段階で一本化完了
- **Cargo workspace.package 補完 (DR-0021)** とのリリース順序: DR-0021 が patch リリース (v0.16.2) で land 済み。本 DR の PR-1 land 時は **minor bump** で次バージョンに乗せ、欠けがちな minor リリースのリズムも揃える

### PR-2 (vcs is) 実装メモ (2026-05-30 確定)

- **述語ラインナップ**: `clean` / `dirty` / `git` / `jj` の 4 つで PR-2 を終える。`ahead` / `behind` 等は fetch とセットで初めて意味を持つので PR-5 以降。jj 固有概念 (`empty @`、`mutable_heads()` 等) は持ち込まない (DR-0020 「両 VCS ユーザが共通理解できるもの」原則)
- **未知述語は exit 2**: `vcs is wibble` は silent false にせず必ず usage エラーで止める。typo による誤分岐 (= 「無いから偽 → else 枝に飛んで本来禁止された操作実行」) の防止
- **predicate-false は silent**: `compare` と同じく stderr に何も出さず `*exitErr{code: 1}` のみ。シェルの `if cmd; then ...` / `&& chain` をノイズ無しで回せる
- **`is git` / `is jj` の「リポ外」挙動**: backend を build した時点で「リポではない」エラー (exit 3) を伝播し、**false (exit 1) には堕とさない**。「git じゃない」と「そもそも VCS じゃない (= 答えられない)」を区別。DR-0020 「曖昧・期待外はエラー」原則の適用
- **`IsClean` 実装**:
  - git: `git diff --quiet` (unstaged) AND `git diff --cached --quiet` (staged) を両方確認。どちらか exit 1 が出れば dirty。**untracked は除外** (= `git diff` 既定挙動を踏襲)。`exec.Cmd.Output()` だと exit 1 を error 扱いしてしまうため、新規 helper `runBackendExitCode` を追加 (exit code を error 区別して返す)
  - jj: `jj log -r @ --no-graph -T 'empty'` で `true` / `false` を直接読む。template keyword の boolean を文字列で受け取る。jj は read で自動 snapshot するため、新規ファイルも `@` に取り込まれ → dirty 扱い (= git との非対称、意図的)
  - ※ PR-2.1 (v0.25.2) で template を `if(parents.len() > 1, "true", empty)` に置換し evil merge を clean 扱いに変えたが、kawaz 意図の誤読だったため PR-2.2 (v0.25.3) で完全 revert。本 PR-2 実装 (`empty` 単体) が canonical。詳細は下記 PR-2.1/PR-2.2 セクション参照
- **`runVcsCmdIs` 配線**: `parseArgs` の `isKnownVerb` に `"is"` を追加、`runVcsCmd` の switch に case 追加、`actionHelpTexts["vcs is"]` 登録の 3 点。これで `vcs is` / `vcs is --help` / `vcs is <pred>` がそれぞれ help / dispatch される
- **PR-1 で導入した helper の再利用**: `emitVcsUsage` (exit 2 + stderr)、`emitVcsErr` (backend 由来 exit を保持しつつ stderr 出力) はそのまま流用。predicate-false だけは「stderr 何も出さない」要件のため helper を通さず `&exitErr{code: exitCodeFalse}` を直接 return

### PR-2.1 (vcs is clean — マージコミット対応、誤読により revert) メモ (2026-05-31 訂正)

- **訂正**: PR-2.1 (v0.25.2) は kawaz 意図の **誤読** だった。kawaz の発言「マージコミット自体は意味があり存在して良い為、clean 判定に問題があると考えるべきです。empty change の条件に親コミット数も含めるべきでしょ」を「evil merge も clean 扱いすべき」と解釈して `if(parents.len() > 1, "true", empty)` 短絡を入れたが、kawaz の正しい意図は「**empty merge は元から clean で合っている** (jj の `empty` template が parent-relative で、merge の tree が親群のマージと一致すれば empty=true になる)」というもの。evil merge (parents>1, non-empty) は **dirty が正解**
- **PR-2.2 (v0.25.3) で完全 revert**: template を `empty` 単体に戻す (PR-2 元実装)。`TestJjBackend_IsClean_MergeNonEmpty` の assert は「clean」→「dirty」に逆向き修正。`TestJjBackend_IsClean_MergeEmpty` は PR-2 元実装でも pass するため維持 (= empty merge → clean を pin する regression test として残す)
- **判定マトリクス (PR-2.2 後 = canonical)**:
  - 通常 empty (parents=1, empty=true) → clean
  - 通常 non-empty (parents=1, empty=false) → dirty
  - merge empty (parents>1, tree == merge-of-parents → empty=true) → clean (`empty` template の parent-relative 仕様で自然に clean)
  - merge non-empty / evil merge (parents>1, empty=false) → **dirty** (一時 PR-2.1 で clean に flip していたものを PR-2.2 で revert)
- **学び**: kawaz の指摘「empty change の条件に親コミット数も含めるべき」は「**empty 判定が merge を考慮できているか確認しろ**」という意味であり、`empty` template の parent-relative 仕様 (jj 一次情報) を確認したら問題ないことが判明する話だった。「条件を追加する変更が要る」と早合点した
- **PR-7 ハマり所との関係 (再確認)**: PR-7 の subagent が遭遇した「VERSION 更新分が dirty と判定されて `vcs:ensure-clean` で落ちた」ケースは parents=1 の non-empty commit (通常の編集 commit) であり、**正常動作**。`jj new` で empty @ を作って commit する対応は適切。PR-2.1 はこのケースとは無関係な誤判断だった

### PR-2.2 (PR-2.1 完全 revert) 実装メモ (2026-05-31 確定)

- **revert スコープ**: PR-2.1 で変更した全 commit (`fix(vcs)` / `test(vcs)` / `docs(help)` / `docs(readme)` / `docs(dr-0020)`) の意味を巻き戻す。実体は hand-edit (= `jj revert` ではない: `MergeEmpty` test は維持、`MergeNonEmpty` test は assert を反転、ドキュメントは PR-2.1 を「誤読として revert」と訂正する形で残す)
- **判定ロジック**: `jjBackend.IsClean` の template を PR-2 元の `empty` 単体に戻す。switch ブランチと "unexpected output" メッセージ文字列も PR-2 元 (`jj log -r @ -T empty: unexpected output %q`) に復旧
- **テスト**: `TestJjBackend_IsClean_MergeNonEmpty` の assert を「IsClean = true want false (evil merge: parents>1 with extra tree edits is dirty)」に変更。`TestJjBackend_IsClean_MergeEmpty` は無変更で維持 (empty merge → clean を pin)。`jjMergeFixture` も再利用継続
- **ドキュメント**: help.go / README / README-ja から PR-2.1 の merge 言及を削除して PR-2 表現に戻す。本 DR は PR-2.1 セクションを「誤読により revert」と訂正する形で残し、新規 PR-2.2 セクション (本セクション) で経緯と canonical な判定マトリクスを記録 (削除ではなく訂正 — `claude-config-dir-isolation` でいう「片面確認」を避けるため、誤った判断と訂正の両方を grep 可能な形で残す)
- **PR-2.2 land 日**: 2026-05-31

### PR-3 (vcs diff) 実装メモ (2026-05-30 確定)

- **コマンド形**: `vcs diff REV [PATH..]`。`argv[0] = REV`、`argv[1:] = paths`。バックエンド統一: git は `git diff REV [-- paths..]` (one-rev 形 = REV vs working copy、未コミット変更含む)、jj は `jj diff --from REV --to @ [-- paths..]`。両者は同じ「REV vs 作業ツリー」セマンティクスを持つ
- **two-rev 形を選ばなかった理由**: `git diff REV HEAD` は HEAD 同士の比較になり未コミット変更を見落とす。`vcs diff` は「現状と REV の差を見たい」用途を想定しているため one-rev 形を採用、jj 側 (`--to @`) と統一できる
- **PATH の宣言的収束ルール**: 存在しない PATH は `os.Stat` でフィルタして黙ってドロップ。kawaz の確定方針 (= 「指定して無くても消えてれば消えた状態に収束」)。`len(paths) > 0 && len(filtered) == 0` のときは backend を呼ばず即 `nil, nil` を返す (この vacuous case を経由しないと `git diff REV --` が「全 path 対象」に broaden して意味が変わる)
- **PATH の見落とし制約 (意図的)**: `REV` には存在するが working copy で削除済みのファイルを **PATH で明示指定** した場合、`os.Stat` でフィルタ落ちして diff に出ない。PATH 無指定の full diff なら削除も含めて表示される — この非対称は宣言的収束のスコープ内の意図的挙動 (= 「いま無いものを名指しで diff する」と「全体を見る」を区別)。将来要件が出たら別フラグ (`--include-deleted` 等) で opt-in する
- **エラーマッピング**: `runBackendCmd` (exit code を error 扱いする系) を使う。`git diff` / `jj diff` は plain run では「diff 有無に関わらず exit 0」「実エラー時のみ非 0」なので `--quiet` / `--exit-code` は付けない。実エラー (REV 解決不能、リポではない) は `*exitErr{code: exitCodeVCSExec}` で wrap → exit 3
- **空 diff と error の区別**: 「diff なし」は空 bytes + nil error、「REV 不能」は exit 3。stdout への書き込みは `len(out) > 0` ガード (空 bytes でも余計な書き込みを避ける)。`-q` / `-qq` は stdout を抑止
- **配線 3 点 (PR-2 と同パターン)**: `parseArgs` の `isKnownVerb` に `"diff"` 追加、`runVcsCmd` の switch に case 追加、`actionHelpTexts["vcs diff"]` 登録。`vcs diff` (引数なし) は per-verb help にフォールバックする既存仕様に乗る
- **interface 拡張**: `vcsBackend.Diff(rev string, paths []string) ([]byte, error)` を追加。`filterExistingPaths` は git / jj 共通の helper として `vcs_backend.go` に集約 (両 backend が同じ規則で path をフィルタする)
- **PR-3 land 日**: 2026-05-30

### PR-3.1 (vcs diff -s/-q) 実装メモ (2026-05-30 確定)

`check-version-bumped` 等のユースケース (claude-plugin-reference の survey から派生) で「`vcs diff` の素の patch を毎回パースするのは重い、もう少し軽い差分有無判定が欲しい」という要求が浮上。当初は新 verb (`vcs is changed REV [PATH..]`) を増やす案も検討されたが、kawaz の確定で **`vcs diff` の verb-local オプション 2 つ (`-s`, `-q`)** に集約。

- **`-s` / `--name-status`**: 出力を 1 ファイル 1 行の `<CODE>\t<path>` 形式 (M/A/D) に切替。git は `git diff --name-status REV [-- PATHS]` をそのまま使う。jj は `jj diff --summary --from REV --to @ [-- PATHS]` の native 出力が `<CODE> <path>` (space 区切り) なので、最初の space のみを tab に置換して git の形式に正規化する (paths-with-spaces は SplitN(_, 2) で右側を保持)。Rename/Copy (R/C) は best-effort — git/jj 間で形式が微妙に違う可能性があるが、kawaz が想定する用途 (M/A/D 判定) はカバー
- **`-q` / `--quiet` の overload**: グローバル `-q` は他 verb で「stdout 抑止」のみだが、`vcs diff` では `git diff --quiet` (= `--exit-code`) と同じ意味に拡張する: **差分なし → exit 0、差分あり → exit 1** (`exitCodeFalse`、`vcs is` と同じ predicate-false コード)、エラーは exit 3 (= `exitCodeVCSExec`)。意図的な overload で、Design rationale は「`diff` は『何か違いがあるか?』が well-posed な唯一の verb」「`git diff --quiet` の mental model を踏襲するのが scripting 観点で自然」(`document-design-rationale.md` ルールに従い code に明記)。他 verb (`get` / `is`) の `-q` 意味は不変
- **`-s` と `-q` の併用**: `-q` 優先 (stdout 空 + 差分有無 exit code)。**code path は 1 つ** — `-q` ブランチも `DiffNameStatus()` の出力長で差分有無を判定する (= name-status の出力が「表示用」と「presence 判定用」を兼ねる)。これにより display と predicate の経路が乖離しない (advisor 提案を採用)
- **interface 拡張方針**: 既存 `Diff(rev, paths) ([]byte, error)` には options を追加せず、新メソッド `DiffNameStatus(rev string, paths []string) ([]byte, error)` を追加する。理由: (1) 既存 `Diff` の caller / test を変更しない (churn 回避)、(2) `HasChanges` 述語を別途設けると `DiffNameStatus` と同じ subprocess を 2 回走らせる懸念 — 出力長で代用すれば 1 回で済む、(3) interface comment にある "grown incrementally as PRs land verbs" の方針に整合
- **path フィルタ**: `Diff` と同じ宣言的収束ルールを `DiffNameStatus` にも適用。全 path filter で 0 件 → backend 呼ばずに empty bytes (= `-q` 経由なら exit 0、`-s` 経由でも stdout 空)
- **parser 配置**: `-s` / `--name-status` は vcs サブコマンドの共通 flag loop で受理する。**v0.20.2 で訂正**: 当初は `runVcsCmdGet` / `runVcsCmdIs` がこのフラグを参照しないため `vcs get root -s` 等は silent no-op (scope 外と判断) としていたが、kawaz CLI 設計 (`rules/cli-design-preferences.md`: 未知 flag は exit 2 + usage hint) と整合せず typo 検出が効かないため、v0.20.2 (バグ修正) で verb-aware reject を実装。`-s` / `--name-status` の case を `out.vcsVerb == "diff"` で gate し、他 verb は generic catch-all で `unknown flag for 'vcs <verb>': <flag>` を返して exit 2 で reject する。verb-local flag が 1 つしかないため verb→flags table ではなく inline gate を採用 (詳細: code の Design rationale comment)
- **PR-3.1 land 日**: 2026-05-30
- **v0.20.2 verb-aware reject 修正日**: 2026-05-30

### PR-4 (vcs commit) 実装メモ (2026-05-30 確定)

PR-4 は read 系 (PR-1〜3.1) に続く最初の **write 系** verb。3 モード (path 必須 / `--staged` / `--amend`) を持ち、それぞれに「empty-no-op」の冪等規律と「`-a` 非提供」の安全装置を組み込む。

- **コマンド形 (3 モード)**: `vcs commit -m MSG PATH..` (path 必須が基本)、`vcs commit -m MSG --staged` (一括)、`vcs commit --amend [-m MSG]` (直前に吸収)。`PATH..` と `--staged` は parser 段階で相互排他 (= exit 2)。`-m` は `--amend` 以外で必須 (amend 時のみ no-edit 許容)
- **jj の path 指定 commit (採用案: jj 0.41 native `jj commit [FILESETS]...`)**: 当初の advisor 入力では (A) `jj split` / (B) `jj squash --from @ --into @-` / (C) 自前 snapshot 制御の 3 案を検討したが、実機 (`jj 0.41`) で `jj help commit` を確認したところ **`jj commit [FILESETS]... -m MSG`** が「指定 path だけ commit、残りは新規 working copy に残す」をそのまま実現することが判明。advisor も「A/B/C は moot、`jj commit` を使え」と確定。`--staged` も `jj commit -m MSG` (filesets なし = 全 @ snapshot) で同じ subcommand で表現できる。よって path / `--staged` 両モードとも 1 系統 (`jj commit ...`)、`--amend` だけ `jj squash --from @ --into @-` で別系統
- **empty-no-op の事前 gate (advisor #1, jj/git 両方)**: jj は **空 commit を許す** ため、nonexistent-only / 変更なしのケースでそのまま実行すると空 change が生成されてしまい DR-0020 (「0 件 → 操作なし」) と矛盾する。両 backend とも commit 実行前に「実際に変更があるか」を確認:
  - jj path: `jj diff --summary --from @- --to @ -- PATHS` の出力が空なら no-op
  - jj `--staged`: `jj log -r @ --no-graph -T empty` (= IsClean と同じ predicate)
  - git path: `git add -- PATHS` を**先に**実行してから `git diff --cached --quiet -- PATHS`
  - git `--staged`: `git diff --cached --quiet` (no paths) で gate
- **git の untracked path 取り扱い (advisor #2)**: ナイーブに `git diff --quiet HEAD -- PATHS` で gate すると、**未追跡の新規ファイル** (`untracked`) が「変更なし」と判定されて静かに no-op で終わる (= ユーザの新規ファイルが永遠に commit されない bug)。対策: path mode では **`git add -- PATHS` を最初に実行**し、index に取り込んでから `git diff --cached --quiet -- PATHS` で gate する。これにより未追跡ファイルも commit 対象に含まれる。red test (`TestGitBackend_Commit_Paths_NewFile`) でこの経路を pin
- **`-a` / `--all` 非提供 (DR-0020 安全装置)**: kawaz CLI 設計 + jj の auto-stage 世界観 (= unstaged 概念がない) から、`commit -a` 的な「unstaged を巻き込む」モードを意図的に提供しない。ただし「unknown flag」の generic reject ではなく、`-a` を **parser で明示的に拾って exit 2 + tailored hint** (= 「`--staged` か PATH.. を使え」) を返す。これにより git ユーザが習慣で `-a` を打った時の usability 損失を最小化
- **dynamic hint (path / `--staged` / `--amend` どれも未指定の場合)**: backend.Kind() を resolve した後に分岐。git なら 「use --staged to commit staged changes, or specify PATH」、jj なら 「specify PATH.. (commit -a is not supported by design); or use --staged for the entire @ change」。advisor #3 の指摘どおり、backend-independent な usage error (-a, --staged+PATH, missing -m) は `newVcsBackend` の前にチェックして exit 2 を返し、dynamic hint だけ resolve 後 (= 非リポなら exit 3)
- **--amend の挙動 (git/jj の差分)**: git は `git commit --amend` で直前の commit に index を吸収、`-m` 指定で message 書き換え、無指定なら `--no-edit`。jj は `jj squash --from @ --into @-` で @ を @- に吸収、`-m` 指定で @- の description を更新、無指定なら preserve。**両 backend とも empty 状態の amend を許容する** (= message-only amend = 「変更なしで message だけ更新」は明示的 rewrite 意図として正当)。よって amend モードは empty-no-op gate を**かけない**
- **MVP amend grammar: `--amend [-m MSG]` のみ (PATH.. / `--staged` は拒否)**: `--amend PATH..` を「path 限定の吸収」と読みたくなるが、(a) git --amend には clean な path-restriction セマンティクスが無い (= jj だけ実装できる非対称な機能になる)、(b) **accept-but-silently-ignore は path 必須の安全哲学に反する最悪パターン** (advisor の指摘)。よって parser step 3.5 で `amend && (len(PATH..) > 0 || --staged)` を exit 2 で拒否。jj 側で `jj squash --from @ --into @- -- PATHS` が動く事実は将来の path-amend feature の土台として残る (PR-N で検討、現状は YAGNI)
- **interface 拡張**: `Commit(opts commitOpts) error` を `vcsBackend` に追加。`commitOpts` struct で paths / message / staged / amend / noEdit を保持。「method per mode」(`CommitPaths` / `CommitStaged` / `CommitAmend`) ではなく 1 method + struct を選んだ理由は、(1) 既存 interface (Diff / DiffNameStatus 等) と同じ「verb 1 つ = method 1 つ」の対応、(2) 将来 flag が増えた時に struct 追加だけで済む、(3) 共通の前処理 (filterExistingPaths) を呼びやすい
- **test 環境の hermeticity (advisor #4)**: PR-4 は最初の write 系 verb なので、kawaz の global `signing.key` を持つ環境では `runBackendCmd("jj", "commit", ...)` が `ssh-keygen -Y sign` 経由で 1Password を叩く可能性がある。対策: `setupJjRepo` が `.jj/repo/config.toml` に `[signing] behavior = "drop"` を**直接書く** (`jj config set --repo` は jj 0.41 では受理しても永続しないため。実機で確認済)。repo-local config は user config を override するので、kawaz の host でも CI のクリーン env でも同じ動作になる
- **配線 4 点 (既存パターン踏襲)**: parser の `isKnownVerb` に `commit` 追加、verb-local flag (`-m` / `--message` / `--staged` / `--amend` / `-a` (= reject 用)) を vcs flag loop に追加、`runVcsCmd` の switch に case 追加、`actionHelpTexts["vcs commit"]` 登録 + `helpVcs` の verbs 一覧に追記
- **PR-4 land 日**: 2026-05-30

### PR-4.1 (vcs commit --amend PATH..  / --staged 受け入れ) 実装メモ (2026-05-30 訂正)

PR-4 で `--amend PATH..` / `--amend --staged` を「MVP grammar 外」として exit 2 で拒否したが、**これは誤った判断**だった。kawaz の元設計 (`squash` 独立サブコマンドを `--amend` フラグに統合した DRY の論拠) は **「commit と amend は完全対称、違いは『新規 commit を作る』か『直前 commit に吸収する』かの 1 点のみ」**。受け入れる selector (`-m MSG PATH..` / `-m MSG --staged`) は両者で同一であるべき。

- **訂正後の grammar**: `vcs commit --amend [-m MSG] [PATH.. | --staged]`。素の `--amend` (= 全変更を吸収する明示 rewrite, ungated) も従来通り受理。step 3.5 の reject ガードを **削除**。`--amend PATH.. --staged` 三重組合せは step 2 (`paths && staged` 相互排他) で引き続き exit 2
- **git equivalence**: path 限定 amend は `git add -- PATHS && git commit --amend [-m MSG | --no-edit] -- PATHS`。pathspec が rewrite 範囲を制限するため、index に無関係の staged 変更が残っていてもそれは index に留まる (実機 git 2.x で検証)。これにより「全 staged 巻き込み」事故が原理的に発生しない (= PR-4 の YAGNI 論拠の前提が崩れる)。`--amend --staged` (path なし) は素の `git commit --amend` と等価 = 明示的シノニムとして受理
- **jj equivalence**: `jj squash --from @ --into @- [-m MSG | -u] [-- PATHS]`。`[FILESETS]...` 位置引数で path 限定吸収が素直に書ける (jj 0.41 で `jj help squash` 確認・実機検証済)。staged / 素の `--amend` は path なしの同コマンド
- **no-edit の罠 (jj squash combined description prompt)**: jj squash は @ が non-empty な description を持ち、squash で empty 化されると **「@ と @- の description を結合した編集 prompt」**を開く (jj 0.41 で `JJ_EDITOR=false` を渡して検証、`Failed to edit description / Editor 'false' exited with exit status: 1` で fail)。bump-semver は非対話的呼び出し前提なので、`--no-edit` は `--use-destination-message` (`-u`) で実装 = @- の description を verbatim 維持。PR-4 の素の amend 実装も同じ trap に潜在的に該当していたため PR-4.1 で同時修正。PR-4 の既存 test (`TestJjBackend_Commit_Amend_NoEdit`) は @ に description が無いケースしか踏んでいなかったため green を維持できていた経緯あり
- **path 限定 amend の no-op gate**: 非 amend path モードと同じ宣言的収束ルール (全 path 不在 / 変更なし → no-op, nil) を適用。素の `--amend` は **gate 無し** (message-only rewrite は意図的に合法) のまま — この非対称は意図通り (= 「path 指定 = path 範囲の宣言、空なら何もしない」 vs 「素の amend = 明示 rewrite 意図」)
- **削除した parser 段階の ガード (step 3.5)**: PR-4 の `args.vcsCommitAmend && (len(args.vcsArgs) > 0 || args.vcsCommitStaged) → exit 2` ブロックを撤去。`runVcsCmdCommit` の手順番号は 1-2-3-4-5-6 のまま (3.5 が抜けた連番)
- **PR-4.1 land 日**: 2026-05-30

### PR-5 (vcs fetch / vcs push) 実装メモ (2026-05-30 確定)

PR-5 は network 系の counterpart。read (PR-1〜3.1) / write-local (PR-4) に続いて remote refs 操作を `vcs` family の傘下に揃える。`vcs push --force` を**意図的に提供しない**ことと、non-ff を **exit 5** で構造的に検出することの 2 点が DR-0020 PR-5 のコアな安全装置。

- **コマンド形 (2 verb)**: `vcs fetch [REMOTE]` (REMOTE 省略時 `origin`) と `vcs push --branch|--bookmark NAME [--remote REMOTE]`。fetch の REMOTE は positional または `--remote` のどちらか (= 両方指定は exit 2、暗黙の優先順位サプライズ回避)。push は **NAME 必須・positional 引数不可** (= `vcs push main` は exit 2、必ず `--branch main` と書く)。理由: 現在の branch から自動推測すると「typo した動詞のせいで意図しない ref を push する」事故を構造的に排除できないため
- **`--branch` canonical / `--bookmark` alias の選定根拠**: 既存コード (`CurrentBranch()` interface method、`current-branch` get-key) が **branch を cross-VCS の共通語彙として既に採用済み**。DR-0020 命名規律 (「共通理解語彙は単一、VCS 固有は alias + 注釈」) と内部一貫性の両方が同じ結論を指す。jj users 向けに `--bookmark` を alias として提供し、help 注釈で「(jj users may also write --bookmark)」と明示。両方同時指定は usage error (同一スロットを共有しているため、後勝ち silent override は避ける)
- **`--force` 意図的に非提供 (DR-0020 PR-5 安全装置)**: force push は remote history の rewrite で、SemVer release ヘルパの責務外。ユーザが本当に必要なときは素の `git push --force-with-lease` / `jj git push --force-with-lease` を使う。`vcs push --force` は exit 2 + usage hint で reject。`--tags` / `--all` も同様に非提供 (= tag push は release 自動化 = `gh release create` 側の仕事、bulk push は責務外)
- **non-fast-forward 検出 (exit 5) と保守的 fallback**: 実機 fixture で git / jj 両 backend の non-ff stderr を観測:
  - git: `[rejected]` + `(fetch first)` or `(non-fast-forward)` + `failed to push some refs`
  - jj: `stale info` + `Failed to push some bookmarks`

  検出器 `isNonFastForward` は安定 marker (`(fetch first)` / `(non-fast-forward)` / `stale info` / `Failed to push some bookmarks`) の `strings.Contains` で判定。locale fragile な hint 行 (= "Updates were rejected..." 等) は使わない。**保守的方針**: 未知の失敗は exit 5 ではなく exit 3 にフォールバック (= `CurrentBranch` の「unknown failure → 安全側の code」と同じ原則)。理由: bad URL / signing 失敗を non-ff と誤分類して「fetch して reconcile しろ」と誘導すると修復経路が外れる、逆 (non-ff を generic とする) の方が損害が小さい
- **共通 hint「remote has diverged...」** (※ PR-5.1 で削除、下記参照): dispatcher が exit 5 を検出したら `bump-semver: vcs push: remote has diverged. fetch and reconcile, then retry. (force push is intentionally not supported)` を stderr に出力していた。「force push not supported」を明示することで「`--force` あるかな?」と探させない (= ヘルプ確認の手間を 1 ステップ削減) という意図だったが、kawaz 確定で「git/jj 其々のエラーメッセージそのまま出して対応はユーザ責務」とのこと、editorial paraphrase は廃止 (= PR-5.1)
- **`jj git push --allow-new` の deprecation 対応 (jj 0.41)**: 実機 (jj 0.41) で確認したところ、`jj git push --bookmark NEW_NAME --remote origin` (= `--allow-new` なし) は新規 bookmark でも warning なく動作。`--allow-new` を渡すと「deprecated, use auto-track-bookmarks instead」の warning が出る。よって **`--allow-new` は渡さない** (= 将来 jj が新規 push を block するようになったら明示 track を追加する。現状は YAGNI)
- **`jj git export` を push 後に必ず呼ぶ (DR-0020 PR-5 要件)** (※ PR-5.1 で retry + 復旧 hint に拡張、下記参照): jj の push 後に `jj git export` を実行して colocated `.git` の ref を同期する。`jj git export` 自体は通常 no-op (= `Nothing changed`) だが、ref 階層衝突 (jj issue #493) / HEAD race (#6098) / packed-refs (#6203) のような edge case で fail し得る。PR-5 では **握りつぶさず exit code を確認** し、失敗時は exit 3 + jj の native stderr を fold した message で surface。事前に panic 文字列を parse するような複雑な hook は入れない方針だったが、kawaz 確定で「再実行で冪等リカバリーなら大きな問題は無い」「対応方が分からん程だとユーザに丸投げしても解決できない」となり、PR-5.1 で 1 回 retry + substring 検出ベースの復旧 hint を追加 (= 公式 issue 由来のパターン別 remedy)
- **interface 拡張**: `Fetch(remote string) error` と `Push(opts pushOpts) error` を `vcsBackend` に追加。`pushOpts` は `name` / `remote` の 2 フィールドのみ — `--force` を struct field として模型化すると将来 accidental 配線を招く (= 構造的にも `--force` ルートを存在させない)
- **`runBackendCapture` 補助関数**: push は **exit code が信号 (= success / non-ff / その他)** なので、`runBackendCmd` (non-zero を error 化) と `runBackendExitCode` (出力捨て) のどちらも合わない。stdout / stderr / code の 3 つを同時に取れる `runBackendCapture` を追加して push 専用に利用
- **test 環境の bare remote**: fixture は **ローカル bare repo を origin として**設定 (= filesystem path)。実 push ではないので「fixture 外で生 git push / jj git push しない」制約に違反しない。非 ff のシミュレートは **attacker clone fixture** (bare を別 path に clone → 異なる commit を作って `git push --force` → bare の main が divergent な commit を指す状態にする) で構成。git fixture の HEAD 不整合 (bare の default branch が master のまま、clone が main を check out できない) を防ぐため、`git init --bare -b main` で bare 作成時に明示
- **parser 配置**: `--branch` / `--bookmark` / `--remote` を vcs flag loop に追加。`--remote` は fetch / push 両方で受理、`--branch` / `--bookmark` は push のみで受理 (verb-aware gate)。inline 実装 (verb→flags table への refactor は PR-6 以降に持ち越し、DR の「verb-local flag が増えたら refactor を検討」trigger を 1 回後ろにずらす)
- **配線 4 点 (既存パターン踏襲)**: parser の `isKnownVerb` に `fetch` / `push` 追加、verb-local flag 追加、`runVcsCmd` の switch に 2 case 追加、`actionHelpTexts["vcs fetch"]` / `["vcs push"]` 登録 + `helpVcs` の verbs 一覧に追記
- **PR-5 land 日**: 2026-05-30

### PR-5.1 (vcs push 補修) 実装メモ (2026-05-30 訂正)

PR-5 で land した実装に kawaz 確定の意向との乖離があったため、4 点を補修
する subset PR-5.1 を続けて投入。PR-4.1 advisor で保留扱いだった bare
`--amend` help 表現の git/jj 分割も同時に修正。

- **`--branch` canonical 維持、`--bookmark` 注釈簡素化**: PR-5 は help body に
  `--bookmark NAME  Alias of --branch. (jj users may also write --bookmark.)`
  という 2 行注釈を持っていたが、これは "alias" を主役化しすぎていて
  「`--branch` 一本化で help に jj では bookmark の意と簡潔記載」
  (kawaz 原文) と乖離。**PR-5.1**: `--branch` の説明文に
  「(jj users: "branch" here means the jj bookmark; the --bookmark spelling
  is also accepted as an exact synonym, ...)」と一行注釈で圧縮。両指定 reject
  ロジック自体は維持 (parser は `--branch` / `--bookmark` 同一スロットを共有、
  両方指定は exit 2)。Usage の独立 `--bookmark` 行 / Examples の
  `vcs push --bookmark main` も削除し、help body から `# alias` 等の補注を排除
- **non-ff 共通 hint 削除 (`bump-semver: vcs push: remote has diverged...`)**:
  PR-5 では dispatcher が exit 5 検出時に「remote has diverged. fetch and
  reconcile, then retry. (force push is intentionally not supported)」を
  必ず stderr に出していたが、kawaz 確定で「non-ff 検出云々: git/jj 其々の
  エラーメッセージそのまま出して対応はユーザ責務以外ある?」のとおり editorial
  paraphrase は不要。**PR-5.1**: dispatcher の hint 構築ブロックを削除し、
  backend で formatPushError(...) によって stderr が ee.msg に折り込まれた
  ものを emitVcsErr が `bump-semver: <msg>` として 1 回だけ出す経路に揃える。
  exit code 5 マッピング (`isNonFastForward` 判定) はそのまま維持
- **`jj git export` 失敗の retry once + 復旧 hint (kawaz 確定)**: PR-5 では
  「失敗を握りつぶさず exit 3」までで止めていたが、kawaz 確定で
  「再実行で冪等リカバリーなら大きな問題は無い」「対応方が分からん程だと
  ユーザに丸投げしても解決できない」のとおり、(a) 1 回 retry → (b) 2 回目も
  失敗なら exit 3 + 具体的 recovery hint の 2 段階に拡張。**PR-5.1**:
  `jjGitExportFunc` package-level seam を導入し、Push() 内で
  `exStderr1, exCode1, exErr1 := jjGitExportFunc()` → 失敗時にもう 1 回 →
  両方失敗で `jjGitExportRecoveryMessage(finalStderr)` を msg として exit 3。
  recovery hint は **substring 検出ベース** で issue 別に固有の手順を返す
  (`#493` ref-hierarchy: `git for-each-ref refs/heads/` で衝突確認 +
  rename/delete、`#6098` HEAD race: `jj git import` で resync、`#6203`
  packed-refs: lock 解除 + retry)。未知 pattern は raw stderr + jj-vcs/jj
  issues URL の generic fallback。seam の存在理由は test 性 — 実 jj で
  「fail once, then succeed」を確実に再現する fixture は組めないため
- **idempotent push 時に git/jj 自身の diagnostic を素通し (`Everything
  up-to-date` / `Nothing changed`)**: PR-5 では success path で stdout/stderr
  を捨てていたため、no-op が silent な exit 0 に見えていた (= 「本当に push
  通ったのか?」が分からない)。**PR-5.1**: `pushPassthroughStdout` /
  `pushPassthroughStderr` package-level buffer を導入し、backend が success
  path で空でない出力を蓄積、dispatcher が `consumePushPassthrough()` 経由で
  読み出してから自身の error を出すことで chronology (push 出力 →
  dispatcher エラー) を保つ。`-q` で stdout 抑制、`-qq` で全抑制
  (= 既存 quiet contract と整合)。事前検証で exit code は既に 0 (`if code
  == 0 { return nil }`) だったので 5→0 のルート変更は不要 — gap は出力
  forwarding 側にあった (advisor 指摘で確定)
- **bare `--amend` help の git/jj 分割表記 (PR-4.1 残)**: PR-4.1 advisor で
  「fold ALL current changes は git では index 限定 (unstaged は含まれない)
  で不正確」と指摘されていた残課題。**PR-5.1**: helpVcsCommit の `--amend`
  bare 行を `--staged` 行と同じ git/jj 分割形式に揃える (git: "folds the
  staged index into HEAD (unstaged worktree changes are NOT included)" /
  jj: "folds the entire @ snapshot into @- (jj auto-stages, ...)" )
- **PR-5.1 land 日**: 2026-05-30

### PR-5.2 (vcs push --jj-bookmark-auto-advance) 実装メモ (2026-05-31 確定)

PR-5.2 は `vcs push` に **jj-only opt-in pre-step** を追加するサブセット PR。
jj 慣習 (bookmark は確定 commit `@-` に置く、`@` は throw-away な working
copy) に不慣れな利用者 / エージェントが「bookmark を `@` に置いたまま push」
「bookmark の手動 move 忘れ」「move 先 `@` 指定」等で手戻り or
無駄コンテキストを生むケースを構造的に減らす。

#### kawaz 確定仕様 (2026-05-31)

- **フラグ名**: `--jj-bookmark-auto-advance`
  - `--jj-` prefix で「jj backend 専用」を名前から明示 (~~git リポでは exit 2
    reject。silent no-op は禁止 = `--jj-` prefix の invariant を裏切る~~ →
    **PR-5.2.1 で訂正**: silent no-op に変更。下記 PR-5.2.1 セクション参照)
  - `bookmark` + `auto-advance` の二層で対象 (bookmark) と動作 (auto-advance)
    の曖昧さを排除
- **opt-in (デフォルト OFF)**: bookmark の move は副作用ある操作で「ユーザが
  意図したとおりに置いた bookmark」を silent に動かすのは禁則。明示的に
  opt-in したときだけ動く
- **target は IsClean() で振り分け**:
  | 状態 | 動作 |
  |---|---|
  | clean (`@` 空) | bookmark を **`@-`** に move (kawaz 常用、jj 慣習) |
  | dirty (`@` 非空) | bookmark を **`@`** に move (= 「immutable 化」pattern。push 後に `@` が immutable 化して jj が自動で新 working copy を作る、describe 有無は問わない) |
- **divergent (祖先にない) は exit 3 + hint で停止**: bookmark を移動しない。
  ancestor 判定は `NAME & ::@` revset の emptiness で行う (= jj の「Refusing
  to move bookmark backwards or sideways」エラーを使わず bump-semver 名で
  hint を出す。利用者が「どのフラグを切れば回避できるか」が一目で分かる)
- **存在しない bookmark は何もせず通常 push に委ねる**: PR-5 が既に持っている
  「bookmark not found」エラー路に一本化 (= naming error を auto-advance の
  責務に含めない)。revset の `present(NAME)` で「missing → 空」「ある → 当該
  commit」を区別、未 wrap の `NAME & ::@` だと missing で revset parser
  エラーになるので必須

#### 旧プランから変わった点 (= advisor / kawaz feedback で訂正)

- **元プラン「dirty なら exit 1 で停止」→ 撤回**。kawaz 確定で
  「clean 前提 / dirty + describe して push の両運用パターンを 1 フラグで
  カバー」となり、IsClean() で target 振り分けに変更。dirty で push を拒否
  したい利用者は自分で `vcs is clean` ガードを書く (= ツール側で禁止しない、
  最小ガード方針)
- **「describe 必須チェック」も削除**。describe 有無は問わない (空 describe
  で push したいユーザ運用も許容)。ただし jj 自身が undescribed commit の
  push を refuse するため、現実的な dirty workflow は describe を伴う
- **元プラン「exit 1」→ exit 3 に変更**。advisor 指摘: `exitCodeFalse` (1)
  は `compare` / `vcs is` 等の predicate verbs 専用、auto-advance の
  precondition refusal (divergent) は VCS-layer の「条件を満たさない」=
  `exitCodeVCSExec` (3) が established taxonomy にも整合 (= 「unknown remote」
  「not a repo」と同じ slot)

#### 実装位置

- `pushOpts.jjBookmarkAutoAdvance bool` 追加 (= 既存 io.Writer 型 stdout /
  stderr フィールドと同居、interface 拡張は不要)
- `jjBackend.Push`: 先頭で flag を見て `autoAdvanceBookmark(name)` を呼ぶ
- `autoAdvanceBookmark`: existence → ancestor → IsClean で target → no-op?
  → forward-move、の順
- ~~`gitBackend.Push`: 防衛的 reject (`exitCodeVCSExec` + 「please file a
  bug」wording)~~ → **PR-5.2.1 で撤去**: backend-prefix general rule
  により git は flag を silent 無視 (= 通常 push のみ実行)
- `cliArgs.vcsPushJjBookmarkAutoAdvance bool` 追加、parser case で
  `--jj-bookmark-auto-advance && vcsVerb == "push"` ガード付きで受理
- ~~`runVcsCmdPush`: backend 取得後、`args.vcsPushJjBookmarkAutoAdvance &&
  b.Kind() == "git"` で `emitVcsUsage(...)` = exit 2 + 「jj-specific; this
  repo uses git」hint~~ → **PR-5.2.1 で撤去** (同上)

#### forward-only 移動の意義 (= `--allow-backwards` を絶対に付けない)

`jj bookmark move NAME --to <target>` は jj デフォルトで forward-only
(backwards / sideways は exit 1 + 「Refusing to move bookmark backwards or
sideways」)。auto-advance の precondition chain (existence + ancestor + 既
target check) が安全側のチェックを担保しているので、move 自体は forward-only
の素のセマンティクスで足りる。**`--allow-backwards` を付けると ancestor
check を silently 無効化する**ことになり、divergent 救済が暴発する。
コード上に Design rationale コメントで「`--allow-backwards` を付けるな」と
明示している (= 将来「動作確認したら『失敗する』エラーが出た、フラグ追加で
直そう」の reflex を予防)

#### jj 仕様の補助知識 (DR ノートに残す価値あり)

- `@` の immutable 化 → jj は自動で新 working copy を作る (jj 仕様)。dirty で
  bookmark を `@` に置いて push → push 後に `@` が immutable 化 → 次回起動時
  jj が自動で `jj new <旧 @>` 相当を行い、新しい throw-away working copy が
  生まれる。利用者は「dirty で push したら次の起動時 @ が空になる」と
  理解しておけば OK
- `present(NAME)` revset は jj 0.41 で動作確認済 (missing → 空、ある → 当該
  change)。`NAME & ::@` 等の素の revset は missing で「Error: Revision X
  doesn't exist」を返すため、existence check には `present()` wrap が必須
- bookmark の forward move は jj の native semantics で「strictly ancestor →
  OK」「same / sideways / backwards → refuse」。bump-semver 側 hint で「can
  only advance, not move sideways」と言い換えているのも同義

#### 観点別マトリクス (テストでカバー)

| flag | backend | working copy | bookmark の位置 | 期待 |
|---|---|---|---|---|
| ON | jj | clean | `@--` (strict ancestor of `@-`) | `@-` に forward-move + push success |
| ON | jj | clean | `@-` (already at target) | no-op + push success |
| ON | jj | clean | 別 branch (divergent) | exit 3 + hint、bookmark 動かさず |
| ON | jj | dirty (described) | `@--` | `@` に forward-move + push success |
| ON | git | (irrelevant) | (irrelevant) | **silent no-op + 通常 push 成功** (PR-5.2.1 で訂正、旧仕様 = exit 2 reject) |
| OFF | jj/git | (irrelevant) | (irrelevant) | PR-5 の既存挙動 (regression なし) |

#### PR-5.2 land 日

2026-05-31

### PR-5.2.1 (backend-prefix general rule — `--jj-*` / `--git-*` flag は他 backend で silent no-op) 訂正メモ (2026-06-01 確定)

PR-5.2.1 は PR-5.2 の `--jj-bookmark-auto-advance` を起点に、`--jj-*` /
`--git-*` prefix フラグの一般ルールを **構造的明示 + silent no-op** に
確定させる訂正 PR。

#### kawaz 確定 (2026-06-01)

> vcs の --{jj,git}-* 系オプションは backend スペシャルで行う必要がある処理や
> 指定のためのオプションで、backend グループ (今は jj/git) しかないが
> その互いの backend じゃない方の prefix の奴は単純に無視 (自分とは関係なくて
> jj 使ってる人用なんだろうな) で通るのが良い。
>
> jj 固有の操作である bookmark-auto-advance をサブコマンドにするのも違うと思うしね。

#### 一般ルール (= backend-prefix general rule)

- `--jj-*` / `--git-*` prefix のフラグは **backend specific** な処理・指定用
- 名前 prefix で「どの backend 向けか」を **構造的に明示**
- 起動中の backend と prefix が **一致** → 当該 backend のロジックで処理
- 起動中の backend と prefix が **不一致** → **silent 無視**、副作用なし、push 等は通常通り続行
- → 同じ起動行 (`vcs push --branch main --jj-bookmark-auto-advance`) を jj リポと git リポの両方で動かせる (= 利用者・スクリプトの分岐ロジック不要)

#### Why silent no-op (PR-5.2 の exit 2 reject から訂正)

PR-5.2 元案: 「`--jj-` prefix の invariant を裏切る = git で silent 無視は禁止、
exit 2 reject すべき」。kawaz の本質再確認で訂正:

- 名前 prefix が「これは jj 向け」と **構造的に告知済み**。git ユーザは見るだけで
  「自分には関係ない」と判断できる
- 利用者が「自分の環境では関係ないかも」と感じる flag を **誤入力扱いで止める**
  のは過剰防衛。jj 向けの hook を持つ汎用 push スクリプトを git ユーザにも
  使ってもらえる方が利便性が高い
- ツール側で「相手 backend の flag を入れたら止める」と決め打つと、利用者が
  `if bump-semver vcs is jj; then ...; else ...; fi` の分岐を書かされる
  (= 構造的明示の意味が薄れる)。本ツール自体の justfile (= ドッグフード) で
  当初その分岐を書く羽目になり、設計の不整合に気付いた

#### サブコマンド化を採らない理由

「jj 固有の操作なら subcommand に分けるべきでは?」も検討対象だが採用しない:

- bookmark-auto-advance は **push の事前処理**。独立 verb にするとユーザは
  `vcs jj-bookmark-advance && vcs push --branch main` のように 2 ステップ
  書くことになり、push の atomic な動作と切り離される
- 名前空間が膨らむ (`vcs jj-*` 系の verb 群が増える可能性)。一方フラグなら
  prefix 一般ルールの中に閉じる
- kawaz: 「jj 固有の操作である bookmark-auto-advance をサブコマンドにする
  のも違う」

#### 実装変更 (PR-5.2 からの差分)

- `runVcsCmdPush` (vcs_cmd.go) の git reject ブロック削除 (= dispatcher で
  通る)
- `gitBackend.Push` (vcs_backend.go) の防衛的 reject 削除 (= flag は
  単純に読み捨て)
- `helpVcsPush` (help.go): セクション見出しを「rejected with exit 2 in a
  git repo」→「silent no-op on git — backend-prefix general rule」、exit
  code セクションから git reject 言及を削除 (exit 3 の divergent refusal
  は jj 側で残るので保持)
- test 更新:
  - `cmd_vcs_push_test.go`: `TestRun_VcsPush_AutoAdvance_GitReject` →
    `TestRun_VcsPush_AutoAdvance_GitSilentNoOp` に rename + 内容書き換え
    (exit 0 + push 完了 + stderr に auto-advance / jj-specific が出ない)
  - `vcs_backend_test.go`: `TestGitBackend_Push_AutoAdvance_Reject` →
    `TestGitBackend_Push_AutoAdvance_SilentNoOp` に rename + 内容書き換え
    (exit 0 + bare に main が反映)

#### justfile push recipe の 1 行化 (= ドッグフード)

PR-5.2 land 後の justfile では、git/jj 分岐を明示的に書いていた:

```just
push: ci check-translations check-version-bumped
    if bump-semver vcs is jj; then bump-semver vcs push --branch main --jj-bookmark-auto-advance; else bump-semver vcs push --branch main; fi
```

PR-5.2.1 で flag が silent no-op になったので、分岐なしの 1 行に collapse:

```just
push: ci check-translations check-version-bumped
    bump-semver vcs push --branch main --jj-bookmark-auto-advance
```

`vcs is` による分岐が消える = 「**1 つの push 起動行を git でも jj でも同じく
書ける**」の体験を本ツール自身が真っ先に享受する。

#### 将来 `--git-*` が追加された場合

同じ一般ルールに従う。例えば仮に `--git-tags` のような flag を追加するなら、
jj リポでは silent 無視 (push は jj backend の通常パスで実行)、git リポでは
当該 flag に応じた処理を git backend 側で行う、という対称構造になる。
backend が増えた場合 (例: hg, fossil, ...) も同じく `--<backend>-*` prefix
で同様のルールが拡張可能。

#### PR-5.2.1 land 日

2026-06-01

### PR-6 (vcs tag push / vcs tag delete) 実装メモ (2026-05-30)

PR-6 で `vcs tag push --rev REV NAME [--remote REMOTE] [--allow-move]` と
`vcs tag delete NAME [--remote REMOTE]` を投入。tag は family 唯一の
two-tier verb (`vcs tag <sub-verb>`) で、parser 側にも追加変更が入る。

#### 確定事項

- **two-tier dispatch**: parser で `vcsTagSubVerb` フィールドを追加し、
  `vcs tag` 単独 = parent help、`vcs tag push|delete` = sub-verb help、
  `vcs tag <unknown>` = exit 2 (top-level の unknown verb と同じ contract)。
  flag scanning は `tagSubVerbStart = 3` で argv[3:] を回す変則を採用
  (verb→flags table への refactor は future PR 案件、現状 sub-verb 1 階層
  ぶんなら inline で読みやすさ優位)。`--rev` / `--allow-move` は
  `vcsVerb == "tag" && vcsTagSubVerb == "push"` 二重 gate で push 限定、
  `--remote` は fetch / push / tag の三 verb 共通 (DR-0020 命名規律: cross-VCS
  共通語は単一語)
- **shared decision matrix (`decideTagPush`)**: backend 間で共通の 4 分岐
  (absent → create+push / 同 rev → skip-create+push / 別 rev +!move → exit 4 /
  別 rev +move → move + force-push) を pure function に切り出し。git/jj
  backend は自分側で resolveXxxRev → existingXxxTagSHA で 2 つの SHA を
  確定させてから `decideTagPush` に渡すだけ。同 rev 冪等 (片落ちリカバリ)
  は **local skip でも push は実行**: 「local にはあるが remote が
  落としている」状態を素直に救うため。同 rev でも remote が一致して
  いれば push は素の no-op になる
- **integrity violation = exit 4 (exitCodeAmbiguous の再利用)**: 別 rev で
  `--allow-move` 無しのケースは「同名 tag が別 commit を指す」整合性違反で、
  既存 `exitCodeAmbiguous` (detached HEAD / 複数 bookmark 等の「答えが一意で
  ない」code) と意味的に揃う ("answer is not uniquely determined")。新規
  code 番号を切らずに済んだのは DR line 36 が事前に「tag/integrity violations
  も 4 を再利用」と reserve していたため。一般 VCS エラー (3) と分離して
  あるので呼び側で「tag が drift した」vs「git/jj 故障」を判別できる
- **REV→SHA + existing SHA pre-check (advisor 指摘で確定)**: jj の
  `jj tag set` は同 rev でも 「Refusing to move tag」で exit 1 になる
  ので素朴に呼ぶと冪等にならない。git の `git tag NAME REV` も既存だと
  exit 1。両 backend とも **自前で SHA 比較してから tool を叩く** 方式
  (= jj/git の native error に頼らず判定)。実装は
  `resolveGitRev` / `existingGitTagSHA` (`git rev-parse -q --verify ...^{commit}` で
  annotated tag の peel もカバー、欠如は空文字返し) /
  `resolveJjRev` / `existingJjTagSHA` (`jj tag list NAME -T self.normal_target().commit_id()`)
- **immutability の remote 到達済み判定は今回入れない**: DR line 69 で
  「immutable 判定 = REV が remote に在る commit に解決されるか」と書いた
  方針について、PR-6 では実装しない (advisor 確定)。理由: (a) test list に
  該当ケースが無い (= 契約として固まっていない)、(b) remote query を要する
  active check は cross-VCS の定義が ambiguous、(c) `--allow-move` の
  ユーザ宣言で安全側の opt-in は十分。reversible な判断として明記し、
  将来 release flow で必要になれば再評価する
- **`--force-with-lease` ではなく `--force` を採用**: PR-6 probing で
  tag ref は remote-tracking ref を持たないため bare `--force-with-lease`
  が「stale info」で拒否されることを確認 (= lease を確立する仕組みが無い)。
  明示的 lease 値 `--force-with-lease=refs/tags/NAME:<remote-sha>` なら
  機能するが ls-remote の追加 round trip が要る。**`--allow-move` の事前
  gate (ユーザ宣言) + 別 rev pre-check (= 何を上書きするか把握済み)** で
  safety stance は満たしているので、`--force` 単体で十分という判断
- **jj の git store path 解決 + colocated/非 colocated 分岐**: jj backend は
  `.jj/repo/store/git_target` を読んで native git push を発行する。`git_target`
  は colocated なら相対パス `../../../.git` (= cwd の `.git` ディレクトリ)、
  非 colocated なら bare の絶対パス。`jjGitPushDir()` で
  「`.git` directory が cwd に存在すれば cwd から push (= 空文字返し)、
  さもなくば `git_target` を resolve」と分岐。colocated で `.git` directory
  経由で push すると user の global hooks (pre-push 等) が
  「fatal: this operation must be run in a work tree」で落ちるため、
  cwd からの push が正しい (= work tree context が要る hook を救う)。
  bare backing の場合は worktree が存在しないので bare path 直で OK
  (= bare 上の push は worktree 不要)
- **delete の冪等性**: git は `git tag -d NAME` が不在で exit 1、しかし
  `git push origin :refs/tags/NAME` は不在 remote ref に対しても
  「warning: deleting a non-existent ref」+ exit 0 が確認できた (= remote
  layer は自然に冪等)。よって git backend は **local だけ pre-check** で
  済む (= `existingGitTagSHA` で空文字なら skip)。jj は `jj tag delete` が
  native で冪等 ("No matching tags" → exit 0) なので pre-check 不要。
  両 backend とも remote 半は無条件実行で OK
- **tag name validation (`validTagName`)**: 空 / 空白文字 / `refs/` prefix を
  3 ケース reject。more aggressive な `git check-ref-format` 全規則 は
  実際にユーザが踏むケースが少ないので over-engineering とした (git/jj が
  深い問題は自分の error で surface する)。`refs/` prefix の reject は
  copy-paste 事故 (`refs/tags/v1` を NAME にして refs/tags/refs/tags/... と
  二重 prefix 化) を catch する
- **非 colocated 経路の pre-push hook 回避 (`--no-verify`)**: 非 colocated
  layout で `git -C <git_target> push origin refs/tags/NAME` を実行する際、
  user の global `core.hooksPath` (kawaz 環境では `~/.dotfiles/config/git/hooks`
  + lefthook) の pre-push hook が `git rev-parse --show-toplevel` を内部で
  叩き、bare repo context では `fatal: this operation must be run in a
  work tree` で exit 128 → push 全体が exit 1。**bare に対する push に
  pre-push hook を効かせる発想自体が hook の前提と乖離している**
  (worktree 想定の lint/format/test 系 hook が大半) ため、非 colocated 経路
  (= `git -C <bare>`) でのみ `--no-verify` を付与する設計とした。colocated 経路
  (cwd push) は worktree が揃うので hook は通常通り効く (= release-gating
  hook を壊さない)。test では `setupJjRepoNonColocatedWithRemote` で 1 fixture
  追加し、non-colocated TagPush / TagDelete を共に green に
- **jj fixture の SHA drift 対策 (test 側)**: `jj git init --git-repo`
  は git HEAD を新規 empty change `@` に move し、commit hash も jj 側で
  rewrite されるケースがある (test 環境では auto-snapshot のたびに変動)。
  test 内で `git rev-parse HEAD` を直接 want SHA として使うと race するため、
  helper `jjResolveRev(t, dir, "@-")` を導入して **bare の tag SHA を読む
  のと同じ moment に jj 側 SHA を解決**。SUT と test の両方が同じ瞬間の
  jj 状態を読むので一致を assert できる
- **PR-6 land 日**: 2026-05-30

### PR-7 (セルフドッグフーディング: 残生呼び出しの置換) 実装メモ (2026-05-30 確定)

PR-1〜PR-6 + PR-4.1 + PR-5.1 で `vcs get` / `vcs is` / `vcs diff` / `vcs commit` /
`vcs fetch` + `vcs push` / `vcs tag push|delete` 各 verb が land した。PR-7 は
**bump-semver 自身の release / build サイクルに残っている生 git/jj 呼び出しを
vcs サブコマンド経由に置換**し、family を最初のドッグフードユーザーで検証する。

#### 置換した箇所

- **Taskfile.pkl `bump-version` task (line 147-151)**:
  `jj commit -m "Release v${new_version}"`
  → `bump-semver vcs commit -m "Release v${new_version}" VERSION`
  (path 必須形)。等価性の根拠は `deps { localEnsureClean }` で事前に working
  copy が clean を保証している点 — `bump-semver ... VERSION --write` 後の唯一の
  変更が VERSION なので、全 commit と path-commit が同結果に収束する。前提が
  崩れた将来の読者向けに本ノート 1 行で根拠を残す。
- **`.github/workflows/release.yml` の `update-homebrew` job (line 258-273)**:
  `git add Formula/bump-semver.rb` / `git diff --staged --quiet ||` /
  `git commit -m ...` / `git push` の 4 行 (旧 line 265-267) を、
  `bump-semver vcs diff -q HEAD -- Formula/bump-semver.rb` でファイルスコープ
  差分判定 → 差分あり時のみ `vcs commit -m "..." Formula/bump-semver.rb`
  + `vcs push --branch main` に置換。`git config user.name/email` 2 行は
  **維持** — bump-semver は git identity を自前設定しないため、fresh runner で
  identity 未設定だと内部の `git commit` shell-out が identity エラーで落ちる。
  `--branch main` 固定は `gh api repos/kawaz/homebrew-tap -q .default_branch`
  で `main` を 1 回裏取り済み。
- **`update-homebrew` job への install step 追加 (line 184-191)**: この job は
  `release.yml` 内で `check-version` とは別 runner なので bump-semver が PATH に
  無い。`actions/download-artifact` で展開された **今 release した版そのものの
  linux/amd64 バイナリ** (`artifacts/bump-semver-linux-amd64/...`) を
  `install -m 0755 ... /usr/local/bin/bump-semver` で配置。check-version 側の
  「直前 release を curl」方式と違い、今 release してる版そのものを使うので
  「commit/push サブコマンドの dogfooding が新版の挙動を検証」する構図になる。

#### 維持した箇所 (= 置換対象外)

- **`gh release create "v${VERSION}" ...` (line 169-174)**: tag 作成 +
  release notes 生成 (`--generate-notes`) + asset upload を **atomic に処理**
  する gh の責務。`vcs tag push` で tag だけ分解しても残り 2 つは別経路 (gh
  / GitHub API 直叩き) に頼ることになり、merit が薄いだけでなく atomicity を
  損ねる。DR-0020 PR-5 で `vcs push` に `--tags` を意図的に提供しなかった
  方針 (= 「tag push は release 自動化 = `gh release create` 側の仕事」) と
  整合。
- **`check-version` job の bump-semver install (curl 経由、line 24-44)**:
  ここは「直前 release を使って今 push してる VERSION の semver 妥当性を
  検証」する構造で、新版バイナリは build job 完走後しか作られないため
  artifact 経由は使えない。bootstrap fallback (= 過去 release ゼロ時に source
  build) も含めて既に dogfood の正しい形になっている。
- **README / README-ja の翻訳ペア**: kawaz 明示除外。

#### ドッグフーディングで検証される vcs サブコマンドの挙動

PR-7 で本 release.yml に乗った PR 自身の push (= v0.25.1) が、**変更後の
release.yml を初めて流す**構造になる。よって次の release 完走で以下が実機検証:

- `vcs diff -q HEAD -- PATH`: PR-3.1 のファイルスコープ presence 判定 (exit
  code 1 == 差分あり) が github-actions 環境で期待通り動くか
- `vcs commit -m MSG PATH..`: PR-4 の path モード commit (= `git add -- PATH`
  内蔵 → empty-no-op gate → `git commit -m MSG -- PATH`) が deploy key 越しの
  homebrew-tap clone 上で動くか
- `vcs push --branch main`: PR-5 の `--branch NAME` 必須形 + PR-5.1 の
  idempotent passthrough が ssh deploy key + remote URL 環境で動くか
- Taskfile 側の `vcs commit -m MSG VERSION`: jj backend 経由で `jj commit
  [FILESETS]... -m MSG` (PR-4 ノート参照) が kawaz の primary jj 0.41 環境で
  release commit を成立させるか

#### DR-0020 PR シリーズ完走 (2026-05-30)

| PR | verb / 内容 | land 時 VERSION |
|---|---|---|
| PR-1 | 基盤 + `vcs get` | v0.19.0 系 |
| PR-2 | `vcs is clean/dirty/git/jj` | v0.20.x |
| PR-3 | `vcs diff REV [PATH..]` | v0.20.x |
| PR-3.1 | `vcs diff -s/-q` + v0.20.2 verb-aware reject 修正 | v0.20.2 |
| PR-4 | `vcs commit -m MSG PATH.. \| --staged \| --amend` | v0.21.x |
| PR-4.1 | `vcs commit --amend [PATH.. \| --staged]` 受け入れ訂正 | v0.21.x |
| PR-5 | `vcs fetch [REMOTE]` / `vcs push --branch NAME` | v0.22.x |
| PR-5.1 | non-ff hint 削除 / `jj git export` retry + 復旧 hint / push passthrough | v0.22.x |
| PR-6 | `vcs tag push --rev REV NAME` / `vcs tag delete NAME` | v0.25.0 |
| **PR-7** | **セルフドッグフード (本 PR)** | **v0.25.1 (本 release)** |
| PR-2.1 | `vcs is clean` jj 判定にマージコミット短絡を追加 (kawaz 意図の誤読、PR-2.2 で revert) | v0.25.2 |
| PR-2.2 | PR-2.1 完全 revert (`empty` 単体に戻す、evil merge は dirty) | v0.25.3 |
| PR-5.2 | `vcs push --jj-bookmark-auto-advance` (jj-only opt-in、clean→`@-` / dirty→`@`、divergent → exit 3) | v0.26.0 |
| PR-5.2.1 | backend-prefix general rule (`--jj-*` / `--git-*` flag は他 backend で silent no-op、PR-5.2 の exit 2 reject を訂正) + justfile push 1 行化 | v0.28.0 |
| PR-Tag-Latest | `vcs tag latest [--source <tag\|release>] [--include-prerelease] [--repository REPO] [--raw\|--json]` (`vcs:latest-tag([REPO])` 入力を即削除して置換、v0 破壊的変更ポリシー) + release.yml 自己ドッグフード移行 | v0.29.0 |

DR-0020 はこれで設計 → 実装 → ドッグフードの 3 段を完了。以降の vcs 関連
変更は本 DR の延長 (= verb 追加 / 既存 verb 改修) として bug fix / 改善 PR で
回す。

### PR-Tag-Latest (`vcs tag latest`, 2026-06-01 確定)

**Status**: **Superseded by [DR-0032](./DR-0032-vcs-get-latest-by-source-verb.md) (2026-06-09)** — `vcs tag latest [--source <tag|release>]` は `vcs get latest-tag` / `vcs get latest-release` の 2 verb 分割に再整理 (= source 軸を verb 名に畳む)、`--raw` 廃止、`--json` schema を `get --json` と同一の version schema に統一、`vcs:latest-tag([REPO])` / `vcs:latest-release([REPO])` 入力 record 復活 (v0.32.0 で land)。下記の設計内容 (source matrix / 出力モード / `--include-prerelease` default / `vcs:latest-tag()` 削除戦略 / release.yml dogfood 移行 / v0.28→v0.29 transient) は v0.29.0 land 時の判断記録として保存、現在の振る舞いは DR-0032 を参照。

**追加 verb**: `vcs tag latest [--source <tag|release>] [--include-prerelease] [--repository REPO] [--raw | --json]`

**責務再定義**: bump-semver が tag を扱う**唯一の意味 = SemVer-like tag**。全 tag list は jj / git 自体で十分 (= 責務外)。`vcs tag latest` は SemVer 2.0.0 parseable な tag のみ filter して最大を返す。

**source matrix**:

| source | repository | 経路 | gh 依存 |
|---|---|---|---|
| `tag` (default) | (なし) | `backend.LatestTag()` (= jj/git tag list) | なし |
| `tag` | `owner/repo` or URL | `git ls-remote --tags <url>` | なし |
| `release` | (なし) | `gh release list` (cwd repo) | **必要** |
| `release` | `owner/repo` or URL | `gh release list -R <repo>` | **必要** |

`--source tag --repository <X>` を gh 不要にした判断: gh は GitHub Release オブジェクト固有の機能 (draft 除外 / publishedAt) を扱うときだけ必要で、純粋な tag list 取得は `git ls-remote` で十分。spec のシンプル不変条件「source が tag なら gh 不要」と既存 helper の再利用を両立させる。

**出力モード** (相互排他):

- default: bare SemVer (Prefix を落とす、例: `1.2.3`)
- `--raw`: 元 tag 文字列のまま (`v1.2.3` / `release-1.2.3` / `pkf-tasks@0.0.13`)
- `--json`: `{"tag":"v1.2.3","version":"1.2.3","commit":"...","date":"..."}` (commit/date は best-effort、source が提供する場合のみ埋まる)

**`--include-prerelease`**: 旧 `vcs:latest-tag()` は常に prerelease を含めていた。新コマンドの default は除外 (= リリース判定で「rc が選ばれて release より大きくなる」事故を防ぐ)。byte-identical 移行は `vcs tag latest --include-prerelease`。

**`vcs:latest-tag()` 削除戦略 (v0 破壊的変更ポリシー)**:

bump-semver は **v0.x = 不安定版** (kawaz 個人 OSS の運用規約) のため、deprecation 期間を設けず即削除する。実装上:

- `resolveVcsFunc` の `case "latest-tag":` を削除 — `vcs:latest-tag()` 入力は **unknown vcs function** エラーになる
- エラーメッセージに「vcs:latest-tag was removed in v0.29.0; use \`bump-semver vcs tag latest\` instead」を含めて移行先を明示
- `latestTagFromRemote` / `parseLsRemoteTags` / `expandRepoArg` / `pickLatestSemverTag` / `backend.LatestTag()` は新コマンドの実装に流用 (= dead code にしない)

**v0 段階での破壊的変更ポリシー** (本 DR で明文化):

- v0.x = 不安定版規約 (kawaz 個人 OSS): 破壊的変更を minor bump で許容、deprecation 期間は設けない
- 「deprecated 警告を出して数 release 維持」は v1.0.0 以降の正式版に乗ってから採用
- v0 段階の即削除は CHANGELOG 等で告知する (= 利用者は移行手順を見られる)

**release.yml dogfood 移行**: 本 PR で `vcs:latest-tag()` を使っていた `release.yml` の version-check ブロックを `vcs tag latest` capture-then-compare に書き換え (= bump-semver 自身が新 subcommand を実機で利用)。初回 release (tag 0 件) で `vcs tag latest` が exit 3 を返す bootstrap ケースも維持。`--include-prerelease` を明示することで旧 `vcs:latest-tag()` の byte-identical 移行 (= prerelease を含む filter) と整合 (README / UPGRADING の移行例と同じ形)。

**v0.28.0 ← v0.29.0 移行 transient (1 回限り)**: `check-version` job は「直前リリース版バイナリ」(`gh release list` の最新) を install して dogfood する設計 (PR-7 と同じ self-dogfooding 構図)。本 PR の release では install されるのが v0.28.0 で、これは `vcs tag latest` を知らないので unknown sub-verb で exit 2 を返す。release.yml は exit code != 0 を bootstrap (初回 release) と同じ分岐に流すため動作はする (= 「VERSION > 既存 tag」検証はこの 1 回スキップされ、二重 release は後段の `gh release view` が防ぐ)。次の release 以降は v0.29.0+ が install されるため通常分岐に乗る。これは PR-7 land 時の同種の transient (DR-0020 line 656 付近) と同じ family。

**既知の下流影響 (v0 break)**: DR-0019 言及のとおり `kawaz/pkf-tasks` の `migrate:check-pkf-tasks-current` が `vcs:latest-tag(kawaz/pkf-tasks)` を直接使う想定。v0.29.0 upgrade で `vcs tag latest --repository kawaz/pkf-tasks` への書き換えが必要 (= 単純な capture-then-compare 移行)。v0 policy では deprecation 期間を設けずに即変更する。

**land 日**: 2026-06-01。

- **PR-7 land 日**: 2026-05-30
- **PR-2.1 land 日**: 2026-05-31 (誤読、PR-2.2 で revert)
- **PR-2.2 land 日**: 2026-05-31
- **PR-Tag-Latest land 日**: 2026-06-01

## 関連

- 上位/関連 DR: DR-0008 (`vcs:` schema 導入)、DR-0016 (`--vcs auto` 一本化)、DR-0019 (`vcs:latest-tag(<arg>)`)
- 設計議論の経緯 + jj 一次情報調査: `docs/journal/2026-05-30-vcs-subcommands-design.md`
- 実装着手時の調査 + PR 分割: `docs/journal/2026-05-30-vcs-subcommands-impl-kickoff.md`
- ROADMAP: `docs/ROADMAP.md` (vcs サブコマンド群の実装項目)
