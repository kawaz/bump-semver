# bump-semver justfile
#
# Canonical task runner. Recipes are intentionally simple — VCS-shaped
# operations (commit/push/clean check/diff) and the translation-pair
# freshness check delegate to `bump-semver vcs` subcommands so the project
# dogfoods its own DR-0020 / DR-0027 / DR-0028 design.
#
# Declaration order is intentional: most-used recipes first so `just --list`
# (and `default`) surface them prominently.

set shell := ["bash", "-euo", "pipefail", "-c"]

set script-interpreter := ["bash", "-euo", "pipefail"]

set positional-arguments

# default behaviour: alias for `list`
default: list

# show the recipe list
list:
    @just --list --unsorted

# ---------- atomic (lint / test / build) ----------

# gofmt + go vet + go.mod/go.sum tidy check
[private]
lint-go:
    gofmt -w .
    go vet ./...
    go mod tidy -diff

# just --fmt (justfile self-format check)
[private]
lint-just:
    just --unstable --fmt --check

# lint-go + lint-just
lint: lint-go lint-just

# go test (ARGS default to ./..., override e.g. `just test ./src/handler_cargo`)
test *ARGS='./...': lint
    go test "$@"

# build host target -> bin/bump-semver
build: lint
    go build -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.version=v$(cat VERSION)" \
      -o bin/bump-semver ./src

# build then run the local binary, forwarding all args (e.g. `just run vcs outdated --help`)
run *ARGS: build
    ./bin/bump-semver "$@"

# lint + test + build (CI entry point)
ci: lint test build

# ---------- gates (push の内部、利用者が直接叩くことほぼなし) ----------

# working copy is clean (dogfood: bump-semver vcs is clean)
[private]
ensure-clean:
    bump-semver vcs is clean

# fail if the current bookmark / branch is not the default. DR-0038 で確定:
# IsWorktree でなく on-default-branch を gate にする (= jj 運用で main
# workspace が secondary worktree でも push を妨げない)。
[private]
check-on-default-branch:
    bump-semver vcs is on-default-branch

# 現在の worktree を default branch (= origin/<default>) に rebase (DR-0038)
sync:
    bump-semver vcs sync --onto $(bump-semver vcs get default-branch)@origin

# secondary worktree で作業した change を default branch に合流させる時、sync の後で default branch bookmark/ref を @ に forward (ref を動かすだけで push はしない、push は `just push` で)。
promote:
    bump-semver vcs promote

# fail if bump-trigger-paths changed since default-branch@origin but VERSION was not bumped
# (DR-0033 dogfood: test 専用の追加では VERSION bump を要求しない)
check-version-bumped: (_check-version-bumped "src/" "go.mod" "go.sum")

# (helper) diff があれば VERSION が default-branch@origin より上がっているか検証
# `--excludes glob:src/**/*_test.go` で test 専用変更を bump-trigger から除外
# (= DR-0033、literal `src/` は内部で glob:src/**/* 扱いになるので exclude が効く)
[private]
[script]
_check-version-bumped *target_paths:
    bn=$(bump-semver vcs get default-branch)
    if ! bump-semver vcs diff -q "${bn}@origin" -- "$@" --excludes 'glob:src/**/*_test.go'; then
        bump-semver compare gt VERSION "vcs:${bn}@origin"
    fi

# fail if VERSION is not greater than the latest release (default-branch@origin の VERSION)
[private]
[script]
check-against-latest-release:
    bn=$(bump-semver vcs get default-branch)
    bump-semver compare gt VERSION "vcs:${bn}@origin"

# translation pair freshness check via `bump-semver vcs outdated`
[private]
check-outdated-translations: ensure-clean
    bump-semver vcs outdated 'glob:**/*-ja.md' '$1/$2.md'

# ---------- release flow ----------

# bump VERSION (default: patch) and create a release commit
bump-version level="patch": ensure-clean
    bump-semver "$1" VERSION --write --quiet
    bump-semver vcs commit -m "Release v$(bump-semver get VERSION)" VERSION

# push default branch to origin with full release gates (release 専用; check-on-default-branch + ci + outdated-translations + version-bumped)
push: check-on-default-branch ci check-outdated-translations check-version-bumped
    bump-semver vcs push --branch "$(bump-semver vcs get default-branch)" --jj-bookmark-auto-advance
    cmux-msg notify --self --text "Monitor で 'just watch' を起動して" 2>/dev/null || true

# release.yml の完了を SHA-pinned watch (push 後の `cmux-msg notify --self` で AI に起動指示が届く)
watch:
    watch-workflow.sh --sha $(bump-semver vcs get commit-id --rev "$(bump-semver vcs get default-branch)") --on-success release.yml 'just on-success-release' kawaz/bump-semver

# push the current feature bookmark/branch to origin (feature 用; ensure-clean + ci のみ、release ゲート無し)
[script]
push-wip: ensure-clean ci
    # bookmark 決定:
    # - ユニークな current-branch → 採用
    # - jj で bookmark 不在 / ambiguous → workspace 名で auto-set (default-branch と衝突なら fail)
    # - git detached HEAD → fail (vcs サブコマンドの stderr に任せる)
    bn=$(bump-semver vcs get default-branch)
    if cur=$(bump-semver vcs get current-branch 2>/dev/null); then
        target="$cur"
    else
        rc=$?
        if [ "$rc" -eq 4 ] && bump-semver vcs is jj; then
            # jj: ambiguous or 不在 → workspace 名を採用
            ws=$(bump-semver vcs get worktree-name)
            [ -n "$ws" ] && [ "$ws" != "$bn" ] || exit 1
            bump-semver vcs bookmark set "$ws" -r @
            target="$ws"
        else
            exit "$rc"
        fi
    fi
    # default branch を push しようとしているなら release 経路 (just push) へ
    [ "$target" != "$bn" ] || exit 1
    bump-semver vcs push --branch "$target" --jj-bookmark-auto-advance

# release.yml workflow が success になった時に AI が実行する action
# (watch-workflow の `--on-success release.yml 'just on-success-release'` 経由で
# 通知 event に `[ACTION:release.yml] just on-success-release` が emit される)
on-success-release:
    # tap repo を直接 git pull (= `brew update` 全 tap 巡回より速い)
    git -C "$(brew --repository)/Library/Taps/kawaz/homebrew-tap" pull --ff-only
    brew upgrade kawaz/tap/bump-semver
    bump-semver --version

# ---------- utility ----------

# display VERSION + binary --version output
version:
    echo "VERSION: $(cat VERSION)"
    if [ -x ./bin/bump-semver ]; then echo "binary: $(./bin/bump-semver --version)"; fi
    if command -v bump-semver >/dev/null 2>&1; then echo "local binary: $(bump-semver --version)"; fi
