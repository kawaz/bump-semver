//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// socketStdin returns one end of a unix socketpair as *os.File. The other
// end is kept open for the test's lifetime (closed via t.Cleanup), modeling
// an agent harness / CI runner that wires a socket to stdin and never sends
// data nor closes it.
func socketStdin(t *testing.T) *os.File {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	read := os.NewFile(uintptr(fds[0]), "stdin-socket")
	peer := os.NewFile(uintptr(fds[1]), "stdin-socket-peer")
	t.Cleanup(func() {
		_ = read.Close()
		_ = peer.Close()
	})
	return read
}

func TestIsStdinPipe_SocketExcluded(t *testing.T) {
	if isStdinPipe(socketStdin(t)) {
		t.Error("isStdinPipe must be false for a socket stdin (open sockets never EOF; reading would hang)")
	}
}

// A socket wired to stdin that stays open without delivering data must not
// make the single-FILE shortcut block in io.ReadAll: the on-disk file is
// read instead and --write works. Regression guard for the 2-hour hang seen
// under an agent harness whose background shell wires stdin to a socket.
func TestRun_SocketStdin_ReadsDiskAndDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	onDisk := "[package]\nname = \"x\"\nversion = \"1.2.3\"\n"
	if err := os.WriteFile(path, []byte(onDisk), 0644); err != nil {
		t.Fatal(err)
	}

	stdin := socketStdin(t)

	done := make(chan error, 1)
	var stdout, stderr bytes.Buffer
	go func() {
		done <- run([]string{"minor", path, "--write", "--no-hint"}, stdin, &stdout, &stderr)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run error: %v\nstderr: %s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run blocked on socket stdin (must fall through to the on-disk file)")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "version = \"1.3.0\"") {
		t.Errorf("on-disk file not bumped; content:\n%s", got)
	}
}
