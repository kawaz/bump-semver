# DR-0023: `get` / `compare` の N 引数化 + `vcs:` borrowing の N 個展開

- Status: Active
- Date: 2026-05-31
- Extends: DR-0006 (compare サブコマンド + FILE|VER 統合), DR-0008 (vcs: 入力モード)
- Related: DR-0007 (--json 出力), DR-0017 (compare precision suffix)

## Context

DR-0006 で導入した `compare OP A B` は 2 入力固定。DR-0008 で導入した
`vcs:REV` borrowing は「兄弟入力から最初の FILE 1 つを借りる」だった。
実運用 (= CI / Justfile) で以下の不便が判明:

1. **複数 OTHER との比較を 1 invocation で書きたい**
   - 「main より上 **かつ** 直前 tag より上」を表したいが、現状は 2 回 `compare gt` を AND する必要がある (shell 側で `&&`)
   - 1 個でも false が出れば exit 1 を返してほしい、stderr で「どれが落ちたか」全部知りたい
2. **複数 FILE の整合性を VCS スナップショットと一括検証したい**
   - `get a b vcs:main@origin` で「a, b, main@origin:a, main@origin:b の 4 source が全部同値か」を 1 行で書きたい
   - 現状の borrow は path 1 つ固定なので `vcs:main@origin:a` 1 source しか展開されない
3. **既存 `compare OP A B` / `get FILE...` との完全互換維持**
   - 古いスクリプトを壊さない範囲で表現力を上げたい

`get` と `compare` の使われ方は対称ではないので、「N 引数化」とまとめても
責務分離は verb 別に違う:

| verb | source 関係 | 不一致時の意味 |
|---|---|---|
| `get` | **全対等** (正解 source なし) | 「全部一致しているか」のピア検証 |
| `compare` | **F1 基準 + N OTHERS** | 「BASE OP 各 OTHER がすべて成立するか」の述語検証 |

## Decision

### A. compare の N 引数化

```
bump-semver compare <OP> <BASE> <OTHER...>      # OTHER 1 個以上
```

- `BASE` = 第 1 引数 (= 基準値、F1)
- `OTHER...` = 第 2 引数以降 (= 比較対象、1 個以上)
- 既存 `compare OP A B` は N=1 の互換ケース

**全評価方式** (短絡禁止): 全 OTHER を評価し、失敗ペアを stderr に列挙してから exit 1 を返す。1 invocation で「どこが落ちたか」が全部見える。

**Exit code**:

- 0: 全 OTHER で predicate 成立
- 1: 1 個以上の OTHER で predicate 不成立 (= 失敗ペアの per-line 詳細を stderr)
- 2: エラー (parse 失敗 / 入力不正 / VCS 失敗等)

**Stderr フォーマット** (失敗時、1 OTHER につき 1 行):

```
compare <OP>: <BASE_LABEL> (<BASE_VALUE>) is not <OP_PHRASE> O<N>=<OTHER_LABEL> (<OTHER_VALUE>)
```

例:

```
compare gt: VERSION (0.26.3) is not greater than O1=vcs:main@origin (0.27.0)
compare gt: VERSION (0.26.3) is not greater than O2=vcs:v1.0.0 (1.0.0)
```

OP_PHRASE は OP + precision 連動:

| OP | 文言 |
|---|---|
| `eq` | `not equal to` |
| `lt` | `not less than` |
| `le` | `not less than or equal to` |
| `gt` | `not greater than` |
| `ge` | `not greater than or equal to` |

precision suffix (DR-0017) 付きは末尾に `(major)` / `(minor)` / `(patch)` を追加 (例: `not equal to (major)`)。

**Quiet flags**:

- `-q` / `--no-hint`: per-OTHER 失敗 listing は **保持**。DR-0010 hint のみ抑制
- `-qq` / `--quiet-all`: per-OTHER 失敗 listing も抑制 (= "quiet-all はあらゆる診断を抑制" 規約に従う)

### B. get の N 引数化 (verb-specific stderr)

`get` の入力は既に可変長だったが、不一致時の意味付けを compare と揃えた:

- **全 source 対等** (= 正解 source なし、ピア検証)
- 全 source 同値 → exit 0、stdout に value 1 つ
- 不一致 → **exit 1** + stderr に `version mismatch:` / `name mismatch:` カラム整列リスト (= 既存 `formatMismatchError` の文字列をそのまま再利用)
- stdout は常に空 (= 不一致時に「どれを採用したか」勝手に出さない)

**version mismatch と name mismatch は同じ規約** (follow-up #35 で訂正): 当初は version 不一致のみ exit 1、name 不一致は exit 2 (= 旧 emitErr 経路) だったが、「全 source 対等のピア検証」という get の責務はどちらの fields でも同じなので、name mismatch も exit 1 + stderr listing に揃えた。bump 系の name mismatch は引き続き exit 2 (= 書き込み拒否)。

**注意**: bump 系 (`major` / `minor` / `patch` / `pre`) は引き続き **exit 2** で diagnostic は `bump-semver: version mismatch:` / `bump-semver: name mismatch:` prefix 付き (= 内部不整合で動作拒否、ユーザに修正を促す意味付け)。get だけ exit code 規約を分離。

**Quiet flags**:

- `-q` / `--no-hint`: per-source listing は保持
- `-qq`: per-source listing も抑制 (compare と同じルール)

### C. borrowing の N 個展開 (verb 別モード)

`resolveInputs` に `peerExpand bool` パラメータを追加し、verb 別に切り替え:

| verb | `peerExpand` | borrow 挙動 |
|---|---|---|
| `compare` | `false` | FILE 省略 `vcs:REV` は **F1 (BASE) の path を借用** (1 OTHER につき 1 source)。今までの「最初の FILE 提供入力」と一致 |
| `get` / bump 系 | `true` | FILE 省略 `vcs:REV` は **兄弟 FILE 全 path にピア展開** (1 個の `vcs:REV` が path 数だけ source を生成) |

borrow source 候補は両モード共通 ([DR-0008](./DR-0008-vcs-input.md) と同じ):

- 実 FILE 起源の入力 (`Cargo.toml`)
- `vcs:REV:FILE` 形式 (file を明示している vcs: 入力)

dedup は位置順 (= 同じ path が複数回現れても 1 回だけ展開対象に追加)。

**具体例** (peer-expand):

```
get a b vcs:main@origin
# = a, b, vcs:main@origin:a, vcs:main@origin:b の 4 source 評価

get a vcs:HEAD~1:b vcs:main
# sibling 候補は {a, b} (b は HEAD~1 の file-explicit から提供)
# = a, vcs:HEAD~1:b, vcs:main:a, vcs:main:b の 4 source 評価

get vcs:main vcs:v1
# sibling 候補は空 → error: vcs: file is required (no file argument to borrow from)
```

**Stderr label の具体化** (follow-up #35): peer-expand で展開された各 vcs source は、不一致時の stderr listing でも borrow 先の FILE を含めて `vcs:HEAD:VERSION` / `vcs:HEAD:b.json` のように区別表示される。当初は両 source とも `vcs:HEAD` のみ表示されてどちらが何の borrow か識別できなかったが、`resolveInputs` の peer-expand 分岐で `originFile = "vcs:REV:FILE"` (= 既存の vcs spec 形) に置換することで解決。spec 形なので人間にもツール (vcsParseSpec) にも自然な表記。

**具体例** (compare の F1 借用、変更なし):

```
compare gt VERSION vcs:main vcs:v1.0.0
# = VERSION (BASE) vs vcs:main:VERSION (O1), VERSION vs vcs:v1.0.0:VERSION (O2)
# = 2 比較を full-eval、両方 true なら exit 0
```

## Rationale

### なぜ get と compare で borrow 挙動を分けるか

`get` は全 source 対等。`get a b vcs:main` で「main の a と main の b を一切無視して main の "1 つだけ" を見る」のは意味不明 (= ユーザ意図は「a と b と main の状態が全部揃ってるか」)。よって sibling 数だけ peer-expand する。

`compare` は F1 基準。`compare gt VERSION vcs:main vcs:v1.0.0` で `vcs:main` と `vcs:v1.0.0` が各々 2 path に膨らむと「VERSION vs (vcs:main:VERSION, vcs:main:b.json, ...) 4 比較」になり、ユーザ意図 (= 「VERSION が main と v1.0.0 の両方より上か」) から逸脱する。F1 借用が正しい。

### なぜ compare は全評価 (短絡禁止) か

CI で「main より上 **かつ** 直前 tag より上」を assert したいケース、失敗時に「どっちが落ちたか」両方知りたい (1 個目だけ報告で止まると問題箇所の特定に手間がかかる)。1 invocation で全部の失敗を集約できる方が UX が良い。

性能影響は無視できる (= OTHER ごとに 1 ファイル取得、せいぜい数個)。

### なぜ get mismatch を exit 1 に変えたか

旧来は `formatMismatchError` 経由で exit 2 (= 利用側からは「何かエラー」)。N 引数化で「全 source 対等のピア検証」が verb の主目的になったので、**predicate-false** (= 値が揃ってない) の意味付けが正しい。exit 1 にすることで shell `if bump-semver get ...; then ok; else mismatch; fi` の素直なパターンが書ける。

bump 系 (`major` / `minor` / `patch` / `pre`) は別: 値が揃ってない状態で書き込みを開始するのは破壊的なので exit 2 (= 動作拒否) のままが正しい。verb 別に exit code を分けることで意味付けが明確になる。

### なぜ stderr に出すか

「sourceless quiet exit 1」は debug 不能。compare false が長らく silent だったのは「2 入力で値が見れば自明」だったから。N OTHER だと「どの OTHER が落ちたか」が値だけでは分からない。stderr に詳細を出すのが正しい。`-qq` で抑制できるので「シェルが exit code だけ使う」用途も維持できる。

## Alternatives (不採用)

- **compare に `--all` / `--any` フラグ**: 「全部成立 vs どれか成立」を切り替える案。**不採用**: complexity 増、ユースケース「全部成立」が圧倒的に多い、欲しけりゃ `||` で shell 側で組める
- **compare 短絡方式**: 1 個目の失敗で即 stderr + return。**不採用**: 失敗集約のメリットを失う
- **get の不一致を exit 2 のまま維持**: 旧来互換重視。**不採用**: N 引数化したのに predicate 系の意味付けが verb で揃わないのは設計汚染
- **peer-expand を get/bump で挙動差**: get だけ peer-expand、bump は単一 borrow。**不採用**: `--write` + `vcs:` は既に rejection されているので「read-only bump」と get で挙動を変える正当な理由がない、むしろ揃える方が予測可能

## Implementation Notes

- `src/main.go` `resolveInputs(inputs, stdin, write, vcsOverride, peerExpand bool)`:
  - `borrowFiles []string` を deduped 位置順で収集 (= 実 FILE + `vcs:REV:FILE` の FILE)
  - `peerExpand=true && spec が file-omitted vcs:REV && len(borrowFiles)>1` のとき、borrow ごとに `resolveInput` を呼んで複数 `resolvedInput` を出す
  - それ以外は従来通り `fileForBorrow` (= borrowFiles[0]) を 1 つ渡す
- `src/main.go` runBump (= get / bump 共用): mismatch 時 `args.action == "get"` なら `&exitErr{code: exitCodeFalse}` + stderr listing、それ以外は従来通り `emitErr` (= exit 2)
- `src/compare.go` runCompare: `len(args.inputs) >= 2` を validate、F1 を collapse、`for i := 1; i < len(resolved); i++` で各 OTHER を評価、失敗を `failures []string` に貯めて最後にまとめて出力 + exit 1
- `formatCompareFailure(args, otherIdx, baseRI, base, otherRI, other) string` で stderr 1 行を組み立て
- `compareOpPhrase(op, precision) string` で OP 文言 mapping

## 関連

- [DR-0006](./DR-0006-pre-release-and-compare.md) — compare サブコマンド (本 DR の前提)
- [DR-0007](./DR-0007-json-output-option.md) — compare は `--json` を拒否 (= 述語専用)、本 DR でも維持
- [DR-0008](./DR-0008-vcs-input.md) — borrowing の基盤、本 DR で peer-expand を追加
- [DR-0017](./DR-0017-compare-precision-suffix.md) — precision suffix。stderr 文言で precision を反映
