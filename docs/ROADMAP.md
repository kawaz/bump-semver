# bump-semver Roadmap

「必要が出たら handler を 1 つ追加」方針 (DR-0001) に従い、以下は **見えている候補** であって即座の実装対象ではない。実需が出たら DR を立てて追加する。

## 候補ハンドラ

実需が見えたら `src/handler_<name>.go` + テストを追加するだけで対応可能な見込みのもの。basename パターンと取得経路だけ整理。

| basename | フォーマット | 取得経路 |
|---|---|---|
| `pyproject.toml` | TOML | `[project].version` または `[tool.poetry].version` |
| `*.gemspec` | Ruby | `s.version = '...'` 行 (regex) |
| `setup.py` / `setup.cfg` | Python | `version = ...` (cfg) / `version='...'` (py) |
| `Chart.yaml` | YAML | `.version` (Helm chart) |
| `composer.json` | JSON | 既に `*.json` で対応済 |
| `mix.exs` | Elixir | `version: "..."` (regex) |
| `build.sbt` | Scala | `version :=` (regex) |
| `pom.xml` | XML | `<version>` (要 XML パース) |

これらは **すべて実需が出たら一括ではなく単独の DR で判断**。網羅は捨てる方針 (DR-0001)。

## 機能候補

### pre-release / build metadata 対応

`1.2.3-alpha.1+build.42` 形式。MVP では `-` / `+` 含む入力をエラーで弾いている (DR-0001 不採用案 D)。kawaz の現用途では未使用。要望が出たら semver パッケージ (`golang.org/x/mod/semver` 等) を導入して対応。

### Cargo workspace の `[workspace.package].version` 対応

`Cargo.toml` がワークスペースルートのとき、`[package]` は無く `[workspace.package].version` だけがある。MVP では非対応 (DR-0002)。実需が出れば `cargoHandler` 内で `[package]` フォールバック → `[workspace.package]` の優先順位で対応可能。

### `--dry-run` の明示化

`--write` を付けない実行が事実上 dry-run になっているが、明示フラグを欲する声が出れば追加検討。現状は不要。

### glob 展開 / 複数ファイル一括 bump

`bump-semver patch '**/Cargo.toml' --write` のように複数ファイルを一括処理する案。kawaz の現用途では justfile 側で1ファイルずつ呼ぶ運用で足りており優先度低。

### `--from-stdin --to-stdout` 明示

stdin pipe 時に `--write` を禁じている現仕様の延長で、明示的なストリームモードを設けるか。現状の暗黙挙動で足りているので不要寄り。

## CI / リリース

### GitHub Actions の Node.js 24 移行

`actions/setup-go@v5` / `actions/upload-artifact@v4` / `extractions/setup-just@v3` が Node.js 20 deprecation 警告を出している。2026-09 の強制移行までに新バージョンへ更新。

### linter 強化

`gofmt -w .` + `go vet ./...` のみ。`staticcheck` / `golangci-lint` を追加するか検討余地あり。kawaz/* の Go プロジェクト方針と合わせる。

### バイナリ署名 / Notarization

macOS / Windows の配布バイナリに対する OS レベルの署名 / Notarization 検討。実需 (`brew install` 後の Gatekeeper 警告) が出てから着手。

## ドキュメント

### 競合ツール比較

`cargo-bump` / `npm version` / `standard-version` / `bump-my-version` 等との差分を README または `docs/research/` に整理。kawaz/jj-worktree の justfile-template-research.md と整合性を取る。

### 使用例集

`docs/findings/` か `docs/MANUAL.md` で、`jj file show <rev> Cargo.toml | bump-semver get Cargo.toml` のような実用パターンを集める。
