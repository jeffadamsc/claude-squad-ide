# Scope Mode VS Code Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Quick Open (Cmd+P), Monaco find/replace passthrough, and ctags-powered go-to-definition to scope mode.

**Architecture:** Backend adds three new Wails bindings (`ListFiles`, `IndexSession`, `LookupSymbol`) backed by `git ls-files` and `universal-ctags`. A shared periodic timer refreshes both indexes every 10 seconds. Frontend adds a `QuickOpen` overlay component with client-side fuzzy matching, a Monaco `DefinitionProvider` for go-to-definition, and hotkey wiring.

**Tech Stack:** Go (Wails bindings, ctags JSON parsing), React + TypeScript (QuickOpen component, fuzzy matcher), Monaco Editor API (DefinitionProvider, peek/reveal actions), universal-ctags (runtime dependency).

**Spec:** `docs/superpowers/specs/2026-03-26-scope-mode-vscode-features-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `app/indexer.go` | Create | ctags runner, symbol table, periodic refresh timer, `ListFiles`/`IndexSession`/`LookupSymbol` logic |
| `app/indexer_test.go` | Create | Tests for ctags parsing, symbol lookup, ListFiles |
| `app/bindings.go` | Modify | Add `ListFiles`, `IndexSession`, `LookupSymbol` binding methods; add eager refresh in `WriteFile`; add indexer field to `SessionAPI` |
| `frontend/src/lib/fuzzyMatch.ts` | Create | Fuzzy file matching scorer |
| `frontend/src/lib/definitionProvider.ts` | Create | Monaco DefinitionProvider registration + navigation logic |
| `frontend/src/components/ScopeMode/QuickOpen.tsx` | Create | Quick Open overlay component |
| `frontend/src/store/sessionStore.ts` | Modify | Add `fileList`, `quickOpenVisible`, `symbolDefinitions` state + actions |
| `frontend/src/components/ScopeMode/ScopeLayout.tsx` | Modify | Render QuickOpen overlay, wire Cmd+P hotkey, trigger index on scope entry |
| `frontend/src/components/ScopeMode/EditorPane.tsx` | Modify | Register DefinitionProvider on mount |
| `frontend/src/hooks/useHotkeys.ts` | Modify | Add Cmd+P binding, ensure Cmd+F passthrough |
| `frontend/src/lib/wails.ts` | Modify | Add type definitions and bindings for new API methods |

---

### Task 1: Backend — ListFiles Binding

**Files:**
- Create: `app/indexer.go`
- Create: `app/indexer_test.go`
- Modify: `app/bindings.go:64-77` (SessionAPI struct)
- Modify: `app/bindings.go` (add ListFiles method)

- [ ] **Step 1: Write the failing test for ListFiles**

Create `app/indexer_test.go`:

```go
package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListFilesInWorktree(t *testing.T) {
	// Create a temp git repo with some files
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Create files
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0644))

	// Add and commit
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	files, err := listFilesInWorktree(dir)
	require.NoError(t, err)
	assert.Contains(t, files, "main.go")
	assert.Contains(t, files, "src/app.go")
	assert.Contains(t, files, "README.md")
	assert.Len(t, files, 3)
}

func TestListFilesInWorktree_ExcludesUntracked(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package main"), 0644))
	runGit(t, dir, "add", "tracked.go")
	runGit(t, dir, "commit", "-m", "init")

	// Create untracked file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package main"), 0644))

	files, err := listFilesInWorktree(dir)
	require.NoError(t, err)
	assert.Contains(t, files, "tracked.go")
	assert.NotContains(t, files, "untracked.go")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestListFiles -v`
Expected: FAIL — `listFilesInWorktree` undefined

- [ ] **Step 3: Implement listFilesInWorktree**

Create `app/indexer.go`:

```go
package app

import (
	"os/exec"
	"strings"
)

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
```

Add the missing `"fmt"` import to the imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestListFiles -v`
Expected: PASS

- [ ] **Step 5: Wire up the ListFiles Wails binding**

In `app/bindings.go`, add the `ListFiles` method to `SessionAPI`:

```go
// ListFiles returns all git-tracked files in the session's worktree.
func (api *SessionAPI) ListFiles(sessionID string) ([]string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	inst, ok := api.instances[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if inst.HostID != "" {
		return api.listFilesRemote(inst)
	}

	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}
	return listFilesInWorktree(worktree)
}

// listFilesRemote runs git ls-files on a remote host via SSH.
func (api *SessionAPI) listFilesRemote(inst *session.Instance) ([]string, error) {
	client, err := api.hostManager.GetClient(inst.HostID)
	if err != nil {
		return nil, fmt.Errorf("SSH connect: %w", err)
	}
	defer api.hostManager.ReleaseClient(inst.HostID)

	path := shellEscape(inst.Path)
	out, err := client.Run(fmt.Sprintf("cd %s && git ls-files", path))
	if err != nil {
		return nil, fmt.Errorf("remote git ls-files: %w", err)
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}
```

- [ ] **Step 6: Commit**

```bash
git add app/indexer.go app/indexer_test.go app/bindings.go
git commit -m "feat(scope): add ListFiles backend binding with git ls-files"
```

---

### Task 2: Backend — ctags Indexer and LookupSymbol

**Files:**
- Modify: `app/indexer.go`
- Modify: `app/indexer_test.go`
- Modify: `app/bindings.go`

- [ ] **Step 1: Write the failing test for ctags JSON parsing**

Add to `app/indexer_test.go`:

```go
func TestParseCtagsJSON(t *testing.T) {
	// Simulated ctags --output-format=json output (one JSON object per line)
	input := `{"_type": "tag", "name": "main", "path": "main.go", "line": 5, "kind": "function", "language": "Go"}
{"_type": "tag", "name": "SessionAPI", "path": "app/bindings.go", "line": 64, "kind": "struct", "language": "Go", "scope": "app"}
{"_type": "tag", "name": "ListFiles", "path": "app/bindings.go", "line": 100, "kind": "function", "language": "Go", "scope": "SessionAPI"}
`
	symbols := parseCtagsJSON([]byte(input))
	assert.Len(t, symbols["main"], 1)
	assert.Equal(t, "main.go", symbols["main"][0].File)
	assert.Equal(t, 5, symbols["main"][0].Line)
	assert.Equal(t, "function", symbols["main"][0].Kind)

	assert.Len(t, symbols["SessionAPI"], 1)
	assert.Len(t, symbols["ListFiles"], 1)
	assert.Equal(t, "SessionAPI", symbols["ListFiles"][0].Scope)
}

func TestParseCtagsJSON_EmptyInput(t *testing.T) {
	symbols := parseCtagsJSON([]byte(""))
	assert.Empty(t, symbols)
}

func TestParseCtagsJSON_IgnoresNonTagLines(t *testing.T) {
	input := `{"_type": "ptag", "name": "JSON_OUTPUT_VERSION", "path": "0.0"}
{"_type": "tag", "name": "Foo", "path": "foo.go", "line": 1, "kind": "function", "language": "Go"}
`
	symbols := parseCtagsJSON([]byte(input))
	assert.Len(t, symbols, 1)
	assert.Contains(t, symbols, "Foo")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestParseCtagsJSON -v`
Expected: FAIL — `parseCtagsJSON` undefined

- [ ] **Step 3: Implement Definition type and parseCtagsJSON**

Add to `app/indexer.go`:

```go
import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestParseCtagsJSON -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for runCtags**

Add to `app/indexer_test.go`:

```go
func TestRunCtags(t *testing.T) {
	// Skip if ctags not installed
	if _, err := exec.LookPath("ctags"); err != nil {
		t.Skip("ctags not installed")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}

func helper() string {
	return "hello"
}
`), 0644))

	symbols, err := runCtags(dir)
	require.NoError(t, err)
	assert.Contains(t, symbols, "main")
	assert.Contains(t, symbols, "helper")
	assert.Equal(t, "main.go", symbols["helper"][0].File)
	assert.Equal(t, "function", symbols["helper"][0].Kind)
}

func TestRunCtags_NotInstalled(t *testing.T) {
	// Use a PATH with no ctags
	t.Setenv("PATH", "/nonexistent")
	symbols, err := runCtags(t.TempDir())
	assert.NoError(t, err) // graceful degradation
	assert.Empty(t, symbols)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestRunCtags -v`
Expected: FAIL — `runCtags` undefined

- [ ] **Step 7: Implement runCtags**

Add to `app/indexer.go`:

```go
// runCtags runs universal-ctags over a directory and returns the symbol table.
// Returns an empty map (not an error) if ctags is not installed.
func runCtags(dir string) (map[string][]Definition, error) {
	ctagsPath, err := exec.LookPath("ctags")
	if err != nil {
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
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestRunCtags -v`
Expected: PASS (or skip if ctags not installed for the first test)

- [ ] **Step 9: Commit**

```bash
git add app/indexer.go app/indexer_test.go
git commit -m "feat(scope): add ctags JSON parser and runner with graceful degradation"
```

---

### Task 3: Backend — SessionIndexer with Periodic Refresh

**Files:**
- Modify: `app/indexer.go`
- Modify: `app/indexer_test.go`
- Modify: `app/bindings.go:64-77` (add indexer fields)

- [ ] **Step 1: Write the failing test for SessionIndexer**

Add to `app/indexer_test.go`:

```go
import (
	"sync"
	"time"
)

func TestSessionIndexer_StartStop(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	idx := NewSessionIndexer(dir)
	idx.Start()

	// Wait for initial index
	time.Sleep(200 * time.Millisecond)

	files := idx.Files()
	assert.Contains(t, files, "main.go")

	symbols := idx.Lookup("main")
	assert.NotEmpty(t, symbols)

	idx.Stop()
}

func TestSessionIndexer_Refresh(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	idx := NewSessionIndexer(dir)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Add a new file and commit
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package main\nfunc helper() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add helper")

	// Trigger manual refresh
	idx.Refresh()
	time.Sleep(200 * time.Millisecond)

	files := idx.Files()
	assert.Contains(t, files, "helper.go")
	assert.Contains(t, idx.Lookup("helper"), Definition{
		Name: "helper", File: "helper.go", Line: 2, Kind: "function", Language: "Go",
	})

	idx.Stop()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestSessionIndexer -v`
Expected: FAIL — `NewSessionIndexer` undefined

- [ ] **Step 3: Implement SessionIndexer**

Add to `app/indexer.go`:

```go
import (
	"sync"
	"time"
)

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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -run TestSessionIndexer -v`
Expected: PASS

- [ ] **Step 5: Wire SessionIndexer into SessionAPI bindings**

In `app/bindings.go`, add an `indexers` field to `SessionAPI`:

```go
// In the SessionAPI struct (line ~64), add:
indexers map[string]*SessionIndexer
```

Initialize it in the constructor (or wherever `SessionAPI` is created) as `make(map[string]*SessionIndexer)`.

Add these binding methods:

```go
// IndexSession starts or restarts the indexer for a scoped session.
func (api *SessionAPI) IndexSession(sessionID string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Stop existing indexer if any
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Stop()
	}

	inst, ok := api.instances[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Remote sessions: not yet supported for indexing
	if inst.HostID != "" {
		return nil
	}

	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}

	idx := NewSessionIndexer(worktree)
	idx.Start()
	api.indexers[sessionID] = idx
	return nil
}

// StopIndexer stops the indexer for a session (called on scope exit).
func (api *SessionAPI) StopIndexer(sessionID string) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Stop()
		delete(api.indexers, sessionID)
	}
}

// LookupSymbol returns definitions for a symbol name in the scoped session.
func (api *SessionAPI) LookupSymbol(sessionID string, symbol string) ([]Definition, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	idx, ok := api.indexers[sessionID]
	if !ok {
		return nil, nil
	}
	return idx.Lookup(symbol), nil
}
```

Update `ListFiles` to use the indexer when available:

```go
func (api *SessionAPI) ListFiles(sessionID string) ([]string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	// Use indexer cache if available
	if idx, ok := api.indexers[sessionID]; ok {
		files := idx.Files()
		if files != nil {
			return files, nil
		}
	}

	inst, ok := api.instances[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if inst.HostID != "" {
		return api.listFilesRemote(inst)
	}

	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}
	return listFilesInWorktree(worktree)
}
```

Add eager refresh in `WriteFile` — at the end of the method, before returning:

```go
// Trigger index refresh after file write
if idx, ok := api.indexers[sessionID]; ok {
	idx.Refresh()
}
```

- [ ] **Step 6: Initialize indexers map**

Find where `SessionAPI` is constructed (check `main.go` or wherever `&SessionAPI{...}` is created) and add `indexers: make(map[string]*SessionIndexer)` to the initialization.

Also update the `Close()` method to stop all indexers:

```go
// In Close(), add before existing cleanup:
for _, idx := range api.indexers {
	idx.Stop()
}
```

- [ ] **Step 7: Run all backend tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./app/ -v`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add app/indexer.go app/indexer_test.go app/bindings.go main.go
git commit -m "feat(scope): add SessionIndexer with periodic refresh and Wails bindings"
```

---

### Task 4: Frontend — Wails Type Definitions and Store State

**Files:**
- Modify: `frontend/src/lib/wails.ts:47-52,83-119`
- Modify: `frontend/src/store/sessionStore.ts`

- [ ] **Step 1: Add Definition type and new bindings to wails.ts**

In `frontend/src/lib/wails.ts`, add after the `DirInfo` interface (around line 73):

```typescript
export interface SymbolDefinition {
  name: string;
  path: string;
  line: number;
  kind: string;
  language: string;
  scope: string;
}
```

In the `window.go.app.SessionAPI` declaration, add after `WriteFile` (line 114):

```typescript
ListFiles(sessionId: string): Promise<string[]>;
IndexSession(sessionId: string): Promise<void>;
StopIndexer(sessionId: string): Promise<void>;
LookupSymbol(sessionId: string, symbol: string): Promise<SymbolDefinition[]>;
```

- [ ] **Step 2: Add scope mode state to sessionStore.ts**

In `frontend/src/store/sessionStore.ts`, add to the `SessionState` interface (after `activeEditorFile` around line 41):

```typescript
fileList: string[];
quickOpenVisible: boolean;
```

Add initial values in the store creation (after `activeEditorFile: null` around line 103):

```typescript
fileList: [],
quickOpenVisible: false,
```

Add actions (in the actions section, after existing scope mode actions):

```typescript
setFileList: (files: string[]) => set({ fileList: files }),

setQuickOpenVisible: (visible: boolean) => set({ quickOpenVisible: visible }),

toggleQuickOpen: () => set((state) => ({ quickOpenVisible: !state.quickOpenVisible })),
```

Update `exitScopeMode` to also clear `fileList` and `quickOpenVisible`:

```typescript
// In exitScopeMode, add to the set() call:
fileList: [],
quickOpenVisible: false,
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/wails.ts frontend/src/store/sessionStore.ts
git commit -m "feat(scope): add Wails type definitions and store state for Quick Open and go-to-definition"
```

---

### Task 5: Frontend — Fuzzy Match Utility

**Files:**
- Create: `frontend/src/lib/fuzzyMatch.ts`

- [ ] **Step 1: Create fuzzyMatch.ts**

Create `frontend/src/lib/fuzzyMatch.ts`:

```typescript
export interface FuzzyResult {
  path: string;
  score: number;
  matches: number[]; // indices of matched characters in the basename
}

/**
 * Fuzzy-match a query against a list of file paths.
 * Returns results sorted by score (highest first), capped at `limit`.
 *
 * Scoring:
 * - Consecutive character matches score higher than spread matches
 * - Matches at the start of the filename score higher
 * - Filename matches score higher than directory path matches
 */
export function fuzzyMatch(
  query: string,
  paths: string[],
  limit = 20
): FuzzyResult[] {
  if (!query) return paths.slice(0, limit).map((p) => ({ path: p, score: 0, matches: [] }));

  const lowerQuery = query.toLowerCase();
  const results: FuzzyResult[] = [];

  for (const path of paths) {
    const basename = path.slice(path.lastIndexOf("/") + 1);
    const lowerBasename = basename.toLowerCase();

    // Try matching against basename first (higher score)
    const basenameResult = matchString(lowerQuery, lowerBasename);
    if (basenameResult) {
      results.push({
        path,
        score: basenameResult.score + 100, // bonus for filename match
        matches: basenameResult.indices,
      });
      continue;
    }

    // Fall back to matching against full path
    const lowerPath = path.toLowerCase();
    const pathResult = matchString(lowerQuery, lowerPath);
    if (pathResult) {
      // Remap match indices to basename portion if possible
      const basenameStart = path.lastIndexOf("/") + 1;
      const basenameMatches = pathResult.indices
        .filter((i) => i >= basenameStart)
        .map((i) => i - basenameStart);
      results.push({
        path,
        score: pathResult.score,
        matches: basenameMatches,
      });
    }
  }

  results.sort((a, b) => b.score - a.score);
  return results.slice(0, limit);
}

function matchString(
  query: string,
  target: string
): { score: number; indices: number[] } | null {
  let qi = 0;
  let score = 0;
  const indices: number[] = [];
  let lastMatchIdx = -2;

  for (let ti = 0; ti < target.length && qi < query.length; ti++) {
    if (target[ti] === query[qi]) {
      indices.push(ti);
      // Consecutive match bonus
      if (ti === lastMatchIdx + 1) {
        score += 10;
      } else {
        score += 5;
      }
      // Start-of-string bonus
      if (ti === 0) {
        score += 15;
      }
      // After separator bonus (/, ., -, _)
      if (ti > 0 && "/.-_".includes(target[ti - 1])) {
        score += 10;
      }
      lastMatchIdx = ti;
      qi++;
    }
  }

  if (qi < query.length) return null; // not all query chars matched
  return { score, indices };
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/lib/fuzzyMatch.ts
git commit -m "feat(scope): add fuzzy file matching utility"
```

---

### Task 6: Frontend — QuickOpen Component

**Files:**
- Create: `frontend/src/components/ScopeMode/QuickOpen.tsx`
- Modify: `frontend/src/components/ScopeMode/ScopeLayout.tsx`
- Modify: `frontend/src/hooks/useHotkeys.ts`

- [ ] **Step 1: Create QuickOpen.tsx**

Create `frontend/src/components/ScopeMode/QuickOpen.tsx`:

```tsx
import { useState, useRef, useEffect, useCallback } from "react";
import { useSessionStore, detectLanguage } from "../../store/sessionStore";
import { fuzzyMatch, type FuzzyResult } from "../../lib/fuzzyMatch";
import { api } from "../../lib/wails";

export function QuickOpen({ sessionId }: { sessionId: string }) {
  const { fileList, quickOpenVisible, setQuickOpenVisible, openEditorFile } =
    useSessionStore();
  const [query, setQuery] = useState("");
  const [selectedIdx, setSelectedIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const results = fuzzyMatch(query, fileList);

  // Focus input when opened
  useEffect(() => {
    if (quickOpenVisible) {
      setQuery("");
      setSelectedIdx(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [quickOpenVisible]);

  // Scroll selected item into view
  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const item = list.children[selectedIdx] as HTMLElement | undefined;
    item?.scrollIntoView({ block: "nearest" });
  }, [selectedIdx]);

  const close = useCallback(() => {
    setQuickOpenVisible(false);
    setQuery("");
  }, [setQuickOpenVisible]);

  const openFile = useCallback(
    async (path: string) => {
      close();
      try {
        const contents = await api().ReadFile(sessionId, path);
        openEditorFile(path, contents, detectLanguage(path));
      } catch (err) {
        console.error("Failed to open file:", err);
      }
    },
    [sessionId, close, openEditorFile]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedIdx((i) => Math.min(i + 1, results.length - 1));
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIdx((i) => Math.max(i - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (results[selectedIdx]) {
            openFile(results[selectedIdx].path);
          }
          break;
        case "Escape":
          e.preventDefault();
          close();
          break;
      }
    },
    [results, selectedIdx, openFile, close]
  );

  if (!quickOpenVisible) return null;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 1000,
        display: "flex",
        justifyContent: "center",
        paddingTop: 48,
      }}
      onClick={close}
    >
      <div
        style={{
          width: 520,
          maxHeight: "60vh",
          background: "var(--mantle)",
          border: "1px solid var(--surface1)",
          borderRadius: 8,
          boxShadow: "0 8px 32px rgba(0,0,0,0.5)",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Search input */}
        <div
          style={{
            padding: "10px 14px",
            borderBottom: "1px solid var(--surface0)",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          <span style={{ color: "var(--overlay0)", fontSize: 14 }}>🔍</span>
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIdx(0);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Search files by name..."
            style={{
              flex: 1,
              background: "transparent",
              border: "none",
              outline: "none",
              color: "var(--text)",
              fontSize: 14,
              fontFamily: "inherit",
            }}
          />
        </div>

        {/* Results */}
        <div ref={listRef} style={{ overflowY: "auto", maxHeight: "calc(60vh - 60px)" }}>
          {results.map((r, i) => (
            <QuickOpenItem
              key={r.path}
              result={r}
              selected={i === selectedIdx}
              onClick={() => openFile(r.path)}
            />
          ))}
          {results.length === 0 && query && (
            <div style={{ padding: "16px 14px", color: "var(--overlay0)", fontSize: 13 }}>
              No matching files
            </div>
          )}
        </div>

        {/* Footer */}
        <div
          style={{
            padding: "6px 14px",
            borderTop: "1px solid var(--surface0)",
            display: "flex",
            justifyContent: "space-between",
            fontSize: 11,
            color: "var(--overlay0)",
          }}
        >
          <span>↑↓ navigate</span>
          <span>⏎ open &nbsp; esc close</span>
        </div>
      </div>
    </div>
  );
}

function QuickOpenItem({
  result,
  selected,
  onClick,
}: {
  result: FuzzyResult;
  selected: boolean;
  onClick: () => void;
}) {
  const path = result.path;
  const lastSlash = path.lastIndexOf("/");
  const basename = path.slice(lastSlash + 1);
  const dir = lastSlash >= 0 ? path.slice(0, lastSlash + 1) : "";

  // Build highlighted filename
  const highlighted: React.ReactNode[] = [];
  let cursor = 0;
  for (const matchIdx of result.matches) {
    if (matchIdx > cursor) {
      highlighted.push(basename.slice(cursor, matchIdx));
    }
    highlighted.push(
      <span key={matchIdx} style={{ color: "var(--yellow)" }}>
        {basename[matchIdx]}
      </span>
    );
    cursor = matchIdx + 1;
  }
  if (cursor < basename.length) {
    highlighted.push(basename.slice(cursor));
  }

  return (
    <div
      onClick={onClick}
      style={{
        padding: "8px 14px",
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        background: selected ? "var(--surface0)" : "transparent",
        cursor: "pointer",
        fontSize: 13,
      }}
    >
      <span style={{ color: selected ? "var(--text)" : "var(--subtext0)" }}>
        {highlighted.length > 0 ? highlighted : basename}
      </span>
      <span style={{ color: "var(--overlay0)", fontSize: 11 }}>{dir}</span>
    </div>
  );
}
```

- [ ] **Step 2: Add QuickOpen to ScopeLayout**

In `frontend/src/components/ScopeMode/ScopeLayout.tsx`:

Add import at the top:

```typescript
import { QuickOpen } from "./QuickOpen"
import { api } from "../../lib/wails"
```

Add store selectors for `setFileList`:

```typescript
const setFileList = useSessionStore((s) => s.setFileList);
```

Add a `useEffect` to load the file list and start the indexer on scope entry:

```typescript
useEffect(() => {
  if (!sessionId) return;

  // Load file list
  api().ListFiles(sessionId).then((files) => {
    if (files) setFileList(files);
  });

  // Start indexer
  api().IndexSession(sessionId);

  // Refresh file list periodically
  const interval = setInterval(() => {
    api().ListFiles(sessionId).then((files) => {
      if (files) setFileList(files);
    });
  }, 10000);

  return () => {
    clearInterval(interval);
    api().StopIndexer(sessionId);
  };
}, [sessionId, setFileList]);
```

Render the `QuickOpen` component inside the top-level div, before or after the Allotment:

```tsx
{sessionId && <QuickOpen sessionId={sessionId} />}
```

- [ ] **Step 3: Wire Cmd+P hotkey**

In `frontend/src/hooks/useHotkeys.ts`, add a new hotkey case inside the existing keydown handler (in the `Ctrl/Cmd + Shift` block):

```typescript
// Add after the existing 'S' case for scope mode (around line 74):
case "p":
case "P": {
  e.preventDefault();
  const { scopeMode, toggleQuickOpen } = useSessionStore.getState();
  if (scopeMode.active) {
    toggleQuickOpen();
  }
  break;
}
```

Note: Check the exact hotkey pattern used. If the existing hotkeys use `e.metaKey` (Cmd on Mac) without Shift, match that pattern. If they use `Ctrl+Shift`, use the same. The key combo should be `Cmd+P` (no Shift) since that's the VS Code convention. Look at how other hotkeys are structured and match accordingly — you may need to add a separate block outside the `Ctrl+Shift` section for `Cmd+P` (meta key only, no shift).

- [ ] **Step 4: Verify Cmd+F passthrough**

Check if `useHotkeys.ts` currently intercepts `Cmd+F`. If it does, add a guard to skip it when scope mode is active and the editor is focused. If it doesn't intercept `Cmd+F`, no change is needed — Monaco handles it natively.

- [ ] **Step 5: Build and verify**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && wails dev`

Test manually:
1. Enter scope mode on a session
2. Press `Cmd+P` — Quick Open overlay should appear
3. Type a filename — results should filter with fuzzy matching and highlighted characters
4. Arrow keys navigate, Enter opens file, Escape closes
5. Click outside closes
6. `Cmd+F` in the editor should open Monaco's find widget

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ScopeMode/QuickOpen.tsx frontend/src/components/ScopeMode/ScopeLayout.tsx frontend/src/hooks/useHotkeys.ts
git commit -m "feat(scope): add Quick Open overlay with fuzzy file search (Cmd+P)"
```

---

### Task 7: Frontend — Monaco DefinitionProvider

**Files:**
- Create: `frontend/src/lib/definitionProvider.ts`
- Modify: `frontend/src/components/ScopeMode/EditorPane.tsx`

- [ ] **Step 1: Create definitionProvider.ts**

Create `frontend/src/lib/definitionProvider.ts`:

```typescript
import type * as Monaco from "monaco-editor";
import { api } from "./wails";
import type { SymbolDefinition } from "./wails";
import { useSessionStore, detectLanguage } from "../store/sessionStore";

const SUPPORTED_LANGUAGES = [
  "go",
  "typescript",
  "javascript",
  "typescriptreact",
  "javascriptreact",
  "c",
  "cpp",
];

/**
 * Register a DefinitionProvider for all supported languages.
 * Call this once when the Monaco editor mounts in scope mode.
 * Returns a disposable to clean up on unmount.
 */
export function registerDefinitionProvider(
  monaco: typeof Monaco,
  sessionId: string
): Monaco.IDisposable {
  const provider: Monaco.languages.DefinitionProvider = {
    provideDefinition: async (
      model,
      position
    ): Promise<Monaco.languages.Definition | null> => {
      const word = model.getWordAtPosition(position);
      if (!word) return null;

      let defs: SymbolDefinition[];
      try {
        defs = await api().LookupSymbol(sessionId, word.word);
      } catch {
        return null;
      }

      if (!defs || defs.length === 0) return null;

      const locations: Monaco.languages.Location[] = [];
      for (const def of defs) {
        // Ensure the target file has a model so Monaco can display it
        const uri = monaco.Uri.file(def.path);
        let targetModel = monaco.editor.getModel(uri);
        if (!targetModel) {
          try {
            const contents = await api().ReadFile(sessionId, def.path);
            const lang = detectLanguage(def.path);
            targetModel = monaco.editor.createModel(contents, lang, uri);
          } catch {
            continue;
          }
        }

        locations.push({
          uri,
          range: new monaco.Range(def.line, 1, def.line, 1),
        });
      }

      return locations.length > 0 ? locations : null;
    },
  };

  const disposables = SUPPORTED_LANGUAGES.map((lang) =>
    monaco.languages.registerDefinitionProvider(lang, provider)
  );

  return {
    dispose: () => disposables.forEach((d) => d.dispose()),
  };
}

/**
 * Handle navigation to a definition.
 * Opens the target file in the editor tab bar if it's not already open.
 */
export function handleDefinitionNavigation(
  uri: Monaco.Uri,
  sessionId: string
): void {
  const path = uri.path.startsWith("/") ? uri.path.slice(1) : uri.path;
  const store = useSessionStore.getState();
  const existing = store.openEditorFiles.find((f) => f.path === path);
  if (!existing) {
    // File will be opened by the DefinitionProvider's model creation
    // Just need to add it to the editor tabs
    const model =
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (globalThis as any).monaco?.editor?.getModel?.(uri);
    if (model) {
      store.openEditorFile(path, model.getValue(), detectLanguage(path));
    }
  } else {
    store.setActiveEditorFile(path);
  }
}
```

- [ ] **Step 2: Integrate DefinitionProvider into EditorPane**

In `frontend/src/components/ScopeMode/EditorPane.tsx`:

Add import:

```typescript
import { registerDefinitionProvider } from "../../lib/definitionProvider"
```

Add a ref to track the disposable:

```typescript
const definitionProviderRef = useRef<{ dispose: () => void } | null>(null);
```

In the `handleMount` callback (the `OnMount` handler), after the theme setup, register the definition provider:

```typescript
const handleMount: OnMount = (editor, monaco) => {
  monaco.editor.defineTheme("catppuccin-mocha", catppuccinMocha as any);
  monaco.editor.setTheme("catppuccin-mocha");

  // Register definition provider for go-to-definition
  if (definitionProviderRef.current) {
    definitionProviderRef.current.dispose();
  }
  definitionProviderRef.current = registerDefinitionProvider(monaco, sessionId);

  // Handle go-to-definition navigation — open file in tab
  editor.onDidChangeModel(() => {
    const model = editor.getModel();
    if (model) {
      const path = model.uri.path.startsWith("/")
        ? model.uri.path.slice(1)
        : model.uri.path;
      const store = useSessionStore.getState();
      const existing = store.openEditorFiles.find((f) => f.path === path);
      if (existing) {
        store.setActiveEditorFile(path);
      }
    }
  });
};
```

Add cleanup in the existing `useEffect` cleanup:

```typescript
useEffect(() => {
  return () => {
    // ... existing cleanup ...
    if (definitionProviderRef.current) {
      definitionProviderRef.current.dispose();
      definitionProviderRef.current = null;
    }
  };
}, []);
```

Update the Monaco `Editor` component options to enable go-to-definition mouse behavior:

```typescript
// In the options prop, add:
gotoLocation: {
  multiple: "peek",
  multipleDefinitions: "peek",
},
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && wails dev`

Test manually:
1. Enter scope mode on a session with Go or TypeScript files
2. Open a file that calls a function defined in another file
3. `Cmd+Click` on the function name — should open the definition file in a new tab
4. `Alt+Click` on a function name — should show inline peek widget
5. If multiple definitions exist, Monaco should show a picker

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/definitionProvider.ts frontend/src/components/ScopeMode/EditorPane.tsx
git commit -m "feat(scope): add ctags-powered go-to-definition with Cmd+Click and Alt+Click peek"
```

---

### Task 8: Integration Testing and Polish

**Files:**
- Various (fixes found during testing)

- [ ] **Step 1: Run full backend test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Build the full app**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && wails build`
Expected: Build succeeds, `build/bin/claude-squad.app` produced

- [ ] **Step 3: Manual integration test**

Test the following scenarios:
1. **Quick Open**: Enter scope mode → `Cmd+P` → type partial filename → verify results → Enter to open → file appears in editor tab
2. **Find/Replace**: Open a file in scope mode → `Cmd+F` → type search term → verify Monaco find widget appears and works
3. **Go-to-Definition (open tab)**: Open a Go file → `Cmd+Click` a function call → verify definition opens in new tab at correct line
4. **Go-to-Definition (peek)**: Open a Go file → `Alt+Click` a function call → verify inline peek widget shows definition
5. **Periodic refresh**: Open scope mode → create a new file via Claude terminal → wait 10s → `Cmd+P` → verify new file appears in results
6. **Graceful degradation**: If ctags not installed, verify scope mode still works (Quick Open and Find work, Cmd+Click silently does nothing)
7. **Scope exit cleanup**: Exit scope mode → verify no lingering timers or indexers

- [ ] **Step 4: Fix any issues found**

Address any bugs discovered during integration testing.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(scope): polish and integration fixes for VS Code-like features"
```
