# Adaptive Memory Management

**Date:** 2026-04-16
**Status:** Approved

## Problem

On an 8 GB MacBook, claude-squad and its child processes consume ~1.85 GB (physical footprint) after running overnight:

- `cs` process: 547 MB (485 MB Go heap swapped to disk)
- 3 idle Claude CLI children: ~1.3 GB (WebKit Malloc, never shrinks)
- 36+ orphaned node processes from past sessions: ~150 MB

The system had 58 MB free, 8.2 GB swap used, and 2.9 GB in the macOS compressor. The root causes:

1. `GOMEMLIMIT` is hardcoded to 512 MiB regardless of system RAM.
2. Go's GC marks memory as free but doesn't return it to the OS. Under memory pressure, macOS swaps these freed-but-held pages rather than reclaiming them.
3. Claude CLI child processes (one per running session) stay alive indefinitely with no idle timeout. Each holds 200-400 MB of WebKit Malloc for conversation context.
4. Child processes spawned by Claude sessions (dev servers, watchers, esbuild) outlive the session and accumulate over time.

## Approach

Adaptive memory management that detects system RAM at startup and scales behavior. Conservative on 8 GB machines, permissive on 32+ GB machines. No user configuration required (but overridable via config).

## Design

### 1. Memory Detection & Adaptive Configuration

**New file: `app/memory.go`**

Detect total system RAM at startup using `syscall.Sysctlbyname("hw.memsize", ...)` on macOS. Derive scaled limits:

| System RAM | GOMEMLIMIT | IdleTimeout |
|------------|------------|-------------|
| <= 8 GB    | 128 MiB    | 15 min      |
| <= 16 GB   | 256 MiB    | 30 min      |
| <= 32 GB   | 384 MiB    | 60 min      |
| > 32 GB    | 512 MiB    | disabled    |

These are defaults. If a future config option is added, user values take precedence.

**Changes to `main.go`:**

Replace the hardcoded `debug.SetMemoryLimit(512 * 1024 * 1024)` with a call to `app/memory.go` that returns the adaptive limit. Pass the computed `IdleTimeout` to `SessionAPI` via `SessionAPIOptions`.

### 2. Go Heap Memory Release

**Changes to `app/indexer_treesitter.go`:**

Add `debug.FreeOSMemory()` at the end of `TreeSitterIndexer.build()`, after the index is built and persisted. This forces Go to return freed heap pages to the OS immediately rather than holding them indefinitely.

This is unconditional (not gated by RAM tier) since it's cheap and always beneficial. The call takes <1ms and prevents the scenario where Go holds hundreds of MB of freed-but-not-returned pages that macOS then swaps.

### 3. Auto-Pause Idle Sessions

When a session has been in `Ready` state (Claude showing the `❯` prompt, waiting for user input) for longer than `IdleTimeout`, automatically pause it. This kills the Claude CLI process and stops the indexer, reclaiming all associated memory.

**Changes to `session/instance.go`:**

Add three fields to `Instance`:

- `IdleSince time.Time` — set when session transitions to idle (prompt visible). Cleared when session becomes active (Running) or receives input.
- `LastViewed time.Time` — touched by `OpenSession()`. Prevents auto-pausing sessions the user is actively looking at.
- `AutoPaused bool` — set to true when auto-paused (vs manually paused). Allows the frontend to show a distinct indicator.

**Changes to `app/bindings.go` — `PollAllStatuses()`:**

This method already iterates all instances and checks `hasPrompt` on every poll cycle. Add idle-tracking logic:

1. If `hasPrompt == true` and `status` is `Running` or `Ready`:
   - If `inst.IdleSince.IsZero()`, set it to `time.Now()`.
   - If `time.Since(inst.IdleSince) > idleTimeout` and `time.Since(inst.LastViewed) > idleTimeout`, auto-pause the session. Set `inst.AutoPaused = true` before calling `PauseSession(id)`.
2. If `hasPrompt == false` (Claude is working), reset `inst.IdleSince` to zero.

**Locking note:** `PollAllStatuses` currently holds `RLock`. The idle check should collect session IDs to auto-pause, then release `RLock`, then call `PauseSession` (which acquires a write lock) for each. This avoids lock upgrade issues.

**Edge cases:**

- Sessions being viewed in the UI are protected by `LastViewed` — `OpenSession()` touches this timestamp. A session won't be auto-paused if the user has it open within the timeout window.
- `PauseSession` already commits all git changes before killing the process, so no work is lost.
- On resume, the existing flow handles everything: worktree recreation, Claude CLI with `--continue`, indexer restart from `.gob` cache.

**Frontend changes:**

The `SessionStatus` struct already has a `Status` field. Add `AutoPaused bool` so the frontend can display "Auto-paused (idle)" vs "Paused" in the session list.

### 4. Orphaned Child Process Cleanup

Processes spawned inside a session's worktree (node dev servers, typed-scss-modules watchers, esbuild instances) can outlive the Claude CLI process and accumulate.

**New file: `session/cleanup.go`**

`killWorktreeProcesses(worktreePath string)`:

1. Run `pgrep -f <worktreePath>` to find PIDs whose command-line arguments reference the worktree.
2. Send `SIGTERM` to each PID.
3. Wait up to 3 seconds for graceful exit.
4. Send `SIGKILL` to any survivors.

**Integration points:**

- Call from `Instance.Pause()` and `Instance.Kill()`, *before* removing the worktree directory (so the path is still matchable in process args).
- Call on app startup in `SessionAPI` initialization: iterate all known worktree paths and kill orphaned processes for any session that is in `Paused` state (i.e., no session process should be running, but orphaned children may exist).

**Safety:** Only targets processes whose args contain the specific worktree path (e.g. `~/.claude-squad/worktrees/jadams/session-name_id/`). This is a narrow match that won't hit unrelated processes.

## Files Touched

| File | Change |
|------|--------|
| `main.go` | Replace hardcoded GOMEMLIMIT with adaptive value from `memory.go`. Pass `IdleTimeout` to `SessionAPIOptions`. |
| `app/memory.go` (new) | System RAM detection via syscall. Compute scaled GOMEMLIMIT and IdleTimeout. |
| `app/indexer_treesitter.go` | Add `debug.FreeOSMemory()` at end of `build()`. |
| `app/bindings.go` | Idle-check logic in `PollAllStatuses()`. Startup orphan cleanup. Accept `IdleTimeout` in `SessionAPIOptions`. |
| `session/instance.go` | Add `IdleSince`, `LastViewed`, `AutoPaused` fields. Touch `LastViewed` in appropriate methods. |
| `session/cleanup.go` (new) | `killWorktreeProcesses()` helper. |

## What This Does NOT Do

- No real-time memory pressure monitoring (reactive approach rejected in favor of proactive).
- No independent indexer unloading timer — auto-pause handles indexer cleanup via the existing `PauseSession` path.
- No user-facing configuration (yet) — adaptive defaults should be correct for all machines. Config can be added later if needed.
