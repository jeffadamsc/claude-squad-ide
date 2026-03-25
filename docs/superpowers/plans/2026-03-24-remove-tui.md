# Remove TUI, Make GUI the Default Entry Point

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the bubbletea TUI and make the Fyne GUI the default `cs` command.

**Architecture:** Delete `app/`, `ui/`, `keys/` directories (TUI code). Move GUI launch logic from the `guiCmd` subcommand into the root command's `RunE`. Remove unused charmbracelet dependencies.

**Tech Stack:** Go, Fyne GUI, cobra CLI

---

### Task 1: Update main.go — Make GUI the default command

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Edit main.go — replace root command body with GUI launch, remove gui subcommand**

Replace the root command's `RunE` with the GUI launch logic (currently in `guiCmd`). Remove the `guiCmd` definition entirely. Remove the `rootCmd.AddCommand(guiCmd)` line from `init()`. Remove the `"claude-squad/app"` import.

The new root `RunE` should be:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    log.Initialize(daemonFlag)
    defer log.Close()

    if daemonFlag {
        cfg := config.LoadConfig()
        err := daemon.RunDaemon(cfg)
        log.ErrorLog.Printf("failed to start daemon %v", err)
        return err
    }

    currentDir, err := filepath.Abs(".")
    if err != nil {
        return fmt.Errorf("failed to get current directory: %w", err)
    }

    if !git.IsGitRepo(currentDir) {
        return fmt.Errorf("error: claude-squad must be run from within a git repository")
    }

    cfg := config.LoadConfig()

    program := cfg.GetProgram()
    if programFlag != "" {
        program = programFlag
    }
    autoYes := cfg.AutoYes
    if autoYesFlag {
        autoYes = true
    }

    return gui.Run(program, autoYes)
},
```

Remove these lines from `init()`:
```go
rootCmd.AddCommand(guiCmd)
```

Remove the entire `guiCmd` variable block.

Remove unused imports: `"claude-squad/app"`, `"context"`, `"encoding/json"`. Keep `"claude-squad/gui"`.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: success, no errors

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: make GUI the default entry point, remove gui subcommand"
```

### Task 2: Delete TUI directories

**Files:**
- Delete: `app/` (entire directory)
- Delete: `ui/` (entire directory)
- Delete: `keys/` (entire directory)

- [ ] **Step 1: Delete the three TUI directories**

```bash
rm -rf app/ ui/ keys/
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: success, no errors

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove TUI code (app/, ui/, keys/)"
```

### Task 3: Remove unused charmbracelet dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Run go mod tidy to remove unused deps**

```bash
cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go mod tidy
```

This should remove: `github.com/charmbracelet/bubbles`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/mattn/go-runewidth`, and their indirect deps (`charmbracelet/x/ansi`, `charmbracelet/x/term`).

- [ ] **Step 2: Verify it compiles and tests pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./... && go test ./...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: remove unused charmbracelet dependencies"
```
