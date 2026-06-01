# bump-semver justfile
#
# Canonical task runner. Recipes are intentionally simple — VCS-shaped
# operations (commit/push/clean check/diff) delegate to `bump-semver vcs`
# subcommands so the project dogfoods its own DR-0020 design.
#
# Translation-pair check (`check-translations`) still routes through `pkf run`
# because the underlying logic lives in kawaz/pkf-tasks; a slim Taskfile.pkl
# is kept solely as a shim host. A standalone CLI for translation checks is a
# separate follow-up.
#
# Declaration order is intentional: most-used recipes first so `just --list`
# (and `default`) surface them prominently.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Paths that, when changed since origin/main, require a VERSION bump
# before push (consumed by check-version-bumped). Literal git pathspec:
# `src/` catches all tracked changes under src/ — including deletions
# and dotfiles. `glob:` was tried here but its filesystem-side semantics
# (no dotfiles by default, gitignored excluded, deletions invisible
# because the file isn't on disk) silently weakens this release gate;
# the glob: dogfood belongs in a use case that genuinely wants
# filesystem expansion, not in a git-pathspec slot.
bump-trigger-paths := "src/"

# show the recipe list (default)
default:
    @just --list --unsorted

# ---------- atomic (lint / test / build) ----------

# gofmt + go vet
lint-go:
    gofmt -w .
    go vet ./...

# pkl format -w (Taskfile.pkl, PklProject*)
lint-pkl:
    pkl format -w .

# lint-go + lint-pkl
lint: lint-go lint-pkl

# go test (ARGS default to ./..., override e.g. `just test ./src/handler_cargo`)
test *ARGS='./...': lint
    go test {{ARGS}}

# build host target -> bin/bump-semver
build: lint
    go build -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.version=v$(cat VERSION)" \
      -o bin/bump-semver ./src

# lint + test + build (CI entry point)
ci: lint test build

# ---------- gates (push の内部、利用者が直接叩くことほぼなし) ----------

# working copy is clean (dogfood: bump-semver vcs is clean)
ensure-clean:
    bump-semver vcs is clean

# fail if bump-trigger-paths changed since origin/main but VERSION was not bumped
check-version-bumped:
    if ! bump-semver vcs diff -q main@origin -- {{bump-trigger-paths}}; then bump-semver compare gt VERSION vcs:main@origin; fi

# fail if VERSION is not greater than the latest release (origin/main の VERSION)
check-against-latest-release:
    bump-semver compare gt VERSION vcs:origin/main

# translation pair check (commit-lag + bilingual links) via pkf shim
check-translations:
    pkf run docs:check-translations

# ---------- release flow ----------

# bump VERSION (default: patch) and create a release commit
bump-version level="patch": ensure-clean
    bump-semver "{{level}}" VERSION --write --quiet
    bump-semver vcs commit -m "Release v$(bump-semver get VERSION)" VERSION

# push to origin/main with gates: ci + check-translations + check-version-bumped
push: ci check-translations check-version-bumped
    bump-semver vcs push --branch main --jj-bookmark-auto-advance

# ---------- utility ----------

# display VERSION + binary --version output
version:
    echo "VERSION: $(cat VERSION)"
    if [ -x ./bin/bump-semver ]; then echo "binary: $(./bin/bump-semver --version)"; fi
