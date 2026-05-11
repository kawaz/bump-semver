package main

import "testing"

func TestParseVersion(t *testing.T) {
	t.Parallel()
	good := []struct {
		in   string
		want Version
	}{
		// Plain X.Y.Z
		{"0.0.0", Version{Sep: ".", Major: 0, Minor: 0, Patch: 0}},
		{"1.2.3", Version{Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"10.20.30", Version{Sep: ".", Major: 10, Minor: 20, Patch: 30}},
		{"  1.2.3  ", Version{Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"1.2.3\n", Version{Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		// Prefix variants
		{"v1.2.3", Version{Prefix: "v", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"ver1.2.3", Version{Prefix: "ver", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"version1.2.3", Version{Prefix: "version", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"v_1.2.3", Version{Prefix: "v_", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"v.1.2.3", Version{Prefix: "v.", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"v-1.2.3", Version{Prefix: "v-", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"ver-1.2.3", Version{Prefix: "ver-", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		{"version_1.2.3", Version{Prefix: "version_", Sep: ".", Major: 1, Minor: 2, Patch: 3}},
		// Separator variants (DR-0006: body sep `-` no longer allowed,
		// only `.` and `_`; `-` collides with pre-release).
		{"1_2_3", Version{Sep: "_", Major: 1, Minor: 2, Patch: 3}},
		{"v1_2_3", Version{Prefix: "v", Sep: "_", Major: 1, Minor: 2, Patch: 3}},
		{"version_1_2_3", Version{Prefix: "version_", Sep: "_", Major: 1, Minor: 2, Patch: 3}},
	}
	for _, tc := range good {
		got, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got.Prefix != tc.want.Prefix || got.Sep != tc.want.Sep ||
			got.Major != tc.want.Major || got.Minor != tc.want.Minor ||
			got.Patch != tc.want.Patch ||
			!equalSS(got.Pre, tc.want.Pre) || !equalSS(got.BuildMetadata, tc.want.BuildMetadata) {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
	bad := []string{
		"",
		"1.2",
		"1.2.3.4",
		// 1.2.3-alpha / 1.2.3+build / 1.2.3-alpha.1+build.42 are now
		// VALID (DR-0006: SemVer 2.0.0 pre-release / build metadata
		// support). See spec_table_test.go.
		"1.2.3 something",
		"1.2.x",
		"01.2.3",
		"1.02.3",
		"-1.2.3",
		"1..3",
		"a.b.c",
		"1.2-3",    // separator 不一致 / `-` no longer body sep
		"1-2.3",    // separator 不一致 / `-` no longer body sep
		"1-2-3",    // body sep `-` no longer allowed (DR-0006)
		"v1-2-3",   // body sep `-` no longer allowed
		"foo1.2.3", // prefix が "foo" は不許可
		"1.2.3v",   // 末尾に余計な文字
		"V1.2.3",   // case-sensitive: 大文字 V は対象外 (regex は小文字 v のみ)
	}
	for _, in := range bad {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) expected error, got nil", in)
		}
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v    Version
		want string
	}{
		{Version{Sep: ".", Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{Version{Prefix: "v", Sep: ".", Major: 1, Minor: 2, Patch: 3}, "v1.2.3"},
		{Version{Prefix: "v_", Sep: "_", Major: 1, Minor: 2, Patch: 3}, "v_1_2_3"},
		// Sep 未指定 (zero value) は "." にフォールバック
		{Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		// Pre-release / build metadata (DR-0006)
		{Version{Sep: ".", Major: 1, Minor: 2, Patch: 3, Pre: []string{"alpha"}}, "1.2.3-alpha"},
		{Version{Sep: ".", Major: 1, Minor: 2, Patch: 3, Pre: []string{"rc", "1"}}, "1.2.3-rc.1"},
		{Version{Sep: ".", Major: 1, Minor: 2, Patch: 3, BuildMetadata: []string{"build", "42"}}, "1.2.3+build.42"},
		{Version{Sep: ".", Major: 1, Minor: 2, Patch: 3, Pre: []string{"rc", "1"}, BuildMetadata: []string{"sha", "abc"}}, "1.2.3-rc.1+sha.abc"},
		{Version{Prefix: "v", Sep: ".", Major: 1, Minor: 2, Patch: 3, Pre: []string{"alpha"}}, "v1.2.3-alpha"},
	}
	for _, tc := range cases {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("(%+v).String() = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestBump(t *testing.T) {
	t.Parallel()
	v := Version{Major: 1, Minor: 2, Patch: 3, Sep: "."}
	cases := []struct {
		action string
		want   string
	}{
		{"major", "2.0.0"},
		{"minor", "1.3.0"},
		{"patch", "1.2.4"},
		{"get", "1.2.3"},
	}
	for _, tc := range cases {
		got, err := v.Bump(tc.action, BumpOptions{})
		if err != nil {
			t.Errorf("Bump(%q) error: %v", tc.action, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("Bump(%q) = %q, want %q", tc.action, got.String(), tc.want)
		}
	}
	if _, err := v.Bump("foo", BumpOptions{}); err == nil {
		t.Error("Bump(\"foo\") expected error, got nil")
	}
}

func TestBump_PreservesPrefixAndSep(t *testing.T) {
	t.Parallel()
	// Body sep `-` is no longer supported (DR-0006); only `.` and `_`.
	cases := []struct {
		in     string
		action string
		want   string
	}{
		{"v1.2.3", "patch", "v1.2.4"},
		{"v1.2.3", "minor", "v1.3.0"},
		{"v1.2.3", "major", "v2.0.0"},
		{"version_1_2_3", "patch", "version_1_2_4"},
		{"version_1_2_3", "minor", "version_1_3_0"},
		{"ver_1_2_3", "major", "ver_2_0_0"},
		{"1_2_3", "patch", "1_2_4"},
	}
	for _, tc := range cases {
		v, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", tc.in, err)
			continue
		}
		nv, err := v.Bump(tc.action, BumpOptions{})
		if err != nil {
			t.Errorf("Bump(%q, %q) error: %v", tc.in, tc.action, err)
			continue
		}
		if got := nv.String(); got != tc.want {
			t.Errorf("%q %s -> %q, want %q", tc.in, tc.action, got, tc.want)
		}
	}
}

// TestParseVersion_PreRelease focuses on pre-release-only details (the
// table-driven spec test covers the cross-product). Goal: pin
// identifier-level decomposition.
func TestParseVersion_PreRelease(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in  string
		pre []string
	}{
		{"1.2.3-alpha", []string{"alpha"}},
		{"1.2.3-rc.1", []string{"rc", "1"}},
		{"1.2.3-0.3.7", []string{"0", "3", "7"}},
		{"1.2.3-x.7.z.92", []string{"x", "7", "z", "92"}},
		{"1.2.3-x-y", []string{"x-y"}},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", c.in, err)
			continue
		}
		if !equalSS(v.Pre, c.pre) {
			t.Errorf("ParseVersion(%q).Pre = %#v, want %#v", c.in, v.Pre, c.pre)
		}
	}
}

// TestParseVersion_BuildMetadata focuses on build-metadata-only
// details. Build metadata IS allowed leading zeros (SemVer 2.0.0 § 10).
func TestParseVersion_BuildMetadata(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		build []string
	}{
		{"1.2.3+build", []string{"build"}},
		{"1.2.3+0", []string{"0"}},
		{"1.2.3+001", []string{"001"}}, // leading zero OK in build metadata
		{"1.2.3+exp.sha.5114f85", []string{"exp", "sha", "5114f85"}},
		{"1.2.3+x-y", []string{"x-y"}},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", c.in, err)
			continue
		}
		if !equalSS(v.BuildMetadata, c.build) {
			t.Errorf("ParseVersion(%q).BuildMetadata = %#v, want %#v", c.in, v.BuildMetadata, c.build)
		}
	}
}

// TestVersion_Compare exercises the SemVer 2.0.0 § 11 ordering rules
// from a different angle than the spec table: it focuses on the
// "exotic" cases (build-metadata ignored, prefix/sep ignored, shorter
// pre-release wins).
func TestVersion_Compare(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want int
	}{
		// shorter pre-release < longer pre-release with matching prefix
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", +1},
		// numeric vs alphanumeric on first identifier
		{"1.0.0-1", "1.0.0-alpha", -1},
		// build metadata wholly ignored
		{"1.0.0+x", "1.0.0+y", 0},
		{"1.0.0-rc.1+a", "1.0.0-rc.1+b", 0},
		// prefix/sep wholly ignored
		{"v1.2.3", "1.2.3", 0},
		{"version_1_2_3", "v1.2.3", 0},
		{"v_1.2.3", "1_2_3", 0},
	}
	for _, c := range cases {
		va, err := ParseVersion(c.a)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", c.a, err)
		}
		vb, err := ParseVersion(c.b)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", c.b, err)
		}
		got := va.Compare(vb)
		if normalize(got) != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestVersion_CompareAt pins DR-0017 precision-aware comparison.
// Precision "" must agree with Compare; "major"/"minor"/"patch"
// truncate the comparison and ignore lower components (including
// pre-release at "patch" precision).
func TestVersion_CompareAt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		a, b      string
		precision string
		want      int
	}{
		// precision = "" matches Compare for all cases.
		{"full-eq", "1.2.3", "1.2.3", "", 0},
		{"full-lt-patch", "1.2.3", "1.2.4", "", -1},
		{"full-pre-lt-release", "1.2.3-rc.1", "1.2.3", "", -1},

		// "major": only X is compared, everything else is ignored.
		{"major-eq-diff-minor", "1.2.3", "1.9.7", "major", 0},
		{"major-eq-diff-patch", "1.0.0", "1.0.99", "major", 0},
		{"major-eq-with-pre", "1.0.0", "1.99.99-rc.1", "major", 0},
		{"major-lt", "1.9.9", "2.0.0", "major", -1},
		{"major-lt-with-pre-on-bigger", "1.9.9", "2.0.0-rc.0", "major", -1},
		{"major-gt", "2.0.0", "1.9.9", "major", +1},

		// "minor": X and Y; Z and pre-release ignored.
		{"minor-eq-diff-patch", "1.2.3", "1.2.9", "minor", 0},
		{"minor-eq-with-pre", "1.2.0", "1.2.9-rc.1", "minor", 0},
		{"minor-lt-on-minor", "1.2.9", "1.3.0", "minor", -1},
		{"minor-lt-with-pre", "1.2.9", "1.3.0-rc.0", "minor", -1},
		{"minor-gt-major-wins", "2.0.0", "1.9.9", "minor", +1},
		{"minor-eq-major-diff-minor", "1.2.3", "1.4.0", "minor", -1},

		// "patch": X, Y, Z; pre-release ignored.
		{"patch-eq-pre-vs-release", "1.2.3", "1.2.3-rc.1", "patch", 0},
		{"patch-eq-different-pre", "1.2.3-rc.1", "1.2.3-rc.99", "patch", 0},
		{"patch-lt", "1.2.3", "1.2.4", "patch", -1},
		{"patch-lt-with-pre-on-bigger", "1.2.3", "1.2.4-rc.0", "patch", -1},
		{"patch-gt-pre-on-smaller", "1.2.4-rc.0", "1.2.3", "patch", +1},
		{"patch-eq-build-metadata", "1.2.3+a", "1.2.3+b", "patch", 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			va, err := ParseVersion(c.a)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", c.a, err)
			}
			vb, err := ParseVersion(c.b)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", c.b, err)
			}
			got := va.CompareAt(vb, c.precision)
			if normalize(got) != c.want {
				t.Errorf("CompareAt(%q, %q, %q) = %d, want %d", c.a, c.b, c.precision, got, c.want)
			}
		})
	}
}

// TestBump_PreAction focuses on the pre action's three modes from the
// caller's perspective (counter-advance / overwrite / remove).
func TestBump_PreAction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		opts    BumpOptions
		want    string
		wantErr bool
	}{
		// counter advance
		{in: "1.2.3-rc.0", opts: BumpOptions{}, want: "1.2.3-rc.1"},
		{in: "1.2.3-rc.42", opts: BumpOptions{}, want: "1.2.3-rc.43"},
		{in: "1.2.3-0", opts: BumpOptions{}, want: "1.2.3-1"}, // 確定論点 C
		// counter advance: errors
		{in: "1.2.3", opts: BumpOptions{}, wantErr: true},
		{in: "1.2.3-rc1", opts: BumpOptions{}, wantErr: true},
		{in: "1.2.3-alpha", opts: BumpOptions{}, wantErr: true},
		// --pre PRE overwrite
		{in: "1.2.3", opts: BumpOptions{Pre: "alpha", PreSet: true}, want: "1.2.3-alpha"},
		{in: "1.2.3-rc.5", opts: BumpOptions{Pre: "rc.0", PreSet: true}, want: "1.2.3-rc.0"},
		{in: "1.2.3-alpha", opts: BumpOptions{Pre: "rc.0", PreSet: true}, want: "1.2.3-rc.0"},
		// --no-pre remove
		{in: "1.2.3-rc.0", opts: BumpOptions{NoPre: true}, want: "1.2.3"},
		{in: "1.2.3", opts: BumpOptions{NoPre: true}, want: "1.2.3"},
		// build metadata is dropped on every pre invocation
		{in: "1.2.3-rc.0+build", opts: BumpOptions{}, want: "1.2.3-rc.1"},
		{in: "1.2.3+build", opts: BumpOptions{NoPre: true}, want: "1.2.3"},
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

// equalSS / normalize: local helpers (avoid clashing with helpers in
// spec_table_test.go which use different names).
func equalSS(a, b []string) bool {
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

func normalize(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return +1
	default:
		return 0
	}
}
