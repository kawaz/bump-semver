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
package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

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
	if cmdStr == "" {
		return resolvedInput{}, fmt.Errorf("cmd: requires a non-empty shell command")
	}

	var stdout, stderr bytes.Buffer
	c := exec.Command("bash", "-c", cmdStr)
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return resolvedInput{}, fmt.Errorf("cmd:%s: %w (stderr: %s)", cmdStr, err, stderrText)
		}
		return resolvedInput{}, fmt.Errorf("cmd:%s: %w", cmdStr, err)
	}

	// Take the first non-empty line of stdout.
	var line string
	for _, l := range strings.Split(stdout.String(), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			line = l
			break
		}
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
