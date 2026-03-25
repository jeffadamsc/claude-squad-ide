package ssh

import (
	"claude-squad/pty"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSSHSession_WriteAndSubscribe(t *testing.T) {
	sess := newSSHSession("test-1", nil, nil)
	sub := sess.Subscribe()
	defer sess.Unsubscribe(sub)

	// Simulate receiving remote output
	sess.feedOutput([]byte("hello world"))

	select {
	case data := <-sub.Ch:
		assert.Equal(t, []byte("hello world"), data)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for subscriber data")
	}
}

func TestSSHSession_GetSnapshot(t *testing.T) {
	sess := newSSHSession("test-2", nil, nil)
	sess.feedOutput([]byte("line1\nline2\n"))

	snapshot := sess.GetSnapshot()
	assert.Contains(t, string(snapshot), "line1")
	assert.Contains(t, string(snapshot), "line2")
}

func TestSSHSession_Monitor(t *testing.T) {
	sess := newSSHSession("test-3", nil, nil)
	sess.feedOutput([]byte("some output"))

	content := sess.monitor.Content()
	assert.Contains(t, content, "some output")

	updated, _ := sess.monitor.HasUpdated()
	assert.True(t, updated)
}

func TestSSHSession_ImplementsStreamableSession(t *testing.T) {
	var _ pty.StreamableSession = (*SSHSession)(nil)
}

func TestSSHProcessManager_ImplementsSessionRegistry(t *testing.T) {
	var _ pty.SessionRegistry = (*SSHProcessManager)(nil)
}
