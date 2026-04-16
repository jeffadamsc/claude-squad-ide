# Adaptive Memory Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce memory footprint on low-RAM machines by detecting system RAM at startup and scaling GOMEMLIMIT, idle session timeouts, and orphaned process cleanup accordingly.

**Architecture:** New `app/memory.go` detects system RAM and computes scaled limits. `PollAllStatuses()` gains idle-tracking logic that auto-pauses stale sessions. `session/cleanup.go` kills orphaned child processes on pause/kill/startup. `debug.FreeOSMemory()` is called after index builds.

**Tech Stack:** Go stdlib (`runtime/debug`, `syscall`), existing session/indexer infrastructure.

**Spec:** `docs/superpowers/specs/2026-04-16-adaptive-memory-management-design.md`

---

### Task 1: System RAM Detection & Adaptive Limits

**Files:**
- Create: `app/memory.go`
- Create: `app/memory_test.go`

- [ ] **Step 1: Write the failing test for `GetSystemMemoryMB`**

```go
// app/memory_test.go
package app

import "testing"

func TestGetSystemMemoryMB(t *testing.T) {
	mb := GetSystemMemoryMB()
	if mb <= 0 {
		t.Fatalf("expected positive system memory, got %d MB", mb)
	}
	// Any modern machine has at least 2 GB
	if mb < 2048 {
		t.Fatalf("expected at least 2048 MB, got %d MB", mb)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestGetSystemMemoryMB -v`
Expected: FAIL — `GetSystemMemoryMB` undefined.

- [ ] **Step 3: Implement `GetSystemMemoryMB`**

```go
// app/memory.go
package app

import (
	"encoding/binary"
	"syscall"
)

// GetSystemMemoryMB returns the total system RAM in megabytes.
// Falls back to 8192 (8 GB) if detection fails.
func GetSystemMemoryMB() int {
	raw, err := syscall.SysctlRaw("hw.memsize")
	if err != nil || len(raw) < 8 {
		return 8192
	}
	memBytes := binary.LittleEndian.Uint64(raw)
	return int(memBytes / (1024 * 1024))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestGetSystemMemoryMB -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for `ComputeMemoryLimits`**

```go
// app/memory_test.go (append — also add "fmt" to the imports)

func TestComputeMemoryLimits(t *testing.T) {
	tests := []struct {
		ramMB             int
		wantGOMEMLIMIT    int64
		wantIdleTimeout   int // minutes, 0 = disabled
	}{
		{8192, 128 * 1024 * 1024, 15},
		{16384, 256 * 1024 * 1024, 30},
		{32768, 384 * 1024 * 1024, 60},
		{65536, 512 * 1024 * 1024, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dMB", tt.ramMB), func(t *testing.T) {
			limits := ComputeMemoryLimits(tt.ramMB)
			if limits.GOMEMLIMIT != tt.wantGOMEMLIMIT {
				t.Errorf("GOMEMLIMIT: got %d, want %d", limits.GOMEMLIMIT, tt.wantGOMEMLIMIT)
			}
			if limits.IdleTimeoutMinutes != tt.wantIdleTimeout {
				t.Errorf("IdleTimeout: got %d min, want %d min", limits.IdleTimeoutMinutes, tt.wantIdleTimeout)
			}
		})
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestComputeMemoryLimits -v`
Expected: FAIL — `ComputeMemoryLimits` undefined.

- [ ] **Step 7: Implement `ComputeMemoryLimits`**

```go
// app/memory.go (append)

// MemoryLimits holds adaptive configuration derived from system RAM.
type MemoryLimits struct {
	GOMEMLIMIT          int64 // bytes, for debug.SetMemoryLimit
	IdleTimeoutMinutes  int   // 0 = disabled
}

// ComputeMemoryLimits returns scaled memory configuration for the given RAM (in MB).
func ComputeMemoryLimits(ramMB int) MemoryLimits {
	switch {
	case ramMB <= 8192:
		return MemoryLimits{
			GOMEMLIMIT:         128 * 1024 * 1024,
			IdleTimeoutMinutes: 15,
		}
	case ramMB <= 16384:
		return MemoryLimits{
			GOMEMLIMIT:         256 * 1024 * 1024,
			IdleTimeoutMinutes: 30,
		}
	case ramMB <= 32768:
		return MemoryLimits{
			GOMEMLIMIT:         384 * 1024 * 1024,
			IdleTimeoutMinutes: 60,
		}
	default:
		return MemoryLimits{
			GOMEMLIMIT:         512 * 1024 * 1024,
			IdleTimeoutMinutes: 0,
		}
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestComputeMemoryLimits -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add app/memory.go app/memory_test.go
git commit -m "feat(memory): add system RAM detection and adaptive limits"
```

---

### Task 2: Wire Adaptive GOMEMLIMIT into Startup

**Files:**
- Modify: `main.go:40-43` — replace hardcoded limit
- Modify: `app/bindings.go:26-28` — add `IdleTimeout` to `SessionAPIOptions`
- Modify: `app/bindings.go:74-88` — add `idleTimeout` field to `SessionAPI`
- Modify: `main.go:67` — pass limits to `SessionAPIOptions`

- [ ] **Step 1: Add `IdleTimeout` to `SessionAPIOptions` and `SessionAPI`**

In `app/bindings.go`, change `SessionAPIOptions`:

```go
// line 26-28
type SessionAPIOptions struct {
	DataDir      string
	IdleTimeout  time.Duration // 0 = disabled
}
```

Add `idleTimeout` field to `SessionAPI` struct (after line 88):

```go
type SessionAPI struct {
	mu            sync.RWMutex
	ctx           context.Context
	instances     map[string]*session.Instance
	storage       *session.Storage
	ptyManager    *ptyPkg.Manager
	wsServer      *ptyPkg.WebSocketServer
	wsPort        int
	cfg           *config.Config
	dirty         bool
	hostManager   *sshPkg.HostManager
	hostStore     *sshPkg.HostStore
	keychainStore *sshPkg.KeychainStore
	indexers      map[string]Indexer
	mcpServer     *MCPIndexServer
	idleTimeout   time.Duration
}
```

Store `opts.IdleTimeout` in the `NewSessionAPI` constructor. In the `api := &SessionAPI{...}` block (line 116), add:

```go
	idleTimeout:   opts.IdleTimeout,
```

- [ ] **Step 2: Replace hardcoded GOMEMLIMIT in `main.go`**

Replace lines 40-43 in `main.go`:

```go
			// Detect system RAM and scale Go's memory limit accordingly.
			// Lower RAM machines get a tighter limit to encourage aggressive GC
			// and faster return of memory to the OS.
			memLimits := appPkg.ComputeMemoryLimits(appPkg.GetSystemMemoryMB())
			debug.SetMemoryLimit(memLimits.GOMEMLIMIT)
```

Replace line 67:

```go
			idleTimeout := time.Duration(memLimits.IdleTimeoutMinutes) * time.Minute

			api, err := appPkg.NewSessionAPI(appPkg.SessionAPIOptions{
				IdleTimeout: idleTimeout,
			})
```

Add `"time"` to the imports in `main.go`.

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: success (no errors)

- [ ] **Step 4: Run existing tests to verify no regressions**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -v -count=1 2>&1 | tail -20`
Expected: all existing tests PASS

- [ ] **Step 5: Commit**

```bash
git add main.go app/bindings.go
git commit -m "feat(memory): wire adaptive GOMEMLIMIT and idle timeout into startup"
```

---

### Task 3: FreeOSMemory After Index Builds

**Files:**
- Modify: `app/indexer_treesitter.go:173` — add `debug.FreeOSMemory()` at end of `build()`

- [ ] **Step 1: Add `debug.FreeOSMemory()` call**

In `app/indexer_treesitter.go`, find the `build()` method. At the very end (before the final `}`), add:

```go
	// Force Go to return freed heap pages to the OS immediately.
	// Without this, Go holds freed pages indefinitely and macOS
	// swaps them rather than reclaiming them.
	debug.FreeOSMemory()
```

Add `"runtime/debug"` to the imports at the top of the file.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: success

- [ ] **Step 3: Run indexer tests to verify no regressions**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestTreeSitter -v -count=1 2>&1 | tail -20`
Expected: all indexer tests PASS

- [ ] **Step 4: Commit**

```bash
git add app/indexer_treesitter.go
git commit -m "perf(memory): call FreeOSMemory after index builds"
```

---

### Task 4: Add Idle Tracking Fields to Instance

**Files:**
- Modify: `session/instance.go:42-93` — add `IdleSince`, `LastViewed`, `AutoPaused` fields
- Modify: `session/instance_test.go` — add tests for idle tracking

- [ ] **Step 1: Write the failing test for idle tracking**

```go
// session/instance_test.go (append)

func TestIdleTracking(t *testing.T) {
	inst := &Instance{
		started: true,
		inPlace: true,
	}

	// Initially not idle
	if !inst.IdleSince.IsZero() {
		t.Error("expected IdleSince to be zero initially")
	}

	// Mark idle
	inst.MarkIdle()
	if inst.IdleSince.IsZero() {
		t.Error("expected IdleSince to be set after MarkIdle")
	}

	// MarkIdle again should not change the timestamp
	first := inst.IdleSince
	inst.MarkIdle()
	if inst.IdleSince != first {
		t.Error("MarkIdle should not update already-set IdleSince")
	}

	// MarkActive clears it
	inst.MarkActive()
	if !inst.IdleSince.IsZero() {
		t.Error("expected IdleSince to be zero after MarkActive")
	}

	// AutoPaused flag
	if inst.AutoPaused {
		t.Error("expected AutoPaused to be false initially")
	}
}

func TestLastViewed(t *testing.T) {
	inst := &Instance{}
	if !inst.LastViewed.IsZero() {
		t.Error("expected LastViewed to be zero initially")
	}
	inst.TouchLastViewed()
	if inst.LastViewed.IsZero() {
		t.Error("expected LastViewed to be set after TouchLastViewed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ -run TestIdleTracking -v`
Expected: FAIL — `IdleSince` field undefined.

- [ ] **Step 3: Add fields and methods to Instance**

In `session/instance.go`, add these fields to the `Instance` struct (after `MCPConfig string`, around line 71):

```go
	// IdleSince tracks when the session became idle (prompt visible, waiting for input).
	// Zero value means the session is not idle.
	IdleSince time.Time
	// LastViewed tracks when the session was last opened in the UI.
	// Used to prevent auto-pausing sessions the user is actively viewing.
	LastViewed time.Time
	// AutoPaused is true when the session was auto-paused due to idle timeout
	// (as opposed to manually paused by the user).
	AutoPaused bool
```

Add these methods (after the `SetStatus` method, around line 254):

```go
// MarkIdle records the current time as when the session became idle.
// Does nothing if already marked idle (preserves the original timestamp).
func (i *Instance) MarkIdle() {
	if i.IdleSince.IsZero() {
		i.IdleSince = time.Now()
	}
}

// MarkActive clears the idle timestamp, indicating the session is working.
func (i *Instance) MarkActive() {
	i.IdleSince = time.Time{}
}

// TouchLastViewed updates the last-viewed timestamp to now.
func (i *Instance) TouchLastViewed() {
	i.LastViewed = time.Now()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ -run "TestIdleTracking|TestLastViewed" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add session/instance.go session/instance_test.go
git commit -m "feat(session): add idle tracking and last-viewed fields to Instance"
```

---

### Task 5: Auto-Pause Idle Sessions in PollAllStatuses

**Files:**
- Modify: `app/bindings.go:590-633` — add idle tracking and auto-pause logic to `PollAllStatuses()`
- Modify: `app/bindings.go:50-58` — add `AutoPaused` to `SessionStatus`
- Modify: `app/bindings.go:370-417` — touch `LastViewed` in `OpenSession()`
- Create: `app/bindings_idle_test.go` — test idle auto-pause logic

- [ ] **Step 1: Write the failing test for auto-pause logic**

```go
// app/bindings_idle_test.go
package app

import (
	"claude-squad/session"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollAllStatuses_AutoPauseIdle(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 1 * time.Millisecond // tiny timeout for testing

	// Create an in-place session so we don't need a real git worktree
	_, err := api.CreateSession(CreateOptions{
		Title:   "idle-test",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["idle-test"]
	require.NotNil(t, inst)

	// Simulate: session is idle for longer than timeout
	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-1 * time.Hour)

	// Poll should detect the idle session
	// Note: auto-pause won't actually work for unstarted sessions,
	// but we verify the idle detection and AutoPaused flag logic
	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	// Session should be marked as auto-paused
	assert.Equal(t, "paused", statuses[0].Status)
	assert.True(t, statuses[0].AutoPaused)
}

func TestPollAllStatuses_SkipRecentlyViewed(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 1 * time.Millisecond

	_, err := api.CreateSession(CreateOptions{
		Title:   "viewed-test",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["viewed-test"]

	// Idle for a long time but recently viewed
	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-1 * time.Hour)
	inst.TouchLastViewed() // viewed just now

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	// Should NOT be auto-paused because it was recently viewed
	assert.NotEqual(t, "paused", statuses[0].Status)
}

func TestPollAllStatuses_DisabledTimeout(t *testing.T) {
	api := newTestAPI(t)
	api.idleTimeout = 0 // disabled

	_, err := api.CreateSession(CreateOptions{
		Title:   "no-timeout",
		Path:    t.TempDir(),
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	inst := api.instances["no-timeout"]
	inst.MarkIdle()
	inst.IdleSince = time.Now().Add(-24 * time.Hour)

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	// Should NOT be auto-paused when timeout is disabled
	assert.NotEqual(t, "paused", statuses[0].Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestPollAllStatuses_AutoPause -v`
Expected: FAIL — `AutoPaused` field undefined on `SessionStatus`.

- [ ] **Step 3: Add `AutoPaused` to `SessionStatus`**

In `app/bindings.go`, add to the `SessionStatus` struct (around line 53-58):

```go
type SessionStatus struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Branch       string    `json:"branch"`
	DiffStats    DiffStats `json:"diffStats"`
	HasPrompt    bool      `json:"hasPrompt"`
	SSHConnected *bool     `json:"sshConnected"`
	AutoPaused   bool      `json:"autoPaused"`
}
```

- [ ] **Step 4: Touch `LastViewed` in `OpenSession`**

In `app/bindings.go`, inside `OpenSession()` (around line 380, after looking up `inst`), add:

```go
	inst.TouchLastViewed()
```

- [ ] **Step 5: Implement idle tracking and auto-pause in `PollAllStatuses`**

Replace the `PollAllStatuses` method in `app/bindings.go` (lines 590-633):

```go
func (api *SessionAPI) PollAllStatuses() ([]SessionStatus, error) {
	api.mu.RLock()

	needsSave := false
	var toAutoPause []string
	result := make([]SessionStatus, 0, len(api.instances))
	for _, inst := range api.instances {
		hasPrompt := inst.HasPrompt()

		// Track idle state
		if hasPrompt && inst.Status != session.Paused {
			inst.MarkIdle()
		} else if !hasPrompt && inst.Status != session.Paused {
			inst.MarkActive()
		}

		// Check for auto-pause candidates
		if api.idleTimeout > 0 &&
			!inst.IdleSince.IsZero() &&
			time.Since(inst.IdleSince) > api.idleTimeout &&
			time.Since(inst.LastViewed) > api.idleTimeout &&
			inst.Status != session.Paused {
			toAutoPause = append(toAutoPause, inst.Title)
		}

		// Check if Claude Code's active session has changed
		if inst.SyncClaudeSessionID() {
			needsSave = true
		}

		var ds DiffStats
		if stats := inst.GetDiffStats(); stats != nil {
			ds = DiffStats{Added: stats.Added, Removed: stats.Removed}
		}

		var sshConnected *bool
		if inst.HostID != "" {
			connected := api.hostManager.IsConnected(inst.HostID)
			sshConnected = &connected
		}

		result = append(result, SessionStatus{
			ID:           inst.Title,
			Status:       statusString(inst.Status),
			Branch:       inst.Branch,
			DiffStats:    ds,
			HasPrompt:    hasPrompt,
			SSHConnected: sshConnected,
			AutoPaused:   inst.AutoPaused,
		})
	}
	api.mu.RUnlock()

	// Auto-pause idle sessions (needs write lock, so done outside RLock)
	for _, id := range toAutoPause {
		api.mu.Lock()
		if inst, ok := api.instances[id]; ok {
			inst.AutoPaused = true
		}
		api.mu.Unlock()

		log.InfoLog.Printf("auto-pausing idle session %q (idle since %v)", id, time.Now())
		if err := api.PauseSession(id); err != nil {
			log.ErrorLog.Printf("auto-pause %q failed: %v", id, err)
		}
	}

	// Update result statuses for auto-paused sessions
	if len(toAutoPause) > 0 {
		paused := make(map[string]bool)
		for _, id := range toAutoPause {
			paused[id] = true
		}
		for i := range result {
			if paused[result[i].ID] {
				result[i].Status = "paused"
				result[i].AutoPaused = true
			}
		}
	}

	if needsSave {
		api.mu.Lock()
		api.dirty = true
		api.saveInstancesLocked()
		api.mu.Unlock()
	}

	return result, nil
}
```

Add `"time"` to the imports if not already present, and ensure `"claude-squad/log"` is imported.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run "TestPollAllStatuses" -v -count=1`
Expected: all three new tests PASS, plus existing `TestSessionStatus_SSHConnected` still passes.

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... -count=1 2>&1 | tail -20`
Expected: all tests PASS

- [ ] **Step 8: Commit**

```bash
git add app/bindings.go app/bindings_idle_test.go
git commit -m "feat(session): auto-pause idle sessions based on adaptive timeout"
```

---

### Task 6: Orphaned Child Process Cleanup

**Files:**
- Create: `session/cleanup.go`
- Create: `session/cleanup_test.go`

- [ ] **Step 1: Write the failing test for `KillWorktreeProcesses`**

```go
// session/cleanup_test.go
package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestKillWorktreeProcesses(t *testing.T) {
	// Create a temp dir to act as a fake worktree
	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "alive")

	// Spawn a background process that references the worktree in its args
	cmd := exec.Command("bash", "-c", "while true; do sleep 1; done # "+tmpDir)
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start background process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Verify process is running
	if err := cmd.Process.Signal(os.Signal(syscall.Signal(0))); err != nil {
		t.Fatalf("process not running: %v", err)
	}

	// Kill worktree processes
	KillWorktreeProcesses(tmpDir)

	// Wait briefly for process to die
	time.Sleep(500 * time.Millisecond)

	// Verify process is dead (Signal(0) checks if alive)
	proc, err := os.FindProcess(pid)
	if err == nil {
		err = proc.Signal(syscall.Signal(0))
	}
	if err == nil {
		t.Error("expected process to be killed, but it's still running")
	}
	_ = marker // suppress unused
}

func TestKillWorktreeProcesses_NoMatch(t *testing.T) {
	// Should not panic or error with a path that matches nothing
	KillWorktreeProcesses("/nonexistent/worktree/path/that/matches/nothing")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ -run TestKillWorktreeProcesses -v`
Expected: FAIL — `KillWorktreeProcesses` undefined.

- [ ] **Step 3: Implement `KillWorktreeProcesses`**

```go
// session/cleanup.go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ -run TestKillWorktreeProcesses -v -count=1`
Expected: PASS

- [ ] **Step 5: Add `syscall` import to cleanup_test.go**

The test file needs `"syscall"` in its imports:

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)
```

- [ ] **Step 6: Commit**

```bash
git add session/cleanup.go session/cleanup_test.go
git commit -m "feat(session): add orphaned child process cleanup for worktrees"
```

---

### Task 7: Wire Cleanup into Pause, Kill, and Startup

**Files:**
- Modify: `session/instance.go:621-700` — call `KillWorktreeProcesses` in `Pause()`
- Modify: `session/instance.go:461-486` — call `KillWorktreeProcesses` in `Kill()` (find the `Kill` method)
- Modify: `app/bindings.go:115-160` — add startup cleanup for orphaned processes

- [ ] **Step 1: Find the Kill method and add cleanup**

Read `session/instance.go` to find the `Kill` method. Add `KillWorktreeProcesses` call before the worktree is removed.

In `Instance.Pause()` (around line 674, after killing the process but before removing the worktree), add:

```go
	// Kill orphaned child processes (dev servers, watchers) that reference
	// this worktree before removing the directory.
	if i.gitWorktree != nil {
		KillWorktreeProcesses(i.gitWorktree.GetWorktreePath())
	}
```

In `Instance.Kill()`, add the same call before worktree cleanup.

- [ ] **Step 2: Add startup orphan cleanup in `NewSessionAPI`**

In `app/bindings.go`, after restoring sessions from state (around line 151, after the `for _, data := range allData` loop), add:

```go
	// Clean up orphaned processes from previous sessions.
	// After a crash or forced quit, child processes (dev servers, watchers)
	// may survive. Kill any that reference worktree paths of paused sessions.
	for _, inst := range api.instances {
		if inst.Status == session.Paused {
			worktree := inst.GetWorktreePath()
			if worktree != "" {
				session.KillWorktreeProcesses(worktree)
			}
		}
	}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: success

- [ ] **Step 4: Run full test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... -count=1 2>&1 | tail -20`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add session/instance.go app/bindings.go
git commit -m "feat(session): wire orphan cleanup into pause, kill, and startup"
```

---

### Task 8: Final Integration Test & Cleanup

**Files:**
- All previously modified files

- [ ] **Step 1: Run the full test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... -count=1 -v 2>&1 | tail -40`
Expected: all tests PASS

- [ ] **Step 2: Verify the build produces a working binary**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && wails build -skipbindings 2>&1 | tail -10`
Expected: build succeeds, `build/bin/claude-squad.app` is created

- [ ] **Step 3: Verify adaptive limits are correct for this machine**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestGetSystemMemoryMB -v`
Expected: reports 8192 MB (or actual RAM)

- [ ] **Step 4: Review all changes**

Run: `git diff main --stat` to verify only the expected files were changed:
- `main.go`
- `app/memory.go` (new)
- `app/memory_test.go` (new)
- `app/bindings.go`
- `app/bindings_idle_test.go` (new)
- `app/indexer_treesitter.go`
- `session/instance.go`
- `session/instance_test.go`
- `session/cleanup.go` (new)
- `session/cleanup_test.go` (new)
- `docs/superpowers/specs/2026-04-16-adaptive-memory-management-design.md`
- `docs/superpowers/plans/2026-04-16-adaptive-memory-management.md`
