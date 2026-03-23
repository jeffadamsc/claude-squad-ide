package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestInPlaceToggle_DefaultOff(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	if o.IsInPlace() {
		t.Error("expected in-place toggle to be off by default")
	}
}

func TestInPlaceToggle_SetInPlace(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	o.SetInPlace(true)
	if !o.IsInPlace() {
		t.Error("expected in-place after SetInPlace(true)")
	}
	o.SetInPlace(false)
	if o.IsInPlace() {
		t.Error("expected not in-place after SetInPlace(false)")
	}
}

func TestInPlaceToggle_FocusOrderWithToggleOn(t *testing.T) {
	// No profile picker: inPlaceToggle(0) → textarea(1) → enterButton(2)
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	o.SetInPlace(true)
	if o.numStops != 3 {
		t.Errorf("expected 3 focus stops with toggle on (no profiles), got %d", o.numStops)
	}
	if !o.isInPlaceToggle() {
		t.Error("expected first focus stop to be in-place toggle")
	}
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	if !o.isTextarea() {
		t.Error("expected textarea after tab from toggle")
	}
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	if !o.isEnterButton() {
		t.Error("expected enter button after tab from textarea")
	}
}

func TestInPlaceToggle_FocusOrderWithToggleOff(t *testing.T) {
	// No profile picker: inPlaceToggle(0) → textarea(1) → branchPicker(2) → enterButton(3)
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	if o.numStops != 4 {
		t.Errorf("expected 4 focus stops with toggle off (no profiles), got %d", o.numStops)
	}
	if !o.isInPlaceToggle() {
		t.Error("expected first focus stop to be in-place toggle")
	}
}

func TestInPlaceToggle_SpaceToggles(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	if o.IsInPlace() {
		t.Error("expected toggle off initially")
	}
	// Focus is on toggle (index 0), press space
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeySpace})
	if !o.IsInPlace() {
		t.Error("expected space to toggle in-place on")
	}
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeySpace})
	if o.IsInPlace() {
		t.Error("expected space to toggle in-place off")
	}
}

func TestInPlaceToggle_SpaceOnlyWorksWhenFocused(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab}) // move to textarea
	if !o.isTextarea() {
		t.Error("expected textarea focus")
	}
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeySpace})
	if o.IsInPlace() {
		t.Error("space on textarea should not toggle in-place")
	}
}

func TestInPlaceToggle_HidesBranchAndSubmodule(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	if o.branchPicker == nil {
		t.Error("expected branch picker when toggle is off")
	}
	o.SetInPlace(true)
	if o.isBranchPicker() {
		t.Error("branch picker should not be focusable when in-place is on")
	}
}
