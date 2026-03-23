package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSubmoduleDiff verifies that Diff() counts added lines after writing a new file,
// and returns an error when baseCommitSHA is empty.
func TestSubmoduleDiff(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)
	subs, _ := ListSubmodules(parentDir)

	parentWorktree := filepath.Join(t.TempDir(), "parent-wt")
	runCmd(t, parentDir, "git", "worktree", "add", "-b", "test-diff", parentWorktree)

	targetPath := filepath.Join(parentWorktree, subs[0].Path)
	sw := NewSubmoduleWorktree(subs[0].Path, subs[0].GitDir, targetPath, "test-diff-branch")
	if err := sw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Write a new file — should show added lines
	writeFile(t, filepath.Join(targetPath, "added.txt"), "line1\nline2\n")
	stats := sw.Diff()
	if stats.Error != nil {
		t.Fatalf("Diff() error: %v", stats.Error)
	}
	if stats.Added == 0 {
		t.Error("expected Added > 0 after writing a new file")
	}

	// A SubmoduleWorktree with empty baseCommitSHA should return an error
	noSHA := NewSubmoduleWorktree("x", subs[0].GitDir, targetPath, "x")
	errStats := noSHA.Diff()
	if errStats.Error == nil {
		t.Error("expected error from Diff() when baseCommitSHA is empty")
	}
}

// TestAggregatedDiffStats verifies that AggregatedDiff() sums changes across parent + submodule.
func TestAggregatedDiffStats(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	gw, _, err := NewGitWorktree(parentDir, "agg-diff-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	subs, _ := ListSubmodules(parentDir)
	if err := gw.InitSubmodules(parentDir, []string{subs[0].Path}); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	// Make a change in the parent worktree
	writeFile(t, filepath.Join(gw.GetWorktreePath(), "parent-change.txt"), "parent line\n")

	// Make a change in the submodule worktree
	sw := gw.GetSubmodules()[subs[0].Path]
	writeFile(t, filepath.Join(sw.GetWorktreePath(), "sub-change.txt"), "sub line\n")

	agg := gw.AggregatedDiff()

	if agg.Parent == nil {
		t.Fatal("expected non-nil Parent diff")
	}
	if agg.Parent.Added == 0 {
		t.Error("expected Parent.Added > 0")
	}

	subStats, ok := agg.Submodules[subs[0].Path]
	if !ok {
		t.Fatalf("expected submodule %q in Submodules map", subs[0].Path)
	}
	if subStats.Added == 0 {
		t.Error("expected submodule Added > 0")
	}

	expectedTotal := agg.Parent.Added + subStats.Added
	if agg.TotalAdded() != expectedTotal {
		t.Errorf("TotalAdded() = %d, want %d", agg.TotalAdded(), expectedTotal)
	}

	expectedRemoved := agg.Parent.Removed + subStats.Removed
	if agg.TotalRemoved() != expectedRemoved {
		t.Errorf("TotalRemoved() = %d, want %d", agg.TotalRemoved(), expectedRemoved)
	}
}

// TestAggregatedDiff_NoSubmodules verifies AggregatedDiff() works when no submodules are initialised.
func TestAggregatedDiff_NoSubmodules(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	gw, _, err := NewGitWorktree(parentDir, "no-sub-diff-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	// Do NOT call InitSubmodules — this is a plain (non-submodule-aware) worktree.

	// Make a change in the parent worktree
	writeFile(t, filepath.Join(gw.GetWorktreePath(), "change.txt"), "line\n")

	agg := gw.AggregatedDiff()

	if agg.Parent == nil {
		t.Fatal("expected non-nil Parent diff")
	}
	if agg.Parent.Added == 0 {
		t.Error("expected Parent.Added > 0")
	}
	if len(agg.Submodules) != 0 {
		t.Errorf("expected empty Submodules map, got %d entries", len(agg.Submodules))
	}
	if agg.TotalAdded() != agg.Parent.Added {
		t.Errorf("TotalAdded() = %d, want %d", agg.TotalAdded(), agg.Parent.Added)
	}
	if agg.TotalRemoved() != agg.Parent.Removed {
		t.Errorf("TotalRemoved() = %d, want %d", agg.TotalRemoved(), agg.Parent.Removed)
	}
}

// TestBaseCommitSHAPreservedOnResume verifies that the original baseCommitSHA is not
// overwritten by the pause commit SHA when the submodule is resumed.
func TestBaseCommitSHAPreservedOnResume(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	gw, _, err := NewGitWorktree(parentDir, "sha-preserve-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	subs, _ := ListSubmodules(parentDir)
	if err := gw.InitSubmodules(parentDir, []string{subs[0].Path}); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	sw := gw.GetSubmodules()[subs[0].Path]
	originalSHA := sw.GetBaseCommitSHA()
	if originalSHA == "" {
		t.Fatal("expected baseCommitSHA to be set after Setup()")
	}

	// Make a change so there's something to commit on pause
	writeFile(t, filepath.Join(sw.GetWorktreePath(), "change.txt"), "data")

	// Pause: commits changes and removes submodule worktrees
	if err := gw.PauseSubmodules(); err != nil {
		t.Fatalf("PauseSubmodules: %v", err)
	}
	if err := gw.DiscardSubmodulePointers(); err != nil {
		t.Fatalf("DiscardSubmodulePointers: %v", err)
	}

	// Remove and re-setup parent worktree (simulates a full pause/resume cycle)
	if err := gw.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Re-setup: %v", err)
	}

	// Resume: should restore baseCommitSHA to originalSHA, not the pause commit
	if err := gw.ResumeSubmodules(); err != nil {
		t.Fatalf("ResumeSubmodules: %v", err)
	}

	resumedSHA := sw.GetBaseCommitSHA()
	if resumedSHA != originalSHA {
		t.Errorf("baseCommitSHA after resume = %q, want original %q", resumedSHA, originalSHA)
	}

	if err := gw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

// TestMultipleSubmodules verifies that two submodules can be initialised, diffed,
// paused, and resumed correctly.
func TestMultipleSubmodules(t *testing.T) {
	parentDir := setupTestRepoWithMultipleSubmodules(t)

	subs, err := ListSubmodules(parentDir)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 submodules, got %d", len(subs))
	}

	gw, _, err := NewGitWorktree(parentDir, "multi-sub-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer gw.Cleanup()

	subPaths := []string{subs[0].Path, subs[1].Path}
	if err := gw.InitSubmodules(parentDir, subPaths); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	if len(gw.GetSubmodules()) != 2 {
		t.Fatalf("expected 2 submodule worktrees, got %d", len(gw.GetSubmodules()))
	}

	// Verify both appear in GetSubmodules()
	for _, p := range subPaths {
		if _, ok := gw.GetSubmodules()[p]; !ok {
			t.Errorf("submodule %q missing from GetSubmodules()", p)
		}
	}

	// Make changes in both submodule worktrees
	for _, p := range subPaths {
		sw := gw.GetSubmodules()[p]
		writeFile(t, filepath.Join(sw.GetWorktreePath(), "change.txt"), "change in "+p)
	}

	// AggregatedDiff should include both
	agg := gw.AggregatedDiff()
	for _, p := range subPaths {
		stats, ok := agg.Submodules[p]
		if !ok {
			t.Errorf("expected submodule %q in AggregatedDiff result", p)
			continue
		}
		if stats.Added == 0 {
			t.Errorf("expected Added > 0 for submodule %q", p)
		}
	}

	// Pause submodules — commits changes and removes git worktrees
	if err := gw.PauseSubmodules(); err != nil {
		t.Fatalf("PauseSubmodules: %v", err)
	}

	// Verify both submodule worktrees are removed immediately after PauseSubmodules
	// (before DiscardSubmodulePointers, which may recreate empty directories)
	for _, p := range subPaths {
		sw := gw.GetSubmodules()[p]
		if _, err := os.Stat(sw.GetWorktreePath()); !os.IsNotExist(err) {
			t.Errorf("expected submodule worktree %q to be removed after PauseSubmodules", p)
		}
	}

	if err := gw.DiscardSubmodulePointers(); err != nil {
		t.Fatalf("DiscardSubmodulePointers: %v", err)
	}

	// Resume submodules — re-creates git worktrees from the committed branches
	if err := gw.ResumeSubmodules(); err != nil {
		t.Fatalf("ResumeSubmodules: %v", err)
	}

	// Verify both worktree directories are back as proper git worktrees
	for _, p := range subPaths {
		sw := gw.GetSubmodules()[p]
		if !IsGitRepo(sw.GetWorktreePath()) {
			t.Errorf("expected submodule worktree %q to be a git repo after resume", p)
		}
	}
}

func TestPauseResumeWithSubmodules(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	gw, _, err := NewGitWorktree(parentDir, "pause-test")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	subs, _ := ListSubmodules(parentDir)
	if err := gw.InitSubmodules(parentDir, []string{subs[0].Path}); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	// Make a change in the submodule worktree
	subWT := gw.GetSubmodules()[subs[0].Path]
	writeFile(t, filepath.Join(subWT.GetWorktreePath(), "change.txt"), "changed")

	// Pause submodules (commit + remove)
	if err := gw.PauseSubmodules(); err != nil {
		t.Fatalf("PauseSubmodules: %v", err)
	}

	// Submodule worktree should be removed
	if _, err := os.Stat(subWT.GetWorktreePath()); !os.IsNotExist(err) {
		t.Error("expected submodule worktree to be removed after pause")
	}

	// Discard parent pointer changes
	if err := gw.DiscardSubmodulePointers(); err != nil {
		t.Fatalf("DiscardSubmodulePointers: %v", err)
	}

	// Resume submodules
	if err := gw.ResumeSubmodules(); err != nil {
		t.Fatalf("ResumeSubmodules: %v", err)
	}

	// Submodule worktree should exist again
	if _, err := os.Stat(subWT.GetWorktreePath()); os.IsNotExist(err) {
		t.Error("expected submodule worktree to exist after resume")
	}

	// Cleanup
	if err := gw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

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

func TestGitWorktreeWithSubmodules(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	gw, _, err := NewGitWorktree(parentDir, "test-session")
	if err != nil {
		t.Fatalf("NewGitWorktree: %v", err)
	}

	if gw.IsSubmoduleAware() {
		t.Error("expected non-submodule-aware worktree by default")
	}

	// Must call Setup before InitSubmodules since it creates the worktree
	if err := gw.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	subs, _ := ListSubmodules(parentDir)
	subPaths := []string{subs[0].Path}
	if err := gw.InitSubmodules(parentDir, subPaths); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}

	if !gw.IsSubmoduleAware() {
		t.Error("expected submodule-aware worktree after InitSubmodules")
	}

	if len(gw.GetSubmodules()) != 1 {
		t.Errorf("expected 1 submodule, got %d", len(gw.GetSubmodules()))
	}

	// Test cleanup handles submodules
	if err := gw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

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
