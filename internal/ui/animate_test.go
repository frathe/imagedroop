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
// for v.animStopped to close - that animate has actually returned; polling
// animFrame with waitForAnimFrame is how a test observes progress in the
// meantime without racing those writes. Both helpers stay in
// harness_test.go as shared harness.

func TestViewerShow_AnimatesGIF(t *testing.T) {
	v := newTestViewer(t)

	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})) // 20ms per frame, fast enough to keep the test quick

	dropAndWait(t, v, storage.NewFileURI(path))

	// animate() writes v.img.Image from its own goroutine for as long as
	// its load token stays current, which the fyne test driver never marshals onto
	// this one - so reading v.img.Image from here at any point before that
	// goroutine has fully stopped would race with those writes, even right
	// after waitForAnimFrame observes a given count: animate is free to
	// keep writing further frames in between that observation and the next
	// statement. animFrame reaching 2 (1 for attemptLoad's own first frame,
	// 1 more for animate's first cycle) is proof the animation loop ran at
	// all; invalidating loadLifecycle and waiting for animStopped then guarantees no
	// further write can happen, at which point animFrame's final value is
	// stable and it's finally safe to read v.img.Image.
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

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})))
	staticURI := uitest.TempJPEGURI(t, "static.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, animURI, staticURI)

	// Capture the first image's animate goroutine before superseding it -
	// once ShowImage(1) bumps gen, that goroutine's close(stopped) is the
	// only signal that it has actually noticed and returned, rather than
	// still being asleep between frames.
	oldAnimStopped := v.animStopped

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	// Wait for the superseded animation goroutine to actually stop instead
	// of sleeping a fixed duration and hoping: it writes v.img.Image from
	// its own goroutine, so reading the field before it's confirmed done
	// would race with that write even though the staleness check means it
	// would never actually overwrite the static image once gen has moved
	// on.
	select {
	case <-oldAnimStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the superseded animation to stop")
	}

	// JPEG is lossy, so a "solid green" square won't decode back to an exact
	// R=0, but green should still clearly dominate; an animation frame
	// bleeding through would show red or blue dominating instead.
	r, g, b, _ := v.img.Image.At(0, 0).RGBA()
	if g <= r || g <= b {
		t.Errorf("expected the static green image to remain displayed, got r=%d g=%d b=%d", r, g, b)
	}
}

// TestInvalidateLoad_WakesAnimateImmediately parks animate in a frame-delay
// sleep far longer than the test and checks lifecycle cancellation wakes it
// immediately rather than waiting for the next frame tick.
func TestInvalidateLoad_WakesAnimateImmediately(t *testing.T) {
	v := newTestViewer(t)

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "slow.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{1000, 1000}))) // 10s per frame, in centiseconds

	dropAndWait(t, v, animURI)

	if v.animStopped == nil {
		t.Fatal("loading an animated GIF should arm animStopped")
	}

	v.loadLifecycle.invalidate()

	waitForAnimStopped(t, v)
	v.loadLifecycle.invalidate() // repeated invalidation must remain safe
}
