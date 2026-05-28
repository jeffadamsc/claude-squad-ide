package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// helperScriptPath writes a small bash script into a temp dir and returns its path.
// It also chmods the script to be executable.
func helperScriptPath(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper script: %v", err)
	}
	return p
}

// processAlive returns true if the given pid is still alive.
// We use signal 0 which performs error-checking but doesn't actually deliver a signal.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func TestRunCommandWithTimeout_SuccessNoTimeout(t *testing.T) {
	gw := &GitWorktree{}
	out, err := gw.runCommandWithTimeout("", 2*time.Second, 1, "echo", "ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected output to contain 'ok', got %q", out)
	}
}

func TestRunCommandWithTimeout_TimesOut(t *testing.T) {
	gw := &GitWorktree{}
	_, err := gw.runCommandWithTimeout("", 200*time.Millisecond, 1, "sleep", "5")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("expected ErrCommandTimeout, got %v", err)
	}
}

func TestRunCommandWithTimeout_RetryThenSucceed(t *testing.T) {
	// Use a script that sleeps long on first call but exits 0 on second.
	// State is tracked via a marker file in the script's directory.
	// The timeout is intentionally generous (2s) — this test exercises retry
	// semantics, not timeout precision. A tighter timeout flakes on busy
	// machines where bash startup + filesystem access can blow past 200ms.
	marker := filepath.Join(t.TempDir(), "marker")
	script := helperScriptPath(t, "flaky.sh", `#!/usr/bin/env bash
if [ -f '`+marker+`' ]; then
  exit 0
fi
touch '`+marker+`'
sleep 30
`)
	gw := &GitWorktree{}
	_, err := gw.runCommandWithTimeout("", 2*time.Second, 2, script)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
}

func TestRunCommandWithTimeout_AllAttemptsFail(t *testing.T) {
	gw := &GitWorktree{}
	_, err := gw.runCommandWithTimeout("", 100*time.Millisecond, 3, "sleep", "5")
	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("expected ErrCommandTimeout, got %v", err)
	}
}

func TestRunCommandWithTimeout_ClampsAttemptsBelowOne(t *testing.T) {
	gw := &GitWorktree{}
	out, err := gw.runCommandWithTimeout("", 2*time.Second, 0, "echo", "clamped")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "clamped") {
		t.Fatalf("expected output to contain 'clamped', got %q", out)
	}
}

func TestRunCommandWithTimeout_KillsChildProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill is POSIX-only")
	}
	// Spawn a script that backgrounds a long sleep and prints its PID,
	// then itself sleeps until killed. When the parent is killed by timeout,
	// the backgrounded sleep should also die because Setpgid put both in
	// the same process group.
	out := filepath.Join(t.TempDir(), "child.pid")
	script := helperScriptPath(t, "spawn.sh", `#!/usr/bin/env bash
sleep 30 &
echo -n $! > '`+out+`'
wait
`)
	gw := &GitWorktree{}
	_, err := gw.runCommandWithTimeout("", 300*time.Millisecond, 1, script)
	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("expected ErrCommandTimeout, got %v", err)
	}

	pidBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	var childPid int
	if _, err := fmt.Sscan(string(pidBytes), &childPid); err != nil {
		t.Fatalf("parse child pid %q: %v", pidBytes, err)
	}

	// Give the kernel a brief moment to deliver SIGKILL and reap the child.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(childPid) {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("backgrounded child pid %d still alive after parent timeout", childPid)
}
