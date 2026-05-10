# DR-0009: lock files の対応対象判断 (npm 以外は対象外)

- Status: Active
- Date: 2026-05-11
- Closes: docs/issue/2026-05-09-bun-lock-support.md, cargo-lock-support.md, pnpm-lock-yaml-support.md, yarn-lock-support.md

## Context

各エコシステムの lock file (`package-lock.json` / `bun.lock` / `pnpm-lock.yaml` / `yarn.lock` / `Cargo.lock`) が **自プロジェクト (root package) の version を保持しているか** を調査し、bump-semver の対応対象を確定する。

調査の動機: 「bump-semver patch package.json bun.lock --write」のような複数 file 一括 bump が必要かを判定するため。lock file が version を持たないなら lock file は bump 対象外。

## Decision

### 対応する (既存)

- **npm `package-lock.json`** (DR-0004 で対応済): `$.version` + `$.packages[""].version` の 2 箇所に root version を保持。`bump-semver patch package.json package-lock.json --write` で同期 bump 必須。

### 対応しない (本 DR で確定)

#### bun `bun.lock`

- root workspace (`workspaces[""]`) には実運用で `version` フィールドが**欠落** (TypeScript 型定義では required だが現実は optional 扱い)
- bun 自身に open issue がある (#18906 workspace versions go stale、#20477 `bun pm pack` が `bun.lock` の version を使う問題)
- bun 公式推奨も `package.json` の bump 後に `bun install --lockfile-only` で再生成
- **加えて JSONC パーサ (コメント + trailing comma) の依存追加が必要**で、対応コストに見合わない

#### pnpm `pnpm-lock.yaml`

- `importers["."]` (root) は `dependencies` / `devDependencies` のみで `version` フィールドなし
- workspace member (`importers["packages/*"]`) も同様、self-version は持たない (path key で識別)
- pnpm v9 lockfile schema を全て確認、root project version は格納されない設計

#### yarn `yarn.lock`

- **classic v1**: lockfile entry は `<name>@<range>` で keyed、root package の self-entry は存在しない
- **Berry v2+**: workspace entry は存在するが、`version` は固定 sentinel `0.0.0-use.local` (実 version は package.json から実行時解決)
- どちらの世代も bump 対象として意味を持たない

#### Cargo `Cargo.lock`

- TOML の `[[package]]` 配列に **自パッケージも含めて全 version を記録** するので技術的には bump 可能
- ただし `cargo check` 等を呼べば自動更新されるため、bump-semver で書き換える価値が薄い (kawaz の justfile も `bump-semver patch Cargo.toml --write && cargo check` パターン)
- 整合性検証用途 (`bump-semver get Cargo.toml Cargo.lock` で「Cargo.toml と Cargo.lock の version が一致しているか」確認) には使えるが、別 issue として別途検討

## Rationale

### npm 系だけが lock file に self-version を持つ特殊ケース

npm の `package-lock.json` は歴史的経緯 (v1 の top-level `version` + v2/v3 の `packages[""]`) で root version を保持する設計。他のエコシステム (bun/pnpm/yarn) は **「version は package.json で管理、lock file は依存解決のみ」** という設計を採用しており、root project の version を lock file に冗長保持しない。

この差異の背景:
- npm は v5 (2017) 時点で lockfile design が固まり、その後 v7 で破壊変更を入れた経緯
- pnpm / yarn / bun はその後発で「package.json が単一の真実」を選択

### 実装コストとメンテコスト

bun.lock 対応に必要なもの:
- JSONC パーサ依存 (`tailscale.com/util/jsonc` 等) → 標準 `encoding/json` 範疇外
- root version が「実運用で欠落」しがちな仕様 → 抽出失敗時の振る舞い設計
- bun 1.2 / 1.3 / 将来の format 変更追従

pnpm-lock.yaml 対応に必要なもの:
- yaml.v3 依存追加
- そもそも対応する value が無い (root version field 自体が schema にない)

これらは「対応するコード書いても結局 root version が取れないので no-op」というオチになり、ユーザに混乱を招く。

### 不採用案: 「lock file を対象から外す」エラーで誘導

`bump-semver patch yarn.lock --write` のように誤って lock file を渡された時に明示的なエラーで誘導する案。検討したが現状の `unsupported file: yarn.lock` で十分機能しており、追加実装は YAGNI。

## Consequences

### issue ファイルの削除

以下 4 件を削除 (knowledge-flow ルール: 解決後は delete、knowledge は本 DR に残す):

- `docs/issue/2026-05-09-bun-lock-support.md`
- `docs/issue/2026-05-09-cargo-lock-support.md`
- `docs/issue/2026-05-09-pnpm-lock-yaml-support.md`
- `docs/issue/2026-05-09-yarn-lock-support.md`

### ROADMAP の更新

`docs/ROADMAP.md` の「候補ハンドラ」テーブルから上記 lock files を削除。代わりに「対応対象外 (DR-0009 参照)」セクションを 1 行追加。

### 将来の選択肢

#### Cargo.lock 整合性検証

整合性検証用途 (`get Cargo.toml Cargo.lock` で同期確認) は実装できる。需要が出たら別 DR で再評価。優先度低。

#### bun.lock の再評価

bun が `bun.lock` の root version を必須化する仕様変更を行ったら再評価。現時点では非対応で正解。

#### `unsupported file:` エラー文言の改善

`yarn.lock` 等を渡された時のエラー文言に「root version is not stored in lock files for this ecosystem」のヒントを追加することは検討価値あり (別 issue / 別 DR)。

## 参考

- [npm package-lock.json schema](https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json)
- [Bun Lockfile docs](https://bun.sh/docs/install/lockfile)
- [pnpm/spec lockfile/6.0.md](https://github.com/pnpm/spec/blob/master/lockfile/6.0.md)
- [Yarn classic yarn.lock docs](https://classic.yarnpkg.com/lang/en/docs/yarn-lock/)
- 調査経緯: docs/journal/2026-05-11-justfile-template-refactoring.md (本 DR は同じ non-stop セッション内)
- DR-0004: 複数 FILE 一括 bump + name 整合性検証 (npm package-lock.json の対応根拠)
