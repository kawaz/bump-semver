# suffix 付きファイルのフォーマット検出 (`Cargo.toml.bak` 等)

レガシー環境では VCS に入っていない手動 backup ファイル (`.bak` / `.backup` / `.orig` / 日付スタンプ等) を比較したい運用が実在する。`vcs:` 入力ではカバーできないため、format detection 側で suffix を吸収する機能を別途検討する。

## ユースケース

```bash
# レガシー環境で、手動 backup と現在を比較
bump-semver compare gt Cargo.toml Cargo.toml.bak

# 日付スタンプ付き backup と比較
bump-semver compare gt Cargo.toml Cargo.toml.20260510

# Emacs 系 backup
bump-semver compare gt Cargo.toml Cargo.toml~
```

現状 (v0.7.0 時点) は `unsupported file: Cargo.toml.bak` でエラー。

## 検討ポイント

### 1. 許容する suffix パターン

**known suffix list 案** (シンプル):
- `.bak` / `.backup` / `.orig` / `.tmp` / `.old`
- `.YYYYMMDD` (8 桁数字)
- `.YYYYMMDD_HHMMSS` (8 桁 + アンダーバー + 6 桁)
- `~` (末尾 tilde、Emacs 系)

**正規表現案** (柔軟):
- `\.(bak|backup|orig|tmp|old)$` または `\.\d{8}(_\d{6})?$` または `~$`

実装的にはどちらでも可。**known suffix list** がメンテしやすく予測可能。

### 2. 複数 suffix チェーン

`Cargo.toml.bak.20260510` のような multi-suffix は許容するか?

- (a) 末尾から 1 段だけ剥がす (`Cargo.toml.bak.20260510` → `Cargo.toml.bak` で match 試行 → 失敗)
- (b) 再帰的に剥がす (`Cargo.toml.bak.20260510` → `Cargo.toml.bak` → `Cargo.toml` で match)

**(b) 再帰** の方が柔軟だが、無限ループ防止が必要 (剥がしてもパターンに該当しなければ stop)。

### 3. confidence 設計

`rules.go` (DR-0005) の path-aware confidence ranked テーブルに統合。

提案: 「suffix-stripped basename で match」を **既存 confidence から 1 段下げて** 試行。

例:
- `Cargo.toml` (confidence 3, path-pinned)
- `Cargo.toml.bak` → suffix 剥がして `Cargo.toml` ルール試行 (confidence 2 相当に降格)

これで「明示的に Cargo.toml と書いた場合」と「suffix 付きで間接的に当てた場合」を区別できる。

### 4. 誤判定リスク

`Cargo.toml.template` (テンプレートファイル) を Cargo.toml ルールで処理 → 中身が違って parse 失敗、エラー。利用者は気づく。

ただし `Cargo.toml.example` のような **意図しないファイル**を間違って渡すリスクはある。

緩和策:
- `.template` `.example` `.sample` のような「明らかに別物」を示す suffix は **吸収しない** (known suffix list から除外、現行通り unsupported error)
- known suffix list を「backup 系のみ」に限定する

### 5. 既存 DR との整合

DR-0005 (path-aware confidence ranked candidates) を拡張する形。新規 DR として:
- DR-00XX (将来): Suffix-aware fallback for legacy backup files

または DR-0005 の更新で済むなら更新。

### 6. fallback マッチを Hint として表示

suffix 吸収で fallback match した場合、stderr に hint を出して透明性を確保する。

```bash
$ bump-semver get Cargo.toml.bak
hint: Cargo.toml.bak matched as Cargo.toml rule (suffix .bak stripped); use --no-hint to suppress
1.2.3
```

利点:
- 利用者は「意図したファイルがどうマッチしたか」即座に確認できる
- 意図せず違うフォーマットと判定された場合 (`Cargo.toml.template` が Cargo.toml ルールに当たる等) を発見できる
- v0.5.0 の `--no-hint` で既に hint 抑制機構があるので、それに乗れる (一貫性高い)

複数 suffix チェーンで多段剥がしした場合も全段の経過を出すと冗長 → **最終的にマッチしたルール名と元ファイル名のペアだけ**を出す形でシンプル。

### 7. テスト戦略

- 各 known suffix で各 format (Cargo.toml, package.json, VERSION) が解決できるか
- multi-suffix チェーン
- 意図しない suffix (`.template`) で unsupported error
- v0.5.0+ の整合性検証 + 複数 file 借用との組み合わせ

## 実装スケッチ

`src/rules.go` の `resolveRule` 内で:

```go
func resolveRule(path string, content []byte) (CandidateRule, Inspection, error) {
    // 1. 通常の path-pattern マッチを試行
    for _, rule := range rulesByConfidence {
        if rule.matches(path) {
            // ...既存ロジック
        }
    }
    // 2. suffix-stripped で再試行
    stripped := stripKnownSuffix(path)
    if stripped != path {
        return resolveRuleWithDowngradedConfidence(stripped, content)
    }
    return error("unsupported file")
}

func stripKnownSuffix(path string) string {
    // .bak / .backup / .orig / .tmp / .old / .YYYYMMDD / .YYYYMMDD_HHMMSS / ~
    // 末尾から 1 段または再帰的に剥がす
}
```

詳細は実装着手時に詰める。

## スコープ外

- リモート/URL 経由のファイル取得 (vcs: でカバー)
- 複数連続 suffix の任意深さ対応 (実用なら 2-3 段で十分)

## 関連

- DR-0005 (path-aware confidence ranked candidates)
- v0.7.0 で実装した `vcs:` 入力でカバーできない範囲を補完する位置付け
- 実需が出たタイミング (kawaz の業務リポ等で suffix 付きファイル比較が必要になった等) で実装着手
