# `Cargo.lock` のサポート

## 背景

Cargo (Rust) が生成する `Cargo.lock` (TOML 形式) には `[[package]]` ブロックで **自パッケージも含む全 version が記録** される。

## 結論 (おそらく対応不要)

`Cargo.toml` を bump した後に **`cargo check` (もしくは何らかの cargo コマンド) を呼べば Cargo.lock が自動更新される**。これは jj-worktree の justfile でも採用しているパターン:

```just
bump-semver level="patch": ensure-clean test build
    bump-semver "{{level}}" Cargo.toml --write
    cargo check --quiet                              # ← Cargo.lock を新 version で更新
    jj describe -m "Release v$(bump-semver get Cargo.toml)"
    ...
```

つまり Cargo.lock は **bump-semver の責務外で OK** (cargo に任せる)。

## ただし別の用途として

**整合性検証** には使える可能性がある:

- `bump-semver get Cargo.toml Cargo.lock` で「Cargo.toml の `[package].version` と Cargo.lock の自パッケージ `[[package]].version` が一致しているか」を確認
- CI で「忘れて Cargo.toml だけ bump して Cargo.lock 更新を push し損ねた」状態を検出できる

これは bump 用途ではなく **検証用途** だが、複数ファイル対応 (親 issue) の流れで自然に得られる機能。

## 想定される実装 (もしやるなら)

- basename `Cargo.lock` で専用 handler に dispatch
- TOML パーサ (BurntSushi/toml は既に依存にある)
- 抽出: `[[package]]` ブロックの中で `name` が Cargo.toml の `[package].name` に一致するエントリの `version`
- ただし bump-semver は単独ファイルとして Cargo.lock を見るとき `Cargo.toml` の name を知らない → 困難
- → **複数 FILE モードでのみ動く特殊化** にする手もある (`Cargo.toml` + `Cargo.lock` 同時指定時に name で突き合わせ)

## 関連

- 親 issue: `2026-05-09-multi-file-mode-with-version-consistency.md`
- 関連 DR (jj-worktree): `Cargo.toml` 書き換え後に `cargo check --quiet` で Cargo.lock 自動更新するパターンを採用済 (`docs/decisions/DR-0003-release-flow.md`)

## 優先度

最低。`cargo check` で十分自動同期されるので、bump-semver で扱う必要性は薄い。整合性検証として欲しいケースが現れたら着手。

報告者: kawaz/jj-worktree main の親 CC — 2026-05-09 (kawaz の指示に基づき起票)
