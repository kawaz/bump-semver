package main

import (
	"strings"
	"testing"
)

func TestResolveCmdInput_Success(t *testing.T) {
	cases := []struct {
		arg      string
		expected string
	}{
		{"cmd:echo v0.15.0", "0.15.0"},
		{"cmd:echo 1.2.3", "1.2.3"},
		{"cmd:echo 1.2.3-rc.1", "1.2.3-rc.1"},
		{"cmd:echo 2.0.0+build.42", "2.0.0+build.42"},
		{"cmd:printf 'v0.1.0\\n'", "0.1.0"},
		// 複数行出力 → 1 行目を採用、leading whitespace は trim
		{"cmd:printf '   v0.5.0  \\nignored\\n'", "0.5.0"},
		// 空行スキップして最初の non-empty 行
		{"cmd:printf '\\n\\nv0.7.0\\n'", "0.7.0"},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			ri, err := resolveCmdInput(tc.arg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := ri.fields[0].Value; got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
			if !strings.HasPrefix(ri.originFile, "cmd:") {
				t.Errorf("origin %q should start with cmd:", ri.originFile)
			}
		})
	}
}

func TestResolveCmdInput_Failure(t *testing.T) {
	cases := []struct {
		name      string
		arg       string
		wantInErr string
	}{
		{"empty command", "cmd:", "non-empty"},
		{"non-version output", "cmd:echo not-a-version", "is not a valid version"},
		{"empty output", "cmd:true", "no output"},
		{"command failure", "cmd:false", "exit status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveCmdInput(tc.arg)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantInErr)
			}
		})
	}
}
