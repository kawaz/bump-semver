# 2026-05-30 vcs サブコマンド群の設計議論 + jj 一次情報調査

DR-0020 (`vcs` サブコマンド群) の決定に至る設計議論の経緯と、その過程で取った jj (Jujutsu) の一次情報調査結果を記録する。

## 経緯の要点 (ハマり所 → 解決のペア)

議論は「Taskfile/justfile に jj/git 分岐を毎回手書きする板挟み」から出発し、`vcs` サブコマンド群へ収束した。各論点の決着:

- **置き場 (bump-semver か独立ツールか)**: 当初「semver ツールに push/commit は責務逸脱」と考えたが、bump-semver の動機が「頻出する複雑パターンを堅牢シンプルに吸収」であり VCS 判定もその射程内、と再定義。bump-semver に集約 (DR-0020 不採用案 1)
- **commit の path 必須**: 「jj/git の staging 差の吸収」と誤解していたが、真の理由は **巻き込み事故防止** (jj はカレントコミット≒working tree で並列作業時に他ファイルを混ぜやすい)。素の git/jj より厳しい意図的制約、と位置づけ直した
- **全部モードの命名**: `--all` (commit -a 誤読) → `--all-in-staged/commit` (冗長) → **`--staged`** に収束。jj ユーザーも「自動ステージ」を理解しているので staged は共通理解語彙。ここから命名規律「共通理解語彙は単一/VCS 固有語彙は併記+注釈」を抽出
- **squash**: 独立サブコマンド案 → commit とオプション体系が完全同一と判明 → `commit --amend` フラグに統合 (DRY)
- **tag の force**: 「force」では冪等リカバリと別 rev 移動が混ざり意図が曖昧 → **`--allow-move`** で分離 (同 rev 冪等は無条件、別 rev 移動のみ明示許可)。delete は `rm -f` 類推で **デフォルト冪等** (`--allow-missing` 不要)
- **help の VCS 依存**: 動的 visibility 案 → 共有齟齬・completion 環境依存の懸念 → **「help は環境非依存 (契約)、エラー/hint は文脈適応 (実行時ガイド)」** の線引きで解決
- **tag 実装方式**: 直接 `git tag` (A 案) か `jj tag set` (B 案) か → jj 調査で v0.35+ の `jj tag set` を確認、jj が tag を把握し ref 乖離を抑える B 案を採用

## jj 一次情報調査 (確定事実)

一次情報のみ (jj 公式 docs `docs.jj-vcs.dev`、GitHub `jj-vcs/jj` の `docs/`・`CHANGELOG.md`・issue)。`martinvonz/jj` は `jj-vcs/jj` にリネーム済み。

### export / import の向きと中身

- `jj git import` = git→jj、`jj git export` = jj→git ([git-compatibility.md](https://github.com/jj-vcs/jj/blob/main/docs/git-compatibility.md))
- export が書き出すのは主に **ref** (bookmark→`refs/heads`、v0.35+ は local tag→`refs/tags` を lightweight として)。commit object は jj 操作時点で既に git object store に存在 (GitBackend が gitoxide で直接書き、`refs/jj/keep/` で GC 防止) ([technical/architecture.md](https://github.com/jj-vcs/jj/blob/main/docs/technical/architecture.md))

### export の自動性と失敗ケース

- **colocated は毎コマンド自動 import/export** ＝頻繁な自動 export は jj 標準運用で安全側 ([git-compatibility.md](https://github.com/jj-vcs/jj/blob/main/docs/git-compatibility.md))
- ただし **export は失敗しうる** (無条件には安全でない):
  - ref 階層衝突 (`refs/heads/test` と `refs/heads/test/foo` 共存で lock 不能) — [issue #493](https://github.com/jj-vcs/jj/issues/493)
  - HEAD 期待値と実値の競合 (`The reference "HEAD" should have content X, actual was Y`) — [issue #6098](https://github.com/jj-vcs/jj/issues/6098)
  - packed-refs 関連不具合 — [issue #6203](https://github.com/jj-vcs/jj/issues/6203)
- → **push 後に export をセットにするなら exit code チェック必須、失敗を握りつぶさない**
- 「export は冪等」と明言した一次情報は見つからず (毎コマンド自動実行の事実が実質的に裏付けるが、公式明言ではない＝不明点)

### colocated / bare

- colocated = `.jj` と `.git` が working copy を共有する hybrid。`jj git init`/`clone` のデフォルト、毎コマンド自動同期
- **bare git で colocated 不可**: colocated 化には `git config --unset core.bare` と HEAD 設定が必須。bare は working copy を持たないので「working copy 共有」という colocated の定義と両立しない (公式の禁止理由の明文記述は無し＝定義上の必然と理解)
- **bare は backing store としては正規サポート**: `jj git init --git-repo=<bare>` で非 colocated 構成可。git worktree に似た挙動で commit は両 repo からアクセス可。**kawaz の git bare + jj workspace はこれに該当**

### tag (直近で大きく変化、バージョン依存に注意)

- **v0.35.0 (2025-11-05) で `jj tag set`/`delete` 追加**。作成/更新した tag は git へ **lightweight tag として常に export** ([CHANGELOG.md](https://github.com/jj-vcs/jj/blob/main/CHANGELOG.md)、[issue #7908](https://github.com/jj-vcs/jj/issues/7908))
- **tag の remote push に jj ネイティブ手段は無い** (`jj git push --tags` 相当なし)。remote へ送るには native `git push` フォールバックが現状 ([#7908](https://github.com/jj-vcs/jj/issues/7908))。→ DR-0020 の「作成は jj tag set、push は git」は正当
- **lightweight tag 1 つで対象 commit が jj 上 immutable 扱い**: 「JJ considers all Git tags ... like annotated tags. So as soon as there is a lightweight tag ..., the respective commit is considered immutable」([#7908](https://github.com/jj-vcs/jj/issues/7908))。確定 commit に打つ前提なら安全装置
- **change-id (可変) vs commit-id (不変)**: tag は git の immutable commit-id を指すので jj の change 書き換えを追わない。古い commit object は `refs/jj/keep/` で GC 保護されるが、tag は「その瞬間のスナップショット」。**tag は「もう動かさない commit」に打つ**
- 旧 `git-compatibility.md` は「lightweight tag は作れるが annotated tag は作れない / tag は Partial」と**ドキュメント追従の遅れ**あり。実挙動はインストール済み jj バージョン依存

### 不明点 (実機検証推奨)

- export の内部差分アルゴリズム (`git.rs` 要読)
- 「外部 bare backing + jj tag set + native git push tag」の具体挙動 (一次ドキュメント無し、合成推論)
- `jj tag set` 製 tag を `jj git push` で送れるようになったか (CHANGELOG 上は push 側 tag 未実装に見えるが最新版要確認)

→ 実装着手時、対象 jj バージョンで `jj git init --git-repo=<bare>` → `jj tag set` → native `git push` tag → `jj git import`/`export` の挙動マトリクスを取る。

## 主要出典

- [git-compatibility.md](https://github.com/jj-vcs/jj/blob/main/docs/git-compatibility.md) / [docs.jj-vcs.dev](https://docs.jj-vcs.dev/latest/git-compatibility/)
- [CHANGELOG.md](https://github.com/jj-vcs/jj/blob/main/CHANGELOG.md) — v0.35.0 tag set / v0.38.0 tag export 修正
- [technical/architecture.md](https://github.com/jj-vcs/jj/blob/main/docs/technical/architecture.md) — GitBackend / refs/jj/keep
- issues: [#493](https://github.com/jj-vcs/jj/issues/493) / [#6098](https://github.com/jj-vcs/jj/issues/6098) / [#6203](https://github.com/jj-vcs/jj/issues/6203) / [#7908](https://github.com/jj-vcs/jj/issues/7908)
