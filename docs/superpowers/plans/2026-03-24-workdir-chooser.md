# Working Directory Chooser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a working directory chooser to the GUI session creation dialog, with persistent preference.

**Architecture:** Add `DefaultWorkDir` to Config for persistence. Extend the new-session dialog with a directory picker (Fyne folder dialog) that triggers branch re-fetching. Pass the selected directory through to `session.NewInstance` instead of hardcoded `"."`.

**Tech Stack:** Go, Fyne v2.7.3 (`dialog.NewFolderOpen`, `storage.NewFileURI`, `storage.ListerForURI`)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `config/config.go` | Modify | Add `DefaultWorkDir` field to `Config` struct |
| `gui/dialogs/new_session.go` | Modify | Add directory picker UI, `WorkDir` to `SessionOptions`, `onDirChanged` callback |
| `gui/app.go` | Modify | Wire `onDirChanged` callback, pass `WorkDir` to instance, save preference |

---

### Task 1: Add DefaultWorkDir to Config

**Files:**
- Modify: `config/config.go:36-47` (Config struct)

- [ ] **Step 1: Add field to Config struct**

In `config/config.go`, add `DefaultWorkDir` to the `Config` struct:

```go
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// Profiles is a list of named program profiles.
	Profiles []Profile `json:"profiles,omitempty"`
	// DefaultWorkDir is the last-used working directory for new sessions.
	// Empty string means current directory.
	DefaultWorkDir string `json:"default_work_dir,omitempty"`
}
```

- [ ] **Step 2: Commit**

```bash
git add config/config.go
git commit -m "feat(config): add DefaultWorkDir field for persistent working directory preference"
```

---

### Task 2: Add WorkDir to SessionOptions and directory picker UI

**Files:**
- Modify: `gui/dialogs/new_session.go`

- [ ] **Step 1: Add WorkDir to SessionOptions**

```go
type SessionOptions struct {
	Name    string
	Prompt  string
	Program string
	Branch  string // empty = new branch from default
	InPlace bool
	WorkDir string // working directory for the session
}
```

- [ ] **Step 2: Add DirChangeResult type and update ShowNewSession signature**

Add a result type for the directory change callback and add new parameters to `ShowNewSession`:

```go
// DirChangeResult holds the branch data returned when the working directory changes.
type DirChangeResult struct {
	DefaultBranch string
	Branches      []string
}
```

Update the function signature to accept `initialWorkDir string` and `onDirChanged func(dir string) DirChangeResult`:

```go
func ShowNewSession(
	profiles []config.Profile,
	defaultBranch string,
	branches []string,
	initialWorkDir string,
	parent fyne.Window,
	onBranchSearch func(dir string, filter string) []string,
	onDirChanged func(dir string) DirChangeResult,
	onSubmit func(SessionOptions),
)
```

- [ ] **Step 3: Declare shared mutable state and branch picker (forward declarations)**

Declare `branchSelect` and mutable label variable before the directory picker so closures can reference them:

```go
// Mutable state shared across closures
selectedDir := initialWorkDir
currentDefaultBranch := defaultBranch
newBranchLabel := fmt.Sprintf("New branch (from %s)", currentDefaultBranch)

// Branch picker (declared early so directory picker closure can update it)
branchOptions := append([]string{newBranchLabel}, branches...)
branchSelect := widget.NewSelect(branchOptions, nil)
branchSelect.SetSelected(newBranchLabel)
```

- [ ] **Step 4: Add directory picker widgets**

After the shared state declarations, add:

```go
// Working directory picker
dirLabel := widget.NewLabel(displayPath(initialWorkDir))
dirLabel.Wrapping = fyne.TextTruncate

dirBrowseBtn := widget.NewButton("Browse...", func() {
	folderDialog := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		selectedDir = uri.Path()
		dirLabel.SetText(displayPath(selectedDir))

		// Re-fetch branches for the new directory
		if onDirChanged != nil {
			result := onDirChanged(selectedDir)
			currentDefaultBranch = result.DefaultBranch
			newBranchLabel = fmt.Sprintf("New branch (from %s)", currentDefaultBranch)
			newOptions := append([]string{newBranchLabel}, result.Branches...)
			branchSelect.Options = newOptions
			branchSelect.SetSelected(newBranchLabel)
			branchSelect.Refresh()
		}
	}, parent)

	// Set initial location for the folder dialog
	if selectedDir != "" {
		uri := storage.NewFileURI(selectedDir)
		lister, err := storage.ListerForURI(uri)
		if err == nil {
			folderDialog.SetLocation(lister)
		}
	}
	folderDialog.Show()
})

dirContainer := container.NewBorder(nil, nil, nil, dirBrowseBtn, dirLabel)
```

- [ ] **Step 5: Update branch search callback to pass directory**

```go
branchSearch.OnChanged = func(filter string) {
	if onBranchSearch == nil {
		return
	}
	filtered := onBranchSearch(selectedDir, filter)
	newOptions := append([]string{newBranchLabel}, filtered...)
	branchSelect.Options = newOptions
	branchSelect.Refresh()
}
```

- [ ] **Step 6: Add Working Directory form item and set WorkDir in submit**

Add the form item to the `items` slice:

```go
items := []*widget.FormItem{
	widget.NewFormItem("Name", nameEntry),
	widget.NewFormItem("Directory", dirContainer),
	widget.NewFormItem("In-place", inPlaceCheck),
	branchFormItem,
	widget.NewFormItem("Prompt", promptEntry),
}
```

In the submit callback, set `WorkDir`:

```go
opts := SessionOptions{
	Name:    nameEntry.Text,
	Prompt:  promptEntry.Text,
	InPlace: inPlaceCheck.Checked,
	WorkDir: selectedDir,
}
```

- [ ] **Step 7: Add displayPath helper and storage import**

Add at the top of the file:

```go
import (
	"claude-squad/config"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)
```

Add helper at the bottom of the file:

```go
// displayPath returns a user-friendly display string for a directory path.
// Replaces home directory prefix with "~" and returns "(current directory)" for empty.
func displayPath(dir string) string {
	if dir == "" {
		return "(current directory)"
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return dir
}
```

- [ ] **Step 8: Commit**

```bash
git add gui/dialogs/new_session.go
git commit -m "feat(gui): add working directory chooser to new session dialog"
```

---

### Task 3: Wire directory chooser in app.go and persist preference

**Files:**
- Modify: `gui/app.go:311-372` (showNewSessionDialog function)

- [ ] **Step 1: Update showNewSessionDialog to pass initial work dir and callbacks**

```go
func showNewSessionDialog(w fyne.Window, cfg *config.Config, defaultProgram string, state *guiState, sb *sidebar.Sidebar, pm *panes.Manager, autoYes bool) {
	// Use saved work dir or current directory
	initialDir := cfg.DefaultWorkDir
	gitDir := initialDir
	if gitDir == "" {
		gitDir = "."
	}

	defaultBranch := git.GetDefaultBranch(gitDir)
	branches, _ := git.SearchBranches(gitDir, "")

	dialogs.ShowNewSession(cfg.GetProfiles(), defaultBranch, branches, initialDir, w,
		func(dir string, filter string) []string {
			searchDir := dir
			if searchDir == "" {
				searchDir = "."
			}
			results, _ := git.SearchBranches(searchDir, filter)
			return results
		},
		func(dir string) dialogs.DirChangeResult {
			defBranch := git.GetDefaultBranch(dir)
			if defBranch == "" {
				defBranch = "main"
			}
			branches, _ := git.SearchBranches(dir, "")
			return dialogs.DirChangeResult{
				DefaultBranch: defBranch,
				Branches:      branches,
			}
		},
		func(opts dialogs.SessionOptions) {
			if opts.Name == "" {
				return
			}
			prog := opts.Program
			if prog == "" {
				prog = defaultProgram
			}

			path := opts.WorkDir
			if path == "" {
				path = "."
			}

			inst, err := session.NewInstance(session.InstanceOptions{
				Title:   opts.Name,
				Path:    path,
				Program: prog,
				InPlace: opts.InPlace,
			})
			if err != nil {
				log.ErrorLog.Printf("failed to create instance: %v", err)
				return
			}
			inst.AutoYes = autoYes
			inst.Prompt = opts.Prompt
			if opts.Branch != "" {
				inst.SetSelectedBranch(opts.Branch)
			}
			inst.SetStatus(session.Loading)
			state.addInstance(inst)
			sb.Update(state.getInstances())

			// Save working directory preference
			cfg.DefaultWorkDir = opts.WorkDir
			if err := config.SaveConfig(cfg); err != nil {
				log.ErrorLog.Printf("failed to save config: %v", err)
			}

			go func() {
				if err := inst.Start(true); err != nil {
					log.ErrorLog.Printf("failed to start instance: %v", err)
					state.removeInstance(inst.Title)
					fyne.Do(func() {
						sb.Update(state.getInstances())
						dialogs.ShowError("Failed to Start Session",
							fmt.Sprintf("Could not start session '%s': %v", opts.Name, err), w)
					})
					return
				}
				if opts.Prompt != "" {
					if err := inst.SendPrompt(opts.Prompt); err != nil {
						log.ErrorLog.Printf("failed to send prompt: %v", err)
					}
					inst.Prompt = ""
				}
				fyne.Do(func() {
					sb.Update(state.getInstances())
				})
				if err := state.storage.SaveInstances(state.getInstances()); err != nil {
					log.ErrorLog.Printf("failed to save instances: %v", err)
				}
			}()
		})
}
```

- [ ] **Step 2: Commit**

```bash
git add gui/app.go
git commit -m "feat(gui): wire working directory chooser and persist preference"
```

---

### Task 4: Manual verification

- [ ] **Step 1: Build and verify**

```bash
go build ./...
```

- [ ] **Step 2: Run the GUI and test**

Launch `cs gui`, create a new session, verify:
1. Directory field shows "(current directory)" by default (or saved preference)
2. Browse button opens native folder picker
3. Selecting a directory updates the label
4. Branch dropdown refreshes with branches from the new directory
5. Creating a session uses the selected directory
6. Re-opening the dialog shows the previously selected directory

- [ ] **Step 3: Final commit if any fixes needed**
