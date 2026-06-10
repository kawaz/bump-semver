//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// On timeout, a `cmd:` child that forked grandchildren (via pipe / subshell /
// multiple commands) must not leak orphaned processes. exec.CommandContext only
// kills the direct child (bash); if bash spawned a grandchild it survives as an
// orphan and keeps running for the rest of its lifetime. resolveCmdInput must
// kill the whole process group so the grandchild dies too.
func TestResolveCmdInput_Timeout_KillsGrandchild(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	prev := cmdTimeoutOverride
	cmdTimeoutOverride = 200 * time.Millisecond
	defer func() { cmdTimeoutOverride = prev }()

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")

	// The pipe forces bash to fork: the left-hand bash records its own PID to a
	// temp file then sleeps 30s; `cat` keeps the pipeline alive. exec only kills
	// the top-level bash, so without group-kill the recorded PID survives.
	arg := "cmd:bash -c 'echo $$ > " + pidFile + "; sleep 30' | cat"

	start := time.Now()
	_, err := resolveCmdInput(arg)
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

	// Read the grandchild PID the inner bash recorded.
	var pid int
	for i := 0; i < 50; i++ { // up to ~500ms for the file to appear
		data, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			if p, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && p > 0 {
				pid = p
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatalf("inner process never recorded its PID to %s", pidFile)
	}

	// Cleanup: make sure we never leak the process regardless of assertion result.
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()

	// Give the kill signal time to propagate, then verify the process is gone.
	// syscall.Kill(pid, 0) returns ESRCH once the process no longer exists.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return // process is gone — group kill worked
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("grandchild pid %d still alive after timeout; process group was not killed", pid)
}
