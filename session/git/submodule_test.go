package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubmoduleWorktreeSetupAndCleanup(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	subs, err := ListSubmodules(parentDir)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 submodule, got %d", len(subs))
	}

	parentWorktree := filepath.Join(t.TempDir(), "parent-wt")
	runCmd(t, parentDir, "git", "worktree", "add", "-b", "test-session", parentWorktree)

	targetPath := filepath.Join(parentWorktree, subs[0].Path)

	sw := NewSubmoduleWorktree(subs[0].Path, subs[0].GitDir, targetPath, "test-sub-branch")

	if err := sw.Setup(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatal("worktree directory was not created")
	}

	if !IsGitRepo(targetPath) {
		t.Fatal("worktree is not a git repo")
	}

	if sw.GetBranchName() != "test-sub-branch" {
		t.Errorf("expected branch 'test-sub-branch', got %q", sw.GetBranchName())
	}

	if err := sw.Remove(); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
}

func TestSubmoduleWorktreeIsDirtyAndCommit(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)
	subs, _ := ListSubmodules(parentDir)

	parentWorktree := filepath.Join(t.TempDir(), "parent-wt")
	runCmd(t, parentDir, "git", "worktree", "add", "-b", "test-dirty", parentWorktree)

	targetPath := filepath.Join(parentWorktree, subs[0].Path)
	sw := NewSubmoduleWorktree(subs[0].Path, subs[0].GitDir, targetPath, "test-dirty-branch")
	if err := sw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	dirty, err := sw.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("expected clean worktree")
	}

	writeFile(t, filepath.Join(targetPath, "new.txt"), "new content")

	dirty, err = sw.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if !dirty {
		t.Error("expected dirty worktree")
	}

	if err := sw.CommitChanges("test commit"); err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}

	dirty, err = sw.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("expected clean worktree after commit")
	}
}
