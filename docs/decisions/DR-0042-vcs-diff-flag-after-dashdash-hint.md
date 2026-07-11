# DR-0042: `vcs diff` の `--` 以後に置かれた flag 風 positional への stderr 警告

- Status: Accepted (2026-07-11)
- Date: 2026-07-11
- Related: DR-0020 (vcs-subcommands), `docs/issue/2026-06-29-vcs-diff-excludes-position-after-double-dash-ignored.md`

## Context

`vcs diff -q REV -- "$@" --excludes 'glob:...'` のように `--excludes` を `--` の後ろに
置くと、`--` の標準セマンティクス (以後は全て positional) により flag として解釈されず
PATH 扱いになる。実害は二段階で悪質:

1. `--excludes` そのものは存在しないパスとして黙って無視される (declarative convergence)
2. 続く `glob:` パターンが **include set に加わる** — 除外したかったファイルだけが
   diff に現れる意味論の反転

実機再現: `vcs diff -s HEAD -- 'glob:*.txt' --excludes 'glob:*_test.txt'` は
`b_test.txt` を **含めて** 出力する (exclude が include に反転)。

## Decision

### 不採用: `--` 以後の flag 解釈 (案 1)

`--` は POSIX 慣習で「以後を positional に固定する」契約。その後ろで flag を解釈するのは
契約自体の破壊で、flag 風の名前を持つパスを渡す口が無くなる。

### 不採用: `--` 以後の flag 風 token を hard error (案 3)

同上 — `--` の存在意義 (flag 風パスの escape hatch) を潰す。exit code 契約にも影響する。

### 採用: help 明文化 + stderr 警告 (案 2 + hint)

- help (`vcs diff` の Notes) に「flag は `--` の前に置く。`--` 以後は全て PATH」を明記
- positional PATH が `-` 始まりのとき stderr に警告を出す:
  stdout / exit code は一切変えない (スクリプト契約不変)。警告は cause + remedy 形式
  (interface-wording) で「`--` の前に移せ」を示す
- cobra の仕様上、`-` 始まりの token が positional に到達するのは `--` 以後のみ
  (前なら flag parse error になる) ので、警告の誤爆は「本当に `-` 始まりのパスを
  渡したい」pathological なケースに限られる。その場合も動作は変わらず stderr が
  1 行増えるだけ

### スコープ

本 DR は `vcs diff` に限定 (issue のアンカー)。`vcs commit` 等の PATH を取る他 verb への
展開は同じ警告 helper を使えるが、必要が観測されてから。

## Consequences

- 誤用は初回実行の stderr で即発覚する (無音の exclude 反転が消える)
- `-q` / `-qq` でも stderr 警告は出る (誤用検出が目的のため verbosity で抑制しない —
  `-qq` はエラーを抑制する契約だが、警告は「これから間違った答えを返す」予告であり
  エラー出力とは責務が異なる)
- 既存の正当な利用 (flag を `--` の前に置く) には無影響
