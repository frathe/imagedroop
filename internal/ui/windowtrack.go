// The window-geometry trackers: how the app knows the size and position
// its window currently has, so both can be persisted across launches (see
// internal/preferences). Fyne exposes neither directly - a size can at
// least be read back off a layout pass, a position needs a poller and a
// trip past the public API into the native handle (internal/winpos).

package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/layout"
)

// windowSizeTracker lays out the window's root content exactly like
// layout.NewStackLayout() while also recording v.win's current full window
// size (Canvas().Size(), which matches what Window.Resize expects) into
// v.windowSize on every layout pass, so the window's last known size is
// available at shutdown for preferences.Save. This is the only way to get
// it: Window has no size getter, and Window.SetOnClosed is a single
// callback slot main() deliberately leaves free for the e2e test suite's
// own close-tracking (see newE2E in e2e_test.go) rather than claiming it
// here too.
//
// Deliberately reads v.win.Canvas().Size() rather than using the size this
// Layout call was handed: that size is the root content's own laid-out
// size, which sits inside the window's padding and so is consistently
// smaller than the window size Resize/Canvas().Size() deal in - recording
// it directly would shrink the persisted geometry by the padding on every
// launch/save cycle.
type windowSizeTracker struct {
	v *viewer
}

func (t windowSizeTracker) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return layout.NewStackLayout().MinSize(objects)
}

func (t windowSizeTracker) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	t.v.windowSize = t.v.win.Canvas().Size()
	layout.NewStackLayout().Layout(objects, size)
}

// windowPosPollInterval is how often startWindowPosPolling samples the
// window's on-screen position while picture-frame mode is off.
const windowPosPollInterval = 1 * time.Second

// startWindowPosPolling launches a background goroutine that keeps
// v.winPos current for the lifetime of the app - the position equivalent
// of windowSizeTracker above. It has to be a poller rather than an event
// hook: unlike a resize, a pure window drag-move triggers no layout pass
// at all, and Fyne's Window has neither a position getter nor a "window
// moved" callback in the first place (see internal/winpos, whose
// RunNative-based Get is the only way to ask the OS directly).
//
// win must satisfy driver.NativeWindow for winpos.Get to ever succeed;
// checked once up front so a window that can't - the fyne test driver's
// windows, which is what every test in this package gets via
// buildViewer/newTestViewer - never gets a poller goroutine running behind
// it at all.
//
// Skips a reading while the slideshow is active: picture-frame mode
// full-screens the window, and a full-screen reading is not the
// manually-placed position this preference is for - it would clobber the
// value the slideshow captured on the way in for its own exit to restore
// (see internal/ui/slideshow).
//
// Each reading is hopped onto the main goroutine via fyne.DoAndWait, the
// same pattern internal/filepicker/darwin.go uses for NSOpenPanel: darwin's
// winpos.Get reaches into NSWindow.frame through cgo, and RunNative
// (window_darwin.go) runs that callback synchronously on whatever goroutine
// called it rather than marshaling to the main thread itself, so without
// this hop every tick reads AppKit state directly from this background
// goroutine - a plain violation of AppKit's main-thread-only rule. It's
// harmless in isolation, but contends with the main thread's own AppKit/GL
// work closely enough to stall it badly under load - e.g. grid.go uploading
// a screenful of thumbnail textures - which is what turned "instant" grid
// population into a multi-minute one. Safe to call from here specifically
// because this goroutine is never the UI goroutine (DoAndWait would
// deadlock if it were) - same guarantee chooseFilesDarwin's own doc comment
// spells out for its call site.
//
// The returned func stops the poller goroutine; Run's SetOnStopped calls it
// just before the final preferences save, so at shutdown the goroutine
// isn't left blocked inside fyne.DoAndWait against an event loop that's
// winding down (the tracker keeps its last reading, so the save still has
// a value). A no-op func, never nil, when no poller started.
func startWindowPosPolling(v *viewer, win fyne.Window) (stop func()) {
	if _, ok := win.(driver.NativeWindow); !ok {
		return func() {}
	}

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(windowPosPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-done:
				return
			}
			if v.slides.Active() {
				continue
			}
			fyne.DoAndWait(func() {
				v.winPos.Capture(win)
			})
		}
	}()

	return func() { close(done) }
}
