// The persistent info overlay (I key).

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/lang"
)

// toggleInfoOverlay is the I key: flips the persistent info card (filename,
// position, pixel dimensions, file size, zoom level) on or off. Modeled on
// the toast card (toast.go) - a pinned overlay rather than a new window -
// but, unlike a toast, it never auto-hides: once on, it stays up across
// navigation and zoom changes until toggled off again.
func (v *viewer) toggleInfoOverlay() {
	v.infoVisible = !v.infoVisible
	v.syncInfoOverlayVisibility()
	v.ForceRepaint()
}

// syncInfoOverlayVisibility shows or hides infoCard to match v.infoVisible,
// but only while there's actually an image on screen to describe - called
// both from toggleInfoOverlay (the preference itself just changed) and
// finishLoad (a fresh image just appeared, which the still-hidden card needs
// to be shown for if the preference was already on). Refreshes the card's
// text before showing it so a toggle-on never briefly displays whatever the
// text last held.
//
// The "Show EXIF data" link is settled here too, rather than in
// updateInfoOverlay: this is the one path that runs when the file on screen
// changes, while updateInfoOverlay also runs on every zoom change - and a
// zoom can't add or remove a file's metadata.
func (v *viewer) syncInfoOverlayVisibility() {
	if v.infoVisible && len(v.state.files) > 0 && v.img.Image != nil {
		v.updateInfoOverlay()
		if v.currentHasEXIF {
			v.exifLink.Show()
		} else {
			v.exifLink.Hide()
		}
		v.infoCard.Show()
	} else {
		v.infoCard.Hide()
	}
}

// updateInfoOverlay refreshes the info card's text from current viewer
// state. A no-op whenever the card isn't supposed to be showing anything -
// toggled off, or no image loaded - so internal/ui/zoom can call it as its
// onChanged callback after every zoom change, unconditionally, without
// checking visibility itself first.
func (v *viewer) updateInfoOverlay() {
	if !v.infoVisible || len(v.state.files) == 0 || v.img.Image == nil {
		return
	}

	w, h := v.displayedDimensions()
	name := v.state.files[v.state.index].Name()
	if n := len(v.state.files); n > 1 {
		name = fmt.Sprintf("%s  (%d/%d)", name, v.state.index+1, n)
	}

	lines := []string{
		name,
		fmt.Sprintf("%d x %d", w, h),
		formatFileSize(v.currentFileSize),
		fmt.Sprintf(lang.L("Zoom: %d%%"), v.zoom.Percent()),
	}
	v.infoText.SetText(strings.Join(lines, "\n"))
}

// displayedDimensions is the pixel size updateInfoOverlay reports. For
// every raster format it's the raster's own bounds, same as before. For a
// vector it is deliberately not that: rasterizeVector replaces v.img.Image
// with a denser raster on every landed re-render, so reading its bounds
// directly would make the reported dimensions climb as the user zooms in -
// an internal implementation detail (how sharp the current raster happens
// to be) leaking into a field that is supposed to answer "how big is this
// image". v.vectorLogical is what the window and the title are already
// built on, so it's what this reports too. A local copy has its axes
// swapped on a 90/270 rotation, exactly as applyRotationLayout's does for
// zoom - v.vectorLogical itself is never mutated, since the re-render
// target in vector.go is built from it in unrotated space.
func (v *viewer) displayedDimensions() (w, h int) {
	if v.vector == nil {
		b := v.img.Image.Bounds()
		return b.Dx(), b.Dy()
	}

	lw, lh := v.vectorLogical.Width, v.vectorLogical.Height
	if v.rotation%2 != 0 {
		lw, lh = lh, lw
	}

	return int(lw + 0.5), int(lh + 0.5)
}

// formatFileSize renders n bytes as a short human-readable size (e.g.
// "2.3 MB"), matching the binary (1024-based) units most OS file browsers
// use for a single file's size, rather than SI (1000-based) ones.
func formatFileSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
