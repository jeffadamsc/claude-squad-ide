package pty

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketServer_ConnectAndRead(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ws := NewWebSocketServer(mgr, mgr)
	server := httptest.NewServer(ws.Handler())
	defer server.Close()

	id, err := mgr.Spawn("/bin/sh", []string{"-c", "echo ws-test-output; sleep 2"}, SpawnOptions{})
	require.NoError(t, err)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/" + id
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	found := false
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "ws-test-output") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to read output via WebSocket")
}

func TestWebSocketServer_WriteInput(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ws := NewWebSocketServer(mgr, mgr)
	server := httptest.NewServer(ws.Handler())
	defer server.Close()

	id, err := mgr.Spawn("/bin/sh", []string{}, SpawnOptions{})
	require.NoError(t, err)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/" + id
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteMessage(websocket.BinaryMessage, []byte("echo hello-from-ws\n"))
	require.NoError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	conn.SetReadDeadline(deadline)
	found := false
	for time.Now().Before(deadline) {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "hello-from-ws") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to read echoed input from WebSocket")
}

func TestWebSocketServer_InvalidSession(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ws := NewWebSocketServer(mgr, mgr)
	server := httptest.NewServer(ws.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/nonexistent"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	}
}

func TestWebSocketServer_Resize(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ws := NewWebSocketServer(mgr, mgr)
	server := httptest.NewServer(ws.Handler())
	defer server.Close()

	id, err := mgr.Spawn("/bin/sh", []string{}, SpawnOptions{})
	require.NoError(t, err)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/" + id
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","rows":48,"cols":120}`))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
}
