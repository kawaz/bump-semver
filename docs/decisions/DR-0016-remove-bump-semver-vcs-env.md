# DR-0016: `BUMP_SEMVER_VCS` 環境変数廃止 + `--vcs auto` 一本化

- Status: Active
- Date: 2026-05-11
- Supersedes: DR-0008 の「VCS 自動判定」セクション (env 部分)

## Context

DR-0008 で `vcs:` 入力モードを導入した時、VCS 検出の優先順位として以下を定義した:

1. `--vcs git|jj` フラグ
2. `BUMP_SEMVER_VCS=git|jj` 環境変数
3. `.jj` 存在 → jj
4. `.git` 存在 → git
5. どちらもなし → エラー

しかし環境変数は実運用でほぼ使われなかった。理由:

- ローカル開発: cwd の `.jj` / `.git` で十分判定できる
- CI: workflow 側で `--vcs jj` を渡すほうが明示的で grep しやすい
- 一度 `export BUMP_SEMVER_VCS=jj` すると **コマンドラインから「自動検出に戻す」手段がなかった** (`--vcs auto` も `--vcs ""` も存在せず、`env -u BUMP_SEMVER_VCS bump-semver ...` のような shell 側の対処が必要)

加えて help セクションのコスト:

- `Global Options:` に `--vcs jj|git ... (overrides BUMP_SEMVER_VCS env)` で 1 行
- `Environment:` セクションが env 1 つのためだけに存在
- README / DESIGN / ROADMAP に「VCS 検出優先順位 4 段」を都度解説

ニッチ機能 1 個のためにドキュメント・help の領域を占有するコストが、利便性を明確に上回っていた。

## Decision

### `BUMP_SEMVER_VCS` 環境変数サポートを廃止

- `detectVcs` から env 読み取りを削除
- help から `Environment:` セクションを削除 (他に env 変数がないのでセクションごと消える)
- README / DESIGN / ROADMAP の優先順位記述を 3 段に縮約

### `--vcs auto` を明示値として許可

`parseVcsOverride` に `"auto"` ケースを追加し、`""` (未指定) と同じく `vcsAuto` へ解決する。挙動は未指定と同じだが、help が

```
--vcs jj|git|auto      Force VCS detection for vcs: inputs (default: auto)
```

と書けるようになる。3 値挙動が自己説明的になり、default が明示される。

### 新しい VCS 検出優先順位

1. `--vcs jj|git` フラグ (`auto` / 未指定は次へ)
2. `.jj` 存在 → jj
3. `.git` 存在 → git
4. どちらもなし → エラー

`.jj` と `.git` が並存する場合 jj が優先される点 (kawaz の git-bare + jj-workspace レイアウト) は DR-0008 から不変。

## Rationale

### 不採用案

**1. env サポートを残しつつ `--vcs auto` を「env も無視」と定義**

`--vcs auto` フラグが env を skip する形にすれば env 経由の固定指定もコマンドラインから解除できる。**不採用**: フラグと env の優先順位ルールが 2 段から 3 段に増える (default / explicit auto / explicit value)。利用者が頭の中で組み立てる優先順位図が複雑化する。元々ニッチな env サポートの延命のために優先順位を複雑化するのは本末転倒。

**2. env だけ残してフラグ側に `auto` を入れない**

現状維持に近い案。**不採用**: env 利用例がそもそもほぼない (kawaz 自身も使っていない)、ユーザに「env でしか解除できない」という落とし穴を残す合理性がない。

**3. `--no-vcs` フラグで env を解除**

`--no-` プレフィックス系の他フラグと揃える案。**不採用**: 「VCS を使わない」と誤解される (実際には「auto detect させる」)。`auto` の方が self-documenting。

### 設計上のポイント

#### `--vcs auto` を default として扱う

`auto` は内部 sentinel `vcsAuto` が元から default 値 (`iota` の 0) で持っていた値。フラグ表記として `auto` を許可することで、内部表現と CLI 表現が揃う:

- フラグ未指定 → `vcs = ""` → `parseVcsOverride("")` → `vcsAuto`
- `--vcs auto` → `vcs = "auto"` → `parseVcsOverride("auto")` → `vcsAuto`

どちらも同じパスを通るので、`"auto"` を許可するのは純粋追加。エラーメッセージは `(expected jj, git, or auto)` に更新。

## Consequences

### 破壊的変更

`BUMP_SEMVER_VCS=jj|git` を CI / 開発環境で設定していた利用者は、`--vcs jj|git` フラグへの置き換えが必要。

```bash
# 旧
export BUMP_SEMVER_VCS=jj
bump-semver compare gt Cargo.toml vcs:main@origin

# 新
bump-semver --vcs jj compare gt Cargo.toml vcs:main@origin
```

ニッチ機能なので影響範囲は限定的と判断。次リリース (v0.13.0 想定) で UPGRADING.md にも明示する。

### 互換性

- `vcs:` 入力を使わない呼び出しは挙動不変
- `--vcs jj|git` フラグの挙動も不変
- `--vcs auto` は新規許可 (`""` / 未指定と同じ意味)

### バージョン

次の minor リリース (v0.13.0 想定) で削除。MAJOR ではなく MINOR で破壊的変更を扱うが、bump-semver は pre-1.0 のため SemVer の MINOR でも互換性のない変更を入れて良い段階。

### ドキュメント更新範囲

- `docs/DESIGN-ja.md` / `docs/DESIGN.md` line 54 付近: 優先順位 4 段 → 3 段、env 削除
- `docs/ROADMAP.md` line 23 付近: 同上
- `README-ja.md` / `README.md`: `--vcs` 行の説明、優先順位リスト
- `docs/journal/2026-05-10-vcs-input-implementation.md`: env 言及行のみ削除 (journal 全体は時系列の事実として残す)
- `docs/decisions/DR-0008-vcs-input.md`: 「VCS 自動判定」セクションに Superseded 注記
- `docs/decisions/INDEX.md`: DR-0016 を Active に追加

## 関連実装

- `src/vcs.go`: `detectVcs` から env 読み取り削除、`parseVcsOverride` に `"auto"` 受理を追加、優先順位コメント更新
- `src/main.go`: help から `Environment:` セクション削除、`--vcs jj|git` → `--vcs jj|git|auto`、`--vcs requires a value` エラーメッセージ更新
- `src/vcs_test.go`: `TestParseVcsOverride` に `"auto"` / `"AUTO"` ケース追加、コメントから `BUMP_SEMVER_VCS` 言及削除
