package winpos

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestTracker_ZeroValueHasNothingRecorded(t *testing.T) {
	var tr Tracker

	if _, _, ok := tr.Get(); ok {
		t.Error("a zero Tracker should report ok=false until something is recorded")
	}
}

func TestTracker_StoreThenGet(t *testing.T) {
	var tr Tracker

	tr.Store(120, 340)

	x, y, ok := tr.Get()
	if !ok {
		t.Fatal("Get should report ok=true after Store")
	}
	if x != 120 || y != 340 {
		t.Errorf("Get = (%d, %d), want (120, 340)", x, y)
	}
}

// A negative coordinate is ordinary on a multi-monitor desktop (a screen
// left of or above the primary one), so it must survive the int32 the
// Tracker stores it in rather than being clamped or dropped.
func TestTracker_StoreKeepsNegativeCoordinates(t *testing.T) {
	var tr Tracker

	tr.Store(-1440, -200)

	x, y, _ := tr.Get()
	if x != -1440 || y != -200 {
		t.Errorf("Get = (%d, %d), want (-1440, -200)", x, y)
	}
}

// The fyne test driver's windows are not driver.NativeWindow, so the
// reading fails - which must leave whatever was already recorded alone,
// since a stale good position beats no position at all.
func TestTracker_FailedCaptureKeepsThePreviousReading(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	var tr Tracker
	tr.Store(50, 60)

	if tr.Capture(win) {
		t.Error("Capture on a non-native test window should report false")
	}

	x, y, ok := tr.Get()
	if !ok || x != 50 || y != 60 {
		t.Errorf("Get = (%d, %d, %v), want the previously stored (50, 60, true)", x, y, ok)
	}
}

func TestTracker_RestoreWithoutARecordingIsANoop(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	var tr Tracker

	tr.Restore(win) // must not panic
}
