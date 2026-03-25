package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

type SpawnOptions struct {
	Dir  string
	Env  []string
	Rows uint16
	Cols uint16
}

type Session struct {
	id      string
	ptmx    *os.File
	cmd     *exec.Cmd
	mu      sync.Mutex
	closed  bool
	monitor *Monitor

	subMu       sync.Mutex
	subscribers map[*Subscriber]struct{}

	// exited is closed when the process exits.
	exited chan struct{}
}

func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, fmt.Errorf("session closed")
	}
	return s.ptmx.Write(p)
}

func (s *Session) Subscribe() *Subscriber {
	sub := &Subscriber{Ch: make(chan []byte, 256)}
	s.subMu.Lock()
	s.subscribers[sub] = struct{}{}
	s.subMu.Unlock()
	return sub
}

func (s *Session) Unsubscribe(sub *Subscriber) {
	s.subMu.Lock()
	delete(s.subscribers, sub)
	s.subMu.Unlock()
}

func (s *Session) GetSnapshot() []byte {
	return []byte(s.monitor.Content())
}

func (s *Session) broadcast(data []byte) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for sub := range s.subscribers {
		// Copy data for each subscriber since slices share backing array
		cp := make([]byte, len(data))
		copy(cp, data)
		select {
		case sub.Ch <- cp:
		default:
			// Drop data if subscriber is too slow
		}
	}
}

func (s *Session) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.ptmx.Close()
	s.cmd.Process.Kill()
	s.cmd.Wait()

	// Close all subscriber channels
	s.subMu.Lock()
	for sub := range s.subscribers {
		close(sub.Ch)
		delete(s.subscribers, sub)
	}
	s.subMu.Unlock()
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	counter  int
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Spawn(program string, args []string, opts SpawnOptions) (string, error) {
	cmd := exec.Command(program, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if opts.Env != nil {
		cmd.Env = append(cmd.Env, opts.Env...)
	}

	rows, cols := opts.Rows, opts.Cols
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return "", fmt.Errorf("pty start: %w", err)
	}

	monitor := NewMonitor(64 * 1024)

	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("session-%d", m.counter)
	sess := &Session{
		id:          id,
		ptmx:        ptmx,
		cmd:         cmd,
		monitor:     monitor,
		subscribers: make(map[*Subscriber]struct{}),
		exited:      make(chan struct{}),
	}
	m.sessions[id] = sess
	m.mu.Unlock()

	// Read PTY output, write to monitor and broadcast to subscribers.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				sess.monitor.Write(buf[:n])
				sess.broadcast(buf[:n])
			}
			if err != nil {
				// Close all subscriber channels on PTY EOF
				sess.subMu.Lock()
				for sub := range sess.subscribers {
					close(sub.Ch)
					delete(sess.subscribers, sub)
				}
				sess.subMu.Unlock()
				return
			}
		}
	}()

	go func() {
		cmd.Wait()
		sess.mu.Lock()
		sess.closed = true
		sess.mu.Unlock()
		close(sess.exited)
	}()

	return id, nil
}

// GetSession returns the raw Session pointer for internal use.
func (m *Manager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Get implements SessionRegistry, returning a StreamableSession.
func (m *Manager) Get(id string) StreamableSession {
	m.mu.RLock()
	sess := m.sessions[id]
	m.mu.RUnlock()
	if sess == nil {
		return nil
	}
	return sess
}

func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	sess.close()
	return nil
}

func (m *Manager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		return fmt.Errorf("session %s is closed", id)
	}

	return pty.Setsize(sess.ptmx, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, sess := range m.sessions {
		sess.close()
		delete(m.sessions, id)
	}
}

func (m *Manager) GetContent(id string) string {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	return sess.monitor.Content()
}

func (m *Manager) HasUpdated(id string) (bool, bool) {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false, false
	}
	return sess.monitor.HasUpdated()
}

func (m *Manager) CheckTrustPrompt(id string) bool {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return sess.monitor.CheckTrustPrompt()
}

// WaitExit blocks until the process exits or the timeout expires.
// Returns true if the process exited within the timeout.
func (m *Manager) WaitExit(id string, timeout time.Duration) bool {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return true
	}
	select {
	case <-sess.exited:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (m *Manager) Write(id string, data []byte) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	_, err := sess.Write(data)
	return err
}
