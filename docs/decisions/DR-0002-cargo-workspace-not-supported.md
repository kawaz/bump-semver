# DR-0002: Cargo workspace の `[workspace.package].version` を MVP では扱わない

- ステータス: Accepted
- 日付: 2026-05-09
- 関連: DR-0001 (flat 4-action + basename 形式判定), `src/handler_cargo.go`

## 文脈

Rust の Cargo は **ワークスペース** という概念を持ち、ワークスペースルートの `Cargo.toml` は典型的に次のような構造をとる:

```toml
[workspace]
members = ["crate-a", "crate-b"]
resolver = "2"

[workspace.package]
version = "1.0.0"
edition = "2021"
authors = ["kawaz"]
license = "MIT"
```

このとき:
- ワークスペースルート Cargo.toml は **`[package]` セクションを持たない** (持たないことが典型的構成)
- 各メンバー crate (`crate-a/Cargo.toml` など) は `[package]` を持ち、`version.workspace = true` でワークスペース版を継承する形

bump-semver の `cargoHandler.Get` / `Replace` は `[package].version` のみを対象としており、ワークスペースルートのファイルを渡すと「missing [package] section」または「missing [package].version line」でエラーになる。

## 決定

**MVP では `[workspace.package].version` を扱わない。**

ワークスペースルートに対する操作は明示的にエラーで弾き、メンバー crate の Cargo.toml に対してのみ動作する。

## 理由

1. **「網羅は捨てる、必要が出たら handler 追加」方針 (DR-0001)** との整合
   - kawaz の現用途 (`port-peeker` / `authsock-warden` / `stable-which` 等) はいずれもシングルクレート構成。Cargo workspace を採用する Rust プロジェクトを kawaz が運用し始めた時点で初めて実需が立ち上がる
2. **誤操作を防ぐ** ためにエラーで弾く方が安全
   - ワークスペースルートの `[workspace.package].version` を変えるとメンバー crate 全体に伝播する破壊力がある。明示的に対応していない以上、暗黙の誤操作を起こさないほうが良い
3. **テストで非対応を明示済**
   - `TestCargoGet_WorkspacePackageNotMatched` がワークスペースルート構造に対して Get がエラーになることを保証。将来「対応する」決定をするときはこのテストを書き換える必要があり、判断ポイントが残る

## 不採用案

### A. `[package]` が無ければ `[workspace.package]` にフォールバック

挙動が暗黙的すぎる。利用者が「このファイルは `[package]` が無いので意図せずワークスペースを更新している」状況を作りやすい。

### B. `--workspace` フラグで明示的にワークスペース対象に切り替え

flat 4-action + 排他フラグのシンプルな CLI 設計 (DR-0001) を崩すことになる。MVP の現用途では不要なため見送り。

### C. ワークスペースルートとメンバーを再帰的に処理

`bump-semver minor /path/to/workspace --recursive` のような案。実装複雑度が高く、本ツールのスコープを超える。実需が出れば `--workspace` フラグ案 (B) と合わせて新規 DR で議論。

## 将来対応するときの方針

- `cargoHandler` を以下のいずれかで拡張:
  1. `[package]` フォールバックで `[workspace.package]` を読む (誤操作リスクあり)
  2. 新 handler `cargoWorkspaceHandler` を分離し、basename だけでは判別できないので **内容ベースのプリチェック** (`[workspace]` セクションの有無) で dispatch する
  3. `--workspace` 明示フラグを追加 (DR-0001 設計を一部緩める判断が要る)
- `TestCargoGet_WorkspacePackageNotMatched` を書き換えるか別テストに退避

## 関連

- DR-0001: flat 4-action + basename 形式判定 (本判断の上位設計)
- `src/handler_cargo.go` / `src/handler_cargo_test.go`
- `docs/ROADMAP.md` の「Cargo workspace の `[workspace.package].version` 対応」セクション
