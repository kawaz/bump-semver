# DR-0004: 複数 FILE 一括 bump + name 整合性検証 + package-lock.json 特殊化

- ステータス: Accepted
- 日付: 2026-05-09
- 関連: DR-0001 (flat actions + format detection), DR-0002 (Cargo workspace 不対応), DR-0003 (prefix と柔軟 separator)

## 文脈

DR-0001 は CLI 表面を `bump-semver <ACTION> <FILE | --value VER> [--write]` の単一 FILE 指定に限定していた。これは MVP として正しい範囲だが、kawaz の主要消費者で実需が出てきた:

- **claude-cmux-msg**: `.claude-plugin/plugin.json` / `.claude-plugin/marketplace.json` / `package.json` の 3 ファイルで `$.version` が常に同期している必要がある (justfile に同じ regex を 3 行書いている状態)
- **npm 系プロジェクト**: `package.json` の `version` と `package-lock.json` 内部の `$.version` + `$.packages[""].version` が同期している必要がある (npm が走るたびに lockfile が更新されるが、`bump-semver` 単独で扱うと不一致を残しうる)

これらを 1 コマンドで一括 bump できれば justfile が `bump-semver patch a.json b.json c.json --write` の 1 行になる。副次効果として、**ファイル間 version ずれを検出できる**: 今は誰もチェックしておらず、リリース時に片方だけ古いまま push される事故が起きうる。

## 決定

### 1. 複数 FILE 許容、`get` を含む全 ACTION で整合性検証を必須化

```
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value VER
```

- `FILE...` は 1 個以上、`--value` と排他 (現状維持)
- 単一 FILE は完全互換 (CLI 互換性破壊なし)
- 全 ACTION (`get` / `major` / `minor` / `patch`) で「全 file の version が一致しているか」を検証
- `get` 単独でも整合性チェックとして CI で使える: `bump-semver get package.json package-lock.json` でリリース前のずれ検知

### 2. name (パッケージ名) 整合性検証

複数 FILE 指定時は **version だけでなく name も整合性検証する**。これにより「別プロジェクトのファイルを誤って一括 bump する事故」を構造的に防ぐ:

| シナリオ | 結果 |
|---|---|
| `package.json` (name=foo) + `package-lock.json` (name=foo) | OK |
| `package.json` (name=foo) + 別ディレクトリの `package-lock.json` (name=bar) | エラー (誤指定検出) |
| `package.json` (name=foo) + `VERSION` (name 取得不可) | OK (name はスキップ、version のみ検証) |
| `Cargo.toml` (name=foo) + `package.json` (name=foo) | OK (奇妙だが name 一致なので通す) |

name は **bump 対象ではない** (検証専用、書き換えない)。

### 3. Handler interface を `Inspect` に再設計

旧 `Get(content) (string, error)` は 1 ファイル = 1 version の前提だったが、`package-lock.json` のように 1 ファイル中に複数の version field と name field があるケースを表現できない。

新 interface:

```go
type Field struct {
    Value string
    Path  string  // human-readable: "$.version", "[package].version" 等
}

type Inspection struct {
    Versions []Field  // 必須 (1+)
    Names    []Field  // optional (0+)
}

type Handler interface {
    Inspect(content []byte) (Inspection, error)
    Replace(content []byte, current, newVersion string) ([]byte, error)
}
```

- `Path` は人が読むエラーメッセージ用の文字列 (厳密な JSONPath / TOMLPath である必要はない)。`$.version` / `[package].version` / `$.packages[""].version` / `(file content)` 等
- `Replace` は version field のみ書き換え、name は触らない (signature は v0.2.0 から不変)

### 4. `package-lock.json` 専用 handler

basename `package-lock.json` で json handler の前段に dispatch する (`*.json` の一般枝より先にマッチ)。

抽出パス:
- top-level `$.version` (自パッケージ version)
- `$.packages[""]` (npm 7+ で必ず存在する root package entry) の `.version` と `.name`
- top-level `$.name` (root package name)

**`$.packages["node_modules/<dep>"]` の version / name は絶対に書き換えない** (依存)。

実装方針: `Replace` は `json.Decoder` で構造を streaming 走査し、上記 2 箇所の version 値の **byte offset** を特定 → 末尾から先頭の順で in-place 置換する。これにより依存エントリの version が偶然 root と同値でも書き換わらない。

`lockfileVersion: 1` (npm 5/6) は **明示エラー**: `unsupported lockfileVersion: 1, please regenerate with npm 7+`。

### 5. エラーメッセージのフォーマット

```
bump-semver: version mismatch:
  package.json:$.version = 1.2.3
  package-lock.json:$.packages[""].version = 1.2.4
```

```
bump-semver: name mismatch:
  package.json:$.name = foo
  package-lock.json:$.name = bar
```

`bump-semver: ` prefix は最初の行だけ。各 file:path = value を改行 + 2 スペースインデントで列挙。これにより grep / awk でファイル一覧の抽出が容易。

### 6. stdin pipe + 複数 FILE は pipe 無視 (cat 慣例)

DR-0001 では「stdin pipe + 単一 FILE = FILE は名前ヒント、内容は stdin から」を定義していた。複数 FILE 時は **stdin pipe を無視して各 FILE を読む** ことに決定 (cat / sed と同じく「明示 FILE が stdin より優先」の慣例)。

これにより CI 環境などで stdin が偶発的に pipe になっていても、複数 FILE 指定が誤判定でエラーになることがない。

### 7. `--write` 時の挙動

- 全 file の全 version field を新 version で書き換え
- 各 file は順次書き戻し、途中失敗時はそのまま `replace <file>: <reason>` で exit non-zero
- **rollback ロジックは持たない**。jj / git で容易に復元できるため、bump-semver 内に複雑さを持ち込まない方針

## 不採用案

### A. 単一 FILE 仕様のまま、justfile 側で for ループで回す

```bash
for f in package.json plugin.json marketplace.json; do
  bump-semver patch "$f" --write
done
```

主要消費者の justfile は短くなるが、**file 間の version 不一致は検出できない** ので事故リスクが残る。本 DR の核心 (整合性検証) を実現できないので不採用。

### B. 自動全 file 検索 (`bump-semver patch` だけでカレントの version files を勝手に検出)

「実装が魔術的」「対象が予想できない」「`Cargo.toml` も `VERSION` も両方ある場合の優先順位が曖昧」になる。明示的な FILE 列挙を維持。

### C. regex `--pattern` フォールバック

DR-0001 で不採用にした方針を再持ち出ししない。新 handler を 1 つ追加する方針を貫く。

### D. Handler を `Get` のまま据え置きで内部リスト返すよう拡張

シグネチャだけ変えても、name 検証や複数 version 検出のために結局構造体が必要。中途半端な変更は避け、`Inspect` で一気に整理する方が後の拡張 (new handler 追加) もやりやすい。

### E. lockfile v1 を best-effort で扱う

`dependencies` ツリーの walk + 自パッケージ版の特定は、npm 7+ の `packages[""]` ほど明確でなく誤動作リスクがある。エラーで弾いて `npm 7+` 移行を促す方が安全。

### F. `package-lock.json` の Replace を regex でやる

旧 jsonHandler 流の「現在値で値特定 → 単一マッチ」だと、依存に同 version のパッケージがあるとマッチが破綻する。`json.Decoder` で structural な走査をして offset を取る方が確実。複雑度は上がるが副作用がない。

## 実装上の注意

- `Inspection.Names` が空のファイルがあっても name 検証は名前を持つ file のみで実施する (= `Cargo.toml` + `VERSION` 混在で `VERSION` の name が無くてもエラーにしない)
- `package-lock.json` 単独でも `$.packages[""]` がない (壊れた lockfile) ならエラー
- `package-lock.json` の top-level `$.name` と `$.packages[""].name` が違う壊れた lockfile は、main の name 整合性検証で自動検出される (Names 配列に 2 件並んで値が違う)

## 影響

- v0.2.0 までの単一 FILE 動作は完全互換 (テスト全通る)
- justfile の version sync が 3 行 → 1 行になる主要消費者: claude-cmux-msg, 将来の npm 系 kawaz リポ
- name 整合性検証で「別プロジェクトのファイル誤指定」事故を構造的に検出可能に

## 関連

- 親 issue (本実装の起票): `docs/issue/2026-05-09-multi-file-mode-with-version-consistency.md` (本 DR 採択により削除)
- スコープ外で残置の lockfile 系 issue (調査のみ): bun-lock / yarn-lock / pnpm-lock-yaml / cargo-lock
- 主要消費者: kawaz/claude-cmux-msg の `.claude-plugin/{plugin,marketplace}.json` + `package.json` 3 ファイル sync
- npm package-lock.json 仕様: https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json
