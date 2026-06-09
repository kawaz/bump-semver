# rev 翻訳を共通基盤化 (vcs: と vcs サブコマンド両方の rev 受け口で利く)

## 課題

`vcs:REV[:FILE]` 入力モード / `vcs` サブコマンド系 (`vcs diff REV` / `vcs tag push --rev REV` / 将来追加予定の `vcs get commit-id --rev REV` 等) で rev を受けるが、backend に応じた revspec 翻訳は **`FetchFile` (jj) のフォールバックでしか実装されていない**。

- `vcs.go:226 altJjRev` が `<remote>/<bookmark>` → `<bookmark>@<remote>` (例: `origin/main` → `main@origin`) を翻訳
- 使用箇所: `vcs_backend.go:603` の `(j *jjBackend).FetchFile` のフォールバックのみ
- 他の rev 受け口 (`(j *jjBackend).Diff` / `DiffNameStatus` / `TagPush --rev` / `resolveJjRev` 等) は **素通し** → jj backend で `origin/main` を渡されると fail

逆方向 (git backend で `main@origin` を渡される) も非対応。

## 提案

**共通基盤 `translateRev(rev string, backend vcsKind) (string, error)`** を新設し、すべての rev 受け口の入口で通す。

```
入力 rev --[translateRev]--> backend-native rev --> backend cmd
```

- 既存 `altJjRev` は基盤に統合して関数自体は廃止
- 拡張対象 (= 共通解釈で書きたい syntax):
  - `<remote>/<bookmark>` ⇔ `<bookmark>@<remote>` (= 既存 altJjRev)
  - `@`, `@-`, `@--` (jj native) ⇔ `HEAD`, `HEAD^`, `HEAD^^` (git native) ※意味重なる範囲のみ
  - `HEAD~N` 系 ⇔ jj revset の到達可能な等価形 (実装可能なら)
  - tag / bookmark / branch 名 / commit_id / change_id は両 backend 共通解釈なので素通し
- backend 固有 syntax (jj の `..` revset 演算子 / git の `@{u}` 等) は **pass-through** (translate せず backend に投げる、解決失敗は backend が報告)

## 派生メリット

- 新規 verb `vcs get commit-id --rev REV` を基盤上に実装すれば、justfile を agnostic に書ける (今は `bump-semver vcs is jj && jj log -r ... || git rev-parse ...` の if-else が必要、本 issue 解決後は `bump-semver vcs get commit-id --rev main` 1 つで OK)
- `vcs:` 入力モードでも `vcs:origin/main:VERSION` を jj backend で受け取れる (現状 fail)

## 設計判断 (DR 級)

DR を立てる規模。論点:

1. **pass-through の境界**: backend 固有 syntax をどこまで通すか / どこから error にするか
2. **対称翻訳の範囲**: `@-`/`HEAD^` は意味重なるが、`@-` は jj では「user の working copy の親」、`HEAD^` は git では「現 branch tip の親」で context が違う。完全対称か、安全な範囲だけ翻訳か
3. **失敗時 exit code**: ambiguous (= jj revset が複数 commit にマッチ) → exit 4、unresolvable → exit 3 (既存規約と整合)
4. **default rev**: `--rev` 省略時のデフォルト (`@` for jj / `HEAD` for git)

## 起源

直接の発端: hyoui の `just push` の hint で push 後の main sha を取りたい場面で、`bump-semver vcs is jj && jj log -r main --no-graph -T 'commit_id' || git rev-parse main` という if-else を書く羽目になった (2026-06-09 セッション)。kawaz の指摘で「rev 翻訳は vcs: と vcs サブコマンド全般で頻出する共通課題、ad-hoc helper を散らさず共通基盤にすべき」と判明。

## 関連

- `vcs.go:217-226` 既存 `altJjRev`
- `vcs_backend.go:603` 唯一の使用箇所 (FetchFile)
- `vcs_backend.go:1573 resolveGitRev` / `1603 resolveJjRev` rev resolver 系
- 派生 issue: `vcs get commit-id --rev REV` verb 新設 (= 本 issue 解決の延長)
