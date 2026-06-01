# 翻訳ペア check CLI: 要件発散と設計議論材料

> Status: 要件発散段階 (kawaz による方針確定前)
> 目的: kawaz が「これはダメ」「意外といける」を判断できる選択肢を網羅する
> 注意: 本ドキュメントは **正解を提示しない**。複数案の trade-off を並列展示し、kawaz の判断材料を提供する

## 背景

`docs:check-translations` (現状は kawaz/pkf-tasks の Pkl で 100+ 行の bash として実装、本リポは `Taskfile.pkl` shim 経由で利用) を単機能 CLI に切り出す follow-up。DR-0022 の残課題で、実現すれば Taskfile.pkl + pkfire 依存をゼロにできる (`lint:pkl` recipe / pkf-tasks pin 監視も道連れに drop 可能)。

kawaz の元アイデア (session e4f74abe, 5c0735cc 2026-05-30):

> ファイル A に対して、A'[s] の導出を正規表現マッチングとかで紐付けルールで導出、A は A' に対して同じコミットまたは古いコミットでなければいけない。A が A' 全てに対して Newer コミットに位置していてはいけない。A リスト (複数 or Glob) を渡したら、それぞれの A に対して A' に対応するファイルを探して、各ファイルの最新が組まれてるコミットを A と全て比べる。bump-semver でやるか別でやるかは要検討。

kawaz の追補 (session 6a0a262f, ee1ebc05 2026-05-30):

> 実際は僕ルールでは `**/*[._-]ja.md` 辺り (実際は曖昧にする必要ないのでハイフンだけで良いが) を A として渡すと、マッチしたあるファイル a があり、a から `-ja` を除去したファイル、また `-他lng.md` を探す**または無ければ新規パス群**を a'∈A' とし〜。

kawaz の今回スコープ確定 (session 6a0a262f, 0c769a9b 2026-05-31):

> 32 はや具体的な引数設計とかはやらなくてよい。やりたいことを引数で表現する形に引数設計を落とし込むの難しいので。マッピング方法やファイルの A' の確定要件はどんなものがあるか？存在するものの集合なのか、なければ作るまで含むのか、など。あり得るかもしれない色々な要件の軸を発想の幅を広げて〜列挙する書き留めておいて下さい。叩き台としての引数設計案も作ってくれるのは構わない。

→ 本ドキュメントは **要件軸の発散** が主、引数設計は叩き台のみ。

## 0. 現行実装との対照 (= 出発点の明示)

現行 `kawaz/pkf-tasks/tasks/docs/translations.pkl` (v2.1.0) は以下を行う:

| 軸 | 現行挙動 |
|---|---|
| 入力 | CLI 引数 (glob 展開済 path 列) / Pkl の `forPairs(...)` 固定リスト / 引数なしで cwd 配下 `*-ja.md` を `find` で auto-discover (`.jj` `.git` `.out` `node_modules` 除外) |
| マッピング | `*-ja.md` 正本 → `${prefix}.md` の **1 対 1 (en のみ、多言語非展開)**。理由は誤巻き込み回避 (例: `data-layout-ja.md` → `data-layout-history.md` を拾わない) |
| 鮮度比較 | VCS の **committer timestamp** (`jj log -T 'committer.timestamp().format("%s")'` または `git log -1 --format=%ct`) で `tgt_ts < src_ts` を lag 判定 |
| 副次 check | links: ja ↔ en 相互リンクの存在 (`> [English](./X.md) | 日本語` / `> English | [日本語](./X-ja.md)`) を head 5 行から検出 |
| 未追跡 file 扱い | timestamp 取得失敗時は 0 に正規化 (exit 2 にならないように) |

## 1. マッピング方法の要件軸

「ja (= A) から派生 (= A') をどう導出するか」のバリエーション。

### 1.1 suffix-strip + 近所探索 (glob ベース、現行実装系)

- 入力 glob で正本集合を確定 (`**/*-ja.md`)
- 各正本から suffix (`-ja`) を strip → 派生候補 (`${prefix}.md`) を導出
- 派生候補が **実在すれば対象**、不在は無視 (= 現行 1 対 1 挙動)
- kawaz ee1ebc05 案: 同じ prefix の `${prefix}-??.md` / `${prefix}-???.md` (2-3 文字 lang code) も派生に含める (多言語拡張)
- 長所: シンプル、命名規約に乗れば設定 0、kawaz 元アイデアに最も近い
- 短所: **誤マッチ問題**: `data-layout-ja.md` の場合 prefix=`data-layout`、`data-layout-history.md` を拾いうる (現行は en 1 対 1 でこれを回避)。多言語拡張するなら誤マッチを別ルールで弾く必要 (例: lang code を ISO 639-1/2 ホワイトリストに限定)
- 短所: 正本側 suffix が 1 種類 (`-ja`) しか想定できない (en 正本 = `*.md` 起点だと拡張子だけになり区別不能)

### 1.2 任意正規表現置換ベース

- 正本 → 派生のマッピングを `s/-ja\.md$/-en.md/` のような任意 regex で表現
- ユーザが置換ルールを config に書く (`pair-rules.yml` 等)
- 長所: 命名柔軟、suffix-strip だけでは表せないパターン (= `i18n/ja/foo.md` ↔ `i18n/en/foo.md` のようなディレクトリ階層別命名) に対応可
- 短所: ユーザ設定要、学習コスト、誤設定でゼロマッチが silent

### 1.3 explicit pair リスト

- ペアを明示列挙: `pair-map.yml` で `README-ja.md: [README.md]` のように書く
- 長所: 規約レス、最も明示的、誤マッチ皆無
- 短所: ファイル数 N に応じて列挙コスト線形増、規約化のメリットを捨てる

### 1.4 frontmatter / 内容ベース

- ファイル先頭 frontmatter / コメントで `<!-- translation-of: README.md -->` のような tag を書く
- 長所: 命名規約フリー、ディレクトリ階層と独立、移動に強い
- 短所: 既存ファイル全部に tag 追加要、tag 漏れが silent

### 1.5 manifest ベース (= 翻訳 manifest 1 ファイルで全体管理)

- 1 ファイル (`translations.json` or `.bump-semver-pairs.yml`) で全ペアを宣言
- 長所: 1 箇所で見通せる、CI で manifest と実態の乖離 check 可
- 短所: ファイル増減のたびに manifest 更新要 (= 二重管理コスト)、ただし auto-discover + manifest diff の組合せで実用化可

### 1.6 双方向マッピング (= どちらが正本かを動的判定)

- ファイル A・B 両方の中身 / mtime / lang 検出から「より新しい方を正本扱い」「翻訳元 frontmatter で動的に判定」
- 長所: 「ja が常に正本」前提を持たないプロジェクトに対応
- 短所: 責務肥大、kawaz の「ja 正本前提」と方向性が違うので採用は本案外想定

### 1.7 ハイブリッド (= デフォルト規約 + override)

- デフォルトは 1.1 (suffix-strip)、override で 1.2 / 1.3 / 1.4 を局所適用
- 長所: 規約適用範囲が広いリポは 0 設定、例外は明示
- 短所: 実装複雑

## 2. A' (派生集合) の確定要件

「派生集合をどう確定し、不在ファイルをどう扱うか」の要件軸。kawaz が明示的に「存在するものの集合なのか、なければ作るまで含むのか」と提起した中核論点。

### 2.1 「派生は実在ファイルのみ」(現行実装の挙動)

- マッピングで導出した派生候補のうち、実ファイルが存在するもののみ check
- 不在は「翻訳まだ始めてない」と解釈、silent pass
- 長所: noise なし、追加コスト 0
- 短所: 「ja に追加した内容に対応する en 派生を作り忘れた」を検知不能
- 短所: kawaz ee1ebc05 の「なければ新規パス群」と整合しない

### 2.2 「派生不在を warn (= 報告 only)」

- 不在派生を stderr / log に出力するが exit code は変えない
- 長所: 見落とし防止、CI を blocking しない、移行に優しい
- 短所: warn が noise 化して読み飛ばされるリスク
- 細分化: warn level を「未作成 OK」「未作成 fail」で flag 切替可能に

### 2.3 「派生不在を fail (= strict)」

- 想定する全 lang の派生が存在しなければ fail
- 必須 lang を引数 / config で指定 (`--require ja,en`)
- 長所: 翻訳網羅性を CI で保証
- 短所: 翻訳途中の段階で常に fail、運用厳しい

### 2.4 「派生不在を auto-create (= stub 生成)」

- 派生不在時は stub ファイル (= 空 or boilerplate or 「TODO: translate」) を auto-create
- 翻訳者が後で中身を埋める
- 長所: 翻訳開始の負担軽減、kawaz ee1ebc05 の「なければ新規パス群」を文字通り解釈するならこの案
- 短所: ツール責務肥大 (= check が write を伴う = 副作用大)、`--check` / `--fix` 二態化 (lint ツールの慣例) が要る
- 細分化: stub 生成は別 verb (`pair init` / `pair scaffold`) に分離する選択肢もある

### 2.5 「派生不在は仮想ペアとして check 対象 (= 鮮度比較で fail)」

- 派生不在 = 「派生の commit 鮮度 = 0 (= 一度も commit されていない)」とみなし、正本との鮮度比較で自動 fail
- 長所: stub 生成 (= 副作用) を伴わず strict 検査と同等の効果
- 短所: 「派生は実在 file 必須」前提を破る、エラーメッセージで「ファイルが存在しない」を明示する工夫要

### 2.6 「派生候補集合の動的拡張」(lang glob)

- 派生 lang を CLI 引数で明示 (`--also en --also fr --also de`)、または auto-discover (= 既存派生 lang を repo 内 scan で発見)
- 静的 (引数) vs 動的 (auto-discover) で挙動分岐
- 長所: 多言語対応、新 lang 追加時の追従性
- 短所: lang code の正典化 (ISO 639-1?) 要、誤検出抑制要

### 2.7 「派生集合の境界に file 種別を含める」

- 派生として md だけでなく `README.txt` (legacy), `README.rst` (Python 系) も含めるかどうか
- 長所: docs 形式混在リポに対応
- 短所: フォーマット軸を更に持ち込む、scope 拡散

## 3. 鮮度比較の要件軸

「A ≤ A' を何で判定するか」の軸。kawaz の元発言は「同じコミットまたは古いコミット」だが、具体機構は複数候補ある。

### 3.1 committer timestamp 比較 (= 現行実装の挙動)

- `jj log -T 'committer.timestamp()'` / `git log -1 --format=%ct` で各 file の最新 commit timestamp を取得、`tgt_ts < src_ts` で lag
- 長所: 実装が枯れている (現行実装そのもの)、VCS 横断で安定
- 短所: rebase / cherry-pick で timestamp が書き換わる、committer date は author date と乖離する場合あり (= `commit --amend` で committer date のみ更新)
- 未追跡 file は 0 に正規化 (現行実装)

### 3.2 commit-id / rev 同一性比較

- 各 file の最新 commit-id を取得、「派生の最新 commit-id == 正本の最新 commit-id」または「派生の commit が正本 commit の子孫」を OK
- 長所: timestamp の罠 (rebase) を回避、kawaz の「同じコミット」を文字通り解釈
- 短所: 「子孫判定」が VCS API として高コスト (= ancestry graph traversal)、特に jj/git 両対応で実装複雑

### 3.3 トポロジカル順序 (= ancestry graph)

- 正本 commit が派生 commit の祖先かどうかを `git merge-base --is-ancestor` 系で判定
- 長所: rebase に頑健、「正本変更が派生に反映されているか」を厳密に答える
- 短所: 単一 commit で同時に両 file を変更したケース (= 同じ commit でも順序は付かず祖先関係なし) を OK 扱いにするか fail にするかの解釈分岐
- 短所: 実装高コスト、特に file 単位の最新 commit を抽出する `git log -1 -- <file>` の出力との合成が必要

### 3.4 内容 hash + 派生 metadata

- 正本の内容 hash を派生の frontmatter / コメントに記録 (`<!-- source-hash: abc123 -->`)、正本変更で hash 不一致になれば fail
- 長所: 順序非依存、明確、git の挙動と独立
- 短所: 各派生に metadata 必須、運用コスト、初回導入のマイグレーション要

### 3.5 mtime ベース

- file mtime で比較
- 長所: VCS 不要、最速
- 短所: checkout / clone 直後 mtime はファイル順序を反映しない、`touch` で揺れる、CI で不安定 → **実用不可寄り**

### 3.6 「N 日許容」(= 鮮度の hysteresis)

- 派生が正本より新しいか同じである必要なく、「正本変更後 N 日以内なら派生が遅れていても OK」
- 長所: 翻訳遅延を許容、人間ペースに合わせる
- 短所: 設定要、release gate には不向き (release 直前で「あと N 日待って」になる)

### 3.7 比較粒度の選択

- 鮮度比較を「file 単位」「行単位 (= 翻訳セクション単位)」「文単位」で行うか
- 長所: 細かい粒度ほど false positive 減 (file の一部だけ変わっても全派生が lag 判定されない)
- 短所: 行/文単位は実装高コスト、現状 file 単位で十分

### 3.8 (補足) untracked / 未 fetch の扱い

- 現行は timestamp=0 に正規化、結果として「正本 untracked / 派生 untracked」がペアで揃ってる時のみ pass
- alternative: 「untracked file が含まれる場合は warn 出して skip」「pre-commit hook での運用なら untracked = 新規追加なので OK 扱い」など、運用文脈で挙動を変える選択肢

## 4. 副次 check の取り扱い

現行実装は commit-lag 以外に **bilingual links 整合性 check** (ja ↔ en の `> [English](./X.md) | 日本語` / `> English | [日本語](./X-ja.md)` 規約) を別 task として持つ。

### 4.1 CLI に links check を含めるか

- A. 含める (= 現行 `check-translations` umbrella と同等)
- B. 含めない (= commit-lag only に絞る、links は別ツール / 別 verb)
- C. plugin / flag で opt-in (`--check links`)

### 4.2 links 規約の汎用性

現行は kawaz 慣習 (`> [English](./X.md) | 日本語`) ハードコード。汎用 CLI 化するなら:

- regex で規約を渡せる (`--link-pattern '> [English](\./.*\.md) | 日本語'`)
- 規約 preset 名 (`--link-style kawaz`) で選択
- そもそも links check は scope 外として落とす

## 5. ユースケース列挙 (使う側の視点)

要件発想の発散用。網羅性が目的、各々に賛否を付けない。

- **release 前 gate**: 派生追従漏れで fail (= pkf-tasks docs:check-translations と同等の用途、Justfile の `check-translations` recipe からの呼び出し)
- **pre-commit hook**: local で commit 前 check、未追跡 file は許容
- **CI on PR**: PR 単位で派生漏れ検出、main マージ前に blocking
- **翻訳開始のスキャフォールド**: ja 新規追加 → 派生 stub 自動生成 (2.4 を採用するなら)
- **鮮度確認 only (report mode)**: exit 0 維持、現状の遅れ状況を一覧表示するだけ
- **鮮度マトリクス**: `README.md` `README-ja.md` `README-fr.md` ... の遅れ commit 数を表で出す
- **lang ごとの統計**: 「en 派生は 90% 追従、fr 派生は 40% 追従」のサマリ
- **ja 起点で派生 N 個全部古い vs 1 個だけ古いの区別**: 全派生で同じだけ遅れているか、特定 lang だけ遅れているか
- **正本変更時に派生 lang を自動算出して issue 起票**: GitHub Actions 連携で「en 派生が遅れています、@translator-en assign」自動 PR / comment
- **派生 lang の追加検出**: 既存 en だけのリポに fr を新規追加した瞬間に「全 ja 正本に対する fr 派生不在」を一覧化
- **bump-semver dogfooding**: 本リポの `README.md` ↔ `README-ja.md` (+ `DESIGN.md` ↔ `DESIGN-ja.md`) に適用、CI に組み込む

## 6. 実装場所の検討軸

### 6.1 bump-semver 内 verb (`bump-semver pair check ...`)

- pros: bump-semver の vcs サブシステム (DR-0020) を共通基盤として再利用、リポへの「正本→派生の鮮度 check」として release gate に親和性高い
- pros: 単一バイナリ配布で済む、ユーザは追加 install 不要
- cons: bump-semver の責務 (semver bump / vcs 操作) と翻訳ペア責務 (ファイル意味論) が軸違い、scope 拡散懸念
- cons: バイナリサイズ増、bump-semver を semver 用途だけで使うユーザにも追加コード

### 6.2 別単機能 CLI (例: `translation-pair`, `pair-watcher`, `i18n-lag`)

- pros: bump-semver の責務肥大化を避ける、独立 release cycle / 独立 versioning
- pros: 単機能の発展余地 (e.g., links check / stub 生成 / 鮮度マトリクス) が制約なく持てる
- cons: 共通基盤 (VCS 操作) を別途実装 or bump-semver vcs に library 依存
- cons: ユーザに追加 install を強いる、homebrew tap も別に必要

### 6.3 bump-semver vcs verb の拡張 (`bump-semver vcs check-pairs ...`)

- pros: vcs ファミリ (DR-0020) との整合、`vcs:latest-tag()` 入力モードの拡張として位置づけ可
- cons: vcs verb の責務 (VCS 操作の抽象化) と翻訳ペア責務 (ファイル意味論) の混在、vcs verb の概念純度が落ちる

### 6.4 bump-semver library + thin CLI

- pros: bump-semver を Go library として release、別 CLI からも import 可
- cons: bump-semver の Go internal 公開 API 化が必要 (現状 main package 直 link 想定)、release 互換管理コスト

### 6.5 「pkfire を捨てない」案 (= 現状維持 + 機能拡張)

- pros: 実装ゼロ、現行 pkf-tasks の translations.pkl を拡張するだけ
- cons: DR-0022 の「pkf 依存を翻訳 check だけに絞る」を経た上で更にゼロ化したい follow-up なので、本選択肢は本タスクの趣旨と逆走

## 7. 叩き台引数設計案 (kawaz 判断用、複数案併記)

**注**: kawaz は「具体的引数設計はやらなくてよい」「叩き台レベルなら歓迎」と確定済。以下は議論呼び水の叩き台で、最適解の提示ではない。

### 案 A: glob + suffix-strip (kawaz 元アイデア準拠)

```
bump-semver pair check --glob '**/*-ja.md' --strip '-ja'
```

- glob で正本集合、`--strip` で派生 path 導出 (`-ja` → 単一派生 1 対 1)
- pros: シンプル、現行実装と等価
- cons: 多言語非対応、kawaz ee1ebc05 の多言語 + 新規パス群とは別方向

### 案 B: glob + suffix-strip + 多言語拡張

```
bump-semver pair check --glob '**/*-ja.md' --strip '-ja' --also '-en' --also '-fr'
```

- pros: 明示的、多言語制御可
- cons: `--also` 連発、lang 増えるたび CLI 引数が伸びる

### 案 C: glob + 自動派生探索

```
bump-semver pair check --glob '**/*-ja.md'
```

- glob から派生候補を自動推論 (= `-ja` 除去版 + 同 prefix + 2-3 文字 lang code suffix)
- pros: 引数最小、kawaz ee1ebc05 の「-他lng.md を探す」を文字通り実装
- cons: 派生規約が暗黙、誤マッチ問題 (`data-layout-history.md` 巻き込み) を別ロジックで弾く必要

### 案 D: explicit pair リスト (config file)

```
bump-semver pair check --map pair-map.yml
```

```yaml
# pair-map.yml
- source: README-ja.md
  derived: [README.md, README-en.md, README-fr.md]
- source: docs/DESIGN-ja.md
  derived: [docs/DESIGN.md]
```

- pros: 柔軟、誤マッチ皆無、frontmatter 不要
- cons: 設定 file 要、ファイル増減で更新コスト

### 案 E: regex マッピングルール

```
bump-semver pair check --glob '**/*-ja.md' --derive 's/-ja\.md$/.md/' --derive 's/-ja\.md$/-en.md/'
```

- pros: 任意の命名規約に対応
- cons: regex 学習コスト、エスケープ罠

### 案 F: ja 正本前提の最小設計

```
bump-semver pair check
```

- 引数なしで `**/*-ja.md` を auto-discover、派生は `-ja` 除去版を実在チェック
- pros: 究極シンプル、kawaz の「ja 正本 = 自分の慣習」前提で 0 引数
- cons: 慣習依存、規約外プロジェクトで使えない

### 案 G: bump-semver vcs サブコマンド埋め込み

```
bump-semver vcs check-pairs --glob '**/*-ja.md' --strip '-ja' [--vcs auto]
```

- pros: vcs ファミリ (DR-0020) との整合
- cons: 6.3 の責務混在 cons

### 案 H: 報告 mode と check mode の分離

```
bump-semver pair check  ...    # exit code で fail
bump-semver pair report ...    # 鮮度マトリクスを stdout / json 出力、exit 0
```

- pros: 用途別に subcommand 分離、CI と人間用途を切り分け
- cons: subcommand 増、scope 拡散

### 案 I: stub auto-create を別 verb に分離

```
bump-semver pair check ...     # 鮮度 check のみ (副作用なし)
bump-semver pair init ...      # 派生不在ファイルの stub 生成 (副作用あり)
```

- pros: 副作用の有無で verb 分離 = lint ツール慣例 (`fmt --check` vs `fmt`)、`--fix` flag より明示的
- cons: 学習要素増

## 8. その他検討事項

- **既存 pkf-tasks `docs:check-translations` との比較**: 現行は commit-lag + links の 2 軸を umbrella で並列実行。新 CLI で links を含めるか否かは 4.1 で論点化
- **bump-semver 自身の dogfooding**: 本リポの `README.md` ↔ `README-ja.md` / `DESIGN.md` ↔ `DESIGN-ja.md` / `UPGRADING.md` (英のみ) で挙動検証可能
- **bump-semver vcs `vcs diff -q` 拡張可能性**: 「ある file が指定 rev より遅れているか」だけなら既存機能の組合せで実現可。網羅 (= ペア × 多言語) が未対応
- **VCS 依存の段階性**: VCS なし (= mtime fallback) を許すか、VCS 必須にするか
- **lang code の正典化**: ISO 639-1 (2 文字: ja/en/fr) / 639-2 (3 文字: jpn/eng/fra) / BCP 47 (`ja-JP`, `zh-Hans`) をどこまでサポート、現行は「2-3 文字 lang code」と緩い regex
- **rule precedence**: 案 A/B/C/D/E を組み合わせる場合の優先順位 (例: explicit map > glob auto-discover)
- **配布形態**: 6.1 採用なら bump-semver release に同梱、6.2 採用なら新 homebrew formula

## 9. 議論起点・次のアクション

- **kawaz レビュー観点**:
  - 1 章: マッピング軸 — どの案を採るか (1.1〜1.7、kawaz ee1ebc05 多言語が起点)
  - 2 章: 派生不在の扱い — 「実在のみ」「stub 生成」「仮想ペア fail」のどれを採るか
  - 3 章: 鮮度比較機構 — committer timestamp (現行) vs commit-id 同一性 vs ancestry 判定
  - 6 章: 実装場所 — bump-semver verb 集約 vs 別 CLI 切り出し
  - 7 章: 叩き台引数案 — 案 A〜I のどれが「いける」「ダメ」か
- **spec 確定後**: 別 PR で DR 起票 (DR-0024 or 次番号) → 実装着手 → DR-0022 follow-up クローズ
- **本ドキュメントの破棄タイミング**: DR 起票後、本 issue メモは `docs/issue/` から delete、議論経緯は journal / DR 本文へ昇格 (kawaz の docs-knowledge-flow ルール準拠)

## 付録: 出典 session

- `e4f74abe-376b-4551-94e1-96d6e5890b1a` (2026-05-30) — 5c0735cc: kawaz の元アイデア「A → A' リスト、コミット鮮度比較」
- `6a0a262f-e376-42e6-a1ee-7bfe0e777bc9` (2026-05-30) — ee1ebc05: 「`**/*[._-]ja.md` 起点、-他lng 探索、または無ければ新規パス群」
- `6a0a262f-e376-42e6-a1ee-7bfe0e777bc9` (2026-05-31) — 0c769a9b: 本 follow-up #32 スコープ確定 (要件発散、叩き台引数案 OK)
