package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// ErrCommandTimeout is returned when a command exceeds its per-attempt timeout
// on every retry attempt. Wraps with errors.Is for caller checks.
var ErrCommandTimeout = errors.New("command timed out")

// runCommandWithTimeout runs an arbitrary command (not just git) under a
// per-attempt timeout, making up to `attempts` total attempts. Only timeouts
// trigger a retry — non-timeout failures are returned immediately.
//
// On cancellation, the entire process group is killed (negative PID kill).
// This is the fix for the observed bug where killing the parent git process
// left its ssh child reparented to launchd and still consuming the network.
//
// For remote executors (SSH-to-remote) this falls back to the regular
// runGitCommand path with no timeout — remote timeout semantics are out of
// scope for v1.
func (g *GitWorktree) runCommandWithTimeout(
	path string,
	perAttempt time.Duration,
	attempts int,
	name string,
	args ...string,
) (string, error) {
	exec := g.getExecutor()
	switch exec.(type) {
	case *RemoteExecutor:
		// Fall through to remote: no timeout protection in v1.
		out, err := exec.Run(path, name, args...)
		return string(out), err
	case *LocalExecutor:
		// Use direct os/exec with process-group kill support (the normal path).
	default:
		// Unknown executor (e.g., test stubs) — delegate without timeout.
		out, err := exec.Run(path, name, args...)
		return string(out), err
	}
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		out, err := runOneAttempt(path, perAttempt, name, args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
		// Only retry on timeout. Non-timeout failures (e.g., git complaining about
		// a missing ref) won't be fixed by retrying.
		if !errors.Is(err, ErrCommandTimeout) {
			return out, err
		}
	}
	return "", lastErr
}

func runOneAttempt(dir string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Put the child in its own process group so we can kill the whole tree
	// (including grandchildren like ssh) by signalling -pgid.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// WaitDelay caps the pipe-drain phase that follows Cancel. Without it,
	// CombinedOutput reads until EOF — if a grandchild survives SIGKILL (e.g.,
	// uninterruptible kernel wait) the parent can still hang on the pipe.
	// The +500ms headroom avoids false positives on slow machines.
	cmd.WaitDelay = timeout + 500*time.Millisecond
	// We return nil from Cancel so the subsequent Wait/CombinedOutput error is the
	// context error (DeadlineExceeded). The caller below distinguishes timeouts via
	// ctx.Err(), not the returned err.
	cmd.Cancel = func() error {
		// Negative PID = signal the entire process group.
		// We use SIGKILL because git/ssh sometimes ignore SIGTERM during network ops.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("%w: %s %v after %s", ErrCommandTimeout, name, args, timeout)
	}
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}
