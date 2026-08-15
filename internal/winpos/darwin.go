//go:build darwin

package winpos

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#include <stdint.h>

// windowTopLeft reads nsWindowPtr's content-area top-left corner and
// converts it out of Cocoa's bottom-left-origin coordinate space into the
// top-left-origin, y-down space GLFW (and so Fyne's RequestPosition) uses -
// the same conversion GLFW's own cocoa_window.m applies internally, so a
// value read here and later handed to Set lands back in the same place.
// The handle crosses the cgo boundary as a uintptr_t, not a void *, so the
// Go side never converts the foreign handle through unsafe.Pointer (which
// go vet's unsafeptr check would flag as a possible misuse).
static void windowTopLeft(uintptr_t nsWindowPtr, int *x, int *y) {
	NSWindow *window = (__bridge NSWindow *)(void *)nsWindowPtr;
	NSRect content = [window contentRectForFrameRect:window.frame];
	CGFloat screenHeight = CGDisplayBounds(CGMainDisplayID()).size.height;
	*x = (int)content.origin.x;
	*y = (int)(screenHeight - (content.origin.y + content.size.height));
}

// zoomIfNotZoomed maximizes nsWindowPtr the same way a user clicking its
// green zoom button would - AppKit's own toggle between the window's
// standard (screen-filling) frame and whatever frame the user last left it
// at. Only acts when it isn't already zoomed, so a second call - reopening
// the grid without ever having manually unmaximized - can't toggle the
// window back down the way a literal second click of the button would.
static void zoomIfNotZoomed(uintptr_t nsWindowPtr) {
	NSWindow *window = (__bridge NSWindow *)(void *)nsWindowPtr;
	if (!window.isZoomed) {
		[window zoom:nil];
	}
}

// unzoomIfZoomed is zoomIfNotZoomed's inverse: AppKit's zoom toggles
// between the standard and user frame, so undoing one is the same -zoom:
// call again, guarded the opposite way so it only fires when the window is
// actually zoomed.
static void unzoomIfZoomed(uintptr_t nsWindowPtr) {
	NSWindow *window = (__bridge NSWindow *)(void *)nsWindowPtr;
	if (window.isZoomed) {
		[window zoom:nil];
	}
}
*/
import "C"

import (
	"fyne.io/fyne/v2/driver"
)

func platformPosition(ctx any) (x, y int, ok bool) {
	mac, isMac := ctx.(driver.MacWindowContext)
	if !isMac || mac.NSWindow == 0 {
		return 0, 0, false
	}

	var cx, cy C.int
	C.windowTopLeft(C.uintptr_t(mac.NSWindow), &cx, &cy)
	return int(cx), int(cy), true
}

func platformMaximize(ctx any) {
	mac, isMac := ctx.(driver.MacWindowContext)
	if !isMac || mac.NSWindow == 0 {
		return
	}

	C.zoomIfNotZoomed(C.uintptr_t(mac.NSWindow))
}

func platformUnmaximize(ctx any) {
	mac, isMac := ctx.(driver.MacWindowContext)
	if !isMac || mac.NSWindow == 0 {
		return
	}

	C.unzoomIfZoomed(C.uintptr_t(mac.NSWindow))
}
