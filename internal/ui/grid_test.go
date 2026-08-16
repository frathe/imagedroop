package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/uitest"
)

// The overview's own behaviour - opening, closing, the highlight, key
// handling, and the thumbnail cache - is covered in internal/ui/grid
// against a fake host. What stays here is the wiring: that G reaches it,
// that it takes over the keyboard while it's up, and that it composes
// correctly with the app's other full-window mode and with a fresh drop.
//
// Those last two are the interesting ones: neither package knows the other
// exists, so the guards that keep the grid and the slideshow from
// overlapping live in this package's dispatcher, and these tests are what
// hold them in place.

func TestHandleKeyEvent_GTogglesGrid(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	warmThumbs(t, v)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG})
	if !v.grid.Visible() {
		t.Fatal("G should open the grid")
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG})
	if v.grid.Visible() {
		t.Error("a second G should close the grid")
	}
}

// TestHandleTypedRune_OnlyReachesTheGridWhileItIsUp is the other half of
// the search wiring: typed characters are a grid-only language, so outside
// it they must be dropped rather than quietly accumulating into a query
// that appears the next time the grid opens.
func TestHandleTypedRune_OnlyReachesTheGridWhileItIsUp(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)
	warmThumbs(t, v)

	v.handleTypedRune('/')
	if v.grid.Searching() {
		t.Fatal("a / typed in the normal image view must not open the grid's search")
	}

	v.grid.Toggle()
	v.handleTypedRune('/')
	v.handleTypedRune('a')

	if !v.grid.Searching() {
		t.Error("a / typed while the grid is up should open its search")
	}
	if v.grid.Query() != "a" {
		t.Errorf("Query() = %q, want %q", v.grid.Query(), "a")
	}
}

// TestHandleTypedRune_GridVisible_SwallowedByDeleteConfirmation: the delete
// card owns the keyboard whole while it is up, the same as in
// handleKeyEvent - a typed character must not edit a search behind it.
func TestHandleTypedRune_GridVisible_SwallowedByDeleteConfirmation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	warmThumbs(t, v)

	v.grid.Toggle()
	v.handleTypedRune('/')
	v.deletion.Request()

	v.handleTypedRune('a')

	if v.grid.Query() != "" {
		t.Errorf("Query() = %q, want it untouched while the delete confirmation is up", v.grid.Query())
	}
}

// TestHandleKeyEvent_GridVisible_SwallowsNavigation is the dispatcher's
// half of the contract: while the grid is up, ordinary navigation must not
// slip through and change what's on screen behind it.
func TestHandleKeyEvent_GridVisible_SwallowsNavigation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	warmThumbs(t, v)
	v.grid.Toggle()
	before := v.index

	// Right is intercepted by the grid (it moves the highlight) rather
	// than falling through to normal next-image navigation.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})

	if v.index != before {
		t.Errorf("index changed to %d while the grid was up, want unchanged from %d", v.index, before)
	}
	if !v.grid.Visible() {
		t.Error("Right should not close the grid")
	}
}

func TestHandleKeyEvent_GridVisible_ReturnNavigatesAndCloses(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	dropAndWait(t, v, a, b)

	warmThumbs(t, v)
	v.grid.Toggle()

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyReturn})
	waitUntilLoaded(t, v)

	if v.grid.Visible() {
		t.Error("committing a cell should close the grid")
	}
	if v.index != 1 {
		t.Errorf("index = %d, want 1 - the highlighted image should now be on screen", v.index)
	}
}

// --- composition with the app's other full-window mode --------------------

func TestHandleKeyEvent_GIsIgnoredDuringPictureFrameMode(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	warmThumbs(t, v)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG})

	if v.grid.Visible() {
		t.Error("the grid should not open while the slideshow owns the screen")
	}
}

func TestEnterPictureFrameMode_ClosesOpenGrid(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	warmThumbs(t, v)
	v.grid.Toggle()
	if !v.grid.Visible() {
		t.Fatal("setup: the grid should be open before entering picture-frame mode")
	}

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	if v.grid.Visible() {
		t.Error("entering picture-frame mode should close the grid")
	}
	if !v.slides.Active() {
		t.Error("picture-frame mode should still turn on")
	}
}

// --- composition with a fresh drop ----------------------------------------

func TestHandleDrop_ClosesOpenGrid(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	warmThumbs(t, v)
	v.grid.Toggle()
	if !v.grid.Visible() {
		t.Fatal("setup: the grid should be open before the second drop")
	}

	b := uitest.TempJPEGURI(t, "b.txt", 4, 4, color.White)
	dropAndWaitScan(t, v, b)

	if v.grid.Visible() {
		t.Error("a new drop should close the grid")
	}
}

// TestShiftDelete_IgnoredWhileGridVisible drives the real shortcut rather
// than the confirmation directly: Shift+Delete is a global shortcut, not
// gated by handleKeyEvent's own grid guard, so without the check
// wireDeleteShortcut passes to deletion.ShortcutHandler it would open a
// confirmation card hidden behind the grid and capture the keyboard out
// from under it. (The guard lives in the wiring, so neither feature
// package needs to know about the other.)
func TestShiftDelete_IgnoredWhileGridVisible(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	handler := &fyne.ShortcutHandler{}
	wireDeleteShortcut(handler, v)

	warmThumbs(t, v)
	v.grid.Toggle()

	handler.TypedShortcut(&fyne.ShortcutCut{Secondary: true})

	if v.deletion.Visible() {
		t.Error("Shift+Delete should be ignored while the grid is showing")
	}
}
