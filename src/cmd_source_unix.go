//go:build unix

package main

import (
	"errors"
	"syscall"
)

// sysProcAttrSetpgid returns a SysProcAttr that places the child in its own
// process group (pgid == child pid). This lets killProcessGroup reach the
// child *and all of its descendants* (grandchildren forked by `bash -c` via
// pipes / subshells), which exec.CommandContext's default kill cannot do — it
// only signals the direct child.
func sysProcAttrSetpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup SIGKILLs the whole process group led by pid. A negative pid
// targets the process group (pgid == pid, established by Setpgid above), so
// every descendant is killed too. ESRCH (no such process) means the group has
// already exited; that is not an error for our purposes.
func killProcessGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
