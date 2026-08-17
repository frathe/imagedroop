package winpos

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// The fyne test driver's windows are not driver.NativeWindow, so there is
// no handle for a reading to come from - Poll must degrade to "no goroutine
// at all" there rather than spinning a ticker that can only ever fail. That
// is what keeps every headless test in internal/ui (and in this package's
// own consumers) from carrying a poller goroutine behind it.

func TestPoll_NonNativeWindowRecordsNothing(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	var tr Tracker
	stop := Poll(win, &tr, nil)
	if stop == nil {
		t.Fatal("Poll should never return a nil stop func")
	}
	stop()

	if _, _, ok := tr.Get(); ok {
		t.Error("Poll recorded a position for a window with no native handle to read one from")
	}
}

func TestPoll_StopIsSafeToCallOnANonNativeWindow(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	var tr Tracker
	Poll(win, &tr, func() bool { return true })()
}
