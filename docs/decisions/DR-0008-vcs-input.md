# DR-0008: `vcs:` 入力モード

- Status: Active (partially superseded — VCS 自動判定 section)
- Date: 2026-05-10
- Extends: DR-0006 (compare サブコマンド + FILE|VER 統合), DR-0007 (--json 出力)
- Superseded-by: DR-0016 (env var portion only — `BUMP_SEMVER_VCS` 環境変数廃止)

## Context

CI/CD で `bump-semver compare` を使う際、頻出するユースケースが 2 つある:

1. **「main ブランチからズレてないか」**: PR 上で VERSION ファイルが remote main より新しいことを確認する (`bump-semver compare gt Cargo.toml vcs:main@origin`)
2. **「最後のリリースより上げてるか」**: tag 一覧から最新 semver tag を取り、現バージョンがそれより大きいことを確認する (`bump-semver compare gt Cargo.toml 'vcs:latest-tag()'`)

これらを既存機能で実現するには `release.yml` 等から `jj file show -r main@origin Cargo.toml | bump-semver compare lt Cargo.toml -` のような shell パイプラインを書く必要があり、

- jj/git の使い分けを CI スクリプト側に書かないといけない
- tag 取得 → semver 順ソート → 最大選択を bash で書くのは面倒
- パイプ越しの content 渡しは「もう一方の入力に対応するファイル名ヒント」を tooling 側で考える必要がある

これを bump-semver 側で吸収する。`vcs:REV` / `vcs:latest-tag()` 入力を受け付けることで、justfile / release.yml 側のシェルパイプを 1 行に閉じる。

## Decision

### 構文

```
bump-semver compare gt FILE vcs:REV[:FILE]      # rev そのまま、jj/git 自動判定
bump-semver compare gt FILE 'vcs:latest-tag()'  # 関数風、tag リストを semver で集計
```

`vcs:` プレフィックスの後ろを見て:

- `<name>(<args>)` 形式 (`(` を含む) → **関数モード**
- それ以外 → **rev モード**

### rev モード

- REV を VCS に素通しで渡してファイル内容取得
  - jj: `jj file show -r <REV> <FILE>`
  - git: `git show <REV>:<FILE>`
- VCS のエラーは transparent に伝達 (ヒント追加しない)
- jj 専用: `origin/main` 形式で remote bookmark 解決失敗したら `main@origin` 形式にフォールバック (内部で 1 回だけリトライ)
- ファイル省略 (`vcs:HEAD~1` のように `:` 後ろがない) → もう一方の引数から FILE を借用
- 借用元なし → `error: vcs: file is required (no file argument to borrow from)`
- `:` の解釈: 最初の `:` を `vcs:` の終端、2 つ目の `:` で REV と FILE を分割
  - `vcs:HEAD~1:Cargo.toml` → REV=`HEAD~1`, FILE=`Cargo.toml`
  - `vcs:HEAD~1` → REV=`HEAD~1`, FILE=借用
  - jj の `main@origin` は `:` を含まないので素直に rev として通る

#### 借用ルール (確定仕様)

借用元候補は **位置順 (引数の左から FILE 提供入力)**。借用源として有効なのは:

- 実 FILE 起源の入力 (`Cargo.toml`)
- `vcs:REV:FILE` 形式 (file を明示している vcs: 入力)

verb によって借用形態が分かれる ([DR-0023](./DR-0023-n-arg-extension.md)):

| verb | 借用挙動 |
|---|---|
| `get` / bump 系 (`peerExpand=true`) | FILE を省略した `vcs:REV` は **兄弟 FILE 全 path にピア展開**。`get a b vcs:main` は `{a, b, vcs:main:a, vcs:main:b}` の 4 source を生成 |
| `compare` (`peerExpand=false`) | FILE を省略した `vcs:REV` は **常に BASE (= 第 1 引数) の path を借用**。`compare gt VERSION vcs:main vcs:v1.0.0` は両 OTHER が VERSION を借用 (= F1 基準の比較セマンティクスと一致) |

具体例:

```
get a b vcs:main                            # peer-expand: 4 source (a, b, vcs:main:a, vcs:main:b)
get Cargo.toml a.json vcs:origin/main       # 同上: 3 source × 1 borrow per file
compare gt VERSION vcs:main vcs:v1.0.0      # 両 OTHER が VERSION の path を借用 (F1 起点)
vcs:main:Cargo.toml vcs:staging             # vcs:staging は vcs:main の Cargo.toml を借用
vcs:main vcs:staging                        # error: 借用源なし
```

**「パース信頼度比較」は採用しない**。借用元判定は信頼度比較ではなく単純な位置順。理由はシンプル + 利用者から見て予測可能 (信頼度比較だと「なぜこれを選んだ?」が見えにくい)。

#### 複数 vcs: 入力

`get` / bump 系は可変長入力なので、複数 vcs: が混ざっても整合性検証 (DR-0004 の allSameValue) に流れて自然に動く。CI で「全環境一致確認」のような用途を想定:

```bash
bump-semver get Cargo.toml vcs:main vcs:staging vcs:production
```

`compare` は DR-0023 で `BASE + OTHER...` の N 引数化済 (F1 基準で全 OTHER を full-eval、失敗ペアごとに stderr 出力)。借用は F1 のみを参照するので、複数 path-less vcs OTHER も問題なく解決される。

### 関数モード (MVP は `latest-tag()` のみ)

- `latest-tag()`: 全 tag を取得 → semver パース可能なものだけ候補 → semver 順 (DR-0006 の `Version.Compare`) で最大を返す
- パース不可の tag (例: `my-special-build`) は黙って無視
- 0 件マッチ → `error: no semver-compatible tags found`
- 引数なし (将来引数版を入れる余地あり、MVP は引数を受け付けない)
- 未知関数名 → `error: unknown vcs function: foo()`

#### tag リスト取得方法

- jj: `jj log -r 'tags()' --no-graph -T 'tags.map(|t| t.name() ++ "\n").join("")'`
- git: `git tag --list`

`jj git fetch --tags` 等は呼ばない (後述「fetch しない方針」)。

### VCS 自動判定

優先順:

1. `--vcs git|jj` フラグ (1 回上書き、`auto` / 未指定は次へ)
2. `.jj` 存在 → jj
3. `.git` 存在 → git
4. どちらもなし → `error: not a git or jj repository`

`.git` が bare で `.jj` も並存している (kawaz の git-bare + jj-workspace 構成) 場合、2 で jj が選ばれる。colocate も `.jj` 優先。

> **DR-0016 で更新**: もともと存在した `BUMP_SEMVER_VCS=git|jj` 環境変数 (旧優先順 2 位) は廃止された。経緯は [DR-0016](./DR-0016-remove-bump-semver-vcs-env.md) を参照。フラグ側には新しく `auto` 値が許可されている (`--vcs auto` = `vcsAuto` 解決、default 表記用)。

### `--write` との関係

`vcs:` は読み取り専用。`bump-semver patch FILE vcs:HEAD --write` のように `--write` と混在させた場合は

```
error: --write cannot be used with vcs: inputs (vcs: is read-only)
```

で停止する。書き戻しは「全入力に書き戻し」が直感的なので、vcs が混ざると曖昧になる。明示的にエラーで止めて、利用者に書き換え粒度を分離してもらう。

### fetch しない方針

- bump-semver は `git fetch` / `jj git fetch` を呼ばない
- `vcs:origin/main` が古い場合は VCS のエラーがそのまま伝わる
- README / DESIGN で明記

## Rationale

### 不採用案

**1. `tag:` 別 prefix を追加**

`vcs:` と並列に `tag:latest` のような専用 prefix を導入する案。**不採用**: 入力モードが 2 個に増える割に得るものが少ない。`vcs:latest-tag()` の関数構文は将来の `current-branch()` 等への自然な拡張パスを既に持っている。

**2. `latest-stable-tag()` / `latest-pre-tag()` を別関数にする**

「リリース済の最新だけ欲しい」「pre-release 含む最新が欲しい」を別関数で表現する案。**不採用**: SemVer 2.0.0 の順序仕様で `1.0.0-rc.1 < 1.0.0` が定義されているので、`latest-tag()` 一本でほぼ全ユースケースをカバーできる。stable のみ欲しいなら `compare ge ... 'vcs:latest-tag()' --no-pre` のように呼び出し側で意味付けすれば良く、CLI が pre-release を意味判定するのは DR-0007 と同じ理由で避ける (CLI は構造分解だけ提供、意味判定は呼び出し側)。

**3. 引数を取る関数 (`tag-after("v1.0.0")` 等)**

「特定 tag 以降のリリース履歴」のような関数。**不採用**: shell 引用が複雑になる、MVP 範囲を超える。将来追加する余地は構文上残してある (`<name>(<args>)`)。

**4. パターン判別 / 中間表現 enum**

`vcs:` の後ろを正規表現や enum で分類して内部で「ここは jj revset」「ここは git ref」と判別する案。**不採用**: VCS に解釈を委ねたほうがシンプルで、対応形式が増えても自動的に追従する。

**5. fetch 自動実行**

`vcs:origin/main` が古いとき自動的に fetch する案。**不採用**: 副作用が大きい (利用者の認証情報を要求する、ネットワーク I/O が発生する、CI で予期しないトラフィックが出る)。利用者が明示的に `git fetch` してから呼ぶ運用に揃える。

**6. change_id を git 側で動かす翻訳**

「jj で書いた `xyzabcd...` (change_id) を git に投げるとき、対応する commit_id に変換する」案。**不採用**: jj 側は change_id 安定だが git 側は commit_id しか知らないので、CI で `git checkout <commit_id>` するのは利用者の責任。bump-semver は VCS の差を埋めない (kawaz 自身も CI 用には `commit_id` 直書きで困っていない)。

### 設計上のポイント

#### jj 優先 + 並存サポート

kawaz の git-bare + jj-workspace レイアウトでは `.git` (bare) と `.jj` が同じディレクトリに並ぶ。両方存在するときは jj が選ばれることで:

- `vcs:main@origin` のような jj revset がそのまま通る
- bare git では `git show` が失敗する操作も jj 経由で読める

colocate (`jj git init --git-repo` した直下) も同じ構成なので、自動判定が一貫する。

#### file 借用

`compare gt Cargo.toml vcs:HEAD~1` のような書き方はユーザの直感に近い (「この Cargo.toml の HEAD~1 と比較したい」)。`vcs:HEAD~1:Cargo.toml` を毎回書かせるより、もう一方の入力から借用してしまう方がスクリプトが簡潔になる。借用先がないとき (`vcs:HEAD~1 vcs:HEAD~2` のような両 vcs 入力) は明示エラーにして利用者に分離させる。

#### 関数モードの構文

`latest-tag()` の括弧は将来の引数版 (`tag-since("v1.0.0")` 等) と一貫した shell-friendly 表記を選んだ。`latest_tag` のような identifier 風だと拡張時に「これは関数だっけ revset だっけ」と判別が難しくなる。括弧は VCS 側の構文と衝突しない (jj revset / git ref で `()` を含むものは存在しない)。

## Consequences

### 互換性

純粋追加機能。`vcs:` プレフィックスを使わない既存の呼び出しは挙動不変。**v0.6.x の任意のスクリプトは v0.7.0 でそのまま動く**。

### 新規モジュール / 依存

- `src/vcs.go`: VCS 検出 + 入力解析 + 外部プロセス起動を集約
- jj/git CLI への依存 (バイナリ存在を前提)
  - 必要になった時のみ呼び出すので、`vcs:` を使わない呼び出しは jj/git 不在環境でも動く
  - jj/git 存在チェックは「`vcs:` 入力を含む invocation の最初」で行う lazy resolution

### 将来拡張

- 他関数: `current-branch()`, `head-hash()`, `bookmarks()` 等
- 引数版関数: `tag-after("v1.0.0")` 等
- `vcs:` 出力 (例: bumped 結果を VCS 内のファイルに直接書き戻す) — 現状は `vcs:` 入力のみで `--write` と排他

### CI への影響

`release.yml` / `ci.yml` 側で `bump-semver compare` を呼ぶ時、いままで `jj file show -r main@origin Cargo.toml | bump-semver compare lt Cargo.toml -` のようなパイプラインで書いていたものを、

```bash
bump-semver compare lt Cargo.toml vcs:main@origin
```

の 1 行に置き換えられる。これは bump-semver 自身の `release.yml` でも適用候補だが、現状の release.yml は VERSION 1 ファイルを単純に書き換えるだけなので、移行は実需が出てから。

## 関連実装

- `src/vcs.go` — `detectVcs` / `parseVcsOverride` / `vcsParseSpec` / `resolveVcsInput` / `vcsFetchFile` / `vcsListTags` / `vcsLatestTag` / `altJjRev`
- `src/main.go` — `--vcs` フラグ、`vcs:` プレフィックス分岐 (resolveInput)、`--write` + vcs: 排他チェック、help text 更新
- `src/compare.go` — vcsOverride を resolveInputs に伝搬
- `src/vcs_test.go` — 単体 (parse / override / altJjRev / splitAndDedup) + 実 git/jj fixture (tags / latest / fetchFile / detectVcs)
- `src/main_test.go` の `TestRun_VcsInput_*` — CLI レイヤから見た振る舞い (rev mode / file borrow / latest-tag / write rejection / 自動判定 override)
