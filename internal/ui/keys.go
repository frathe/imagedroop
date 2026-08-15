// The keyboard dispatcher: every unmodified key press lands here.

package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// defaultKeyModifiers reports the keyboard modifiers currently held, which
// a fyne.KeyEvent doesn't carry: desktop.Driver.CurrentKeyModifiers is kept
// in sync by the glfw driver on every key event regardless of which widget
// has focus, unlike a window-level SetOnKeyDown/SetOnKeyUp hook, which Fyne
// only calls when nothing focusable currently has focus. Both consumers -
// Shift+R below and internal/ui/zoom's Shift+scroll pan - reach it through
// the viewer's keyModifiers field rather than calling it directly, so tests
// can stub it per-viewer: Fyne's test driver doesn't implement
// desktop.Driver at all, so the type assertion here is always false under
// test.
func defaultKeyModifiers() fyne.KeyModifier {
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		return d.CurrentKeyModifiers()
	}

	return 0
}

// handleKeyEvent dispatches a single key press: F1 opens the manual,
// Escape cancels a scan in progress, resets back to the initial state, or
// closes the window once there's nothing left to reset/cancel, the
// arrow/Home/End keys walk through the dropped files, S cycles the sort
// order (see internal/filesort), M toggles merge mode, and 0/1/+/- control
// zoom (see internal/ui/zoom). Wired to the window's canvas via
// SetOnTypedKey in buildViewer (build.go), so tests can drive the exact
// same dispatch instead of reimplementing it.
func (v *viewer) handleKeyEvent(ev *fyne.KeyEvent) {
	// The delete confirmation (Shift+Delete, see internal/ui/deletion) takes
	// over the keyboard entirely while it's up: every other key here -
	// navigation, zoom, S/M/P/I, even Escape's own usual meaning - would
	// either be confusing (what does "next image" mean with a delete
	// pending?) or actively dangerous (Escape closing the window instead of
	// just dismissing the prompt) if it fell through to the switch below.
	if v.deletion.Visible() {
		v.deletion.HandleKey(ev)
		return
	}

	// The grid overview (G key, see internal/ui/grid) takes over the
	// keyboard the same way the delete confirmation does above: arrow keys
	// move the highlighted cell, Return opens whichever cell is
	// highlighted, and Escape/G back out without picking anything. Every
	// other key does nothing.
	if v.grid.Visible() {
		v.grid.HandleKey(ev)
		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		// Handled before the navigation guard below so Escape still works
		// while an image is loading or scanning. While picture-frame mode
		// is on, Escape leaves it (like any other full-screen app) instead
		// of resetting the session - press it again afterwards for that.
		// A scan in progress takes priority over both the close and reset
		// branches below: len(v.files) == 0 is exactly the state a
		// first-ever drop's scan runs in, so without this check Escape
		// would close the window out from under a scan the user meant to
		// cancel instead.
		if v.slides.Active() {
			v.slides.Exit()
		} else if v.scanning {
			v.cancelScan()
		} else if len(v.files) == 0 {
			v.win.Close()
		} else {
			v.reset()
		}

		return
	case fyne.KeyF1:
		// Handled before the navigation guard below so help stays
		// reachable while an image is still loading.
		v.help.ShowManual()

		return
	case fyne.KeyM:
		// Handled before the navigation guard below so the mode can be set
		// before the first drop, or flipped while a scan/decode is still
		// running, without being ignored.
		v.toggleMergeMode()

		return
	case fyne.KeyP:
		// Handled before the navigation guard below so picture-frame mode
		// can be toggled off even while an image is still loading.
		v.togglePictureFrameMode()

		return
	case fyne.KeyG:
		// Handled before the navigation guard below, same as P, so the
		// grid can be opened with only one file loaded (nothing to
		// navigate to yet) or while a decode is still in flight.
		//
		// The two full-window modes don't compose (P already claims
		// Escape to leave), so the guard lives here in the dispatcher
		// rather than inside either package: neither needs to know the
		// other exists.
		if !v.slides.Active() {
			v.grid.Toggle()
		}

		return
	case fyne.KeyI:
		// Handled before the navigation guard below, same as M/P, so it
		// works before the first image ever loads too (the card just stays
		// hidden until one does - see syncInfoOverlayVisibility).
		v.toggleInfoOverlay()

		return
	case fyne.KeyE:
		// Handled before the navigation guard below, same as I - the EXIF panel
		// itself no-ops with nothing loaded yet.
		v.exif.Show()

		return
	case fyne.Key0:
		// Zoom shortcuts are handled before the navigation guard below,
		// same as M/P, so they work with only one file loaded (nothing to
		// navigate to) or mid-decode. 0 also clears any view rotation, same
		// as it clears a manual zoom level.
		v.resetRotation()
		v.zoom.FitToWindow()

		return
	case fyne.Key1:
		v.zoom.ActualSize()

		return
	case fyne.KeyPlus, fyne.KeyEqual:
		v.zoom.In()

		return
	case fyne.KeyMinus:
		v.zoom.Out()

		return
	case fyne.KeyR:
		// Handled before the navigation guard below, same as the zoom keys,
		// so rotation works with only one file loaded or mid-decode.
		// keyModifiers is the same Shift check the zoom view's
		// Shift+scroll-to-pan uses - a KeyEvent carries no modifier state of
		// its own (see defaultKeyModifiers above).
		if v.keyModifiers()&fyne.KeyModifierShift != 0 {
			v.rotateBy(-1)
		} else {
			v.rotateBy(1)
		}

		return
	}

	// While picture-frame mode is on, Up/Down tune the auto-advance
	// interval instead of navigating - navigation still works via
	// Left/Right/Home/End below. Handled before the navigation guard so the
	// interval can be tuned even with only one file loaded or while an
	// image is loading.
	if v.slides.Active() {
		switch ev.Name {
		case fyne.KeyUp:
			v.slides.AdjustInterval(time.Second)
			return
		case fyne.KeyDown:
			v.slides.AdjustInterval(-time.Second)
			return
		}
	}

	// Ignore repeat events fired while the previous image is still
	// decoding/rendering, instead of piling up decodes for images the
	// user has already navigated past.
	if len(v.files) < 2 || v.loading.Load() {
		return
	}

	switch ev.Name {
	case fyne.KeyRight, fyne.KeyDown:
		v.ShowImage(v.index + 1)
	case fyne.KeyLeft, fyne.KeyUp:
		v.ShowImage(v.index - 1)
	case fyne.KeyHome:
		v.ShowImage(0)
	case fyne.KeyEnd:
		v.ShowImage(len(v.files) - 1)
	case fyne.KeyS:
		v.toggleSort()
	default:
		return
	}

	// A manual navigation restarts the auto-advance countdown, so it always
	// gets the full interval starting from what you just navigated to
	// rather than picking up wherever the countdown for the old image left
	// off.
	if v.slides.Active() {
		v.slides.Kick()
	}
}
