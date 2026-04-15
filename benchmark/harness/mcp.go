package harness

import (
	"fmt"
	"time"

	"claude-squad/app"
)

// IndexerType specifies which indexer backend to use.
type IndexerType string

const (
	IndexerCtags      IndexerType = "ctags"
	IndexerTreeSitter IndexerType = "treesitter"
)

// MCPServer wraps the MCP index server for benchmark use.
type MCPServer struct {
	indexer     app.Indexer
	tsIndexer   *app.TreeSitterIndexer // for tree-sitter specific methods
	server      *app.MCPIndexServer
	port        int
	indexerType IndexerType
}

// StartMCPServer starts an MCP server for the given workdir using ctags.
func StartMCPServer(workdir string) (*MCPServer, error) {
	return StartMCPServerWithType(workdir, IndexerCtags)
}

// StartMCPServerWithType starts an MCP server with the specified indexer type.
func StartMCPServerWithType(workdir string, indexerType IndexerType) (*MCPServer, error) {
	var indexer app.Indexer
	var tsIndexer *app.TreeSitterIndexer
	var server *app.MCPIndexServer

	switch indexerType {
	case IndexerTreeSitter:
		tsIndexer = app.NewTreeSitterIndexer(workdir)
		tsIndexer.Start()
		indexer = tsIndexer
		server = app.NewMCPIndexServerStandaloneTS(tsIndexer)
	default:
		ctagsIndexer := app.NewSessionIndexer(workdir)
		ctagsIndexer.Start()
		indexer = ctagsIndexer
		server = app.NewMCPIndexServerStandalone(ctagsIndexer)
	}

	port, err := server.Start()
	if err != nil {
		indexer.Stop()
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	return &MCPServer{
		indexer:     indexer,
		tsIndexer:   tsIndexer,
		server:      server,
		port:        port,
		indexerType: indexerType,
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

// Type returns the indexer type being used.
func (m *MCPServer) Type() IndexerType {
	return m.indexerType
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
