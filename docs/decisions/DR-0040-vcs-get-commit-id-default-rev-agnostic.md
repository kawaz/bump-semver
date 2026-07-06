# DR-0040: `vcs get commit-id` のデフォルト rev を backend-agnostic 化 (jj: `@` → 最新の固定コミット)

- Status: Accepted (2026-07-06)
- Date: 2026-07-06
- Related: DR-0031 (translate-rev-common-foundation, default rev 節を本 DR が supersede), `docs/issue/2026-07-06-commit-id-jj-default-rev-points-to-working-copy.md`

## Context

`vcs get commit-id` は agnostic API を謳いながら、`--rev` 省略時のデフォルトが backend ごとに異なる概念を指していた:

- git `HEAD` = 最後に**固定された**コミット (作業ツリーの未コミット変更を含まない)
- jj `@` = mutable な working-copy コミット (空のことも、未コミット作業を抱えることもある)

git `HEAD` の jj 対応物は `@-` (直前に固定されたコミット) であり `@` ではない。gh-monitor の push 後 CI watch (SHA pin) 用途でこれが露見した: `@` が空コミットのとき、default は実 push 対象と異なる SHA を返し、CI run が存在せず watcher が no-match-timeout まで無駄常駐する退行になった。

## Decision

### jj backend のデフォルト rev を `heads((::@-) & (~empty() | merges()))` に変更

git backend の `HEAD` は変更しない (既に「最後に固定されたコミット」を指しており、agnostic な意味論と一致している)。

素直な `@-` 案では不十分: `@-` 自体が空コミット (`jj new` 直後で describe/edit していない) のことがあり、その場合 `@-` は「実体のない」コミットを返してしまう。`heads((::@-) & (~empty() | merges()))` は `@-` を起点に祖先集合を辿り、空でない最新の実体コミットを探す。空マージ (親 ≥ 2、ファイル変更なし) は `merges()` で救済する — `~empty()` だけだと空マージが除外され、祖先集合内に複数の head (マージの各親) が残って ambiguous (該当なし) になる。

### 実機検証 (jj 0.42.0)

5 シナリオで実測、期待通りに動作した:

| シナリオ | `@-` の実体 | 素直な `@-` | 本 revset |
|---|---|---|---|
| 通常 (`@-` 固定済み・非空) | 非空 | `@-` と一致 | `@-` と一致 |
| `@` のみ空 (`@-` は非空) | 非空 | `@-` と一致 | `@-` と一致 |
| `@-` 自体が空コミット | 空 | 空コミットを返す (退行) | 直前の非空コミットまで遡る |
| 空マージ (親 2、変更なし) | 空・マージ | 空コミットを返す (退行) | マージ自身を返す (`~empty()` 単体だと親 2 つに割れ ambiguous) |
| evil merge (マージ + 追加編集) | 非空・マージ | `@-` と一致 | `@-` と一致 (`merges()` 救済は no-op) |

### 不採用にした条件: description gate

「空マージだけを救うために `merges() & description あり` を条件に足す」案を検討したが不採用。push される merge が description を持つ保証がない (`jj new a b` して describe せず push できる) ため、無記述だが実在の push 対象 merge を取りこぼし、元の退行を再発させる。取りこぼす対象 (無記述 merge が `@-` になるケース) はその merge SHA が実在し push されていれば CI も走るため、pin して実害がない。構造的な `merges()` (親の数) の方が意味的な description 有無より堅牢。

## Consequences

- 影響範囲は kawaz プロジェクト群のみ
- 旧挙動 (`@` を取得) が必要なら `--rev @` を明示指定すれば残せる (後方互換の逃げ道あり)
- help / godoc / flag 文言 (`cobra_help_text.go` / `cli_types.go` / `cobra_vcs.go`) を「@ for jj / HEAD for git」から「両 backend で最新の固定コミット」に統一

## Related

- [DR-0031](./DR-0031-translate-rev-common-foundation.md) — 「default rev: jj backend `@`」節を本 DR が supersede (rev 翻訳の共通基盤自体は不変)
- `docs/issue/2026-07-06-commit-id-jj-default-rev-points-to-working-copy.md` — 起票元、両案の検討経緯
