package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
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
// TestShowAbout_OpensAndRaisesSameWindow (help/about_test.go): a plain
// widget.Label content, like About's single heading, so it doesn't hit the
// test theme's font-combination limits the way manual.md's RichText does
// (see the F1/showManual note at the end of e2e_test.go) and can be
// exercised directly.
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
// the current file's metadata (uitest.EncodeJPEG embeds none, so the
// no-metadata message - imaging.ReadMetadata's own behavior is covered
// directly in internal/imaging) and, per exifwin.Window.Refresh's comment,
// keeps itself current across navigation while still open, mirroring how the
// info overlay behaves (TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation).
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

	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if got := v.exif.Text().Text; got != want {
		t.Errorf("exifText after navigating should stay in sync, got %q, want %q", got, want)
	}
}

// TestHandleKeyEvent_EOpensExifWindow checks the E keybinding reaches
// v.exif.Show, mirroring how the I/M/P keys are each tested against
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
// link (build.go's infoCard wiring) reaches v.exif.Show, mirroring how
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

// TestExifLink_HiddenWhenTheImageHasNoExifData covers the other half of the
// link's job: offering "Show EXIF data" for a file that has none is a
// promise the panel can only answer with "No EXIF metadata found in this
// file." uitest.EncodeJPEG embeds no metadata at all, so a plain test JPEG
// is exactly that case.
func TestExifLink_HiddenWhenTheImageHasNoExifData(t *testing.T) {
	v, _, _ := newTestUI(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	v.toggleInfoOverlay()

	if !v.infoCard.Visible() {
		t.Fatal("the info card itself should be showing")
	}
	if v.exifLink.Visible() {
		t.Error("the EXIF link should be hidden for a file with no EXIF metadata")
	}
}

func TestExifLink_ShownWhenTheImageHasExifData(t *testing.T) {
	v, _, _ := newTestUI(t)

	withExif := uitest.WriteTempFile(t, "dated.jpg", uitest.CaptureDateJPEG(t, 40, 20, "2024:08:12 14:33:02"))
	dropAndWait(t, v, storage.NewFileURI(withExif))

	v.toggleInfoOverlay()

	if !v.exifLink.Visible() {
		t.Error("the EXIF link should be shown for a file that has EXIF metadata")
	}
}

// The link has to follow the file on screen, not just whatever was loaded
// when the overlay was switched on.
func TestExifLink_VisibilityFollowsNavigation(t *testing.T) {
	v, _, _ := newTestUI(t)

	withExif := uitest.WriteTempFile(t, "dated.jpg", uitest.CaptureDateJPEG(t, 40, 20, "2024:08:12 14:33:02"))
	plain := uitest.TempJPEGURI(t, "plain.jpg", 40, 20, color.White)
	dropAndWait(t, v, storage.NewFileURI(withExif), plain)

	v.toggleInfoOverlay()
	if !v.exifLink.Visible() {
		t.Fatal("the EXIF link should be shown for the first file, which has EXIF metadata")
	}

	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.exifLink.Visible() {
		t.Error("the EXIF link should hide again after navigating to a file with no EXIF metadata")
	}

	v.ShowImage(v.state.index - 1)
	waitUntilLoaded(t, v)

	if !v.exifLink.Visible() {
		t.Error("the EXIF link should come back when navigating to the file that does have EXIF metadata")
	}
}
