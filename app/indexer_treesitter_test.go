package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTreeSitterIndexerBuild(t *testing.T) {
	tmp := t.TempDir()

	// Create minimal git repo
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)

	// Create a Go file
	goCode := []byte(`package main

func Hello() string {
    return "hello"
}
`)
	os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)

	// Initialize git and add file
	exec.Command("git", "-C", tmp, "init").Run()
	exec.Command("git", "-C", tmp, "add", "main.go").Run()

	idx := NewTreeSitterIndexer(tmp)
	idx.Start()

	// Wait for build
	time.Sleep(100 * time.Millisecond)

	// Check files
	files := idx.Files()
	if len(files) != 1 || files[0] != "main.go" {
		t.Errorf("Files() = %v, want [main.go]", files)
	}

	// Check symbols
	defs := idx.Lookup("Hello")
	if len(defs) != 1 {
		t.Errorf("Lookup(Hello) = %d results, want 1", len(defs))
	}

	idx.Stop()
}

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
