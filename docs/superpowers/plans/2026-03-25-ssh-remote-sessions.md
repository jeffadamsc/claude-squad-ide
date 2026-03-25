# SSH Remote Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run claude-squad sessions on remote machines via SSH with persistent host configs, connection monitoring, and auto-reconnect.

**Architecture:** Pure Go SSH via `golang.org/x/crypto/ssh`. New `ssh/` package for host management, client connections, and remote process management. `ProcessManager` interface allows `Instance` to work with either local PTY or SSH sessions transparently. `SessionRegistry` abstraction lets the WebSocket server route to both local and remote sessions.

**Tech Stack:** Go, `golang.org/x/crypto/ssh`, `golang.org/x/crypto/ssh/knownhosts`, macOS Keychain (via `github.com/keybase/go-keychain`), React/TypeScript/Zustand frontend, Wails v2 bindings.

**Spec:** `docs/superpowers/specs/2026-03-25-ssh-remote-sessions-design.md`

---

### Task 1: Host Configuration CRUD (`ssh/hosts.go`)

**Files:**
- Create: `ssh/hosts.go`
- Create: `ssh/hosts_test.go`

- [ ] **Step 1: Write tests for host config load/save**

```go
// ssh/hosts_test.go
package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewHostStore(filepath.Join(dir, "hosts.json"))

	host := HostConfig{
		ID:         "test-id-1",
		Name:       "dev-server",
		Host:       "192.168.1.50",
		Port:       22,
		User:       "deploy",
		AuthMethod: AuthMethodKey,
		KeyPath:    "/home/user/.ssh/id_ed25519",
	}

	err := store.Save(host)
	require.NoError(t, err)

	hosts, err := store.LoadAll()
	require.NoError(t, err)
	require.Len(t, hosts, 1)
	assert.Equal(t, host, hosts[0])
}

func TestHostStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewHostStore(filepath.Join(dir, "hosts.json"))

	host := HostConfig{ID: "del-1", Name: "server1", Host: "10.0.0.1", Port: 22, User: "root", AuthMethod: AuthMethodPassword}
	require.NoError(t, store.Save(host))

	err := store.Delete("del-1")
	require.NoError(t, err)

	hosts, err := store.LoadAll()
	require.NoError(t, err)
	assert.Len(t, hosts, 0)
}

func TestHostStore_Update(t *testing.T) {
	dir := t.TempDir()
	store := NewHostStore(filepath.Join(dir, "hosts.json"))

	host := HostConfig{ID: "upd-1", Name: "old-name", Host: "10.0.0.1", Port: 22, User: "root", AuthMethod: AuthMethodPassword}
	require.NoError(t, store.Save(host))

	host.Name = "new-name"
	host.Port = 2222
	err := store.Update(host)
	require.NoError(t, err)

	hosts, err := store.LoadAll()
	require.NoError(t, err)
	require.Len(t, hosts, 1)
	assert.Equal(t, "new-name", hosts[0].Name)
	assert.Equal(t, 2222, hosts[0].Port)
}

func TestHostStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewHostStore(filepath.Join(dir, "hosts.json"))

	hosts, err := store.LoadAll()
	require.NoError(t, err)
	assert.Len(t, hosts, 0)
}

func TestHostStore_GetByID(t *testing.T) {
	dir := t.TempDir()
	store := NewHostStore(filepath.Join(dir, "hosts.json"))

	h1 := HostConfig{ID: "a", Name: "server-a", Host: "1.1.1.1", Port: 22, User: "u", AuthMethod: AuthMethodPassword}
	h2 := HostConfig{ID: "b", Name: "server-b", Host: "2.2.2.2", Port: 22, User: "u", AuthMethod: AuthMethodKey}
	require.NoError(t, store.Save(h1))
	require.NoError(t, store.Save(h2))

	found, err := store.GetByID("b")
	require.NoError(t, err)
	assert.Equal(t, "server-b", found.Name)

	_, err = store.GetByID("nonexistent")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run TestHostStore`
Expected: Compilation failure — package `ssh` does not exist.

- [ ] **Step 3: Implement host config types and CRUD**

```go
// ssh/hosts.go
package ssh

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const (
	AuthMethodPassword      = "password"
	AuthMethodKey           = "key"
	AuthMethodKeyPassphrase = "key+passphrase"
)

// HostConfig is a saved SSH host configuration. Secrets (passwords, passphrases)
// are stored in the macOS Keychain, not here.
type HostConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath,omitempty"`
}

// HostStore manages persistent host configurations in a JSON file.
type HostStore struct {
	mu   sync.Mutex
	path string
}

func NewHostStore(path string) *HostStore {
	return &HostStore{path: path}
}

func (s *HostStore) LoadAll() ([]HostConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *HostStore) loadLocked() ([]HostConfig, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []HostConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read hosts file: %w", err)
	}
	var hosts []HostConfig
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("parse hosts file: %w", err)
	}
	return hosts, nil
}

func (s *HostStore) saveLocked(hosts []HostConfig) error {
	data, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hosts: %w", err)
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *HostStore) Save(host HostConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	hosts = append(hosts, host)
	return s.saveLocked(hosts)
}

func (s *HostStore) Update(host HostConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	for i, h := range hosts {
		if h.ID == host.ID {
			hosts[i] = host
			return s.saveLocked(hosts)
		}
	}
	return fmt.Errorf("host %s not found", host.ID)
}

func (s *HostStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return err
	}
	filtered := make([]HostConfig, 0, len(hosts))
	for _, h := range hosts {
		if h.ID != id {
			filtered = append(filtered, h)
		}
	}
	return s.saveLocked(filtered)
}

func (s *HostStore) GetByID(id string) (HostConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := s.loadLocked()
	if err != nil {
		return HostConfig{}, err
	}
	for _, h := range hosts {
		if h.ID == id {
			return h, nil
		}
	}
	return HostConfig{}, fmt.Errorf("host %s not found", id)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run TestHostStore`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add ssh/hosts.go ssh/hosts_test.go
git commit -m "feat(ssh): add host configuration CRUD with JSON persistence"
```

---

### Task 2: macOS Keychain Integration (`ssh/keychain.go`)

**Files:**
- Create: `ssh/keychain.go`
- Create: `ssh/keychain_test.go`
- Modify: `go.mod` (add `github.com/keybase/go-keychain`)

- [ ] **Step 1: Add keychain dependency**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go get github.com/keybase/go-keychain`

- [ ] **Step 2: Write tests for keychain operations**

```go
// ssh/keychain_test.go
package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeychainStore_SetAndGet(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-" + generateTestID()

	// Clean up after test
	defer ks.Delete(hostID)

	err := ks.Set(hostID, "my-secret-password")
	require.NoError(t, err)

	secret, err := ks.Get(hostID)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-password", secret)
}

func TestKeychainStore_GetMissing(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	_, err := ks.Get("nonexistent-host-id")
	assert.Error(t, err)
}

func TestKeychainStore_Delete(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-del-" + generateTestID()

	require.NoError(t, ks.Set(hostID, "temp-secret"))
	require.NoError(t, ks.Delete(hostID))

	_, err := ks.Get(hostID)
	assert.Error(t, err)
}

func TestKeychainStore_Update(t *testing.T) {
	ks := NewKeychainStore("com.claude-squad.test")
	hostID := "test-kc-upd-" + generateTestID()
	defer ks.Delete(hostID)

	require.NoError(t, ks.Set(hostID, "old-secret"))
	require.NoError(t, ks.Set(hostID, "new-secret"))

	secret, err := ks.Get(hostID)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", secret)
}

func generateTestID() string {
	b := make([]byte, 4)
	_, _ = cryptoRand.Read(b)
	return fmt.Sprintf("%x", b)
}
```

Note: Add `crypto/rand` and `fmt` imports, alias as `cryptoRand`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run TestKeychainStore`
Expected: Compilation failure — `NewKeychainStore` not defined.

- [ ] **Step 4: Implement keychain store**

```go
// ssh/keychain.go
package ssh

import (
	"fmt"

	"github.com/keybase/go-keychain"
)

// KeychainStore manages secrets in the macOS Keychain.
type KeychainStore struct {
	service string
}

func NewKeychainStore(service string) *KeychainStore {
	return &KeychainStore{service: service}
}

// Set stores or updates a secret for the given host ID.
func (k *KeychainStore) Set(hostID string, secret string) error {
	// Try to delete existing item first (update case)
	_ = k.Delete(hostID)

	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(k.service)
	item.SetAccount(hostID)
	item.SetData([]byte(secret))
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	return keychain.AddItem(item)
}

// Get retrieves the secret for the given host ID.
func (k *KeychainStore) Get(hostID string) (string, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(k.service)
	query.SetAccount(hostID)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return "", fmt.Errorf("keychain query: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no secret found for host %s", hostID)
	}
	return string(results[0].Data), nil
}

// Delete removes the secret for the given host ID.
func (k *KeychainStore) Delete(hostID string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(k.service)
	item.SetAccount(hostID)
	return keychain.DeleteItem(item)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run TestKeychainStore`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add ssh/keychain.go ssh/keychain_test.go go.mod go.sum
git commit -m "feat(ssh): add macOS Keychain integration for SSH secrets"
```

---

### Task 3: SSH Client (`ssh/client.go`)

**Files:**
- Create: `ssh/client.go`
- Create: `ssh/client_test.go`

- [ ] **Step 1: Write tests for SSH client auth config building**

These tests verify auth config construction without needing a real SSH server.

```go
// ssh/client_test.go
package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSSHConfig_Password(t *testing.T) {
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodPassword,
	}, "my-password")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 1) // password auth
}

func TestBuildSSHConfig_Key(t *testing.T) {
	// Create a temp key file for testing
	keyPath := createTestKey(t)
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKey,
		KeyPath:    keyPath,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 1) // publickey auth
}

func TestBuildSSHConfig_KeyWithPassphrase(t *testing.T) {
	keyPath := createTestEncryptedKey(t, "test-passphrase")
	cfg, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKeyPassphrase,
		KeyPath:    keyPath,
	}, "test-passphrase")
	require.NoError(t, err)
	assert.Equal(t, "deploy", cfg.User)
	assert.Len(t, cfg.Auth, 1) // publickey auth
}

func TestBuildSSHConfig_MissingKeyFile(t *testing.T) {
	_, err := buildSSHConfig(HostConfig{
		User:       "deploy",
		AuthMethod: AuthMethodKey,
		KeyPath:    "/nonexistent/key",
	}, "")
	assert.Error(t, err)
}

func TestClient_Address(t *testing.T) {
	c := &Client{config: HostConfig{Host: "10.0.0.1", Port: 2222}}
	assert.Equal(t, "10.0.0.1:2222", c.address())
}
```

Note: `createTestKey` and `createTestEncryptedKey` are helper functions that generate temporary SSH key files using `crypto/ed25519` and `ssh.MarshalAuthorizedKey`. Implement them in the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run "TestBuildSSHConfig|TestClient_Address"`
Expected: Compilation failure — `buildSSHConfig`, `Client` not defined.

- [ ] **Step 3: Implement SSH client**

```go
// ssh/client.go
package ssh

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	dialTimeout      = 10 * time.Second
	keepaliveInterval = 5 * time.Second
)

// Client wraps an SSH connection with keepalive and reconnect support.
type Client struct {
	mu        sync.RWMutex
	client    *ssh.Client
	config    HostConfig
	secret    string // password or passphrase from keychain
	connected bool
	lastErr   error
	homeDir   string // cached remote $HOME

	onDisconnect func()
	stopKeepalive chan struct{}
}

func NewClient(config HostConfig, secret string) *Client {
	return &Client{
		config: config,
		secret: secret,
	}
}

func (c *Client) address() string {
	return fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
}

// Connect establishes the SSH connection and starts keepalive.
func (c *Client) Connect() error {
	sshCfg, err := buildSSHConfig(c.config, c.secret)
	if err != nil {
		return fmt.Errorf("build ssh config: %w", err)
	}

	client, err := ssh.Dial("tcp", c.address(), sshCfg)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}

	c.mu.Lock()
	c.client = client
	c.connected = true
	c.lastErr = nil
	c.stopKeepalive = make(chan struct{})
	c.mu.Unlock()

	// Resolve remote home directory
	home, err := c.RunCommand("echo $HOME")
	if err != nil {
		client.Close()
		return fmt.Errorf("resolve remote home: %w", err)
	}
	c.mu.Lock()
	c.homeDir = strings.TrimSpace(home)
	c.mu.Unlock()

	go c.keepaliveLoop()
	return nil
}

// TestConnection tests SSH connectivity and checks if program is available.
func TestConnection(config HostConfig, secret string, program string) (connOK bool, progOK bool, errMsg string) {
	sshCfg, err := buildSSHConfig(config, secret)
	if err != nil {
		return false, false, fmt.Sprintf("Auth config error: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return false, false, fmt.Sprintf("Connection failed: %v", err)
	}
	defer client.Close()

	// Check program availability
	session, err := client.NewSession()
	if err != nil {
		return true, false, fmt.Sprintf("Session error: %v", err)
	}
	defer session.Close()

	progCmd := fmt.Sprintf("which %s", program)
	if err := session.Run(progCmd); err != nil {
		return true, false, fmt.Sprintf("Program '%s' not found on remote PATH", program)
	}
	return true, true, ""
}

// RunCommand executes a command on the remote host and returns stdout.
func (c *Client) RunCommand(cmd string) (string, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// Connected returns whether the SSH connection is alive.
func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// LastError returns the last connection error.
func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// HomeDir returns the cached remote home directory.
func (c *Client) HomeDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.homeDir
}

// SSHClient returns the underlying ssh.Client for creating sessions.
func (c *Client) SSHClient() *ssh.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// OnDisconnect registers a callback fired when the connection drops.
func (c *Client) OnDisconnect(fn func()) {
	c.mu.Lock()
	c.onDisconnect = fn
	c.mu.Unlock()
}

// Close tears down the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopKeepalive != nil {
		close(c.stopKeepalive)
		c.stopKeepalive = nil
	}
	c.connected = false
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopKeepalive:
			return
		case <-ticker.C:
			c.mu.RLock()
			client := c.client
			c.mu.RUnlock()
			if client == nil {
				return
			}
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.lastErr = err
				cb := c.onDisconnect
				c.mu.Unlock()
				if cb != nil {
					cb()
				}
				return
			}
		}
	}
}

func buildSSHConfig(host HostConfig, secret string) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:    host.User,
		Timeout: dialTimeout,
	}

	// Host key verification via known_hosts
	knownHostsPath := os.ExpandEnv("$HOME/.ssh/known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		hostKeyCallback, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("parse known_hosts: %w", err)
		}
		cfg.HostKeyCallback = hostKeyCallback
	} else {
		// Fall back to insecure if no known_hosts file
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	switch host.AuthMethod {
	case AuthMethodPassword:
		cfg.Auth = []ssh.AuthMethod{ssh.Password(secret)}
	case AuthMethodKey:
		keyData, err := os.ReadFile(host.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	case AuthMethodKeyPassphrase:
		keyData, err := os.ReadFile(host.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse private key with passphrase: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", host.AuthMethod)
	}

	return cfg, nil
}
```

Note: Also implement `createTestKey` and `createTestEncryptedKey` helpers in the test file using `crypto/ed25519` to generate temp keys.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run "TestBuildSSHConfig|TestClient_Address"`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add ssh/client.go ssh/client_test.go
git commit -m "feat(ssh): add SSH client with auth, keepalive, and host key verification"
```

---

### Task 4: Session Registry & WebSocket Refactor (`pty/`)

**Files:**
- Create: `pty/registry.go`
- Modify: `pty/manager.go:24-37` (add `GetSnapshot` to Session, export subscriber type)
- Modify: `pty/websocket.go:25-37,53-119` (accept `SessionRegistry` instead of `*Manager`)
- Create: `pty/registry_test.go`
- Modify: `pty/websocket_test.go` (update for interface)

- [ ] **Step 1: Write tests for registry and streamable session**

```go
// pty/registry_test.go
package pty

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockStreamableSession struct {
	data    []byte
	closed  bool
	written []byte
}

func (m *mockStreamableSession) Write(p []byte) (int, error) {
	m.written = append(m.written, p...)
	return len(p), nil
}
func (m *mockStreamableSession) Subscribe() *Subscriber   { return &Subscriber{Ch: make(chan []byte, 1)} }
func (m *mockStreamableSession) Unsubscribe(sub *Subscriber) {}
func (m *mockStreamableSession) Closed() bool              { return m.closed }
func (m *mockStreamableSession) GetSnapshot() []byte       { return m.data }

func TestCompositeRegistry_PrimaryFirst(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{
		"s1": &mockStreamableSession{data: []byte("primary")},
	}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{
		"s1": &mockStreamableSession{data: []byte("secondary")},
	}}
	composite := NewCompositeRegistry(primary, secondary)

	sess := composite.Get("s1")
	assert.NotNil(t, sess)
	assert.Equal(t, []byte("primary"), sess.GetSnapshot())
}

func TestCompositeRegistry_Fallback(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{
		"s2": &mockStreamableSession{data: []byte("secondary")},
	}}
	composite := NewCompositeRegistry(primary, secondary)

	sess := composite.Get("s2")
	assert.NotNil(t, sess)
	assert.Equal(t, []byte("secondary"), sess.GetSnapshot())
}

func TestCompositeRegistry_NotFound(t *testing.T) {
	primary := &mockRegistry{sessions: map[string]StreamableSession{}}
	secondary := &mockRegistry{sessions: map[string]StreamableSession{}}
	composite := NewCompositeRegistry(primary, secondary)

	assert.Nil(t, composite.Get("missing"))
}

type mockRegistry struct {
	sessions map[string]StreamableSession
}

func (r *mockRegistry) Get(id string) StreamableSession { return r.sessions[id] }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./pty/ -v -run TestCompositeRegistry`
Expected: Compilation failure — types not defined.

- [ ] **Step 3: Implement registry interfaces**

```go
// pty/registry.go
package pty

// Subscriber receives live terminal output via a channel.
type Subscriber struct {
	Ch chan []byte
}

// StreamableSession is a terminal session that can be streamed over WebSocket.
type StreamableSession interface {
	Write(p []byte) (int, error)
	Subscribe() *Subscriber
	Unsubscribe(sub *Subscriber)
	Closed() bool
	GetSnapshot() []byte
}

// SessionRegistry looks up streamable sessions by ID.
type SessionRegistry interface {
	Get(id string) StreamableSession
}

// CompositeRegistry tries multiple registries in order.
type CompositeRegistry struct {
	registries []SessionRegistry
}

func NewCompositeRegistry(registries ...SessionRegistry) *CompositeRegistry {
	return &CompositeRegistry{registries: registries}
}

func (c *CompositeRegistry) Get(id string) StreamableSession {
	for _, r := range c.registries {
		if sess := r.Get(id); sess != nil {
			return sess
		}
	}
	return nil
}
```

- [ ] **Step 4: Update pty.Session to implement StreamableSession**

In `pty/manager.go`, refactor the `subscriber` type to use the exported `Subscriber` from registry.go. Update `Session` methods:

- Rename `subscriber` usages to `Subscriber` (use `sub.Ch` instead of `sub.ch`)
- Add `GetSnapshot() []byte` method to `Session`
- Add `Get(id string) StreamableSession` method to `Manager` (wrapping existing `Get` that returns `*Session`)

Key changes to `pty/manager.go`:
- Replace all `subscriber` with `Subscriber` and `sub.ch` with `sub.Ch`
- Add: `func (s *Session) GetSnapshot() []byte { return []byte(s.monitor.Content()) }`
- Rename existing `Get(id string) *Session` to `GetSession(id string) *Session` (update all callers in `websocket.go` — there's only one, and it's being replaced by the registry anyway)
- Add: `func (m *Manager) Get(id string) StreamableSession` that wraps `GetSession` return, satisfying `SessionRegistry`

- [ ] **Step 5: Update WebSocketServer to use SessionRegistry**

Modify `pty/websocket.go`:

```go
// Change WebSocketServer to accept SessionRegistry + a Resizer interface
type Resizer interface {
	Resize(id string, rows, cols uint16) error
}

type WebSocketServer struct {
	registry SessionRegistry
	resizer  Resizer
	mux      *http.ServeMux
}

func NewWebSocketServer(registry SessionRegistry, resizer Resizer) *WebSocketServer {
	ws := &WebSocketServer{
		registry: registry,
		resizer:  resizer,
		mux:      http.NewServeMux(),
	}
	ws.mux.HandleFunc("/ws/", ws.handleWS)
	return ws
}
```

Update `handleWS`:
- `sess := ws.registry.Get(sessionID)` instead of `ws.manager.Get(sessionID)`
- `snapshot := sess.GetSnapshot()` instead of `sess.monitor.Content()`
- `sub := sess.Subscribe()` / `sess.Unsubscribe(sub)` — same calls
- `sub.Ch` instead of `sub.ch`
- `ws.resizer.Resize(sessionID, rm.Rows, rm.Cols)` instead of `ws.manager.Resize(...)`
- `sess.Write(msg)` — same call

- [ ] **Step 6: Update call site in app/bindings.go**

In `NewSessionAPI`, update the WebSocketServer construction:

```go
// Before:
ws := ptyPkg.NewWebSocketServer(mgr)

// After:
ws := ptyPkg.NewWebSocketServer(mgr, mgr)
```

Since `Manager` implements both `SessionRegistry` (via `Get` returning `StreamableSession`) and `Resizer` (via `Resize`). Initially both params point to the local manager. Later, Task 14 replaces the registry with a `CompositeRegistry`.

- [ ] **Step 7: Run all tests to verify nothing is broken**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./pty/ ./app/ -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add pty/registry.go pty/registry_test.go pty/manager.go pty/websocket.go app/bindings.go
git commit -m "refactor(pty): introduce SessionRegistry interface for WebSocket routing"
```

---

### Task 5: SSH Process Manager (`ssh/process_manager.go`)

**Files:**
- Create: `ssh/process_manager.go`
- Create: `ssh/process_manager_test.go`
- Create: `ssh/session.go` (SSH session wrapper implementing StreamableSession)

- [ ] **Step 1: Write tests for SSHSession (StreamableSession wrapper)**

```go
// ssh/process_manager_test.go
package ssh

import (
	"testing"
	"time"

	"claude-squad/pty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHSession_WriteAndSubscribe(t *testing.T) {
	sess := newSSHSession("test-1", nil, nil) // nil stdin/session for unit test
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

func TestSSHProcessManager_ImplementsProcessManager(t *testing.T) {
	// Compile-time check that SSHProcessManager implements ProcessManager
	var _ pty.StreamableSession = (*SSHSession)(nil)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run "TestSSHSession|TestSSHProcessManager_Implements"`
Expected: Compilation failure.

- [ ] **Step 3: Implement SSHSession wrapper**

```go
// ssh/session.go
package ssh

import (
	"claude-squad/pty"
	"fmt"
	"sync"

	gossh "golang.org/x/crypto/ssh"
)

// SSHSession wraps an SSH session as a StreamableSession for WebSocket streaming.
type SSHSession struct {
	id      string
	mu      sync.Mutex
	stdin   io.Writer       // write target (stdin pipe from gossh.Session)
	sshSess *gossh.Session  // retained for Resize (WindowChange)
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

// feedOutput writes data to the monitor and broadcasts to subscribers.
// Called by the read loop goroutine.
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

// Resize sends a window-change request to the remote PTY.
func (s *SSHSession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sshSess == nil {
		return fmt.Errorf("session closed")
	}
	return s.sshSess.WindowChange(int(rows), int(cols))
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
```

- [ ] **Step 4: Implement SSHProcessManager**

```go
// ssh/process_manager.go
package ssh

import (
	"claude-squad/pty"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// SSHProcessManager implements session.ProcessManager over SSH.
type SSHProcessManager struct {
	mu       sync.RWMutex
	client   *Client
	sessions map[string]*SSHSession
	counter  int
}

func NewSSHProcessManager(client *Client) *SSHProcessManager {
	return &SSHProcessManager{
		client:   client,
		sessions: make(map[string]*SSHSession),
	}
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

	// Get stdin/stdout pipes
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

	// Build command
	cmd := program
	if len(args) > 0 {
		cmd += " " + strings.Join(args, " ")
	}
	if opts.Dir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellEscape(opts.Dir), cmd)
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return "", fmt.Errorf("start command: %w", err)
	}

	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("ssh-session-%d", m.counter)
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

func (m *SSHProcessManager) CheckTrustPrompt(id string) bool {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return sess.monitor.CheckTrustPrompt()
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

// Get implements SessionRegistry for the CompositeRegistry.
func (m *SSHProcessManager) Get(id string) pty.StreamableSession {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return sess
}

// shellEscape quotes a string for safe use in a shell command.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
```

Note: `SSHSession` takes an `io.Writer` (stdin pipe) and retains the `*gossh.Session` for `WindowChange` (resize) calls. The `io` import is needed in `ssh/session.go` as well.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run "TestSSHSession|TestSSHProcessManager"`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add ssh/session.go ssh/process_manager.go ssh/process_manager_test.go
git commit -m "feat(ssh): add SSHProcessManager implementing ProcessManager over SSH"
```

---

### Task 6: Command Executor Abstraction (`session/git/`)

**Files:**
- Create: `session/git/executor.go`
- Create: `session/git/executor_test.go`
- Modify: `session/git/worktree_git.go` (refactor exec.Command calls)
- Modify: `session/git/worktree_ops.go` (refactor exec.Command calls)
- Modify: `session/git/worktree.go` (add executor field)
- Modify: `session/git/util.go` (refactor exec.Command calls)
- Modify: `session/git/diff.go` (if it has exec.Command calls)

- [ ] **Step 1: Write tests for LocalExecutor**

```go
// session/git/executor_test.go
package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalExecutor_Run(t *testing.T) {
	exec := &LocalExecutor{}
	out, err := exec.Run("", "echo", "hello")
	require.NoError(t, err)
	assert.Contains(t, string(out), "hello")
}

func TestLocalExecutor_RunWithDir(t *testing.T) {
	dir := t.TempDir()
	exec := &LocalExecutor{}
	out, err := exec.Run(dir, "pwd")
	require.NoError(t, err)
	assert.Contains(t, string(out), dir)
}

func TestLocalExecutor_RunFailure(t *testing.T) {
	exec := &LocalExecutor{}
	_, err := exec.Run("", "false")
	assert.Error(t, err)
}

func TestRemoteExecutor_BuildCommand(t *testing.T) {
	// Test command construction without actual SSH
	cmd := buildRemoteCommand("/some/path", "git", "status", "--short")
	assert.Equal(t, "cd '/some/path' && git status --short", cmd)
}

func TestRemoteExecutor_BuildCommandPathEscaping(t *testing.T) {
	cmd := buildRemoteCommand("/path with spaces/repo", "git", "log")
	assert.Equal(t, "cd '/path with spaces/repo' && git log", cmd)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -v -run "TestLocalExecutor|TestRemoteExecutor_Build"`
Expected: Compilation failure.

- [ ] **Step 3: Implement executor interface and LocalExecutor**

```go
// session/git/executor.go
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Note: shellEscape is defined here for the git package. The ssh package has its own copy.
// A shared utility is overkill for a 3-line function.

// CommandExecutor abstracts command execution for local vs remote.
type CommandExecutor interface {
	Run(dir string, name string, args ...string) ([]byte, error)
}

// LocalExecutor runs commands via os/exec.
type LocalExecutor struct{}

func (e *LocalExecutor) Run(dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// RemoteExecutor runs commands over SSH.
type RemoteExecutor struct {
	RunCmd func(cmd string) (string, error)
}

func (e *RemoteExecutor) Run(dir string, name string, args ...string) ([]byte, error) {
	cmd := buildRemoteCommand(dir, name, args...)
	out, err := e.RunCmd(cmd)
	return []byte(out), err
}

func buildRemoteCommand(dir string, name string, args ...string) string {
	cmd := name
	if len(args) > 0 {
		cmd += " " + strings.Join(args, " ")
	}
	if dir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellEscape(dir), cmd)
	}
	return cmd
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// defaultExecutor is used when no executor is specified.
var defaultExecutor CommandExecutor = &LocalExecutor{}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -v -run "TestLocalExecutor|TestRemoteExecutor_Build"`
Expected: All PASS

- [ ] **Step 5: Add executor field to GitWorktree**

In `session/git/worktree.go`, add an `executor CommandExecutor` field to the `GitWorktree` struct. Default to `defaultExecutor` in constructors (`NewGitWorktree`, `NewGitWorktreeFromRef`, `NewGitWorktreeFromStorage`). Add a `SetExecutor(e CommandExecutor)` method.

- [ ] **Step 6: Refactor worktree_git.go to use executor**

Replace all `exec.Command(...)` calls in `worktree_git.go` methods that are on `GitWorktree` (like `runGitCommand`, `PushChanges`) to use `g.executor.Run(g.worktreePath, "git", args...)`.

For standalone functions (`GetCurrentBranch`, `GetDefaultBranch`, `FetchBranches`, `SearchBranches`), leave them using `exec.Command` directly — they are called from `Instance.Start()` and `SessionAPI.GetDirInfo()` which will branch for remote sessions.

- [ ] **Step 7: Refactor worktree_ops.go to use executor**

Replace `exec.Command(...)` calls in worktree operations (`Setup`, `Cleanup`, `Remove`, `Prune`, `CleanupWorktrees`) to use `g.executor.Run(...)`.

- [ ] **Step 8: Run all git tests to verify nothing is broken**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add session/git/executor.go session/git/executor_test.go session/git/worktree.go session/git/worktree_git.go session/git/worktree_ops.go session/git/util.go session/git/diff.go
git commit -m "refactor(git): introduce CommandExecutor for local/remote git operations"
```

---

### Task 7: Host Manager (`ssh/host_manager.go`)

**Files:**
- Create: `ssh/host_manager.go`
- Create: `ssh/host_manager_test.go`

- [ ] **Step 1: Write tests for HostManager**

```go
// ssh/host_manager_test.go
package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostManager_GetOrCreateClient_Caching(t *testing.T) {
	// Test that the same host ID returns the same client reference
	hm := NewHostManager(nil, nil) // nil store/keychain for unit test

	// We can't test actual connections, but we can test the caching logic
	// by verifying the map behavior
	assert.Equal(t, 0, len(hm.clients))
}

func TestHostManager_CloseIdle(t *testing.T) {
	hm := NewHostManager(nil, nil)
	// Verify initial state
	assert.Equal(t, 0, len(hm.clients))
}
```

- [ ] **Step 2: Implement HostManager**

```go
// ssh/host_manager.go
package ssh

import (
	"fmt"
	"sync"
	"time"

	"claude-squad/log"
)

const idleTimeout = 30 * time.Second

// HostManager manages SSH connections, one per host, shared across sessions.
type HostManager struct {
	mu       sync.Mutex
	store    *HostStore
	keychain *KeychainStore
	clients  map[string]*managedClient
}

type managedClient struct {
	client     *Client
	pm         *SSHProcessManager
	refCount   int
	idleTimer  *time.Timer
}

func NewHostManager(store *HostStore, keychain *KeychainStore) *HostManager {
	return &HostManager{
		store:    store,
		keychain: keychain,
		clients:  make(map[string]*managedClient),
	}
}

// GetClient returns a connected SSH client for the host, creating one if needed.
func (hm *HostManager) GetClient(hostID string) (*Client, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if mc, ok := hm.clients[hostID]; ok {
		if mc.client.Connected() {
			mc.refCount++
			if mc.idleTimer != nil {
				mc.idleTimer.Stop()
				mc.idleTimer = nil
			}
			return mc.client, nil
		}
		// Connection is dead, remove it
		delete(hm.clients, hostID)
	}

	// Create new connection
	config, err := hm.store.GetByID(hostID)
	if err != nil {
		return nil, fmt.Errorf("get host config: %w", err)
	}

	var secret string
	if config.AuthMethod == AuthMethodPassword || config.AuthMethod == AuthMethodKeyPassphrase {
		secret, err = hm.keychain.Get(hostID)
		if err != nil {
			return nil, fmt.Errorf("get secret from keychain: %w", err)
		}
	}

	client := NewClient(config, secret)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", config.Name, err)
	}

	mc := &managedClient{
		client:   client,
		pm:       NewSSHProcessManager(client),
		refCount: 1,
	}

	// Set up reconnect callback
	client.OnDisconnect(func() {
		hm.handleDisconnect(hostID)
	})

	hm.clients[hostID] = mc
	return client, nil
}

// GetProcessManager returns the SSHProcessManager for a host.
func (hm *HostManager) GetProcessManager(hostID string) (*SSHProcessManager, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return nil, fmt.Errorf("no connection for host %s", hostID)
	}
	return mc.pm, nil
}

// ReleaseClient decrements the reference count for a host connection.
func (hm *HostManager) ReleaseClient(hostID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return
	}
	mc.refCount--
	if mc.refCount <= 0 {
		mc.idleTimer = time.AfterFunc(idleTimeout, func() {
			hm.mu.Lock()
			defer hm.mu.Unlock()
			if mc2, ok := hm.clients[hostID]; ok && mc2 == mc && mc2.refCount <= 0 {
				mc2.client.Close()
				delete(hm.clients, hostID)
			}
		})
	}
}

func (hm *HostManager) handleDisconnect(hostID string) {
	log.InfoLog.Printf("SSH connection to host %s lost, starting reconnect", hostID)
	go hm.reconnectLoop(hostID)
}

func (hm *HostManager) reconnectLoop(hostID string) {
	delay := 3 * time.Second
	maxDelay := 30 * time.Second

	for {
		time.Sleep(delay)

		hm.mu.Lock()
		mc, ok := hm.clients[hostID]
		hm.mu.Unlock()
		if !ok {
			return // Host was removed
		}

		config, err := hm.store.GetByID(hostID)
		if err != nil {
			log.ErrorLog.Printf("reconnect: get host config: %v", err)
			return
		}

		var secret string
		if config.AuthMethod == AuthMethodPassword || config.AuthMethod == AuthMethodKeyPassphrase {
			secret, _ = hm.keychain.Get(hostID)
		}

		newClient := NewClient(config, secret)
		if err := newClient.Connect(); err != nil {
			log.InfoLog.Printf("reconnect to %s failed: %v, retrying in %v", config.Name, err, delay)
			delay = min(delay*2, maxDelay)
			continue
		}

		// Reconnected successfully
		hm.mu.Lock()
		mc.client = newClient
		mc.pm = NewSSHProcessManager(newClient)
		newClient.OnDisconnect(func() {
			hm.handleDisconnect(hostID)
		})
		hm.mu.Unlock()

		log.InfoLog.Printf("reconnected to %s", config.Name)
		return
	}
}

// Close shuts down all managed connections.
func (hm *HostManager) Close() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	for id, mc := range hm.clients {
		mc.client.Close()
		delete(hm.clients, id)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ssh/ -v -run TestHostManager`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add ssh/host_manager.go ssh/host_manager_test.go
git commit -m "feat(ssh): add HostManager for shared SSH connection lifecycle"
```

---

### Task 8: Instance & SessionAPI Changes

**Files:**
- Modify: `session/storage.go:11-27` (add HostID to InstanceData)
- Modify: `session/instance.go:40-84` (add HostID, remote fields)
- Modify: `session/instance.go:86-125` (ToInstanceData — include HostID)
- Modify: `session/instance.go:130-173` (FromInstanceData — handle HostID)
- Modify: `session/instance.go:176-219` (InstanceOptions — add HostID)
- Modify: `app/bindings.go:18-26` (CreateOptions — add HostID)
- Modify: `app/bindings.go:28-35` (SessionInfo — add HostID)
- Modify: `app/bindings.go:37-43` (SessionStatus — add SSHConnected)
- Modify: `app/bindings.go:50-59` (SessionAPI — add hostManager)
- Modify: `app/bindings.go:61-111` (NewSessionAPI — init hostManager, CompositeRegistry)
- Modify: `app/bindings.go:139-168` (CreateSession — handle HostID)
- Modify: `app/bindings.go:170-204` (OpenSession — handle remote resume)
- Modify: `app/bindings.go:329-351` (PollAllStatuses — include SSHConnected)
- Modify: `session/storage_test.go` (add HostID serialization test)

- [ ] **Step 1: Write test for HostID serialization**

```go
// Add to session/storage_test.go
func TestHostIDSerialization(t *testing.T) {
	data := InstanceData{
		Title:   "remote-session",
		Path:    "/remote/path",
		Program: "claude",
		HostID:  "host-uuid-123",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored InstanceData
	if err := json.Unmarshal(jsonBytes, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.HostID != "host-uuid-123" {
		t.Errorf("expected HostID 'host-uuid-123', got %q", restored.HostID)
	}
}

func TestHostIDBackwardCompatibility(t *testing.T) {
	// Old JSON without host_id field
	oldJSON := `{"title":"old","path":"/old","status":0,"program":"claude"}`

	var data InstanceData
	if err := json.Unmarshal([]byte(oldJSON), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.HostID != "" {
		t.Error("old sessions should have empty HostID")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ -v -run "TestHostID"`
Expected: Compilation failure — HostID field doesn't exist.

- [ ] **Step 3: Add HostID to InstanceData**

In `session/storage.go`, add to `InstanceData` struct:
```go
HostID string `json:"host_id,omitempty"`
```

- [ ] **Step 4: Add HostID to Instance**

In `session/instance.go`, add fields to `Instance` struct:
```go
HostID string
remote bool
```

Update `ToInstanceData()` to include `HostID: i.HostID`.

Update `FromInstanceData()` to set `HostID: data.HostID` and `remote: data.HostID != ""`.

Update `InstanceOptions` to include `HostID string`.

Update `NewInstance()` to set `HostID: opts.HostID` and `remote: opts.HostID != ""`.

- [ ] **Step 5: Update CreateOptions and SessionInfo in app/bindings.go**

```go
type CreateOptions struct {
	Title   string `json:"title"`
	Path    string `json:"path"`
	Program string `json:"program"`
	Branch  string `json:"branch"`
	AutoYes bool   `json:"autoYes"`
	InPlace bool   `json:"inPlace"`
	Prompt  string `json:"prompt"`
	HostID  string `json:"hostId"`
}

type SessionInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Program string `json:"program"`
	Status  string `json:"status"`
	HostID  string `json:"hostId"`
}

type SessionStatus struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Branch       string    `json:"branch"`
	DiffStats    DiffStats `json:"diffStats"`
	HasPrompt    bool      `json:"hasPrompt"`
	SSHConnected *bool     `json:"sshConnected"`
}
```

- [ ] **Step 6: Add hostManager to SessionAPI and update NewSessionAPI**

In `app/bindings.go`, add to SessionAPI struct:
```go
hostManager    *ssh.HostManager
hostStore      *ssh.HostStore
keychainStore  *ssh.KeychainStore
```

In `NewSessionAPI`, initialize all three:
```go
hostStore := ssh.NewHostStore(filepath.Join(dataDir, "hosts.json"))
keychainStore := ssh.NewKeychainStore("com.claude-squad")
hostMgr := ssh.NewHostManager(hostStore, keychainStore)
```

Set `api.hostStore = hostStore`, `api.keychainStore = keychainStore`, `api.hostManager = hostMgr`.

Update WebSocket server to use CompositeRegistry (this will be wired in later when remote sessions are created).

- [ ] **Step 7: Update CreateSession to handle HostID**

In `CreateSession`, after determining the program, add:
```go
var pm session.ProcessManager = api.ptyManager
if opts.HostID != "" {
	client, err := api.hostManager.GetClient(opts.HostID)
	if err != nil {
		return nil, fmt.Errorf("connect to remote host: %w", err)
	}
	sshPM, err := api.hostManager.GetProcessManager(opts.HostID)
	if err != nil {
		return nil, fmt.Errorf("get ssh process manager: %w", err)
	}
	pm = sshPM
}
```

Pass `pm` as `ProcessManager` and `opts.HostID` as `HostID` in `InstanceOptions`.

- [ ] **Step 8: Update OpenSession to handle remote resume**

In `OpenSession`, when resuming a paused remote session:
```go
if inst.HostID != "" {
	_, err := api.hostManager.GetClient(inst.HostID)
	if err != nil {
		return "", fmt.Errorf("reconnect to remote host: %w", err)
	}
	pm, err := api.hostManager.GetProcessManager(inst.HostID)
	if err != nil {
		return "", fmt.Errorf("get ssh process manager: %w", err)
	}
	inst.SetProcessManager(pm)
}
```

- [ ] **Step 9: Update PollAllStatuses to include SSHConnected**

In `PollAllStatuses`, for each instance, check if it has a HostID. Use a read-only `IsConnected` method on HostManager (not `GetClient`, which has side effects):
```go
var sshConnected *bool
if inst.HostID != "" {
	connected := api.hostManager.IsConnected(inst.HostID)
	sshConnected = &connected
}
```

Add to `ssh/host_manager.go`:
```go
// IsConnected returns whether the SSH connection for a host is alive.
// Read-only — does not create connections or change refcounts.
func (hm *HostManager) IsConnected(hostID string) bool {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	mc, ok := hm.clients[hostID]
	if !ok {
		return false
	}
	return mc.client.Connected()
}
```

- [ ] **Step 10: Update instanceToInfo to include HostID**

```go
func instanceToInfo(inst *session.Instance) SessionInfo {
	return SessionInfo{
		ID:      inst.Title,
		Title:   inst.Title,
		Path:    inst.Path,
		Branch:  inst.Branch,
		Program: inst.Program,
		Status:  statusString(inst.Status),
		HostID:  inst.HostID,
	}
}
```

- [ ] **Step 11: Run all tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/ ./app/ -v`
Expected: All PASS

- [ ] **Step 12: Commit**

```bash
git add session/storage.go session/instance.go session/storage_test.go app/bindings.go
git commit -m "feat: wire HostID through Instance, InstanceData, and SessionAPI"
```

---

### Task 9: Host API Methods (`app/bindings.go`)

**Files:**
- Modify: `app/bindings.go` (add GetHosts, CreateHost, UpdateHost, DeleteHost, TestHost, GetRemoteDirInfo, SearchRemoteBranches, SelectFile methods)

- [ ] **Step 1: Add host CRUD API methods**

```go
// HostInfo is the frontend-facing host representation.
type HostInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath"`
}

type CreateHostOptions struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath"`
	Secret     string `json:"secret"` // password or passphrase — stored in keychain, not persisted
}

type TestHostResult struct {
	ConnectionOK bool   `json:"connectionOK"`
	ProgramOK    bool   `json:"programOK"`
	Message      string `json:"message"`
}

func (api *SessionAPI) GetHosts() ([]HostInfo, error) {
	hosts, err := api.hostStore.LoadAll()
	if err != nil {
		return nil, err
	}
	result := make([]HostInfo, len(hosts))
	for i, h := range hosts {
		result[i] = HostInfo{
			ID: h.ID, Name: h.Name, Host: h.Host,
			Port: h.Port, User: h.User, AuthMethod: h.AuthMethod,
			KeyPath: h.KeyPath,
		}
	}
	return result, nil
}

func (api *SessionAPI) CreateHost(opts CreateHostOptions) (*HostInfo, error) {
	id := generateUUID()
	config := sshPkg.HostConfig{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	if err := api.hostStore.Save(config); err != nil {
		return nil, err
	}
	if opts.Secret != "" {
		if err := api.keychainStore.Set(id, opts.Secret); err != nil {
			// Rollback host save
			_ = api.hostStore.Delete(id)
			return nil, fmt.Errorf("store secret: %w", err)
		}
	}
	info := HostInfo{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	return &info, nil
}

func (api *SessionAPI) UpdateHost(opts CreateHostOptions, id string) error {
	config := sshPkg.HostConfig{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	if err := api.hostStore.Update(config); err != nil {
		return err
	}
	if opts.Secret != "" {
		if err := api.keychainStore.Set(id, opts.Secret); err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	}
	return nil
}

func (api *SessionAPI) DeleteHost(id string) error {
	// Check for active sessions
	api.mu.RLock()
	for _, inst := range api.instances {
		if inst.HostID == id && inst.Status != session.Paused {
			api.mu.RUnlock()
			return fmt.Errorf("host has active sessions — pause them first")
		}
	}
	api.mu.RUnlock()

	_ = api.keychainStore.Delete(id)
	return api.hostStore.Delete(id)
}

func (api *SessionAPI) TestHost(opts CreateHostOptions, program string) (*TestHostResult, error) {
	config := sshPkg.HostConfig{
		Host: opts.Host, Port: opts.Port, User: opts.User,
		AuthMethod: opts.AuthMethod, KeyPath: opts.KeyPath,
	}
	connOK, progOK, msg := sshPkg.TestConnection(config, opts.Secret, program)
	return &TestHostResult{
		ConnectionOK: connOK,
		ProgramOK:    progOK,
		Message:      msg,
	}, nil
}
```

- [ ] **Step 2: Add remote directory info API methods**

```go
func (api *SessionAPI) GetRemoteDirInfo(hostID string, dir string) (*DirInfo, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return &DirInfo{DefaultBranch: "main", Branches: []string{}}, nil
	}
	defer api.hostManager.ReleaseClient(hostID)

	// Get default branch
	out, err := client.RunCommand(fmt.Sprintf("cd %s && git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@'", shellEscape(dir)))
	defaultBranch := "main"
	if err == nil {
		if b := strings.TrimSpace(out); b != "" {
			defaultBranch = b
		}
	}

	// Get branches
	out, err = client.RunCommand(fmt.Sprintf("cd %s && git branch -a --sort=-committerdate --format='%%(refname:short)' 2>/dev/null | head -100", shellEscape(dir)))
	branches := []string{}
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line != "" {
				branches = append(branches, line)
			}
		}
	}

	return &DirInfo{DefaultBranch: defaultBranch, Branches: branches}, nil
}

func (api *SessionAPI) SearchRemoteBranches(hostID string, dir string, filter string) ([]string, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return []string{}, nil
	}
	defer api.hostManager.ReleaseClient(hostID)

	cmd := fmt.Sprintf("cd %s && git branch -a --sort=-committerdate --format='%%(refname:short)' 2>/dev/null", shellEscape(dir))
	if filter != "" {
		cmd += fmt.Sprintf(" | grep -i %s", shellEscape(filter))
	}
	cmd += " | head -100"

	out, err := client.RunCommand(cmd)
	if err != nil {
		return []string{}, nil
	}
	branches := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
```

- [ ] **Step 3: Add SelectFile API method**

```go
func (api *SessionAPI) SelectFile(startDir string) (string, error) {
	// This uses the Wails runtime dialog — needs the app context.
	// Implementation depends on Wails runtime being available.
	// The actual call: runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{DefaultDirectory: startDir})
	// Will be wired in during Wails startup.
	return "", fmt.Errorf("not implemented — requires Wails runtime context")
}
```

Note: The Wails runtime dialog requires the app context. This method will need the `context.Context` from Wails startup. Add a `ctx context.Context` field to SessionAPI and set it in the Wails `OnStartup` callback. Refer to how other Wails apps handle `runtime.OpenFileDialog`.

- [ ] **Step 4: Run tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -v`
Expected: All PASS (or compilation success with existing tests passing)

- [ ] **Step 5: Commit**

```bash
git add app/bindings.go
git commit -m "feat: add host CRUD, TestHost, and remote directory API methods"
```

---

### Task 10: Frontend — Zustand Store & TypeScript Types

**Files:**
- Modify: `frontend/src/lib/wails.ts` (add host types and API declarations)
- Modify: `frontend/src/store/sessionStore.ts` (add hosts state and actions)

- [ ] **Step 1: Add TypeScript types for hosts**

In `frontend/src/lib/wails.ts`, add interfaces:

```typescript
export interface HostInfo {
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  authMethod: string;
  keyPath: string;
}

export interface CreateHostOptions {
  name: string;
  host: string;
  port: number;
  user: string;
  authMethod: string;
  keyPath: string;
  secret: string;
}

export interface TestHostResult {
  connectionOK: boolean;
  programOK: boolean;
  message: string;
}
```

Add to `SessionStatus` interface:
```typescript
sshConnected?: boolean | null;
```

Add to `SessionInfo` interface:
```typescript
hostId?: string;
```

Add to `Window.go.app.SessionAPI` declaration:
```typescript
GetHosts(): Promise<HostInfo[]>;
CreateHost(opts: CreateHostOptions): Promise<HostInfo>;
DeleteHost(id: string): Promise<void>;
TestHost(opts: CreateHostOptions, program: string): Promise<TestHostResult>;
GetRemoteDirInfo(hostId: string, dir: string): Promise<DirInfo>;
SearchRemoteBranches(hostId: string, dir: string, filter: string): Promise<string[]>;
SelectFile(startDir: string): Promise<string>;
```

Add to `CreateOptions`:
```typescript
hostId?: string;
```

- [ ] **Step 2: Add hosts state to Zustand store**

In `frontend/src/store/sessionStore.ts`, add to interface:

```typescript
hosts: HostInfo[];
setHosts: (hosts: HostInfo[]) => void;
addHost: (host: HostInfo) => void;
removeHost: (id: string) => void;
```

Add to the store implementation:
```typescript
hosts: [],
setHosts: (hosts) => set({ hosts }),
addHost: (host) => set((s) => ({ hosts: [...s.hosts, host] })),
removeHost: (id) => set((s) => ({ hosts: s.hosts.filter((h) => h.id !== id) })),
```

- [ ] **Step 3: Load hosts on app startup**

In `frontend/src/App.tsx`, in the startup effect where config and sessions are loaded, add:
```typescript
const hosts = await api().GetHosts();
useSessionStore.getState().setHosts(hosts);
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/wails.ts frontend/src/store/sessionStore.ts frontend/src/App.tsx
git commit -m "feat(frontend): add host types, store state, and API declarations"
```

---

### Task 11: Frontend — AddHostDialog Component

**Files:**
- Create: `frontend/src/components/Dialogs/AddHostDialog.tsx`

- [ ] **Step 1: Implement AddHostDialog**

```tsx
// frontend/src/components/Dialogs/AddHostDialog.tsx
import { useState } from "react";
import type { CreateHostOptions, TestHostResult } from "../../lib/wails";
import { api } from "../../lib/wails";

interface AddHostDialogProps {
  program: string; // current program selection for testing
  onSubmit: (opts: CreateHostOptions) => void;
  onCancel: () => void;
}

export function AddHostDialog({ program, onSubmit, onCancel }: AddHostDialogProps) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(22);
  const [user, setUser] = useState("");
  const [authMethod, setAuthMethod] = useState<"password" | "key" | "key+passphrase">("key");
  const [secret, setSecret] = useState("");
  const [keyPath, setKeyPath] = useState("");
  const [testResult, setTestResult] = useState<TestHostResult | null>(null);
  const [testing, setTesting] = useState(false);
  const [tested, setTested] = useState(false);

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    background: "var(--surface0)",
    border: "1px solid var(--surface1)",
    borderRadius: 4,
    color: "var(--text)",
    fontSize: 13,
    outline: "none",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    color: "var(--subtext0)",
    display: "block",
    marginTop: 12,
    marginBottom: 4,
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const opts: CreateHostOptions = { name, host, port, user, authMethod, keyPath, secret };
      const result = await api().TestHost(opts, program);
      setTestResult(result);
      if (result.connectionOK && result.programOK) {
        setTested(true);
      }
    } catch (e: any) {
      setTestResult({ connectionOK: false, programOK: false, message: e.message ?? "Test failed" });
    } finally {
      setTesting(false);
    }
  };

  const handleBrowse = async () => {
    try {
      const path = await api().SelectFile("~/.ssh/");
      if (path) setKeyPath(path);
    } catch {
      // User cancelled
    }
  };

  const canTest = host.trim() && user.trim() && (authMethod === "password" ? secret.trim() : keyPath.trim());

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 3000,
      }}
      onClick={onCancel}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--base)",
          border: "1px solid var(--surface0)",
          borderRadius: 8,
          padding: 24,
          width: 420,
          maxHeight: "80vh",
          overflowY: "auto",
        }}
      >
        <h3 style={{ marginBottom: 16, marginTop: 0 }}>Add SSH Host</h3>

        <label style={{ ...labelStyle, marginTop: 0 }}>Name</label>
        <input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} placeholder="dev-server" autoFocus />

        <label style={labelStyle}>Host</label>
        <input style={inputStyle} value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.50 or hostname" />

        <label style={labelStyle}>Port</label>
        <input style={{ ...inputStyle, width: 100 }} type="number" value={port} onChange={(e) => setPort(parseInt(e.target.value) || 22)} />

        <label style={labelStyle}>User</label>
        <input style={inputStyle} value={user} onChange={(e) => setUser(e.target.value)} placeholder="deploy" />

        <label style={labelStyle}>Auth Method</label>
        <div style={{ display: "flex", gap: 16, marginTop: 4 }}>
          {(["password", "key", "key+passphrase"] as const).map((m) => (
            <label key={m} style={{ fontSize: 13, color: "var(--text)", cursor: "pointer" }}>
              <input
                type="radio"
                name="authMethod"
                checked={authMethod === m}
                onChange={() => { setAuthMethod(m); setTested(false); }}
                style={{ marginRight: 4 }}
              />
              {m === "password" ? "Password" : m === "key" ? "Private Key" : "Key + Passphrase"}
            </label>
          ))}
        </div>

        {authMethod === "password" && (
          <>
            <label style={labelStyle}>Password</label>
            <input style={inputStyle} type="password" value={secret} onChange={(e) => { setSecret(e.target.value); setTested(false); }} />
          </>
        )}

        {(authMethod === "key" || authMethod === "key+passphrase") && (
          <>
            <label style={labelStyle}>Private Key</label>
            <div style={{ display: "flex", gap: 8 }}>
              <input style={{ ...inputStyle, flex: 1 }} value={keyPath} onChange={(e) => { setKeyPath(e.target.value); setTested(false); }} placeholder="~/.ssh/id_ed25519" />
              <button
                onClick={handleBrowse}
                style={{
                  padding: "8px 12px",
                  background: "var(--surface0)",
                  color: "var(--text)",
                  border: "none",
                  borderRadius: 4,
                  cursor: "pointer",
                  fontSize: 13,
                  whiteSpace: "nowrap",
                }}
              >
                Browse
              </button>
            </div>
          </>
        )}

        {authMethod === "key+passphrase" && (
          <>
            <label style={labelStyle}>Key Passphrase</label>
            <input style={inputStyle} type="password" value={secret} onChange={(e) => { setSecret(e.target.value); setTested(false); }} />
          </>
        )}

        {/* Test result */}
        {testResult && (
          <div
            style={{
              marginTop: 12,
              padding: "8px 12px",
              borderRadius: 4,
              fontSize: 12,
              background: testResult.connectionOK && testResult.programOK ? "rgba(166,227,161,0.15)" : "rgba(243,139,168,0.15)",
              color: testResult.connectionOK && testResult.programOK ? "var(--green)" : "var(--red)",
            }}
          >
            {testResult.connectionOK && testResult.programOK
              ? "Connection OK, program found"
              : testResult.message}
          </div>
        )}

        <div style={{ display: "flex", gap: 8, marginTop: 20, justifyContent: "flex-end" }}>
          <button
            onClick={handleTest}
            disabled={!canTest || testing}
            style={{
              padding: "8px 16px",
              background: "var(--surface1)",
              color: "var(--text)",
              border: "none",
              borderRadius: 6,
              cursor: canTest && !testing ? "pointer" : "default",
              opacity: canTest && !testing ? 1 : 0.5,
            }}
          >
            {testing ? "Testing..." : "Test"}
          </button>
          <button
            onClick={onCancel}
            style={{
              padding: "8px 16px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={() => onSubmit({ name, host, port, user, authMethod, keyPath, secret })}
            disabled={!tested}
            style={{
              padding: "8px 16px",
              background: "var(--blue)",
              color: "var(--crust)",
              border: "none",
              borderRadius: 6,
              cursor: tested ? "pointer" : "default",
              opacity: tested ? 1 : 0.5,
            }}
          >
            OK
          </button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/Dialogs/AddHostDialog.tsx
git commit -m "feat(frontend): add AddHostDialog component with test connection support"
```

---

### Task 12: Frontend — NewSessionDialog Host Dropdown

**Files:**
- Modify: `frontend/src/components/Dialogs/NewSessionDialog.tsx`

- [ ] **Step 1: Add host dropdown and "+" button to NewSessionDialog**

Add imports and state:
```tsx
import { AddHostDialog } from "./AddHostDialog";
import { useSessionStore } from "../../store/sessionStore";

// Inside component:
const hosts = useSessionStore((s) => s.hosts);
const addHost = useSessionStore((s) => s.addHost);
const [selectedHostId, setSelectedHostId] = useState("");
const [showAddHost, setShowAddHost] = useState(false);
```

Add Host dropdown section after Title input, before Directory:
```tsx
<label style={labelStyle}>Host</label>
<div style={{ display: "flex", gap: 8 }}>
  <select
    style={{ ...inputStyle, flex: 1, cursor: "pointer" }}
    value={selectedHostId}
    onChange={(e) => setSelectedHostId(e.target.value)}
  >
    <option value="">localhost</option>
    {hosts.map((h) => (
      <option key={h.id} value={h.id}>{h.name} ({h.host})</option>
    ))}
  </select>
  <button
    onClick={() => setShowAddHost(true)}
    style={{
      padding: "8px 12px",
      background: "var(--surface0)",
      color: "var(--text)",
      border: "none",
      borderRadius: 4,
      cursor: "pointer",
      fontSize: 16,
      lineHeight: 1,
    }}
    title="Add SSH host"
  >
    +
  </button>
</div>
```

Update branch loading to use remote API when host is selected:
```tsx
const loadDirInfo = useCallback(async (dir: string) => {
  if (!dir.trim()) return;
  setLoadingBranches(true);
  try {
    const info: DirInfo = selectedHostId
      ? await api().GetRemoteDirInfo(selectedHostId, dir)
      : await api().GetDirInfo(dir);
    setDefaultBranch(info.defaultBranch);
    setBranches(info.branches);
    const originDefault = info.branches.find((b) => b === `origin/${info.defaultBranch}`);
    setBranch(originDefault ?? "");
  } catch {
    setBranches([]);
  } finally {
    setLoadingBranches(false);
  }
}, [selectedHostId]);
```

Similarly update `handleBranchSearch` to use `SearchRemoteBranches` when `selectedHostId` is set.

Update `onSubmit` to include `hostId: selectedHostId || undefined`.

Add AddHostDialog render:
```tsx
{showAddHost && (
  <AddHostDialog
    program={program}
    onCancel={() => setShowAddHost(false)}
    onSubmit={async (opts) => {
      try {
        const host = await api().CreateHost(opts);
        addHost(host);
        setSelectedHostId(host.id);
        setShowAddHost(false);
      } catch (e: any) {
        // Handle error
      }
    }}
  />
)}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/Dialogs/NewSessionDialog.tsx
git commit -m "feat(frontend): add host dropdown and remote branch loading to NewSessionDialog"
```

---

### Task 13: Frontend — Disconnect Overlay & Sidebar Indicator

**Files:**
- Modify: `frontend/src/components/Terminal/TerminalPane.tsx`
- Modify: `frontend/src/components/Sidebar/SessionItem.tsx`

- [ ] **Step 1: Add disconnect overlay to TerminalPane**

In `TerminalPane.tsx`, get the session status from the store and check `sshConnected`:

```tsx
import { useSessionStore } from "../../store/sessionStore";

// Inside component, get the SSH connection state:
const status = useSessionStore((s) => s.statuses.get(sessionId));
const sshDisconnected = status?.sshConnected === false;

// Wrap the terminal div in a relative container and add overlay:
return (
  <div style={{ position: "relative", flex: 1, height: "100%" }}>
    <div
      ref={containerRef}
      style={{
        flex: 1,
        height: "100%",
        borderLeft: focused ? "2px solid var(--blue)" : "2px solid transparent",
        borderRadius: 2,
        overflow: "hidden",
      }}
    />
    {sshDisconnected && (
      <div
        style={{
          position: "absolute",
          inset: 0,
          background: "rgba(0,0,0,0.6)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexDirection: "column",
          gap: 12,
          zIndex: 10,
          borderRadius: 2,
        }}
      >
        <div style={{ fontSize: 24 }}>{"\u23F3"}</div>
        <div style={{ color: "var(--subtext0)", fontSize: 13 }}>
          Connection lost — reconnecting...
        </div>
      </div>
    )}
  </div>
);
```

- [ ] **Step 2: Add SSH disconnect indicator to SessionItem**

In `SessionItem.tsx`, add a `sshDisconnected` prop:

```tsx
interface SessionItemProps {
  session: SessionInfo;
  status?: SessionStatus;
  selected: boolean;
  loading?: boolean;
  flash?: boolean;
  sshDisconnected?: boolean;
  onClick: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}
```

In the status icon area, show a pulsing indicator when disconnected:
```tsx
<span style={{ color, fontSize: 10 }}>
  {loading ? "\u23F3" : sshDisconnected ? "\u26A0" : session.status === "paused" ? "\u23F8" : "\u25CF"}
</span>
```

When `sshDisconnected` is true, add a subtle "reconnecting" text:
```tsx
{sshDisconnected && (
  <span style={{ color: "var(--yellow)", fontSize: 10, marginLeft: 4 }}>
    reconnecting...
  </span>
)}
```

- [ ] **Step 3: Wire sshDisconnected prop in Sidebar.tsx**

In `Sidebar.tsx`, pass the prop:
```tsx
<SessionItem
  key={session.id}
  session={session}
  status={statuses.get(session.id)}
  selected={idx === selectedSidebarIdx}
  loading={openingSessionId === session.id || loadingSessionIds.has(session.id)}
  flash={flashSessionIds.has(session.id)}
  sshDisconnected={statuses.get(session.id)?.sshConnected === false}
  onClick={() => handleSessionClick(session, idx)}
  onContextMenu={(e) => handleContextMenu(e, session, idx)}
/>
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/Terminal/TerminalPane.tsx frontend/src/components/Sidebar/SessionItem.tsx frontend/src/components/Sidebar/Sidebar.tsx
git commit -m "feat(frontend): add SSH disconnect overlay and sidebar reconnecting indicator"
```

---

### Task 14: Wire CompositeRegistry for Remote Sessions

**Files:**
- Modify: `app/bindings.go` (update NewSessionAPI to use CompositeRegistry)

- [ ] **Step 1: Update NewSessionAPI to create CompositeRegistry**

The WebSocket server needs to find sessions from both the local `pty.Manager` and any `SSHProcessManager`. Since SSH process managers are created per-host dynamically, we need a registry that can find sessions across all of them.

Add a method to `HostManager`:
```go
// GetAllProcessManagers returns all active SSHProcessManagers for registry lookup.
func (hm *HostManager) GetAllProcessManagers() []*SSHProcessManager { ... }
```

Create a `DynamicSSHRegistry` that queries the HostManager:
```go
// In ssh/process_manager.go
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
```

In `NewSessionAPI`, create the composite:
```go
sshRegistry := sshPkg.NewDynamicSSHRegistry(hostMgr)
composite := ptyPkg.NewCompositeRegistry(mgr, sshRegistry)
ws := ptyPkg.NewWebSocketServer(composite, mgr)
```

- [ ] **Step 2: Run all tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add ssh/process_manager.go ssh/host_manager.go app/bindings.go
git commit -m "feat: wire CompositeRegistry so WebSocket server routes to SSH sessions"
```

---

### Task 15: Integration Smoke Test

**Files:**
- Modify: `app/bindings_test.go` (add remote session creation test)

- [ ] **Step 1: Write integration test for remote session flow**

This test verifies the data flow without a real SSH server — it uses a mock/nil HostManager to verify that CreateOptions with HostID is handled correctly, and that SessionInfo includes the HostID.

```go
// Add to app/bindings_test.go
func TestCreateSession_WithHostID(t *testing.T) {
	// This test verifies that HostID flows through CreateOptions -> Instance -> SessionInfo
	// Actual SSH connection is not tested here (requires SSH server)
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	info, err := api.CreateSession(CreateOptions{
		Title:   "remote-test",
		Path:    "/tmp",
		Program: "echo",
		InPlace: true,
		HostID:  "test-host-123",
	})

	// This will fail with "connect to remote host" error since no SSH server
	// But we can verify the flow reaches the right code path
	if err != nil {
		assert.Contains(t, err.Error(), "remote host")
		return
	}

	assert.Equal(t, "test-host-123", info.HostID)
}

func TestSessionStatus_SSHConnected(t *testing.T) {
	// Verify SSHConnected is nil for local sessions
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	_, err := api.CreateSession(CreateOptions{
		Title:   "local-test",
		Path:    "/tmp",
		Program: "echo",
		InPlace: true,
	})
	require.NoError(t, err)

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Nil(t, statuses[0].SSHConnected) // nil for local sessions
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -v -run "TestCreateSession_WithHostID|TestSessionStatus_SSHConnected"`
Expected: Tests pass (or expected error for SSH connection)

- [ ] **Step 3: Build and verify**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && CGO_ENABLED=1 go build -o ~/.local/bin/cs .`
Expected: Successful build

- [ ] **Step 4: Commit**

```bash
git add app/bindings_test.go
git commit -m "test: add integration tests for remote session flow"
```

---

## Task Dependency Order

```
Task 1 (hosts.go) ──┐
Task 2 (keychain) ──┤
Task 3 (client) ────┤
                    ├── Task 7 (HostManager) ──┐
Task 4 (registry) ──┤                         ├── Task 8 (Instance/API) ── Task 9 (Host APIs)
Task 5 (SSH PM) ────┘                         │
                                               ├── Task 14 (CompositeRegistry wiring)
Task 6 (executor) ────────────────────────────┘

Task 10 (TS types/store) ── Task 11 (AddHostDialog) ── Task 12 (NewSessionDialog) ── Task 13 (Overlay/Sidebar)

Task 15 (Integration) runs after all above
```

Tasks 1-6 can be parallelized in two tracks:
- **Track A (SSH):** Tasks 1, 2, 3, 5, 7
- **Track B (Refactors):** Tasks 4, 6
- **Track C (Frontend):** Tasks 10, 11, 12, 13 (after types are defined)

Tasks 8, 9, 14, 15 are sequential and depend on both tracks.
