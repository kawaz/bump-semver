# bump-semver

# デフォルト: レシピ一覧
default:
    @just --list

# format + lint (auto-fix 込み、残った警告はエラー)
lint:
    gofmt -w .
    go vet ./...

# テスト
test: lint
    go test ./...

# release ビルド (ローカル動作確認用、host target)
# `-buildvcs=false` は jj+git-bare 構成で go の VCS スタンプ取得が失敗する回避策
# (port-peeker と同じ理由)
build: lint
    go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=v$(cat VERSION)" -o bin/bump-semver ./src

# ビルドして実行
run *ARGS: build
    ./bin/bump-semver {{ARGS}}

# CI で呼ぶ単一エントリ (lint→test→build を依存重複排除で1回ずつ)
ci: lint test build

# ワーキングコピーがクリーン (empty change) であることを確認
# `lint` を依存に取ることで auto-fix で生じた変更を確実に検出する
# (just は依存重複を排除するので lint は1回だけ走る)
ensure-clean: lint
    test "$(jj log -r @ --no-graph -T 'empty')" = "true"

# 翻訳ペア (*-ja.md / *.md) の整合性チェック
# テンプレ: ~/.claude/rules/docs-structure.md の「check-translations の実装」セクション
check-translations: ensure-clean
    #!/usr/bin/env bash
    set -euo pipefail
    die() { echo "$*" >&2; exit 1; }

    file_ts() {
        local f="$1"
        if [ -d .jj ]; then
            jj log --no-graph -T 'committer.timestamp().format("%s")' \
                -r "latest(::@ & files('$f'))" 2>/dev/null || echo 0
        else
            git log -1 --format=%ct -- "$f" 2>/dev/null || echo 0
        fi
    }

    while IFS= read -r ja; do
        en="${ja/-ja/}"
        [ -f "$en" ] || die "ERROR: $ja exists but $en is missing"
        head -5 "$ja" | grep -qF "> [English](./${en##*/}) | 日本語" \
            || die "ERROR: $ja: missing '> [English](./${en##*/}) | 日本語' link near the top"
        head -5 "$en" | grep -qF "> English | [日本語](./${ja##*/})" \
            || die "ERROR: $en: missing '> English | [日本語](./${ja##*/})' link near the top"
        ja_ts=$(file_ts "$ja")
        en_ts=$(file_ts "$en")
        [ "$ja_ts" -le "$en_ts" ] \
            || die "ERROR: $ja was updated after $en. Update the English translation before pushing."
    done < <(find . -name '*-ja.md' -not -path './.git/*' -not -path './.jj/*')

# VERSION ファイルが origin/main から退化していないことを確認 (push 前 sanity check)
# bump-semver 自身を使った dogfooding (./bin/bump-semver はビルド済を参照)
# - VERSION > origin/main: OK (bump 含む新規変更)
# - VERSION == origin/main: OK (VERSION 以外の変更で push する時)
# - VERSION < origin/main: ERROR (退化、想定外)
# git fetch は走らせない (利用者責任、現時点の手元の origin/main を見る)
check-version-not-regressed: build
    #!/usr/bin/env bash
    set -euo pipefail
    set +e
    ./bin/bump-semver compare lt VERSION vcs:origin/main --no-hint
    cmp_exit=$?
    set -e
    if [ "$cmp_exit" = "0" ]; then
        echo "ERROR: VERSION is regressed below origin/main; aborting push" >&2
        ./bin/bump-semver get VERSION vcs:origin/main || true
        exit 1
    fi
    # exit 1 (>=) または 2 (no origin/main yet) は許容

# push (依存階層で lint/ensure-clean は重複排除されて1回ずつ実行)
push: ensure-clean test check-translations check-version-not-regressed
    jj bookmark set main -r @-
    jj git push --bookmark main

# VERSION を bump して Release commit を push (CI が tag + GitHub Release を作成)
# bump-semver 自身を使った dogfooding (./bin/bump-semver はビルド済を参照)
bump-version bump="patch": ensure-clean test build
    #!/usr/bin/env bash
    set -euo pipefail

    # VERSION ファイルの変更が main に push されると release.yml が検出して
    # tag (v$VERSION) と GitHub Releases (--generate-notes) を自動作成する。

    current=$(cat VERSION | tr -d '[:space:]')
    new_version=$(./bin/bump-semver {{bump}} VERSION --write --no-hint)
    echo "Version: ${current} -> ${new_version}"

    # @ は空 change (ensure-clean で確認済)。VERSION を書き換えて Release commit に
    jj describe -m "Release v${new_version}"
    jj new

    # push (release.yml がここから走る)
    just push

    # release.yml を watch
    sleep 3
    run_id=$(gh run list --repo kawaz/bump-semver --workflow=release.yml --limit 1 --json databaseId -q '.[0].databaseId')
    gh run watch "$run_id" --repo kawaz/bump-semver
