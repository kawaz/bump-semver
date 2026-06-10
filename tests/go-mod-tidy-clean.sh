#!/usr/bin/env bash
# go.mod / go.sum が `go mod tidy` 済みであることを保証する回帰テスト。
#
# 誤った `// indirect` マーク (直接 import なのに indirect 扱い) や、
# 欠落した module hash を検出する。`go mod tidy -diff` (Go 1.23+) は差分が
# あれば非ゼロ終了するので、それをそのまま gate にする。
#
# justfile の lint-go レシピにも同等の `go mod tidy -diff` を組み込んでいるが、
# 本スクリプトは「テストとして単体で叩ける」形を提供する (tests/ci-jj-runs.sh と対)。
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if go mod tidy -diff; then
	echo "PASS: go.mod / go.sum is tidy"
	exit 0
else
	echo "FAIL: go.mod / go.sum is not tidy. Run 'go mod tidy' and commit the result." >&2
	exit 1
fi
