# idea: yaml の Replace に current 値照合 (TOML 同種の防御) を追加

- Date: 2026-06-10
- Status: idea

## Context

`format_toml.go` の `tomlAssertMatchedValue` は、Inspect が読んだ値と regex が掴んだ行の値が
一致しているかを Replace 前に検証し、ズレがあれば early error で止める仕組みを持つ。
`format_yaml.go` には同種の検証がない。

## yaml 側の現状とリスク評価

yaml の Replace 実装は column-0 anchor を前提としたトップレベル限定の構造で、
対象行のズレが起きにくい設計になっている。TOML と比べてリスクは低い。

ただし `format_yaml.go` の Replace 処理は Inspect が取得した current 値を受け取る口があるにも関わらず
その値を使っておらず、TOML と構造が揃っていない。

## 対応内容

TOML の `tomlAssertMatchedValue` と同様に、yaml の Replace で:

1. Inspect が返した current 値と regex がマッチした行から読み取れる値が一致しているかを確認
2. 一致しない場合は early error で止める

## 懸念点

- yaml は TOML に比べて多値・ネスト構造のパターンが少なく、誤検出が起きにくい
- 対応コストに対して得られる安全性向上が小さい可能性がある

着手前に実装コストと効果を見積もって判断する。
