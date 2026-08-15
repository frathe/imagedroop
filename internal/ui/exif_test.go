package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/uitest"
)

// --- EXIF panel wiring ------------------------------------------------
//
// The panel itself lives in internal/ui/exifwin; these cover the viewer's
// side of it - the E key, the info-overlay link, and staying in sync as
// navigation changes which file is current.

// TestShowExifWindow_NoopWithNothingLoaded guards against E (or the info
// overlay link, before it's even reachable - see
// TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded for the same idea applied
// to the info card itself) trying to open a window with no file to read
// metadata from.
func TestShowExifWindow_NoopWithNothingLoaded(t *testing.T) {
	v, _, _ := newTestUI(t)

	v.exif.Show()

	if v.exif.Open() {
		t.Error("showExifWindow should no-op with nothing loaded")
	}
}

// TestShowExifWindow_OpensAndRaisesSameWindow mirrors
// TestShowAbout_OpensAndRaisesSameWindow (about_test.go): a plain
// widget.Label content, like About's single heading, so it doesn't hit the
// test theme's font-combination limits the way manual.md's RichText does
// (see the comment above TestE2E_EscapeQuitsWhenNothingLoaded in
// e2e_test.go) and can be exercised directly.
func TestShowExifWindow_OpensAndRaisesSameWindow(t *testing.T) {
	v, _, _ := newTestUI(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	v.exif.Show()

	win := v.exif.Window()
	if win == nil {
		t.Fatal("showExifWindow did not open a window")
	}

	v.exif.Show()

	if v.exif.Window() != win {
		t.Error("a second showExifWindow call should raise the existing window, not open a new one")
	}

	win.Close()

	if v.exif.Open() {
		t.Error("closing the EXIF window should leave the singleton closed")
	}
}

// TestShowExifWindow_ContentAndRefreshOnNavigation checks the window shows
// the current file's metadata (encodeJPEG embeds none, so the no-metadata
// message - imaging.ReadMetadata's own behavior is covered directly in
// internal/imaging) and, per refreshExifWindow's comment, keeps itself
// current across navigation while still open, mirroring how the info
// overlay behaves (TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation).
func TestShowExifWindow_ContentAndRefreshOnNavigation(t *testing.T) {
	v, _, _ := newTestUI(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 40, 20, color.White)
	dropAndWait(t, v, a, b)

	v.exif.Show()

	want := "No EXIF metadata found in this file."
	if got := v.exif.Text().Text; got != want {
		t.Errorf("exifText = %q, want %q", got, want)
	}

	v.ShowImage(v.index + 1)
	waitUntilLoaded(t, v)

	if got := v.exif.Text().Text; got != want {
		t.Errorf("exifText after navigating should stay in sync, got %q, want %q", got, want)
	}
}

// TestHandleKeyEvent_EOpensExifWindow checks the E keybinding reaches
// showExifWindow, mirroring how the I/M/P keys are each tested against
// their own handler.
func TestHandleKeyEvent_EOpensExifWindow(t *testing.T) {
	v, _, _ := newTestUI(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyE})

	if !v.exif.Open() {
		t.Error("E should open the EXIF window")
	}
}

// TestExifLink_OpensExifWindow checks the info overlay's "Show EXIF data"
// link (build.go's infoCard wiring) reaches showExifWindow, mirroring how
// e2e_test.go drives restoreLink's own OnTapped directly rather than a real
// simulated click.
func TestExifLink_OpensExifWindow(t *testing.T) {
	v, _, _ := newTestUI(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	v.exifLink.OnTapped()

	if !v.exif.Open() {
		t.Error("the info overlay's EXIF link should open the EXIF window")
	}
}
