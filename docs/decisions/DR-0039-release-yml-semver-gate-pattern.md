# DR-0039: release.yml semver gate canonical pattern (latest-release + latest-tag 並列)

- Status: Active
- Date: 2026-06-28

## Context

`gh release create` は GH 内に release + tag object を作るが **origin の git ref に tag を push しない** ([cli/cli#4357](https://github.com/cli/cli/issues/4357))。よって `actions/checkout` 後の `vcs get latest-tag` は **GH の最新 release より古い tag** を返すケースがある。

更に `gh release create --latest=automatic` (= default) は **date を含むスコアリング**で Latest を決めるため、**後発の小 version が Latest に昇格する**観測あり (kawaz/die 実機: v0.1.x 既存の上に後発 v0.0.2 が Latest 取った)。`--latest=true/false` 明示も「上書き条件をリリース側で判定する必要」が残るため解決にならない。

homebrew tap 更新等の後段経路は「現 push version」を信用するため、**check-version 段階で gate して release を作らせない**のが正解。release.yml の semver gate を強化する。

## Decision

release.yml の check-version step で **`latest-release` と `latest-tag` の両軸を並列 check** する。**短絡せず両方評価**、どちらかが「現 VERSION より小でない」を満たさなければ release を skip。

### canonical pattern

```yaml
FAIL=0
if LATEST_REL=$(bump-semver vcs get latest-release --repository "$REPO" 2>/dev/null); then
  if ! bump-semver compare gt "$CURRENT" "$LATEST_REL" -qq; then
    echo "::error::v${CURRENT} is not greater than latest GH release v${LATEST_REL}"
    FAIL=1
  fi
fi
if LATEST_TAG=$(bump-semver vcs get latest-tag --include-prerelease --vcs git 2>/dev/null); then
  if ! bump-semver compare gt "$CURRENT" "$LATEST_TAG" -qq; then
    echo "::error::v${CURRENT} is not greater than latest git tag v${LATEST_TAG}"
    FAIL=1
  fi
fi
[ "$FAIL" = "1" ] && exit 1
# 最終 guard: 同 version の重複 release 防止
gh release view "v${CURRENT}" --repo "$REPO" >/dev/null 2>&1 || echo "changed=true"
```

各軸が空 (= bootstrap / 初回 release / 古い binary の transient) なら **その軸の gate は skip**、他方の軸だけで判定する。両方空なら完全 bootstrap 扱い。

### 規範参照

本 DR は判断の根拠記録に留め、**実装の正本は `.github/workflows/release.yml` 本体**。他リポはそれを参照して同型 pattern を適用する。

## Alternatives Considered

- **`vcs get latest-tag` 単独 gate**: `gh release create` の git tag 非 push 仕様で gap がある (= 既存 v0.42.0 GH release に対し latest-tag が v0.40.1 を返す観測あり)。VERSION = 0.41.5 を push したら gate を抜けて release できてしまう
- **`gh release view "v${CURRENT}"` の重複確認だけ**: 「同じ tag が無い」しか見ないので downgrade が通る。kawaz/die の事故元
- **`gh release create --latest=true/false` 明示**: 上書き条件を caller 側で判定する必要が残るため複雑化、しかも release 自体は作成済になる
- **DR の長文文書化中心**: 規範参照は実装本体 (= release.yml) に集約する方が他リポの追従コストが低い。本 DR は判断 history のみ

## Consequences

### Pros

- `latest-release` の GH 経路で git ref の遅延 gap を埋める
- 両軸並列 check で「片側 source が信頼できない」ケースに耐性
- 後発の小 version が Latest に昇格する GH の罠 (= `--latest=automatic` の date-priority) を release.yml で完全 gate
- canonical pattern 化により他リポ (kawaz/die 等) が同型実装で追従できる

### Cons / Trade-offs

- `vcs get latest-release` は `gh` API 呼び出しが必要 (= `GH_TOKEN` 依存、ローカル実機では `--repository` 明示)
- 初回 release / 移行期は両軸とも bootstrap 扱い、最終の `gh release view` 重複確認だけが防衛になる (= 二重 release は防げるが downgrade は防げない、ただし初回なので問題ない)

## 関連

- [DR-0032](./DR-0032-vcs-get-latest-by-source-verb.md) — `vcs get latest-tag` / `latest-release` の source 軸 verb 設計
- [DR-0020](./DR-0020-vcs-subcommands.md) — vcs サブコマンド全体方針
- kawaz/die dogfood 報告 (2026-06-28、session 911732b3): canonical の hole を実証 + fix を投げた経緯
- [cli/cli#4357](https://github.com/cli/cli/issues/4357) — `gh release create` が git tag を origin に push しない仕様
