// Package exifwin is the EXIF metadata window: a small panel listing the
// current image's camera settings, opened with the E key or the info
// overlay's "Show EXIF data" link.
//
// It takes no host interface, only a `current` func: everything it needs
// from the app is "which file is on screen, if any", and one accessor is a
// smaller, more honest dependency than an interface with a single method.
package exifwin

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/imagedrop/internal/imaging"
	"github.com/frathe/imagedrop/internal/ui/widgets"
)

const (
	exifW = 420.0
	exifH = 360.0
)

// Window is the EXIF panel. At most one is open at a time (widgets.
// Singleton): a second request raises the existing window rather than
// stacking up duplicates.
type Window struct {
	app fyne.App

	// current reports the file whose metadata should be shown, and whether
	// there is one at all - the app's "currently displayed image", which
	// changes under this window as the user navigates.
	current func() (fyne.URI, bool)

	win widgets.Singleton

	// text is the panel's content label, live only while the window is
	// open (nil otherwise, which is what makes Refresh a no-op then).
	text *widget.Label
}

// New returns the EXIF window for application. current is called on every
// open and refresh to find the file to read.
func New(application fyne.App, current func() (fyne.URI, bool)) *Window {
	return &Window{app: application, current: current}
}

// Show opens the panel, or raises it and syncs it to the current image if
// it's already open. A no-op when nothing is displayed, since there's no
// file to read metadata from.
func (w *Window) Show() {
	if _, ok := w.current(); !ok {
		return
	}

	// Raising an already-open window must first sync it to whatever image
	// is now current; Refresh no-ops while the window isn't open yet (text
	// nil), so the fresh-window path below isn't affected.
	w.Refresh()

	w.win.Show(w.app, lang.L("EXIF Data"), fyne.NewSize(exifW, exifH), func() fyne.CanvasObject {
		w.text = widget.NewLabel("")
		w.text.Wrapping = fyne.TextWrapWord
		w.Refresh()

		return container.NewScroll(container.NewPadded(w.text))
	}, func() {
		w.text = nil
	})
}

// Refresh re-reads the current file's raw bytes and updates the panel from
// them. A no-op while the window isn't open. Called both from Show (opening,
// or raising an already-open window onto whatever image is now current) and
// by the app's finishLoad, so navigating to a different image while the
// window is up keeps it in sync instead of showing a stale file's metadata.
//
// Re-reading from disk here rather than keeping the raw bytes from the
// original decode around is a deliberate trade: the image cache only ever
// holds decoded pixels (see its own size comment), and the EXIF window is an
// on-demand, comparatively rare action - not worth doubling every cached
// entry's memory with raw file bytes it usually never needs.
func (w *Window) Refresh() {
	u, ok := w.current()
	if w.text == nil || !ok {
		return
	}

	data, _, err := imaging.ReadAndProbe(u)
	if err != nil {
		w.text.SetText(lang.L("Could not read this file's metadata."))
		return
	}

	w.text.SetText(formatExifMetadata(imaging.ReadMetadata(data)))
}

// Open reports whether the panel is currently showing.
func (w *Window) Open() bool {
	return w.win.Open()
}

// Window returns the open window, or nil when it's closed - the identity
// callers and tests use to tell "raised the same window" from "opened a
// second one".
func (w *Window) Window() fyne.Window {
	return w.win.Window()
}

// Text returns the panel's content label while it's open, or nil - the
// rendered metadata, for callers and tests that need to read it back.
func (w *Window) Text() *widget.Label {
	return w.text
}

// formatExifMetadata renders m as one display line per field that's
// actually set - a file with only some tags (or, for non-JPEG formats,
// none at all) just shows fewer lines rather than a wall of blanks.
func formatExifMetadata(m imaging.Metadata) string {
	fields := []struct {
		label, value string
	}{
		{lang.L("Camera"), strings.TrimSpace(m.Make + " " + m.Model)},
		{lang.L("Lens"), m.LensModel},
		{lang.L("Exposure"), m.ExposureTime},
		{lang.L("Aperture"), m.FNumber},
		{lang.L("ISO"), m.ISO},
		{lang.L("Focal length"), m.FocalLength},
		{lang.L("Date taken"), m.DateTaken},
	}

	var lines []string
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", f.label, f.value))
	}

	if len(lines) == 0 {
		return lang.L("No EXIF metadata found in this file.")
	}

	return strings.Join(lines, "\n")
}
