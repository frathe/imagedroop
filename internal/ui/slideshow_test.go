// Picture-frame mode as the app wires it: the P key reaching the
// controller, the interplay with navigation and reset, and the load path
// feeding it each image's animation length. The controller's own state
// machine - entering, exiting, the interval, the advance/staleness logic -
// is tested against a fake host in internal/ui/slideshow.

package ui

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/ui/slideshow"
	"github.com/frathe/picfetch/internal/uitest"
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
	if len(v.state.files) != 2 {
		t.Errorf("files = %d, want 2 to remain loaded after leaving picture-frame mode", len(v.state.files))
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
	if len(v.state.files) != 2 {
		t.Errorf("files = %d, want the loaded set untouched by Escape while in picture-frame mode", len(v.state.files))
	}

	// A second Escape, now that picture-frame mode is off, falls through to
	// the usual reset behavior.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if v.state.files != nil {
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
	startIndex := v.state.index

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyUp})
	if want := slideshow.DefaultInterval + time.Second; v.slides.Interval() != want {
		t.Errorf("interval after Up = %v, want %v", v.slides.Interval(), want)
	}
	if v.state.index != startIndex {
		t.Error("Up should not navigate while in picture-frame mode")
	}

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyDown})
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyDown})
	if want := slideshow.DefaultInterval - time.Second; v.slides.Interval() != want {
		t.Errorf("interval after Up then two Downs = %v, want %v", v.slides.Interval(), want)
	}
	if v.state.index != startIndex {
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

	if v.state.index != 1 {
		t.Errorf("index = %d, want 1 after Down outside picture-frame mode", v.state.index)
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

	if v.state.index != 1 {
		t.Errorf("index = %d, want 1 after Right in picture-frame mode", v.state.index)
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
	if v.state.index != 1 {
		t.Fatalf("index = %d, want 1 after the first Advance", v.state.index)
	}

	// A slideshow left running has to loop rather than stop at the end.
	v.Advance()
	waitUntilLoaded(t, v)
	if v.state.index != 0 {
		t.Errorf("index = %d, want 0 - Advance past the last file wraps around", v.state.index)
	}
}

// --- animation duration tracking -------------------------------------------

func TestShow_TracksAnimatedGIFLoopDuration(t *testing.T) {
	v := newTestViewer(t)
	parkAnimate(v)

	// Delays still have to sum to 30s - that's what AnimDuration reports -
	// but they no longer park the goroutine: frameAfter does, so ShowImage
	// below cannot race animate's writes to displayFrames/displayFrameIdx.
	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{1000, 2000}))) // 10s + 20s = 30s total loop, in centiseconds
	staticURI := uitest.TempJPEGURI(t, "static.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, animURI, staticURI)

	if got, want := v.slides.AnimDuration(), 30*time.Second; got != want {
		t.Errorf("AnimDuration after loading the gif = %v, want %v", got, want)
	}

	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if got := v.slides.AnimDuration(); got != 0 {
		t.Errorf("AnimDuration after loading a static image = %v, want 0", got)
	}
}

// --- shuffle (Shift+P) -------------------------------------------------

func TestHandleKeyEvent_ShiftPTogglesShuffleAndPrefixesTitle(t *testing.T) {
	v := newTestViewer(t)

	if title := v.win.Title(); strings.Contains(title, "[shuffle]") {
		t.Fatalf("title = %q, should not start prefixed before Shift+P is ever pressed", title)
	}

	// Shift+P works even with nothing loaded yet, and takes effect
	// immediately, the same as M does for merge mode.
	stubKeyModifiers(t, v, fyne.KeyModifierShift)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyP})

	if v.slides.Active() {
		t.Fatal("Shift+P should toggle shuffle, not enter picture-frame mode")
	}
	if !v.slides.Shuffle() {
		t.Fatal("Shuffle() = false, want true after Shift+P")
	}
	if title := v.win.Title(); !strings.HasPrefix(title, "[shuffle] ") {
		t.Fatalf("title = %q, want it prefixed with [shuffle] right after Shift+P", title)
	}

	// Shift+P again turns it back off.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyP})
	if v.slides.Shuffle() {
		t.Error("Shuffle() = true, want false after a second Shift+P")
	}
	if title := v.win.Title(); strings.Contains(title, "[shuffle]") {
		t.Errorf("title = %q, want the [shuffle] prefix gone after toggling Shift+P again", title)
	}
}

func TestHandleKeyEvent_PlainPStillEntersPictureFrameModeAfterShiftP(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	stubKeyModifiers(t, v, fyne.KeyModifierShift)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyP})
	if v.slides.Active() {
		t.Fatal("Shift+P should not enter picture-frame mode")
	}

	stubKeyModifiers(t, v, 0)
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyP})
	t.Cleanup(func() { settleSlideshow(t, v) })

	if !v.slides.Active() {
		t.Fatal("plain P should still enter picture-frame mode after an earlier Shift+P set shuffle")
	}
}

func TestAdvance_ShuffleOnNeverRepeatsCurrentIndex(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	c := uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.RGBA{B: 255, A: 255})
	dropAndWait(t, v, a, b, c)

	v.slides.SetShuffle(true)

	for i := range 20 {
		before := v.state.index
		v.Advance()
		waitUntilLoaded(t, v)

		if v.state.index == before {
			t.Fatalf("iteration %d: index stayed at %d after Advance with shuffle on", i, before)
		}
		if v.state.index < 0 || v.state.index >= len(v.state.files) {
			t.Fatalf("iteration %d: index = %d out of range", i, v.state.index)
		}
	}
}

// --- settings window Host methods ---------------------------------------

// TestSlideShuffleGetterSetter is SlideShuffle/SetSlideShuffle - the
// settings window's binding - as opposed to the Shift+P flip already
// covered above.
func TestSlideShuffleGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if v.SlideShuffle() {
		t.Fatal("SlideShuffle() = true, want false by default")
	}

	v.SetSlideShuffle(true)
	if !v.SlideShuffle() {
		t.Error("SlideShuffle() = false, want true after SetSlideShuffle(true)")
	}
	if title := v.win.Title(); !strings.HasPrefix(title, "[shuffle] ") {
		t.Errorf("title = %q, want it prefixed right after SetSlideShuffle(true)", title)
	}

	v.SetSlideShuffle(false)
	if v.SlideShuffle() {
		t.Error("SlideShuffle() = true, want false after SetSlideShuffle(false)")
	}
}

// TestSlideInterval_DefaultsBeforePictureFrameModeEverRuns checks the
// getter substitutes slideshow.DefaultInterval for the controller's own
// "never chosen" zero, so the settings window shows the pace picture-frame
// mode will actually start at rather than a bare 0s.
func TestSlideInterval_DefaultsBeforePictureFrameModeEverRuns(t *testing.T) {
	v := newTestViewer(t)

	if got := v.SlideInterval(); got != slideshow.DefaultInterval {
		t.Errorf("SlideInterval() = %v, want the default %v before picture-frame mode ever runs", got, slideshow.DefaultInterval)
	}
}

func TestSetSlideInterval_TakesEffect(t *testing.T) {
	v := newTestViewer(t)

	v.SetSlideInterval(30 * time.Second)

	if got := v.SlideInterval(); got != 30*time.Second {
		t.Errorf("SlideInterval() = %v, want 30s", got)
	}
}

// TestSetSlideInterval_ClampsToMinimum mirrors AdjustInterval's own floor
// (Up/Down while picture-frame mode is active) - a settings-window value
// below it would otherwise make the auto-advance spin needlessly fast.
func TestSetSlideInterval_ClampsToMinimum(t *testing.T) {
	v := newTestViewer(t)

	v.SetSlideInterval(100 * time.Millisecond)

	if got := v.SlideInterval(); got != slideshow.MinInterval {
		t.Errorf("SlideInterval() = %v, want it clamped to MinInterval (%v)", got, slideshow.MinInterval)
	}
}

// --- randomOtherIndex --------------------------------------------------

func TestRandomOtherIndex_EmptyOrSingleReturnsCurrentUnchanged(t *testing.T) {
	if got := randomOtherIndex(0, 0); got != 0 {
		t.Errorf("randomOtherIndex(0, 0) = %d, want 0 (nothing to pick from)", got)
	}
	if got := randomOtherIndex(1, 0); got != 0 {
		t.Errorf("randomOtherIndex(1, 0) = %d, want 0 (no other index exists)", got)
	}
}

func TestRandomOtherIndex_NeverReturnsCurrent(t *testing.T) {
	for n := 2; n <= 6; n++ {
		for current := range n {
			for range 200 {
				got := randomOtherIndex(n, current)
				if got == current {
					t.Fatalf("randomOtherIndex(%d, %d) returned current index %d", n, current, got)
				}
				if got < 0 || got >= n {
					t.Fatalf("randomOtherIndex(%d, %d) = %d, out of range", n, current, got)
				}
			}
		}
	}
}

func TestRandomOtherIndex_CoversEveryOtherIndex(t *testing.T) {
	const n = 4
	const current = 1
	seen := make(map[int]bool)

	for range 1000 {
		seen[randomOtherIndex(n, current)] = true
	}

	for i := range n {
		if i == current {
			continue
		}
		if !seen[i] {
			t.Errorf("index %d never appeared across 1000 draws of randomOtherIndex(%d, %d)", i, n, current)
		}
	}
}

// --- crossfade -----------------------------------------------------------

func TestShowImage_InPictureFrameModeEndsFullyOpaque(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.RGBA{G: 255, A: 255})
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.img.Translucency != 0 {
		t.Errorf("Translucency after a picture-frame-mode navigation = %v, want 0 (fully faded in)", v.img.Translucency)
	}
}

func TestTogglePictureFrameMode_ExitResetsFade(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	// Simulate a fade caught mid-transition, as if leaving picture-frame
	// mode landed exactly between ShowImage's fade-out and finishLoad's
	// fade-in.
	v.img.Translucency = 0.5
	v.fadeAnim = fyne.NewAnimation(time.Hour, func(float32) {})

	v.togglePictureFrameMode()

	if v.img.Translucency != 0 {
		t.Errorf("Translucency after leaving picture-frame mode = %v, want 0", v.img.Translucency)
	}
	if v.fadeAnim != nil {
		t.Error("fadeAnim should be cleared after leaving picture-frame mode")
	}
}

func TestHandleKeyEvent_EscapeResetsFade(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.img.Translucency = 0.5
	v.fadeAnim = fyne.NewAnimation(time.Hour, func(float32) {})

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if v.img.Translucency != 0 {
		t.Errorf("Translucency after Escape out of picture-frame mode = %v, want 0", v.img.Translucency)
	}
}

func TestReset_ResetsFadeLeftMidTransition(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	v.togglePictureFrameMode()
	t.Cleanup(func() { settleSlideshow(t, v) })

	v.img.Translucency = 0.5
	v.fadeAnim = fyne.NewAnimation(time.Hour, func(float32) {})

	v.reset()

	if v.img.Translucency != 0 {
		t.Errorf("Translucency after reset mid-transition = %v, want 0", v.img.Translucency)
	}
}
