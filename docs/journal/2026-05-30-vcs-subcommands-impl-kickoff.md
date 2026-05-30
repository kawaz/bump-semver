# 2026-05-30 vcs サブコマンド群 実装着手 (DR-0020 PR-1 kickoff)

同日付の [2026-05-30-vcs-subcommands-design.md](./2026-05-30-vcs-subcommands-design.md) (設計議論編) の続編。
設計議論で DR-0020 が確定したのを受け、実装着手のための調査・方針確定・PR-1 スコープ切り出しを記録する。

## 経緯

設計確定後、実装に着手する前に並列で 3 軸の調査を回した:

- **設計サマリ** (DR-0020 最終形の再確認): 9 サブコマンド、横断原則 (get/is 二分、path 必須、宣言的収束、commit 0 個も exit 0)
- **現状コード構造**: `src/` フラット単一 package、既存 `compare` サブコマンドと並列に `vcs` 親サブコマンドを追加する形が妥当
- **jj 実機マトリクス**: 0.41.0 で `jj tag set` / `jj git push --tags` の有無を確定

各調査で判明した実装上の制約:

| 観点 | 判明事項 |
|---|---|
| 既存 `vcs:` 入力モード | `resolveVcsInput` は version 入力の解決責務。新 `vcs` サブコマンド群とは責務が異なるので **別実装**。ただし `vcsListTags` / `vcsLatestTag` / `vcsFetchFile` は新 backend から薄く再利用 |
| tag remote push の唯一経路 | `jj tag set` (0.35+) は lightweight tag のみ、`jj git push --tags` は **存在しない** → `git -C $(git_target) push origin <tag>` のみ |
| bump-semver/main の VCS 構成 | git bare + jj colocated (workspace 方式)、`.jj/repo/store/git_target` が bare を指す。`vcs` backend は jj 側から git_target 経由で git CLI を叩く前提で良い |
| 2 階層 dispatcher | `vcs <verb>` (+ `vcs tag <verb>`) を組む必要あり。既存 `parseArgs` は 1 階層、ここで親サブコマンド対応を入れる |

## 確定方針 (kawaz 承認、2026-05-30)

| 論点 | 確定内容 |
|---|---|
| PR 分割粒度 | **7 PR** (PR-1 基盤+get / PR-2 is / PR-3 diff / PR-4 commit / PR-5 fetch+push / PR-6 tag / PR-7 移行+docs) |
| `vcs is clean` の untracked 扱い | **除外** (tracked のみ判定)。`--include-untracked` フラグは将来用に interface に隙間を残す (= 今回は未実装、シグネチャだけ予約) |
| exit code 規約 | `0`=真/成功 / `1`=偽 (is 系) / `2`=usage / `3`=VCS 実行エラー / `4`=曖昧・整合性違反 / `5`=non-ff push |
| jj 対応範囲 | **0.35+** をサポート、CI matrix は `0.35` / `0.41` / `latest` の 3 種 |

## PR-1 スコープ (最小 valuable)

「vcs サブコマンド群の基盤」+「最小のリードオンリー verb」をひとまとめに切る。
ここまで通れば後続 PR は同じパターンの繰り返しになる、というラインで設定:

- **共通基盤**
  - `vcsBackend` interface (`Root()` / `CurrentBranch()` / `Kind()` の最小 3 メソッド)
  - `gitBackend` / `jjBackend` 実装
  - jj 0.35+ バージョンチェック (起動時 or 初回 backend 取得時)
- **サブコマンド**
  - `vcs get root`
  - `vcs get backend`
  - `vcs get current-branch`
- **基盤コード**
  - exit code 規約定数定義 (`src/exit.go`)
  - `main.go` の `parseArgs` / `run` に `vcs` 親サブコマンド dispatcher 追加
  - `help.go` に `vcs` / `vcs get` のヘルプ追加 (kawaz の CLI 設計好み準拠: セクション分け / ロングオプション / `--help` デフォルト表示)
- **テスト**
  - `vcs_backend_test.go` (`gitBackend` / `jjBackend` を table-driven で)
  - `main_test.go` (`vcs get` の integration テスト)
- **docs**
  - README / README-ja に `vcs get` セクション追加
  - DR-0020 に「実装ノート」節を追加 (PR 分割粒度・exit code 規約・jj 対応範囲をリンク先として固定)

## 隙間判断 (実装中に決めるべき細部 → 以下の推奨で動く)

確定方針で詰めきらないが、実装中に手が止まるとマズいレベルの細部:

| 隙間 | 推奨 |
|---|---|
| `vcs is clean` untracked | 除外 (確定方針通り)。`--include-untracked` は interface 引数として予約のみ、PR-2 では default false |
| commit 引数エラー時の hint | backend の `Kind()` で動的に文言切替。git なら `"--staged を使うか PATH を指定"` 、jj なら `"PATH を指定 (commit -a は意図的に非提供)"` |
| `current-branch` の jj 定義 | `@-` (working copy parent) に紐づく bookmark が一意ならそれを返す。複数 or anonymous ならエラー (exit 4) |
| `vcs push` non-ff hint | 共通文言: `"remote has diverged. fetch and reconcile, then retry. (force push is intentionally not supported)"` |
| jj 設定依存 | `jj auto-track-bookmarks` 等の user config 前提に依存しない (CI 壊れ対策)。bookmark の track 状態は明示確認する |

## 次の action

- PR-1 を bookmark `vcs-pr1-base-and-get` で着手
- TDD で進める (失敗テスト先行 → 最小実装 → リファクタ)
- `jj describe` + `pkf run push` (品質ゲート通過) で PR 作成
- tag/Release は打たない (自動化に任せる、`release-flow-awareness.md` 準拠)
