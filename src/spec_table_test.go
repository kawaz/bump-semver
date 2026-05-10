package main

// Spec-driven tests transcribed from DR-0006
// (docs/decisions/DR-0006-pre-release-and-compare.md).
//
// These tests intentionally reproduce the DR's tables verbatim so that the
// DR remains the single source of truth: every row here corresponds to a
// row or sentence in the DR, and any divergence must be reflected in the
// DR first.
//
// Phase 1 scope: Parse, BumpDropDefault, Compare, PreAction.

import (
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// TestSpec_Parse: SemVer 2.0.0 + kawaz prefix/sep extension
//
// DR-0006 § "拡張 prefix / separator (DR-0003 の更新)":
//   - 本体は [._] のみ (sep1 == sep2 強制)
//   - pre-release は SemVer 仕様準拠 (`-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*`)
//   - build metadata は SemVer 仕様準拠 (`+...`)
//   - 数値識別子は leading zero 禁止 (本体・pre 共通)、build metadata は許容
//
// ----------------------------------------------------------------------
func TestSpec_Parse(t *testing.T) {
	t.Parallel()
	type goodCase struct {
		in            string
		prefix        string
		sep           string
		major         int
		minor         int
		patch         int
		pre           []string
		buildMetadata []string
	}
	good := []goodCase{
		// Plain X.Y.Z
		{in: "0.0.0", sep: ".", major: 0, minor: 0, patch: 0},
		{in: "1.2.3", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "10.20.30", sep: ".", major: 10, minor: 20, patch: 30},
		// Underscore separator
		{in: "1_2_3", sep: "_", major: 1, minor: 2, patch: 3},
		// Prefix
		{in: "v1.2.3", prefix: "v", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "ver1.2.3", prefix: "ver", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "version1.2.3", prefix: "version", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "v_1.2.3", prefix: "v_", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "v.1.2.3", prefix: "v.", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "v-1.2.3", prefix: "v-", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "version_1_2_3", prefix: "version_", sep: "_", major: 1, minor: 2, patch: 3},
		// Pre-release (SemVer 2.0.0)
		{in: "1.2.3-alpha", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"alpha"}},
		{in: "1.2.3-rc.1", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"rc", "1"}},
		{in: "1.2.3-rc.0", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"rc", "0"}},
		{in: "1.2.3-0", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"0"}},
		{in: "1.2.3-rc1", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"rc1"}},
		{in: "1.2.3-alpha.1.2", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"alpha", "1", "2"}},
		{in: "1.2.3-x-y-z.-", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"x-y-z", "-"}},
		// Build metadata (SemVer 2.0.0)
		{in: "1.2.3+build", sep: ".", major: 1, minor: 2, patch: 3, buildMetadata: []string{"build"}},
		{in: "1.2.3+build.1", sep: ".", major: 1, minor: 2, patch: 3, buildMetadata: []string{"build", "1"}},
		{in: "1.2.3+001", sep: ".", major: 1, minor: 2, patch: 3, buildMetadata: []string{"001"}}, // build metadata は leading zero OK
		// Pre + build
		{in: "1.2.3-rc.1+build.42", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"rc", "1"}, buildMetadata: []string{"build", "42"}},
		{in: "v1.2.3-alpha+exp.sha.5114f85", prefix: "v", sep: ".", major: 1, minor: 2, patch: 3, pre: []string{"alpha"}, buildMetadata: []string{"exp", "sha", "5114f85"}},
		// Whitespace tolerance
		{in: "  1.2.3  ", sep: ".", major: 1, minor: 2, patch: 3},
		{in: "1.2.3\n", sep: ".", major: 1, minor: 2, patch: 3},
	}
	for _, tc := range good {
		got, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got.Prefix != tc.prefix || got.Sep != tc.sep ||
			got.Major != tc.major || got.Minor != tc.minor || got.Patch != tc.patch {
			t.Errorf("ParseVersion(%q) base = {Prefix:%q Sep:%q M:%d m:%d p:%d}, want {Prefix:%q Sep:%q M:%d m:%d p:%d}",
				tc.in, got.Prefix, got.Sep, got.Major, got.Minor, got.Patch,
				tc.prefix, tc.sep, tc.major, tc.minor, tc.patch)
		}
		if !equalStringSlice(got.Pre, tc.pre) {
			t.Errorf("ParseVersion(%q).Pre = %#v, want %#v", tc.in, got.Pre, tc.pre)
		}
		if !equalStringSlice(got.BuildMetadata, tc.buildMetadata) {
			t.Errorf("ParseVersion(%q).BuildMetadata = %#v, want %#v", tc.in, got.BuildMetadata, tc.buildMetadata)
		}
	}

	bad := []string{
		"",
		"1.2",
		"1.2.3.4",
		"1.2.3 something",
		"1.2.x",
		"01.2.3",         // leading zero in main
		"1.02.3",         // leading zero in main
		"1.2.03",         // leading zero in main
		"-1.2.3",         // negative
		"1..3",           // empty component
		"a.b.c",          // non-numeric
		"1.2-3",          // sep mismatch
		"1-2.3",          // sep mismatch
		"foo1.2.3",       // unsupported prefix
		"1.2.3v",         // trailing junk
		"V1.2.3",         // case-sensitive
		"1.2.3-",         // empty pre
		"1.2.3+",         // empty build
		"1.2.3-+build",   // empty pre with build
		"1.2.3-01",       // leading zero in numeric pre identifier
		"1.2.3-rc.01",    // leading zero in numeric pre identifier
		"1.2.3-rc..1",    // empty pre identifier
		"1.2.3-rc.1+",    // trailing empty build
		"1.2.3+build..1", // empty build identifier
		"1.2.3+build/x",  // invalid char in build
		"1.2.3-rc.1$",    // invalid trailing char
		"1-2-3",          // body sep `-` no longer allowed (DR-0006 update)
		"v1-2-3",         // body sep `-`
		"ver-1-2-3",      // body sep `-`
	}
	for _, in := range bad {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) expected error, got nil", in)
		}
	}
}

// ----------------------------------------------------------------------
// TestSpec_BumpDropDefault: DR-0006 § "bump 挙動 (drop デフォルト)"
//
// |               Input | patch     | pre                    | pre --pre alpha | pre --no-pre |
// |--------------------:|-----------|------------------------|-----------------|--------------|
// |               1.2.3 | 1.2.4     | error: not pre-release | 1.2.3-alpha     | (nop) 1.2.3  |
// |          1.2.3-rc.0 | 1.2.4     | 1.2.3-rc.1             | 1.2.3-alpha     | 1.2.3        |
// |          1.2.3-rc1  | 1.2.4     | error: not incremental | 1.2.3-alpha     | 1.2.3        |
// |        1.2.3+build  | 1.2.4     | error: not pre-release | 1.2.3-alpha     | (nop) 1.2.3  |
// |   1.2.3-rc.0+build  | 1.2.4     | 1.2.3-rc.1             | 1.2.3-alpha     | 1.2.3        |
// ----------------------------------------------------------------------
func TestSpec_BumpDropDefault(t *testing.T) {
	t.Parallel()
	type cell struct {
		want    string // expected output (when wantErr=false)
		wantErr bool
	}
	type row struct {
		in           string
		patch        cell
		preBare      cell // `pre` action with no flags
		preWithAlpha cell // `pre` action with --pre alpha
		preNoPre     cell // `pre` action with --no-pre
	}
	rows := []row{
		{
			in:           "1.2.3",
			patch:        cell{want: "1.2.4"},
			preBare:      cell{wantErr: true},
			preWithAlpha: cell{want: "1.2.3-alpha"},
			preNoPre:     cell{want: "1.2.3"},
		},
		{
			in:           "1.2.3-rc.0",
			patch:        cell{want: "1.2.4"}, // pre dropped
			preBare:      cell{want: "1.2.3-rc.1"},
			preWithAlpha: cell{want: "1.2.3-alpha"},
			preNoPre:     cell{want: "1.2.3"},
		},
		{
			in:           "1.2.3-rc1",
			patch:        cell{want: "1.2.4"},
			preBare:      cell{wantErr: true}, // not incremental
			preWithAlpha: cell{want: "1.2.3-alpha"},
			preNoPre:     cell{want: "1.2.3"},
		},
		{
			in:           "1.2.3+build",
			patch:        cell{want: "1.2.4"}, // build dropped
			preBare:      cell{wantErr: true},
			preWithAlpha: cell{want: "1.2.3-alpha"},
			preNoPre:     cell{want: "1.2.3"},
		},
		{
			in:           "1.2.3-rc.0+build",
			patch:        cell{want: "1.2.4"}, // both dropped
			preBare:      cell{want: "1.2.3-rc.1"},
			preWithAlpha: cell{want: "1.2.3-alpha"},
			preNoPre:     cell{want: "1.2.3"},
		},
	}

	check := func(t *testing.T, in, action string, opts BumpOptions, c cell) {
		t.Helper()
		v, err := ParseVersion(in)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", in, err)
		}
		got, err := v.Bump(action, opts)
		if c.wantErr {
			if err == nil {
				t.Errorf("Bump(%q, %q, %+v) expected error, got %q", in, action, opts, got.String())
			}
			return
		}
		if err != nil {
			t.Errorf("Bump(%q, %q, %+v) unexpected error: %v", in, action, opts, err)
			return
		}
		if got.String() != c.want {
			t.Errorf("Bump(%q, %q, %+v) = %q, want %q", in, action, opts, got.String(), c.want)
		}
	}

	for _, r := range rows {
		check(t, r.in, "patch", BumpOptions{}, r.patch)
		check(t, r.in, "pre", BumpOptions{}, r.preBare)
		check(t, r.in, "pre", BumpOptions{Pre: "alpha", PreSet: true}, r.preWithAlpha)
		check(t, r.in, "pre", BumpOptions{NoPre: true}, r.preNoPre)
	}
}

// ----------------------------------------------------------------------
// TestSpec_Compare: SemVer 2.0.0 § 11 Precedence
//
// 1. major/minor/patch numerical
// 2. pre-release < release (1.0.0-rc.1 < 1.0.0)
// 3. pre identifiers compared field-by-field:
//   - both numeric: numeric compare
//   - both alphanumeric: ASCII compare
//   - numeric < alphanumeric
//   - shorter list < longer list (when prefix matches)
//
// 4. build metadata is IGNORED for ordering (1.0.0+a == 1.0.0+b)
// 5. prefix/sep is IGNORED for ordering (v1.2.3 == 1.2.3 == version_1_2_3)
// ----------------------------------------------------------------------
func TestSpec_Compare(t *testing.T) {
	t.Parallel()
	type cmp struct {
		a, b string
		want int // -1, 0, 1
	}
	cases := []cmp{
		// MAJOR/MINOR/PATCH
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", +1},
		{"1.0.0", "1.0.0", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.10.0", "1.9.0", +1}, // numeric not lex
		// pre-release < release
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", +1},
		{"1.0.0-alpha", "1.0.0-alpha", 0},
		// SemVer 2.0.0 official example chain
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1}, // numeric < alpha at idx 1
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-beta.2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1}, // numeric compare, not lex
		{"1.0.0-beta.11", "1.0.0-rc.1", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		// numeric vs alphanumeric at same position
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0-alpha", "1.0.0-1", +1},
		// build metadata ignored
		{"1.0.0+build1", "1.0.0+build2", 0},
		{"1.0.0+a", "1.0.0+b", 0},
		{"1.0.0-rc.1+a", "1.0.0-rc.1+z", 0},
		{"1.0.0", "1.0.0+build", 0},
		// prefix and sep ignored
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "version_1_2_3", 0},
		{"version_1_2_3", "1.2.3", 0},
		{"v_1.2.3", "ver-1.2.3", 0},
	}

	for _, c := range cases {
		va, err := ParseVersion(c.a)
		if err != nil {
			t.Fatalf("ParseVersion(%q) err: %v", c.a, err)
		}
		vb, err := ParseVersion(c.b)
		if err != nil {
			t.Fatalf("ParseVersion(%q) err: %v", c.b, err)
		}
		got := va.Compare(vb)
		if normalizeCmp(got) != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		// symmetry
		gotRev := vb.Compare(va)
		if normalizeCmp(gotRev) != -c.want {
			t.Errorf("Compare(%q, %q) = %d (reverse), want %d", c.b, c.a, gotRev, -c.want)
		}
	}
}

// ----------------------------------------------------------------------
// TestSpec_PreAction: pre アクション詳細仕様
//
// DR-0006 § "pre アクションの詳細":
//   - 引数なし: 末尾 pre 識別子が pure numeric なら +1、それ以外エラー
//   - --pre PRE: 上書き (元 pre 有無問わず)
//   - --no-pre: 削除 (元 pre 不在でも nop)
//   - 確定論点 C: `pre 1.2.3-0 → 1.2.3-1` (single pure numeric は許容)
//
// ----------------------------------------------------------------------
func TestSpec_PreAction(t *testing.T) {
	t.Parallel()
	type tc struct {
		in      string
		opts    BumpOptions
		want    string
		wantErr bool
	}
	cases := []tc{
		// counter advance (last identifier is pure numeric)
		{"1.2.3-rc.0", BumpOptions{}, "1.2.3-rc.1", false},
		{"1.2.3-rc.9", BumpOptions{}, "1.2.3-rc.10", false},
		{"1.2.3-alpha.0", BumpOptions{}, "1.2.3-alpha.1", false},
		{"1.2.3-0", BumpOptions{}, "1.2.3-1", false},             // 確定論点 C: single numeric OK
		{"1.2.3-9", BumpOptions{}, "1.2.3-10", false},            // single numeric counter advance
		{"1.2.3-rc.0+build", BumpOptions{}, "1.2.3-rc.1", false}, // build dropped
		// errors: no pre / not incremental
		{"1.2.3", BumpOptions{}, "", true},         // no pre at all
		{"1.2.3+build", BumpOptions{}, "", true},   // no pre
		{"1.2.3-rc1", BumpOptions{}, "", true},     // single alphanumeric
		{"1.2.3-alpha", BumpOptions{}, "", true},   // single alpha
		{"1.2.3-rc.beta", BumpOptions{}, "", true}, // last is alpha
		// --pre PRE: overwrite
		{"1.2.3", BumpOptions{Pre: "alpha", PreSet: true}, "1.2.3-alpha", false},
		{"1.2.3-rc.0", BumpOptions{Pre: "alpha", PreSet: true}, "1.2.3-alpha", false},
		{"1.2.3-rc.0", BumpOptions{Pre: "rc.0", PreSet: true}, "1.2.3-rc.0", false}, // overwrite to same
		{"1.2.3-rc.5", BumpOptions{Pre: "rc.0", PreSet: true}, "1.2.3-rc.0", false}, // counter rewind allowed
		{"1.2.3-rc.0", BumpOptions{Pre: "alpha.beta.gamma", PreSet: true}, "1.2.3-alpha.beta.gamma", false},
		// --pre with build dropped
		{"1.2.3+build", BumpOptions{Pre: "alpha", PreSet: true}, "1.2.3-alpha", false},
		{"1.2.3-rc.0+build", BumpOptions{Pre: "alpha", PreSet: true}, "1.2.3-alpha", false},
		// --no-pre: remove
		{"1.2.3", BumpOptions{NoPre: true}, "1.2.3", false}, // nop on bare
		{"1.2.3-rc.0", BumpOptions{NoPre: true}, "1.2.3", false},
		{"1.2.3+build", BumpOptions{NoPre: true}, "1.2.3", false}, // build also dropped
		{"1.2.3-rc.0+build", BumpOptions{NoPre: true}, "1.2.3", false},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", c.in, err)
		}
		got, err := v.Bump("pre", c.opts)
		if c.wantErr {
			if err == nil {
				t.Errorf("Bump(%q, pre, %+v) expected error, got %q", c.in, c.opts, got.String())
			}
			continue
		}
		if err != nil {
			t.Errorf("Bump(%q, pre, %+v) unexpected error: %v", c.in, c.opts, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("Bump(%q, pre, %+v) = %q, want %q", c.in, c.opts, got.String(), c.want)
		}
	}
}

// ----------------------------------------------------------------------
// TestSpec_PreActionErrorMessages: DR-0006 § "pre アクションの詳細"
// エラー例:
//   - pre 1.2.3       → "1.2.3 does not have a pre-release, use --pre PRE"
//   - pre 1.2.3-rc1   → "rc1 is not incremental, use --pre PRE"
//   - pre 1.2.3-alpha → "alpha is not incremental, use --pre PRE"
//
// ----------------------------------------------------------------------
func TestSpec_PreActionErrorMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantSub  string // substring expected to appear in err.Error()
		wantBase string // X.Y.Z prefix mentioned in error (only for "no pre")
	}{
		{"1.2.3", "1.2.3 does not have a pre-release, use --pre PRE", "1.2.3"},
		{"1.2.3-rc1", "rc1 is not incremental, use --pre PRE", ""},
		{"1.2.3-alpha", "alpha is not incremental, use --pre PRE", ""},
		{"v1.2.3", "v1.2.3 does not have a pre-release, use --pre PRE", "v1.2.3"},
		{"1.2.3+build", "1.2.3 does not have a pre-release, use --pre PRE", "1.2.3"}, // build stripped from msg
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", c.in, err)
		}
		_, err = v.Bump("pre", BumpOptions{})
		if err == nil {
			t.Errorf("Bump(%q, pre) expected error, got nil", c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("Bump(%q, pre) error = %q, want substring %q", c.in, err.Error(), c.wantSub)
		}
	}
}

// ----------------------------------------------------------------------
// TestSpec_JSONOutput: DR-0007 § "確定スキーマ" + § "pre_id / pre_rest の分割定義"
//
// Each row exercises Version.ToJSON for one input shape from the DR's
// example tables, verifying:
//   - version  (input format preserved: prefix + body sep)
//   - semver   (strict form: prefix removed, body sep normalised to ".")
//   - major / minor / patch
//   - pre / pre_id / pre_rest (split at first '.', or null)
//   - build_metadata / build_id / build_rest (same rule)
//
// ----------------------------------------------------------------------
func TestSpec_JSONOutput(t *testing.T) {
	t.Parallel()
	type tc struct {
		in            string
		wantVersion   string
		wantSemver    string
		wantMajor     int
		wantMinor     int
		wantPatch     int
		wantPre       *string
		wantPreID     *string
		wantPreRest   *string
		wantBuild     *string
		wantBuildID   *string
		wantBuildRest *string
	}
	sp := func(s string) *string { return &s }
	cases := []tc{
		// Plain version: pre/build all null.
		{
			in: "1.2.3", wantVersion: "1.2.3", wantSemver: "1.2.3",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
		},
		// Pre-release decomposition (DR-0007 example table).
		{
			in: "1.2.3-rc.1", wantVersion: "1.2.3-rc.1", wantSemver: "1.2.3-rc.1",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("rc.1"), wantPreID: sp("rc"), wantPreRest: sp("1"),
		},
		{
			in: "1.2.3-alpha.beta.5", wantVersion: "1.2.3-alpha.beta.5", wantSemver: "1.2.3-alpha.beta.5",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("alpha.beta.5"), wantPreID: sp("alpha"), wantPreRest: sp("beta.5"),
		},
		{
			in: "1.2.3-alpha", wantVersion: "1.2.3-alpha", wantSemver: "1.2.3-alpha",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("alpha"), wantPreID: sp("alpha"), wantPreRest: nil,
		},
		{
			in: "1.2.3-rc1", wantVersion: "1.2.3-rc1", wantSemver: "1.2.3-rc1",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("rc1"), wantPreID: sp("rc1"), wantPreRest: nil,
		},
		{
			in: "1.2.3-0", wantVersion: "1.2.3-0", wantSemver: "1.2.3-0",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("0"), wantPreID: sp("0"), wantPreRest: nil,
		},
		{
			in: "1.2.3-0.3.7", wantVersion: "1.2.3-0.3.7", wantSemver: "1.2.3-0.3.7",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("0.3.7"), wantPreID: sp("0"), wantPreRest: sp("3.7"),
		},
		// Build metadata follows the same rule.
		{
			in: "1.2.3+build.42", wantVersion: "1.2.3+build.42", wantSemver: "1.2.3+build.42",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantBuild: sp("build.42"), wantBuildID: sp("build"), wantBuildRest: sp("42"),
		},
		{
			in: "1.2.3+build", wantVersion: "1.2.3+build", wantSemver: "1.2.3+build",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantBuild: sp("build"), wantBuildID: sp("build"), wantBuildRest: nil,
		},
		// Combined pre + build.
		{
			in: "1.2.3-rc.1+build.42", wantVersion: "1.2.3-rc.1+build.42", wantSemver: "1.2.3-rc.1+build.42",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("rc.1"), wantPreID: sp("rc"), wantPreRest: sp("1"),
			wantBuild: sp("build.42"), wantBuildID: sp("build"), wantBuildRest: sp("42"),
		},
		// Prefix + body sep: version preserves, semver normalises (DR-0007 § 2).
		{
			in: "v1.2.3", wantVersion: "v1.2.3", wantSemver: "1.2.3",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
		},
		{
			in: "version_1_2_3", wantVersion: "version_1_2_3", wantSemver: "1.2.3",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
		},
		{
			// prefix `v_` + body sep `.` + pre + build — covers the full
			// surface area (the DR's headline example uses this shape).
			in: "v_1.2.3-rc.1+build.42", wantVersion: "v_1.2.3-rc.1+build.42", wantSemver: "1.2.3-rc.1+build.42",
			wantMajor: 1, wantMinor: 2, wantPatch: 3,
			wantPre: sp("rc.1"), wantPreID: sp("rc"), wantPreRest: sp("1"),
			wantBuild: sp("build.42"), wantBuildID: sp("build"), wantBuildRest: sp("42"),
		},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", c.in, err)
		}
		got := v.ToJSON(nil)
		if got.Version != c.wantVersion {
			t.Errorf("%q: Version = %q, want %q", c.in, got.Version, c.wantVersion)
		}
		if got.Semver != c.wantSemver {
			t.Errorf("%q: Semver = %q, want %q", c.in, got.Semver, c.wantSemver)
		}
		if got.Major != c.wantMajor || got.Minor != c.wantMinor || got.Patch != c.wantPatch {
			t.Errorf("%q: M.m.p = %d.%d.%d, want %d.%d.%d", c.in,
				got.Major, got.Minor, got.Patch, c.wantMajor, c.wantMinor, c.wantPatch)
		}
		checkOptStr(t, c.in, "pre", got.Pre, c.wantPre)
		checkOptStr(t, c.in, "pre_id", got.PreID, c.wantPreID)
		checkOptStr(t, c.in, "pre_rest", got.PreRest, c.wantPreRest)
		checkOptStr(t, c.in, "build_metadata", got.BuildMetadata, c.wantBuild)
		checkOptStr(t, c.in, "build_id", got.BuildID, c.wantBuildID)
		checkOptStr(t, c.in, "build_rest", got.BuildRest, c.wantBuildRest)
		// Name nil by default: VER-origin parses don't carry a name.
		if got.Name != nil {
			t.Errorf("%q: Name = %v, want nil (VER origin)", c.in, *got.Name)
		}
	}
}

// checkOptStr compares two *string values for equality, distinguishing
// nil (= JSON null) from an empty string. Errors include the input
// label and field name so failure output points at the offending row.
func checkOptStr(t *testing.T, in, field string, got, want *string) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		// match
	case got == nil && want != nil:
		t.Errorf("%q: %s = nil, want %q", in, field, *want)
	case got != nil && want == nil:
		t.Errorf("%q: %s = %q, want nil", in, field, *got)
	case *got != *want:
		t.Errorf("%q: %s = %q, want %q", in, field, *got, *want)
	}
}

// ----------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// normalizeCmp normalizes a Compare result to one of -1, 0, 1.
func normalizeCmp(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return +1
	default:
		return 0
	}
}
