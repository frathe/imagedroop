package winpos

import (
	"testing"
	"time"

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

// PollAt is the general form Poll is built on, and inherits the same
// degradation: no native handle means no goroutine and no callback, rather
// than a ticker spinning on a read that can only ever fail.

func TestPollAt_NonNativeWindowNeverCallsBack(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	called := make(chan struct{}, 1)
	stop := PollAt(win, time.Millisecond, nil, func(x, y int) {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	if stop == nil {
		t.Fatal("PollAt should never return a nil stop func")
	}
	defer stop()

	select {
	case <-called:
		t.Error("PollAt called back for a window with no native handle to read a position from")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPollAt_StopIsSafeToCallOnANonNativeWindow(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	PollAt(win, time.Millisecond, func() bool { return true }, func(int, int) {})()
}
