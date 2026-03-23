# Submodule-Aware Sessions — Technical Details

This document describes how Claude Squad manages git submodules within sessions, enabling isolated concurrent development across a monorepo with multiple submodules.

## Architecture

Each session creates an isolated workspace using git worktrees. For repos with submodules, the architecture extends to create **nested worktrees**: a parent worktree plus one worktree per active submodule.

```
~/.claude-squad/worktrees/
  session-abc/                  ← parent worktree (git worktree of main repo)
    verve-backend/              ← submodule worktree (git worktree of submodule's git dir)
    verve-portal/               ← another submodule worktree
```

Two sessions can work on the **same submodule** simultaneously — each gets its own branch and worktree, so changes are fully isolated.

### Key Design Decisions

- **Dynamic git dir discovery**: Submodule git directories are found via `git -C <path> rev-parse --git-dir` rather than assuming a path convention. This handles all git configurations (embedded `.git/modules/`, external git dirs, etc.).
- **Lazy initialization**: Submodule worktrees are only created for submodules you select, not all of them. This keeps sessions lightweight when your repo has many submodules.
- **Parent pointer isolation**: During pause, submodule pointer changes in the parent worktree are discarded (`git checkout -- <submodule-paths>`). This prevents one session's submodule commits from leaking into the parent's tracked state.

## Data Model

### SubmoduleWorktree

Each active submodule in a session is represented by a `SubmoduleWorktree` struct:

| Field | Description |
|-------|-------------|
| `submodulePath` | Relative path within parent repo (e.g., `verve-backend`) |
| `gitDir` | Absolute path to the submodule's git directory |
| `worktreePath` | Absolute path to the submodule's worktree (inside parent worktree) |
| `branchName` | Branch name (matches parent session branch for consistency) |
| `baseCommitSHA` | Original commit the worktree was created from (for diffing) |

### GitWorktree Extensions

The parent `GitWorktree` gains:
- `submodules map[string]*SubmoduleWorktree` — active submodule worktrees, keyed by relative path
- `isSubmoduleAware bool` — whether this session has submodule support enabled

### Serialization

Submodule state is persisted alongside the parent worktree data with `omitempty` JSON tags, ensuring backward compatibility — sessions created before submodule support work exactly as before.

## Session Lifecycle

### Creation

1. User creates a new session (`n` or `N`)
2. If the repo has submodules, a multi-select picker appears below the branch picker
3. User selects submodules with `space` (or `a` to select all)
4. Parent worktree is created via `git worktree add`
5. For each selected submodule:
   - The submodule's git directory is discovered
   - The empty placeholder directory (from parent worktree) is removed
   - A new worktree is created: `git --git-dir=<gitdir> worktree add [-b] <branch> <path>`

### Active Session

While a session is running:
- The AI agent works inside the parent worktree, which contains real submodule worktrees (not just git pointers)
- Diffs are aggregated across parent + all submodules, shown with section headers in the diff pane
- The session list shows active submodule names in brackets: `feature-branch [backend,portal]`

### Pause (Checkout)

When pausing a session (`c`), the order matters:

1. **Submodules first**: For each submodule, commit any dirty changes, then remove the worktree
2. **Discard parent pointers**: Run `git checkout -- <submodule-paths>` in the parent worktree to revert any submodule pointer changes
3. **Parent last**: Commit parent changes and remove the parent worktree

This ordering prevents submodule pointer drift from contaminating the parent's commit history.

### Resume

When resuming a paused session (`r`):

1. Parent worktree is recreated via `git worktree add`
2. All submodules are deinitialized (`git submodule deinit --all -f`) to clear stale `.git` files
3. Each submodule worktree is recreated from its branch
4. The original `baseCommitSHA` is preserved so diffs continue to show changes since session creation

### Push

When pushing a session (`s`):

1. Parent branch is pushed normally
2. If submodule-aware, each submodule branch is pushed independently
3. A reminder is shown: "Remember to update submodule pointers in the parent repo when ready"

Submodule pushes are independent of the parent — you update submodule pointers in the parent repo as a separate step when you're ready to integrate.

### Kill (Delete)

When killing a session (`D`):

1. Each submodule worktree is removed and its branch deleted (unless it was pre-existing)
2. Parent worktree is removed and its branch deleted
3. Worktree references are pruned

### Global Cleanup

The `cs reset` command handles submodule worktrees by:

1. Walking each parent worktree directory for `.git` files (not directories)
2. Reading the `gitdir:` pointer from each `.git` file
3. Running `git --git-dir=<gitdir> worktree remove --force` to properly unregister
4. Then removing the parent directory and pruning

## Diff Aggregation

Diffs are computed per-component and aggregated:

- **Parent diff**: `git diff <baseCommitSHA>` in the parent worktree
- **Submodule diffs**: `git diff <baseCommitSHA>` in each submodule worktree
- **Combined stats**: `TotalAdded = parent.Added + sum(submodule.Added)`, same for removed
- **Combined content**: Section headers (`--- parent ---`, `--- verve-backend ---`) separate each component's diff output

The session list shows combined `+N,-M` stats. The diff pane shows the full content with section headers.

## File Layout

| File | Purpose |
|------|---------|
| `session/git/detect.go` | `ListSubmodules()`, `HasSubmodules()` — submodule discovery |
| `session/git/submodule.go` | `SubmoduleWorktree` struct — per-submodule lifecycle |
| `session/git/worktree.go` | `GitWorktree` extensions — `InitSubmodules()`, `RestoreSubmodules()` |
| `session/git/worktree_ops.go` | Pause/resume/cleanup operations |
| `session/git/worktree_git.go` | Push operations including `PushSubmoduleChanges()` |
| `session/git/diff.go` | `AggregatedDiffStats`, `AggregatedDiff()` |
| `session/storage.go` | `SubmoduleWorktreeData` serialization |
| `session/instance.go` | Instance lifecycle integration |
| `ui/overlay/submodulePicker.go` | Multi-select picker UI component |
| `ui/overlay/textInput.go` | Overlay integration for session creation |
| `ui/list.go` | Submodule indicators in session list |
| `app/app.go` | App-level wiring |

## Test Coverage

Tests are organized by scope:

| Test File | What It Covers |
|-----------|---------------|
| `session/git/detect_test.go` | Submodule detection, edge cases (no submodules, not a git repo), multi-submodule helper |
| `session/git/submodule_test.go` | SubmoduleWorktree CRUD, dirty/commit, diff, aggregated diff, baseCommitSHA preservation, multiple submodules, pause/resume |
| `session/git/integration_test.go` | Full end-to-end lifecycle: detect → create → init → modify → diff → pause → resume → verify → cleanup |
| `session/storage_test.go` | JSON serialization round-trips, backward compatibility, multiple submodules |
| `ui/overlay/submodulePicker_test.go` | Selection, select-all, navigation, empty state, unfocused state |
