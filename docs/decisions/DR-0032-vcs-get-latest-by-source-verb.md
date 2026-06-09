# DR-0032: `vcs get latest-{tag,release}` への再整理 + input record 復活 + JSON schema 統一

- Status: Active
- Date: 2026-06-09
- Supersedes (再 supersede): DR-0020 PR-Tag-Latest (= `vcs tag latest [--source <tag|release>]` 設計を「source 軸を verb 名に畳む」方針で再整理)
- Partial revive: DR-0019 (= `vcs:latest-tag([REPO])` 入力 record を `vcs:latest-release([REPO])` と並列に復活、DR-0019 の `@`-peel fallback / 信頼境界・remote URL 受け取りロジックは流用)
- Related: DR-0007 (`--json` schema)、DR-0008 (`vcs:` 関数モード基本構文)、DR-0020 (`vcs` subcommand family 全体)、DR-0031 (`translateRev` 共通基盤化 — 本 DR とは独立、`expandRepoArg` / `translateRev` は別経路で維持)

## Context

DR-0020 PR-Tag-Latest (v0.29.0 land) で以下を決定した:

1. `vcs tag latest [--source <tag|release>] [--include-prerelease] [--repository REPO] [--raw | --json]` subcommand 新設
2. `vcs:latest-tag([REPO])` 入力 record を削除 (= deprecation 期間なしの即削除、v0 破壊的変更ポリシー)
3. JSON schema を独自 `{"tag":..., "version":..., "commit":..., "date":...}` で設計

v0.29.0 land 後の利用と review で以下の歪みが顕在化した:

### 問題 1: `vcs tag latest --source release` の責務不純

`vcs tag` family は「tag を扱う」verb 集 (tag push / tag delete / tag latest)。`--source release` を指定すると GitHub Release オブジェクトを読みに行くが、これは **tag そのものではなく Release entity** (= tag を付帯情報の 1 つに持つ別 entity)。

「tag 配下の verb が release を読む」のは naming 上の嘘で、help / docs の説明コストも高い (= `--source` flag の説明文で「ただし release は tag じゃない」を毎回断る必要がある)。`vcs tag list` を持たない設計判断 (= jj / git native のほうが richer) と整合性も悪い (= tag list を提供しないのに tag-from-release 取得を tag family に置く)。

### 問題 2: input record 削除の退化

DR-0008 で導入した `vcs:latest-tag()` 関数モードの **本質的価値** は `compare gt FILE 'vcs:latest-tag()'` の 1-liner ergonomic にあった。subcommand 化で:

```bash
LATEST=$(bump-semver vcs tag latest --include-prerelease)
bump-semver compare gt VERSION "$LATEST"
```

の 2 行 (= shell 変数キャプチャ必須) に退化。release.yml のような CI 文脈は元々 shell に降りるので影響軽微だが、justfile / pkfire / 手動コマンドラインでの ergonomic 損失は実在する。

DR-0020 PR-Tag-Latest 時点では「subcommand が richer なので duplication 避けた」が削除理由だったが、richer なオプション (= `--source` / `--include-prerelease` / `--raw` / `--json`) は **subcommand 経路だけで提供すれば足りる**。input record は default subset (= cwd の最大 stable tag を version 値で返す) を提供するだけで本質的価値を回復できる。

### 問題 3: JSON schema 非対称

`get --json` は 12 field の version schema (`name` / `version` / `semver` / `major` / `minor` / `patch` / `pre` / `pre_id` / `pre_rest` / `build_metadata` / `build_id` / `build_rest`) — DR-0007 で定めた標準形。

`vcs tag latest --json` は 4 field (`tag` / `version` / `commit` / `date`) — 独自 schema。実機確認すると `commit` / `date` は default で空文字 (= release path でしか populated されない)、実用情報は `tag` (= 生の tag string) と `version` (= bare semver) のみ。

問題:
- 同じ「version 値を返す verb」が異なる schema を返す → consumer 側で分岐実装が必要
- `tag` field の生 string 情報は `get --json` の `.version` (= raw 入力形保持) で代替可能
- `commit` / `date` は実用上 noise

## Decision

### 概念モデル

**source 軸を `--flag` ではなく verb 名に畳む**。`source = tag` / `source = release` を 2 つの独立した verb として表現する。

| 操作 | subcommand | input record |
|---|---|---|
| 最大 stable tag (cwd / 他 git repo) | `vcs get latest-tag [--repository R]` | `vcs:latest-tag([R])` |
| 最新 GitHub Release | `vcs get latest-release [--repository R]` | `vcs:latest-release([R])` |
| prerelease 含む | `--include-prerelease` (両 verb 共通) | input record は stable のみ固定 |
| 出力形式 | `--json` (version schema) / text default | input record は単一 version 値 |

### 設計原則

#### 原則 1: source 軸 = verb 名 (= flag にしない)

`--source` flag を持たない。「tag を読む」と「release を読む」は責務が異なる 2 verb として並列定義する。

利点:
- 名前と責務が一致 (= `latest-tag` は git/jj tag list、`latest-release` は GH Release object)
- `vcs get` family の「読む」セマンティクスと整合
- 関数モード (input record) でも option を持ち込まずに symmetric な 2 verb で表現できる (= DR-0008 の「関数モードは raw string 引数のみ」精神と整合)
- 将来 source が増えたら新 verb 追加で対応 (= `vcs get latest-manifest` / `vcs get latest-changelog` 等)、option 軸を肥大化させない

#### 原則 2: subcommand と input record は symmetric

subcommand `vcs get latest-tag` と input record `vcs:latest-tag()` は **完全に対称な命名・責務** を持つ。

| 軸 | subcommand | input record |
|---|---|---|
| verb 名 | `latest-tag` / `latest-release` | `latest-tag` / `latest-release` (同名) |
| repository 引数 | `--repository R` | `(R)` 関数引数 |
| prerelease 取り込み | `--include-prerelease` flag | **非対応** (= stable only 固定) |
| 出力形式 | text default / `--json` で version schema | 単一 version 値 (= 関数モードは scalar 返却が本質) |
| 利用文脈 | shell script / CI / capture-then-compare | `compare gt FILE 'vcs:latest-tag()'` の 1-liner |

利用者は同じ概念モデル (= 「最新 tag を読む」「最新 release を読む」) で 2 つの構文を使い分けられる。subset の境界 (= prerelease 取り込み / JSON 出力は subcommand のみ) は input record の関数モード制約から自然に決まる。

#### 原則 3: JSON schema を version schema に統一

`vcs get latest-tag --json` / `vcs get latest-release --json` の出力は **`get --json` と同一の 12 field version schema** を返す。

```jsonc
{"name":"pkf-tasks","version":"pkf-tasks@0.0.13","semver":"0.0.13","major":0,"minor":0,"patch":13,
 "pre":null,"pre_id":null,"pre_rest":null,
 "build_metadata":null,"build_id":null,"build_rest":null}
```

- `.version` = 生 tag / release name 文字列 (= 旧 `--raw` 相当の情報を内包)
- `.semver` = canonical bare 形 (= 旧 default text 相当)
- `.name` = monorepo / multi-component 前置情報 (= `pkf-tasks@0.0.13` の `pkf-tasks`)、無ければ null
- `commit` / `date` field は廃止 (= 実機確認で default 空文字、実用情報ゼロ)

これにより `get vcs:latest-tag() --json` と `vcs get latest-tag --json` は完全同一出力になる (= input record と subcommand の symmetric 化が JSON 経路でも貫徹)。

#### 原則 4: `--raw` 廃止

旧 `vcs tag latest --raw` (= 生 tag 文字列を返す flag) は `--json` の `.version` field で代替可能 (= 情報的に redundant)。

text mode 限定の補助 flag を持たないことで:
- 「value (= version) と ref name (= raw tag string) を 1 verb に混在させる責務不純」が解消
- 生 tag string が必要な利用者は `vcs get latest-tag --json | jq -r .version` または `git tag --sort=-version:refname | head -1` で取得
- subcommand の output shape が「default = bare semver / `--json` = version schema」の 2 mode に simplify

#### 原則 5: `expandRepoArg` と `translateRev` は別経路を維持 (DR-0031 との整合)

DR-0031 で `translateRev` (= jj の `bookmark@remote` ↔ git の `remote/bookmark` 相互翻訳) が rev 受信系 (`FetchFile` / `Diff` / `vcs:REV[:FILE]` 等) に集約された。本 DR の `--repository` / `vcs:latest-{tag,release}(R)` の `R` は **repository 引数** であって rev ではない。

`expandRepoArg` (= `<owner>/<repo>` → `https://github.com/<owner>/<repo>` 展開、HTTPS/SSH URL passthrough) と `translateRev` は字面上どちらも「`/` を含む文字列」を扱うが、**文脈 (関数の責務 / 呼び出し位置) で意味が確定する**。共通 normalize 層に乗せようとしない (= 字面の類似で誤って merge しない)。

具体的:
- `vcs:latest-tag(kawaz/bump-semver)` → 関数名で「repo 引数」と確定 → `expandRepoArg` 経路
- `vcs:main@origin` の REV 部分 → `vcs:REV` schema で「rev 引数」と確定 → `translateRev` 経路

### CLI surface (確定形)

```
bump-semver vcs get latest-tag [--include-prerelease] [--repository REPO] [--json]
bump-semver vcs get latest-release [--include-prerelease] [--repository REPO] [--json]
```

- 両 verb とも default text 出力 = bare SemVer (`1.2.3`)
- `--json` 指定で version schema (12 field)
- `--include-prerelease` で SemVer prerelease (`1.2.3-rc.1` 等) を candidate に含める
- `--repository REPO`:
  - `latest-tag --repository R` → `git ls-remote --tags <R>` 経路 (gh 不要)
  - `latest-release --repository R` → `gh release list -R <R>` 経路 (gh 必要)
- `--repository` 省略時は cwd VCS

`vcs tag latest` subcommand は **削除** (= v0 破壊的変更ポリシー、deprecation 期間なし)。

### Input record surface (確定形)

```
vcs:latest-tag([REPO])
vcs:latest-release([REPO])
```

- 引数省略 = cwd VCS
- 引数 `REPO` = `owner/repo` 短縮 or full HTTPS/SSH URL (DR-0019 と同 schema、`expandRepoArg` で正規化)
- 返却 = 単一 version 値 (= bare SemVer 形、= subcommand `--json` の `.semver` 相当)
- prerelease は **常に除外** (= stable のみ、`--include-prerelease` 相当不可)

利用例:
```
bump-semver get 'vcs:latest-tag()'                       # cwd の最大 stable tag
bump-semver get 'vcs:latest-release(kawaz/pkf-tasks)'    # 他リポの最新 GH Release
bump-semver compare gt VERSION 'vcs:latest-tag()'        # 1-liner CI 比較
```

DR-0008 の関数モード dispatch (`resolveVcsFunc`) で `case "latest-tag"` を **復活** + `case "latest-release"` を **新設**。DR-0019 の `expandRepoArg` / `@`-peel fallback / 信頼境界記述はそのまま流用 (= remote URL の正当性は呼び出し側責任)。

### subcommand と input record の使い分け推奨

| 場面 | 推奨 |
|---|---|
| CI / release workflow (= shell 内で複数 step) | subcommand (= `--include-prerelease` / `--json` の richer option が要る) |
| Justfile / pkfire / 手動コマンドライン (= 1-liner ergonomic 優先) | input record |
| pre-release も判定したい | subcommand 一択 (= input record は stable only) |
| プログラム消費 (= JSON parse) | subcommand `--json` |

### JSON schema 統一の含意

`compare --json` を新設しない (= 比較結果 schema は version schema と異質、統一不可能、不要)。

## 代替案検討

### 不採用: `--source <tag|release>` flag 維持 (= DR-0020 PR-Tag-Latest 形を保つ)

責務不純 (= tag family が release を読む)、input record で source を表現できない (= 関数モードに keyword args 持ち込みが必要、DR-0008 精神に反する)。今回の再整理動機の主因。

### 不採用: `vcs get latest-version --source tag|release` (= verb 名統一 + flag で source)

中間案。subcommand は綺麗だが input record 側で source 軸の表現に詰まる (= `vcs:latest-version(source=release)` を許すと keyword args 一般化が必要、DR-0008 が「関数モードは raw string 引数のみ」と決めた原則に反する)。

```
不採用案: vcs:latest-version([REPO])  # source は省略可能? 不可能?
```

source を input record で表現できないなら subcommand との symmetric 性が崩れる (= subcommand では source 切替可能、input record では不可、という非対称)。verb 名に source を畳む案 (= 採用案) のほうが symmetric。

### 不採用: input record 復活させず subcommand のみ (= DR-0020 PR-Tag-Latest 形を維持、source 軸だけ verb 名に畳む)

1-liner ergonomic 損失 (= DR-0008 で確立した justfile / 手動利用文脈の利便性) が回復しない。bump-semver の input mode (= DR-0008 schema 全体) の整合性も「rev mode は使えるが関数モードは empty」という歪さが残る。

### 不採用: `commit` / `date` field を JSON schema に保持

実機確認で default 空文字、release path でしか populated されない。`get --json` schema との非対称コストを払って維持する利得が小さい (= GH Release の `publishedAt` を直接欲しい利用者は `gh release list --json` 直接呼び出しのほうが情報が richer)。

### 不採用: `--raw` flag 保持

text mode 限定の補助 flag、JSON `.version` field で代替可能。「value (version) と ref name (raw tag) を 1 verb に混在」する責務不純を解消するため廃止。

## 影響範囲 / migration

### 内部実装

- `src/cmd_vcs_tag_latest.go` (272 行) → `src/cmd_vcs_get_latest_tag.go` / `src/cmd_vcs_get_latest_release.go` に分割 (= 共通 helper は `src/vcs.go` に集約継続)
- `src/cli_parse.go` line 643-680 の `vcsTagOpts` から `LatestSource` / `LatestRaw` field を廃止、新 vcsGet dispatch に `latest-tag` / `latest-release` を追加
- `src/vcs_cmd.go` line 39 (`vcsGetKeys`) に `latest-tag` / `latest-release` 追加、line 93-111 の switch に case 追加
- `src/resolve.go` の `resolveVcsFunc` (旧「unknown vcs function」エラー emit) に `case "latest-tag"` / `case "latest-release"` を復活
- `src/help.go` line 985-1040 の `vcs tag latest` 説明を `vcs get latest-tag` / `vcs get latest-release` に移行
- `pickLatestSemverTag` (= `src/vcs.go` line 347) は両 verb と input record で共有
- JSON serialize 経路は `get` の version schema serializer (= `versionToJSON` 相当) を再利用
- `expandRepoArg` / `translateRev` は別経路を維持 (= 原則 5、DR-0031 と整合)

### test 移行

- `src/cmd_vcs_tag_latest_test.go` (350 行 / 1 関数 / 9 subtests) → `cmd_vcs_get_latest_tag_test.go` / `cmd_vcs_get_latest_release_test.go` に split
- `src/cmd_vcs_input_test.go` の `TestRun_VcsInput_LatestTag_Removed` (= 削除確認 test) を `TestRun_VcsInput_LatestTag` / `TestRun_VcsInput_LatestRelease` に書き換え (= 機能 test 化)
- DR-0019 期の `vcs:latest-tag(REPO)` test 観点 (= `@`-peel / monorepo-style / URL 各形式) を新 input record にカバー

### 外部 user 影響 (v0 break)

- **`.github/workflows/release.yml`** (= 本リポ): `vcs tag latest --include-prerelease` → `vcs get latest-tag --include-prerelease` に書き換え
- **`kawaz/pkf-tasks` の `migrate:check-pkf-tasks-current`** (DR-0019 / DR-0020 で言及): 旧 `vcs tag latest --repository kawaz/pkf-tasks` から `vcs get latest-tag --repository kawaz/pkf-tasks` への置換、または `vcs:latest-tag(kawaz/pkf-tasks)` 復活経路へ
- **kawaz personal rules** `release-flow-awareness.md`: 既に source-agnostic な形に簡略化済 (= 触らない)

### v0.31.x → v0.32.0 移行 transient

DR-0020 PR-Tag-Latest と同種の self-dogfooding transient が発生する: `check-version` job が install する「直前 release 版バイナリ」は v0.31.x で、これは `vcs get latest-tag` を知らないので unknown verb で exit 2。release.yml が exit code != 0 を bootstrap 分岐に流す設計を維持していれば動作する (= 「VERSION > 既存 tag」検証はこの 1 回スキップ、二重 release は後段の `gh release view` が防ぐ)。次の release 以降は v0.32.0+ が install されるため通常分岐に乗る。

### CHANGELOG / docs 更新

- CHANGELOG v0.32.0 entry に「`vcs tag latest` 削除」「`vcs get latest-tag` / `vcs get latest-release` 新設」「`vcs:latest-tag()` / `vcs:latest-release()` 入力 record 復活 (DR-0020 で削除した v0.29.0 から再導入)」「JSON schema を version schema に統一」「`--source` / `--raw` flag 廃止」を明記
- `docs/DESIGN.md` / `docs/DESIGN-ja.md`: `vcs` family table の `tag latest` 行を削除、`get` family table に `latest-tag` / `latest-release` 行を追加
- `docs/decisions/DR-0020-vcs-subcommands.md` PR-Tag-Latest section に「Superseded by DR-0032 (2026-06-09)」注記を追加 (= DR-0019 を DR-0020 が supersede した方式と同じ)
- `docs/decisions/INDEX.md` の DR-0020 行に supersede 関係注記、DR-0032 を Active section に追加

## v0 段階の破壊的変更ポリシー (DR-0020 で明文化済、再確認)

bump-semver は v0.x = 不安定版 (kawaz 個人 OSS 運用規約)、破壊的変更は minor bump で許容、deprecation 期間なし。本 DR の即削除 (= 旧 `vcs tag latest` / 旧 JSON schema) もこの規約下で実施する。利用者影響は CHANGELOG / migration 例 で告知する。

## land 順

1. DR-0032 起票 (本 file) + DR-0020 PR-Tag-Latest section に supersede 注記 + INDEX 更新 (= 本 DR)
2. 実装: `vcs get latest-{tag,release}` subcommand + 旧 `vcs tag latest` 削除 + JSON schema 統一
3. 実装: input record `vcs:latest-{tag,release}([REPO])` 復活 + 新設
4. test 移行: subcommand 用 + input record 用
5. release.yml / DESIGN / DESIGN-ja 更新
6. CHANGELOG + VERSION bump (v0.32.0)
7. push (= `just push` 経由) → CI watch → release workflow が tag / GH Release を自動作成

## 補足: 関連 OSS への波及

DR-0019 言及の `kawaz/pkf-tasks` の dogfood が `vcs:latest-tag(kawaz/pkf-tasks)` ベースで書かれていた場合、本 DR でその構文が **復活** するので migration なし (= v0.28.x → v0.29.0 で書き換えが必要だった分が v0.32.0 で再び不要になる、という挙動)。ただし v0.29.0 で `vcs tag latest --repository` 経路に migration 済の場合は `vcs get latest-tag --repository` への再書き換えが必要 (= subcommand 経路も rename された)。
