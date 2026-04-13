package app

import (
	"claude-squad/config"
	logPkg "claude-squad/log"
	ptyPkg "claude-squad/pty"
	"claude-squad/session"
	sshPkg "claude-squad/ssh"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var initLogOnce sync.Once

func TestSessionAPI_CreateAndLoad(t *testing.T) {
	api := newTestAPI(t)

	info, err := api.CreateSession(CreateOptions{
		Title:   "test-session",
		Path:    "/tmp",
		Program: "echo hello",
	})
	require.NoError(t, err)
	assert.Equal(t, "test-session", info.Title)
	assert.NotEmpty(t, info.ID)

	sessions, err := api.LoadSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "test-session", sessions[0].Title)
}

func TestSessionAPI_GetWebSocketPort(t *testing.T) {
	api := newTestAPI(t)
	port := api.GetWebSocketPort()
	assert.Greater(t, port, 0)
}

func TestCreateSession_WithHostID(t *testing.T) {
	api := newTestAPI(t)

	_, err := api.CreateSession(CreateOptions{
		Title:   "remote-test",
		Path:    "/tmp",
		Program: "echo",
		InPlace: true,
		HostID:  "test-host-123",
	})

	// Should fail with connection error since no real SSH server
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote host")
}

func TestSessionStatus_SSHConnected(t *testing.T) {
	api := newTestAPI(t)

	_, err := api.CreateSession(CreateOptions{
		Title:   "local-test",
		Path:    "/tmp",
		Program: "echo hello",
		InPlace: true,
	})
	require.NoError(t, err)

	statuses, err := api.PollAllStatuses()
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Nil(t, statuses[0].SSHConnected) // nil for local sessions
}

// newTestAPI creates a SessionAPI with isolated storage (empty state).
func newTestAPI(t *testing.T) *SessionAPI {
	t.Helper()

	initLogOnce.Do(func() { logPkg.Initialize(false) })

	// Override HOME so SaveState writes to a temp dir instead of the real
	// ~/.claude-squad/state.json. Without this, test cleanup overwrites the
	// user's real session data.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	mgr := ptyPkg.NewManager()
	ws := ptyPkg.NewWebSocketServer(mgr, mgr)
	port, err := ws.ListenAndServe()
	require.NoError(t, err)

	// Use empty state so tests don't load real sessions
	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	cfg := config.LoadConfig()

	tmpDir := t.TempDir()
	hostStore := sshPkg.NewHostStore(filepath.Join(tmpDir, "hosts.json"))
	keychainStore := sshPkg.NewKeychainStore("com.claude-squad.test")
	hostMgr := sshPkg.NewHostManager(hostStore, keychainStore)

	api := &SessionAPI{
		instances:     make(map[string]*session.Instance),
		storage:       storage,
		ptyManager:    mgr,
		wsServer:      ws,
		wsPort:        port,
		cfg:           cfg,
		hostManager:   hostMgr,
		hostStore:     hostStore,
		keychainStore: keychainStore,
		indexers:      make(map[string]Indexer),
		mcpServer:     NewMCPIndexServer(nil),
	}
	t.Cleanup(func() { api.Close() })
	return api
}

func TestSetMCPConfig_Disabled(t *testing.T) {
	api := &SessionAPI{
		mcpServer: &MCPIndexServer{},
	}
	// Start MCP server so it has a port
	api.mcpServer.port = 12345

	inst := &session.Instance{
		Title:   "test",
		Program: "claude",
	}

	// MCPEnabled = false should not set config
	disabled := false
	opts := CreateOptions{MCPEnabled: &disabled}
	api.setMCPConfigWithOpts(inst, opts)
	assert.Empty(t, inst.MCPConfig)

	// MCPEnabled = true should set config
	enabled := true
	opts = CreateOptions{MCPEnabled: &enabled}
	api.setMCPConfigWithOpts(inst, opts)
	assert.NotEmpty(t, inst.MCPConfig)

	// MCPEnabled = nil (default) should set config
	opts = CreateOptions{MCPEnabled: nil}
	api.setMCPConfigWithOpts(inst, opts)
	assert.NotEmpty(t, inst.MCPConfig)
}
