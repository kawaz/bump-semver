package main

import "testing"

func TestVersionHandler_Get(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"0.0.0\n", "0.0.0"},
		{"  1.2.3  \n", "1.2.3"},
		{"1.2.3", "1.2.3"},
	}
	for _, tc := range cases {
		got, err := (versionHandler{}).Get([]byte(tc.in))
		if err != nil {
			t.Errorf("Get(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Get(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := (versionHandler{}).Get([]byte("   \n")); err == nil {
		t.Error("expected error for empty content")
	}
}

func TestVersionHandler_Replace(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"0.0.0\n", "1.0.0\n"},
		{"0.0.0", "1.0.0"},
	}
	for _, tc := range cases {
		got, err := (versionHandler{}).Replace([]byte(tc.in), "1.0.0")
		if err != nil {
			t.Errorf("Replace(%q) error: %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("Replace(%q) = %q, want %q", tc.in, string(got), tc.want)
		}
	}
}
