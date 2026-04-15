package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TreeSitterIndexer provides tree-sitter based symbol indexing.
// Implements the Indexer interface with additional methods for
// call graph analysis and BM25 search.
type TreeSitterIndexer struct {
	worktree string

	mu           sync.RWMutex
	files        []string
	symbols      map[string][]Symbol
	fileSymbols  map[string][]Symbol   // symbols by file for incremental updates
	fileModTimes map[string]time.Time  // file -> last mod time for incremental indexing
	callgraph    *CallGraph
	search       *SymbolIndex
	store        *IndexStore

	cancel  context.CancelFunc
	done    chan struct{}
	refresh chan struct{}
}

// NewTreeSitterIndexer creates a new tree-sitter indexer for the worktree.
func NewTreeSitterIndexer(worktree string) *TreeSitterIndexer {
	return &TreeSitterIndexer{
		worktree:     worktree,
		symbols:      make(map[string][]Symbol),
		fileSymbols:  make(map[string][]Symbol),
		fileModTimes: make(map[string]time.Time),
		search:       NewSymbolIndex(),
		store:        NewIndexStore(worktree),
		done:         make(chan struct{}),
		refresh:      make(chan struct{}, 1),
	}
}

// getHeadCommit returns the current HEAD commit hash for the worktree.
func getHeadCommit(worktree string) string {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	if idx.search != nil {
		idx.search.Close()
	}
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
	// Try to load from disk first (only on initial build)
	idx.mu.RLock()
	hasSymbols := len(idx.symbols) > 0
	idx.mu.RUnlock()

	if !hasSymbols {
		if symbols, cg, commit, err := idx.store.Load(); err == nil && symbols != nil {
			currentCommit := getHeadCommit(idx.worktree)
			if commit == currentCommit {
				idx.mu.Lock()
				idx.symbols = symbols
				idx.callgraph = cg
				// Rebuild fileSymbols from loaded symbols
				idx.fileSymbols = make(map[string][]Symbol)
				for _, syms := range symbols {
					for _, sym := range syms {
						idx.fileSymbols[sym.File] = append(idx.fileSymbols[sym.File], sym)
					}
				}
				idx.mu.Unlock()

				var allSyms []Symbol
				for _, syms := range symbols {
					allSyms = append(allSyms, syms...)
				}
				idx.search.IndexBatch(allSyms)

				logInfo("treesitter: loaded %d symbols from cache", len(allSyms))

				files, _ := listFilesInWorktreeCtx(ctx, idx.worktree)
				idx.mu.Lock()
				idx.files = files
				// Initialize mod times for loaded files
				for _, f := range files {
					fullPath := filepath.Join(idx.worktree, f)
					if info, err := os.Stat(fullPath); err == nil {
						idx.fileModTimes[f] = info.ModTime()
					}
				}
				idx.mu.Unlock()
				return
			}
		}
	}

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

	// Step 2: Determine which files need re-indexing (incremental)
	idx.mu.RLock()
	oldModTimes := idx.fileModTimes
	oldFileSymbols := idx.fileSymbols
	idx.mu.RUnlock()

	var changedFiles []string
	var deletedFiles []string
	newModTimes := make(map[string]time.Time)
	currentFiles := make(map[string]bool)
	var existingFiles []string

	for _, file := range files {
		fullPath := filepath.Join(idx.worktree, file)
		info, err := os.Stat(fullPath)
		if err != nil {
			// File doesn't exist (deleted but still in git index)
			continue
		}
		existingFiles = append(existingFiles, file)
		currentFiles[file] = true
		modTime := info.ModTime()
		newModTimes[file] = modTime

		// Check if file changed
		if oldMod, ok := oldModTimes[file]; !ok || !modTime.Equal(oldMod) {
			changedFiles = append(changedFiles, file)
		}
	}
	files = existingFiles // Use only existing files

	// Find deleted files
	for file := range oldModTimes {
		if !currentFiles[file] {
			deletedFiles = append(deletedFiles, file)
		}
	}

	// If nothing changed and we have existing data, skip rebuild
	if len(changedFiles) == 0 && len(deletedFiles) == 0 && hasSymbols {
		idx.mu.Lock()
		idx.files = files
		idx.mu.Unlock()
		return
	}

	// Step 3: Parse changed files
	newFileSymbols := make(map[string][]Symbol)
	var newRefs []Reference

	// Copy unchanged file symbols
	for file, syms := range oldFileSymbols {
		if currentFiles[file] && !contains(changedFiles, file) {
			newFileSymbols[file] = syms
		}
	}

	// Parse changed files
	for _, file := range changedFiles {
		if ctx.Err() != nil {
			return
		}

		fullPath := filepath.Join(idx.worktree, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		if isBinary(content) {
			continue
		}

		syms, refs, err := extractSymbols(file, content)
		if err != nil {
			logError("treesitter parse %s: %v", file, err)
			continue
		}

		newFileSymbols[file] = syms
		newRefs = append(newRefs, refs...)
	}

	if ctx.Err() != nil {
		return
	}

	// Rebuild symbols map from fileSymbols
	symbols := make(map[string][]Symbol)
	var allRefs []Reference

	for _, syms := range newFileSymbols {
		for _, sym := range syms {
			symbols[sym.Name] = append(symbols[sym.Name], sym)
		}
	}

	// For call graph, we need all refs - re-extract from unchanged files too
	// (This is a trade-off: storing refs would use more memory)
	for file := range newFileSymbols {
		if !contains(changedFiles, file) {
			// Re-extract refs from unchanged file
			fullPath := filepath.Join(idx.worktree, file)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			_, refs, _ := extractSymbols(file, content)
			allRefs = append(allRefs, refs...)
		}
	}
	allRefs = append(allRefs, newRefs...)

	// Count for logging
	nSymbols := 0
	for _, defs := range symbols {
		nSymbols += len(defs)
	}
	logInfo("treesitter build(%s): %d files (%d changed, %d deleted), %d symbols",
		idx.worktree, len(files), len(changedFiles), len(deletedFiles), nSymbols)

	// Build call graph
	callgraph := NewCallGraph()
	for _, ref := range allRefs {
		callgraph.AddReference(ref)
	}

	// Update search index
	var allSyms []Symbol
	for _, syms := range symbols {
		allSyms = append(allSyms, syms...)
	}
	idx.search.Clear()
	idx.search.IndexBatch(allSyms)

	// Step 4: Update state
	idx.mu.Lock()
	idx.files = files
	idx.symbols = symbols
	idx.fileSymbols = newFileSymbols
	idx.fileModTimes = newModTimes
	idx.callgraph = callgraph
	idx.mu.Unlock()

	commit := getHeadCommit(idx.worktree)
	if err := idx.store.Save(symbols, callgraph, commit); err != nil {
		logError("treesitter: failed to persist index: %v", err)
	}
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// SearchSymbols returns symbols matching query, ranked by BM25.
func (idx *TreeSitterIndexer) SearchSymbols(query string, limit int) []Symbol {
	return idx.search.Search(query, limit)
}

// SearchResult holds a symbol with its content and token estimate.
type SearchResult struct {
	Symbol  Symbol `json:"symbol"`
	Content string `json:"content,omitempty"`
	Tokens  int    `json:"tokens"`
}

// SearchWithBudget returns symbols matching query, packed within a token budget.
// Returns results in relevance order (BM25) until the budget is exhausted.
// If includeContent is true, loads the symbol's source code.
func (idx *TreeSitterIndexer) SearchWithBudget(query string, maxTokens int, includeContent bool) []SearchResult {
	// Search with a generous limit first
	symbols := idx.search.Search(query, 100)
	if len(symbols) == 0 {
		return nil
	}

	var results []SearchResult
	usedTokens := 0

	for _, sym := range symbols {
		var content string
		var symTokens int

		if includeContent {
			// Load content using byte offsets
			c, err := idx.GetSymbolContent(sym)
			if err != nil {
				continue
			}
			content = c
			symTokens = EstimateTokens(content)
		} else {
			// Estimate tokens from signature or name
			if sym.Signature != "" {
				symTokens = EstimateTokens(sym.Signature)
			} else {
				symTokens = EstimateTokens(sym.Name) + 5 // overhead for metadata
			}
		}

		// Check if adding this symbol would exceed budget
		if usedTokens+symTokens > maxTokens {
			// If we haven't added anything yet, add at least one result
			if len(results) == 0 {
				results = append(results, SearchResult{
					Symbol:  sym,
					Content: content,
					Tokens:  symTokens,
				})
			}
			break
		}

		results = append(results, SearchResult{
			Symbol:  sym,
			Content: content,
			Tokens:  symTokens,
		})
		usedTokens += symTokens
	}

	return results
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

// GetCentrality returns centrality metrics for a symbol.
func (idx *TreeSitterIndexer) GetCentrality(symbol string) SymbolCentrality {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.callgraph == nil {
		return SymbolCentrality{Symbol: symbol}
	}
	return idx.callgraph.GetCentrality(symbol)
}

// TopSymbolsByCentrality returns the most important symbols by centrality score.
func (idx *TreeSitterIndexer) TopSymbolsByCentrality(limit int) []SymbolCentrality {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.callgraph == nil {
		return nil
	}
	all := idx.callgraph.ComputeCentrality()
	if len(all) <= limit {
		return all
	}
	return all[:limit]
}

// GetBlastRadius returns the impact analysis for changing a symbol.
// Shows what would break if the symbol is modified.
func (idx *TreeSitterIndexer) GetBlastRadius(symbol string) BlastRadius {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.callgraph == nil {
		return BlastRadius{Symbol: symbol}
	}
	return idx.callgraph.GetBlastRadius(symbol, 5)
}

// GetPageRank returns PageRank scores for all symbols.
func (idx *TreeSitterIndexer) GetPageRank() []PageRankResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.callgraph == nil {
		return nil
	}
	return idx.callgraph.ComputePageRank()
}

// FindDeadCode returns symbols that appear to be unused.
func (idx *TreeSitterIndexer) FindDeadCode() []DeadCodeResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.callgraph == nil {
		return nil
	}
	return idx.callgraph.FindDeadCode(idx.symbols)
}

// GetSymbolContent retrieves the exact source code for a symbol using byte offsets.
// This is more efficient than line-based retrieval as it extracts only the exact bytes.
func (idx *TreeSitterIndexer) GetSymbolContent(sym Symbol) (string, error) {
	fullPath := filepath.Join(idx.worktree, sym.File)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	if sym.EndByte > uint32(len(content)) {
		return "", fmt.Errorf("symbol end byte %d exceeds file length %d", sym.EndByte, len(content))
	}

	return string(content[sym.StartByte:sym.EndByte]), nil
}

// EstimateTokens estimates the token count for a string (rough: ~4 chars per token).
func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// ContextBundle holds a primary symbol with its related symbols.
type ContextBundle struct {
	Primary  SearchResult   `json:"primary"`
	Related  []SearchResult `json:"related,omitempty"`
	TotalTokens int         `json:"total_tokens"`
}

// SearchWithContext returns symbols with their related context (callees) bundled.
// This provides complete context for understanding a symbol within a token budget.
func (idx *TreeSitterIndexer) SearchWithContext(query string, maxTokens int) []ContextBundle {
	// Search for primary symbols
	primarySymbols := idx.search.Search(query, 20)
	if len(primarySymbols) == 0 {
		return nil
	}

	var bundles []ContextBundle
	usedTokens := 0

	for _, sym := range primarySymbols {
		// Get primary symbol content
		content, err := idx.GetSymbolContent(sym)
		if err != nil {
			continue
		}
		primaryTokens := EstimateTokens(content)

		// Check if we can fit the primary symbol
		if usedTokens+primaryTokens > maxTokens && len(bundles) > 0 {
			break
		}

		bundle := ContextBundle{
			Primary: SearchResult{
				Symbol:  sym,
				Content: content,
				Tokens:  primaryTokens,
			},
			TotalTokens: primaryTokens,
		}

		// Find related symbols (callees)
		callees := idx.FindCallees(sym.Name)
		relatedBudget := (maxTokens - usedTokens - primaryTokens) / 2 // reserve half for other bundles
		relatedTokens := 0

		// De-duplicate callees by symbol name
		seen := make(map[string]bool)
		for _, ref := range callees {
			if seen[ref.Symbol] {
				continue
			}
			seen[ref.Symbol] = true

			// Look up the callee symbol
			calleeDefs := idx.LookupSymbol(ref.Symbol)
			if len(calleeDefs) == 0 {
				continue
			}

			callee := calleeDefs[0]
			calleeContent, err := idx.GetSymbolContent(callee)
			if err != nil {
				continue
			}

			calleeTokens := EstimateTokens(calleeContent)
			if relatedTokens+calleeTokens > relatedBudget {
				continue
			}

			bundle.Related = append(bundle.Related, SearchResult{
				Symbol:  callee,
				Content: calleeContent,
				Tokens:  calleeTokens,
			})
			relatedTokens += calleeTokens
			bundle.TotalTokens += calleeTokens
		}

		bundles = append(bundles, bundle)
		usedTokens += bundle.TotalTokens

		if usedTokens >= maxTokens {
			break
		}
	}

	return bundles
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

