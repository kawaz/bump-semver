# 2026-05-11 justfile テンプレ化向け refactor + dogfooding

v0.5.0 / 0.6.0 / 0.7.0 / 0.7.1 のリリース後、justfile を kawaz/* リポ共通テンプレ候補として大規模 refactor。just docs 全面適用 + マルチペルソナレビュー反映。

## 経緯

1. **v0.5.0** (DR-0006): pre-release / build-metadata 対応 + compare サブコマンド + FILE|VER 統合
2. **v0.6.0** (DR-0007): `--json` 出力オプション
3. **v0.7.0** (DR-0008): `vcs:` 入力モード + `latest-tag()` 関数
4. **v0.7.1**: `--version --json` + Examples 修正 (dogfooding 由来)
5. **dogfooding**: justfile / release.yml で bump-semver 自身を活用 (BUG fix: Homebrew formula test の `--value` 廃止対応)
6. **justfile 大規模 refactor**: テンプレ化向け整理

## v0.7.x リリース時の発見・判断

### `--version --json` (ユーザ提案)

`--json` の責務「全 JSON 化」を `--version` にも拡張。`bump-semver --version --json | jq -r .semver` のような使い方が release.yml で書ける。

私が一度「設計を歪める」と (a) 案を撤回提案したが、ユーザは「そうじゃなくて (a) でいけ」と判断 → 既存の `--json` スキーマを再利用するだけなので歪みは小さい。実装は version 文字列を ParseVersion → ToJSON で出力するだけ。

### Examples 整理

dogfooding 過程で help text の Examples に問題発覚:

- `compare gt Cargo.toml vcs:origin/main` → 普通は `compare lt` が欲しい (stale チェック)
- `patch Cargo.toml --pre rc.0` → 元値が見えなくて意味不明、`patch 1.2.3-rc.0 --pre rc.0` に
- `compare eq Cargo.toml package.json` → 現実的でないペア、`Claude plugin の 3 ファイル整合性` に置換
- `version_1_2_3` 例が抜けてた (kawaz 拡張 sep `_` の代表例)

### Homebrew formula の latent BUG

`release.yml` の Homebrew formula test (line 195-197) が **v0.5.0 で廃止した `--value` を使っていた**。`brew test bump-semver` でしか発火せず、update-homebrew workflow では走らないので潜伏。dogfooding で発見、位置引数 + `compare eq` smoke test に修正。

## justfile 大規模 refactor の経緯

### きっかけ: ユーザの「`./bin/bump-semver` を使うのは違くね？」

`justfile` の bump-version レシピで `./bin/bump-semver` (self-build) を使っていたが、テンプレ化対象として他リポでも使えるように `bump-semver` (PATH 経由、brew 等で入る) に切替。

### `jj commit` で empty change を許容する罠

ユーザの「`jj commit -m "Release v$(bump-semver ...)"` で 1 行に書けないか?」提案で実証実験:
- `jj commit` は **空 change でも成功** (デフォルトで `--allow-empty` 相当)
- bump-semver が失敗すると `Release v` の壊れ commit ができる
- bash の `set -e` でも `$()` 内エラーは command 全体の exit code に影響しない
- 結論: `var=$(...) && jj commit -m "Release v${var}"` の形式が安全

### `--version --json` 検証

`set -eu -o pipefail` 環境で `var=$(failing_cmd)` は **コマンド代入の特殊扱いで `set -e` が効く** (確認済み)。一方 `cmd "$(failing_cmd)"` は cmd 自身の exit code しか見ないので `set -e` は反応しない (`inherit_errexit` でも親には効かない)。

### just docs 全面適用 + `[script]` 採用

ユーザ「サブエージェントに任せず、メインで just docs (https://just.systems) ちゃんと読め」 → メインで読み直して以下を採用:

- `set unstable / guards / lazy / shell / script-interpreter`
- `[script]` 属性で recipe 全体を 1 bash スクリプト化 (exit 0 早期 return 可)
- `?` sigil で linewise recipe の guard 早期 return
- `if {{ is-jj }}; then ...; fi` イディオム (path_exists の "true"/"false" 文字列を bash 組み込み true/false コマンドとして評価)
- 依存パラメータ `(_check-translation "README")` で 3 ファイル明示列挙
- `file_name()` 等の built-in functions

### マルチペルソナレビュー (`/itumono-full-review`) の発見

5 ペルソナ + codex で並列レビュー:

**Critical 1**: `check-version-bumped` で `[ -z "$(jj diff ...)" ]` パターンが jj diff の error を silently swallow → 誤 PASS バグ。`main@origin` 未 track 時等で発動。3 expert (Bash / Just / jj-git) が同じ指摘 = 高確信度。修正: 結果を変数経由で受けて exit code を明示確認。

**Critical 2 (codex)**: `push` / `bump-version` が jj 専用 → kawaz テンプレは jj 統一前提と明文化することで仕様化。

**Critical 3 (codex)**: `main` / `origin` ハードコード → kawaz テンプレで `main`/`origin` 統一前提なので変数化せず。

**Warning**: `if {{ is-jj }}` の `true`/`false` PATH 依存 (実害薄、現状維持)、VCS 不在環境のガード不足 (現状維持)、`?` sigil 説明不足 (コメント追加対応)、`gofmt -w` の副作用 (コメント追加対応)、`?` sigil で ja.md 誤削除を見逃す (design rationale としてコメント追加)。

**Security**: PASS (引数経由 injection は理論的に存在するが呼び出し元ハードコードで実害なし)。

## kawaz テンプレ前提の明文化

justfile 冒頭に:
```
# bump-semver
#
# kawaz/* リポの共通テンプレ候補。基本は jj 統一前提 (git bare + jj workspace)、
# main / origin / src/ 等のハードコードは kawaz スタイルに揃えてある。
# 各リポで上書きするのは bump-trigger-paths くらい。
# git only リポへ流用する場合は push / bump-version の jj 呼び出しを書き換える必要あり。
```

## ハマり所メモ

### `?` sigil の挙動

`?cmd` は **cmd が exit 1 (= false) なら recipe stop (success 扱い)**、exit 0 なら続行。「guard」セマンティクス。

例:
- `?test -f X` → X 存在で続行、不在で recipe stop
- `?(test -d .jj && cmd)` → 真なら続行、偽なら stop

`set guards := true` 必須 (1.47.0+)。

### `path_exists` の挙動

`path_exists('.jj')` は `"true"` または `"false"` (string) を返す。bash の `if {{ is-jj }}; then` で展開すると `if true; then` または `if false; then` で、bash builtin の `true`/`false` コマンドが exit 0/1 を返す → if 通る/通らない、で動く。エレガントだが PATH 完全空 (`enable -n true false` 等) では壊れる。

### jj diff の `--quiet`

git の `--quiet` と異なり、**jj diff は exit code が常に 0**。差分有無で exit が変わらない。`--summary` の出力空判定 (`[ -z "$(...)" ]`) で代替。さらに `jj diff` 自体が失敗 (rev 未 track 等) すると stderr エラーで stdout 空 → 誤判定。**変数経由で exit code を明示確認**が必要。

### `inherit_errexit` の限界

bash の `inherit_errexit` を有効にしても、`cmd "$(failing)"` の subshell エラーは「subshell が早期終了するだけ」で親 cmd は引数として空文字列を受け取って実行。`set -e` は cmd 自身の exit code しか見ない。

つまり:
- `var=$(failing)` → 親の `set -e` で stop ✓
- `cmd "$(failing)"` → 親の `set -e` 効かず、cmd は空引数で実行 ✗

### path_exists は lstat 系

`.jj` が壊れた symlink でも `true` を返す可能性。kawaz の構成では実ディレクトリなので問題なし。

## 参考

- DR-0006 / DR-0007 / DR-0008 (本日の v0.5/0.6/0.7 リリース)
- マルチペルソナレビュー: 5 expert + codex 並列、`/itumono-full-review` 経由
- next: `/itumono-nonstop` で issue 消化 (lock file 仕様調査並列起動済み)
