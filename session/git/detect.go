package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// SubmoduleInfo describes a discovered submodule in a parent repo.
type SubmoduleInfo struct {
	// Path is the relative path within the parent repo (e.g., "verve-backend")
	Path string
	// GitDir is the absolute path to the submodule's git directory,
	// discovered via `git rev-parse --git-dir`.
	GitDir string
}

// ListSubmodules returns all submodules in the given repo path.
// It discovers each submodule's git directory dynamically.
func ListSubmodules(repoPath string) ([]SubmoduleInfo, error) {
	cmd := exec.Command("git", "-C", repoPath, "submodule", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list submodules: %s (%w)", output, err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var submodules []SubmoduleInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: " <sha> <path> (<describe>)" or "+<sha> <path> (<describe>)"
		// Strip leading +/- status char
		line = strings.TrimLeft(line, "+-")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		subPath := parts[1]

		// Discover the submodule's git directory dynamically
		absSubPath := filepath.Join(repoPath, subPath)
		gitDirCmd := exec.Command("git", "-C", absSubPath, "rev-parse", "--git-dir")
		gitDirOutput, err := gitDirCmd.CombinedOutput()
		if err != nil {
			// Submodule may not be initialized; skip it
			continue
		}
		gitDir := strings.TrimSpace(string(gitDirOutput))
		// Make absolute if relative
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(absSubPath, gitDir)
		}
		gitDir, _ = filepath.Abs(gitDir)

		submodules = append(submodules, SubmoduleInfo{
			Path:   subPath,
			GitDir: gitDir,
		})
	}

	return submodules, nil
}

// HasSubmodules returns true if the repo at repoPath contains any submodules.
func HasSubmodules(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "submodule", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}
