package wingesture

import (
	"math"
	"testing"
	"time"
)

// sample is one window position at a moment in a drag, relative to the
// start of that drag. Both the synthesised paths below and the recorded
// real ones in realdrag_test.go are expressed in it.
type sample struct {
	ms   int
	x, y int
}

var base = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

// feed replays samples through d and returns the first firing Result, or
// the zero Result if the gesture never fired.
func feed(d *Detector, samples []sample) Result {
	var fired Result
	for _, s := range samples {
		got := d.Add(base.Add(time.Duration(s.ms)*time.Millisecond), s.x, s.y)
		if got.Detected && !fired.Detected {
			fired = got
		}
	}
	return fired
}

// spiralPath synthesises an Archimedean spiral: turns full revolutions
// around (cx, cy) with the radius sweeping linearly from r0 to r1, sampled
// n times at stepMs apart. Screen coordinates are y-down, so a clockwise
// path on screen is one whose angle *increases* - the single convention
// this whole package rests on (see TestClockwiseOnScreenMeansIncreasingAngle).
func spiralPath(cx, cy, r0, r1, turns float64, clockwise bool, n, stepMs int) []sample {
	out := make([]sample, 0, n)
	for i := range n {
		f := float64(i) / float64(n-1)
		theta := f * turns * 2 * math.Pi
		if !clockwise {
			theta = -theta
		}
		r := r0 + (r1-r0)*f
		out = append(out, sample{
			ms: i * stepMs,
			x:  int(math.Round(cx + r*math.Cos(theta))),
			y:  int(math.Round(cy + r*math.Sin(theta))),
		})
	}
	return out
}

func TestClockwiseInwardSpiralFires(t *testing.T) {
	d := New(Config{})

	got := feed(d, spiralPath(600, 400, 120, 30, 2, true, 40, 100))

	if !got.Detected {
		t.Fatal("a two-turn inward clockwise spiral should fire")
	}
	if got.Direction != Clockwise {
		t.Errorf("Direction = %v, want Clockwise", got.Direction)
	}
	if !got.Inward {
		t.Error("Inward = false, want true for a shrinking spiral")
	}
}

func TestCounterClockwiseSpiralReportsCounterClockwise(t *testing.T) {
	d := New(Config{})

	got := feed(d, spiralPath(600, 400, 120, 30, 2, false, 40, 100))

	if !got.Detected {
		t.Fatal("a two-turn inward counter-clockwise spiral should fire")
	}
	if got.Direction != CounterClockwise {
		t.Errorf("Direction = %v, want CounterClockwise", got.Direction)
	}
}

func TestOutwardSpiralReportsNotInward(t *testing.T) {
	d := New(Config{})

	got := feed(d, spiralPath(600, 400, 30, 120, 2, true, 40, 100))

	if !got.Detected {
		t.Fatal("a two-turn outward spiral should fire")
	}
	if got.Inward {
		t.Error("Inward = true, want false for a growing spiral")
	}
}

// The whole direction convention hinges on screen coordinates being y-down
// (internal/winpos returns a top-left origin). A path that a user watching
// the screen would call clockwise - right, then down, then left, then up -
// must report Clockwise. Getting this backwards is the single easiest
// mistake in the package and would be invisible in every other test, since
// they are all built from spiralPath's own convention.
func TestClockwiseOnScreenMeansIncreasingAngle(t *testing.T) {
	d := New(Config{})

	// Two laps of an explicit right -> down -> left -> up square-ish loop,
	// shrinking so it reads as a spiral rather than a circle.
	var path []sample
	ms := 0
	for lap, r := range []float64{120, 100, 80, 60} {
		_ = lap
		for _, p := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			for step := range 3 {
				f := float64(step) / 3
				ang := math.Atan2(p[1], p[0]) + f*math.Pi/2
				path = append(path, sample{
					ms: ms,
					x:  int(math.Round(600 + r*math.Cos(ang))),
					y:  int(math.Round(400 + r*math.Sin(ang))),
				})
				ms += 100
			}
		}
	}

	got := feed(d, path)

	if !got.Detected {
		t.Fatal("the explicit right/down/left/up loop should fire")
	}
	if got.Direction != Clockwise {
		t.Errorf("Direction = %v, want Clockwise: right-then-down is clockwise on a y-down screen", got.Direction)
	}
}

func TestStraightDragDoesNotFire(t *testing.T) {
	d := New(Config{})

	var path []sample
	for i := range 40 {
		path = append(path, sample{ms: i * 100, x: 200 + i*20, y: 300 + i*5})
	}

	if got := feed(d, path); got.Detected {
		t.Errorf("a straight drag across the desktop fired as a spiral: %+v", got)
	}
}

// A circle has the turns and the sign consistency of a spiral and is
// separated from one only by its radius refusing to trend. Without that
// check every idle circular fidget would open the easter egg.
func TestPerfectCircleDoesNotFire(t *testing.T) {
	d := New(Config{})

	if got := feed(d, spiralPath(600, 400, 100, 100, 3, true, 60, 100)); got.Detected {
		t.Errorf("a constant-radius circle fired as a spiral: %+v", got)
	}
}

func TestSingleTurnDoesNotFire(t *testing.T) {
	d := New(Config{})

	if got := feed(d, spiralPath(600, 400, 120, 40, 1, true, 20, 100)); got.Detected {
		t.Errorf("one turn fired, but the default threshold is 1.5: %+v", got)
	}
}

// A spiral has to be drawn in one motion. Two half-gestures either side of
// a pause - the user dragging the window somewhere, thinking, then dragging
// it again - must not add up to one.
func TestIdleGapResetsTheGesture(t *testing.T) {
	d := New(Config{})

	first := spiralPath(600, 400, 120, 70, 1, true, 20, 100)
	second := spiralPath(600, 400, 70, 30, 1, true, 20, 100)
	lastMs := first[len(first)-1].ms
	for i := range second {
		second[i].ms += lastMs + 1500 // well past the 400ms idle gap
	}

	if got := feed(d, append(first, second...)); got.Detected {
		t.Errorf("two one-turn halves either side of a pause fired as one gesture: %+v", got)
	}
}

// The OS only reports a new window position about ten times a second, so a
// faster poller sees the same coordinates repeatedly. Those repeats carry no
// angular information and must not crowd the buffer or count as samples.
func TestRepeatedIdenticalPositionsAreIgnored(t *testing.T) {
	d := New(Config{})

	var padded []sample
	for _, s := range spiralPath(600, 400, 120, 30, 2, true, 40, 100) {
		padded = append(padded, s)
		for rep := 1; rep <= 5; rep++ {
			padded = append(padded, sample{ms: s.ms + rep*16, x: s.x, y: s.y})
		}
	}

	got := feed(d, padded)

	if !got.Detected {
		t.Fatal("a spiral oversampled with duplicate positions should still fire")
	}
	if got.Direction != Clockwise {
		t.Errorf("Direction = %v, want Clockwise", got.Direction)
	}
}

func TestTinyJitterDoesNotFire(t *testing.T) {
	d := New(Config{})

	if got := feed(d, spiralPath(600, 400, 8, 2, 3, true, 40, 100)); got.Detected {
		t.Errorf("an eight-pixel fidget fired as a spiral: %+v", got)
	}
}

// Firing has to be an edge, not a level: the detector clears itself so one
// drawn spiral opens the easter egg exactly once, however much longer the
// user keeps swirling.
func TestFiresOnceThenResets(t *testing.T) {
	d := New(Config{})

	path := spiralPath(600, 400, 140, 20, 4, true, 80, 100)
	fires := 0
	for _, s := range path {
		if d.Add(base.Add(time.Duration(s.ms)*time.Millisecond), s.x, s.y).Detected {
			fires++
		}
	}

	if fires != 1 {
		t.Errorf("fired %d times over one continuous spiral, want exactly 1", fires)
	}
}

func TestResetDiscardsThePartialGesture(t *testing.T) {
	d := New(Config{})

	path := spiralPath(600, 400, 120, 30, 2, true, 40, 100)
	for _, s := range path[:30] {
		d.Add(base.Add(time.Duration(s.ms)*time.Millisecond), s.x, s.y)
	}
	d.Reset()

	fired := false
	for _, s := range path[30:] {
		if d.Add(base.Add(time.Duration(s.ms)*time.Millisecond), s.x, s.y).Detected {
			fired = true
		}
	}

	if fired {
		t.Error("the tail of a spiral fired after Reset discarded its head")
	}
}
