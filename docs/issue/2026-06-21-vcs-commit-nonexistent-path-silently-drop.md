---
title: vcs commit が削除された path を黙殺する (nonexistent-path silently drop の考慮漏れ)
status: wip
category: bug
created: 2026-06-21T12:15:49+09:00
last_read:
open_entered: 2026-06-21T12:15:49+09:00
wip_entered: 2026-06-21T12:22:31+09:00
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: 自リポ TODO
---

# vcs commit が削除された path を黙殺する (nonexistent-path silently drop の考慮漏れ)

## 概要

`vcs commit -m "msg" <files...>` でパス指定する際、そのパスが削除済み（git で言う "deleted" 状態）の場合、コマンドが黙殺してそのパスを commit に含めないことがある。

## 背景

`push-workflow.md` ルール等で「自分が修正したファイルだけをパス指定して固定する」運用を推奨している。削除操作を含む change を commit しようとした際に、削除済みのパスを指定しても commit に含まれずサイレントに無視される挙動が発生する可能性がある。

これにより以下の問題が起きる:

- 削除したつもりのファイルが commit に含まれず、次の commit や push で思わぬ状態になる
- エラーが出ないため問題に気づけない
- パス指定を信頼する運用のもとでは、特に見落としやすい

関連 issue: [vcs-commit-path-include-deletes](./2026-06-20-vcs-commit-path-include-deletes.md) (削除を含めるオプション追加の要望)

## 受け入れ条件

- [ ] 存在しないパス（削除済みを含む）を `vcs commit <path>` に指定したとき、警告またはエラーを出す
- [ ] または、削除済みパスも commit に含める動作に変更する（`vcs-commit-path-include-deletes` issue と連携）
- [ ] いずれの場合も黙殺（silent drop）しない

## TODO

<!-- wip 時のみ -->

- [ ] 実際の挙動を実機で確認（削除済みパス指定時に何が起きるか）
- [ ] git / jj それぞれの経路での挙動差を確認
- [ ] 修正方針を決定（警告出す or 削除も含める）

---

## 方針確定 (2026-06-21, kawaz 判断)

「内部で deleted/untracked を区別して両立 (非破壊)」案は採用しない。**デフォルト反転 (= git 文脈に合わせる) で確定**。

理由 (kawaz):
- git 文脈に合わせた挙動の方が自然 (= principle of least astonishment)
- 「無視成功」は bump-semver 固有用途であり、justfile に一度書いて終わる類の操作なので、例外動作をデフォルトにすべきでない

= 破壊的変更だが移行コストは小さい (justfile 内のリリース処理が大半、現物 file 名固定が多い)。

## 利用箇所 grep 結果 (2026-06-21)

ローカル全リポ grep (`~/.local/share/repos/github.com/kawaz/` + `kawaz123/`) で `bump-semver vcs commit` の呼び出しを列挙。`--staged` 型は今回の変更で影響なし、path 指定型のみ移行対象。

### path 指定型 (= 要確認、ただし全 path 常時存在なら移行不要)

- `kawaz/bump-semver/main/justfile` (VERSION)
- `kawaz/{jj-worktree, stable-which, cache-warden}/main/justfile` (Cargo.toml Cargo.lock)
- `kawaz/claude-cmux-msg/main/justfile` ({{ version-files }})
- `kawaz/{claude-plugin-reference, claude-gh-monitor, claude-nandakke, claude-push-guard}/main/justfile` (.claude-plugin/plugin.json .claude-plugin/marketplace.json)
- `kawaz/claude-statusline/main/justfile` (package.json)
- `kawaz/claude-local-issue/main/justfile` (VERSION)
- `kawaz/dotfiles/justfile` (flake.lock)
- `kawaz/claude-local-issue/main/commands/{write,read,update,migrate}.md` (skill 内、path 指定で issue file + INDEX.md を commit)

### --staged 型 (= 影響なし)

- `kawaz/hyoui/{main, verify-sighup}/justfile`
- `kawaz/claude-local-issue/main/commands/update.md` の close フロー (= 本 issue 解決後に撤去する暫定 workaround)

### 移行コスト評価

- 単一 file 固定型 (Cargo.toml + Cargo.lock 等) は対象 file が必ず存在するため、デフォルト反転後も挙動不変 → 移行不要
- 真に影響を受けるのは「複数候補から無いものを skip」する `{{ version-files }}` パターン (= claude-cmux-msg) と、似たパターンを持つ可能性のある skill 内コマンド
- → bump-semver 本体修正後、各リポを確認して必要なものだけ `--allow-nonexistent-path` (フラグ名要検討) 追加
