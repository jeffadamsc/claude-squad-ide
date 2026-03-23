package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
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

func TestBranchPicker_IsNewBranch(t *testing.T) {
	bp := NewBranchPicker()
	if !bp.IsNewBranch() {
		t.Error("expected IsNewBranch() true when on HEAD option")
	}
	if bp.BaseBranch() != "HEAD" {
		t.Errorf("expected BaseBranch() = HEAD, got %s", bp.BaseBranch())
	}
}

func TestBranchPicker_NewBranchOptionsWithOrigin(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main"})
	items := bp.visibleItems()
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	if items[0] != "New branch (from origin/main)" {
		t.Errorf("expected first item to be origin/main option, got %s", items[0])
	}
	if items[1] != "New branch (from HEAD)" {
		t.Errorf("expected second item to be HEAD option, got %s", items[1])
	}
	if bp.BaseBranch() != "origin/main" {
		t.Errorf("expected BaseBranch() = origin/main, got %s", bp.BaseBranch())
	}
}

func TestBranchPicker_NewBranchOptionsWithBoth(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main", "origin/master"})
	items := bp.visibleItems()
	if len(items) < 3 {
		t.Fatalf("expected at least 3 items, got %d", len(items))
	}
	if items[0] != "New branch (from origin/main)" {
		t.Errorf("expected first = origin/main option, got %s", items[0])
	}
	if items[1] != "New branch (from origin/master)" {
		t.Errorf("expected second = origin/master option, got %s", items[1])
	}
	if items[2] != "New branch (from HEAD)" {
		t.Errorf("expected third = HEAD option, got %s", items[2])
	}
}

func TestBranchPicker_ExistingBranchNotNewBranch(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetResults([]string{"feature/foo"}, 0)
	bp.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if bp.IsNewBranch() {
		t.Error("expected IsNewBranch() false when on existing branch")
	}
	if bp.GetSelectedBranch() != "feature/foo" {
		t.Errorf("expected GetSelectedBranch() = feature/foo, got %s", bp.GetSelectedBranch())
	}
}

func TestBranchPicker_FilterHidesNewBranchOnExactMatch(t *testing.T) {
	bp := NewBranchPicker()
	bp.SetNewBranchOptions([]string{"origin/main"})
	bp.filter = "feature/foo"
	bp.SetResults([]string{"feature/foo"}, bp.filterVersion)
	items := bp.visibleItems()
	for _, item := range items {
		if strings.HasPrefix(item, "New branch") {
			t.Errorf("expected no New branch options when filter matches exactly, got %s", item)
		}
	}
}

func TestTextInputOverlay_GetBaseBranch_NoBranchPicker(t *testing.T) {
	o := NewTextInputOverlay("Prompt", "")
	if got := o.GetBaseBranch(); got != "HEAD" {
		t.Errorf("expected GetBaseBranch() = HEAD for plain overlay, got %s", got)
	}
}

func TestTextInputOverlay_GetBaseBranch_NewBranchDefault(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	if got := o.GetBaseBranch(); got != "HEAD" {
		t.Errorf("expected GetBaseBranch() = HEAD by default, got %s", got)
	}
}

func TestTextInputOverlay_GetBaseBranch_WithOriginMain(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	o.SetNewBranchOptions([]string{"origin/main"})
	if got := o.GetBaseBranch(); got != "origin/main" {
		t.Errorf("expected GetBaseBranch() = origin/main after SetNewBranchOptions, got %s", got)
	}
}

func TestTextInputOverlay_GetBaseBranch_ExistingBranch(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Prompt", "", nil)
	o.SetBranchResults([]string{"feature/foo"}, 0)
	// Tab twice to reach the branch picker: toggle(0) → textarea(1) → branchPicker(2)
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	// Press Down to move from the HEAD new-branch option to the existing branch
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if got := o.GetBaseBranch(); got != "HEAD" {
		t.Errorf("expected GetBaseBranch() = HEAD when existing branch selected, got %s", got)
	}
}

func TestTextInputOverlay_SetNewBranchOptions_NoBranchPicker(t *testing.T) {
	o := NewTextInputOverlay("Prompt", "")
	// Should be a no-op and not panic
	o.SetNewBranchOptions([]string{"origin/main"})
}
