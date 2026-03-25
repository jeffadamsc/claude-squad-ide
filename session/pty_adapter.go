package session

import (
	ptyPkg "claude-squad/pty"
)

// Program constants for supported AI coding assistants.
const (
	ProgramClaude = "claude"
	ProgramAider  = "aider"
	ProgramGemini = "gemini"
)

// ProcessManager abstracts the terminal process lifecycle.
// Implemented by pty.Manager (replaces tmux.TmuxSession).
type ProcessManager interface {
	Spawn(program string, args []string, opts ptyPkg.SpawnOptions) (string, error)
	Kill(id string) error
	Resize(id string, rows, cols uint16) error
	HasUpdated(id string) (updated bool, hasPrompt bool)
	CheckTrustPrompt(id string) bool
	GetContent(id string) string
	Write(id string, data []byte) error
}
