# DR-0033: `--excludes PATTERN` flag + `file:LIST` 入力 prefix 追加

- Status: Active
- Date: 2026-06-10
- Extends: DR-0024 (`glob:` prefix + `--glob-*` family、`file:LIST` 将来案と exclude pattern future scope を本 DR で land)
- Related: DR-0020 (vcs subcommand family)、DR-0027 / DR-0028 (`vcs outdated` の glob-backref spec)

## Context

DR-0024 で `glob:` prefix と `--glob-dotfile` / `--glob-gitignored` / `--glob-ignorecase` family を導入したが、以下を **MVP 範囲外として明示的に scope-out** した:

1. **exclude pattern** (= 「include の中からこれを除く」): 引数渡しが複雑化、表現難。**将来 `file:LIST` で代替** と記述。
2. **`file:LIST` 入力 prefix** (= 外部 file から path list を流し込む): DR-0024 の RoadMap 上では "future" として登録。

v0.32.1 land 直前に `release.yml` / `justfile` の `check-version-bumped` task で「src/ 配下の変更で bump-trigger するが、**test file (`*_test.go`) は除外したい**」という実需が顕在化。test 追加だけでも check-version-bumped が VERSION bump を要求してしまい、patch release の連発に繋がる。

具体的に発生した事象 (= v0.32.1 release):
- DR-0032 land 後に test coverage を追加 (`vcs:latest-tag()` の monorepo-style / mixed / no-tags 観点)
- 純粋な test 追加で behavior 変更ゼロ、release 不要のはず
- `check-version-bumped` が `src/` 配下の diff を検知して VERSION bump を要求
- 結果: 0.32.1 patch bump を実行、release.yml が自動 release を起こす (= 「test 追加だけの release」が記録された)

この種の noise release を防ぐには `bump-trigger` 対象から test を除外する仕組みが必要。

## Decision

### `file:<path>` 入力 prefix の追加

DR-0024 の `glob:<pattern>` と並列の **入力 prefix** として `file:<path>` を追加する。

```
file:<path>
```

挙動:
- `<path>` が指す file を読み、**1 行 = 1 path** として展開
- 各行は **literal path** か **`glob:` prefix** を受け付ける (= 行内で再帰的に展開、`file:` のさらに再帰はサポート外)
- `#` で始まる行 / 空行は **スキップ** (= コメント / 区切り目的)
- file が存在しない / 読めない場合は usage error (= exit 2)
- 行内の `glob:` パターンは外側の `--glob-*` flag に従う (= `--glob-gitignored=false` を指定すれば file: 内 glob: も尊重しない)

例:
```
# .bump-semver-files (excludes/includes 共通の path list file)
src/main.go
src/parse.go
glob:src/handlers/*.go
# テストは含めない (= コメントで意図記録)
```

```bash
bump-semver vcs diff REV file:.bump-semver-files
```

### `--excludes PATTERN` flag の追加

`vcs diff` / `vcs commit` / `vcs outdated` (TO 側) の各 verb で **post-filter** として exclude pattern を受け付ける。

```
bump-semver vcs diff [REV] [PATH..] [--excludes PATTERN]...
bump-semver vcs commit [-m MSG] [PATH..] [--excludes PATTERN]...
bump-semver vcs outdated FROM TO[..] [--excludes PATTERN]...
```

挙動:
- `PATTERN` は **literal path** / **`glob:`** / **`file:`** のいずれか (= 位置引数と同じ shape を受け入れる)
- 必須引数 + 複数指定可 + append (= 各 `--excludes` を独立した exclude pattern として登録、すべて評価)
- **post-filter (= 順序非依存)**: 最終 set = include set ∖ exclude set
- include set が空 (= 位置引数省略) → **全 path include** → exclude 適用
- include set が空 (= 位置引数を与えたが全 expand 結果 0) → **空のまま** (= DR-0020 declarative-convergence ルール継承)

### 設計原則

#### 原則 1: post-filter (= 順序非依存)

include / exclude の **順序に意味を持たせない**。利用者が `--excludes` を位置引数の前/後/間どこに置いても結果は同一。

利点:
- release gate コマンド (= CI で機械実行) で「順序 typo で silent green」を回避
- mental model がシンプル (= 「include 集合から exclude 集合を引く」だけ)
- git pathspec の `:!pat` も post-filter、慣習一致
- help 記述コストが小さい

不採用: rsync-style 順序依存 (= `--include` と `--exclude` を順番に適用、後勝ち / 再 include 可能)
- 再 include の use case は `vcs diff` (= release gate / path filter) で実需が薄い
- rsync の pathspec は世間で「最も誤読される」CLI 構文の 1 つ (= stackoverflow 質問数の多さ)
- 順序ハマりは CI で silent failure を起こしやすい

#### 原則 2: `!`-prefix shorthand 非採用

位置引数で `!pattern` を exclude shorthand として解釈する案を **明示的に却下**。

理由:
- **gitignore セマンティック反転**: gitignore では `!pat` = 「ignore 解除 (= include)」、本 DR の include = exclude の対称設計だと逆極性。debug 時に気づきにくいタイプの誤動作を生む
- **git pathspec 慣習**: git 自体は `:!pat` (= 2 文字 prefix) を採用、`!pat` 単独は使わない (= gitignore との衝突回避)
- **bash `!` history expansion**: `!foo` を素で書くと shell の history 展開と衝突、クォート必須を強要する
- **位置引数に include/exclude を混在させると順序依存に見える**: 原則 1 で post-filter を選んだのに、見た目で「順序に意味ありそう」と誤読される。認知負荷の半分を背負い直す

代わりに `--excludes` flag 一本で表現、include は位置 / exclude は flag の **明確な分離** で混乱回避。

将来どうしても shorthand が必要になったら `:!pat` (= git pathspec 流儀の 2 文字 prefix) を採用、`!pat` 単独は永久に予約しない。

#### 原則 3: `glob:` / `file:` は include / exclude 両方で動く

`PATTERN` の shape は位置引数と完全に対称。これにより:
- `--excludes file:.bump-semver-exclude` で外部 file 由来の exclude list 運用が可能
- `vcs diff REV file:.bump-semver-files --excludes glob:**/*_test.go` のような mixed 利用が自然
- 将来 `glob:` / `file:` 以外の prefix (= `cmd:` 等) を追加する場合も exclude 側に対称展開可能

#### 原則 4: gitignore 関連の挙動は変更しない

DR-0024 の `--glob-gitignored=true|false` family はそのまま、`file:LIST` 内の `glob:` パターンにも適用される (= 外側 flag が内側展開を制御)。

`vcs diff` / `vcs commit` 自体は tracked content snapshot 同士の比較なので、未追跡 file (= `node_modules` 等) は `.gitignore` の有無に関わらず diff に出ない (= 本 DR の責務外)。

### 適用 verb (= 本 DR の land 範囲)

| verb | `--excludes` | `file:` 位置引数 |
|---|---|---|
| `vcs diff` | **land** (= 本 DR の immediate need) | **land** |
| `vcs commit` | follow-up (= 別 DR で land) | follow-up |
| `vcs outdated` (TO 側) | follow-up (DR-0027 / DR-0028 の glob-backref spec との統合検証要) | follow-up |
| `get` / bump 系 (= `glob:` 既対応) | 検討 (= 利用ケース次第) | follow-up |

本 DR の land 対象は `vcs diff` のみ。release.yml dogfood の用途に対応できれば immediate need 解消。`vcs commit` / `vcs outdated` への展開は実需が顕在化したタイミングで follow-up。

### dogfood 移行

`justfile` の `check-version-bumped` task を以下に書き換え:

```just
check-version-bumped: (_check-version-bumped "src/" "go.mod" "go.sum")

_check-version-bumped *target_paths:
    if ! bump-semver vcs diff -q main@origin -- "$@" --excludes 'glob:src/**/*_test.go'; then
        bump-semver compare gt VERSION vcs:main@origin
    fi
```

これにより、`src/` 配下の test 専用追加 (= `*_test.go` のみ変更) では bump-trigger が発火しなくなる。本体コード (= 非 test の `.go` file) が変更されたときのみ VERSION bump を要求する。

## 代替案検討

### 不採用: rsync-style 順序依存 filter

`vcs diff REV --excludes A path1/ --excludes B path2/` の各 flag/位置を **順次評価** し、後の flag が前の結果を上書きする方式。再 include 可能。

不採用理由 (= 原則 1 で詳述):
- release gate での silent green / silent red リスク
- 利用者の認知負荷が大きい
- 実需 (= 再 include) が薄い

### 不採用: `!pattern` shorthand

位置引数で `!pat` を exclude として扱う方式。

不採用理由 (= 原則 2 で詳述):
- gitignore セマンティック反転
- bash `!` history expansion
- 位置引数で include/exclude を混在させると順序依存に見える

### 不採用: `--include` flag 追加で位置引数を deprecate

位置引数 = include の暗黙慣習を捨て、`--include` / `--excludes` 両方を flag 化する案。

不採用理由:
- 既存 verb (= `vcs diff REV PATH..`) の互換性を破壊
- 位置引数 = include は git/jj 自身の慣習 (= `git diff -- PATH..`) と一致、利用者の mental model に合う
- flag 重複で typing コスト増 (= 大半の caller は include のみ書く)

### 不採用: 各行 file:LIST が `file:` 再帰展開

`file:.bump-semver-files` の中の行で `file:another-list` を書ける方式。

不採用理由 (MVP scope-out):
- 再帰展開 = 循環検出 + depth limit のコスト
- 実需が薄い (= 1 階層で大半の use case が回る)
- 将来必要になったら追加可能 (= forward-compatible)

## 影響範囲 / migration

### 内部実装 (= v0.33.4 phase 2 v2 land 時点の最終形)

- `src/file_input.go`: `hasFilePrefix` / `parseFileSpec` / `expandFileSpec` / `readPatternListFile` (= `--excludes file:` 用)。nested `file:` は両 helper で reject (= MVP scope-out、字面 check で循環防止)
- `src/glob.go`: `expandGlobInputs` を glob:/file: 両対応に拡張、`dedupPreserveOrder` を出口に追加。旧 `excludeInputs` (set-subtraction) は test 用 helper として残置
- `src/cli_parse.go`: `--excludes PATTERN` flag parser (= `vcs diff` verb-local)、`vcsDiffOpts.Excludes []string` field 追加
- `src/vcs_backend.go`: `Diff` / `DiffNameStatus` signature に `excludes []string` 追加。`buildGitPathspec` / `buildJjPathspec` / `trimGlobPrefix` 新規
- `src/vcs_cmd.go` `runVcsCmdDiff`: include は `expandGlobInputs` で literal pass、excludes は `flattenExcludePatterns` で `file:` を展開、backend に forward
- `src/help.go`: `vcs diff` help に `--excludes` 説明、入力モード表 (`vcs:` / `cmd:` / `glob:` 一覧) に `file:` 追加

### test 追加範囲

- `file:LIST` 基本展開 (= literal / glob: / コメント / 空行)
- `file:` の file 不在 → exit 2
- `file:` 内の `file:` (= 再帰非対応) → 適切なエラーまたは literal 扱い
- `--excludes` 基本動作 (= include set ∖ exclude set)
- `--excludes` repeatable (= 複数指定で全部評価)
- 位置と flag の順序非依存性確認 (= 順序入れ替えで結果同一)
- include 空 (= 位置引数省略) + `--excludes` のみ → 全 - exclude
- include 空 (= 位置引数を与えたが expand 結果 0) + `--excludes` → 空のまま
- exclude 0-match → include set 不変
- exclude が include 完全包含 → 空
- `glob:` / `file:` を `--excludes` 値として混在

### 外部 user 影響

- 後方互換: 既存呼び出しは破壊しない (= `--excludes` 省略時は現状動作と同一、`file:` を含まない呼び出しは現状動作と同一)
- v0.32.x → v0.33.0 minor bump (= 新機能追加、breaking 無し)

### CHANGELOG / docs

- README / README-ja: `vcs diff` 説明に `--excludes` 例追加、`file:` 入力 prefix を入力モード表に追加 (翻訳 pair 同期)
- `docs/DESIGN.md` / `docs/DESIGN-ja.md`: 既存の `vcs:` / `cmd:` / `glob:` 入力 prefix セクションに `file:` を追加
- `docs/decisions/INDEX.md`: DR-0033 を Active section に追加
- `docs/decisions/DR-0024-glob-prefix.md`: 「将来 `file:LIST` で代替」「exclude pattern future scope」の記述を「DR-0033 で land 済」に書き換え

## land 順

1. DR-0033 起票 (本 file) + DR-0024 の future scope 記述を更新 + INDEX 更新
2. 実装: `file:LIST` 展開 + `expandInputs` 拡張 + `--excludes` flag + `vcs diff` 統合
3. test: 上記 test 追加範囲
4. help / docs: README / README-ja / DESIGN / DESIGN-ja 同期更新
5. justfile `check-version-bumped` dogfood 移行 (= `_test.go` 除外)
6. VERSION bump v0.33.0
7. push (= `just push` 経由) → CI watch → release workflow

## 補足: phase 2 で land 済 (2026-06-10) — literal directory 透過対応 + 削除 file 検知

DR-0033 land の翌日に、literal directory selector に対する file-level exclude が動かない問題、および削除 file が見逃される問題を是正した。

### v0.33.2 (= phase 2 v1、後に revert)

最初の試行: `expandVcsPathInputs` helper で literal directory を内部的に `glob:<dir>/**/*` 展開 (= dotfile 強制 on)、その後 set-subtraction で excludes を引く設計。

`vcs diff src/ --excludes 'glob:src/**/*_test.go'` の include 期待通り動作するようになったが、**致命的 bug が露呈**:

> `doublestar.FilepathGlob` は **現状の file system しか walk しない** ため、REV に存在し working copy で削除された file が include set に含まれず、`git diff` / `jj diff` の出力にも出ない (= 削除検知が消える)。

codex stop-time review が指摘。

### v0.33.4 (= phase 2 v2、現実装)

設計を **backend pathspec への forward** に転換:

- `vcsBackend.Diff` / `DiffNameStatus` signature を `(rev, paths, excludes)` に拡張
- **git**: exclude pattern を `:(exclude,glob)<pat>` magic pathspec 長形に変換して append (= `:!pat` 短縮形は `**` を解釈しないので不採用、実機検証済)
- **jj**: fileset 言語の単一式 `(inc1 | inc2) ~ exc1 ~ exc2` に組み立てて 1 引数として渡す (= 別 args 渡しは jj が union 解釈で wrong、jj 0.41 で実機検証済)
- `file:` 形式の excludes は dispatcher 側で flat pattern list に展開してから backend に渡す (backend は `file:` shape を知らない)
- `expandVcsPathInputs` / 自前 set-subtraction は削除、include 側は `expandGlobInputs` (= glob: / file: 展開のみ) で backend に literal pass
- `expandGlobInputs` 出口に `dedupPreserveOrder` を追加 (= include 重複排除)

これにより:

- **削除 file 透過**: backend (= git/jj) が REV と現状を比較するので、REV にあって working で削除された file も diff に出る
- **literal directory 透過**: `vcs diff src/ --excludes 'glob:src/**/*_test.go'` で `src/` は backend pathspec として、test 除外も backend pathspec として両立
- **argv scaling**: 自前で 1000-file 展開する必要なし (= include / exclude pathspec 個別 entry)
- **backend native 表現力活用**: git / jj 自身の pathspec / fileset 機能 (= magic pathspec / 演算子) に乗れる

### get / bump / compare 系の扱い

`expandVcsPathInputs` は削除済 (= v0.33.4 で revert)。`expandGlobInputs` (= glob: / file: 展開のみ) は引き続き全 verb 共通の入口。`vcs diff` だけが backend pathspec forward を追加で行う。`get` / `bump` / `compare` 系は backend pathspec を介さない (= FILE content を直接読む) ので、本 DR の影響を受けない。

## Security: shell injection 不可能、pathspec syntax 衝突は UX issue として受容 (v0.33.4 で確認)

backend pathspec forward (= phase 2 v2、`:(exclude,glob)pat` / jj fileset) で
user 入力を string 連結する経路があるため、injection リスクを確認した。

**1. shell injection は不可能**

`exec.Command("git", args...)` / `exec.Command("jj", args...)` は **`/bin/sh` を
経由しない**。シェルメタ文字 (`;` / `&&` / `||` / `$()` / バックティック / `>`
リダイレクト 等) はすべて argv の literal 文字として渡る。user が
`--excludes ';rm -rf /'` を渡しても、git/jj が受け取るのは literal pattern
`:(exclude,glob);rm -rf /` で、ファイルマッチに失敗するだけ (= no shell exec)。

**2. jj fileset breakout の可否**

`buildJjPathspec` は単一 fileset 式を文字列連結で組み立てる:

```go
sb.WriteString(p)  // user-controlled exclude pattern
```

user が `--excludes ') ~ ./important'` を渡すと、結果の fileset 式は
`(includes) ~ ) ~ ./important` となり jj は **parse error** を返す。コード実行や
exclude 範囲の不正拡張には繋がらない (= jj 側が syntax として reject)。

仮に演算子付き pattern で「他の場所を巻き込んだ exclude」を狙っても、
- exclude を追加する pattern (`X ~ Y`) → そもそも user は `--excludes Y` を別途指定可能、増分情報ゼロ
- include を狭める pattern (`X & Z`) → exclude セクションでは include 側に効かない (= 単純な差分)

→ 攻撃シナリオなし。

**3. git magic pathspec breakout の可否**

`buildGitPathspec` は `":(exclude,glob)" + trimGlobPrefix(e)` で連結。git の
magic は **nested しない** (= `:(exclude,glob):(top)X` の `:(top)` は body の
literal char として解釈、再 magic として扱われない)。

user が `--excludes ':(top)X'` を渡しても、git は `:(exclude,glob):(top)X` を
「exclude+glob magic、pattern body は `:(top)X`」と解釈する。`:(top)` は
literal の `:`、`(`、`t`、`o`、`p`、`)` の連続 char で、何にも match しないだけ。

→ 攻撃シナリオなし。

**4. pathspec syntax 衝突は UX issue として受容**

実機検証 (v0.33.4 land 時):

| ケース | git の挙動 | jj の挙動 |
|---|---|---|
| `src/file with space.go` | ✓ argv 個別 token、literal 解釈 | ✓ 同 |
| `src/sub(weird)/x.go` (= `()` 含む dir) | ✓ literal 扱い | ✓ literal 扱い |
| `--excludes 'src/x ~ glob:**/main.go'` (= injection 風) | ✓ literal pattern、何も match せず安全 | parse error (jj fileset の `~` を演算子と誤解釈、ただし攻撃にはならない) |

- パス内の `(` `)` `~` `&` `|` 等が **jj fileset の演算子** と衝突する場合、jj は
  parse error を返す (= 該当 file が exclude されず、おそらく diff 結果に出る、
  または backend が error 終了)
- パスが `:` で始まる場合、git は magic pathspec と誤解釈する可能性

これらは **「変な名前のファイルを使うユーザ側の責任」** として明示的に受容する
(= bump-semver 側でエスケープ層を追加するコストが高く、99% の利用者には不要)。
レア case で問題が顕在化したら、`:(literal)` 風の明示エスケープを後出し追加可能。

## 補足: 他 verb への将来展開 (= phase 3 候補)

`vcs commit` / `vcs outdated` への `--excludes` / `file:` 適用は、本 DR の land 後の実需観察で判断:

- `vcs commit --staged PATH.. --excludes`: 「staged 全部からこれだけ除外して commit」は git/jj の標準 idiom にない (= staging area 操作で先に excluded すべき)、需要薄そう
- `vcs outdated FROM TO[..] --excludes`: TO 側の探索結果から特定 path を除外したい用途は出てきうる、DR-0028 の glob-backref spec との統合は要検証

`--excludes` のセマンティクスは「include ∖ exclude」の集合差分で固定されているので、他 verb に展開する際の interface は同じ shape で増設できる (= 認知負荷を増やさない)。phase 2 で確立した backend pathspec forward 方式 (= git `:(exclude,glob)pat` / jj fileset `(inc) ~ exc`) を他 verb の backend 実装でも再利用可能。
