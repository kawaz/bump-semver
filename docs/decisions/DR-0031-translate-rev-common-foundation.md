# DR-0031: rev 翻訳を共通基盤化 (vcs: / vcs サブコマンドの全 rev 受け口)

## Status

Accepted (2026-06-09)

## Context

`vcs:REV[:FILE]` 入力モード / `vcs` サブコマンド系 (`vcs diff REV` / `vcs tag push --rev REV` / 将来追加予定の `vcs get commit-id --rev REV` 等) で rev を受けるが、backend に応じた revspec 翻訳は **`FetchFile` (jj) のフォールバックでしか実装されていない**。

- `vcs.go:226 altJjRev` が `<remote>/<bookmark>` → `<bookmark>@<remote>` (例: `origin/main` → `main@origin`) を翻訳
- 使用箇所: `vcs_backend.go:603` の `(j *jjBackend).FetchFile` のフォールバックのみ
- 他の rev 受け口 (`(j *jjBackend).Diff` / `DiffNameStatus` / `TagPush --rev` (`resolveJjRev` 経由) / `resolveGitRev`) は **素通し** → jj backend で `origin/main` を渡されると fail
- DR-0020 が掲げる「vcs subcommands は git/jj agnostic」の思想に対し、rev 受け口だけ部分対応のまま放置

直接の発端: 2026-06-09 セッションで hyoui の `just push` hint で push 後の main sha を取りたい場面、`bump-semver vcs is jj && jj log -r main --no-graph -T 'commit_id' || git rev-parse main` の if-else を justfile に書く羽目になった。

## Decision

### 共通基盤関数

```go
// translateRev は user-supplied rev を backend native な rev に翻訳する。
// 翻訳不要 (= 共通 syntax / backend 固有 syntax) は pass-through。
func translateRev(rev string, kind vcsKind) string
```

- 入力: user-supplied rev (git/jj どちらの syntax でも可能性あり)
- 出力: backend-native rev (= そのまま backend cmd の `-r` / rev arg に渡せる string)
- error は返さない (= 翻訳は best-effort、解決失敗は呼び出し先の resolver/cmd が報告)

### 適用箇所 (= 全 rev 受け口)

| 適用先 | 現状 | DR 適用後 |
|---|---|---|
| `(j *jjBackend).FetchFile` | altJjRev フォールバック | 入口で translateRev、フォールバック削除 |
| `(j *jjBackend).Diff` / `DiffNameStatus` | 素通し | 入口で translateRev |
| `resolveJjRev` (= `TagPush --rev`, 将来 `commit-id` verb の中核) | 素通し | 入口で translateRev |
| `(g *gitBackend).*` の rev 受け口 | 素通し | 入口で translateRev (= git native は no-op、jj syntax の `main@origin` は `origin/main` に翻訳) |
| `vcs:REV[:FILE]` 入力モード | (jj は altJjRev 経由) | input mode の rev parsing で translateRev を 1 度通す |

### 翻訳ルール (MVP, v1)

1. **`<remote>/<bookmark>` ⇔ `<bookmark>@<remote>`** (single-slash only)
   - 例: `origin/main` ↔ `main@origin`
   - 既存 `altJjRev` のロジックを翻訳基盤に統合
   - 双方向: kind=jj なら git syntax を jj syntax に、kind=git なら jj syntax を git syntax に
2. **共通 syntax は pass-through** (= 両 backend が解釈できる文字列はそのまま)
   - tag 名 / bookmark 名 / branch 名 (`main`, `feature/x` 等の single segment / multi-segment)
   - full commit_id / change_id (40-char hex)
3. **backend 固有 syntax は pass-through** (= 翻訳せず backend に投げ、解決失敗は backend が報告)
   - jj 固有: revset 演算子 (`..`, `::`, `|`, `&`, `~`), 短縮 change_id, `latest(...)`, `roots(...)` 等
   - git 固有: `^`, `~N`, `@{u}`, `@{upstream}`, `HEAD`, `ORIG_HEAD`, `FETCH_HEAD`, etc

### 翻訳判定アルゴリズム (= pass-through の境界)

```
if rev contains exactly one "/" AND no other special chars (= [.~^@:]):
    swap halves with "@"
else:
    return rev unchanged
```

例:
- `origin/main` → 翻訳 → `main@origin` (kind=jj 時) / `main@origin` → 翻訳 → `origin/main` (kind=git 時)
- `feature/foo/bar` → pass-through (slash 複数、ambiguous なので user に backend native で書かせる)
- `HEAD~1` → pass-through (git native)
- `@-` → pass-through (jj native)
- `main` → pass-through (= 両 backend で名前解決)
- `abc1234` → pass-through (= sha/change_id)

### 対称翻訳は v1 では実装しない

`@-` ⇔ `HEAD^`, `@--` ⇔ `HEAD^^` 等の対称翻訳は意味重なるが context が違う (jj `@-` は working copy の親 = describe 済みの commit、git `HEAD^` は branch tip の親)。判定境界が複雑、user の意図ズレが起こり得る。**後続 DR で再検討**。

### default rev

- jj backend: `@`
- git backend: `HEAD`
- 関数 signature では `--rev` 省略は呼び出し側責任 (translateRev は non-empty rev を期待 = empty なら panic 防止のため pass-through 同等)

### 失敗時 exit code (= 既存規約に従う)

- 解決不能 (resolveJjRev / resolveGitRev / backend cmd 失敗): **exit 3** (VCS subprocess error)
- ambiguous (jj revset が複数 commit にマッチ): **exit 4** (ambiguous answer)
- usage error (= caller が空 rev を渡す等): **exit 2** (usage error)

これらは DR-0020 の exit code 体系をそのまま継承。translateRev 自体は exit code を発しない (= 翻訳結果が空でも `nil` rev でもなく、入力 rev のまま返す)。

### MVP scope (v0.32.0)

- `translateRev` 関数新設 (`src/vcs.go`)
- 既存 `altJjRev` のロジック統合 (= `altJjRev` は廃止、translateRev 内に inline 化)
- 全 6 系統に適用:
  1. `(j *jjBackend).FetchFile` (= 既存 altJjRev フォールバック置換)
  2. `(j *jjBackend).Diff` / `DiffNameStatus`
  3. `resolveJjRev` (= TagPush --rev 経路)
  4. `(g *gitBackend).FetchFile` / `Diff` / `DiffNameStatus`
  5. `resolveGitRev`
  6. `vcs:REV[:FILE]` input mode の rev parsing
- unit test (`translateRev_test.go`): 共通 syntax / backend 固有 syntax / 翻訳パターン (片方向 × 2)
- 回帰 test: `vcs diff origin/main` が jj backend で動く (旧 fail パターン)

### 派生 DR/issue (out of scope)

- 対称翻訳 (`@-`/`HEAD^` 等): 後続 DR で再検討。MVP に含めると判定境界が複雑化、bug 温床。
- `vcs get commit-id --rev REV` 新 verb: 本 DR 解決の延長で別 commit。translateRev 基盤を直接利用。

## Consequences

### Pros

- 全 rev 受け口で git users が `origin/main` を打っても通る (今は FetchFile のみ)
- 同様に jj users が `main@origin` を git backend で打っても通る
- 派生 verb (`vcs get commit-id --rev`) 実装範囲が狭くなる (= 翻訳ロジック再利用)
- ad-hoc fallback が散らからない (= maintainability 向上)
- DR-0020 の vcs-agnostic 思想を rev 受け口でも貫徹
- justfile を agnostic に書けるようになる (= 派生 verb 実装後に「if-else を消す」)

### Cons

- 翻訳レイヤが 1 つ増える (= 認知負荷+わずか、ただし関数 1 つで透明性ある)
- 翻訳判定が「単一 `/` のみ」固定なので、`feature/foo/bar@origin` 等は user 側で明示
- 翻訳失敗時の挙動が「backend にそのまま渡す」なので、user が誤った rev を渡したときの error message が backend native (= bump-semver wrapper の error message と質が異なる)
- altJjRev test を translateRev test に migrate する手間 (= 既存 test 案件、MVP に含める)

### Rejected alternatives

- **(a) 各 rev 受け口で altJjRev を個別に呼ぶ**: ad-hoc 散らかり、覚えて忘れる、maintenance 負債。今の状態の延長で、本質的解決にならない。
- **(b) bump-semver が独自 rev DSL を定義 (LISP 風 / s-expression / etc)**: user 学習コスト大、既存 syntax との衝突、scope creep。bump-semver の存在意義 (= 既存 syntax を吸収) と相反。
- **(c) 翻訳を諦めて user に backend-native rev を打たせる**: DR-0020 の vcs-agnostic 思想に反する。
- **(d) MVP に対称翻訳 (`@-`↔`HEAD^`) を含める**: 判定境界が複雑、context 違いで意図ズレ、bug 温床。後続 DR で慎重に。

## Related

- [DR-0020](./DR-0020-vcs-subcommands.md) — vcs subcommands 思想の正本 (= 本 DR は rev 受け口でその思想を貫徹)
- [DR-0008](./DR-0008-vcs-input-mode.md) — `vcs:REV[:FILE]` 入力モード (= 本 DR の適用先 6 番目)
- [DR-0019](./DR-0019-vcs-latest-tag-remote-arg.md) — `vcs:latest-tag(<arg>)` の remote arg 対応 (= vcs: 入力モードの拡張系譜)
- [DR-0027](./DR-0027-derived-sync-mini-dsl-and-regex-reject.md) — `vcs outdated` の入力 normalize (= 似た思想で別ドメインの normalize)
- 起源 issue: `docs/issue/2026-06-09-translate-rev-common-foundation.md` (本 DR 採択 + 実装完了で削除)
