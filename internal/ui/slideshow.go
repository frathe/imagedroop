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
	v.slides.ToggleShuffle()
	v.applyTitle()
}
