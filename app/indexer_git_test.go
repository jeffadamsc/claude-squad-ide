package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDiffOutput(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,5 @@ func Hello() {
+    fmt.Println("new line")
+    fmt.Println("another new line")
@@ -20,2 +22,1 @@ func Goodbye() {
-    old line
-    another old line
+    replacement line`

	hunks := parseDiffOutput(diff)

	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}

	// First hunk: additions at lines 10-12
	h1 := hunks[0]
	if h1.File != "main.go" {
		t.Errorf("hunk 1 file = %q, want main.go", h1.File)
	}
	if len(h1.AddedLines) != 2 {
		t.Errorf("hunk 1 added lines = %d, want 2", len(h1.AddedLines))
	}

	// Second hunk: removals and additions
	h2 := hunks[1]
	if len(h2.RemovedLines) != 2 {
		t.Errorf("hunk 2 removed lines = %d, want 2", len(h2.RemovedLines))
	}
	if len(h2.AddedLines) != 1 {
		t.Errorf("hunk 2 added lines = %d, want 1", len(h2.AddedLines))
	}
}

func TestGetChangedSymbols(t *testing.T) {
	tmp := t.TempDir()

	// Initialize git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmp
		cmd.Run()
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create initial file with two functions
	initialCode := `package main

func Hello() string {
	return "hello"
}

func Goodbye() string {
	return "goodbye"
}
`
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(initialCode), 0644)
	run("add", "main.go")
	run("commit", "-m", "initial")

	// Start indexer and let it build
	idx := NewTreeSitterIndexer(tmp)
	idx.Start()
	time.Sleep(200 * time.Millisecond)

	// Modify the Hello function
	modifiedCode := `package main

func Hello() string {
	return "hello world" // modified
}

func Goodbye() string {
	return "goodbye"
}
`
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte(modifiedCode), 0644)

	// Refresh index to pick up new line positions
	idx.Refresh()
	time.Sleep(200 * time.Millisecond)

	// Get changed symbols (HEAD vs working dir)
	changed, err := idx.GetChangedSymbols("HEAD", "")
	if err != nil {
		t.Fatalf("GetChangedSymbols failed: %v", err)
	}

	// Should find Hello as modified
	foundHello := false
	for _, cs := range changed {
		if cs.Symbol.Name == "Hello" {
			foundHello = true
			if cs.ChangeType != ChangeModified {
				t.Errorf("Hello change type = %q, want modified", cs.ChangeType)
			}
		}
		// Goodbye should not be in the list (unchanged)
		if cs.Symbol.Name == "Goodbye" {
			t.Error("Goodbye should not be in changed symbols")
		}
	}

	if !foundHello {
		t.Error("Hello should be in changed symbols")
	}

	idx.Stop()
}

func TestGetChangedFiles(t *testing.T) {
	tmp := t.TempDir()

	// Initialize git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmp
		cmd.Run()
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create initial file
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0644)
	run("add", "main.go")
	run("commit", "-m", "initial")

	// Modify it
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n// modified"), 0644)

	idx := NewTreeSitterIndexer(tmp)

	files, err := idx.GetChangedFiles("HEAD", "")
	if err != nil {
		t.Fatalf("GetChangedFiles failed: %v", err)
	}

	if len(files) != 1 || files[0] != "main.go" {
		t.Errorf("GetChangedFiles = %v, want [main.go]", files)
	}
}
