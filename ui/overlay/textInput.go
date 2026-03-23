package overlay

import (
	"claude-squad/config"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	tiStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	tiTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginBottom(1)

	tiButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	tiFocusedButtonStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("0"))

	tiDividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// TextInputOverlay represents a text input overlay with state management.
type TextInputOverlay struct {
	textarea      textarea.Model
	Title         string
	FocusIndex    int // index into focusable stops
	Submitted     bool
	Canceled      bool
	OnSubmit      func()
	width         int
	height        int
	profilePicker   *ProfilePicker
	branchPicker    *BranchPicker
	submodulePicker *SubmodulePicker
	numStops        int  // total number of focus stops
	inPlace         bool // whether in-place mode is active (no git isolation)
}

// NewTextInputOverlay creates a new text input overlay with the given title and initial value.
func NewTextInputOverlay(title string, initialValue string) *TextInputOverlay {
	ti := newTextarea(initialValue)
	return &TextInputOverlay{
		textarea: ti,
		Title:    title,
		numStops: 3, // toggle + textarea + enter button
	}
}

// NewTextInputOverlayWithBranchPicker creates a text input overlay that includes an
// empty branch picker. Results are populated asynchronously via SetBranchResults.
func NewTextInputOverlayWithBranchPicker(title string, initialValue string, profiles []config.Profile) *TextInputOverlay {
	ti := newTextarea(initialValue)
	bp := NewBranchPicker()

	var pp *ProfilePicker
	if len(profiles) > 0 {
		pp = NewProfilePicker(profiles)
	}

	numStops := 4 // toggle + textarea + branch picker + enter button
	if pp != nil && pp.HasMultiple() {
		numStops = 5 // toggle + profile picker + textarea + branch picker + enter button
	}

	overlay := &TextInputOverlay{
		textarea:      ti,
		Title:         title,
		profilePicker: pp,
		branchPicker:  bp,
		numStops:      numStops,
	}
	overlay.updateFocusState()
	return overlay
}

// NewTextInputOverlayWithSubmodules creates a text input overlay that includes a branch picker
// and a submodule picker for selecting which submodules to initialize in the worktree.
func NewTextInputOverlayWithSubmodules(title string, initialValue string, profiles []config.Profile, submodules []string) *TextInputOverlay {
	overlay := NewTextInputOverlayWithBranchPicker(title, initialValue, profiles)
	if len(submodules) > 0 {
		overlay.submodulePicker = NewSubmodulePicker(submodules)
		overlay.numStops++
	}
	return overlay
}

func newTextarea(initialValue string) textarea.Model {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.CharLimit = 0
	ti.MaxHeight = 0
	return ti
}

func (t *TextInputOverlay) SetSize(width, height int) {
	t.textarea.SetHeight(height)
	t.width = width
	t.height = height
	if t.branchPicker != nil {
		t.branchPicker.SetWidth(width - 6)
	}
	if t.profilePicker != nil {
		t.profilePicker.SetWidth(width - 6)
	}
	if t.submodulePicker != nil {
		t.submodulePicker.SetWidth(width - 6)
	}
}

// Init initializes the text input overlay model
func (t *TextInputOverlay) Init() tea.Cmd {
	return textarea.Blink
}

// View renders the model's view
func (t *TextInputOverlay) View() string {
	return t.Render()
}

// isInPlaceToggle returns true if the current focus is on the in-place toggle.
// The toggle is always at index 0.
func (t *TextInputOverlay) isInPlaceToggle() bool {
	return t.FocusIndex == 0
}

// isProfilePicker returns true if the current focus is on the profile picker.
func (t *TextInputOverlay) isProfilePicker() bool {
	if t.profilePicker == nil || !t.profilePicker.HasMultiple() {
		return false
	}
	return t.FocusIndex == 1 // toggle(0), profile(1)
}

// isTextarea returns true if the current focus is on the textarea.
func (t *TextInputOverlay) isTextarea() bool {
	offset := 1 // toggle is at 0
	if t.profilePicker != nil && t.profilePicker.HasMultiple() {
		offset = 2 // toggle(0), profile(1), textarea(2)
	}
	return t.FocusIndex == offset
}

// isEnterButton returns true if the current focus is on the enter button.
func (t *TextInputOverlay) isEnterButton() bool {
	return t.FocusIndex == t.numStops-1
}

// isBranchPicker returns true if the current focus is on the branch picker.
func (t *TextInputOverlay) isBranchPicker() bool {
	if t.branchPicker == nil || t.inPlace {
		return false
	}
	offset := 2 // toggle(0), textarea(1), branch(2)
	if t.profilePicker != nil && t.profilePicker.HasMultiple() {
		offset = 3 // toggle(0), profile(1), textarea(2), branch(3)
	}
	return t.FocusIndex == offset
}

// isSubmodulePicker returns true if the current focus is on the submodule picker.
func (t *TextInputOverlay) isSubmodulePicker() bool {
	if t.submodulePicker == nil || t.inPlace {
		return false
	}
	offset := 3 // toggle(0), textarea(1), branch(2), submodule(3)
	if t.profilePicker != nil && t.profilePicker.HasMultiple() {
		offset = 4 // toggle(0), profile(1), textarea(2), branch(3), submodule(4)
	}
	return t.FocusIndex == offset
}

// setFocusIndex sets the focus index and syncs focus state.
func (t *TextInputOverlay) setFocusIndex(i int) {
	t.FocusIndex = i
	t.updateFocusState()
}

// updateFocusState syncs the textarea/branchPicker/profilePicker focus/blur state.
func (t *TextInputOverlay) updateFocusState() {
	if t.isTextarea() {
		t.textarea.Focus()
	} else {
		t.textarea.Blur()
	}
	if t.branchPicker != nil {
		if t.isBranchPicker() {
			t.branchPicker.Focus()
		} else {
			t.branchPicker.Blur()
		}
	}
	if t.profilePicker != nil {
		if t.isProfilePicker() {
			t.profilePicker.Focus()
		} else {
			t.profilePicker.Blur()
		}
	}
	if t.submodulePicker != nil {
		if t.isSubmodulePicker() {
			t.submodulePicker.Focus()
		} else {
			t.submodulePicker.Blur()
		}
	}
}

// IsInPlace returns whether in-place mode is active.
func (t *TextInputOverlay) IsInPlace() bool {
	return t.inPlace
}

// SetInPlace sets in-place mode and recalculates focus stops.
func (t *TextInputOverlay) SetInPlace(inPlace bool) {
	t.inPlace = inPlace
	t.recalcNumStops()
	t.FocusIndex = 0
	t.updateFocusState()
}

// recalcNumStops recalculates the total number of focus stops based on current state.
func (t *TextInputOverlay) recalcNumStops() {
	stops := 3 // toggle + textarea + enter button
	if t.profilePicker != nil && t.profilePicker.HasMultiple() {
		stops++
	}
	if !t.inPlace {
		if t.branchPicker != nil {
			stops++
		}
		if t.submodulePicker != nil {
			stops++
		}
	}
	t.numStops = stops
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns (shouldClose, branchFilterChanged).
func (t *TextInputOverlay) HandleKeyPress(msg tea.KeyMsg) (bool, bool) {
	switch msg.Type {
	case tea.KeyTab:
		t.setFocusIndex((t.FocusIndex + 1) % t.numStops)
		return false, false
	case tea.KeyShiftTab:
		t.setFocusIndex((t.FocusIndex - 1 + t.numStops) % t.numStops)
		return false, false
	case tea.KeyEsc:
		t.Canceled = true
		return true, false
	case tea.KeyEnter:
		if t.isEnterButton() {
			t.Submitted = true
			if t.OnSubmit != nil {
				t.OnSubmit()
			}
			return true, false
		}
		if t.isBranchPicker() {
			// Enter on branch picker = advance to next stop
			t.setFocusIndex(t.FocusIndex + 1)
			return false, false
		}
		if t.isSubmodulePicker() {
			// Enter on submodule picker = advance to enter button
			t.setFocusIndex(t.numStops - 1)
			return false, false
		}
		if t.isProfilePicker() {
			// Enter on profile picker = advance to textarea
			t.setFocusIndex(t.FocusIndex + 1)
			return false, false
		}
		// Send enter to textarea
		if t.isTextarea() {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return false, false
	default:
		// Space toggles in-place when toggle is focused
		if t.isInPlaceToggle() && msg.String() == " " {
			t.inPlace = !t.inPlace
			t.recalcNumStops()
			t.updateFocusState()
			return false, false
		}
		if t.isTextarea() {
			t.textarea, _ = t.textarea.Update(msg)
			return false, false
		}
		if t.isProfilePicker() {
			if msg.Type == tea.KeyLeft || msg.Type == tea.KeyRight {
				t.profilePicker.HandleKeyPress(msg)
			}
			return false, false
		}
		if t.isBranchPicker() {
			_, filterChanged := t.branchPicker.HandleKeyPress(msg)
			return false, filterChanged
		}
		if t.isSubmodulePicker() {
			t.submodulePicker.HandleKeyPress(msg)
			return false, false
		}
		return false, false
	}
}

// GetValue returns the current value of the text input.
func (t *TextInputOverlay) GetValue() string {
	return t.textarea.Value()
}

// GetSelectedBranch returns the selected branch name from the branch picker.
// Returns empty string if no branch picker is present or "New branch" is selected.
func (t *TextInputOverlay) GetSelectedBranch() string {
	if t.branchPicker == nil {
		return ""
	}
	return t.branchPicker.GetSelectedBranch()
}

// GetSelectedProgram returns the program string from the selected profile.
// Returns empty string if no profile picker is present.
func (t *TextInputOverlay) GetSelectedProgram() string {
	if t.profilePicker == nil {
		return ""
	}
	return t.profilePicker.GetSelectedProfile().Program
}

// GetSelectedSubmodules returns the selected submodule paths from the submodule picker.
// Returns nil if no submodule picker is present.
func (t *TextInputOverlay) GetSelectedSubmodules() []string {
	if t.submodulePicker == nil {
		return nil
	}
	return t.submodulePicker.GetSelected()
}

// BranchFilterVersion returns the current filter version from the branch picker.
// Returns 0 if no branch picker is present.
func (t *TextInputOverlay) BranchFilterVersion() uint64 {
	if t.branchPicker == nil {
		return 0
	}
	return t.branchPicker.GetFilterVersion()
}

// BranchFilter returns the current filter text from the branch picker.
func (t *TextInputOverlay) BranchFilter() string {
	if t.branchPicker == nil {
		return ""
	}
	return t.branchPicker.GetFilter()
}

// SetBranchResults updates the branch picker with search results.
// version must match the picker's current filterVersion to be accepted.
func (t *TextInputOverlay) SetBranchResults(branches []string, version uint64) {
	if t.branchPicker == nil {
		return
	}
	t.branchPicker.SetResults(branches, version)
}

// IsSubmitted returns whether the form was submitted.
func (t *TextInputOverlay) IsSubmitted() bool {
	return t.Submitted
}

// IsCanceled returns whether the form was canceled.
func (t *TextInputOverlay) IsCanceled() bool {
	return t.Canceled
}

// SetOnSubmit sets a callback function for form submission.
func (t *TextInputOverlay) SetOnSubmit(onSubmit func()) {
	t.OnSubmit = onSubmit
}

// Render renders the text input overlay.
func (t *TextInputOverlay) Render() string {
	// Inner content width (accounting for padding and borders)
	innerWidth := t.width - 6
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Set textarea width to fit within the overlay
	t.textarea.SetWidth(innerWidth)

	// Build a horizontal divider line
	divider := tiDividerStyle.Render(strings.Repeat("─", innerWidth))

	// Build the view
	var content string

	// Render in-place toggle
	toggleLabel := "[ ] In-place (no git isolation)"
	if t.inPlace {
		toggleLabel = "[x] In-place (no git isolation)"
	}
	if t.isInPlaceToggle() {
		content += lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).Render(toggleLabel) + "\n\n"
	} else {
		content += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(toggleLabel) + "\n\n"
	}
	content += divider + "\n\n"

	// Render profile picker if present, above the prompt
	if t.profilePicker != nil {
		content += t.profilePicker.Render() + "\n\n"
		content += divider + "\n\n"
	}

	content += tiTitleStyle.Render(t.Title) + "\n"
	content += t.textarea.View() + "\n\n"

	// Render branch picker if present, with dividers
	if t.branchPicker != nil && !t.inPlace {
		content += divider + "\n\n"
		content += t.branchPicker.Render() + "\n\n"
	}

	// Render submodule picker if present
	if t.submodulePicker != nil && !t.submodulePicker.IsEmpty() && !t.inPlace {
		content += divider + "\n\n"
		content += t.submodulePicker.Render() + "\n\n"
	}

	content += divider + "\n\n"

	// Render enter button with appropriate style
	enterButton := " Enter "
	if t.isEnterButton() {
		enterButton = tiFocusedButtonStyle.Render(enterButton)
	} else {
		enterButton = tiButtonStyle.Render(enterButton)
	}
	content += enterButton

	return tiStyle.Render(content)
}
