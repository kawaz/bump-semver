# DR-0021: Cargo workspace の `[workspace.package].version` に対応する

- ステータス: Accepted
- 日付: 2026-05-30
- 関連: DR-0002 (本 DR が supersede), DR-0001 (flat 4-action + 形式判定), DR-0005 (path-aware confidence-ranked rules), DR-0014 (TOML section-scoped Replace + OR/first-match-wins VersionPaths), `src/rules.go` / `src/format_toml.go`

## 文脈

DR-0002 で「Cargo workspace の `[workspace.package].version` は MVP では非対応」を Accepted とした。その時点では kawaz の現用途 (`port-peeker` / `authsock-warden` / `stable-which` 等) がすべてシングルクレート構成で、workspace の実需が無かったため。

その後、kawaz/hyoui (Rust workspace project) が `[workspace.package]` 構成に移行 (VERSION ファイルを廃止し Cargo.toml を version 正本化) した結果、実需が立ち上がった:

```bash
$ bump-semver get Cargo.toml
bump-semver: Cargo.toml: *.toml (fallback): missing version
```

ワークスペースルートの `Cargo.toml` は典型的に `[package]` を持たず、version は `[workspace.package].version` にあり、メンバー crate が `version.workspace = true` で継承する。これが bump-version の主要対象になるが、既存ルール (`[package].version` のみ) では拾えず、`pkf run bump-version` task が回らない実害が発生した。

加えて DR-0002 以降、**DR-0014 で同種のパターンが既に確立済み**である点が状況を変えた: `pyproject.toml` ルールは `[project].version` を試し、無ければ `[tool.poetry].version` にフォールバックする (OR / first-match-wins)。TOML format はセクションスコープの Replace を `[tool.poetry]` のようなドット区切りセクションまで一般化済みで、`[workspace.package]` も同じ機構で扱える。

## 決定

**`Cargo.toml` ルールの `VersionPaths` / `NamePaths` を OR フォールバックで拡張し、`[workspace.package].version` に対応する。**

```
VersionPaths: []string{".package.version", ".workspace.package.version"}
NamePaths:    []string{".package.name", ".workspace.package.name"}
```

- **優先順位**: `[package].version` が先。crate 自身が publish する version なので、両方ある場合 (workspace-shared フィールドも宣言するメンバー crate) は `[package].version` が勝つ。`[package]` が無いとき (典型的な workspace-root) のみ `[workspace.package].version` にフォールバック。
- **Replace**: Inspect が選んだ path と同じセクションを書き換える (`tomlReplace` が再 Inspect して hit path → section path に変換する既存実装に乗る)。`[workspace.package]` のドット区切りセクションは DR-0014 の `tomlReplaceInSection` がそのまま処理する。
- **透明性**: マッチした path は `get` 出力・診断メッセージ・`--json` の path 表示にそのまま出る (`[workspace.package].version`)。利用者は自分が何の version を bump しているかを常に確認できる。

実装は rules.go テーブルの 1 行拡張のみ。新 format も新ハンドラも不要。

## 理由

1. **DR-0014 で確立した OR フォールバックパターンと完全に同形** — `pyproject.toml` の `[project]` → `[tool.poetry]` と同じ機構を流用する。新たな概念を持ち込まない。
2. **DR-0005 の path-aware confidence-ranked rule 体制と整合** — basename 決め打ちの単一ルール拡張で済み、内容ベース dispatch (案 2) のような別レイヤを足さない。
3. **DR-0001 の flat 4-action + auto-detect CLI を崩さない** — `--workspace` フラグ (案 3) を入れない。bump-semver の価値の核である「フラグ不要の auto-detect」を維持。
4. **DR-0002 の「暗黙的すぎる」懸念は透明性で解消** — workspace の version 変更は全メンバー crate に伝播する破壊力があるが、これは**マッチ path の可視化**で対処するのが筋。`get` / `--json` / 診断にマッチ path が出るので、利用者は意図せず workspace を更新している状況に気づける。「ブロックして弾く」のは auto-detect の利便性を犠牲にする過剰防衛で、DR-0014 で同じ判断 (フォールバックを許し、マッチ path を出す) を既に下している。

## 不採用案 (DR-0002 の「将来対応するときの方針」3 案を再評価)

### 案 2. 新 handler `cargoWorkspaceHandler` を分離し内容ベースで dispatch

`[workspace]` セクションの有無で別ハンドラに振り分ける案。**却下**: DR-0005 で basename ベース→ path-aware confidence-ranked rule に移行した設計の流れに逆行する。内容ベース dispatch というレイヤを 1 形式のために新設するのは責務肥大。OR/first-match-wins VersionPaths という既存の枠組み (DR-0014) で表現できる以上、新ハンドラは過剰。

### 案 3. `--workspace` 明示フラグ

`bump-semver minor Cargo.toml --workspace` で明示的に workspace 対象に切り替える案。**却下**: DR-0001 の flat 4-action + auto-detect 設計を緩める。「どのファイルがどの形式か」をフラグで人間に指定させるのは、basename/内容で自動判定する本ツールの設計思想と矛盾する。`pyproject.toml` の二系統 (PEP 621 / Poetry) をフラグ無しで自動フォールバックしているのに Cargo だけフラグ必須にするのは一貫性も欠く。

### 案 (DR-0002 案 C). 再帰的に workspace + members を一括処理

`--recursive` でルート + 全メンバーを処理する案。**却下 (スコープ外)**: 本 DR の対象は「ワークスペースルートの version 正本を 1 つ bump する」こと。`version.workspace = true` を使うメンバーはルートを継承するので、ルート 1 ファイルの bump で伝播は完結する。複数ファイル一括は ROADMAP の glob 展開項目で別途検討。

## DR-0002 との関係

本 DR は DR-0002 を **supersede** する。DR-0002 の判断 (MVP では非対応、誤操作防止のためエラーで弾く) は当時の文脈 (workspace の実需なし) では妥当だったが、(a) 実需が立ち上がり、(b) DR-0014 で同形のフォールバック機構が確立した、の 2 点で前提が変わった。DR-0002 が懸念した「暗黙的すぎる」点は、マッチ path の可視化という透明性アプローチで解消する。

## テスト

- `TestCargoInspect_WorkspacePackageNotMatched` を `TestCargoInspect_WorkspacePackageFallback` に書き換え (DR-0002 の「将来対応時に書き換える」指示どおり)。
- 追加: `TestCargoInspect_PackageWinsOverWorkspacePackage` (両方ある時の優先順位)、`TestCargoReplace_WorkspacePackage` (コメント・隣接セクション保全)、`TestCargoReplace_PackageWinsOverWorkspacePackage` (Replace も `[package]` 優先)、`TestCargoInspect_NeitherPackageNorWorkspacePackage` (両方無い時はエラー継続)。

## 関連

- DR-0002: Cargo workspace を MVP で非対応とした判断 (本 DR が supersede)
- DR-0014: TOML section-scoped Replace + OR/first-match-wins VersionPaths (流用元の機構)
- `src/rules.go` の `Cargo.toml` ルール / `src/format_toml.go`
- `docs/ROADMAP.md` の「Cargo workspace の `[workspace.package].version` 対応」セクション
