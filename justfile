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

# gofmt + go vet
[private]
lint-go:
    gofmt -w .
    go vet ./...

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

# fail if bump-trigger-paths changed since origin/main but VERSION was not bumped
# (DR-0033 dogfood: test 専用の追加では VERSION bump を要求しない)
check-version-bumped: (_check-version-bumped "src/" "go.mod" "go.sum")

# (helper) diff があれば VERSION が origin/main より上がっているか検証
# `--excludes glob:src/**/*_test.go` で test 専用変更を bump-trigger から除外
# (= DR-0033、literal `src/` は内部で glob:src/**/* 扱いになるので exclude が効く)
[private]
[script]
_check-version-bumped *target_paths:
    if ! bump-semver vcs diff -q main@origin -- "$@" --excludes 'glob:src/**/*_test.go'; then
        bump-semver compare gt VERSION vcs:main@origin
    fi

# fail if VERSION is not greater than the latest release (origin/main の VERSION)
[private]
check-against-latest-release:
    bump-semver compare gt VERSION vcs:origin/main

# translation pair freshness check via `bump-semver vcs outdated`
[private]
check-outdated-translations: ensure-clean
    bump-semver vcs outdated 'glob:**/*-ja.md' '$1/$2.md'

# ---------- release flow ----------

# bump VERSION (default: patch) and create a release commit
bump-version level="patch": ensure-clean
    bump-semver "$1" VERSION --write --quiet
    bump-semver vcs commit -m "Release v$(bump-semver get VERSION)" VERSION

# push to origin/main with gates
push: ci check-outdated-translations check-version-bumped
    bump-semver vcs push --branch main --jj-bookmark-auto-advance
    @echo "[hint] gh-monitor:watch-workflow --sha $(bump-semver vcs is jj && jj log -r main --no-graph -T 'commit_id' || git rev-parse main) --on-success release.yml 'just on-success-release' kawaz/bump-semver"

# release.yml workflow が success になった時に AI が実行する action
# (watch-workflow の `--on-success release.yml 'just on-success-release'` 経由で
# 通知 event に `[ACTION:release.yml] just on-success-release` が emit される)
on-success-release:
    brew upgrade kawaz/tap/bump-semver
    bump-semver --version

# ---------- utility ----------

# display VERSION + binary --version output
version:
    echo "VERSION: $(cat VERSION)"
    if [ -x ./bin/bump-semver ]; then echo "binary: $(./bin/bump-semver --version)"; fi
    if command -v bump-semver >/dev/null 2>&1; then echo "local binary: $(bump-semver --version)"; fi
