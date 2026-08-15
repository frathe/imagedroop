package winpos

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// The fyne test driver's windows implement neither driver.NativeWindow nor
// desktop.Window, mirroring every headless test in the main package (see
// startWindowPosPolling's own doc comment in main.go) - Get and Set must
// degrade to a harmless no-op there rather than panicking on a failed type
// assertion.

func TestGet_NonNativeWindowReportsNotOK(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	if _, _, ok := Get(win); ok {
		t.Error("Get on a non-native test window should report ok=false")
	}
}

func TestSet_NonDesktopWindowDoesNotPanic(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	Set(win, 100, 200)
}
