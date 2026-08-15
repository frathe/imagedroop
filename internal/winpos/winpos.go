// Package winpos reads and writes the on-screen position of a Fyne desktop
// window. fyne.Window can only request a position (driver/desktop.Window's
// RequestPosition is write-only) and fires no "window moved" event, so
// there is no supported way to find out where a window currently sits -
// which main.go needs in order to persist a manually-dragged position
// across launches (see internal/preferences).
//
// Get works around that by reaching past the public API into the raw
// platform window handle Fyne's own glfw driver hands out via
// driver.NativeWindow.RunNative, and asking the OS directly: ClientToScreen
// on Windows, XTranslateCoordinates on X11, and an NSWindow frame read
// (converted out of Cocoa's bottom-left-origin coordinate space) on macOS -
// see the platform-specific files. Each mirrors exactly what Fyne's glfw
// driver itself does internally for its own xpos/ypos bookkeeping, so a
// value round-tripped through Get then Set lands back where it started.
//
// Backends with no native handle at all - the fyne test driver, mobile,
// wasm - or with no reliable position query - Wayland, which
// RequestPosition itself already documents as "may be ignored" - make Get
// report ok=false. Callers should treat that as "nothing to save", not an
// error.
package winpos

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
)

// Get returns win's current on-screen position and whether it could be read
// at all.
func Get(win fyne.Window) (x, y int, ok bool) {
	native, isNative := win.(driver.NativeWindow)
	if !isNative {
		return 0, 0, false
	}
	native.RunNative(func(ctx any) {
		x, y, ok = platformPosition(ctx)
	})
	return x, y, ok
}

// Set moves win to (x, y) - the same coordinates Get returns, so a value
// read back by Get can be handed straight to Set on a later launch. A no-op
// on backends that don't support desktop.Window at all, or that
// RequestPosition itself documents as ignoring the request.
func Set(win fyne.Window, x, y int) {
	if dw, isDesktop := win.(desktop.Window); isDesktop {
		dw.RequestPosition(x, y)
	}
}

// Maximize grows win to fill the screen's available work area - the same
// end state as the OS's own maximize button or title-bar double-click, with
// window chrome and the dock/taskbar left alone. That's deliberately not
// SetFullScreen, which Fyne does support natively: full-screen drops the
// chrome and covers everything, which is picture-frame mode's look (see
// slideshow.Controller), not what a merely-roomier window needs. Since
// Fyne exposes no size-changing verb beyond Resize and SetFullScreen, this
// reaches past it into the native window the same way Get does - see the
// platform-specific files. A no-op wherever there's no native handle to
// reach (the fyne test driver, Wayland, mobile, wasm).
func Maximize(win fyne.Window) {
	native, isNative := win.(driver.NativeWindow)
	if !isNative {
		return
	}
	native.RunNative(func(ctx any) {
		platformMaximize(ctx)
	})
}

// Unmaximize undoes Maximize, the same end state as clicking the OS's own
// restore button (or double-clicking a maximized title bar) - chrome and
// the dock/taskbar left alone, same as Maximize. A plain Resize alone can't
// do this on its own: on Linux and Windows, the state Maximize sets is
// tracked by the window manager/OS independently of window geometry, so a
// Resize call made while it's still set is silently ignored, or only
// changes the size the OS remembers for after an eventual un-maximize -
// see the platform-specific files, and callers should follow this with a
// Set/Restore, since the OS's own un-maximize placement rarely lands back
// where the window was before Maximize grew it. A no-op wherever there's
// no native handle to reach, same as Maximize.
func Unmaximize(win fyne.Window) {
	native, isNative := win.(driver.NativeWindow)
	if !isNative {
		return
	}
	native.RunNative(func(ctx any) {
		platformUnmaximize(ctx)
	})
}
