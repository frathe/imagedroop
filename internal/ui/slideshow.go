package ui

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
	v.slides.Toggle()
}
