package session

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestKillWorktreeProcesses(t *testing.T) {
	// Create a temp dir to act as a fake worktree
	tmpDir := t.TempDir()

	// Spawn a background process that references the worktree in its args
	cmd := exec.Command("bash", "-c", "while true; do sleep 1; done # "+tmpDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start background process: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Verify process is running
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("process not running: %v", err)
	}

	// Start waiting for the process to exit in the background so it doesn't
	// become a zombie after being killed (zombies still respond to Signal(0)).
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	// Kill worktree processes
	KillWorktreeProcesses(tmpDir)

	// Wait for the process to actually exit (reaping the zombie).
	select {
	case <-waitDone:
		// process exited — success
	case <-time.After(2 * time.Second):
		t.Error("expected process to be killed within 2s, but it is still running")
	}
}

func TestKillWorktreeProcesses_NoMatch(t *testing.T) {
	// Should not panic or error with a path that matches nothing
	KillWorktreeProcesses("/nonexistent/worktree/path/that/matches/nothing")
}
