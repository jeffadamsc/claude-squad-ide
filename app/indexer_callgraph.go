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

// SymbolCentrality holds centrality metrics for a symbol.
type SymbolCentrality struct {
	Symbol    string  `json:"symbol"`
	InDegree  int     `json:"in_degree"`  // how many places call this symbol
	OutDegree int     `json:"out_degree"` // how many symbols this calls
	Score     float64 `json:"score"`      // combined centrality score
}

// ComputeCentrality calculates centrality scores for all symbols.
// Uses a simple degree-based centrality: Score = InDegree + 0.5*OutDegree
// This weights being called (usage) higher than calling others.
func (cg *CallGraph) ComputeCentrality() []SymbolCentrality {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Collect all symbols
	symbols := make(map[string]*SymbolCentrality)

	// Count in-degree (how many places call each symbol)
	for symbol, refs := range cg.callers {
		if _, ok := symbols[symbol]; !ok {
			symbols[symbol] = &SymbolCentrality{Symbol: symbol}
		}
		symbols[symbol].InDegree = len(refs)
	}

	// Count out-degree (how many symbols each caller calls)
	for caller, refs := range cg.callees {
		if _, ok := symbols[caller]; !ok {
			symbols[caller] = &SymbolCentrality{Symbol: caller}
		}
		// Count unique callees
		seen := make(map[string]bool)
		for _, ref := range refs {
			seen[ref.Symbol] = true
		}
		symbols[caller].OutDegree = len(seen)
	}

	// Compute combined score and convert to slice
	var result []SymbolCentrality
	for _, sc := range symbols {
		// Weight in-degree higher since being called indicates importance
		sc.Score = float64(sc.InDegree) + 0.5*float64(sc.OutDegree)
		result = append(result, *sc)
	}

	// Sort by score descending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Score > result[i].Score {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// GetCentrality returns the centrality score for a specific symbol.
func (cg *CallGraph) GetCentrality(symbol string) SymbolCentrality {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	sc := SymbolCentrality{Symbol: symbol}

	if refs, ok := cg.callers[symbol]; ok {
		sc.InDegree = len(refs)
	}

	if refs, ok := cg.callees[symbol]; ok {
		seen := make(map[string]bool)
		for _, ref := range refs {
			seen[ref.Symbol] = true
		}
		sc.OutDegree = len(seen)
	}

	sc.Score = float64(sc.InDegree) + 0.5*float64(sc.OutDegree)
	return sc
}
