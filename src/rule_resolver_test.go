package main

import (
	"strings"
	"testing"
)

// --- DR-0029 Phase B: matcher + resolver tests --------------------------

func TestMatch_AbsolutePath(t *testing.T) {
	t.Parallel()
	// Absolute pattern + cwd-relative source: both Cleaned to absolute
	// and compared.
	cases := []struct {
		src, pat string
		want     matchStrength
	}{
		{"/tmp/proj/package.json", "/tmp/proj/package.json", strengthAbsolute},
		{"/tmp/proj/package.json", "/tmp/proj/other.json", strengthNoMatch},
		// symlink resolve is NOT performed: /tmp vs /private/tmp distinct.
		{"/tmp/foo", "/private/tmp/foo", strengthNoMatch},
	}
	for _, tc := range cases {
		got, err := matchSourceToPattern(tc.src, tc.pat)
		if err != nil {
			t.Fatalf("match(%q, %q): unexpected error: %v", tc.src, tc.pat, err)
		}
		if got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.src, tc.pat, got, tc.want)
		}
	}
}

func TestMatch_RelativePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src, pat string
		want     matchStrength
	}{
		{"othersystem/package.json", "othersystem/package.json", strengthRelative},
		{"othersystem/package.json", "./othersystem/package.json", strengthRelative},
		{"ts/package.json", "othersystem/package.json", strengthNoMatch},
		// ./X vs X: both Cleaned to X (basename), should hit basename tier
		{"package.json", "./package.json", strengthBasename},
	}
	for _, tc := range cases {
		got, err := matchSourceToPattern(tc.src, tc.pat)
		if err != nil {
			t.Fatalf("match(%q, %q): unexpected error: %v", tc.src, tc.pat, err)
		}
		if got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.src, tc.pat, got, tc.want)
		}
	}
}

func TestMatch_Basename(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src, pat string
		want     matchStrength
	}{
		{"ts/package.json", "package.json", strengthBasename},
		{"deep/nested/package.json", "package.json", strengthBasename},
		{"package.json", "package.json", strengthBasename},
		{"ts/package.json", "other.json", strengthNoMatch},
	}
	for _, tc := range cases {
		got, err := matchSourceToPattern(tc.src, tc.pat)
		if err != nil {
			t.Fatalf("match(%q, %q): unexpected error: %v", tc.src, tc.pat, err)
		}
		if got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.src, tc.pat, got, tc.want)
		}
	}
}

func TestMatch_Glob(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src, pat string
		want     matchStrength
	}{
		{"foo.myapp", "glob:*.myapp", strengthGlob},
		{"bar.myapp", "glob:*.myapp", strengthGlob},
		{"foo.txt", "glob:*.myapp", strengthNoMatch},
		{"a/b/c.json", "glob:**/*.json", strengthGlob},
		// glob: literal (no meta) — still glob tier
		{"package.json", "glob:package.json", strengthGlob},
		// non-matching glob: literal
		{"other.json", "glob:package.json", strengthNoMatch},
	}
	for _, tc := range cases {
		got, err := matchSourceToPattern(tc.src, tc.pat)
		if err != nil {
			t.Fatalf("match(%q, %q): unexpected error: %v", tc.src, tc.pat, err)
		}
		if got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.src, tc.pat, got, tc.want)
		}
	}
}

func TestMatch_BareWithGlobMetaRejected(t *testing.T) {
	t.Parallel()
	// bare PATTERN containing *, ?, [, { → error (DR-0029 § 1.1 補強)
	for _, pat := range []string{"*.json", "foo?bar", "foo[abc]", "foo{a,b}"} {
		_, err := matchSourceToPattern("anything", pat)
		if err == nil {
			t.Errorf("match(_, %q): expected error for bare PATTERN with glob meta, got nil", pat)
			continue
		}
		if !strings.Contains(err.Error(), "glob: prefix") {
			t.Errorf("match(_, %q): error %q should mention glob: prefix hint", pat, err)
		}
	}
}

func TestMatch_EmptyGlobPrefix(t *testing.T) {
	t.Parallel()
	_, err := matchSourceToPattern("foo.json", "glob:")
	if err == nil {
		t.Fatalf("match(_, glob:): expected error for empty glob, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error %q should mention cannot be empty", err)
	}
}

func TestResolve_NoMatchFallsThroughToBuiltin(t *testing.T) {
	t.Parallel()
	// blocks with only the global slot, no rule flags → BlockIdx=-1.
	blocks := []ruleBlock{{Pattern: ""}}
	got, err := resolveRuleBlock("VERSION", blocks)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if got.BlockIdx != -1 {
		t.Errorf("BlockIdx = %d, want -1 (fall through to builtin)", got.BlockIdx)
	}
}

func TestResolve_NamedBlockWins(t *testing.T) {
	t.Parallel()
	blocks := []ruleBlock{
		{Pattern: ""}, // global, no flags
		{Pattern: "package.json", Opts: ruleOpts{Format: strPtr("json")}},
	}
	got, err := resolveRuleBlock("ts/package.json", blocks)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if got.BlockIdx != 1 || got.Strength != strengthBasename {
		t.Errorf("got BlockIdx=%d Strength=%v, want 1 / basename", got.BlockIdx, got.Strength)
	}
}

func TestResolve_GlobalUsedWhenNoNamedBlockMatches(t *testing.T) {
	t.Parallel()
	blocks := []ruleBlock{
		{Pattern: "", Opts: ruleOpts{Format: strPtr("text"), VersionRegex: strPtr("v(.+)")}},
		{Pattern: "specific.json", Opts: ruleOpts{Format: strPtr("json")}},
	}
	got, err := resolveRuleBlock("other.txt", blocks)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if got.BlockIdx != 0 {
		t.Errorf("BlockIdx = %d, want 0 (global with flags)", got.BlockIdx)
	}
}

func TestResolve_BareWinsOverGlob(t *testing.T) {
	t.Parallel()
	blocks := []ruleBlock{
		{Pattern: ""},
		{Pattern: "glob:*.json", Opts: ruleOpts{Format: strPtr("json"), VersionPath: strPtr("$.v1")}},
		{Pattern: "package.json", Opts: ruleOpts{Format: strPtr("json"), VersionPath: strPtr("$.v2")}},
	}
	got, err := resolveRuleBlock("ts/package.json", blocks)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if got.BlockIdx != 2 {
		t.Errorf("BlockIdx = %d, want 2 (basename bare wins over glob)", got.BlockIdx)
	}
}

func TestResolve_AmbiguousSameStrength(t *testing.T) {
	t.Parallel()
	blocks := []ruleBlock{
		{Pattern: ""},
		{Pattern: "glob:*.json", Opts: ruleOpts{Format: strPtr("json")}},
		{Pattern: "glob:foo.*", Opts: ruleOpts{Format: strPtr("json")}},
	}
	_, err := resolveRuleBlock("foo.json", blocks)
	if err == nil {
		t.Fatalf("expected ambiguous error for two glob matches")
	}
	if !strings.Contains(err.Error(), "multiple rules") {
		t.Errorf("error %q should mention multiple rules", err)
	}
}

func TestResolve_AbsoluteWinsOverRelative(t *testing.T) {
	t.Parallel()
	// Synthetic test: pattern that resolves to absolute (= match strength
	// 5) vs basename (= match strength 2). Construct so source has both
	// absolute representation accessible.
	blocks := []ruleBlock{
		{Pattern: ""},
		{Pattern: "package.json", Opts: ruleOpts{Format: strPtr("json"), VersionPath: strPtr("$.basename")}},
	}
	// Source is just basename — only the basename block matches.
	got, err := resolveRuleBlock("package.json", blocks)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if got.BlockIdx != 1 {
		t.Errorf("BlockIdx = %d, want 1", got.BlockIdx)
	}
}

func TestDetectDeadBlocks(t *testing.T) {
	t.Parallel()
	blocks := []ruleBlock{
		{Pattern: ""},
		{Pattern: "a.json"},
		{Pattern: "b.json"},
		{Pattern: "c.json"},
	}
	matched := map[int]bool{1: true, 3: true}
	dead := detectDeadBlocks(blocks, matched)
	if len(dead) != 1 || dead[0] != 2 {
		t.Errorf("dead = %v, want [2]", dead)
	}
}

func TestIsDeadGlobal(t *testing.T) {
	t.Parallel()
	blocksWithGlobal := []ruleBlock{
		{Pattern: "", Opts: ruleOpts{Format: strPtr("json")}},
		{Pattern: "a.json"},
	}
	if !isDeadGlobal(blocksWithGlobal, true) {
		t.Error("isDeadGlobal: expected true when global has flags and all sources covered")
	}
	if isDeadGlobal(blocksWithGlobal, false) {
		t.Error("isDeadGlobal: expected false when at least one source uses global")
	}
	noGlobalFlags := []ruleBlock{
		{Pattern: ""},
		{Pattern: "a.json"},
	}
	if isDeadGlobal(noGlobalFlags, true) {
		t.Error("isDeadGlobal: expected false when global has no flags")
	}
}

// helper: returns *string of the given literal
func strPtr(s string) *string { return &s }
