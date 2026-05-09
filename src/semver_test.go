package main

import "testing"

func TestParseVersion(t *testing.T) {
	t.Parallel()
	good := []struct {
		in   string
		want Version
	}{
		{"0.0.0", Version{0, 0, 0}},
		{"1.2.3", Version{1, 2, 3}},
		{"10.20.30", Version{10, 20, 30}},
		{"  1.2.3  ", Version{1, 2, 3}},
		{"1.2.3\n", Version{1, 2, 3}},
	}
	for _, tc := range good {
		got, err := ParseVersion(tc.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	bad := []string{
		"",
		"1.2",
		"1.2.3.4",
		"1.2.3-alpha",
		"1.2.3+build",
		"1.2.3-alpha.1+build.42",
		"v1.2.3",
		"1.2.x",
		"01.2.3",
		"1.02.3",
		"-1.2.3",
		"1..3",
		"a.b.c",
	}
	for _, in := range bad {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) expected error, got nil", in)
		}
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	if got := (Version{1, 2, 3}).String(); got != "1.2.3" {
		t.Errorf("String = %q, want 1.2.3", got)
	}
}

func TestBump(t *testing.T) {
	t.Parallel()
	v := Version{1, 2, 3}
	cases := []struct {
		action string
		want   Version
	}{
		{"major", Version{2, 0, 0}},
		{"minor", Version{1, 3, 0}},
		{"patch", Version{1, 2, 4}},
		{"get", Version{1, 2, 3}},
	}
	for _, tc := range cases {
		got, err := v.Bump(tc.action)
		if err != nil {
			t.Errorf("Bump(%q) error: %v", tc.action, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Bump(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
	if _, err := v.Bump("foo"); err == nil {
		t.Error("Bump(\"foo\") expected error, got nil")
	}
}
