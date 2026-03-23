package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	runCmd(t, repo, "git", "checkout", "-b", "master")
	runCmd(t, repo, "git", "push", "origin", "master")
	runCmd(t, repo, "git", "checkout", "main")
	branches := DetectDefaultRemoteBranches(repo)
	if len(branches) != 2 || branches[0] != "origin/main" || branches[1] != "origin/master" {
		t.Errorf("expected [origin/main, origin/master], got %v", branches)
	}
}

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
