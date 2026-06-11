# DR-0036: パッケージ分割は見送り、ファイルレベル整理を採用

- Status: Active
- Date: 2026-06-11
- Related: 旧 issue `docs/issue/2026-06-10-package-split-consideration.md` を解決

## Context

`src/` 配下は単一の `main` パッケージ (フラット約 80 ファイル) で構成されており、
そのうち `vcs_backend.go` が 2184 行に達していた。責務の境界は概念的には明確で
(`format` 系 / `vcs` 系 / `cli` 系)、Go の `internal/` 配下へのパッケージ分割が
候補として挙がっていた。

着手前に依存関係を実地調査したところ、以下が判明した:

- **rules ⇄ format の双方向結合**: 両者は共通型 (`Rule` / `FormatResult` 等) と
  ディスパッチを介して相互参照しており、別パッケージに割ると即 import cycle になる。
- **resolve ⇄ vcs の潜在的双方向**: 現状は片方向に見えるが、共通型の所属次第で
  容易に循環し得る境界。
- **`exitErr` の複数層への染み出し**: 終了コードを運ぶ `exitErr` が CLI dispatch / cobra /
  compare / vcs の各層から参照されており、パッケージ分割するには共有型パッケージの新設が前提。
- **export 改名の規模**: パッケージ境界を引くと 100 を超える識別子の export 改名が
  必要になり、同一パッケージ内テスト約 20 本も全面書き換えになる。

単一バイナリ CLI という規模に対して、この設計コストは得られる便益を上回る。
現状の「全 unexported + 薄いアダプタ」設計はそれ自体一貫しており、可読性の主たる
痛点は「1 ファイルが大きすぎる」ことに局所化されていた。

## Decision

### 1. パッケージ分割 (`internal/` 化) は見送る

import cycle・共有型パッケージ新設・大規模 export 改名・テスト全面書き換えの
コストが、単一バイナリ CLI の規模に対して便益を上回るため採用しない。現設計の
「全 unexported + 薄いアダプタ」の一貫性を維持する。

### 2. `vcs_backend.go` をファイル 3 分割する (同一パッケージ内)

可読性の痛点 (2184 行の単一ファイル) には、パッケージ境界を引かずに **同一
パッケージ内のファイル分割**で対応する。export 改名もテスト変更も不要:

- `vcs_backend_git.go`: `gitBackend` のメソッド群 + git 固有ヘルパ
  (`buildGitPathspec` / `trimGlobPrefix` / `gitTagPushRemote` / `gitTagDeleteRemote` /
  `resolveGitRev` / `existingGitTagSHA`)
- `vcs_backend_jj.go`: `jjBackend` のメソッド群 + jj 固有ヘルパ
  (`autoAdvanceBookmark` / `jjGitExport*` / `buildJjPathspec` / `normalizeJjNameStatus` /
  `resolveJjRev` / `existingJjTagSHA` / `jjStringLiteral` 等)
- `vcs_backend.go` (本体): `vcsBackend` interface・各 opts 型・`newVcsBackend` factory・
  両 backend 共通ヘルパ (`runBackendCmd` / `runBackendExitCode` / `runBackendCapture` /
  `filterExistingPaths` / `writePushDiagnostic` / `decideTagPush` / `formatPushError` /
  `isNonFastForward` / `shortSHA` / `parseEpochOrZero` 等)

`vcs_backend_test.go` も対応する 3 ファイル (`_git_test` / `_jj_test` / 共通) に分割する。

分割は宣言の cut & paste のみで行い、コードは 1 文字も書き換えない (package 宣言と
import 文の再構成のみ例外)。分割前後で全宣言のシグネチャ集合が一致することを
機械的に確認した。

## 不採用案

### (a) `internal/` フルパッケージ分割

`format` / `vcs` / `cli` を別パッケージに切り出す案。Context の通り rules ⇄ format /
resolve ⇄ vcs の双方向結合と `exitErr` の複数層への染み出しにより、共有型パッケージ新設 +
100 識別子超の export 改名 + テスト約 20 本の書き換えが必要。単一バイナリ CLI の
規模に対し設計コストが便益を上回るため不採用。

### (b) `vcs` 系だけをパッケージ分割

最も独立性が高く見える `vcs` 系のみを別パッケージに切り出す案。しかし `vcs` も
`exitErr` と translateRev 周辺で本体パッケージと結合しており、切り出すには結局
共有型パッケージの先行整備が要る。現時点で `vcs` を独立利用する需要もないため、
コスト先払いの価値がない。同一パッケージ内ファイル分割で可読性の痛点は解消できる。

## 再検討トリガ

以下が発生したらパッケージ分割を再評価する:

- `vcs` 系を別ツール (独立 CLI / ライブラリ) として切り出す需要が出たとき。この場合は
  共有型パッケージの先行整備コストを払う正当性が生まれる。
- `format` 系プラグインを外部パッケージから登録したい等、パッケージ境界が機能要件に
  なったとき。
- 単一パッケージのビルド時間 / テスト実行時間が無視できない規模に膨らんだとき。
