# MCP Benchmark System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a benchmark harness to measure MCP index server effectiveness by running Claude Code with/without MCP and generating comparative reports.

**Architecture:** Subprocess-based harness spawns `claude --print` with predefined prompts, parses JSON output for metrics, compares MCP vs no-MCP runs, generates terminal/JSON reports.

**Tech Stack:** Go, cobra (CLI), existing MCP server code, Claude Code CLI

---

## File Structure

### New Files
| Path | Responsibility |
|------|----------------|
| `benchmark/cmd/cs-benchmark/main.go` | CLI entry point, flag parsing |
| `benchmark/harness/runner.go` | Spawn Claude subprocess, capture output |
| `benchmark/harness/metrics.go` | Parse JSON output, extract token/tool metrics |
| `benchmark/harness/report.go` | Generate terminal tables and JSON reports |
| `benchmark/harness/mcp.go` | Standalone MCP server for benchmarks |
| `benchmark/tasks/task.go` | Task interface and registry |
| `benchmark/tasks/symbol.go` | Symbol lookup tasks |
| `benchmark/tasks/understanding.go` | Code understanding tasks |
| `benchmark/tasks/edit.go` | Small edit tasks |
| `benchmark/tasks/crossfile.go` | Cross-file tasks |
| `benchmark/harness/runner_test.go` | Runner tests |
| `benchmark/harness/metrics_test.go` | Metrics parsing tests |

### Modified Files
| Path | Change |
|------|--------|
| `app/bindings.go:30-39` | Add MCPEnabled field to CreateOptions |
| `app/bindings.go:200-215` | Update setMCPConfig to check MCPEnabled |
| `app/mcp_server.go` | Add standalone constructor |

---

## Task 1: Add MCPEnabled to CreateOptions

**Files:**
- Modify: `app/bindings.go:30-39`
- Modify: `app/bindings.go:200-215`
- Test: `app/bindings_test.go`

- [ ] **Step 1: Write failing test for MCPEnabled toggle**

Add to `app/bindings_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./app -run TestSetMCPConfig_Disabled -v
```

Expected: FAIL - `setMCPConfigWithOpts` not defined

- [ ] **Step 3: Add MCPEnabled field to CreateOptions**

In `app/bindings.go`, update CreateOptions:

```go
type CreateOptions struct {
	Title      string `json:"title"`
	Path       string `json:"path"`
	Program    string `json:"program"`
	Branch     string `json:"branch"`
	AutoYes    bool   `json:"autoYes"`
	InPlace    bool   `json:"inPlace"`
	Prompt     string `json:"prompt"`
	HostID     string `json:"hostId"`
	MCPEnabled *bool  `json:"mcpEnabled,omitempty"`
}
```

- [ ] **Step 4: Add setMCPConfigWithOpts helper**

In `app/bindings.go`, add new method and update existing one:

```go
// setMCPConfigWithOpts sets the MCP configuration on an instance,
// respecting the MCPEnabled option.
func (api *SessionAPI) setMCPConfigWithOpts(inst *session.Instance, opts CreateOptions) {
	if api.mcpServer == nil {
		return
	}
	// Check if MCP is explicitly disabled
	if opts.MCPEnabled != nil && !*opts.MCPEnabled {
		return
	}
	// Only set MCP config for Claude Code sessions
	if !strings.HasSuffix(inst.Program, session.ProgramClaude) {
		return
	}
	inst.MCPConfig = api.mcpServer.GenerateMCPConfig(inst.Title)
}

// setMCPConfig is the existing helper (for backward compatibility)
func (api *SessionAPI) setMCPConfig(inst *session.Instance) {
	api.setMCPConfigWithOpts(inst, CreateOptions{})
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./app -run TestSetMCPConfig_Disabled -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add app/bindings.go app/bindings_test.go
git commit -m "feat: add MCPEnabled toggle to CreateOptions

Allows per-session control of MCP index server injection.
- nil (default): MCP enabled
- true: explicitly enabled
- false: disabled for this session"
```

---

## Task 2: Add Standalone MCP Server Constructor

**Files:**
- Modify: `app/mcp_server.go`
- Test: `app/mcp_server_test.go`

- [ ] **Step 1: Write failing test for standalone MCP server**

Add to `app/mcp_server_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./app -run TestMCPIndexServer_Standalone -v
```

Expected: FAIL - `NewMCPIndexServerStandalone` not defined

- [ ] **Step 3: Add standalone constructor**

In `app/mcp_server.go`, add:

```go
// NewMCPIndexServerStandalone creates an MCP server backed by a single indexer.
// Use this for benchmarks or standalone operation without SessionAPI.
func NewMCPIndexServerStandalone(indexer *SessionIndexer) *MCPIndexServer {
	return &MCPIndexServer{
		standaloneIndexer: indexer,
	}
}
```

Update the struct to include the standalone field:

```go
type MCPIndexServer struct {
	mu                sync.RWMutex
	api               *SessionAPI
	standaloneIndexer *SessionIndexer  // For standalone mode
	server            *server.StreamableHTTPServer
	listener          net.Listener
	port              int
}
```

- [ ] **Step 4: Update tool handlers to check standalone indexer**

In `app/mcp_server.go`, update `handleLookupSymbol` and other handlers to check standalone mode:

```go
// getIndexer returns the indexer for the given session ID.
// In standalone mode, returns the standalone indexer regardless of session ID.
func (m *MCPIndexServer) getIndexer(sessionID string) (*SessionIndexer, error) {
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
```

Update `handleLookupSymbol`:

```go
func (m *MCPIndexServer) handleLookupSymbol(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := getSessionID(ctx)
	
	idx, err := m.getIndexer(sessionID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid argument: %v", err)), nil
	}

	defs := idx.Lookup(name)
	if len(defs) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No definitions found for symbol %q", name)), nil
	}

	result, _ := json.MarshalIndent(defs, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}
```

Apply similar changes to `handleListFiles`, `handleGetFileOutline`, `handleReadLines`, `handleSearchSymbols`.

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./app -run TestMCPIndexServer_Standalone -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add app/mcp_server.go app/mcp_server_test.go
git commit -m "feat: add standalone MCP server constructor

NewMCPIndexServerStandalone allows running MCP server with a
single indexer, without requiring full SessionAPI. Enables
benchmark harness to run independently of claude-squad app."
```

---

## Task 3: Create Benchmark Task Interface

**Files:**
- Create: `benchmark/tasks/task.go`

- [ ] **Step 1: Create benchmark directory structure**

```bash
mkdir -p benchmark/cmd/cs-benchmark benchmark/harness benchmark/tasks
```

- [ ] **Step 2: Create task interface and registry**

Create `benchmark/tasks/task.go`:

```go
package tasks

// Task defines a benchmark task that can be run with or without MCP.
type Task interface {
	// Name returns a unique identifier for the task (e.g., "symbol-find-session")
	Name() string
	// Category returns the task category (symbol, understanding, edit, crossfile)
	Category() string
	// Prompt returns the prompt to send to Claude
	Prompt() string
	// Validate checks if the output is correct (optional, return nil to skip)
	Validate(output string) error
}

// Registry holds all registered tasks.
var Registry = make(map[string]Task)

// Register adds a task to the registry.
func Register(t Task) {
	Registry[t.Name()] = t
}

// GetByCategory returns all tasks in a category.
func GetByCategory(category string) []Task {
	var tasks []Task
	for _, t := range Registry {
		if t.Category() == category {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// GetAll returns all registered tasks.
func GetAll() []Task {
	tasks := make([]Task, 0, len(Registry))
	for _, t := range Registry {
		tasks = append(tasks, t)
	}
	return tasks
}

// Categories returns the list of valid category names.
func Categories() []string {
	return []string{"symbol", "understanding", "edit", "crossfile"}
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./benchmark/tasks/...
```

Expected: Success

- [ ] **Step 4: Commit**

```bash
git add benchmark/tasks/task.go
git commit -m "feat(benchmark): add task interface and registry"
```

---

## Task 4: Create Symbol Lookup Tasks

**Files:**
- Create: `benchmark/tasks/symbol.go`

- [ ] **Step 1: Create symbol lookup tasks**

Create `benchmark/tasks/symbol.go`:

```go
package tasks

import "strings"

func init() {
	Register(&SymbolFindSession{})
	Register(&SymbolFindType{})
	Register(&SymbolFindUsages{})
}

// SymbolFindSession finds the Session model definition.
type SymbolFindSession struct{}

func (t *SymbolFindSession) Name() string     { return "symbol-find-session" }
func (t *SymbolFindSession) Category() string { return "symbol" }
func (t *SymbolFindSession) Prompt() string {
	return "Where is the Session struct defined in verve-backend? Show me the file path and the struct definition."
}
func (t *SymbolFindSession) Validate(output string) error {
	if !strings.Contains(output, "models/session.go") {
		return nil // Validation is optional, don't fail
	}
	return nil
}

// SymbolFindType finds type definitions and usages.
type SymbolFindType struct{}

func (t *SymbolFindType) Name() string     { return "symbol-find-type" }
func (t *SymbolFindType) Category() string { return "symbol" }
func (t *SymbolFindType) Prompt() string {
	return "Find the DiffStats type in verve-backend. Where is it defined and what functions use it?"
}
func (t *SymbolFindType) Validate(output string) error { return nil }

// SymbolFindUsages finds all usages of a function.
type SymbolFindUsages struct{}

func (t *SymbolFindUsages) Name() string     { return "symbol-find-usages" }
func (t *SymbolFindUsages) Category() string { return "symbol" }
func (t *SymbolFindUsages) Prompt() string {
	return "Find all places where Validate() is called on a Session model in verve-backend."
}
func (t *SymbolFindUsages) Validate(output string) error { return nil }
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/tasks/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/tasks/symbol.go
git commit -m "feat(benchmark): add symbol lookup tasks"
```

---

## Task 5: Create Understanding Tasks

**Files:**
- Create: `benchmark/tasks/understanding.go`

- [ ] **Step 1: Create understanding tasks**

Create `benchmark/tasks/understanding.go`:

```go
package tasks

func init() {
	Register(&UnderstandSessionLifecycle{})
	Register(&UnderstandAPIEndpoint{})
	Register(&UnderstandGateway{})
}

// UnderstandSessionLifecycle asks Claude to explain session flow.
type UnderstandSessionLifecycle struct{}

func (t *UnderstandSessionLifecycle) Name() string     { return "understand-session-lifecycle" }
func (t *UnderstandSessionLifecycle) Category() string { return "understanding" }
func (t *UnderstandSessionLifecycle) Prompt() string {
	return "Explain how a session is created, processed, and stored in verve-backend. Trace the code path from the API handler through to the database."
}
func (t *UnderstandSessionLifecycle) Validate(output string) error { return nil }

// UnderstandAPIEndpoint asks Claude to trace an API endpoint.
type UnderstandAPIEndpoint struct{}

func (t *UnderstandAPIEndpoint) Name() string     { return "understand-api-endpoint" }
func (t *UnderstandAPIEndpoint) Category() string { return "understanding" }
func (t *UnderstandAPIEndpoint) Prompt() string {
	return "What does the gateway_api/session_create endpoint do? Trace the request flow through the handler."
}
func (t *UnderstandAPIEndpoint) Validate(output string) error { return nil }

// UnderstandGateway asks Claude to explain gateway communication.
type UnderstandGateway struct{}

func (t *UnderstandGateway) Name() string     { return "understand-gateway" }
func (t *UnderstandGateway) Category() string { return "understanding" }
func (t *UnderstandGateway) Prompt() string {
	return "How does verve-gateway communicate with verve-backend? What protocol and data format are used?"
}
func (t *UnderstandGateway) Validate(output string) error { return nil }
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/tasks/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/tasks/understanding.go
git commit -m "feat(benchmark): add code understanding tasks"
```

---

## Task 6: Create Edit Tasks

**Files:**
- Create: `benchmark/tasks/edit.go`

- [ ] **Step 1: Create edit tasks**

Create `benchmark/tasks/edit.go`:

```go
package tasks

func init() {
	Register(&EditAddLog{})
	Register(&EditAddComment{})
}

// EditAddLog asks Claude to add a log statement.
type EditAddLog struct{}

func (t *EditAddLog) Name() string     { return "edit-add-log" }
func (t *EditAddLog) Category() string { return "edit" }
func (t *EditAddLog) Prompt() string {
	return "Add a debug log statement at the start of the Validate function on the Session model in verve-backend/models/session.go. The log should print 'Validating session' followed by the session ID."
}
func (t *EditAddLog) Validate(output string) error { return nil }

// EditAddComment asks Claude to add a comment.
type EditAddComment struct{}

func (t *EditAddComment) Name() string     { return "edit-add-comment" }
func (t *EditAddComment) Category() string { return "edit" }
func (t *EditAddComment) Prompt() string {
	return "Add a comment explaining the purpose of the TableName function on the Session model in verve-backend/models/session.go."
}
func (t *EditAddComment) Validate(output string) error { return nil }
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/tasks/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/tasks/edit.go
git commit -m "feat(benchmark): add small edit tasks"
```

---

## Task 7: Create Cross-File Tasks

**Files:**
- Create: `benchmark/tasks/crossfile.go`

- [ ] **Step 1: Create cross-file tasks**

Create `benchmark/tasks/crossfile.go`:

```go
package tasks

func init() {
	Register(&CrossfileNewEndpoint{})
	Register(&CrossfileRefactor{})
}

// CrossfileNewEndpoint asks Claude to add a new endpoint.
type CrossfileNewEndpoint struct{}

func (t *CrossfileNewEndpoint) Name() string     { return "crossfile-new-endpoint" }
func (t *CrossfileNewEndpoint) Category() string { return "crossfile" }
func (t *CrossfileNewEndpoint) Prompt() string {
	return "Add a new health check endpoint to verve-backend following the pattern of existing gateway_api endpoints. The endpoint should be at /health and return a simple JSON response with status: ok."
}
func (t *CrossfileNewEndpoint) Validate(output string) error { return nil }

// CrossfileRefactor asks Claude to make code consistent across files.
type CrossfileRefactor struct{}

func (t *CrossfileRefactor) Name() string     { return "crossfile-refactor" }
func (t *CrossfileRefactor) Category() string { return "crossfile" }
func (t *CrossfileRefactor) Prompt() string {
	return "Look at how error handling is done in verve-backend/pkg/api. Find an inconsistency in how errors are returned and make it consistent with the predominant pattern."
}
func (t *CrossfileRefactor) Validate(output string) error { return nil }
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/tasks/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/tasks/crossfile.go
git commit -m "feat(benchmark): add cross-file tasks"
```

---

## Task 8: Create Metrics Types and Parser

**Files:**
- Create: `benchmark/harness/metrics.go`
- Create: `benchmark/harness/metrics_test.go`

- [ ] **Step 1: Write failing test for metrics parsing**

Create `benchmark/harness/metrics_test.go`:

```go
package harness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetrics(t *testing.T) {
	// Sample stream-json output from Claude
	output := `{"type":"system","subtype":"init","apiKeySource":"keychain"}
{"type":"assistant","message":{"id":"msg_01","role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Read","input":{"file_path":"/test.go"}}],"usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"assistant","message":{"id":"msg_02","role":"assistant","content":[{"type":"tool_use","id":"tu_2","name":"Grep","input":{"pattern":"func"}}],"usage":{"input_tokens":150,"output_tokens":75}}}
{"type":"result","subtype":"success","cost_usd":0.01,"is_error":false,"duration_ms":5000,"duration_api_ms":4500,"input_tokens":250,"output_tokens":125}`

	metrics, err := ParseMetrics(output)
	require.NoError(t, err)

	assert.Equal(t, 250, metrics.InputTokens)
	assert.Equal(t, 125, metrics.OutputTokens)
	assert.Equal(t, 2, len(metrics.ToolCalls))
	assert.Equal(t, "Read", metrics.ToolCalls[0].Name)
	assert.Equal(t, "Grep", metrics.ToolCalls[1].Name)
	assert.Equal(t, 1, metrics.ReadCount)
	assert.Equal(t, 1, metrics.GrepCount)
	assert.True(t, metrics.Success)
}

func TestParseMetrics_Error(t *testing.T) {
	output := `{"type":"result","subtype":"error","is_error":true,"error":"API error"}`

	metrics, err := ParseMetrics(output)
	require.NoError(t, err)
	assert.False(t, metrics.Success)
	assert.Equal(t, "API error", metrics.Error)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./benchmark/harness -run TestParseMetrics -v
```

Expected: FAIL - package doesn't exist

- [ ] **Step 3: Create metrics types and parser**

Create `benchmark/harness/metrics.go`:

```go
package harness

import (
	"bufio"
	"encoding/json"
	"strings"
	"time"
)

// TaskMetrics holds the metrics collected from a single task run.
type TaskMetrics struct {
	TaskName    string
	MCPEnabled  bool

	// Token usage
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CacheReads   int
	CacheWrites  int

	// Tool usage
	ToolCalls    []ToolCall
	ReadCount    int
	GrepCount    int
	EditCount    int
	MCPToolCount int

	// Timing
	WallTime time.Duration

	// Outcome
	Success bool
	Error   string
}

// ToolCall represents a single tool invocation.
type ToolCall struct {
	Name         string
	InputTokens  int
	OutputTokens int
}

// streamMessage represents a line from Claude's stream-json output.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Message *struct {
		Content []struct {
			Type  string `json:"type"`
			Name  string `json:"name,omitempty"`
		} `json:"content,omitempty"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`
	
	// Result fields
	IsError      bool   `json:"is_error,omitempty"`
	ErrorMsg     string `json:"error,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	DurationMs   int    `json:"duration_ms,omitempty"`
}

// ParseMetrics parses Claude's stream-json output into TaskMetrics.
func ParseMetrics(output string) (*TaskMetrics, error) {
	metrics := &TaskMetrics{
		Success: true,
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip unparseable lines
		}

		switch msg.Type {
		case "assistant":
			if msg.Message != nil {
				// Extract tool calls
				for _, content := range msg.Message.Content {
					if content.Type == "tool_use" {
						tc := ToolCall{Name: content.Name}
						metrics.ToolCalls = append(metrics.ToolCalls, tc)

						// Count by tool type
						switch content.Name {
						case "Read":
							metrics.ReadCount++
						case "Grep":
							metrics.GrepCount++
						case "Edit", "Write":
							metrics.EditCount++
						case "lookup_symbol", "list_files", "get_file_outline", "read_lines", "search_symbols":
							metrics.MCPToolCount++
						}
					}
				}
			}

		case "result":
			metrics.InputTokens = msg.InputTokens
			metrics.OutputTokens = msg.OutputTokens
			metrics.TotalTokens = msg.InputTokens + msg.OutputTokens
			metrics.WallTime = time.Duration(msg.DurationMs) * time.Millisecond

			if msg.IsError {
				metrics.Success = false
				metrics.Error = msg.ErrorMsg
			}
		}
	}

	return metrics, scanner.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./benchmark/harness -run TestParseMetrics -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add benchmark/harness/metrics.go benchmark/harness/metrics_test.go
git commit -m "feat(benchmark): add metrics types and parser"
```

---

## Task 9: Create Task Runner

**Files:**
- Create: `benchmark/harness/runner.go`
- Create: `benchmark/harness/runner_test.go`

- [ ] **Step 1: Create runner implementation**

Create `benchmark/harness/runner.go`:

```go
package harness

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"claude-squad/benchmark/tasks"
)

// RunConfig holds configuration for a benchmark run.
type RunConfig struct {
	Workdir    string
	MCPConfig  string // Empty = no MCP
	Timeout    time.Duration
	Verbose    bool
}

// RunResult holds the result of running a single task.
type RunResult struct {
	Task       tasks.Task
	MCPEnabled bool
	Metrics    *TaskMetrics
	Output     string
	Error      error
}

// RunTask runs a single task with the given configuration.
func RunTask(ctx context.Context, task tasks.Task, cfg RunConfig) *RunResult {
	result := &RunResult{
		Task:       task,
		MCPEnabled: cfg.MCPConfig != "",
	}

	// Build command arguments
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"-p", task.Prompt(),
	}

	if cfg.MCPConfig != "" {
		args = append(args, "--mcp-config", cfg.MCPConfig)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = cfg.Workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	wallTime := time.Since(startTime)

	result.Output = stdout.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Errorf("timeout after %v", cfg.Timeout)
		} else {
			result.Error = fmt.Errorf("claude exited with error: %w\nstderr: %s", err, stderr.String())
		}
		result.Metrics = &TaskMetrics{
			TaskName:   task.Name(),
			MCPEnabled: cfg.MCPConfig != "",
			WallTime:   wallTime,
			Success:    false,
			Error:      result.Error.Error(),
		}
		return result
	}

	// Parse metrics from output
	metrics, err := ParseMetrics(result.Output)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse metrics: %w", err)
		result.Metrics = &TaskMetrics{
			TaskName:   task.Name(),
			MCPEnabled: cfg.MCPConfig != "",
			WallTime:   wallTime,
			Success:    false,
			Error:      result.Error.Error(),
		}
		return result
	}

	metrics.TaskName = task.Name()
	metrics.MCPEnabled = cfg.MCPConfig != ""
	metrics.WallTime = wallTime
	result.Metrics = metrics

	// Run validation if provided
	if err := task.Validate(result.Output); err != nil {
		result.Metrics.Success = false
		result.Metrics.Error = fmt.Sprintf("validation failed: %v", err)
	}

	return result
}

// Runner manages running multiple tasks.
type Runner struct {
	Config  RunConfig
	Results []*RunResult
}

// NewRunner creates a new benchmark runner.
func NewRunner(cfg RunConfig) *Runner {
	return &Runner{Config: cfg}
}

// Run executes all provided tasks with and without MCP.
func (r *Runner) Run(ctx context.Context, taskList []tasks.Task, mcpConfig string, mcpOnly, noMCPOnly bool) {
	for _, task := range taskList {
		// Run without MCP
		if !mcpOnly {
			cfg := r.Config
			cfg.MCPConfig = ""
			result := RunTask(ctx, task, cfg)
			r.Results = append(r.Results, result)
		}

		// Run with MCP
		if !noMCPOnly {
			cfg := r.Config
			cfg.MCPConfig = mcpConfig
			result := RunTask(ctx, task, cfg)
			r.Results = append(r.Results, result)
		}
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/harness/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/harness/runner.go
git commit -m "feat(benchmark): add task runner"
```

---

## Task 10: Create Report Generator

**Files:**
- Create: `benchmark/harness/report.go`

- [ ] **Step 1: Create report generator**

Create `benchmark/harness/report.go`:

```go
package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Report holds aggregated benchmark results.
type Report struct {
	Workdir   string                 `json:"workdir"`
	Timestamp time.Time              `json:"timestamp"`
	Summary   ReportSummary          `json:"summary"`
	ByTask    map[string]*TaskReport `json:"tasks"`
}

// ReportSummary holds aggregate metrics.
type ReportSummary struct {
	NoMCP    AggregateMetrics `json:"no_mcp"`
	WithMCP  AggregateMetrics `json:"with_mcp"`
	ChangePct ChangeMetrics   `json:"change_pct"`
}

// AggregateMetrics holds summed metrics.
type AggregateMetrics struct {
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	ToolCalls    int           `json:"tool_calls"`
	ReadCalls    int           `json:"read_calls"`
	GrepCalls    int           `json:"grep_calls"`
	MCPCalls     int           `json:"mcp_calls"`
	TotalTime    time.Duration `json:"total_time"`
	TasksPassed  int           `json:"tasks_passed"`
	TasksTotal   int           `json:"tasks_total"`
}

// ChangeMetrics holds percentage changes.
type ChangeMetrics struct {
	InputTokens float64 `json:"input_tokens"`
	OutputTokens float64 `json:"output_tokens"`
	ToolCalls   float64 `json:"tool_calls"`
	TotalTime   float64 `json:"total_time"`
}

// TaskReport holds per-task comparison.
type TaskReport struct {
	Name      string         `json:"name"`
	Category  string         `json:"category"`
	NoMCP     *TaskMetrics   `json:"no_mcp,omitempty"`
	WithMCP   *TaskMetrics   `json:"with_mcp,omitempty"`
	ChangePct *ChangeMetrics `json:"change_pct,omitempty"`
}

// GenerateReport creates a report from run results.
func GenerateReport(workdir string, results []*RunResult) *Report {
	report := &Report{
		Workdir:   workdir,
		Timestamp: time.Now(),
		ByTask:    make(map[string]*TaskReport),
	}

	// Group results by task
	for _, r := range results {
		taskName := r.Task.Name()
		if _, ok := report.ByTask[taskName]; !ok {
			report.ByTask[taskName] = &TaskReport{
				Name:     taskName,
				Category: r.Task.Category(),
			}
		}

		tr := report.ByTask[taskName]
		if r.MCPEnabled {
			tr.WithMCP = r.Metrics
			report.Summary.WithMCP.TasksTotal++
			if r.Metrics.Success {
				report.Summary.WithMCP.TasksPassed++
			}
			report.Summary.WithMCP.InputTokens += r.Metrics.InputTokens
			report.Summary.WithMCP.OutputTokens += r.Metrics.OutputTokens
			report.Summary.WithMCP.ToolCalls += len(r.Metrics.ToolCalls)
			report.Summary.WithMCP.ReadCalls += r.Metrics.ReadCount
			report.Summary.WithMCP.GrepCalls += r.Metrics.GrepCount
			report.Summary.WithMCP.MCPCalls += r.Metrics.MCPToolCount
			report.Summary.WithMCP.TotalTime += r.Metrics.WallTime
		} else {
			tr.NoMCP = r.Metrics
			report.Summary.NoMCP.TasksTotal++
			if r.Metrics.Success {
				report.Summary.NoMCP.TasksPassed++
			}
			report.Summary.NoMCP.InputTokens += r.Metrics.InputTokens
			report.Summary.NoMCP.OutputTokens += r.Metrics.OutputTokens
			report.Summary.NoMCP.ToolCalls += len(r.Metrics.ToolCalls)
			report.Summary.NoMCP.ReadCalls += r.Metrics.ReadCount
			report.Summary.NoMCP.GrepCalls += r.Metrics.GrepCount
			report.Summary.NoMCP.TotalTime += r.Metrics.WallTime
		}
	}

	// Calculate percentage changes
	report.Summary.ChangePct = calcChange(report.Summary.NoMCP, report.Summary.WithMCP)

	// Calculate per-task changes
	for _, tr := range report.ByTask {
		if tr.NoMCP != nil && tr.WithMCP != nil {
			change := ChangeMetrics{
				InputTokens:  pctChange(tr.NoMCP.InputTokens, tr.WithMCP.InputTokens),
				OutputTokens: pctChange(tr.NoMCP.OutputTokens, tr.WithMCP.OutputTokens),
				ToolCalls:    pctChange(len(tr.NoMCP.ToolCalls), len(tr.WithMCP.ToolCalls)),
				TotalTime:    pctChange(int(tr.NoMCP.WallTime.Milliseconds()), int(tr.WithMCP.WallTime.Milliseconds())),
			}
			tr.ChangePct = &change
		}
	}

	return report
}

func pctChange(old, new int) float64 {
	if old == 0 {
		return 0
	}
	return float64(new-old) / float64(old) * 100
}

func calcChange(old, new AggregateMetrics) ChangeMetrics {
	return ChangeMetrics{
		InputTokens:  pctChange(old.InputTokens, new.InputTokens),
		OutputTokens: pctChange(old.OutputTokens, new.OutputTokens),
		ToolCalls:    pctChange(old.ToolCalls, new.ToolCalls),
		TotalTime:    pctChange(int(old.TotalTime.Milliseconds()), int(new.TotalTime.Milliseconds())),
	}
}

// WriteJSON writes the report as JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteTerminal writes a formatted terminal report.
func (r *Report) WriteTerminal(w io.Writer) {
	fmt.Fprintf(w, "MCP Index Benchmark Report\n")
	fmt.Fprintf(w, "==========================\n")
	fmt.Fprintf(w, "Workdir: %s\n", r.Workdir)
	fmt.Fprintf(w, "Date: %s\n\n", r.Timestamp.Format("2006-01-02 15:04:05"))

	fmt.Fprintf(w, "SUMMARY\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 70))
	fmt.Fprintf(w, "%-25s %12s %12s %10s\n", "", "No MCP", "With MCP", "Change")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 70))
	fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Input Tokens", r.Summary.NoMCP.InputTokens, r.Summary.WithMCP.InputTokens, r.Summary.ChangePct.InputTokens)
	fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Output Tokens", r.Summary.NoMCP.OutputTokens, r.Summary.WithMCP.OutputTokens, r.Summary.ChangePct.OutputTokens)
	fmt.Fprintf(w, "%-25s %12d %12d %+9.0f%%\n", "Tool Calls", r.Summary.NoMCP.ToolCalls, r.Summary.WithMCP.ToolCalls, r.Summary.ChangePct.ToolCalls)
	fmt.Fprintf(w, "%-25s %12d %12d\n", "Read Calls", r.Summary.NoMCP.ReadCalls, r.Summary.WithMCP.ReadCalls)
	fmt.Fprintf(w, "%-25s %12d %12d\n", "Grep Calls", r.Summary.NoMCP.GrepCalls, r.Summary.WithMCP.GrepCalls)
	fmt.Fprintf(w, "%-25s %12s %12d\n", "MCP Calls", "-", r.Summary.WithMCP.MCPCalls)
	fmt.Fprintf(w, "%-25s %12s %12s %+9.0f%%\n", "Total Time", formatDuration(r.Summary.NoMCP.TotalTime), formatDuration(r.Summary.WithMCP.TotalTime), r.Summary.ChangePct.TotalTime)
	fmt.Fprintf(w, "%-25s %9d/%d %9d/%d\n", "Tasks Passed", r.Summary.NoMCP.TasksPassed, r.Summary.NoMCP.TasksTotal, r.Summary.WithMCP.TasksPassed, r.Summary.WithMCP.TasksTotal)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("-", 70))

	fmt.Fprintf(w, "BY TASK\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	fmt.Fprintf(w, "%-30s %10s %10s %10s %10s\n", "Task", "No MCP", "With MCP", "Change", "Status")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
	for _, tr := range r.ByTask {
		noMCPTokens := 0
		withMCPTokens := 0
		status := "?"
		change := 0.0
		if tr.NoMCP != nil {
			noMCPTokens = tr.NoMCP.InputTokens
		}
		if tr.WithMCP != nil {
			withMCPTokens = tr.WithMCP.InputTokens
		}
		if tr.ChangePct != nil {
			change = tr.ChangePct.InputTokens
		}
		if (tr.NoMCP == nil || tr.NoMCP.Success) && (tr.WithMCP == nil || tr.WithMCP.Success) {
			status = "PASS"
		} else {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%-30s %10d %10d %+9.0f%% %10s\n", tr.Name, noMCPTokens, withMCPTokens, change, status)
	}
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/harness/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/harness/report.go
git commit -m "feat(benchmark): add report generator"
```

---

## Task 11: Create MCP Server Helper for Benchmark

**Files:**
- Create: `benchmark/harness/mcp.go`

- [ ] **Step 1: Create MCP helper**

Create `benchmark/harness/mcp.go`:

```go
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./benchmark/harness/...
```

Expected: Success

- [ ] **Step 3: Commit**

```bash
git add benchmark/harness/mcp.go
git commit -m "feat(benchmark): add MCP server helper"
```

---

## Task 12: Create CLI Entry Point

**Files:**
- Create: `benchmark/cmd/cs-benchmark/main.go`

- [ ] **Step 1: Create CLI**

Create `benchmark/cmd/cs-benchmark/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claude-squad/benchmark/harness"
	"claude-squad/benchmark/tasks"
	// Import task packages to register them
	_ "claude-squad/benchmark/tasks"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse flags
	var (
		workdir    string
		taskFilter string
		jsonOutput bool
		mcpOnly    bool
		noMCPOnly  bool
		verbose    bool
		timeout    time.Duration
	)

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--workdir" && i+1 < len(os.Args):
			i++
			workdir = os.Args[i]
		case arg == "--tasks" && i+1 < len(os.Args):
			i++
			taskFilter = os.Args[i]
		case arg == "--json":
			jsonOutput = true
		case arg == "--mcp-only":
			mcpOnly = true
		case arg == "--no-mcp-only":
			noMCPOnly = true
		case arg == "--verbose":
			verbose = true
		case arg == "--timeout" && i+1 < len(os.Args):
			i++
			var err error
			timeout, err = time.ParseDuration(os.Args[i])
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}
		case arg == "--help" || arg == "-h":
			printUsage()
			return nil
		}
	}

	if workdir == "" {
		return fmt.Errorf("--workdir is required")
	}

	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted, cleaning up...")
		cancel()
	}()

	// Get tasks to run
	var taskList []tasks.Task
	if taskFilter == "" {
		taskList = tasks.GetAll()
	} else {
		categories := strings.Split(taskFilter, ",")
		for _, cat := range categories {
			taskList = append(taskList, tasks.GetByCategory(strings.TrimSpace(cat))...)
		}
	}

	if len(taskList) == 0 {
		return fmt.Errorf("no tasks to run")
	}

	if !jsonOutput {
		fmt.Printf("Running %d tasks in %s\n", len(taskList), workdir)
	}

	// Start MCP server (if needed)
	var mcpConfig string
	if !noMCPOnly {
		if !jsonOutput {
			fmt.Println("Starting MCP index server...")
		}
		mcpServer, err := harness.StartMCPServer(workdir)
		if err != nil {
			return fmt.Errorf("failed to start MCP server: %w", err)
		}
		defer mcpServer.Stop()

		// Wait for indexer to build
		time.Sleep(2 * time.Second)
		mcpConfig = mcpServer.Config()
		if !jsonOutput {
			fmt.Printf("MCP server running on port %d\n", mcpServer.Port())
		}
	}

	// Run tasks
	runner := harness.NewRunner(harness.RunConfig{
		Workdir: workdir,
		Timeout: timeout,
		Verbose: verbose,
	})

	for i, task := range taskList {
		if ctx.Err() != nil {
			break
		}

		if !jsonOutput {
			fmt.Printf("[%d/%d] Running: %s\n", i+1, len(taskList), task.Name())
		}

		// Run without MCP
		if !mcpOnly {
			if !jsonOutput && verbose {
				fmt.Printf("  Without MCP...\n")
			}
			cfg := runner.Config
			cfg.MCPConfig = ""
			result := harness.RunTask(ctx, task, cfg)
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [NO MCP] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [NO MCP] %d input tokens, %d tool calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls))
				}
			}
		}

		// Run with MCP
		if !noMCPOnly && mcpConfig != "" {
			if !jsonOutput && verbose {
				fmt.Printf("  With MCP...\n")
			}
			cfg := runner.Config
			cfg.MCPConfig = mcpConfig
			result := harness.RunTask(ctx, task, cfg)
			runner.Results = append(runner.Results, result)
			if !jsonOutput {
				if result.Error != nil {
					fmt.Printf("  [W/ MCP] FAIL: %v\n", result.Error)
				} else {
					fmt.Printf("  [W/ MCP] %d input tokens, %d tool calls, %d MCP calls\n", result.Metrics.InputTokens, len(result.Metrics.ToolCalls), result.Metrics.MCPToolCount)
				}
			}
		}
	}

	// Generate report
	report := harness.GenerateReport(workdir, runner.Results)

	if jsonOutput {
		report.WriteJSON(os.Stdout)
	} else {
		fmt.Println()
		report.WriteTerminal(os.Stdout)
	}

	return nil
}

func printUsage() {
	fmt.Println(`cs-benchmark - MCP Index Server Benchmark Tool

Usage:
  cs-benchmark --workdir <path> [options]

Options:
  --workdir <path>    Target codebase to run tasks against (required)
  --tasks <list>      Comma-separated categories: symbol,understanding,edit,crossfile
  --json              Output JSON instead of terminal tables
  --mcp-only          Only run MCP-enabled variant
  --no-mcp-only       Only run MCP-disabled variant  
  --verbose           Show detailed output
  --timeout <dur>     Per-task timeout (default: 5m)
  --help, -h          Show this help

Examples:
  cs-benchmark --workdir ~/code/myproject
  cs-benchmark --workdir ~/code/myproject --tasks symbol,understanding
  cs-benchmark --workdir ~/code/myproject --json > report.json`)
}
```

- [ ] **Step 2: Build the CLI**

```bash
go build -o bin/cs-benchmark ./benchmark/cmd/cs-benchmark
```

Expected: Success

- [ ] **Step 3: Test help output**

```bash
./bin/cs-benchmark --help
```

Expected: Shows usage information

- [ ] **Step 4: Commit**

```bash
git add benchmark/cmd/cs-benchmark/main.go
git commit -m "feat(benchmark): add CLI entry point

cs-benchmark runs predefined tasks with/without MCP and
generates comparative reports showing token savings."
```

---

## Task 13: Integration Test

**Files:**
- Test manually

- [ ] **Step 1: Run a quick test with symbol tasks**

```bash
./bin/cs-benchmark --workdir /Users/jadams/go/src/bitbucket.org/vervemotion --tasks symbol --timeout 2m
```

Expected: Shows results for symbol tasks with token comparison

- [ ] **Step 2: Run with JSON output**

```bash
./bin/cs-benchmark --workdir /Users/jadams/go/src/bitbucket.org/vervemotion --tasks symbol --timeout 2m --json
```

Expected: JSON output with metrics

- [ ] **Step 3: Commit final state**

```bash
git add -A
git commit -m "feat(benchmark): complete benchmark harness implementation

Includes:
- Task interface and predefined tasks (symbol, understanding, edit, crossfile)
- Metrics parsing from Claude's stream-json output
- Task runner with MCP/no-MCP variants
- Report generation (terminal tables + JSON)
- cs-benchmark CLI"
```

---

## Summary

This plan implements:
1. **Per-session MCP toggle** - `MCPEnabled` field in CreateOptions
2. **Standalone MCP server** - `NewMCPIndexServerStandalone` constructor
3. **Task framework** - Interface, registry, 10 predefined tasks
4. **Metrics collection** - Parse Claude's stream-json output
5. **Report generation** - Terminal tables and JSON output
6. **CLI tool** - `cs-benchmark` with full flag support

After implementation, run benchmarks with:
```bash
./bin/cs-benchmark --workdir /path/to/vervemotion
```
