# issue: 複数 Cargo.toml の一括 bump が name mismatch で常に失敗する (workspace で実用不可)

- Date: 2026-06-10
- Status: open
- 発見元: kawaz/cache-warden の justfile 整備 (2026-06-10 dogfooding)

## 現象

Rust workspace の複数 crate を一括 bump しようとしたところ `name mismatch:` で exit 2:

```bash
$ bump-semver patch \
    crates/cache-warden/Cargo.toml \
    crates/cache-warden-cli/Cargo.toml \
    --write
bump-semver: name mismatch:
  crates/cache-warden/Cargo.toml:     cache-warden
  crates/cache-warden-cli/Cargo.toml: cache-warden-cli
```

## 背景: 現仕様の確認

`src/cli_dispatch.go` の name mismatch 検証 (DR-0023 / follow-up #35):

- 複数の FILE 入力に対して `[package].name` を集約し、全て一致しない場合は
  `bump` では exit 2 (usage error)、`get` では exit 1 (predicate-false) を返す
- 意図は「誤ったファイルを混ぜた誤爆防止」であり、設計上は正しい

Rust workspace のメンバー crate は当然 package name が異なるため、
「複数 FILE は name 一致必須」の現仕様だと multi-crate workspace の一括 bump が
構造的に不可能になる。

## 正攻法での解決 (利用側で対応済み)

Rust workspace の標準パターンに従い、root `Cargo.toml` に
`[workspace.package] version` を置き、メンバー crate は `version.workspace = true` で継承:

```toml
# root Cargo.toml (workspace root)
[workspace.package]
version = "0.1.1"

# crates/cache-warden/Cargo.toml
[package]
version.workspace = true

# crates/cache-warden-cli/Cargo.toml
[package]
version.workspace = true
```

この構成にすると bump 対象を root 1 ファイルのみにでき、
DR-0021 の `[workspace.package].version` フォールバックで綺麗に動く:

```bash
$ bump-semver patch Cargo.toml --write   # root のみを対象
$ bump-semver get Cargo.toml             # → 0.1.2
```

メンバー crate は `version.workspace = true` 経由で自動追従するため、
bump-semver 側には 1 ファイルの操作だけで workspace 全体のバージョン管理が完結する。

## 提案 (どちらか / 両方)

(a) **name mismatch エラーメッセージに workspace 運用のヒントを追加** (推奨・低コスト)

  例えば以下のような内容をエラーに添えると、ユーザが正攻法に辿り着きやすくなる:

  ```
  bump-semver: name mismatch:
    crates/cache-warden/Cargo.toml:     cache-warden
    crates/cache-warden-cli/Cargo.toml: cache-warden-cli
  hint: Rust workspace では [workspace.package] version を root Cargo.toml に置き、
  メンバーは version.workspace = true で継承すると 1 ファイルの bump で済みます。
  ```

  dogfooding 所感では (a) だけでも実用上は十分。
  name 一致検証自体は誤爆防止として正しく、外す必要はない。

(b) **opt-in フラグ (`--allow-name-mismatch` 等) で別名複数ファイルの一括 bump を許可**

  - メリット: workspace 以外のユースケース (モノレポ、同一バージョンを持つ複数の独立パッケージ) にも対応できる
  - デメリット: 誤爆防止の価値が薄れる、設計判断が必要 (DR 相当)
  - 所感: Rust workspace は (a) の正攻法で解決するため (b) の優先度は低い

## 関連

- DR-0021: `[workspace.package].version` フォールバック実装 (commit `939205ef`)
- DR-0023: get の mismatch を exit 1 (predicate-false) として扱う設計
