package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"claude-squad/log"
)

// Definition represents a symbol definition from ctags.
type Definition struct {
	Name     string `json:"name"`
	File     string `json:"path"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Scope    string `json:"scope"`
}

// Symbol is the tree-sitter-based symbol with extended fields.
// Replaces Definition for the new indexer.
type Symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`       // function, type, variable, method, class
	File       string `json:"file"`
	Line       int    `json:"line"`
	EndLine    int    `json:"end_line"`   // for extracting full body
	Column     int    `json:"column"`
	StartByte  uint32 `json:"start_byte"` // byte offset for precise retrieval
	EndByte    uint32 `json:"end_byte"`   // byte offset for precise retrieval
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

// IndexerType determines which indexer to use.
type IndexerType string

const (
	IndexerCtags      IndexerType = "ctags"
	IndexerTreeSitter IndexerType = "treesitter"
)

// DefaultIndexer is the default indexer type for new sessions.
var DefaultIndexer = IndexerTreeSitter

// createIndexer creates an indexer of the given type for the worktree.
func createIndexer(worktree string, indexerType IndexerType) Indexer {
	switch indexerType {
	case IndexerTreeSitter:
		return NewTreeSitterIndexer(worktree)
	default:
		return NewSessionIndexer(worktree)
	}
}

// ctagsEntry is the raw JSON structure from ctags --output-format=json.
type ctagsEntry struct {
	Type     string `json:"_type"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Scope    string `json:"scope"`
}

// listFilesInWorktree returns all git-tracked files in the given worktree directory.
func listFilesInWorktree(worktree string) ([]string, error) {
	return listFilesInWorktreeCtx(context.Background(), worktree)
}

func listFilesInWorktreeCtx(ctx context.Context, worktree string) ([]string, error) {
	// Use --recurse-submodules to include files from git submodules
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--recurse-submodules")
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		// Fall back to plain ls-files if --recurse-submodules isn't supported
		cmd = exec.CommandContext(ctx, "git", "ls-files")
		cmd.Dir = worktree
		out, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git ls-files: %w", err)
		}
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// parseCtagsJSON parses ctags JSON output into a symbol table.
func parseCtagsJSON(data []byte) map[string][]Definition {
	symbols := make(map[string][]Definition)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry ctagsEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != "tag" {
			continue
		}
		def := Definition{
			Name:     entry.Name,
			File:     entry.Path,
			Line:     entry.Line,
			Kind:     entry.Kind,
			Language: entry.Language,
			Scope:    entry.Scope,
		}
		symbols[entry.Name] = append(symbols[entry.Name], def)
	}
	return symbols
}

// findUniversalCtags locates the universal-ctags binary.
// Checks the bundled location inside the .app first, then falls back to PATH.
// Returns empty string if not found or if the system ctags is not universal-ctags.
func findUniversalCtags() string {
	// Check bundled ctags inside the .app bundle first.
	// The binary lives at: .app/Contents/MacOS/cs (our binary)
	// So the bundled ctags is at: .app/Contents/Resources/ctags/ctags
	if exe, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(exe), "..", "Resources", "ctags", "ctags")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Fall back to PATH lookup
	for _, name := range []string{"uctags", "ctags"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, "--version")
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		if strings.Contains(string(out), "Universal Ctags") {
			return path
		}
	}
	return ""
}

// runCtags runs universal-ctags over a directory and returns the symbol table.
// Returns an empty map (not an error) if universal-ctags is not installed.
func runCtags(dir string) (map[string][]Definition, error) {
	return runCtagsCtx(context.Background(), dir, nil)
}

// runCtagsCtx runs ctags with context cancellation support.
// If files is non-nil, only those files are indexed (via -L -) instead of recursive scan.
func runCtagsCtx(ctx context.Context, dir string, files []string) (map[string][]Definition, error) {
	ctagsPath := findUniversalCtags()
	if ctagsPath == "" {
		return make(map[string][]Definition), nil
	}

	args := []string{
		"--output-format=json",
		"--languages=Go,C,C++,JavaScript,TypeScript",
	}

	if files != nil {
		// Index only the listed files via stdin — much faster than -R on large repos
		args = append(args, "-L", "-")
	} else {
		args = append(args, "-R", ".")
	}

	cmd := exec.CommandContext(ctx, ctagsPath, args...)
	cmd.Dir = dir

	if files != nil {
		cmd.Stdin = strings.NewReader(strings.Join(files, "\n"))
	}

	out, err := cmd.Output()
	if err != nil {
		return make(map[string][]Definition), nil
	}
	return parseCtagsJSON(out), nil
}

// SessionIndexer manages file list and symbol index for a scoped session.
type SessionIndexer struct {
	worktree string

	mu      sync.RWMutex
	files   []string
	symbols map[string][]Definition

	cancel context.CancelFunc
	done   chan struct{}
	refresh chan struct{}
}

// NewSessionIndexer creates a new indexer for the given worktree.
func NewSessionIndexer(worktree string) *SessionIndexer {
	return &SessionIndexer{
		worktree: worktree,
		symbols:  make(map[string][]Definition),
		done:     make(chan struct{}),
		refresh:  make(chan struct{}, 1),
	}
}

// Start begins the indexer background loop with an immediate build.
// Returns immediately — the build runs in the background goroutine.
func (idx *SessionIndexer) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	idx.cancel = cancel
	go idx.loop(ctx)
	idx.Refresh() // trigger immediate build without blocking caller
}

// Stop halts the indexer, killing any in-progress external commands immediately.
func (idx *SessionIndexer) Stop() {
	if idx.cancel != nil {
		idx.cancel()
	}
	<-idx.done
}

// Refresh triggers an immediate re-index. Non-blocking.
func (idx *SessionIndexer) Refresh() {
	select {
	case idx.refresh <- struct{}{}:
	default:
	}
}

// Worktree returns the worktree path.
func (idx *SessionIndexer) Worktree() string {
	return idx.worktree
}

// Files returns the current list of tracked files.
func (idx *SessionIndexer) Files() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]string, len(idx.files))
	copy(out, idx.files)
	return out
}

// Lookup returns definitions matching the given symbol name.
func (idx *SessionIndexer) Lookup(name string) []Definition {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	defs := idx.symbols[name]
	out := make([]Definition, len(defs))
	copy(out, defs)
	return out
}

// AllSymbols returns a copy of the entire symbol table.
func (idx *SessionIndexer) AllSymbols() map[string][]Definition {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make(map[string][]Definition, len(idx.symbols))
	for k, v := range idx.symbols {
		defs := make([]Definition, len(v))
		copy(defs, v)
		out[k] = defs
	}
	return out
}

func logInfo(format string, args ...interface{}) {
	if log.InfoLog != nil {
		log.InfoLog.Printf(format, args...)
	}
}

func logError(format string, args ...interface{}) {
	if log.ErrorLog != nil {
		log.ErrorLog.Printf(format, args...)
	}
}

// filesChanged returns true if the new file list differs from the cached one.
func (idx *SessionIndexer) filesChanged(newFiles []string) bool {
	idx.mu.RLock()
	oldFiles := idx.files
	idx.mu.RUnlock()

	if len(oldFiles) != len(newFiles) {
		return true
	}
	for i := range oldFiles {
		if oldFiles[i] != newFiles[i] {
			return true
		}
	}
	return false
}

// build gets the file list first (fast), updates state, then runs ctags on those files.
// On periodic refreshes (force=false), it skips the expensive ctags step if the file list
// hasn't changed, avoiding unnecessary CPU/memory churn.
func (idx *SessionIndexer) build(ctx context.Context, force bool) {
	// Step 1: get file list (fast — just git ls-files)
	files, filesErr := listFilesInWorktreeCtx(ctx, idx.worktree)
	if filesErr != nil {
		if ctx.Err() != nil {
			return // cancelled
		}
		logError("indexer build(%s): listFiles error: %v", idx.worktree, filesErr)
	}

	if ctx.Err() != nil {
		return
	}

	// Skip expensive ctags rebuild if file list hasn't changed and this isn't a forced refresh
	if !force && !idx.filesChanged(files) {
		return
	}

	// Update files immediately so search works even before ctags finishes
	idx.mu.Lock()
	idx.files = files
	idx.mu.Unlock()

	if ctx.Err() != nil {
		return
	}

	// Step 2: run ctags on only the git-tracked files (not recursive -R)
	symbols, ctagsErr := runCtagsCtx(ctx, idx.worktree, files)
	if ctagsErr != nil {
		if ctx.Err() != nil {
			return // cancelled
		}
		logError("indexer build(%s): runCtags error: %v", idx.worktree, ctagsErr)
	}

	if ctx.Err() != nil {
		return
	}

	nSymbols := 0
	for _, defs := range symbols {
		nSymbols += len(defs)
	}
	logInfo("indexer build(%s): %d files, %d symbols (%d names)", idx.worktree, len(files), nSymbols, len(symbols))

	idx.mu.Lock()
	idx.symbols = symbols
	idx.mu.Unlock()
}

func (idx *SessionIndexer) loop(ctx context.Context) {
	defer close(idx.done)
	// Wait for the first refresh signal (sent immediately by Start)
	select {
	case <-ctx.Done():
		return
	case <-idx.refresh:
		idx.build(ctx, true) // force: first build always runs fully
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idx.build(ctx, false) // periodic: skip ctags if files unchanged
		case <-idx.refresh:
			idx.build(ctx, true) // explicit refresh: always rebuild
		}
	}
}
