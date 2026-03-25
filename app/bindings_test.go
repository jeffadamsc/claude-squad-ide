package app

import (
	"claude-squad/config"
	ptyPkg "claude-squad/pty"
	"claude-squad/session"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// newTestAPI creates a SessionAPI with isolated storage (empty state).
func newTestAPI(t *testing.T) *SessionAPI {
	t.Helper()

	mgr := ptyPkg.NewManager()
	ws := ptyPkg.NewWebSocketServer(mgr, mgr)
	port, err := ws.ListenAndServe()
	require.NoError(t, err)

	// Use empty state so tests don't load real sessions
	state := config.DefaultState()
	storage, err := session.NewStorage(state)
	require.NoError(t, err)

	cfg := config.LoadConfig()

	api := &SessionAPI{
		instances:  make(map[string]*session.Instance),
		storage:    storage,
		ptyManager: mgr,
		wsServer:   ws,
		wsPort:     port,
		cfg:        cfg,
	}
	t.Cleanup(func() { api.Close() })
	return api
}
