# bump-semver Roadmap

「必要が出たら 1 行追加」方針 (DR-0001 + DR-0005) に従い、以下は **見えている候補** であって即座の実装対象ではない。実需が出たら DR を立てて追加する。

## Done (実装済)

過去ロードマップから移送。実装履歴の参考用に残す。

### Cargo workspace の `[workspace.package].version` 対応 (DR-0021)

ワークスペースルートの `Cargo.toml` は `[package]` を持たず version が `[workspace.package].version` にあり、メンバー crate が `version.workspace = true` で継承する。`Cargo.toml` ルールの `VersionPaths` / `NamePaths` を `[package]` → `[workspace.package]` の OR フォールバック (DR-0014 で確立した `pyproject.toml` と同形の first-match-wins) で拡張して対応。両方ある場合は crate 自身の `[package].version` が優先。マッチ path は `get` / `--json` / 診断に出るので、利用者は何を bump しているか確認できる (DR-0002 の「暗黙的すぎる」懸念への回答)。rules.go テーブルの 1 行拡張のみ、新 format / 新ハンドラ不要。DR-0002 を supersede。詳細は [DR-0021](./decisions/DR-0021-cargo-workspace-package-version.md)。

### JVM (Gradle) / .NET (csproj 系) / Maven (pom.xml) / Haskell (cabal) / RPM (spec) 対応 + 新 format `xml-element` (v0.14.0 / DR-0018)

`pom.xml` (Maven) と `*.csproj` / `*.fsproj` / `*.vbproj` (.NET MSBuild) のために、`<key>/<string>` ペア専用の既存 `xml` format とは別系統の slash-rooted XML path format `xml-element` を新設。`/project/version` のような element path で値を取得 / 書き換え (XML 名前空間は local name で比較、byte range splice で DOCTYPE / 属性順序 / インデント完全保持)。同時に DR-0012 の regex format を拡張して `build.gradle` (Groovy DSL の 3 形 `version = '...'` / `version "..."` / `version = "..."` を 1 regex で吸収)、`build.gradle.kts` (Kotlin DSL)、`*.cabal` (Haskell、`cabal-version:` と line-anchored で区別)、`*.spec` (RPM、capital V で `Name:` / `Release:` と区別) を path-pinned / basename / glob 各レイヤで追加。詳細は [DR-0018](./decisions/DR-0018-jvm-dotnet-haskell-rpm-support.md) を参照。

### `compare` の precision suffix (v0.13.0 / DR-0017)

`compare` OP に `-major` / `-minor` / `-patch` suffix を許可し、比較対象の component を切り詰めて評価する 5 base × 4 precision = **20 OP** に拡張。`eq-major 1.2.3 1.9.7` → true (同じ major)、`eq-patch 1.2.3 1.2.3-rc.1` → true (pre-release 無視) など。CI で「メジャー upgrade を検知したい」「pre-release 違いは無視して同じ release version か知りたい」用途を 1 行で表現できる。`Version.CompareAt(other, precision)` で precision-aware 比較を提供し、既存 `Compare(other)` は `CompareAt(other, "")` への薄いラッパー化 (互換維持)。詳細は [DR-0017](./decisions/DR-0017-compare-precision-suffix.md)。

### `BUMP_SEMVER_VCS` 環境変数廃止 + `--vcs auto` 明示値 + help 3 段化 (v0.13.0 / DR-0016 + help 改修)

DR-0008 で導入した env による VCS 検出 override (`BUMP_SEMVER_VCS=jj|git`) を廃止し、`--vcs jj|git|auto` フラグ単独で制御する形に整理 (一度 env を export すると CLI から auto detect に戻せない罠の解消 + help セクション圧縮)。`auto` を default 明示値として許可。あわせて help を 3 段化: `--help` (短)、`--help-full` (完全リファレンス)、`bump-semver <action> --help` (action 固有: helpBump / helpPre / helpGet / helpCompare)。help 定数を `src/help.go` に分離。詳細は [DR-0016](./decisions/DR-0016-remove-bump-semver-vcs-env.md) を参照。

### Xcode `project.pbxproj` (multi-match 同期) + `Info.plist` (XML plist) (v0.12.0 / DR-0015)

`format_pbxproj.go` を新設し、Xcode の `<project>.xcodeproj/project.pbxproj` の OpenStep plist 内に複数行ある `MARKETING_VERSION = ...;` を **同期更新** する形式を実装した。Inspect は全マッチを `line:N` Path 付きで返し、不一致時は main.go 既存の `formatMismatchError` で column-aligned に表示される。`format_xml.go` を新設し、`Info.plist` (XML plist) の `<key>CFBundleShortVersionString</key><string>...</string>` ペアを `encoding/xml` Decoder で位置特定 + byte range 書き換えで処理 (DOCTYPE / インデント / 兄弟 key 完全保持)。Xcode 11+ の `$(MARKETING_VERSION)` placeholder は ParseVersion 失敗 → `unsupported file:` で落ちるのが自然な振る舞い。`CFBundleVersion` (build number) はスコープ外。詳細は [DR-0015](./decisions/DR-0015-pbxproj-and-info-plist.md) を参照。

### TOML section-scoped Replace + `pyproject.toml` / `mojoproject.toml` (v0.11.0 / DR-0014)

`format_toml.go` の Replace を section-scoped 一般化 (`tomlReplaceInSection`) し、`pyproject.toml` (`[project].version` (try) → `[tool.poetry].version` の OR fallback) と `mojoproject.toml` (`[workspace].version`) を path-pinned confidence 3 ルールとして追加した。TOML format 全体の VersionPaths semantics を「first-match-wins (OR)」に変更 (JSON は AND 維持)。両方のセクションを持つ pyproject.toml は最初の hit だけ書き換える MVP 仕様。詳細は [DR-0014](./decisions/DR-0014-toml-section-scoped.md) を参照。

### `--version --json` 対応 (v0.7.1)

`bump-semver --version --json` で自バイナリのバージョンを `--json` と同じ構造化スキーマで出力 (`jq -r .semver` 等で取り回せる)。`--version` 単独は従来通り `vX.Y.Z` プレーン出力。`--version` に `--json` 以外のフラグ / 位置引数を渡すとエラー (silent ignore を排除)。

### `vcs:` 入力モード (v0.7.0 / DR-0008)

`vcs:REV[:FILE]` で jj/git の他リビジョンの内容を入力として受け付ける。VCS は `--vcs jj|git|auto` フラグ → `.jj` → `.git` の優先順で自動判定 (`.jj` と `.git` 並存時は jj 優先)。fetch は自動実行しない (副作用回避)。`--write` と排他 (vcs: は read-only)。FILE 省略時は位置順で最初の sibling から借用。`BUMP_SEMVER_VCS` 環境変数は v0.13 で廃止 (DR-0016)。最新 tag 取得は v0.29.0 で `vcs:latest-tag([REPO])` 入力から `vcs tag latest` サブコマンドへ移行 (DR-0020 PR-Tag-Latest)。

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

現状の `format=json/toml/yaml/plain/regex/pbxproj/xml/xml-element` 8 つに加えて、実需順の追加候補:

- **jsonc** (JSON with comments / trailing commas): Bun bun.lock / VS Code 系 settings.json 等
- **`CFBundleVersion` (Xcode build number)**: SemVer ではなく整数 / build hash / commit count なので bump-semver スコープ外。CI で別途埋めるのが慣例
- **mixed-content XML / CDATA**: `xml-element` は inner text を byte range で splice する単純な実装。複雑な XML (CDATA / mixed content) を扱うには別 format / オプション拡張が必要

v0.8.0 (DR-0011) で `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback (top-level `.version`) を追加。v0.9.0 (DR-0012) で `regex` format を導入し `*.xcconfig` / `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `*.gemspec` / `mix.exs` / `build.sbt` の 8 種類を一括追加。v0.10.0 (DR-0013) で backup 系 suffix (`Cargo.toml.bak` / `package.json.20260510` / `Chart.yaml~` 等) を 1 段だけ剥がして既存ルールに通す suffix-stripped fallback を追加。v0.11.0 (DR-0014) で TOML section-scoped Replace を一般化し `pyproject.toml` (`[project].version` + `[tool.poetry].version` try-fallback) / `mojoproject.toml` (`[workspace].version`) を path-pinned 化。v0.12.0 (DR-0015) で `project.pbxproj` (multi-match 同期 + 不一致 mismatch 出力) と `Info.plist` (XML plist の byte-range 書き換え) を path-pinned 化し `pbxproj` / `xml` の 2 format を新設。v0.14.0 (DR-0018) で JVM (Gradle Groovy/Kotlin DSL) / .NET MSBuild (`*.csproj` / `*.fsproj` / `*.vbproj`) / Maven (`pom.xml`) / Haskell (`*.cabal`) / RPM (`*.spec`) を一括追加し、新 format `xml-element` (slash-rooted XML path lookup) を導入。nested YAML (`spec.version` 等) や `pyproject.toml` の `dynamic = ["version"]` 等は実需に応じて追加する。

## 機能候補

### `vcs` サブコマンド群 (git/jj 吸収のリリース/push 定型操作)

Taskfile/justfile に jj/git 分岐を毎回手書きする板挟みを解消する、git/jj 共通の最小サブセットを吸収するサブコマンド群。`vcs get root|backend|current-branch` / `vcs is <pred>` / `vcs diff` / `vcs commit [--staged|--amend]` / `vcs push --branch|--bookmark` / `vcs tag push --rev REV NAME [--allow-move]` / `vcs tag delete` (冪等)。設計哲学・各仕様・jj 一次情報調査は [DR-0020](./decisions/DR-0020-vcs-subcommands.md) と [journal](./journal/2026-05-30-vcs-subcommands-design.md) に確定済み (jj v0.35+ 前提)。実装は read 系 (get/is/diff) → commit/push → tag (jj export・immutability 連動) の順。

### pre-release のラベル昇格 (alpha → beta → rc → stable)

poetry `--next-phase` 相当。`pre 1.2.3-alpha → 1.2.3-beta` のような順序昇格は v0.5.0 では非対応。需要が出れば追加検討 (DR-0006 スコープ外項目)。

### `sort` / `valid` アクション

複数 VER のソート (`sort` action) や、パース可能性チェックのみの `valid` action は v0.5.0 では非対応。`compare` 以外の比較系として将来検討 (DR-0006 スコープ外項目)。

### `--dry-run` の明示化

`--write` を付けない実行が事実上 dry-run になっているが、明示フラグを欲する声が出れば追加検討。現状は不要。

### glob 展開 / 複数ファイル一括 bump

`bump-semver patch '**/Cargo.toml' --write` のように複数ファイルを一括処理する案。kawaz の現用途では Taskfile.pkl の bump-version task で1ファイルずつ呼ぶ運用で足りており優先度低。

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
