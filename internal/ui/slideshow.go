package ui

import (
	"time"

	"github.com/frathe/picfetch/internal/ui/slideshow"
)

// togglePictureFrameMode flips picture-frame mode - the slideshow - on or
// off, bound to P (see handleKeyEvent). Everything about the mode itself
// lives in internal/ui/slideshow; what stays here is the one thing that
// package must not know: the app's other full-window mode.
//
// The two don't compose, so entering one closes the other - orchestrated
// here rather than inside either package, the same way the G key's mirror
// guard is (see handleKeyEvent). Closing the grid unconditionally, before
// even knowing which way this toggle goes, is the simpler half of that
// pair: the grid is already closed on the way out, so closing it again
// costs nothing.
func (v *viewer) togglePictureFrameMode() {
	v.grid.Close()

	wasActive := v.slides.Active()
	v.slides.Toggle()

	// resetFade only matters on the way out - Toggle just called Exit()
	// internally - since there is nothing to reset on the way in. Handled
	// here rather than inside slideshow.Exit itself, the same reason
	// togglePictureFrameMode itself exists: the slideshow package doesn't
	// know v.img exists.
	if wasActive {
		v.resetFade()
	}
}

// toggleSlideshowShuffle flips whether picture-frame mode's auto-advance
// (Shift+P, see handleKeyEvent) picks a random other file instead of the
// next one in order, and immediately reflects it in the window title via
// the "[shuffle]" prefix - the same way toggleMergeMode does for merge
// mode. Works whether picture-frame mode is currently on or off, the same
// as M and S do for their own standing preferences: it just pre-arms the
// order for whenever picture-frame mode next runs.
func (v *viewer) toggleSlideshowShuffle() {
	v.SetSlideShuffle(!v.slides.Shuffle())
}

// SetSlideShuffle sets picture-frame mode's shuffle order directly - the
// settings window's binding for the toggle above.
func (v *viewer) SetSlideShuffle(on bool) {
	v.slides.SetShuffle(on)
	v.applyTitle()
}

// SlideShuffle reports whether picture-frame mode's auto-advance is
// currently shuffled - the settings window's getter.
func (v *viewer) SlideShuffle() bool {
	return v.slides.Shuffle()
}

// SlideInterval is the picture-frame auto-advance interval - the settings
// window's getter. Substitutes slideshow.DefaultInterval for the
// controller's own 0 ("never chosen yet" - see Controller.Interval), so the
// settings window shows the pace picture-frame mode will actually start at
// instead of a bare zero before the mode has ever been entered.
func (v *viewer) SlideInterval() time.Duration {
	if d := v.slides.Interval(); d > 0 {
		return d
	}

	return slideshow.DefaultInterval
}

// SetSlideInterval sets the picture-frame auto-advance interval directly -
// the settings window's binding, mirroring Up/Down's AdjustInterval while
// the mode is active. Clamped to slideshow.MinInterval the same way
// AdjustInterval is, and kicks the countdown already in progress so a
// change made while the mode is active applies right away.
func (v *viewer) SetSlideInterval(d time.Duration) {
	if d < slideshow.MinInterval {
		d = slideshow.MinInterval
	}

	v.slides.SetInterval(d)
	v.slides.Kick()
}
