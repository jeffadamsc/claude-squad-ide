package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepoWithSubmodules(t *testing.T) (parentDir string, submoduleDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	subRemote := filepath.Join(tmpDir, "sub-remote.git")
	runCmd(t, "", "git", "init", "--bare", subRemote)

	subWork := filepath.Join(tmpDir, "sub-work")
	runCmd(t, "", "git", "init", subWork)
	runCmd(t, subWork, "git", "config", "user.email", "test@test.com")
	runCmd(t, subWork, "git", "config", "user.name", "Test")
	writeFile(t, filepath.Join(subWork, "file.txt"), "hello")
	runCmd(t, subWork, "git", "add", ".")
	runCmd(t, subWork, "git", "commit", "-m", "init sub")
	runCmd(t, subWork, "git", "remote", "add", "origin", subRemote)
	runCmd(t, subWork, "git", "push", "origin", "HEAD:main")

	parentDir = filepath.Join(tmpDir, "parent")
	runCmd(t, "", "git", "init", parentDir)
	runCmd(t, parentDir, "git", "config", "user.email", "test@test.com")
	runCmd(t, parentDir, "git", "config", "user.name", "Test")

	runCmd(t, parentDir, "git", "-c", "protocol.file.allow=always", "submodule", "add", subRemote, "my-submodule")
	runCmd(t, parentDir, "git", "commit", "-m", "add submodule")

	submoduleDir = filepath.Join(parentDir, "my-submodule")
	return parentDir, submoduleDir
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s (%v)", name, args, out, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestListSubmodules(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	subs, err := ListSubmodules(parentDir)
	if err != nil {
		t.Fatalf("ListSubmodules failed: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 submodule, got %d", len(subs))
	}
	if subs[0].Path != "my-submodule" {
		t.Errorf("expected path 'my-submodule', got %q", subs[0].Path)
	}
	if subs[0].GitDir == "" {
		t.Error("expected GitDir to be set")
	}
}

func TestListSubmodules_NoSubmodules(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd(t, "", "git", "init", tmpDir)

	subs, err := ListSubmodules(tmpDir)
	if err != nil {
		t.Fatalf("ListSubmodules failed: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 submodules, got %d", len(subs))
	}
}

func TestHasSubmodules(t *testing.T) {
	parentDir, _ := setupTestRepoWithSubmodules(t)

	if !HasSubmodules(parentDir) {
		t.Error("expected HasSubmodules to return true")
	}

	tmpDir := t.TempDir()
	runCmd(t, "", "git", "init", tmpDir)
	if HasSubmodules(tmpDir) {
		t.Error("expected HasSubmodules to return false for repo without submodules")
	}
}
