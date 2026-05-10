# DR-0014: TOML section-scoped Replace の一般化と pyproject.toml / mojoproject.toml 対応

- Status: Accepted
- Date: 2026-05-11
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」)、DR-0005 (path-aware confidence ranked candidates)、DR-0011 (TOML top-level fallback)、ROADMAP の `pyproject.toml` / `mojoproject.toml` 候補

## Context

DR-0011 で TOML の top-level `version = "..."` 抽出 / 書き換え (`tomlReplaceTopLevel`) を導入した時点で「`pyproject.toml` の `[project].version` は将来別 DR で扱う」と明示的にスコープ外としていた (DR-0011 § 4)。

実需:

- `pyproject.toml`: PEP 621 (`[project]` セクション) が現代の Python パッケージ標準。Poetry 旧形式は `[tool.poetry]` セクションを使う。両方とも section-scoped で、top-level fallback では拾えない。
- `mojoproject.toml`: Modular Mojo の package manifest。`[workspace]` セクション内に `name` / `version` を持つ section-scoped 形式。

これらは「実需が出てから path-pinned ルール (confidence 3) を 1 行追加」(DR-0001 + DR-0005) の典型的なターゲット。DR-0011 で断った `[project].version` 対応をここで決着させ、TOML 専用の section-scoped Replace を一般化する。

現状の `format_toml.go` は section-scoped Replace を **2 経路ハードコード** で持っている:

1. `tomlReplaceCargoPackage`: `[package]` セクション内の `version = "..."` (Cargo.toml 専用)
2. `tomlReplaceTopLevel`: section ヘッダ前の top-level 領域 (DR-0011 fallback)

3 つ目以降のセクションを増やすたびに専用関数を増やすのは **設計が正しくない** (DR-0001「必要が出たら 1 行追加」の精神に反する: 1 行どころか 1 関数追加になる)。section path を引数に取る一般化関数 1 つにまとめる。

## Decision

### 1. TOML section-scoped Replace の一般化

`tomlReplaceInSection(content, sectionPath, newVersion)` を追加する:

```go
// sectionPath 例: "package" / "project" / "workspace" / "tool.poetry"
// "" を渡すと top-level (= DR-0011 の fallback)
func tomlReplaceInSection(content []byte, sectionPath, newVersion string) ([]byte, error)
```

実装方針 (既存の Cargo / top-level コードと同じ行ベース regex):

- `sectionPath == ""`: `cargoSectionStartRe` (`^\s*\[`) で見つけた最初のセクションヘッダの直前までを top-level 領域として扱う
- `sectionPath != ""`: `[<sectionPath>]` のヘッダ行を `(?m)^\s*\[<sectionPath>\]\s*$` で検索し、その直後から次のセクションヘッダ (`^\s*\[`) までを section 領域として扱う
- 領域内の `^\s*version\s*=\s*(["'])([^"']*)(["'])` を書き換え (既存の `cargoVersionLineRe` を再利用)
- セクションが見つからない / version 行が無い場合はエラー

既存の `tomlReplaceCargoPackage` / `tomlReplaceTopLevel` は **`tomlReplaceInSection` を呼ぶ thin wrapper として残す** か、そのまま統合する。本 DR では **統合する** (両者が完全に一般化のサブセットなので、wrapper を残すと dead code 候補になる)。dispatch は `tomlReplace` の switch で path string → section path 変換 1 行で済む。

### 2. TOML 形式での VersionPaths の OR 試行 (try-fallback)

`pyproject.toml` は PEP 621 (`[project].version`) と Poetry 旧形式 (`[tool.poetry].version`) のどちらかを持つ。両方持つのは異常 (利用者責任)。ルール 1 行で複数 path を試行する仕組みは既に `package-lock.json` で `VersionPaths []string` として導入されている。

ただし semantics が異なる:

| 形式 | VersionPaths の解釈 | 不一致時 |
|---|---|---|
| JSON (`package-lock.json`) | **AND** (全部存在 + 全部同値が必要) | 整合性検証で mismatch エラー (DR-0004) |
| TOML (`pyproject.toml`) | **OR** (最初に見つかったものが採用) | N/A (1 つしか採用しない) |

→ **TOML format は OR semantics** にする。具体的には `tomlInspect` の loop を「最初に found=true になった path で確定して return」に変える。

これは TOML format 全体の semantics 変更で、既存の Cargo (`.package.version`) / top-level fallback (`.version`) は VersionPaths が 1 個だけなので AND/OR の差は出ない (純粋な後方互換)。

JSON format の AND semantics は package-lock.json 用に維持する (それが lock file の self-consistency 検査の自然な実装)。

### 3. pyproject.toml ルール (path-pinned, confidence 3)

```go
{
    Name:         "pyproject.toml",
    Basename:     "pyproject.toml",
    Confidence:   3,
    Format:       "toml",
    NamePaths:    []string{".project.name", ".tool.poetry.name"},
    VersionPaths: []string{".project.version", ".tool.poetry.version"},
},
```

- VersionPaths は OR semantics: PEP 621 を優先、次に Poetry 旧形式
- NamePaths も同様の OR (TOML format 全体で OR)。Names は optional なので片方だけ抽出できれば十分
- Replace は最初に extract できた path に対応する section だけ書き換える (両方持つ場合の同期は **MVP では skip**)

### 4. mojoproject.toml ルール (path-pinned, confidence 3)

```go
{
    Name:         "mojoproject.toml",
    Basename:     "mojoproject.toml",
    Confidence:   3,
    Format:       "toml",
    NamePaths:    []string{".workspace.name"},
    VersionPaths: []string{".workspace.version"},
},
```

シンプルな single-path ルール。Replace は `tomlReplaceInSection(content, "workspace", newVersion)`。

### 5. tomlReplace の dispatch

`tomlReplace` は現状 `switch rule.VersionPaths[0]` で hard-code 分岐していた。section-scoped 一般化に伴い、以下に変える:

1. `tomlInspect` を再実行して、どの VersionPath が hit したかを再決定する (cheap)
2. hit した path から section path を導出 (`.project.version` → `"project"`, `.workspace.version` → `"workspace"`, `.tool.poetry.version` → `"tool.poetry"`, `.package.version` → `"package"`, `.version` → `""`)
3. `tomlReplaceInSection(content, sectionPath, newVersion)` を呼ぶ

「再実行で同じ path が hit する」前提は Inspect → Replace 間に content が変わらないので成立 (handler の使用パターン上、Replace は Inspect 後すぐに呼ばれる)。

### 6. 整合性チェック (両方の値が一致しているか) は MVP で skip

`pyproject.toml` で `[project].version` と `[tool.poetry].version` の両方が存在し、かつ値が異なる場合の検証はしない。理由:

- 両方持つこと自体が異常な状態 (PEP 621 移行中の中間状態としては理論的にありうる)
- 検出して mismatch エラーを出すのは便利だが、Inspect の return shape を変える必要がある (現状は `Versions []Field` で、AND セマンティクスの JSON と semantics 衝突)
- 必要が出たら別 DR で扱う (DR-0001 哲学)

利用者責任: 両方持つ pyproject.toml では bump-semver は最初に hit した方 (`[project].version`) しか書き換えない。

### 7. ROADMAP の更新

`docs/ROADMAP.md` の「未対応フォーマット候補」テーブルから `pyproject.toml` / `mojoproject.toml` 行を削除し、Done セクションに本 DR への参照を追加する。

## 不採用案

### A. TOML AST 完全 round-trip

`BurntSushi/toml.Unmarshal` で構造体に decode → 値を上書き → Marshal で書き戻す案。

却下理由:
- TOML AST round-trip は **コメント・空行・key 順序を保持できない** (`BurntSushi/toml` は preserve mode を持たない)
- 既存の Cargo / top-level 実装と同じく行ベース regex で十分かつ堅牢
- DR-0011 で同じ判断 (YAML round-trip 不採用) と整合性が取れる
- 将来 preserve 系の TOML library が登場すれば再評価する余地はある

### B. `[project]` のみ対応で `[tool.poetry]` 切り捨て

「PEP 621 が標準なので Poetry 旧形式は無視する」案。

却下理由:
- 実プロジェクトには `[tool.poetry]` のみのリポジトリが (移行コストの理由で) 数多く存在する
- 切り捨てると「pyproject.toml なのに `unsupported` で落ちる」現象が出てユーザを困惑させる
- try-fallback 1 行追加で吸収できるコストに対して、利益 (網羅性) が大きい

### C. 全 path から重複検証 + mismatch エラー

`pyproject.toml` で `[project].version` と `[tool.poetry].version` の両方を読み、値が異なれば mismatch エラーで落とす案 (DR-0004 multi-file 整合性検証の延長)。

却下理由:
- TOML format 全体の VersionPaths semantics を AND に倒すか OR に倒すかの選択になり、AND だと「pyproject.toml に `[project].version` だけある (典型) で fail」が起きるので不可
- "extracted のうち重複がある場合だけ検証する" という第三の semantics は Inspect の return shape 変更が必要で本 DR スコープ外
- 必要が出たら別 DR (検査専用のオプションフィールドを追加する形が自然)

### D. tomlReplaceInSection を引数 1 個 (CandidateRule) で呼ぶ設計

「Replace 関数に rule 全体と current を渡し、関数内で path → section 変換」する案。

却下理由:
- Replace の中で再 Inspect が必要 (どの VersionPath が hit したか不明) で、結局同じ計算量
- section path string を直接受け取る方が「section-scoped Replace」というドメイン関数として宣言性が高い
- テストも sectionPath 文字列を直接渡せば済むので独立に検証しやすい

### E. pyproject.toml の `dynamic = ["version"]` 対応

`[project] dynamic = ["version"]` で版を別ファイル (`__about__.py` 等) に逃がすパターン。

却下理由 (本 DR スコープ外):
- 別ファイル参照は format をまたぐ依存解決が必要で複雑
- 実際にこのパターンが使われている割合は低い (静的 pyproject.toml が支配的)
- 必要が出たら別 DR で `format=python-dynamic` のような新フォーマットとして検討

## Consequences

### 実装変更

- `src/format_toml.go`:
  - `tomlReplaceInSection(content, sectionPath, newVersion)` 新規 (一般化)
  - `tomlReplaceCargoPackage` / `tomlReplaceTopLevel` を削除し thin dispatch に統合
  - `tomlReplace` の dispatch を `path → section path` 変換ベースに置換 (Inspect の hit 情報から)
  - `tomlInspect` は OR semantics に変更 (最初の hit で return)
- `src/rules.go`: `pyproject.toml` / `mojoproject.toml` の path-pinned confidence 3 ルール 2 個追加
- `src/format_toml_test.go`: section-scoped inspect/replace、複数 section 同名 version 非干渉、section 不在エラーのテスト追加
- `src/handler_test.go`: pyproject.toml / mojoproject.toml の rule resolution テスト追加
- README / README-ja: 対応形式テーブルに 2 行追加 (confidence 3)
- UPGRADING.md: v0.10.x → v0.11.0 セクション追加 (純粋追加)
- ROADMAP.md: Done セクションに移動

### 後方互換

- CLI 表面は完全に維持
- 既存テスト (Cargo.toml の `[package].version`、`*.toml` fallback) は TOML format の OR semantics 化を経ても結果が変わらない (VersionPaths が 1 個だけのため)
- 既存挙動への副作用なし (純粋追加)

### v0.10.x → v0.11.0

新ルール追加 (path-pinned confidence 3 が 2 個増える) なので minor bump。`UPGRADING.md` に「v0.10.x → v0.11.0」セクションを追加。

### 後続の機能候補

- `pyproject.toml` の `dynamic = ["version"]` 対応 (別 DR、別フォーマット候補)
- `[tool.poetry]` と `[project]` 両方が存在する pyproject.toml で値が乖離している場合の mismatch 検証 (別 DR、Inspection の shape 変更必要)
- `*.pbxproj` の **複数 match を同期更新する format** (ROADMAP に既出、本 DR と直交)

## 関連

- DR-0001: 「必要が出たら 1 行追加」(本 DR は典型例: 2 行 + 一般化)
- DR-0005: path-aware confidence ranked dispatcher (本 DR の前提機構、confidence 3 path-pinned)
- DR-0011: TOML top-level fallback (本 DR で section-scoped に拡張、`tomlReplaceTopLevel` を一般化に統合)
- ROADMAP: `pyproject.toml` / `mojoproject.toml` 行を本 DR で消化 (Done に移動)
