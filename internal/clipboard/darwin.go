//go:build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#include <stdlib.h>

// copyFileURLs puts count file paths onto the general pasteboard as NSURLs -
// the representation Finder's own Copy writes, and the one it reads back on
// Paste. Deliberately not an AppleScript shell-out like copyImageDarwin's:
// AppleScript can express a single "POSIX file" on the clipboard, but has no
// reliable form for a *list* of them, and scripting Finder to do it would
// trigger the same one-time Automation permission prompt internal/trash
// exists to avoid.
//
// writeObjects: is a plain pasteboard write with no UI of its own, so unlike
// filepicker's NSOpenPanel this needs no hop to the main thread - the
// pasteboard object is created and used entirely within this call, never
// shared across threads. Returns NULL on success, or a malloc'd error
// message string.
static char *copyFileURLs(const char **paths, int count) {
	@autoreleasepool {
		NSMutableArray<NSURL *> *urls = [NSMutableArray arrayWithCapacity:count];
		for (int i = 0; i < count; i++) {
			NSString *path = [NSString stringWithUTF8String:paths[i]];
			if (path == nil) {
				return strdup("file name is not valid UTF-8");
			}
			[urls addObject:[NSURL fileURLWithPath:path]];
		}

		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		if (![pb writeObjects:urls]) {
			return strdup("the pasteboard rejected the file references");
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

// copyFilesDarwin puts paths onto the general pasteboard as file references,
// in-process via AppKit - see copyFileURLs above for why not an osascript
// shell-out.
func copyFilesDarwin(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	cPaths := make([]*C.char, len(paths))
	defer func() {
		for _, p := range cPaths {
			C.free(unsafe.Pointer(p))
		}
	}()
	for i, p := range paths {
		cPaths[i] = C.CString(p)
	}

	cErr := C.copyFileURLs((**C.char)(unsafe.Pointer(&cPaths[0])), C.int(len(cPaths)))
	if cErr == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cErr))

	return errors.New(C.GoString(cErr))
}
