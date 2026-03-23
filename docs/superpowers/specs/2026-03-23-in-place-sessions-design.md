# In-Place Sessions Design Spec

## Goal

Add an "in-place" session mode that runs the AI agent directly in the current working directory without creating git branches, worktrees, or any git state. This is for users who have already set up their workspace (e.g., on the right branch) and want Claude to work directly there.

## Motivation

The default session model creates isolated git worktrees per session — great for parallel work, but unnecessary when you're already on the right branch and want changes made directly. Creating worktrees adds startup time, disk usage, and complexity that isn't needed for this workflow.

## Design

### Session Creation

The session creation overlay gains a toggle at the top of the form:

```
[ ] In-place (no git isolation)
```

Two entry points:
- `n` — opens the overlay with the toggle **off** (default, existing behavior)
- `i` — opens the overlay with the toggle **on** (pre-selected for in-place)

When the in-place toggle is on:
- The branch picker is **hidden** (no branch to pick — you're working in cwd)
- The submodule picker is **hidden** (no worktree to initialize submodules in)
- The user only fills in the session name (and optionally selects a profile)

The toggle is part of the tab order, placed **before** the text area. The user can press `space` to toggle it, or use `tab` to skip past it.

### Data Model

`Instance` gains:
- `inPlace bool` field — true when this is an in-place session

`InstanceData` (storage) gains:
- `InPlace bool `json:"in_place,omitempty"`` — for serialization/deserialization

When `inPlace` is true:
- `gitWorktree` is `nil` throughout the instance's lifetime
- `Path` is set to the current working directory (used as the tmux session's working dir)
- `Branch` is set to whatever branch cwd is on at creation time (informational only, read via `git branch --show-current`)

### Instance Lifecycle

#### Start

When `inPlace` is true and `firstTimeSetup` is true:
- Skip all worktree creation (`NewGitWorktree`, `Setup`, `InitSubmodules`)
- Start the tmux session with `i.Path` as the working directory (instead of `i.gitWorktree.GetWorktreePath()`)
- Read the current branch name from cwd for display purposes

When `inPlace` is true and `firstTimeSetup` is false (restoring from storage):
- Skip worktree restoration
- Restore tmux session as normal

#### Kill

When `inPlace` is true:
- Close tmux session
- Skip worktree cleanup entirely (`gitWorktree` is nil)
- No git operations

#### Pause

When `inPlace` is true:
- Detach tmux session (same as normal)
- Skip all git operations (no dirty check, no commit, no worktree removal, no prune)
- Set status to Paused

#### Resume

When `inPlace` is true:
- Skip branch-checked-out check
- Skip worktree setup
- Skip submodule resume
- Restart tmux session in `i.Path`
- Set status to Running

#### UpdateDiffStats

When `inPlace` is true:
- Set `diffStats` to nil (no base commit to diff against)
- Return nil (no error)

#### Push

When `inPlace` is true:
- The `KeySubmit` (`s`) handler shows an error: "Push is not available for in-place sessions"

#### Serialization

`ToInstanceData`:
- Set `data.InPlace = true`
- Skip worktree data serialization (leave `Worktree` as zero value)

`FromInstanceData`:
- When `data.InPlace` is true, skip worktree restoration, set `inPlace = true`
- For paused in-place sessions: restore tmux session reference, set started = true
- For non-paused in-place sessions: call `Start(false)` as usual (which skips worktree setup because `inPlace` is true)

### Nil Guard Strategy

Rather than adding nil checks at every `gitWorktree` call site, the approach is to **early-return** in each Instance method when `inPlace` is true. This keeps the nil-guard logic concentrated in Instance methods rather than scattered across the codebase.

Methods that need in-place handling:
- `Start()` — skip worktree creation, use `i.Path` for tmux
- `Kill()` — skip worktree cleanup
- `Pause()` — skip git operations, just detach tmux
- `Resume()` — skip worktree setup, just restart tmux
- `UpdateDiffStats()` — return nil
- `GetGitWorktree()` — return error
- `ToInstanceData()` — skip worktree data
- `RepoName()` — derive from `i.Path` instead of `gitWorktree`
- `Paused()` helper — works as-is (checks status, not worktree)

### UI Changes

#### TextInputOverlay

- New `inPlaceToggle bool` field
- New `SetInPlace(bool)` method (called when `i` opens the overlay)
- Toggle rendered at the top of the overlay, before the text input
- When toggled on, branch picker and submodule picker are hidden (not rendered, not focusable)
- Focus order when in-place: inPlaceToggle → profilePicker → textarea → enterButton
- Focus order when not in-place: inPlaceToggle → profilePicker → textarea → branchPicker → submodulePicker → enterButton

#### Session List (ui/list.go)

- When `inPlace` is true, show `[in-place]` where the branch name normally appears
- No diff stats shown (they'll be nil)

#### App Keybindings (keys/keys.go)

- Add `KeyInPlace` iota value
- Map `"i"` to `KeyInPlace`
- Add keybinding help: `i` → "in-place session"

#### App Handler (app/app.go)

- `KeyInPlace` handler: same as `KeyNew` but calls `SetInPlace(true)` on the overlay before showing it
- `KeySubmit` handler: check `inPlace` and show error instead of pushing
- `statePrompt` submit handler: read `inPlaceToggle` from overlay, set on instance

### Accessor

`Instance` gains:
- `IsInPlace() bool` — returns `i.inPlace`
- `SetInPlace(bool)` — sets `i.inPlace`

The overlay gains:
- `IsInPlace() bool` — returns current toggle state
- `SetInPlace(bool)` — sets the toggle and updates visibility of branch/submodule pickers

### Backward Compatibility

- `omitempty` on the `in_place` JSON field means old sessions without the field deserialize as `inPlace = false` (normal sessions)
- No changes to existing session behavior
- The `i` key is currently unmapped

### Out of Scope

- In-place sessions do not support diff display (no base commit)
- In-place sessions do not support push (no dedicated branch)
- In-place sessions do not support checkout-to-clipboard on pause (no branch to copy)
- Multiple in-place sessions in the same directory are allowed but the user is responsible for conflicts (no isolation)
