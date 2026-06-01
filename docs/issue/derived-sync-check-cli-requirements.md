# 派生ファイル鮮度同期 check CLI: 要件発散と設計議論材料

> Status: 要件発散段階 (kawaz による方針確定前)
> 目的: kawaz が「これはダメ」「意外といける」を判断できる選択肢を網羅する
> 注意: 本ドキュメントは **正解を提示しない**。複数案の trade-off を並列展示し、kawaz の判断材料を提供する

## 背景

本ツールの本質は **「source (派生元) が更新されたら、それと一緒に更新されるべき derived (派生先) が古いままじゃないか」を確認する汎用 check**。タスクランナーが「特定パターンの target list の更新を cache 差分で検出して再実行する」文脈に近い。

代表的な同構造ケース:

| ケース | source | derived (set) |
|---|---|---|
| bundle / ビルド成果物 | `src/**/*.ts` | `lib/**/*.{js,mjs,d.ts,js.map}` |
| 生成コード | `*.proto` | `generated/*.{go,py,ts}` |
| schema migration | `schema.sql` 追加 | `migrations/*.sql` 対応分の存在 |
| lock file | `package.json` | `package-lock.json`, `pnpm-lock.yaml` |
| DB schema doc | `schema.sql` | `docs/schema.md` |
| 翻訳ペア | `README.md` | `README-ja.md`, `README-fr.md`, ... |

「source 更新 → derived も同コミット以降でなきゃおかしい」が共通骨格。翻訳ペアは **1 ケース** にすぎず、本 CLI は汎用 derived sync check として設計する。

### 着想元 (follow-up #32 起点)

kawaz の元アイデア (session e4f74abe, 5c0735cc 2026-05-30):

> ファイル A に対して、A'[s] の導出を正規表現マッチングとかで紐付けルールで導出、A は A' に対して同じコミットまたは古いコミットでなければいけない。A が A' 全てに対して Newer コミットに位置していてはいけない。A リスト (複数 or Glob) を渡したら、それぞれの A に対して A' に対応するファイルを探して、各ファイルの最新が組まれてるコミットを A と全て比べる。bump-semver でやるか別でやるかは要検討。

kawaz の追補 (session 6a0a262f, ee1ebc05 2026-05-30) — 翻訳ペアを 1 例として挙げた:

> 実際は僕ルールでは `**/*[._-]ja.md` 辺り (実際は曖昧にする必要ないのでハイフンだけで良いが) を A として渡すと、マッチしたあるファイル a があり、a から `-ja` を除去したファイル、また `-他lng.md` を探す**または無ければ新規パス群**を a'∈A' とし〜。

kawaz のスコープ確定 (session 6a0a262f, 0c769a9b 2026-05-31):

> 32 はや具体的な引数設計とかはやらなくてよい。やりたいことを引数で表現する形に引数設計を落とし込むの難しいので。マッピング方法やファイルの A' の確定要件はどんなものがあるか？存在するものの集合なのか、なければ作るまで含むのか、など。あり得るかもしれない色々な要件の軸を発想の幅を広げて〜列挙する書き留めておいて下さい。叩き台としての引数設計案も作ってくれるのは構わない。

kawaz の **本質汎化** 訂正 (2026-06-01) — 翻訳特化思考を解除:

> 元々使ってた例が翻訳チェックでしたが、本質は、単にこれが更新されたら、一緒に更新されるべきこっちが古いままじゃないか？を確認すべきだよねっていうチェック用のツール流として考えています。タスクランナーとかで、特定パターンのターゲットリストの更新チェックをしてキャッシュとの変化があれば実行とかそういう文脈に近いと思ってます。`src/**/*.ts` に変化があるのに、そのコミットと同じか以降のコミットに bundle 版である `lib/` 内の js とか mjs とか sourcemap ファイルとかが存在してなきゃおかしいよね? みたいな話と同じです。なので lng サフィックスは ISO を採用するか? みたいなことを考え始めてるのはずれてます。

→ 本ドキュメントは **要件軸の発散** が主、引数設計は叩き台のみ。

## 0. 現行実装との対照 (= 出発点の明示)

現行 `kawaz/pkf-tasks/tasks/docs/translations.pkl` (v2.1.0) は **翻訳ペア特化** の実装で、本 CLI 化はこの 1 ケース実装を **汎用 derived sync check** に昇格させる位置づけ。

| 軸 | 現行挙動 (翻訳特化) | 汎化視点 |
|---|---|---|
| 入力 | CLI 引数 (glob 展開済 path 列) / Pkl の `forPairs(...)` 固定リスト / 引数なしで cwd 配下 `*-ja.md` を `find` で auto-discover (`.jj` `.git` `.out` `node_modules` 除外) | source 集合の指定方法は glob / manifest / explicit list / auto-discover の選択軸として汎化可 |
| マッピング | `*-ja.md` 正本 → `${prefix}.md` の **1 対 1** (誤巻き込み回避: `data-layout-ja.md` → `data-layout-history.md` を拾わない) | source → derived の関係表現 (suffix-strip / regex / explicit / metadata / manifest) として汎化、誤マッチ問題は本質的に汎用論点 |
| 鮮度比較 | VCS の **committer timestamp** (`jj log -T 'committer.timestamp().format("%s")'` または `git log -1 --format=%ct`) で `tgt_ts < src_ts` を lag 判定 | 派生がどこでも (bundle / generated / 翻訳) 同じ比較機構で扱える、汎用機構として最大の流用範囲 |
| 副次 check | links: ja ↔ en 相互リンクの存在 (`> [English](./X.md) | 日本語` / `> English | [日本語](./X-ja.md)`) を head 5 行から検出 | **翻訳特化機能**。汎化 CLI では各ケース固有 (= links / sourcemap / format-check) の plugin 的扱い |
| 未追跡 file 扱い | timestamp 取得失敗時は 0 に正規化 (exit 2 にならないように) | 汎用論点 (= source / derived どちらが未追跡か、運用文脈で挙動分岐) |

## 1. マッピング方法の要件軸

「source (= A) から derived (= A') をどう導出するか」のバリエーション。各軸の例を **bundle / generated / 翻訳** で複数列挙する。

### 1.1 suffix-strip / suffix-attach + 兄弟探索 (glob ベース、現行実装系の汎化)

- 入力 glob で source 集合を確定し、各 source から suffix の strip / attach で derived path を導出
- 例:
  - bundle: `src/foo.ts` → strip `src/` + attach `lib/` + 拡張子変換 `.ts→.{js,mjs,d.ts,js.map}` で 4 derived
  - test ペア: `src/foo.go` → suffix attach `_test.go` で `src/foo_test.go` を derived (もしくは逆方向)
  - 翻訳: `README.md` → suffix attach `-ja` で `README-ja.md`
- derived が **実在すれば対象**、不在は無視 (= 現行 1 対 1 挙動)
- kawaz ee1ebc05 案 (翻訳ケース): 同 prefix の `${prefix}-<別 suffix>.md` も derived に含める
- 長所: シンプル、命名規約に乗れば設定 0、kawaz 元アイデアに最も近い
- 短所: **誤マッチ問題**: 翻訳例で `data-layout-ja.md` → prefix=`data-layout`、`data-layout-history.md` を巻き込む。汎用ケースでも `src/foo.ts` strip → `src/foo-old.ts` を巻き込む等。マッチ範囲を狭める追加ルール (= 完全 suffix 一致 / ホワイトリスト) が必要

### 1.2 任意正規表現置換ベース

- source → derived のマッピングを `s/^src\/(.+)\.ts$/lib\/$1.js/` のような任意 regex で表現
- ユーザが置換ルールを config に書く (`sync-rules.yml` 等)
- 例:
  - bundle: `s|^src/(.+)\.ts$|lib/$1.js|`, `s|^src/(.+)\.ts$|lib/$1.d.ts|`
  - generated: `s|^proto/(.+)\.proto$|generated/go/$1.pb.go|`
  - 翻訳: `s|-ja\.md$|.md|`, `s|-ja\.md$|-en.md|`
- 長所: 命名柔軟、ディレクトリ階層別命名 (= `i18n/ja/foo.md` ↔ `i18n/en/foo.md`、`proto/` ↔ `generated/`) に対応可
- 短所: ユーザ設定要、学習コスト、誤設定でゼロマッチが silent

### 1.3 explicit pair リスト

- ペアを明示列挙: `sync-map.yml` で個別に書く
- 例:
  ```yaml
  - source: package.json
    derived: [package-lock.json, pnpm-lock.yaml]
  - source: README.md
    derived: [README-ja.md, README-fr.md]
  - source: schema.sql
    derived: [docs/schema.md]
  ```
- 長所: 規約レス、最も明示的、誤マッチ皆無
- 短所: ファイル数 N に応じて列挙コスト線形増、規約化のメリットを捨てる

### 1.4 source への参照 metadata ベース

- derived ファイル側に「source への参照」を frontmatter / コメント / 別 file で宣言
- 例:
  - 翻訳: `<!-- translation-of: README.md -->` を `README-ja.md` の頭に
  - 生成コード: `// Generated from: proto/foo.proto` を `generated/foo.pb.go` の頭に
  - bundle: `// @source src/foo.ts` を `lib/foo.js` の頭に (TS の build 系は既に sourcemap 経由で参照を持っている → 1.2 と組合せ可)
- 長所: 命名規約フリー、ディレクトリ階層と独立、移動に強い
- 短所: 既存ファイル全部に metadata 追加要、漏れが silent

### 1.5 manifest ベース (= 単一ファイルで全体管理)

- 1 ファイル (`sync-manifest.json` or `.sync-pairs.yml`) で全ペアを宣言
- bundle ツール (= esbuild, vite, rollup) の出力 manifest を流用する選択肢もある
- 長所: 1 箇所で見通せる、CI で manifest と実態の乖離 check 可
- 短所: ファイル増減のたびに manifest 更新要 (= 二重管理コスト)、ただし auto-discover + manifest diff の組合せで実用化可

### 1.6 双方向 / 多方向マッピング (= どちらが source かを動的判定)

- source ⇔ derived の方向を動的に決める or 双方向に揃える
- 例:
  - 翻訳: lang 検出で「より新しい方を正本扱い」
  - DB schema: `schema.sql` ↔ `docs/schema.md` を双方向、どちらか新しい方が source
  - lock file 整合: `package.json` と `package-lock.json` のどちらが先に更新されたかで判定
- 長所: 「source が常に固定」前提を持たないケースに対応
- 短所: 責務肥大、kawaz の元アイデア (= 「A → A'」片方向) と方向性が違うので採用は本案外想定

### 1.7 ハイブリッド (= デフォルト規約 + override)

- デフォルトは 1.1 (suffix-strip)、override で 1.2 / 1.3 / 1.4 を局所適用
- 長所: 規約適用範囲が広いリポは 0 設定、例外は明示
- 短所: 実装複雑

## 2. A' (派生集合) の確定要件

「派生集合をどう確定し、不在ファイルをどう扱うか」の要件軸。kawaz が明示的に「存在するものの集合なのか、なければ作るまで含むのか」と提起した中核論点。

### 2.1 「派生は実在ファイルのみ」(現行実装の挙動)

- マッピングで導出した derived 候補のうち、実ファイルが存在するもののみ check
- 不在は「まだ作ってない」と解釈、silent pass
- 例:
  - bundle: `lib/foo.js` が無ければ「まだビルドしてない」として silent pass
  - 翻訳: `README-fr.md` が無ければ「翻訳まだ始めてない」として silent pass
- 長所: noise なし、追加コスト 0
- 短所: 「source に追加した内容に対応する derived を作り忘れた」を検知不能
- 短所: kawaz ee1ebc05 の「なければ新規パス群」と整合しない

### 2.2 「派生不在を warn (= 報告 only)」

- 不在 derived を stderr / log に出力するが exit code は変えない
- 長所: 見落とし防止、CI を blocking しない、移行に優しい
- 短所: warn が noise 化して読み飛ばされるリスク
- 細分化: warn level を「未作成 OK」「未作成 fail」で flag 切替可能に

### 2.3 「派生不在を fail (= strict)」

- 想定する derived 全種別が存在しなければ fail
- 必須種別を引数 / config で指定 (`--require js,mjs,d.ts` / `--require ja,en`)
- 例:
  - bundle: `lib/foo.js` `lib/foo.mjs` `lib/foo.d.ts` のいずれか欠ければ fail
  - 翻訳: 指定 lang の derived 不在で fail
- 長所: derived 網羅性を CI で保証
- 短所: 作業途中段階で常に fail、運用厳しい

### 2.4 「派生不在を auto-create (= stub 生成)」

- 派生不在時は stub ファイル (= 空 or boilerplate or 「TODO」) を auto-create
- 後で中身を埋める
- 例:
  - 翻訳: 「TODO: translate」テンプレ生成
  - 生成コード: 本来は生成器を呼ぶべきなので stub は不適 (auto-create は基本不向き)
  - schema migration: `migrations/NNNN-<diff>.sql` の空 stub 生成
- 長所: 作業開始の負担軽減、kawaz ee1ebc05 の「なければ新規パス群」を文字通り解釈するならこの案
- 短所: ツール責務肥大 (= check が write を伴う = 副作用大)、`--check` / `--fix` 二態化 (lint ツールの慣例) が要る
- 細分化: stub 生成は別 verb (`sync init` / `sync scaffold`) に分離する選択肢もある

### 2.5 「派生不在は仮想ペアとして check 対象 (= 鮮度比較で fail)」

- 派生不在 = 「派生の commit 鮮度 = 0 (= 一度も commit されていない)」とみなし、source との鮮度比較で自動 fail
- 長所: stub 生成 (= 副作用) を伴わず strict 検査と同等の効果
- 短所: 「派生は実在 file 必須」前提を破る、エラーメッセージで「ファイルが存在しない」を明示する工夫要

### 2.6 「派生候補集合の動的拡張」

- derived の種別 / 種別群を CLI 引数で明示 (`--also js --also mjs --also d.ts` / `--also -en --also -fr`)、または auto-discover (= 既存 derived の種別を repo 内 scan で発見)
- 静的 (引数) vs 動的 (auto-discover) で挙動分岐
- 長所: 多種別対応、新種別追加時の追従性
- 短所: 種別の境界定義 (= 何を「同じ派生群」として扱うか) の正典化、誤検出抑制要

### 2.7 「派生集合の境界に file 種別を含める」

- 同じ source から派生する別フォーマットも含めるか
- 例:
  - bundle: `.js` だけでなく `.mjs` `.d.ts` `.js.map` も
  - docs: `README.md` だけでなく `README.txt` (legacy), `README.rst` (Python 系) も
- 長所: フォーマット混在リポに対応
- 短所: フォーマット軸を更に持ち込む、scope 拡散

## 3. 鮮度比較の要件軸

「A ≤ A' を何で判定するか」の軸。kawaz の元発言は「同じコミットまたは古いコミット」だが、具体機構は複数候補ある。**ここはケース不問で同じ機構が流用できる** (= bundle / generated / 翻訳いずれでも同じ比較で済む)。

### 3.1 committer timestamp 比較 (= 現行実装の挙動)

- `jj log -T 'committer.timestamp()'` / `git log -1 --format=%ct` で各 file の最新 commit timestamp を取得、`tgt_ts < src_ts` で lag
- 長所: 実装が枯れている (現行実装そのもの)、VCS 横断で安定
- 短所: rebase / cherry-pick で timestamp が書き換わる、committer date は author date と乖離する場合あり (= `commit --amend` で committer date のみ更新)
- 未追跡 file は 0 に正規化 (現行実装)

### 3.2 commit-id / rev 同一性比較

- 各 file の最新 commit-id を取得、「derived の最新 commit-id == source の最新 commit-id」または「derived の commit が source commit の子孫」を OK
- 長所: timestamp の罠 (rebase) を回避、kawaz の「同じコミット」を文字通り解釈
- 短所: 「子孫判定」が VCS API として高コスト (= ancestry graph traversal)、特に jj/git 両対応で実装複雑

### 3.3 トポロジカル順序 (= ancestry graph)

- source commit が derived commit の祖先かどうかを `git merge-base --is-ancestor` 系で判定
- 長所: rebase に頑健、「source 変更が derived に反映されているか」を厳密に答える
- 短所: 単一 commit で同時に両 file を変更したケース (= 同じ commit でも順序は付かず祖先関係なし) を OK 扱いにするか fail にするかの解釈分岐
- 短所: 実装高コスト、特に file 単位の最新 commit を抽出する `git log -1 -- <file>` の出力との合成が必要

### 3.4 内容 hash + 派生 metadata

- source の内容 hash を derived の frontmatter / コメントに記録 (`<!-- source-hash: abc123 -->` / `// @source-hash: abc123`)、source 変更で hash 不一致になれば fail
- 長所: 順序非依存、明確、VCS の挙動と独立
- 短所: 各 derived に metadata 必須、運用コスト、初回導入のマイグレーション要

### 3.5 mtime ベース

- file mtime で比較
- 長所: VCS 不要、最速 (= make の鮮度判定と同じ)
- 短所: checkout / clone 直後 mtime はファイル順序を反映しない、`touch` で揺れる、CI で不安定 → **VCS ベースのリポでは実用不可寄り**、ただし「ローカル開発で make 的に使う」ユースケースだけなら有効

### 3.6 「N 日許容」(= 鮮度の hysteresis)

- derived が source より新しいか同じである必要なく、「source 変更後 N 日以内なら derived が遅れていても OK」
- 長所: 人間ペースの作業 (= 翻訳遅延) を許容
- 短所: 設定要、release gate には不向き (release 直前で「あと N 日待って」になる)

### 3.7 比較粒度の選択

- 鮮度比較を「file 単位」「行単位 (= セクション / chunk 単位)」「文単位」で行うか
- 長所: 細かい粒度ほど false positive 減 (file の一部だけ変わっても全 derived が lag 判定されない)
- 短所: 行/文単位は実装高コスト、現状 file 単位で十分

### 3.8 (補足) untracked / 未 fetch の扱い

- 現行は timestamp=0 に正規化、結果として「source untracked / derived untracked」がペアで揃ってる時のみ pass
- alternative: 「untracked file が含まれる場合は warn 出して skip」「pre-commit hook での運用なら untracked = 新規追加なので OK 扱い」など、運用文脈で挙動を変える選択肢

## 4. 副次 check の取り扱い

現行実装は commit-lag 以外に **翻訳特化の bilingual links 整合性 check** を別 task として持つ (= ja ↔ en の `> [English](./X.md) | 日本語` 規約)。汎用 CLI 化に際しては、**副次 check は各ケース固有** であり本体機能に含めるか否かが論点になる。

### 4.1 CLI に副次 check を含めるか

- A. 含める (= 現行 `check-translations` umbrella と同等、ケース別 check を plugin 的に並列実行)
- B. 含めない (= commit-lag only に絞る、副次 check は別ツール / 別 verb)
- C. plugin / flag で opt-in (`--check links`, `--check sourcemap`)

### 4.2 ケース別の副次 check 例 (= 汎用 CLI で plugin 化するなら)

- 翻訳: ja ↔ en 相互リンクの整合性 (現行実装の links check)
- bundle: sourcemap の `sources` が実在の src path を指しているか、`.d.ts` の export 一致
- 生成コード: フォーマット (= `gofmt`, `prettier`) check、生成器の version pin
- lock file: `package.json` の dependency 解決が lock file と矛盾しないか

副次 check はケース固有度が高く、本体 (= 鮮度比較) と分離する設計の方が見通しが良い。本体に詰め込むと「翻訳特化機能」が再び忍び込む。

## 5. ユースケース列挙 (使う側の視点)

要件発想の発散用。網羅性が目的、各々に賛否を付けない。

- **release 前 gate**: derived 追従漏れで fail (= bundle 未更新 / 翻訳遅延の双方に同じ仕組み)
- **pre-commit hook**: local で commit 前 check、未追跡 file は許容
- **CI on PR**: PR 単位で派生漏れ検出、main マージ前に blocking
- **スキャフォールド**: source 新規追加 → derived stub 自動生成 (2.4 を採用するなら)
  - 翻訳: 「TODO: translate」stub
  - schema migration: 空 migration file stub
- **鮮度確認 only (report mode)**: exit 0 維持、現状の遅れ状況を一覧表示するだけ
- **鮮度マトリクス**: 同 source に対する複数 derived 種別の遅れ commit 数を表で出す
  - bundle: `foo.js` `foo.mjs` `foo.d.ts` の追従状況
  - 翻訳: `README-ja.md` `README-fr.md` の追従状況
- **タスクランナーの affected 検知**: make / npm scripts / pkfire 系で「source 変更 → derived 再生成」のトリガー判定として利用 (= 鮮度 lag があるものだけ rebuild 対象)
- **source 変更時に責任者 assign**: GitHub Actions 連携で「derived が遅れています、@maintainer assign」自動 PR / comment
- **新 derived 種別の追加検出**: 既存セットに新種別 (= 新 lang / 新 build target) を加えた瞬間に「全 source に対する新 derived 不在」を一覧化
- **bump-semver dogfooding**: 本リポの `README.md` ↔ `README-ja.md` (+ `DESIGN.md` ↔ `DESIGN-ja.md`) に翻訳ケースとして適用、将来 src ↔ generated ケース等が出れば追加

## 6. 実装場所の検討軸

### 6.1 bump-semver 内 verb (`bump-semver sync check ...` 等)

- pros: bump-semver の vcs サブシステム (DR-0020) を共通基盤として再利用、リポへの「source→derived の鮮度 check」として release gate に親和性高い
- pros: 単一バイナリ配布で済む、ユーザは追加 install 不要
- cons: bump-semver の責務 (semver bump / vcs 操作) と sync check 責務 (file 間の派生関係) が軸違い、scope 拡散懸念
- cons: バイナリサイズ増、bump-semver を semver 用途だけで使うユーザにも追加コード

### 6.2 別単機能 CLI (例: `sync-check`, `derived-watcher`, `freshness`)

- pros: bump-semver の責務肥大化を避ける、独立 release cycle / 独立 versioning
- pros: 単機能の発展余地 (e.g., 副次 check / stub 生成 / 鮮度マトリクス) が制約なく持てる
- cons: 共通基盤 (VCS 操作) を別途実装 or bump-semver vcs に library 依存
- cons: ユーザに追加 install を強いる、homebrew tap も別に必要

### 6.3 bump-semver vcs verb の拡張 (`bump-semver vcs check-derived ...`)

- pros: vcs ファミリ (DR-0020) との整合、`vcs:latest-tag()` 入力モードの拡張として位置づけ可
- cons: vcs verb の責務 (VCS 操作の抽象化) と sync check 責務 (file 間の派生関係) の混在、vcs verb の概念純度が落ちる

### 6.4 bump-semver library + thin CLI

- pros: bump-semver を Go library として release、別 CLI からも import 可
- cons: bump-semver の Go internal 公開 API 化が必要 (現状 main package 直 link 想定)、release 互換管理コスト

### 6.5 「pkfire を捨てない」案 (= 現状維持 + 機能拡張)

- pros: 実装ゼロ、現行 pkf-tasks の translations.pkl を拡張するだけ
- cons: DR-0022 の「pkf 依存を翻訳 check だけに絞る」を経た上で更にゼロ化したい follow-up なので、本選択肢は本タスクの趣旨と逆走
- cons: そもそも pkf-tasks の現行実装は翻訳特化で、汎用 derived sync check への拡張は再設計に近い

## 7. 叩き台引数設計案 (kawaz 判断用、複数案併記)

**注**: kawaz は「具体的引数設計はやらなくてよい」「叩き台レベルなら歓迎」と確定済。以下は議論呼び水の叩き台で、最適解の提示ではない。verb 名 (`sync` / `derived` / `outdated` 等) も含めて kawaz 判断対象。

### 案 A: glob + suffix 操作 (kawaz 元アイデア準拠)

```
# bundle
bump-semver sync check --glob 'src/**/*.ts' --derive 'lib/$1.js'
# 翻訳
bump-semver sync check --glob '**/*.md' --not-glob '**/*-*.md' --derive '$1-ja.md'
```

- glob で source 集合、`--derive` で派生 path 導出
- pros: シンプル
- cons: 1 source = 1 derived 前提 (複数 derived は `--derive` 連発)、glob 除外がやや煩雑

### 案 B: glob + 複数 derive 種別

```
# bundle
bump-semver sync check --glob 'src/**/*.ts' \
  --derive 'lib/$1.js' --derive 'lib/$1.mjs' --derive 'lib/$1.d.ts'
# 翻訳
bump-semver sync check --glob '**/*.md' --exclude '**/*-*.md' \
  --derive '$1-ja.md' --derive '$1-fr.md'
```

- pros: 明示的、複数 derived 種別を制御可
- cons: `--derive` 連発、種別増えるたび CLI 引数が伸びる

### 案 C: glob + 自動派生探索

```
# 翻訳
bump-semver sync check --glob '**/*-ja.md'
```

- glob から derived 候補を自動推論 (= suffix strip 版 + 同 prefix の別 suffix)
- pros: 引数最小、kawaz ee1ebc05 の「-他 suffix を探す」を文字通り実装
- cons: 派生規約が暗黙、誤マッチ問題 (`-history.md` 巻き込み等) を別ロジックで弾く必要

### 案 D: explicit pair リスト (config file)

```
bump-semver sync check --map sync-map.yml
```

```yaml
# sync-map.yml
- source: src/foo.ts
  derived: [lib/foo.js, lib/foo.mjs, lib/foo.d.ts]
- source: proto/api.proto
  derived: [generated/api.pb.go, generated/api_pb2.py]
- source: README.md
  derived: [README-ja.md]
```

- pros: 柔軟、誤マッチ皆無、metadata 不要
- cons: 設定 file 要、ファイル増減で更新コスト

### 案 E: regex マッピングルール

```
# bundle
bump-semver sync check --glob 'src/**/*.ts' \
  --derive 's|^src/(.+)\.ts$|lib/$1.js|' \
  --derive 's|^src/(.+)\.ts$|lib/$1.d.ts|'
# 翻訳
bump-semver sync check --glob '**/*-ja.md' --derive 's|-ja\.md$|.md|'
```

- pros: 任意の命名規約に対応
- cons: regex 学習コスト、エスケープ罠

### 案 F: 慣習前提の最小設計

```
bump-semver sync check
```

- 引数なしで「リポ固有の規約 file」 (`.sync.yml`) を読み込んで実行 (= プロジェクト設定駆動)
- pros: 究極シンプル、CI 用途に最適
- cons: 設定 file が前提、初回導入コスト

### 案 G: bump-semver vcs サブコマンド埋め込み

```
bump-semver vcs check-derived --glob 'src/**/*.ts' --derive 'lib/$1.js' [--vcs auto]
```

- pros: vcs ファミリ (DR-0020) との整合
- cons: 6.3 の責務混在 cons

### 案 H: 報告 mode と check mode の分離

```
bump-semver sync check  ...    # exit code で fail
bump-semver sync report ...    # 鮮度マトリクスを stdout / json 出力、exit 0
```

- pros: 用途別に subcommand 分離、CI と人間用途を切り分け
- cons: subcommand 増、scope 拡散

### 案 I: stub auto-create を別 verb に分離

```
bump-semver sync check ...     # 鮮度 check のみ (副作用なし)
bump-semver sync init ...      # 派生不在ファイルの stub 生成 (副作用あり)
```

- pros: 副作用の有無で verb 分離 = lint ツール慣例 (`fmt --check` vs `fmt`)、`--fix` flag より明示的
- cons: 学習要素増

## 8. その他検討事項

- **既存 pkf-tasks `docs:check-translations` との比較**: 現行は commit-lag + 翻訳特化 links の 2 軸を umbrella で並列実行。新 CLI は本体を汎用 sync check に絞り、翻訳特化の links は副次 check 扱い (4 章)
- **bump-semver 自身の dogfooding**: 本リポの `README.md` ↔ `README-ja.md` / `DESIGN.md` ↔ `DESIGN-ja.md` / `UPGRADING.md` (英のみ) で挙動検証可能 (= 翻訳ケース)。将来 src ↔ generated 等のケースが追加されれば dogfooding 範囲が広がる
- **bump-semver vcs `vcs diff -q` 拡張可能性**: 「ある file が指定 rev より遅れているか」だけなら既存機能の組合せで実現可。網羅 (= source × 複数 derived 種別) が未対応
- **VCS 依存の段階性**: VCS なし (= mtime fallback) を許すか、VCS 必須にするか
- **派生 suffix / regex pattern の表現力**: 1 章マッピング軸での regex の許容範囲 (= 後方参照、glob like vs PCRE、エスケープ規約) は汎用論点として残る
- **rule precedence**: 案 A〜E を組み合わせる場合の優先順位 (例: explicit map > glob auto-discover)
- **配布形態**: 6.1 採用なら bump-semver release に同梱、6.2 採用なら新 homebrew formula

## 9. 議論起点・次のアクション

- **kawaz レビュー観点**:
  - 1 章: マッピング軸 — source → derived の派生関係をどう表現するか (1.1〜1.7、suffix-strip / regex / explicit / metadata / manifest)
  - 2 章: 派生不在の扱い — 「実在のみ」「stub 生成」「仮想ペア fail」のどれを採るか
  - 3 章: 鮮度比較機構 — committer timestamp (現行) vs commit-id 同一性 vs ancestry 判定
  - 6 章: 実装場所 — bump-semver verb 集約 vs 別 CLI 切り出し
  - 7 章: 叩き台引数案 — verb 名 (`sync` / `derived` / `outdated` 等) 含めて案 A〜I のどれが「いける」「ダメ」か
- **spec 確定後**: 別 PR で DR 起票 (DR-0024 or 次番号) → 実装着手 → DR-0022 follow-up クローズ
- **本ドキュメントの破棄タイミング**: DR 起票後、本 issue メモは `docs/issue/` から delete、議論経緯は journal / DR 本文へ昇格 (kawaz の docs-knowledge-flow ルール準拠)

## 10. kawaz 追加 DSL 案 (2026-06-01)

`glob:` prefix 入力モード (= v0.30.0 で land、DR-0024) を **基盤** にした派生 sync check の **mini-DSL** 案。kawaz の自己評価「必須展開とマッチがうまく噛み合ってシンプルルールながらも矛盾なさそう」。

### 10.1 後方参照 `$N`

glob の wildcard マッチ展開**パーツ**毎に `$1` 〜 `$9` の後方参照を生成:

- `*` = 1 group
- `**` = 1 group
- `[]` = 1 group (= 文字クラス 1 個)
- `{}` = 1 group (= 分岐の選択値)

例:

```
vcs sync-check 'glob:**/*-ja.md' '$1/$2.md'
# -ja が正本、$1 = ** マッチ (= ディレクトリパス)、$2 = * マッチ (= basename 前半)
```

### 10.2 必須展開とマッチの使い分け

- **`{a,b,c}` 分岐展開** = **全展開後の path が必須** (= 全部存在チェック)
- **`*` / `**` / `[]` wildcard** = **マッチ集合 (= 任意、不在 OK)**

これが kawaz の「必須展開とマッチがうまく噛み合う」の核心。

例:

```
vcs sync-check 'glob:**/*-ja.md' '$1/$2.md' 'glob:$1/$2-*.md'
# 第 1 派生 = $1/$2.md (= en、必須 literal path)
# 第 2 派生 = glob:$1/$2-*.md (= 任意マッチ、$1/$2-fr.md 等あれば対象、なければ無視)

vcs sync-check 'glob:**/*-ja.md' '$1/$2.md' 'glob:$1/$2-{cn,fr}.md'
# 第 1 派生 = en (必須)、第 2 派生 = cn と fr (全展開必須)
```

### 10.3 ペア区切り `--` (N ペア対応)

複数 (source, derived) ペアを 1 コマンドで:

```
vcs sync-check FROM TO[..]                              # 1 ペア (-- 省略可)
vcs sync-check -- FROM1 TO1[..] -- FROM2 TO2[..]        # 複数ペア (-- 必須、ambiguity 排除)
```

タスクランナー内で「翻訳ペア + bundle ペア + proto ペア」を 1 コマンドで集約 = プロセス起動コスト削減 + VCS lookup 集約。

### 10.4 FROM 側の `{...}` 分岐

```
vcs sync-check 'glob:**/*-{en,ja}.md' 'glob:$1/$2-[a-z][a-z].md'
# en/ja どちらの更新でも基準扱い、$1 = **、$2 = * 部分
# 各 FROM 起点について TO[..] 共有
```

または:

```
vcs sync-check 'glob:{FROM1,FROM2}' TO[..]
# 複数 FROM を 1 ペアで集約、TO[..] 共有 (= TO 重複記述回避)
```

### 10.5 A が A' 集合に含まれる場合の自動除外

`-ja.md` 正本に対して TO 側 glob `glob:$1/$2-*.md` がマッチした場合、正本自身 (`$1/$2-ja.md`) も pattern マッチしうるが、**A は A' から自動除外** (= 起点を派生集合に含めない)。

### 10.6 後方参照の埋め込み構文

- `$1` 〜 `$9` は 1 桁前提
- `${1}00` で曖昧回避 (= `$100` だと「$1 + リテラル `00`」か「$100 (= 100 桁) 参照」か曖昧)
- `${10}` 等の 2 桁参照も技術的にサポート可能、ただし実用上 1-9 で十分

### 10.7 制限事項 (kawaz 確定 2026-06-01)

`,` / `[` / `]` / `{` / `}` / `*` / `~` などを含むファイル名は glob 経由で扱えない:
- 異常な命名は対象外と割り切る
- 回避手段: 通常の path 指定で個別渡し (`bump-semver vcs diff -- path/to/odd[file].ts`)
- **クオート / エスケープ仕様は導入しない** (= bug 温床回避、仕様簡素化)

### 10.8 subcommand path の議論

- `vcs sync-check` (= VCS subverb、kawaz 例で使われた)
- `bump-semver pair check` (= pair 配下)
- `bump-semver derived check` (= derived 配下)

VCS と直交 (= ファイル鮮度 check は VCS 機能じゃない) なので `vcs` 配下が適切かは spec 確定時に判断。

### 10.9 Phase 分離

- **Phase 1**: `glob:` prefix 単体 (= v0.30.0 land 済、DR-0024)
- **Phase 2**: `$N` 後方参照 + ペア区切り + 自動除外 = `vcs sync-check` (or 別 subcommand) の本体実装
- 本 issue は Phase 2 の要件発散材料、kawaz spec 確定後に DR 起票 → 実装

---

## 付録: 出典 session

- `e4f74abe-376b-4551-94e1-96d6e5890b1a` (2026-05-30) — 5c0735cc: kawaz の元アイデア「A → A' リスト、コミット鮮度比較」
- `6a0a262f-e376-42e6-a1ee-7bfe0e777bc9` (2026-05-30) — ee1ebc05: 「`**/*[._-]ja.md` 起点、別 suffix 探索、または無ければ新規パス群」(= 翻訳例で語られたが本質は派生群拡張の話)
- `6a0a262f-e376-42e6-a1ee-7bfe0e777bc9` (2026-05-31) — 0c769a9b: 本 follow-up #32 スコープ確定 (要件発散、叩き台引数案 OK)
- `e7c503b3-f269-42c5-a83d-b73ffc78346f` (2026-06-01): mini-DSL 案 (= `$N` 後方参照、N ペア、`glob:{...}` source 分岐、制限事項) + `glob:` prefix v0.30.0 land
- (2026-06-01) — 本質汎化訂正: 「翻訳ペアは 1 ケースで、本質は source → derived 鮮度同期の汎用 check。bundle / generated 等同構造ケース多数。lang code 正典化議論はズレてる」
