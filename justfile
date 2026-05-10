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

# ワーキングコピーがクリーンであることを確認 (jj / git 両対応)
# `lint` を依存に取ることで auto-fix で生じた変更を確実に検出する
ensure-clean: lint
    [ ! -d .jj ] || test "$(jj log -r @ --no-graph -T 'empty')" = "true"
    [   -d .jj ] || [ -z "$(git status --porcelain)" ]

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

# VERSION が origin/main より退化していないこと (push 前 sanity check、dogfooding)
# compare lt: exit 0 = 退化(NG)、exit 1 = OK (>=)、exit 2 = origin/main 不在 (許容)
# fetch は走らせない (利用者責任)
check-version-not-regressed: build
    ! ./bin/bump-semver compare lt VERSION vcs:origin/main --no-hint

# push (依存階層で lint/ensure-clean は重複排除されて1回ずつ実行)
push: ensure-clean test check-translations check-version-not-regressed
    jj bookmark set main -r @-
    jj git push --bookmark main

# VERSION を bump して Release commit を作成 (bump-semver 自身を使う dogfooding)
# push はレシピに含めない: 確認後に `just push` を別途実行する
bump-version bump="patch": ensure-clean test build
    new_version=$(./bin/bump-semver {{bump}} VERSION --write --no-hint) && jj describe -m "Release v${new_version}" && jj new
