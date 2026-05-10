# bump-semver Roadmap

「必要が出たら 1 行追加」方針 (DR-0001 + DR-0005) に従い、以下は **見えている候補** であって即座の実装対象ではない。実需が出たら DR を立てて追加する。

## Done (実装済)

過去ロードマップから移送。実装履歴の参考用に残す。

### `--version --json` 対応 (v0.7.1)

`bump-semver --version --json` で自バイナリのバージョンを `--json` と同じ構造化スキーマで出力 (`jq -r .semver` 等で取り回せる)。`--version` 単独は従来通り `vX.Y.Z` プレーン出力。`--version` に `--json` 以外のフラグ / 位置引数を渡すとエラー (silent ignore を排除)。

### `vcs:` 入力モード (v0.7.0 / DR-0008)

`vcs:REV[:FILE]` / `vcs:latest-tag()` で jj/git の他リビジョン・最新 tag を入力として受け付ける。VCS は `--vcs` / `BUMP_SEMVER_VCS` / `.jj` / `.git` の優先順で自動判定 (`.jj` と `.git` 並存時は jj 優先)。fetch は自動実行しない (副作用回避)。`--write` と排他 (vcs: は read-only)。FILE 省略時は位置順で最初の sibling から借用。

### `--json` 出力オプション (v0.6.0 / DR-0007)

`get` / `major` / `minor` / `patch` / `pre` で `--json` を受け付け、`name` / `version` / `semver` / `major` / `minor` / `patch` / `pre` / `pre_id` / `pre_rest` / `build_metadata` / `build_id` / `build_rest` を 1 行 JSON で出力。`compare` は exit code 主役の設計のため対象外。`version` (フォーマット保持) と `semver` (strict) の二本立て、pre/build は最初の `.` で分割した構造分解のみ提供 (意味判定は CLI 側で背負わない)。

### pre-release / build metadata 対応 (v0.5.0 / DR-0006)

`1.2.3-alpha.1+build.42` 形式を SemVer 2.0.0 仕様準拠でパース・bump・比較できるようになった。bump 時は default で drop、`--pre` / `--build-metadata` で明示的に再付与する単一規則 (npm 流 strip-don't-bump とは異なる)。

### `compare` サブコマンド (v0.5.0 / DR-0006)

`compare {eq|lt|le|gt|ge}` で 2 つの INPUT (FILE / VER / `-`) を SemVer 2.0.0 順序で比較。終了コード `0`/`1`/`2` (`test` 慣習)。

### `pre` アクション (v0.5.0 / DR-0006)

pre-release counter advance / 上書き / 削除を `pre` アクション + `--pre` / `--no-pre` で操作。

### FILE | VER | `-` 統合 (v0.5.0 / DR-0006)

`--value` フラグを廃止し、位置引数で FILE パスと VER 文字列と `-` (stdin) を統一受理。

## 候補ハンドラ

DR-0005 の path-aware confidence ranked テーブルにより、新フォーマット追加は基本「`rules.go` のテーブルに 1 行追加」で済む。新 format (yaml / xml / 独自) が必要なら `format_<name>.go` を 1 つ追加 + `tryRule` / `formatReplace` の switch に分岐を 1 行追加。

| basename / パス | format | 抽出パス |
|---|---|---|
| `pyproject.toml` | TOML | `.project.version` または `.tool.poetry.version` (top-level fallback では section-scoped を拾えないので path-pinned 化が必要) |
| `mojoproject.toml` | TOML | `[workspace].version` (TOML section-scoped、現状未対応。`pyproject.toml` と一緒に section-scoped 対応で吸収) |
| `Chart.yaml` | YAML | `.version` (現状は `*.yaml` fallback で動く。Helm chart 専用 path-pinned 化は実需次第) |
| `setup.py` / `setup.cfg` | Python | `version = ...` (cfg) / `version='...'` (py) |
| `composer.json` | JSON | 既に `*.json` fallback で対応済 |
| `pom.xml` | **xml (新規、encoding/xml)** | `<version>` |
| `*.pbxproj` (Xcode) | **複数同期更新 format (新規)** | 同一ファイル内の複数 `MARKETING_VERSION` を一括更新 |
| `Info.plist` (iOS/macOS) | **xml/plist (新規)** | `CFBundleShortVersionString` + `CFBundleVersion` の二重管理 |

これらは **すべて実需が出たら単独の DR で判断**。網羅は捨てる方針 (DR-0001)。

### 対応対象外 (DR-0009)

以下の lock files は **自プロジェクトの version を保持しない** ため bump-semver の対象外:

- `bun.lock` (root の `version` フィールドが実運用で欠落、加えて JSONC パーサ依存が必要)
- `pnpm-lock.yaml` (`importers["."]` は依存のみ、self-version を持たない設計)
- `yarn.lock` (classic は self-entry なし、Berry は sentinel `0.0.0-use.local`)
- `Cargo.lock` (`[[package]]` に self version はあるが、`cargo check` で自動同期するため bump 対象外)

詳細は [DR-0009](./decisions/DR-0009-lockfile-support-scope.md) 参照。

### 未対応フォーマット候補

現状の `format=json/toml/yaml/plain/regex` 5 つに加えて、実需順の追加候補:

- **jsonc** (JSON with comments / trailing commas): Bun bun.lock / VS Code 系 settings.json 等
- **xml** (標準 `encoding/xml`): Maven `pom.xml` / Android Gradle 系
- **plist** (Apple バイナリ/XML plist): `Info.plist` の `CFBundleShortVersionString` 等
- **複数同期更新 format**: Xcode `*.pbxproj` の build settings 群を同期更新 (DR-0012 の regex format は 1 マッチ限定なのでスコープ外)

v0.8.0 (DR-0011) で `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback (top-level `.version`) を追加。v0.9.0 (DR-0012) で `regex` format を導入し `*.xcconfig` / `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `*.gemspec` / `mix.exs` / `build.sbt` の 8 種類を一括追加。v0.10.0 (DR-0013) で backup 系 suffix (`Cargo.toml.bak` / `package.json.20260510` / `Chart.yaml~` 等) を 1 段だけ剥がして既存ルールに通す suffix-stripped fallback を追加。section-scoped (`pyproject.toml` の `[project].version` / `mojoproject.toml` の `[workspace].version` 等) や nested YAML (`spec.version` 等) は今後の path-pinned ルール / TOML section-scoped Replace として実需に応じて追加する。

## 機能候補

### Cargo workspace の `[workspace.package].version` 対応

`Cargo.toml` がワークスペースルートのとき、`[package]` は無く `[workspace.package].version` だけがある。MVP では非対応 (DR-0002)。DR-0005 の path-aware ルール体制で `[package]` フォールバック → `[workspace.package]` の優先順位で対応可能 (rules.go テーブル拡張)。

### pre-release のラベル昇格 (alpha → beta → rc → stable)

poetry `--next-phase` 相当。`pre 1.2.3-alpha → 1.2.3-beta` のような順序昇格は v0.5.0 では非対応。需要が出れば追加検討 (DR-0006 スコープ外項目)。

### `sort` / `valid` アクション

複数 VER のソート (`sort` action) や、パース可能性チェックのみの `valid` action は v0.5.0 では非対応。`compare` 以外の比較系として将来検討 (DR-0006 スコープ外項目)。

### `--dry-run` の明示化

`--write` を付けない実行が事実上 dry-run になっているが、明示フラグを欲する声が出れば追加検討。現状は不要。

### glob 展開 / 複数ファイル一括 bump

`bump-semver patch '**/Cargo.toml' --write` のように複数ファイルを一括処理する案。kawaz の現用途では justfile 側で1ファイルずつ呼ぶ運用で足りており優先度低。

### `--from-stdin --to-stdout` 明示

stdin pipe 時に `--write` を禁じている現仕様の延長で、明示的なストリームモードを設けるか。現状の暗黙挙動 + `-` INPUT で足りているので不要寄り。

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
