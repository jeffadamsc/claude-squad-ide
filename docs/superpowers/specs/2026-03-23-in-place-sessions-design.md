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

The toggle is part of the tab order, placed **before** the text area. When the toggle is focused, `space` toggles the value. This is a new key handler specifically for the toggle focus stop — it does not affect the textarea (which is a separate focus stop).

### Data Model

`Instance` gains:
- `inPlace bool` field — true when this is an in-place session

`InstanceOptions` gains:
- `InPlace bool` — set during construction (cleaner than calling `SetInPlace` after)

`InstanceData` (storage) gains:
- `InPlace bool` with `json:"in_place,omitempty"` — for serialization/deserialization

When `inPlace` is true:
- `gitWorktree` is `nil` throughout the instance's lifetime
- `Path` is set to the current working directory (used as the tmux session's working dir)
- `Branch` is set to whatever branch cwd is on at creation time (informational only, read via `git branch --show-current`). If cwd is not a git repo, `Branch` is set to empty string.

### Instance Lifecycle

**Critical implementation note:** Every Instance method that accesses `gitWorktree` must early-return when `inPlace` is true, **before** any `gitWorktree` access. A nil `gitWorktree` will panic otherwise.

#### Start

When `inPlace` is true and `firstTimeSetup` is true:
- Skip all worktree creation (`NewGitWorktree`, `Setup`, `InitSubmodules`)
- Start the tmux session with `i.Path` as the working directory (instead of `i.gitWorktree.GetWorktreePath()`)
- Read the current branch name from cwd for display purposes (best-effort, empty string if not a git repo)

When `inPlace` is true and `firstTimeSetup` is false (restoring from storage):
- Skip worktree restoration
- Restore tmux session as normal

#### Kill

When `inPlace` is true:
- Close tmux session
- Skip worktree cleanup entirely (`gitWorktree` is nil)
- No git operations

**App-level handler (`KeyKill` in `app/app.go`):** The kill handler currently calls `GetGitWorktree()` and `IsBranchCheckedOut()` before killing. For in-place sessions, skip this entire branch-checkout check (there's no branch to worry about). Check `selected.IsInPlace()` at the top of the handler.

#### Pause

When `inPlace` is true, the early-return must happen at the top of `Pause()`, **before** `i.gitWorktree.IsSubmoduleAware()` (which is the first gitWorktree access):
- Detach tmux session via `i.tmuxSession.DetachSafely()`
- Set status to Paused
- Skip all git operations (no dirty check, no commit, no worktree removal, no prune, no clipboard write)
- Return nil

The `KeyCheckout` handler in `app/app.go` calls `selected.Pause()` — this works for in-place sessions because `Pause()` handles the early return. No additional changes needed in the handler.

#### Resume

When `inPlace` is true, the early-return must happen at the top of `Resume()`, **before** `i.gitWorktree.IsBranchCheckedOut()`:
- Skip branch-checked-out check
- Skip worktree setup and submodule resume
- Start tmux session with `i.Path` as the working directory (via `i.tmuxSession.Start(i.Path)`)
- Set status to Running
- Return nil

#### UpdateDiffStats

When `inPlace` is true, the early-return must happen at the top, **before** `i.gitWorktree.IsSubmoduleAware()`:
- Set `diffStats` to nil (no base commit to diff against)
- Return nil (no error)

#### Push

When `inPlace` is true:
- The `KeySubmit` (`p`) handler in `app/app.go` checks `selected.IsInPlace()` at the top of the case, **before** creating the confirmation modal or calling `GetGitWorktree()`, and shows an error: "Push is not available for in-place sessions"

#### RepoName

When `inPlace` is true:
- Derive repo name from `i.Path` using `filepath.Base()` instead of calling `i.gitWorktree.GetRepoName()`
- This is called from `ui/list.go` for repo-tracking (multiple repos display). It will work correctly — the list's `addRepo`/`rmRepo` tracking will use the path-derived name.

#### Serialization

`ToInstanceData`:
- Set `data.InPlace = true`
- Skip worktree data serialization entirely (leave `Worktree` as zero value)
- Skip diff stats serialization (leave `DiffStats` as zero value)

`FromInstanceData`:
- When `data.InPlace` is true:
  - Set `inPlace = true` on the instance
  - **Do not** construct a `GitWorktreeFromStorage` — skip the entire worktree construction block (lines 138-162 in current code)
  - Set `diffStats` to nil (no diff stats to restore)
  - For paused in-place sessions: create tmux session reference, set `started = true`
  - For non-paused in-place sessions: call `Start(false)` which skips worktree setup because `inPlace` is true

### Nil Guard Strategy

The approach is to **early-return** in each Instance method when `i.inPlace` is true. This keeps the nil-guard logic concentrated in Instance methods rather than scattered across the codebase.

**Instance methods** that need in-place early-returns (before any `gitWorktree` access):
- `Start()` — skip worktree creation, use `i.Path` for tmux
- `Kill()` — skip worktree cleanup
- `Pause()` — skip git operations, just detach tmux and set Paused
- `Resume()` — skip worktree setup, just restart tmux in `i.Path`
- `UpdateDiffStats()` — set nil, return nil
- `GetGitWorktree()` — return nil + error
- `ToInstanceData()` — skip worktree data
- `RepoName()` — derive from `i.Path` via `filepath.Base()`
- `GetActiveSubmodulePaths()` — return nil (existing nil-guards already handle this)

**App-level handlers** in `app/app.go` that access `GetGitWorktree()` directly and need `IsInPlace()` checks:
- `KeyKill` handler — skip `IsBranchCheckedOut` check
- `KeySubmit` handler — show error instead of pushing

### UI Changes

#### TextInputOverlay

- New `inPlaceToggle bool` field
- New `SetInPlace(bool)` / `IsInPlace() bool` methods
- Toggle rendered at the top of the overlay, before the profile picker
- When focused, `space` toggles the value (new key handler for this focus stop only)
- When toggled on, branch picker and submodule picker are hidden (not rendered, not in tab order)
- The `numStops` calculation and all focus index helpers (`isProfilePicker()`, `isTextarea()`, etc.) must be updated to account for the new toggle stop and the dynamic removal of branch/submodule stops. The current code uses hardcoded index arithmetic — this needs to become dynamic based on which pickers are visible.
- Focus order when in-place: inPlaceToggle → profilePicker → textarea → enterButton
- Focus order when not in-place: inPlaceToggle → profilePicker → textarea → branchPicker → submodulePicker → enterButton

#### Session List (ui/list.go)

- When `inPlace` is true (via `i.IsInPlace()`), show `[in-place]` where the branch name normally appears
- No diff stats shown (they'll be nil)

#### App Keybindings (keys/keys.go)

- Add `KeyInPlace` iota value
- Map `"i"` to `KeyInPlace`
- Add keybinding help: `i` → "in-place session"

#### App Handler (app/app.go)

- `KeyInPlace` handler: same as `KeyNew` but calls `SetInPlace(true)` on the overlay before showing it
- `KeyKill` handler: check `selected.IsInPlace()` and skip branch-checkout check
- `KeySubmit` handler: check `selected.IsInPlace()` and show error instead of pushing
- `statePrompt` submit handler: read `IsInPlace()` from overlay, pass to `InstanceOptions`

### Accessor

`Instance` gains:
- `IsInPlace() bool` — returns `i.inPlace`

The overlay gains:
- `IsInPlace() bool` — returns current toggle state
- `SetInPlace(bool)` — sets the toggle and updates visibility of branch/submodule pickers

### Non-Git Directories

In-place sessions work in non-git directories. The only git operation during creation is reading the current branch name (`git branch --show-current`) for display purposes — if this fails, `Branch` is set to an empty string and the session list shows `[in-place]` with no branch name. All other operations are git-free.

### Backward Compatibility

- `omitempty` on the `in_place` JSON field means old sessions without the field deserialize as `inPlace = false` (normal sessions)
- No changes to existing session behavior
- The `i` key is currently unmapped

### Out of Scope

- In-place sessions do not support diff display (no base commit)
- In-place sessions do not support push (no dedicated branch)
- In-place sessions do not support checkout-to-clipboard on pause (no branch to copy). The `KeyCheckout` handler still works — it calls `Pause()` which detaches the tmux session.
- Multiple in-place sessions in the same directory are allowed but the user is responsible for conflicts (no isolation)
