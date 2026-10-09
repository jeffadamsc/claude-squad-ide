package git

import (
	"claude-squad/log"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Exposed as var (not const) for test override; production callers should not mutate.
var (
	// submoduleInitTimeout bounds a single `git submodule update --init -- <name>` call.
	submoduleInitTimeout = 2 * time.Minute
	// submoduleFetchTimeout bounds a single `git fetch origin` inside one submodule.
	submoduleFetchTimeout = 60 * time.Second
	// submoduleFetchAttempts is the total number of fetch attempts (so 1 = no retry).
	submoduleFetchAttempts = 2
)

// Setup creates a new worktree for the session and initializes submodules.
// Returns SetupResult.SubmoduleFailures listing any submodules that failed.
func (g *GitWorktree) Setup() (SetupResult, error) {
	// Ensure worktrees parent directory exists.
	// The worktreePath was already resolved (local or remote) by the constructor.
	worktreesDir := filepath.Dir(g.worktreePath)
	exec := g.getExecutor()
	if _, isRemote := exec.(*RemoteExecutor); isRemote {
		if _, err := exec.Run("", "mkdir", "-p", worktreesDir); err != nil {
			return SetupResult{}, fmt.Errorf("failed to create remote worktree directory: %w", err)
		}
	} else {
		if err := os.MkdirAll(worktreesDir, 0755); err != nil {
			return SetupResult{}, err
		}
	}

	// If this worktree uses a pre-existing branch, always set up from that branch
	// (it may exist locally or only on the remote).
	if g.isExistingBranch {
		return g.setupFromExistingBranch()
	}

	// If a base ref is specified, create a new branch from that ref
	if g.baseRef != "" {
		return g.setupFromRef()
	}

	// Check if branch exists using git CLI (much faster than go-git PlainOpen)
	_, err := g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", g.branchName))
	if err == nil {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch
func (g *GitWorktree) setupFromExistingBranch() (SetupResult, error) {
	// Directory already created in Setup(), skip duplicate creation

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Check if the local branch exists
	_, localErr := g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", g.branchName))
	if localErr != nil {
		// Local branch doesn't exist — check if remote tracking branch exists
		_, remoteErr := g.runGitCommand(g.repoPath, "show-ref", "--verify", fmt.Sprintf("refs/remotes/origin/%s", g.branchName))
		if remoteErr != nil {
			return SetupResult{}, fmt.Errorf("branch %s not found locally or on remote", g.branchName)
		}
		// Create a local tracking branch via worktree add -b
		if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, fmt.Sprintf("origin/%s", g.branchName)); err != nil {
			return SetupResult{}, fmt.Errorf("failed to create worktree from remote branch %s: %w", g.branchName, err)
		}
		return g.runSubmoduleInit()
	}

	// Create a new worktree from the existing local branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return SetupResult{}, fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	return g.runSubmoduleInit()
}

// setupNewWorktree creates a new worktree from HEAD
func (g *GitWorktree) setupNewWorktree() (SetupResult, error) {
	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Clean up any existing branch using git CLI (much faster than go-git PlainOpen)
	_, _ = g.runGitCommand(g.repoPath, "branch", "-D", g.branchName) // Ignore error if branch doesn't exist

	output, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") ||
			strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
			return SetupResult{}, fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
		}
		return SetupResult{}, fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = headCommit

	// Create a new worktree from the HEAD commit
	// Otherwise, we'll inherit uncommitted changes from the previous worktree.
	// This way, we can start the worktree with a clean slate.
	// TODO: we might want to give an option to use main/master instead of the current branch.
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		return SetupResult{}, fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	return g.runSubmoduleInit()
}

// setupFromRef creates a new worktree with a new branch based on a specific ref.
func (g *GitWorktree) setupFromRef() (SetupResult, error) {
	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath)

	// Clean up any existing branch
	_, _ = g.runGitCommand(g.repoPath, "branch", "-D", g.branchName)

	// Resolve the ref to a commit SHA for baseCommitSHA
	output, err := g.runGitCommand(g.repoPath, "rev-parse", g.baseRef)
	if err != nil {
		return SetupResult{}, fmt.Errorf("failed to resolve ref %s: %w", g.baseRef, err)
	}
	g.baseCommitSHA = strings.TrimSpace(output)

	// Create worktree with new branch based on the ref
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, g.baseRef); err != nil {
		return SetupResult{}, fmt.Errorf("failed to create worktree from ref %s: %w", g.baseRef, err)
	}

	return g.runSubmoduleInit()
}

// runSubmoduleInit calls initAndFetchSubmodules and wraps the result in SetupResult.
// A hard error (e.g. enumeration failure) is returned as the error return; per-submodule
// failures are surfaced in SetupResult.SubmoduleFailures.
func (g *GitWorktree) runSubmoduleInit() (SetupResult, error) {
	failures, err := g.initAndFetchSubmodules()
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{SubmoduleFailures: failures}, nil
}

// initAndFetchSubmodules initializes each submodule in the worktree with
// per-submodule timeouts and retries. Returns a slice describing any
// submodules that ended in a bad state; that slice is non-nil only on failures.
//
// The error return is reserved for "could not enumerate submodules" failures.
// Per-submodule failures are reported via the slice — they never abort setup.
//
// This intentionally skips the bulk `git submodule update --init --recursive`
// step that an earlier design called for. Per-submodule init via
// initFetchAndVerifyOne provides better timeout isolation: if one submodule
// hangs, only that submodule fails, not the entire batch.
func (g *GitWorktree) initAndFetchSubmodules() ([]SubmoduleSetupResult, error) {
	// Check if .gitmodules exists in the worktree
	exec := g.getExecutor()
	if _, isRemote := exec.(*RemoteExecutor); isRemote {
		if _, err := exec.Run("", "test", "-f", g.worktreePath+"/.gitmodules"); err != nil {
			return nil, nil // no submodules
		}
	} else {
		if _, err := os.Stat(filepath.Join(g.worktreePath, ".gitmodules")); os.IsNotExist(err) {
			return nil, nil
		}
	}

	// Enumerate submodule paths from .gitmodules.
	// We use `git config -f .gitmodules` rather than `submodule foreach` so a
	// hang in one submodule's hooks can't block enumeration.
	output, err := g.runGitCommand(g.worktreePath, "config", "-f", ".gitmodules",
		"--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		// git config --get-regexp exits 1 when no keys match — this is the normal
		// "no submodules registered" case (e.g., .gitmodules exists but is empty).
		// We can't easily distinguish that from a real malformed-file error, so
		// we use the trimmed output as a tiebreaker: empty output = no submodules.
		if strings.TrimSpace(output) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("submodule enumeration via git config: %w", err)
	}

	var failures []SubmoduleSetupResult
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Each line is of the form: "submodule.<name>.path <path>"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		smRel := strings.TrimSpace(parts[1])
		if smRel == "" {
			continue
		}
		res, ok := g.initFetchAndVerifyOne(smRel)
		if !ok {
			log.WarningLog.Printf("submodule %s: %s failed: %v", res.Name, res.Stage, res.Err)
			failures = append(failures, res)
		}
	}
	return failures, nil
}

// Cleanup removes the worktree and associated branch
func (g *GitWorktree) Cleanup() error {
	var errs []error

	// Check if worktree path exists before attempting removal
	exec := g.getExecutor()
	worktreeExists := false
	if _, isRemote := exec.(*RemoteExecutor); isRemote {
		if _, err := exec.Run("", "test", "-d", g.worktreePath); err == nil {
			worktreeExists = true
		}
	} else {
		if _, err := os.Stat(g.worktreePath); err == nil {
			worktreeExists = true
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to check worktree path: %w", err))
		}
	}
	if worktreeExists {
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			errs = append(errs, err)
		}
	}

	// Delete the branch using git CLI, but skip if this is a pre-existing branch
	if !g.isExistingBranch {
		if _, err := g.runGitCommand(g.repoPath, "branch", "-D", g.branchName); err != nil {
			// Only log if it's not a "branch not found" error
			if !strings.Contains(err.Error(), "not found") {
				errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", g.branchName, err))
			}
		}
	}

	// Prune the worktree to clean up any remaining references
	if err := g.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return g.combineErrors(errs)
	}

	return nil
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	// Remove the worktree using git command
	if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktrees and their associated branches
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Get a list of all branches associated with worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse the output to extract branch names
	worktreeBranches := make(map[string]string)
	currentWorktree := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			// Extract branch name from refs/heads/branch-name
			branchName := strings.TrimPrefix(branchPath, "refs/heads/")
			if currentWorktree != "" {
				worktreeBranches[currentWorktree] = branchName
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			worktreePath := filepath.Join(worktreesDir, entry.Name())

			// Delete the branch associated with this worktree if found
			for path, branch := range worktreeBranches {
				if strings.Contains(path, entry.Name()) {
					// Delete the branch
					deleteCmd := exec.Command("git", "branch", "-D", branch)
					if err := deleteCmd.Run(); err != nil {
						// Log the error but continue with other worktrees
						log.ErrorLog.Printf("failed to delete branch %s: %v", branch, err)
					}
					break
				}
			}

			// Remove the worktree directory
			os.RemoveAll(worktreePath)
		}
	}

	// You have to prune the cleaned up worktrees.
	cmd = exec.Command("git", "worktree", "prune")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}

// initFetchAndVerifyOne runs init + fetch + checkout + verify for a single submodule.
// Returns (result, true) on success; result.Err is nil in that case.
// Returns (result-with-error, false) on any failure; the first failed stage is recorded.
//
// Timeouts: fetch is bounded by submoduleFetchTimeout × submoduleFetchAttempts.
func (g *GitWorktree) initFetchAndVerifyOne(submoduleRel string) (SubmoduleSetupResult, bool) {
	smPath := filepath.Join(g.worktreePath, submoduleRel)
	result := SubmoduleSetupResult{Name: submoduleRel}

	// Stage 1: init this specific submodule. We use the per-submodule form
	// of "submodule update --init" so failures are scoped to this name.
	if err := g.initSubmodule(submoduleRel); err != nil {
		result.Stage = SubmoduleStageInit
		result.Err = err
		return result, false
	}

	// Stage 2: fetch origin with retry.
	if _, err := g.runCommandWithTimeout(
		smPath,
		submoduleFetchTimeout,
		submoduleFetchAttempts,
		"git", "fetch", "origin",
	); err != nil {
		result.Stage = SubmoduleStageFetch
		result.Err = err
		return result, false
	}

	// Stage 2b: checkout the submodule's default branch. We resolve it from the
	// remote's HEAD (refs/remotes/origin/HEAD) rather than hardcoding origin/main,
	// because submodules differ — e.g. verve-portal defaults to "develop".
	// GetDefaultBranchWithExecutor falls back to main/master if HEAD is unset.
	// No timeout (local op).
	defaultBranch := GetDefaultBranchWithExecutor(smPath, g.getExecutor())
	if _, err := g.runGitCommand(smPath, "checkout", "origin/"+defaultBranch); err != nil {
		// Group checkout failure under "fetch" stage for UX simplicity.
		result.Stage = SubmoduleStageFetch
		result.Err = fmt.Errorf("checkout origin/%s: %w", defaultBranch, err)
		return result, false
	}

	// Stage 3: verify the working directory is non-empty. This catches the
	// 2026-05-28 secondary bug where fetch claimed success but the working
	// tree was never populated.
	if empty, err := isSubmoduleDirEmpty(smPath); err != nil {
		result.Stage = SubmoduleStageVerify
		result.Err = fmt.Errorf("verify directory: %w", err)
		return result, false
	} else if empty {
		result.Stage = SubmoduleStageVerify
		result.Err = fmt.Errorf("submodule directory %s is empty after init+fetch+checkout", submoduleRel)
		return result, false
	}

	return result, true
}

// initSubmodule runs `git submodule update --init` for one submodule.
//
// A worktree gets its own submodule repos under .git/worktrees/<wt>/modules,
// so without a reference every session re-clones every submodule over the
// network. When the source repo already has a copy, clone from it with
// --reference and copy the objects in with --dissociate, so a later gc in the
// source repo can't break this worktree. If the reference clone fails, clone
// from the remote as before.
func (g *GitWorktree) initSubmodule(submoduleRel string) error {
	if ref := g.localSubmoduleReference(submoduleRel); ref != "" {
		_, err := g.runCommandWithTimeout(
			g.worktreePath,
			submoduleInitTimeout,
			1,
			"git", "submodule", "update", "--init", "--reference", ref, "--dissociate", "--", submoduleRel,
		)
		if err == nil {
			return nil
		}
		log.WarningLog.Printf("submodule %s: init with reference %s failed, cloning from remote: %v", submoduleRel, ref, err)
	}
	_, err := g.runCommandWithTimeout(
		g.worktreePath,
		submoduleInitTimeout,
		1, // no retry on init — if it can't even init, retry won't help
		"git", "submodule", "update", "--init", "--", submoduleRel,
	)
	return err
}

// localSubmoduleReference returns the git directory of an existing copy of the
// submodule in the source repo (g.repoPath), or "" if there isn't one. It
// checks the superproject's modules/<path> directory first, then a standalone
// clone at <repoPath>/<path>/.git. Submodule names in .gitmodules are assumed
// to match their paths, which is git's default.
func (g *GitWorktree) localSubmoduleReference(submoduleRel string) string {
	if g.repoPath == "" {
		return ""
	}
	exec := g.getExecutor()
	var candidates []string
	if out, err := g.runGitCommand(g.repoPath, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		if commonDir := strings.TrimSpace(out); commonDir != "" {
			candidates = append(candidates, filepath.Join(commonDir, "modules", submoduleRel))
		}
	}
	candidates = append(candidates, filepath.Join(g.repoPath, submoduleRel, ".git"))
	for _, c := range candidates {
		if _, err := exec.Run("", "test", "-d", filepath.Join(c, "objects")); err == nil {
			return c
		}
	}
	return ""
}

// isSubmoduleDirEmpty returns true if the directory contains nothing other
// than a `.git` entry. A submodule whose working tree is just `.git` is
// flagged as a failure deliberately — submodules in this codebase always
// contain source files, so a `.git`-only directory means the fetch+checkout
// pipeline finished without populating content (the 2026-05-28 secondary
// bug).
func isSubmoduleDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		return false, nil
	}
	return true, nil
}

// RetrySubmodule re-runs init+fetch+verify for a single submodule by name.
// It's a thin public wrapper over initFetchAndVerifyOne so app-layer code
// doesn't need to reach into the unexported method.
//
// Defense-in-depth: names containing ".." or starting with "/" are rejected
// to guard against path-traversal if a caller passes attacker-influenced input.
func (g *GitWorktree) RetrySubmodule(name string) (SubmoduleSetupResult, bool) {
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		result := SubmoduleSetupResult{Name: name, Stage: SubmoduleStageInit}
		result.Err = fmt.Errorf("invalid submodule name %q", name)
		return result, false
	}
	return g.initFetchAndVerifyOne(name)
}
