# DR-0005: basename 決め打ちから path-aware confidence ranked candidates へ

- ステータス: Accepted
- 日付: 2026-05-09
- 関連: DR-0001 (basename 自動判定の精神は維持、テーブル化で拡張)、DR-0004 (multi-file での name/version 整合性検証は再利用)

## 文脈

v0.3.0 までの dispatch は basename 決め打ちだった:

```go
switch {
case base == "Cargo.toml":     return cargoHandler{}, nil
case base == "VERSION":        return versionHandler{}, nil
case base == "package-lock.json": return npmLockHandler{}, nil
case strings.HasSuffix(base, ".json"): return jsonHandler{}, nil  // top-level $.version 決め打ち
default:                       return nil, errUnsupported
}
```

claude-cmux-msg の bump-semver 移行作業 (cmux-msg-impl ワーカー) で `.claude-plugin/marketplace.json` の version が `$.metadata.version` (ネスト) と判明。`bump-semver get .claude-plugin/marketplace.json` がそのまま `JSON: missing top-level "version"` で落ちた。

「個別 handler を増やす」 (`handler_marketplace_json.go`) は **対応ファイル種別が増えるたびに handler 乱立** → いずれ `/simplify` で統合される未来が見え、そもそも筋が悪い。最初からテーブル + 確度ランキング + フォールバックで書くのが正解 (kawaz の判断、2026-05-09)。

## 決定

### 1. CandidateRule テーブルへの一元化

dispatch を「path-pattern + format + 抽出パス」のテーブルにする。テーブルは confidence で並び、上から評価:

```go
type CandidateRule struct {
    Name         string   // 表示名 (エラーメッセージ用)
    PathSuffix   string   // 高確度: ".claude-plugin/marketplace.json"
    Basename     string   // 中/高確度: "marketplace.json" / "package.json"
    Glob         string   // 低確度: "*.json"
    Confidence   int      // 3=path-pinned, 2=basename-only, 1=glob fallback
    Format       string   // "json" / "toml" / "plain"
    NamePaths    []string // 0+ (optional)
    VersionPaths []string // 1+ (必須)
}
```

### 2. 確度ランキング + try → fallback

- **3 (path-pinned)**: `.claude-plugin/marketplace.json` のような相対 suffix。最も具体的に当たるルールを優先
- **2 (basename only)**: 任意ディレクトリ下の `marketplace.json` / `plugin.json` 等
- **1 (glob fallback)**: `*.json` 一般

dispatch アルゴリズム:

1. 全ルールを confidence 降順で巡回
2. ルールの path-pattern が input path にマッチ → 抽出 (Inspect) を試行
3. 抽出成功 (全 VersionPath が x.y.z パース可能なら成功) → そのルールを採用
4. 失敗 → 次のルールへ降りる
5. 全ルールが失敗したら、最後のエラーを path + ルール名付きで返す

これにより:
- `./.claude-plugin/marketplace.json` → confidence 3 のルールで `$.metadata.version` 抽出
- `./任意/marketplace.json` → confidence 2 で `$.metadata.version` を try、失敗なら confidence 1 (`*.json` fallback) で `$.version`
- `./config.json` のような任意 JSON → confidence 1 fallback で `$.version`、無ければエラー

### 3. Handler interface (Inspect/Replace) は維持

既存の `Handler` interface (`Inspect`, `Replace`) は触らない。dispatch 結果を `ruleHandler` という単一の concrete type にラップして main.go から呼ぶ。multi-file 整合性検証 (DR-0004) は変更不要。

### 4. JSON path 表記の追加

複数の version field と nested 構造を扱うため、jq 風サブセットの path 表記を内部で持つ:

```
.version
.metadata.version
.packages[""].version
.packages["node_modules/foo"].version
```

文法は `.identifier` と `["quoted-string"]` の繰り返し。実装は `src/jsonpath.go` (`parseJSONPath` / `jsonPathExtract` / `locateJSONPath`)。

`Replace` は `json.Decoder` で構造を streaming 走査して各 path の値の byte offset を集め、末尾から in-place 置換する。これにより `package-lock.json` の依存エントリ (`$.packages["node_modules/..."]`) が偶然 root と同 version でも書き換わらないことが保証される (DR-0004 と同じ手法を一般化)。

### 5. Format-specific の責務分離

```
src/
  rules.go          # CandidateRule + テーブル + dispatch (try → fallback)
  jsonpath.go       # path parser + extractor + locator (+ skipValue/findQuotedRange helper)
  format_json.go    # json: Inspect / Replace
  format_toml.go    # toml: Inspect / Replace ([package].version 限定)
  format_plain.go   # plain: Inspect / Replace
  handler.go        # Field / Inspection / Handler interface + ruleHandler
  main.go           # CLI parse + multi-file orchestration (変更最小)
  semver.go         # 既存維持
```

旧 `handler_cargo.go` / `handler_json.go` / `handler_npm_lock.go` / `handler_version.go` は削除。

### 6. 後方互換

CLI 表面は完全に維持:

- `bump-semver patch package.json --write` → 既存通り (confidence 3 で package.json ルールにマッチ、または confidence 1 fallback で動く)
- 単一 FILE / 複数 FILE / `--value` / `--write` / stdin pipe の挙動は不変
- DR-0004 の name/version 整合性検証も全 file 横断で動く

抽出失敗時のエラー文面は変わる (新フォーマットでは "rule name + path" を含むようになる) が、CLI 互換性破壊ではない。

## 不採用案

### A. 個別 handler の追加 (`handler_marketplace_json.go` 等)

ファイル種別が増えるたびに handler 乱立、`/simplify` の統合候補リストに必ず入る。最初から避ける。

### B. `--json-pointer /metadata/version` フラグ

CLI 表面が肥大、利用者の覚えるパターンが増える。組み込みテーブルで吸収するのが筋 (DR-0001 「regex フォールバック不採用」と同じ精神)。

### C. JSON Schema / `$schema` を見て自動判別

`marketplace.json` には `$schema` が無い場合もある (kawaz の運用では現状無し)。判別根拠が不安定で、テーブル + suffix で十分。

### D. テーブルではなく function dispatcher

各ルールを `func(path, content) (Inspection, error)` として登録するスタイル。テーブルの宣言性が失われ、エディタで一覧したいときに不利。Go の zero-value 互換も活きない。

### E. confidence を「ファイル名スコア」ではなく「抽出成功スコア」にする

「実際に抽出できたかどうか」を後ろから決める方式。これは現実装と等価だが、ルールのレスポンス時間が遅くなる場合がある (path で fail させられないため)。Path-pattern matching を gate にする現方式の方が単純で速い。

## 影響

- 対応ファイル種別の追加が **テーブル 1 行** で済む。Helm Chart.yaml / Bun bun.lock / Cargo.lock 等は format-specific 関数 1 つ + テーブル数行で対応可能 (将来の minor リリース)
- `handler_*.go` の個別ファイルが 4 個 → 削除、format-specific 3 ファイル + rules.go + jsonpath.go に再構成
- claude-cmux-msg の `.claude-plugin/marketplace.json` 移行が unblock、3 ファイル一括 bump (DR-0004 multi-file) と組み合わせて 1 行に集約可能

## 関連

- 親 issue: `docs/issue/2026-05-09-path-aware-confidence-ranked-candidates.md` (本 DR 採択により削除)
- DR-0001: 基本仕様 (flat 4-action + basename 自動判定の精神は本 DR で維持・拡張)
- DR-0004: 複数 FILE 整合性検証 (本 DR と直交、組み合わせで最強)
- 関連 issue (lockfile 系、本 DR 体制で吸収可能になった): `2026-05-09-bun-lock-support.md` / `2026-05-09-yarn-lock-support.md` / `2026-05-09-pnpm-lock-yaml-support.md` / `2026-05-09-cargo-lock-support.md`
- 主要消費者: kawaz/claude-cmux-msg の `version-bump` レシピが 1 行になる
