# Terminal Pane Context Menu

## Summary

Add a right-click context menu to terminal panes in the claude-squad GUI. Replace the current right-click-to-paste behavior with a `PopUpMenu` offering copy, paste, split, pause, and kill actions.

## Context Menu Items

| Item | Condition | Action |
|------|-----------|--------|
| Copy | Enabled only when text is selected | Copy selected text to clipboard |
| Paste | Always enabled | Paste clipboard content into terminal |
| *(separator)* | | |
| Split Horizontal | Always enabled | Split pane top/bottom |
| Split Vertical | Always enabled | Split pane left/right |
| *(separator)* | | |
| Pause Session | Always enabled (only reachable on active sessions) | Pause the session |
| Kill Session | Always enabled | Kill the session, no confirmation |

Resume Session is intentionally excluded — paused sessions are not displayed in terminal panes, so resume would never be reachable.

## Approach

Use Fyne's built-in `widget.PopUpMenu` for native look, automatic dismiss-on-click-outside, and keyboard navigation.

## Implementation Points

### 1. Terminal widget callback (`fyne-terminal-fork/mouse.go`)

- Remove existing right-click-to-paste behavior from `MouseDown`
- Add exported callback field: `OnSecondaryMouseDown func(fyne.Position)`
- On secondary button press, invoke the callback with the event position (converted to canvas coordinates)

### 2. Context menu construction (`gui/panes/pane.go`)

- Set `OnSecondaryMouseDown` on the terminal widget when connecting
- In the callback, build a `fyne.NewMenu` with the items above
- Query `t.HasSelectedText()` (new exported method) to determine if Copy should be enabled or disabled (grayed out)
- Show the menu via `widget.NewPopUpMenu` at the click position on the canvas
- Menu item handlers call existing functions:
  - Copy: `t.copySelectedText(clipboard)`
  - Paste: `t.pasteText(clipboard)`
  - Split: pane manager's `SplitHorizontal()` / `SplitVertical()`
  - Pause: instance's pause function
  - Kill: instance's kill function

### 3. New exported method on Terminal (`fyne-terminal-fork/select.go`)

- `HasSelectedText() bool` — returns true if there is an active text selection

### Files Changed

- `fyne-terminal-fork/mouse.go` — remove right-click paste, add `OnSecondaryMouseDown` callback
- `fyne-terminal-fork/term.go` — add `OnSecondaryMouseDown` field to Terminal struct
- `fyne-terminal-fork/select.go` — add `HasSelectedText()` method
- `gui/panes/pane.go` — construct and show context menu on right-click callback
