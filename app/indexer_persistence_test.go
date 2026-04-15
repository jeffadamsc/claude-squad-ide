// app/indexer_persistence_test.go
package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexPersistence(t *testing.T) {
	tmp := t.TempDir()

	// Create test data
	symbols := map[string][]Symbol{
		"Foo": {{Name: "Foo", Kind: "function", File: "main.go", Line: 10}},
		"Bar": {{Name: "Bar", Kind: "type", File: "types.go", Line: 5}},
	}

	cg := NewCallGraph()
	cg.AddReference(Reference{Symbol: "Foo", Caller: "main", File: "main.go"})

	// Save
	store := NewIndexStore(tmp)
	err := store.Save(symbols, cg, "abc123")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify files exist
	indexDir := filepath.Join(tmp, ".claude-squad", "index")
	if _, err := os.Stat(filepath.Join(indexDir, "meta.json")); err != nil {
		t.Error("meta.json not created")
	}
	if _, err := os.Stat(filepath.Join(indexDir, "symbols.gob")); err != nil {
		t.Error("symbols.gob not created")
	}

	// Load
	loadedSyms, loadedCG, commit, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if commit != "abc123" {
		t.Errorf("commit = %q, want abc123", commit)
	}
	if len(loadedSyms) != 2 {
		t.Errorf("loaded %d symbols, want 2", len(loadedSyms))
	}
	if len(loadedCG.FindCallers("Foo")) != 1 {
		t.Error("callgraph not restored")
	}
}
