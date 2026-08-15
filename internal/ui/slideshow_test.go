// Picture-frame mode as the app wires it: the P key reaching the
// controller, the interplay with navigation and reset, and the load path
// feeding it each image's animation length. The controller's own state
// machine - entering, exiting, the interval, the advance/staleness logic -
// is tested against a fake host in internal/ui/slideshow.

package ui

import (
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/imagedrop/internal/ui/slideshow"
	"github.com/frathe/imagedrop/internal/uitest"
)

// --- toggling --------------------------------------------------------------

func TestTogglePictureFrameMode_EntersAndExitsFullScreen(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })
	if !v.slides.Active() {
		t.Error("picture-frame mode should be on after the first toggle")
	}
	if !v.win.FullScreen() {
		t.Error("window should be full-screen after entering picture-frame mode")
	}

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })
	if v.slides.Active() {
		t.Error("picture-frame mode should be off after the second toggle")
	}
	if v.win.FullScreen() {
		t.Error("window should leave full-screen after exiting picture-frame mode")
	}

	// Exiting must not touch the loaded set.
	if len(v.files) != 2 {
		t.Errorf("files = %d, want 2 to remain loaded after leaving picture-frame mode", len(v.files))
	}
}

// --- key handling ------------------------------------------------------

func TestHandleKeyEvent_PEntersPictureFrameMode(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyP})
	t.Cleanup(func() { settleSlideshow(t, v) })

	if !v.slides.Active() || !v.win.FullScreen() {
		t.Error("P should enter picture-frame mode and full-screen the window")
	}
}

func TestHandleKeyEvent_EscapeLeavesPictureFrameModeWithoutResetting(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if v.slides.Active() {
		t.Error("Escape should leave picture-frame mode")
	}
	if v.win.FullScreen() {
		t.Error("Escape should leave full-screen")
	}
	if len(v.files) != 2 {
		t.Errorf("files = %d, want the loaded set untouched by Escape while in picture-frame mode", len(v.files))
	}

	// A second Escape, now that picture-frame mode is off, falls through to
	// the usual reset behavior.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if v.files != nil {
		t.Error("a second Escape should reset the session, same as usual")
	}
}

func TestHandleKeyEvent_UpDownAdjustIntervalInsteadOfNavigating(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })
	startIndex := v.index

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyUp})
	if want := slideshow.DefaultInterval + time.Second; v.slides.Interval() != want {
		t.Errorf("interval after Up = %v, want %v", v.slides.Interval(), want)
	}
	if v.index != startIndex {
		t.Error("Up should not navigate while in picture-frame mode")
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyDown})
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyDown})
	if want := slideshow.DefaultInterval - time.Second; v.slides.Interval() != want {
		t.Errorf("interval after Up then two Downs = %v, want %v", v.slides.Interval(), want)
	}
	if v.index != startIndex {
		t.Error("Down should not navigate while in picture-frame mode")
	}
}

func TestHandleKeyEvent_UpDownNavigateOutsidePictureFrameMode(t *testing.T) {
	// The mirror of the test above: the interval binding is scoped to
	// picture-frame mode, so with it off the same keys must still be plain
	// navigation.
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	dropAndWait(t, v, a, b)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyDown})
	waitUntilLoaded(t, v)

	if v.index != 1 {
		t.Errorf("index = %d, want 1 after Down outside picture-frame mode", v.index)
	}
	if v.slides.Interval() != 0 {
		t.Errorf("interval = %v, want it untouched by a navigation key", v.slides.Interval())
	}
}

func TestHandleKeyEvent_LeftRightStillNavigateInPictureFrameMode(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})
	waitUntilLoaded(t, v)

	if v.index != 1 {
		t.Errorf("index = %d, want 1 after Right in picture-frame mode", v.index)
	}
}

func TestClearToDropzone_ExitsPictureFrameMode(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.reset()

	if v.slides.Active() {
		t.Error("reset should leave picture-frame mode")
	}
	if v.win.FullScreen() {
		t.Error("reset should leave full-screen")
	}
}

// --- Advance (the Host method the auto-advance calls) ----------------------

func TestAdvance_WrapsAroundAtTheEnd(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	dropAndWait(t, v, a, b)

	v.Advance()
	waitUntilLoaded(t, v)
	if v.index != 1 {
		t.Fatalf("index = %d, want 1 after the first Advance", v.index)
	}

	// A slideshow left running has to loop rather than stop at the end.
	v.Advance()
	waitUntilLoaded(t, v)
	if v.index != 0 {
		t.Errorf("index = %d, want 0 - Advance past the last file wraps around", v.index)
	}
}

// --- animation duration tracking -------------------------------------------

func TestShow_TracksAnimatedGIFLoopDuration(t *testing.T) {
	v := newTestViewer(t)

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{5, 10}))) // 50ms + 100ms = 150ms total loop
	staticURI := uitest.TempJPEGURI(t, "static.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, animURI, staticURI)

	if got, want := v.slides.AnimDuration(), 150*time.Millisecond; got != want {
		t.Errorf("AnimDuration after loading the gif = %v, want %v", got, want)
	}

	v.ShowImage(v.index + 1)
	waitUntilLoaded(t, v)

	if got := v.slides.AnimDuration(); got != 0 {
		t.Errorf("AnimDuration after loading a static image = %v, want 0", got)
	}
}
