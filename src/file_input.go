package main

// file_input.go — DR-0033 `file:<path>` 入力 prefix の展開ロジック。
//
// `file:` は外部 file から path list を流し込む input prefix。各行が:
//   - literal path (相対 / 絶対 / `~/` 含)
//   - `glob:<pattern>` (内部で再帰的に glob 展開、外側 globOpts に従う)
//   - `#` で始まる行 or 空行 → スキップ (= コメント / 区切り)
//
// `file:` の再帰展開 (= `file:LIST` の中で `file:another` を書く) は MVP 範囲外
// (= 循環検出 + depth limit のコストを払うほどの実需が現状ない)。本実装では
// `file:` 行は usage error として reject、将来必要なら forward-compatible に拡張。

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// hasFilePrefix reports whether spec begins with the `file:` selector prefix.
func hasFilePrefix(spec string) bool {
	return strings.HasPrefix(spec, "file:")
}

// parseFileSpec strips the `file:` prefix from spec. Empty paths are rejected
// (`file:` with no body is a usage error).
func parseFileSpec(spec string) (string, error) {
	if !hasFilePrefix(spec) {
		return "", fmt.Errorf("not a file: spec: %q", spec)
	}
	path := strings.TrimPrefix(spec, "file:")
	if path == "" {
		return "", fmt.Errorf("file: path is empty")
	}
	return path, nil
}

// expandFileSpec reads the file at path and returns the list of expanded paths
// (literal lines pass through verbatim; `glob:` lines are expanded via the
// shared glob layer with the caller-provided opts). Comment lines (`#` prefix)
// and blank lines are skipped.
//
// Nested `file:` references inside a list are rejected as usage errors (MVP
// scope-out, see DR-0033 § "代替案検討").
func expandFileSpec(path string, opts globOpts) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("file:%s: %w", path, err)
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hasFilePrefix(line) {
			return nil, fmt.Errorf("file:%s:%d: nested file: is not supported (MVP scope, see DR-0033)", path, lineNo)
		}
		if hasGlobPrefix(line) {
			pat, perr := parseGlobSpec(line)
			if perr != nil {
				return nil, fmt.Errorf("file:%s:%d: %w", path, lineNo, perr)
			}
			matches, gerr := expandGlob(pat, opts, defaultHomeFn)
			if gerr != nil {
				return nil, fmt.Errorf("file:%s:%d: %w", path, lineNo, gerr)
			}
			out = append(out, matches...)
			continue
		}
		out = append(out, line)
	}
	if serr := scanner.Err(); serr != nil {
		return nil, fmt.Errorf("file:%s: read: %w", path, serr)
	}
	return out, nil
}
