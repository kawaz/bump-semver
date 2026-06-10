# issue: `get -q` / `-qq` が値 (stdout) まで抑制し、ガード用途で空文字列になる

- Date: 2026-06-10
- Status: open
- 発見元: kawaz/cache-warden の justfile 整備 (2026-06-10 dogfooding)

## 現象

利用側の justfile で、バージョンを変数に受け取ってガード判定に使う慣用パターン:

```bash
ref=$(bump-semver get vcs:main@origin:Cargo.toml -qq 2>/dev/null)
bump-semver compare gt Cargo.toml "$ref"
```

このとき `-qq` を付けると `$ref` が空文字列になり、後続の compare が
`"" is neither a file nor a valid version` で exit 2 となる。
exit code 自体は 0 なので「成功したが空」という状態になりガードが破綻する。

## 再現 (2026-06-10 実機確認)

```
$ bump-semver get Cargo.toml
0.1.1

$ bump-semver get Cargo.toml -qq
(空出力)

$ bump-semver get Cargo.toml -q
(空出力)
```

## 現仕様の確認

`src/cli_types.go` によるフラグの suppressionレベル定義:

| フラグ | SuppressHint | SuppressStdout | SuppressError |
|---|---|---|---|
| (なし) | false | false | false |
| `--no-hint` | true | false | false |
| `-q` / `--quiet` | true | **true** | false |
| `-qq` / `--quiet-all` | true | **true** | **true** |

`ShouldSuppressStdout()` は `-q` 以上で true となり、`get` の stdout 値出力
(`fmt.Fprintln(stdout, newV.String())`) もこれで制御されているため、
`-q` / `-qq` で値が消える。

## 論点

`get` は「バージョン値そのものが成果物」のサブコマンド。`bump` / `compare` と異なり
stdout が primary output であり、stdout を黙らせることに実用上の需要がない。

- `-q` / `-qq` の本来の動機は **hints/警告 (stderr) を機械利用時に邪魔にならないよう抑制すること**
  であると思われるが、現仕様では `get` の値も一緒に消える
- `get ... -qq 2>/dev/null` のような「stderr を捨てつつ値だけ取る」イディオムが
  意図通りに機能しない (= 機械利用で最も踏みやすいパターンで壊れる)
- `--no-hint` を使えば stdout は維持されるが、stderr の error line も残るため
  `2>/dev/null` と組み合わせないと CI ログが汚れる

## 利用側のワークアラウンド (対応済み)

`-qq` を外し `--no-hint` + `2>/dev/null` + 空チェックで対応:

```bash
ref=$(bump-semver get vcs:main@origin:Cargo.toml --no-hint 2>/dev/null)
[ -n "$ref" ] || { echo "skip: could not get ref version" >&2; exit 0; }
bump-semver compare gt Cargo.toml "$ref"
```

## 提案 (どちらか)

(a) **`get` では `-q` / `-qq` でも stdout 値出力を維持する** (推奨)
    - 理由: `get` の stdout は primary output であり、suppress する動機がない
    - 実装イメージ: `ShouldSuppressStdout()` の判定を `get` action では skip するか、
      `get` 専用の verbosity path を設ける

(b) **現挙動を仕様として明記する** (次善策)
    - `--help` の `-q` / `-qq` 説明に「`get` では値 (stdout) も消えます」と明記
    - エラーメッセージに「get で値を機械取得したい場合は --no-hint を使ってください」のヒント追加
    - 仕様明記でもガードのような機械利用で踏みやすい罠は残るため、(a) の方が望ましい
