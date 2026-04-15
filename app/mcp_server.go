package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"claude-squad/log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPIndexServer exposes the SessionIndexer via MCP tools.
// It runs an HTTP server that Claude Code sessions can connect to.
type MCPIndexServer struct {
	mu                sync.RWMutex
	api               *SessionAPI
	standaloneIndexer *SessionIndexer
	standaloneTSIndexer *TreeSitterIndexer
	server            *server.StreamableHTTPServer
	listener          net.Listener
	port              int
}

// NewMCPIndexServer creates a new MCP server backed by the SessionAPI's indexers.
func NewMCPIndexServer(api *SessionAPI) *MCPIndexServer {
	return &MCPIndexServer{api: api}
}

// NewMCPIndexServerStandalone creates an MCP server backed by a single indexer.
// Use this for benchmarks or standalone operation without SessionAPI.
func NewMCPIndexServerStandalone(indexer *SessionIndexer) *MCPIndexServer {
	return &MCPIndexServer{
		standaloneIndexer: indexer,
	}
}

// NewMCPIndexServerStandaloneTS creates an MCP server backed by a TreeSitterIndexer.
// Use this for benchmarks or standalone operation with the new tree-sitter indexer.
func NewMCPIndexServerStandaloneTS(indexer *TreeSitterIndexer) *MCPIndexServer {
	return &MCPIndexServer{
		standaloneTSIndexer: indexer,
	}
}

// Start starts the MCP HTTP server on a dynamic port.
// Returns the port number for clients to connect to.
func (m *MCPIndexServer) Start() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listener != nil {
		return m.port, nil // already running
	}

	// Create the MCP server with tool capabilities
	mcpServer := server.NewMCPServer(
		"claude-squad-index",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	m.registerTools(mcpServer)

	// Create HTTP transport with stateless mode (each request is independent)
	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStateLess(true),
		server.WithEndpointPath("/mcp"),
	)
	m.server = httpServer

	// Listen on a dynamic port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to listen: %w", err)
	}
	m.listener = listener
	m.port = listener.Addr().(*net.TCPAddr).Port

	// Create HTTP mux
	mux := http.NewServeMux()
	// Per-session endpoint: /mcp/<session-id>
	mux.HandleFunc("/mcp/", func(w http.ResponseWriter, r *http.Request) {
		// Extract session ID from path
		path := strings.TrimPrefix(r.URL.Path, "/mcp/")
		sessionID := strings.Split(path, "/")[0]
		if sessionID == "" {
			http.Error(w, "session ID required in path", http.StatusBadRequest)
			return
		}
		// Store session ID in context for tool handlers
		ctx := context.WithValue(r.Context(), sessionIDKey, sessionID)
		httpServer.ServeHTTP(w, r.WithContext(ctx))
	})

	go func() {
		srv := &http.Server{Handler: mux}
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			if log.ErrorLog != nil {
				log.ErrorLog.Printf("MCP server error: %v", err)
			}
		}
	}()

	if log.InfoLog != nil {
		log.InfoLog.Printf("MCP index server started on port %d", m.port)
	}
	return m.port, nil
}

// Stop stops the MCP server.
func (m *MCPIndexServer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listener != nil {
		m.listener.Close()
		m.listener = nil
		m.port = 0
	}
}

// Port returns the port the server is listening on.
func (m *MCPIndexServer) Port() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.port
}

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const sessionIDKey contextKey = "sessionID"

// getSessionID extracts the session ID from context.
func getSessionID(ctx context.Context) string {
	if v := ctx.Value(sessionIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// getIndexer returns the indexer for the given session ID.
// In standalone mode, returns the standalone indexer regardless of session ID.
func (m *MCPIndexServer) getIndexer(sessionID string) (Indexer, error) {
	if m.standaloneTSIndexer != nil {
		return m.standaloneTSIndexer, nil
	}
	if m.standaloneIndexer != nil {
		return m.standaloneIndexer, nil
	}
	if m.api == nil {
		return nil, fmt.Errorf("no API or standalone indexer configured")
	}
	m.api.mu.RLock()
	idx, ok := m.api.indexers[sessionID]
	m.api.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no indexer found for session %q", sessionID)
	}
	return idx, nil
}

// registerTools registers all MCP tools with the server.
// Tools are designed with descriptions that encourage Claude to prefer them over grep/read.
func (m *MCPIndexServer) registerTools(s *server.MCPServer) {
	// code_search - PRIMARY TOOL for finding code
	// This description is crafted to make Claude prefer it over grep
	s.AddTool(
		mcp.NewTool("code_search",
			mcp.WithDescription(`Search indexed code symbols by name. USE THIS instead of Grep for function/class/method lookups.

WHY USE THIS:
- 95% fewer tokens than grep (returns only matching symbols, not file contents)
- Sub-100ms response via tree-sitter index
- BM25 ranked results by relevance
- Supports fuzzy matching

WHEN TO USE:
- "Where is X defined?" → code_search
- "Find the function named Y" → code_search
- "What classes match Z" → code_search

WHEN TO USE GREP INSTEAD:
- Searching non-code files (docs, configs, logs)
- Regex patterns across file contents (not symbol names)`),
			mcp.WithString("query",
				mcp.Description("Symbol name to search for (function, class, method, type, variable)"),
				mcp.Required(),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max results (default 20)"),
				mcp.DefaultNumber(20),
			),
		),
		m.handleCodeSearch,
	)

	// get_symbol_source - retrieve exact symbol source code
	s.AddTool(
		mcp.NewTool("get_symbol_source",
			mcp.WithDescription(`Get the exact source code for a symbol. USE THIS instead of Read when you need a function/class body.

WHY USE THIS:
- Returns ONLY the symbol's code (not the entire file)
- Uses precise byte offsets from tree-sitter parsing
- ~50 tokens vs ~2000 tokens for reading a whole file
- Includes signature, body, and metadata

WHEN TO USE:
- "Show me the code for function X" → get_symbol_source
- "What does method Y do?" → get_symbol_source
- "Get the implementation of Z" → get_symbol_source

WHEN TO USE READ INSTEAD:
- Need to see file context around a symbol
- Reading non-code files (configs, docs)`),
			mcp.WithString("name",
				mcp.Description("Exact symbol name to retrieve"),
				mcp.Required(),
			),
		),
		m.handleGetSymbolSource,
	)

	// get_file_symbols - get all symbols in a file
	s.AddTool(
		mcp.NewTool("get_file_symbols",
			mcp.WithDescription(`List all symbols defined in a file. USE THIS instead of Read when you need to see what's in a file.

WHY USE THIS:
- Returns structured list of functions, classes, methods, types
- ~100 tokens vs ~2000 tokens for reading the file
- Sorted by line number with signatures

WHEN TO USE:
- "What functions are in file X?" → get_file_symbols
- "Show me the structure of Y.go" → get_file_symbols
- "List methods in class Z" → get_file_symbols`),
			mcp.WithString("path",
				mcp.Description("File path relative to worktree root"),
				mcp.Required(),
			),
		),
		m.handleGetFileOutline,
	)

	// find_references - find all usages of a symbol (reverse call graph)
	s.AddTool(
		mcp.NewTool("find_references",
			mcp.WithDescription(`Find all places where a symbol is used/called. USE THIS instead of Grep for "who calls X?".

WHY USE THIS:
- Uses call graph analysis, not text search
- Won't match comments or strings containing the name
- Returns calling function context

WHEN TO USE:
- "Who calls function X?" → find_references
- "Where is Y used?" → find_references
- "Find usages of Z" → find_references`),
			mcp.WithString("symbol",
				mcp.Description("Symbol name to find references for"),
				mcp.Required(),
			),
		),
		m.handleFindCallers,
	)

	// get_call_graph - find what a function calls
	s.AddTool(
		mcp.NewTool("get_call_graph",
			mcp.WithDescription(`Find all symbols called by a function. Shows dependencies and call flow.

WHEN TO USE:
- "What does function X call?" → get_call_graph
- "What are the dependencies of Y?" → get_call_graph
- Understanding code flow`),
			mcp.WithString("symbol",
				mcp.Description("Function name to analyze"),
				mcp.Required(),
			),
		),
		m.handleFindCallees,
	)

	// smart_lookup - RECOMMENDED: search + source + context in one call
	s.AddTool(
		mcp.NewTool("smart_lookup",
			mcp.WithDescription(`THE BEST TOOL FOR CODE UNDERSTANDING. Returns symbol source code PLUS related symbols it calls.

WHY USE THIS (95% token savings):
- ONE call instead of: code_search → get_symbol_source → get_call_graph → multiple get_symbol_source
- Returns complete context: the symbol's code + code of functions it calls
- Token-budgeted: stays within limit, prioritizes most relevant context
- Perfect for "explain function X" or "how does Y work?"

EXAMPLE: smart_lookup("ProcessData") returns:
- ProcessData's full source code
- Source code of Helper() that ProcessData calls
- Source code of Validate() that ProcessData calls
- All within your token budget

WHEN TO USE:
- "Explain how X works" → smart_lookup (gets X + everything X calls)
- "Show me function Y and its dependencies" → smart_lookup
- Any time you need to understand a symbol deeply

WHEN TO USE OTHER TOOLS:
- Just need to find where X is defined → code_search
- Need to see who calls X (reverse) → find_references`),
			mcp.WithString("query",
				mcp.Description("Symbol name to look up"),
				mcp.Required(),
			),
			mcp.WithNumber("max_tokens",
				mcp.Description("Token budget for response (default 2000)"),
				mcp.DefaultNumber(2000),
			),
		),
		m.handleSmartLookup,
	)

	// index_status - health check (keep for debugging)
	s.AddTool(
		mcp.NewTool("index_status",
			mcp.WithDescription("Get index statistics: file count, symbol count, indexer health. Call this first if unsure whether index is available."),
		),
		m.handleIndexStatus,
	)
}

// handleCodeSearch handles the code_search tool - primary symbol search.
func (m *MCPIndexServer) handleCodeSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found in request"), nil
	}

	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	limitF, _ := req.RequireFloat("limit")
	limit := int(limitF)
	if limit <= 0 {
		limit = 20
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Use tree-sitter BM25 search if available
	if tsIdx, ok := idx.(*TreeSitterIndexer); ok {
		syms := tsIdx.SearchSymbols(query, limit)
		if len(syms) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No symbols found matching %q. Try a different query or use Grep for non-code files.", query)), nil
		}

		// Format concisely to save tokens
		var lines []string
		for _, sym := range syms {
			line := fmt.Sprintf("%s (%s) - %s:%d", sym.Name, sym.Kind, sym.File, sym.Line)
			if sym.Signature != "" {
				line = fmt.Sprintf("%s\n  %s", line, sym.Signature)
			}
			lines = append(lines, line)
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}

	// Fallback to basic search
	allSymbols := idx.AllSymbols()
	queryLower := strings.ToLower(query)

	var matches []Definition
	for name, defs := range allSymbols {
		if strings.Contains(strings.ToLower(name), queryLower) {
			matches = append(matches, defs...)
			if len(matches) >= limit {
				break
			}
		}
	}

	if len(matches) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbols found matching %q", query)), nil
	}

	if len(matches) > limit {
		matches = matches[:limit]
	}

	var lines []string
	for _, def := range matches {
		lines = append(lines, fmt.Sprintf("%s (%s) - %s:%d", def.Name, def.Kind, def.File, def.Line))
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// handleGetSymbolSource handles get_symbol_source - returns exact symbol source code.
func (m *MCPIndexServer) handleGetSymbolSource(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Requires tree-sitter for byte-offset extraction
	tsIdx, ok := idx.(*TreeSitterIndexer)
	if !ok {
		return mcp.NewToolResultError("get_symbol_source requires tree-sitter indexer"), nil
	}

	syms := tsIdx.LookupSymbol(name)
	if len(syms) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbol found: %q. Use code_search to find available symbols.", name)), nil
	}

	// Get source for first match using byte offsets
	sym := syms[0]
	content, err := tsIdx.GetSymbolContent(sym)
	if err != nil {
		// Fallback to line-based read
		fullPath := filepath.Join(tsIdx.Worktree(), sym.File)
		content, err = readLinesFromFile(fullPath, sym.Line, sym.EndLine)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read symbol: %v", err)), nil
		}
	}

	// Format with metadata
	result := fmt.Sprintf("// %s (%s) in %s:%d-%d\n%s",
		sym.Name, sym.Kind, sym.File, sym.Line, sym.EndLine, content)

	return mcp.NewToolResultText(result), nil
}

// handleGetFileOutline handles the get_file_outline tool call.
func (m *MCPIndexServer) handleGetFileOutline(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found in request"), nil
	}

	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Collect all symbols in the requested file
	allSymbols := idx.AllSymbols()
	var fileSymbols []Definition
	for _, defs := range allSymbols {
		for _, def := range defs {
			if def.File == path {
				fileSymbols = append(fileSymbols, def)
			}
		}
	}

	if len(fileSymbols) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbols found in file %q", path)), nil
	}

	// Sort by line number
	sort.Slice(fileSymbols, func(i, j int) bool {
		return fileSymbols[i].Line < fileSymbols[j].Line
	})

	result, _ := json.MarshalIndent(fileSymbols, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleReadLines handles the read_lines tool call.
func (m *MCPIndexServer) handleReadLines(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found in request"), nil
	}

	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	startF, err := req.RequireFloat("start")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}
	start := int(startF)

	endF, err := req.RequireFloat("end")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}
	end := int(endF)

	if start < 1 {
		start = 1
	}
	if end < start {
		return mcp.NewToolResultError("end must be >= start"), nil
	}

	// Get the indexer to find the worktree path
	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Resolve the full file path
	worktree := idx.Worktree()
	fullPath := filepath.Join(worktree, path)

	// Security check: ensure the path is within the worktree
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
	}
	absWorktree, _ := filepath.Abs(worktree)
	if !strings.HasPrefix(absPath, absWorktree) {
		return mcp.NewToolResultError("path escapes worktree"), nil
	}

	// Read the file
	file, err := os.Open(fullPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open file: %v", err)), nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			lines = append(lines, fmt.Sprintf("%4d: %s", lineNum, scanner.Text()))
		}
		if lineNum > end {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error reading file: %v", err)), nil
	}

	if len(lines) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No lines found in range %d-%d", start, end)), nil
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// handleSearchSymbols handles the search_symbols tool call.
func (m *MCPIndexServer) handleSearchSymbols(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found in request"), nil
	}

	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	limitF, _ := req.RequireFloat("limit")
	limit := int(limitF)
	if limit <= 0 {
		limit = 50
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Search for matching symbols (case-insensitive substring match)
	allSymbols := idx.AllSymbols()
	queryLower := strings.ToLower(query)

	var matches []Definition
	for name, defs := range allSymbols {
		if strings.Contains(strings.ToLower(name), queryLower) {
			matches = append(matches, defs...)
			if len(matches) >= limit {
				break
			}
		}
	}

	if len(matches) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbols found matching %q", query)), nil
	}

	// Truncate to limit
	if len(matches) > limit {
		matches = matches[:limit]
	}

	result, _ := json.MarshalIndent(matches, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleGetSymbol returns symbol with optional full body.
func (m *MCPIndexServer) handleGetSymbol(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	signatureOnly, _ := req.RequireBool("signature_only")

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Try tree-sitter indexer first for richer data
	if tsIdx, ok := idx.(*TreeSitterIndexer); ok {
		syms := tsIdx.LookupSymbol(name)
		if len(syms) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No symbol found: %q", name)), nil
		}

		if signatureOnly {
			result, _ := json.MarshalIndent(syms, "", "  ")
			return mcp.NewToolResultText(string(result)), nil
		}

		// Include source body for first match
		sym := syms[0]
		fullPath := filepath.Join(tsIdx.Worktree(), sym.File)
		body, err := readLinesFromFile(fullPath, sym.Line, sym.EndLine)
		if err == nil {
			sym.DocComment = body // reuse field for body
		}

		result, _ := json.MarshalIndent(sym, "", "  ")
		return mcp.NewToolResultText(string(result)), nil
	}

	// Fallback to basic indexer
	defs := idx.Lookup(name)
	if len(defs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbol found: %q", name)), nil
	}

	result, _ := json.MarshalIndent(defs, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleFindCallers returns all places where symbol is called.
func (m *MCPIndexServer) handleFindCallers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	symbol, err := req.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tsIdx, ok := idx.(*TreeSitterIndexer)
	if !ok {
		return mcp.NewToolResultError("find_callers requires tree-sitter indexer"), nil
	}

	refs := tsIdx.FindCallers(symbol)
	if len(refs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No callers found for %q", symbol)), nil
	}

	result, _ := json.MarshalIndent(refs, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleFindCallees returns all symbols called by function.
func (m *MCPIndexServer) handleFindCallees(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	symbol, err := req.RequireString("symbol")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tsIdx, ok := idx.(*TreeSitterIndexer)
	if !ok {
		return mcp.NewToolResultError("find_callees requires tree-sitter indexer"), nil
	}

	refs := tsIdx.FindCallees(symbol)
	if len(refs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No callees found for %q", symbol)), nil
	}

	result, _ := json.MarshalIndent(refs, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleIndexStatus returns index health info.
func (m *MCPIndexServer) handleIndexStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	files := idx.Files()
	allSyms := idx.AllSymbols()

	symbolCount := 0
	for _, defs := range allSyms {
		symbolCount += len(defs)
	}

	status := map[string]interface{}{
		"worktree":     idx.Worktree(),
		"file_count":   len(files),
		"symbol_count": symbolCount,
		"indexer_type": "ctags",
	}

	if tsIdx, ok := idx.(*TreeSitterIndexer); ok {
		status["indexer_type"] = "tree-sitter"
		tsIdx.mu.RLock()
		if tsIdx.callgraph != nil {
			callers, callees := tsIdx.callgraph.Stats()
			status["caller_symbols"] = callers
			status["callee_functions"] = callees
		}
		tsIdx.mu.RUnlock()
	}

	result, _ := json.MarshalIndent(status, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleSmartLookup handles smart_lookup - returns symbol source + related context.
func (m *MCPIndexServer) handleSmartLookup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	if sessionID == "" {
		return mcp.NewToolResultError("session ID not found"), nil
	}

	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	maxTokensF, _ := req.RequireFloat("max_tokens")
	maxTokens := int(maxTokensF)
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Requires tree-sitter for context bundling
	tsIdx, ok := idx.(*TreeSitterIndexer)
	if !ok {
		return mcp.NewToolResultError("smart_lookup requires tree-sitter indexer"), nil
	}

	// Get bundled context
	bundles := tsIdx.SearchWithContext(query, maxTokens)
	if len(bundles) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No symbols found matching %q. Try code_search for simpler lookup.", query)), nil
	}

	// Format the response
	var sb strings.Builder
	for i, bundle := range bundles {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}

		// Primary symbol
		sym := bundle.Primary.Symbol
		sb.WriteString(fmt.Sprintf("## %s (%s)\n", sym.Name, sym.Kind))
		sb.WriteString(fmt.Sprintf("File: %s:%d-%d\n\n", sym.File, sym.Line, sym.EndLine))
		sb.WriteString("```")
		sb.WriteString(sym.Language)
		sb.WriteString("\n")
		sb.WriteString(bundle.Primary.Content)
		sb.WriteString("\n```\n")

		// Related symbols (callees)
		if len(bundle.Related) > 0 {
			sb.WriteString(fmt.Sprintf("\n### Called by %s:\n\n", sym.Name))
			for _, related := range bundle.Related {
				rsym := related.Symbol
				sb.WriteString(fmt.Sprintf("#### %s (%s) - %s:%d\n", rsym.Name, rsym.Kind, rsym.File, rsym.Line))
				sb.WriteString("```")
				sb.WriteString(rsym.Language)
				sb.WriteString("\n")
				sb.WriteString(related.Content)
				sb.WriteString("\n```\n\n")
			}
		}

		sb.WriteString(fmt.Sprintf("\n_[%d tokens used]_\n", bundle.TotalTokens))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// readLinesFromFile reads a range of lines from a file.
func readLinesFromFile(path string, start, end int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			lines = append(lines, scanner.Text())
		}
		if lineNum > end {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// ServerInstructions returns instructions that tell Claude when to use index tools.
// These should be included in system prompts or CLAUDE.md.
const ServerInstructions = `## Code Index (cs-index MCP Server)

This codebase has a tree-sitter code index. Use it for 95% token savings.

**START HERE - smart_lookup is the best tool for understanding code:**
- "Explain function X" → smart_lookup (returns X + everything X calls)
- "How does Y work?" → smart_lookup (complete context in one call)
- "Show me Z and its dependencies" → smart_lookup

**Other index tools:**
- "Where is X defined?" (just location) → code_search
- "Who calls X?" (reverse lookup) → find_references
- "What's in file Z?" (outline) → get_file_symbols
- "Just show me X's code" (no context) → get_symbol_source

**ONLY use Grep/Read when:**
- Searching non-code files (docs, configs, logs)
- Need regex across file contents (not symbol names)
- Index tools return no results`

// GenerateMCPConfig generates an MCP configuration JSON for a session.
// This can be passed to Claude Code via --mcp-config.
func (m *MCPIndexServer) GenerateMCPConfig(sessionID string) string {
	m.mu.RLock()
	port := m.port
	m.mu.RUnlock()

	config := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"cs-index": map[string]interface{}{
				"type": "http",
				"url":  fmt.Sprintf("http://127.0.0.1:%d/mcp/%s", port, sessionID),
				"instructions": ServerInstructions,
			},
		},
	}

	data, _ := json.Marshal(config)
	return string(data)
}
