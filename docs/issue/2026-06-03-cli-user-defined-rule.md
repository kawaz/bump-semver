# Issue: CLI から「自分のファイルにこの rule」を指定する口

- Status: Draft (議論段階、レビュー反映後 / Decision 未確定)
- Date: 2026-06-03
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」), DR-0005 (path-aware confidence ranked candidates), DR-0012 (regex format builtin), DR-0023 (N-arg + vcs: borrow), DR-0024 (glob: prefix matcher), DR-0027 (mini-DSL / `regex:` 不採用), DR-0028 (glob spec v0.1.0)
- Prerequisite issue: `2026-06-03-format-regex-to-text-unification.md` (= DR-0030 候補)。
  本 DR の `--format` enum (Phase 1 は **4 値**: text/json/yaml/toml、xml は Phase 1
  範囲外で codex Critical C-3 反映) は DR-0030 の format=regex 廃止が前提。
  実装順は DR-0030 → DR-0029。

## Context

bump-semver は当初 **「シンプルに使えること」が一番、面倒な指定はしなくて良い** を掲げて
basename / glob → builtin rule の自動判定に倒した (DR-0001, DR-0005)。release 自動化が
実用段階に入り、kawaz は **個人使用だけでなく他者に勧めたい** 段階に到達 (2026-06-03)。

その場合、未知ユーザの「自分が使いたいファイル」が builtin に未対応だと体験が悪い。
ユーザ側で 2 つの逃げ道が欲しい:

1. **「対応してほしい」要望を上げる窓口** (= 別 issue: ISSUE_TEMPLATE / CONTRIBUTING 整備)
2. **「自分で指定できる口」** (= 本 issue)

本 issue は (2) のみを扱う。(1) は別 issue で起票。

### 既存の関連機構

- **DR-0012 `regex` format**: builtin CandidateRule の 1 形式として regex を持つが、
  CLI からの直接指定は不可。`VersionRegex` / `NameRegex` は Go コードで定義された
  rule にしか書けない。
- **DR-0023 N-arg + `vcs:` borrow**: get / compare の入力を可変長化、`vcs:REV` を
  兄弟 FILE で展開する話。**rule 定義そのものは扱わない**。
- **DR-0027 `regex:` 却下**: 派生 sync check (`vcs outdated`) の **mapping syntax** で
  regex を使う案を却下。これは **fs マッチング用途** の話で、本 issue (= ファイル内の
  version 抜き取り) とは別軸。本 issue では却下対象外。

### prior art

- [mattn/bump](https://github.com/mattn/bump) — 単純な正規表現 1 個でファイル内の
  version を bump する Go ツール。シンプル極まる。
- npm `version` script, `semantic-release` の plugin 機構 — config / plugin で
  独自ファイルに対応。
- `release-please` の `extra-files` — JSON path / generic file の path 指定で
  追加ファイルを bump。

## Goal

bump-semver の builtin に **未対応のファイル** をユーザが **1 度の指定** で扱える
ようにする。core 哲学 (「最小限で済む」) は壊さない。

### 前提制約 (= 議論で確定済、2026-06-03)

- **regex 単独では json/yaml/toml の path アクセスがキツい** → **構造化 path も
  Phase 1 必須**。regex 先行案 (= 旧推奨) は撤回。
- **bump-semver は複数 SOURCE 指定が前提** (= peer 検証、DR-0023)。明示 rule も
  **各 SOURCE に紐付け可能** な構文が必須。1 invocation で「a.json は path、
  custom.txt は regex」のような混在も自然に書ける必要がある。

## Options

提案は (1) 「入口の形」と (2) 「指定構文」の 2 軸で直交。

### 軸 1: 入口の形

| 案 | 形 | 特徴 |
|---|---|---|
| A | CLI flag (`--regex=...` / `--version-path=...`) | 単発で完結、scriptable。1 ファイル = 1 invocation |
| B | config file (`.bump-semver.toml` / `bump-semver.config.{yaml,json}` 等) | project 全体を一元管理。CI から `bump-semver get` 引数なしで全 source 検証 |
| C | A + B 両方 | 単発は flag、project は config。kawaz pkf-tasks との相性は B が強い |

### 軸 2: 指定構文 (= ファイル内のどこに version があるか)

| 案 | 例 | 利点 / 弱点 |
|---|---|---|
| (i) regex 1 個 | `--format text --version-regex 'version="([^"]+)"'` | mattn/bump 互換、最強の表現力、format=text で構造化なし。シェル escape の罠 |
| (ii) parser + path | `--format json --version-path '$.plugin.version'` | 構造化 file (json/yaml/toml/xml) で書きやすい、dot-path 直感的。dot-path 仕様の自前実装が必要 (jq subset / json-pointer / `xpath` 等) |
| (iii) parser auto + path | `--version-path '$.plugin.version'`<br>(format は拡張子推定) | 軸 (ii) のさらに短縮。拡張子と中身の乖離 (json なのに .txt) で破綻 |
| ~~(iv) builtin format + override~~ | ~~`--format=regex --version-regex=...`~~ | **DR-0030 で format=regex 廃止により消滅** (= 「format=text + version-regex」で表現、軸 (i) と同じ意味) |

(i)(ii) の両方を許すと表現力 max。(iii) は (ii) の syntactic sugar。

**Phase 1 で (i)(ii) 両方を含める** ことを前提として、以後の議論を進める
(2026-06-03 確定)。理由: regex 単独だと json/yaml/toml の path アクセスが
不便で、それなら最初から path も入れた方が syntactic に統一感が出る。
**format enum は Phase 1 で 4 値** (text/json/yaml/toml)。xml は Phase 2+ で別 path
language (= XPath subset 等) を別 DR で設計後に解禁 (= codex Critical C-3 反映)。
DR-0030 で format=regex を廃止して text + version-regex に統合する前提 (= 二重命名
問題解消)。

### 軸 3: 複数 SOURCE への rule 紐付け (= 複数ファイル明示指定の組織化)

bump-semver は複数 SOURCE 指定が core (DR-0023)。明示 rule も SOURCE 単位で
紐付く必要がある。3 案:

#### 案 1: `--define-rule <PATTERN>` ブロック方式 (= kawaz 起案、2026-06-03)

`--define-rule <PATTERN>` で「これから出る rule 系 flag は **PATTERN にマッチする
SOURCE** に紐づく」というスコープを開く。`<PATTERN>` は完全一致でなく、
**path matcher として一致度 scoring を経て** SOURCE と紐付く (= builtin の
basename/glob と同列の matcher として扱う、DR-0005 の confidence ranked
candidates と同精神)。

##### 1.1 PATTERN 仕様

- **基本**: 1 文字以上の path string。空文字 / `/` 単独は error。
  - 先頭 `/` → 絶対パス指定 (tier 5)
  - それ以外 (`./` 始まり含む) → 相対パス指定 (tier 3)
  - `/` を含まない単一 segment → basename 指定 (tier 2)
- **path 正規化** (= 2026-06-03 nitpick 反映で確定):
  - PATTERN・SOURCE とも `filepath.Clean` 相当 + cwd 基準で **正規化してから比較**
  - **symlink resolve は行わない** (= `/tmp` vs `/private/tmp` の macOS symlink で
    意図的に別 path 扱い、symlink で別名を付けている設計意図を壊さない)
  - 末尾 `/` は `filepath.Clean` で吸収 (= bump-semver の入力は file のみ、dir は来ない)
  - 詳細は [評価規則 (= 後段の Flag のスコープ規約 § 評価規則)] 参照
- **glob**: DR-0024 互換の `glob:<pattern>` プレフィックスで複数 SOURCE に一括適用。
  tier 1 の matcher は **DR-0024 の `doublestar.Match` ベース** (= capture / substitute は
  本 DR の tier 1 では使わない、DR-0028 の `**` 0-segment / `path.Clean` 等の
  substitute 用拡張も本 DR の matcher 用途では未使用)
  ```
  --define-rule 'glob:*.myapp' --format json --version-path '$.app.version'
  ```
  - `glob:` prefix が付いた PATTERN は **glob meta 文字の有無に関わらず常に tier 1**
    (= `glob:package.json` のような meta なし literal も tier 1 として扱う、ユーザが
    意図的に tier を下げる手段として動作)
- **bare PATTERN の glob meta 文字**: bare PATTERN は **完全 literal**。`*` / `?` / `[` / `{`
  等の glob meta が含まれた場合は **error** (= 'use glob: prefix for glob pattern' hint
  付き)。glob 動作が欲しい場合は `glob:` prefix 必須 (= DR-0024 と整合、bare PATTERN で
  glob 解釈する経路は持たない)
- **PATTERN の先頭 `-`**: `--` separator または `glob:` prefix の後に書く (= argparse の
  flag と紛れないように)
- **regex**: `regex:` プレフィックスは **未採用** (DR-0027 で `regex:` 却下と整合。
  matcher は glob で十分)
- **`vcs:`**: PATTERN として **書かない** (= 確定済、kawaz 2026-06-03)。`vcs:` 入力の
  rule 解決は実 path に正規化される:
  - **借用形式** (= `vcs:REV`、兄弟 FILE から展開) → **peer-expand された各 vcs source は、
    対応する兄弟 FILE 側の rule をそれぞれ独立に借用** (例: `get a.json b.json vcs:main` で
    `a.json` と `b.json` が別 rule を持つ場合、`vcs:main:a.json` は `a.json` の rule、
    `vcs:main:b.json` は `b.json` の rule をそれぞれ独立に継承)
  - **単独形式** (= `vcs:REV:FILE`、FILE 明示) → **VCS root 相対** で FILE を解釈し、
    その path 用に定義した rule を適用 (= cwd でなく VCS root 基準。`vcs root` の値を使う、
    DR-0024 と同じ機構)。PATTERN とマッチさせる時も **PATTERN を VCS root 相対に正規化**
    してから比較 (例: cwd が subdir でも `--define-rule etc/VERSION` は VCS root の
    `etc/VERSION` を指す)
  - どちらの形式でも実 path 側で `--define-rule` すれば足りる。`vcs:` の prefix 自体は
    「値の出所 (= source kind)」を示すもので、path matcher としての PATTERN には
    書かない (= 書いても意味的に冗長で、実 path 表現で同じことが書ける)

##### 1.2 PATTERN ↔ SOURCE の一致度 scoring

複数 SOURCE と複数 `--define-rule` の組み合わせから、**各 SOURCE につき 1 rule を選ぶ**。
選び方は **一致度の高い順** (= 2026-06-03 nitpick 反映で **4 段に整理**、旧 tier 4
「ドット相対」は cwd 依存で再現性なく tier 3 と区別できないため削除):

| tier | matcher | 例 |
|---|---|---|
| 5 | **絶対パス完全一致** | `--define-rule /home/x/proj/package.json` vs `/home/x/proj/package.json` |
| 3 | **相対パス完全一致** (cwd 起点で正規化後の比較) | `--define-rule othersystem/package.json` vs `othersystem/package.json`、`--define-rule ./package.json` vs `package.json` も同 tier (= 正規化後一致) |
| 2 | **basename 一致** | `--define-rule package.json` vs `ts/package.json` |
| 1 | **glob マッチ** | `--define-rule glob:*.myapp` vs `foo.myapp` (`glob:` prefix が付いた PATTERN は meta 有無問わず全て tier 1) |
| 0 | **builtin** (= CLI rule 未マッチで fallback) | 内部の CandidateRule (DR-0005 confidence と同じ ranking) |

**`./X` と `X` の関係** (= 旧 tier 4 削除に伴う規約):

- PATTERN `./X` は **正規化後 `X`** として扱う (= `filepath.Clean`)
- SOURCE `./X` も同様に正規化後 `X`
- 正規化後の文字列完全一致で tier 3 確定
- 「ドット相対」は **tier 3 の単なる書き方** であり、別 tier を作らない

**tier 5 (絶対パス) の正規化**:

- `filepath.Clean` で `..` / 末尾 `/` / `./` 重複を吸収して比較
- **symlink resolve はしない** (= 設計意図を尊重)
- 例: SOURCE が `/tmp/proj/foo.json` で PATTERN が `/private/tmp/proj/foo.json` の場合は
  **tier 5 にマッチしない** (= macOS の `/tmp -> /private/tmp` symlink でも別 path 扱い)

**glob を最下層に置いた理由** (= 2026-06-03 確定):

- glob は **pattern 中身の判定** が必要 (= 範囲指定)、bare path は **literal 一致**
  で具体性が一意。異種比較では bare 常勝が自然。
- 「相手がドット付き相対名」「basename 一致」のような bare path tier (2-5) と
  glob が同 SOURCE にヒットしても、**bare が勝つ** (= glob の負け)。これにより
  「glob でざっくり指定しつつ、特定 path だけ別 rule で上書き」が自然に動く。
- glob 同士の一致度比較は **判定不能** (= `src/**/*` vs `**/*.ts` のどちらが
  具体的かはユーザ意図次第、`**` や `*` の数で機械的に決めると逆に分かりにくい)。
  → glob 同士の重複ヒットは **常に ambiguous error**。

**重複 SOURCE の扱い**: 同一 path を複数 positional に書いた場合 (= `bump-semver get
package.json package.json`) は **dedup しない**。各 SOURCE は独立に rule 解決する
(= peer 検証で同値性を確かめるなら同 file を 2 回読んでも結果は同じ、surprise 最小)。

**tier と confidence の axis 関係** (= 2026-06-03 nitpick 反映):

- 本 DR の tier scoring は **SOURCE と PATTERN の紐付け解決** という axis (= CLI rule
  matcher)
- DR-0005 confidence は **builtin 内部の rule fallback** という別 axis
- 両者は **直交**。評価順は:
  1. `--define-rule` ヒット判定 (tier 1+ で 1 つ確定) → ヒット時はそれで rule 確定
  2. ヒット無しなら builtin の confidence 3 → 2 → 1 fallback (= DR-0005 そのまま)
  3. DR-0010 fallback hint は (2) の confidence 1 マッチで従来通り発火 (= 本 DR で
     挙動変更なし)
- **CLI rule (tier 1+) は builtin (tier 0) より常に優先** (= 案 X、0f 確定)。help では
  「user-defined rules > builtin rules」と表現

**CLI rule のマッチ失敗時** (= 2026-06-03 codex 反映、High H-1): CLI rule が tier 1+ で
マッチした後、その rule で version/name の抽出が失敗した場合は **hard error**:

- builtin への自動 fallback しない (= ユーザが明示指定した rule で失敗したのは
  「明示 rule の責任」であり、builtin に逃げるのは surprise)
- error message は **source path + matched PATTERN + 失敗した field (version/name) +
  原因 (regex no match / regex multi match / path not found / non-string scalar /
  type mismatch 等)** を含める
- builtin の try/fallback は **builtin テーブル内部だけに閉じる** (= DR-0005
  confidence ranking は CLI rule とは独立、CLI rule の責任範囲では fallback しない)
- これは「論点 0d 補強」として明示的に pin: CLI rule の extraction failure は
  silent fallback しない (= typo 検出が確実、debug 容易)

##### 1.3 具体例 (= kawaz 提示、2026-06-03、tier 4 削除に伴い更新)

```
bump-semver get \
  package.json ts/package.json othersystem/package.json \
  --define-rule othersystem/package.json --version-path '$.latest.version' \
  --define-rule ./package.json --version-path '$.original.version'
```

- `package.json` → `--define-rule ./package.json` (tier 3、正規化後 `package.json` で
  一致) > builtin (tier 0) → `$.original.version` 採用
- `ts/package.json` → `--define-rule` 該当無 → builtin (basename `package.json`)
  → 通常の `$.version` 採用
- `othersystem/package.json` → `--define-rule othersystem/package.json` (tier 3) >
  builtin (tier 0) → `$.latest.version` 採用

##### 1.4 長所

- 1 flag = 1 値で素直。順序依存の曖昧さなし
- 構文 (i) regex と (ii) path の混在が同 invocation で自然
- **PATTERN matcher として builtin と整合** (= DR-0005 confidence ranking の拡張)
- glob: で複数ファイル一括適用が自然 (DR-0024 と整合)
- block 単位なので flag 増えても OK (`--name-regex` 等)

##### 1.5 短所 / 論点

- 一致度 tier 表を実装 + ドキュメント化する必要 (= cognitive load)
- 同 tier 複数 `--define-rule` ヒット時の挙動 (= ambiguous)
- builtin との tier 比較 (= 「CLI rule は常に builtin より上」vs 「tier 番号で公正に比較」)
- `--define-rule X` を書き忘れた `--format`/`--version-path` flag (= 孤立 flag) の error UX
- **kawaz CLI 規約 (オプション位置固定を避ける) と背反する例外** (= 2026-06-03 nitpick 反映)。
  本機能は「`--define-rule` ブロック」という宣言的構造を導入するため、ブロック開始の
  位置がスコープ semantics に直接効くトレードオフを取った。ブロック内の flag 順序は
  不問だが、`--define-rule` の位置自体は順序依存。`cli-design-preferences.md` の規約から
  外れる唯一の例外 → DR 確定時に Design rationale をコード側にも明記する
  (`document-design-rationale.md` ルールに従う)。

#### 案 2: `--rule 'k=v,k=v'` の 1 flag 集約

```
bump-semver get VERSION package.json \
  --rule 'src=VERSION,format=text,version-regex=^version: [vV]?([0-9.]+)' \
  --rule 'src=package.json,format=json,version-path=$.version'
```

長所:
- 1 SOURCE = 1 flag で凝集度高
- shell 1 line でも読みやすい (= --define-rule より短い)

短所:
- comma / equal がエスケープ地獄 (regex に , や = が含まれると詰む)
- key=value miniformat の自前パーサが必要
- 値の quoting が二重 (shell + miniformat)

#### 案 3: positional 直後 inline (= 直前紐付け)

```
bump-semver get \
  VERSION    --format text --version-regex '...' \
  package.json --format json --version-path '$.version'
```

長所:
- 構造が視覚的に明確 (SOURCE と rule が並ぶ)
- `--define-rule` のラベル名を書かなくて良い

短所:
- positional と flag の境界が ambiguous (= 「次に来る positional」が新 SOURCE か
  rule の値か)。とくに `--rule` 系 flag が複数引数を取る場合
- `bump-semver get a.json --regex R` のように **1 SOURCE だけ rule** を指定する
  例で、`a.json` だけ rule あり / 後続 SOURCE が無い、の境界が読みにくい
- shell history で「直前 SOURCE」の文脈が崩れやすい (= 行編集で順序が崩れる)

### 推奨方向 (= 案 1 を採用候補、対抗案として 2/3 を並置)

kawaz 起案の **案 1 (`--define-rule <SOURCE>`)** を Phase 1 の推奨採用候補とする。
理由: 表現力 / 読みやすさ / 拡張性 (= 後で flag 追加できる) の 3 軸でバランスが
最も良い。案 2 はエスケープ問題が重く、案 3 は positional/flag 境界が曖昧。

### dot-path 仕様 (案 (ii) を取る場合) — 後段「dot-path 仕様 (= 構文 (ii) で `--version-path` に書ける文字列)」で MVP の最小 subset を確定済

## 設計上の論点 (= 未決)

### `--define-rule` ブロックまわり (= 案 1 採用前提の細部)

#### 確定済 (= 2026-06-03 議論で決定)

- **0b 確定**: positional SOURCE 順と `--define-rule` 順の **一致強制は不採用**。
  `--define-rule <PATTERN>` は pattern matcher として SOURCE と紐付く (1.2 節)。
- **0d 確定**: CLI rule (= `--define-rule`) は **builtin より優先** (= ユーザが
  明示的に書いた指示が builtin の自動判定に勝つ)。「組み込みと被る場合は
  ユーザ定義優先」(kawaz)。具体的な tier 比較は論点 0f を参照。
- **0j-l 確定**: builtin 無効化オプション (`--no-builtin-rules` / `--builtin-fallback-only`
  / config の `disable_builtin` リスト) は **Phase 1 では含めない**。0d (CLI rule
  優先) で「`--define-rule <衝突 path> --format=... --version-path=...` で上書き」が
  事実上の防御になるため、専用 flag を最初から導入しない。後発 builtin との
  衝突が実際に起き、上書きでは不十分な事例 (= 自作 rule も書きたくないユースケース)
  が観測されてから Phase 2+ で別 DR として再検討。
- **0a 確定**: `--define-rule` の **強制は不要**。`--format` / `--version-regex` /
  `--version-path` 等は `--define-rule` の前に書けば **グローバル** (= 全 SOURCE
  デフォルト) として機能する。単一ファイル / local vs remote の peer 比較などの
  典型ユースは「グローバルだけ」で書ける。複数 SOURCE で個別指定が必要な
  ときだけ `--define-rule` を使う (= ブロック宣言)。グローバルとブロックの混在も
  OK で、ブロックがグローバルを override する。
  - **0a 補強 (= 2026-06-03 nitpick 反映)**: typo 防御のため、`--define-rule` を 1 つ
    以上書いた invocation では、**最初の `--define-rule` 出現以降に置いた rule 系 flag
    は必ずいずれかのブロックに属さねばならない** (= ブロックを跨ぐ位置に rule 系 flag を
    置くと error)。グローバル flag は **最初の `--define-rule` より前にだけ書ける**。
    これによりユーザが `--define-rule` を typo して `--definerule` と書いた場合、
    その後の `--format`/`--version-path` 等は「ブロック外の rule 系 flag」として error
    になり、silent な誤動作 (= 意図せず global として処理される) を防げる。draft の
    パターン 1/2/3 (= 後段) はこの規約で全て表現可能。
- **論点 4 確定**: bump 系 (`patch` / `minor` / `major` / `pre`) の **--write も
  CLI rule で許可**。理由: write ができないと bump-semver の存在意義が薄れる
  (= mattn/bump 相当の核機能はそこ)。get / compare / bump 全 verb で CLI rule が
  動く。
  - **論点 4 安全弁 (= 2026-06-03 nitpick 反映)**: 複数 SOURCE 一括 write 時の
    **atomicity** を Phase 1 必須規約とする:
    1. 全 SOURCE について **rule 解決 + version/name 値抽出 + 整合性検証** (= 全 source で
       同 version) を **先に全件実行**
    2. 全件成功してから初めて **書き込み開始** (= 1 SOURCE でも失敗したら 1 ファイルも
       書かない)
    3. 書き込み中の物理 IO 失敗 (= partial write 後の rollback) は別問題で本 DR scope 外
  - これにより「誤った regex で版以外を破壊」事故は「validate 段階で抽出失敗 → 書き込み
    一切なし」のフローで防げる。dry-run preview は Phase 2+ 検討。
  - **論点 4 書き戻しアルゴリズム** (= 2026-06-03 codex 反映、Critical C-1):
    `--version-path` と `--version-regex` の併用時 (= path で取った値に regex を適用)
    の **bump --write** の書き戻し仕様を Phase 1 で pin する:
    1. `--version-path` で scalar string を 1 個取得 (= 非 string scalar は error、
       array / object も error)
    2. その string に `--version-regex` を適用、capture group 1 を取得
    3. group 1 の **byte range だけ** を新 version 文字列に置換、それ以外の文字
       (= prefix `myapp v` / suffix の空白 / quotes 等) は **そのまま preserve**
    4. 書き換えた新 string を **元の path** へ scalar として戻す (= 木構造の再
       シリアライズ時に当該 path の値だけ差し替え、他の field は変更しない)
    5. `--version-regex` のみ (= path なし) の場合は **ファイル全文** に regex を当て、
       group 1 の byte range だけを置換 (= 旧 builtin `regex` format / DR-0012 と同じ
       挙動、line-anchored 推奨)
    6. `--version-path` のみ (= regex なし) の場合は path で取った scalar string を
       version として読み、書き戻し時はそれを **新 version 文字列で完全置換**
       (= path 値全体が version、prefix / suffix なし前提)
  - 例 (`info.json` の `$.name` から `"myapp v1.0.5"` を取り、regex で `1.0.5` を抜く
    場合の patch bump):
    ```
    bump-semver patch info.json --format json --version-path '$.name' \
      --version-regex 'v(\d+\.\d+\.\d+)' --write
    # before: {"name": "myapp v1.0.5"}
    # after:  {"name": "myapp v1.0.6"}
    # → $.name 全体を置換せず、regex group 1 (= "1.0.5") の byte range だけ "1.0.6"
    #   に置換、prefix "myapp v" は preserve
    ```
- **0c 確定**: 同 tier の `--define-rule` が同じ SOURCE に複数ヒットした場合は
  **ambiguous error**。最後勝ち / 最初勝ち は採用しない。理由: 構造的曖昧さを
  silent に決めると debug 困難。glob 同士の重複ヒットは「pattern 中身の判定が
  必要で、機械的な precedence は decision 困難」ため、**glob 同士は常に
  ambiguous** (= 1 SOURCE に glob `--define-rule` が 2 個ヒットしたら error)。
  bare path 同士 (tier 2-5) の重複ヒットは同 PATTERN を 2 度書いた時のみ
  発生し、これも error (= 重複宣言を意図的に許す理由がない)。
- **0f 確定**: CLI rule vs builtin の tier 比較は **案 X (CLI 常勝)**。
  「CLI rule が tier 1 以上でマッチすれば builtin より上」をシンプルに採用。
  将来 config file (Phase 2 / 後発) が入る場合の precedence 想定:

  ```
  builtin < user config (~/.config/bump-semver/config.toml) < project config
  (./.bump-semver.toml) < CLI (--define-rule / --format / --version-path ...)
  ```

  一般的な「広い設定 < 狭い設定 < ad-hoc 指定」の流れ。user config / project
  config は本 DR の Phase 1 scope 外だが、将来導入時の precedence 規約として
  記載しておく。

- **0g 確定**: short option (`-D`) は **不採用**。理由: 将来 `d` 始まりの強い
  オプションが出てきた時に ambiguous、`-D` は gcc の preprocessor define
  (`-D<name>=<val>`) を強く連想させて意味が衝突する。kawaz CLI 規約の
  「ショートオプションを指示なく追加しない」と整合。
- **0h 確定**: PATTERN matcher の prefix は **`glob:` のみ受け付ける**。`regex:`
  は DR-0027 で却下と整合。`exact:` / `name:` 等の追加 prefix は bare PATTERN
  の tier 2-5 で自然カバーできるため不要。大抵のケースで `glob:` も不要。
  - **0h 補強 (= 2026-06-03 nitpick 反映)**: bare PATTERN に glob meta 文字 (`*` / `?` /
    `[` / `{`) が含まれた場合は **error** (hint: 'use glob: prefix for glob pattern')。
    リテラルとしてこれらの文字を path 名に含めたい場合は現状規約では救えない
    (= Phase 2+ で escape 検討 TODO)。これにより「bare = literal、prefixed = glob」の
    境界が機械的に検証可能になり、gitignore 系の「bare で glob 解釈」習慣との乖離も
    error で警告される。
- **0i 確定**: PATTERN の declaration order は semantics に組み込まない (= 0c
  で「同 tier 重複は error」と確定済、order は無関係)。

### 将来コンフリクト防御 (= builtin 後発追加への備え)

- **論点 0j: builtin 無効化オプションの必要性** (= kawaz 2026-06-03):
  bump-semver の builtin に **後発で追加された rule が、たまたまユーザの自作
  アプリで使っていたファイル名と衝突** するケースに備えたい。例: ユーザが
  自作アプリで `myapp.config` を使っている → bump-semver 後発バージョンで
  `MyApp` (別物) 用に `myapp.config` の builtin rule が入る → ユーザの
  `bump-semver get myapp.config` が**別プロジェクト用の builtin に偶然マッチ
  して誤検出**。  
  防御策: **builtin の basename / glob 一致を無効化し、拡張子フォールバックだけ
  残す** オプション。設計案 3 つ:
  - **案 P: `--no-builtin-rules <PATTERN>`** ピンポイント無効化 (= 衝突した 1 ファイル
    だけ builtin OFF。tier matcher と同じ PATTERN 文法)
  - **案 Q: `--builtin-fallback-only`** invocation 全体で「basename / glob を
    無効化、`*.json` / `*.yaml` / `*.toml` 等の拡張子 generic fallback だけ
    残す」。プロジェクト全体で防御する用途
  - **案 R: `.bump-semver.toml` で `disable_builtin = ["myapp.config"]` の
    list を持つ** (= Phase 3 config 連動)。 CI で一貫した防御
- **論点 0k: Phase 1 で含めるか**: 「対応してほしい窓口 (= 別 issue)」が機能
  すれば builtin への新規追加は kawaz の手元でレビューを経るので、衝突は事前
  検出できる可能性が高い。一方、bump-semver を使う他者にとっては「自分が
  関知しないうちに builtin が増える」リスクは残る。Phase 1 で **案 P
  (`--no-builtin-rules`) だけ MVP 提供**、案 Q / R は需要ベースで別 phase、が現実的。
- **論点 0l: 無効化 vs override**: 論点 0d の「CLI rule が builtin より優先」が
  あるので、衝突対象に対しては `--define-rule <衝突ファイル> --format=... --version-path=...`
  で **上書き** すれば事実上の防御になる。`--no-builtin-rules` は「自作アプリの
  ルールも書きたくない、単に builtin に触ってほしくない」用途のための独立 flag。
  「rule 上書きで間に合う / 専用 flag で意図を明示すべき」のどちらに倒すか。

### 論点ステータス (= 2026-06-03 codex M-1 反映で Closed / Open 明示分離)

#### Open (= 本 DR Phase 1 で確定不要、Phase 2 で別 DR 起票時に決める)

- **論点 1 (Open)**: B (config file) の format。TOML / YAML / JSON のどれを正本にするか。
  bump-semver 自身は config を持たない方針だったので新規導入の重み有。kawaz/* は
  TOML (Taskfile.pkl は別物) 寄りな印象。**本 DR scope 外** (= Phase 2 別 DR)。

#### Closed (= 本 DR で確定済、reopen 不要)

- **論点 2 (Closed)**: builtin との precedence → § 1.2 と 0d で確定済 (= CLI rule
  常勝、confidence は別 axis、CLI rule 失敗は hard error)。
- **論点 3 (Closed)**: 構文 (i)(ii) 両方を MVP に含めるか → 確定: **両方含める**
  (= 議論初期の旧推奨 "regex 先行" は撤回、§ Phase 1 で必要な flag で text +
  json/yaml/toml の対称構造として提供)。
- **論点 4 (Closed)**: bump 系 (write) で CLI rule を許すか → 確定: **許可**
  + write atomicity + path/regex 併用時の書き戻しアルゴリズム pin (= § 推奨方向 +
  論点 4 確定 + 論点 4 書き戻しアルゴリズム)。
- **論点 5 (Closed)**: `vcs:` borrowing の rule 解決 → § 1.1 「`vcs:` の扱い」で確定
  (= 借用形式は対応する兄弟 FILE の rule を独立に継承、単独形式は VCS root 相対で
  rule 解決)。
- **論点 6 (Closed、nitpick + codex H-3 補強)**: name 抽出は **Phase 1 に含める**。
  `--name-regex` / `--name-path` を `--version-regex` / `--version-path` と
  対称に提供。kawaz CLI 設計の対称性原則と整合 (= 4 flag セット全部入れる方が自然)。
  - **multi-file bump 時の name safety rail** (= 2026-06-03 codex 反映、High H-3):
    builtin の既存挙動 (= multi-input 時に name も cross-check して「別 project を
    一緒に bump しない」guard、README L476-478) を user-defined rule でも維持。
    具体的には:
    - user-defined rule で `--name-regex` / `--name-path` を **書いた** source は
      name-check 対象 (= 値が一致しない他 source と組み合わせると mismatch error)
    - **書かなかった** source は name-check 対象から **除外** されるが、その際
      **stderr に warning hint** を出す (= "note: source &lt;path&gt; has no name source,
      multi-source name consistency check skipped for this entry; consider adding
      `--name-regex` / `--name-path` if this is a multi-project bump")
    - silent downgrade はしない (= name-check が暗黙にスキップされて別 project を
      一緒に bump する事故を防ぐ)
    - `--no-hint` / `-q` / `-qq` で抑制可能 (= 既存規約と整合、意図的に skip したい
      ケース用)
  - これにより既存の safety rail が user-defined rule 導入で薄まる事故を防ぐ
- **論点 7 (Closed、codex C-2 で強化)**: CLI `--version-regex` は
  **builtin (DR-0012 / first-match-only) よりも厳格** な cardinality 規約を採る:
  - **`get` / `compare` 時**: regex を当てて **exact one match** (= match 数 1) を期待。
    `0 match` も `2+ match` も **error** (= ユーザの明示指定なので、暗黙の first-match
    依存を避ける)
  - **`bump --write` 時**: 同じく **exact one match** で error 発生時は **書き込み一切
    なし** (= 論点 4 atomicity と整合)
  - **理由**: user-defined rule は「ユーザが明示的に書いた指示」であり、複数候補行の
    silent な first-match 採用は debug 困難で誤書換え事故の温床。builtin の
    first-match-only (= DR-0012) はテストで担保された安全領域だが、CLI rule は任意 regex を
    許すため exact-one が安全側。`(?m)^` line-anchor は **推奨**するが強制しない (= 任意
    regex の柔軟性は残す、ただし exact-one 制約で誤書換えを防ぐ)
  - **同規約を name 側にも適用** (`--name-regex`、対称性): exact-one、`0 match` で
    name 不一致警告は出さず error。name は optional なので「--name-regex を書いた以上は
    1 match を期待」が筋
  - **対比 (= builtin との挙動差を明文化)**:
    - builtin `regex` format (DR-0030 後は `text + version-regex`): **first match only**
      (DR-0012、`(?m)^` line-anchor で 1 マッチに収束させる前提)
    - CLI `--version-regex`: **exact one match** (= 厳格化、ユーザ明示指定の責任)
- **論点 8 (Closed)**: error UX (= regex がマッチしない / dot-path が解決しない時の
  メッセージ)。
  - CLI rule (`--define-rule`) の失敗は **hard error** (= builtin への自動 fallback
    なし、§ 1.2 末尾規約 + codex H-1 反映)
  - error message は **source path + matched PATTERN + 失敗 field + 原因** を含む
  - dot-path 解決失敗時の error フォーマットは **builtin の path 解決失敗と同じ
    フォーマット** (= 統一)、具体 message は実装時
  - builtin の confidence 1 fallback hint (DR-0010) は本 DR で挙動変更なし

## 推奨方向 (= 議論で固まりつつある形、Decision 直前段階)

`C (flag + config) × (i)(ii) 両方 × 案 1 (--define-rule ブロック) × グローバル+ブロック scope` を **段階導入**:

1. **Phase 1 (本 DR)**: CLI flag (A) + 構文 (i) regex / (ii) path 両方 +
   `--define-rule <PATTERN>` ブロック方式 (案 1) + グローバル/ブロック scope +
   全 verb (get / compare / bump 系 write) で CLI rule が動く。**bump --write
   も CLI rule で解禁** (= 論点 4 確定)。
2. **Phase 2** (別 DR): config file (B) 導入。`.bump-semver.toml` 等。CI 連携で
   グローバル設定として機能。
   - **Phase 2 config schema は Phase 1 CLI block の direct serialization では
     ない** (= 2026-06-03 codex 反映、High H-2): Phase 1 CLI block は「CLI 体験
     として最適化された形」であり、Phase 2 config は「internal rule object に近い
     form」を採用する。具体的には:
     - `version_paths: []` (= 複数 path OR fallback、現行 builtin の
       `Cargo.toml`/`pyproject.toml` の OR 構造に対応)
     - `match_mode: "exactly-one" | "first"` (= CLI rule は exactly-one、builtin は
       first を表現するため)
     - `rewrite_mode: "scalar" | "multi-sync"` (= 現行 builtin の `project.pbxproj`
       / `Info.plist` の multi-update 構造に対応)
   - これにより Phase 1 syntax は dead-end ではなく、CLI 体験の seed として残り、
     Phase 2 config schema は独立した rule object 表現として設計される
   - Phase 1 → Phase 2 の移行は **CLI も config も並立** で、CLI block を config に
     「機械変換」するツール (`bump-semver config init` 等) は Phase 2+ で別途
3. **Phase 3+** (= 需要ドリブン、別 DR): builtin 無効化 (`--no-builtin-rules` 等、
   論点 0j-l)、dot-path 高度化 (quoted key 等)、その他。

旧推奨 (= regex 先行 + read-only 限定) は撤回。理由は 「前提制約」セクション +
論点 4 確定。

## Help / ドキュメント表示の方針 (= 2026-06-03 確定)

`--help` / README で builtin 機能を見せるとき、**「対応フォーマット一覧」ではなく
「組み込みルール一覧」** として並べる。

理由:
- ユーザが `--define-rule` で追加するのも **rule の塊** (= format + version-path/regex
  + name-path/regex の組み合わせ単位)。help でも同じ単位で見せれば、ユーザが
  「自分が追加するイメージ」と builtin が直接対応する
- 将来 builtin 無効化 (`--no-builtin-rules <PATTERN>`、論点 0j-l) を入れる時に
  「この rule を無効化する」と説明が一直線になる (= 「この format を無効化」だと
  json 全体が止まる印象になり実態と乖離)

**format 露出の 2 層構成** (= 2026-06-03 codex M-3 反映):

- **builtin 側**: rule table として見せる (= path matcher + Confidence + Format +
  Version source + Name source の 1 行表記)。format は table の 1 列で見せるが、
  ユーザが直接操作する API ではない。「`package.json` には JSON parser + `$.version`」
  のような **rule 単位の組み合わせ** が表面 (= 上記 § 表示イメージ)
- **user-defined 側**: `--format` enum を「Rule fields」セクションで **明示的に公開**
  (= 上記 § 想定 CLI 表面 / § Phase 1 で必要な flag)。ここでは format がユーザ API
  なので、`text|json|yaml|toml` の選択肢と各の意味を 1 等地で見せる
- これにより「builtin は rule の塊」「user-defined は field 単位で組み立てる」の
  二相設計を help で明示。「内部分類を隠したい」と「format を見せたい」の表面的
  矛盾は **見せる場所を分ける** ことで解消

### 表示イメージ (= help / README、draft)

DR-0030 の format=regex 廃止後の表記。Path matcher の右に **Confidence** 列を追加し、
DR-0005 confidence (3=path-pinned / 2=basename / 1=glob fallback) を明示。本 DR の
PATTERN match strength (= CLI rule の matcher) と直交する別 axis である事が一発で
分かる (= 初心者ペルソナ反映: 列名を略さない、`Conf 0` の数字衝突を避ける、表下に
別 axis 注記):

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
  ...

  Confidence: 3 = path-pinned exact (e.g. `.claude-plugin/marketplace.json`)
              2 = basename exact (e.g. `package.json` anywhere in tree)
              1 = glob fallback (e.g. `*.json`)

User-defined rules (--define-rule): not in the table above? Define your own rule.
  Rules you define on the CLI **always override** builtin rules (the two are
  separate axes: builtin uses Confidence, --define-rule uses PATTERN match
  strength). See `bump-semver --help-full` § User-defined rules for syntax,
  match-strength table, and examples.
```

- 既存 builtin の 1 ルール = 1 行で path matcher (basename / glob) と中身 (format
  + version-path/regex + name-path/regex) を一覧表示
- `Conf` 列で DR-0005 confidence を見せ、`fallback` 表記 = confidence 1 と機械的に対応
- 末尾に「user-defined rules」セクションで CLI rule の入口を案内、tier 関係も 1 行で
- `--define-rule` の詳細は **`bump-semver --help-full`** の "User-defined rules"
  セクションで **tier 表 (5/3/2/1/0)** と「extend builtin rules with your own」を統一
  解説 (= flag に対する `--help` は存在しない、`--help-full` で集約)
- 旧 `regex` 表記は DR-0030 で `text + version-regex` に統合 (= help からも消える)

### Help 階層への配置規約 (= 2026-06-03 実機調査 + kawaz 指示)

**現状の help 階層** (= 2026-06-03 `bin/bump-semver --help` / `--help-full` 実機確認):

- `bump-semver --help`: command 一覧 + 入力種別 + builtin 表へのポインタ (短い)
- `bump-semver --help-full`: 全 builtin 表 + 全 verb + 全 option (長い、本ファイル参照)
- `bump-semver <verb> --help`: 各 verb の Inputs / Options / Examples (= get / compare /
  patch / minor / major / pre / vcs)
- `bump-semver <verb> --help-full`: **現状未実装** (= unknown option エラー)、root の
  `--help-full` で代用
- `bump-semver vcs <subverb> --help`: **現状未実装** (= 各 vcs サブコマンドの個別 help
  経路が無い、`vcs get/is/diff/...` の `--help` は root vcs --help でカバー)

**DR-0029 で追加する flag の配置**:

| flag | get/compare/patch/minor/major/pre `--help` | root `--help-full` | vcs `--help` |
|---|---|---|---|
| `--define-rule <PATTERN>` | Options セクションに 1 行 summary + `(see --help-full)` 誘導 | 専用セクション「User-defined rules」で詳細 (= tier 表 + syntax + 例) | 出さない (= 意味なし) |
| `--format <text\|json\|yaml\|toml>` | 同上 | 同上 (§ Phase 1 で必要な flag の表、xml は Phase 1 範囲外) | 出さない |
| `--version-path <DOTPATH>` | 同上 | 同上 | 出さない |
| `--version-regex <PATTERN>` | 同上 | 同上 | 出さない |
| `--name-path <DOTPATH>` | 同上 | 同上 | 出さない |
| `--name-regex <PATTERN>` | 同上 | 同上 | 出さない |

理由:

- bump 系 verb (get / compare / patch / minor / major / pre) は version 抽出を扱うので
  全 verb で `--define-rule` 等が意味を持つ
- vcs サブコマンド (= get root / is clean / commit / push / tag 等) は version 抽出を
  しないので、help に rule 系 flag を出さない (= cognitive load 削減)
- 各 verb の `--help` で全 flag 詳細を書くと 6 重複で読みにくい → summary 1 行 + 詳細は
  `--help-full` 誘導が cli-design-preferences.md 「過不足のないコンテキスト対応補完」と
  整合 (= 補完候補は出るが詳細は一箇所に集約)

**root `--help-full` の "User-defined rules" セクション (= draft 仕様)**:

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
  —  (no --define-rule match) → falls through to builtin (= Conf 3/2/1 で評価)

  注: 数字 5/3/2/1 は内部 score をそのまま見せる (= 4 等の空き番号は将来用、無理に
      連番化しない)。Confidence (= builtin の別 axis) と数字が混同しないよう、
      --define-rule 不マッチは "—" 表記 (= 数字を使わない)。

Format options:
  --format text          全文 / regex で抽出 (--version-regex 必須、exact one match)
  --format json|yaml|toml
                         構造化 file (--version-path で抽出、--version-regex も併用可)

  (--format xml は Phase 2+ で別 path language と共に解禁予定、Phase 1 では未対応)

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

See also: 'Builtin rules' table above (tier 0 で fallback される CandidateRule 一覧).
```

**各 verb `--help` での summary 行 (= 1 行表記、初心者ペルソナ反映で目的ベース文面)**:

```
Options:
  ...
  --define-rule <PATTERN>    Define how to extract version from <PATTERN> files
                             (use when your file is not in the builtin table; see --help-full)
  --format <FMT>             How to parse the file: text (regex) | json | yaml | toml
                             (xml is Phase 2+)
  --version-path <DOTPATH>   For json/yaml/toml: where the version field is (e.g. $.version)
  --version-regex <PATTERN>  For text format: regex with one capture group for the version
                             (exact one match required, 0/2+ matches = error)
  --name-path <DOTPATH>      Optional: where the package name field is (json/yaml/toml)
  --name-regex <PATTERN>     Optional: regex with one capture group for the package name
```

**root `--help` (= short help) への入口誘導 1 行追加** (= 初心者ペルソナ反映):

builtin 表へのポインタ (= 現状 `Files are auto-detected by basename (Cargo.toml, ...)`)
の **直下** に以下を追加:

```
Not in the table? Define your own rule with --define-rule (see --help-full).
```

これがないと、初心者は short help を読み終わって「対応してないなら諦め」と判断し
`--help-full` を開く動機が生まれない。1 行で入口を提示する。

**Phase 2+ 改善余地** (= 本 DR scope 外、別 issue で扱う):

- 各 verb に `--help-full` を実装する (= 現在 root のみ、kawaz CLI 規約「各レベルで help
  階層」と整合化)
- `bump-semver vcs <subverb> --help` の階層を整える (= 現状 `vcs tag --help` 等が
  unknown action error)
- これらは DR-0029 単独で必須ではない (= flag detail を root `--help-full` に集約する
  暫定運用で Phase 1 が動く)
- **error UX 文面規約** (= 初心者ペルソナ反映): `--format text` で `--version-regex`
  が抜けた / dot-path 解決失敗 / regex がマッチしない 等の error message に「直し方の
  例」を含める規約 (例: `--format text requires --version-regex to extract the version;
  example: --version-regex 'v?(\d+\.\d+\.\d+)'`)。Phase 1 では「error UX 規約は別 issue」
  と明示しておけば、後追いで漏れない
- **Builtin 表の完全列挙** (= 初心者ペルソナ反映): root `--help-full` の Builtin rules
  表は **省略なしで全件列挙** する (= 現状 `--help-full` の Supported file formats 一覧
  と同等の網羅性を、新表記で維持)。初心者が「自分の file が `...` の中に隠れているかも」
  と判断保留せずに済む

## 想定 CLI 表面 (Phase 1, draft)

```
# 単純: peer 検証 (= text + json)
bump-semver get \
  VERSION package.json \
  --define-rule VERSION --format text --version-regex '^version:\s*[vV]?([0-9]+\.[0-9]+\.[0-9]+)' \
  --define-rule ./package.json --format json --version-path '$.version'

# builtin との混在 (= plugin.json は builtin の json rule で自動判定)
bump-semver get plugin.json my-custom-file \
  --define-rule my-custom-file --format text --version-regex '...'

# glob 適用 (= xxx.myapp すべてに同 rule)
bump-semver get a.myapp b.myapp c.myapp \
  --define-rule 'glob:*.myapp' --format json --version-path '$.app.version'

# 同 basename を複数 path で区別 (= 一致度 scoring、詳細例は § 1.3 参照)
bump-semver get \
  package.json ts/package.json othersystem/package.json \
  --define-rule othersystem/package.json --version-path '$.latest.version' \
  --define-rule ./package.json --version-path '$.original.version'
# tier scoring 詳細は § 1.3 参照 (= 全 tier が tier 3 / builtin の組合せで決まる)

# compare (= F1 基準で OTHER と比較、DR-0023)
bump-semver compare gt \
  VERSION vcs:main@origin \
  --define-rule VERSION --format text --version-regex '...'
# = VERSION (CLI rule) vs vcs:main@origin:VERSION (F1 借用、VERSION の rule を継承)
# 注: DR-0023 § C で compare は peerExpand=false (= F1 の path を借用)、
#     vcs source は 1 つだけ。借用元 (= VERSION) の rule を独立に継承する。
```

### Phase 1 で必要な flag

(= DR-0030 の format=regex 廃止が前提。`--format` enum は **4 値**、xml は Phase 1
スコープ外 = codex Critical C-3 反映)

- `--format <text|json|yaml|toml>`: ファイルの構造分類
  - `text` → パーサなし。`--version-regex` 必須、`--version-path` は使用不可 (= error)
  - `json|yaml|toml` → パーサで木構造化、`--version-path` 主、`--version-regex` も
    併用可 (= path 値への 2 段抽出 or 全文 regex)
  - **`xml` は Phase 1 では `--format` の対象外** (= 2026-06-03 codex 反映、Critical
    C-3): XML は JSON/YAML/TOML と木の semantics が違う (= 要素繰り返し / 属性 /
    テキストノード / 名前空間 / root anchoring) ため、共通 dot-path で扱う契約に
    無理がある。Phase 1 では XML 自作 file は対象外、Phase 2+ で別 path language
    (= XPath subset / slash-rooted path / 既存 builtin `xml-element` 仕様の踏襲) を
    別 DR で設計してから露出する。これにより、Phase 1 で曖昧な XML 契約を pin して
    後で互換破壊する事故を回避
- `--version-regex <PATTERN>`: regex で version を抽出 (capture group 1)。text 必須、
  json/yaml/toml で optional。**exact one match 期待** (= 0 / 2+ match は error、
  論点 7 確定)。line-anchored 推奨 (= `(?m)^...`、強制ではない)
- `--version-path <DOTPATH>`: 構造化 path で version を抽出。json/yaml/toml で
  使用、text では error
- `--name-regex <PATTERN>` (optional): name 抽出 (= `--version-regex` と対称、exact
  one match 期待)
- `--name-path <DOTPATH>` (optional): name 抽出 (構造化、`--version-path` と対称)
- `--define-rule <PATTERN>`: ブロックスコープを開く (1.x 節)

**CLI rule の部分継承禁止** (= 2026-06-03 nitpick 反映): CLI rule (= `--define-rule` ブロック
または global) で 1 つでも rule 系 flag を指定した時点で、その SOURCE への **builtin
継承は無し**。明示指定 flag のみで rule を構築する (= `--format` も明示必須)。これにより
「CLI rule で format だけ override して残りは builtin」のような暗黙継承が発生せず、挙動
予測が容易。

### Path / Regex 併記時の挙動 (= 2026-06-03 確定)

`--version-path` と `--version-regex` は **併記可能**。挙動は組み合わせで決まる:

| --version-path | --version-regex | 挙動 |
|---|---|---|
| あり | なし | path で取得した値 = version |
| なし | あり | **全文** に regex を適用、capture group 1 = version |
| あり | あり | **path で取得した値に regex を適用**、capture group 1 = version |
| なし | なし | error (= どちらか必須) |

3 ケース目の例: `{"name": "myapp v1.0.5"}` で `name` フィールドから version
だけ抜く:

```
bump-semver get info.json \
  --format json \
  --version-path '$.name' \
  --version-regex 'v(\d+\.\d+\.\d+)'
# → "myapp v1.0.5" を path で取得、regex で "1.0.5" を抽出
```

同じ規約を `--name-path` + `--name-regex` にも適用 (= 対称性)。

**設計判断**:
- `--version-regex` の **default は無い** (= `(.*)` 等を暗黙適用しない)。理由:
  default があると path 単独指定で意図せず全文 capture が走り、debug 困難な
  silent failure を生む。併記は明示の opt-in に限る。
- regex の capture group は **必ず group 1** で取る (DR-0012 builtin `regex` format
  と同規約)。

### Flag のスコープ規約 (= 2026-06-03 確定、nitpick 反映で補強)

`--format` / `--version-regex` / `--version-path` / `--name-regex` / `--name-path`
は **書く位置** でスコープが決まる:

| 位置 | スコープ | 例 |
|---|---|---|
| 最初の `--define-rule` より前 (or `--define-rule` 不在) | **グローバル** = 全 SOURCE のデフォルト | `... X Y --format text --version-regex R` |
| `--define-rule <PATTERN>` の後 | **ブロック** = 当該 PATTERN にマッチした SOURCE のみ。**positional SOURCE はブロックを終了しない** (= 透過、kawaz CLI 規約「positional がオプションの後にも置ける」と整合) | `... --define-rule X --format text --version-regex R 別.json --define-rule Y --format json --version-path P` (= 別.json は X block 内、Y block で次の SOURCE) |

**ブロック外の rule 系 flag は構造的に発生しない** (= 上の表 2 行で全位置を網羅、第 3 状態は無い)。
例外的に「ブロック外」が観測されるのは **`--define-rule` の typo / 抜け** (= `--definerule X`
と書いて argparse が `--definerule X` を unknown option として落とすか、`--define-rule`
そのものが書かれずに rule 系 flag だけが invocation の途中で出現する) のみ。typo 防御は
**argparse 層** (= unknown option を error) + **0a 補強規約** (= 「最初の `--define-rule` より
前のみ global flag を許す」厳密順序) で実現する。

**評価規則**:

1. 各 SOURCE につき 1 rule を選ぶ
2. SOURCE にマッチした **最も具体的なブロック** (= 一致度 tier 最大、§ 1.2 表) を 1 つ
   特定する。複数同 tier 衝突は ambiguous error (= 0c 規約)
3. 該当ブロックが存在し、そのブロック内に rule 系 flag が **1 つでも** あれば、
   そのブロックは **rule 完全宣言** とみなす:
   - `--format` を含む全 rule 系 flag を **そのブロック内のみ** で評価する
   - グローバルからも builtin からも **継承しない** (= CLI rule の部分継承禁止)
   - 必須 flag (= `--format` と `--version-path`/`--version-regex` のどちらか) が
     不足する場合は error
4. 該当ブロックが存在するが flag が空 (= 「`--define-rule X --define-rule Y --format json`」
   のような X block) の場合は **error** (= 空 block は意味なし、明示禁止)
5. 該当ブロックが存在しない場合は **グローバル rule** を評価する:
   - グローバルに rule 系 flag が 1 つでもあれば、その内容で rule 完全宣言とみなす
     (= builtin 継承なし、ブロックと同じ評価規則)
   - グローバルにも rule 系 flag が無ければ **builtin にフォールバック** (= 0d 規約、
     builtin の confidence ranking で評価)

この規則により「block で `--format` だけ書いたら global の `--version-regex` が
leak する」という曖昧解釈 (= 2026-06-03 nitpick B7) は排除される。block と global は
それぞれ独立した「rule 完全宣言」単位であり、format を境界に rule の組が暗黙融合する
ことはない。

**dead block / dead global 検出** (= 2026-06-03 2 周目 nitpick 反映):

- **dead block**: invocation 内に「どの SOURCE にもマッチしなかった `--define-rule`
  ブロック」がある場合は **error** (= ユーザが書いた `--define-rule` が silent に無視
  されると debug 困難。例: `bump-semver get a.json --define-rule b.json --format text
  --version-regex '...'` で `b.json` が SOURCE 一覧に無い)
- **dead global**: 全 SOURCE がいずれかの `--define-rule` ブロックにマッチした場合、
  グローバル rule 系 flag は使われない (= dead code)。これは **warning** で hint 出力
  (= error にしない、書き間違いと「override されない SOURCE を想定して書いた」が
  区別困難なため)。stderr に「note: global --format/--version-* flags are unused
  because every SOURCE is covered by --define-rule blocks」を出す。`--no-hint` /
  `-q` / `-qq` で抑制可能 (= 既存規約と整合)

**ブロックの終了**:

- 次の `--define-rule` で次のブロックが開く (= 前ブロック確定)
- invocation 終了で最後のブロック確定
- 明示的なブロック終了 flag (= `--define-end` 等) は **不要** (= 設計過剰)

**block 内の flag 重複**:

- 同 block 内に同じ rule 系 flag を 2 回書いた場合 (= `--format json --format text`) は
  **error** (= last-write-wins より surprise 少。意図不明)

#### 典型パターン 3 種

```
# パターン 1: グローバルのみ (= 全 SOURCE が同 rule、--define-rule 不要)
bump-semver get X Y --format text --version-regex '^version\s*=\s*"([^"]+)"'

# パターン 2: ブロックのみ (= 全 SOURCE を個別指定、グローバル無し)
bump-semver get X Y \
  --define-rule X --format text --version-regex '...' \
  --define-rule Y --format json --version-path '$.version'

# パターン 3: グローバル + 個別 override (= 大部分は同じ、例外だけ --define-rule)
bump-semver get X Y Z \
  --format json --version-path '$.version' \
  --define-rule X --format text --version-regex '...'
# Y, Z は --define-rule に該当しないので グローバル rule (json + $.version) で評価
# X は --define-rule X block 内で rule 完全宣言 (= text + regex、グローバルは継承しない)
```

**典型ユース**: パターン 1 が最頻 (= 単一ファイル / local vs remote の peer 比較)。
パターン 2/3 は複数ファイル混在で使う。パターン 3 の冗長性をオプション名で防ぐ
(`--global-format` 等) のは **誰にとっても不利益** (= cognitive load 増、
single ユースが大半なのに lookahead で正解が見えない) なので採用しない。

### dot-path 仕様 (= 構文 (ii) で `--version-path` に書ける文字列)

MVP は **最小 subset**:

```
plugin.version              # object access (= obj.plugin.version)
deps[0].version             # array index + object access
$.plugin.version            # JSONPath 風 (先頭 $ optional)
```

`jq` / JSONPath の完全互換は MVP scope 外。`.` (key) + `[N]` (index) + 先頭 `$`
の 3 要素のみ。`"key with dot"` の quoted key は Phase 2+ 検討。
yaml / toml も同じ dot-path 構文を共有 (= parse 結果を tree として扱う)。xml は
Phase 1 では `--format` 対象外 (= Phase 2+ で別 path language を別 DR で設計、codex
Critical C-3 反映)。

## 関連 DR / Issue

- DR-0001: basename 自動判定の哲学 (= 本 issue で「外す」決断ではなく「補完する」設計)
- DR-0005: path-aware confidence ranked candidates (= CLI rule の confidence 位置付け)
- DR-0010: confidence-1 fallback hint (= error UX の prior art)
- DR-0012: builtin `regex` format (= 本 issue の syntax 案 (i) の prior art)
- DR-0017: compare precision suffix (= compare との互換性)
- DR-0023: N-arg + `vcs:` borrow (= 論点 5 の peer-expand)
- DR-0027: `regex:` 却下 (= mapping syntax の話、本 issue とは別軸)

## Next action

- 残未確定論点 (1: config 形式、2: precedence、3: 構文混在、5-8: vcs/name/regex/error UX) を
  確定 → 主要部は 2026-06-03 nitpick 反映で close 済、残は config (Phase 2 別 DR scope) と
  論点 1 (= 本 DR の Phase 1 範囲外、Phase 2 で扱う)
- DR-0030 (format=regex 廃止) を先に確定 → 実装
- 本 issue を `DR-0029-cli-user-defined-rule-phase1.md` として確定 DR 起票
- Phase 1 実装 = 1 PR、Phase 2+ は別 PR で順次
- DR 確定時、コード側に Design rationale コメント必須:
  「Design rationale: --define-rule blocks deliberately make `--format`/`--version-*`/
  `--name-*` order-dependent (scope-sensitive). This is the only flag family in
  bump-semver with positional flag semantics. See DR-0029.」
- **issue → DR rename 時の注記 strip 必須** (= `feedback_no_process_noise_in_final_docs`
  ルール):
  - 「(= 2026-06-03 nitpick 反映)」「(= 2026-06-03 確定)」「(= 2026-06-03 2 周目 nitpick
    反映)」等の出自注記を一括削除
  - 軸 2 表の (iv) 取り消し線 / 「~~論点 4: ...~~」等の取り消し線を削除
  - 「確定済」/「既存の論点 (引き続き未決)」/「論点」のメタセクション構造を「Decision」
    (= 確定事項のみ) と「Phase 2+ scope」(= 残論点のうち本 DR で扱わないもの) の 2 章に
    再構成
  - draft 段階の議論プロセス (= 「軸 1: 入口の形」「軸 2: 指定構文」「案 1/案 2/案 3」
    の比較表) は削除し、採用案 (= 案 1 `--define-rule` ブロック方式) の説明に集約

## 別 issue で扱う (= 本 issue 範囲外)

- ISSUE_TEMPLATE 整備 (format request 用の form)
- CONTRIBUTING.md 整備
- 自動ラベル (`format-request` ラベルの作成)

これらは「対応してほしい」窓口側の整備で、別 issue として `docs/issue/2026-06-03-format-request-window.md` (仮) に起票。
