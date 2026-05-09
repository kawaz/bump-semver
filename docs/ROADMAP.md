# bump-semver Roadmap

「必要が出たら 1 行追加」方針 (DR-0001 + DR-0005) に従い、以下は **見えている候補** であって即座の実装対象ではない。実需が出たら DR を立てて追加する。

## 候補ハンドラ

DR-0005 の path-aware confidence ranked テーブルにより、新フォーマット追加は基本「`rules.go` のテーブルに 1 行追加」で済む。新 format (yaml / xml / 独自) が必要なら `format_<name>.go` を 1 つ追加 + `tryRule` / `formatReplace` の switch に分岐を 1 行追加。

| basename / パス | format | 抽出パス |
|---|---|---|
| `pyproject.toml` | TOML | `.project.version` または `.tool.poetry.version` (try → fallback) |
| `Chart.yaml` | **yaml (新規)** | `.version` (Helm chart) |
| `bun.lock` | **jsonc (新規、JSONC parser)** | 仕様調査要 |
| `pnpm-lock.yaml` | **yaml (新規)** | `.importers["."].version` 等、仕様調査要 |
| `Cargo.lock` | TOML | `[[package]]` 配列の自パッケージ突き合わせ (path 表現拡張要) |
| `*.gemspec` | Ruby (regex) | `s.version = '...'` 行 |
| `setup.py` / `setup.cfg` | Python | `version = ...` (cfg) / `version='...'` (py) |
| `composer.json` | JSON | 既に `*.json` fallback で対応済 |
| `mix.exs` | Elixir (regex) | `version: "..."` |
| `build.sbt` | Scala (regex) | `version :=` |
| `pom.xml` | **xml (新規、encoding/xml)** | `<version>` |

これらは **すべて実需が出たら単独の DR で判断**。網羅は捨てる方針 (DR-0001)。

### 未対応フォーマット候補

現状の `format=json/toml/plain` 3 つに加えて、実需順の追加候補:

- **yaml** (`gopkg.in/yaml.v3`): Helm Chart.yaml / pnpm-lock.yaml / GitHub Actions workflows 等
- **jsonc** (JSON with comments / trailing commas): Bun bun.lock / VS Code 系 settings.json 等
- **xml** (標準 `encoding/xml`): Maven `pom.xml` / Android Gradle 系

`jsonpath.go` の path 抽出は `map[string]interface{}` ベースなので yaml.v3 の Unmarshal 結果でもそのまま使える見込み。yaml 対応は format_yaml.go 1 ファイル + rules.go の switch 1 行追加で済む。

## 機能候補

### pre-release / build metadata 対応

`1.2.3-alpha.1+build.42` 形式。MVP では `-` / `+` 含む入力をエラーで弾いている (DR-0001 不採用案 D + DR-0003 不採用案 D)。kawaz の現用途では未使用。要望が出たら semver パッケージ (`golang.org/x/mod/semver` 等) を導入して対応。

なお prefix (`v` / `ver` / `version`) と separator (`. _ -`) の柔軟化は **DR-0003 で対応済**。`v1.2.3` / `version_1_2_3` / `ver-1-2-3` などはそのまま受理し、bump 後も prefix と separator を保持する。

### Cargo workspace の `[workspace.package].version` 対応

`Cargo.toml` がワークスペースルートのとき、`[package]` は無く `[workspace.package].version` だけがある。MVP では非対応 (DR-0002)。DR-0005 の path-aware ルール体制で `[package]` フォールバック → `[workspace.package]` の優先順位で対応可能 (rules.go テーブル拡張)。

## 機能候補

### pre-release / build metadata 対応

`1.2.3-alpha.1+build.42` 形式。MVP では `-` / `+` 含む入力をエラーで弾いている (DR-0001 不採用案 D + DR-0003 不採用案 D)。kawaz の現用途では未使用。要望が出たら semver パッケージ (`golang.org/x/mod/semver` 等) を導入して対応。

なお prefix (`v` / `ver` / `version`) と separator (`. _ -`) の柔軟化は **DR-0003 で対応済**。`v1.2.3` / `version_1_2_3` / `ver-1-2-3` などはそのまま受理し、bump 後も prefix と separator を保持する。

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

### dispatch 構造の再評価 (format 数 10+ 時)

DR-0005 は `Format=string + switch` で 3 format を扱う。format が 10 を超えてきたら、`CandidateRule` に `Inspect func(...)` `Replace func(...)` の function field を持たせて switch を消す案 (closure 注入) を再評価する。今は宣言性 + 型安全のため switch を維持。

### バイナリ署名 / Notarization

macOS / Windows の配布バイナリに対する OS レベルの署名 / Notarization 検討。実需 (`brew install` 後の Gatekeeper 警告) が出てから着手。

## ドキュメント

### 競合ツール比較

`cargo-bump` / `npm version` / `standard-version` / `bump-my-version` 等との差分を README または `docs/research/` に整理。kawaz/jj-worktree の justfile-template-research.md と整合性を取る。

### 使用例集

`docs/findings/` か `docs/MANUAL.md` で、`jj file show <rev> Cargo.toml | bump-semver get Cargo.toml` のような実用パターンを集める。
