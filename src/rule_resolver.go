package main

import (
	"fmt"
	"path/filepath"
	"strings"

	doublestar "github.com/bmatcuk/doublestar/v4"
)

// rule_resolver.go implements DR-0029 § "PATTERN match strength" and
// § "Flag のスコープ規約 評価規則". Given a positional SOURCE and the
// parsed ruleBlocks slice, it returns the effective ruleBlock (or nil
// when builtin should win).
//
// Implementation notes:
//
//   - Path normalisation uses filepath.Clean; symlink resolve is NOT
//     performed (DR-0029 § 1.1: macOS /tmp vs /private/tmp must stay
//     distinct, design intent is preserved by symlinks).
//   - Glob matching reuses DR-0024's doublestar.Match (matcher only;
//     DR-0028's substitute/backref extensions are not used here).
//   - bare PATTERN with glob meta characters (`*` `?` `[` `{`) is
//     explicitly rejected; users must add the `glob:` prefix to opt
//     into glob semantics (DR-0029 § 1.1 補強, no silent gitignore-
//     style interpretation).

// matchStrength is the DR-0029 PATTERN match strength scoring. Higher
// = more specific. strengthNoMatch (-1) signals "no match".
type matchStrength int

const (
	strengthNoMatch  matchStrength = -1
	strengthBuiltin  matchStrength = 0 // builtin fallback (= no --define-rule matched)
	strengthGlob     matchStrength = 1
	strengthBasename matchStrength = 2
	strengthRelative matchStrength = 3
	strengthAbsolute matchStrength = 5
)

// String renders a strength value for help / error message UX.
func (s matchStrength) String() string {
	switch s {
	case strengthAbsolute:
		return "5 (absolute path)"
	case strengthRelative:
		return "3 (relative path)"
	case strengthBasename:
		return "2 (basename)"
	case strengthGlob:
		return "1 (glob)"
	case strengthBuiltin:
		return "0 (builtin)"
	case strengthNoMatch:
		return "no match"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

// hasGlobMeta reports whether `s` contains any glob meta character that
// would change matching semantics under doublestar. Used to reject
// `--define-rule` PATTERNs that look like globs but lack the `glob:`
// prefix.
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[{")
}

// matchSourceToPattern returns the strength at which `pattern` matches
// `sourcePath`. Returns strengthNoMatch (-1) if no match. Returns an
// error only when the pattern itself is malformed (e.g. bare PATTERN
// with glob meta, broken glob: pattern) — these are user errors
// surfaced at parse-tail / resolve time rather than silent no-matches.
//
// Both sourcePath and pattern are filepath.Clean'd before comparison.
// Symlink resolve is deliberately NOT performed (DR-0029 § 1.1).
func matchSourceToPattern(sourcePath, pattern string) (matchStrength, error) {
	if strings.HasPrefix(pattern, "glob:") {
		glob := strings.TrimPrefix(pattern, "glob:")
		if glob == "" {
			return strengthNoMatch, fmt.Errorf("glob: PATTERN cannot be empty after the prefix")
		}
		matched, err := doublestar.Match(glob, sourcePath)
		if err != nil {
			return strengthNoMatch, fmt.Errorf("glob:%s: %w", glob, err)
		}
		if matched {
			return strengthGlob, nil
		}
		return strengthNoMatch, nil
	}
	// Bare PATTERN: reject glob meta to surface the missing glob:
	// prefix as a clear error rather than a silent literal mis-match.
	if hasGlobMeta(pattern) {
		return strengthNoMatch, fmt.Errorf("PATTERN %q contains glob meta character; use the glob: prefix to opt into glob matching (e.g. glob:%s)", pattern, pattern)
	}
	cleanPattern := filepath.Clean(pattern)
	cleanSource := filepath.Clean(sourcePath)
	// Absolute path (tier 5): pattern starts with `/`. Compare to
	// cwd-relative source's absolute path. symlink resolve is NOT
	// performed (DR-0029 § 1.1 explicit).
	if filepath.IsAbs(cleanPattern) {
		absSrc, err := filepath.Abs(cleanSource)
		if err != nil {
			return strengthNoMatch, fmt.Errorf("filepath.Abs(%q): %w", cleanSource, err)
		}
		if cleanPattern == filepath.Clean(absSrc) {
			return strengthAbsolute, nil
		}
		return strengthNoMatch, nil
	}
	// Basename (tier 2): no path separator in PATTERN. ./X is NOT
	// basename (./ adds a separator after Clean(./X) = X check below).
	// We use the post-Clean pattern so "X" and "./X" both reach the
	// relative branch with cleanPattern == "X", which is then treated
	// uniformly.
	if !strings.Contains(cleanPattern, "/") {
		if filepath.Base(cleanSource) == cleanPattern {
			return strengthBasename, nil
		}
		// Also a basename match if the SOURCE itself is a bare basename:
		// pattern "X" vs source "X" (no separator) — both Clean to the
		// same string. Treat as basename tier (cwd-rooted single name
		// is equivalent to basename for matching purposes).
		if cleanSource == cleanPattern {
			return strengthBasename, nil
		}
		return strengthNoMatch, nil
	}
	// Relative path (tier 3): both Clean'd, compare verbatim.
	if cleanPattern == cleanSource {
		return strengthRelative, nil
	}
	return strengthNoMatch, nil
}

// blockMatch carries the resolution result for one SOURCE.
type blockMatch struct {
	BlockIdx int           // 0 = global, >0 = named block index in ruleBlocks
	Strength matchStrength // strengthBuiltin for global fall-through, or named-block strength
}

// resolveRuleBlock returns the effective rule block index for the
// given sourcePath, plus the matched strength. When no named block
// matches AND the global block carries rule flags, returns the global
// block (BlockIdx=0, Strength=strengthBuiltin to mark "no named
// match"). When no named block matches AND global has no rule flags,
// returns BlockIdx=-1 (= use builtin auto-detection).
//
// Ambiguous error when multiple named blocks match at the same
// strength (DR-0029 § "Ambiguous / dead / 失敗時の規約 0c").
func resolveRuleBlock(sourcePath string, blocks []ruleBlock) (blockMatch, error) {
	none := blockMatch{BlockIdx: -1, Strength: strengthNoMatch}
	if len(blocks) == 0 {
		return none, nil
	}
	var winners []int
	maxStrength := strengthNoMatch
	for i := 1; i < len(blocks); i++ { // skip global (index 0)
		s, err := matchSourceToPattern(sourcePath, blocks[i].Pattern)
		if err != nil {
			return none, fmt.Errorf("--define-rule %q: %w", blocks[i].Pattern, err)
		}
		if s == strengthNoMatch {
			continue
		}
		if s > maxStrength {
			maxStrength = s
			winners = winners[:0]
			winners = append(winners, i)
		} else if s == maxStrength {
			winners = append(winners, i)
		}
	}
	if len(winners) > 1 {
		labels := make([]string, 0, len(winners))
		for _, w := range winners {
			labels = append(labels, fmt.Sprintf("--define-rule %q (declared position %d)", blocks[w].Pattern, w))
		}
		return none, fmt.Errorf("SOURCE %q matches multiple rules at strength %s:\n  %s\nhint: make one PATTERN more specific (e.g. switch glob to bare path) or remove the duplicate --define-rule",
			sourcePath, maxStrength, strings.Join(labels, "\n  "))
	}
	if len(winners) == 1 {
		return blockMatch{BlockIdx: winners[0], Strength: maxStrength}, nil
	}
	// No named block matched. Use the global block if it has rule flags;
	// else fall through to builtin auto-detection.
	if blocks[0].Opts.hasAny() {
		return blockMatch{BlockIdx: 0, Strength: strengthBuiltin}, nil
	}
	return none, nil
}

// detectDeadBlocks returns the list of named-block indexes (1..N) that
// were NOT matched by any of the SOURCES in the resolution map. The
// global block (index 0) is never considered "dead" by this function
// — it's covered by a separate "dead global" detection (= global flags
// used despite every SOURCE being covered by a named block).
//
// matchedNamedIdxs is the set of named-block indexes that won at least
// one SOURCE; e.g. {1: true, 3: true} means blocks 1 and 3 matched
// something, block 2 (if present) did not.
func detectDeadBlocks(blocks []ruleBlock, matchedNamedIdxs map[int]bool) []int {
	dead := []int{}
	for i := 1; i < len(blocks); i++ {
		if !matchedNamedIdxs[i] {
			dead = append(dead, i)
		}
	}
	return dead
}

// isDeadGlobal reports whether the global block carries rule flags
// that go unused because every SOURCE was covered by a named block.
// This is a WARNING condition (not an error) per DR-0029 § "dead
// global" — the dispatcher emits a stderr hint that --no-hint /
// -q / -qq suppresses, but execution continues.
func isDeadGlobal(blocks []ruleBlock, allSourcesMatchedNamed bool) bool {
	if len(blocks) == 0 || !allSourcesMatchedNamed {
		return false
	}
	return blocks[0].Opts.hasAny()
}
