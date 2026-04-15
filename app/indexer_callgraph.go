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

// DeadCodeResult represents a potentially unused symbol.
type DeadCodeResult struct {
	Symbol     string  `json:"symbol"`
	File       string  `json:"file"`
	Line       int     `json:"line"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"` // 0.0 - 1.0
	Reason     string  `json:"reason"`
}

// FindDeadCode finds symbols that appear to be unused.
// Excludes entry points (main, init, exported symbols based on naming convention).
func (cg *CallGraph) FindDeadCode(symbols map[string][]Symbol) []DeadCodeResult {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var results []DeadCodeResult

	for name, syms := range symbols {
		// Skip likely entry points
		if isLikelyEntryPoint(name) {
			continue
		}

		// Check if this symbol has any callers
		callers := cg.callers[name]
		if len(callers) == 0 {
			// No callers found - potentially dead code
			for _, sym := range syms {
				confidence := 0.5 // Base confidence

				// Higher confidence for non-exported symbols (lowercase in Go)
				if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
					confidence += 0.3
				}

				// Lower confidence for types (might be used via reflection)
				if sym.Kind == "type" || sym.Kind == "interface" {
					confidence -= 0.2
				}

				// Lower confidence for methods (might be interface implementations)
				if sym.Kind == "method" {
					confidence -= 0.1
				}

				// Higher confidence for functions
				if sym.Kind == "function" {
					confidence += 0.1
				}

				// Clamp to [0, 1]
				if confidence < 0 {
					confidence = 0
				}
				if confidence > 1 {
					confidence = 1
				}

				results = append(results, DeadCodeResult{
					Symbol:     name,
					File:       sym.File,
					Line:       sym.Line,
					Kind:       sym.Kind,
					Confidence: confidence,
					Reason:     "no callers found in call graph",
				})
			}
		}
	}

	// Sort by confidence descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Confidence > results[i].Confidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// isLikelyEntryPoint returns true if the symbol name suggests it's an entry point.
func isLikelyEntryPoint(name string) bool {
	// Common entry points
	entryPoints := []string{
		"main", "Main", "init", "Init",
		"Run", "Start", "Execute", "Handle",
		"New", // constructors
	}

	for _, ep := range entryPoints {
		if name == ep {
			return true
		}
	}

	// Test functions
	if len(name) > 4 && name[:4] == "Test" {
		return true
	}
	if len(name) > 9 && name[:9] == "Benchmark" {
		return true
	}
	if len(name) > 7 && name[:7] == "Example" {
		return true
	}

	return false
}

// SymbolCentrality holds centrality metrics for a symbol.
type SymbolCentrality struct {
	Symbol    string  `json:"symbol"`
	InDegree  int     `json:"in_degree"`  // how many places call this symbol
	OutDegree int     `json:"out_degree"` // how many symbols this calls
	Score     float64 `json:"score"`      // combined centrality score
}

// ComputeCentrality calculates centrality scores for all symbols.
// Uses PageRank for accurate importance scoring, with degree counts for reference.
func (cg *CallGraph) ComputeCentrality() []SymbolCentrality {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Collect all symbols with degree counts
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

	// Compute PageRank scores
	pageRank := cg.computePageRankLocked()

	// Set scores from PageRank
	var result []SymbolCentrality
	for _, sc := range symbols {
		if pr, ok := pageRank[sc.Symbol]; ok {
			sc.Score = pr * 100 // Scale up for readability
		} else {
			// Fallback to degree-based
			sc.Score = float64(sc.InDegree) + 0.5*float64(sc.OutDegree)
		}
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

// BlastRadius represents the impact of changing a symbol.
type BlastRadius struct {
	Symbol           string          `json:"symbol"`
	DirectDependents int             `json:"direct_dependents"`
	TotalDependents  int             `json:"total_dependents"`
	MaxDepth         int             `json:"max_depth"`
	RiskScore        float64         `json:"risk_score"` // 0.0 - 1.0
	Dependents       []DependentInfo `json:"dependents"`
}

// DependentInfo describes a symbol that depends on another.
type DependentInfo struct {
	Symbol string `json:"symbol"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Depth  int    `json:"depth"` // 1 = direct, 2 = transitive, etc.
}

// GetBlastRadius calculates what would be affected if a symbol changes.
// Uses depth-weighted scoring: risk = sum(1/depth^0.7) normalized to 0-1.
func (cg *CallGraph) GetBlastRadius(symbol string, maxDepth int) BlastRadius {
	if maxDepth <= 0 {
		maxDepth = 5 // default max depth
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	result := BlastRadius{
		Symbol: symbol,
	}

	// BFS to find all dependents with depth tracking
	visited := make(map[string]int) // symbol -> depth
	queue := []struct {
		sym   string
		depth int
	}{{symbol, 0}}

	var rawScore float64

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		// Find callers of this symbol
		callers := cg.callers[curr.sym]
		for _, ref := range callers {
			caller := ref.Caller
			if caller == "" {
				continue
			}

			newDepth := curr.depth + 1
			if newDepth > maxDepth {
				continue
			}

			// Skip if already visited at same or lower depth
			if existingDepth, ok := visited[caller]; ok && existingDepth <= newDepth {
				continue
			}
			visited[caller] = newDepth

			// Add to dependents
			result.Dependents = append(result.Dependents, DependentInfo{
				Symbol: caller,
				File:   ref.File,
				Line:   ref.Line,
				Depth:  newDepth,
			})

			// Update max depth
			if newDepth > result.MaxDepth {
				result.MaxDepth = newDepth
			}

			// Accumulate risk score (depth-weighted)
			// Using 1/depth^0.7 gives reasonable decay
			rawScore += 1.0 / pow(float64(newDepth), 0.7)

			// Queue for further traversal
			queue = append(queue, struct {
				sym   string
				depth int
			}{caller, newDepth})
		}
	}

	// Count direct vs total dependents
	for _, dep := range result.Dependents {
		if dep.Depth == 1 {
			result.DirectDependents++
		}
	}
	result.TotalDependents = len(result.Dependents)

	// Normalize risk score to 0-1 range
	// Use sigmoid-like function: score / (score + k) where k controls scaling
	if rawScore > 0 {
		result.RiskScore = rawScore / (rawScore + 5.0)
	}

	return result
}

// pow computes x^y for floats (simple implementation for small exponents)
func pow(x, y float64) float64 {
	if y == 0 {
		return 1
	}
	if y == 1 {
		return x
	}
	// Use logarithm for fractional exponents
	if x <= 0 {
		return 0
	}
	// exp(y * ln(x))
	return exp(y * ln(x))
}

// ln computes natural logarithm using Taylor series (good enough for our purposes)
func ln(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Use the identity ln(x) = 2 * arctanh((x-1)/(x+1)) for better convergence
	// For simplicity, use Go's math package indirectly via type conversion
	// Actually, let's just use a simple approximation
	result := 0.0
	term := (x - 1) / (x + 1)
	termSq := term * term
	power := term
	for i := 1; i < 20; i += 2 {
		result += power / float64(i)
		power *= termSq
	}
	return 2 * result
}

// exp computes e^x using Taylor series
func exp(x float64) float64 {
	result := 1.0
	term := 1.0
	for i := 1; i < 20; i++ {
		term *= x / float64(i)
		result += term
	}
	return result
}

// GetCentrality returns the centrality score for a specific symbol.
// Uses PageRank for more accurate importance scoring.
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

	// Use PageRank score if available
	pageRank := cg.computePageRankLocked()
	if pr, ok := pageRank[symbol]; ok {
		sc.Score = pr * 100 // Scale up for readability
	} else {
		// Fallback to simple degree-based score
		sc.Score = float64(sc.InDegree) + 0.5*float64(sc.OutDegree)
	}
	return sc
}

// PageRankResult holds PageRank scores for symbols.
type PageRankResult struct {
	Symbol string  `json:"symbol"`
	Score  float64 `json:"score"`
}

// ComputePageRank computes PageRank scores for all symbols in the call graph.
// Returns scores sorted by rank descending.
func (cg *CallGraph) ComputePageRank() []PageRankResult {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	scores := cg.computePageRankLocked()

	// Convert to sorted slice
	var results []PageRankResult
	for sym, score := range scores {
		results = append(results, PageRankResult{Symbol: sym, Score: score})
	}

	// Sort by score descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// computePageRankLocked computes PageRank (must hold read lock).
func (cg *CallGraph) computePageRankLocked() map[string]float64 {
	const (
		damping    = 0.85
		iterations = 20
		epsilon    = 0.0001
	)

	// Collect all symbols
	symbols := make(map[string]bool)
	for sym := range cg.callers {
		symbols[sym] = true
	}
	for sym := range cg.callees {
		symbols[sym] = true
	}

	if len(symbols) == 0 {
		return nil
	}

	n := float64(len(symbols))
	initial := 1.0 / n

	// Initialize scores
	scores := make(map[string]float64)
	for sym := range symbols {
		scores[sym] = initial
	}

	// Build out-degree map (unique callees per caller)
	outDegree := make(map[string]int)
	for caller, refs := range cg.callees {
		seen := make(map[string]bool)
		for _, ref := range refs {
			seen[ref.Symbol] = true
		}
		outDegree[caller] = len(seen)
	}

	// Iterative PageRank
	for iter := 0; iter < iterations; iter++ {
		newScores := make(map[string]float64)
		maxDiff := 0.0

		for sym := range symbols {
			// Base score (random jump)
			rank := (1 - damping) / n

			// Add contribution from callers
			callers := cg.callers[sym]
			for _, ref := range callers {
				caller := ref.Caller
				if caller == "" {
					continue
				}
				if out := outDegree[caller]; out > 0 {
					rank += damping * scores[caller] / float64(out)
				}
			}

			newScores[sym] = rank

			diff := rank - scores[sym]
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		scores = newScores

		// Check convergence
		if maxDiff < epsilon {
			break
		}
	}

	return scores
}
