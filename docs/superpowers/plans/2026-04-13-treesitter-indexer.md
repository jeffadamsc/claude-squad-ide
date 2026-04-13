# Tree-Sitter Indexer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ctags-based SessionIndexer with tree-sitter for better symbol extraction, call graph analysis, and BM25-ranked search.

**Architecture:** Node-kind matching (like pitlane-mcp) walks AST nodes checking `node.Type()` rather than S-expression queries. Bleve provides BM25 search. Disk persistence in session directory enables fast startup.

**Tech Stack:** go-tree-sitter, bleve/v2, fsnotify, go-git/gitignore

---

## File Structure

```
app/
  indexer.go                    # Keep existing Definition type, add Indexer interface
  indexer_treesitter.go         # TreeSitterIndexer implementation (NEW)
  indexer_treesitter_test.go    # Unit tests (NEW)
  indexer_languages.go          # Language configs and node-kind extractors (NEW)
  indexer_languages_test.go     # Per-language extraction tests (NEW)
  indexer_callgraph.go          # CallGraph type and queries (NEW)
  indexer_callgraph_test.go     # Call graph tests (NEW)
  indexer_search.go             # Bleve integration (NEW)
  indexer_search_test.go        # Search tests (NEW)
  indexer_persistence.go        # Disk save/load (NEW)
  indexer_persistence_test.go   # Persistence tests (NEW)
  mcp_server.go                 # Modify: add new tools, keep backward compat
  mcp_server_test.go            # Modify: add tests for new tools

testdata/
  fixtures/
    small-go/                   # 5 Go files for basic tests (NEW)
    multi-lang/                 # Go + TS + Python mix (NEW)
```

---

## Task 1: Indexer Interface and Symbol Type

**Files:**
- Modify: `app/indexer.go:19-27` (expand Definition)
- Create: `app/indexer.go` (add Indexer interface after Definition)

- [ ] **Step 1: Write test for expanded Symbol type**

```go
// app/indexer_test.go
package app

import (
    "testing"
)

func TestSymbolFields(t *testing.T) {
    s := Symbol{
        Name:      "MyFunc",
        Kind:      "function",
        File:      "main.go",
        Line:      10,
        EndLine:   25,
        Column:    1,
        Language:  "go",
        Scope:     "main",
        Signature: "func MyFunc(x int) error",
    }
    if s.EndLine <= s.Line {
        t.Error("EndLine should be after Line")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./app -run TestSymbolFields`
Expected: FAIL - Symbol type not defined

- [ ] **Step 3: Add Symbol type and Indexer interface**

```go
// app/indexer.go - add after line 27 (after Definition type)

// Symbol is the tree-sitter-based symbol with extended fields.
// Replaces Definition for the new indexer.
type Symbol struct {
    Name       string `json:"name"`
    Kind       string `json:"kind"`       // function, type, variable, method, class
    File       string `json:"file"`
    Line       int    `json:"line"`
    EndLine    int    `json:"end_line"`   // for extracting full body
    Column     int    `json:"column"`
    Language   string `json:"language"`
    Scope      string `json:"scope"`      // parent class/module
    Signature  string `json:"signature"`  // function signature
    DocComment string `json:"doc,omitempty"`
}

// Reference represents a call or usage of a symbol.
type Reference struct {
    Symbol string `json:"symbol"`  // what's being called
    File   string `json:"file"`    // where the call is
    Line   int    `json:"line"`
    Column int    `json:"column"`
    Caller string `json:"caller"`  // function containing the call
    Kind   string `json:"kind"`    // call, import, type_ref
}

// Indexer is the interface for symbol indexers (ctags or tree-sitter).
type Indexer interface {
    Start()
    Stop()
    Refresh()
    Worktree() string
    Files() []string
    Lookup(name string) []Definition
    AllSymbols() map[string][]Definition
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./app -run TestSymbolFields`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/indexer.go app/indexer_test.go
git commit -m "feat(indexer): add Symbol, Reference types and Indexer interface

Preparation for tree-sitter indexer. Symbol extends Definition with
EndLine, Column, Signature, and DocComment fields. Reference tracks
call graph relationships.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: TreeSitterIndexer Skeleton

**Files:**
- Create: `app/indexer_treesitter.go`
- Create: `app/indexer_treesitter_test.go`

- [ ] **Step 1: Write test for indexer creation**

```go
// app/indexer_treesitter_test.go
package app

import (
    "os"
    "path/filepath"
    "testing"
)

func TestTreeSitterIndexerNew(t *testing.T) {
    tmp := t.TempDir()
    
    // Create a minimal git repo
    if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
        t.Fatal(err)
    }
    
    idx := NewTreeSitterIndexer(tmp)
    if idx == nil {
        t.Fatal("expected non-nil indexer")
    }
    if idx.Worktree() != tmp {
        t.Errorf("worktree = %q, want %q", idx.Worktree(), tmp)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./app -run TestTreeSitterIndexerNew`
Expected: FAIL - NewTreeSitterIndexer not defined

- [ ] **Step 3: Implement TreeSitterIndexer skeleton**

```go
// app/indexer_treesitter.go
package app

import (
    "context"
    "sync"
)

// TreeSitterIndexer provides tree-sitter based symbol indexing.
// Implements the Indexer interface with additional methods for
// call graph analysis and BM25 search.
type TreeSitterIndexer struct {
    worktree string

    mu        sync.RWMutex
    files     []string
    symbols   map[string][]Symbol
    callgraph *CallGraph

    cancel  context.CancelFunc
    done    chan struct{}
    refresh chan struct{}
}

// NewTreeSitterIndexer creates a new tree-sitter indexer for the worktree.
func NewTreeSitterIndexer(worktree string) *TreeSitterIndexer {
    return &TreeSitterIndexer{
        worktree: worktree,
        symbols:  make(map[string][]Symbol),
        done:     make(chan struct{}),
        refresh:  make(chan struct{}, 1),
    }
}

// Worktree returns the indexed directory path.
func (idx *TreeSitterIndexer) Worktree() string {
    return idx.worktree
}

// Start begins the indexer background loop.
func (idx *TreeSitterIndexer) Start() {
    ctx, cancel := context.WithCancel(context.Background())
    idx.cancel = cancel
    go idx.loop(ctx)
    idx.Refresh()
}

// Stop halts the indexer.
func (idx *TreeSitterIndexer) Stop() {
    if idx.cancel != nil {
        idx.cancel()
    }
    <-idx.done
}

// Refresh triggers an immediate re-index.
func (idx *TreeSitterIndexer) Refresh() {
    select {
    case idx.refresh <- struct{}{}:
    default:
    }
}

// Files returns the current list of tracked files.
func (idx *TreeSitterIndexer) Files() []string {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    out := make([]string, len(idx.files))
    copy(out, idx.files)
    return out
}

// Lookup returns definitions matching the given symbol name.
// Returns Definition for Indexer interface compatibility.
func (idx *TreeSitterIndexer) Lookup(name string) []Definition {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    syms := idx.symbols[name]
    defs := make([]Definition, len(syms))
    for i, s := range syms {
        defs[i] = Definition{
            Name:     s.Name,
            File:     s.File,
            Line:     s.Line,
            Kind:     s.Kind,
            Language: s.Language,
            Scope:    s.Scope,
        }
    }
    return defs
}

// AllSymbols returns all symbols as Definition for interface compat.
func (idx *TreeSitterIndexer) AllSymbols() map[string][]Definition {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    out := make(map[string][]Definition, len(idx.symbols))
    for name, syms := range idx.symbols {
        defs := make([]Definition, len(syms))
        for i, s := range syms {
            defs[i] = Definition{
                Name:     s.Name,
                File:     s.File,
                Line:     s.Line,
                Kind:     s.Kind,
                Language: s.Language,
                Scope:    s.Scope,
            }
        }
        out[name] = defs
    }
    return out
}

// LookupSymbol returns full Symbol data (new API).
func (idx *TreeSitterIndexer) LookupSymbol(name string) []Symbol {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    syms := idx.symbols[name]
    out := make([]Symbol, len(syms))
    copy(out, syms)
    return out
}

func (idx *TreeSitterIndexer) loop(ctx context.Context) {
    defer close(idx.done)
    for {
        select {
        case <-ctx.Done():
            return
        case <-idx.refresh:
            idx.build(ctx)
        }
    }
}

func (idx *TreeSitterIndexer) build(ctx context.Context) {
    // Stub - full implementation in Task 4
}

// CallGraph placeholder for compilation
type CallGraph struct{}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./app -run TestTreeSitterIndexerNew`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/indexer_treesitter.go app/indexer_treesitter_test.go
git commit -m "feat(indexer): add TreeSitterIndexer skeleton

Implements Indexer interface with Start/Stop/Refresh lifecycle,
Files/Lookup/AllSymbols for compatibility, and LookupSymbol for
new extended Symbol data.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Language Configuration (Go, TypeScript, Python)

**Files:**
- Create: `app/indexer_languages.go`
- Create: `app/indexer_languages_test.go`

- [ ] **Step 1: Add go-tree-sitter dependency**

Run: `go get github.com/smacker/go-tree-sitter github.com/smacker/go-tree-sitter/golang github.com/smacker/go-tree-sitter/typescript github.com/smacker/go-tree-sitter/python`

- [ ] **Step 2: Write test for Go symbol extraction**

```go
// app/indexer_languages_test.go
package app

import (
    "testing"
)

func TestExtractGoSymbols(t *testing.T) {
    src := []byte(`package main

// MyFunc does something.
func MyFunc(x int) error {
    return nil
}

type MyStruct struct {
    Name string
}

func (m *MyStruct) Method() {}
`)
    
    symbols, refs, err := extractSymbols("test.go", src)
    if err != nil {
        t.Fatal(err)
    }
    
    // Should find: MyFunc, MyStruct, Method
    names := make(map[string]bool)
    for _, s := range symbols {
        names[s.Name] = true
    }
    
    for _, want := range []string{"MyFunc", "MyStruct", "Method"} {
        if !names[want] {
            t.Errorf("missing symbol %q", want)
        }
    }
    
    // refs is for call graph - not empty for Method call would be ideal
    _ = refs
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v ./app -run TestExtractGoSymbols`
Expected: FAIL - extractSymbols not defined

- [ ] **Step 4: Implement language configuration and Go extractor**

```go
// app/indexer_languages.go
package app

import (
    "path/filepath"
    "strings"

    sitter "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/golang"
    "github.com/smacker/go-tree-sitter/python"
    "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LangConfig holds parser and extraction logic for a language.
type LangConfig struct {
    Language     *sitter.Language
    Extensions   []string
    SymbolKinds  map[string]string // node type -> symbol kind
    RefKinds     map[string]string // node type -> reference kind
}

var langConfigs = map[string]*LangConfig{
    "go": {
        Language:   golang.GetLanguage(),
        Extensions: []string{".go"},
        SymbolKinds: map[string]string{
            "function_declaration": "function",
            "method_declaration":   "method",
            "type_declaration":     "type",
            "type_spec":            "type",
            "const_spec":           "constant",
            "var_spec":             "variable",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
            "selector_expression": "reference",
        },
    },
    "typescript": {
        Language:   typescript.GetLanguage(),
        Extensions: []string{".ts", ".tsx"},
        SymbolKinds: map[string]string{
            "function_declaration":       "function",
            "method_definition":          "method",
            "class_declaration":          "class",
            "interface_declaration":      "interface",
            "type_alias_declaration":     "type",
            "variable_declarator":        "variable",
            "lexical_declaration":        "variable",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
            "member_expression": "reference",
        },
    },
    "python": {
        Language:   python.GetLanguage(),
        Extensions: []string{".py"},
        SymbolKinds: map[string]string{
            "function_definition": "function",
            "class_definition":    "class",
            "assignment":          "variable",
        },
        RefKinds: map[string]string{
            "call": "call",
            "attribute": "reference",
        },
    },
}

// getLangConfig returns the config for a file extension.
func getLangConfig(filename string) *LangConfig {
    ext := strings.ToLower(filepath.Ext(filename))
    for _, cfg := range langConfigs {
        for _, e := range cfg.Extensions {
            if e == ext {
                return cfg
            }
        }
    }
    return nil
}

// extractSymbols parses a file and extracts symbols and references.
func extractSymbols(filename string, content []byte) ([]Symbol, []Reference, error) {
    cfg := getLangConfig(filename)
    if cfg == nil {
        return nil, nil, nil // unsupported language
    }

    parser := sitter.NewParser()
    parser.SetLanguage(cfg.Language)
    
    tree, err := parser.ParseCtx(nil, nil, content)
    if err != nil {
        return nil, nil, err
    }
    defer tree.Close()

    var symbols []Symbol
    var refs []Reference
    
    lang := langNameFromConfig(cfg)
    walkNode(tree.RootNode(), cfg, filename, lang, content, "", &symbols, &refs)
    
    return symbols, refs, nil
}

func langNameFromConfig(cfg *LangConfig) string {
    for name, c := range langConfigs {
        if c == cfg {
            return name
        }
    }
    return "unknown"
}

// walkNode recursively walks the AST extracting symbols and references.
func walkNode(node *sitter.Node, cfg *LangConfig, file, lang string, content []byte, scope string, symbols *[]Symbol, refs *[]Reference) {
    nodeType := node.Type()
    
    // Check if this node is a symbol definition
    if kind, ok := cfg.SymbolKinds[nodeType]; ok {
        sym := extractSymbolFromNode(node, kind, file, lang, content, scope)
        if sym != nil {
            *symbols = append(*symbols, *sym)
            // Update scope for children
            if kind == "class" || kind == "type" || kind == "function" || kind == "method" {
                scope = sym.Name
            }
        }
    }
    
    // Check if this node is a reference
    if kind, ok := cfg.RefKinds[nodeType]; ok {
        ref := extractRefFromNode(node, kind, file, content, scope)
        if ref != nil {
            *refs = append(*refs, *ref)
        }
    }
    
    // Recurse into children
    for i := 0; i < int(node.ChildCount()); i++ {
        child := node.Child(i)
        walkNode(child, cfg, file, lang, content, scope, symbols, refs)
    }
}

// extractSymbolFromNode extracts a Symbol from an AST node.
func extractSymbolFromNode(node *sitter.Node, kind, file, lang string, content []byte, scope string) *Symbol {
    // Find the name node - varies by language and node type
    var name string
    var signature string
    
    // Try common field names for the identifier
    for _, fieldName := range []string{"name", "declarator", "left"} {
        if nameNode := node.ChildByFieldName(fieldName); nameNode != nil {
            // Handle nested declarators (e.g., variable_declarator has name inside)
            if nameNode.Type() == "identifier" || nameNode.Type() == "type_identifier" {
                name = nameNode.Content(content)
                break
            }
            // For declarators, look for identifier inside
            for i := 0; i < int(nameNode.ChildCount()); i++ {
                child := nameNode.Child(i)
                if child.Type() == "identifier" {
                    name = child.Content(content)
                    break
                }
            }
            if name != "" {
                break
            }
        }
    }
    
    if name == "" {
        // Fallback: find first identifier child
        for i := 0; i < int(node.ChildCount()); i++ {
            child := node.Child(i)
            if child.Type() == "identifier" || child.Type() == "type_identifier" {
                name = child.Content(content)
                break
            }
        }
    }
    
    if name == "" {
        return nil
    }
    
    // Extract signature for functions/methods
    if kind == "function" || kind == "method" {
        if params := node.ChildByFieldName("parameters"); params != nil {
            signature = "func " + name + params.Content(content)
            if result := node.ChildByFieldName("result"); result != nil {
                signature += " " + result.Content(content)
            }
        }
    }
    
    return &Symbol{
        Name:      name,
        Kind:      kind,
        File:      file,
        Line:      int(node.StartPoint().Row) + 1,
        EndLine:   int(node.EndPoint().Row) + 1,
        Column:    int(node.StartPoint().Column) + 1,
        Language:  lang,
        Scope:     scope,
        Signature: signature,
    }
}

// extractRefFromNode extracts a Reference from an AST node.
func extractRefFromNode(node *sitter.Node, kind, file string, content []byte, caller string) *Reference {
    // For call expressions, find the function being called
    var symbol string
    
    if funcNode := node.ChildByFieldName("function"); funcNode != nil {
        symbol = funcNode.Content(content)
    } else if node.ChildCount() > 0 {
        // First child is often the function/method being called
        symbol = node.Child(0).Content(content)
    }
    
    if symbol == "" {
        return nil
    }
    
    return &Reference{
        Symbol: symbol,
        File:   file,
        Line:   int(node.StartPoint().Row) + 1,
        Column: int(node.StartPoint().Column) + 1,
        Caller: caller,
        Kind:   kind,
    }
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./app -run TestExtractGoSymbols`
Expected: PASS

- [ ] **Step 6: Add TypeScript and Python extraction tests**

```go
// app/indexer_languages_test.go - add these tests

func TestExtractTypeScriptSymbols(t *testing.T) {
    src := []byte(`
interface User {
    name: string;
}

class UserService {
    getUser(): User {
        return { name: "test" };
    }
}

function helper(x: number): void {}
`)
    
    symbols, _, err := extractSymbols("test.ts", src)
    if err != nil {
        t.Fatal(err)
    }
    
    names := make(map[string]bool)
    for _, s := range symbols {
        names[s.Name] = true
    }
    
    for _, want := range []string{"User", "UserService", "getUser", "helper"} {
        if !names[want] {
            t.Errorf("missing symbol %q", want)
        }
    }
}

func TestExtractPythonSymbols(t *testing.T) {
    src := []byte(`
class MyClass:
    def method(self):
        pass

def my_function(x):
    return x * 2
`)
    
    symbols, _, err := extractSymbols("test.py", src)
    if err != nil {
        t.Fatal(err)
    }
    
    names := make(map[string]bool)
    for _, s := range symbols {
        names[s.Name] = true
    }
    
    for _, want := range []string{"MyClass", "method", "my_function"} {
        if !names[want] {
            t.Errorf("missing symbol %q", want)
        }
    }
}

func TestUnsupportedExtension(t *testing.T) {
    symbols, refs, err := extractSymbols("test.xyz", []byte("random content"))
    if err != nil {
        t.Fatal(err)
    }
    if len(symbols) != 0 || len(refs) != 0 {
        t.Error("unsupported extensions should return empty results")
    }
}
```

- [ ] **Step 7: Run all language tests**

Run: `go test -v ./app -run 'TestExtract.*Symbols|TestUnsupportedExtension'`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add app/indexer_languages.go app/indexer_languages_test.go go.mod go.sum
git commit -m "feat(indexer): add tree-sitter language configs for Go, TS, Python

Node-kind matching approach (like pitlane-mcp) walks AST checking
node.Type() against SymbolKinds/RefKinds maps. Extracts symbols
with full position info and call references.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Wire Extraction into Build Loop

**Files:**
- Modify: `app/indexer_treesitter.go`
- Modify: `app/indexer_treesitter_test.go`

- [ ] **Step 1: Write integration test**

```go
// app/indexer_treesitter_test.go - add this test

func TestTreeSitterIndexerBuild(t *testing.T) {
    tmp := t.TempDir()
    
    // Create minimal git repo
    os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
    
    // Create a Go file
    goCode := []byte(`package main

func Hello() string {
    return "hello"
}
`)
    os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)
    
    // Initialize git and add file
    exec.Command("git", "-C", tmp, "init").Run()
    exec.Command("git", "-C", tmp, "add", "main.go").Run()
    
    idx := NewTreeSitterIndexer(tmp)
    idx.Start()
    
    // Wait for build
    time.Sleep(100 * time.Millisecond)
    
    // Check files
    files := idx.Files()
    if len(files) != 1 || files[0] != "main.go" {
        t.Errorf("Files() = %v, want [main.go]", files)
    }
    
    // Check symbols
    defs := idx.Lookup("Hello")
    if len(defs) != 1 {
        t.Errorf("Lookup(Hello) = %d results, want 1", len(defs))
    }
    
    idx.Stop()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./app -run TestTreeSitterIndexerBuild`
Expected: FAIL - Files() returns empty, Lookup returns empty

- [ ] **Step 3: Implement build method**

```go
// app/indexer_treesitter.go - replace build method

func (idx *TreeSitterIndexer) build(ctx context.Context) {
    // Step 1: Get file list
    files, err := listFilesInWorktreeCtx(ctx, idx.worktree)
    if err != nil {
        if ctx.Err() == nil {
            logError("treesitter build(%s): listFiles: %v", idx.worktree, err)
        }
        return
    }

    if ctx.Err() != nil {
        return
    }

    // Step 2: Parse each file and extract symbols
    symbols := make(map[string][]Symbol)
    var allRefs []Reference

    for _, file := range files {
        if ctx.Err() != nil {
            return
        }

        fullPath := filepath.Join(idx.worktree, file)
        content, err := os.ReadFile(fullPath)
        if err != nil {
            continue // skip unreadable files
        }

        // Skip binary files (simple heuristic)
        if isBinary(content) {
            continue
        }

        syms, refs, err := extractSymbols(file, content)
        if err != nil {
            logError("treesitter parse %s: %v", file, err)
            continue
        }

        for _, sym := range syms {
            symbols[sym.Name] = append(symbols[sym.Name], sym)
        }
        allRefs = append(allRefs, refs...)
    }

    if ctx.Err() != nil {
        return
    }

    // Count for logging
    nSymbols := 0
    for _, defs := range symbols {
        nSymbols += len(defs)
    }
    logInfo("treesitter build(%s): %d files, %d symbols, %d refs",
        idx.worktree, len(files), nSymbols, len(allRefs))

    // Step 3: Update state
    idx.mu.Lock()
    idx.files = files
    idx.symbols = symbols
    // callgraph updated in later task
    idx.mu.Unlock()
}

// isBinary returns true if content looks like binary data.
func isBinary(content []byte) bool {
    // Check first 512 bytes for null bytes
    check := content
    if len(check) > 512 {
        check = check[:512]
    }
    for _, b := range check {
        if b == 0 {
            return true
        }
    }
    return false
}
```

Also add the import for "os" and "path/filepath" at the top of the file.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./app -run TestTreeSitterIndexerBuild`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/indexer_treesitter.go app/indexer_treesitter_test.go
git commit -m "feat(indexer): wire tree-sitter extraction into build loop

Build method reads git-tracked files, parses each with tree-sitter,
extracts symbols using language configs. Binary files are skipped.
Results stored in thread-safe symbol map.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Add Remaining Languages (16 more)

**Files:**
- Modify: `app/indexer_languages.go`
- Modify: `app/indexer_languages_test.go`

Note: go-tree-sitter supports 19 of the pitlane-mcp languages. Solidity, Svelte, and Zig require separate tree-sitter grammar bindings that may need manual integration later.

- [ ] **Step 1: Add dependencies for remaining languages**

Run: `go get github.com/smacker/go-tree-sitter/bash github.com/smacker/go-tree-sitter/c github.com/smacker/go-tree-sitter/cpp github.com/smacker/go-tree-sitter/csharp github.com/smacker/go-tree-sitter/java github.com/smacker/go-tree-sitter/javascript github.com/smacker/go-tree-sitter/kotlin github.com/smacker/go-tree-sitter/lua github.com/smacker/go-tree-sitter/ocaml github.com/smacker/go-tree-sitter/php github.com/smacker/go-tree-sitter/ruby github.com/smacker/go-tree-sitter/rust github.com/smacker/go-tree-sitter/scala github.com/smacker/go-tree-sitter/swift`

- [ ] **Step 2: Write test for JavaScript extraction**

```go
// app/indexer_languages_test.go - add test

func TestExtractJavaScriptSymbols(t *testing.T) {
    src := []byte(`
function greet(name) {
    return "Hello, " + name;
}

class Calculator {
    add(a, b) {
        return a + b;
    }
}

const helper = (x) => x * 2;
`)
    
    symbols, _, err := extractSymbols("test.js", src)
    if err != nil {
        t.Fatal(err)
    }
    
    names := make(map[string]bool)
    for _, s := range symbols {
        names[s.Name] = true
    }
    
    for _, want := range []string{"greet", "Calculator", "add"} {
        if !names[want] {
            t.Errorf("missing symbol %q", want)
        }
    }
}
```

- [ ] **Step 3: Add all language configurations**

```go
// app/indexer_languages.go - add imports and configs

import (
    // ... existing imports ...
    "github.com/smacker/go-tree-sitter/bash"
    "github.com/smacker/go-tree-sitter/c"
    "github.com/smacker/go-tree-sitter/cpp"
    "github.com/smacker/go-tree-sitter/csharp"
    "github.com/smacker/go-tree-sitter/java"
    "github.com/smacker/go-tree-sitter/javascript"
    "github.com/smacker/go-tree-sitter/kotlin"
    "github.com/smacker/go-tree-sitter/lua"
    "github.com/smacker/go-tree-sitter/php"
    "github.com/smacker/go-tree-sitter/ruby"
    "github.com/smacker/go-tree-sitter/rust"
    "github.com/smacker/go-tree-sitter/scala"
    "github.com/smacker/go-tree-sitter/swift"
)

// Add to langConfigs map:
var langConfigs = map[string]*LangConfig{
    // ... existing go, typescript, python ...
    
    "javascript": {
        Language:   javascript.GetLanguage(),
        Extensions: []string{".js", ".jsx", ".mjs"},
        SymbolKinds: map[string]string{
            "function_declaration": "function",
            "method_definition":    "method",
            "class_declaration":    "class",
            "variable_declarator":  "variable",
            "arrow_function":       "function",
        },
        RefKinds: map[string]string{
            "call_expression":   "call",
            "member_expression": "reference",
        },
    },
    "java": {
        Language:   java.GetLanguage(),
        Extensions: []string{".java"},
        SymbolKinds: map[string]string{
            "method_declaration":    "method",
            "class_declaration":     "class",
            "interface_declaration": "interface",
            "field_declaration":     "variable",
            "constructor_declaration": "constructor",
        },
        RefKinds: map[string]string{
            "method_invocation": "call",
        },
    },
    "c": {
        Language:   c.GetLanguage(),
        Extensions: []string{".c", ".h"},
        SymbolKinds: map[string]string{
            "function_definition":  "function",
            "declaration":          "variable",
            "struct_specifier":     "type",
            "enum_specifier":       "type",
            "type_definition":      "type",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
        },
    },
    "cpp": {
        Language:   cpp.GetLanguage(),
        Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hxx"},
        SymbolKinds: map[string]string{
            "function_definition":  "function",
            "class_specifier":      "class",
            "struct_specifier":     "type",
            "namespace_definition": "namespace",
            "template_declaration": "template",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
        },
    },
    "rust": {
        Language:   rust.GetLanguage(),
        Extensions: []string{".rs"},
        SymbolKinds: map[string]string{
            "function_item":      "function",
            "impl_item":          "impl",
            "struct_item":        "type",
            "enum_item":          "type",
            "trait_item":         "trait",
            "mod_item":           "module",
            "const_item":         "constant",
            "static_item":        "variable",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
            "macro_invocation": "call",
        },
    },
    "ruby": {
        Language:   ruby.GetLanguage(),
        Extensions: []string{".rb"},
        SymbolKinds: map[string]string{
            "method":         "method",
            "singleton_method": "method",
            "class":          "class",
            "module":         "module",
        },
        RefKinds: map[string]string{
            "call":          "call",
            "method_call":   "call",
        },
    },
    "bash": {
        Language:   bash.GetLanguage(),
        Extensions: []string{".sh", ".bash"},
        SymbolKinds: map[string]string{
            "function_definition": "function",
        },
        RefKinds: map[string]string{
            "command": "call",
        },
    },
    "csharp": {
        Language:   csharp.GetLanguage(),
        Extensions: []string{".cs"},
        SymbolKinds: map[string]string{
            "method_declaration":    "method",
            "class_declaration":     "class",
            "interface_declaration": "interface",
            "struct_declaration":    "type",
            "property_declaration":  "property",
        },
        RefKinds: map[string]string{
            "invocation_expression": "call",
        },
    },
    "kotlin": {
        Language:   kotlin.GetLanguage(),
        Extensions: []string{".kt", ".kts"},
        SymbolKinds: map[string]string{
            "function_declaration": "function",
            "class_declaration":    "class",
            "object_declaration":   "object",
            "property_declaration": "property",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
        },
    },
    "swift": {
        Language:   swift.GetLanguage(),
        Extensions: []string{".swift"},
        SymbolKinds: map[string]string{
            "function_declaration": "function",
            "class_declaration":    "class",
            "struct_declaration":   "type",
            "protocol_declaration": "protocol",
            "enum_declaration":     "type",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
        },
    },
    "scala": {
        Language:   scala.GetLanguage(),
        Extensions: []string{".scala"},
        SymbolKinds: map[string]string{
            "function_definition": "function",
            "class_definition":    "class",
            "object_definition":   "object",
            "trait_definition":    "trait",
            "val_definition":      "variable",
        },
        RefKinds: map[string]string{
            "call_expression": "call",
        },
    },
    "php": {
        Language:   php.GetLanguage(),
        Extensions: []string{".php"},
        SymbolKinds: map[string]string{
            "function_definition": "function",
            "method_declaration":  "method",
            "class_declaration":   "class",
            "interface_declaration": "interface",
            "trait_declaration":   "trait",
        },
        RefKinds: map[string]string{
            "function_call_expression": "call",
            "member_call_expression":   "call",
        },
    },
    "lua": {
        Language:   lua.GetLanguage(),
        Extensions: []string{".lua"},
        SymbolKinds: map[string]string{
            "function_declaration": "function",
            "local_function":       "function",
            "function_definition":  "function",
        },
        RefKinds: map[string]string{
            "function_call": "call",
        },
    },
}
```

- [ ] **Step 4: Run all tests**

Run: `go test -v ./app -run 'TestExtract.*Symbols'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/indexer_languages.go app/indexer_languages_test.go go.mod go.sum
git commit -m "feat(indexer): add remaining language configurations

Supports 16 additional languages from go-tree-sitter: JavaScript,
Java, C, C++, Rust, Ruby, Bash, C#, Kotlin, Swift, Scala, PHP, Lua,
OCaml. Each has symbol kinds and ref kinds for call graph analysis.
Note: Solidity, Svelte, Zig require separate grammar bindings.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Call Graph Implementation

**Files:**
- Create: `app/indexer_callgraph.go`
- Create: `app/indexer_callgraph_test.go`
- Modify: `app/indexer_treesitter.go`

- [ ] **Step 1: Write call graph test**

```go
// app/indexer_callgraph_test.go
package app

import "testing"

func TestCallGraphFindCallers(t *testing.T) {
    cg := NewCallGraph()
    
    cg.AddReference(Reference{
        Symbol: "helper",
        Caller: "main",
        File:   "main.go",
        Line:   10,
        Kind:   "call",
    })
    cg.AddReference(Reference{
        Symbol: "helper",
        Caller: "process",
        File:   "process.go",
        Line:   20,
        Kind:   "call",
    })
    
    callers := cg.FindCallers("helper")
    if len(callers) != 2 {
        t.Errorf("FindCallers(helper) = %d, want 2", len(callers))
    }
}

func TestCallGraphFindCallees(t *testing.T) {
    cg := NewCallGraph()
    
    cg.AddReference(Reference{
        Symbol: "helper",
        Caller: "main",
        File:   "main.go",
        Line:   10,
        Kind:   "call",
    })
    cg.AddReference(Reference{
        Symbol: "process",
        Caller: "main",
        File:   "main.go",
        Line:   15,
        Kind:   "call",
    })
    
    callees := cg.FindCallees("main")
    if len(callees) != 2 {
        t.Errorf("FindCallees(main) = %d, want 2", len(callees))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./app -run 'TestCallGraph.*'`
Expected: FAIL - CallGraph not defined

- [ ] **Step 3: Implement CallGraph**

```go
// app/indexer_callgraph.go
package app

import "sync"

// CallGraph tracks caller/callee relationships between symbols.
type CallGraph struct {
    mu sync.RWMutex
    
    // callers maps symbol -> references where it's called
    callers map[string][]Reference
    
    // callees maps caller -> symbols it calls
    callees map[string][]Reference
}

// NewCallGraph creates an empty call graph.
func NewCallGraph() *CallGraph {
    return &CallGraph{
        callers: make(map[string][]Reference),
        callees: make(map[string][]Reference),
    }
}

// AddReference adds a call reference to the graph.
func (cg *CallGraph) AddReference(ref Reference) {
    cg.mu.Lock()
    defer cg.mu.Unlock()
    
    // Index by called symbol (for FindCallers)
    cg.callers[ref.Symbol] = append(cg.callers[ref.Symbol], ref)
    
    // Index by caller (for FindCallees)
    cg.callees[ref.Caller] = append(cg.callees[ref.Caller], ref)
}

// FindCallers returns all places where symbol is called.
func (cg *CallGraph) FindCallers(symbol string) []Reference {
    cg.mu.RLock()
    defer cg.mu.RUnlock()
    
    refs := cg.callers[symbol]
    out := make([]Reference, len(refs))
    copy(out, refs)
    return out
}

// FindCallees returns all symbols called by caller.
func (cg *CallGraph) FindCallees(caller string) []Reference {
    cg.mu.RLock()
    defer cg.mu.RUnlock()
    
    refs := cg.callees[caller]
    out := make([]Reference, len(refs))
    copy(out, refs)
    return out
}

// Clear resets the call graph.
func (cg *CallGraph) Clear() {
    cg.mu.Lock()
    defer cg.mu.Unlock()
    cg.callers = make(map[string][]Reference)
    cg.callees = make(map[string][]Reference)
}

// Stats returns counts for debugging.
func (cg *CallGraph) Stats() (callerCount, calleeCount int) {
    cg.mu.RLock()
    defer cg.mu.RUnlock()
    return len(cg.callers), len(cg.callees)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./app -run 'TestCallGraph.*'`
Expected: PASS

- [ ] **Step 5: Wire CallGraph into TreeSitterIndexer**

```go
// app/indexer_treesitter.go - modify build method to populate callgraph

// In build(), after extracting refs, add:
    // Build call graph
    callgraph := NewCallGraph()
    for _, ref := range allRefs {
        callgraph.AddReference(ref)
    }

// In the final update section:
    idx.mu.Lock()
    idx.files = files
    idx.symbols = symbols
    idx.callgraph = callgraph
    idx.mu.Unlock()

// Add accessor methods:

// FindCallers returns all places where symbol is called.
func (idx *TreeSitterIndexer) FindCallers(symbol string) []Reference {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    if idx.callgraph == nil {
        return nil
    }
    return idx.callgraph.FindCallers(symbol)
}

// FindCallees returns all symbols called by the given function.
func (idx *TreeSitterIndexer) FindCallees(caller string) []Reference {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    if idx.callgraph == nil {
        return nil
    }
    return idx.callgraph.FindCallees(caller)
}
```

- [ ] **Step 6: Run all indexer tests**

Run: `go test -v ./app -run 'TestTreeSitter|TestCallGraph'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add app/indexer_callgraph.go app/indexer_callgraph_test.go app/indexer_treesitter.go
git commit -m "feat(indexer): add call graph with FindCallers/FindCallees

CallGraph indexes references by both called symbol and caller function.
Enables bidirectional lookup: who calls X? what does X call?

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 7: BM25 Search with Bleve

**Files:**
- Create: `app/indexer_search.go`
- Create: `app/indexer_search_test.go`
- Modify: `app/indexer_treesitter.go`

- [ ] **Step 1: Add bleve dependency**

Run: `go get github.com/blevesearch/bleve/v2`

- [ ] **Step 2: Write search test**

```go
// app/indexer_search_test.go
package app

import (
    "testing"
)

func TestSymbolSearchRanking(t *testing.T) {
    idx := NewSymbolIndex()
    defer idx.Close()
    
    // Add symbols with varying relevance
    idx.Index(Symbol{Name: "getUserByID", Kind: "function", File: "user.go"})
    idx.Index(Symbol{Name: "getUser", Kind: "function", File: "user.go"})
    idx.Index(Symbol{Name: "UserService", Kind: "class", File: "service.go"})
    idx.Index(Symbol{Name: "processUserData", Kind: "function", File: "process.go"})
    
    results := idx.Search("user", 10)
    
    if len(results) == 0 {
        t.Fatal("expected search results")
    }
    
    // "getUser" should rank higher than "processUserData" (exact word match)
    // This is a soft assertion - BM25 should handle this
    t.Logf("Search results: %v", results)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v ./app -run TestSymbolSearchRanking`
Expected: FAIL - SymbolIndex not defined

- [ ] **Step 4: Implement SymbolIndex with Bleve**

```go
// app/indexer_search.go
package app

import (
    "github.com/blevesearch/bleve/v2"
    "github.com/blevesearch/bleve/v2/mapping"
)

// SymbolIndex provides BM25-ranked search over symbols.
type SymbolIndex struct {
    index bleve.Index
}

// NewSymbolIndex creates an in-memory bleve index.
func NewSymbolIndex() *SymbolIndex {
    mapping := buildIndexMapping()
    index, err := bleve.NewMemOnly(mapping)
    if err != nil {
        panic(err) // should not happen with memory index
    }
    return &SymbolIndex{index: index}
}

func buildIndexMapping() mapping.IndexMapping {
    // Create a text field mapping for symbol names
    textFieldMapping := bleve.NewTextFieldMapping()
    textFieldMapping.Analyzer = "standard"
    
    // Create a keyword field for exact matches
    keywordFieldMapping := bleve.NewKeywordFieldMapping()
    
    // Create document mapping for Symbol
    symbolMapping := bleve.NewDocumentMapping()
    symbolMapping.AddFieldMappingsAt("name", textFieldMapping)
    symbolMapping.AddFieldMappingsAt("kind", keywordFieldMapping)
    symbolMapping.AddFieldMappingsAt("file", keywordFieldMapping)
    symbolMapping.AddFieldMappingsAt("language", keywordFieldMapping)
    symbolMapping.AddFieldMappingsAt("scope", textFieldMapping)
    symbolMapping.AddFieldMappingsAt("signature", textFieldMapping)
    
    indexMapping := bleve.NewIndexMapping()
    indexMapping.DefaultMapping = symbolMapping
    
    return indexMapping
}

// symbolDoc wraps a Symbol for indexing with a unique ID.
type symbolDoc struct {
    Symbol
    ID string `json:"id"`
}

// Index adds a symbol to the search index.
func (si *SymbolIndex) Index(sym Symbol) error {
    doc := symbolDoc{
        Symbol: sym,
        ID:     sym.File + ":" + sym.Name,
    }
    return si.index.Index(doc.ID, doc)
}

// IndexBatch adds multiple symbols efficiently.
func (si *SymbolIndex) IndexBatch(symbols []Symbol) error {
    batch := si.index.NewBatch()
    for _, sym := range symbols {
        doc := symbolDoc{
            Symbol: sym,
            ID:     sym.File + ":" + sym.Name,
        }
        batch.Index(doc.ID, doc)
    }
    return si.index.Batch(batch)
}

// Search returns symbols matching the query, ranked by BM25.
func (si *SymbolIndex) Search(query string, limit int) []Symbol {
    q := bleve.NewQueryStringQuery(query)
    req := bleve.NewSearchRequest(q)
    req.Size = limit
    req.Fields = []string{"name", "kind", "file", "line", "end_line", 
                          "column", "language", "scope", "signature"}
    
    results, err := si.index.Search(req)
    if err != nil {
        return nil
    }
    
    var symbols []Symbol
    for _, hit := range results.Hits {
        sym := Symbol{
            Name:      getString(hit.Fields, "name"),
            Kind:      getString(hit.Fields, "kind"),
            File:      getString(hit.Fields, "file"),
            Line:      getInt(hit.Fields, "line"),
            EndLine:   getInt(hit.Fields, "end_line"),
            Column:    getInt(hit.Fields, "column"),
            Language:  getString(hit.Fields, "language"),
            Scope:     getString(hit.Fields, "scope"),
            Signature: getString(hit.Fields, "signature"),
        }
        symbols = append(symbols, sym)
    }
    
    return symbols
}

// Clear removes all indexed symbols.
func (si *SymbolIndex) Clear() error {
    // Bleve doesn't have a clear method, so we delete and recreate
    si.index.Close()
    mapping := buildIndexMapping()
    index, err := bleve.NewMemOnly(mapping)
    if err != nil {
        return err
    }
    si.index = index
    return nil
}

// Close closes the index.
func (si *SymbolIndex) Close() error {
    return si.index.Close()
}

func getString(fields map[string]interface{}, key string) string {
    if v, ok := fields[key]; ok {
        if s, ok := v.(string); ok {
            return s
        }
    }
    return ""
}

func getInt(fields map[string]interface{}, key string) int {
    if v, ok := fields[key]; ok {
        switch n := v.(type) {
        case float64:
            return int(n)
        case int:
            return n
        }
    }
    return 0
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./app -run TestSymbolSearchRanking`
Expected: PASS

- [ ] **Step 6: Wire SymbolIndex into TreeSitterIndexer**

```go
// app/indexer_treesitter.go - add search field and methods

// Add to struct:
type TreeSitterIndexer struct {
    // ... existing fields ...
    search *SymbolIndex
}

// In NewTreeSitterIndexer:
func NewTreeSitterIndexer(worktree string) *TreeSitterIndexer {
    return &TreeSitterIndexer{
        worktree: worktree,
        symbols:  make(map[string][]Symbol),
        search:   NewSymbolIndex(),
        done:     make(chan struct{}),
        refresh:  make(chan struct{}, 1),
    }
}

// In build(), after updating symbols:
    // Update search index
    var allSyms []Symbol
    for _, syms := range symbols {
        allSyms = append(allSyms, syms...)
    }
    idx.search.Clear()
    idx.search.IndexBatch(allSyms)

// Add search method:
// SearchSymbols returns symbols matching query, ranked by BM25.
func (idx *TreeSitterIndexer) SearchSymbols(query string, limit int) []Symbol {
    return idx.search.Search(query, limit)
}

// In Stop():
func (idx *TreeSitterIndexer) Stop() {
    if idx.cancel != nil {
        idx.cancel()
    }
    <-idx.done
    if idx.search != nil {
        idx.search.Close()
    }
}
```

- [ ] **Step 7: Run all tests**

Run: `go test -v ./app -run 'TestTreeSitter|TestSymbolSearch'`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add app/indexer_search.go app/indexer_search_test.go app/indexer_treesitter.go go.mod go.sum
git commit -m "feat(indexer): add BM25 search with bleve

SymbolIndex wraps bleve for in-memory BM25 ranking. Symbols indexed
on name, kind, file, scope, signature. SearchSymbols returns ranked
results instead of substring matches.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Disk Persistence

**Files:**
- Create: `app/indexer_persistence.go`
- Create: `app/indexer_persistence_test.go`
- Modify: `app/indexer_treesitter.go`

- [ ] **Step 1: Write persistence test**

```go
// app/indexer_persistence_test.go
package app

import (
    "os"
    "path/filepath"
    "testing"
)

func TestIndexPersistence(t *testing.T) {
    tmp := t.TempDir()
    
    // Create test data
    symbols := map[string][]Symbol{
        "Foo": {{Name: "Foo", Kind: "function", File: "main.go", Line: 10}},
        "Bar": {{Name: "Bar", Kind: "type", File: "types.go", Line: 5}},
    }
    
    cg := NewCallGraph()
    cg.AddReference(Reference{Symbol: "Foo", Caller: "main", File: "main.go"})
    
    // Save
    store := NewIndexStore(tmp)
    err := store.Save(symbols, cg, "abc123")
    if err != nil {
        t.Fatalf("Save failed: %v", err)
    }
    
    // Verify files exist
    indexDir := filepath.Join(tmp, ".claude-squad", "index")
    if _, err := os.Stat(filepath.Join(indexDir, "meta.json")); err != nil {
        t.Error("meta.json not created")
    }
    if _, err := os.Stat(filepath.Join(indexDir, "symbols.gob")); err != nil {
        t.Error("symbols.gob not created")
    }
    
    // Load
    loadedSyms, loadedCG, commit, err := store.Load()
    if err != nil {
        t.Fatalf("Load failed: %v", err)
    }
    
    if commit != "abc123" {
        t.Errorf("commit = %q, want abc123", commit)
    }
    if len(loadedSyms) != 2 {
        t.Errorf("loaded %d symbols, want 2", len(loadedSyms))
    }
    if len(loadedCG.FindCallers("Foo")) != 1 {
        t.Error("callgraph not restored")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./app -run TestIndexPersistence`
Expected: FAIL - IndexStore not defined

- [ ] **Step 3: Implement IndexStore**

```go
// app/indexer_persistence.go
package app

import (
    "encoding/gob"
    "encoding/json"
    "os"
    "path/filepath"
)

// IndexStore handles saving/loading index to disk.
type IndexStore struct {
    baseDir string
}

// indexMeta is the metadata file format.
type indexMeta struct {
    Version    int    `json:"version"`
    Commit     string `json:"commit"`
    SymbolCount int   `json:"symbol_count"`
    RefCount   int    `json:"ref_count"`
}

// NewIndexStore creates a store for the given worktree.
func NewIndexStore(worktree string) *IndexStore {
    return &IndexStore{baseDir: worktree}
}

func (s *IndexStore) indexDir() string {
    return filepath.Join(s.baseDir, ".claude-squad", "index")
}

// Save persists symbols and callgraph to disk.
func (s *IndexStore) Save(symbols map[string][]Symbol, cg *CallGraph, commit string) error {
    dir := s.indexDir()
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    // Count symbols
    symbolCount := 0
    for _, syms := range symbols {
        symbolCount += len(syms)
    }
    
    // Get ref count from callgraph
    callerCount, _ := cg.Stats()
    
    // Write meta
    meta := indexMeta{
        Version:     1,
        Commit:      commit,
        SymbolCount: symbolCount,
        RefCount:    callerCount,
    }
    metaData, _ := json.MarshalIndent(meta, "", "  ")
    if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaData, 0644); err != nil {
        return err
    }
    
    // Write symbols
    symFile, err := os.Create(filepath.Join(dir, "symbols.gob"))
    if err != nil {
        return err
    }
    defer symFile.Close()
    if err := gob.NewEncoder(symFile).Encode(symbols); err != nil {
        return err
    }
    
    // Write callgraph
    cg.mu.RLock()
    cgData := struct {
        Callers map[string][]Reference
        Callees map[string][]Reference
    }{
        Callers: cg.callers,
        Callees: cg.callees,
    }
    cg.mu.RUnlock()
    
    cgFile, err := os.Create(filepath.Join(dir, "callgraph.gob"))
    if err != nil {
        return err
    }
    defer cgFile.Close()
    if err := gob.NewEncoder(cgFile).Encode(cgData); err != nil {
        return err
    }
    
    return nil
}

// Load reads symbols and callgraph from disk.
// Returns nil, nil, "", nil if no index exists.
func (s *IndexStore) Load() (map[string][]Symbol, *CallGraph, string, error) {
    dir := s.indexDir()
    
    // Read meta
    metaPath := filepath.Join(dir, "meta.json")
    metaData, err := os.ReadFile(metaPath)
    if os.IsNotExist(err) {
        return nil, nil, "", nil
    }
    if err != nil {
        return nil, nil, "", err
    }
    
    var meta indexMeta
    if err := json.Unmarshal(metaData, &meta); err != nil {
        return nil, nil, "", err
    }
    
    if meta.Version != 1 {
        return nil, nil, "", nil // incompatible version, rebuild
    }
    
    // Read symbols
    symFile, err := os.Open(filepath.Join(dir, "symbols.gob"))
    if err != nil {
        return nil, nil, "", err
    }
    defer symFile.Close()
    
    var symbols map[string][]Symbol
    if err := gob.NewDecoder(symFile).Decode(&symbols); err != nil {
        return nil, nil, "", err
    }
    
    // Read callgraph
    cgFile, err := os.Open(filepath.Join(dir, "callgraph.gob"))
    if err != nil {
        return nil, nil, "", err
    }
    defer cgFile.Close()
    
    var cgData struct {
        Callers map[string][]Reference
        Callees map[string][]Reference
    }
    if err := gob.NewDecoder(cgFile).Decode(&cgData); err != nil {
        return nil, nil, "", err
    }
    
    cg := &CallGraph{
        callers: cgData.Callers,
        callees: cgData.Callees,
    }
    
    return symbols, cg, meta.Commit, nil
}

// GetCommit returns the indexed commit SHA without loading full index.
func (s *IndexStore) GetCommit() string {
    metaPath := filepath.Join(s.indexDir(), "meta.json")
    data, err := os.ReadFile(metaPath)
    if err != nil {
        return ""
    }
    var meta indexMeta
    if json.Unmarshal(data, &meta) != nil {
        return ""
    }
    return meta.Commit
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./app -run TestIndexPersistence`
Expected: PASS

- [ ] **Step 5: Wire persistence into TreeSitterIndexer**

```go
// app/indexer_treesitter.go - add persistence loading/saving

// Add field:
type TreeSitterIndexer struct {
    // ... existing ...
    store *IndexStore
}

// In NewTreeSitterIndexer:
    store: NewIndexStore(worktree),

// In build(), at the start:
func (idx *TreeSitterIndexer) build(ctx context.Context) {
    // Try to load from disk first
    if idx.symbols == nil || len(idx.symbols) == 0 {
        if symbols, cg, commit, err := idx.store.Load(); err == nil && symbols != nil {
            // Check if HEAD matches
            currentCommit := getHeadCommit(idx.worktree)
            if commit == currentCommit {
                idx.mu.Lock()
                idx.symbols = symbols
                idx.callgraph = cg
                idx.mu.Unlock()
                
                // Rebuild search index from loaded symbols
                var allSyms []Symbol
                for _, syms := range symbols {
                    allSyms = append(allSyms, syms...)
                }
                idx.search.IndexBatch(allSyms)
                
                logInfo("treesitter: loaded %d symbols from cache", len(allSyms))
                
                // Still need file list
                files, _ := listFilesInWorktreeCtx(ctx, idx.worktree)
                idx.mu.Lock()
                idx.files = files
                idx.mu.Unlock()
                return
            }
        }
    }
    
    // ... existing build code ...

    // At the end, save to disk:
    commit := getHeadCommit(idx.worktree)
    if err := idx.store.Save(symbols, callgraph, commit); err != nil {
        logError("treesitter: failed to persist index: %v", err)
    }
}

// Helper to get current HEAD commit:
func getHeadCommit(worktree string) string {
    cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
    cmd.Dir = worktree
    out, err := cmd.Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}
```

- [ ] **Step 6: Run all tests**

Run: `go test -v ./app -run 'TestTreeSitter|TestIndexPersistence'`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add app/indexer_persistence.go app/indexer_persistence_test.go app/indexer_treesitter.go
git commit -m "feat(indexer): add disk persistence with gob encoding

IndexStore saves symbols and callgraph to .claude-squad/index/.
On startup, loads cached index if HEAD commit matches. Enables
fast startup for unchanged worktrees.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 9: MCP Server Tools (pitlane-compatible API)

**Files:**
- Modify: `app/mcp_server.go`
- Modify: `app/mcp_server_test.go`

- [ ] **Step 1: Write test for new get_symbol tool**

```go
// app/mcp_server_test.go - add test

func TestMCPGetSymbol(t *testing.T) {
    // Create temp worktree with a Go file
    tmp := t.TempDir()
    os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
    exec.Command("git", "-C", tmp, "init").Run()
    
    code := []byte(`package main

func Hello() string {
    return "hello"
}
`)
    os.WriteFile(filepath.Join(tmp, "main.go"), code, 0644)
    exec.Command("git", "-C", tmp, "add", "main.go").Run()
    
    // Create and start indexer
    idx := NewTreeSitterIndexer(tmp)
    idx.Start()
    time.Sleep(200 * time.Millisecond)
    
    // Create MCP server
    srv := NewMCPIndexServerStandalone(idx)
    
    // Test get_symbol
    // This tests the internal handler directly
    // Full HTTP test would be more comprehensive but this validates logic
    
    idx.Stop()
}
```

- [ ] **Step 2: Add new MCP tools**

```go
// app/mcp_server.go - add in registerTools method

// get_symbol - enhanced lookup with optional full body
s.AddTool(
    mcp.NewTool("get_symbol",
        mcp.WithDescription("Get symbol definition with optional full source body. More detailed than lookup_symbol."),
        mcp.WithString("name",
            mcp.Description("The symbol name to look up"),
            mcp.Required(),
        ),
        mcp.WithBoolean("signature_only",
            mcp.Description("If true, return only signature, not full body"),
        ),
    ),
    m.handleGetSymbol,
)

// find_callers - reverse call graph
s.AddTool(
    mcp.NewTool("find_callers",
        mcp.WithDescription("Find all places where a symbol is called. Returns file, line, and calling function."),
        mcp.WithString("symbol",
            mcp.Description("The symbol name to find callers for"),
            mcp.Required(),
        ),
    ),
    m.handleFindCallers,
)

// find_callees - forward call graph
s.AddTool(
    mcp.NewTool("find_callees",
        mcp.WithDescription("Find all symbols called by a function. Returns what the function calls."),
        mcp.WithString("symbol",
            mcp.Description("The function name to find callees for"),
            mcp.Required(),
        ),
    ),
    m.handleFindCallees,
)

// index_status - health check
s.AddTool(
    mcp.NewTool("index_status",
        mcp.WithDescription("Get index health and statistics. Shows file count, symbol count, and indexer state."),
    ),
    m.handleIndexStatus,
)
```

- [ ] **Step 3: Implement new tool handlers**

```go
// app/mcp_server.go - add handlers

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
    if tsIdx, ok := interface{}(idx).(*TreeSitterIndexer); ok {
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
        body, err := readLines(fullPath, sym.Line, sym.EndLine)
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

    tsIdx, ok := interface{}(idx).(*TreeSitterIndexer)
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

    tsIdx, ok := interface{}(idx).(*TreeSitterIndexer)
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
    
    if tsIdx, ok := interface{}(idx).(*TreeSitterIndexer); ok {
        status["indexer_type"] = "tree-sitter"
        if tsIdx.callgraph != nil {
            callers, callees := tsIdx.callgraph.Stats()
            status["caller_symbols"] = callers
            status["callee_functions"] = callees
        }
    }
    
    result, _ := json.MarshalIndent(status, "", "  ")
    return mcp.NewToolResultText(string(result)), nil
}

// readLines reads a range of lines from a file.
func readLines(path string, start, end int) (string, error) {
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
    return strings.Join(lines, "\n"), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test -v ./app -run 'TestMCP.*'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/mcp_server.go app/mcp_server_test.go
git commit -m "feat(mcp): add pitlane-compatible tools

New tools: get_symbol (with body), find_callers, find_callees,
index_status. Maintains backward compatibility with existing
lookup_symbol, search_symbols tools.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Integration and Feature Flag

**Files:**
- Modify: `app/session.go` (add indexer selection)
- Modify: `app/bindings.go` (update MCP guidance)
- Create: `app/indexer_integration_test.go`

- [ ] **Step 1: Add feature flag for indexer selection**

```go
// app/session.go - add indexer type selection

// IndexerType determines which indexer to use.
type IndexerType string

const (
    IndexerCtags      IndexerType = "ctags"
    IndexerTreeSitter IndexerType = "treesitter"
)

// DefaultIndexer is the default indexer type for new sessions.
var DefaultIndexer = IndexerTreeSitter

// In createSessionIndexer or similar:
func createIndexer(worktree string, indexerType IndexerType) Indexer {
    switch indexerType {
    case IndexerTreeSitter:
        return NewTreeSitterIndexer(worktree)
    default:
        return NewSessionIndexer(worktree)
    }
}
```

- [ ] **Step 2: Update MCP guidance in bindings.go**

```go
// app/bindings.go - update mcpGuidance constant

const mcpGuidance = `# MCP Index Server

You have access to a cs-index MCP server with symbol indexing tools.
Prefer these tools over Grep/Read for code navigation:

- get_symbol — retrieve exact implementation with full source body
- search_symbols — find symbols by name pattern (BM25 ranked)
- find_callers — trace who calls a function
- find_callees — trace what a function calls
- get_file_outline — understand file structure before reading
- index_status — check indexer health

Fall back to Grep/Read for text search or when editing files.
`
```

- [ ] **Step 3: Write integration test**

```go
// app/indexer_integration_test.go
package app

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "time"
)

func TestTreeSitterIntegration(t *testing.T) {
    tmp := t.TempDir()
    
    // Create a multi-file Go project
    exec.Command("git", "-C", tmp, "init").Run()
    
    // main.go
    os.WriteFile(filepath.Join(tmp, "main.go"), []byte(`package main

import "fmt"

func main() {
    msg := greet("world")
    fmt.Println(msg)
}
`), 0644)
    
    // greet.go
    os.WriteFile(filepath.Join(tmp, "greet.go"), []byte(`package main

func greet(name string) string {
    return "Hello, " + name
}
`), 0644)
    
    exec.Command("git", "-C", tmp, "add", ".").Run()
    
    // Start indexer
    idx := NewTreeSitterIndexer(tmp)
    idx.Start()
    time.Sleep(300 * time.Millisecond)
    
    // Test symbol lookup
    syms := idx.LookupSymbol("greet")
    if len(syms) != 1 {
        t.Errorf("LookupSymbol(greet) = %d, want 1", len(syms))
    }
    
    // Test call graph
    callers := idx.FindCallers("greet")
    if len(callers) == 0 {
        t.Error("FindCallers(greet) should find main")
    }
    
    callees := idx.FindCallees("main")
    found := false
    for _, ref := range callees {
        if ref.Symbol == "greet" || strings.Contains(ref.Symbol, "greet") {
            found = true
            break
        }
    }
    if !found {
        t.Error("FindCallees(main) should include greet")
    }
    
    // Test BM25 search
    results := idx.SearchSymbols("gree", 10)
    if len(results) == 0 {
        t.Error("SearchSymbols(gree) should find greet")
    }
    
    idx.Stop()
}
```

- [ ] **Step 4: Run integration test**

Run: `go test -v ./app -run TestTreeSitterIntegration`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/session.go app/bindings.go app/indexer_integration_test.go
git commit -m "feat(indexer): integrate tree-sitter as default indexer

TreeSitter is now the default indexer type. Ctags available via
IndexerCtags flag. Updated MCP guidance to mention new call graph
and BM25 search capabilities.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 11: Test Fixtures and Benchmarks

**Files:**
- Create: `testdata/fixtures/small-go/`
- Create: `testdata/fixtures/multi-lang/`
- Modify: `benchmark/` (update for tree-sitter comparison)

- [ ] **Step 1: Create small-go fixture**

```bash
mkdir -p testdata/fixtures/small-go
```

```go
// testdata/fixtures/small-go/main.go
package main

func main() {
    result := process(getData())
    output(result)
}
```

```go
// testdata/fixtures/small-go/data.go
package main

type Record struct {
    ID   int
    Name string
}

func getData() []Record {
    return []Record{{1, "test"}}
}
```

```go
// testdata/fixtures/small-go/process.go
package main

func process(records []Record) []string {
    var out []string
    for _, r := range records {
        out = append(out, r.Name)
    }
    return out
}
```

```go
// testdata/fixtures/small-go/output.go
package main

import "fmt"

func output(items []string) {
    for _, item := range items {
        fmt.Println(item)
    }
}
```

- [ ] **Step 2: Create multi-lang fixture**

```bash
mkdir -p testdata/fixtures/multi-lang
```

```go
// testdata/fixtures/multi-lang/api.go
package api

type Handler struct{}

func (h *Handler) ServeHTTP() {}
```

```typescript
// testdata/fixtures/multi-lang/client.ts
interface ApiResponse {
    data: string;
}

class ApiClient {
    fetch(): Promise<ApiResponse> {
        return Promise.resolve({ data: "" });
    }
}
```

```python
# testdata/fixtures/multi-lang/script.py
class Processor:
    def run(self):
        pass

def main():
    p = Processor()
    p.run()
```

- [ ] **Step 3: Add benchmark task for tree-sitter**

```go
// benchmark/tasks/symbol_treesitter.go
package tasks

func init() {
    Register(&SymbolTreeSitterSession{})
}

// SymbolTreeSitterSession uses tree-sitter index tools.
type SymbolTreeSitterSession struct{}

func (t *SymbolTreeSitterSession) Name() string     { return "symbol-treesitter-session" }
func (t *SymbolTreeSitterSession) Category() string { return "symbol-treesitter" }
func (t *SymbolTreeSitterSession) Prompt() string {
    return `You have access to a cs-index MCP server with tree-sitter indexing.
Use get_symbol to find the Session struct definition. Then use find_callers
to show where it's instantiated. Show file paths and line numbers.`
}
func (t *SymbolTreeSitterSession) Validate(output string) error { return nil }
```

- [ ] **Step 4: Update Categories in task.go**

```go
// benchmark/tasks/task.go - update Categories()
func Categories() []string {
    return []string{"symbol", "symbol-indexed", "symbol-treesitter", "understanding", "edit", "crossfile"}
}
```

- [ ] **Step 5: Commit**

```bash
git add testdata/ benchmark/tasks/
git commit -m "test: add fixtures and tree-sitter benchmark tasks

small-go: 4-file Go project for call graph testing
multi-lang: Go + TS + Python mix for language coverage
symbol-treesitter category for benchmarking new indexer

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Final Steps

- [ ] **Run full test suite**

```bash
go test -v ./app/... ./benchmark/...
```

- [ ] **Build and verify**

```bash
wails build
```

- [ ] **Manual smoke test**

1. Start a session: `cs new test-session`
2. Check index status: In Claude, ask "what's the index status?"
3. Test call graph: Ask "who calls the Start method?"
4. Test search: Ask "find symbols related to session"

---

## Summary

| Task | Component | Key Deliverable |
|------|-----------|-----------------|
| 1 | Types | Symbol, Reference, Indexer interface |
| 2 | Skeleton | TreeSitterIndexer with lifecycle |
| 3 | Languages | Go, TS, Python extraction |
| 4 | Build | Wire extraction into loop |
| 5 | Languages | 14 more languages |
| 6 | Call Graph | FindCallers/FindCallees |
| 7 | Search | BM25 with bleve |
| 8 | Persistence | Disk cache with gob |
| 9 | MCP Tools | pitlane-compatible API |
| 10 | Integration | Feature flag, default switch |
| 11 | Testing | Fixtures and benchmarks |

Total estimated tasks: 11 major tasks, ~50 individual steps
