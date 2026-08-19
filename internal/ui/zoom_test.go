// Zoom as the app wires it: the keys reaching the component, the reset on
// navigation, and the widget sitting in the real window's content. The
// zoom/pan state machine and its geometry - fit scale, clamping, anchored
// scroll-zoom, the cursor - are tested against a bare canvas.Image in
// internal/ui/zoom.

package ui

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/uitest"
)

// --- handleKeyEvent dispatch -------------------------------------------

func TestHandleKeyEvent_ZoomShortcuts(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 400, 200, color.White)
	dropAndWait(t, v, a)

	// A single loaded file has nothing to navigate to, but zoom shortcuts
	// must still work - they're dispatched ahead of the navigation guard.
	if !v.zoom.Fitting() {
		t.Fatal("a freshly loaded image should start at fit scale")
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.Key1})
	if v.zoom.Fitting() || v.zoom.Percent() != 100 {
		t.Errorf("after 1: fitting=%v percent=%d, want fitting=false percent=100",
			v.zoom.Fitting(), v.zoom.Percent())
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyPlus})
	zoomedIn := v.zoom.Percent()
	if zoomedIn <= 100 {
		t.Errorf("after +: percent = %d, want more than 100", zoomedIn)
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyMinus})
	if got := v.zoom.Percent(); got != 100 {
		t.Errorf("after -: percent = %d, want back to 100", got)
	}

	// = is the unshifted + on most layouts, so it's bound to zoom in too.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEqual})
	if got := v.zoom.Percent(); got != zoomedIn {
		t.Errorf("after =: percent = %d, want %d - the same step + applies", got, zoomedIn)
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.Key0})
	if !v.zoom.Fitting() {
		t.Error("after 0: should be back in fit mode")
	}
}

// TestHandleKeyEvent_ZoomInThenOutClearsGrabCursor is an end-to-end
// regression test for the exact steps reported: load an image, press +
// once, press - once, and the hand cursor stuck around even though the
// image was back to exactly filling the window - see the zoom package's
// TestInThenOutLeavesNothingToPan for the underlying float32 cause.
func TestHandleKeyEvent_ZoomInThenOutClearsGrabCursor(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 377, 251, color.White) // odd dims, prone to rounding
	dropAndWait(t, v, a)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyPlus})
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyMinus})

	if v.zoom.CanPan() {
		t.Errorf("zoom in then out should leave nothing to pan, got CanPan() = true (percent=%d)",
			v.zoom.Percent())
	}
}

// --- reset on navigation -------------------------------------------------

func TestShow_ResetsZoomOnNavigation(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.zoom.ActualSize()
	if v.zoom.Fitting() {
		t.Fatal("setup: expected zoom to be off before navigating")
	}

	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if !v.zoom.Fitting() {
		t.Error("navigating to a new image should reset zoom back to fit")
	}
}

// --- the widget in the real content tree ---------------------------------

func TestZoomWidget_LayoutFillsViewportAtFit(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	contentSize := v.win.Content().Size()
	if got := v.img.Size(); !uitest.ApproxEqual(got.Width, contentSize.Width) || !uitest.ApproxEqual(got.Height, contentSize.Height) {
		t.Errorf("img size = %v, want the window content's size %v while fitting", got, contentSize)
	}
}

// --- syncWindowToZoom (window tracks user zoom) --------------------------

// windowSizeFor is a test helper that returns the canvas size that
// resizeToImage produces for a given image rectangle and viewer's current
// max-window cap. Using a real test.Window (not arithmetic) keeps the
// helper in sync with resizeToImage's own rounding.
func windowSizeFor(t *testing.T, v *viewer, w, h int) (width, height float32) {
	t.Helper()

	win := test.NewWindow(nil)
	defer win.Close()
	resizeToImage(win, image.Rect(0, 0, w, h), v.maxWinW, v.maxWinH)
	size := win.Canvas().Size()
	return size.Width, size.Height
}

func TestSyncWindowToZoom_ZoomInGrowsWindow(t *testing.T) {
	v := newTestViewer(t)
	// 800×600: above startW×startH, well below the default cap.
	a := uitest.TempJPEGURI(t, "a.jpg", 800, 600, color.White)
	dropAndWait(t, v, a)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyPlus})

	nw, nh := v.displayedDimensions()
	s := v.zoom.Scale()
	scaledW := int(float32(nw)*s + 0.5)
	scaledH := int(float32(nh)*s + 0.5)
	wantW, wantH := windowSizeFor(t, v, scaledW, scaledH)

	got := v.win.Canvas().Size()
	if !uitest.ApproxEqual(got.Width, wantW) || !uitest.ApproxEqual(got.Height, wantH) {
		t.Errorf("window after zoom-in = %vx%v, want %vx%v (resizeToImage of %dx%d)",
			got.Width, got.Height, wantW, wantH, scaledW, scaledH)
	}
}

func TestSyncWindowToZoom_FitToWindowRestoresWindow(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 800, 600, color.White)
	dropAndWait(t, v, a)

	// Zoom in to grow the window beyond the fit size.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyPlus})

	// Key0 calls FitToWindow → syncWindowToZoom with Fitting()==true, which
	// must resize back to resizeToImage of the unscaled native bounds.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.Key0})

	nw, nh := v.displayedDimensions()
	wantW, wantH := windowSizeFor(t, v, nw, nh)

	got := v.win.Canvas().Size()
	if !uitest.ApproxEqual(got.Width, wantW) || !uitest.ApproxEqual(got.Height, wantH) {
		t.Errorf("window after FitToWindow = %vx%v, want %vx%v (native fit %dx%d)",
			got.Width, got.Height, wantW, wantH, nw, nh)
	}
}

func TestSyncWindowToZoom_ActualSizeOnLargeImage(t *testing.T) {
	v := newTestViewer(t)
	// 800×600 is larger than startW×startH, so Key1 (100%) should size
	// the window to exactly the native pixel dims (no floor or cap needed).
	a := uitest.TempJPEGURI(t, "a.jpg", 800, 600, color.White)
	dropAndWait(t, v, a)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.Key1})

	nw, nh := v.displayedDimensions()
	wantW, wantH := windowSizeFor(t, v, nw, nh) // scale=1 → same as native

	got := v.win.Canvas().Size()
	if !uitest.ApproxEqual(got.Width, wantW) || !uitest.ApproxEqual(got.Height, wantH) {
		t.Errorf("window after ActualSize = %vx%v, want %vx%v (native %dx%d at 100%%)",
			got.Width, got.Height, wantW, wantH, nw, nh)
	}
}

func TestSyncWindowToZoom_TinyImageZoomOutStaysAtFloor(t *testing.T) {
	v := newTestViewer(t)
	// 50×50: resizeToImage floors the window to startW×startH on load, and
	// any zoom-out (which makes the scaled target even smaller) must still
	// stay there rather than shrinking below the minimum grabbable size.
	a := uitest.TempJPEGURI(t, "a.jpg", 50, 50, color.White)
	dropAndWait(t, v, a)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyMinus})

	got := v.win.Canvas().Size()
	if !uitest.ApproxEqual(got.Width, startW) || !uitest.ApproxEqual(got.Height, startH) {
		t.Errorf("window after zoom-out on tiny image = %vx%v, want startW×startH %vx%v",
			got.Width, got.Height, float32(startW), float32(startH))
	}
}

func TestSyncWindowToZoom_LargeImageStaysCapped(t *testing.T) {
	v := newTestViewer(t)
	// 3000×2000: resizeToImage caps the window to maxWinW×maxWinH on load.
	// Zooming in should keep the window at that same capped size.
	a := uitest.TempJPEGURI(t, "a.jpg", 3000, 2000, color.White)
	dropAndWait(t, v, a)

	loadSize := v.win.Canvas().Size()

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyPlus})

	// The scaled target (native × scale) still exceeds the max cap, so
	// resizeToImage clamps again and the window stays at approximately the
	// load size — verified by computing the expected size the same way
	// syncWindowToZoom does and letting resizeToImage do the clamping.
	nw, nh := v.displayedDimensions()
	s := v.zoom.Scale()
	scaledW := int(float32(nw)*s + 0.5)
	scaledH := int(float32(nh)*s + 0.5)
	wantW, wantH := windowSizeFor(t, v, scaledW, scaledH)

	got := v.win.Canvas().Size()
	if !uitest.ApproxEqual(got.Width, wantW) || !uitest.ApproxEqual(got.Height, wantH) {
		t.Errorf("window after zoom-in on large image = %vx%v, want %vx%v (capped from %dx%d)",
			got.Width, got.Height, wantW, wantH, scaledW, scaledH)
	}
	_ = loadSize // load size documented for context; what matters is the cap
}

func TestSyncWindowToZoom_GridVisibleDoesNotResize(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 800, 600, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 800, 600, color.White)
	dropAndWait(t, v, a, b)

	warmThumbs(t, v)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG}) // open grid
	if !v.grid.Visible() {
		t.Fatal("setup: grid should be visible")
	}

	gridSize := v.win.Canvas().Size()

	// Call zoom.In() directly — bypassing the handleKeyEvent grid guard —
	// to confirm syncWindowToZoom's own guard prevents the resize.
	v.zoom.In()

	got := v.win.Canvas().Size()
	if got != gridSize {
		t.Errorf("window changed while grid was open: %v → %v; syncWindowToZoom must be a no-op",
			gridSize, got)
	}
}

func TestSyncWindowToZoom_SlideshowActiveDoesNotResize(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 800, 600, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 800, 600, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })
	if !v.slides.Active() {
		t.Fatal("setup: slideshow should be active")
	}

	slideshowSize := v.win.Canvas().Size()

	// Call zoom.In() directly to confirm syncWindowToZoom's slideshow guard.
	v.zoom.In()

	got := v.win.Canvas().Size()
	if got != slideshowSize {
		t.Errorf("window changed while slideshow was active: %v → %v; syncWindowToZoom must be a no-op",
			slideshowSize, got)
	}
}
