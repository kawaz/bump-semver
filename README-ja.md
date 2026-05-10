# bump-semver

> [English](./README.md) | 日本語

バージョン管理用ファイル中の semver 文字列を取得・bump・比較するための、絞り込まれた CLI。ファイル形式は basename で自動判定 (`--pattern` regex フラグ不要)、5 つの flat なアクション (`major` / `minor` / `patch` / `pre` / `get`) と 1 つのネストサブコマンド (`compare`) を持つ。新しいバージョンは常に stdout に出力するのでシェルパイプラインに合成しやすい。

## なぜ作ったか

既存のバージョン bump CLI は「汎用すぎて毎回 regex を指定する必要がある」「特定のファイル形式しか扱えない」のどちらかに偏っている。`bump-semver` は真逆の立場を取り、kawaz が実際に使うファイル形式だけを正確にサポートし、新しい形式は具体的な必要が出たときに追加する。結果として「小さい・断定的・予測可能」な kawaz スタイルのツールになる。

## インストール

```bash
brew install kawaz/tap/bump-semver
```

`kawaz/tap` は [`kawaz/homebrew-tap`](https://github.com/kawaz/homebrew-tap) のこと。2ステップ等価形式: `brew tap kawaz/tap && brew install bump-semver`。

Linux / macOS / Windows (amd64, arm64) のビルド済みバイナリも GitHub Releases に公開。

## 使い方

```
bump-semver <ACTION> <INPUT...> [flags]
bump-semver compare <OP> <INPUT> <INPUT>
bump-semver --version [--json]
bump-semver --help
```

`<INPUT>` は **FILE パス** / **生の VER 文字列** / **`-` (stdin から VER 1 行読込)** / **`vcs:REV[:FILE]` または `vcs:<関数>(...)`** (VCS 経由で取得、[vcs: 入力](#vcs-入力) 参照) のいずれかで、複数指定時は混在可能。

### アクション

| アクション | 効果 |
|---|---|
| `major` | major を bump (`X.0.0`)。pre-release / build metadata はデフォルトで drop |
| `minor` | minor を bump (`x.Y.0`)。同上 |
| `patch` | patch を bump (`x.y.Z`)。同上 |
| `pre`   | pre-release の counter advance / 上書き / 削除 (3 モード、後述) |
| `get`   | 現在のバージョンを出力 (整合性チェック兼用) |

### compare サブコマンド

```
bump-semver compare <OP> <INPUT> <INPUT>
```

`<OP>` は `eq` / `lt` / `le` / `gt` / `ge` のいずれか。SemVer 2.0.0 順序仕様準拠で比較する (build metadata は順序比較から除外、prefix / sep の違いは正規化)。

| OP | 真となる条件 |
|---|---|
| `eq` | 第1引数 == 第2引数 |
| `lt` | 第1引数 <  第2引数 |
| `le` | 第1引数 <= 第2引数 |
| `gt` | 第1引数 >  第2引数 |
| `ge` | 第1引数 >= 第2引数 |

終了コード: `0` = 真 / `1` = 偽 / `2` = エラー (`test` / `dpkg --compare-versions` 慣習)。

```bash
bump-semver compare eq Cargo.toml v1.2.3 && echo same
bump-semver compare lt 1.2.3-rc.1 1.2.3                       # exit 0 (rc < 確定版)
bump-semver compare lt Cargo.toml < <(jj file show -r main@origin Cargo.toml)
                                                              # main からズレてないか CI チェック
```

### フラグ

| フラグ | 説明 |
|---|---|
| `--pre PRE`            | pre-release 識別子を設定 (例 `--pre rc.0`) |
| `--no-pre`             | pre-release を削除 |
| `--build-metadata META`| build metadata を設定 (例 `--build-metadata sha.abc`) |
| `--no-build-metadata`  | build metadata を削除 |
| `--write`              | bump 結果を各 FILE 入力に書き戻す (`major` / `minor` / `patch` / `pre` のみ) |
| `--vcs jj\|git`         | `vcs:` 入力の VCS を強制指定 (`BUMP_SEMVER_VCS` 環境変数より優先) |
| `--no-hint`            | 全 `hint:` 行を抑制 (fallback match / unsupported file / 「files not modified」) |
| `-q`, `--quiet`        | stdout と全 `hint:` 行を抑制 |
| `-qq`, `--quiet-all`   | stdout / hint / エラー出力をすべて抑制 (debug 時注意) |
| `--json`               | `get` / `major` / `minor` / `patch` / `pre` の出力を構造化 JSON にする (`compare` では不可) |
| `--version`, `-V`      | バイナリのバージョン |
| `--help`, `-h`         | ヘルプ |

排他: `--pre` と `--no-pre` 同時指定はエラー、`--build-metadata` と `--no-build-metadata` 同時指定はエラー、`--write` と `get` / `compare` の組み合わせはエラー。

`-q` / `-qq` / `--no-hint` は排他チェックなし: `-qq` は `-q` の上位互換、`-q` は `--no-hint` の上位互換 (両方指定でも黙って吸収)。`compare` は元々 stdout を持たないので `-q` は stdout 抑制部分は no-op。

`bump-semver` は通常の stdout に加えて状況に応じて stderr に `hint:` 行を 1 つ以上出力する。すべての hint は共通の `hint:` prefix を持ち、`--no-hint` / `-q` / `-qq` で一括抑制される。現状の hint 一覧:

| Hint | 発火条件 | 対象 / 抑制 |
|---|---|---|
| `hint: <path> matched as *.<ext> fallback. Open issue if explicit support is needed.` | FILE 入力が confidence 1 fallback で解決された (`*.json` は DR-0010、`*.yaml` / `*.yml` / `*.toml` は DR-0011)。該当ファイルごとに 1 行 | FILE を resolve するすべての場面 (`get` / bump 系 / `compare`) |
| `hint: Open issue at https://github.com/kawaz/bump-semver/issues if support is needed.` | FILE が `unsupported file:` でエラーになった時、その直後の hint 行 | 上に同じ |
| `hint: <N> file(s) not modified; use --write to update or --no-hint to suppress` | bump 系 (`major` / `minor` / `patch` / `pre`) で FILE 入力があり `--write` 未指定 | bump 系のみ。VER のみの bump や `get` / `compare` では出ない |

### 入力 (INPUT)

| 形式 | 解釈 |
|---|---|
| FILE | サポート形式のファイルパス (basename で自動判定) |
| VER  | semver 文字列を直接 (`1.2.3` / `v1.2.3` / `1.2.3-rc.1+build.42` 等) |
| `-`  | stdin から VER を 1 行読込 (1 回のみ使用可) |
| `vcs:REV[:FILE]` | jj/git の `<REV>` 時点のファイル内容から取得 (自動判定、[vcs: 入力](#vcs-入力) 参照) |
| `vcs:latest-tag()` | jj/git のタグ一覧から最大の semver-compat 値を取得 |

`1.2.3` という名前のローカルファイルを明示したいときは `./1.2.3` で曖昧さを回避 (Unix 慣習)。

### サポートする version 形式

```
本体: (v|ver|version)?[._]?\d+[._]\d+[._]\d+      (sep1 == sep2 を強制)
pre:  -<id>(.<id>)*                                (SemVer 2.0.0 仕様準拠)
meta: +<id>(.<id>)*                                (SemVer 2.0.0 仕様準拠)
```

- prefix `v` / `ver` / `version` は省略可 (例 `v1.2.3`, `version_1_2_3`)
- 本体セパレータは `.` または `_` のいずれか、両側で一致が必要 (DR-0003 + DR-0006)
- 本体に `-` セパレータは **不可** (pre-release の `-` と衝突するため)
- pre-release: `rc.0`, `alpha`, `beta.1` 等。数値のみ識別子は leading zero 禁止
- build metadata: `build.42`, `sha.5114f85` 等。leading zero 許容 (SemVer 仕様)

入力にあった prefix と sep は出力で**保持される**。

### bump 挙動 (drop デフォルト)

bump 時、`--pre` / `--build-metadata` を明示しない限り、既存の pre-release / build metadata は **drop** する (DR-0006)。

| 入力 | `patch` | `pre` | `pre --pre alpha` | `pre --no-pre` |
|---|---|---|---|---|
| `1.2.3`            | `1.2.4` | error: pre-release 不在 | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0`       | `1.2.4` (drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |
| `1.2.3-rc1`        | `1.2.4` | error: not incremental | `1.2.3-alpha` | `1.2.3` |
| `1.2.3+build`      | `1.2.4` (drop) | error: pre-release 不在 | `1.2.3-alpha` | `1.2.3` (nop) |
| `1.2.3-rc.0+build` | `1.2.4` (両 drop) | `1.2.3-rc.1` | `1.2.3-alpha` | `1.2.3` |

これは **npm 流の strip-don't-bump (`patch 1.2.3-rc.0 → 1.2.3`) とは異なる**。`patch` は常に patch を上げる、`--pre` / `--build-metadata` を明示しなければ drop、という単一規則を採用 (内部一貫性優先)。

### `pre` アクションの 3 モード

- **引数なし (`pre INPUT`)**: 既存 pre が `rc.N` のように末尾識別子が pure numeric のときのみ counter advance。それ以外 (`rc1` 等の英数字混在) はエラー
- **`--pre PRE`**: PRE 値で完全上書き (元 pre 有無問わず、巻き戻りも許容)
- **`--no-pre`**: pre 削除 (元 pre 不在でも nop)

### サポートするファイル形式

判定は **path-aware confidence ranked** (DR-0005)。各 FILE に対して確度順にルールを試行し、高確度ルールの path-pattern にマッチしても抽出失敗 (例: `.metadata.version` を持たない `marketplace.json`) なら次ルールへ降りる。最低確度の fallback ルールが top-level `.version` を持つ任意 `*.json` を網羅する。

| 確度 | パターン | 形式 | version パス | name パス |
|---|---|---|---|---|
| **3** (path-pinned) | `.claude-plugin/marketplace.json` | JSON | `$.metadata.version` | `$.name` |
| **3** | `.claude-plugin/plugin.json` | JSON | `$.version` | `$.name` |
| **3** | `package.json` | JSON | `$.version` | `$.name` |
| **3** | `package-lock.json` | JSON | `$.version`, `$.packages[""].version` | `$.name`, `$.packages[""].name` |
| **3** | `Cargo.toml` | TOML | `[package].version` | `[package].name` |
| **3** | `VERSION` | plain text | (ファイル内容) | — |
| **2** (basename) | 任意 dir の `marketplace.json` | JSON | `$.metadata.version` (try) | `$.name` |
| **2** | 任意 dir の `plugin.json` | JSON | `$.version` (try) | `$.name` |
| **2** | `v.mod` (V) | regex | `version: '...'` | `name: '...'` |
| **2** | `build.zig.zon` (Zig) | regex | `.version = "..."` | — |
| **2** | `mix.exs` (Elixir) | regex | `version: "..."` | — |
| **2** | `build.sbt` (Scala) | regex | `version := "..."` | — |
| **1** (fallback) | `*.json` | JSON | `$.version` | `$.name` |
| **1** (fallback) | `*.yaml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.yml` | YAML | `.version` (top-level) | `.name` |
| **1** (fallback) | `*.toml` | TOML | `version` (top-level) | `name` |
| **1** (fallback) | `*.xcconfig` (Xcode) | regex | `MARKETING_VERSION = ...` | — |
| **1** (fallback) | `*.podspec` (CocoaPods) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |
| **1** (fallback) | `*.nimble` (Nim) | regex | `version = "..."` | — |
| **1** (fallback) | `*.gemspec` (Ruby) | regex | `s.version = '...'` / `spec.version = "..."` | `s.name` / `spec.name` |

未対応ファイル (例: `README.md`, `Cargo.lock`) は `unsupported file: <path>` で明示エラー。新フォーマット追加 = テーブル 1 行追加 (+ 必要なら新 format-specific 関数 1 つ) で済む構造 (`--pattern` regex フラグは設計上持たない)。

YAML / TOML fallback (DR-0011) は **top-level キーだけ**を見る。section 配下 / nested mapping 配下の `version` は意図的に対象外。`Cargo.toml` は引き続き confidence-3 ルールが優先されるので `[package].version` の挙動は不変。multi-document YAML (`---` 区切り) は最初の document のみ。これらの新ルールでも DR-0010 の fallback hint が出る (`--no-hint` で抑制可能)。

DR-0012 の `regex` フォーマットは「version が 1 行のソースコード式で書かれる」8 つの言語マニフェスト (xcconfig / podspec / nimble / v.mod / build.zig.zon / gemspec / mix.exs / build.sbt) をカバーする。**最初のマッチ 1 個** だけが読み書きされ、quote style と version 行末尾のコメントは保持される。同一ファイル内で複数の version を同期更新する必要があるケース (Xcode `*.pbxproj` / `Info.plist` 等) は意図的にスコープ外。

#### Suffix-stripped fallback (DR-0013)

どのルールにも直接マッチしないパスは、basename 末尾の **backup 系 suffix を 1 段だけ剥がして** 既存ルール表で再試行される。採用されたルールの confidence は 1 段下げて報告 (3→2, 2→1, 1→1) され、`hint:` 行が stderr に出るので解決経路が透明に保たれる。多段 suffix (`Cargo.toml.bak.20260510`) は **末尾 1 段のみ** 剥がす (再帰しない)。

| Suffix | 例 | 解決先 |
|---|---|---|
| `.bak` / `.backup` / `.orig` / `.tmp` / `.old` | `Cargo.toml.bak` | `Cargo.toml` ルール (confidence 2) |
| `.YYYYMMDD` (8 桁数字) | `package.json.20260510` | `package.json` ルール (confidence 2) |
| `.YYYYMMDD_HHMMSS` (8+`_`+6 桁数字) | `Chart.yaml.20260510_120000` | `*.yaml` fallback (confidence 1) |
| 末尾 `~` (Emacs / vi 系) | `Cargo.toml~` | `Cargo.toml` ルール (confidence 2) |

```bash
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

Template 系 suffix (`.template` / `.example` / `.sample` / `.dist`) は **意図的に剥がさない**。中身が placeholder のことが多く、本物の manifest として静かに扱うのは現状の `unsupported file:` エラーよりも危険なため。template から抽出したい場合は backup 系 suffix にコピーすればよい (`cp Cargo.toml.template Cargo.toml.tmp`)。

suffix hint は既存の `hint:` prefix を共有し、`--no-hint` / `-q` / `-qq` で DR-0010 fallback hint と同じように抑制できる。両方発火するケース (`unknown.json.bak` → `.bak` 剥がし → `*.json` glob) では suffix hint が先に出る。

npm `package-lock.json` のみ特別扱い: lockfile v1 (npm 5/6) は `unsupported lockfileVersion: 1, please regenerate with npm 7+` エラー。依存エントリ (`$.packages["node_modules/..."]`) は仮に値が同じでも書き換わらない。

### 複数 INPUT: 整合性検証

複数 INPUT を渡すと 1 つの単位として処理される。全 INPUT 間で version は事前に一致している必要がある (不一致なら `version mismatch:` でカラム揃え縦列挙)。検出された package name も取れた範囲で整合性検証され、別プロジェクトのファイルを誤って一括 bump する事故を構造的に防ぐ。name は書き戻し対象ではない。

```bash
bump-semver patch package.json package-lock.json --write
bump-semver get   .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json
bump-semver patch 1.2.3 a.json b.json --write   # VER 引数で「期待値」を指定して整合性確認、結果は a/b に書き戻す
```

複数 INPUT 指定時の `get` は CI 用の整合性チェックとして機能する (`--write` 不要、全 version が一致しているかだけ検証)。

`--write` 時、書き戻し対象は **FILE 入力のみ**。VER / stdin 入力は参照値として整合性検証だけに使われる。`--write` 指定時に FILE 入力が 1 つもない場合はエラー (`--write requires at least one FILE`)。

### stdin パイプ

stdin がパイプ **かつ INPUT が単一の FILE のとき**、その FILE は名前ヒントとして扱われ、内容は stdin から読み込まれる (legacy ショートカット、後方互換)。複数 INPUT のときは stdin pipe は無視される。ファイルをチェックアウトせずにリビジョン間で比較したい時に有用:

```bash
jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml
```

`-` を INPUT として明示すれば、stdin から VER 1 行を読み込む新方式 (FILE 入力と混在可能):

```bash
echo 1.2.3 | bump-semver compare eq Cargo.toml -
```

### 使用例

```bash
bump-semver patch Cargo.toml --write              # bump + 書き戻し、新バージョンを出力
bump-semver minor package.json                    # メモリ上で bump、新バージョン出力 (ファイル不変)
bump-semver get .claude-plugin/plugin.json        # 現在のバージョン
bump-semver patch 1.2.3                           # 1.2.4 (VER 直接指定)
bump-semver patch v1.2.3                          # v1.2.4 (prefix を保持)
bump-semver minor version_1_2_3                   # version_1_3_0 (prefix + separator を保持)
bump-semver pre 1.2.3-rc.0                        # 1.2.3-rc.1 (counter advance)
bump-semver pre 1.2.3 --pre rc.0                  # 1.2.3-rc.0 (上書き)
bump-semver patch 1.2.3-rc.0 --pre rc.0           # 1.2.4-rc.0 (bump + pre 再付与)
bump-semver patch 1.2.3-rc.0 --no-pre             # 1.2.4 (drop して bump、確定昇格相当)
bump-semver compare lt 1.2.3-rc.1 1.2.3           # exit 0 (rc < 確定)
bump-semver compare eq .claude-plugin/plugin.json .claude-plugin/marketplace.json package.json   # 3 ファイル整合性チェック
bump-semver get   Cargo.toml --json               # jq 連携向け構造化出力
bump-semver patch Cargo.toml --json               # bump 後のバージョンを完全分解
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'   # ready to release? (CI: bump 済か確認)
bump-semver compare lt Cargo.toml vcs:origin/main      # stale vs remote main? (pull 必要か)
```

### JSON 出力 (`--json`)

`get` と bump 系 (`major` / `minor` / `patch` / `pre`) は `--json` を受け付ける。出力は末尾改行付きの JSON 1 行 (DR-0007)、`jq` にそのまま渡せる。`compare` は exit code が答えなので `--json` 非対応。

```bash
bump-semver get Cargo.toml --json
# {"name":"my-pkg","version":"1.2.3","semver":"1.2.3","major":1,"minor":2,"patch":3,"pre":null,"pre_id":null,"pre_rest":null,"build_metadata":null,"build_id":null,"build_rest":null}

bump-semver patch v_1.2.3-rc.1+build.42 --json
# {"name":null,"version":"v_1.2.4","semver":"1.2.4","major":1,"minor":2,"patch":4,"pre":null,...}
```

| フィールド | 型 | 内容 |
|---|---|---|
| `name` | string \| null | FILE 起源の name (例 `package.json $.name`) を集約。VER / stdin 起源では null |
| `version` | string | 入力フォーマット保持 (prefix + 本体 sep を維持) |
| `semver` | string | strict SemVer 2.0.0 形式 (prefix 除去 + 本体 sep を `.` に正規化) |
| `major` / `minor` / `patch` | int | 数値要素 |
| `pre` | string \| null | pre-release 識別子を `.` で結合した文字列 (例 `"rc.1"`)、不在なら null |
| `pre_id` / `pre_rest` | string \| null | `pre` を最初の `.` で分割。 `.` がなければ `pre_rest` は null |
| `build_metadata` | string \| null | build metadata 識別子の結合文字列 (例 `"build.42"`)、不在なら null |
| `build_id` / `build_rest` | string \| null | `pre` と同じ「最初の `.` で分割」ルール |

CLI が提供するのは **構造分解のみ**。「counter advance 可能か」のような意味判定はしない (必要なら `bump-semver pre VER` を実行して exit code を見る運用)。

### vcs: 入力

`vcs:` で始まる位置引数は VCS (jj または git) 経由で解決される。リリース前のドリフトチェックや「最新 tag より上げているか」の比較を、`jj file show | bump-semver compare lt - ...` のような shell パイプを書かずに 1 行で実現できる (DR-0008)。

```bash
# 直近のリリースタグより上げてるか (CI で push 前にチェック)
bump-semver compare gt Cargo.toml 'vcs:latest-tag()'

# 自分が main から遅れてないか (stale チェック)
bump-semver compare lt Cargo.toml vcs:origin/main

# 前コミットからバージョン変わってる?
bump-semver compare eq Cargo.toml vcs:HEAD~1            # FILE は相手から借用
bump-semver compare eq Cargo.toml vcs:HEAD~1:Cargo.toml # 明示形式
```

| 形式 | 解釈 |
|---|---|
| `vcs:REV[:FILE]` | `<REV>` 時点の `<FILE>` を VCS から読み出す。最初の `:` は `vcs:` プレフィックス、2 つ目の `:` で REV と FILE を分割。FILE 省略時は位置順で最初の sibling (FILE 起源 or `vcs:REV:FILE` 形式) から借用 |
| `vcs:latest-tag()` | 全 tag を取得し、semver パース不可なものは無視、SemVer 2.0.0 順序で最大を返す。0 件なら `no semver-compatible tags found` エラー |

**VCS 自動判定** (優先順):

1. `--vcs jj|git` フラグ (最優先)
2. `BUMP_SEMVER_VCS=jj|git` 環境変数
3. cwd または親に `.jj` ディレクトリ → jj
4. `.git` ディレクトリ → git
5. それ以外 → エラー (`not a git or jj repository`)

`.jj` と `.git` が並存している場合 (jj colocate モード、kawaz の git-bare + jj-workspace 構成) は **jj が優先**。jj の revset 言語は git ref のスーパーセットなので。

**`--write` と `vcs:` は排他**。VCS の中身に書き戻す機能は持たない (commit/amend が必要になりスコープ外)。混在させると `--write cannot be used with vcs: inputs (vcs: is read-only)` エラー。

**`bump-semver` は `git fetch` / `jj git fetch` を自動実行しない**。`vcs:origin/main` が古い場合は VCS 側のエラーがそのまま伝わる。CI では明示的に fetch してから bump-semver を呼ぶ運用にする。

CI スクリプトを VCS 中立にしたい場合は jj/git どちらでも通る形式 (`origin/main` (jj 側で `main@origin` に自動フォールバック) / commit hash / `latest-tag()`) を推奨。

### エラーメッセージの形式

エラーは stderr に `bump-semver: <reason>` として 1 行出力される。grep でフィルタする運用も想定し、起源 (VER / FILE) によってフォーマットを使い分けている。

**VER 起源** (位置引数または stdin 経由の生 semver 文字列):

```
bump-semver: rc1 is not incremental, use --pre PRE
bump-semver: 1.2.3 does not have a pre-release, use --pre PRE
```

**FILE 起源** (ファイルから読まれた version): file path + version field path で wrap される。

```
bump-semver: Cargo.toml:[package].version=1.2.3-rc1: rc1 is not incremental, use --pre PRE
bump-semver: package.json:$.version=1.2.3: 1.2.3 does not have a pre-release, use --pre PRE
```

**不一致エラー** (複数 INPUT で値がズレている場合): カラム整列で縦列挙される。

```
bump-semver: version mismatch:
  Cargo.toml:[package].version = 1.2.3
  package.json:$.version       = 1.2.4
  <argv>                       = 1.2.3-rc.1
```

起源ラベル: `<file>:<path>` (FILE 起源) / `<argv>` または `<argv:N>` (位置引数の VER) / `<stdin>` (`-` 経由)。

### 終了コード

- `0` — 成功 / compare 述語が真
- `1` — compare 述語が偽
- `2` — エラー (パース失敗、整合性 NG、未対応ファイル、排他オプション違反、IO エラー等)

## v0.4.x からの移行

v0.5.0 で 3 つの破壊変更が入っている。詳細とサンプルは [UPGRADING.md](./UPGRADING.md) を参照:

1. **`--value` フラグ廃止** → 位置引数で直接 VER を渡す (`bump-semver patch 1.2.3`)
2. **本体セパレータ `-` 廃止** → `.` または `_` を使う (`1-2-3` は不可)
3. **bump 系のエラー時 exit code 1 → 2** (compare 規約に合わせて統一)

## 開発状況

v0.9.0 で `regex` フォーマット (DR-0012) を導入。1 行 regex で書き換える汎用 format により 8 種類のファイル (`*.xcconfig` / `*.podspec` / `*.nimble` / `v.mod` / `build.zig.zon` / `*.gemspec` / `mix.exs` / `build.sbt`) を一括追加した。v0.8.0 で `*.yaml` / `*.yml` / `*.toml` の confidence 1 fallback (DR-0011)、v0.7.0 で `vcs:` 入力モード (DR-0008) — `vcs:REV[:FILE]` / `vcs:latest-tag()` で jj/git の他リビジョン・最新 tag を自動判定で取得。直前: v0.6.0 で `--json` 出力 (DR-0007)、v0.5.0 で pre-release / build metadata 対応 + `compare` サブコマンド + `pre` アクション + FILE/VER 統合 (DR-0006)。今後も「必要が出たら handler を 1 つ追加」(DR-0001) 方針で拡張する。設計判断は [docs/decisions/](./docs/decisions/)、将来検討項目は [docs/ROADMAP.md](./docs/ROADMAP.md) を参照。

## ライセンス

[MIT](LICENSE)
