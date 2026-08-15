//go:build darwin

package filepicker

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

// runOpenPanel shows an app-modal NSOpenPanel that allows files, folders,
// and multi-select all at once - a combination none of AppleScript's
// Standard Additions pickers offer. Must be called on the main thread (an
// AppKit requirement); chooseFilesDarwin below guarantees that. Returns a
// malloc'd, newline-joined POSIX path list; "" on cancel.
static char *runOpenPanel(const char *message) {
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	panel.message = [NSString stringWithUTF8String:message];
	panel.canChooseFiles = YES;
	panel.canChooseDirectories = YES;
	panel.allowsMultipleSelection = YES;

	if ([panel runModal] != NSModalResponseOK) {
		return strdup("");
	}
	NSMutableArray<NSString *> *paths = [NSMutableArray array];
	for (NSURL *url in panel.URLs) {
		[paths addObject:url.path];
	}
	return strdup([paths componentsJoinedByString:@"\n"].UTF8String);
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

// chooseFilesDarwin runs AppKit's NSOpenPanel in-process rather than
// shelling out the way the Linux/Windows choosers do. In-process is
// load-bearing, not a style choice: this picker went through two
// subprocess generations first - Standard Additions' "choose file or
// folder" (a command that doesn't exist; it parses as a boolean `or` of a
// bare "choose file" and the `folder` property and dies at the first
// "with"), then AppleScriptObjC driving NSOpenPanel inside osascript,
// which did open a panel, but one owned by a background process that macOS
// refuses to make the active app: it appeared *behind* the app window and
// auto-dismissed on the first click, when that click tried and failed to
// activate osascript. A panel owned by this app - a regular, frontmost GUI
// app - has neither problem, and is how every native app shows this
// dialog. AppKit requires the panel on the main thread, which on darwin is
// exactly Fyne's UI thread, hence the fyne.DoAndWait hop; the UI behind
// the panel freezes for the duration, which is what app-modal means. That
// also means this must never be called *from* the UI goroutine (DoAndWait
// would deadlock) - production only reaches it via the viewer's
// openFileDialog background goroutine. A cancel returns empty output and a
// nil error, matching the other platforms' cancel contract.
func chooseFilesDarwin() ([]byte, error) {
	cMsg := C.CString(lang.L("Open images"))
	defer C.free(unsafe.Pointer(cMsg))

	var out []byte
	fyne.DoAndWait(func() {
		cOut := C.runOpenPanel(cMsg)
		defer C.free(unsafe.Pointer(cOut))
		out = []byte(C.GoString(cOut))
	})
	return out, nil
}
