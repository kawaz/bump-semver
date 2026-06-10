# bump-semver Roadmap

「必要が出たら 1 行追加」方針 (DR-0001 + DR-0005) に従い、以下は **見えている候補** であって即座の実装対象ではない。実需が出たら DR を立てて追加する。

## 候補ハンドラ

DR-0005 の path-aware confidence ranked テーブルにより、新フォーマット追加は基本「`rules.go` のテーブルに 1 行追加」で済む。新 format (yaml / xml / 独自) が必要なら `format_<name>.go` を 1 つ追加 + `tryRule` / `formatReplace` の switch に分岐を 1 行追加。

| basename / パス | format | 抽出パス |
|---|---|---|
| `Chart.yaml` | YAML | `.version` (現状は `*.yaml` fallback で動く。Helm chart 専用 path-pinned 化は実需次第) |
| `setup.py` / `setup.cfg` | Python | `version = ...` (cfg) / `version='...'` (py) |
| `composer.json` | JSON | 既に `*.json` fallback で対応済 |
| `pubspec.yaml` (Dart/Flutter) | YAML | `version: ...` (現状は `*.yaml` fallback で動く。path-pinned 化は実需次第) |
| `*.csproj` の `<AssemblyVersion>` / `<FileVersion>` | xml-element 拡張 | 現状は `<Version>` のみ対応 (DR-0018)、複数 version field の同期は実需次第 |

これらは **すべて実需が出たら単独の DR で判断**。網羅は捨てる方針 (DR-0001)。

### 対応対象外 (DR-0009)

以下の lock files は **自プロジェクトの version を保持しない** ため bump-semver の対象外:

- `bun.lock` (root の `version` フィールドが実運用で欠落、加えて JSONC パーサ依存が必要)
- `pnpm-lock.yaml` (`importers["."]` は依存のみ、self-version を持たない設計)
- `yarn.lock` (classic は self-entry なし、Berry は sentinel `0.0.0-use.local`)
- `Cargo.lock` (`[[package]]` に self version はあるが、`cargo check` で自動同期するため bump 対象外)

詳細は [DR-0009](./decisions/DR-0009-lockfile-support-scope.md) 参照。

### 未対応フォーマット候補

現状の `format=text/json/yaml/toml/xml/xml-element/pbxproj` 7 つに加えて、実需順の追加候補:

- **jsonc** (JSON with comments / trailing commas): Bun bun.lock / VS Code 系 settings.json 等
- **`CFBundleVersion` (Xcode build number)**: SemVer ではなく整数 / build hash / commit count なので bump-semver スコープ外。CI で別途埋めるのが慣例
- **mixed-content XML / CDATA**: `xml-element` は inner text を byte range で splice する単純な実装。複雑な XML (CDATA / mixed content) を扱うには別 format / オプション拡張が必要

nested YAML (`spec.version` 等) や `pyproject.toml` の `dynamic = ["version"]` 等は実需に応じて追加する。

## 機能候補

### pre-release のラベル昇格 (alpha → beta → rc → stable)

poetry `--next-phase` 相当。`pre 1.2.3-alpha → 1.2.3-beta` のような順序昇格は現状非対応。需要が出れば追加検討 (DR-0006 スコープ外項目)。

### `sort` / `valid` アクション

複数 VER のソート (`sort` action) や、パース可能性チェックのみの `valid` action は現状非対応。`compare` 以外の比較系として将来検討 (DR-0006 スコープ外項目)。

### `--dry-run` の明示化

`--write` を付けない実行が事実上 dry-run になっているが、明示フラグを欲する声が出れば追加検討。現状は不要。

### glob 展開 / 複数ファイル一括 bump

`bump-semver patch '**/Cargo.toml' --write` のように複数ファイルを一括処理する案。justfile の bump-version task で 1 ファイルずつ呼ぶ現運用で足りており優先度低。

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
