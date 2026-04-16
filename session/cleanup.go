package session

import (
	"bytes"
	"claude-squad/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// KillWorktreeProcesses finds and terminates any processes whose command-line
// arguments reference the given worktree path. This catches orphaned child
// processes (dev servers, file watchers, esbuild) that outlive their parent
// Claude session.
func KillWorktreeProcesses(worktreePath string) {
	if worktreePath == "" {
		return
	}

	cmd := exec.Command("pgrep", "-f", worktreePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		// pgrep exits 1 when no matches found — that's fine
		return
	}

	myPID := os.Getpid()
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		// Don't kill ourselves
		if pid == myPID {
			continue
		}
		pids = append(pids, pid)
	}

	if len(pids) == 0 {
		return
	}

	log.InfoLog.Printf("cleanup: killing %d orphaned processes for worktree %s", len(pids), worktreePath)

	// Send SIGTERM first
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			log.InfoLog.Printf("cleanup: SIGTERM pid %d: %v", pid, err)
		}
	}

	// Wait 3 seconds for graceful shutdown
	time.Sleep(3 * time.Second)

	// SIGKILL survivors
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		// Check if still alive
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			continue // already dead
		}
		log.InfoLog.Printf("cleanup: SIGKILL pid %d (didn't exit after SIGTERM)", pid)
		proc.Signal(syscall.SIGKILL)
	}
}
