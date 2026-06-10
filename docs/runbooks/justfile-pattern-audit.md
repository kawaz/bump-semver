# Runbook: justfile pattern audit

bump-semver は `docs-structure` rule で justfile の canonical owner と定められている。他リポ (kawaz/*) の justfile に古い pattern が残存しているか、逆に他リポ固有の良 pattern が canonical に未取り込みでないかを定期的にチェックする手順。

## 適用ケース

- bump-semver の justfile に新 pattern を land した直後 (= 他リポ展開のため)
- 定期 (= 月 1 等) の棚卸し

## 手順

### 1. 観察

```bash
ls -lt ~/.local/share/repos/github.com/kawaz/*{,/main}/justfile | head
```

最近 mtime 順で表示。アクティブな上位 ~10 件を読む。

### 2. 各 justfile を current canonical と diff

bump-semver の justfile (= 本リポ) と各 project の justfile を読み比べ、以下を観察:

- **旧 hint format / 旧 task 名 / 廃止コマンド** の残存
  - 例: `bump-semver vcs tag latest` (= DR-0032 で `vcs get latest-tag` に移動)
  - 例: `bump-semver vcs is jj && jj log ... || git rev-parse main` (= `vcs get commit-id --rev main` に簡略化可能)
  - 例: `@echo` の release hint が `--on-success` 形式に未移行
- **廃止された justfile recipe pattern**
- **その project だけにある独自 pattern**
  - 良ければ canonical へ吸い上げ候補
  - 例: 今日の `--on-success release.yml 'just on-success-release'` フローは bump-semver 1 リポから出発し、他リポへ展開する patten

### 3. アクション

| 発見種別 | 対応 |
|---|---|
| 古いパターン残存 | 該当 project の `docs/issue/` に起票 (= 自リポなら直接修正、他リポなら依頼) |
| 新規良パターン発見 | bump-semver canonical justfile に取り込み + 関連 DR / runbook に記録 + 他リポへ展開計画 |
| 単発の独自 pattern (= 採用しなくて良いもの) | 記録不要 |

## 解釈ガイド

- 80%: 古いパターン残存の検出が主 (= 棚卸しが本来の価値)
- 20%: project 固有の良 pattern 発見 (= 採用すれば全体向上)
- 走査 cost は低い (= 数分)、定期実施で justfile の一貫性維持

## 関連

- `docs-structure` skill — justfile の canonical 方針
- `release-flow-awareness.md` rule — release flow と justfile の関連
