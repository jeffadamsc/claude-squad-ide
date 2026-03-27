package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
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
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
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
// Returns empty string if not found or if the system ctags is not universal-ctags.
func findUniversalCtags() string {
	// Try common universal-ctags binary names
	for _, name := range []string{"uctags", "ctags"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		// Verify it's universal-ctags by checking --version
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
	ctagsPath := findUniversalCtags()
	if ctagsPath == "" {
		return make(map[string][]Definition), nil
	}

	cmd := exec.Command(ctagsPath,
		"--output-format=json",
		"-R",
		"--languages=Go,C,C++,JavaScript,TypeScript",
		".",
	)
	cmd.Dir = dir
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

	stopCh  chan struct{}
	done    chan struct{}
	refresh chan struct{}
}

// NewSessionIndexer creates a new indexer for the given worktree.
func NewSessionIndexer(worktree string) *SessionIndexer {
	return &SessionIndexer{
		worktree: worktree,
		symbols:  make(map[string][]Definition),
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
		refresh:  make(chan struct{}, 1),
	}
}

// Start begins the indexer with an immediate build and periodic refresh.
func (idx *SessionIndexer) Start() {
	idx.build()
	go idx.loop()
}

// Stop halts the periodic refresh.
func (idx *SessionIndexer) Stop() {
	close(idx.stopCh)
	<-idx.done
}

// Refresh triggers an immediate re-index. Non-blocking.
func (idx *SessionIndexer) Refresh() {
	select {
	case idx.refresh <- struct{}{}:
	default:
	}
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

func (idx *SessionIndexer) build() {
	files, _ := listFilesInWorktree(idx.worktree)
	symbols, _ := runCtags(idx.worktree)

	idx.mu.Lock()
	idx.files = files
	idx.symbols = symbols
	idx.mu.Unlock()
}

func (idx *SessionIndexer) loop() {
	defer close(idx.done)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-idx.stopCh:
			return
		case <-ticker.C:
			idx.build()
		case <-idx.refresh:
			idx.build()
		}
	}
}
