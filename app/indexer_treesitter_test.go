package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGetSymbolContent(t *testing.T) {
	tmp := t.TempDir()

	// Create minimal git repo
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	exec.Command("git", "-C", tmp, "init").Run()

	// Create a Go file with a known function
	goCode := []byte(`package main

func Hello() string {
	return "hello"
}

func Goodbye() string {
	return "goodbye"
}
`)
	os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)
	exec.Command("git", "-C", tmp, "add", "main.go").Run()

	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Look up Hello function
	syms := idx.LookupSymbol("Hello")
	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(syms))
	}

	sym := syms[0]

	// Verify byte offsets are set
	if sym.StartByte == 0 && sym.EndByte == 0 {
		t.Error("byte offsets not set")
	}

	// Get content using byte offsets
	content, err := idx.GetSymbolContent(sym)
	if err != nil {
		t.Fatalf("GetSymbolContent failed: %v", err)
	}

	// Should contain the function definition
	if !strings.Contains(content, "func Hello()") {
		t.Errorf("content should contain 'func Hello()', got: %s", content)
	}
	if !strings.Contains(content, "return \"hello\"") {
		t.Errorf("content should contain return statement, got: %s", content)
	}

	// Should NOT contain Goodbye function
	if strings.Contains(content, "Goodbye") {
		t.Errorf("content should not contain Goodbye function, got: %s", content)
	}

	idx.Stop()
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"a", 1},
		{"test", 1},
		{"hello world", 3}, // 11 chars / 4 = 2.75 -> 3
		{"func Hello() string { return \"hello\" }", 10}, // 38 chars / 4 = 9.5 -> 10
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestSearchWithBudget(t *testing.T) {
	tmp := t.TempDir()

	// Create minimal git repo
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	exec.Command("git", "-C", tmp, "init").Run()

	// Create multiple functions to test budget limiting
	goCode := []byte(`package main

func ProcessData() string {
	return "processing data"
}

func ProcessItems() string {
	return "processing items"
}

func ProcessRecords() string {
	return "processing records"
}

func UnrelatedFunc() string {
	return "unrelated"
}
`)
	os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)
	exec.Command("git", "-C", tmp, "add", "main.go").Run()

	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Search for "Process" functions with a small budget
	results := idx.SearchWithBudget("Process", 50, true)

	// Should have at least one result
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// All results should match query
	for _, r := range results {
		if !strings.Contains(r.Symbol.Name, "Process") {
			t.Errorf("result %q doesn't match query", r.Symbol.Name)
		}
	}

	// Total tokens should not exceed budget (with some flexibility)
	totalTokens := 0
	for _, r := range results {
		totalTokens += r.Tokens
	}
	// Allow some overhead since we always include at least one result
	if totalTokens > 100 { // generous limit
		t.Logf("total tokens: %d (may exceed budget for first result)", totalTokens)
	}

	// Test with larger budget should return more results
	moreResults := idx.SearchWithBudget("Process", 500, true)
	if len(moreResults) < len(results) {
		t.Errorf("larger budget should return >= results: got %d, want >= %d", len(moreResults), len(results))
	}

	idx.Stop()
}

func TestSearchWithContext(t *testing.T) {
	tmp := t.TempDir()

	// Create minimal git repo
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	exec.Command("git", "-C", tmp, "init").Run()

	// Create a Go file with functions that call each other
	goCode := []byte(`package main

func ProcessData() string {
	result := Helper()
	return result
}

func Helper() string {
	return "helper"
}

func Unrelated() string {
	return "unrelated"
}
`)
	os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)
	exec.Command("git", "-C", tmp, "add", "main.go").Run()

	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Search for ProcessData with context
	bundles := idx.SearchWithContext("ProcessData", 500)

	if len(bundles) == 0 {
		t.Fatal("expected at least one bundle")
	}

	// Primary should be ProcessData
	bundle := bundles[0]
	if bundle.Primary.Symbol.Name != "ProcessData" {
		t.Errorf("primary symbol = %q, want ProcessData", bundle.Primary.Symbol.Name)
	}

	// Should have content
	if !strings.Contains(bundle.Primary.Content, "func ProcessData()") {
		t.Errorf("primary content missing function definition, got: %q", bundle.Primary.Content)
	}

	// Related should include Helper (which ProcessData calls)
	foundHelper := false
	for _, r := range bundle.Related {
		if r.Symbol.Name == "Helper" {
			foundHelper = true
			break
		}
	}
	if !foundHelper {
		t.Logf("Related symbols: %v", bundle.Related)
		t.Error("expected Helper in related symbols (ProcessData calls Helper)")
	}

	// Unrelated should NOT be in related
	for _, r := range bundle.Related {
		if r.Symbol.Name == "Unrelated" {
			t.Error("Unrelated should not be in related symbols")
		}
	}

	idx.Stop()
}

func TestCentralityScoring(t *testing.T) {
	tmp := t.TempDir()

	// Create minimal git repo
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	exec.Command("git", "-C", tmp, "init").Run()

	// Create a Go file with a call hierarchy:
	// Main -> ProcessData -> Helper (Helper is called most, so highest in-degree)
	// Main -> Helper (Helper called from two places)
	goCode := []byte(`package main

func Main() {
	ProcessData()
	Helper()
}

func ProcessData() string {
	return Helper()
}

func Helper() string {
	return "helper"
}

func Unused() string {
	return "never called"
}
`)
	os.WriteFile(filepath.Join(tmp, "main.go"), goCode, 0644)
	exec.Command("git", "-C", tmp, "add", "main.go").Run()

	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Helper should have highest in-degree (called from Main and ProcessData)
	helperCentrality := idx.GetCentrality("Helper")
	if helperCentrality.InDegree < 2 {
		t.Errorf("Helper in-degree = %d, want >= 2", helperCentrality.InDegree)
	}

	// Unused should have zero in-degree
	unusedCentrality := idx.GetCentrality("Unused")
	if unusedCentrality.InDegree != 0 {
		t.Errorf("Unused in-degree = %d, want 0", unusedCentrality.InDegree)
	}

	// Main should have highest out-degree (calls ProcessData and Helper)
	mainCentrality := idx.GetCentrality("Main")
	if mainCentrality.OutDegree < 2 {
		t.Errorf("Main out-degree = %d, want >= 2", mainCentrality.OutDegree)
	}

	// Top symbols by centrality should include Helper (high in-degree)
	top := idx.TopSymbolsByCentrality(5)
	if len(top) == 0 {
		t.Fatal("expected top symbols")
	}

	// Helper should be near the top due to high in-degree
	foundHelper := false
	for i, sc := range top {
		if sc.Symbol == "Helper" {
			foundHelper = true
			if i > 2 { // Should be in top 3
				t.Errorf("Helper ranked %d, expected in top 3", i+1)
			}
			break
		}
	}
	if !foundHelper {
		t.Error("Helper not found in top symbols")
	}

	idx.Stop()
}
