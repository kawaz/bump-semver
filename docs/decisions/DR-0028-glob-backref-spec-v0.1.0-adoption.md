# DR-0028 — glob-backref spec v0.1.0 採用 (`vcs outdated` 本実装統合)

- **Status**: Active
- **Date**: 2026-06-02
- **Extends**: [DR-0024](./DR-0024-glob-prefix.md) (`glob:` prefix), [DR-0027](./DR-0027-derived-sync-mini-dsl-and-regex-reject.md) (mini-DSL 採用 + `regex:` 不採用)
- **Partially supersedes**: DR-0027 — mapping DSL の構造を spec v0.1.0 (`docs/specs/glob-backref-v0.1.0.md`) で再設計。**「`regex:` 不採用」「`vcs outdated` verb 採用」「`--explain` 必須」「自動除外」は維持**。supersede したのは「実装上の matching semantics / backref numbering / TO escape」の 3 点のみ。

## 採用宣言

bump-semver の `vcs outdated` 実装を、独立 spec **glob-backref v0.1.0** (`docs/specs/glob-backref-v0.1.0.md`) 準拠に統合する。

spec 自体は **言語非依存**。bump-semver は最初の参照実装として位置付け、将来 TS / Rust / MoonBit 等の独立実装が同 spec を参照する想定。

## 旧 DR-0027 構造との差分 (= supersede 範囲)

| 観点 | 旧 DR-0027 (v1 実装) | v0.1.0 spec (本 DR) |
|---|---|---|
| `{}` の matching | regex `(a\|b)` で alternation | **直積全展開** (= AST level で N concrete pair に分解) |
| `**` 0-segment match の TO 反映 | `$1=""` → `collapseSlashes` で `lib//foo` を `lib/foo` に潰す | `$1="."` → `path.Clean("./foo")` で `foo` に normalize |
| TO 側 captured value の再 glob 解釈 | regex substitute なので原理上は safe だが invariant 明文化なし | **TO `glob:` 時は char-class wrap で literal 化** (= `a*b` → `a[*]b`)、template 外側の glob meta は生かす |
| `*`/`[]` の空文字 match | silent skip (= 露出しないまま流れる) | **panic** (= "grammar drift assertion"、release gate での silent failure 防止) |
| backref 番号 | 可変パーツ出現順 | 同 (= 仕様維持) |
| `${0}` / `$0` | error | **matched path 全体** (= `lookupCap` 経由で常に safe access) |
| `$N` 範囲外 | error | **空文字** (= help / Examples で「番号付け規則」を明示する設計) |
| `$10` 形式 | error (= `${1}0` のみ) | error (= `${10}` 必須、`$10` は ambiguous で reject) |
| literal-FROM 不在 | silent exit 0 | **default warn + exit 0** (= 互換性)、`--strict` flag で **exit 1** |

## 旧 v1 の構造的問題 4 件と本 DR での解消

PoC (v2) + 本実装統合で以下を根本対応:

1. **leading-slash false-missing** (= `'glob:**/*-ja.md'` `'${1}/${2}.md'` で root の `README-ja.md` が `/README.md` に化ける)
   - 根本原因: v1 では `**` 0-segment match で `$1=""`、TO substitute 結果が `/README.md` (= 絶対 path 化)
   - v0.1.0 spec: `**` 0-segment → `$1="."` + `path.Clean` で `README.md` に正規化 (§2.2.2 / §3.5)

2. **TO 側 backref captures の再 glob 解釈リスク** (= 病的 filename `a*b` を含む captured value が TO の `glob:` 解釈で再展開される silent path drift)
   - 根本原因: v1 では substitute 結果に対する 2 段目 glob 解釈時の escape 仕様なし
   - v0.1.0 spec §3.4.2: char-class wrap (`*` → `[*]` 等) で literal 化、template 外側の meta は生かす

3. **`*`/`[]` の grammar drift silent skip** (= regex `*` が空文字に match して silent fresh と誤判定する CI silent failure)
   - 根本原因: v1 では doublestar match と capture regex の不整合を skip 処理 (= continue)
   - v0.1.0 spec §2.2.3 / §7: **panic** (= release gate での silent failure 防止)

4. **literal-FROM 不在の silent green CI** (= typo 'd literal FROM `README-ja.MD` が「ファイルなし → vacuously true」で release gate を通過)
   - 根本原因: v1 ではすべての FROM 不在を「nothing to check, exit 0」と扱う
   - v0.1.0 spec + 本実装: `--strict` flag で literal-FROM 不在を **exit 1** にできる (= default warn 維持で互換性確保)

## 影響範囲

### 削除 (= v1 完全撤去)

- `src/glob_backref.go` (= v1 実装、465 行)
- `src/glob_backref_test.go` (= v1 test、334 行)

### rename + 統合 (= v2 PoC を本実装に昇格)

- `src/glob_backref_v2.go` → `src/glob_backref.go`
- `src/glob_backref_v2_test.go` → `src/glob_backref_test.go`
- API: `MatchV2` → `Match`、`SubstituteV2` → `Substitute`、`OptionsV2` 撤去 (= 既存 `globOpts` に統合)

### rewrite

- `src/cmd_vcs_outdated.go` (= v2 API ベースに書き直し、`--strict` 追加、error wrap fix)
- `src/cmd_vcs_outdated_test.go` (= v2 挙動 pin に update)
- `src/help.go::helpVcsOutdated` (= `commit(s)` 統一、`--strict` 追加、shell escape 維持。`?` は MVP scope 外として parser で reject — 仕様 §2.1 に揃え、help / README から `?` の言及を削除)
- `README.md` / `README-ja.md` 該当節

## v0.1.0 で **scope-out** している事項 (= spec §2.1)

- `{}` ネスト
- `?` 単一文字 wildcard (= 将来予約、parser で graceful reject。理由: doublestar は `?` を wildcard 扱いするが capture-regex は literal 扱いになり、ユーザ入力で spec §7 の grammar-drift panic が誤発火する。explicit reject で「未実装の wildcard」と「実装 bug」を分離する)
- `[^...]` complement char class
- backslash / quote escape
- `~user/...` home 展開
- 病的 filename (= 特殊文字を含むファイル名) は対象外、user 責任
- cross-source 自動除外 (= per-source のみ採用、cross-source は v0.2 送り)
- named capture (`${name:pattern}`)

これらは spec v0.2 以降 / 別 DR / 別 spec で扱う (= 本 DR では決定しない)。

## bump-semver 参照実装の選択 (= spec §15.6)

- API: `MatchCollect` + `Substitute` + `ExpandPairs` の collect 3 API
- stream variant (= `Match` / iterator) は未提供 (= 中小規模リポ想定、将来 spec 拡張時に追加)
- `walkOrder=depth` 固定 / `walkConcurrency=1` 固定 / `followSymlinks=false` 固定
- `dotfile` / `gitignored` / `ignorecase` は既存 `globOpts` (DR-0024) 経由で CLI flag に bind
- error policy: `raise` 固定 (= silent skip しない)、grammar drift は panic

将来 stream / cancellation / parallel walk 等は別言語実装 (= TS の AsyncIterable、Rust の Iterator) で活用想定。

## 検証

- 18 spec test (= `src/glob_backref_test.go` の T1-T18) で言語非依存 vector を pin
- cmd test (= `src/cmd_vcs_outdated_test.go`) で release gate semantics を pin (= stale/fresh/missing/`--strict`/`--explain`/auto-exclude/`--glob-*` flag 連動)
- ドッグフーディング: `bump-semver vcs outdated 'glob:**/*-ja.md' '${1}/${2}.md'` で README-ja → README freshness check が動作

## 関連

- spec: `docs/specs/glob-backref-v0.1.0.md`
- `docs/journal/2026-06-02-mini-dsl-rewrite-and-blocker-fix.md` — 統合作業の経緯
