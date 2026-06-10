#!/usr/bin/env bash
# jj VCS バックエンドのテストが「jj があれば実走し、無ければ skip する」
# 双方向を保証する gate。
#
# CI に jj をインストールする目的は、jj バックエンド (DR-0020 / DR-0026 /
# DR-0027 / DR-0028) のテスト群を実際に走らせること。jj が PATH に無いと
# それらは t.Skip("git+jj fixture requires both binaries") で素通りし、緑に
# なってしまう。本スクリプトは:
#   a. jj present  -> skip メッセージが現れない (= 実走している)
#   b. jj absent   -> skip メッセージが現れる   (= guard が機能している)
# を assert する。
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

SKIP_MSG="git+jj fixture requires both binaries"
# jj バックエンドを叩くテストに絞って実行時間を抑える (VCS / jj 系)。
TEST_SELECTOR='VCS|Jj|jj'

if ! command -v git >/dev/null 2>&1; then
	echo "SKIP: git not installed; cannot run jj-runs gate" >&2
	exit 0
fi
if ! command -v jj >/dev/null 2>&1; then
	echo "FAIL: jj not installed. The jj VCS backend tests would be skipped in CI." >&2
	echo "      Install jj (see .github/workflows/ci.yml) before relying on jj coverage." >&2
	exit 1
fi

# 一時ファイル / ディレクトリは先にまとめて確保し、単一の trap で掃除する。
present_log="$(mktemp)"
absent_log="$(mktemp)"
nojj_bin="$(mktemp -d)"
trap 'rm -f "$present_log" "$absent_log"; rm -rf "$nojj_bin"' EXIT

# (a) jj present: skip メッセージが出ないこと
if ! go test ./src/ -count=1 -run "$TEST_SELECTOR" -v >"$present_log" 2>&1; then
	echo "FAIL: jj backend tests failed with jj present:" >&2
	cat "$present_log" >&2
	exit 1
fi
if grep -q "$SKIP_MSG" "$present_log"; then
	echo "FAIL: jj is installed but jj backend tests were still skipped." >&2
	grep -n "$SKIP_MSG" "$present_log" >&2
	exit 1
fi
echo "PASS: jj present -> jj backend tests run (no '$SKIP_MSG' skip)"

# (b) jj absent: skip メッセージが出ること (guard 機能の確認)
# jj だけを PATH から消す。jj のディレクトリごと除くと go / git まで巻き添えで
# 消える環境があるため、必要なツールへの symlink だけを置いた専用 bin を作り、
# PATH をそこ単独にする (jj は symlink しない = 見えなくなる)。
for tool in go git sh bash env; do
	tp="$(command -v "$tool" 2>/dev/null || true)"
	[ -n "$tp" ] && ln -sf "$tp" "$nojj_bin/$tool"
done
if PATH="$nojj_bin" command -v jj >/dev/null 2>&1; then
	echo "SKIP(b): jj still reachable; cannot simulate absence" >&2
else
	PATH="$nojj_bin" go test ./src/ -count=1 -run "$TEST_SELECTOR" -v >"$absent_log" 2>&1 || true
	if grep -q "$SKIP_MSG" "$absent_log"; then
		echo "PASS: jj absent -> jj backend tests skip (guard works)"
	else
		echo "FAIL: jj absent but no '$SKIP_MSG' skip observed; guard may be broken." >&2
		exit 1
	fi
fi

echo "OK: jj-runs gate passed"
