# bump-semver
#
# kawaz/* リポの共通テンプレ候補。基本は jj 統一前提 (git bare + jj workspace)、
# main / origin / src/ 等のハードコードは kawaz スタイルに揃えてある。
# 各リポで上書きするのは bump-trigger-paths くらい。
# git only リポへ流用する場合は push / bump-version の jj 呼び出しを書き換える必要あり。
# ---------- settings ----------

set unstable := true
set guards := true
set lazy := true
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set script-interpreter := ["bash", "-eu", "-o", "pipefail"]

# ---------- variables ----------
# jj/git 判定 (path_exists は "true"/"false" 文字列を返すので、bash の `if {{ is-jj }}; then` で
# bash builtin true/false コマンドとして評価できる)。jj/git 両方が並存するときは jj 優先で
# git は false に倒す (kawaz の git bare + .jj 構成と整合)。

is-jj := path_exists('.jj')
is-git := if is-jj == "true" { "false" } else { path_exists('.git') }

# bump-version トリガとなる product code パス (テンプレ流用時に各リポで上書き)。
# docs/ や *.md だけの変更なら VERSION bump 不要。

bump-trigger-paths := "src/"

# ---------- default ----------

# レシピ一覧を表示
default:
    @just --list

# ---------- main entries (利用者が直接叩く) ----------

# push (release.yml が VERSION 変更を検出して tag + GitHub Release を作成)
push: ensure-clean test check-translations check-version-bumped
    jj bookmark set main -r @-
    jj git push --bookmark main

# VERSION を bump して Release commit を作成 (push は別途 `just push`)
bump-version bump="patch": ensure-clean
    new_version=$(bump-semver {{ bump }} VERSION --write --no-hint) && jj commit -m "Release v${new_version}"

# CI 単一エントリ (lint→test→build を依存重複排除で1回ずつ保証)
ci: lint test build

# ---------- dev recipes (push/ci の依存、利用者が直接叩くこともある) ----------

# format + lint (auto-fix 込み、残った警告はエラー、justfile も canonical 確認)
lint:
    # 注: gofmt -w が dirty を生むと ensure-clean が落ちる → push 前に commit して再実行する想定
    gofmt -w .
    go vet ./...
    just --fmt --check --unstable

# テスト
test: lint
    go test ./...

# release ビルド (ローカル動作確認用、host target)
build: lint
    # `-buildvcs=false` は jj+git-bare 構成で go の VCS スタンプ取得が失敗する回避策 (port-peeker と同じ理由)
    go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=v$(cat VERSION)" -o bin/bump-semver ./src

# ビルドして実行
run *ARGS: build
    ./bin/bump-semver {{ ARGS }}

# ---------- check recipes (push の sanity 検証、基本は push 経由でしか叩かない) ----------

# ワーキングコピーがクリーン (jj は @ が empty、git は porcelain 空)
ensure-clean: lint
    if {{ is-jj }}; then [ "$(jj log -r @ --no-graph -T 'empty')" = "true" ]; fi
    if {{ is-git }}; then [ -z "$(git status --porcelain)" ]; fi

# 翻訳ペア (NAME-ja.md / NAME.md) の整合性チェック (対象は明示列挙、-ja.md 不在ならスキップ)
check-translations: ensure-clean (_check-translation "README") (_check-translation "docs/DESIGN") (_check-translation "docs/MANUAL")

# 翻訳ペア NAME-ja.md / NAME.md の存在 + 相互リンク + timestamp 順序確認

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

# product code に変更があれば VERSION も origin/main より bump 済 (変更なしならスキップ)

# bump-semver compare gt VERSION vcs:origin/main: ローカル VERSION > origin/main の VERSION なら exit 0
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
