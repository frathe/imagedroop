package winpos

import (
	"sync/atomic"

	"fyne.io/fyne/v2"
)

// Tracker remembers a window's last known on-screen position. Get (above)
// can only answer "where is this window right now", and only from the main
// thread on some platforms - but the position has to survive moments when
// asking is impossible or wrong: a full-screen window reports the screen's
// corner rather than where the user left it, and at shutdown the event loop
// the read has to hop through is already winding down.
//
// So a Tracker is the last *good* reading, kept by whoever is in a position
// to take one: a background poller sampling it while the window sits
// normally, and picture-frame mode capturing it once, deliberately, right
// before going full-screen. Everyone else - the shutdown preferences save,
// the un-full-screen restore - reads it back instead of asking the OS at a
// moment when the answer would be wrong.
//
// All state is atomic because those readers and writers are on different
// goroutines: the poller runs on its own, everything else on the UI one.
// The zero Tracker is ready to use and reports ok=false until something
// records a position in it.
type Tracker struct {
	x, y  atomic.Int32
	valid atomic.Bool
}

// Store records (x, y) as the last known position. Used to seed a Tracker
// from a position that didn't come from Get at all - the one loaded from
// preferences at startup, which is a perfectly good "last known" value
// until the first live reading replaces it.
func (t *Tracker) Store(x, y int) {
	t.x.Store(int32(x))
	t.y.Store(int32(y))
	t.valid.Store(true)
}

// Get returns the last recorded position - not a fresh reading; see the
// package-level Get for that - with ok=false until one has been recorded.
func (t *Tracker) Get() (x, y int, ok bool) {
	if !t.valid.Load() {
		return 0, 0, false
	}

	return int(t.x.Load()), int(t.y.Load()), true
}

// Capture takes a fresh reading of win and records it, reporting whether
// the read succeeded. A failed read leaves the previous recording intact
// rather than invalidating it: "couldn't ask right now" is not "the window
// has no position", and the older value is still the best one available.
//
// Subject to the same constraint as the package-level Get it wraps: on
// macOS the reading touches AppKit, so a caller on a background goroutine
// must hop through fyne.DoAndWait.
func (t *Tracker) Capture(win fyne.Window) bool {
	x, y, ok := Get(win)
	if !ok {
		return false
	}
	t.Store(x, y)

	return true
}

// Restore moves win back to the recorded position, and does nothing at all
// until one has been recorded - a window with no remembered position keeps
// whatever placement the OS gave it.
func (t *Tracker) Restore(win fyne.Window) {
	x, y, ok := t.Get()
	if !ok {
		return
	}
	Set(win, x, y)
}
