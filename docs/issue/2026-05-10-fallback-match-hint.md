# fallback マッチ時の Hint 表示 (`*.json` 等で動いた場合の案内)

`unknown.json` のような未知のファイル名でも、現在は DR-0005 の confidence 1 fallback (`*.json`) で動く。利用者は「動いた、OK」で済ませがちだが:

- 意図せず違うルールで判定された可能性に気づきにくい
- このファイル形式に明示対応が必要なら issue 起票してほしい (DR-0001 「必要が出たら 1 行追加」哲学のフィードバックループ)

これを促すため、**fallback マッチした際に hint を stderr に出す**。

## 提案

### 発動条件

DR-0005 の confidence ranked candidates で、**confidence 1 (glob fallback)** でマッチした場合のみ hint。

- confidence 3 (path-pinned: `.claude-plugin/plugin.json` 等) → hint なし、明示対応されているので
- confidence 2 (basename: 任意 dir の `plugin.json` 等) → hint なし、basename で明示対応
- confidence 1 (`*.json` fallback) → **hint 出す**

将来 suffix 吸収 (`docs/issue/2026-05-10-suffix-stripped-format-detection.md`) が実装されたときも、同じ機構で hint を出す (機構を共通化)。

### 文面案

```
hint: unknown.json matched as *.json fallback ($.version). Open an issue at
      https://github.com/kawaz/bump-semver/issues if explicit support is
      needed for this filename.
```

短縮形:
```
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
```

短縮形の方が CI ログでうるさくなくて良さそう。GH URL は help text や README で案内すれば十分。

### `--no-hint` で抑制

v0.5.0 の `--no-hint` / `-q` / `-qq` で既存通り抑制可能。CI では `--no-hint` を付ける運用も自然。

### v0.5.0 の hint との共存

両方発火するケース (`bump-semver patch unknown.json`):

```
hint: unknown.json matched as *.json fallback. Open issue if explicit support is needed.
hint: 1 file not modified; use --write to update or --no-hint to suppress
```

`hint:` prefix で揃えて、両方とも `--no-hint` で抑制。

順序: fallback hint を先、`--write` hint を後に出すのが自然 (発生時系列順)。

## 検討ポイント

### 1. 複数ファイル指定時

```bash
bump-semver get a.json b.json unknown.json
```

`unknown.json` だけ fallback、a.json と b.json は明示対応済みの場合:

- (a) **fallback 該当ファイルだけ列挙**: `hint: unknown.json matched as *.json fallback...`
- (b) **全ファイル列挙**: 冗長

(a) 推奨。複数該当なら 1 行ずつ列挙。

### 2. confidence 2 でも hint 出すか

confidence 2 (basename-pinned、任意 dir で `plugin.json` を `.claude-plugin/plugin.json` として扱う) は **明示対応済み**なので hint 不要。

ただし「confidence 2 でマッチしたが、本当はそのファイルが Claude plugin じゃない場合」のリスクはある (例: 全く無関係な `plugin.json`)。ここは現状エラーで処理されている (DR-0005 の挙動: confidence 2 で match して抽出失敗 → 次のルールへ降りる) ので問題ない。

### 3. 完全な未対応ファイル

`unknown.toml` のように fallback も無いケース:
- 現状: `unsupported file: unknown.toml` エラー
- 変更不要、エラーメッセージで GH issue 案内を追加するのも良い:

```
error: unsupported file: unknown.toml
       Open an issue at https://github.com/kawaz/bump-semver/issues if support is needed.
```

これは既存エラー文言の拡張で、hint とは別軸。

## 実装スケッチ

`src/handler.go` の `resolveRule` または `Inspect` で、最終マッチしたルールの confidence を返り値に含める。

```go
func resolveRule(...) (CandidateRule, Inspection, error) {
    // 既存ロジック
    return rule, insp, nil
}

// 呼び出し側 (main.go の resolveInput) で:
if rule.Confidence == 1 && !args.noHint && !args.quiet && !args.quietAll {
    fmt.Fprintf(stderr, "hint: %s matched as %s fallback. Open issue if explicit support is needed.\n", file, rule.Glob)
}
```

`Inspection` 構造体に `MatchedRuleConfidence int` を含めるか、別の戻り値で返すかは実装時に判断。

## スコープ外

- 完全未対応ファイル (`unknown.toml`) のエラーメッセージ拡張 → 既存エラー文言の改善で対応可、別途
- suffix 吸収 (`Cargo.toml.bak`) の hint → suffix 吸収 issue で扱う

## 関連

- DR-0005 (path-aware confidence ranked candidates)
- v0.5.0 の `--no-hint` / `-q` / `-qq` (hint 抑制機構)
- DR-0001 (「必要が出たら 1 行追加」哲学、ユーザフィードバック起点)
- 関連 issue: [2026-05-10-suffix-stripped-format-detection.md](./2026-05-10-suffix-stripped-format-detection.md)
