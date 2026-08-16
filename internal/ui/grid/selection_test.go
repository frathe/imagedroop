package grid

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// click drives OnSelected the way a real tap on a cell does, with mods held.
// The grid reads the modifiers through Host rather than off the event,
// because a Fyne tap carries none - see Host.Modifiers.
func click(g *Overview, host *fakeHost, id int, mods fyne.KeyModifier) {
	host.mods = mods
	g.wrap.Select(id)
	host.mods = 0
}

// --- gestures --------------------------------------------------------------

// TestOnSelected_PlainClickStillOpensTheImage is the regression guard on the
// gesture that existed before multi-select: an unmodified click means "show
// me this one", exactly as it always has.
func TestOnSelected_PlainClickStillOpensTheImage(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	click(g, host, 1, 0)

	if want := []int{1}; !slices.Equal(host.shown, want) {
		t.Errorf("ShowImage calls = %v, want %v", host.shown, want)
	}
	if g.Visible() {
		t.Error("a plain click should close the grid")
	}
	if g.SelectionCount() != 0 {
		t.Errorf("SelectionCount() = %d after a plain click, want 0", g.SelectionCount())
	}
}

func TestOnSelected_ModifierClickSelectsWithoutOpening(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	click(g, host, 1, fyne.KeyModifierShortcutDefault)

	if want := []int{1}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
	if len(host.shown) != 0 {
		t.Errorf("ShowImage calls = %v, want none - a modifier click picks, it does not open", host.shown)
	}
	if !g.Visible() {
		t.Error("a modifier click should leave the grid open")
	}
}

// TestOnSelected_ModifierClickHandsTheKeyboardBack: Fyne's GridWrap grabs
// canvas focus on every tap, and this app dispatches every key from the
// unfocused canvas handler. Close is what normally undoes that; a click that
// deliberately keeps the grid open has to undo it itself, or the arrow keys
// and '/' stop arriving.
func TestOnSelected_ModifierClickHandsTheKeyboardBack(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")

	click(g, host, 0, fyne.KeyModifierShortcutDefault)

	if host.unfocused == 0 {
		t.Error("a modifier click should release canvas focus, or every later key press is swallowed")
	}
}

// TestOnSelected_ShiftClickHandsTheKeyboardBack: a range extension keeps the
// grid open just as a toggle does, so it owes the same debt to canvas focus.
func TestOnSelected_ShiftClickHandsTheKeyboardBack(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	click(g, host, 0, fyne.KeyModifierShortcutDefault)
	host.unfocused = 0 // ignore what the anchoring click already did

	click(g, host, 2, fyne.KeyModifierShift)

	if host.unfocused == 0 {
		t.Error("a Shift+click should release canvas focus, or every later key press is swallowed")
	}
}

func TestOnSelected_ModifierClickTwiceDeselects(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")

	click(g, host, 1, fyne.KeyModifierShortcutDefault)
	click(g, host, 1, fyne.KeyModifierShortcutDefault)

	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v after clicking the same cell twice, want empty", g.Selection())
	}
}

func TestOnSelected_ShiftClickExtendsFromTheAnchor(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg")

	click(g, host, 1, fyne.KeyModifierShortcutDefault)
	click(g, host, 3, fyne.KeyModifierShift)

	if want := []int{1, 2, 3}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
	if len(host.shown) != 0 {
		t.Errorf("ShowImage calls = %v, want none", host.shown)
	}
}

// TestOnSelected_ShiftClickExtendsBackwardsToo: the anchor can sit either
// side of the cell that ends the range.
func TestOnSelected_ShiftClickExtendsBackwardsToo(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")

	click(g, host, 3, fyne.KeyModifierShortcutDefault)
	click(g, host, 1, fyne.KeyModifierShift)

	if want := []int{1, 2, 3}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
}

// TestOnSelected_ShiftClickWithNoAnchorPicksJustThatCell: shift-clicking
// first thing has no range to extend, so it behaves like a modifier click
// rather than doing nothing.
func TestOnSelected_ShiftClickWithNoAnchorPicksJustThatCell(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	click(g, host, 2, fyne.KeyModifierShift)

	if want := []int{2}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
}

// TestOnSelected_ShiftClickSpansTheFilteredSubsetOnly is the display-index
// mapping the filter forces: the range covers the cells actually drawn
// between the two clicks, not the host indices between them.
func TestOnSelected_ShiftClickSpansTheFilteredSubsetOnly(t *testing.T) {
	g, host := openGrid(t, "sun1.jpg", "moon.jpg", "sun2.jpg", "star.jpg", "sun3.jpg")
	typeQuery(g, "sun") // display 0,1,2 -> host 0,2,4

	click(g, host, 0, fyne.KeyModifierShortcutDefault)
	click(g, host, 2, fyne.KeyModifierShift)

	if want := []int{0, 2, 4}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v - the range must skip the filtered-out files", g.Selection(), want)
	}
}

// --- keys ------------------------------------------------------------------

func TestHandleKey_SpaceTogglesTheHighlightedCell(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace})

	if want := []int{1}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v after a second Space, want empty", g.Selection())
	}
}

// TestHandleKey_SpaceIsAQueryCharacterWhileSearching: a space typed into the
// search bar arrives as both a rune and a key event, and it must not also
// toggle whatever cell happens to be highlighted.
func TestHandleKey_SpaceIsAQueryCharacterWhileSearching(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg")
	typeQuery(g, "a")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace})

	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v, want empty while typing a query", g.Selection())
	}
}

func TestSelectAll_PicksEveryCell(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	g.SelectAll()

	if want := []int{0, 1, 2}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
}

// TestSelectAll_PicksOnlyTheFilteredSubset is the combination the whole
// feature is for: narrow with '/', take the lot, act on it.
func TestSelectAll_PicksOnlyTheFilteredSubset(t *testing.T) {
	g, _ := openGrid(t, "sun1.jpg", "moon.jpg", "sun2.jpg")
	typeQuery(g, "sun")

	g.SelectAll()

	if want := []int{0, 2}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", g.Selection(), want)
	}
}

// TestHandleKey_EscapeClearsSelectionBeforeSearch pins the staging: each
// Escape undoes one layer, so nothing that took effort to build is thrown
// away by the keystroke that was meant to undo something smaller.
func TestHandleKey_EscapeClearsSelectionBeforeSearch(t *testing.T) {
	g, host := openGrid(t, "sun1.jpg", "moon.jpg", "sun2.jpg")
	typeQuery(g, "sun")
	click(g, host, 0, fyne.KeyModifierShortcutDefault)

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if g.SelectionCount() != 0 {
		t.Fatalf("Selection() = %v after the first Escape, want empty", g.Selection())
	}
	if !g.Searching() {
		t.Fatal("the first Escape should clear the selection, not the search")
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if g.Searching() {
		t.Fatal("the second Escape should clear the search")
	}
	if !g.Visible() {
		t.Fatal("the second Escape should not also close the grid")
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if g.Visible() {
		t.Error("the third Escape should close the grid")
	}
}

// TestHandleKey_GIsInertWhileASelectionIsPending: closing the grid out from
// under a selection the user is still assembling would silently discard it,
// the same reason G already stops closing while a search is open.
func TestHandleKey_GIsInertWhileASelectionIsPending(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")
	click(g, host, 0, fyne.KeyModifierShortcutDefault)

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyG})

	if !g.Visible() {
		t.Error("G should not close the grid while a selection is pending")
	}
}

// --- selection lifetime ----------------------------------------------------

// TestSelection_SurvivesAFilterChange is why the set holds host indices
// rather than the display indices the user actually clicked: narrowing the
// grid and widening it again must not silently drop what was picked.
func TestSelection_SurvivesAFilterChange(t *testing.T) {
	g, host := openGrid(t, "sun.jpg", "moon.jpg")

	click(g, host, 1, fyne.KeyModifierShortcutDefault) // moon.jpg, host index 1
	typeQuery(g, "sun")                                // moon.jpg is no longer drawn
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})  // clears the selection first
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})  // ... then the search

	if g.SelectionCount() != 0 {
		t.Fatalf("Selection() = %v, want the Escape staging to have cleared it", g.Selection())
	}

	click(g, host, 1, fyne.KeyModifierShortcutDefault)
	typeQuery(g, "sun")

	if want := []int{1}; !slices.Equal(g.Selection(), want) {
		t.Errorf("Selection() = %v while filtered, want %v kept", g.Selection(), want)
	}
}

// TestClose_ClearsTheSelection: like the filter, a selection is a way of
// working with the grid rather than a standing setting - each open starts
// clean, and the app's defensive Close on every drop can't carry a selection
// from the previous file set into the new one.
func TestClose_ClearsTheSelection(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")
	click(g, host, 0, fyne.KeyModifierShortcutDefault)

	g.Close()

	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v after Close, want empty", g.Selection())
	}
}

func TestClearSelection_Empties(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")
	click(g, host, 0, fyne.KeyModifierShortcutDefault)

	g.ClearSelection()

	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v after ClearSelection, want empty", g.Selection())
	}
}

// TestFilesChanged_ClearsTheSelectionAndClampsTheHighlight is what the app
// calls once a batch delete has actually shrunk the file set: every index the
// grid was holding now means a different file, or none at all.
func TestFilesChanged_ClearsTheSelectionAndClampsTheHighlight(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")
	click(g, host, 2, fyne.KeyModifierShortcutDefault)
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	host.files = host.files[:1] // two of the three were just trashed
	g.FilesChanged()

	if g.SelectionCount() != 0 {
		t.Errorf("Selection() = %v after FilesChanged, want empty", g.Selection())
	}
	if g.Highlight() >= len(host.files) {
		t.Errorf("Highlight() = %d with %d files left, want it clamped into range", g.Highlight(), len(host.files))
	}
}

// --- targets ---------------------------------------------------------------

func TestTargets_IsTheSelectionWhenThereIsOne(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	click(g, host, 0, fyne.KeyModifierShortcutDefault)
	click(g, host, 2, fyne.KeyModifierShortcutDefault)

	if want := []int{0, 2}; !slices.Equal(g.Targets(), want) {
		t.Errorf("Targets() = %v, want %v", g.Targets(), want)
	}
}

// TestTargets_FallsBackToTheHighlightedCell keeps the batch shortcuts
// meaningful with nothing explicitly picked: the keyboard cursor is a
// selection of one.
func TestTargets_FallsBackToTheHighlightedCell(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg", "c.jpg")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	if want := []int{1}; !slices.Equal(g.Targets(), want) {
		t.Errorf("Targets() = %v, want %v (the highlighted cell)", g.Targets(), want)
	}
}

// TestTargets_MapsTheHighlightThroughTheFilter: the fallback is a display
// index, and everything leaving this package speaks the host's numbering.
func TestTargets_MapsTheHighlightThroughTheFilter(t *testing.T) {
	g, _ := openGrid(t, "moon.jpg", "sun.jpg")
	typeQuery(g, "sun") // one cell drawn, display 0 -> host 1

	if want := []int{1}; !slices.Equal(g.Targets(), want) {
		t.Errorf("Targets() = %v, want %v", g.Targets(), want)
	}
}

func TestTargets_IsEmptyWithNothingLoaded(t *testing.T) {
	g := newOverview(t, &fakeHost{})

	if got := g.Targets(); len(got) != 0 {
		t.Errorf("Targets() = %v, want empty with no files", got)
	}
}

// --- cell rendering --------------------------------------------------------

func TestSetCellSelected_TogglesTheTint(t *testing.T) {
	tint := canvas.NewRectangle(color.Transparent)

	setCellSelected(tint, true)
	if !tint.Visible() {
		t.Error("selecting should show the tint")
	}

	setCellSelected(tint, false)
	if tint.Visible() {
		t.Error("deselecting should hide the tint")
	}
}
