package ui

import (
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file owns animate, load.go's per-frame goroutine for an animated
// GIF: that it actually advances frames on its own goroutine independent of
// the test, that navigating away supersedes and stops the previous image's
// animation rather than letting it bleed through onto the new one, and that
// cancelling loadLifecycle wakes a sleeping frame delay immediately instead
// of leaving it to sleep out the rest of the interval. Cancellation is what
// *stops* an animation, so that wake-up test belongs to animate's contract
// here rather than to invalidateLoad's own load_test.go.
//
// animate writes v.img.Image and bumps the v.animFrame atomic from its own
// goroutine (see its comment in load.go), so a test goroutine may never read
// v.img.Image until it has confirmed - via waitForAnimStopped, which waits
// for v.anim to finish - that animate has actually returned; polling
// animFrame with waitForAnimFrame is how a test observes progress in the
// meantime without racing those writes. Both helpers stay in
// harness_test.go as shared harness. Tests that need a known frame index
// (or to supersede a live animation without racing finishLoad) replace
// viewer.frameAfter with a frameClock before the first drop.

// frameClock is time.After that a test steps. parked is signalled each time
// After is called, so the test can wait until animate is sitting in its
// select rather than inside fyne.Do - the window where ShowImage's
// finishLoad would race displayFrames/displayFrameIdx under the test driver.
type frameClock struct {
	ticks  chan time.Time
	parked chan struct{}
}

func newFrameClock() *frameClock {
	return &frameClock{
		ticks:  make(chan time.Time),
		parked: make(chan struct{}, 1),
	}
}

func (c *frameClock) After(time.Duration) <-chan time.Time {
	select {
	case c.parked <- struct{}{}:
	default:
	}
	return c.ticks
}

func (c *frameClock) waitParked(t *testing.T) {
	t.Helper()
	select {
	case <-c.parked:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for animate to park on the frame clock")
	}
}

func (c *frameClock) tick(t *testing.T) {
	t.Helper()
	select {
	case c.ticks <- time.Time{}:
	case <-time.After(testTimeout):
		t.Fatal("timed out releasing a frame tick")
	}
}

func TestViewerShow_AnimatesGIF(t *testing.T) {
	v := newTestViewer(t)

	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})) // 20ms per frame, fast enough to keep the test quick

	dropAndWait(t, v, storage.NewFileURI(path))

	// animate() writes v.img.Image from its own goroutine for as long as its
	// load token stays current, which the fyne test driver never marshals onto
	// this one - so reading v.img.Image from here at any point before that
	// goroutine has fully stopped would race with those writes, even right
	// after waitForAnimFrame observes a given count: animate is free to keep
	// writing further frames in between that observation and the next
	// statement. animFrame reaching 2 (1 for attemptLoad's own first frame, 1
	// more for animate's first cycle) is proof the animation loop ran at all;
	// invalidating loadLifecycle and waiting for v.anim to finish then
	// guarantees no further write can happen, at which point animFrame's final
	// value is stable and it's finally safe to read v.img.Image.
	waitForAnimFrame(t, v, 2)

	v.loadLifecycle.invalidate()
	waitForAnimStopped(t, v)

	// Frame 0 (red) is written on odd counts (attemptLoad's initial write
	// is count 1), frame 1 (blue) on even ones - whichever count animate
	// happened to stop on, this checks the frame it left on screen actually
	// matches the data for that count instead of stale or corrupted pixels.
	n := v.animFrame.Load()
	wantBlue := n%2 == 0

	r, _, b, _ := v.img.Image.At(0, 0).RGBA()
	if wantBlue && b == 0 {
		t.Fatalf("expected the blue frame at animFrame=%d, got r=%d b=%d", n, r, b)
	}
	if !wantBlue && r == 0 {
		t.Fatalf("expected the red frame at animFrame=%d, got r=%d b=%d", n, r, b)
	}
}

func TestViewerShow_NavigatingAwayStopsAnimation(t *testing.T) {
	v := newTestViewer(t)
	clock := newFrameClock()

	// Write-once, before the drop: the same rule as vector.after. 10s GIF
	// delays so a missing seam cannot pass this by firing time.After on its
	// own inside testTimeout; the clock is what has to advance the frame.
	v.frameAfter = clock.After

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{1000, 1000})))
	staticURI := uitest.TempJPEGURI(t, "static.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, animURI, staticURI)
	clock.waitParked(t)
	clock.tick(t)
	waitForAnimFrame(t, v, 2)
	// After() is only called again once fyne.Do has returned, so parking
	// here is the happens-before that lets this goroutine read
	// displayFrameIdx (and later call ShowImage) without racing animate's
	// write under the test driver.
	clock.waitParked(t)

	if v.displayFrameIdx != 1 {
		t.Fatalf("displayFrameIdx = %d after one clock tick, want 1 - the animation must have actually cycled", v.displayFrameIdx)
	}
	if _, _, b, _ := v.img.Image.At(0, 0).RGBA(); b == 0 {
		t.Fatal("expected the blue frame on screen after one clock tick")
	}

	oldAnim := v.anim.Current()

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	waitHandle(t, "the superseded animation to stop", oldAnim)

	// JPEG is lossy, so a "solid green" square won't decode back to an exact
	// R=0, but green should still clearly dominate; an animation frame
	// bleeding through would show red or blue dominating instead.
	r, g, b, _ := v.img.Image.At(0, 0).RGBA()
	if g <= r || g <= b {
		t.Errorf("expected the static green image to remain displayed, got r=%d g=%d b=%d", r, g, b)
	}
}

// TestInvalidateLoad_WakesAnimateImmediately parks animate on a clock that
// never ticks and checks lifecycle cancellation wakes it immediately rather
// than waiting for the next frame.
func TestInvalidateLoad_WakesAnimateImmediately(t *testing.T) {
	v := newTestViewer(t)
	parkAnimate(v)

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "slow.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})))

	dropAndWait(t, v, animURI)

	if !v.anim.Begun() {
		t.Fatal("loading an animated GIF should arm the animation signal")
	}

	v.loadLifecycle.invalidate()

	waitForAnimStopped(t, v)
	v.loadLifecycle.invalidate() // repeated invalidation must remain safe
}
