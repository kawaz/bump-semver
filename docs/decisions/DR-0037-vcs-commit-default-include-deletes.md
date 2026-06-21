# DR-0037: `vcs commit` のデフォルト挙動反転 — 削除 path も素直に git/jj に渡す

- Status: Accepted (2026-06-21)
- Date: 2026-06-21
- Related: DR-0020 (vcs-subcommands, Commit PR-4 の初期設計), `docs/issue/2026-06-21-vcs-commit-nonexistent-path-silently-drop.md`

## Context

DR-0020 PR-4 は `vcs commit -m MSG PATH..` の path mode に **declarative-convergence** (= 指定 path のうち存在するものだけ commit、不在は無視) を採用した。設計の意図は「`package.json` / `Cargo.toml` / `VERSION` 等の候補を言語を問わず羅列し、存在するものだけ commit する怠惰パターン」の成立。

この設計は bump 系 justfile では便利に機能するが、**削除された tracked path** を扱う場面で破綻する:

- `os.Stat` ベースのフィルタ (`filterExistingPaths`) は「削除された tracked file」と「最初から存在しない file」を区別しない。どちらも `os.Stat` で ENOENT になるため、同じようにフィルタ落ちする
- `mv old new` パターンで `old` の delete を commit しようとすると、`old` が PATH に含まれていても無声で drop され commit から除外される
- この挙動は claude-local-issue 開発 (v0.2.0–v0.2.2) で 3 回再現、`cp + rm + --staged` の workaround で逃げていた

原因は bump-semver 側の設計にあり、利用側のワークアラウンドで問題を隠すのは不適切。

## Decision

### デフォルト挙動を反転する

`vcs commit -m MSG PATH..` の path mode で `filterExistingPaths` の呼び出しを廃止し、**指定 path をそのまま git/jj に渡す**:

- **git**: `git add -A -- PATHS` で modified/added/deleted を全て stage してから commit。`-A` (= `--all`) により working tree での削除も index に反映される
- **jj**: `jj commit FILESETS -m MSG` に paths を fileset として渡す。jj はもともと working copy 全体を snapshot するため、deleted path も透過的に扱われる

typo や存在しない path は git/jj 自身がエラーを返す (= principle of least astonishment)。利用者は意図しない no-op を silent に踏まなくなる。

### 旧挙動は `--allow-nonexistent-path` でオプトイン

複数候補を並べて存在しないものを skip する bump 用途が必要な場合は `--allow-nonexistent-path` を明示指定する:

```just
# 旧挙動 (複数候補から存在するものだけ commit)
bump-semver vcs commit -m "chore: bump version" \
  --allow-nonexistent-path \
  package.json Cargo.toml VERSION go.mod

# 新デフォルト (指定 path を素直に渡す)
bump-semver vcs commit -m "chore: bump version" VERSION
```

### フラグ名の選定理由

- 既存の `--allow-move` / `--allow-empty` と整合する `--allow-X` 命名規律を踏襲
- `--ignore-missing` は `-m MSG` の `-m` 短縮形と並んで誤読リスクがある
- `--allow-nonexistent-path` は動作対象 (nonexistent path) と許可方向 (allow) が明確

## Consequences

### Breaking change

新デフォルト挙動は後方互換でない。CHANGELOG に BREAKING として記載し、MINOR bump。

### 影響範囲

grep 調査の結果 (issue 参照):

- **影響なし**: 単一 file 固定型 (`Cargo.toml Cargo.lock`、`VERSION`、`package.json` 等、常時存在する file 列挙)。対象 file が必ず存在するため、デフォルト反転後も挙動不変
- **要移行**: 「複数候補から無いものを skip」するパターン (例: claude-cmux-msg の `{{ version-files }}` 展開)。これらは `--allow-nonexistent-path` を追加して旧挙動に戻す

移行コストは小さい。bump 系 justfile は 1 リポ 1 行が大半で、対象リポは限定的。

### `vcs diff` の declarative-convergence は変更しない

`filterExistingPaths` は `vcs diff` (DR-0020 PR-3) 用途では維持される。DR-0020 PR-3 の仕様 「存在しない PATH は無視」は diff verb の意図と整合しており、本 DR の影響を受けない。本 DR は `vcs commit` の path mode にのみ作用する。

## Migration

各リポの justfile を以下の観点で確認する:

1. `bump-semver vcs commit -m MSG PATH..` の PATH が「常時存在する file 名」のみか
2. 「あるかもしれない候補リスト」が並んでいる場合は `--allow-nonexistent-path` を追加

```bash
# 利用リポの justfile / shell script を grep して列挙
grep -rn 'bump-semver vcs commit' --include='justfile' --include='*.sh' --include='*.bash' <repos-root>
```

## Related

- [DR-0020](./DR-0020-vcs-subcommands.md) — `vcs` サブコマンド群の初期設計。Commit の path mode (PR-4) で declarative-convergence を規定していた。本 DR は Commit path mode についてその規定を反転する (Diff / `--staged` / `--amend` 各モードへの影響なし)
- `docs/issue/2026-06-21-vcs-commit-nonexistent-path-silently-drop.md` — 起票元。`mv old new` パターンでの削除黙殺が 3 回再現した経緯を記録
