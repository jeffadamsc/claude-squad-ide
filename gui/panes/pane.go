package panes

import (
	"claude-squad/session"
	"fmt"
	"image/color"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

var (
	colorGreen      = color.NRGBA{R: 0xa6, G: 0xe3, B: 0xa1, A: 0xff}
	colorYellow     = color.NRGBA{R: 0xf9, G: 0xe2, B: 0xaf, A: 0xff}
	colorOverlay    = color.NRGBA{R: 0x6c, G: 0x70, B: 0x86, A: 0xff}
	colorFocusBorder = color.NRGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff} // blue accent
	colorDimBorder   = color.NRGBA{R: 0x45, G: 0x47, B: 0x5a, A: 0xff}
)

// ShortcutRegistrar registers hotkey shortcuts on a target that supports AddShortcut.
type ShortcutRegistrar func(target ShortcutAdder)

// ShortcutAdder is anything with an AddShortcut method (Canvas, ShortcutHandler, etc).
type ShortcutAdder interface {
	AddShortcut(shortcut fyne.Shortcut, handler func(fyne.Shortcut))
}

// PaneActions provides callbacks for context menu actions that require
// access to the pane manager or application state.
type PaneActions struct {
	SplitHorizontal func()
	SplitVertical   func()
	PauseSession    func(*session.Instance)
	KillSession     func(*session.Instance)
}

// Pane represents a single terminal pane with a header bar.
type Pane struct {
	container    *fyne.Container // outer: stack(focusBorder, inner, overlay)
	inner        *fyne.Container // border layout with header + content
	header       *fyne.Container
	titleLabel   *widget.Label
	statusIcon   *canvas.Text
	branchLabel  *widget.Label
	hintLabel    *widget.Label
	focusBorder  *canvas.Rectangle
	overlay      *tapOverlay
	conn         *TerminalConnection
	canvas       fyne.Canvas
	focused      bool
	onFocus      func(*Pane)
	registerKeys ShortcutRegistrar
	actions      PaneActions
}

// tapOverlay is an invisible full-pane overlay that intercepts clicks.
// It fires the onTap callback and then forwards keyboard focus to an
// optional focusable widget (the terminal) so typing still works.
type tapOverlay struct {
	widget.BaseWidget
	onTap          func()
	onSecondaryTap func(*fyne.PointEvent)
	onScroll       func(dy float32)
	focusable      fyne.Focusable // set when a terminal is connected
	canvas         fyne.Canvas
}

// Scrolled implements fyne.Scrollable for hover-based mouse wheel scrolling.
func (t *tapOverlay) Scrolled(ev *fyne.ScrollEvent) {
	if t.onScroll != nil {
		t.onScroll(ev.Scrolled.DY)
	}
}

func newTapOverlay(c fyne.Canvas, onTap func()) *tapOverlay {
	t := &tapOverlay{canvas: c, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapOverlay) Tapped(_ *fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
	if t.focusable != nil && t.canvas != nil {
		t.canvas.Focus(t.focusable)
	}
}

func (t *tapOverlay) SecondaryTapped(ev *fyne.PointEvent) {
	if t.onSecondaryTap != nil {
		t.onSecondaryTap(ev)
	}
}

func (t *tapOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// NewPane creates a new empty pane.
func NewPane(onFocus func(*Pane), registerKeys ShortcutRegistrar, actions PaneActions, c fyne.Canvas) *Pane {
	p := &Pane{
		conn:         NewTerminalConnection(),
		registerKeys: registerKeys,
		actions:      actions,
		canvas:       c,
		titleLabel:  widget.NewLabel("No session"),
		statusIcon:  canvas.NewText("", colorOverlay),
		branchLabel: widget.NewLabel(""),
		onFocus:     onFocus,
	}

	p.branchLabel.TextStyle = fyne.TextStyle{Italic: true}
	p.statusIcon.TextSize = 14
	p.hintLabel = widget.NewLabel(hintText())
	p.hintLabel.TextStyle = fyne.TextStyle{Italic: true}

	p.header = container.NewHBox(
		p.statusIcon,
		p.titleLabel,
		layout.NewSpacer(),
		p.hintLabel,
		p.branchLabel,
	)

	p.focusBorder = canvas.NewRectangle(colorDimBorder)
	p.focusBorder.StrokeWidth = 2
	p.focusBorder.StrokeColor = colorDimBorder
	p.focusBorder.FillColor = color.Transparent

	// Transparent overlay on top of everything — catches clicks to focus this pane
	p.overlay = newTapOverlay(c, func() {
		if p.onFocus != nil {
			p.onFocus(p)
		}
	})
	p.overlay.onSecondaryTap = func(ev *fyne.PointEvent) {
		// Focus this pane first so split/pause/kill act on the correct pane
		if p.onFocus != nil {
			p.onFocus(p)
		}
		p.showContextMenu(ev)
	}

	const scrollLines = 3
	p.overlay.onScroll = func(dy float32) {
		term := p.conn.Terminal()
		if term == nil {
			return
		}
		if dy > 0 {
			term.ScrollUp(scrollLines)
		} else if dy < 0 {
			term.ScrollDown(scrollLines)
		}
	}

	emptyLabel := widget.NewLabel("Select a session to open here")
	emptyLabel.Alignment = fyne.TextAlignCenter
	emptyContent := container.NewCenter(emptyLabel)

	p.inner = container.NewBorder(p.header, nil, nil, nil, emptyContent)
	// Stack order: border (back), content (middle), overlay (front — intercepts taps)
	p.container = container.NewStack(p.focusBorder, p.inner, p.overlay)
	return p
}

// Widget returns the fyne canvas object for this pane.
func (p *Pane) Widget() fyne.CanvasObject {
	return p.container
}

// OpenSession connects this pane to a session's tmux terminal.
func (p *Pane) OpenSession(inst *session.Instance) error {
	if err := p.conn.Connect(inst); err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}
	p.titleLabel.SetText(inst.Title)
	p.branchLabel.SetText(inst.Branch)
	p.updateStatus(inst)

	// Register hotkey shortcuts on the terminal widget so they work when it has focus
	if p.registerKeys != nil {
		p.registerKeys(p.conn.Terminal())
	}

	// Tell the overlay to forward keyboard focus to this terminal after tap
	p.overlay.focusable = p.conn.Terminal()

	// Replace the pane content with the terminal widget
	p.inner.Objects = []fyne.CanvasObject{p.header, p.conn.Terminal()}
	p.inner.Layout = layout.NewBorderLayout(p.header, nil, nil, nil)
	p.inner.Refresh()
	return nil
}

// CloseSession disconnects the terminal and shows empty state.
func (p *Pane) CloseSession() {
	p.conn.Disconnect()
	p.overlay.focusable = nil
	p.titleLabel.SetText("No session")
	p.branchLabel.SetText("")
	p.statusIcon.Text = ""

	emptyLabel := widget.NewLabel("Select a session to open here")
	emptyLabel.Alignment = fyne.TextAlignCenter

	p.inner.Objects = []fyne.CanvasObject{p.header, container.NewCenter(emptyLabel)}
	p.inner.Layout = layout.NewBorderLayout(p.header, nil, nil, nil)
	p.inner.Refresh()
}

// Instance returns the connected instance, or nil.
func (p *Pane) Instance() *session.Instance {
	return p.conn.Instance()
}

// SetFocused updates the visual focus state of this pane.
func (p *Pane) SetFocused(focused bool) {
	p.focused = focused
	if focused {
		p.focusBorder.StrokeColor = colorFocusBorder
	} else {
		p.focusBorder.StrokeColor = colorDimBorder
	}
	p.focusBorder.Refresh()
}

// IsFocused returns whether this pane is focused.
func (p *Pane) IsFocused() bool {
	return p.focused
}

// updateStatus updates the status icon based on instance state.
func (p *Pane) updateStatus(inst *session.Instance) {
	if inst == nil {
		p.statusIcon.Text = ""
		return
	}
	switch inst.Status {
	case session.Running:
		p.statusIcon.Text = "●"
		p.statusIcon.Color = colorGreen
	case session.Ready:
		p.statusIcon.Text = "▲"
		p.statusIcon.Color = colorYellow
	case session.Loading:
		p.statusIcon.Text = "◌"
		p.statusIcon.Color = colorOverlay
	case session.Paused:
		p.statusIcon.Text = "⏸"
		p.statusIcon.Color = colorOverlay
	}
	p.statusIcon.Refresh()
}

// SendPageUp writes a PageUp escape sequence to the terminal PTY.
// Tmux interprets this to enter/scroll copy-mode one page up.
func (p *Pane) SendPageUp() {
	if term := p.conn.Terminal(); term != nil {
		term.Write([]byte("\x1b[5~"))
	}
}

// SendPageDown writes a PageDown escape sequence to the terminal PTY.
// Tmux interprets this to scroll copy-mode one page down.
func (p *Pane) SendPageDown() {
	if term := p.conn.Terminal(); term != nil {
		term.Write([]byte("\x1b[6~"))
	}
}

// UpdateStatus refreshes the pane header from current instance state.
func (p *Pane) UpdateStatus() {
	inst := p.conn.Instance()
	p.updateStatus(inst)
}

// Disconnect cleans up the terminal connection.
func (p *Pane) Disconnect() {
	p.conn.Disconnect()
}

func (p *Pane) showContextMenu(ev *fyne.PointEvent) {
	term := p.conn.Terminal()
	inst := p.conn.Instance()

	copyItem := fyne.NewMenuItem("Copy", func() {
		if term != nil {
			term.CopySelectedText(fyne.CurrentApp().Clipboard())
		}
	})
	if term == nil || !term.HasSelectedText() {
		copyItem.Disabled = true
	}

	pasteItem := fyne.NewMenuItem("Paste", func() {
		if term != nil {
			term.PasteText(fyne.CurrentApp().Clipboard())
		}
	})

	splitHItem := fyne.NewMenuItem("Split Horizontal", func() {
		if p.actions.SplitHorizontal != nil {
			p.actions.SplitHorizontal()
		}
	})

	splitVItem := fyne.NewMenuItem("Split Vertical", func() {
		if p.actions.SplitVertical != nil {
			p.actions.SplitVertical()
		}
	})

	pauseItem := fyne.NewMenuItem("Pause Session", func() {
		if inst != nil && p.actions.PauseSession != nil {
			p.actions.PauseSession(inst)
		}
	})
	if inst == nil {
		pauseItem.Disabled = true
	}

	killItem := fyne.NewMenuItem("Kill Session", func() {
		if inst != nil && p.actions.KillSession != nil {
			p.actions.KillSession(inst)
		}
	})
	if inst == nil {
		killItem.Disabled = true
	}

	menu := fyne.NewMenu("",
		copyItem,
		pasteItem,
		fyne.NewMenuItemSeparator(),
		splitHItem,
		splitVItem,
		fyne.NewMenuItemSeparator(),
		pauseItem,
		killItem,
	)

	popUp := widget.NewPopUpMenu(menu, p.canvas)
	popUp.ShowAtPosition(ev.AbsolutePosition)
}

func hintText() string {
	mod := "Ctrl+Shift"
	if runtime.GOOS == "darwin" {
		mod = "⌘⇧"
	}
	return fmt.Sprintf("Split: %s+\\  %s+-  |  Close: %s+W  |  Nav: %s+←→↑↓", mod, mod, mod, mod)
}
