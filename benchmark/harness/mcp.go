package harness

import (
	"fmt"
	"time"

	"claude-squad/app"
)

// MCPServer wraps the MCP index server for benchmark use.
type MCPServer struct {
	indexer *app.SessionIndexer
	server  *app.MCPIndexServer
	port    int
}

// StartMCPServer starts an MCP server for the given workdir.
func StartMCPServer(workdir string) (*MCPServer, error) {
	indexer := app.NewSessionIndexer(workdir)
	indexer.Start()

	server := app.NewMCPIndexServerStandalone(indexer)
	port, err := server.Start()
	if err != nil {
		indexer.Stop()
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	return &MCPServer{
		indexer: indexer,
		server:  server,
		port:    port,
	}, nil
}

// Stop stops the MCP server and indexer.
func (m *MCPServer) Stop() {
	if m.server != nil {
		m.server.Stop()
	}
	if m.indexer != nil {
		m.indexer.Stop()
	}
}

// Config returns the MCP configuration JSON for Claude.
func (m *MCPServer) Config() string {
	return m.server.GenerateMCPConfig("benchmark")
}

// Port returns the server port.
func (m *MCPServer) Port() int {
	return m.port
}

// WaitForIndex waits until the indexer has symbols available.
// Returns the number of symbol names indexed, or 0 if timeout.
func (m *MCPServer) WaitForIndex(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		symbols := m.indexer.AllSymbols()
		if len(symbols) > 0 {
			return len(symbols)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return 0
}

// SymbolCount returns the current number of indexed symbol names.
func (m *MCPServer) SymbolCount() int {
	return len(m.indexer.AllSymbols())
}
