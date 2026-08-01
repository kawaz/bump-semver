# 裁定・確認待ち一覧 (ユーザ用)

## 運用規約

<details>
<summary>ゼロコンテキストエージェント向け（本セクションは消さない）</summary>

- 裁定/確認待ち項目を 1項目=1ラベル=1セクション で記載
- ラベル形式: XX-Q1（XX は 2-3 文字、バッチやセッション内で一意、Qn単独の使い回し禁止、長期一意性は不要)
- 依頼形式: 「👺XX-Q1 の裁定お願いします」（参照用途ではラベルに👺を付けない。誤陽性がユーザのハイライト/アラームを汚す）
- チャット提示と同一ターンで本ファイルに記録 + path 指定 commit (push はリリース窓に同乗)
- 裁定が下りたら該当セクションを即削除し、内容は正規の記録先 (DR / issue / journal / close_reason) へ反映。本ファイルは常に「現在待ち」だけを持つ
- 参照は[]()で提示（リポ内は相対、リポ外はフルパス）
- 初版質問/依頼は長文で書かない（ユーザが説明を求めらたら本ファイルに説明を追加し、チャットで👺ラベルで再依頼）
- **選択肢・確認項目は `- [ ] a: …` 形式（チェックボックス + ラベル）で書く**。
  Q / C で記法を分けない。回答は「チェックを付ける」でも「XX-Q1a」と言葉で返すでも通る
  （複数まとめてチェックし「チェックしたよ」の一言で済ませる運用を想定）

</details>

## 裁定待ち

### 👺ANP-Q1: `--allow-nonexistent-path` ヒント誘導問題の対処方針

参照: [docs/issue/2026-07-31-vcs-commit-allow-nonexistent-path-hint-drops-deletions.md](issue/2026-07-31-vcs-commit-allow-nonexistent-path-hint-drops-deletions.md)

- [ ] a (推奨): ヒント文言のみ改善 — "silently drop" の drop 対象に **tracked-but-deleted も含む** ことをヒント/help text に明示し、削除意図の path があるケースへの安易な適用を抑制。実装挙動は据え置き。
- [ ] b: フラグ semantics 見直し — `--allow-nonexistent-path` を「truly unknown path (@- でも tracked でない) のみ drop、tracked-but-deleted は保持」に変更。破壊変更 + legacy `bump` 互換の意図から乖離。
- [ ] c: 何もしない (close as won't-fix) — フラグ名通りの動作、利用側 (claude-local-issue) は既に「フラグを使わない」invariant で対策済み、実害の直接証拠なし。

推奨理由: (b) はフラグ名 (nonexistent = filesystem 上に無い) と意味論的に乖離しないし legacy 互換破壊。(c) は誤誘導リスクが残る。(a) は最小侵襲でフラグ名の意味論を保ちつつ drop 対象を明示できる。なお現状のデフォルト挙動 (フラグ無し) は削除 path を正しく commit するので、そもそも利用側の 3 path 指定パターンでフラグを付ける必要はない (= 実装バグではなく文言誘導の問題)。

## 確認待ち

(なし)
