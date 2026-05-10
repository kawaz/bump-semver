# DR-0006: pre-release / build-metadata 対応と compare サブコマンド + FILE|VER 統合

- Status: Active
- Date: 2026-05-10
- Extends: DR-0001 (アクション数), DR-0003 (本体セパレータ規則), DR-0004 (整合性検証範囲)

## Context

ユーザから compare 機能 (`bump-semver eq FILE1 FILE2` 等) の要望。equality だけでなく順序比較 (`lt`/`gt`/`le`/`ge`) を含めるなら、pre-release を含むバージョン (`1.2.3-rc.1`) が比較対象に来るのは時間の問題。pre-release を入れる以上、build metadata と合わせて SemVer 2.0.0 構文・順序仕様にフル準拠する設計とする。

合わせて、`--value VER` の値直接指定を廃止し、位置引数 (`FILE | VER` 統合) でシンプル化する。

## Decision

### アクション構成

5 つの bump/read 系 + ネスト型 compare:

| カテゴリ | アクション | 用途 |
|---|---|---|
| bump | `major` / `minor` / `patch` | SemVer 数字 bump |
| bump | `pre` | pre-release counter advance / 設定 / 削除 |
| read | `get` | 値取得 + 整合性チェック |
| compare (ネスト) | `compare {eq\|lt\|gt\|le\|ge}` | 二項比較 |

DR-0001 の「flat 4-action」を **5-action + compare ネスト**に拡張。`compare` をネストにすることで bump/read 系のフラット性を維持。

### 横断フラグ

- `--pre PRE` / `--no-pre` (排他)
- `--build-metadata META` / `--no-build-metadata` (排他)
- `--write` (既存、bump 系のみ)

### bump 挙動 (drop デフォルト)

bump 時、`--pre` / `--build-metadata` を明示しない限り、既存の pre-release / build metadata は **drop** する。「pre は次バージョンへの WIP」という npm 流 strip-don't-bump 解釈は採用しない (Rationale 参照)。

| Input | `patch` | `pre` | `pre --pre alpha` | `pre --no-pre` |
|---|---|---|---|---|
| `1.2.3` | `1.2.4` | error: not pre-release | `1.2.3-alpha` | (nop) `1.2.3` |
| `1.2.3-rc.0` | `1.2.4` (drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |
| `1.2.3-rc1` | `1.2.4` | error: not incremental | `1.2.3-alpha` | `1.2.3` |
| `1.2.3+build` | `1.2.4` (drop) | error: not pre-release | `1.2.3-alpha` | (nop) `1.2.3` |
| `1.2.3-rc.0+build` | `1.2.4` (両 drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |

### pre アクションの詳細

- **引数なし (`pre X`)**: 既存 pre が `rc.N` 形式 (最後の識別子が数値) のときのみ counter advance、それ以外エラー
- **`--pre PRE`**: PRE 値で完全上書き (元 pre 有無問わず、巻き戻りも許容)
- **`--no-pre`**: pre 削除 (元 pre 不在でも nop)

エラー例:
- `pre 1.2.3` → `error: 1.2.3 does not have a pre-release, use --pre PRE`
- `pre 1.2.3-rc1` → `error: rc1 is not incremental, use --pre PRE`
- `pre 1.2.3-alpha` → 同上

### 比較セマンティクス

SemVer 2.0.0 順序比較準拠:

1. MAJOR/MINOR/PATCH 数値比較
2. pre-release あり < 同 base の確定版 (`1.0.0-rc.1 < 1.0.0`)
3. pre-release 同士は仕様の識別子比較 (数値識別子 < 英数字識別子、ASCII 順)
4. **build metadata は順序比較から完全に除外** (`1.0.0+a == 1.0.0+b`)
5. prefix / sep の違いは正規化して比較 (`v1.2.3` == `1.2.3` == `version_1_2_3`)

exit code: `0` = 真, `1` = 偽, `2` = エラー (`test` / `dpkg --compare-versions` 慣習)。

### 拡張 prefix / separator (DR-0003 の更新)

DR-0003 で本体セパレータに `[._-]` を許容していたが、pre-release の `-` と衝突するため**本体は `[._]` のみに絞る**。`-` セパレータはサポート対象外。

```
本体: (v|ver|version)?[._]?\d+[._]\d+\3\d+    (sep1 == sep2 を強制)
pre:  -[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*       (SemVer 仕様)
meta: \+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*      (SemVer 仕様)
```

数値のみの識別子 (本体・pre 共通) は **leading zero 禁止** (SemVer 仕様)。build metadata は leading zero 許容 (仕様)。

### FILE | VER 統合 (`--value` 廃止)

- 位置引数で受ける、semver パース可能なら値、不可ならファイル扱い
- `--value` フラグ廃止 (後方互換破壊、major bump 候補)
- `1.2.3` 名のファイルがあるカレントなら `./1.2.3` で明示 (Unix 慣習)
- 複数引数でファイル/値混在可能 (`get Cargo.toml package.json 1.2.3` で 3 者一致確認)
- `-` (単一引数) で stdin から VER 読込

### 出力時の prefix/sep 保持

入力にあった prefix と sep は出力で**保持**される。strict semver ユーザに影響なし (`1.2.3` を入れたら `1.2.4` が出る)、kawaz 拡張ユーザにも影響なし (`v_1.2.3` を入れたら `v_1.2.4` が出る)。

## Rationale

### 不採用案と理由

**1. npm 流 strip-don't-bump (`patch 1.2.3-rc.0 → 1.2.3`)**

業界慣習だが「pre が attach されているときだけ patch の挙動が変わる」という暗黙ルールが生まれる。bump-semver は「断定的・予測可能」哲学で、**内部一貫性を優先**。`patch` は常に patch を上げる、`--pre`/`--build-metadata` 明示なければ drop、という単一規則で済む。npm 流ユーザの混乱は README で明記して回避。

**2. `release` verb 専用追加**

確定昇格 (`1.2.3-rc.0 → 1.2.3`) のため独立 verb を作る案。**不採用**: `--no-pre` フラグで横断的に対応できる、`--feature/--no-feature` ペアの CLI 慣習に乗れる、アクション数を抑えられる。

**3. `pre 1.2.3-rc.0 → 1.2.4-rc.1` (patch 自動 bump)**

「`rc.0` と `rc.1` は別バージョンだから patch を上げるべき」哲学だが、SemVer の標準解釈は **`1.2.3-rc.N` は `1.2.3` への WIP イテレーション**。患者 bump すると「リリース対象が次々ズレていく」ワークフロー崩壊が起きる。**不採用**: `pre 1.2.3-rc.0 → 1.2.3-rc.1` (counter advance のみ)。両方上げたければ `patch X --pre rc.0` を明示。

**4. `pre=-rc1` 構文 (アクション名に `=`)**

CLI 慣習からの逸脱が大きすぎる。シェル補完が壊れる、引数パーサで特殊扱い必要、先例なし (調査済み)。**不採用**: `--pre rc.0` の標準フラグ形式。

**5. `--value` 維持 + `FILE | VER` 統合の両立**

後方互換のため両方残す案。**不採用**: 引数パースが複雑になり、サンプル記述も嘘っぽくなる。`./1.2.3` で曖昧さ回避できる以上、`--value` を残す価値が薄い。

**6. 本体セパレータ `-` の維持 (DR-0003)**

`1-2-3` 形式の本体セパレータ `-` を許容していたが、pre-release の `-` と構文上衝突する。**不採用**: `[._]` のみに絞る。実害は少ない (kawaz 自身も `-` セパレータの実用例を挙げられない)。

### 既存ツール調査の要点

- **npm `node-semver` `inc()` がデファクト**: poetry / Rust `semver` crate / hatch / cz bump も準拠
- **strip-don't-bump 流が圧倒的多数派** だが、これは「pre は WIP」解釈に基づく。本リポは内部一貫性を優先して逸脱
- **build metadata は全ツール drop**、保持するツールはゼロ
- **比較 IF は dpkg 形式 (`v1 op v2`, exit 0/1) のみ確立**
- **`sort -V` は SemVer 順ではない** (`1.0.0-rc.1` が `1.0.0` の後にソートされる、実証済み)

## Consequences

### 既存 DR との関係

- **DR-0001 (flat 4-action)**: 5-action + compare ネストに拡張。flat 哲学は bump/read 系で維持
- **DR-0003 (prefix and flexible separator)**: 本体セパレータから `-` を外し `[._]` のみに。pre-release/build metadata のセパレータは SemVer 仕様準拠を追加
- **DR-0004 (multi-file consistency)**: 整合性検証の対象を「version 全体 (prefix + base + pre + metadata)」に拡張。値とファイル混在に対応
- **DR-0005 (path-aware confidence ranked)**: 影響なし

### 後方互換破壊点 (major bump 必要)

- **`--value` フラグ廃止**: `--value 1.2.3` を使っているシェルスクリプトは `1.2.3` 直接指定に書き換え
- **本体セパレータ `-` 不許可**: `1-2-3` 形式は不可になる (`1.2.3` または `1_2_3`)

これらは `v0.5.0` (or `v1.0.0`) で導入。CHANGELOG / README に移行ガイドを明記。

### スコープ外 (将来検討)

- **本体への pre-release/build-metadata の `_` セパレータ拡張**: `version_1_2_3-rc.1` は対応するが、`version_1_2_3_rc_1` のような全体 `_` 統一は対応しない
- **pre-release のラベル昇格 (alpha → beta → rc → stable)**: poetry `--next-phase` 相当。需要が出れば追加
- **`compare` 以外の比較系**: `sort` (複数 VER のソート) や `valid` (パース可能性チェック) は将来検討

### 参考

- SemVer 2.0.0: https://semver.org/spec/v2.0.0.html
- 議論経緯: docs/journal/2026-05-10-pre-release-and-compare-design.md
- 実装計画: (作業中)
