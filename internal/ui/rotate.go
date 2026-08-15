package ui

import "github.com/frathe/imagedrop/internal/imaging"

// rotateBy is the R key (clockwise, steps=1) / Shift+R (counter-clockwise,
// steps=-1): rotates the displayed image by one 90-degree step, view-only -
// see imaging.RotateSteps's doc for why this is safe to compose with the
// EXIF orientation already baked into the decoded pixels, and why repeated
// presses never degrade the image. It's a no-op before any image has
// loaded. Like a fresh navigation (finishLoad), a rotation resets zoom back
// to fit and, since a 90/270-degree turn swaps which axis is which, resizes
// the window to match - a manual zoom level or window size chosen for the
// old orientation rarely still makes sense once the axes have swapped.
func (v *viewer) rotateBy(steps int) {
	if len(v.displayFrames) == 0 {
		return
	}

	v.rotation = ((v.rotation+steps)%4 + 4) % 4
	v.redrawRotatedFrame()
	v.applyRotationLayout()
}

// resetRotation is the other half of the 0 key (see zoom.FitToWindow):
// clears any view-only rotation back to the image's native EXIF
// orientation, the same way 0 resets zoom back to fit.
func (v *viewer) resetRotation() {
	if v.rotation == 0 {
		return
	}

	v.rotation = 0
	v.redrawRotatedFrame()
	v.applyRotationLayout()
}

// redrawRotatedFrame recomputes v.img.Image from the current unrotated
// frame (displayFrames[displayFrameIdx]) and rotation, and refreshes the
// canvas. Shared by rotateBy/resetRotation (a key press) and finishLoad/
// animate (loading a fresh image or advancing a GIF), so a rotation applied
// mid-animation keeps being applied to every later frame too, not just the
// one that happened to be on screen when R was pressed.
func (v *viewer) redrawRotatedFrame() {
	v.img.Image = imaging.RotateSteps(v.displayFrames[v.displayFrameIdx], v.rotation)
	v.img.Refresh()
	v.animFrame.Add(1)
}

// applyRotationLayout re-fits and, outside picture-frame mode (where the
// window is already full-screen with nothing to resize - see finishLoad's
// matching comment), resizes the window to the just-redrawn frame's bounds.
// Mirrors finishLoad's own ordering: re-fit first, for immediate visual
// feedback against whatever viewport size the zoom view currently has
// cached, then the window resize, whose own layout pass will re-lay it out
// against the authoritative new size.
func (v *viewer) applyRotationLayout() {
	v.zoom.ResetToFit()

	if !v.slides.Active() {
		v.undoGridMaximize()
		resizeToImage(v.win, v.img.Image.Bounds())
	}

	v.updateInfoOverlay()
}
