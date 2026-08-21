// Package wingesture recognises a spiral drawn by dragging a window around
// the desktop, and which way round it was drawn.
//
// It is deliberately pure: no Fyne, no cgo, no clock of its own. Callers
// feed it timestamped window positions - internal/winpos is where those
// come from - and it answers with a Result. Everything platform-shaped
// stays on the other side of that boundary, which is what makes the
// recognition itself testable at all (see realdrag_test.go, which replays
// positions recorded from real drags).
//
// # Coordinates
//
// Positions are screen coordinates with a top-left origin and y pointing
// *down*, which is what internal/winpos.Get returns on every platform. That
// convention decides the direction sign: on a y-down screen the angle
// atan2(y-cy, x-cx) *increases* as a point travels clockwise as the user
// sees it, so a positive accumulated angle is Clockwise. Flip the axis and
// every direction in this package silently reverses, which is why
// TestClockwiseOnScreenMeansIncreasingAngle pins it explicitly rather than
// relying on the synthesised paths, which all share one helper's idea of
// which way round a spiral goes.
//
// # Sampling
//
// The rate the caller samples at is not the rate this sees. During a native
// title-bar drag the OS reports a moved window roughly ten times a second
// (measured on macOS: polling at 60 Hz returned the same coordinates six
// times over before they changed), so Detector dedupes repeated positions
// and the thresholds below are set for about ten distinct points per second
// - some ten to thirteen per turn at a natural drawing speed.
//
// That ceiling is also why there is no per-sample minimum radius: dropping
// samples near the centre of a tight spiral widens the angle between the
// ones that remain, pushing single steps closer to the 180-degree point
// where the shortest-arc wrap in analyse would read the rotation backwards.
// An aliased step lands against the majority sign instead, so
// MinConsistency already catches it - the check that rejects a noisy path
// rejects an undersampled one too.
package wingesture

import "time"

// Direction is which way round a recognised spiral was drawn, as the user
// sees it on screen.
type Direction int

const (
	// NoDirection is the zero value, carried by a Result that did not fire.
	NoDirection Direction = iota
	Clockwise
	CounterClockwise
)

func (d Direction) String() string {
	switch d {
	case Clockwise:
		return "clockwise"
	case CounterClockwise:
		return "counter-clockwise"
	case NoDirection:
		return "none"
	default:
		// Not a value this package produces; say so rather than let a bad
		// conversion masquerade as a gesture that did not fire.
		return "invalid"
	}
}

// Result describes a recognised gesture. The zero Result - Detected false -
// is what every position that does not complete a spiral returns.
type Result struct {
	Detected  bool
	Direction Direction

	// Inward reports that the spiral tightened towards its centre rather
	// than opening away from it.
	Inward bool
}

// Config holds the recognition thresholds. Every zero field falls back to
// the default beside it, so New(Config{}) is the tuned configuration and
// callers override only what they mean to - the defaults are not package
// variables precisely so a test can tighten one without reaching into
// shared state (see AGENTS.md on package-level test seams).
type Config struct {
	// MinTurns is how many full revolutions the gesture must complete.
	// Default 1.5. Recorded real drags reach 2.6 turns comfortably, while
	// a window merely being shuffled around a desk rarely passes half of
	// one.
	MinTurns float64

	// MinConsistency is the fraction of angular steps that must share the
	// majority direction, between 0 and 1. Default 0.85. This is what
	// separates a drawn spiral from a wobble, and doubles as the guard
	// against an undersampled step aliasing past 180 degrees.
	MinConsistency float64

	// MinMeanRadius is how far, in pixels, the path must average from its
	// own centre. Default 30. Below that the angles are dominated by the
	// integer rounding of the window position itself.
	MinMeanRadius float64

	// MinRadiusTrend is how much the radius must grow or shrink per turn,
	// as a fraction of the mean radius. Default 0.15. This is the whole of
	// the difference between a spiral and a circle; without it, idly
	// swirling the window in a steady loop would fire.
	MinRadiusTrend float64

	// IdleGap is how long a pause ends the gesture. Default 400ms.
	// Measured mid-drag gaps reach 130ms, so this clears a genuine
	// hesitation without splitting one continuous motion in two.
	IdleGap time.Duration

	// Window is how much history the detector keeps. Default 4s - long
	// enough for a whole spiral (recorded ones run 1.6s to 2.5s) and short
	// enough that a gesture cannot be assembled from minutes of unrelated
	// window shuffling.
	Window time.Duration

	// MinSamples is how many distinct positions must accumulate before the
	// path is worth analysing at all. Default 12.
	MinSamples int
}

// withDefaults fills every zero field with the documented default.
func (c Config) withDefaults() Config {
	if c.MinTurns == 0 {
		c.MinTurns = 1.5
	}
	if c.MinConsistency == 0 {
		c.MinConsistency = 0.85
	}
	if c.MinMeanRadius == 0 {
		c.MinMeanRadius = 30
	}
	if c.MinRadiusTrend == 0 {
		c.MinRadiusTrend = 0.15
	}
	if c.IdleGap == 0 {
		c.IdleGap = 400 * time.Millisecond
	}
	if c.Window == 0 {
		c.Window = 4 * time.Second
	}
	if c.MinSamples == 0 {
		c.MinSamples = 12
	}
	return c
}
