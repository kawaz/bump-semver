package main

import (
	"strings"
	"testing"
	"time"
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
		// whitespace-only command is treated the same as empty (consistent
		// behaviour regardless of how the trailing space slipped in).
		{"whitespace-only command", "cmd:   ", "non-empty"},
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

// A child that takes longer than cmdTimeout must be killed and surface
// a clear timeout error. We override the constant via a wrapper instead
// of using a 30s sleep so the test runs in under a second.
func TestResolveCmdInput_Timeout(t *testing.T) {
	prev := cmdTimeoutOverride
	cmdTimeoutOverride = 200 * time.Millisecond
	defer func() { cmdTimeoutOverride = prev }()

	start := time.Now()
	_, err := resolveCmdInput("cmd:sleep 5")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long (%v); kill path may not be wired", elapsed)
	}
}

// stdout past the per-call cap must not exhaust memory; we still want
// the first non-empty line if it shows up before the truncation point.
func TestResolveCmdInput_StdoutTruncation(t *testing.T) {
	// Emit "1.2.3\n" followed by 200 KiB of zeros — line 1 is well within
	// the cap, so we should still parse 1.2.3 even though the tail is
	// truncated.
	ri, err := resolveCmdInput("cmd:printf '1.2.3\\n'; head -c 200000 /dev/zero")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := ri.fields[0].Value; got != "1.2.3" {
		t.Errorf("got %q, want %q", got, "1.2.3")
	}
}
