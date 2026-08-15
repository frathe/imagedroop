// Zoom as the app wires it: the keys reaching the component, the reset on
// navigation, and the widget sitting in the real window's content. The
// zoom/pan state machine and its geometry - fit scale, clamping, anchored
// scroll-zoom, the cursor - are tested against a bare canvas.Image in
// internal/ui/zoom.

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/uitest"
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

	v.ShowImage(v.index + 1)
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
