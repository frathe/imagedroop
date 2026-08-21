// The window-geometry trackers: how the app knows the size and position
// its window currently has, so both can be persisted across launches (see
// internal/preferences). Fyne exposes neither directly - a size can at
// least be read back off a layout pass, a position needs a poller and a
// trip past the public API into the native handle (internal/winpos).
//
// Both mechanisms themselves are shared with the secondary windows that
// remember their own geometry (widgets.Singleton, see internal/ui/exifwin
// and internal/ui/settingswin), so both live outside this package now -
// what's left here is the app's own binding of each to the main window.

package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/winpos"
)

// windowSizeTracker keeps v.windowSize current with the window's own size,
// so the last known size is available at shutdown for preferences.Save -
// the app's binding of widgets.NewSizeTracker, which owns the layout itself
// and the padding trap behind reading Canvas().Size() rather than the size
// the layout pass is handed.
//
// A layout is the only way to get the size back at all: Window has no size
// getter, and Window.SetOnClosed is a single callback slot main()
// deliberately leaves free for the e2e test suite's own close-tracking (see
// newE2E in e2e_test.go) rather than claiming it here too.
func windowSizeTracker(v *viewer, win fyne.Window) fyne.Layout {
	return widgets.NewSizeTracker(win, &v.windowSize)
}

// noPollerStop marks the construction-time state before Run starts runtime
// position polling.
func noPollerStop() {}

// startWindowPosPolling keeps v.winPos current for the lifetime of the app -
// the position equivalent of windowSizeTracker above, and the app's binding
// of winpos.PollAt, which owns the loop itself and every reason it has to be
// a poller at all. What is this package's own is the skip rule: no reading
// while the slideshow is active, since picture-frame mode full-screens the
// window and a full-screen reading is not the manually-placed position this
// preference is for - it would clobber the value the slideshow captured on
// the way in for its own exit to restore (see internal/ui/slideshow).
//
// It samples at winpos.GestureInterval rather than the leisurely
// winpos.PollInterval a remembered position alone would need, because the
// readings feed the spiral drag gesture too (gesture.go), which cares about
// the path the window took and not just where it stopped. One poller serves
// both: v.recordWindowPosition is where the single reading fans out.
//
// The returned func stops the poller goroutine; Run's SetOnStopped calls it
// just before the final preferences save, so at shutdown the goroutine
// isn't left blocked inside fyne.DoAndWait against an event loop that's
// winding down (the tracker keeps its last reading, so the save still has
// a value). A no-op func, never nil, when no poller started.
func startWindowPosPolling(v *viewer, win fyne.Window) (stop func()) {
	if v.slides == nil {
		panic("ui: startWindowPosPolling called before slideshow construction")
	}
	return winpos.PollAt(win, winpos.GestureInterval, v.slides.Active, v.recordWindowPosition)
}

// widgetGeometry and prefGeometry translate one secondary window's geometry
// between the two packages that each own their own copy of those four
// values - the widget that remembers them while the app runs, and the store
// that keeps them between launches. Deliberately a translation rather than
// one shared type: internal/preferences would otherwise have to import a UI
// package, or widgets a persistence one, and this package is already where
// every other preference is translated (filesort.FromPref, the zero-means-
// default caps in startup.go).
func widgetGeometry(g preferences.WindowGeometry) widgets.Geometry {
	return widgets.Geometry{X: g.X, Y: g.Y, PositionSet: g.PositionSet, Size: g.Size}
}

func prefGeometry(g widgets.Geometry) preferences.WindowGeometry {
	return preferences.WindowGeometry{X: g.X, Y: g.Y, PositionSet: g.PositionSet, Size: g.Size}
}
