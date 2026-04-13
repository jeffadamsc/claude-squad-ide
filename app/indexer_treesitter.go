package app

import (
	"context"
	"os"
	"path/filepath"
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

	// Build call graph
	callgraph := NewCallGraph()
	for _, ref := range allRefs {
		callgraph.AddReference(ref)
	}

	// Step 3: Update state
	idx.mu.Lock()
	idx.files = files
	idx.symbols = symbols
	idx.callgraph = callgraph
	idx.mu.Unlock()
}

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

