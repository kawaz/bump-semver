# DR-0035: `--write` の二相化 + アトミック書き込みによる manifest 破損防止 (C-2)

- Status: Active
- Date: 2026-06-11
- Related: DR-0004 §7 (複数 file 書き戻しの挙動 / rollback 不採用) を **部分 supersede**

## Context

`--write` で複数 file (例: `Cargo.toml` + `package.json`) を一括 bump する際、
旧実装は 1 つのループ内で「Replace を計算 → そのまま `os.WriteFile` で直書き」を
file ごとに逐次実行していた (cli_dispatch.go の write ブロック)。

この構造には 2 つの破損リスクがあった:

1. **部分更新 (torn update across files)**: 2 つ目の file の Replace 計算が失敗すると、
   1 つ目の file は既に書き換わった状態で exit する。version 同期が前提の
   manifest 群 (DR-0004 の主要ユースケース) が不整合のまま残る。
2. **書きかけ file (torn file)**: `os.WriteFile` は対象 file を truncate してから
   書き込むため、書き込み中にプロセスが kill される / I/O エラーが起きると、
   中身が途中までの壊れた file が残りうる。

DR-0004 §7 は「rollback ロジックは持たない (jj/git で復元可能、複雑さを持ち込まない)」
と決定していた。本 DR はこの **rollback 不採用の判断は維持**したまま、別軸の
**prevention (破損の事前回避)** を導入して §7 を部分 supersede する。rollback と
prevention は別概念であり、「壊れたら戻す」を持たないことと「そもそも壊さない」ことは
両立する。

## Decision

### 1. 二相化 (compute → write、all-or-nothing)

`writeBumpedFiles` を 2 フェーズに分割する:

- **フェーズ 1 (compute)**: 全 file の Replace を先に全部実行し、新しい内容のバイト列を
  メモリ上に揃える。1 つでも Replace が失敗したら **何も書かずに**エラー終了する。
- **フェーズ 2 (write)**: フェーズ 1 が全件成功した後で初めて、各 file を実際に書き込む。

これにより「1 つ目だけ書き換わって 2 つ目の Replace 計算で落ちる」class の部分更新が
構造的に消える。

### 2. アトミック書き込み (temp + rename)

各 file の書き込みは `atomicWriteFile` で行う:

1. 対象と同一ディレクトリに temp file を作成 (`os.CreateTemp`)
2. 新内容を書き込み → close
3. 元 file の permission bits を temp に適用 (`os.Chmod`)
4. `os.Rename` で temp を対象に被せる

同一 filesystem 上の rename はアトミックなので、読み手が書きかけの中身を観測することは
なく、torn file が残らない。失敗経路では temp file を `defer` で確実に削除する。

### 3. symlink の実体解決

旧 `os.WriteFile` は symlink を辿って実体に書いていた。temp+rename を素朴に行うと
rename が symlink 自体を regular file に置換してしまうため、書き込み前に
`filepath.EvalSymlinks` で実体パスを解決し、実体側で temp+rename + chmod する。
これにより symlink は symlink のまま維持され、実体 file の内容と mode が更新される。

### 4. 残余リスク (= rollback 不採用 DR-0004 §7 維持の帰結)

フェーズ 2 (write) の途中で失敗した場合 (例: 2 つ目の rename が EACCES) は、
1 つ目の file が既に commit された状態が残りうる。これは **rollback を持たない方針
(DR-0004 §7 維持) の意図された帰結**であり、VCS でツリーを復元する前提で許容する。

フェーズ 1 で全 Replace を先に成功させてあるため、フェーズ 2 で失敗する原因は
「ディスク満杯 / 権限 / I/O エラー」等の環境要因に限られ、Replace ロジック起因の
部分更新は起きない (= 最も起きやすい破損 class は phase 1 で排除済み)。

## Consequences

- 複数 file の Replace 計算失敗で 1 つ目だけ書き換わる事故が起きなくなった
- プロセス kill / 書き込みエラーで書きかけ file が残らなくなった
- symlink 入力は symlink を維持したまま実体を更新する (旧挙動は実体直書きで symlink は
  維持されていたが、temp+rename 化で symlink 破壊を防ぐ実体解決が必要になった)
- permission bits は引き続き保持 (temp に chmod してから rename)
- 書き込み順 / stdout / JSON / hint 出力 / exit code 語彙はすべて不変
- 残余リスク: phase 2 の後段失敗による部分書き込み窓は残る (rollback 不採用の帰結)

## 不採用案

### A. rollback / undo を実装する

書き込み前に全 file の元内容を退避し、途中失敗で書き戻す案。DR-0004 §7 が明示的に
「VCS で復元できるので持ち込まない」と判断済み。prevention (本 DR) で最も起きやすい
破損 class は消えるため、複雑な rollback を持つ価値が低い。§7 の判断を維持する。

### B. 全 file を 1 つの temp ディレクトリに書いてから一括 rename

複数 file をまたぐ「全か無か」の write を実現できるが、file ごとに別ディレクトリ
(別 filesystem) にある場合 rename がアトミックにならず、結局 file 単位の rename に
分解される。複雑さに見合わない。file 単位 atomic + phase 分離で十分。

### C. fsync して耐クラッシュ性を上げる

rename 前に `tmp.Sync()` を呼びクラッシュ後も新内容を保証する案。bump-semver は
VCS 配下の manifest を書く用途で、電源断レベルの耐性は要件外。fsync は遅く、本 DR の
目的 (アプリ層の torn 回避) には不要。

## 関連

- 実装: `src/cli_dispatch.go` (`writeBumpedFiles` / `atomicWriteFile` / `pendingWrite`)
- テスト: `src/cli_write_atomic_test.go` (all-or-nothing / 正常 2 file / symlink 維持 + 実体更新 / symlink 経由 mode 保持)、`src/main_test.go` (`TestRun_FileWritePreservesMode` / `TestRun_MultiFile_AllSame` 既存)
- supersede 対象: DR-0004 §7 (write 時挙動。rollback 不採用は維持、prevention を追加)
