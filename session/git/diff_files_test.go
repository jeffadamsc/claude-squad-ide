package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDiffFiles_ModifiedFile(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := worktreeDir(t, "wt-diff")

	// Write a file and commit it as the base
	if err := os.WriteFile(filepath.Join(repo, "hello.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "add", "hello.go")
	gitCmd(t, repo, "commit", "-m", "add hello.go")
	baseCommit := headSHA(t, repo)

	gw := &GitWorktree{
		repoPath:      repo,
		worktreePath:  wtPath,
		branchName:    "test-diff",
		baseCommitSHA: baseCommit,
		executor:      defaultExecutor,
	}
	if _, err := gw.Setup(); err != nil {
		t.Fatal(err)
	}

	// Modify the file in the worktree
	if err := os.WriteFile(filepath.Join(wtPath, "hello.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := gw.GetDiffFiles()
	if err != nil {
		t.Fatalf("GetDiffFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	f := files[0]
	if f.Path != "hello.go" {
		t.Errorf("path = %q, want %q", f.Path, "hello.go")
	}
	if f.Status != "modified" {
		t.Errorf("status = %q, want %q", f.Status, "modified")
	}
	if f.OldContent != "package main\n" {
		t.Errorf("oldContent = %q, want %q", f.OldContent, "package main\n")
	}
	if f.NewContent != "package main\n\nfunc main() {}\n" {
		t.Errorf("newContent = %q, want %q", f.NewContent, "package main\n\nfunc main() {}\n")
	}
	if f.Submodule != "" {
		t.Errorf("submodule = %q, want empty", f.Submodule)
	}
}

func TestGetDiffFiles_AddedFile(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := worktreeDir(t, "wt-diff-add")
	baseCommit := headSHA(t, repo)

	gw := &GitWorktree{
		repoPath:      repo,
		worktreePath:  wtPath,
		branchName:    "test-diff-add",
		baseCommitSHA: baseCommit,
		executor:      defaultExecutor,
	}
	if _, err := gw.Setup(); err != nil {
		t.Fatal(err)
	}

	// Create a new file in the worktree
	if err := os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := gw.GetDiffFiles()
	if err != nil {
		t.Fatalf("GetDiffFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Status != "added" {
		t.Errorf("status = %q, want %q", files[0].Status, "added")
	}
	if files[0].OldContent != "" {
		t.Errorf("oldContent should be empty for added file, got %q", files[0].OldContent)
	}
	if files[0].NewContent != "hello\n" {
		t.Errorf("newContent = %q, want %q", files[0].NewContent, "hello\n")
	}
}

func TestGetDiffFiles_DeletedFile(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := worktreeDir(t, "wt-diff-del")

	// Create and commit a file
	if err := os.WriteFile(filepath.Join(repo, "remove.txt"), []byte("bye\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "add", "remove.txt")
	gitCmd(t, repo, "commit", "-m", "add remove.txt")
	baseCommit := headSHA(t, repo)

	gw := &GitWorktree{
		repoPath:      repo,
		worktreePath:  wtPath,
		branchName:    "test-diff-del",
		baseCommitSHA: baseCommit,
		executor:      defaultExecutor,
	}
	if _, err := gw.Setup(); err != nil {
		t.Fatal(err)
	}

	// Delete the file in the worktree
	if err := os.Remove(filepath.Join(wtPath, "remove.txt")); err != nil {
		t.Fatal(err)
	}

	files, err := gw.GetDiffFiles()
	if err != nil {
		t.Fatalf("GetDiffFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Status != "deleted" {
		t.Errorf("status = %q, want %q", files[0].Status, "deleted")
	}
	if files[0].OldContent != "bye\n" {
		t.Errorf("oldContent = %q, want %q", files[0].OldContent, "bye\n")
	}
	if files[0].NewContent != "" {
		t.Errorf("newContent should be empty for deleted file, got %q", files[0].NewContent)
	}
}

func TestGetDiffFiles_NoChanges(t *testing.T) {
	repo := createTestRepo(t)
	wtPath := worktreeDir(t, "wt-diff-none")
	baseCommit := headSHA(t, repo)

	gw := &GitWorktree{
		repoPath:      repo,
		worktreePath:  wtPath,
		branchName:    "test-diff-none",
		baseCommitSHA: baseCommit,
		executor:      defaultExecutor,
	}
	if _, err := gw.Setup(); err != nil {
		t.Fatal(err)
	}

	files, err := gw.GetDiffFiles()
	if err != nil {
		t.Fatalf("GetDiffFiles() error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}
