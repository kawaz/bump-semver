# bug: `get Cargo.toml` が GitHub Actions ubuntu-latest でのみ fallback rule に落ちて失敗する

- Date: 2026-06-10
- Status: open
- Priority: 高 (= kawaz/hyoui の release workflow を v0.2.6 以降ずっと止めていた。hyoui 側は
  workaround (perl 直読み fallback) 済みだが、`get` / `compare` を CI で使う全リポに波及しうる)
- 報告者: kawaz/hyoui の release 復旧作業中に発見 (Claude main session)

## 現象

workspace-root layout の Cargo.toml (`[workspace.package].version` のみ、`[package]` なし) に対する
`bump-semver get Cargo.toml --no-hint` が、**GitHub Actions ubuntu-latest 上でのみ**
`bump-semver: Cargo.toml: *.toml (fallback): missing version` (exit 2) で失敗する。

DR-0021 の workspace.package fallback (`939205ef`, 2026-05-30) を含む v0.34.1 / v0.35.0 の
両 release binary で再現。**同一 sha256 の binary + 同一 sha256 の入力**が、Actions 外では成功する。

## 再現マトリクス (2026-06-10 検証)

対象入力: kawaz/hyoui の Cargo.toml (workspace root, `[workspace.package] version = "0.3.1"`)。
sha256: `ad1792b1d52813fa8d82224aeb8ad61e5b653b87a99e38ffbb04c1e8daf6def5`。
od ダンプも Actions 上で取得済みで正常 (BOM 無し、LF、`[workspace]` → `[workspace.package]` → `version = "0.3.1"`)。

| 実行環境 | binary | sha256 一致 | 結果 |
|---|---|---|---|
| GitHub Actions ubuntu-latest (実 x86_64) | linux-amd64 v0.35.0 | `3930ac1c...` (基準) | **失敗** `*.toml (fallback): missing version` |
| GitHub Actions ubuntu-latest | linux-amd64 v0.34.1 | (未採取) | **失敗** (同エラー、run 27272488364 + rerun でも再現 = 決定的) |
| Docker linux/amd64 (Rosetta emu on macOS) + debian | linux-amd64 v0.35.0 | `3930ac1c...` **一致** | 成功 `0.3.1` |
| Docker linux/amd64 + debian | linux-amd64 v0.34.1 (release asset) | - | 成功 |
| Docker linux/amd64 + debian + git init 済み dir | 同上 | - | 成功 (.git 有無は無関係) |
| macOS arm64 (brew kawaz/tap) | darwin-arm64 v0.34.1 | - | 成功 |

過去の hyoui release run も全て同エラー:
- 2026-06-04 22:58Z (v0.2.6 リリース試行, run 26984604612) — 同エラー
- 2026-06-02 13:26Z (run 26822813695) — 未精査だが failure
- 2026-06-10 (v0.3.0 試行, run 27272488364 + 手動 rerun) — 同エラー
- 2026-06-10 (v0.3.1, run 27274719596) — `get` は同エラー、hyoui 側 perl fallback で release 自体は成功

## 棄却済みの仮説

- binary 差: sha256 完全一致で棄却
- 入力差: sha256 完全一致 + Actions 上の od ダンプ正常で棄却
- workspace 対応漏れ: `939205ef` (DR-0021 fallback) は v0.34.1/v0.35.0 の ancestor、
  かつ同 binary が Docker で `[workspace.package].version` を読めることを確認済み
- `.git` 有無 / ディレクトリコンテキスト: Docker で git init 有無・リポ全体 mount の
  両方を試し差なし
- env 依存の挙動分岐: `grep Getenv/environ src/*.go` で実体なし (コメントのみ)
- transient: 手動 rerun でも再現 = 決定的

## 残る仮説 (未検証)

1. **実 x86_64 CPU と Rosetta 2 エミュレーションの挙動差**: Docker 検証は全て
   aarch64 ホスト上の linux/amd64 エミュ。Go ランタイム or 依存 toml ライブラリに
   CPU feature 依存の分岐があり、実 CPU でのみバグ path を踏む可能性。
   → 検証方法: bump-semver リポに ubuntu-latest 上で workspace-root Cargo.toml の
   `get` を回す再現 CI job を作る (これが最速。再現すれば Actions 上で printf debug / bisect 可能)
2. resolveRule のエラー表示は「最後に試した rule (= fallback)」のものなので、
   実際に失敗しているのは confidence-3 の Cargo.toml rule の Inspect。
   toml parse 自体か VersionPaths 評価のどちらで落ちているかを切り分けるログが現状ない
   → `get` に診断オプション (例: `--debug-rules` で各 rule の失敗理由を stderr に列挙) が
   あると今回のような環境依存問題の切り分けが一気に楽になる

## hyoui 側の workaround (参考)

`.github/workflows/release.yml` の check-version step で `get` 失敗時に
sha256/od 診断を出して `[workspace.package].version` を perl 直読みにフォールバック。
根因解消後に外す予定。

## 次のアクション

1. bump-semver リポに再現 CI job (ubuntu-latest + workspace-root Cargo.toml fixture) を追加
2. 再現したら Actions 上で rule ごとの失敗理由を printf debug → 根因特定
3. `--debug-rules` 相当の診断オプション追加を検討 (再発時の切り分け用)
