package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTreeSitterIntegration(t *testing.T) {
	tmp := t.TempDir()

	// Create a multi-file Go project
	exec.Command("git", "-C", tmp, "init").Run()

	// main.go
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(`package main

import "fmt"

func main() {
	msg := greet("world")
	fmt.Println(msg)
}
`), 0644)

	// greet.go
	os.WriteFile(filepath.Join(tmp, "greet.go"), []byte(`package main

func greet(name string) string {
	return "Hello, " + name
}
`), 0644)

	exec.Command("git", "-C", tmp, "add", ".").Run()

	// Start indexer
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(300 * time.Millisecond)

	// Test symbol lookup
	syms := idx.LookupSymbol("greet")
	if len(syms) != 1 {
		t.Errorf("LookupSymbol(greet) = %d, want 1", len(syms))
	}

	// Test call graph
	callers := idx.FindCallers("greet")
	if len(callers) == 0 {
		t.Error("FindCallers(greet) should find main")
	}

	callees := idx.FindCallees("main")
	found := false
	for _, ref := range callees {
		if ref.Symbol == "greet" || strings.Contains(ref.Symbol, "greet") {
			found = true
			break
		}
	}
	if !found {
		t.Error("FindCallees(main) should include greet")
	}

	// Test BM25 search
	results := idx.SearchSymbols("gree", 10)
	if len(results) == 0 {
		t.Error("SearchSymbols(gree) should find greet")
	}

	idx.Stop()
}

func TestCreateIndexer(t *testing.T) {
	tmp := t.TempDir()

	// Test tree-sitter indexer creation
	tsIdx := createIndexer(tmp, IndexerTreeSitter)
	if tsIdx == nil {
		t.Fatal("createIndexer(TreeSitter) returned nil")
	}
	if _, ok := tsIdx.(*TreeSitterIndexer); !ok {
		t.Error("createIndexer(TreeSitter) should return *TreeSitterIndexer")
	}

	// Test ctags indexer creation
	ctagsIdx := createIndexer(tmp, IndexerCtags)
	if ctagsIdx == nil {
		t.Fatal("createIndexer(Ctags) returned nil")
	}
	if _, ok := ctagsIdx.(*SessionIndexer); !ok {
		t.Error("createIndexer(Ctags) should return *SessionIndexer")
	}

	// Test default (unknown type) falls back to ctags
	defaultIdx := createIndexer(tmp, IndexerType("unknown"))
	if defaultIdx == nil {
		t.Fatal("createIndexer(unknown) returned nil")
	}
	if _, ok := defaultIdx.(*SessionIndexer); !ok {
		t.Error("createIndexer(unknown) should fall back to *SessionIndexer")
	}
}

func TestDefaultIndexerIsTreeSitter(t *testing.T) {
	if DefaultIndexer != IndexerTreeSitter {
		t.Errorf("DefaultIndexer = %v, want %v", DefaultIndexer, IndexerTreeSitter)
	}
}
