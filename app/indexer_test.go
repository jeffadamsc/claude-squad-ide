package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	return dir
}

func TestListFilesInWorktree(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	files, err := listFilesInWorktree(dir)
	require.NoError(t, err)
	assert.Contains(t, files, "main.go")
	assert.Contains(t, files, "src/app.go")
	assert.Contains(t, files, "README.md")
	assert.Len(t, files, 3)
}

func TestListFilesInWorktree_ExcludesUntracked(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package main"), 0644))
	runGit(t, dir, "add", "tracked.go")
	runGit(t, dir, "commit", "-m", "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package main"), 0644))

	files, err := listFilesInWorktree(dir)
	require.NoError(t, err)
	assert.Contains(t, files, "tracked.go")
	assert.NotContains(t, files, "untracked.go")
}

func TestParseCtagsJSON(t *testing.T) {
	input := `{"_type": "tag", "name": "main", "path": "main.go", "line": 5, "kind": "function", "language": "Go"}
{"_type": "tag", "name": "SessionAPI", "path": "app/bindings.go", "line": 64, "kind": "struct", "language": "Go", "scope": "app"}
{"_type": "tag", "name": "ListFiles", "path": "app/bindings.go", "line": 100, "kind": "function", "language": "Go", "scope": "SessionAPI"}
`
	symbols := parseCtagsJSON([]byte(input))
	assert.Len(t, symbols["main"], 1)
	assert.Equal(t, "main.go", symbols["main"][0].File)
	assert.Equal(t, 5, symbols["main"][0].Line)
	assert.Equal(t, "function", symbols["main"][0].Kind)
	assert.Len(t, symbols["SessionAPI"], 1)
	assert.Len(t, symbols["ListFiles"], 1)
	assert.Equal(t, "SessionAPI", symbols["ListFiles"][0].Scope)
}

func TestParseCtagsJSON_EmptyInput(t *testing.T) {
	symbols := parseCtagsJSON([]byte(""))
	assert.Empty(t, symbols)
}

func TestParseCtagsJSON_IgnoresNonTagLines(t *testing.T) {
	input := `{"_type": "ptag", "name": "JSON_OUTPUT_VERSION", "path": "0.0"}
{"_type": "tag", "name": "Foo", "path": "foo.go", "line": 1, "kind": "function", "language": "Go"}
`
	symbols := parseCtagsJSON([]byte(input))
	assert.Len(t, symbols, 1)
	assert.Contains(t, symbols, "Foo")
}

func TestRunCtags(t *testing.T) {
	if findUniversalCtags() == "" {
		t.Skip("universal-ctags not installed")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}

func helper() string {
	return "hello"
}
`), 0644))

	symbols, err := runCtags(dir)
	require.NoError(t, err)
	assert.Contains(t, symbols, "main")
	assert.Contains(t, symbols, "helper")
	assert.Equal(t, "main.go", symbols["helper"][0].File)
	assert.Equal(t, "func", symbols["helper"][0].Kind)
}

func TestRunCtags_NotInstalled(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	symbols, err := runCtags(t.TempDir())
	assert.NoError(t, err)
	assert.Empty(t, symbols)
}

func TestSessionIndexer_StartStop(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	idx := NewSessionIndexer(dir)
	idx.Start()
	defer idx.Stop()

	// Start() is async — wait for the initial build to complete
	time.Sleep(500 * time.Millisecond)

	files := idx.Files()
	assert.Contains(t, files, "main.go")
}

func TestSessionIndexer_Refresh(t *testing.T) {
	dir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "init")

	idx := NewSessionIndexer(dir)
	idx.Start()
	defer idx.Stop()
	time.Sleep(500 * time.Millisecond)

	// Add a new file and commit
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package main\nfunc helper() {}"), 0644))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add helper")

	idx.Refresh()
	time.Sleep(200 * time.Millisecond)

	files := idx.Files()
	assert.Contains(t, files, "helper.go")
}

func TestSymbolFields(t *testing.T) {
	s := Symbol{
		Name:      "MyFunc",
		Kind:      "function",
		File:      "main.go",
		Line:      10,
		EndLine:   25,
		Column:    1,
		Language:  "go",
		Scope:     "main",
		Signature: "func MyFunc(x int) error",
	}
	if s.EndLine <= s.Line {
		t.Error("EndLine should be after Line")
	}
}
