package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFullSubmoduleSessionLifecycle(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	// 1. Detect submodules
	subs, err := ListSubmodules(parentDir)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	if len(subs) == 0 {
		t.Fatal("expected at least one submodule")
	}

	// 2. Create parent worktree
	gw, _, err := NewGitWorktree(parentDir, "integration-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	// 3. Init submodules
	subPaths := []string{subs[0].Path}
	if err := gw.InitSubmodules(parentDir, subPaths); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	if !gw.IsSubmoduleAware() {
		t.Fatal("expected submodule-aware worktree")
	}

	// 4. Make changes in submodule
	sw := gw.GetSubmodules()[subs[0].Path]
	if sw == nil {
		t.Fatalf("submodule %s not found in worktree", subs[0].Path)
	}
	writeFile(t, filepath.Join(sw.GetWorktreePath(), "test.txt"), "test")

	// 5. Verify diff
	agg := gw.AggregatedDiff()
	subDiff := agg.Submodules[subs[0].Path]
	if subDiff == nil {
		t.Fatal("expected submodule diff")
	}
	if subDiff.Added == 0 {
		t.Error("expected added lines in submodule diff")
	}
	if agg.TotalAdded() == 0 {
		t.Error("expected TotalAdded > 0")
	}

	// 6. Pause — commit submodule changes, discard pointers, remove worktrees
	if err := gw.PauseSubmodules(); err != nil {
		t.Fatalf("PauseSubmodules: %v", err)
	}
	if err := gw.DiscardSubmodulePointers(); err != nil {
		t.Fatalf("DiscardSubmodulePointers: %v", err)
	}

	// 7. Remove parent worktree
	if err := gw.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// 8. Resume — re-create parent worktree and submodule worktrees
	if err := gw.Setup(); err != nil {
		t.Fatalf("Re-setup: %v", err)
	}
	if err := gw.ResumeSubmodules(); err != nil {
		t.Fatalf("ResumeSubmodules: %v", err)
	}

	// 9. Verify submodule change survived the pause/resume cycle
	content, err := os.ReadFile(filepath.Join(sw.GetWorktreePath(), "test.txt"))
	if err != nil {
		t.Fatalf("reading file after resume: %v", err)
	}
	if string(content) != "test" {
		t.Errorf("expected 'test', got %q", string(content))
	}

	// 10. Cleanup
	if err := gw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}
