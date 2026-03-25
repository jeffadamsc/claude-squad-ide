package git

import (
	"claude-squad/config"
	"claude-squad/log"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func getWorktreeDirectory() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "worktrees"), nil
}

// GitWorktree manages git worktree operations for a session
type GitWorktree struct {
	// Path to the repository
	repoPath string
	// Path to the worktree
	worktreePath string
	// Name of the session
	sessionName string
	// Branch name for the worktree
	branchName string
	// Base commit hash for the worktree
	baseCommitSHA string
	// isExistingBranch is true if the branch existed before the session was created.
	// When true, the branch will not be deleted on cleanup.
	isExistingBranch bool
	// baseRef is the ref to base a new branch on (e.g., "origin/main").
	// Only used during Setup for new worktrees. Empty means use HEAD.
	baseRef string
	// executor abstracts command execution (local vs remote SSH).
	executor CommandExecutor
}

func NewGitWorktreeFromStorage(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string, isExistingBranch bool) *GitWorktree {
	return &GitWorktree{
		repoPath:         repoPath,
		worktreePath:     worktreePath,
		sessionName:      sessionName,
		branchName:       branchName,
		baseCommitSHA:    baseCommitSHA,
		isExistingBranch: isExistingBranch,
		executor:         defaultExecutor,
	}
}

// resolveWorktreePaths resolves the repo root and generates a unique worktree path for the given branch name.
func resolveWorktreePaths(repoPath string, branchName string) (resolvedRepo string, worktreePath string, err error) {
	return resolveWorktreePathsWithExecutor(repoPath, branchName, nil)
}

// resolveWorktreePathsWithExecutor resolves paths using the given executor.
// When exec is nil (or a LocalExecutor), paths are resolved locally.
// When exec is a RemoteExecutor, paths are resolved on the remote machine.
func resolveWorktreePathsWithExecutor(repoPath string, branchName string, exec CommandExecutor) (resolvedRepo string, worktreePath string, err error) {
	if exec == nil {
		exec = defaultExecutor
	}

	_, isRemote := exec.(*RemoteExecutor)

	var absPath string
	if isRemote {
		// Remote: the path is already absolute on the remote machine
		absPath = repoPath
	} else {
		absPath, err = filepath.Abs(repoPath)
		if err != nil {
			log.ErrorLog.Printf("git worktree path abs error, falling back to repoPath %s: %s", repoPath, err)
			absPath = repoPath
		}
	}

	// Find git repo root via executor
	out, err := exec.Run(absPath, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("failed to find Git repository root from path: %s (output: %s, err: %w)", absPath, strings.TrimSpace(string(out)), err)
	}
	resolvedRepo = strings.TrimSpace(string(out))

	// Determine worktree base directory
	var worktreeDir string
	if isRemote {
		// On the remote machine, use ~/.claude-squad/worktrees/
		homeOut, err := exec.Run("", "echo", "$HOME")
		if err != nil {
			return "", "", fmt.Errorf("failed to resolve remote home: %w", err)
		}
		worktreeDir = strings.TrimSpace(string(homeOut)) + "/.claude-squad/worktrees"
	} else {
		worktreeDir, err = getWorktreeDirectory()
		if err != nil {
			return "", "", err
		}
	}

	worktreePath = worktreeDir + "/" + sanitizeBranchName(branchName)
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	return resolvedRepo, worktreePath, nil
}

// NewGitWorktree creates a new GitWorktree instance
func NewGitWorktree(repoPath string, sessionName string) (tree *GitWorktree, branchname string, err error) {
	return NewGitWorktreeWithExecutor(repoPath, sessionName, nil)
}

// NewGitWorktreeWithExecutor creates a new GitWorktree using the given executor.
func NewGitWorktreeWithExecutor(repoPath string, sessionName string, exec CommandExecutor) (tree *GitWorktree, branchname string, err error) {
	if exec == nil {
		exec = defaultExecutor
	}
	cfg := config.LoadConfig()
	branchName := fmt.Sprintf("%s%s", cfg.BranchPrefix, sessionName)
	// Sanitize the final branch name to handle invalid characters from any source
	// (e.g., backslashes from Windows domain usernames like DOMAIN\user)
	branchName = sanitizeBranchName(branchName)

	repoPath, worktreePath, err := resolveWorktreePathsWithExecutor(repoPath, branchName, exec)
	if err != nil {
		return nil, "", err
	}

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
		executor:     exec,
	}, branchName, nil
}

// NewGitWorktreeFromBranch creates a new GitWorktree that uses an existing branch.
// The branch will not be deleted on cleanup.
func NewGitWorktreeFromBranch(repoPath string, branchName string, sessionName string) (*GitWorktree, error) {
	return NewGitWorktreeFromBranchWithExecutor(repoPath, branchName, sessionName, nil)
}

// NewGitWorktreeFromBranchWithExecutor creates a new GitWorktree from an existing branch using the given executor.
func NewGitWorktreeFromBranchWithExecutor(repoPath string, branchName string, sessionName string, exec CommandExecutor) (*GitWorktree, error) {
	if exec == nil {
		exec = defaultExecutor
	}
	repoPath, worktreePath, err := resolveWorktreePathsWithExecutor(repoPath, branchName, exec)
	if err != nil {
		return nil, err
	}

	return &GitWorktree{
		repoPath:         repoPath,
		sessionName:      sessionName,
		branchName:       branchName,
		worktreePath:     worktreePath,
		isExistingBranch: true,
		executor:         exec,
	}, nil
}

// NewGitWorktreeFromRef creates a new GitWorktree with a new branch based on a specific ref
// (e.g., "origin/main"). The new branch is named using the configured branch prefix + session name.
func NewGitWorktreeFromRef(repoPath string, baseRef string, sessionName string) (tree *GitWorktree, branchName string, err error) {
	return NewGitWorktreeFromRefWithExecutor(repoPath, baseRef, sessionName, nil)
}

// NewGitWorktreeFromRefWithExecutor creates a new GitWorktree from a ref using the given executor.
func NewGitWorktreeFromRefWithExecutor(repoPath string, baseRef string, sessionName string, exec CommandExecutor) (tree *GitWorktree, branchName string, err error) {
	if exec == nil {
		exec = defaultExecutor
	}
	cfg := config.LoadConfig()
	branchName = fmt.Sprintf("%s%s", cfg.BranchPrefix, sessionName)
	branchName = sanitizeBranchName(branchName)

	repoPath, worktreePath, err := resolveWorktreePathsWithExecutor(repoPath, branchName, exec)
	if err != nil {
		return nil, "", err
	}

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
		baseRef:      baseRef,
		executor:     exec,
	}, branchName, nil
}

// SetExecutor overrides the command executor (e.g., for remote SSH execution).
func (g *GitWorktree) SetExecutor(e CommandExecutor) {
	g.executor = e
}

// IsExistingBranch returns whether this worktree uses a pre-existing branch
func (g *GitWorktree) IsExistingBranch() bool {
	return g.isExistingBranch
}

// GetWorktreePath returns the path to the worktree
func (g *GitWorktree) GetWorktreePath() string {
	return g.worktreePath
}

// GetBranchName returns the name of the branch associated with this worktree
func (g *GitWorktree) GetBranchName() string {
	return g.branchName
}

// GetRepoPath returns the path to the repository
func (g *GitWorktree) GetRepoPath() string {
	return g.repoPath
}

// GetRepoName returns the name of the repository (last part of the repoPath).
func (g *GitWorktree) GetRepoName() string {
	return filepath.Base(g.repoPath)
}

// GetBaseCommitSHA returns the base commit SHA for the worktree
func (g *GitWorktree) GetBaseCommitSHA() string {
	return g.baseCommitSHA
}
