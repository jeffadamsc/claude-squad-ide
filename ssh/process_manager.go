package ssh

import (
	"claude-squad/pty"
	"fmt"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// SSHProcessManager implements session.ProcessManager over SSH.
type SSHProcessManager struct {
	mu       sync.RWMutex
	client   *Client
	hostID   string
	sessions map[string]*SSHSession
	counter  int
}

func NewSSHProcessManager(client *Client, hostID string) *SSHProcessManager {
	return &SSHProcessManager{
		client:   client,
		hostID:   hostID,
		sessions: make(map[string]*SSHSession),
	}
}

// UpdateClient replaces the underlying SSH client after a reconnect.
// Existing sessions retain their snapshot buffers so WebSocket clients
// can still retrieve the last terminal state via GetSnapshot.
func (m *SSHProcessManager) UpdateClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = client
}

func (m *SSHProcessManager) Spawn(program string, args []string, opts pty.SpawnOptions) (string, error) {
	sshClient := m.client.SSHClient()
	if sshClient == nil {
		return "", fmt.Errorf("ssh not connected")
	}

	session, err := sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("new ssh session: %w", err)
	}

	// Request PTY
	rows, cols := opts.Rows, opts.Cols
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		session.Close()
		return "", fmt.Errorf("request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return "", fmt.Errorf("stdout pipe: %w", err)
	}

	// Build command — wrap in login shell so ~/.profile/.bashrc are sourced
	innerCmd := program
	if len(args) > 0 {
		innerCmd += " " + strings.Join(args, " ")
	}
	if opts.Dir != "" {
		innerCmd = fmt.Sprintf("cd %s && %s", ShellEscape(opts.Dir), innerCmd)
	}
	cmd := fmt.Sprintf("bash -lc %s", ShellEscape(innerCmd))

	if err := session.Start(cmd); err != nil {
		session.Close()
		return "", fmt.Errorf("start command: %w", err)
	}

	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("ssh-%s-%d", m.hostID, m.counter)
	sshSess := newSSHSession(id, stdin, session)
	m.sessions[id] = sshSess
	m.mu.Unlock()

	// Read stdout and feed to session
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				sshSess.feedOutput(buf[:n])
			}
			if err != nil {
				sshSess.subMu.Lock()
				for sub := range sshSess.subscribers {
					close(sub.Ch)
					delete(sshSess.subscribers, sub)
				}
				sshSess.subMu.Unlock()
				return
			}
		}
	}()

	// Wait for session exit
	go func() {
		session.Wait()
		sshSess.mu.Lock()
		sshSess.closed = true
		sshSess.mu.Unlock()
		close(sshSess.exited)
	}()

	return id, nil
}

func (m *SSHProcessManager) Kill(id string) error {
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

func (m *SSHProcessManager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return sess.Resize(rows, cols)
}

func (m *SSHProcessManager) Write(id string, data []byte) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	_, err := sess.Write(data)
	return err
}

func (m *SSHProcessManager) GetContent(id string) string {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	return sess.monitor.Content()
}

func (m *SSHProcessManager) HasUpdated(id string) (bool, bool) {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false, false
	}
	return sess.monitor.HasUpdated()
}

func (m *SSHProcessManager) HasPrompt(id string) bool {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return sess.monitor.HasPrompt()
}

func (m *SSHProcessManager) CheckTrustPrompt(id string) bool {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return sess.monitor.CheckTrustPrompt()
}

func (m *SSHProcessManager) GetPID(id string) int {
	// SSH sessions don't expose the remote PID directly.
	return 0
}

func (m *SSHProcessManager) WaitExit(id string, timeout time.Duration) bool {
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

// Get implements pty.SessionRegistry, returning a StreamableSession.
func (m *SSHProcessManager) Get(id string) pty.StreamableSession {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return sess
}

// DynamicSSHRegistry queries all active SSHProcessManagers via HostManager.
type DynamicSSHRegistry struct {
	hm *HostManager
}

func NewDynamicSSHRegistry(hm *HostManager) *DynamicSSHRegistry {
	return &DynamicSSHRegistry{hm: hm}
}

func (r *DynamicSSHRegistry) Get(id string) pty.StreamableSession {
	for _, pm := range r.hm.GetAllProcessManagers() {
		if sess := pm.Get(id); sess != nil {
			return sess
		}
	}
	return nil
}

func (r *DynamicSSHRegistry) Resize(id string, rows, cols uint16) error {
	for _, pm := range r.hm.GetAllProcessManagers() {
		if err := pm.Resize(id, rows, cols); err == nil {
			return nil
		}
	}
	return fmt.Errorf("ssh session %s not found", id)
}
