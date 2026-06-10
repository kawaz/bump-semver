// Package main: `cmd:<shell-command>` input source.
//
// Read-only input that executes `<shell-command>` via `bash -c`, takes the
// first non-empty line of stdout, strips an optional leading `v`, and parses
// it as SemVer. The use case is comparing bin-embedded version strings
// (e.g. `cmd:pkf run run -- --version`) against the same version files at
// release time, so a forgotten `go build` after `bump-version` is detected.
//
// Design:
//   - Same precedence slot as `vcs:` (read-only, rejected by `--write`).
//   - Origin label is the literal `cmd:<command>` so error messages /
//     `bump-semver compare` output preserve the round-trip exactly.
//   - Errors from the child process include its stderr to make debug easy.
//   - The child has a hard 30s timeout; stdout is bounded at 64 KiB and
//     stderr at 4 KiB. This is a defensive boundary against a misbehaving
//     or malicious version-emitting command — we only need the first
//     SemVer-shaped line, so anything past that is noise to ignore.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	// cmdTimeoutDefault bounds wall-clock execution of a `cmd:` child.
	// Picked generously enough for `bin --version` style invocations on a
	// cold runner (where dynamic linkers / JIT warm-up can add seconds)
	// while still preventing a hung command from blocking a release pipeline.
	cmdTimeoutDefault = 30 * time.Second
	// cmdStdoutLimit caps stdout bytes. We only need the first non-empty
	// line of a SemVer-shaped version; anything past this is either
	// pathological output (cat /dev/urandom) or a wrong command, and
	// either way buffering the whole thing wastes memory.
	cmdStdoutLimit = 64 * 1024
	// cmdStderrLimit caps stderr bytes used to compose the error message
	// on child failure. Truncation is reported.
	cmdStderrLimit = 4 * 1024
)

// cmdTimeoutOverride is set by tests to compress the wait, kept here so
// production code references one effective-value lookup. Defaults to 0
// which means "use cmdTimeoutDefault".
var cmdTimeoutOverride time.Duration

// resolveCmdInput handles `cmd:<shell-command>` inputs by executing the
// command and parsing the first non-empty stdout line as SemVer. The
// command runs via `bash -c` so the user can use pipes, redirects, env
// expansion, etc. without quoting tricks.
//
// Leading "v" is stripped from the output line (e.g. `v0.15.0` → `0.15.0`)
// so bin --version outputs that follow the common `v<semver>` convention
// just work.
func resolveCmdInput(arg string) (resolvedInput, error) {
	cmdStr := strings.TrimPrefix(arg, "cmd:")
	if strings.TrimSpace(cmdStr) == "" {
		return resolvedInput{}, fmt.Errorf("cmd: requires a non-empty shell command")
	}

	timeout := cmdTimeoutDefault
	if cmdTimeoutOverride > 0 {
		timeout = cmdTimeoutOverride
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	// Put the child in its own process group and, on ctx expiry, kill the
	// whole group instead of just the direct child. `bash -c "a | b"` and
	// other pipelines/subshells fork grandchildren that exec.CommandContext's
	// default kill (direct child only) would orphan — they survive as leaks
	// and, worse, keep stdout/stderr fds open so c.Wait() blocks until they
	// exit on their own. Group-kill plus WaitDelay closes both holes.
	c.SysProcAttr = sysProcAttrSetpgid()
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return killProcessGroup(c.Process.Pid)
	}
	// After Cancel runs, give the process group a brief window to release the
	// stdout/stderr fds; if any descendant somehow survives, WaitDelay forces
	// c.Wait() to return rather than hang on the inherited pipe.
	c.WaitDelay = 100 * time.Millisecond
	// Truncated buffers via io.MultiWriter + a counting wrapper.
	stdoutBuf := &limitedBuffer{cap: cmdStdoutLimit}
	stderrBuf := &limitedBuffer{cap: cmdStderrLimit}
	c.Stdout = stdoutBuf
	c.Stderr = stderrBuf

	runErr := c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return resolvedInput{}, fmt.Errorf("cmd:%s: timed out after %s", cmdStr, timeout)
	}
	if runErr != nil {
		stderrText := strings.TrimSpace(stderrBuf.String())
		if stderrBuf.truncated {
			stderrText += " (stderr truncated)"
		}
		if stderrText != "" {
			return resolvedInput{}, fmt.Errorf("cmd:%s: %w (stderr: %s)", cmdStr, runErr, stderrText)
		}
		return resolvedInput{}, fmt.Errorf("cmd:%s: %w", cmdStr, runErr)
	}

	// Take the first non-empty line of stdout (Scanner handles CRLF on
	// the trim below).
	scanner := bufio.NewScanner(bytes.NewReader(stdoutBuf.Bytes()))
	scanner.Buffer(make([]byte, 0, 64*1024), cmdStdoutLimit)
	var line string
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		if l != "" {
			line = l
			break
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return resolvedInput{}, fmt.Errorf("cmd:%s: read stdout: %w", cmdStr, err)
	}
	if line == "" {
		return resolvedInput{}, fmt.Errorf("cmd:%s: command produced no output", cmdStr)
	}
	line = strings.TrimPrefix(line, "v") // tolerate leading "v" in version strings

	v, err := ParseVersion(line)
	if err != nil {
		return resolvedInput{}, fmt.Errorf("cmd:%s: %q is not a valid version: %w", cmdStr, line, err)
	}

	label := "cmd:" + cmdStr
	ri := resolvedInput{originFile: label}
	ri.fields = []locatedField{{File: ri.originFile, Value: v.String()}}
	return ri, nil
}

// limitedBuffer is an io.Writer that records up to `cap` bytes and
// silently discards the rest, marking `truncated` when overflow happens.
// We don't want to fail the command run on overflow — bash itself
// happily writes to a pipe past any cap — we just want to bound what
// bump-semver itself holds in memory.
type limitedBuffer struct {
	buf       bytes.Buffer
	cap       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.cap - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil // pretend we wrote it all so the child keeps running
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string { return b.buf.String() }
func (b *limitedBuffer) Bytes() []byte  { return b.buf.Bytes() }
