# 2026-06-01 セッション引き継ぎ

前セッションで v0.30.0 (`glob:` prefix + DR-0024) まで land 済。残作業を `docs/issue/` 経由で引き継ぎ。

## 残作業 (優先度順)

### High: justfile 復活候補 + `glob:` dogfood

- 起票: [`docs/issue/justfile-revival-candidates.md`](../issue/justfile-revival-candidates.md)
- スコープ: `bump-trigger-paths` 変数化 + `glob:src/**` で `check-version-bumped` を template 化 (= `glob:` の最初の実機 dogfood)
- 同梱可: `test *ARGS` 復活 (Medium、kawaz 判断保留)
- kawaz 同意: High 優先で復活 OK (旧 justfile 比較で確認)

### Medium: auto-advance bug fix

- 起票: [`docs/issue/auto-advance-no-description-push-fail.md`](../issue/auto-advance-no-description-push-fail.md)
- 内容: `--jj-bookmark-auto-advance` が dirty + no description な @ で push 拒否ループ
- fix 案 A/B/C 提示済、A 推奨 (= description 必須 check 追加、早期 fail で hint)

### Low: 派生 sync check CLI (Phase 2)

- 起票: [`docs/issue/derived-sync-check-cli-requirements.md`](../issue/derived-sync-check-cli-requirements.md)
- 内容: 要件発散 10 章 + 叩き台引数案 + kawaz mini-DSL (`$N` 後方参照 / `--` 区切り N ペア / `glob:{...}` source 分岐 / A 自動除外)
- kawaz レビュー → DR 起票 → Phase 2 実装

## 別リポの未着手

- `~/.local/share/repos/github.com/kawaz/claude-rules-personal/main/for-me/rules/.draft-gh-issue-guard-for-kawaz-repos.md`
- kawaz/* リポへの `gh issue create` を hook で guard するアイデア、未 push、kawaz 仕様確定後に implement + push

## 運用方針 (前セッション確定)

- **シリアル化 default** (= 並列 subagent 同 workspace で禁止、必要なら `jj workspace add` で隔離)
- **Monitor は ephemeral** (= push → green 確認 → TaskStop で停止)
- **`just push` 経由** (= push-workflow rule)
- **`just bump-version` は 1 回だけ** (= over-bump 罠回避、過去 3 回ハマり)
- **codex plugin 使う** (= `/codex:review` / `/codex:adversarial-review` / `/codex:rescue`、瑣末点 filter は plugin 任せ、`--瑣末な点指摘禁止` は撤回済)
- **kawaz リモート時 AskUserQuestion 禁止** (= advisor で判断)
- **1Password 系エラー: 1 回再試行 → ダメなら say + 停止**
- **kawaz リポは `docs/issue/<file>.md` でローカル起票** (= GitHub issue 非使用)

## 直近 v0.30.0 land のハマり所

- `jj abandon` で wvponwop (= bookmark が乗ってた empty @) を破棄、main bookmark **道連れ delete** された
  - 復旧: `jj bookmark set main --allow-backwards -r <release>`
- auto-advance bug (上記 #43) で push 拒否ループ → `bump-semver vcs push --branch main` 直叩きで復旧
- gofmt 差分が working copy に残ったまま push サイクル → 副次的に上記 bug を誘発

## 着手前

```bash
jj log -r '..@' --no-pager --limit 3
# main = 60d03b0d (= "docs(issue): justfile 復活候補...") が最新のはず
# @ は empty + clean なら順次開始
```
