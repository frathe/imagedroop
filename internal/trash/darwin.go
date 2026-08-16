//go:build darwin

package trash

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>

// recycleFile moves path to the Trash via NSWorkspace - the same mechanism
// Finder's own "Move to Trash" uses. Unlike an AppleScript "tell
// application \"Finder\" to delete", this calls a system framework
// directly rather than scripting another app, so it never triggers the
// one-time Automation permission prompt Apple Events would. recycleURLs:
// completionHandler: is asynchronous and documented safe to call from any
// thread; the semaphore below only gives this a synchronous signature to
// match os.Remove - no fyne.DoAndWait hop to the main thread needed, unlike
// filepicker's NSOpenPanel, since there is no modal panel involved. Returns
// NULL on success, or a malloc'd error message string.
static char *recycleFile(const char *path) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		__block char *errMsg = NULL;
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);

		[[NSWorkspace sharedWorkspace] recycleURLs:@[url] completionHandler:^(NSDictionary<NSURL *, NSURL *> *newURLs, NSError *error) {
			if (error) {
				errMsg = strdup(error.localizedDescription.UTF8String);
			}
			dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
		return errMsg;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// moveDarwin moves path to the Trash via AppKit's NSWorkspace, in-process -
// see recycleFile above for why not an osascript shell-out.
func moveDarwin(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	cErr := C.recycleFile(cPath)
	if cErr == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cErr))
	return errors.New(C.GoString(cErr))
}
