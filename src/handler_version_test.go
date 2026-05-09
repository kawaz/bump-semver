package main

import "testing"

func TestVersionHandler_Inspect(t *testing.T) {
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
		insp, err := (versionHandler{}).Inspect([]byte(tc.in))
		if err != nil {
			t.Errorf("Inspect(%q) error: %v", tc.in, err)
			continue
		}
		if len(insp.Versions) != 1 || insp.Versions[0].Value != tc.want {
			t.Errorf("Inspect(%q).Versions = %+v, want one Version with Value=%q", tc.in, insp.Versions, tc.want)
		}
		if len(insp.Names) != 0 {
			t.Errorf("Inspect(%q).Names should be empty, got %+v", tc.in, insp.Names)
		}
	}
	if _, err := (versionHandler{}).Inspect([]byte("   \n")); err == nil {
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
		got, err := (versionHandler{}).Replace([]byte(tc.in), "0.0.0", "1.0.0")
		if err != nil {
			t.Errorf("Replace(%q) error: %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("Replace(%q) = %q, want %q", tc.in, string(got), tc.want)
		}
	}
}
