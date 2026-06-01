# Justfile 復活候補 + `glob:` prefix dogfood

Status: 未着手 (2026-06-01)、kawaz 同意あり (= bump-trigger-paths は High、test *ARGS は Medium)

## 背景

pkfire 試行前 (= 2026-05-13 以前) の justfile と現在 (v0.30.0+) の比較で出た復活候補 + `glob:` prefix (DR-0024, v0.30.0 land 済) の dogfood 機会。

## 1. bump-trigger-paths 変数化 + `glob:` dogfood (High)

### 旧 justfile

`bump-trigger-paths := "src/"` のような変数定義で、kawaz テンプレを他リポに流用時 `src/` を上書きできる設計。

### 現状 (v0.30.0)

`check-version-bumped` recipe で `src/` ハードコード:

```just
check-version-bumped:
    if ! bump-semver vcs diff -q main@origin -- src/; then bump-semver compare gt VERSION vcs:main@origin; fi
```

### 復活案 + `glob:` dogfood

```just
bump-trigger-paths := "glob:src/**"

check-version-bumped:
    if ! bump-semver vcs diff -q main@origin -- {{bump-trigger-paths}}; then bump-semver compare gt VERSION vcs:main@origin; fi
```

利点:
- kawaz テンプレ化 (= 他リポで `bump-trigger-paths := "glob:src/**:configs/**"` 等に上書き可)
- `glob:` prefix の最初の実機 dogfood (= justfile / release.yml / Taskfile.pkl で `glob:` 使用箇所が現状 0、自己 dogfood の機会喪失中)
- `**` (= recursive match) で src 配下全階層をカバー

### kawaz の元判断

- glob 責務は B 案 (= `glob:` prefix で bump-semver 側に持たせる) で確定
- 「タスクランナー内多段引数渡しの安定化」が採用理由

## 2. test *ARGS 復活 (Medium)

### 旧 justfile

```just
test *ARGS='./...':
    go test {{ARGS}}
```

`just test ./path/to/pkg` で特定 package テスト可能。

### 現状 (v0.30.0)

```just
test: lint
    go test ./...
```

ハードコード `./...`、引数で特定 pkg 指定不可。

### 復活案

```just
test *ARGS='./...': lint
    go test {{ARGS}}
```

利点:
- 緊急時に特定 package のみ test 可能 (= `just test ./src/handler_cargo`)
- 1 行追加で機能復活、コスト最小

### kawaz の判断状態

「できた機能をなくしたは惜しい」+ Medium 優先、ただし最終判断未確認。

## 3. その他の dogfood 機会

`glob:` prefix の使用候補 (= 1 件目以降):
- release.yml の何かの path 集合化
- ci.yml のテスト対象 glob 化
- (= 派生 sync check CLI 実装時に Phase 2 で本格 dogfood)

## 次のアクション

- kawaz 確定後に PR 起こす:
  - Phase 1: bump-trigger-paths 変数化 + `glob:` dogfood
  - Phase 2 (optional): test *ARGS 復活
- DR は不要 (= 既に DR-0024 で `glob:` 仕様 land 済、使用例追加だけ)
- VERSION bump = patch (= tooling 変更、機能不変)
