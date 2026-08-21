package wingesture

import "time"

// point is one distinct window position and when it was seen.
type point struct {
	t    time.Time
	x, y int
}

// Detector accumulates window positions and reports when they trace a
// spiral. It is not safe for concurrent use; callers feed it from a single
// goroutine (in this app, the one that samples the window position).
type Detector struct {
	cfg Config

	pts []point

	// last is the most recent position seen, kept apart from pts because it
	// outlives the buffer: it is what an idle gap is measured against, and
	// pts is emptied both when a gesture fires and when one is abandoned.
	last     point
	haveLast bool

	// armed is false between a gesture firing and the next idle gap. One
	// continuous swirl should open the easter egg once, not once per extra
	// turn the user keeps drawing, so firing latches the detector off until
	// the motion actually stops.
	armed bool
}

// New builds a Detector. A zero Config gives the tuned defaults.
func New(cfg Config) *Detector {
	return &Detector{cfg: cfg.withDefaults(), armed: true}
}

// Add feeds one sampled window position and reports whether it completed a
// spiral. Repeated identical positions are ignored, so a caller may sample
// faster than the OS actually reports window movement.
func (d *Detector) Add(t time.Time, x, y int) Result {
	if d.haveLast && t.Sub(d.last.t) > d.cfg.IdleGap {
		d.pts = d.pts[:0]
		d.armed = true
	}

	// A duplicate carries no angular information. It deliberately does not
	// refresh last either: holding the window still is a pause, and must
	// age into an idle gap exactly as letting go of it does.
	if d.haveLast && x == d.last.x && y == d.last.y {
		return Result{}
	}

	d.last = point{t: t, x: x, y: y}
	d.haveLast = true

	if !d.armed {
		return Result{}
	}

	d.pts = append(d.pts, d.last)
	d.trim(t)

	if len(d.pts) < d.cfg.MinSamples {
		return Result{}
	}

	res, ok := analyse(d.pts, d.cfg)
	if !ok {
		return Result{}
	}

	d.armed = false
	d.pts = d.pts[:0]

	return res
}

// Reset discards whatever partial gesture has accumulated. The next
// position starts a fresh one.
func (d *Detector) Reset() {
	d.pts = d.pts[:0]
	d.haveLast = false
	d.armed = true
}

// trim drops points that have aged out of the configured window, keeping
// the buffer bounded however long the user keeps dragging.
func (d *Detector) trim(now time.Time) {
	cut := 0
	for cut < len(d.pts) && now.Sub(d.pts[cut].t) > d.cfg.Window {
		cut++
	}
	if cut > 0 {
		d.pts = append(d.pts[:0], d.pts[cut:]...)
	}
}
