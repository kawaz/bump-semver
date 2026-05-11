# 2026-05-11 pkfire PoC: justfile を Taskfile.pkl に翻訳

## 背景

- kawaz/bump-semver の justfile を canonical として運用していたが、mizchi/pkfire (Pkl で書く typed タスクランナー + Bazel 風 incremental cache) を試したくなった
- 方針: justfile はロールバック先として残し、pkfire をメイン入口にする
- pkfire 0.3.0、pkl 0.31.1、pkf は `go install github.com/mizchi/pkfire/cmd/pkf@latest` (dev build)

## やったこと

`justfile` の全レシピ (lint / test / build / ci / ensure-clean / check-translations / check-version-bumped / push / bump-version) を `Taskfile.pkl` に等価翻訳。`pkf list` `pkf graph` `pkf run` で動作確認。

## ハマり所と解決策

### 1. タスク名 regex が `/` を許容しない

pkfire の `Task.name` は `^[a-zA-Z][a-zA-Z0-9_:.-]*$`。`check-translation:docs/DESIGN` の `/` が違反:

```
Type constraint `matches(Regex(#"^[a-zA-Z][a-zA-Z0-9_:.-]*$"#))` violated.
Value: "check-translation:docs/DESIGN"
```

解決: dogfood example の `Platform` クラスと同じパターンで、ファイルパス (`path`) とタスク名キー (`key`) を分離した struct を作る。

```pkl
local class TranslationPair {
  path: String
  fixed key: String = path.replaceAll("/", "-")
}
```

`key` で `docs-DESIGN` 形式、`path` で `docs/DESIGN` 形式を使い分け。

### 2. 引数受け取りレシピは pkfire に存在しない

justfile の `bump-version bump="patch"` のような引数付きレシピは pkfire にない。Task は静的な定義。

解決: name-suffix で 3 タスクに展開:

```pkl
local function bumpVersionTask(level: String): Task = new {
  name = "bump-version:\(level)"
  cmd = #"new_version=$(bump-semver \#(level) VERSION --write --no-hint) && jj commit -m "Release v${new_version}""#
  ...
}

local bumpVersionTasks: Listing<Task> = new {
  for (level in new Listing<String> { "patch"; "minor"; "major" }) {
    bumpVersionTask(level)
  }
}
```

`pkf run bump-version:patch` のように叩く。これは pkfire の流儀として自然 (just の引数より明示的)。

### 3. 関数呼び出しは値を返すので、同じ呼び出しを 2 箇所で書くと別インスタンスになる

`local function checkTranslation(p): Task` を `tasks { checkTranslation("README") }` と `deps { checkTranslation("README") }` で 2 回呼ぶと、Task インスタンスが 2 つ生成され、同じ name が衝突 (pkfire は duplicate name を error)。

解決: 一度 `Listing<Task>` に束ねて、両方からそれを参照:

```pkl
local checkTranslationTasks: Listing<Task> = new {
  for (p in translationPairs) { checkTranslation(p) }
}
// tasks 側
tasks { ...checkTranslationTasks }
// deps 側
local checkTranslations: Task = new { ..., deps = checkTranslationTasks }
```

dogfood の `local builds: Listing<Task>` と同じパターン。

### 4. run *ARGS は移植不可

`just run -- --foo bar` のような可変引数レシピは pkfire ではサポートされない。justfile に残すか、用途を絞って固定タスクに分解する。今回は justfile に残した。

### 5. gh credential helper の brew tap 罠

`brew tap pkl-lang/pkl` が失敗:

```
/opt/homebrew/bin/gh auth git-credential get: /opt/homebrew/bin/gh: No such file or directory
fatal: could not read Username for 'https://github.com': Device not configured
```

原因: git config に `credential.https://github.com.helper=!/opt/homebrew/bin/gh auth git-credential` が登録されているが、実際の gh は nix-managed (`/etc/profiles/per-user/kawaz/bin/gh`) にある。dotfile の不整合。

回避: `pkl` は bottle 配布なので tap clone が失敗しても `brew install pkl` 自体は通る。tap dir に直接 `git -c credential.helper= clone ...` するか、最低限 install だけ走らせれば動く。

dotfile 側で credential.helper のパスを修正するか、helper 設定を消すべき (今回は触らず、別タスクで)。

## 検証結果

| 検証 | 結果 |
|---|---|
| `pkf list` / `pkf graph --format mermaid` | OK (14 タスク、DAG 正常) |
| `pkf run lint` 2回目 | `hit b2efcd108d32` (cache hit、即終了) |
| `pkf run test` | OK (9.2s、lint は cache hit) |
| `pkf run build` | OK (`bin/bump-semver` 生成、lint hit) |
| `pkf run ci` | OK (lint/test/build 全 hit) |
| `pkf run check-translations` | OK (-ja.md 不在ペアは skip) |
| `pkf run ensure-clean` / `check-version-bumped` | 正常失敗 (gate として機能) |

## 評価 (PoC 時点)

良い点:
- typed deps: `deps { lint }` が直接 Task 参照、typo は Pkl 評価時に検出
- inputs ベース cache が想定通り効く (lint hit で test/build の前段がスキップ)
- DAG 並列実行と mermaid 可視化が便利
- gate 系 (ensure-clean, check-version-bumped) も自然に DAG に組み込める

注意点 / 今後の検討:
- 引数受け取りができないので name-suffix 展開でカバー (足し算的だが、justfile の引数より明示的とも言える)
- run *ARGS は justfile に残すしかない
- pkf は dev build (go install で latest)。0.3.0 リリースタグでのバイナリ配布もあるが、`brew tap kawaz/tap` に formula を作るのは要検討
- Pkl 学習コストは思ったより低い。dogfood example が良い手本

## 次のステップ

- しばらく `pkf run X` で運用、不便点を蓄積
- 問題が出たら `just X` にフォールバック
- 他リポ (kawaz/claude-cmux-msg, kuu.mbt 等) への横展開判断は半年後に再評価
