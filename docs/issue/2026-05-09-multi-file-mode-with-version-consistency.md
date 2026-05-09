# 複数 FILE 対応 + ファイル間 version 一致検証 + lockfile 系の特殊 handler

## 背景

現状の `bump-semver` は単一 FILE のみ受け付ける (`parseArgs` が 2 つ目の positional でエラー):

```
$ bump-semver patch /tmp/foo.json /tmp/bar.json --write
bump-semver: multiple FILE arguments: /tmp/bar.json
```

しかし kawaz の主要ユースケース (claude-cmux-msg, npm/Bun 系プロジェクト) では **複数ファイルの version が常に同期している必要がある**:

- claude-cmux-msg: `.claude-plugin/plugin.json` / `.claude-plugin/marketplace.json` / `package.json` の 3 ファイルで `.version` が一致
- npm 系: `package.json` の `version` と `package-lock.json` 内部の複数の `.version` が一致

これらを 1 コマンドで一括 bump できれば、justfile が `bump-semver patch a.json b.json c.json --write` の 1 行になる (現状は 3 行に regex を書いている、もしくはループ)。

副次効果として、**ファイル間の version ずれを bump-semver 自身が検出** できる (今は誰もチェックしていないので、リリース時に片方だけ古いまま push されるリスクあり)。

## 仕様提案

### 1. 複数 FILE を許容

```
bump-semver <ACTION> <FILE...> [--write]
bump-semver <ACTION> --value <VER>
```

- `FILE...` は 1 個以上 (`--value` と排他)
- `--value` は単一 VER パース用、`FILE...` と排他なのは現状通り
- stdin pipe 時は FILE が複数あったらエラー (FILE = 1 個のときの名前ヒント用途として現状仕様維持)

### 2. version 一致検証

`get` / `major` / `minor` / `patch` の **すべて** で:

1. 各 FILE から「対象 version (複数あり得る、後述)」を全部抽出
2. **全 version が同一であること** を検証
3. 不一致なら `bump-semver: version mismatch: a.json=1.2.3, b.json=1.2.4 (.packages[""])` のようにファイル名 + 対象パスを示してエラー

`get` モード単独でも検証として機能する → CI で `bump-semver get package.json package-lock.json` を回せばリリース前のずれ検知に使える。

### 2-b. name (パッケージ名) 一致検証

複数 FILE 指定時は **パッケージ名も整合性検証** する。これにより「別プロジェクトのファイルを誤って一括 bump する事故」を構造的に防ぐ。

各 handler は version だけでなく「自パッケージの name (取得可能ならば)」を返す:

| handler | name 抽出パス |
|---|---|
| `package.json` (json handler の特殊化) | `.name` |
| `package-lock.json` | top-level `.name` + `.packages[""].name` |
| `Cargo.toml` | `[package].name` |
| `Cargo.lock` (将来) | `[[package]]` で自パッケージのもの (= Cargo.toml の name で突き合わせ) |
| `VERSION` | 取得不可 (name フィールドなし) |
| その他 `*.json` (`.claude-plugin/plugin.json` / `marketplace.json` / `moon.mod.json`) | `.name` があれば取得、なければ取得不可 |

検証ロジック:

1. 各 FILE から (version, name?) のペアを抽出 (name は optional)
2. **name が取れたファイル間で全部一致するか** を検証 (取れないファイルはスキップ)
3. 不一致なら `bump-semver: name mismatch: package.json=foo, package-lock.json=bar` のようなエラー

これは **誤ったファイルセットを検出** する防壁:
- ✅ `package.json` (`name=foo`) + `package-lock.json` (`name=foo`) → OK
- ❌ `package.json` (`name=foo`) + 別ディレクトリの `package-lock.json` (`name=bar`) → エラー (誤指定)
- ✅ `package.json` (`name=foo`) + `VERSION` (name 取得不可) → OK (name はスキップ、version のみ検証)
- ✅ `Cargo.toml` (`name=foo`) + `package.json` (`name=foo`) → OK (奇妙な組み合わせだが name が一致するなら通す)

注: name は **bump 対象ではない** (書き換えない、検証専用)。

### 3. bump 時の書き戻し (`--write`)

- 全ファイルの全 version を新 version で書き換え
- ファイル間で「何箇所書き戻すか」が違っても、全部同じ新 version に揃える
- 書き戻しは順次 (各ファイル独立)、途中失敗したら stderr に何行目で失敗したかを残して exit non-zero (一部書き換え済の状態が残る = 中途半端だが、jj/git で容易にロールバック可能なので rollback ロジックは持たない)

### 4. stdout 出力

- 成功時、新 version を 1 行
- 全ファイル全 version 同じになるので 1 行で十分

### 5. lockfile 系の特殊 handler

#### 5-1. `package-lock.json` (npm 7+ lockfile v2/v3)

basename `package-lock.json` で json handler とは別の専用 handler に dispatch する。

対象 version 抽出パス:

- top-level `.version` ← 自パッケージの version
- `.packages[""]` の `.version` ← npm 7+ で必ず存在する root package entry

(npm 5-6 の lockfile v1 形式 `dependencies` ツリーは MVP では非対応で OK。`lockfileVersion` を見て v1 はエラー or 警告)

`.packages["node_modules/<name>"]` の `.version` は **依存の version なので bump 対象外** (絶対に書き換えない)。

```json
{
  "name": "claude-cmux-msg",
  "version": "0.24.0",          ← 対象 (top-level)
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "claude-cmux-msg",
      "version": "0.24.0"        ← 対象 (root entry)
    },
    "node_modules/some-dep": {
      "version": "1.0.0"          ← 対象外 (依存)
    }
  }
}
```

#### 5-2. 複合シナリオ (`package.json` + `package-lock.json`)

`bump-semver patch package.json package-lock.json --write` のとき、対象 version は **3 箇所以上**:

- `package.json` の `.version`
- `package-lock.json` の top-level `.version`
- `package-lock.json` の `.packages[""].version`

全部一致していなければエラー、bump 時は全部同じ新 version に更新。

#### 5-3. その他の lockfile

将来追加候補 (今回の MVP 範囲外):

- `Cargo.lock` ← Cargo workspace の `[[package]]` で自パッケージ version を持つ。`Cargo.toml` 書き換え後に `cargo check` で自動更新されるので bump-semver の責務外で OK
- `bun.lockb` ← バイナリ形式、扱い困難、現状 npm のように内部に version 持たない
- `pnpm-lock.yaml` ← pnpm 系、別 issue で
- `composer.lock` (PHP) / `Gemfile.lock` (Ruby) / `poetry.lock` (Python) ← 同上、必要が出たら別 issue

「網羅は捨て、必要が出たら handler を 1 つ追加」(DR-0001) 方針に従う。

### 6. handler interface 拡張

現状 (推測):

```go
type Handler interface {
    Get(content []byte) (string, error)
    Replace(content []byte, newVer string) ([]byte, error)
}
```

→ 1 ファイルに複数 version field がある + name 検証を担うため:

```go
type Handler interface {
    // ファイルから検出された全 version + 全 name を返す
    // (1 ファイル内で複数箇所が対象なら全部、name は取れなければ nil)
    Inspect(content []byte) (Inspection, error)
    // 全 version field を newVer で書き換え (name は触らない)
    Replace(content []byte, newVer string) ([]byte, error)
}

type Inspection struct {
    Versions []Field  // 必須、1 件以上
    Names    []Field  // optional、name を持つ handler のみ
}

type Field struct {
    Value string
    Path  string  // "$.version" / "$.packages[''].version" / "$.name" 等、エラー表示用
}
```

`Inspect` を返すことで、複数 version / name 検証ロジックがファイル間で共通化できる:

```go
// 全ファイルから全 version + 全 name を集める
allVers, allNames := []FilePathField{}, []FilePathField{}
for _, f := range files {
    insp, _ := handler.Inspect(content)
    allVers = append(allVers, prefixed(f, insp.Versions)...)
    allNames = append(allNames, prefixed(f, insp.Names)...)
}

// version は全部同一か検証 (必須)
if !allSame(allVers) {
    return error("version mismatch: ...")
}

// name は取れたものだけで一致確認 (取れないファイルはスキップ)
if !allSame(allNames) {
    return error("name mismatch: ...")
}
```

## CLI 互換性

現状の `bump-semver patch FILE [--write]` は **そのまま動く** (FILE が 1 個の場合の特殊化として包含)。下位互換破壊なし。

エラーメッセージだけ変わる:

- 旧: `bump-semver: multiple FILE arguments: <2nd>`
- 新: (FILE が 2 個以上の場合は正常動作、version 不一致時は `version mismatch: ...`)

## テスト追加観点

- 単一 FILE → 既存テストそのまま
- 複数 FILE で全 version 一致 + name 一致 → 全部書き換え + stdout 1 行
- 複数 FILE で version 不一致 → エラー + どこが不一致かを stderr に
- 複数 FILE で **name 不一致** (異なるプロジェクトのファイル誤指定) → エラー
- `package-lock.json` 単独で top-level と packages[""] の version 不一致 → エラー
- `package-lock.json` 単独で top-level と packages[""] の name 不一致 → エラー (壊れた lockfile)
- `package.json` + `package-lock.json` 複合で全部一致 → bump
- `package.json` + `package-lock.json` 複合で version 不一致 → エラー
- `package.json` + `package-lock.json` 複合で name 不一致 (foo + bar) → エラー
- name を取れない handler との混合 (`Cargo.toml` + `VERSION`) → name はスキップ、version 一致なら OK
- npm v1 lockfile (`lockfileVersion: 1` または `dependencies` だけ) → エラー or 警告
- 依存 (`packages["node_modules/foo"]`) の version / name は **絶対に書き換えない** ことを golden ファイルで確認

## 関連

- DR-0001 (flat actions + format detection): 「網羅は捨て、必要が出たら handler 追加」方針と整合
- claude-cmux-msg の migrate issue: `kawaz/claude-cmux-msg/main/docs/issue/2026-05-09-migrate-to-bump-semver-and-just-ci.md` (3 ファイル一括 bump の主要消費者、本拡張で 3 行 → 1 行になる)
- npm package-lock.json 仕様: https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json

## 優先度

中。bump-semver v0.2.0 の単一ファイル仕様で当面は justfile から for ループで回せば動く (`for f in a.json b.json; do bump-semver patch "$f" --write; done`)。ただし version 不一致検出ができないので、リリース時の事故リスクが残る。本拡張で claude-cmux-msg 等の主要消費者の justfile が大幅にシンプル化 + 不整合検出能力を獲得。

報告者: kawaz/jj-worktree main の親 CC (session_id: `718c6cc3-b154-4de5-9cbe-cccd6dcfa407`) — 2026-05-09 (kawaz の指示に基づき起票)
