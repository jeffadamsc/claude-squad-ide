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
