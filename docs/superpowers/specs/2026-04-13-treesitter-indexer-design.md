# Tree-Sitter Indexer Design

## Overview

Replace the ctags-based `SessionIndexer` with a tree-sitter-based implementation that provides better symbol extraction, call graph analysis, and BM25-ranked search. The design ports pitlane-mcp's approach to Go while sharing in-memory state with claude-squad's session management.

## Goals

1. **Better relevance** — BM25-ranked search instead of substring matching
2. **Call graph support** — `find_callers`/`find_callees` for code navigation
3. **24 language support** — Full pitlane-mcp parity
4. **pitlane-compatible API** — Same MCP tools for familiarity
5. **Shared memory** — In-process indexer, no subprocess overhead
6. **Disk persistence** — Fast startup from cached index

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  SessionAPI                                                      │
│  ┌─────────────────┐    ┌─────────────────────────────────────┐ │
│  │  Session        │    │  TreeSitterIndexer (replaces ctags) │ │
│  │  Management     │───▶│  ┌─────────────┐ ┌───────────────┐  │ │
│  └─────────────────┘    │  │ QueryEngine │ │ CallGraph     │  │ │
│                         │  └─────────────┘ └───────────────┘  │ │
│                         │  ┌─────────────┐ ┌───────────────┐  │ │
│                         │  │ BleveSearch │ │ Persistence   │  │ │
│                         │  └─────────────┘ └───────────────┘  │ │
│                         └─────────────────────────────────────┘ │
│                                       │                         │
│                                       ▼                         │
│                         ┌─────────────────────────────────────┐ │
│                         │  MCPIndexServer (pitlane API)       │ │
│                         │  • get_symbol      • find_callers   │ │
│                         │  • search_symbols  • find_callees   │ │
│                         │  • search_content  • list_files     │ │
│                         └─────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Components

### TreeSitterIndexer

Replaces `SessionIndexer` with same public interface plus new methods.

```go
type TreeSitterIndexer struct {
    worktree    string
    gitignore   *gitignore.Matcher    // .gitignore parsing
    engine      *QueryEngine          // tree-sitter execution
    symbols     *SymbolStore          // in-memory symbol table
    callgraph   *CallGraph            // caller/callee relationships
    search      *bleve.Index          // BM25 full-text search
    persistence *IndexStore           // disk persistence
    watcher     *fsnotify.Watcher     // file change detection
}

// Existing interface (unchanged)
func (idx *TreeSitterIndexer) Start()
func (idx *TreeSitterIndexer) Stop()
func (idx *TreeSitterIndexer) Refresh()
func (idx *TreeSitterIndexer) Lookup(name string) []Definition
func (idx *TreeSitterIndexer) AllSymbols() map[string][]Definition
func (idx *TreeSitterIndexer) Files() []string
func (idx *TreeSitterIndexer) Worktree() string

// New methods for pitlane-compatible API
func (idx *TreeSitterIndexer) GetSymbol(name string, signatureOnly bool) *Symbol
func (idx *TreeSitterIndexer) SearchSymbols(query string, limit int) []Symbol
func (idx *TreeSitterIndexer) SearchContent(query string, limit int) []Match
func (idx *TreeSitterIndexer) FindCallers(symbol string) []Reference
func (idx *TreeSitterIndexer) FindCallees(symbol string) []Reference
```

### QueryEngine

Executes tree-sitter queries across 24 languages.

```go
type QueryEngine struct {
    parsers map[string]*sitter.Parser   // language -> parser
    queries map[string]*sitter.Query    // language -> symbol query
}

func (e *QueryEngine) ParseFile(path string, content []byte) (*ParseResult, error)
func (e *QueryEngine) ExtractSymbols(tree *sitter.Tree, lang string) []Symbol
func (e *QueryEngine) ExtractReferences(tree *sitter.Tree, lang string) []Reference
```

### File Structure

```
app/
  indexer.go              # thin wrapper delegating to TreeSitterIndexer
  indexer_treesitter.go   # TreeSitterIndexer implementation
  indexer_queries.go      # embedded .scm query strings (24 languages)
  indexer_callgraph.go    # call graph building and queries
  indexer_search.go       # bleve integration
  indexer_persistence.go  # disk storage/loading
  mcp_server.go           # updated with pitlane-compatible tools
```

## Data Structures

### Symbol

```go
type Symbol struct {
    Name       string   `json:"name"`
    Kind       string   `json:"kind"`       // function, type, variable, method, class
    File       string   `json:"file"`
    Line       int      `json:"line"`
    EndLine    int      `json:"end_line"`   // for extracting full body
    Column     int      `json:"column"`
    Language   string   `json:"language"`
    Scope      string   `json:"scope"`      // parent class/module
    Signature  string   `json:"signature"`  // function signature
    DocComment string   `json:"doc,omitempty"`
}
```

### Reference (for call graph)

```go
type Reference struct {
    Symbol     string   `json:"symbol"`     // what's being called
    File       string   `json:"file"`       // where the call is
    Line       int      `json:"line"`
    Column     int      `json:"column"`
    Caller     string   `json:"caller"`     // function containing the call
    Kind       string   `json:"kind"`       // call, import, type_ref
}
```

### Persisted Index

Stored in session's active directory (worktree or in-place):

```
<session-dir>/.claude-squad/index/
  ├── meta.json           # version, last indexed commit, file count
  ├── symbols.bin         # gob-encoded symbol table
  ├── callgraph.bin       # gob-encoded references
  └── search.bleve/       # bleve index directory
```

## MCP Tools (pitlane-compatible)

| Tool | Description |
|------|-------------|
| `get_symbol` | Get symbol definition with optional full body |
| `search_symbols` | BM25-ranked symbol search |
| `search_content` | Full-text search in file contents |
| `get_file_outline` | Symbols in a file |
| `list_files` | Git-tracked files |
| `read_file` | Read file or line range |
| `find_callers` | Who calls this symbol? |
| `find_callees` | What does this symbol call? |
| `index_status` | Index health and stats |

### Tool Signatures

```go
// get_symbol: Returns definition + optionally full source
// Input: {name: string, signature_only?: bool}
// Output: Symbol with source code if signature_only=false

// search_symbols: BM25-ranked search
// Input: {query: string, limit?: int}
// Output: []Symbol ranked by relevance

// search_content: Full-text search in code
// Input: {query: string, limit?: int}  
// Output: []Match with file, line, context

// find_callers: Reverse call graph lookup
// Input: {symbol: string}
// Output: []Reference showing where symbol is called

// find_callees: Forward call graph lookup
// Input: {symbol: string}
// Output: []Reference showing what symbol calls
```

## Indexing Flow

### Initial Build

1. Check for persisted index at `.claude-squad/index/`
2. If found and valid (commit matches HEAD): load from disk
3. Otherwise full rebuild:
   - Run `git ls-files`
   - Filter via `.gitignore`
   - Parse each file in parallel by language
   - Extract symbols and references
   - Build bleve index
   - Persist to disk

### Incremental Updates

File watcher (fsnotify) detects changes:
1. Check `.gitignore` — skip if ignored
2. Re-parse single file
3. Update symbols (remove old, add new)
4. Update call graph references
5. Update bleve index
6. Mark dirty, persist on idle or Stop

### Branch Switch Detection

Monitor `.git/HEAD` or check HEAD sha every 10s:
- If HEAD changed significantly: full rebuild
- Same commit: no action

## File Exclusions

Use `.gitignore` as the source of truth for exclusions. Parse with `github.com/go-git/go-git/v5/plumbing/format/gitignore` (already a dependency).

This automatically excludes:
- `vendor/`, `node_modules/`
- Build artifacts
- IDE files
- Project-specific exclusions

## Supported Languages (24)

Ported from pitlane-mcp (MIT licensed):

bash, c, cpp, csharp, go, java, javascript, kotlin, lua, objc, php, python, ruby, rust, solidity, svelte, swift, typescript, zig

Plus additional tree-sitter grammars available in go-tree-sitter.

## Dependencies

```go
import (
    sitter "github.com/smacker/go-tree-sitter"           // tree-sitter bindings
    "github.com/smacker/go-tree-sitter/golang"           // Go grammar
    "github.com/smacker/go-tree-sitter/typescript"       // TS grammar
    // ... 24 language grammars
    
    "github.com/blevesearch/bleve/v2"                    // BM25 search
    "github.com/fsnotify/fsnotify"                       // file watching
    "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)
```

## Error Handling

- **Parse failures**: Log warning, skip file, don't fail entire index
- **Unknown extensions**: Skip silently
- **Binary files**: Detect via content sniffing, skip
- **Large repos**: Process in batches (1000 files), show progress
- **Index corruption**: Validate checksum on load, rebuild if corrupt
- **Concurrent access**: `sync.RWMutex` for symbols, bleve handles its own

## CLAUDE.md Guidance

Updated guidance written to session worktrees:

```markdown
# MCP Index Server

Prefer index tools over Grep/Read for code navigation:

- `get_symbol` — retrieve exact implementation instead of reading whole files
- `search_symbols` — find symbols by name pattern (BM25 ranked)
- `find_callers`/`find_callees` — trace call relationships
- `get_file_outline` — understand file structure before reading

Fall back to Grep/Read for text search or when editing files.
```

## Testing Strategy

### Unit Tests
- Per-language query validation (24 languages)
- Call graph construction and queries
- BM25 ranking relevance
- Persistence round-trip

### Integration Tests
- Index real repo fixtures
- Incremental update on file change
- .gitignore exclusion verification
- Branch switch rebuild

### Benchmark Comparison
- Update `cs-benchmark` to compare ctags vs tree-sitter
- Measure token savings with improved relevance
- Track MCP tool call counts

### Test Fixtures
```
testdata/
  fixtures/
    small-go-project/       # 10 files, basic Go
    multi-language/         # Go + TS + Python mix
    with-vendor/            # includes vendor/, node_modules/
```

## Migration Path

1. Implement `TreeSitterIndexer` with same interface as `SessionIndexer`
2. Add feature flag to switch between backends
3. Update MCP server with new tools (backward compatible)
4. Default to tree-sitter for new sessions
5. Eventually deprecate ctags backend
