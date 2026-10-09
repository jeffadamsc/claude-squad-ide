package git

// Integration tests for Setup() with real git submodules using local file:// repos.
//
// These tests exercise the full production code path:
//   Setup() → initAndFetchSubmodules() → initFetchAndVerifyOne() → runCommandWithTimeout()
//
// The hang/kill machinery is tested at the executor level in executor_timeout_test.go.
// This file tests the integration of Setup() with real git submodule init/fetch against
// real local file:// remotes.
//
// Note on file:// protocol: git 2.38+ requires protocol.file.allow=always for submodule
// operations that clone from file:// URLs. We configure this via GIT_CONFIG_COUNT env
// vars (t.Setenv) so all child git processes spawned by the production code inherit it.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// allowFileProtocol sets the GIT_CONFIG env vars so that all child git processes spawned
// during the test allow the file:// transport (required by git 2.38+).
// t.Setenv automatically cleans up at test end.
func allowFileProtocol(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
}

// createSubmoduleRepo creates a scratch git repo with a sample file (f.txt = "content\n")
// and returns the absolute repo path. It sets user.email/user.name so commits work in
// environments without a global git config.
func createSubmoduleRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createSubmoduleRepo %v: %s (%v)", args, out, err)
		}
	}
	// Write a sample file so the submodule directory is non-empty after checkout
	sampleFile := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(sampleFile, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "f.txt"},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createSubmoduleRepo %v: %s (%v)", args, out, err)
		}
	}
	return dir
}

// createSubmoduleRepoWithBranch is like createSubmoduleRepo but renames the repo's
// default branch to branchName (e.g. "develop"). This lets tests exercise submodules
// whose remote HEAD is neither "main" nor "master".
func createSubmoduleRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	dir := createSubmoduleRepo(t)
	cmd := exec.Command("git", "branch", "-m", branchName)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rename default branch to %q: %s (%v)", branchName, out, err)
	}
	// Point HEAD's symref at the renamed branch so clones pick it up as origin/HEAD.
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/"+branchName)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set HEAD to %q: %s (%v)", branchName, out, err)
	}
	return dir
}

// createParentRepoWithSubmodule creates a parent git repo, adds the given sub repo as
// a submodule named submoduleName, and commits the result. Returns the parent repo path.
// Requires allowFileProtocol(t) to be called first.
func createParentRepoWithSubmodule(t *testing.T, subRepoPath string, submoduleName string) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createParentRepo %v: %s (%v)", args, out, err)
		}
	}
	// Initial commit so the repo is valid
	{
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", "initial")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createParentRepo initial commit: %s (%v)", out, err)
		}
	}

	// Add submodule using file:// URL
	subURL := fmt.Sprintf("file://%s", subRepoPath)
	{
		cmd := exec.Command("git", "submodule", "add", subURL, submoduleName)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git submodule add: %s (%v)", out, err)
		}
	}
	// Commit the submodule
	for _, args := range [][]string{
		{"git", "add", ".gitmodules", submoduleName},
		{"git", "commit", "-m", "add submodule"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createParentRepo commit submodule %v: %s (%v)", args, out, err)
		}
	}
	return dir
}

// createParentRepoWithBrokenSubmodule creates a parent git repo that has a submodule
// pointing at a non-existent path, via the "add real, then rewrite URL" technique.
// The .gitmodules and git config both reflect the broken URL after this call.
func createParentRepoWithBrokenSubmodule(t *testing.T, subRepoPath string, submoduleName string) string {
	t.Helper()
	// Start with a real submodule so the SHA is recorded in the index
	parentPath := createParentRepoWithSubmodule(t, subRepoPath, submoduleName)

	// Rewrite .gitmodules to a non-existent path
	brokenURL := fmt.Sprintf("file:///tmp/does-not-exist-%d.git", os.Getpid())
	gitmodules := fmt.Sprintf("[submodule %q]\n\tpath = %s\n\turl = %s\n", submoduleName, submoduleName, brokenURL)
	if err := os.WriteFile(filepath.Join(parentPath, ".gitmodules"), []byte(gitmodules), 0o644); err != nil {
		t.Fatalf("rewrite .gitmodules: %v", err)
	}

	// Sync .gitmodules -> git config so the cached URL is updated
	{
		cmd := exec.Command("git", "submodule", "sync")
		cmd.Dir = parentPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git submodule sync: %s (%v)", out, err)
		}
	}

	// Commit the broken .gitmodules
	for _, args := range [][]string{
		{"git", "add", ".gitmodules"},
		{"git", "commit", "-m", "break submodule url"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = parentPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("createParentRepoWithBrokenSubmodule %v: %s (%v)", args, out, err)
		}
	}
	return parentPath
}

// buildTestWorktree creates a GitWorktree pointing at a fresh worktree path for
// the given parent repo. The caller is responsible for calling gw.Cleanup().
func buildTestWorktree(t *testing.T, parentRepoPath string) *GitWorktree {
	t.Helper()
	wtPath := worktreeDir(t, "integration-wt")
	return &GitWorktree{
		repoPath:     parentRepoPath,
		worktreePath: wtPath,
		sessionName:  "integration-test",
		branchName:   fmt.Sprintf("cs-integration-%d", os.Getpid()),
	}
}

// TestSetup_HappyPath_PopulatesSubmodule verifies that Setup() with a healthy
// local file:// submodule populates the submodule directory and returns no failures.
func TestSetup_HappyPath_PopulatesSubmodule(t *testing.T) {
	allowFileProtocol(t)

	subRepo := createSubmoduleRepo(t)
	parentRepo := createParentRepoWithSubmodule(t, subRepo, "mysub")

	gw := buildTestWorktree(t, parentRepo)
	defer gw.Cleanup()

	result, err := gw.Setup()
	if err != nil {
		t.Fatalf("Setup() returned error: %v", err)
	}
	if len(result.SubmoduleFailures) != 0 {
		t.Errorf("expected no submodule failures, got %d: %+v", len(result.SubmoduleFailures), result.SubmoduleFailures)
	}

	// The submodule directory should be non-empty (contains f.txt)
	sampleFile := filepath.Join(gw.worktreePath, "mysub", "f.txt")
	if _, err := os.Stat(sampleFile); os.IsNotExist(err) {
		t.Errorf("expected submodule file %s to exist after Setup()", sampleFile)
	}
}

// TestSetup_DefaultBranchDevelop_PopulatesSubmodule verifies that a submodule whose
// remote default branch is "develop" (neither "main" nor "master") is checked out and
// populated without a recorded failure. This guards against the regression where the
// checkout step hardcoded origin/main with only an origin/master fallback, which broke
// setup for submodules like verve-portal (default branch: develop).
func TestSetup_DefaultBranchDevelop_PopulatesSubmodule(t *testing.T) {
	allowFileProtocol(t)

	subRepo := createSubmoduleRepoWithBranch(t, "develop")
	parentRepo := createParentRepoWithSubmodule(t, subRepo, "mysub")

	gw := buildTestWorktree(t, parentRepo)
	defer gw.Cleanup()

	result, err := gw.Setup()
	if err != nil {
		t.Fatalf("Setup() returned error: %v", err)
	}
	if len(result.SubmoduleFailures) != 0 {
		t.Errorf("expected no submodule failures for develop-default submodule, got %d: %+v",
			len(result.SubmoduleFailures), result.SubmoduleFailures)
	}

	// The submodule directory should be non-empty (contains f.txt)
	sampleFile := filepath.Join(gw.worktreePath, "mysub", "f.txt")
	if _, err := os.Stat(sampleFile); os.IsNotExist(err) {
		t.Errorf("expected submodule file %s to exist after Setup()", sampleFile)
	}
}

// TestSetup_BrokenSubmodule_RecordsFailure verifies that a submodule whose URL
// points at a non-existent repo causes a recorded SubmoduleSetupResult failure
// (not a hard error), and that Setup() does not abort.
func TestSetup_BrokenSubmodule_RecordsFailure(t *testing.T) {
	allowFileProtocol(t)

	subRepo := createSubmoduleRepo(t)
	parentRepo := createParentRepoWithBrokenSubmodule(t, subRepo, "mysub")

	gw := buildTestWorktree(t, parentRepo)
	defer gw.Cleanup()

	result, err := gw.Setup()
	if err != nil {
		t.Fatalf("Setup() returned hard error (wanted soft failure): %v", err)
	}
	if len(result.SubmoduleFailures) != 1 {
		t.Fatalf("expected 1 submodule failure, got %d: %+v", len(result.SubmoduleFailures), result.SubmoduleFailures)
	}

	failure := result.SubmoduleFailures[0]
	if failure.Name != "mysub" {
		t.Errorf("failure.Name = %q, want %q", failure.Name, "mysub")
	}
	if failure.Stage != SubmoduleStageInit {
		t.Errorf("failure.Stage = %q, want %q", failure.Stage, SubmoduleStageInit)
	}
	if failure.Err == nil {
		t.Error("failure.Err is nil, expected non-nil")
	}
}

// TestSetup_NoSubmodules_NoFailures verifies that a repo without a .gitmodules
// file returns an empty SetupResult without error.
func TestSetup_NoSubmodules_NoFailures(t *testing.T) {
	// No submodules means no file:// protocol needed
	parentRepo := createTestRepo(t) // plain repo, no .gitmodules

	gw := buildTestWorktree(t, parentRepo)
	defer gw.Cleanup()

	result, err := gw.Setup()
	if err != nil {
		t.Fatalf("Setup() returned error: %v", err)
	}
	if len(result.SubmoduleFailures) != 0 {
		t.Errorf("expected no failures for repo without submodules, got %+v", result.SubmoduleFailures)
	}
}

// TestSetup_BrokenSubmodule_RetryAfterFix_Succeeds verifies the recovery path:
// after Setup() records a failure for a broken submodule, fixing the URL in
// .gitmodules (+ sync) and calling RetrySubmodule() succeeds and populates the
// working tree.
func TestSetup_BrokenSubmodule_RetryAfterFix_Succeeds(t *testing.T) {
	allowFileProtocol(t)

	subRepo := createSubmoduleRepo(t)
	parentRepo := createParentRepoWithBrokenSubmodule(t, subRepo, "mysub")

	gw := buildTestWorktree(t, parentRepo)
	defer gw.Cleanup()

	// First call: should fail on the broken URL
	firstResult, err := gw.Setup()
	if err != nil {
		t.Fatalf("Setup() returned hard error: %v", err)
	}
	if len(firstResult.SubmoduleFailures) == 0 {
		t.Fatal("expected initial Setup() to fail on broken submodule, but it succeeded")
	}

	// Fix the .gitmodules in the worktree to point to the real submodule repo
	fixedURL := fmt.Sprintf("file://%s", subRepo)
	fixedGitmodules := fmt.Sprintf("[submodule \"mysub\"]\n\tpath = mysub\n\turl = %s\n", fixedURL)
	if err := os.WriteFile(filepath.Join(gw.worktreePath, ".gitmodules"), []byte(fixedGitmodules), 0o644); err != nil {
		t.Fatalf("write fixed .gitmodules: %v", err)
	}

	// Sync .gitmodules -> git config in the worktree so the URL is updated
	{
		cmd := exec.Command("git", "submodule", "sync")
		cmd.Dir = gw.worktreePath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git submodule sync after fix: %s (%v)", out, err)
		}
	}

	// Retry should now succeed
	retryResult, ok := gw.RetrySubmodule("mysub")
	if !ok {
		t.Fatalf("RetrySubmodule() returned false after URL fix: stage=%s err=%v", retryResult.Stage, retryResult.Err)
	}
	if retryResult.Err != nil {
		t.Errorf("RetrySubmodule() returned non-nil Err: %v", retryResult.Err)
	}

	// The submodule directory should now be populated
	sampleFile := filepath.Join(gw.worktreePath, "mysub", "f.txt")
	if _, err := os.Stat(sampleFile); os.IsNotExist(err) {
		t.Errorf("expected %s to exist after successful retry", sampleFile)
	}
}

// TestSetup_ClonesSubmoduleThroughLocalReference verifies that Setup() clones
// submodules through the source repo's existing copy (with --reference) and
// copies the objects in (with --dissociate). GIT_TRACE records every git
// process, so the trace shows whether the clone received the reference. The
// negative control removes the copy and expects a plain clone.
func TestSetup_ClonesSubmoduleThroughLocalReference(t *testing.T) {
	allowFileProtocol(t)

	setup := func(t *testing.T, removeLocalCopy bool) (*GitWorktree, string, string) {
		t.Helper()
		subRepo := createSubmoduleRepo(t)
		parentRepo := createParentRepoWithSubmodule(t, subRepo, "mysub")
		localCopy := filepath.Join(parentRepo, ".git", "modules", "mysub")
		if removeLocalCopy {
			if err := os.RemoveAll(localCopy); err != nil {
				t.Fatalf("remove local copy: %v", err)
			}
		}
		tracePath := filepath.Join(t.TempDir(), "git-trace.log")
		t.Setenv("GIT_TRACE", tracePath)
		gw := buildTestWorktree(t, parentRepo)
		t.Cleanup(func() { _ = gw.Cleanup() })

		result, err := gw.Setup()
		if err != nil {
			t.Fatalf("Setup() returned error: %v", err)
		}
		if len(result.SubmoduleFailures) != 0 {
			t.Fatalf("expected no submodule failures, got %+v", result.SubmoduleFailures)
		}
		trace, err := os.ReadFile(tracePath)
		if err != nil {
			t.Fatalf("read git trace: %v", err)
		}
		return gw, localCopy, string(trace)
	}

	t.Run("with local copy", func(t *testing.T) {
		gw, localCopy, trace := setup(t, false)

		if got := gw.localSubmoduleReference("mysub"); !sameDir(t, got, localCopy) {
			t.Fatalf("localSubmoduleReference = %q, want %q", got, localCopy)
		}
		if !strings.Contains(trace, "--reference") || !strings.Contains(trace, filepath.Join("modules", "mysub")) {
			t.Errorf("git trace shows no clone with --reference to the local copy:\n%s", trace)
		}
		if !strings.Contains(trace, "--dissociate") {
			t.Errorf("git trace shows no --dissociate:\n%s", trace)
		}
		// --dissociate must leave the worktree's clone with no alternates link
		// back into the source repo.
		out, err := exec.Command("git", "-C", filepath.Join(gw.worktreePath, "mysub"),
			"rev-parse", "--path-format=absolute", "--git-path", "objects/info/alternates").Output()
		if err != nil {
			t.Fatalf("rev-parse alternates path: %v", err)
		}
		if alt := strings.TrimSpace(string(out)); fileExists(alt) {
			t.Errorf("worktree submodule clone still has alternates file %s", alt)
		}
		if !fileExists(filepath.Join(gw.worktreePath, "mysub", "f.txt")) {
			t.Errorf("submodule not populated")
		}
	})

	t.Run("without local copy", func(t *testing.T) {
		gw, _, trace := setup(t, true)

		if got := gw.localSubmoduleReference("mysub"); got != "" {
			t.Fatalf("localSubmoduleReference = %q, want empty", got)
		}
		if strings.Contains(trace, "--reference") {
			t.Errorf("expected a plain clone without --reference, trace:\n%s", trace)
		}
	})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// sameDir reports whether two paths name the same directory, resolving symlinks
// such as macOS's /var -> /private/var.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
