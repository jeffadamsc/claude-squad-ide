# Base Branch Selection for New Sessions

**Date:** 2026-03-23
**Status:** Approved

## Problem

When creating a new session, `setupNewWorktree()` always branches from HEAD (the current checked-out commit). This means if you're on a feature branch, new sessions inherit that branch's state. Users often want to start fresh from the canonical default branch (`origin/main` or `origin/master`), with the latest remote state.

There is a TODO in `worktree_ops.go:91` that calls this out.

## Design

### Branch Picker Changes

The branch picker (`ui/overlay/branchPicker.go`) currently shows a hardcoded first option `"New branch (from HEAD)"`. This changes to a dynamic list of "new branch" options based on detected remote branches.

**Detection:** At picker initialization, run `git branch -r` and check for `origin/main` and `origin/master`. Build the options list in this order:

1. "New branch (from origin/main)" -- if detected
2. "New branch (from origin/master)" -- if detected
3. "New branch (from HEAD)"
4. Existing branches (as today, sorted by commit date)

**Exported interface:** The picker needs to communicate not just "new branch vs existing" but also which base was chosen. Add methods:

- `IsNewBranch() bool` -- true if any "New branch" option was selected
- `BaseBranch() string` -- returns `"origin/main"`, `"origin/master"`, or `"HEAD"`

The constant `newBranchOption` becomes a set of possible new-branch options. Downstream code that checks `Selected() == newBranchOption` changes to use `IsNewBranch()` instead.

### Fetch at Session Creation

When the user confirms a new session and the selected base is an origin branch (not HEAD), a `git fetch origin` runs before worktree creation.

**Location:** In `worktree_ops.go`, at the top of `setupNewWorktree()`. If the base is `origin/main` or `origin/master`, run `git fetch origin` first, then resolve the base commit with `git rev-parse <baseBranch>` instead of `git rev-parse HEAD`.

**Error handling:** If fetch fails (no network), warn but proceed using the local state of the remote-tracking branch. The branch may be stale but it's better than failing entirely.

### Worktree Creation Changes

`setupNewWorktree()` currently always resolves HEAD:

```
git rev-parse HEAD -> headCommit
git worktree add -b {branch} {path} {headCommit}
```

This changes to accept a base ref parameter:

- If base is `origin/main` or `origin/master`: `git rev-parse origin/main` -> baseCommit
- If base is `HEAD`: existing behavior, `git rev-parse HEAD` -> baseCommit

The worktree is then created from the resolved base commit:

```
git worktree add -b {branch} {path} {baseCommit}
```

**Data flow:** The `GitWorktree` struct gets a new `baseBranch string` field. `Instance.selectedBaseBranch` (new field) carries the user's choice from the UI to the `GitWorktree` constructor, which passes it to `setupNewWorktree()`.

### Submodule Handling

Submodules do NOT use the parent's base branch choice. Each submodule auto-detects its own default branch independently during `SubmoduleWorktree.Setup()`:

1. Check if `origin/main` exists in the submodule: `git branch -r --list origin/main`
2. If not, check `origin/master`
3. If neither exists, fall back to HEAD

This detection logic is a shared helper function (`detectDefaultRemoteBranch(repoPath)`) reusable by both the branch picker and submodule setup.

Each submodule also runs `git fetch origin` before branching (with the same error-tolerant handling as the parent).

### Persistence

Add `BaseBranch string` to the `InstanceData` struct in `storage.go` for JSON serialization. This is informational after initial creation (the worktree already exists on the right branch) but is needed for display and submodule resume logic.

## Files Changed

- `ui/overlay/branchPicker.go` -- dynamic new-branch options, `IsNewBranch()`, `BaseBranch()` methods
- `session/git/worktree_ops.go` -- accept base ref, fetch origin, resolve correct base commit
- `session/git/worktree.go` -- `baseBranch` field on `GitWorktree`
- `session/git/submodule.go` -- auto-detect default branch, fetch before setup
- `session/git/detect.go` -- shared `detectDefaultRemoteBranch()` helper
- `session/instance.go` -- `selectedBaseBranch` field, pass through to worktree
- `session/storage.go` -- `BaseBranch` field on `InstanceData`
- `app/app.go` -- pass base branch selection from overlay to instance

## Not in Scope

- Config-level base branch setting (can be added later if needed)
- Per-submodule UI selection of base branch (submodules auto-detect)
- Eager fetch on picker open (fetch happens at creation time only)
