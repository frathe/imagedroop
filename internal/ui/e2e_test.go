// e2e_test.go drives the real app the way a user would: these tests use the
// same startup load, buildViewer assembly, and geometry restoration as Run,
// so the production wiring cannot drift into a hand-copied test replica.
//
// Each scenario checks both state (files/visibility/index - fast, exact,
// portable) and a full-window screenshot compared against a golden master
// under testdata/ via Fyne's own test.AssertRendersToImage. The state
// checks are the real regression guard; the image compare catches z-order
// and appearance bugs state alone can't see (e.g. the bug this suite adds
// a case for: a stale image left visible behind an error toast).
//
// Golden masters are machine/Fyne-version specific (font rendering varies
// across OS and font-hinting differences aren't fully covered by Fyne's
// built-in pixel-tolerance). If a legitimate visual change makes a master
// stale, inspect the new render written to testdata/failed/<name>.png and,
// if it looks right, copy it over testdata/<name>.png to accept it as the
// new baseline.
//
// Run just this suite with: go test -run TestE2E ./...
package ui

import (
	"errors"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/filepicker"
	"github.com/frathe/picfetch/internal/session"
	"github.com/frathe/picfetch/internal/uitest"
)

func TestE2E_InitialLaunchShowsWelcome(t *testing.T) {
	v, win, _ := newTestUI(t)

	if v.img.Visible() {
		t.Error("no image should be visible on first launch")
	}
	if !v.dropzone.Visible() {
		t.Error("drop zone should be visible on first launch")
	}
	if !v.welcomeArt.Visible() {
		t.Error("welcome art should be visible on first launch")
	}
	if v.emptyStateArt.Visible() {
		t.Error("placeholder art should not be visible on first launch")
	}
	if v.toast.card.Visible() {
		t.Error("no toast should be visible on first launch")
	}

	test.AssertRendersToImage(t, "initial_launch.png", win.Canvas())
}

// TestE2E_HoveringDropzoneHighlightsBorderThenReverts exercises
// dropzoneArt's onHover wiring (components.go) - the border around the drop
// zone should brighten while the pointer is over it and return to exactly
// the initial-launch look once it leaves, confirmed against the same golden
// master rather than a second one.
func TestE2E_HoveringDropzoneHighlightsBorderThenReverts(t *testing.T) {
	v, win, _ := newTestUI(t)

	v.dropzoneArt.MouseIn(&desktop.MouseEvent{})
	test.AssertRendersToImage(t, "dropzone_hover.png", win.Canvas())

	v.dropzoneArt.MouseOut()
	test.AssertRendersToImage(t, "initial_launch.png", win.Canvas())
}

func TestE2E_SuccessfulDropShowsImage(t *testing.T) {
	v, win, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 40, 30, color.RGBA{G: 200, A: 255})
	dropAndWait(t, v, jpegURI)

	if !v.img.Visible() || v.img.Image == nil {
		t.Fatal("expected an image to be loaded and visible")
	}
	if v.dropzone.Visible() {
		t.Error("drop zone should be hidden once an image is showing")
	}

	test.AssertRendersToImage(t, "successful_drop.png", win.Canvas())
}

func TestE2E_BadDropWithNothingLoadedShowsPlaceholder(t *testing.T) {
	v, win, _ := newTestUI(t)

	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if !v.toast.card.Visible() {
		t.Error("expected a toast for the unsupported drop")
	}
	if !v.emptyStateArt.Visible() {
		t.Error("expected the error placeholder art to be showing")
	}
	if v.welcomeArt.Visible() {
		t.Error("welcome art should be hidden once a drop has been attempted")
	}

	test.AssertRendersToImage(t, "bad_drop_fresh.png", win.Canvas())
	settleToast(t, v)
}

// TestE2E_BadDropAfterImagesClearsDisplay is a regression test: dropping
// something unsupported after images were already loaded used to leave the
// last image visible behind the error toast and placeholder art, because
// the empty-image branches showed the placeholder without ever clearing
// the previous v.img/v.state.files. ShowEmptyStateError (viewer.go) fixes this
// by fully resetting the display before showing the error.
func TestE2E_BadDropAfterImagesClearsDisplay(t *testing.T) {
	v, win, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 40, 30, color.RGBA{B: 200, A: 255})
	dropAndWait(t, v, jpegURI)

	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if v.img.Visible() || v.img.Image != nil {
		t.Error("the previous image must not linger behind the error")
	}
	if v.state.files != nil {
		t.Errorf("files = %v, want nil after a drop with nothing supported", v.state.files)
	}
	if !v.emptyStateArt.Visible() {
		t.Error("expected the error placeholder art in place of the cleared image")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast for the bad drop")
	}

	test.AssertRendersToImage(t, "bad_drop_after_images.png", win.Canvas())
	settleToast(t, v)
}

func TestE2E_EscapeResetsAfterImagesLoaded(t *testing.T) {
	v, win, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 40, 30, color.RGBA{R: 200, A: 255})
	dropAndWait(t, v, jpegURI)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after Escape resets", v.state.files)
	}
	if v.img.Visible() {
		t.Error("image should be hidden after Escape resets")
	}
	if !v.welcomeArt.Visible() {
		t.Error("welcome art should be back after Escape resets")
	}

	test.AssertRendersToImage(t, "after_escape_reset.png", win.Canvas())
}

// TestE2E_LaunchWithSavedSessionShowsRestoreLink builds the app with a
// session already saved in the cache (as a previous run's session.Save call
// would leave it - see internal/session) instead of going through newE2E,
// since the saved session needs to exist before buildViewer constructs the
// welcome screen around it.
func TestE2E_LaunchWithSavedSessionShowsRestoreLink(t *testing.T) {
	application := test.NewApp()

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{G: 200, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 40, 30, color.RGBA{B: 200, A: 255})
	session.Save(application, []fyne.URI{a, b})

	v, win := buildStartupViewer(application)
	defer win.Close()

	if !v.restoreLink.Visible() {
		t.Fatal("restoreLink should be visible when a session was saved")
	}

	test.AssertRendersToImage(t, "launch_with_restore_link.png", win.Canvas())

	v.restoreLink.OnTapped()
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if !v.img.Visible() || v.img.Image == nil {
		t.Fatal("expected an image to be loaded after tapping restoreLink")
	}
	if v.restoreLink.Visible() {
		t.Error("restoreLink should hide once the session has been restored")
	}

	test.AssertRendersToImage(t, "after_restore_session.png", win.Canvas())
}

// TestE2E_TappingRestoreLinkRestoresNotFileDialog guards dropzoneArt's
// bigger tap target (components.go): restoreLink is now nested inside it, and
// Fyne's hit-testing must still resolve a tap on restoreLink's own rendered
// position to restoreLink rather than to the wrapping dropzoneArt - or
// tapping "Restore last session" would silently open the file chooser
// instead of restoring anything.
func TestE2E_TappingRestoreLinkRestoresNotFileDialog(t *testing.T) {
	application := test.NewApp()

	saved := uitest.TempJPEGURI(t, "saved.jpg", 40, 30, color.RGBA{G: 200, A: 255})
	session.Save(application, []fyne.URI{saved})

	v, win := buildStartupViewer(application)
	defer win.Close()

	if !v.restoreLink.Visible() {
		t.Fatal("restoreLink should be visible when a session was saved")
	}

	chooserCalled := false
	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) {
		chooserCalled = true
		return nil, errors.New("stub: should not be reached")
	}

	size := v.restoreLink.Size()
	pos := application.Driver().AbsolutePositionForObject(v.restoreLink).
		Add(fyne.NewPos(size.Width/2, size.Height/2))
	test.TapCanvas(win.Canvas(), pos)

	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if chooserCalled {
		t.Fatal("tapping restoreLink should restore the session, not open the file chooser")
	}
	if !v.img.Visible() || v.img.Image == nil {
		t.Fatal("expected an image to be loaded after tapping restoreLink")
	}
}

func TestE2E_EscapeQuitsWhenNothingLoaded(t *testing.T) {
	v, _, closed := newTestUI(t)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if !closed() {
		t.Error("Escape should close the window when nothing is loaded")
	}
}

// TestE2E_EscapeCancelsScanInsteadOfClosing checks the priority handleKeyEvent
// gives Escape while a scan is in flight: len(v.state.files) == 0 is exactly the
// state a first-ever drop's scan runs in, so without the v.scanning check
// ahead of it, this would otherwise hit the "nothing loaded" branch above
// and close the window out from under a scan the user meant to cancel.
// v.scanning is set directly rather than racing a real background scan -
// see TestHandleDrop_SupersededScanGoroutineExits in library_test.go for why.
func TestE2E_EscapeCancelsScanInsteadOfClosing(t *testing.T) {
	v, _, closed := newTestUI(t)

	v.scanLifecycle.begin()
	v.scanning = true
	v.scanSpinner.Show()
	v.scanLabel.Show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if closed() {
		t.Error("Escape should cancel the in-flight scan, not close the window")
	}
	if v.scanning {
		t.Error("scanning should be false after Escape cancels it")
	}
	if !v.dropzone.Visible() || !v.welcomeArt.Visible() {
		t.Error("drop zone/welcome art should be restored after Escape cancels the scan")
	}
}

// TestE2E_DeleteConfirmationShowsWhichButtonReturnWillPress is a rendered
// test on purpose. The confirmation's own unit tests
// (deletion.TestSetSelection_TogglesRingVisibility) assert the selection
// rings' Visible flags, and those flags were always right - yet the card
// still drew identically whichever button was selected, because each ring
// was stacked at the same size as its button and a Fyne button paints an
// opaque background over its whole area. Only a pixel comparison can tell
// "the ring object is visible" from "the user can see the ring", so the
// guard against that regression is these two masters: nothing else in this
// suite would notice the highlight silently disappearing again.
func TestE2E_DeleteConfirmationShowsWhichButtonReturnWillPress(t *testing.T) {
	v, win, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 40, 30, color.RGBA{G: 200, A: 255})
	dropAndWait(t, v, jpegURI)

	v.deletion.Request()

	if !v.deletion.Visible() {
		t.Fatal("the confirmation card should be up after Request")
	}

	test.AssertRendersToImage(t, "delete_confirm_cancel.png", win.Canvas())

	// Right moves the selection onto the red button - the render must
	// differ from the one above, or Return's target is invisible.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})

	test.AssertRendersToImage(t, "delete_confirm_danger.png", win.Canvas())
}

// F1/showManual is intentionally not covered here: Fyne's test theme only
// defines fonts for 6 specific TextStyle combinations (test/theme.go), and
// the manual's markdown produces at least one combination outside that set
// (likely bold-inside-a-code-span), so CachedFontFace hits a nil font
// resource and panics - a gap in Fyne's test theme, not in this app.
