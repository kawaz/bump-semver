# Issue: 対応ファイル追加要望の窓口整備 (= ISSUE_TEMPLATE / CONTRIBUTING / labels)

- Status: Memo (Decision 不要、Phase 1 で実施するだけのもの)
- Date: 2026-06-03
- Related: 本 issue は `2026-06-03-cli-user-defined-rule.md` の姉妹編。あちらが「ユーザが自分で
  指定できる口」(= `--define-rule`)、こちらが「未対応ファイルを **builtin に追加してほしい**
  という要望を kawaz が受け取る窓口」を扱う。

## Context

bump-semver を **個人使用だけでなく他者に勧めたい** 段階に到達 (kawaz、2026-06-03)。
他者の自作 / 業務リポで使うファイル形式が builtin に未対応の場合、逃げ道は 2 つ:

1. **`--define-rule` で自分で指定する** (= 別 issue で扱う、Phase 1 で実装予定)
2. **「builtin に追加してほしい」と要望を上げる** (= 本 issue で扱う)

(2) の経路が現状ほぼ空白なので整備する。

## やること (= 実装 todo、決定事項なし)

### 1. `.github/ISSUE_TEMPLATE/format-request.yml` の新設

GitHub の **issue form** (= yaml で定義する構造化テンプレ) で、ユーザに以下を尋ねる:

| field | 説明 | 必須? |
|---|---|---|
| ファイル名 / 拡張子 | `myapp.config` / `*.zon` 等 | yes |
| 何で使われるファイルか | ツール名 / 言語 / フレームワーク + URL | yes |
| サンプルファイル (PRE-formatted) | version 行が分かる短い実例 (5-20 行) | yes |
| version のパス | サンプルでの位置を `jq`/dot-path/regex どれかで | yes |
| name のパス (optional) | 同上 (任意) | no |
| 関連リポ / 公式ドキュメント | リンクが分かれば添付 | no |

form 化することで:
- ユーザが情報を出し惜しまない (= 入力欄が見えるので埋めようとする)
- kawaz が後で読み返すとき、過不足が分かりやすい
- bot で初期分類できる

### 2. 自動ラベル

`format-request` ラベルを repo に作成 (色は green / blue 系、視認性重視)。
issue form の上で `labels: [format-request]` で自動付与。

将来別カテゴリの要望テンプレが増えたら `enhancement` / `bug` 等の既存系列と
被らない名前空間で。

### 3. `CONTRIBUTING.md` の新設

最小構成:
- bump-semver のスコープ (= version 抜き取り / 書き換え) と非スコープ (= 設定管理ツール全般ではない)
- format-request を上げる前にユーザ側で `--define-rule` で代替できないか自己診断する案内
- PR 受付の前提 (DR がある場合は DR を参照しての変更が必要、テスト必須、等)
- レビュー周期の目安 (kawaz は週末しか触れない、等)

短くて十分。

### 4. README に「対応してほしいファイルがあれば」セクション追記

README / README-ja の末尾近くに、format-request issue へのリンクと
`--define-rule` の存在 (= 待たずに自分で書く道) を案内。

## 議論不要なもの (= memo 段階で明示)

- form の項目選定は kawaz の好みで最終決定
- ラベル文言 (`format-request` で問題ない)
- CONTRIBUTING の長さ (短く)

DR 起票は不要。Phase 1 と並行して実装する純粋な雑務扱い。

## 関連

- `2026-06-03-cli-user-defined-rule.md` (= 姉妹編、`--define-rule` の DR draft)
- DR-0001 (= 「必要が出たら 1 行追加」哲学)
- 他リポの参考: `kawaz/claude-session-analysis` 等の既存 kawaz/* リポは ISSUE_TEMPLATE
  を持っていない (= 本 issue で雛形を作って他にも展開可能)
