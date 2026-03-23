package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SubmoduleWorktree manages a git worktree for a single submodule within a session.
type SubmoduleWorktree struct {
	submodulePath    string
	gitDir           string
	worktreePath     string
	branchName       string
	baseCommitSHA    string
	isExistingBranch bool
}

func NewSubmoduleWorktree(submodulePath, gitDir, worktreePath, branchName string) *SubmoduleWorktree {
	return &SubmoduleWorktree{
		submodulePath: submodulePath,
		gitDir:        gitDir,
		worktreePath:  worktreePath,
		branchName:    branchName,
	}
}

func NewSubmoduleWorktreeFromStorage(submodulePath, gitDir, worktreePath, branchName, baseCommitSHA string, isExistingBranch bool) *SubmoduleWorktree {
	return &SubmoduleWorktree{
		submodulePath:    submodulePath,
		gitDir:           gitDir,
		worktreePath:     worktreePath,
		branchName:       branchName,
		baseCommitSHA:    baseCommitSHA,
		isExistingBranch: isExistingBranch,
	}
}

func (s *SubmoduleWorktree) Setup() error {
	if err := os.RemoveAll(s.worktreePath); err != nil {
		return fmt.Errorf("failed to clean target path: %w", err)
	}

	headOutput, err := s.runGitDirCommand("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get submodule HEAD: %w", err)
	}
	s.baseCommitSHA = strings.TrimSpace(headOutput)

	_, err = s.runGitDirCommand("show-ref", "--verify", fmt.Sprintf("refs/heads/%s", s.branchName))
	if err == nil {
		s.isExistingBranch = true
		_, err = s.runGitDirCommand("worktree", "add", s.worktreePath, s.branchName)
	} else {
		_, err = s.runGitDirCommand("worktree", "add", "-b", s.branchName, s.worktreePath)
	}
	if err != nil {
		return fmt.Errorf("failed to create submodule worktree: %w", err)
	}
	return nil
}

func (s *SubmoduleWorktree) Remove() error {
	_, err := s.runGitDirCommand("worktree", "remove", "--force", s.worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove submodule worktree: %w", err)
	}
	return nil
}

func (s *SubmoduleWorktree) Cleanup() error {
	if _, err := os.Stat(s.worktreePath); err == nil {
		if err := s.Remove(); err != nil {
			_ = os.RemoveAll(s.worktreePath)
		}
	}
	_, _ = s.runGitDirCommand("worktree", "prune")
	if !s.isExistingBranch {
		_, _ = s.runGitDirCommand("branch", "-D", s.branchName)
	}
	return nil
}

func (s *SubmoduleWorktree) IsDirty() (bool, error) {
	output, err := s.runWorktreeCommand("status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check submodule status: %w", err)
	}
	return strings.TrimSpace(output) != "", nil
}

func (s *SubmoduleWorktree) CommitChanges(message string) error {
	dirty, err := s.IsDirty()
	if err != nil {
		return err
	}
	if !dirty {
		return nil
	}
	if _, err := s.runWorktreeCommand("add", "."); err != nil {
		return fmt.Errorf("failed to stage submodule changes: %w", err)
	}
	if _, err := s.runWorktreeCommand("commit", "-m", message, "--no-verify"); err != nil {
		return fmt.Errorf("failed to commit submodule changes: %w", err)
	}
	return nil
}

// Diff returns the diff stats for this submodule worktree.
// Uses the same line-counting approach as GitWorktree.Diff() for consistency.
func (s *SubmoduleWorktree) Diff() *DiffStats {
	if s.baseCommitSHA == "" {
		return &DiffStats{Error: fmt.Errorf("base commit SHA not set")}
	}
	stats := &DiffStats{}
	_, _ = s.runWorktreeCommand("add", "-N", ".")
	content, err := s.runWorktreeCommand("--no-pager", "diff", s.baseCommitSHA)
	if err != nil {
		stats.Error = err
		return stats
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content
	return stats
}

func (s *SubmoduleWorktree) PushChanges() error {
	dirty, err := s.IsDirty()
	if err != nil {
		return err
	}
	if dirty {
		if err := s.CommitChanges("[claudesquad] pre-push commit"); err != nil {
			return err
		}
	}
	_, err = s.runWorktreeCommand("push", "-u", "origin", s.branchName)
	if err != nil {
		return fmt.Errorf("failed to push submodule %s: %w", s.submodulePath, err)
	}
	return nil
}

// Accessors
func (s *SubmoduleWorktree) GetSubmodulePath() string { return s.submodulePath }
func (s *SubmoduleWorktree) GetGitDir() string        { return s.gitDir }
func (s *SubmoduleWorktree) GetWorktreePath() string  { return s.worktreePath }
func (s *SubmoduleWorktree) GetBranchName() string    { return s.branchName }
func (s *SubmoduleWorktree) GetBaseCommitSHA() string  { return s.baseCommitSHA }
func (s *SubmoduleWorktree) SetBaseCommitSHA(sha string) { s.baseCommitSHA = sha }
func (s *SubmoduleWorktree) IsExistingBranch() bool    { return s.isExistingBranch }

func (s *SubmoduleWorktree) runGitDirCommand(args ...string) (string, error) {
	baseArgs := []string{"--git-dir=" + s.gitDir}
	cmd := exec.Command("git", append(baseArgs, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", output, err)
	}
	return string(output), nil
}

func (s *SubmoduleWorktree) runWorktreeCommand(args ...string) (string, error) {
	baseArgs := []string{"-C", s.worktreePath}
	cmd := exec.Command("git", append(baseArgs, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", output, err)
	}
	return string(output), nil
}
