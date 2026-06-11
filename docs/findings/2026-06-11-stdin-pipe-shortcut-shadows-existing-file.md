# `get FILE` が空 stdin パイプ環境 (GitHub Actions 等) でのみ失敗する原因と修正

- Date: 2026-06-11
- 関連: 旧 issue `docs/issue/2026-06-10-get-cargo-toml-fails-only-on-github-actions.md` (解決済・削除)
- 関連 DR: DR-0021 (cargo workspace.package fallback), DR-0029 (stdin-pipe shortcut + CLI rule)
- 修正: `src/resolve.go` stdin-pipe shortcut を「pipe 内容が空か」で分岐 (非空=pipe 優先で DR-0004 §6 の契約維持 / 空=ディスクへフォールバック / 空かつ不在=ヒント込みエラー)

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

DR-0004 §6 の契約は「単一 FILE + stdin pipe のとき FILE は名前ヒント、内容は stdin から」。
当初の修正案 (発火条件に `!statFileExists` を AND し、実在ファイルなら pipe を無視) はこの契約を
破る: `jj file show v0.1.0 Cargo.toml | bump-semver get Cargo.toml` のように **実在ファイルの
過去リビジョン内容を pipe で渡す**正規ユースケースで、ディスクの現在版が勝ち pipe が黙って無視
される (silent wrong-answer)。

確定した仕様: **発火条件は `isStdinPipe` のみ**に戻し、shortcut 内で **pipe 内容が空かどうか**で
分岐する (`resolveFilePipeOrDisk`):

- **pipe 内容が非空 → pipe が勝つ** (DR-0004 §6 維持。FILE は名前ヒント)。
- **pipe 内容が空 (0 バイト) かつ FILE 実在 → ディスクの FILE 読みにフォールバック**
  (= GitHub Actions の writer 不在 FIFO 救済)。通常の位置引数解決ループへ落とし、`resolveFileWithRules`
  / `os.ReadFile` を単一の真実経路として通す。
- **pipe 内容が空かつ FILE 不在 → `file %q not found and piped stdin was empty` エラー**。
  ユーザの実際の誤り (パス typo) に辿り着けるよう、不在パス名と空 pipe の両方を文言に含める。

これにより `-` マーカ経路 (`readStdinLine`) と shortcut 経路 (`io.ReadAll`) の空入力ハンドリングが
揃い、空入力が「versionless document」として静かに通る silent wrong-answer も解消する。

## 検証マトリクス (修正後, ローカル darwin/arm64)

| 入力 | stdin | 期待 | 結果 |
|---|---|---|---|
| 実在 `workspace-root/Cargo.toml` | 空パイプ (writer 即 close) | ディスクへフォールバック → `0.3.1` | OK exit 0 |
| 実在 `Cargo.toml` (`version=9.9.9`) | `version=0.1.0` を pipe | **pipe 優先** (名前ヒント) → `0.1.0` | OK exit 0 |
| 非存在 `does-not-exist.json` | `{"version":"9.9.9"}` を pipe | shortcut → `9.9.9` | OK exit 0 (name-hint 用途) |
| 非存在 `does-not-exist.json` | 空パイプ | エラー: not found + empty | OK exit 2 |
| 非存在 `package.json` (既存テスト) | `{"version":"1.2.3"}` を pipe | shortcut → `1.2.4` | OK (TestRun_StdinPipe) |
| 非存在 `package.json` + `--write` | `{"version":"1.2.3"}` を pipe | --write incompatible エラー | OK (TestRun_StdinPipeWriteRejected) |

回帰テスト: `src/stdin_pipe_existing_file_test.go`
(`TestRun_ExistingFile_EmptyPipeFallsBackToDisk`,
`TestRun_WorkspaceRootFixture_EmptyPipeFallsBackToDisk`,
`TestRun_ExistingFile_NonEmptyPipeWins`,
`TestRun_MissingFile_EmptyPipeErrorsWithHint`)。
fixture: `tests/fixtures/workspace-root/Cargo.toml`。

## 診断オプションについての所見 (将来 TODO, 優先度低)

旧 issue の `--debug-rules` 案は今回のバグには直接効かなかった (rule 解決でなく stdin 経路で
空内容になるため)。より有効なのは「**読んだ content のバイト数 + 入力解決経路 (file / stdin-pipe /
vcs)**」を 1 行出す軽量トレース (`-vv` 相当)。`resolved input0 from <stdin-pipe|file>, N bytes`
だけで同種問題の切り分けが一瞬になる。フル `--debug-rules` より安価。
