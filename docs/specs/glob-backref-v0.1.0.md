# glob-backref spec v0.1.0 (draft)

> **Status**: draft、kawaz レビュー前
> **Date**: 2026-06-02
> **Source of truth**: 本 doc は **言語非依存仕様** の正本。将来独立 OSS リポ (= `kawaz/glob-backref-spec` 等) へ extract 想定。bump-semver 内 `vcs outdated` (DR-0027 → DR-0028) を含む各実装は本 spec に準拠する。

## 0. Overview

「**後方参照 (backref) 可能な glob**」を、ファイルシステム上のパス pattern matching DSL として定義する。`{}` 直積展開 + `*`/`**`/`[]` capture + `${N}` substitute + path normalization を統合し、source → derived の path 対応関係を 1 行で表現可能にする。

主用途:
- 派生 sync check (例: bump-semver `vcs outdated`、translation lag check、bundle freshness、generated code 鮮度)
- batch rename / file 整列 (例: zsh `zmv` 系の置換)
- build tool / template engine の input → output mapping (例: snakemake 風 DAG)

prior art の位置づけ:
- **位置 backref 系譜** (= 採用): MS-DOS COMMAND.COM `copy *.txt *.bak`、Unix `mmv '*.txt' '#1.bak'` — 「可変パーツが順に backref レジスタに足される」
- **明示 capture 系譜** (= 不採用): zsh `zmv '(*).txt' '$1.bak'` — `()` 必須、glob の filename 表現性犠牲

本 spec は **位置 backref 系譜** を `{}` 直積展開 + path normalization + grammar drift 検出と統合した形で、prior art を **超える**新パターン。

## 1. Rationale

### 1.1 Why glob (not regex)

regex 拡張で fs マッチを表現する方向 (= 「regex に path-aware token を追加 + fs walk wrap」) も検討した上で却下:

- **regex engine の言語別差**: PCRE / RE2 / Oniguruma / ECMAScript / .NET 等で挙動 drift、多言語実装で仕様準拠困難
- **`{}` 必須 vs `*`/`**`/`[]` 任意の自然な二分が regex で表現困難**: regex の `(?:a|b)` は OR、glob `{a,b}` の「全展開必須」semantics と意味違う
- **`**` 0-segment + path-aware semantic が regex の string semantic と矛盾**: 「path separator を跨いで 0 以上の segment」は glob 固有概念、regex の `.*` では fs walk 抜きで意味化できない
- **glob 文化との非整合**: shell / make / build tool / file matching の主戦場が glob ベースで、regex は text processing 文化、ユーザ母集団が違う
- **filename 表現性**: regex は `()` 必須で `(foo)/(bar)-ja.md` のような `()` 含むファイル名を扱えない (= `()` を literal にするには escape 仕様必要、本 spec では escape 仕様無し方針継承)

→ glob を base に backref を追加する方向で確定。

### 1.2 Why backref in glob

既存 glob は「pattern match → matched paths」の出力のみで、matched path の **どの部分が pattern のどの可変パーツに対応したか** の情報を expose しない (= doublestar v4、go-zglob、stdlib filepath.Match、PowerShell `Get-ChildItem` 等共通)。

しかし派生 path 構築 (例: `src/foo.ts` → `lib/foo.js`)、batch rename (例: `*.txt` → `*.bak`)、template input → output mapping 等の用途で **「マッチした可変部分を別の文字列に埋め込みたい」** は頻出。

prior art (MS-DOS `copy *.txt *.bak` 系) は位置 backref を実現していたが、`{}` 直積 / `path.Clean` / grammar drift 検出 / 多言語仕様 等は未統合。本 spec で初めて統合する。

## 2. Grammar (= AST tokens)

glob pattern は 4 種の可変パーツ + literal で構成:

| Token | Notation | Match semantics | Backref capture |
|---|---|---|---|
| `*` | `*` | 0 文字以上、**path separator 跨がない** (= 1 segment 内のみ) | yes |
| `**` | `**` | 0 segment 以上、**path separator 跨ぐ** (= recursive)。**独立した path component として現れた時のみ recursive**、`foo**` / `**bar` / `**foo**` は `*` 相当 | yes |
| `{a,b,c}` | `{a,b,c}` | 全選択肢 (= a, b, c) で **直積展開** (= 各選択肢毎に concrete pair に分解)。Cartesian (= 複数 `{}` で N×M) | yes (= 選ばれた選択肢の literal 値) |
| `[abc0-9]` | `[abc0-9]` | 文字クラス、**1 文字 match** (= range / set) | yes |
| literal | (それ以外) | 文字通り | no (= capture 対象外) |

### 2.1 Out of scope (= MVP 不採用)

- **`?` 単一文字**: doublestar 等の glob には存在するが、本 spec では capture / non-capture 双方で導入しない (= 紛れを避ける、需要発生時に v0.2 で検討)
- **`{}` ネスト**: 1 階層のみ (= `{a,{b,c}}` は invalid)。direct 直積展開難 + 番号付け曖昧
- **`[]` 文字クラスの complement (= `[^abc]`)**: POSIX 規約だが MVP では invalid
- **`~` home 展開**: FROM 側 base path 解決は `glob:` prefix layer (= DR-0024) の責務、本 spec は pattern AST のみ
- **escape 仕様**: backslash / quote エスケープなし (DR-0024 §10.7 継承)。特殊文字 (`*`/`**`/`{}`/`[]`/`?`/`~`/`,`) を含むファイル名は本 spec の対象外

### 2.2 Wildcards 仕様詳細

#### 2.2.1 `**` の path-aware 解釈

`**` が「独立した path component」として現れた時のみ recursive:

```
**          → 全 file 再帰 (= find . と同等)
**/foo      → 任意深度の foo
foo/**      → foo/ 配下の任意深度
foo/**/bar  → foo と bar の間の任意深度 dir
**foo       → *foo 相当 (= 1 segment 内、recursive ではない)
foo**       → foo* 相当 (= 1 segment 内、recursive ではない)
**foo**     → *foo* 相当 (= 1 segment 内、recursive ではない)
```

「独立」の定義: `**` の直前 / 直後が path separator (`/`) または string boundary (= 開始 / 終了)。
これは bash globstar、doublestar v4、ninja、PowerShell 等の業界標準。

#### 2.2.2 `**` 0-segment match

`**` は 0 segment match を許す (= path separator 0 個):

```
pattern: **/foo
matches:
  foo         ← **=0 segment
  a/foo       ← **=1 segment (= "a")
  a/b/foo     ← **=2 segments (= "a/b")
```

0 segment match 時の `${N}` 値は **`.` (= current dir)**:

```
pattern: **/foo
TO:      ${1}/foo
match: foo       → $1="." → "./foo" → path.Clean → "foo"
match: a/foo     → $1="a"  → "a/foo"
match: a/b/foo   → $1="a/b" → "a/b/foo"
```

これにより leading-slash bug (= `${1}/foo` で `$1=""` の時 `/foo` 絶対 path 化) を回避。

#### 2.2.3 `*`/`[]` の zero match は grammar drift

`*` は通常 1 文字以上 match (= 0 文字 match を許す変種もあるが本 spec では 0 文字 match を grammar drift と判定)、`[]` 文字クラスは必ず 1 文字 match。

実装で `*`/`[]` の captured 値が空文字になった場合は **panic** (= grammar drift assertion):

```
internal: %s wildcard matched empty string at pattern %q, path %q — grammar drift
```

silent skip / silent fresh 判定は厳格に禁止 (= release gate での silent failure 防止)。

#### 2.2.4 `{}` 空選択肢

`{,a,b}` のような空選択肢 (= 空文字を選ぶ分岐) は **user 意図として許容**:

```
pattern: README{,-ja}.md
matches:
  README.md      ← {} 分岐 = ""
  README-ja.md   ← {} 分岐 = "-ja"
```

空選択肢由来の `${N}=""` は grammar drift ではない (= user 意図)、panic しない。

## 3. Matching semantics

### 3.1 Step 1: `{}` 直積展開

FROM および TO 両方の `{}` を **直積展開**。1 logical pair → N concrete pairs:

```
input:
  FROM: 'glob:**/*.{jpg,webp}'
  TO:   '$1/$2.$3.sha256'

output (= concrete pairs):
  FROM: 'glob:**/*.{jpg}'    TO: '$1/$2.$3.sha256'  (= {} 分岐 = "jpg")
  FROM: 'glob:**/*.{webp}'   TO: '$1/$2.$3.sha256'  (= {} 分岐 = "webp")
```

複数 `{}` で Cartesian:

```
input:
  FROM: 'glob:**/*.{ts,tsx}'
  TO:   'lib/$1/$2.{js,mjs}'

output:
  FROM: 'glob:**/*.{ts}'     TO: 'lib/$1/$2.{js}'
  FROM: 'glob:**/*.{ts}'     TO: 'lib/$1/$2.{mjs}'
  FROM: 'glob:**/*.{tsx}'    TO: 'lib/$1/$2.{js}'
  FROM: 'glob:**/*.{tsx}'    TO: 'lib/$1/$2.{mjs}'
```

注: 複数 `{}` の数が増えると直積爆発。実用上数個まで想定、性能上の上限なし (= 仕様の単純化を優先、需要発生時に limit / streaming を検討)。

### 3.2 Step 2: concrete pair の wildcard を capture regex 化

各 concrete pair の FROM pattern (= `{}` 展開後) で `*`/`**`/`[]` を capture group 化、literal は `regex.QuoteMeta` で escape:

```
FROM pattern: '**/*.{jpg}'         (= concrete after step 1)
              → '**/*.jpg'         (= {} 展開で literal 化)
              → '(.*)/([^/]*)\.jpg' (= regex 化、step 2 output)

* → ([^/]*)         (= 1 segment 内、0 文字以上)
** → (.*)           (= path separator 跨ぐ、0 segment 許す)
[abc] → ([abc])     (= 1 文字)
literal → regex.QuoteMeta(literal)
```

### 3.3 Step 3: fs walk + match

fs walk は glob ライブラリ (= doublestar 等) に委譲し、matched path 集合を取得。各 matched path に対し step 2 の capture regex で match + capture 抽出:

```
matched path: "src/foo.jpg"
capture regex: '(.*)/([^/]*)\.jpg'
captures: ["src", "foo"]  (= [$1, $2])
```

**Grammar drift detection**: doublestar が match した path に対し、step 2 の capture regex が match しなかった場合は panic (= grammar drift):

```
internal: doublestar matched %q but capture-regex %q did not — grammar drift
```

silent skip 禁止。

### 3.4 Step 4: TO 側 literal substitute

TO テンプレを **literal segment と `${N}` placeholder の tokenize**、`${N}` 置換は **value をリテラル埋め込み** (= value 内 `*`/`{}`/`[]` は再 glob 解釈しない):

```
TO template: 'lib/$1/$2.js'
captures: ["src", "foo"]
substituted: 'lib/src/foo.js'  (= literal embed)

TO template: 'lib/$1/$2.js'
captures: ["src", "a{b,c}"]   (= captured value に {} を含むケース、e.g. 病的 filename)
substituted: 'lib/src/a{b,c}.js'  (= literal embed、{} は分岐展開されない)
```

これにより captured 値内の glob 特殊文字が誤って分岐展開される silent path drift を防止。

### 3.4.1 TO 側 `glob:` 解釈時の value escape (kawaz 確定 2026-06-02)

TO テンプレに `glob:` prefix が指定された場合 (= 派生先も glob で多数マッチさせる用途、例 `"glob:$1/**/$2-*.md{,.backup[0-9]}"`)、substitute 結果は **2 段目の glob 解釈** にかけられる。`$N` value は FROM glob 結果なので glob meta (`*`/`**`/`{}`/`[]`) を含み得る (= 病的 filename の場合)。

**value 部分は glob library の quote ルールでクオートして literal 化、glob 展開させない。template 外側の glob keyword は escape せず生かす。**

例:
```
template:   'glob:$1/**/$2-*.md{,.backup[0-9]}'
captures:   ['foo*bar', 'docs']  (= $1='foo*bar', $2='docs')

substitute (= value を quote):
  $1 → [f][o][o][*][b][a][r]   (= 1 文字ずつ文字クラス wrap で literal 化)
  $2 → [d][o][c][s]

result:     'glob:[f][o][o][*][b][a][r]/**/[d][o][c][s]-*.md{,.backup[0-9]}'
```

template 外側の `**`、`*.md`、`{,.backup[0-9]}` は **glob として生かす**。value embed 部分のみ literal 化。

### 3.4.2 escape 戦略 (= 言語非依存ルール)

portable subset (= DR-0024 §10.7) では backslash escape (`\*`) は OS 依存 (= Windows path separator 衝突) で不採用。**文字クラス wrap (= 1 文字を `[c]` で囲んで literal 化)** が portable な escape 手段:

| Wildcard char | Quote 後 | 効果 |
|---|---|---|
| `*` | `[*]` | 1 文字 `*` の literal |
| `?` | `[?]` | 1 文字 `?` の literal |
| `{` | `[{]` | 1 文字 `{` の literal |
| `}` | `[}]` | 1 文字 `}` の literal |
| `[` | `[[]` | 1 文字 `[` の literal |
| `]` | `[]]` | 1 文字 `]` の literal |
| `,` | `[,]` | 1 文字 `,` の literal (= `{}` 内分岐区切りの誤解釈防止) |
| その他 | そのまま | literal char、quote 不要 |

各言語実装は **library 提供の escape API** (例 Python `glob.escape`、zsh `(q)`) があれば使う、なければ上記文字クラス wrap で自前実装。

### 3.4.3 value 内に **改行 / null byte** が含まれた場合

POSIX path として valid な範囲 (= `/` `\0` 以外の任意 byte) を spec では受け入れる、ただし `\0` は path 文字列の終端なので glob library が拒否する可能性、実装委ね (= 実装で reject or escape 試行)。

注: TO 側 `glob:` 採用は v0.1.0 必須 (= 派生先も glob 用途、kawaz 例 `**/<base>.{md,backup}` 等)。escape は仕様で固定、各実装はそれに準拠。

### 3.5 Step 5: path normalization

substituted result に `path.Clean` (= POSIX path normalization) を適用:

```
"./foo"        → "foo"
"lib/./foo"    → "lib/foo"
"lib//foo"     → "lib/foo"
"foo/../bar"   → "bar"
"/foo"         → "/foo"  (= absolute path として保持、user 意図)
```

これにより `**` 0-segment match 由来の `./` / `//` を吸収。

## 4. Backref numbering

### 4.1 Rule

可変パーツに **出現順** で番号付与。`{}` も番号消費:

```
pattern: 'glob:{a,{b,{c*},*/sub,**/*.ts,n[0-9].txt}'

backref 番号 (= 大外から出現順):
  $0 = matched path 全体 (= ga、暗黙)
  $1 = {a,{b,{c*},*/sub,**/*.ts,n[0-9].txt}  (= 大外 {})
  $2 = {b,{c*}}                              (= ネスト 1) ← ただし MVP 不可、§2.1 参照
  $3 = {c*}                                  (= ネスト 2) ← 同上 MVP 不可
  $4 = *                                     (= $3 内、MVP 不可)
  $5 = *                                     (= */sub の *)
  $6 = **                                    (= **/*.ts の **)
  $7 = *                                     (= **/*.ts の *)
  $8 = [0-9]                                 (= n[0-9].txt の文字クラス)
```

(MVP では `{}` ネスト不可 (= §2.1) のため上記 $2 / $3 / $4 は実装対象外、将来拡張で対応)

### 4.2 マッチしなかった分岐の backref

`{}` 直積展開で 1 分岐を選んだ時、**選ばれなかった分岐内**の backref は **空文字、エラーにしない**:

```
pattern: 'glob:{a*,b*}'  (= 2 分岐)
TO:      'derived-$1-$2-$3.txt'

match: "a-foo" → 分岐 "a*" を選択
  $1 = "a*"     (= {} 大外、選ばれた分岐の literal 値)
  $2 = "-foo"   (= a* の * = "-foo")
  $3 = ""       (= 選ばれなかった b* の * は空文字)
result: "derived-a*--foo--.txt"
```

注: backref が空文字になった時 user が違和感を持つ可能性 (= `derived--foo--.txt` のような結果)、help / Examples で「`{a,b}` 分岐は番号付け規則上 2 つの wildcard を別番号として消費する」と明示要。

### 4.3 `${N}` placeholder 形式

| Form | Behavior |
|---|---|
| `$1` 〜 `$9` | 1 桁 backref 参照 |
| `${1}` 〜 `${9}` | 同上、明示形式 (= 連続 literal との衝突回避、例 `${1}0` = `$1` + literal "0") |
| `${10}` 以降 | 2 桁以上 backref 参照、`${}` 必須 |
| `$10` | **ambiguous error** (= `${1}0` か `${10}` か曖昧、reject) |
| `$0` | matched path 全体 (= 暗黙、特殊 backref) |
| `${0}` | 同上 |
| `$N` (N が範囲外) | 空文字 (= ペアの可変パーツ総数を超えた N、エラーにしない) |
| `${N}` (N が範囲外) | 同上 |

### 4.4 `${name}` named capture (= v0.1.0 scope-out、v0.2 候補)

将来拡張で named capture 構文 `${name:pattern}` (FROM 側で定義) + `${name}` (TO 側で参照) を導入想定。MVP では:

- `${...}` の中身が **数字 1-9 のみ accept**、他は usage error
- `${name:pattern}` 形式の FROM pattern は invalid
- `${name}` 参照は invalid (= 数字以外の identifier reject)
- 将来 v0.2 で `${...}` を「数字なら位置参照、非数字なら named 定義/参照」の両用に拡張可能な設計余地を MVP 段階から保持

## 5. Pair separator (= `--`)

複数 (FROM, TO) ペアを 1 invocation で集約する場合、`--` で区切る:

```
1 ペア (= `--` 省略可):
  vcs outdated FROM TO

複数ペア (= `--` 必須):
  vcs outdated -- F1 T1 -- F2 T2 -- F3 T3
```

各ペア独立 (= ペア間で `${N}` 名前空間分離)。

注: `--` は POSIX CLI 慣習で end-of-options の意味があり、shell によっては解釈衝突。本 spec consume 側 (= 実装) は verb-gated parsing で `--` を pair separator として再定義要、help で明示要。

## 6. Auto-exclusion (= 自動除外)

FROM がマッチした path 自身が TO の glob 展開結果に含まれた場合、その派生集合から除外:

```
FROM: 'glob:**/*.md'
TO:   'glob:**/*.txt'  (= 別 fs check 例)

match FROM: README.md, docs/foo.md
match TO:   README.txt, docs/foo.txt
→ FROM 自身 (= README.md, docs/foo.md) が TO の match 結果に含まれない、自動除外なし

別ケース:
FROM: 'glob:README{,-ja}.md'  (= README.md, README-ja.md にマッチ)
TO:   'README$1.md'           (= ${1} = 分岐 literal)

match FROM: README.md     → TO substituted: README.md     (= 自分自身、除外要)
match FROM: README-ja.md  → TO substituted: README-ja.md  (= 自分自身、除外要)
```

自動除外の単位:
- **per-source**: 各 matched FROM path について、それを起点に展開した TO path のうち FROM path 自身と一致するもの (= sourcePath == derivedPath) を除外
- **cross-source**: 別 source FROM がマッチした path と derived path が一致する場合の扱い → **本 spec v0.1.0 では未定義**、需要発生時に v0.2 で追加

## 7. Error model

| Error class | Trigger | Behavior |
|---|---|---|
| Usage error | argv parse、`--` 不一致、`$10` ambiguous、`${name}` 等 v0.2 syntax | exit 2、stderr に usage hint |
| Pattern error | invalid `{}` ネスト、`[]` 不正 range、escape syntax 等 | exit 2、stderr に pattern エラー位置 |
| Grammar drift | doublestar match × capture regex no-match、`*`/`[]` 空文字 match | **panic** (= internal bug、release gate での silent failure 防止) |
| VCS / fs error | fs walk 失敗、permission denied、subprocess error | exit 3、stderr に原因 |
| Predicate false | 派生鮮度 lag (= derived ts < source ts)、必須派生不在 | exit 1 (= consumer 側 verb の semantic、例: bump-semver `vcs outdated`) |
| Fresh | 全派生鮮度 OK | exit 0 |

## 8. Shell escape

`${N}` / `{}` / `--` は shell の特殊文字と衝突するため、**single quote 必須**を help / Examples / error で明示:

```
✓ vcs outdated 'glob:src/**/*.ts' 'lib/$1/$2.js'
✗ vcs outdated glob:src/**/*.ts lib/$1/$2.js     (= shell が $1 を展開、bash で空文字)
✗ vcs outdated "glob:src/**/*.ts" "lib/$1/$2.js" (= 同上、double quote でも $ 展開)
```

エスケープ仕様は本 spec で **導入しない** (DR-0024 §10.7 継承)。user が single quote で囲む責任。

## 9. File naming constraints

本 spec が扱える pathname に以下の **literal 文字を含まない** 前提 (= グローバル制約、§2.1 と整合):

- `*` / `**` (= wildcard)
- `{` / `}` / `,` (= 分岐)
- `[` / `]` (= 文字クラス)
- `?` (= 将来予約)
- `~` (= home 展開、`glob:` prefix layer の責務だが本 spec も非対応扱い)

特殊文字を含むファイル名 (= 病的 filename) は本 spec の対象外。実装は capture 値に literal として保持するが、TO 側 substitute / `glob:` prefix 再解釈で誤動作する可能性 → 「対象外」ドメインとして user 責任。

## 10. Considered alternatives (= 不採用案、却下理由付き)

### 10.1 `()` capture (= zsh `zmv` 系譜)

FROM 内 `()` で明示 capture、TO 側 `$N` で参照。

却下理由:
- glob 本来の filename-representability を犠牲: `(foo)/(bar)-ja.md` のような括弧含む path 表現不可
- escape 仕様 (`\(`, `\)`) の導入を強制 → DR-0024 §10.7「クオート / エスケポ仕様は導入しない」と矛盾
- 既存 glob ユーザの認知負荷 (= shell の `()` = subshell、regex の `()` = group との混同)

### 10.2 regex + fs walk (= regex に path-aware token と fs walk を後付け)

regex engine をベースに、`**` / `*` を path-aware special token として追加、fs walk wrap で fs match を実現。

却下理由 (= kawaz 2026-06-02 脳内検討で「厳しそう」判定):
- regex engine の言語別差 (= PCRE / RE2 / Oniguruma / ECMAScript / .NET) で多言語実装挙動 drift
- `{}` 必須 vs `*` 任意の二分が regex 上で表現困難 (= regex `(?:a|b)` は OR、glob `{a,b}` の全展開必須と意味違う)
- `**` 0-segment + path-aware semantic が regex の string semantic と矛盾
- glob 文化 (= shell / make / build tool) と regex 文化 (= sed / awk / log scraping) で母集団違う
- AST 複雑度 (= regex 全機能 + fs walk + path-aware token) が glob + backref より大きい

### 10.3 named capture (`${name:pattern}` 構文) の MVP 採用

FROM 側で `${name:pattern}` で命名、TO 側で `${name}` 参照 (例: `glob:{dir:**}/{base:*}.ts` → `${dir}/${base}.js`)。

却下理由 (= v0.1.0 では):
- 位置 backref `$N` が実用上 2-3 個までで体感破綻するのは 4 個超、MVP scope では `$N` 一本で十分 (= kawaz 確定 2026-06-02)
- `${...}` 構文を「数字なら位置参照、非数字なら named」の両用にする設計余地は v0.1.0 から保持
- 需要発生時 (= 複雑パターンで `$N` 番号管理が破綻) に v0.2 で導入

将来 v0.2 で `${name:pattern}` 構文採用余地あり (= kawaz 確定方向)。

### 10.4 GENERATOR_CMD (`cmd:...`) / lambda script (`lambda:js,...`) / explicit pair YAML

これらは「(a) 位置 backref glob の対抗ではなく **別経路**」(= kawaz DR-0027 議論)。本 spec は (a) のみ定義、別経路は別 spec / 別 DR で扱う。

## 11. Reference implementations

| Language | Status | Reference |
|---|---|---|
| Go | implementing (= bump-semver `vcs outdated` MVP rewrite、DR-0028 で land 予定) | `github.com/kawaz/bump-semver/src` |
| TypeScript | planned | TBD |
| Rust | planned | TBD |
| MoonBit | planned | TBD |

将来独立 OSS リポ (= `kawaz/glob-backref-{go,ts,rust,mbt}` 等) で各言語 implementation を公開想定。

## 12. Test vectors

(言語非依存の test vector、JSON で配布想定。各実装は本 vectors を pass すること。MVP では Go 実装の `glob_backref_test.go` を vector source として、json export 仕組みは将来追加。)

### 12.1 Vector 構造案

```json
[
  {
    "name": "T1-bundle",
    "pattern": "glob:src/**/*.ts",
    "fixtures": ["src/foo.ts", "src/a/b/bar.ts"],
    "expected_captures": [
      {"path": "src/foo.ts", "captures": ["src", "", "foo"]},
      {"path": "src/a/b/bar.ts", "captures": ["src", "a/b", "bar"]}
    ]
  },
  {
    "name": "T1-bundle-TO-substitute",
    "captures": ["src", "a/b", "bar"],
    "to_template": "lib/${1}/${2}.js",
    "expected": "lib/src/a/b/bar.js"
  },
  {
    "name": "leading-slash-zero-segment",
    "pattern": "glob:**/*-ja.md",
    "match": "README-ja.md",
    "captures": [".", "README"],  // = ** 0-segment は "."
    "to_template": "${1}/${2}.md",
    "expected": "README.md"  // = path.Clean で "./README.md" → "README.md"
  }
]
```

(詳細 vectors は実装と並行で追加)

## 13. Versioning

本 spec は **semver** に従う:

- v0.X.Y: pre-1.0、breaking change 可
- v1.0.0 リリース後は破壊的変更を慎重に判断 (= 各言語実装の追従コスト)

v0.1.0 で MVP scope (= 上記 §2 grammar、`$N` 一本、`{}` ネスト不可、特殊文字含む filename 対象外) を確定。v0.2 で named capture (= `${name:pattern}`)、v0.3 以降で `{}` ネスト / `?` 単一文字 / `cmd:` GENERATOR scheme 等を検討。

## 14. Open questions (= kawaz レビュー対象)

1. **§4.2 「未マッチ分岐の backref は空文字」の semantics** が user 直感と一致するか? help / Examples での明示で十分か?
2. **§6 cross-source 自動除外**を v0.1.0 で扱うか v0.2 送りか
3. **§9 病的 filename** (= 特殊文字含む) を本 spec から完全 scope-out か、`--strict` 等のオプションで対応するか
4. **§12 test vectors** の JSON format を spec の一部として確定するか、各実装の test fixture format に委ねるか
5. **本 spec の置き場所** = bump-semver 内 `docs/specs/` 当面保持、独立リポ (= `kawaz/glob-backref-spec`) extract のタイミング判断
6. **§3.1 直積爆発**の上限規定 (= `{}` × `{}` × ... の組み合わせ数) を入れるか、性能面は実装責任とするか

## 15. API surface guideline (= 言語非依存の推奨 API、各言語 idiom 採用可)

仕様準拠の各言語実装が提供すべき機能を「**必須**」「**推奨**」「**optional**」に分類。具体的な signature は言語 idiom (= Go `iter.Seq` / TS `AsyncIterable` / Rust `Iterator` / MoonBit ?) で実装委ね。

### 15.1 Core (必須)

**Match API** (= FROM pattern を fs にマッチ、capture 付きで返す):

```
matchCollect(pattern: string, root: string, opts?: Options)
  → Array<Match>
```

```
Match {
  path: string         // matched fs path (root 相対)
  captures: string[]   // [$0, $1, $2, ...] (= $0 は path 全体)
  // optional: kindByCapture: ('*' | '**' | '{}' | '[]')[]  ← rich variant の場合
}
```

**Substitute API** (= TO template + captures から派生 path 生成):

```
substitute(template: string, captures: string[]) → string
```

**Pair expansion API** (= `{}` 直積展開):

```
expandPairs(from: string, to: string) → Array<{fromConcrete: string, toConcrete: string}>
```

### 15.2 Stream variant (推奨、大規模 fs / cancellation 用途)

Match API の stream / iterator 形式:

```
match(pattern: string, root: string, opts?: Options)
  → Iterator<Match>  // 言語 idiom: Go iter.Seq, TS AsyncIterable, Rust Iterator
```

- **lazy evaluation**: walk と match を 1 pass、yield しながら順次出力
- **cancellation**: iterator close で walk 中断 (= Go context、TS AbortSignal、Rust Drop)
- 大規模 tree / time-budget が効く用途で必要、MVP では optional

### 15.3 Options (各実装で共通推奨)

```
Options {
  // Walk control
  walkOrder?: "depth" | "breadth"   // default "depth" (= fs 慣習)
  walkConcurrency?: number          // default 1 (= sequential、deterministic 順序)
                                    // > 1 で parallel walk worker、I/O overlap
                                    // upper bound は実装で clamp (= OS fd limit / CPU 周辺)
                                    // > 1 時の結果順序は unspecified (= 完了順 or 実装依存)
  maxResults?: number               // default unlimited、安全弁
  maxDepth?: number                 // default unlimited、recursive ** 上限
  followSymlinks?: boolean          // default false (= symlink loop 防止)

  // Filter (DR-0024 系継承)
  dotfiles?: boolean                // default false
  gitignored?: boolean              // default true (= respect)
  ignorecase?: boolean              // default false

  // Error handling
  onError?: "raise" | "collect" | "skip"
                                    // default "raise"
                                    // (= grammar drift は常に panic 不可避、それ以外の walk/fs error を制御)
}
```

**`walkConcurrency` 設計 note**:
- default `1` (= sequential、order deterministic、互換性最重要視)
- `> 1` で並列 walk worker spawn、I/O wait を overlap
- 実装側で upper bound clamp (= 例: Go `runtime.NumCPU() * 4`、または OS fd limit の半分等、過剰並列で fd 枯渇 / 他プロセス影響を防止)
- 並列時の **結果順序は unspecified** (= 単一順序保証を捨てる)、`maxResults` 適用順序も unspecified
- `onError=raise` × 並列 = 他 worker への cancel propagation 要 (= 言語 idiom、Go context、TS AbortSignal、Rust tokio 等)
- 小規模 fs (= file 数 < 100 程度) では spawn overhead > 効果、user 判断
- 主用途: 大規模 monorepo (= file 数 10k+)、network fs (= I/O latency 大)、CI 並列実行

### 15.4 Error model 詳細 (= §7 補足)

| Error class | When | Behavior (default) | Override |
|---|---|---|---|
| `GrammarDriftError` | doublestar match × capture regex no-match、`*`/`[]` 空文字 match | **panic / unrecoverable** | 不可 (= internal bug、release gate での silent failure 防止) |
| `PatternSyntaxError` | invalid `{}` ネスト、不正 `[]` range、`$10` ambiguous 等 | raise on API call | 不可 (= user 入力 validation、必ず raise) |
| `WalkError` | fs walk 中の permission denied、symlink loop、I/O error | `onError` で制御 (raise / collect / skip) | optional |
| `SubstituteError` | TO template の `${N}` 範囲外 (= MVP では空文字、エラーにしない) | (なし、§4.3 参照) | — |

### 15.5 必須 / 推奨 / optional 一覧

| 機能 | v0.1.0 必須 | 推奨 | optional |
|---|---|---|---|
| `matchCollect` (collect API) | ○ | | |
| `substitute` | ○ | | |
| `expandPairs` | ○ | | |
| `match` (stream variant) | | ○ | |
| `walkOrder` option | | ○ (= default "depth" 必須) | |
| `walkConcurrency` option | | | ○ (= default 1、`>1` 並列実装は optional、order unspecified) |
| `maxResults` / `maxDepth` | | ○ (= 安全弁) | |
| `followSymlinks` | | ○ (= default false 必須) | |
| `dotfiles` / `gitignored` / `ignorecase` | ○ (= DR-0024 系継承、default 値遵守) | | |
| `onError` policy | | ○ (= "raise" default 必須、collect/skip 提供推奨) | |
| `rich` Match variant (= kindByCapture) | | | ○ (= debug / introspection 用、v0.2 で正式化検討) |
| Cancellation API | | ○ (stream variant とセット) | |

### 15.6 bump-semver Go MVP の選択 (= 参照実装、v0.1.0 規模)

- `matchCollect` + `substitute` + `expandPairs` の collect API 一式
- `match` stream variant は **未提供** (= 将来 extract 時に追加)
- options: `walkOrder=depth` 固定、`walkConcurrency=1` 固定 (= 並列 walk 未採用、bump-semver は中小規模リポ想定)、`maxResults=unlimited`、`followSymlinks=false`、`dotfiles`/`gitignored`/`ignorecase` は CLI flag 経由 (= DR-0024 継承)
- error policy: `raise` 固定 (= `vcs outdated` は release gate 用途、silent skip しない)

将来 stream / cancellation / `onError=collect` 等は別言語実装 (= TS の AsyncIterable、Rust の Iterator) で活用想定。

## 16. Open questions の追加 (= §14 から拡張)

§14 既出 6 件 + 本章追加:

7. **§15.2 stream variant** を v0.1.0 必須にするか v0.2 送りか (= Go MVP は collect でも十分、TS / Rust 実装時に必須化判断?)
8. **§15.3 walkOrder** の default は depth で良いか (= breadth 採用ケースはあるか、progress 表示 / shallow-first 用途)
9. **§15.4 `WalkError` の onError policy** で `collect` / `skip` を明確に分けるか (= `collect` は errors を返り値 second tuple として、`skip` は silent)
10. **§15.5 rich Match variant** (= kindByCapture) を v0.1.0 で正式化するか v0.2 送りか (= debug / `--explain` mode で活きる情報)
11. **§15.3 `walkConcurrency`** の default は `1` で良いか (= 結果順序 deterministic 重視)、upper bound clamp の規定値 (= NumCPU 系? fd limit 半分? 実装委ね?)
12. **`walkConcurrency > 1` × `onError=raise` 時の cancel propagation** semantics を spec で固定するか、言語 idiom 委ねるか

## 17. Misc notes / scratch (= 雑多メモ、整理前の論点集積、後で必要に応じて昇格)

> v0.1.0 の整理時点でまだ精査されてないが、忘れたくない論点。後で「足りない」より「ごちゃっとでも書いておく」優先。

### Filesystem semantics

- **Case sensitivity の OS 別 default 挙動**: macOS APFS = case-insensitive、Linux ext4/btrfs = case-sensitive、Windows NTFS = case-insensitive。`ignorecase` option 未指定時の挙動は **OS native** に従うか **spec で固定** (= "default false") か未確定。多言語実装で drift しやすい
- **Unicode normalize form**: path string は valid UTF-8 前提だが、NFC / NFD どちらで match するか。macOS の HFS+ は NFD で保存、Linux は file 自身が持つ bytes、ファイル名比較で normalize 不一致だと「同名なのにマッチしない」事故発生。bytes-level vs grapheme-level マッチの選択
- **Empty pattern / non-existent root**: `pattern=""` の挙動 (= error / 全 match / no match)、`root` 存在しない時の挙動 (= error / empty result) を spec で固定
- **Trailing slash semantics**: `glob:foo/` で `foo` ディレクトリのみマッチ vs `glob:foo` で file or dir 両方マッチ、の区別

### Security

- **Path traversal**: 捕捉した `$N` 値内に `..` が含まれた時の挙動 (= TO substitute で `lib/${1}/foo` が `lib/../foo` に展開、`path.Clean` で root 外脱出可能性)
- **Symlink escape**: `followSymlinks=true` 時、symlink 経由で `root` 外に walk する場合の境界 (= root prefix check 必須 vs 実装委ね)
- **Resource exhaustion**: 巨大 fs / 巨大 pattern / 巨大 captured 値 (= `**` で path 1000 segments) で OOM 防止

### Performance

- **Walk time budget**: `maxResults` で件数 limit、`maxDepth` で深さ limit はあるが **時間軸 limit** 別。CI で「30 秒で切る」用途、cancellation API と整合
- **Pattern compile cache**: 同一 pattern を別 invocation で再コンパイルする無駄、`sync.Map` / `LRU` cache の余地 (= 実装委ね or spec で推奨?)
- **Glob walk dedup**: 複数 pair が同じ root を walk する場合の dedup (= 1 invocation 内、N pair × M files で N×M walk 避けたい)

### Implementation strategy (= 現実的落としどころ)

**kawaz 設計の核心 (2026-06-02 確認)**:
- `{}` を **要素数 1 になるまで全展開** = capture group の単位が自然に確定
- 分割時に **スコープごとの backref index を渡す** = `{}` 内の wildcard も外と同じ番号空間で出現順 increment
- `$10` 越えする現実 case は 99% で 1% 未満、$1-$5 中心の運用想定 = MVP scope はシンプルに留める正当性

**具体的 PoC 実装手順** (= 既存ツール = doublestar v4 で現実的に作れるか実験):
1. **自前 AST parser** で pattern を AST 化 (= literal / `*` / `**` / `{}` / `[]` ノード)
2. **`{}` 直積全展開** = AST 走査で `{}` ノードを 1 分岐ずつ literal 化、N concrete AST に展開
3. **各 concrete AST → capture regex 生成** (= literal は `regexp.QuoteMeta`、wildcard は capture group)
4. **fs walk は doublestar 委譲** (= concrete pattern を doublestar に渡して matched paths 取得)
5. **matched path × capture regex で match + capture 抽出**
6. **TO 側 literal substitute + `path.Clean`**

これで動けば本実装 (= DR-0028 + 既存 commit 103ece79 rewrite) へ昇格。

### 後で昇格すべき項目 (= 追跡用)

- §10.4 で `cmd:` GENERATOR / `lambda:js` を「別経路」として scope-out したが、本 spec scope に絶対含まれない (= 仕様外) ことを明示する追加章
- §15 API surface guideline は API 概念で書いたが、各言語 idiom mapping table (= Go signature / TS signature / Rust trait / MoonBit signature) は実装と並行で追加
- v0.2 候補リスト (= named capture / `{}` ネスト / `?` / `cmd:` / cross-source 自動除外 / time budget / pattern compile cache) を別 doc / changelog で管理

### Open question (拡張)

13. Case sensitivity の OS 別 default を **spec で固定** (= 例: `ignorecase=false` 固定、OS native 動作と乖離する場合は明示) vs **OS native** vs **option 必須化**
14. Unicode normalize を **spec で固定** (= NFC default) vs **OS native** vs **bytes-level (= 比較せず)**
15. Path traversal `..` を TO substitute で **無条件 reject** vs **`path.Clean` 後の root prefix check で reject** vs **user 責任**
16. Walk time budget (= `walkDeadline?: Duration`) を v0.1.0 必須 / 推奨 / optional のどれにするか
17. Pattern compile cache を spec で推奨するか実装委ねるか
18. Glob walk dedup を v0.1.0 で扱うか v0.2 か (= 同一 root の重複 walk 排除)

### Open question (実装観察由来、2026-06-02 追加 = bump-semver v0.31.0 の `vcs outdated` 実装で表面化)

> 以下は実装側 verb (= consumer) の挙動 pin で発覚した曖昧点。spec は consumer 側 verb の semantic
> を細かく規定していないが、複数言語で同じ verb を実装する時に drift しうるので明示候補。
> 詳細は `docs/testing/vcs-outdated-coverage.md` §5 を参照。

19. **`--strict` (or 同等 flag) と `--explain` (or 同等 diagnostic mode) の優先順位**: bump-semver v0.31.0 では `--explain` が勝ち、stale / lit-miss いずれの場合も exit 0。`--strict` の意図 (= silent-green CI hole 塞ぎ) と矛盾する印象あり。仕様で「diagnostic mode は exit code を override してよいか」を spec で明示するか consumer 委ねるか
20. **diagnostic mode (`--explain`) で `[missing, will fail]` 等「失敗するぞ」と表記しつつ exit 0** を返す挙動の整合性。文言修正 (= 「missing (would be reported)」等) or `--explain` でも missing は exit 1 にする等の余地
21. **複数 pair invocation で 1 pair の literal miss が他 pair の stale row を silence させる**こと (= short-circuit) の妥当性。aggregate 化 vs short-circuit の policy を spec で固定するか
22. **空 TO 文字列 `""` の扱い**: bump-semver v0.31.0 では `path.Clean("") = "."` 経由で cwd dir を derived として ts 比較し、通常 fresh となる。`glob:` 空 body が usage error と扱われる (= consumer 実装で reject) のと対比して非対称。空 TO は usage error に格上げすべきか
23. **複数 pair で pair N の pattern syntax error が pair 1..N-1 の結果を捨てる short-circuit** が現実的に user 体験を損なうか。errors を aggregate する semantic を推奨するか
24. **cross-source 自動除外** (= §6 で v0.1.0 未定義): consumer 実装は per-source のみ。v0.2 で「derived path が他 source とぶつかった時の扱い」を確定する候補軸。bump-semver v0.31.0 はこの cell を「keep, not exclude」で pin 済 (= 比較 baseline)
25. **(解消済、bump-semver v0.31.1 で fix)** `ignorecase` option は fs walk layer (= glob library) と §3.2 capture regex の **両方に伝播必須**。片側だけ (= v0.31.0 の bump-semver 実装は walk のみ) に伝播すると、case-different match で §3.3 grammar drift panic が出る (= walk match × regex no-match)。本 OQ は bump-semver v0.31.1 で `MatchCollect → expandConcrete → buildRawAndRegex` chain に `caseInsensitive` を thread し、capture regex 先頭に `(?i)` を prepend する形で解消した。spec としては「`ignorecase` は walk と capture regex の両方に同じ mode で伝播する」を §3.2 への暗黙前提として残す
