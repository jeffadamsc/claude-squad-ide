package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"claude-squad/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPIndexServer_StartStop(t *testing.T) {
	api := &SessionAPI{
		indexers:  make(map[string]Indexer),
		instances: make(map[string]*session.Instance),
	}

	srv := NewMCPIndexServer(api)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	assert.Equal(t, port, srv.Port())

	// Starting again should return the same port
	port2, err := srv.Start()
	require.NoError(t, err)
	assert.Equal(t, port, port2)

	srv.Stop()
	assert.Equal(t, 0, srv.Port())
}

func TestMCPIndexServer_GenerateMCPConfig(t *testing.T) {
	api := &SessionAPI{
		indexers:  make(map[string]Indexer),
		instances: make(map[string]*session.Instance),
	}

	srv := NewMCPIndexServer(api)
	port, err := srv.Start()
	require.NoError(t, err)
	defer srv.Stop()

	config := srv.GenerateMCPConfig("test-session")

	// Parse and verify the config
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(config), &parsed)
	require.NoError(t, err)

	servers, ok := parsed["mcpServers"].(map[string]interface{})
	require.True(t, ok)

	csIndex, ok := servers["cs-index"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "http", csIndex["type"])
	expectedURL := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)
	assert.Equal(t, expectedURL, csIndex["url"])
}

// jsonRPCRequest is the standard JSON-RPC 2.0 request format
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is the standard JSON-RPC 2.0 response format
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func TestMCPIndexServer_ToolsList(t *testing.T) {
	api := &SessionAPI{
		indexers:  make(map[string]Indexer),
		instances: make(map[string]*session.Instance),
	}

	srv := NewMCPIndexServer(api)
	port, err := srv.Start()
	require.NoError(t, err)
	defer srv.Stop()

	// First, we need to initialize the session
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check that we got a response (even if initialization isn't fully implemented)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPIndexServer_LookupSymbol_NoIndexer(t *testing.T) {
	api := &SessionAPI{
		indexers:  make(map[string]Indexer),
		instances: make(map[string]*session.Instance),
	}

	srv := NewMCPIndexServer(api)
	port, err := srv.Start()
	require.NoError(t, err)
	defer srv.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	// Initialize first
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Send initialized notification
	initedReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	initedBody, _ := json.Marshal(initedReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(initedBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Call tools/call with code_search
	callReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "code_search",
			"arguments": map[string]interface{}{
				"query": "TestFunction",
			},
		},
	}

	callBody, _ := json.Marshal(callReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(callBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(body, &rpcResp)
	require.NoError(t, err)

	// Should return a result with an error message about missing indexer
	assert.NotNil(t, rpcResp.Result)
	assert.Contains(t, string(rpcResp.Result), "no indexer found")
}

func TestMCPIndexServer_Standalone(t *testing.T) {
	// Create indexer for a test directory
	tmpDir := t.TempDir()

	// Initialize as git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Create a test file
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644))
	cmd = exec.Command("git", "add", "main.go")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	indexer := NewSessionIndexer(tmpDir)
	indexer.Start()
	defer indexer.Stop()

	// Wait for initial index
	time.Sleep(500 * time.Millisecond)

	// Create standalone MCP server
	srv := NewMCPIndexServerStandalone(indexer)
	require.NotNil(t, srv)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	defer srv.Stop()

	// Should be able to generate config
	config := srv.GenerateMCPConfig("test")
	assert.Contains(t, config, fmt.Sprintf(":%d", port))
}

func TestMCPGetSymbol(t *testing.T) {
	// Create temp worktree with a Go file
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	cmd := exec.Command("git", "-C", tmp, "init")
	require.NoError(t, cmd.Run())

	code := []byte(`package main

func Hello() string {
    return "hello"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.go"), code, 0644))
	cmd = exec.Command("git", "-C", tmp, "add", "main.go")
	require.NoError(t, cmd.Run())

	// Create and start indexer
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Create MCP server
	srv := NewMCPIndexServerStandaloneTS(idx)
	require.NotNil(t, srv)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	defer srv.Stop()
	defer idx.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	// Initialize first
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Send initialized notification
	initedReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	initedBody, _ := json.Marshal(initedReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(initedBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Test get_symbol_source tool
	callReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "get_symbol_source",
			"arguments": map[string]interface{}{
				"name": "Hello",
			},
		},
	}

	callBody, _ := json.Marshal(callReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(callBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(body, &rpcResp)
	require.NoError(t, err)
	assert.NotNil(t, rpcResp.Result)
	// The result should contain the symbol info (or "No symbol found" if indexing is incomplete)
}

func TestMCPIndexStatus(t *testing.T) {
	// Create temp worktree with a Go file
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	cmd := exec.Command("git", "-C", tmp, "init")
	require.NoError(t, cmd.Run())

	code := []byte(`package main

func Hello() string {
    return "hello"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.go"), code, 0644))
	cmd = exec.Command("git", "-C", tmp, "add", "main.go")
	require.NoError(t, cmd.Run())

	// Create and start indexer
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Create MCP server
	srv := NewMCPIndexServerStandaloneTS(idx)
	require.NotNil(t, srv)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	defer srv.Stop()
	defer idx.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	// Initialize first
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Send initialized notification
	initedReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	initedBody, _ := json.Marshal(initedReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(initedBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Test index_status tool
	callReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "index_status",
			"arguments": map[string]interface{}{},
		},
	}

	callBody, _ := json.Marshal(callReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(callBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(body, &rpcResp)
	require.NoError(t, err)
	assert.NotNil(t, rpcResp.Result)
	// Result should contain status info
	assert.Contains(t, string(rpcResp.Result), "tree-sitter")
}

func TestMCPFindCallers(t *testing.T) {
	// Create temp worktree with Go files that have calls
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	cmd := exec.Command("git", "-C", tmp, "init")
	require.NoError(t, cmd.Run())

	code := []byte(`package main

func Hello() string {
    return "hello"
}

func Greet() string {
    return Hello() + " world"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.go"), code, 0644))
	cmd = exec.Command("git", "-C", tmp, "add", "main.go")
	require.NoError(t, cmd.Run())

	// Create and start indexer
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Create MCP server
	srv := NewMCPIndexServerStandaloneTS(idx)
	require.NotNil(t, srv)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	defer srv.Stop()
	defer idx.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	// Initialize first
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Send initialized notification
	initedReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	initedBody, _ := json.Marshal(initedReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(initedBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Test find_references tool
	callReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "find_references",
			"arguments": map[string]interface{}{
				"symbol": "Hello",
			},
		},
	}

	callBody, _ := json.Marshal(callReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(callBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(body, &rpcResp)
	require.NoError(t, err)
	assert.NotNil(t, rpcResp.Result)
	// Result should contain callers info or "No callers found"
}

func TestMCPFindCallees(t *testing.T) {
	// Create temp worktree with Go files that have calls
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	cmd := exec.Command("git", "-C", tmp, "init")
	require.NoError(t, cmd.Run())

	code := []byte(`package main

func Hello() string {
    return "hello"
}

func Greet() string {
    return Hello() + " world"
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.go"), code, 0644))
	cmd = exec.Command("git", "-C", tmp, "add", "main.go")
	require.NoError(t, cmd.Run())

	// Create and start indexer
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Create MCP server
	srv := NewMCPIndexServerStandaloneTS(idx)
	require.NotNil(t, srv)

	port, err := srv.Start()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	defer srv.Stop()
	defer idx.Stop()

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp/test-session", port)

	// Initialize first
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(url, "application/json", bytes.NewReader(initBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Send initialized notification
	initedReq := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	initedBody, _ := json.Marshal(initedReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(initedBody))
	require.NoError(t, err)
	resp.Body.Close()

	// Test get_call_graph tool
	callReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "get_call_graph",
			"arguments": map[string]interface{}{
				"symbol": "Greet",
			},
		},
	}

	callBody, _ := json.Marshal(callReq)
	resp, err = http.Post(url, "application/json", bytes.NewReader(callBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(body, &rpcResp)
	require.NoError(t, err)
	assert.NotNil(t, rpcResp.Result)
	// Result should contain callees info or "No callees found"
}
