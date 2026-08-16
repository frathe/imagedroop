//go:build darwin

package wallpaper

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#include <stdlib.h>

// setWallpaper points every screen's desktop picture at path via
// NSWorkspace - the same call the Wallpaper settings pane ends up making.
// Unlike an AppleScript "tell application \"System Events\" to set picture
// of every desktop", this calls a system framework directly rather than
// scripting another app, so it never triggers the one-time Automation
// permission prompt Apple Events would.
//
// setDesktopImageURL:forScreen:options:error: is synchronous and safe to
// call off the main thread, so this needs neither the semaphore
// internal/trash's asynchronous recycleURLs: does nor a fyne.DoAndWait hop
// to the main thread the way internal/filepicker's NSOpenPanel does - there
// is no modal panel and no completion handler involved.
//
// Each screen's existing options (the user's own Fill Screen / Fit to
// Screen / Stretch choice, and their background color) are read back and
// passed through, so setting a wallpaper from this app changes the picture
// and nothing else. Returns NULL on success, or a malloc'd error message.
static char *setWallpaper(const char *path) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		NSArray<NSScreen *> *screens = [NSScreen screens];
		if (screens.count == 0) {
			return strdup("no screen is attached");
		}

		for (NSScreen *screen in screens) {
			NSDictionary *options = [[NSWorkspace sharedWorkspace] desktopImageOptionsForScreen:screen];
			NSError *error = nil;
			if (![[NSWorkspace sharedWorkspace] setDesktopImageURL:url
			                                            forScreen:screen
			                                              options:options ? options : @{}
			                                                error:&error]) {
				return strdup(error.localizedDescription.UTF8String);
			}
		}
		return NULL;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// setDarwin makes path the desktop picture on every attached screen via
// AppKit's NSWorkspace, in-process - see setWallpaper above for why not an
// osascript shell-out.
func setDarwin(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	cErr := C.setWallpaper(cPath)
	if cErr == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cErr))
	return errors.New(C.GoString(cErr))
}
