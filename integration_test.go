package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"claude-squad/pty"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_SpawnAndWebSocket(t *testing.T) {
	mgr := pty.NewManager()
	defer mgr.Close()

	ws := pty.NewWebSocketServer(mgr, mgr)
	port, err := ws.ListenAndServe()
	require.NoError(t, err)

	// Spawn a shell that echoes a marker string
	id, err := mgr.Spawn("/bin/sh", []string{"-c", "echo integration-test-output; sleep 5"}, pty.SpawnOptions{})
	require.NoError(t, err)

	// Connect WebSocket
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws/%s", port, id)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Read output via WebSocket
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	found := false
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "integration-test-output") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to read output via WebSocket")

	// Verify monitor captured it too
	content := mgr.GetContent(id)
	assert.Contains(t, content, "integration-test-output")
}
