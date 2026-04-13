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
