package ui

import "testing"

// What stays here is startWindowPosPolling's contract with main(): that it
// always hands back a usable stop func, and that it refuses to run before
// the slideshow exists. The poller goroutine itself is unreachable from a
// test - under the fyne test driver the window is never a
// driver.NativeWindow - so that stop func is the whole of the surface this
// file can observe.
//
// windowtrack.go's other half is tested elsewhere on purpose:
// TestWindowSizeTracker_RecordsResizes lives in preferences_wiring_test.go,
// because what it checks is geometry surviving a launch rather than
// anything about the poller.

// TestStartWindowPosPolling_TestDriverGetsNoopStop pins the stop-func
// contract: under the fyne test driver the window is never a
// driver.NativeWindow, so no poller goroutine starts - but the returned
// stop func must still be non-nil and safe to call, since main()'s
// SetOnStopped calls it unconditionally. (The goroutine path itself can't
// run under the test driver at all - see startWindowPosPolling's comment.)
func TestStartWindowPosPolling_TestDriverGetsNoopStop(t *testing.T) {
	v := newTestViewer(t)

	if v.stopWinPosPoll == nil {
		t.Fatal("buildStartupViewer should initialize stopWinPosPoll")
	}
	v.stopWinPosPoll() // safe before Run replaces it with the live poller's stop

	stop := startWindowPosPolling(v, v.win)
	if stop == nil {
		t.Fatal("startWindowPosPolling should never return a nil stop func")
	}
	stop() // must not panic or block
}

func TestStartWindowPosPolling_PanicsWithoutConstructedSlideshow(t *testing.T) {
	const want = "ui: startWindowPosPolling called before slideshow construction"

	defer func() {
		if got := recover(); got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()

	startWindowPosPolling(&viewer{}, nil)
}
