# 2026-05-10 pre-release / compare サブコマンド設計議論

DR-0006 の設計議論経緯。最終的な決定事項は DR-0006 にまとまっているので、本ファイルは**転換点で何を考えたか**を残す目的。

## 起点: compare 要望

ユーザ提案:

```
bump-semver eq FILE1 FILE2
bump-semver eq FILE1 < FILE2
bump-semver eq FILE1 --value VER2
```

`eq/lt/gt/le/ge` を入れたい、と。実用的な動機は `bump-semver eq Cargo.toml < <(jj file show Cargo.toml -r main@origin)` のような「main からズレてないか」CI チェック。

## 転換点 1: アクションの性質が混在する問題

最初に引っかかったのは「flat 4-action」哲学 (DR-0001) との相性。`major/minor/patch/get` は状態変換と抽出だが、`eq/lt/gt` は二項述語で性質が違う。トップレベルで混ぜるとアクション数が 4→9 に倍増し、「断定的・予測可能」のポジショニングが崩れる。

→ `compare {eq|lt|gt|le|ge}` のネスト型に決定。bump/read 系のフラット性を維持。

## 転換点 2: pre-release を入れるか

`compare` だけ入れて pre-release は入れない方向もありうるが、`1.0.0-rc.1 < 1.0.0` のような順序比較は SemVer 仕様の核心部分。`compare` を仕様準拠にするなら pre-release / build metadata 対応も必要。

ユーザ判断: 全アクションで pre-release 対応。

## 転換点 3: bump 時の pre 挙動 (最大の論点)

`bump-semver patch 1.2.3-rc.0` の挙動候補:

| 解釈 | 結果 | 採用 |
|---|---|---|
| A: pre 確定昇格 (npm 流 strip-don't-bump) | `1.2.3` | ✗ |
| B: pre drop + patch bump (内部一貫性) | `1.2.4` | **採用** |
| C: pre 保持 (`1.2.4-rc.0`) | — | 推奨せず |
| D: pre が混じったらエラー | — | 保守的 |

既存ツール調査結果 (sub-agent で 12 ツール調べた):

- npm `node-semver` `inc()` がデファクト → 案 A (`1.2.3`)
- poetry / Rust `semver` crate / hatch / cz bump (semver2) も同じ
- 採用したツールゼロ: 案 B
- 業界はほぼ全部 A 流

迷いどころ: **A は業界標準だが「pre が attach されてるときだけ patch の挙動が変わる」という暗黙ルール**を生む。bump-semver は「断定的・予測可能」哲学。

ユーザ判断: **B 採用**。`patch X` は常に patch を上げる、`--pre`/`--build-metadata` 明示なければ drop、という単一規則で押し切る。npm 流ユーザの混乱は README で明記して回避。

## 転換点 4: pre アクションの自動 patch bump

ユーザが「`pre 1.2.3-rc.0 → 1.2.4-rc.1` (patch 自動 bump) が意味論的に正しいのでは」と提案。

私の指摘:
- 業界標準は `1.2.3-rc.0 → 1.2.3-rc.1` (patch 不変)
- SemVer 解釈では `rc.N` は「`1.2.3` への WIP イテレーション」
- 自動 patch bump だと `rc.1 = 1.2.4-rc.1`、`rc.2 = 1.2.5-rc.2` とズレ続け、ワークフロー崩壊
- 採用案 (`patch X` で drop+bump) と組み合わせると、`patch 1.2.3-rc.0` が strip 流になり**非対称性**が出る

ユーザ判断: 撤回。**`pre X` は counter advance のみ、patch 不変**。両方上げたければ `patch X --pre rc.0` で明示。

## 転換点 5: `release` verb vs `--no-pre` フラグ

確定昇格 (`1.2.3-rc.0 → 1.2.3`) の動線設計。

私の最初の提案: **`--no-pre` / `--no-build-metadata` フラグ**で横断的対応、`release` verb は作らない。

```
bump-semver pre 1.2.3-rc.0 --no-pre   → 1.2.3
```

途中で気づいた問題: **`pre --no-pre` は語義矛盾**。「pre アクションで pre を消す」は意味的に変。

私が撤回提案: `release` verb 復活。

ユーザ判断: **`--no-pre` 採用、`release` 不要**。「`--feature/--no-feature` ペアは CLI 慣習として広く認知されてる」と。確かに `--color/--no-color` 等の前例多数で、語義矛盾と感じるのは私の感覚過敏だった。

教訓: CLI 慣習を過小評価しない。

## 転換点 6: FILE | VER 統合 (`--value` 廃止)

サンプル書きながらユーザが気づいた: `--value VER` と `FILE` の二系統がドキュメント上扱いづらい。

```bash
# Before
bump-semver patch --value 1.2.3
bump-semver patch Cargo.toml

# After
bump-semver patch 1.2.3
bump-semver patch Cargo.toml
```

`1.2.3` というファイル名と曖昧になるが、Unix 慣習で `./1.2.3` で明示すれば回避可。

副次効果: 複数引数で値とファイル混在 OK にすると `bump-semver get Cargo.toml package.json 1.2.3` で 3 者一致確認が書ける (CI 用途で便利)。

ユーザ判断: **`--value` 廃止、位置引数で `FILE | VER` 統合**。後方互換破壊だが、シンプルさ優先。

## 転換点 7: pre 識別子記法 (`rc1` vs `rc.1`)

SemVer 仕様上どちらも valid:
- `rc1` は単一識別子 (英数字、ASCII 比較)
- `rc.1` は 2 識別子 (`rc` + `1`、後者は数値比較)

ユーザの最初の提案 `pre=-rc1` 構文 (アクション名に `=` を入れる) は**外れすぎ**で却下。`--pre rc.0` の標準フラグ形式に。

`pre 1.2.3-rc1` (counter advance 試行) はエラー: 単一識別子 `rc1` から数字を取り出して bump するのはヒューリスティックすぎる。明示的に `--pre` 必須。

`pre 1.2.3-rc.0` (counter advance) のみ OK: 末尾識別子が数値なら `+1` する。

## 転換点 8: 本体セパレータ `-` の廃止

DR-0003 で本体セパレータに `[._-]` を許容していたが、pre-release の `-` と衝突する:

```
1-2-3-rc.1   ← 本体 sep `-` か pre-release `-` か曖昧
```

ユーザ判断: 本体は `[._]` のみに絞る。`-` セパレータは廃止 (実用例なし、kawaz 自身も使ってない)。後方互換破壊だが影響軽微。

## 転換点 9: テスト戦略 (テーブル fixture vs 個別 unit test)

ユーザ案: 議論で書いたテーブル (input → expected) をそのまま fixture テストに転用。

私が提案: テーブル fixture は「**仕様書としてのテスト** (spec-as-test)」、個別 unit test は「**特定の境界値・分岐の回帰検出**」、目的が違うので**両方作る**。

仕様変更時、テーブルは「議論の結果が裏切られないか」即検証できる。個別 unit test は失敗時に「どの条件で破綻したか」即特定できる。重複コストは小さく価値が大きい。

ユーザ判断: 賛成、両立。

## 設計上のハマり所メモ

### 1. `pre` アクションの引数なし vs `--pre` 付きの意味

```
pre X              # pre counter advance (要 pre 存在 + rc.N 形式)
pre X --pre PRE    # PRE 値で完全上書き (pre 存在不要)
pre X --no-pre     # pre 削除 (pre 存在不要)
```

同じアクションで挙動が分岐する。実装時にパーサで明確に分ける必要あり。エラーメッセージは丁寧に:

- `pre 1.2.3` → `error: 1.2.3 does not have a pre-release, use --pre PRE`
- `pre 1.2.3-rc1` → `error: rc1 is not incremental, use --pre PRE`
- `pre 1.2.3-alpha` → 同上

### 2. `pre 1.2.3-rc.0+build` の build metadata 扱い

判断 (DR-0006): pre アクションでも build metadata は drop デフォルト。

```
pre 1.2.3-rc.0+build                       → 1.2.3-rc.1   (build drop)
pre 1.2.3-rc.0+build --build-metadata new  → 1.2.3-rc.1+new
pre 1.2.3-rc.0+build --no-build-metadata   → 1.2.3-rc.1   (明示 drop、デフォルトと同じ)
```

判断 3 (drop デフォルト) を全 bump 系で一貫させた結果。

### 3. compare で値とファイル混在

```
bump-semver compare eq Cargo.toml 1.2.3
```

このとき "Cargo.toml の version が 1.2.3 と一致するか" になるが、Cargo.toml 側に prefix `v` が付いてたら? → DR-0006 通り正規化して比較。`v1.2.3` == `1.2.3` で真。

### 4. `sort -V` は SemVer 順ではない

実証 (sub-agent 調査):

```
$ printf '1.0.0\n1.0.0-rc.1\n1.0.0-alpha\n' | sort -V
1.0.0
1.0.0-alpha     ← pre-release が stable の後にソート (SemVer と逆)
1.0.0-rc.1
```

`sort -V` の代わりに使えるツールとして `bump-semver compare` が役立つ。

## 残された宿題

- 実装計画書 plan.md (Plan agent で策定中)
- codex レビュー受ける
- 実装本番

DR-0006 で書いたスコープ外項目:
- pre-release のラベル昇格 (alpha → beta → rc → stable)
- 複数 VER ソート (sort サブコマンド)
- 値の妥当性チェック (valid サブコマンド)

これらは需要が出てから追加。
