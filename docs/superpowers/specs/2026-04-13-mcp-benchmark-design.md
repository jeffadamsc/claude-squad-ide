# MCP Index Benchmark System Design

## Overview

A benchmark harness to measure the effectiveness of the MCP index server in reducing token usage and improving task performance. The system runs Claude Code against a standard set of tasks with and without MCP enabled, then generates comparative reports.

## Goals

1. Quantify token savings from using the MCP index server
2. Measure performance impact across different task types
3. Provide reproducible benchmarks for iterating on indexer improvements
4. Enable per-session MCP toggle for manual A/B testing

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  cs-benchmark CLI                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Task Runner │  │  Metrics    │  │  Report Generator   │  │
│  │             │──│  Collector  │──│  (terminal + JSON)  │  │
│  └──────┬──────┘  └─────────────┘  └─────────────────────┘  │
└─────────┼───────────────────────────────────────────────────┘
          │ spawns subprocess
          ▼
┌─────────────────────────────────────────────────────────────┐
│  claude --print --output-format stream-json                 │
│  --mcp-config <config>  (when MCP enabled)                  │
│  Working dir: target codebase                               │
└─────────────────────────────────────────────────────────────┘
          │ connects to (when MCP enabled)
          ▼
┌─────────────────────────────────────────────────────────────┐
│  MCP Index Server (started by benchmark harness)            │
│  localhost:<port>/mcp/<task-id>                             │
└─────────────────────────────────────────────────────────────┘
```

**Key decisions:**
- Benchmark harness starts its own MCP server (independent of claude-squad app)
- Each task runs twice: once with MCP, once without
- Tasks run sequentially to avoid interference
- Results are collected and compared after all tasks complete

## Directory Structure

```
benchmark/
├── cmd/
│   └── cs-benchmark/main.go    # CLI entry point
├── harness/
│   ├── runner.go               # Spawns Claude Code, captures output
│   ├── metrics.go              # Parse tokens, tool calls from JSON
│   └── report.go               # Generate comparison reports
├── tasks/
│   ├── task.go                 # Task interface and registry
│   ├── symbol_lookup.go        # Symbol lookup tasks
│   ├── code_understanding.go   # Code understanding tasks
│   ├── small_edit.go           # Small edit tasks
│   └── cross_file.go           # Cross-file tasks
└── testdata/
    └── expected/               # Optional expected outputs for validation
```

## Per-Session MCP Toggle

Add `MCPEnabled` field to `CreateOptions` in `app/bindings.go`:

```go
type CreateOptions struct {
    // ... existing fields ...
    MCPEnabled *bool `json:"mcpEnabled,omitempty"` // nil = default (true)
}
```

Behavior:
- `nil` (omitted): MCP enabled by default
- `true`: Explicitly enable MCP
- `false`: Disable MCP for this session

Update `setMCPConfig()` to check this field before injecting MCP config.

## CLI Interface

```bash
# Run all benchmark tasks
cs-benchmark --workdir /path/to/vervemotion

# Run specific task categories
cs-benchmark --workdir /path/to/vervemotion --tasks symbol,understanding

# JSON output for scripting
cs-benchmark --workdir /path/to/vervemotion --json

# Run only one variant (for debugging)
cs-benchmark --workdir /path/to/vervemotion --mcp-only
cs-benchmark --workdir /path/to/vervemotion --no-mcp-only

# Verbose output (show Claude's responses)
cs-benchmark --workdir /path/to/vervemotion --verbose
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--workdir` | required | Target codebase to run tasks against |
| `--tasks` | all | Comma-separated: `symbol,understanding,edit,crossfile` |
| `--json` | false | Output JSON instead of terminal tables |
| `--mcp-only` | false | Only run MCP-enabled variant |
| `--no-mcp-only` | false | Only run MCP-disabled variant |
| `--verbose` | false | Show full Claude responses |
| `--timeout` | 5m | Per-task timeout |

## Task Definitions

### Interface

```go
type Task interface {
    Name() string                  // e.g., "symbol-lookup-handler"
    Category() string              // symbol, understanding, edit, crossfile
    Prompt() string                // The prompt to send to Claude
    Validate(output string) error  // Optional: check if output is correct
}
```

### Predefined Tasks (vervemotion codebase)

**Note:** These prompts reference specific symbols that should be verified in the vervemotion codebase before implementation. Prompts may need adjustment to reference actual function/type names.

#### Symbol Lookup
| Name | Prompt |
|------|--------|
| `symbol-find-handler` | "Find all usages of the HandleRequest function in verve-backend" |
| `symbol-find-type` | "Where is the Session struct defined and what files use it?" |
| `symbol-find-constant` | "Find where API_VERSION is defined and all places it's referenced" |

#### Code Understanding
| Name | Prompt |
|------|--------|
| `understand-session-lifecycle` | "Explain how a session is created, processed, and stored in verve-backend" |
| `understand-api-endpoint` | "What does the /api/v1/sessions endpoint do? Trace the request flow" |
| `understand-gateway` | "How does verve-gateway communicate with verve-backend?" |

#### Small Edits
| Name | Prompt |
|------|--------|
| `edit-add-log` | "Add a debug log statement at the start of the CreateSession function in verve-backend" |
| `edit-add-comment` | "Add a comment explaining the purpose of the validateToken function" |

#### Cross-File
| Name | Prompt |
|------|--------|
| `crossfile-new-endpoint` | "Add a new /api/v1/health endpoint to verve-backend following the pattern of existing endpoints" |
| `crossfile-refactor` | "The error handling in CreateSession should follow the pattern used in UpdateSession - make it consistent" |

## Metrics Collection

### Data Structure

```go
type TaskMetrics struct {
    TaskName     string
    MCPEnabled   bool
    
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
    WallTime     time.Duration
    
    // Outcome
    Success      bool
    Error        string
}

type ToolCall struct {
    Name         string
    InputTokens  int
    OutputTokens int
}
```

### Collection Method

1. Spawn `claude --print --output-format stream-json -p "<prompt>"`
2. Read JSON lines from stdout as they arrive
3. Parse `assistant` messages for tool calls and token counts
4. Parse final `result` message for totals
5. Measure wall time from spawn to exit

## Report Generation

### Terminal Output

```
MCP Index Benchmark Report
==========================
Workdir: /Users/jadams/go/src/bitbucket.org/vervemotion
Date: 2026-04-13 15:30:00

SUMMARY
┌─────────────────────┬───────────┬───────────┬──────────┐
│                     │  No MCP   │  With MCP │  Change  │
├─────────────────────┼───────────┼───────────┼──────────┤
│ Total Input Tokens  │   45,230  │   12,450  │   -72%   │
│ Total Output Tokens │    8,120  │    7,890  │    -3%   │
│ Total Tool Calls    │      156  │       42  │   -73%   │
│ Read Calls          │       89  │       12  │   -87%   │
│ Grep Calls          │       45  │        8  │   -82%   │
│ MCP Calls           │        0  │       22  │     -    │
│ Total Time          │    4m32s  │    2m15s  │   -50%   │
│ Tasks Passed        │     10/10 │     10/10 │     -    │
└─────────────────────┴───────────┴───────────┴──────────┘
```

### JSON Output

```json
{
  "workdir": "/path/to/vervemotion",
  "timestamp": "2026-04-13T15:30:00Z",
  "summary": {
    "no_mcp": {"input_tokens": 45230, "output_tokens": 8120},
    "with_mcp": {"input_tokens": 12450, "output_tokens": 7890},
    "change_pct": {"input_tokens": -72.5, "output_tokens": -2.8}
  },
  "tasks": [
    {"name": "symbol-find-handler", "no_mcp": {...}, "with_mcp": {...}}
  ]
}
```

## Error Handling

### Task Timeout
- Default 5 minutes per task (configurable via `--timeout`)
- Timeout results in task failure; harness continues to next task

### Claude Code Errors
- Non-zero exit: capture stderr, mark task failed
- JSON parse errors: mark failed, save raw output
- API rate limits: retry once after 30s, then fail

### Task Validation
Optional `Validate(output string) error` method checks expected results.

### Comparison Validity
- If one variant fails but the other succeeds, report it but don't compute percentage change
- Both must succeed for a valid comparison

## Implementation Notes

### MCP Server for Benchmark

The benchmark harness reuses `app.MCPIndexServer` and `app.SessionIndexer` but runs them standalone:

```go
// Create indexer for target workdir
indexer := app.NewSessionIndexer(workdir)
indexer.Start()
defer indexer.Stop()

// Start MCP server
mcpServer := app.NewMCPIndexServerStandalone(indexer)
port, _ := mcpServer.Start()
defer mcpServer.Stop()

// Generate MCP config for Claude
config := mcpServer.GenerateMCPConfig("benchmark")
```

This requires a small refactor to allow `MCPIndexServer` to work without a full `SessionAPI`.

### Subprocess Execution

```go
func runTask(task Task, mcpConfig string, timeout time.Duration) (*TaskMetrics, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    args := []string{"--print", "--output-format", "stream-json", "-p", task.Prompt()}
    if mcpConfig != "" {
        args = append(args, "--mcp-config", mcpConfig)
    }
    
    cmd := exec.CommandContext(ctx, "claude", args...)
    cmd.Dir = workdir
    
    // Capture and parse output...
}
```

## Future Enhancements

- Historical tracking: save JSON reports with timestamps for trend analysis
- CI integration: run benchmarks on PRs that modify the indexer
- Additional task types: debugging tasks, test-writing tasks
- Multiple codebases: run against different repos to test generalization
