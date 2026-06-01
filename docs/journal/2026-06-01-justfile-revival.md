# 2026-06-01 justfile 復活候補 land (bump-trigger-paths + test *ARGS)

pkfire 試行前 (= 2026-05-13 以前) の justfile と DR-0022 で justfile 回帰後 (v0.30.0+) の
比較で出た復活候補 2 件を land。

## 1. bump-trigger-paths 変数化 (literal `src/`)

### Before

```just
# fail if src/ changed since origin/main but VERSION was not bumped
check-version-bumped:
    if ! bump-semver vcs diff -q main@origin -- src/; then bump-semver compare gt VERSION vcs:main@origin; fi
```

`src/` が recipe 内ハードコード。テンプレ流用時に上書きできない。

### After

```just
bump-trigger-paths := "src/"

check-version-bumped:
    if ! bump-semver vcs diff -q main@origin -- {{bump-trigger-paths}}; then bump-semver compare gt VERSION vcs:main@origin; fi
```

利点:
- kawaz テンプレ化 (= 他リポで `bump-trigger-paths := "src/ configs/"` 等に上書き可)
- 値は **git pathspec の literal `src/`**。tracked changes 網羅 (削除 / dotfile /
  gitignored-but-tracked を含む)

### `glob:` dogfood として試した → 撤回

初版は `bump-trigger-paths := "glob:src/**"` で land したが、`/code-review` で
**`glob:` (= bump-semver filesystem 展開) と `src/` (= git pathspec) は意味論が違う**
ことが判明:

| 観点 | `src/` git pathspec | `glob:src/**` bump-semver filesystem |
|---|---|---|
| 削除 file | git tree で検出 ✓ | 現 working copy に無い = enumerate されない ✗ |
| dotfile (`src/.foo`) | tracked なら含む | デフォルト除外 (`--glob-dotfile=true` 要) |
| `.gitignore` 内 tracked | tracked なら含む | デフォルト除外 |
| ディレクトリ | implicit 全配下 | files-only |

**典型 regression**: `src/old.go` を削除して push、VERSION bump 忘れ → `glob:src/**`
は削除済 file を enumerate しない → diff 対象から外れる → check-version-bumped が
**silent pass** → 未 bump release。

→ release gate (= 「src/ に変更があれば VERSION bump 必須」) は **git pathspec
semantic** を必要とする。`glob:` dogfood は build output 集合等の filesystem-side で
完結する用途で活かす方が筋。本 land では撤回、literal `src/` を採用。

## 2. test *ARGS 復活

### Before

```just
test: lint
    go test ./...
```

特定 package テスト不可。

### After

```just
test *ARGS='./...': lint
    go test {{ARGS}}
```

利点:
- `just test ./src/handler_cargo` のような特定 pkg test 可能
- 引数なし時は従来通り `./...` (互換)
- 1 行差分で機能復活

## 関連

- DR-0022: Justfile 回帰 (= 本 land で justfile の表現力が一段広がる)
- DR-0024: `glob:` prefix (= 本 land で dogfood 候補としたが、release gate の用途
  には semantic mismatch があり撤回。dogfood の場所選びは follow-up で)
- 起点 issue: `docs/issue/justfile-revival-candidates.md` (= 本 land で resolve、delete)
