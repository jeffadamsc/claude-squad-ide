package git

import "fmt"

// SubmoduleStage labels which phase of submodule setup failed.
// Kept as a string so it serializes cleanly across the Wails bridge.
type SubmoduleStage string

const (
	SubmoduleStageInit   SubmoduleStage = "init"
	SubmoduleStageFetch  SubmoduleStage = "fetch"
	SubmoduleStageVerify SubmoduleStage = "verify"
)

// SubmoduleSetupResult records a single submodule that ended up in a bad state
// after Setup. Submodules that succeed are not represented — the absence of a
// result is the success signal.
type SubmoduleSetupResult struct {
	Name  string         `json:"name"`  // path relative to worktree root
	Stage SubmoduleStage `json:"stage"` // first stage that failed
	Err   error          `json:"-"`     // not JSON-serialized; use ErrorString for transport
}

// ErrorString returns a printable form of the error, safe for JSON / IPC.
func (r SubmoduleSetupResult) ErrorString() string {
	if r.Err == nil {
		return ""
	}
	return r.Err.Error()
}

// SetupResult is the new return shape for GitWorktree.Setup. It carries any
// per-submodule failures encountered during the submodule-init phase.
// All-success setup returns SubmoduleFailures == nil.
type SetupResult struct {
	SubmoduleFailures []SubmoduleSetupResult
}

// HasFailures returns true iff at least one submodule did not end up populated.
func (s SetupResult) HasFailures() bool {
	return len(s.SubmoduleFailures) > 0
}

// String renders a short multi-line summary, useful for logs.
func (s SetupResult) String() string {
	if !s.HasFailures() {
		return "SetupResult{ok}"
	}
	return fmt.Sprintf("SetupResult{%d submodule failures}", len(s.SubmoduleFailures))
}
