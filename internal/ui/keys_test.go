package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/uitest"
)

// The dispatcher's per-feature handovers are tested beside the features they
// hand over to (delete_test.go, export_test.go, grid_test.go). What stays
// here is the one guard that belongs to no feature at all: a Fyne dialog is
// a canvas overlay, and while one is up it owns the keyboard whole.
//
// Both halves matter and both are tested below. A dialog that Fyne draws but
// this app never asked about would otherwise let Escape reset the session
// behind it; and this app's own modal surfaces - the delete card, the export
// prompt, the grid, the info card, the toast - are layers of the window
// content stack built in build.go rather than canvas overlays, so the guard
// must not touch them.

// TestHandleKeyEvent_IgnoredWhileAFyneDialogIsUp is the regression this guard
// exists for: with the Manage Favorites dialog up, Escape used to fall
// through to the switch below and reset the whole session behind it, because
// Fyne only consults the *top* overlay's focus manager and a dialog that
// focuses nothing leaves Canvas().Focused() nil - which is exactly when the
// glfw driver routes a key to the canvas's unfocused handler, this
// dispatcher.
func TestHandleKeyEvent_IgnoredWhileAFyneDialogIsUp(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.showManageFavorites()
	if n := len(v.win.Canvas().Overlays().List()); n != 1 {
		t.Fatalf("setup: overlay count = %d, want the Manage Favorites dialog", n)
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if len(v.state.files) != 2 {
		t.Errorf("files = %d, want the session left alone behind the dialog", len(v.state.files))
	}
	if n := len(v.win.Canvas().Overlays().List()); n != 1 {
		t.Errorf("overlay count = %d, want the dialog still up", n)
	}
}

// TestHandleKeyEvent_DialogSwallowsTheAppsOtherKeysToo: Escape is the
// dangerous one, but every other binding is just as wrong behind a modal -
// G would open the grid underneath it, the arrows would navigate.
func TestHandleKeyEvent_DialogSwallowsTheAppsOtherKeysToo(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)
	warmThumbs(t, v)

	v.showManageFavorites()
	startIndex := v.state.index

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG})
	if v.grid.Visible() {
		t.Error("G opened the grid behind the dialog")
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})
	if v.state.index != startIndex {
		t.Errorf("index = %d, want %d: the arrows navigated behind the dialog", v.state.index, startIndex)
	}
}

// TestHandleTypedRune_IgnoredWhileAFyneDialogIsUp is the typed-character
// twin: the grid's search must not keep collecting characters aimed at a
// dialog on top of it.
func TestHandleTypedRune_IgnoredWhileAFyneDialogIsUp(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	warmThumbs(t, v)

	v.grid.Toggle()
	v.handleTypedRune('/')
	if !v.grid.Searching() {
		t.Fatal("setup: / should have opened the grid's search")
	}

	v.showManageFavorites()
	if n := len(v.win.Canvas().Overlays().List()); n != 1 {
		t.Fatalf("setup: overlay count = %d, want the Manage Favorites dialog", n)
	}

	v.handleTypedRune('a')

	if v.grid.Query() != "" {
		t.Errorf("Query() = %q, want it untouched while a dialog is up", v.grid.Query())
	}
}

// TestHandleKeyEvent_DeleteCardIsNotACanvasOverlay guards the other side of
// the guard: this app's own cards live in the window content stack, so the
// overlay check must leave them dispatching exactly as before. The two
// assertions are deliberately paired - the premise (no overlay) is what makes
// the behaviour (keys still arrive) proof of anything.
func TestHandleKeyEvent_DeleteCardIsNotACanvasOverlay(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.deletion.Request()
	if n := len(v.win.Canvas().Overlays().List()); n != 0 {
		t.Fatalf("overlay count = %d, want the delete card to stay out of the canvas overlays", n)
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if v.deletion.Visible() {
		t.Error("Escape no longer reaches the delete card")
	}
	if len(v.state.files) != 2 {
		t.Error("Escape on the card reset the loaded file set")
	}
}

// TestHandleTypedRune_GridIsNotACanvasOverlay is the same premise for the
// grid and for runes: it is content, not an overlay, so its search still
// receives what is typed.
func TestHandleTypedRune_GridIsNotACanvasOverlay(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	warmThumbs(t, v)

	v.grid.Toggle()
	if n := len(v.win.Canvas().Overlays().List()); n != 0 {
		t.Fatalf("overlay count = %d, want the grid to stay out of the canvas overlays", n)
	}

	v.handleTypedRune('/')
	v.handleTypedRune('a')

	if v.grid.Query() != "a" {
		t.Errorf("Query() = %q, want %q: the grid stopped receiving runes", v.grid.Query(), "a")
	}
}
