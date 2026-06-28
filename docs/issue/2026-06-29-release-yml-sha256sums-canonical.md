---
title: release.yml canonical pattern に SHA256SUMS 添付を組み込む提案
status: open
category: design
created: 2026-06-29T01:08:05+09:00
last_read:
open_entered: 2026-06-29T01:08:05+09:00
wip_entered:
blocked_entered:
pending_entered:
discarded_entered:
resolved_entered:
discard_reason:
pending_reason:
close_reason:
blocked_by:
origin: kawaz/die
---

# release.yml canonical pattern に SHA256SUMS 添付を組み込む提案

## 概要

bump-semver の release.yml (= kawaz/* binary 配布リポの canonical テンプレ) に **SHA256SUMS 添付** ステップを組み込んでほしい。

## 背景

バイナリ配布系の release で hash manifest 不在は監査 / 整合性検証の観点から好ましくない。利用者は CI が build した binary が手元のものと一致してるかを確認する手段がない (= MITM / 改ざん / ダウンロード破損が検出できない)。

kawaz/die session で本日 (2026-06-29) 同件を指摘され、v0.3.4 で die 側に対応を実装、release artifact に SHA256SUMS が出るようにした (release: https://github.com/kawaz/die/releases/tag/v0.3.4)。

## die 側で採用した実装

release.yml の release job 内、download-artifact → gh release create の **間** に 1 step 追加:

```yaml
- uses: actions/download-artifact@v4
  with:
    merge-multiple: true
- name: Generate SHA256SUMS
  run: |
    set -euo pipefail
    # Deterministic ordering (sort) so the manifest is reproducible.
    # Format = GNU coreutils `sha256sum` output (= "<hash>  <filename>"),
    # verifiable on any host with `sha256sum -c SHA256SUMS`.
    sha256sum die-* | sort -k2 > SHA256SUMS
    cat SHA256SUMS
- name: Create release with tag
  ...
  run: |
    ...
    gh release create "v${VERSION}" \
      --repo "$REPO" \
      ...
      die-* SHA256SUMS
```

設計上のポイント:

- **sort -k2 で deterministic**: filename でソート、再 run しても byte-identical
- **GNU coreutils 形式**: 利用者は `sha256sum -c SHA256SUMS --ignore-missing` で検証可能、追加 tool 不要
- **gh release create の引数末尾に SHA256SUMS を追加**: 既存の `die-*` glob と並べるだけで release に出る

## 受け入れ条件

- [ ] bump-semver 自身の release.yml に SHA256SUMS 生成 + 添付 step が追加されている
- [ ] release.yml 内コメント or docs/decisions/ で「SHA256SUMS 必須」を canonical pattern として明示している
- [ ] bump-semver v? の release に SHA256SUMS が実際に添付されている

## bump-semver canonical への展開

bump-semver の release.yml は **kawaz/* バイナリ配布リポの canonical テンプレ**として位置付けられてる。die / authsock-warden / cache-warden / hyoui / jj-worktree / stable-which 等が同 pattern を踏襲してる。

ここに SHA256SUMS 添付を組み込めば、他リポも自然に追従する形になる:

1. **bump-semver 自身の release.yml に同 step を追加** (= self-host)
2. release.yml 内コメント / docs/decisions/ で「SHA256SUMS 必須」を canonical pattern として明示
3. 他リポ (die / authsock-warden / cache-warden / ...) が次の更新時に追従できる形に

## バイナリ名の glob 部分

`die-*` の部分は repo ごとに binary 名 prefix が違う (例: bump-semver は `bump-semver-*`)。canonical テンプレ化する時は変数化 (例: env: `BINARY_PREFIX: bump-semver`) するか、各リポが自分で書き換える前提でテンプレ提示するか、判断は bump-semver session 側にお任せ。

## 推奨優先度

中。**バイナリ配布の業界標準** (= GitHub Actions の release で SHA256SUMS 同梱は GoReleaser / cargo-dist / 多くの OSS が採用) なので、kawaz/* リポも同じ形にすると安心感が上がる。本日 die session が先行実装したが、canonical 経由で他リポにも波及させたい。

## reference

- die v0.3.4 release (= 実装後): https://github.com/kawaz/die/releases/tag/v0.3.4
- die release.yml の該当 commit: da521759 (`feat(release): attach SHA256SUMS manifest to every release`)
- kawaz/die session 911732b3-2e6b-4733-b035-5974e5f3f67f の本日後半 (2026-06-29 00 時頃 +)
