# DR-0013: 既知 suffix を剥がして既存ルールで再判定する fallback (`Cargo.toml.bak` / `package.json.20260510` 等)

- Status: Accepted
- Date: 2026-05-11
- Closes: docs/issue/2026-05-10-suffix-stripped-format-detection.md
- Related: DR-0001 (basename 自動判定 + 「必要が出たら 1 行追加」), DR-0005 (path-aware confidence ranked candidates), DR-0010 (confidence 1 fallback hint), DR-0011 / DR-0012 (直近の format 拡張パターン)

## Context

レガシー環境では VCS に入れていない手動 backup ファイル (`Cargo.toml.bak` / `package.json.20260510` / `Chart.yaml~` 等) を `bump-semver` で読みたい / 比較したい運用が実在する。`vcs:` 入力 (DR-0008) は VCS に入っているリビジョン専用なので、disk 上に保存された任意 suffix 付きファイルはカバーできない。

現状 (v0.9.x) では `bump-semver get Cargo.toml.bak` は `unsupported file: Cargo.toml.bak` でエラーする。利用者は:

- 一度 `cp Cargo.toml.bak /tmp/Cargo.toml` でリネームして `bump-semver` に通す
- 諦めて自前 `sed` を書く

のどちらかの動線を強いられる。これは DR-0001 「必要が出たら 1 行追加」哲学からも、DR-0010 「fallback で動かして hint で透明性を確保」哲学からも一歩遅れている状態。

ファイル名の suffix を剥がして既存ルールに通せば実需 95% は吸収できる。残った誤判定 (`Cargo.toml.template` のように中身が別物の場合) は DR-0010 hint と extraction failure で利用者に伝わる。

## Decision

### 1. 既知 suffix のリスト

以下のいずれかが basename の **末尾 1 段**として現れたら剥がす対象とする:

- `.bak`
- `.backup`
- `.orig`
- `.tmp`
- `.old`
- `.YYYYMMDD` (8 桁数字、例 `.20260510`)
- `.YYYYMMDD_HHMMSS` (8 桁数字 + `_` + 6 桁数字、例 `.20260510_120000`)
- 末尾 `~` (Emacs 系 backup)

`~` 以外はすべて basename の末尾に `.` がついた形で剥がす。判定はあくまで「末尾 1 段」であり、`Cargo.toml.bak.20260510` のような多段 suffix は **末尾 1 段だけ** を剥がす (= `Cargo.toml.bak` で再試行 → 失敗で終了)。詳細は決定事項 4 で後述。

### 2. 判定フロー

DR-0005 の `resolveRule` を以下のように拡張する:

1. 通常の path-pattern マッチを confidence 降順で全試行 (既存挙動)
2. **すべてのルールが失敗** したとき、basename から既知 suffix を 1 段剥がした path で **再帰せずに** もう一度全試行 (= 通常マッチの 2 周目)
3. 2 周目で hit したルールを採用、ただし confidence は **1 段下げて** 扱う (Inspection の `MatchedConfidence` に反映)
4. 2 周目でも失敗、または剥がせる suffix が無いなら従来どおり `unsupported file:` エラー

「ルール解決成功 → 既存挙動」「ルール解決失敗 → suffix を剥がしてもう一度」という二段構造。実装は `resolveRule` の最後 (matched なし or 全ルール extraction 失敗) で `stripKnownSuffix` を呼び出して再試行する 1 ブロックの追加で済む。

### 3. confidence の降格

通常マッチで採用された confidence を suffix stripping 経由では 1 段下げる:

| 通常マッチ confidence | 元ルール例 | suffix stripping 後の confidence |
|---|---|---|
| 3 (path-pinned) | `Cargo.toml` / `package.json` | **2** |
| 2 (basename) | `marketplace.json` / `mix.exs` | **1** |
| 1 (glob fallback) | `*.json` / `*.yaml` | **1** (= floor、これ以上下げない) |

confidence 1 が床。「suffix stripping 経由かつ glob fallback」は **同じ confidence 1 で扱うが、後述の suffix hint で透明性を確保する** (DR-0010 hint と直交)。

#### 「内部実装上は同じ confidence 1 で扱って hint 出すのが筋」

issue で言及の通り。confidence 1 の床を割って 0 や負値にすると、`MatchedConfidence == 1` を見ている既存ロジック (DR-0010 hint emission) が壊れる。代わりに「confidence は 1 のまま、suffix stripping したという事実だけ別フィールドで持つ」設計を採用する。

### 4. 多段 suffix は末尾 1 段のみ (YAGNI)

`Cargo.toml.bak.20260510` のようなチェーンは:

- `.20260510` を剥がして `Cargo.toml.bak` で再試行 → 失敗
- そこで停止 (`.bak` をさらに剥がして `Cargo.toml` で再試行する再帰は **しない**)
- 結果: `unsupported file:` エラー

理由:

- 末尾 1 段で実需の 95% (`.bak` 単体 / `.20260510` 単体) を吸収できる
- 再帰すると無限ループ防止のループカウンタや「どの段で剥がしたか」の hint 文言が複雑化
- `.bak.20260510` のような重複命名はそもそも稀で、必要が顕在化したら別 DR で再帰版を追加 (DR-0001 哲学)
- `~` は basename 末尾の 1 文字なので 1 段判定とは別軸 (剥がしても `.bak` 等が出てくる組み合わせは滅多にない)

issue の検討ポイント (a) 採用。

### 5. 不対応 suffix

以下は **意図的に known list に入れない** (現状通り `unsupported file:` エラーで止める):

- `.template`
- `.example`
- `.sample`
- `.dist`

これらは「同じファイル名にぶら下がるが中身は別物 (テンプレート / サンプル)」という性格で、suffix を剥がして元ルールに通すと中身の不一致で extraction error になる確率が高い。誤って成功した場合のリスクも高い (`Cargo.toml.template` の placeholder version `0.0.0` を本物として読んでしまう)。

利用者が本当に template から抽出したいなら、`cp Cargo.toml.template Cargo.toml.tmp && bump-semver get Cargo.toml.tmp` のように **明示的な backup 系 suffix にリネーム** すれば動く。`.template` を間接的に救う動線を持たせるよりも、利用者の意図を明確にする方が安全。

### 6. Hint 出力 (DR-0010 機構の再利用)

suffix stripping で fallback hit したとき、stderr に 1 行の `hint:` を出す。文言は:

```
hint: <orig-path> matched as <stripped-basename> rule (suffix <suffix> stripped); use --no-hint to suppress
```

例:
```
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

`~` の場合:
```
$ bump-semver get Cargo.toml~
hint: Cargo.toml~ matched as Cargo.toml rule (suffix ~ stripped); use --no-hint to suppress
1.2.3
```

#### DR-0010 hint との共存

suffix stripping した結果、剥がした basename が confidence 1 (glob) ルールに該当した場合、`unknown.json.bak` のような連鎖ケースでは **両方の hint が同時に出る**:

```
$ bump-semver get unknown.json.bak
hint: unknown.json.bak matched as unknown.json rule (suffix .bak stripped); use --no-hint to suppress
hint: unknown.json.bak matched as *.json fallback. Open issue if explicit support is needed.
1.2.3
```

利用者にとって透明性が一番高いので、両方出す。両方とも `hint:` prefix で揃っているので `--no-hint` 一発で抑制できる。

順序は **suffix hint が先、DR-0010 fallback hint が後** (suffix stripping は「ファイル名の解釈」、fallback hint は「中身の解釈」で、概念的に前者が先)。

#### 抑制機構

DR-0010 と完全に共有:

- `--no-hint`: 全 hint 抑制
- `-q` / `--quiet`: stdout + 全 hint 抑制
- `-qq` / `--quiet-all`: stdout + 全 hint + error 抑制

suffix hint 専用フラグは追加しない (利用者が覚えるパターンを増やさない)。

### 7. Inspection 構造体への追加

`MatchedSuffixStripped string` フィールドを `Inspection` に追加する:

- 空文字列なら「suffix を剥がしていない (= 通常マッチ)」
- 非空なら剥がした suffix の文字列 (`.bak` / `.20260510` / `~` 等)

`MatchedConfidence` / `MatchedGlob` (DR-0010 で導入) と並ぶ「rule 解決のメタ情報」フィールド。`emitFallbackHints` (`src/main.go`) はこれを見て suffix hint を出力する。

### 8. multi-file での name/version 整合性検証は変わらない

DR-0004 の cross-input consistency 検証はファイル**内容**を見るので、suffix stripping は無関係。例:

```bash
bump-semver patch Cargo.toml Cargo.toml.bak --write
```

- `Cargo.toml` (confidence 3, 通常マッチ)
- `Cargo.toml.bak` (confidence 2, suffix stripping)

両者から抽出された version が一致していれば bump し、`--write` で `Cargo.toml.bak` も書き戻される。`Cargo.toml.bak` 側は suffix hint が 1 行出る。

`--write` で suffix 付きファイルを書き戻すべきかは利用者責任 (backup ファイルを bump するのは特殊用途)。ツール側で禁止はしない (vcs: のように「読み取り専用」性質ではない)。

### 9. テスト戦略

- `src/suffix_test.go` 新規:
  - `stripKnownSuffix` の単体テスト (各 known suffix で剥がせる、未知 suffix で剥がさない、多段は 1 段のみ)
  - 各 known suffix で各 format (Cargo.toml / package.json / VERSION / Chart.yaml / build.zig.zon / `*.gemspec`) が解決できる
  - `.template` / `.example` で `unsupported file:` エラー
  - `Cargo.toml.bak.20260510` で 1 段だけ剥がして失敗 (= 再帰しない)
- `src/hint_test.go` 拡張:
  - suffix hint 出力 + `--no-hint` / `-q` / `-qq` 抑制
  - DR-0010 hint との共存 (両方発火、`--no-hint` で両方消える、suffix hint が先)
- 既存テスト全通し (regression なし)

## 不採用案

### A. 再帰的に suffix を剥がす (issue 検討ポイント (b))

`Cargo.toml.bak.20260510` → `Cargo.toml.bak` → `Cargo.toml` まで剥がす案。検討したが:

- 実需が薄い (`.bak.20260510` のような重複命名はそもそも稀)
- 無限ループ防止のループカウンタ + どの段で剥がしたかの hint 文言が複雑化 (1 段だけなら単純)
- 必要になった時点で別 DR で「再帰版」を opt-in で追加できる (デフォルト挙動を後から拡張)

末尾 1 段のみで実需の大半を吸収できるという判断。YAGNI。

### B. `.template` / `.example` / `.sample` も known suffix に含める

「現状通り unsupported error にしておくと、template から version を抽出したいユーザは諦める」という主張で含める案。検討したが:

- template 系の中身は placeholder (`0.0.0` / `__VERSION__`) であることが多く、誤って実 version として読む方が事故
- 利用者が本当に template から抽出したいなら、`.tmp` 等の backup 系にリネームすれば動く (動線は確保されている)
- 「suffix で意図を明示する」という設計の側面を維持

### C. 専用エラー型 `suffixStrippedFailedError`

「suffix stripping で再試行したが失敗した」を `unsupported file:` と区別するため別エラー型にする案。検討したが:

- 利用者にとっての違いは小さい (どちらも「対応してない」「issue を立てて」の動線で済む)
- DR-0010 の `unsupportedFileError` の hint (`Open issue at https://...`) と同じ誘導でよい
- エラー型を増やすと `errors.As` の分岐が増えるだけで価値が低い

引き続き `unsupportedFileError` 単一型で扱う。

### D. suffix list を CLI フラグで拡張可能に (`--strip-suffix .custom`)

利用者が任意の suffix を追加できるようにする案。検討したが:

- DR-0001 「ルール 1 行追加」哲学に反する (組み込みテーブルで吸収するのが筋)
- known suffix list は backup 系の慣習をカバーする目的で定義しており、ad-hoc な拡張点は不要
- 必要が出たら DR を立てて known list に追加すればいい (5 種 + 日付 2 形式 + `~` で大体カバー済み)

### E. confidence 1 を割って 0 / 負値にする

「suffix stripping 経由は本来のマッチより明確に劣る」という主張で confidence 0 / 負値にする案。検討したが:

- `MatchedConfidence == 1` を gate にしている既存ロジック (DR-0010 hint emission) が壊れる
- confidence の単調性 (3 > 2 > 1) を崩すと dispatcher 内部の sort が複雑化
- 「fallback hit である」事実は `MatchedSuffixStripped` フィールドで持てば十分

confidence 1 を床として、suffix stripping は別フィールドで補助情報として持つ設計に。

### F. suffix hint を `warning:` prefix で出す

DR-0010 と同様の議論。「suffix で動いた」のは正常系で警告ではない (動かせる範囲で動かす + 透明性を hint で確保)。`hint:` prefix で抑制機構と挙動を完全共有する方が一貫性が高い。

## Consequences

### 実装変更

- `src/suffix.go` 新規: `stripKnownSuffix(path) (newPath, suffix string, ok bool)` ヘルパ
- `src/rules.go` の `resolveRule`: 通常マッチ全失敗時に suffix stripping を試行する 1 ブロック追加
- `src/handler.go` の `Inspection`: `MatchedSuffixStripped string` フィールド追加
- `src/main.go` の `emitFallbackHints`: suffix hint 出力ロジック追加 (DR-0010 hint と並列、suffix hint が先)
- `src/suffix_test.go` 新規: 単体 + 統合テスト
- `src/hint_test.go` 拡張: suffix hint テスト + 共存テスト + 抑制テスト
- README / README-ja: 「Supported file formats」セクションに suffix-stripped fallback の説明追加
- `UPGRADING.md`: v0.9.x → v0.10.0 セクション (純粋追加、互換性維持)

### 後方互換

- CLI 表面は完全維持
- これまで `unsupported file:` だった backup 系 suffix 付きファイルが新たに動く (純粋追加)
- 既存テストへの影響なし (既存ファイル名は通常マッチでヒットするので suffix stripping パスに入らない)
- stderr の `hint:` 行が増えるだけ (stdout は不変、CI の stdout pipe は影響なし)

### v0.9.x → v0.10.0

新機能追加なので minor bump (SemVer 0.x.y 慣習: 後方互換が保たれていても新規挙動追加は minor)。`UPGRADING.md` に v0.9.x → v0.10.0 セクションを追加。

### ROADMAP

`docs/ROADMAP.md` の「次のフェーズ」候補から「suffix 吸収 (Cargo.toml.bak 等)」を削除 (本 DR で消化)。

### 次のフェーズ

- TOML section-scoped 対応 (`pyproject.toml` の `[project].version`、`mojoproject.toml` の `[workspace].version`) → 別 DR
- `*.pbxproj` 専用 format (複数同期更新) → 別 DR
- Mojo 対応 (上の TOML section-scoped に同梱) → 別 DR

## 関連

- DR-0001: 「必要が出たら 1 行追加」哲学
- DR-0005: confidence ranked dispatcher (本 DR の前提機構)
- DR-0008: `vcs:` 入力 (本 DR は disk 上 backup を補完する位置付け)
- DR-0010: confidence 1 fallback hint (本 DR の hint 機構を共有)
- DR-0011: yaml/yml/toml fallback (本 DR の suffix stripping は全 format で動く)
- DR-0012: regex format (本 DR の suffix stripping は regex 形式 (`*.gemspec.bak` 等) でも動く)
