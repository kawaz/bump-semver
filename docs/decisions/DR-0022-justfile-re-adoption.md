# DR-0022: Justfile re-adoption (Taskfile.pkl → Justfile への回帰)

- Status: Active
- Date: 2026-05-31

## Context

bump-semver の task runner は段階的に変遷した:

1. **初期**: Justfile (シンプル、ただし VCS 判定や定型 boilerplate が泥臭くなる)
2. **中期**: 長大化した shell スクリプトを外出しできるリモートインポート機構を持つ pkfire (`pkf-tasks` 経由で `vcs.*` / `semver.*` / `docs.*` / `migrate.*` を import) に移行
3. **DR-0020 land 後 (= v0.26.0)**: bump-semver 自身が `vcs` サブコマンド群 (get / is / diff / commit / push / tag) を実装。「git/jj を判定して分岐する長いスクリプト」が `bump-semver vcs <verb>` 一発で書けるようになり、Taskfile を泥臭く書き下す必要が消えた

中期で pkfire に逃したのは泥臭い部分への対症療法であり、

- 「特別綺麗なわけではなく臭いものに蓋をした」
- 「ルールがリモートにあることで深く理解せず使ったり、適当な劣化タスクをまた作ったりする」

という負債を抱えていた。DR-0020 以降は **「頻出パターンを単機能 CLI でシンプルに実行できる」** という、本来 kawaz が目指していた「臭いものを蓋せず分解する」アプローチが完成しているため、task runner を Justfile に回帰させても泥臭さは戻らない。

## Decision

Taskfile.pkl + pkfire/pkf-tasks 経由のフローを **Justfile に回帰**する。VCS 系の定型操作は `bump-semver vcs` サブコマンドにドッグフードで委譲し、Justfile 側は記述を最小に保つ。

### スコープ (今回の移行で達成)

- **Justfile を canonical task runner にする** (`just push` / `just bump-version` / `just ci` 等)
- **Taskfile.pkl は slim 化**: `docs:check-translations` (および leaves) のみを抱えた shim として残す。Justfile の `check-translations` recipe は `pkf run docs:check-translations` を shell out する
- CI workflow (`.github/workflows/ci.yml`) を `pkf run ci` → `just ci` に切替

### Justfile の構成原則

| 原則 | 適用 |
|------|------|
| 1 recipe 1 目的 | `lint-go` / `lint-pkl` / `lint` を分離。複合は dependencies で表現 |
| 複雑ロジックは外部委譲 | VCS 判定は `bump-semver vcs is/diff/commit/push` に委譲、Justfile 側は分岐ロジックを持たない |
| shell 拡張は最小 | 多行 if/then は `#!/usr/bin/env bash` shebang recipe で完結。inline `set -euo pipefail` で fail-fast |
| 表示順 = 利用頻度 | `default` → `lint`/`test`/`build` → `ci` → gates → `bump-version` → `push` の宣言順、`just --list --unsorted` (default recipe) で常時 surface |

### pkf 経由で残した機能と理由

| 機能 | 理由 |
|------|------|
| `docs:check-translations` (commit-lag + bilingual links) | 100+ 行の bash で auto-discovery / glob 展開 / VCS timestamp 取得を含み、Justfile 直書きでは複雑度が爆発する。「頻出パターンの単機能 CLI 化」は本 DR スコープ外、別タスクで `translation-pair` 系 CLI として切り出す候補 |

### 不採用にした選択肢 (drop, not ported)

- **`test:workflow` (`pkf affected --check`)**: pkfire の workflowTests DSL がないと意味を持たない。Justfile 化と同時に Taskfile DAG の概念ごと撤去するので、対応する gate も不要
- **`migrate:check-pkf-tasks-current` / `migrate:check-pkfire-current`**: Taskfile.pkl 自身の remote import が陳腐化していないかを監視する gate。slim Taskfile では `...kawaz.docs.tasks` だけを登録するため、これら migrate task 自体が pkf 経由でも露出しなくなる (= 完全に drop)。Justfile 主軸では Taskfile の更新は release の critical path から外れる (= 翻訳 shim だけが影響範囲) ため、shim 寿命までは pin staleness を許容する trade-off を取る。翻訳 CLI 化と同時に Taskfile.pkl ごと廃止する想定

### 残課題 (follow-up)

- 翻訳ペア check (`docs:check-translations` の中身) の単機能 CLI 化。実現後は Taskfile.pkl + pkfire への依存をゼロにできる
- 完了時点で本 DR を update し、Taskfile.pkl 削除 + `lint-pkl` recipe 撤去を別 DR (or follow-up commit) で実施

## Why pkfire → just へ戻すのか

pkfire は「リモートインポートでタスクライブラリを共有できる」という強みを持つが、本リポでは:

- 共有先が `pkf-tasks` 単一 + `vcs.*` の主要部分は DR-0020 で内製化済 → 共有のメリットが薄い
- 利用者 (= kawaz 一人 + AI agent) が深く理解せず使うとブラックボックス化する負債
- VCS 判定の堅牢性は `bump-semver vcs` に委ねれば Justfile 側は素朴に書ける

→ **「単機能 CLI が完成した領域から順にシンプルな task runner に回帰する」** という方針は、bump-semver の本来の動機 (「複雑だが頻出するパターンを単機能ツールで意図通りに実行する」) と整合的。pkfire は将来「単機能 CLI 化が困難な複雑タスク」を抱える別リポでは有効な選択肢として残る。

## Consequences

### Positive

- task 定義が `Justfile` に集約され、Pkl コードを読まずに全体把握できる (= 認知負荷低減)
- VCS 周りの定型操作が `bump-semver vcs` 経由になり、bump-semver 自身のセルフテストが日常的に走る (= ドッグフード強化)
- 残された pkf 依存が翻訳 check のみ = 次の単機能 CLI 化ターゲットが明確

### Negative / Trade-off

- `Taskfile.pkl` と `Justfile` が両方存在する過渡期が発生する (翻訳 CLI 化までの間)
- pkfire の workflowTests による Taskfile 構造変更検出が失われる (= Justfile では `just --list` の smoke test 以上のものは持たない)

## Related

- DR-0020: `vcs` サブコマンド群 (本 DR の前提)
- ROADMAP: 翻訳ペア check の単機能 CLI 化を follow-up に追加予定
