package app

import (
	"claude-squad/session"
	ptyPkg "claude-squad/pty"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// slowProcessManager implements session.ProcessManager with a configurable
// delay in Kill so the test can observe whether the lock is held during Kill.
type slowProcessManager struct {
	killDelay time.Duration
	// killStarted is written (atomically) when Kill begins, so the test can
	// synchronise accurately on "Kill has entered its slow path".
	killStarted atomic.Bool
}

func (s *slowProcessManager) Spawn(program string, args []string, opts ptyPkg.SpawnOptions) (string, error) {
	return "fake-pid", nil
}

func (s *slowProcessManager) Kill(id string) error {
	s.killStarted.Store(true)
	time.Sleep(s.killDelay)
	return nil
}

func (s *slowProcessManager) Resize(id string, rows, cols uint16) error { return nil }

func (s *slowProcessManager) HasUpdated(id string) (bool, bool) { return false, false }

func (s *slowProcessManager) HasPrompt(id string) bool { return false }

func (s *slowProcessManager) CheckTrustPrompt(id string) bool { return false }

func (s *slowProcessManager) GetContent(id string) string { return "" }

func (s *slowProcessManager) Write(id string, data []byte) error { return nil }

func (s *slowProcessManager) WaitExit(id string, timeout time.Duration) bool { return false }

func (s *slowProcessManager) GetPID(id string) int { return 0 }

// TestKillSession_DoesNotBlockOtherCalls verifies that the refactored KillSession
// releases api.mu BEFORE calling inst.Kill(), so concurrent read-lock callers
// (e.g. LoadSessions) are not blocked for the duration of the slow Kill.
//
// Design:
//   - A slowProcessManager imposes a 300 ms delay in Kill().
//   - KillSession is launched in a goroutine.
//   - We wait until Kill() has actually started (killStarted flag), confirming
//     the lock has been released and we're inside the slow path.
//   - Then we call LoadSessions (which needs mu.RLock) and measure how long it
//     takes. It should return nearly immediately; if the lock were still held it
//     would block for ~300 ms.
func TestKillSession_DoesNotBlockOtherCalls(t *testing.T) {
	const killDelay = 300 * time.Millisecond

	api := newTestAPI(t)

	// Build a started Instance backed by our slow PM. We use InPlace mode so
	// no git worktree is created or cleaned up — the only slow operation is the
	// PM's Kill, which we fully control.
	spm := &slowProcessManager{killDelay: killDelay}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:          "victim",
		Path:           t.TempDir(),
		Program:        "echo noop",
		InPlace:        true,
		ProcessManager: spm,
	})
	require.NoError(t, err)

	// Start(false) calls spawnProcess → spm.Spawn → returns "fake-pid" → sets
	// started=true so Kill() will exercise the slow PM path.
	require.NoError(t, inst.Start(false))

	// Inject directly into the API's instance map (bypasses the full
	// CreateSession flow which would try to spawn a real git worktree or
	// create a real PTY process).
	api.mu.Lock()
	api.instances["victim"] = inst
	api.mu.Unlock()

	killDone := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_ = api.KillSession("victim")
		killDone <- time.Since(start)
	}()

	// Wait until Kill() has started (i.e., the lock has been released and we're
	// inside the slow path). Poll briefly — typically <1 ms.
	deadline := time.Now().Add(2 * time.Second)
	for !spm.killStarted.Load() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for inst.Kill() to start — KillSession may be stuck holding the lock")
		}
		time.Sleep(time.Millisecond)
	}

	// At this point KillSession has released the lock and inst.Kill() is sleeping
	// for killDelay. LoadSessions should acquire the RLock and return immediately.
	loadStart := time.Now()
	_, err = api.LoadSessions()
	loadElapsed := time.Since(loadStart)
	require.NoError(t, err)

	killElapsed := <-killDone

	t.Logf("LoadSessions took %v; KillSession took %v", loadElapsed, killElapsed)

	// The threshold: LoadSessions must complete in less than half of killDelay.
	// If the lock were held during Kill, LoadSessions would wait for ~killDelay.
	maxAllowed := killDelay / 2
	if loadElapsed > maxAllowed {
		t.Errorf("LoadSessions took %v (limit %v, KillSession total %v) — "+
			"lock appears still held during inst.Kill()",
			loadElapsed, maxAllowed, killElapsed)
	}
}
