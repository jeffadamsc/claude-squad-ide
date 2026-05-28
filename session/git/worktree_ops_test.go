package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitFetchAndVerifyOne_InitFailsRecorded(t *testing.T) {
	// A worktree whose submodule "missing" cannot be init'd. The helper should
	// return an Init-stage failure rather than panicking or returning silent
	// success. Uses stubFailingInitExecutor to drive the Init failure path
	// deterministically (no real git invocation).
	wt := t.TempDir()
	smPath := filepath.Join(wt, "missing")
	if err := os.MkdirAll(smPath, 0o755); err != nil {
		t.Fatal(err)
	}

	gw := &GitWorktree{
		worktreePath: wt,
		executor:     &stubFailingInitExecutor{},
	}
	res, ok := gw.initFetchAndVerifyOne("missing")
	if ok {
		t.Fatalf("expected failure, got success: %+v", res)
	}
	if res.Name != "missing" {
		t.Errorf("Name = %q, want %q", res.Name, "missing")
	}
	if res.Stage != SubmoduleStageInit {
		t.Errorf("Stage = %q, want init", res.Stage)
	}
	if res.Err == nil {
		t.Errorf("Err is nil, expected non-nil")
	}
}

func TestInitFetchAndVerifyOne_VerifyFailsOnEmptyDir(t *testing.T) {
	// This test uses a stub executor that pretends fetch+checkout succeed
	// but leaves the directory empty. We expect a Verify-stage failure.
	wt := t.TempDir()
	smPath := filepath.Join(wt, "empty")
	if err := os.MkdirAll(smPath, 0o755); err != nil {
		t.Fatal(err)
	}

	gw := &GitWorktree{
		worktreePath: wt,
		executor:     &stubAlwaysOKExecutor{},
	}
	res, ok := gw.initFetchAndVerifyOne("empty")
	if ok {
		t.Fatalf("expected verify failure, got success")
	}
	if res.Stage != SubmoduleStageVerify {
		t.Errorf("Stage = %q, want verify", res.Stage)
	}
}

func TestSubmoduleSetupResult_ErrorString(t *testing.T) {
	r := SubmoduleSetupResult{Name: "x", Stage: SubmoduleStageFetch, Err: errors.New("boom")}
	got := r.ErrorString()
	if got == "" {
		t.Fatal("ErrorString returned empty")
	}
}

// stubAlwaysOKExecutor pretends every command succeeded with empty output.
type stubAlwaysOKExecutor struct{}

func (s *stubAlwaysOKExecutor) Run(dir, name string, args ...string) ([]byte, error) {
	return []byte{}, nil
}

// stubFailingInitExecutor fails any "git submodule update --init" call but
// returns success (empty bytes) for every other command. Used to drive
// initFetchAndVerifyOne into the Init-stage failure path deterministically.
type stubFailingInitExecutor struct{}

func (s *stubFailingInitExecutor) Run(dir, name string, args ...string) ([]byte, error) {
	if name == "git" && len(args) >= 3 && args[0] == "submodule" && args[1] == "update" && args[2] == "--init" {
		return []byte("simulated init failure"), errors.New("simulated init failure")
	}
	return []byte{}, nil
}

func TestInitAndFetchSubmodules_NoGitmodulesReturnsNil(t *testing.T) {
	wt := t.TempDir()
	gw := &GitWorktree{worktreePath: wt}
	results, err := gw.initAndFetchSubmodules()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results when .gitmodules missing, got %+v", results)
	}
}

// stubEnumerationFailExecutor fails the `git config -f .gitmodules --get-regexp`
// call with non-empty output (simulating a malformed .gitmodules), and returns
// success for all other calls. Used to drive the new error-return path of
// initAndFetchSubmodules.
type stubEnumerationFailExecutor struct{}

func (s *stubEnumerationFailExecutor) Run(dir, name string, args ...string) ([]byte, error) {
	if name == "git" && len(args) >= 6 && args[2] == "config" && args[3] == "-f" && args[4] == ".gitmodules" {
		return []byte("garbled output from a broken .gitmodules\n"), errors.New("fatal: bad config")
	}
	return []byte{}, nil
}

func TestInitAndFetchSubmodules_EnumerationErrorPropagates(t *testing.T) {
	wt := t.TempDir()
	// Create an empty .gitmodules so the existence check passes.
	if err := os.WriteFile(filepath.Join(wt, ".gitmodules"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	gw := &GitWorktree{
		worktreePath: wt,
		executor:     &stubEnumerationFailExecutor{},
	}
	_, err := gw.initAndFetchSubmodules()
	if err == nil {
		t.Fatal("expected enumeration error to bubble up, got nil")
	}
}

// stubEmptyExit1Executor simulates `git config --get-regexp` exiting 1 with
// empty output — the "no keys matched" case that should be treated silently.
type stubEmptyExit1Executor struct{}

func (s *stubEmptyExit1Executor) Run(dir, name string, args ...string) ([]byte, error) {
	if name == "git" && len(args) >= 6 && args[2] == "config" && args[3] == "-f" && args[4] == ".gitmodules" {
		return []byte(""), errors.New("exit status 1")
	}
	return []byte{}, nil
}

func TestInitAndFetchSubmodules_NoSubmodulesRegisteredReturnsNil(t *testing.T) {
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".gitmodules"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	gw := &GitWorktree{
		worktreePath: wt,
		executor:     &stubEmptyExit1Executor{},
	}
	results, err := gw.initAndFetchSubmodules()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %+v", results)
	}
}
