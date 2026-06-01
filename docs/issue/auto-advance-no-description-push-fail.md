# Bug: `--jj-bookmark-auto-advance` が no description な dirty @ を push しようとして fail

Status: 未解決 (2026-06-01 確認、glob: prefix PR (v0.30.0) land 中に再現)

## 再現手順

1. `just push` 走らせる
2. 内部 `ci` → `lint-go` で `gofmt -w .` 実行
3. 未整形 file (例: 別 session が忘れた整形差分) があれば working copy が dirty 化
4. `--jj-bookmark-auto-advance` の dirty 判定 → @ に bookmark set
5. @ は jj が auto-create した new empty change (= **no description**) + gofmt 差分
6. push 時 jj が「`Won't push commit XXX since it has no description`」で exit 3
7. 再度 `just push` してもループ (= gofmt -w が再度走る → 同じ状態)

## 復旧方法

`--jj-bookmark-auto-advance` 抜きで直接 push:

```sh
jj bookmark set main --allow-backwards -r <release-commit>  # 必要なら
bump-semver vcs push --branch main                           # auto-advance なし
```

これで land 復旧、ただし @ に gofmt 差分が残るので次 push までに別 commit にする or restore 必要。

## 根本原因

`--jj-bookmark-auto-advance` の dirty 判定が、@ の description 有無を check してない:
- dirty → @ に set (現状)
- 「dirty かつ no description」 → push 拒否される状況が予期されてない

## fix 候補

### A. auto-advance に description 必須チェック追加

dirty + @ に description あり → set、@ に description なし → エラー + hint「`jj describe` してから retry」。これで早期 fail、ループ防止。

### B. auto-advance を @- のみに set (= 元仕様変更)

kawaz 元仕様「dirty なら @ に set (= 先端 commit immutable 化 pattern)」を撤回、常に @- に set。シンプルだが「dirty + describe 済 + push したい」運用を捨てる。

### C. `lint-go` の `gofmt -w` を pre-check 化

push 前に `gofmt -d .` で diff 確認、整形差分があれば fail。user に「先に整形 commit しろ」と促す。ただし `lint-go` の責務拡張 + just push の gate 増加。

### 推奨

**A 案**: 元仕様維持しつつ早期 fail。description なしの dirty @ で auto-advance が走ると **明示的エラー**、ユーザに「`jj describe` してから retry」案内。ループ防止 + kawaz の元方針 (= 先端 commit immutable 化 pattern) も維持。

## 関連

- DR-0020 PR-5.2: `--jj-bookmark-auto-advance` 元設計
- DR-0020 PR-5.2.1: git で silent no-op 化 (= backend-prefix general rule)
- PR-2.2: `vcs is clean` の jj 判定を `empty` template 単体に revert (= empty な change は clean のはず、ただし description 有無は別軸)
