# DR-0029: CLI から「自分のファイルにこの rule」を指定する口 (Phase 1)

- Status: Active
- Date: 2026-06-03
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」), DR-0005 (path-aware confidence ranked candidates), DR-0010 (fallback hint), DR-0023 (N-arg + `vcs:` borrow), DR-0024 (`glob:` prefix matcher), DR-0027 (`regex:` 不採用), DR-0028 (glob spec v0.1.0)
- Prerequisite: DR-0030 (format=regex 廃止 + format=text + version-regex 統合)。本 DR の `--format` enum (5 値: text/json/yaml/toml/xml) は DR-0030 後の internal 整理を前提とする。実装順は **DR-0030 → DR-0029**。

## Context

bump-semver は basename / glob → builtin rule の自動判定で「シンプルに使えること」を
core に据えてきた (DR-0001 / DR-0005)。release 自動化が実用段階に入り、他者に勧める
段階で **未知ユーザが自分のファイルを扱えない場合の体験が悪い** という課題が顕在化:

1. builtin 未対応の自社製フォーマットを扱いたい
2. 既存 builtin と被るが project 固有の path で値を抽出したい (= override)
3. 借用形式 `vcs:REV` (DR-0023) の rule も含めて一貫制御したい

逃げ道は 2 つあり:

- **「対応してほしい」要望窓口** (= 別 issue: ISSUE_TEMPLATE / CONTRIBUTING 整備)
- **「自分で指定できる口」** (本 DR)

本 DR は後者のみを扱う。core 哲学 (「最小限で済む」) を壊さず、必要な人が必要な分だけ
明示指定できる入口を CLI に開ける。

## Decision

### Syntax: `--define-rule <PATTERN>` ブロック方式

新規 flag を 6 つ追加 (= get / compare / bump 全 verb で使用可):

| flag | 役割 |
|---|---|
| `--define-rule <PATTERN>` | ブロックスコープを開く (= 以降の rule 系 flag は `<PATTERN>` にマッチする SOURCE に紐付く) |
| `--format <text\|json\|yaml\|toml>` | ファイル構造分類 (= parser 選択) |
| `--version-path <DOTPATH>` | 構造化 path で version 抽出 (json/yaml/toml/xml)、text では error |
| `--version-regex <PATTERN>` | regex で version 抽出 (text 必須、json/yaml/toml/xml で path 値への 2 段抽出 or 全文 regex として併用可) |
| `--name-path <DOTPATH>` | name 抽出 (構造化、optional) |
| `--name-regex <PATTERN>` | name 抽出 (regex、optional) |

`--format` enum は `text|json|yaml|toml|xml` の 5 値。xml は json/yaml/toml と **同じ
dot-path 言語** を使う (= パス言語統一)。XML の構造差 (node が child element と attribute の
両方を持つ) は、最終 path セグメントを **child / attribute 両方で解決** することで吸収する:

- どちらか一方だけ値あり → それを採用
- 両方あり & **同値** → 採用 (peer 一貫性の精神、write 時は両方書き換え)
- 両方あり & **異値** → ambiguous error
- どちらもなし → not found error

XML の textContent は読み取り時に trim、write は trim 前後の空白を維持して値部分だけ
byte-range 置換する (= path+regex 併用 write と同じピンポイント修正の原則)。

`--define-rule` の前 (= 最初の `--define-rule` 出現前) に書く rule 系 flag は **グローバル**
スコープで、全 SOURCE のデフォルトとして機能する。

### PATTERN match strength

複数 SOURCE と複数 `--define-rule` の組み合わせから、各 SOURCE につき 1 rule を選ぶ。
選び方は match strength の高い順:

| Strength | matcher | 例 |
|---|---|---|
| 5 | 絶対パス完全一致 | `--define-rule /home/x/proj/package.json` vs `/home/x/proj/package.json` |
| 3 | 相対パス完全一致 (cwd 起点で正規化後の比較) | `--define-rule othersystem/package.json` vs `othersystem/package.json`、`--define-rule ./package.json` vs `package.json` も同 strength (= 正規化後一致) |
| 2 | basename 一致 | `--define-rule package.json` vs `ts/package.json` |
| 1 | glob マッチ | `--define-rule glob:*.myapp` vs `foo.myapp` (`glob:` prefix が付いた PATTERN は meta 有無問わず常に 1) |
| — | (no `--define-rule` match) → builtin にフォールバック | 内部の CandidateRule (DR-0005 confidence と同じ ranking) |

PATTERN と SOURCE は両方とも `filepath.Clean` + cwd 起点で正規化してから比較する。
**symlink resolve は行わない** (= macOS の `/tmp` vs `/private/tmp` は別 path 扱い、
symlink で別名を付けている設計意図を尊重)。

### PATTERN 仕様

- 基本: 1 文字以上の path string、空文字 / `/` 単独は error
- 先頭 `/` → 絶対パス指定 (strength 5)
- それ以外 (`./` 始まり含む) → 相対パス指定 (strength 3、`./X` は正規化後 `X`)
- `/` を含まない単一 segment → basename 指定 (strength 2)
- `glob:` prefix → glob 指定 (strength 1)。matcher は DR-0024 の `doublestar.Match` ベース
  (= capture / substitute は本 DR では使わない、DR-0028 の `**` 0-segment / `path.Clean`
  等の substitute 用拡張も本 DR の matcher 用途では未使用)
- bare PATTERN は **完全 literal**。`*` / `?` / `[` / `{` 等の glob meta が含まれた場合は
  **error** (hint: 'use glob: prefix for glob pattern')。glob 動作が欲しい場合は
  `glob:` prefix 必須
- PATTERN の先頭 `-`: `--` separator または `glob:` prefix の後に書く (= argparse の
  flag と紛れないように)
- `regex:` prefix は未採用 (DR-0027 と整合)
- **`vcs:` は PATTERN として書かない**。`vcs:` 入力の rule 解決は実 path に正規化される:
  - **借用形式** (`vcs:REV`、兄弟 FILE から peer-expand): peer-expand された各 vcs source は
    対応する兄弟 FILE 側の rule をそれぞれ独立に借用 (例: `get a.json b.json vcs:main` で
    `vcs:main:a.json` は `a.json` の rule、`vcs:main:b.json` は `b.json` の rule)
  - **単独形式** (`vcs:REV:FILE`): VCS root 相対で FILE を解釈。PATTERN とマッチさせる時も
    **PATTERN を VCS root 相対に正規化** してから比較

### tier vs builtin confidence の axis 関係

本 DR の strength scoring は **SOURCE と PATTERN の紐付け解決** という axis。DR-0005
confidence は **builtin 内部の rule fallback** という別 axis。両者は **直交**:

1. `--define-rule` ヒット判定 (strength 1+ で 1 つ確定) → ヒット時はそれで rule 確定
2. ヒット無しなら builtin の confidence 3 → 2 → 1 fallback (= DR-0005 そのまま)
3. DR-0010 fallback hint は (2) の confidence 1 マッチで従来通り発火 (= 本 DR で挙動変更なし)

**CLI rule (strength 1+) は builtin (confidence 0 相当) より常に優先**。

### Ambiguous / dead / 失敗時の規約

- **同 strength 重複**: 1 SOURCE が複数 `--define-rule` に同 strength でヒット → **error**
  (= 構造的曖昧さを silent に決めない)。glob 同士の重複は機械的 precedence 判定不能のため
  常に error
- **重複 SOURCE**: 同一 path を複数 positional に書いた場合 (= `bump-semver get
  package.json package.json`) は **dedup しない**、各 SOURCE は独立に rule 解決
- **CLI rule extraction failure**: CLI rule (`--define-rule` or グローバル) が strength
  1+ でマッチした後、その rule で version/name の抽出が失敗した場合は **hard error**
  (= builtin への自動 fallback なし)。error message は source path + matched PATTERN +
  失敗 field + 原因 (regex no match / regex multi match / path not found / non-string
  scalar / type mismatch 等) を含む
- **dead block**: invocation 内に「どの SOURCE にもマッチしなかった `--define-rule`
  ブロック」がある → **error** (= ユーザが書いた `--define-rule` が silent に無視
  されると debug 困難)

### Flag のスコープ規約

`--format` / `--version-regex` / `--version-path` / `--name-regex` / `--name-path` は
書く位置でスコープが決まる:

| 位置 | スコープ |
|---|---|
| 最初の `--define-rule` より前 (or `--define-rule` 不在) | グローバル (= 全 SOURCE のデフォルト) |
| `--define-rule <PATTERN>` の後 | ブロック (= PATTERN にマッチした SOURCE のみ)、positional SOURCE はブロックを跨いで透過 |
| 最初の `--define-rule` 以降のブロック外位置 | 構造的に発生しない (= `--define-rule` の typo / 抜けでしか観測されず、argparse の unknown option error or 最後の評価規則で error) |

**評価規則**:

1. 各 SOURCE につき 1 rule を選ぶ
2. SOURCE にマッチした最も具体的なブロック (= strength 最大) を 1 つ特定する。複数同
   strength 衝突は ambiguous error
3. 該当ブロックが存在し、そのブロック内に rule 系 flag が **1 つでも** あれば、その
   ブロックは **rule 完全宣言** とみなす:
   - `--format` を含む全 rule 系 flag をブロック内のみで評価
   - グローバルからも builtin からも継承しない (= CLI rule の部分継承禁止)
   - 必須 flag (= `--format` と `--version-path`/`--version-regex` のどちらか) が不足する
     場合は error
4. 該当ブロックが存在するが flag が空 (= 「`--define-rule X --define-rule Y --format
   json`」のような X block) → error (= 空 block は意味なし、明示禁止)
5. 該当ブロックが存在しない場合は **グローバル rule** を評価:
   - グローバルに rule 系 flag が 1 つでもあれば、その内容で rule 完全宣言とみなす
   - グローバルにも rule 系 flag が無ければ builtin にフォールバック

これにより「block で `--format` だけ書いたら global の `--version-regex` が leak する」
という曖昧解釈は排除される。block と global はそれぞれ独立した「rule 完全宣言」単位。

**ブロックの終了**:

- 次の `--define-rule` で次のブロックが開く
- invocation 終了で最後のブロック確定
- 明示的なブロック終了 flag は不要 (= 設計過剰)

**同 block 内の flag 重複** (= `--format json --format text`) → **error** (= last-write-wins
より surprise 少、意図不明)

### Path / Regex 併記時の挙動

`--version-path` と `--version-regex` は併記可能:

| `--version-path` | `--version-regex` | 挙動 |
|---|---|---|
| あり | なし | path で取得した値 = version |
| なし | あり | 全文に regex を適用、capture group 1 = version |
| あり | あり | path で取得した値に regex を適用、capture group 1 = version |
| なし | なし | error |

`--name-path` + `--name-regex` も対称。

### `--version-regex` の cardinality 規約

CLI `--version-regex` は **builtin (DR-0012 / first-match-only) よりも厳格** な cardinality
を採る:

- get / compare 時: regex を当てて **exact one match** を期待。0 match も 2+ match も
  **error**
- bump --write 時: 同じく exact one match、error 発生時は書き込み一切なし (= atomicity)
- 理由: user-defined rule は明示指定であり、複数候補行の silent な first-match 採用は
  debug 困難で誤書換え事故の温床。builtin の first-match-only (DR-0012) はテストで担保
  された安全領域だが、CLI rule は任意 regex を許すため exact-one が安全側
- `(?m)^` line-anchor は推奨だが強制しない (= 任意 regex の柔軟性は残し、exact-one 制約
  で誤書換えを防ぐ)
- 同規約を `--name-regex` にも適用 (= 対称性)

**builtin との挙動差**:

- builtin `regex` 系 (DR-0030 後は `text + version-regex`): first match only (DR-0012、
  `(?m)^` line-anchor で 1 マッチに収束させる前提)
- CLI `--version-regex`: exact one match (= 厳格化、ユーザ明示指定の責任)

### bump --write の atomicity + 書き戻しアルゴリズム

複数 SOURCE 一括 write 時の atomicity:

1. 全 SOURCE について rule 解決 + version/name 値抽出 + 整合性検証 (= 全 source で同
   version) を先に全件実行
2. 全件成功してから初めて書き込み開始 (= 1 SOURCE でも失敗したら 1 ファイルも書かない)
3. 書き込み中の物理 IO 失敗 (= partial write 後の rollback) は別問題で本 DR scope 外

**書き戻しアルゴリズム** (= `--version-path` と `--version-regex` の組み合わせ別):

1. **`--version-path` のみ**: path で取った scalar string を version として読み、書き戻し時は
   path 値全体を新 version 文字列で完全置換
2. **`--version-regex` のみ** (path なし): ファイル全文に regex を当て、capture group 1 の
   byte range だけを新 version 文字列で置換 (= DR-0012 と同じ挙動、line-anchored 推奨)
3. **`--version-path` + `--version-regex`** (併用): path で scalar string を 1 個取得
   (非 string scalar / array / object は error)、その string に regex を適用して capture
   group 1 を取得、group 1 の byte range だけを新 version に置換 (= prefix / suffix は
   preserve)、書き換えた新 string を元の path へ scalar として戻す

例 (`info.json` の `$.name` から `"myapp v1.0.5"` を取り、regex で `1.0.5` を抜く場合の
patch bump):

```
bump-semver patch info.json --format json --version-path '$.name' \
  --version-regex 'v(\d+\.\d+\.\d+)' --write
# before: {"name": "myapp v1.0.5"}
# after:  {"name": "myapp v1.0.6"}
# → $.name 全体を置換せず、regex group 1 (= "1.0.5") の byte range だけ "1.0.6"
#   に置換、prefix "myapp v" は preserve
```

### name safety rail

builtin の既存挙動 (= multi-input 時に name も cross-check して「別 project を一緒に
bump しない」guard) を user-defined rule でも維持:

- user-defined rule で `--name-regex` / `--name-path` を **書いた** source は name-check
  対象 (= 値が一致しない他 source と組み合わせると mismatch error)
- 書かなかった source は name-check 対象から除外されるが、stderr に **warning hint** を
  出す ("note: source <path> has no name source, multi-source name consistency check
  skipped for this entry; consider adding `--name-regex` / `--name-path` if this is a
  multi-project bump")
- silent downgrade はしない (= name-check が暗黙にスキップされて別 project を一緒に
  bump する事故を防ぐ)
- `--no-hint` / `-q` / `-qq` で抑制可能 (= 既存規約と整合)

### dot-path 仕様 (MVP 最小 subset)

`--version-path` / `--name-path` に書ける文字列:

```
plugin.version              # object access (= obj.plugin.version)
deps[0].version             # array index + object access
$.plugin.version            # JSONPath 風 (先頭 $ optional)
```

`jq` / JSONPath の完全互換は MVP scope 外。`.` (key) + `[N]` (index) + 先頭 `$` の 3 要素
のみ。`"key with dot"` の quoted key は将来検討。yaml / toml / xml も同じ dot-path 構文を
共有 (= parse 結果を tree として扱う。xml は最終セグメントを child/attribute 両方で解決)。

### Help / docs 配置

bump-semver は `bump-semver --help` (short) と `bump-semver --help-full` (long) の 2 段
+ 各 verb の `--help` を持つ。本 DR の新規 flag の配置:

**`bump-semver --help-full` の "Builtin rules" 表** (= DR-0030 後の表記、Confidence 列付き):

```
Builtin rules:

  Path matcher           Confidence  Format  Version source                  Name source
  ─────────────────────  ──────────  ──────  ──────────────────────────────  ──────────────
  package.json               3       json    $.version                       $.name
  Cargo.toml                 3       toml    $.package.version               $.package.name
  pyproject.toml             3       toml    $.project.version               $.project.name
  *.podspec                  2       text    (?m)^\s*s\.version\s*=\s*"..."  s.name = "..."
  build.zig.zon              3       text    (?m)^\s*\.version\s*=\s*"..."   (n/a)
  *.json (fallback)          1       json    $.version                       $.name
  *.yaml (fallback)          1       yaml    $.version                       $.name
  ...                                                                                       (full list, not truncated in actual output)

  Confidence: 3 = path-pinned exact / 2 = basename exact / 1 = glob fallback

User-defined rules (--define-rule): not in the table above? Define your own rule.
  Rules you define on the CLI always override builtin rules (the two are separate
  axes: builtin uses Confidence, --define-rule uses PATTERN match strength).
  See `bump-semver --help-full` § User-defined rules for syntax, match-strength
  table, and examples.
```

**`bump-semver --help-full` の "User-defined rules" セクション**:

```
User-defined rules (--define-rule)
  ────────────────────────────────

Syntax:
  bump-semver <verb> <INPUT...> [--format F] [--version-path P] [--version-regex R] \
              [--name-path P] [--name-regex R] \
              [--define-rule <PATTERN> ...]

PATTERN match strength (higher = more specific, --define-rule wins over builtin):
  5  absolute path      /home/user/proj/package.json
  3  relative path      othersystem/package.json  (./X も正規化後 5/3 のいずれか)
  2  basename           package.json (= "ts/package.json" 等に basename 一致)
  1  glob               glob:*.myapp (= meta 有無を問わず常に 1)
  —  (no --define-rule match) → falls through to builtin (= Confidence 3/2/1 で評価)

Format options:
  --format text          全文 / regex で抽出 (--version-regex 必須、exact one match)
  --format json|yaml|toml|xml
                         構造化 file (--version-path で抽出、--version-regex も併用可)
                         xml は child element / attribute 両方を解決 (同値なら採用、
                         異値は ambiguous error、textContent は trim)

Scope rules:
  最初の --define-rule より前: global (= 全 SOURCE のデフォルト)
  --define-rule <PATTERN> の後: block (= PATTERN にマッチした SOURCE のみ)
  positional SOURCE は block を跨いで透過 (= block 終了は次の --define-rule のみ)

  Block と global は独立した "rule 完全宣言" 単位。block で 1 つでも rule 系 flag を
  書いたら、その block はそれだけで完結 (= global / builtin から継承しない)。

Examples:
  # 単一ファイル (global のみ)
  bump-semver get my.txt --format text --version-regex 'v(\d+\.\d+\.\d+)'

  # 複数ファイル別 rule
  bump-semver get a.txt b.json \
    --define-rule a.txt --format text --version-regex '...' \
    --define-rule b.json --format json --version-path '$.version'

  # glob 一括 (一部のみ override)
  bump-semver get *.myapp special.myapp --format json --version-path '$.app.version' \
    --define-rule special.myapp --format text --version-regex '...'
```

**各 verb (`get` / `compare` / `patch` / `minor` / `major` / `pre`) の `--help` summary**
(= 目的ベース、1 行表記):

```
Options:
  ...
  --define-rule <PATTERN>    Define how to extract version from <PATTERN> files
                             (use when your file is not in the builtin table; see --help-full)
  --format <FMT>             How to parse the file: text (regex) | json | yaml | toml | xml
  --version-path <DOTPATH>   For json/yaml/toml/xml: where the version field is (e.g. $.version)
  --version-regex <PATTERN>  For text format: regex with one capture group for the version
                             (exact one match required, 0/2+ matches = error)
  --name-path <DOTPATH>      Optional: where the package name field is (json/yaml/toml)
  --name-regex <PATTERN>     Optional: regex with one capture group for the package name
```

**`bump-semver --help` (short) への入口誘導 1 行追加**: builtin 表へのポインタの直下に

```
Not in the table? Define your own rule with --define-rule (see --help-full).
```

を追加。これで初心者が short help を読み終わって「対応してないなら諦め」と判断する前に
入口を提示する。

**vcs サブコマンド配下では rule 系 flag を help に出さない** (= version 抽出をしない、
意味なし)。

### Help の 2 層構成

- builtin 側: rule table として見せる (= path matcher + Confidence + Format + Version
  source + Name source の 1 行表記)。format は table の 1 列で見せるが、ユーザが直接
  操作する API ではない
- user-defined 側: `--format` enum を "Rule fields" セクションで明示的に公開。ここでは
  format がユーザ API なので、`text|json|yaml|toml` の選択肢と意味を 1 等地で見せる

これにより「内部分類を隠したい」と「format を見せたい」の表面的矛盾は **見せる場所を
分ける** ことで解消。

### Implementation note

- 位置依存パーサ規約のコード内コメント:
  `Design rationale: --define-rule blocks deliberately make --format/--version-*/--name-*
  order-dependent (scope-sensitive). This is the only flag family in bump-semver with
  positional flag semantics. See DR-0029.`
- `document-design-rationale.md` ルール (= 規約から外れる設計は意図をコメントに明記) に
  従う

## Consequences

### Positive

- builtin 未対応のファイル / 既存 builtin と被るが path 異なる project / `vcs:` 借用形式
  含む一貫制御、の 3 ケースをすべて 1 invocation で書ける
- core 哲学 (= 「最小限で済む」「basename 自動判定」) は CLI rule 不使用時は完全維持
- ユーザ明示指定の責任を CLI rule に閉じ込め (= hard error + exact-one match)、builtin
  の挙動には影響しない
- builtin の name safety rail を user-defined でも維持 (= warning hint で silent
  downgrade 防止)

### Negative

- 位置依存パーサ (= `--define-rule` の position が scope semantics に効く) が kawaz CLI
  規約「オプション位置固定を避ける」と背反 (= 意図的例外、Design rationale で明記)
- match strength scoring + Confidence の 2 軸を help / docs で説明する cognitive load
- bump --write 時の書き戻しアルゴリズム pin (= path + regex 併用の挙動が概念複雑)

### Neutral

- 引数指定 (CLI flag) のみで完結する。設定ファイルは導入しない (= 後述「不採用」)。

## 不採用 (= 設計方針として持たない)

- **config file** (= `.bump-semver.toml` 等)。導入すると (1) user グローバル設定 ×
  project 設定 × ディレクトリ階層の多段マージ戦略が必要になり、(2) 環境依存の暗黙
  デフォルトが生まれる。デフォルトと違う設定ファイルが置かれた環境で同一コマンドが
  別動作になるのはポータビリティを損なう。**rule 指定は引数 (`--define-rule` 等) のみ**
  とし、実行コマンドにすべての挙動が現れる状態を維持する。
- **builtin 無効化オプション** (= `--no-builtin-rules` / `--disable-builtin <PATTERN>`)。
  CLI rule の override で事実上の防御が効くため不要。後発 builtin との衝突が実際に起き、
  上書きでは不十分な事例が観測されてから別途検討。
- **dead-global warning** (= 全 SOURCE が named block でカバーされ global flag が
  dead code になるケースの警告)。書き間違いと「override されない SOURCE を想定して
  書いた」が区別できず、警告がノイズになるため出さない。

## Out of scope (= 将来、需要が出たら別途)

- **dot-path 高度化** (= quoted key `"key with dot"` 等)
- **error UX 文面規約** (= `--format text` で `--version-regex` が抜けた / dot-path 解決
  失敗 / regex がマッチしない 等の error message に「直し方の例」を含める規約)
- **各 verb の `--help-full` 実装** (= 現在 root のみ。補足が要る verb が出たら個別追加、
  シンプルな verb は root 集約で足りる)
- **`bump-semver vcs <subverb> --help` 階層整備** (= 現状 `vcs tag --help` 等が unknown
  action error)
- **Builtin 表の完全列挙保証** (= root `--help-full` の Builtin rules 表は省略なしで
  全件列挙)
- **dry-run preview** (= bump --write の安全弁強化)

## Related DR / Issue

- DR-0001: basename 自動判定の哲学 (= 本 DR で「外す」決断ではなく「補完する」設計)
- DR-0005: path-aware confidence ranked candidates (= 本 DR の strength axis と直交する
  別 axis、help で並列表示)
- DR-0010: confidence-1 fallback hint (= 本 DR で挙動変更なし)
- DR-0012: builtin `regex` format (= DR-0030 で format=text に統合済、本 DR は統合後の
  enum 5 値 を前提)
- DR-0017: compare precision suffix
- DR-0023: N-arg + `vcs:` borrow (= 本 DR の borrow 形式 / 単独形式の rule 解決規約と
  接続)
- DR-0024: `glob:` prefix matcher (= 本 DR の strength 1 は `doublestar.Match` ベース)
- DR-0027: `regex:` 却下 (= matching syntax の話、本 DR で PATTERN prefix として `regex:`
  を採用しないことと整合)
- DR-0028: glob spec v0.1.0 (= 本 DR の strength 1 では substitute / capture 用拡張は
  使わない)
- DR-0030: format=regex 概念廃止 → format=text + version-regex 統合 (= 本 DR の `--format`
  enum の前提)
- format-request memo (`docs/issue/2026-06-03-format-request-window.md`): builtin 追加
  要望窓口の整備 (= 別軸、本 DR と並行で Phase 1 実装着手時に処理)
