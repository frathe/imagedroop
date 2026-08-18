// Package ui is the application itself: the viewer, its widgets, and every
// feature wired onto it. The module root's package main is only the entry
// point - it builds the fyne.App, loads translations, and calls Run below.
package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/session"
)

const (
	appTitle = "PicFetch"

	// size of the empty drop zone
	startW = 520.0
	startH = 340.0

	// height of the "loading next image" bar pinned to the top edge
	loadingBarHeight = 5.0
)

// Run builds the viewer's window and hands control to Fyne's event loop,
// which it does not return from until the window closes. initial is the
// file set to open on startup (command-line arguments, resolved to URIs by
// the caller); empty for a plain launch.
func Run(application fyne.App, initial []fyne.URI) {
	view, window := buildViewer(application)
	view.favorites.SetDir(favstore.DefaultDir())

	// Deferred to SetOnStarted rather than called right away: it ends up
	// calling handleDrop, which touches widgets directly (no fyne.Do) the
	// same way SetOnDropped's callback does, so it needs the event loop
	// already running rather than firing before ShowAndRun below starts it.
	if len(initial) > 0 {
		application.Lifecycle().SetOnStarted(func() {
			view.handleDrop(initial)
		})
	}

	// Wired via SetOnStopped, not run after ShowAndRun returns: Fyne's own
	// app.Preferences() schedules its on-disk flush through a debounced
	// change listener (app.newPreferences in fyne itself) that, once
	// tripped, defers the actual write to a goroutine gated on fyne.DoAndWait
	// - which needs the driver's event loop still alive to ever run. Calling
	// preferences.Save after ShowAndRun returns loses that race every time:
	// the loop has already wound down by then, so the debounced write for
	// everything but the very first preference key never lands, and the
	// process exits before it could anyway. Fyne calls its own equivalent
	// save (SetOnStoppedHookExecuted, app.go) immediately *after* whatever
	// SetOnStopped callback is registered here finishes (see
	// (*Lifecycle).OnStopped) - and Run() blocks on WaitForEvents until both
	// have run - so writing the preferences from here piggybacks on that
	// same guaranteed-synchronous flush instead of racing it.
	application.Lifecycle().SetOnStopped(func() {
		// Stopped first, all three of them: each poller hops through
		// fyne.DoAndWait on every tick, and the event loop they need is
		// about to wind down. The secondary windows only have one running
		// if they're still open right now, and StopTracking says so itself.
		view.stopWinPosPoll()
		view.settings.StopTracking()
		view.exif.StopTracking()
		close(view.vectorStop)

		session.Save(application, view.unsortedFiles)
		preferences.Save(application, view.currentPreferences())
	})

	window.ShowAndRun()
}

// currentPreferences is everything worth remembering about this run, ready
// for preferences.Save. Split out of the SetOnStopped callback above purely
// so it can be read back in a test - the callback itself only ever runs
// inside a live Fyne event loop.
//
// view.windowSize is kept current by windowSizeTracker (windowtrack.go) on
// every layout, so it already reflects the window's last size by the time
// the app stops. view.winPos is kept current the same way by
// startWindowPosPolling's background poller, plus the slideshow's own
// capture/restore around full-screen (internal/ui/slideshow). The two
// secondary windows track their own geometry the same way and hand it over
// whole (see widgets.Singleton), including for a window the user closed
// again long ago.
func (v *viewer) currentPreferences() preferences.State {
	posX, posY, posSet := v.winPos.Get()

	return preferences.State{
		SortMode:          v.sortMode.PrefValue(),
		MergeMode:         v.mergeMode,
		SlideInterval:     v.slides.Interval(),
		SlideShuffle:      v.slides.Shuffle(),
		MaxScanFiles:      v.maxScan,
		MaxWindowWidth:    v.maxWinW,
		MaxWindowHeight:   v.maxWinH,
		MaxImageCacheMB:   v.imgCacheMB,
		MaxThumbCacheMB:   v.thumbCacheMB,
		MaxFileSizeMB:     v.maxFileMB,
		WindowSize:        v.windowSize,
		WindowPosX:        posX,
		WindowPosY:        posY,
		WindowPositionSet: posSet,
		SettingsWindow:    prefGeometry(v.settings.Geometry()),
		ExifWindow:        prefGeometry(v.exif.Geometry()),
	}
}
