# DR-0011: `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback 追加

- Status: Accepted
- Date: 2026-05-11
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」)、DR-0005 (path-aware confidence ranked candidates)、DR-0010 (confidence 1 fallback hint)

## Context

DR-0005 で導入した confidence 1 fallback は現状 `*.json` のみ。実需としては:

- Helm `Chart.yaml` の `.version` (生 YAML、top-level)
- GitHub Actions workflow / kustomization 系の `version: ...` メタデータ
- `pyproject.toml` の top-level / `[project].version`
- 各種 manifest YAML (top-level `version`)

これらは「basename 専用ルールを書くほどではないが、top-level `.version` を持つ規約は YAML / TOML の世界でも広く流通している」という JSON と同じ状況。`*.json` fallback の精神を YAML / TOML にも横展開すれば、テーブル 3 行追加で実需の大半を吸収できる。

完全未対応 (`unsupported file: foo.yaml`) のままにすると、利用者は `bump-semver` を諦めて自前 sed を書きに行ってしまう。fallback で動くようにしておけば、合わなかったケースだけが DR-0010 hint 経由で issue に上がってきて、必要に応じて高 confidence ルールに昇格できる。

## Decision

### 1. confidence 1 fallback を 3 形式に拡張

`rules.go` に以下の 3 行を追加する:

| 確度 | パターン | 形式 | version パス | name パス |
|---|---|---|---|---|
| **1** (既存) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (新) | `*.yaml` | YAML | `.version` | `.name` |
| **1** (新) | `*.yml` | YAML | `.version` | `.name` |
| **1** (新) | `*.toml` | TOML | `version` (top-level) | `name` (top-level) |

`*.yaml` と `*.yml` は形式上同じだが、glob は別エントリで持つ (利用者の慣習として両方が存在し、`*.{yaml,yml}` のような OR は CandidateRule に組み込まれていないため)。`*.json` 既存ルールと一貫した粒度。

### 2. 上位ルールが先に解決される (precedence)

新 fallback は最低確度なので、既存の confidence 3 ルールには影響しない:

- `Cargo.toml` (`[package].version`、confidence 3) → これまで通り。`*.toml` fallback には降りない
- `package.json` (confidence 3) → これまで通り
- 任意 dir の `Cargo.toml` も confidence 3 で解決 (basename match)

すなわち本 DR は **これまで `unsupported file:` だったケースだけを救う**。後方互換性は完全に保たれる。

### 3. YAML パース: gopkg.in/yaml.v3

実装は `gopkg.in/yaml.v3` の `yaml.Unmarshal` を使う:

- root が map のとき `map[string]interface{}` として decode → `jsonpath.go` の `jsonPathExtract` がそのまま動く
- multi-document YAML (`---` セパレータ) は **最初の document のみ対象**。Helm Chart.yaml や workflow ファイルは single document が普通で、複数 doc を 1 ファイルにまとめる Kubernetes manifest は version meta を持たないことが多い (将来必要なら別 DR で再検討)

依存追加の正当化:
- 標準ライブラリには YAML パーサが無い
- `yaml.v3` は kubernetes-sigs / helm / 各種 Go ツールが採用するデファクト
- v3.0.1 (現在最新) は MIT license。ライセンス互換問題なし
- バイナリサイズ増は約 600KB。`bump-semver` 全体 (~10MB) から見れば小さい

### 4. TOML top-level version

`format_toml.go` の既存 Cargo.toml ロジックは `[package]` セクション限定。`*.toml` fallback では top-level (セクションヘッダ無しエリア) の `version` を読む:

```toml
# pyproject.toml で [project] セクション以外に書かれるパターン
version = "1.2.3"
name = "my-package"

[some-other]
...
```

#### 実装方針

- **Inspect**: `toml.Unmarshal` 結果の `map[string]interface{}` を `jsonPathExtract(doc, ".version")` で抽出 (既存の path-extraction 機構をそのまま使う)
- **Replace**: 行ベース regex で **section ヘッダ前の領域** にある `^version = "..."$` を書き換え。具体的には `^\s*\[` が出現する前までを top-level 領域とみなす

Cargo.toml の `[package].version` ルール (path-pinned confidence 3) と top-level fallback (confidence 1) は **path で完全に区別される** (rule 単位に Replace ロジックが分岐)。共存可能。

#### `pyproject.toml` の `[project].version` は本 DR では未対応

`pyproject.toml` の典型は section-scoped `[project] version = "..."` だが、本 DR では top-level fallback のみ対応する。理由:

- top-level の場合は path-pinned ルール無しでも fallback でカバー可能
- section-scoped の `[project].version` は `pyproject.toml` 特化の path-pinned ルール (confidence 3) として将来追加するのが筋。`*.toml` fallback で section 探索まで踏み込むと、Cargo.toml-like なファイル全般が誤マッチするリスクがある
- 実需が出た時点で `pyproject.toml` 専用ルールを 1 行 + 必要なら top-level → `[project].version` の two-level fallback を rule 内で実装する別 DR を立てる

### 5. YAML Replace: 行ベース regex

`yaml.Marshal` で再シリアライズすると以下の問題がある:

- コメント (`# foo`) が消える
- key の順序が変わる
- block scalar / flow style / アンカー / エイリアスの形式が崩壊する可能性

これを避けるため、**行ベース regex** で `^version: "..."` または `^version: ...` を書き換える方式を取る。具体的には:

- 行頭 (インデント無し、top-level に限定) の `version:` を捕捉
- value 部はクオート有無を保持 (`"1.2.3"` / `'1.2.3'` / `1.2.3` のいずれも対応)
- インラインコメント (`version: 1.2.3 # comment`) は保持

最初に発見した行のみ書き換える (multi-document の保険にもなる)。

### 6. テスト戦略

- `format_yaml_test.go` 新規: top-level `.version` の Inspect / Replace、クオート種別保持、コメント保持
- `format_toml_test.go` 新規: top-level `version` の Inspect / Replace、Cargo.toml 既存ロジックとの非干渉
- `handler_test.go` 拡張: `*.yaml` / `*.yml` / `*.toml` が confidence 1 で解決される
- `spec_table_test.go` の DR-0010 hint テーブル拡張: yaml / yml / toml fallback でも `hint:` が出る
- 既存の `unknown.toml` を使った unsupported-file テスト (`hint_test.go`) は **`unknown.xml` 等に変更** (xml は本 DR でも未対応のまま)

### 7. unsupported file エラーの誘導文言は維持

`*.xml` / `*.gemspec` 等は引き続き `unsupported file:` でエラー。DR-0010 の issue 誘導文言はそのまま機能する。

## 不採用案

### A. `pyproject.toml` の `[project].version` まで本 DR で対応

「top-level → `[project].version` → `[tool.poetry].version` の三段 fallback を rule 内で持つ」案。検討したが、本 DR のスコープを最小化するため見送り。`pyproject.toml` 専用ルール (path-pinned confidence 3) として別 DR で扱うのが構造的に正しい。

### B. YAML を `yaml.Marshal` で書き戻す

コメント保持・key 順序保持の都合で却下。`yaml.v3` には preserve mode 相当の機能が無い。行 regex で十分かつ堅牢。

### C. `*.yaml` と `*.yml` を 1 ルールに統合

`Glob` を 2 つ持てる構造にする案。CandidateRule のシンプルさを崩すコストに見合わない (3 行が 2 行になるだけ)。テーブル粒度を維持。

### D. Multi-document YAML 対応

`yaml.NewDecoder` で全 document を順次走査する案。Helm Chart.yaml / workflows は single document が普通で、現在の実需に存在しない。複雑性が増えるだけ。必要になった時点で別 DR。

### E. 完全未対応のまま放置

「YAML / TOML はそれぞれ巨大な仕様空間で `*.json` 同様の単純 fallback は当てにならない」という保守的な立場。検討したが、

- top-level `.version` という規約は YAML / TOML の世界でも JSON と同程度に普及している
- DR-0010 の hint 機構が「fallback で動いた」事実を可視化するので、誤マッチに気付く動線がある
- 完全未対応のままだと自前 sed の沼にユーザを送り込むだけ

として却下。

## Consequences

### 実装変更

- 依存追加: `gopkg.in/yaml.v3 v3.0.1` (go.mod / go.sum)
- `src/rules.go`: confidence 1 ルール 3 個追加 (`*.yaml`, `*.yml`, `*.toml`)。`tryRule` / `formatReplace` の switch に `"yaml"` 分岐追加
- `src/format_yaml.go` 新規: yaml.Unmarshal → jsonPathExtract で Inspect、行 regex で Replace
- `src/format_toml.go` 拡張: top-level `version` の Inspect / Replace を追加 (既存 Cargo.toml 用ロジックと共存)
- テスト追加 (`format_yaml_test.go` / `format_toml_test.go` / 既存 `hint_test.go` / `handler_test.go` の拡張)
- README / README-ja / UPGRADING の更新

### 後方互換

- CLI 表面は完全維持
- 既存テストへの影響: `unknown.toml` を使っている unsupported file テストは `unknown.xml` に変更 (3 箇所、`hint_test.go`)
- これまで `unsupported file:` でエラーだった `*.yaml` / `*.yml` / `*.toml` のうち、top-level `.version` を持つものは新たに動く (純粋追加)

### v0.7.x → v0.8.0

新フォーマット追加なので minor bump (SemVer 0.x.y 慣習: 後方互換が保たれていても新規形式追加は minor)。`UPGRADING.md` に「v0.7.x → v0.8.0」セクションを追加。

### ROADMAP

`docs/ROADMAP.md` 「未対応フォーマット候補」から **yaml** と **toml fallback 部分** を除外。`pyproject.toml` の `[project].version` と Helm Chart.yaml の `[chart].version` は依然として候補に残す (path-pinned confidence 3 ルール候補として)。

### 次のフェーズ

DR-0010 と本 DR で「fallback マッチ → hint → 実需に基づく path-pinned 昇格」のサイクルが整う。次の suffix 吸収 DR (`docs/issue/2026-05-10-suffix-stripped-format-detection.md`) で同じ hint 機構を再利用予定。

## 関連

- DR-0001: 「必要が出たら 1 行追加」哲学
- DR-0005: confidence ranked dispatcher (本 DR の前提機構)
- DR-0010: confidence 1 fallback hint (本 DR で 3 形式に横展開)
- ROADMAP: yaml / toml の未対応欄を本 DR で部分的に消化
- 関連 issue: `docs/issue/2026-05-10-suffix-stripped-format-detection.md` (suffix 吸収、別途)
