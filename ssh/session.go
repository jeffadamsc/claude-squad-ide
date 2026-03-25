package ssh

import (
	"claude-squad/pty"
	"fmt"
	"io"
	"sync"

	gossh "golang.org/x/crypto/ssh"
)

// SSHSession wraps an SSH session as a StreamableSession for WebSocket streaming.
type SSHSession struct {
	id      string
	mu      sync.Mutex
	stdin   io.Writer      // write target (stdin pipe from gossh.Session)
	sshSess *gossh.Session // retained for Resize (WindowChange)
	closed  bool
	monitor *pty.Monitor
	exited  chan struct{}

	subMu       sync.Mutex
	subscribers map[*pty.Subscriber]struct{}
}

func newSSHSession(id string, stdin io.Writer, sshSess *gossh.Session) *SSHSession {
	return &SSHSession{
		id:          id,
		stdin:       stdin,
		sshSess:     sshSess,
		monitor:     pty.NewMonitor(64 * 1024),
		subscribers: make(map[*pty.Subscriber]struct{}),
		exited:      make(chan struct{}),
	}
}

func (s *SSHSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stdin == nil {
		return 0, fmt.Errorf("session closed")
	}
	return s.stdin.Write(p)
}

func (s *SSHSession) Subscribe() *pty.Subscriber {
	sub := &pty.Subscriber{Ch: make(chan []byte, 256)}
	s.subMu.Lock()
	s.subscribers[sub] = struct{}{}
	s.subMu.Unlock()
	return sub
}

func (s *SSHSession) Unsubscribe(sub *pty.Subscriber) {
	s.subMu.Lock()
	delete(s.subscribers, sub)
	s.subMu.Unlock()
}

func (s *SSHSession) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *SSHSession) GetSnapshot() []byte {
	return []byte(s.monitor.Content())
}

// Resize sends a window-change request to the remote PTY.
func (s *SSHSession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sshSess == nil {
		return fmt.Errorf("session closed")
	}
	return s.sshSess.WindowChange(int(rows), int(cols))
}

// feedOutput writes data to the monitor and broadcasts to subscribers.
func (s *SSHSession) feedOutput(data []byte) {
	s.monitor.Write(data)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for sub := range s.subscribers {
		cp := make([]byte, len(data))
		copy(cp, data)
		select {
		case sub.Ch <- cp:
		default:
		}
	}
}

func (s *SSHSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.sshSess != nil {
		s.sshSess.Close()
	}
	s.subMu.Lock()
	for sub := range s.subscribers {
		close(sub.Ch)
		delete(s.subscribers, sub)
	}
	s.subMu.Unlock()
}
