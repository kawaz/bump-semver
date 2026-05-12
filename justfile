# bump-semver
#
# kawaz/* リポの共通テンプレ候補。基本は jj 統一前提 (git bare + jj workspace)、
# main / origin / src/ 等のハードコードは kawaz スタイルに揃えてある。
# 各リポで上書きするのは bump-trigger-paths くらい。
# git only リポへ流用する場合は push / bump-version の jj 呼び出しを書き換える必要あり。
# ---------- settings ----------

set unstable
set guards
set lazy
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set script-interpreter := ["bash", "-eu", "-o", "pipefail"]

# ---------- variables ----------
# jj/git 判定。`jj root` / `git rev-parse` を使って repository を**親方向に探索**して
# 判定する (path_exists は cwd 直下しか見ないので jj workspace 内では false になる罠)。
# shell() で「true"/"false" の文字列を生成し、bash の `if {{ is-jj }}; then` で
# bash builtin true/false コマンドとして評価できる形に揃える。
# jj/git 両方が並存するときは jj 優先で git は false に倒す (git bare + .jj 構成と整合)。

is-jj := shell('jj root >/dev/null 2>&1 && echo true || echo false')
is-git := if is-jj == "true" { "false" } else { shell('git rev-parse --git-dir >/dev/null 2>&1 && echo true || echo false') }

# bump-version トリガとなる product code パス (テンプレ流用時に各リポで上書き)。
# docs/ や *.md だけの変更なら VERSION bump 不要。

bump-trigger-paths := "src/"

# ---------- main entries (宣言順 = 利用頻度の高い順) ----------

# default is list
default: list

# list recipes (order matters)
list:
    @just --list --unsorted

# build + run
run *ARGS: build
    ./bin/bump-semver {{ ARGS }}

# go test
test *ARGS='./...': lint
    go test {{ ARGS }}

# push with some checks
push: ensure-clean test check-translations check-version-bumped
    jj bookmark set main -r @-
    jj git push --bookmark main

# bump version file(s) for Release
bump-version level="patch": ensure-clean
    new_version=$(bump-semver {{ level }} VERSION --write --no-hint) && jj commit -m "Release v${new_version}"

# ---------- atomic dev recipes ----------

# build (host target)
build: lint
    # -buildvcs=false: jj+git-bare で VCS スタンプ取得が失敗する回避策
    go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=v$(cat VERSION)" -o bin/bump-semver ./src

# format + lint (with auto-fix)
lint:
    gofmt -w .
    go vet ./...
    just --fmt --check --unstable

# lint + build + test
ci: lint test build

# ---------- meta / checks (default 経由 / push 内部、直接叩くことはほぼない) ----------

# working copy is clean
ensure-clean: lint
    if {{ is-jj }}; then [ "$(jj log -r @ --no-graph -T 'empty')" = "true" ]; fi
    if {{ is-git }}; then [ -z "$(git status --porcelain)" ]; fi

# translation pair (NAME-ja.md / NAME.md) integrity
check-translations: ensure-clean (_check-translation "README") (_check-translation "docs/DESIGN") (_check-translation "docs/MANUAL")

# fail if product code changed but version file(s) not bumped
[script]
check-version-bumped:
    # 変更なしなら早期 return (success)
    if {{ is-jj }}; then
        # jj diff の exit code は常に 0 だが、main@origin 未 track 等の失敗は伝播させる必要がある
        diff_out=$(jj diff -r 'main@origin..@' --summary -- {{ bump-trigger-paths }}) || { echo 'ERROR: jj diff failed (main@origin not tracked? run jj git fetch first)' >&2; exit 1; }
        [ -z "$diff_out" ] && exit 0
    elif {{ is-git }}; then
        git diff --quiet origin/main -- {{ bump-trigger-paths }} && exit 0
    fi
    # バージョン更新済みだったなら return (success)
    bump-semver compare gt VERSION vcs:origin/main --no-hint && exit 0
    echo 'ERROR: code changed but VERSION not bumped; run "just bump-version"' >&2; exit 1

# ---------- private helpers ----------

# `?` sigil (set guards := true) で -ja.md 不在時は recipe 全体を success として早期 return
_check-translation name:
    ?test -f {{ name }}-ja.md
    test -f {{ name }}.md
    head -5 {{ name }}-ja.md | grep -qF "> [English](./{{ file_name(name) }}.md) | 日本語"
    head -5 {{ name }}.md    | grep -qF "> English | [日本語](./{{ file_name(name) }}-ja.md)"
    test "$(just _file-ts {{ name }}-ja.md)" -le "$(just _file-ts {{ name }}.md)"

# file の最終 commit timestamp (jj/git 自動切替、stdout に epoch 秒)
[script]
_file-ts file:
    if {{ is-jj }}; then
        jj log --no-graph -T 'committer.timestamp().format("%s")' -r "latest(::@ & files('{{ file }}'))" 2>/dev/null || echo 0
    elif {{ is-git }}; then
        git log -1 --format=%ct -- {{ file }} 2>/dev/null || echo 0
    else
        echo 0
    fi
