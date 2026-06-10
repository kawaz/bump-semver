//go:build !unix

package main

import (
	"os"
	"syscall"
)

// Non-unix (Windows) fallback: process groups via Setpgid and negative-PID
// signalling are POSIX-only. Returning nil here leaves exec.CommandContext's
// default behaviour in place (kill the direct child only). Killing a whole
// process tree on Windows requires Job Objects, which is out of scope for this
// fix; grandchild orphans can still linger there.
//
// Design rationale: unix is the release target where `cmd:` pipelines are run
// in CI; degrading to direct-child kill on Windows keeps the build portable
// without pulling in a Windows-specific process-tree teardown.
func sysProcAttrSetpgid() *syscall.SysProcAttr {
	return nil
}

// killProcessGroup falls back to killing just the direct child on non-unix.
func killProcessGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = p.Kill()
	return nil
}
