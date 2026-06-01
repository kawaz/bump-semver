# DR-0024: `glob:<pattern>` 入力モード

- Status: Active
- Date: 2026-06-01
- Extends: DR-0008 (`vcs:` 入力モード), DR-0023 (N 引数化), DR-0020 (vcs subcommands)
- Related: DR-0022 (Justfile 回帰: タスクランナーからの呼び出しが主戦場)

## Context

bump-semver は justfile / pkfire 等のタスクランナーから多段引数渡しで呼ばれることが多い。タスクランナー → bump-semver のレイヤを跨ぐ際に、**shell の glob 展開がブレる**問題が出る:

- bash と zsh で `**` の挙動が違う (`shopt -s globstar` 要件、デフォルト挙動の差)
- jj / git 経由でフックされる場合、shell の選択がランナー依存になる
- `--` 越しに引数を中継する際、シェル展開が「ランナーの言語」で1回、子コマンドの shell で再度、と二重展開される
- shell 不変な「ファイルセット表現」が現状ない: 既存の path 入力は literal のみ、`vcs:` も literal の path しか取らない

結果として「`src/**/*.ts` を全部 bump-semver に渡したい」という単純な要求が、ランナー / shell / OS の組み合わせで何種類もの workaround を生む。

代替案 codex adversarial review (2026-06-01 実施) では以下が指摘された:

- **AXIS 1 (簡潔さ)**: 既存の `xargs` / shell glob で十分ではないか → kawaz 反論: shell ブレが主因。`glob:` prefix で **bump-semver 側に展開責務を持たせる** ことでブレが消える
- **AXIS 3 (依存コストと網羅責務)**: glob 仕様を bump-semver が抱え込むと長期メンテ負担 → 本 DR で **MVP の仕様を完全明文化** + 採用ライブラリの fidelity 限界も明記して負担境界を画定する
- **AXIS 4 (CLI コンセプト膨張)**: prefix が増えて学習負担増 → kawaz 反論: `vcs:` の successful precedent あり。prefix は `<scheme>:<body>` の URL-like patternで認知負荷低
- **AXIS 6 (オルタナティブ)**: `--glob` flag (= 単一引数を glob として扱う) や hybrid → kawaz 反論: prefix は **位置非依存** で `vcs:` / literal / VER と混在可能。flag は「どの引数が glob か」が argv 位置に依存して explicitness が落ちる

AXIS 1/4/6 への反論は kawaz の運用前提 (= タスクランナー主戦場、prefix モデル先行) で決着。AXIS 3 は本 DR の仕様完全明文化で対応。

## Decision

### `glob:<pattern>` prefix を導入

```
bump-semver get glob:src/**/*.json
bump-semver vcs diff -q main@origin -- 'glob:src/**/*.{ts,tsx}'
bump-semver compare gt '0.30.0' glob:packages/*/package.json
```

- `<pattern>` は shell に渡らず bump-semver が直接展開
- 既存の `vcs:` / `path` / `VER` / `-` (stdin) と **位置不問で混在可能**
- 0-match は **silent skip** (DR-0020 declarative-convergence と整合)

### パターン仕様 (MVP)

採用パターン (= doublestar v4 + 独自前処理で実現):

| パターン | 機能 |
|---|---|
| `*` | basename ワイルドカード (path separator 跨ぎ無し) |
| `**` | recursive directory match |
| `[0-9abc]` | 文字クラス (POSIX 風) |
| `{jpg,webp}` | 分岐展開 |
| `~` / `~/...` | home 展開 (= `os.UserHomeDir()`) |

**MVP 不採用**:

- `~user/...` (= 他ユーザの home): pwent lookup が POSIX 非ポータブル、kawaz 個人ユース皆無
- shell の謎拡張 (zsh の `^pattern` 反転、`#pattern` glob qualifier 等): bump-semver は portable subset のみ
- **exclude pattern** (= `!**/test/*` 等): 引数渡しが複雑化 (= argv 1 個に複数 pattern を圧縮する必要)、表現難。`file:LIST` 将来案で代替 (= ユーザ側で `grep -v` 等した結果を渡す)

### フラグ仕様 (`--glob-*` 3 種)

| フラグ | 値構文 | デフォルト | 単独指定時 |
|---|---|---|---|
| `--glob-dotfile` | **必須引数** `=true\|=false` | `false` (= 除外) | エラー |
| `--glob-gitignored` | **必須引数** `=true\|=false` | `true` (= 尊重) | エラー |
| `--glob-ignorecase` | **省略可** `[=true\|=false]` | `false` (= 区別) | `true` |

**Design rationale (値構文)**:

- `--glob-dotfile` / `--glob-gitignored` 単独形は「何を?」(include/exclude/respect/ignore) が曖昧。**必須引数化で polarity を明示** させる
- `--glob-ignorecase` は **動詞自体に polarity が含まれる** (= 「無視する」)。単独形 = `true` で慣習通り
- **`=value` のみ**、space 区切り (`--glob-dotfile true`) は不採用: 「`true` が次の引数なのか positional なのか」のレキシカルな曖昧性を排除 (= 他 flag より strict)。bump-semver の他フラグ (`--vcs jj` 等) とは規約が違うが、bool 専用の glob 系では `=value` 一本化で迷いがない

**Gitignored default の表現**:

- 内部表現は `*bool` (nil = default = true)。plain bool だと zero-value が false になり「明示 false」と「未指定」を区別できない
- ユーザ視点では「`--glob-gitignored=true` がデフォルト」と覚えればよい

### no-match 挙動

- silent skip (= exit 0、空結果として後続処理に渡す)
- 既存 `vcs diff -q -- nonexistent` の挙動と統一
- 例外: **すべての input が `glob:` で全部 0-match だった場合**、bump/compare/get では「最低 1 入力」ガードが exit 2 で発火 (= 黙って 0 件を bump にかけない安全側)

### 0-match × `vcs diff` の widening 防止 (重要)

`vcs diff REV` を pathspec 無しで呼ぶと **「全体の diff」に widen する** ことが既存実装 (`gitBackend.Diff` のコメント参照) で防止されている。glob: 経由で同じ事故を防ぐため、dispatcher 側で **「selectors を 1 つ以上与えたが expansion が空」のケースを短絡** する:

```go
selectorsGiven := len(rawPaths) > 0
paths, _ := expandGlobInputs(rawPaths, args.glob)
if selectorsGiven && len(paths) == 0 {
    return nil // diff nothing, NEVER widen
}
```

`vcs commit` も同様の短絡を入れる (= path モードで paths 空 + staged/amend なし → no-op success)。

### 採用ライブラリ + fidelity 開示 (AXIS 3 対応)

- **`github.com/bmatcuk/doublestar/v4`** (v4.10.0): `**` / `{}` / `[]` / POSIX 文字クラス対応、`fs.FS` ベース、`WithCaseInsensitive` / `WithNoHidden` / `WithFilesOnly` のフラグサポート
- **`github.com/sabhiram/go-gitignore`**: `.gitignore` parser。 .gitignore semantic の **完全互換は保証しない** (= AXIS 3 honest scope)
  - サポート範囲: glob base にある **1 つの .gitignore ファイル**
  - **未対応**: nested .gitignore (subdirectory 内の .gitignore)、`core.excludesfile` / global excludes、negation precedence の git 厳密実装
  - リポ外 (= `.gitignore` 不在) では silent no-op (エラーにしない、`--glob-gitignored=true` でも reasonable 動作)
- **`~` 展開** は doublestar 渡し前に独自前処理 (= `os.UserHomeDir()` + `filepath.Join`)。`homeFn` injectable でテスタブル

### `file:LIST` 将来候補 (本 PR 範囲外)

将来追加の可能性のあるモード (= MVP では不採用):

```
bump-semver vcs diff -q HEAD -- file:files.txt
```

- LIST file 1 行 1 path (literal)
- exclude を別経路で実現 (= ユーザが `grep -v` した結果を file: で渡す)
- 優先度: 低。glob: の MVP 運用で困った時に検討

## 統合点 (2 つあり、意味が違う)

| 統合点 | dispatcher | glob: の意味 |
|---|---|---|
| bump / compare / get | `resolveInputs` → `expandGlobInputs` | **FILE version-source** に展開。各 path は `resolveFile` を経由して通常の FILE 入力として扱う |
| vcs diff / vcs commit | dispatcher 内で `expandGlobInputs` を直接 call | **path filter** に展開。backend (`b.Diff` / `b.Commit`) に paths として渡す |

両者で共有するのは `expandGlob(pattern, opts, homeFn)` のコア関数のみ。

## 不採用案

### A. shell に任せる (status quo)

却下理由:

- bash/zsh の `**` 挙動差、`shopt -s globstar` の要件
- タスクランナー → shell → bump-semver の 2 段展開が予測困難
- kawaz の運用前提 (= タスクランナー多段引数渡し) で破綻

### B. `--glob` フラグ (= 単一引数の中身を glob と解釈)

```
bump-semver get --glob "src/**/*.ts"
```

却下理由:

- 「argv のどれが glob か」が flag 位置に依存
- 複数 glob を渡す方法が直感的でない (= flag 複数指定?)
- 既存の `vcs:` prefix モデル (= 位置不問の `<scheme>:<body>`) と整合しない

### C. hybrid (= literal path も内部で glob 展開試行)

例: `bump-semver get 'src/*.ts'` を渡したら literal が無ければ glob として解釈

却下理由:

- 「`src/*.ts` というファイル名のリテラル」と「glob `src/*.ts`」が曖昧
- 既存の literal path 解釈 (= 「存在しないファイルはエラー」) との互換が壊れる
- prefix で明示する方が **読む人間にとっても判別容易**

## 影響範囲

### 新規

- `src/glob.go` — `expandGlob` + `expandGlobInputs` + `parseGlobSpec` + `expandTilde`
- `src/glob_test.go` — パターン matrix / フラグ / dispatcher 統合テスト
- 依存追加: `github.com/bmatcuk/doublestar/v4` v4.10.0 / `github.com/sabhiram/go-gitignore`

### 既存変更

- `src/cli_parse.go` — `globOpts` 追加、`parseGlobFlag` helper、`parseSharedFlags` / `parseVcsArgs` に統合
- `src/resolve.go` — `resolveInputsOpts.Glob` 追加、`resolveInputs` 先頭で glob: 事前展開
- `src/cli_dispatch.go` — glob 0-match 時の "no inputs after glob expansion" exit 2 ガード
- `src/compare.go` — `resolved` count 不変条件を glob 対応に緩和
- `src/vcs_cmd.go` — `runVcsCmdDiff` / `runVcsCmdCommit` で `expandGlobInputs` + selectorsGiven 短絡
- README / README-ja — `glob:` 仕様セクション追加 (翻訳ペア同期)

## codex adversarial review との対応

| AXIS | 内容 | 対応 |
|---|---|---|
| 1 | 「shell glob で十分ではないか」 | kawaz の運用前提 (タスクランナー多段、shell ブレ) を明示。本 DR Context 節 |
| 3 | 「glob 仕様を内包する依存コスト」 | 本 DR で MVP 仕様を完全明文化 + 採用ライブラリの fidelity 限界も明記。`file:LIST` 将来案で exclude 等の追加スコープも事前に scope-out |
| 4 | 「CLI コンセプト膨張」 | `vcs:` prefix の successful precedent あり。`<scheme>:<body>` モデルは認知負荷低 |
| 6 | 「`--glob` flag / hybrid」 | 不採用案 B/C で trade-off 明記 |

## 検証

- パターン matrix (`*`/`**`/`[]`/`{}`/`~`) ごとに glob_test.go で fixture 検証
- フラグ matrix (default / dotfile=true / gitignored=false / ignorecase=true)
- 0-match × `vcs diff` の widening 防止 (anti-regression test)
- フラグ verb-aware reject (= `vcs get --glob-dotfile=true` は exit 2)
- bump/compare/get の glob: 統合 (= 1 個でも match した文書から version を読む)

## メモ

- doublestar v4 の case-insensitive 動作は **マッチした path の casing を pattern 側から取る**。case-sensitive な FS 上で `src/A.TS` パターンに `src/a.ts` がヒットすると `src/A.TS` (= 不存在) が返り得る。kawaz の主用途は macOS APFS (= case-insensitive) なので実害なし。将来 Linux で問題化したら後処理で canonicalize 検討
- `--glob-gitignored` の "git fidelity" 限界は本 DR で開示済み。ユーザに完全互換を期待させない
