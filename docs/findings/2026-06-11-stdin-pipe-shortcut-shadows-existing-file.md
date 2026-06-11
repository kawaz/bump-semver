# `get FILE` が空 stdin パイプ環境 (GitHub Actions 等) でのみ失敗する原因と修正

- Date: 2026-06-11
- 関連: 旧 issue `docs/issue/2026-06-10-get-cargo-toml-fails-only-on-github-actions.md` (解決済・削除)
- 関連 DR: DR-0021 (cargo workspace.package fallback), DR-0029 (stdin-pipe shortcut + CLI rule)
- 修正: `src/resolve.go` stdin-pipe shortcut の発火条件にファイル不在ゲートを追加 + 空内容を明示エラー化

## 判明した事実

- 真の原因は **CPU アーキでも Rosetta でもなく stdin の形状**。`bump-semver get FILE` を
  引数 1 個で呼ぶと、入力解決が「入力 1 個 && `-`/`vcs:` でない && stdin がパイプ」のとき
  **legacy stdin-pipe shortcut** に入り、ディスク上のファイルを無視して stdin から内容を読む。
- GitHub Actions の `run:` step の stdin は **writer 不在の名前付きパイプ (FIFO, mode `prw-------`)**。
  `isStdinPipe()` は true を返すため shortcut が発火し、`io.ReadAll(stdin)` は即 EOF で **0 バイト**を返す。
- 空内容を `toml.Unmarshal` してもエラーにならず**空の map** になる → `.package.version` も
  `.workspace.package.version` も missing → confidence-3 Cargo.toml rule 失敗 → confidence-1
  `*.toml` fallback も `.version` 無しで失敗 → `*.toml (fallback): missing version` (exit 2)。
- これは **silent wrong-answer 型**のバグ。例外でなく「最後に試した rule のエラー」として表面化するため、
  あたかも toml parse か workspace 対応の問題に見えるが、実体は **入力内容が空**。
- アーキ非依存。ローカル (darwin/arm64) でも `printf '' | bump-semver get FILE --no-hint` で
  完全再現 (exit 2, 同一エラー, stdinMode=`prw-rw----`)。旧 issue の「実 x86_64 のみ失敗 /
  Rosetta Docker は成功」は **誤った相関 (red herring)**。Docker 検証は TTY か非パイプ stdin
  だったため file 経路を通って成功していただけ。

## 実用的な示唆 / 切り分けの勘所

- **環境依存で「CI でのみ失敗」を見たら、まず stdin/stdout/stderr の形状 (TTY / pipe / FIFO /
  closed fd) を疑う**。CPU feature やアーキの相関は最後に疑う。同一 sha256 binary + 同一 sha256
  入力が環境で結果を変える場合、変数は「プロセスへの配線」(fd の種類・env・cwd) 側にある。
- 再現は「実環境のコピー」でなく **病的な配線を最小再現** するのが速い。今回は
  `printf '' | cmd FILE` (= 空パイプ stdin) で 1 行再現できた。
- shortcut/fallback 系のロジックは「**入力 0 バイト**」を独立した失敗モードとして扱うこと。
  空入力が下流で「versionless document」として静かに通ると、誤った下流エラーに化ける。

## 修正の詳細

第一 (設計修正): stdin-pipe shortcut の発火条件に **`!statFileExists(inputs[0])`** を AND。
shortcut の本来意図は `cat my.txt | bump-semver get my.txt` のように **存在しないファイル名を
フォーマットのヒント**として使い内容は pipe から読む用途。実在ファイルは常にディスクが真実なので
`os.ReadFile` 経路 (stdin 非依存) を通す。

- 既存テスト温存の根拠: `TestRun_StdinPipe` (`package.json`),
  `TestDefineRule_StdinPipe_ExtensionWithoutBuiltin` (`myapp.env`) はどちらも
  **存在しないファイル名**を使うため、不在ゲートで全て pass のまま。
- 唯一の挙動変更: **実在 path 名 + 別内容を pipe** した病的ケースで「pipe でなく file を読む」に
  変わる。これは未定義・非意図動作なので守る価値が薄い。同一内容を pipe するケースは結果同一で実害なし。

第二 (防御, self-check の対極補完): `resolveFileFromStdinWithRules` で読んだ content が空なら
**`stdin: empty input`** で明示エラー。`-` マーカ経路 (`readStdinLine`) は既に空入力を
エラーにしていたのに pipe-shortcut 経路 (`io.ReadAll`) だけ空チェックが抜けていた (片面実装)。
これ単体でも今回の silent wrong-answer を「分かるエラー」に変えるが、根治は第一。両方入れて堅くした。

## 検証マトリクス (修正後, ローカル darwin/arm64)

| 入力 | stdin | 期待 | 結果 |
|---|---|---|---|
| 実在 `workspace-root/Cargo.toml` | 空パイプ (writer 即 close) | file を読む → `0.3.1` | OK exit 0 |
| 実在 `Cargo.toml` | `[workspace.package] version=1.1.1` を pipe | **file 優先** → `0.3.1` | OK exit 0 |
| 非存在 `does-not-exist.json` | `{"version":"9.9.9"}` を pipe | shortcut → `9.9.9` | OK exit 0 (name-hint 用途温存) |
| 非存在 `package.json` (既存テスト) | `{"version":"1.2.3"}` を pipe | shortcut → `1.2.4` | OK (TestRun_StdinPipe) |

回帰テスト: `src/stdin_pipe_existing_file_test.go`
(`TestRun_ExistingFile_NotShadowedByEmptyStdinPipe`,
`TestRun_WorkspaceRootFixture_NotShadowedByEmptyStdinPipe`)。
fixture: `tests/fixtures/workspace-root/Cargo.toml`。

## 診断オプションについての所見 (将来 TODO, 優先度低)

旧 issue の `--debug-rules` 案は今回のバグには直接効かなかった (rule 解決でなく stdin 経路で
空内容になるため)。より有効なのは「**読んだ content のバイト数 + 入力解決経路 (file / stdin-pipe /
vcs)**」を 1 行出す軽量トレース (`-vv` 相当)。`resolved input0 from <stdin-pipe|file>, N bytes`
だけで同種問題の切り分けが一瞬になる。フル `--debug-rules` より安価。
