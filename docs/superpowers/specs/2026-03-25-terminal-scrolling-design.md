# Terminal Pane Scrolling via Tmux Copy-Mode

## Problem

Terminal panes in the GUI display claude instance output but have no scrollback capability. When there's a lot of text, users cannot scroll up to read earlier content. The forked fyne-terminal widget only stores visible rows with no history buffer.

## Solution

Leverage tmux's built-in copy-mode and scrollback buffer. Intercept mouse wheel and keyboard events in the GUI, forward them as escape sequences to the tmux PTY, and let tmux handle scrollback rendering natively.

## Design

### Mouse Wheel Scrolling (Hover-Based)

The `tapOverlay` on each pane implements Fyne's `Scrollable` interface (`Scrolled(*fyne.ScrollEvent)`). When the user scrolls over any pane — regardless of focus state — the overlay routes the event to that pane's terminal connection.

The terminal widget exposes `ScrollUp(lines int)` and `ScrollDown(lines int)` methods that write SGR-encoded mouse wheel escape sequences to the PTY:

- Scroll up: `\x1b[<64;Col;RowM`
- Scroll down: `\x1b[<65;Col;RowM`

Tmux with `mouse on` interprets these sequences. Scrolling up enters copy-mode automatically and scrolls through the scrollback buffer. Scrolling down moves forward, and copy-mode exits when reaching the bottom.

### Keyboard Scrolling

New hotkeys registered in the hotkey system:

| Hotkey | Action |
|--------|--------|
| Ctrl+Shift+PageUp | Scroll up one page in focused pane |
| Ctrl+Shift+PageDown | Scroll down one page in focused pane |

These send the same escape sequences as mouse wheel but with a larger line count (matching the pane's visible row count for page-sized jumps).

### Tmux Configuration

Tmux sessions must have mouse mode enabled. Add `set -g mouse on` to the tmux session configuration during session creation. This enables tmux to interpret mouse wheel escape sequences and enter/exit copy-mode automatically.

## Files Changed

| File | Change |
|------|--------|
| `gui/panes/pane.go` | `tapOverlay` implements `Scrollable` interface; routes scroll events to terminal connection |
| `fyne-terminal-fork/input.go` | Add `ScrollUp(lines)` / `ScrollDown(lines)` methods that write mouse wheel escape sequences to PTY |
| `fyne-terminal-fork/term.go` | Export scroll methods publicly on the `Terminal` type |
| `gui/hotkeys.go` | Add Ctrl+Shift+PageUp/PageDown hotkeys for page scrolling |
| Session/tmux setup code | Ensure `set -g mouse on` is configured for tmux sessions |

## Scroll Event Flow

```
Mouse wheel on pane
  -> tapOverlay.Scrolled(event)
  -> look up pane's terminal connection
  -> terminal.ScrollUp(lines) or terminal.ScrollDown(lines)
  -> write SGR mouse wheel escape sequence to PTY
  -> tmux receives sequence, enters/manages copy-mode
  -> tmux re-renders terminal content with scrolled view
  -> fyne-terminal widget displays updated content
```

## Edge Cases

- **New output while scrolled**: Tmux copy-mode stays at the scrolled position. New content appends below. The user remains at their scroll position until they scroll back to the bottom or press `q`/`Escape` to exit copy-mode.
- **Multiple panes**: Hover-based routing ensures only the pane under the cursor receives scroll events. No cross-talk between panes.
- **Scroll speed**: Each mouse wheel tick sends 3 lines of scroll. Adjustable if needed.
- **Pane resize while in copy-mode**: Tmux handles re-rendering the scrollback at the new size.

## Non-Goals

- Custom scrollback buffer in the terminal widget (future enhancement)
- Scrollbar UI element
- Search within scrollback (tmux copy-mode supports `/` search natively)
