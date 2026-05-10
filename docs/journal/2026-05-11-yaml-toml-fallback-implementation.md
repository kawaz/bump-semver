# v0.8.0 実装ジャーナル: `*.yaml` / `*.yml` / `*.toml` confidence 1 fallback

DR-0011 の実装記録。設計判断の経緯と実装中に出た細部の判断を残す。

## 背景

DR-0010 で `*.json` fallback hint が動くようになり、「fallback で動く → hint で issue 化を促す → 必要に応じて path-pinned 昇格」のフィードバックループが整った。次の自然な拡張として YAML / TOML を `*.json` 並みに格上げする。

実需としては Helm `Chart.yaml` / GitHub Actions / `pyproject.toml` の top-level `version` が念頭。

## 主要な判断

### 1. YAML パーサ依存の追加

`gopkg.in/yaml.v3` を依存に追加。標準ライブラリには YAML パーサがないので避けられない。バイナリは ~600KB 増えたが許容範囲。MIT ライセンスで `BurntSushi/toml` と整合性問題なし。

### 2. Replace は yaml.Marshal せず regex で書き換える

`yaml.Marshal` で round-trip するとコメント・key 順序・block スタイルが破壊される。`yaml.v3` には preserve mode 相当の機能がないので、行 regex で `^version: ...$` を捕捉してその value 部分だけを差し替える方針。

regex 設計の試行錯誤:

- 最初は `(?:"...")|(?:'...')|(?:bare)` の三段 alternation を一発でやろうとしたが、
  - 量化子 `[^"]*` の greedy/lazy 切り替えが厄介
  - submatch index の管理が複雑
  - bare scalar の trailing comment / whitespace ハンドリングが正規表現だけでは不安定
- → **value-tail (行末まで全部捕獲) → 後段 Go 関数で文字列スキャン** に分離。`yamlValueRange()` で `"..."` / `'...'` / bare を判定。可読性 / メンテ性で優位、テスト追加もしやすい。

### 3. TOML top-level Replace の section 境界

top-level `version = ...` を regex で捕捉するとき、section 配下の `version = ...` を間違って拾わないために **content の先頭から最初の `[section]` ヘッダまで** を `region` として切り出してから regex 適用。`cargoSectionStartRe` を Cargo 用ロジックと共有。

`tomlReplace` が path で分岐 (`.package.version` / `.version`) して 2 関数を呼び分ける構造にした。Cargo.toml は依然 confidence 3 path-pinned で先に解決されるので干渉なし。

### 4. `*.yaml` と `*.yml` を別エントリに

CandidateRule の Glob は単一文字列。`*.{yaml,yml}` のような alternation を入れると pathMatches 側に拡張が要る。テーブル粒度を維持して 2 行で書くほうがシンプル。

### 5. `[project].version` (section-scoped) は本 DR では未対応

pyproject.toml の典型は section-scoped。これを `*.toml` fallback で拾えるようにすると、Cargo.toml-like なファイルが誤マッチするリスクが上がる。section-scoped は path-pinned ルールとして将来別 DR で扱う。

### 6. `unknown.toml` を使っていた既存テストの更新

DR-0010 の hint_test.go で `unknown.toml` (= 未対応の代表) として扱っていたケースが、本 DR で `*.toml` fallback に解決されてしまうので、`unknown.xml` に変更。`xml` は本 DR でも未対応のまま。3 箇所一括 (`replace_all` で楽)。

## 実装ハマり所

### tomlDisplayPath の単一セグメント挙動

既存の `tomlDisplayPath` は `len(segs) < 2` なら raw path をそのまま返していた (`.version` のまま)。top-level fallback 用には `version` (leading dot なし) のほうが TOML 慣習的に自然なので、`len(segs) == 1` のケースだけ `segs[0]` を返すように修正。display 用ヘルパなので Inspect 結果の `Path` 表示にだけ影響する。

### YAML 単一引用符のエスケープ

YAML の single-quote は `''` でエスケープ表現。`yamlValueRange` で素朴に最初の `'` で止めると `version: 'a''b'` のようなケースで早すぎる切断になるので、`''` を skip するロジックを入れた。実用上ほぼ出ない (version 文字列に single quote は入らない) が将来の保険。

### nested `version:` の column-0 アンカー

YAML は indentation で nesting するので、`(?m)^version:` (行頭) にすれば nested は自動的に弾ける。テストでガード済 (`TestYamlReplace_DoesNotTouchNestedVersion`)。

## 今後の拡張余地

- `pyproject.toml` 専用 path-pinned ルール (section-scoped `[project].version`)
- Helm Chart.yaml 専用ルール (`apiVersion: v2` / `kind: Chart` を見て信頼度を上げる)
- 多 document YAML の選択 (`---` 区切りで n 番目のドキュメント、Kustomize 等)

いずれも実需が出てから別 DR で。

## 関連

- DR-0011 (本作業の判断記録)
- DR-0005 (前提となる confidence ranked dispatcher)
- DR-0010 (fallback hint 機構、本作業で 3 形式に横展開)
- 次フェーズ: `docs/issue/2026-05-10-suffix-stripped-format-detection.md` (suffix 吸収)
