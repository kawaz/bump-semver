# bump-semver Justfile
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

# go test
test: lint
    go test ./...

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
    bump-semver vcs is clean || { echo "working tree is dirty" >&2; exit 1; }

# fail if src/ changed since origin/main but VERSION was not bumped
check-version-bumped:
    #!/usr/bin/env bash
    set -euo pipefail
    compare_ref='main@origin'
    if bump-semver vcs diff -q "$compare_ref" -- src/; then
      exit 0
    fi
    if ! bump-semver compare gt VERSION "vcs:${compare_ref}:VERSION" --no-hint; then
      echo "ERROR: VERSION not bumped since $compare_ref" >&2
      echo "       run: just bump-version" >&2
      exit 1
    fi

# fail if VERSION is not greater than the latest semver tag (local release pre-check)
check-against-latest-release:
    #!/usr/bin/env bash
    set -euo pipefail
    bump-semver vcs fetch tags
    latest=$(bump-semver get 'vcs:latest-tag()' --no-hint)
    if ! bump-semver compare gt VERSION "$latest" --no-hint; then
      echo "ERROR: VERSION ($(cat VERSION)) not greater than latest tag ($latest)" >&2
      exit 1
    fi

# translation pair check (commit-lag + bilingual links) via pkf shim
check-translations:
    pkf run docs:check-translations

# ---------- release flow ----------

# bump VERSION (default: patch) and create a release commit
bump-version level="patch": ensure-clean
    #!/usr/bin/env bash
    set -euo pipefail
    new_version=$(bump-semver "{{level}}" VERSION --write --no-hint)
    bump-semver vcs commit -m "Release v${new_version}" VERSION

# push to origin/main with gates: ci + check-translations + check-version-bumped
push: ci check-translations check-version-bumped
    #!/usr/bin/env bash
    set -euo pipefail
    # Dogfood DR-0020 PR-5.2: --jj-bookmark-auto-advance handles the jj
    # bookmark advance (clean → @-, dirty → @) inside `vcs push` itself.
    # The flag is jj-specific (exit 2 on git by design), so branch on
    # `vcs is jj` rather than passing it unconditionally.
    if bump-semver vcs is jj; then
      bump-semver vcs push --branch main --jj-bookmark-auto-advance
    else
      bump-semver vcs push --branch main
    fi

# ---------- utility passthroughs ----------

# semver compare via bump-semver (e.g. `just semver-compare gt 1.2.3 1.2.4`)
semver-compare *ARGS:
    bump-semver compare {{ARGS}}

# display VERSION + binary --version output
version:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "VERSION: $(cat VERSION)"
    if [ -x ./bin/bump-semver ]; then
      echo "binary: $(./bin/bump-semver --version)"
    fi
