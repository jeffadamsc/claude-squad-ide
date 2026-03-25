package session

import (
	"claude-squad/log"
	"claude-squad/pty"
	"claude-squad/session/git"
	"crypto/rand"
	"path/filepath"

	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
)

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type Status int

const (
	// Running is the status when the instance is running and claude is working.
	Running Status = iota
	// Ready is if the claude instance is ready to be interacted with (waiting for user input).
	Ready
	// Loading is if the instance is loading (if we are starting it up or something).
	Loading
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused
)

// Instance is a running instance of claude code.
type Instance struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace.
	Path string
	// Branch is the branch of the instance.
	Branch string
	// Status is the status of the instance.
	Status Status
	// Program is the program to run in the instance.
	Program string
	// Height is the height of the instance.
	Height int
	// Width is the width of the instance.
	Width int
	// CreatedAt is the time the instance was created.
	CreatedAt time.Time
	// UpdatedAt is the time the instance was last updated.
	UpdatedAt time.Time
	// AutoYes is true if the instance should automatically press enter when prompted.
	AutoYes bool
	// Prompt is the initial prompt to pass to the instance on startup
	Prompt string
	// ClaudeSessionID is the UUID of the claude conversation, used for --resume
	ClaudeSessionID string
	// HostID is the SSH host ID for remote sessions (empty = local).
	HostID string

	// gitExecutor is the command executor for git operations (local or remote SSH).
	gitExecutor git.CommandExecutor
	// DiffStats stores the current git diff statistics
	diffStats *git.DiffStats

	// selectedBranch is the existing branch to start on (empty = new branch from HEAD)
	selectedBranch string
	// inPlace is true when the session runs directly in the working directory
	// without git worktree isolation. gitWorktree will be nil.
	inPlace bool

	// The below fields are initialized upon calling Start().

	started bool
	// processManager manages the terminal process lifecycle.
	processManager ProcessManager
	// processID is the ID of the running process within processManager.
	processID string
	// gitWorktree is the git worktree for the instance.
	gitWorktree *git.GitWorktree
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:           i.Title,
		Path:            i.Path,
		Branch:          i.Branch,
		Status:          i.Status,
		Height:          i.Height,
		Width:           i.Width,
		CreatedAt:       i.CreatedAt,
		UpdatedAt:       time.Now(),
		Program:         i.Program,
		AutoYes:         i.AutoYes,
		InPlace:         i.inPlace,
		ClaudeSessionID: i.ClaudeSessionID,
		HostID:          i.HostID,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:         i.gitWorktree.GetRepoPath(),
			WorktreePath:     i.gitWorktree.GetWorktreePath(),
			SessionName:      i.Title,
			BranchName:       i.gitWorktree.GetBranchName(),
			BaseCommitSHA:    i.gitWorktree.GetBaseCommitSHA(),
			IsExistingBranch: i.gitWorktree.IsExistingBranch(),
		}
	}

	// Only include diff stats if they exist
	if i.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.diffStats.Added,
			Removed: i.diffStats.Removed,
			Content: i.diffStats.Content,
		}
	}

	return data
}

// FromInstanceData creates a new Instance from serialized data.
// The ProcessManager is required for non-paused instances to restore the process.
// Pass nil if only loading metadata (e.g. for paused sessions).
func FromInstanceData(data InstanceData, pm ProcessManager) (*Instance, error) {
	instance := &Instance{
		Title:           data.Title,
		Path:            data.Path,
		Branch:          data.Branch,
		Status:          data.Status,
		Height:          data.Height,
		Width:           data.Width,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
		Program:         data.Program,
		inPlace:         data.InPlace,
		ClaudeSessionID: data.ClaudeSessionID,
		HostID:          data.HostID,
		processManager:  pm,
		diffStats: &git.DiffStats{
			Added:   data.DiffStats.Added,
			Removed: data.DiffStats.Removed,
			Content: data.DiffStats.Content,
		},
	}

	// Only create git worktree from storage if not an in-place session
	if !data.InPlace {
		instance.gitWorktree = git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
			data.Worktree.IsExistingBranch,
		)
	}

	if instance.Paused() {
		// Paused instances have no running process; processID is empty.
		instance.started = true
	} else {
		if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// Options for creating a new instance
type InstanceOptions struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace.
	Path string
	// Program is the program to run in the instance (e.g. "claude", "aider --model ollama_chat/gemma3:1b")
	Program string
	// If AutoYes is true, then
	AutoYes bool
	// Branch is an existing branch name to start the session on (empty = new branch from HEAD)
	Branch string
	// InPlace runs the session directly in the working directory without git isolation.
	InPlace bool
	// Prompt is the initial prompt to pass to the instance on startup.
	Prompt string
	// HostID is the SSH host ID for remote sessions (empty = local).
	HostID string
	// ProcessManager manages the terminal process lifecycle.
	ProcessManager ProcessManager
	// GitExecutor overrides git command execution (e.g., for remote SSH). Nil means local.
	GitExecutor git.CommandExecutor
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	t := time.Now()

	// Convert path to absolute (skip for remote sessions where the path is on another machine)
	path := opts.Path
	if opts.HostID == "" {
		absPath, err := filepath.Abs(opts.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}
		path = absPath
	}

	return &Instance{
		Title:          opts.Title,
		Status:         Ready,
		Path:           path,
		Program:        opts.Program,
		Height:         0,
		Width:          0,
		CreatedAt:      t,
		UpdatedAt:      t,
		AutoYes:        opts.AutoYes,
		Prompt:         opts.Prompt,
		HostID:         opts.HostID,
		selectedBranch: opts.Branch,
		inPlace:        opts.InPlace,
		processManager: opts.ProcessManager,
		gitExecutor:    opts.GitExecutor,
	}, nil
}

func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.gitWorktree == nil {
		return filepath.Base(i.Path), nil
	}
	return i.gitWorktree.GetRepoName(), nil
}

func (i *Instance) SetStatus(status Status) {
	i.Status = status
}

// SetSelectedBranch sets the branch to use when starting the instance.
func (i *Instance) SetSelectedBranch(branch string) {
	i.selectedBranch = branch
}

// IsInPlace returns whether this session runs in-place without git isolation.
func (i *Instance) IsInPlace() bool {
	return i.inPlace
}

// spawnProcess spawns the program in processManager using the given working directory.
// It parses i.Program into command + args and stores the returned process ID.
// If resume is true and the program is claude, --resume is added to resume
// the conversation. If the resume fails (e.g. conversation not found),
// the process is killed and retried with a fresh session ID.
func (i *Instance) spawnProcess(dir string, resume bool) error {
	if i.processManager == nil {
		return fmt.Errorf("process manager not set")
	}
	fields := strings.Fields(i.Program)
	if len(fields) == 0 {
		return fmt.Errorf("program is empty")
	}
	program := fields[0]
	args := fields[1:]

	isClaude := strings.HasSuffix(program, ProgramClaude)

	if isClaude {
		args = append(args, "--allow-dangerously-skip-permissions")
		if resume && i.ClaudeSessionID != "" {
			args = append(args, "--resume", i.ClaudeSessionID)
		} else if i.ClaudeSessionID == "" {
			// First start: generate a session ID so we can resume later
			id := generateUUID()
			i.ClaudeSessionID = id
			args = append(args, "--session-id", id)
		}
	}

	id, err := i.processManager.Spawn(program, args, pty.SpawnOptions{Dir: dir})
	if err != nil {
		return err
	}
	i.processID = id

	// If we attempted a resume, check for early exit (conversation not found).
	if isClaude && resume && i.ClaudeSessionID != "" {
		if exited := i.processManager.WaitExit(id, 3*time.Second); exited {
			output := i.processManager.GetContent(id)
			if strings.Contains(output, "No conversation found") {
				log.InfoLog.Printf("claude --resume failed (conversation not found), starting fresh session")
				// Kill the dead process entry.
				_ = i.processManager.Kill(id)
				// Clear stale session ID and start fresh.
				i.ClaudeSessionID = ""
				return i.spawnProcess(dir, false)
			}
		}
	}

	return nil
}

// firstTimeSetup is true if this is a new instance. Otherwise, it's one loaded from storage.
func (i *Instance) Start(firstTimeSetup bool) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	if firstTimeSetup {
		exec := i.gitExecutor // nil means local (default)
		if i.inPlace {
			// In-place: no worktree, set branch from current working directory
			log.InfoLog.Printf("Start(%s): in-place mode, getting current branch", i.Title)
			if branch, err := git.GetCurrentBranchWithExecutor(i.Path, exec); err == nil && branch != "" {
				i.Branch = branch
			}
		} else if i.selectedBranch != "" {
			// Fetch latest remote state so origin/* refs (and submodule pointers) are current.
			log.InfoLog.Printf("Start(%s): fetching branches for path=%s", i.Title, i.Path)
			git.FetchBranchesWithExecutor(i.Path, exec)
			log.InfoLog.Printf("Start(%s): creating worktree from ref %s", i.Title, i.selectedBranch)
			// Create a new branch based on the selected branch. We use the selected
			// branch as a ref rather than checking it out directly, because the
			// selected branch may already be checked out in the main worktree.
			gitWorktree, branchName, err := git.NewGitWorktreeFromRefWithExecutor(i.Path, i.selectedBranch, i.Title, exec)
			if err != nil {
				return fmt.Errorf("failed to create git worktree from branch %s: %w", i.selectedBranch, err)
			}
			i.gitWorktree = gitWorktree
			i.Branch = branchName
		} else {
			// Default: fetch origin and create worktree from remote default branch
			log.InfoLog.Printf("Start(%s): fetching branches for path=%s", i.Title, i.Path)
			git.FetchBranchesWithExecutor(i.Path, exec)
			log.InfoLog.Printf("Start(%s): getting default branch", i.Title)
			defaultBranch := git.GetDefaultBranchWithExecutor(i.Path, exec)
			baseRef := fmt.Sprintf("origin/%s", defaultBranch)
			log.InfoLog.Printf("Start(%s): creating worktree from ref %s", i.Title, baseRef)
			gitWorktree, branchName, err := git.NewGitWorktreeFromRefWithExecutor(i.Path, baseRef, i.Title, exec)
			if err != nil {
				// Fall back to HEAD if origin ref fails
				log.InfoLog.Printf("Start(%s): ref failed, falling back to HEAD: %v", i.Title, err)
				gitWorktree, branchName, err = git.NewGitWorktreeWithExecutor(i.Path, i.Title, exec)
				if err != nil {
					return fmt.Errorf("failed to create git worktree: %w", err)
				}
			}
			i.gitWorktree = gitWorktree
			i.Branch = branchName
		}
	}

	// Setup error handler to cleanup resources on any error
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	// Determine the working directory for the process.
	var workDir string
	if i.inPlace {
		workDir = i.Path
	} else if i.gitWorktree != nil {
		if !firstTimeSetup {
			// Worktree is already set up from storage; no need to re-setup.
		} else {
			log.InfoLog.Printf("Start(%s): running worktree Setup", i.Title)
			if err := i.gitWorktree.Setup(); err != nil {
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
			log.InfoLog.Printf("Start(%s): worktree Setup complete", i.Title)
		}
		workDir = i.gitWorktree.GetWorktreePath()
	} else {
		workDir = i.Path
	}

	log.InfoLog.Printf("Start(%s): spawning process in %s", i.Title, workDir)
	// Spawn the process.
	if err := i.spawnProcess(workDir, false); err != nil {
		if i.gitWorktree != nil && firstTimeSetup {
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
		}
		setupErr = fmt.Errorf("failed to start new session: %w", err)
		return setupErr
	}

	i.SetStatus(Running)

	return nil
}

// Kill terminates the instance and cleans up all resources
func (i *Instance) Kill() error {
	if !i.started {
		// If instance was never started, just return success
		return nil
	}

	var errs []error

	// Always try to cleanup both resources, even if one fails
	// Kill the process first since it's using the git worktree
	if i.processManager != nil && i.processID != "" {
		if err := i.processManager.Kill(i.processID); err != nil {
			errs = append(errs, fmt.Errorf("failed to kill process: %w", err))
		}
		i.processID = ""
	}

	// Then clean up git worktree
	if i.gitWorktree != nil {
		if err := i.gitWorktree.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
		}
	}

	return i.combineErrors(errs)
}

// combineErrors combines multiple errors into a single error
func (i *Instance) combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple cleanup errors occurred:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", errMsg)
}

func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused || i.processManager == nil || i.processID == "" {
		return "", nil
	}
	return i.processManager.GetContent(i.processID), nil
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started || i.processManager == nil || i.processID == "" {
		return false, false
	}
	return i.processManager.HasUpdated(i.processID)
}

// CheckAndHandleTrustPrompt checks for and dismisses the trust prompt for supported programs.
func (i *Instance) CheckAndHandleTrustPrompt() bool {
	if !i.started || i.processManager == nil || i.processID == "" {
		return false
	}
	program := i.Program
	if !strings.HasSuffix(program, ProgramClaude) &&
		!strings.HasSuffix(program, ProgramAider) &&
		!strings.HasSuffix(program, ProgramGemini) {
		return false
	}
	if i.processManager.CheckTrustPrompt(i.processID) {
		_ = i.processManager.Write(i.processID, []byte("\n"))
		return true
	}
	return false
}

// TapEnter sends an enter key press to the process if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes || i.processManager == nil || i.processID == "" {
		return
	}
	if err := i.processManager.Write(i.processID, []byte("\n")); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

// Attach is not supported in the PTY/WebSocket architecture.
func (i *Instance) Attach() (chan struct{}, error) {
	return nil, fmt.Errorf("attach not supported: use WebSocket")
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused || i.processManager == nil || i.processID == "" {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.processManager.Resize(i.processID, uint16(height), uint16(width))
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

// GetWorktreePath returns the worktree path for the instance, or empty string if unavailable
func (i *Instance) GetWorktreePath() string {
	if i.gitWorktree == nil {
		return ""
	}
	return i.gitWorktree.GetWorktreePath()
}

func (i *Instance) Started() bool {
	return i.started
}

// GetProcessID returns the PTY process ID for WebSocket routing.
func (i *Instance) GetProcessID() string {
	return i.processID
}

// SetTitle sets the title of the instance. Returns an error if the instance has started.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

func (i *Instance) Paused() bool {
	return i.Status == Paused
}


// TmuxAlive returns true if a process is currently running for this instance.
func (i *Instance) TmuxAlive() bool {
	return i.processID != ""
}

// Pause stops the process and removes the worktree, preserving the branch
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	// In-place sessions: just kill the process, no git operations
	if i.inPlace {
		if i.processManager == nil {
			return fmt.Errorf("process manager is nil")
		}
		if i.processID != "" {
			if err := i.processManager.Kill(i.processID); err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
			i.processID = ""
		}
		i.SetStatus(Paused)
		return nil
	}

	var errs []error

	// Check if there are any changes to commit
	if dirty, err := i.gitWorktree.IsDirty(); err != nil {
		errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
		log.ErrorLog.Print(err)
	} else if dirty {
		// Commit changes locally (without pushing to GitHub)
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
		if err := i.gitWorktree.CommitChanges(commitMsg); err != nil {
			errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
			log.ErrorLog.Print(err)
			// Return early if we can't commit changes to avoid corrupted state
			return i.combineErrors(errs)
		}
	}

	// Kill the process (process dies on pause, branch preserved)
	if i.processManager == nil {
		return fmt.Errorf("process manager is nil")
	}
	if i.processID != "" {
		if err := i.processManager.Kill(i.processID); err != nil {
			errs = append(errs, fmt.Errorf("failed to kill process: %w", err))
			log.ErrorLog.Print(err)
			// Continue with pause process even if kill fails
		}
		i.processID = ""
	}

	// Check if worktree exists before trying to remove it
	if i.pathExists(i.gitWorktree.GetWorktreePath()) {
		// Remove worktree but keep branch
		if err := i.gitWorktree.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}

		// Only prune if remove was successful
		if err := i.gitWorktree.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.SetStatus(Paused)
	_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	return nil
}

// Resume recreates the worktree and spawns a fresh process
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	// In-place sessions: just spawn a fresh process in working directory
	if i.inPlace {
		if i.processManager == nil {
			return fmt.Errorf("process manager is nil")
		}
		if err := i.spawnProcess(i.Path, true); err != nil {
			return fmt.Errorf("failed to resume session: %w", err)
		}
		i.SetStatus(Running)
		return nil
	}

	// Check if branch is checked out
	if checked, err := i.gitWorktree.IsBranchCheckedOut(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	} else if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	// Setup git worktree only if it doesn't already exist
	wtPath := i.gitWorktree.GetWorktreePath()
	if !i.pathExists(wtPath) {
		log.InfoLog.Printf("worktree %s not found, running Setup", wtPath)
		if err := i.gitWorktree.Setup(); err != nil {
			log.ErrorLog.Print(err)
			return fmt.Errorf("failed to setup git worktree: %w", err)
		}
	} else {
		log.InfoLog.Printf("worktree %s already exists, skipping Setup", wtPath)
	}

	// Spawn a fresh process
	if i.processManager == nil {
		return fmt.Errorf("process manager is nil")
	}
	if err := i.spawnProcess(i.gitWorktree.GetWorktreePath(), true); err != nil {
		log.ErrorLog.Print(err)
		// Cleanup git worktree if process creation fails
		if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
			err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			log.ErrorLog.Print(err)
		}
		return fmt.Errorf("failed to start new session: %w", err)
	}

	i.SetStatus(Running)
	return nil
}

// UpdateDiffStats updates the git diff statistics for this instance
func (i *Instance) UpdateDiffStats() error {
	if !i.started {
		i.diffStats = nil
		return nil
	}

	if i.Status == Paused {
		// Keep the previous diff stats if the instance is paused
		return nil
	}

	if i.gitWorktree == nil {
		i.diffStats = nil
		return nil
	}

	stats := i.gitWorktree.Diff()
	if stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			// Worktree is not fully set up yet, not an error
			i.diffStats = nil
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}

	i.diffStats = stats
	return nil
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	return i.diffStats
}

// SendPrompt sends a prompt to the process
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.processManager == nil || i.processID == "" {
		return fmt.Errorf("process not initialized")
	}
	if err := i.processManager.Write(i.processID, []byte(prompt+"\n")); err != nil {
		return fmt.Errorf("error sending prompt to process: %w", err)
	}
	return nil
}

// PreviewFullHistory captures the entire buffered output for this instance
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused || i.processManager == nil || i.processID == "" {
		return "", nil
	}
	return i.processManager.GetContent(i.processID), nil
}

// SetProcessManager sets the process manager for this instance.
func (i *Instance) SetProcessManager(pm ProcessManager) {
	i.processManager = pm
}

// SetGitExecutor overrides the git command executor (e.g., for remote SSH).
func (i *Instance) SetGitExecutor(exec git.CommandExecutor) {
	i.gitExecutor = exec
	if i.gitWorktree != nil {
		i.gitWorktree.SetExecutor(exec)
	}
}

// pathExists checks if a path exists, using the remote executor for remote sessions.
func (i *Instance) pathExists(path string) bool {
	if i.HostID != "" && i.gitExecutor != nil {
		_, err := i.gitExecutor.Run("", "test", "-d", path)
		return err == nil
	}
	_, err := os.Stat(path)
	return err == nil
}

// SendKeys sends keys to the process
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused || i.processManager == nil || i.processID == "" {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	return i.processManager.Write(i.processID, []byte(keys))
}
