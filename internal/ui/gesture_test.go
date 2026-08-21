package ui

import (
	"math"
	"testing"
)

// spiralDragPath synthesises the window positions a user would produce by
// swirling the window inward through two turns. The geometry itself is
// internal/wingesture's business and tested exhaustively there, including
// against positions recorded from real drags; what these tests cover is
// this package's wiring of it.
func spiralDragPath(clockwise bool) [][2]int {
	const n = 40
	out := make([][2]int, 0, n)
	for i := range n {
		f := float64(i) / float64(n-1)
		theta := f * 2 * 2 * math.Pi
		if !clockwise {
			theta = -theta
		}
		r := 120 - 90*f
		out = append(out, [2]int{
			int(math.Round(600 + r*math.Cos(theta))),
			int(math.Round(400 + r*math.Sin(theta))),
		})
	}
	return out
}

func TestRecordWindowPosition_ClockwiseSpiralDragFiresTheGesture(t *testing.T) {
	v := newTestViewer(t)

	var fired []bool
	v.spiralGesture = func(clockwise bool) { fired = append(fired, clockwise) }

	for _, p := range spiralDragPath(true) {
		v.recordWindowPosition(p[0], p[1])
	}

	if len(fired) != 1 {
		t.Fatalf("gesture fired %d times, want exactly 1", len(fired))
	}
	if !fired[0] {
		t.Error("clockwise = false; a clockwise drag should open a clockwise spiral")
	}
}

func TestRecordWindowPosition_CounterClockwiseSpiralDragFiresTheOtherWay(t *testing.T) {
	v := newTestViewer(t)

	var fired []bool
	v.spiralGesture = func(clockwise bool) { fired = append(fired, clockwise) }

	for _, p := range spiralDragPath(false) {
		v.recordWindowPosition(p[0], p[1])
	}

	if len(fired) != 1 {
		t.Fatalf("gesture fired %d times, want exactly 1", len(fired))
	}
	if fired[0] {
		t.Error("clockwise = true; a counter-clockwise drag should open a counter-clockwise spiral")
	}
}

// Ordinary window repositioning must stay ordinary. This is the check that
// keeps the easter egg from ambushing someone who is just tidying their
// desktop.
func TestRecordWindowPosition_StraightDragDoesNotFireTheGesture(t *testing.T) {
	v := newTestViewer(t)

	fired := false
	v.spiralGesture = func(bool) { fired = true }

	for i := range 40 {
		v.recordWindowPosition(200+i*20, 300+i*5)
	}

	if fired {
		t.Error("dragging the window across the desktop in a straight line opened the easter egg")
	}
}

// The gesture rides along on the poller that already existed to remember
// where the user left the window; that original job must survive the
// addition.
func TestRecordWindowPosition_StillFeedsThePositionTracker(t *testing.T) {
	v := newTestViewer(t)
	v.spiralGesture = func(bool) {}

	v.recordWindowPosition(1234, -56)

	x, y, ok := v.winPos.Get()
	if !ok {
		t.Fatal("winPos has no reading after recordWindowPosition")
	}
	if x != 1234 || y != -56 {
		t.Errorf("winPos = (%d, %d), want (1234, -56)", x, y)
	}
}
