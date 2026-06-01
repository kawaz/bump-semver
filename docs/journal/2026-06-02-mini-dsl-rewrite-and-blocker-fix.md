# 2026-06-02 — mini-DSL rewrite + release blocker 集約 fix

## 経緯

DR-0027 で `vcs outdated` の mini-DSL を採用し commit `103ece79` (= mvp v1) で land。直後のマルチペルソナレビュー + stop hook で 4 件の release blocker + /simplify 集約 cleanup が浮上。

並行して kawaz が「glob-backref を独立 OSS spec として切り出すべきだ」と方針提示、`docs/specs/glob-backref-v0.1.0.md` を起草。PoC v2 (`e269358e`) を `src/glob_backref_v2.go` として並走実装し、spec 準拠 feasibility を確認。

統合フェーズ (= 本作業) で:

1. PoC v2 を本実装に昇格 (= v1 削除、v2 → canonical rename + identifier simplify)
2. cmd 層を v2 API に rewrite
3. release blocker 4 件を統合 fix
4. /simplify 集約 cleanup
5. DR-0028 起票 + journal 残し

を 1 つの統合 commit に整理した。

## spec v0.1.0 採用判断

kawaz 設計の核 (= `{}` を要素数 1 まで全展開 + scoped backref index) で、v1 が抱えていた 4 構造的問題が **設計レベルで根本対応**できることを PoC で確認:

| v1 の問題 | v0.1.0 spec の対応 |
|---|---|
| leading-slash bug (`${1}/foo` → `/foo`) | `**` 0-segment = `.` + `path.Clean` (§2.2.2 / §3.5) |
| `*`/`[]` 空文字 silent skip | grammar drift = **panic** (§2.2.3 / §7) |
| TO captured value 再 glob 解釈 | char-class wrap escape (§3.4.2) |
| literal-FROM 不在 silent green CI | `--strict` flag で exit 1 (= 互換性のため default は warn) |

regex 経路は spec §1.1 で却下、`{}` AST 全展開 + doublestar 委譲の 2 段構成に統一。

## v1 → v2 統合の実装ハマり所

### advisor 指摘 #1: gitignore 漏れ

PoC v2 の `walkOne` は doublestar 直接呼出しで `OptionsV2.Gitignored` を読まないデッドフィールド。PoC test 18 件は `.gitignore` を作らないので gap を見逃していたが、cmd test `TestRun_VcsOutdated_GlobFlagsApply` が `--glob-gitignored` の default respect + 切替を明示 pin している。

→ **解決**: `walkOne` は既存 `expandGlob` (DR-0024) に委譲。`globOpts` を spec 言うところの `Options` の代わりにそのまま受け取り、`OptionsV2` 構造を削除。これで dotfile / gitignored / ignorecase が一括で uniform に。

### advisor 指摘 #2: captures index 規約反転

v1: `MatchedPath.Captures[0]` = `$1`。
v2: `Match.Captures[0]` = `$0` (= full path)、`[1]` = `$1`。

→ **解決**: `Substitute` (旧 `SubstituteV2`) の `lookupCap` は index = N で直接 access (= `$0` 含めて自然)。cmd 層は `m.Captures` を slice せず whole で渡す。

### advisor 指摘 #3: FROM-brace は ExpandPairs に通さない

spec §4.1: FROM の `{}` は `$N` slot を 1 つ消費する (= T4 で `**/*.{jpg,webp}` → `$3="jpg"`)。`MatchCollect` の内部 `expandConcrete` は brace slot を `slotBinding{isLiteral:true}` で保持するため OK。

しかし `ExpandPairs`→`braceLiteralExpansions` は brace を literal にしてしまい slot binding を失う。

→ **解決**: cmd 層は FROM をそのまま `MatchCollect` に渡す (= brace handling は内部完結)。`ExpandPairs` は TO 側の `{}` mandatory 展開専用に使う (= `derivedRowsFor` で TO に対してのみ呼ぶ)。`TestRun_VcsOutdated_FromBraceCapture` で end-to-end pin。

### advisor 指摘 #5: literal vs glob 区別を `--strict` 判定に thread

旧 cmd は `parseGlobSpec` で `glob:` を剥がした後、literal/glob の差を失う (= `len(sources)==0` で一律 vacuously true)。`--strict` は **literal** FROM 不在のみ exit 1 にすべき (= glob: 経路の no-match は仕様上「nothing to check」が正しい)。

→ **解決**: `stripGlobPrefix` (= helper、`hasGlobPrefix`+`parseGlobSpec` 2 重を /simplify 集約) が `(body, wasLiteral, err)` を返す。`evaluateOutdatedPair` の return shape を `([]rows, literalMissed string, err)` に拡張、cmd top level が `literalMisses` を集約。

## release blocker 4 件の最終解消

1. **leading-slash** — spec §2.2.2 (`**` 0-segment = `.`) + §3.5 (`path.Clean`)。test T5/T18 + cmd test `TestRun_VcsOutdated_LeadingSlashDogfood`
2. **exit code misclassification** — `errors.As(perr, &ee)` で wrap chain を walk (旧 `perr.(*exitErr)` は `fmt.Errorf("... %w", terr)` で box された後の assertion miss)
3. **silent green CI on typo'd literal FROM** — `--strict` flag 新設。default は warn + exit 0 (= 後方互換)、`--strict` で exit 1。glob: no-match は `--strict` 対象外
4. **TO 側 backref captures の再 glob 解釈** — `Substitute` 内 `glob:` 検出 → `classWrapEscape` 自動適用。test T16 + runtime invariant `panic` で leak 検出

## /simplify 集約 cleanup

- `predicateLine` + `explainStatus` → 1 `formatRow(row, mode)` に collapse。`commit(s)` 統一 (= 旧 `commits` / `commit` 不整合解消)、`untracked:` / `untracked,` の差解消
- `hasGlobPrefix` + `parseGlobSpec` 2 重呼び出し → `stripGlobPrefix` helper に集約 (= literal-vs-glob 判定もここで返す、`--strict` thread に再利用)
- `expandBraces` → `ExpandPairs` 経由で消費
- `collapseSlashes` → `path.Clean` で吸収
- inline doc-comment 重複 (= `glob_backref_v2.go` 冒頭 19 行 + `cmd_vcs_outdated.go` 冒頭 26 行) を spec/DR pointer のみに圧縮 (= DR-0025 指摘 pattern の再発防止)

## マルチペルソナ low priority 対応

- `?` を help "Capture rules" に追記 (= 「match single char, NOT captured」明示)
- cross-source 自動除外 gap は spec §6 で v0.2 送り、本実装は per-source のみ、help で明示
- bare verb `vcs outdated` (引数なし) → 既存 cli_parse の `helpAction` ルーティングが拾うので変更なし。help 文言は「exit 0 / no args → help」を明示

## 残課題 (= v0.2 以降)

spec §14 / §16 の open questions、特に:

- `walkConcurrency > 1` の cancel propagation (Rust / TS 実装時)
- `{name:pattern}` named capture (= `$N` 番号管理が破綻する複雑パターン用)
- cross-source 自動除外
- glob walk dedup (= 同一 root の重複 walk 排除)
- pattern compile cache

これらは需要発生時に v0.2 spec で扱う。bump-semver 本体は v0.1.0 規模で十分。

## 整理した commit

`feat(vcs): add 'vcs outdated' verb with glob-backref spec v0.1.0 (DR-0027 + DR-0028)` 1 commit に squash。
