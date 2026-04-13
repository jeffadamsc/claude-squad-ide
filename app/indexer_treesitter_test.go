package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTreeSitterIndexerNew(t *testing.T) {
	tmp := t.TempDir()

	// Create a minimal git repo
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	idx := NewTreeSitterIndexer(tmp)
	if idx == nil {
		t.Fatal("expected non-nil indexer")
	}
	if idx.Worktree() != tmp {
		t.Errorf("worktree = %q, want %q", idx.Worktree(), tmp)
	}
}
