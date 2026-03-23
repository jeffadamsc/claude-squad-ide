# Base Branch Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to choose origin/main or origin/master as the base branch when creating new sessions, with auto-detection for submodules.

**Architecture:** Add `detectDefaultRemoteBranch()` helper in worktree_git.go, update branch picker to show dynamic new-branch options, modify `setupNewWorktree()` to accept a base ref and fetch before branching, and wire the selection through Instance to GitWorktree.

**Tech Stack:** Go, bubbletea TUI framework, git CLI

**Spec:** `docs/superpowers/specs/2026-03-23-base-branch-selection-design.md`

---

### Task 1: Add `detectDefaultRemoteBranch()` helper

**Files:**
- Modify: `session/git/worktree_git.go:1-53`
- Create: `session/git/worktree_git_test.go`

- [ ] **Step 1: Write the failing test**

```go
// In session/git/worktree_git_test.go
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupBareRemote(t *testing.T, defaultBranch string) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runCmd(t, "", "git", "init", "--bare", "--initial-branch="+defaultBranch, remote)
	return remote
}

func setupRepoWithRemote(t *testing.T, remoteBranch string) string {
	t.Helper()
	remote := setupBareRemote(t, remoteBranch)
	repo := filepath.Join(t.TempDir(), "repo")
	runCmd(t, "", "git", "clone", remote, repo)
	// Create initial commit so remote branch exists
	writeFile(t, filepath.Join(repo, "README.md"), "init")
	runCmd(t, repo, "git", "add", ".")
	runCmd(t, repo, "git", "commit", "-m", "init")
	runCmd(t, repo, "git", "push", "origin", remoteBranch)
	return repo
}

func TestDetectDefaultRemoteBranch_Main(t *testing.T) {
	repo := setupRepoWithRemote(t, "main")
	branches := DetectDefaultRemoteBranches(repo)
	if len(branches) != 1 || branches[0] != "origin/main" {
		t.Errorf("expected [origin/main], got %v", branches)
	}
}

func TestDetectDefaultRemoteBranch_Master(t *testing.T) {
	repo := setupRepoWithRemote(t, "master")
	branches := DetectDefaultRemoteBranches(repo)
	if len(branches) != 1 || branches[0] != "origin/master" {
		t.Errorf("expected [origin/master], got %v", branches)
	}
}

func TestDetectDefaultRemoteBranch_Neither(t *testing.T) {
	repo := setupRepoWithRemote(t, "develop")
	branches := DetectDefaultRemoteBranches(repo)
	if len(branches) != 0 {
		t.Errorf("expected empty, got %v", branches)
	}
}

func TestDetectDefaultRemoteBranch_Both(t *testing.T) {
	repo := setupRepoWithRemote(t, "main")
	// Create a master branch too
	runCmd(t, repo, "git", "checkout", "-b", "master")
	runCmd(t, repo, "git", "push", "origin", "master")
	runCmd(t, repo, "git", "checkout", "main")
	branches := DetectDefaultRemoteBranches(repo)
	if len(branches) != 2 || branches[0] != "origin/main" || branches[1] != "origin/master" {
		t.Errorf("expected [origin/main, origin/master], got %v", branches)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run TestDetectDefaultRemoteBranch -v`
Expected: FAIL with "DetectDefaultRemoteBranches not defined"

- [ ] **Step 3: Write minimal implementation**

Add to `session/git/worktree_git.go` after the `SearchBranches` function:

```go
// DetectDefaultRemoteBranches checks which of origin/main and origin/master exist
// in the given repo. Returns a slice in display order (main first, then master).
// Uses git rev-parse --verify for clean existence checks with no output parsing.
func DetectDefaultRemoteBranches(repoPath string) []string {
	var branches []string
	for _, ref := range []string{"origin/main", "origin/master"} {
		cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", ref)
		if err := cmd.Run(); err == nil {
			branches = append(branches, ref)
		}
	}
	return branches
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run TestDetectDefaultRemoteBranch -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add session/git/worktree_git.go session/git/worktree_git_test.go
git commit -m "feat: add DetectDefaultRemoteBranches helper"
```

---

### Task 2: Update branch picker with dynamic new-branch options

**Files:**
- Modify: `ui/overlay/branchPicker.go:1-212`
- Modify: `ui/overlay/textInput_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `ui/overlay/textInput_test.go`:

```go
func TestBranchPicker_IsNewBranch(t *testing.T) {
	bp := NewBranchPicker()
	// Default cursor is on the first item (HEAD option), should be new branch
	if !bp.IsNewBranch() {
		t.Error("expected IsNewBranch() true when on HEAD option")
	}
	if bp.BaseBranch() != "HEAD" {
		t.Errorf("expected BaseBranch() = HEAD, got %s", bp.BaseBranch())
	}
}

func TestBranchPicker_NewBranchOptionsWithOrigin(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main"})
	items := bp.visibleItems()
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	if items[0] != "New branch (from origin/main)" {
		t.Errorf("expected first item to be origin/main option, got %s", items[0])
	}
	if items[1] != "New branch (from HEAD)" {
		t.Errorf("expected second item to be HEAD option, got %s", items[1])
	}
	// Cursor at 0 should be origin/main
	if bp.BaseBranch() != "origin/main" {
		t.Errorf("expected BaseBranch() = origin/main, got %s", bp.BaseBranch())
	}
}

func TestBranchPicker_NewBranchOptionsWithBoth(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main", "origin/master"})
	items := bp.visibleItems()
	if len(items) < 3 {
		t.Fatalf("expected at least 3 items, got %d", len(items))
	}
	if items[0] != "New branch (from origin/main)" {
		t.Errorf("expected first = origin/main option, got %s", items[0])
	}
	if items[1] != "New branch (from origin/master)" {
		t.Errorf("expected second = origin/master option, got %s", items[1])
	}
	if items[2] != "New branch (from HEAD)" {
		t.Errorf("expected third = HEAD option, got %s", items[2])
	}
}

func TestBranchPicker_ExistingBranchNotNewBranch(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetResults([]string{"feature/foo"}, 0)
	// Move cursor to the existing branch (index 1, after HEAD)
	bp.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if bp.IsNewBranch() {
		t.Error("expected IsNewBranch() false when on existing branch")
	}
	if bp.GetSelectedBranch() != "feature/foo" {
		t.Errorf("expected GetSelectedBranch() = feature/foo, got %s", bp.GetSelectedBranch())
	}
}

func TestBranchPicker_FilterHidesNewBranchOnExactMatch(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main"})
	bp.filter = "feature/foo"
	bp.SetResults([]string{"feature/foo"}, bp.filterVersion)
	items := bp.visibleItems()
	// When filter exactly matches a branch, new-branch options should be hidden
	for _, item := range items {
		if len(item) >= 10 && item[:10] == "New branch" {
			t.Errorf("expected no New branch options when filter matches exactly, got %s", item)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ui/overlay/ -run 'TestBranchPicker_(IsNewBranch|NewBranchOptions|ExistingBranch|FilterHides)' -v`
Expected: FAIL (methods don't exist yet)

- [ ] **Step 3: Implement branch picker changes**

Replace the `newBranchOption` constant and update `BranchPicker` in `ui/overlay/branchPicker.go`:

```go
const newBranchFromHEAD = "New branch (from HEAD)"

type BranchPicker struct {
	results          []string // current search results (from git)
	filter           string   // current filter text
	filterVersion    uint64   // incremented on each filter change
	cursor           int      // index into visibleItems()
	focused          bool
	width            int
	showNewBranch    bool     // whether to show the "New branch" options
	newBranchOptions []string // e.g. ["origin/main", "origin/master"] detected from remote
}
```

Add `SetNewBranchOptions`:

```go
// SetNewBranchOptions sets the detected remote default branches (e.g. ["origin/main"]).
// These are shown as "New branch (from origin/main)" etc. before the HEAD option.
func (bp *BranchPicker) SetNewBranchOptions(remoteBranches []string) {
	bp.newBranchOptions = remoteBranches
}
```

Update `visibleItems()`:

```go
func (bp *BranchPicker) visibleItems() []string {
	var items []string
	if bp.showNewBranch {
		for _, ref := range bp.newBranchOptions {
			items = append(items, "New branch (from "+ref+")")
		}
		items = append(items, newBranchFromHEAD)
	}
	items = append(items, bp.results...)
	return items
}
```

Add `IsNewBranch()` and `BaseBranch()`:

```go
// IsNewBranch returns true if the currently selected item is any "New branch" option.
func (bp *BranchPicker) IsNewBranch() bool {
	items := bp.visibleItems()
	if bp.cursor < 0 || bp.cursor >= len(items) {
		return true // default to new branch
	}
	return strings.HasPrefix(items[bp.cursor], "New branch")
}

// BaseBranch returns the base ref for the selected new-branch option.
// Returns "origin/main", "origin/master", or "HEAD".
// Only meaningful when IsNewBranch() is true.
func (bp *BranchPicker) BaseBranch() string {
	items := bp.visibleItems()
	if bp.cursor < 0 || bp.cursor >= len(items) {
		return "HEAD"
	}
	selected := items[bp.cursor]
	for _, ref := range bp.newBranchOptions {
		if selected == "New branch (from "+ref+")" {
			return ref
		}
	}
	return "HEAD"
}
```

Update `GetSelectedBranch()` to use the new constant:

```go
func (bp *BranchPicker) GetSelectedBranch() string {
	items := bp.visibleItems()
	if bp.cursor < 0 || bp.cursor >= len(items) {
		return ""
	}
	selected := items[bp.cursor]
	if strings.HasPrefix(selected, "New branch") {
		return ""
	}
	return selected
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./ui/overlay/ -v`
Expected: PASS (all existing + new tests)

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/branchPicker.go ui/overlay/textInput_test.go
git commit -m "feat: add dynamic new-branch options to branch picker"
```

---

### Task 3: Add `baseBranch` field to GitWorktree and update `setupNewWorktree()`

**Files:**
- Modify: `session/git/worktree.go:21-95`
- Modify: `session/git/worktree_ops.go:68-97`
- Modify: `session/git/worktree_git_test.go`

- [ ] **Step 1: Write the failing test**

Add to `session/git/worktree_git_test.go`:

```go
func TestSetupNewWorktree_FromOriginMain(t *testing.T) {
	repo := setupRepoWithRemote(t, "main")
	// Make a local commit so HEAD diverges from origin/main
	writeFile(t, filepath.Join(repo, "local.txt"), "local change")
	runCmd(t, repo, "git", "add", ".")
	runCmd(t, repo, "git", "commit", "-m", "local divergence")

	gw, _, err := NewGitWorktreeWithBase(repo, "test-from-main", "origin/main")
	if err != nil {
		t.Fatalf("NewGitWorktreeWithBase: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	// The worktree should NOT contain the local-only file
	if _, err := os.Stat(filepath.Join(gw.GetWorktreePath(), "local.txt")); err == nil {
		t.Error("expected local.txt to NOT exist in worktree branched from origin/main")
	}
	// The worktree should contain README.md from the initial commit
	if _, err := os.Stat(filepath.Join(gw.GetWorktreePath(), "README.md")); err != nil {
		t.Error("expected README.md to exist in worktree branched from origin/main")
	}
	// baseCommitSHA should match origin/main, not HEAD
	originMainSHA := strings.TrimSpace(runCmdOutput(t, repo, "git", "rev-parse", "origin/main"))
	if gw.GetBaseCommitSHA() != originMainSHA {
		t.Errorf("expected baseCommitSHA = %s (origin/main), got %s", originMainSHA, gw.GetBaseCommitSHA())
	}
}

func TestSetupNewWorktree_FromHEAD(t *testing.T) {
	repo := setupRepoWithRemote(t, "main")
	writeFile(t, filepath.Join(repo, "local.txt"), "local change")
	runCmd(t, repo, "git", "add", ".")
	runCmd(t, repo, "git", "commit", "-m", "local divergence")

	gw, _, err := NewGitWorktreeWithBase(repo, "test-from-head", "HEAD")
	if err != nil {
		t.Fatalf("NewGitWorktreeWithBase: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	// The worktree SHOULD contain the local-only file
	if _, err := os.Stat(filepath.Join(gw.GetWorktreePath(), "local.txt")); err != nil {
		t.Error("expected local.txt to exist in worktree branched from HEAD")
	}
}

// Helper to capture command output
func runCmdOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %s (%v)", args, out, err)
	}
	return string(out)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run 'TestSetupNewWorktree_From' -v`
Expected: FAIL with "NewGitWorktreeWithBase not defined"

- [ ] **Step 3: Implement the changes**

Add `baseBranch` field to `GitWorktree` in `session/git/worktree.go`:

```go
type GitWorktree struct {
	repoPath         string
	worktreePath     string
	sessionName      string
	branchName       string
	baseCommitSHA    string
	baseBranch       string // "origin/main", "origin/master", or "" (HEAD)
	isExistingBranch bool
	submodules       map[string]*SubmoduleWorktree
	isSubmoduleAware bool
}
```

Add `NewGitWorktreeWithBase` constructor in `session/git/worktree.go`:

```go
// NewGitWorktreeWithBase creates a new GitWorktree that branches from the specified base ref.
// baseBranch can be "origin/main", "origin/master", or "HEAD" (or empty for HEAD).
func NewGitWorktreeWithBase(repoPath string, sessionName string, baseBranch string) (tree *GitWorktree, branchname string, err error) {
	cfg := config.LoadConfig()
	branchName := fmt.Sprintf("%s%s", cfg.BranchPrefix, sessionName)
	branchName = sanitizeBranchName(branchName)

	repoPath, worktreePath, err := resolveWorktreePaths(repoPath, branchName)
	if err != nil {
		return nil, "", err
	}

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
		baseBranch:   baseBranch,
	}, branchName, nil
}
```

Update `setupNewWorktree()` in `session/git/worktree_ops.go`:

```go
func (g *GitWorktree) setupNewWorktree() error {
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath)
	_, _ = g.runGitCommand(g.repoPath, "branch", "-D", g.branchName)

	// Determine the base ref to branch from
	baseRef := "HEAD"
	if g.baseBranch != "" && g.baseBranch != "HEAD" {
		// Fetch origin to ensure we have the latest state
		if _, err := g.runGitCommand(g.repoPath, "fetch", "origin"); err != nil {
			log.WarningLog.Printf("fetch origin failed (proceeding with local state): %v", err)
		}
		baseRef = g.baseBranch
	}

	output, err := g.runGitCommand(g.repoPath, "rev-parse", baseRef)
	if err != nil {
		if baseRef == "HEAD" {
			if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
				strings.Contains(err.Error(), "fatal: not a valid object name") ||
				strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
				return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
			}
		}
		return fmt.Errorf("failed to get commit hash for %s: %w", baseRef, err)
	}
	baseCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = baseCommit

	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, baseCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", baseCommit, err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run 'TestSetupNewWorktree_From|TestDetectDefaultRemoteBranch' -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to verify no regressions**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -v`
Expected: PASS (all tests)

- [ ] **Step 6: Commit**

```bash
git add session/git/worktree.go session/git/worktree_ops.go session/git/worktree_git_test.go
git commit -m "feat: support base branch selection in worktree creation"
```

---

### Task 4: Update Instance and storage to carry base branch ref

**Files:**
- Modify: `session/instance.go:33-77,186-284`
- Modify: `session/storage.go:10-55`

- [ ] **Step 1: Add `baseBranchRef` field to Instance**

In `session/instance.go`, add after `selectedBranch`:

```go
// baseBranchRef is the ref to branch from ("origin/main", "origin/master", or "" for HEAD)
baseBranchRef string
```

Add setter:

```go
// SetBaseBranchRef sets the base branch ref for worktree creation.
func (i *Instance) SetBaseBranchRef(ref string) {
	i.baseBranchRef = ref
}
```

- [ ] **Step 2: Update Instance.Start() to use NewGitWorktreeWithBase**

In `session/instance.go`, in the `Start()` method, change the else branch (new branch, not existing) at ~line 277:

```go
} else {
	var gitWorktree *git.GitWorktree
	var branchName string
	var err error
	if i.baseBranchRef != "" && i.baseBranchRef != "HEAD" {
		gitWorktree, branchName, err = git.NewGitWorktreeWithBase(i.Path, i.Title, i.baseBranchRef)
	} else {
		gitWorktree, branchName, err = git.NewGitWorktree(i.Path, i.Title)
	}
	if err != nil {
		return fmt.Errorf("failed to create git worktree: %w", err)
	}
	i.gitWorktree = gitWorktree
	i.Branch = branchName
}
```

- [ ] **Step 3: Add BaseBranch to InstanceData**

In `session/storage.go`, add to `InstanceData`:

```go
BaseBranch string `json:"base_branch,omitempty"`
```

- [ ] **Step 4: Update ToInstanceData and FromInstanceData**

In `ToInstanceData()`, add:

```go
data.BaseBranch = i.baseBranchRef
```

In `FromInstanceData()`, add:

```go
instance.baseBranchRef = data.BaseBranch
```

- [ ] **Step 5: Run all tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add session/instance.go session/storage.go
git commit -m "feat: wire baseBranchRef through Instance and storage"
```

---

### Task 5: Wire branch picker detection and selection through app.go

**Files:**
- Modify: `app/app.go:590-618,460-513`
- Modify: `ui/overlay/textInput.go:61-97,330-337`

- [ ] **Step 1: Add GetBaseBranch to TextInputOverlay**

In `ui/overlay/textInput.go`, add after `GetSelectedBranch()`:

```go
// GetBaseBranch returns the base branch ref from the branch picker.
// Returns "HEAD" if no branch picker or an existing branch is selected.
func (t *TextInputOverlay) GetBaseBranch() string {
	if t.branchPicker == nil {
		return "HEAD"
	}
	if !t.branchPicker.IsNewBranch() {
		return "HEAD"
	}
	return t.branchPicker.BaseBranch()
}
```

- [ ] **Step 2: Add SetNewBranchOptions to TextInputOverlay**

In `ui/overlay/textInput.go`, add:

```go
// SetNewBranchOptions passes detected remote default branches to the branch picker.
func (t *TextInputOverlay) SetNewBranchOptions(remoteBranches []string) {
	if t.branchPicker != nil {
		t.branchPicker.SetNewBranchOptions(remoteBranches)
	}
}
```

- [ ] **Step 3: Detect remote branches when entering statePrompt in app.go**

In `app/app.go`, there are two places where the TextInputOverlay with branch picker is created (both call `m.newPromptOverlay()`):

1. In the `stateNew` enter handler that transitions to `statePrompt` (around line 394)
2. In the `instanceStartedMsg` handler when `msg.promptAfterName` is true (around line 290)

After each `m.textInputOverlay = m.newPromptOverlay()` call, add:

```go
// Detect remote default branches for new-branch options
currentDir, _ := os.Getwd()
remoteBranches := git.DetectDefaultRemoteBranches(currentDir)
m.textInputOverlay.SetNewBranchOptions(remoteBranches)
```

- [ ] **Step 4: Pass baseBranchRef when starting instance**

In `app/app.go`, in the prompt submission handler (~line 478-513), after setting selectedBranch and before calling `Start()`, add:

```go
baseBranch := m.textInputOverlay.GetBaseBranch()
if baseBranch != "" && baseBranch != "HEAD" {
	selected.SetBaseBranchRef(baseBranch)
}
```

- [ ] **Step 5: Build and verify compilation**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build ./...`
Expected: Build succeeds with no errors

- [ ] **Step 6: Run all tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... 2>&1 | tail -20`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add app/app.go ui/overlay/textInput.go
git commit -m "feat: wire base branch detection and selection through UI to instance"
```

---

### Task 6: Update submodule setup to auto-detect and fetch

**Files:**
- Modify: `session/git/submodule.go:40-62`

- [ ] **Step 1: Write the failing test**

Add to `session/git/submodule_test.go`:

```go
func TestSubmoduleSetup_FetchesAndDetectsBase(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)
	subs, err := ListSubmodules(parentDir)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}

	parentWorktree := filepath.Join(t.TempDir(), "parent-wt")
	runCmd(t, parentDir, "git", "worktree", "add", "-b", "test-sub-detect", parentWorktree)

	targetPath := filepath.Join(parentWorktree, subs[0].Path)
	sw := NewSubmoduleWorktree(subs[0].Path, subs[0].GitDir, targetPath, "test-sub-detect-branch")
	if err := sw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// baseCommitSHA should be set (non-empty)
	if sw.GetBaseCommitSHA() == "" {
		t.Error("expected baseCommitSHA to be set after Setup()")
	}
}
```

Note: The submodule test repos created by `setupTestRepoWithSubmodules` may not have a remote, so fetch will gracefully no-op. The key behavior change is that Setup() will attempt to detect and use a default remote branch. In repos without a remote (like test repos), it falls back to HEAD, preserving existing behavior.

- [ ] **Step 2: Run test to verify current behavior**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run TestSubmoduleSetup_FetchesAndDetectsBase -v`
Expected: PASS (this test validates existing behavior is preserved)

- [ ] **Step 3: Update SubmoduleWorktree.Setup() to fetch and detect base**

In `session/git/submodule.go`, update `Setup()`:

```go
func (s *SubmoduleWorktree) Setup() error {
	if err := os.RemoveAll(s.worktreePath); err != nil {
		return fmt.Errorf("failed to clean target path: %w", err)
	}

	// Fetch origin to ensure latest state (best-effort)
	_, _ = s.runGitDirCommand("fetch", "origin")

	// Detect the default remote branch for this submodule
	baseRef := "HEAD"
	detected := detectDefaultRemoteBranchFromGitDir(s.gitDir)
	if detected != "" {
		baseRef = detected
	}

	headOutput, err := s.runGitDirCommand("rev-parse", baseRef)
	if err != nil {
		// Fall back to HEAD if the detected ref fails
		if baseRef != "HEAD" {
			headOutput, err = s.runGitDirCommand("rev-parse", "HEAD")
		}
		if err != nil {
			return fmt.Errorf("failed to get submodule base commit: %w", err)
		}
	}
	s.baseCommitSHA = strings.TrimSpace(headOutput)

	_, err = s.runGitDirCommand("show-ref", "--verify", fmt.Sprintf("refs/heads/%s", s.branchName))
	if err == nil {
		s.isExistingBranch = true
		_, err = s.runGitDirCommand("worktree", "add", s.worktreePath, s.branchName)
	} else {
		// Branch from the detected base commit
		_, err = s.runGitDirCommand("worktree", "add", "-b", s.branchName, s.worktreePath, s.baseCommitSHA)
	}
	if err != nil {
		return fmt.Errorf("failed to create submodule worktree: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Add `detectDefaultRemoteBranchFromGitDir` helper**

Add to `session/git/worktree_git.go`:

```go
// detectDefaultRemoteBranchFromGitDir checks for origin/main or origin/master
// using --git-dir (for submodules that don't have a working directory yet).
// Returns the first found ref, or empty string if neither exists.
func detectDefaultRemoteBranchFromGitDir(gitDir string) string {
	for _, ref := range []string{"origin/main", "origin/master"} {
		cmd := exec.Command("git", "--git-dir="+gitDir, "rev-parse", "--verify", ref)
		if err := cmd.Run(); err == nil {
			return ref
		}
	}
	return ""
}
```

- [ ] **Step 5: Run all submodule tests**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./session/git/ -run 'TestSubmodule|TestAggregated' -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... 2>&1 | tail -20`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add session/git/submodule.go session/git/worktree_git.go
git commit -m "feat: submodule worktrees auto-detect and fetch default remote branch"
```

---

### Task 7: Remove TODO and final verification

**Files:**
- Modify: `session/git/worktree_ops.go:88-91`

- [ ] **Step 1: Remove the TODO comment**

The TODO at `worktree_ops.go:91` is now implemented. Remove or update the comment block to reflect the new behavior.

Old:
```go
// Create a new worktree from the HEAD commit
// Otherwise, we'll inherit uncommitted changes from the previous worktree.
// This way, we can start the worktree with a clean slate.
// TODO: we might want to give an option to use main/master instead of the current branch.
```

New (if any comment is needed, it should be minimal since the code is self-explanatory after the refactor).

- [ ] **Step 2: Run full test suite**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go test ./... 2>&1 | tail -20`
Expected: All tests PASS

- [ ] **Step 3: Build the binary and smoke test**

Run: `cd /Users/jadams/go/src/bitbucket.org/vervemotion/claude-squad && go build -o cs . && echo "Build OK"`
Expected: "Build OK"

- [ ] **Step 4: Commit**

```bash
git add session/git/worktree_ops.go
git commit -m "chore: remove resolved TODO for base branch selection"
```
