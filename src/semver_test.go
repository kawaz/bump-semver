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
		// Separator variants
		{"1_2_3", Version{Sep: "_", Major: 1, Minor: 2, Patch: 3}},
		{"1-2-3", Version{Sep: "-", Major: 1, Minor: 2, Patch: 3}},
		{"v1_2_3", Version{Prefix: "v", Sep: "_", Major: 1, Minor: 2, Patch: 3}},
		{"v1-2-3", Version{Prefix: "v", Sep: "-", Major: 1, Minor: 2, Patch: 3}},
		{"version_1_2_3", Version{Prefix: "version_", Sep: "_", Major: 1, Minor: 2, Patch: 3}},
	}
	for _, tc := range good {
		got, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
	bad := []string{
		"",
		"1.2",
		"1.2.3.4",
		"1.2.3-alpha",
		"1.2.3+build",
		"1.2.3-alpha.1+build.42",
		"1.2.3 something",
		"1.2.x",
		"01.2.3",
		"1.02.3",
		"-1.2.3",
		"1..3",
		"a.b.c",
		"1.2-3",    // separator 不一致
		"1-2.3",    // separator 不一致
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
		{Version{Prefix: "version-", Sep: "-", Major: 1, Minor: 2, Patch: 3}, "version-1-2-3"},
		// Sep 未指定 (zero value) は "." にフォールバック
		{Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
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
		want   Version
	}{
		{"major", Version{Sep: ".", Major: 2, Minor: 0, Patch: 0}},
		{"minor", Version{Sep: ".", Major: 1, Minor: 3, Patch: 0}},
		{"patch", Version{Sep: ".", Major: 1, Minor: 2, Patch: 4}},
		{"get", Version{Sep: ".", Major: 1, Minor: 2, Patch: 3}},
	}
	for _, tc := range cases {
		got, err := v.Bump(tc.action)
		if err != nil {
			t.Errorf("Bump(%q) error: %v", tc.action, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Bump(%q) = %+v, want %+v", tc.action, got, tc.want)
		}
	}
	if _, err := v.Bump("foo"); err == nil {
		t.Error("Bump(\"foo\") expected error, got nil")
	}
}

func TestBump_PreservesPrefixAndSep(t *testing.T) {
	t.Parallel()
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
		{"ver-1-2-3", "major", "ver-2-0-0"},
		{"1_2_3", "patch", "1_2_4"},
		{"1-2-3", "minor", "1-3-0"},
	}
	for _, tc := range cases {
		v, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", tc.in, err)
			continue
		}
		nv, err := v.Bump(tc.action)
		if err != nil {
			t.Errorf("Bump(%q, %q) error: %v", tc.in, tc.action, err)
			continue
		}
		if got := nv.String(); got != tc.want {
			t.Errorf("%q %s -> %q, want %q", tc.in, tc.action, got, tc.want)
		}
	}
}
