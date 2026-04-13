package harness

import (
	"fmt"

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
