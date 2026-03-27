# Scope Mode: VS Code-Like Features

**Date:** 2026-03-26
**Status:** Approved

## Overview

Add three VS Code-inspired features to scope mode: Quick Open file search, Monaco find/replace, and ctags-powered go-to-definition. These make scope mode a more capable code navigation environment, especially important when Claude is actively editing files in a session's worktree.

## Features

### 1. Quick Open (Cmd+P)

A command palette overlay for fuzzy file search across the entire worktree.

**Frontend — `QuickOpen.tsx`:**
- Modal overlay centered horizontally at the top of the scope layout
- Search input with magnifying glass icon, auto-focused on open
- Results list showing filename (with fuzzy match highlights) and relative directory path
- First result auto-selected; arrow keys navigate; Enter opens file in editor tab; Escape closes
- Click outside to dismiss
- Maximum ~20 visible results, scrollable

**Backend — `ListFiles(sessionId) → string[]`:**
- Runs `git ls-files` in the session's worktree (local) or over SSH (remote)
- Returns flat list of relative file paths
- Called on scope mode entry, result cached in Zustand store
- Refreshed by periodic timer (see Shared Infrastructure)

**Fuzzy matching — `lib/fuzzyMatch.ts`:**
- Client-side filtering against cached file list (no backend round-trips while typing)
- Match anywhere in the full path, prefer filename matches over directory matches
- Scoring: consecutive character matches score higher than spread matches; filename matches score higher than path-only matches
- Highlight matched characters in the result display
- Implement as a lightweight ~50-line scorer (no npm dependency needed)

### 2. Find / Replace (Cmd+F)

Monaco Editor's built-in find/replace widget. Already functional within the editor.

**Work required:**
- Ensure `Cmd+F` is not intercepted by the app's global hotkey handler (`useHotkeys`) when the Monaco editor is focused
- Monaco's `editor.action.startFindAction` should fire natively
- No custom UI or backend changes needed

### 3. Go-to-Definition (Cmd+Click / Alt+Click)

Navigate from a symbol usage to its definition using a ctags-built index.

**Frontend — `lib/definitionProvider.ts`:**
- Register a Monaco `DefinitionProvider` for languages: `go`, `typescript`, `javascript`, `typescriptreact`, `javascriptreact`, `c`, `cpp`
- On trigger: extract the word at the cursor position, call `LookupSymbol(sessionId, word)`
- Single result: navigate directly
- Multiple results: Monaco shows its built-in picker
- `Cmd+Click` triggers `editor.action.revealDefinition` — opens the target file in a new editor tab, scrolled to the definition line
- `Alt+Click` triggers `editor.action.peekDefinition` — shows an inline peek widget with the definition context, plus open-in-tab and close buttons
- When navigating to a file not yet open, call `ReadFile()` to fetch contents and add to editor tabs

**Backend — `app/indexer.go` (new file):**

`IndexSession(sessionId)`:
- Runs `ctags --output-format=json -R --languages=Go,C,C++,JavaScript,TypeScript .` in the session's worktree
- For remote sessions: runs the same command over SSH
- Parses JSON output into an in-memory symbol table: `map[string][]Definition`
- Each `Definition`: `{Name, File, Line, Kind, Scope, Language}`
- Called on scope mode entry, refreshed by periodic timer

`LookupSymbol(sessionId, symbol) → []Definition`:
- Wails-bound method callable from the frontend
- Looks up the symbol name in the in-memory index
- Returns all matching definitions (may be multiple for overloaded names or same-named symbols across packages)

**Graceful degradation:**
- If `ctags` is not installed on the host (or remote machine), `IndexSession` returns an empty index
- Go-to-definition silently produces no results — no errors shown to the user
- Quick Open and Find/Replace work independently of ctags

## Shared Infrastructure

### Periodic Refresh Timer

A single Go-side timer per scoped session, managed in `app/indexer.go`:

- **Tick interval:** 10 seconds
- **On each tick:**
  1. Re-run `git ls-files` → update cached file list
  2. Re-run `ctags` → rebuild symbol index
- **Lifecycle:** Timer starts when `enterScopeMode` is called for a session, stops when scope mode is exited
- **Eager refresh:** Also triggered immediately after any `WriteFile()` call, debounced to avoid rapid successive rebuilds
- **Frontend notification:** After each refresh, the frontend polls or is notified to update the cached file list in the Zustand store. Simplest approach: frontend calls `ListFiles()` on a matching interval, or backend emits a Wails event

### Monaco Integration Pattern

The `DefinitionProvider` registration happens in `EditorPane.tsx` when the Monaco editor mounts. The provider is registered once and applies to all supported languages. It calls the backend `LookupSymbol` binding asynchronously and returns Monaco `Location` objects pointing to the resolved file URIs.

For files not yet loaded in the editor, the provider calls `ReadFile()` to fetch contents, creates a Monaco model for the file, and returns a location within that model. This enables peek to show the definition content without opening a full tab.

## New Files

| File | Type | Purpose |
|------|------|---------|
| `frontend/src/components/ScopeMode/QuickOpen.tsx` | Frontend | Quick Open overlay component |
| `frontend/src/lib/fuzzyMatch.ts` | Frontend | Fuzzy file matching scorer |
| `frontend/src/lib/definitionProvider.ts` | Frontend | Monaco DefinitionProvider registration |
| `app/indexer.go` | Backend | ctags runner, symbol table, periodic refresh timer |

## Modified Files

| File | Changes |
|------|---------|
| `app/bindings.go` | Add `ListFiles()`, `LookupSymbol()` bindings; trigger eager refresh in `WriteFile()` |
| `frontend/src/store/sessionStore.ts` | Add `fileList: string[]`, `setFileList()`, `quickOpenVisible` state |
| `frontend/src/components/ScopeMode/ScopeLayout.tsx` | Render `QuickOpen` overlay; wire `Cmd+P` hotkey |
| `frontend/src/components/ScopeMode/EditorPane.tsx` | Register `DefinitionProvider` on mount; handle definition navigation |
| `frontend/src/hooks/useHotkeys.ts` | Ensure `Cmd+F` passes through to Monaco when editor is focused |

## Dependencies

- **Runtime:** `universal-ctags` must be installed on the host machine (`brew install universal-ctags` on macOS). For remote SSH sessions, ctags must be available on the remote.
- **Frontend:** No new npm packages. Fuzzy matching is a small custom implementation. Monaco APIs (`registerDefinitionProvider`, peek/reveal actions) are built-in.

## Error Handling

- **ctags not installed:** `IndexSession` detects missing binary, logs a warning, returns empty index. No user-facing error.
- **ctags fails on a file:** ctags skips unparseable files by default. Index contains whatever succeeded.
- **Large repos:** ctags is fast (indexes Linux kernel in ~10s). The 10s refresh interval is conservative; for very large repos, indexing may overlap with the next tick — use a mutex to skip overlapping runs.
- **Remote session SSH errors:** Same pattern as existing `ListDirectory`/`ReadFile` — return empty results, log error.
- **Binary files in git ls-files:** Already filtered by existing `.gitignore` patterns. No additional filtering needed.
