package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"github.com/frathe/picfetch/internal/winpos"
)

// Singleton is a secondary window that should exist at most once - the
// manual, the About box, the EXIF panel, the Settings panel. Its zero value
// is a closed window that remembers nothing, so it can be embedded as a
// plain field and used without construction; call Remember to make it keep
// its geometry as well.
type Singleton struct {
	win fyne.Window

	// remember is what Remember turns on: without it none of the fields
	// below are ever written or read, and this is exactly the window it
	// always was. The manual and the About box (internal/ui/help) open at a
	// fixed size in whatever place the OS picks, and want nothing else.
	remember bool

	// pos and size are the geometry Remember seeds and Geometry reports.
	// Both deliberately outlive the window itself: the app saves them at
	// shutdown (see internal/preferences), which is long after the user
	// closed the panel they belong to. pos is a winpos.Tracker for the
	// reason that type exists at all - a position can only be read from the
	// live window, and only at moments when asking is possible.
	pos  winpos.Tracker
	size fyne.Size

	// stopPoll stops the position poller behind the open window, and is nil
	// whenever none is running. Called on close, and by StopTracking at
	// shutdown for a window still open then.
	stopPoll func()
}

// Geometry is where a Singleton window was last seen: its on-screen
// position and its size. The zero value means "nothing remembered", which
// is what a window opens with the first time. PositionSet distinguishes
// "at (0,0)" from "nowhere recorded" - (0,0) is a perfectly good position -
// and a zero Size means the window falls back to the size its own caller
// passes Show.
//
// Deliberately this package's own type rather than
// preferences.WindowGeometry, which is the same four values: what a window
// remembers is UI state, and how it is stored between launches is the app's
// business - internal/ui translates between the two, the same way it
// already does for every other preference.
type Geometry struct {
	X, Y        int
	PositionSet bool
	Size        fyne.Size
}

// Remember turns on geometry memory for this window and seeds it with g -
// typically what the last run saved. A seeded position is applied to the
// next window built and, until a live reading replaces it, is also what
// Geometry keeps reporting, so a run that never gets one saves last run's
// good value rather than a zero.
func (s *Singleton) Remember(g Geometry) {
	s.remember = true
	s.size = g.Size
	if g.PositionSet {
		s.pos.Store(g.X, g.Y)
	}
}

// Geometry reports where the window currently is and how big it is, or the
// last values known while it was open. Reports the zero value for a window
// that was never told to Remember anything.
func (s *Singleton) Geometry() Geometry {
	if !s.remember {
		return Geometry{}
	}

	x, y, ok := s.pos.Get()

	return Geometry{X: x, Y: y, PositionSet: ok, Size: s.size}
}

// StopTracking stops the position poller without closing the window, for
// the one case closing doesn't cover: the app shutting down while the
// window is still open, where the poller must stop before the event loop
// its readings hop through winds down (see winpos.Poll). Safe to call at
// any time, including repeatedly and on a window that never tracked
// anything; the remembered geometry survives it, so a save afterwards still
// has values.
func (s *Singleton) StopTracking() {
	if s.stopPoll != nil {
		s.stopPoll()
		s.stopPoll = nil
	}
}

// Show raises the window if it's already open, or builds a fresh one from
// build and shows it.
//
// The new window is resized *before* content is set - a window starts at a
// zero size, and laying a wrapped RichText out at zero width panics in this
// Fyne version (widget/richtext.go's row-bounds computation isn't zero-size
// safe). size is the caller's own default, used whenever there's no
// remembered size to open at instead. Escape closes just this window - it
// must not reset or quit the app the way it does in the image window.
// Closing forgets the window so the next call opens a fresh one rather than
// trying to raise a closed one; onClosed, if non-nil, runs then too for any
// extra teardown.
func (s *Singleton) Show(app fyne.App, title string, size fyne.Size, build func() fyne.CanvasObject, onClosed func()) {
	if s.win != nil {
		s.win.Show()
		s.win.RequestFocus()

		return
	}

	win := app.NewWindow(title)

	if s.remember && s.size.Width > 0 && s.size.Height > 0 {
		size = s.size
	}
	win.Resize(size)

	content := build()
	if s.remember {
		// Wrapping the content is how the size gets read back at all - see
		// NewSizeTracker, and note it records the window's size rather than
		// this container's own.
		content = container.New(NewSizeTracker(win, &s.size), content)
	}
	win.SetContent(content)

	win.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			win.Close()
		}
	})

	win.SetOnClosed(func() {
		s.win = nil
		s.StopTracking()
		if onClosed != nil {
			onClosed()
		}
	})

	s.win = win

	// Applied before Show for the reason internal/ui's buildViewer applies
	// the main window's saved position before its own: RequestPosition on a
	// window that isn't up yet just primes the coordinates the glfw driver's
	// window-creation path uses once it does run.
	if s.remember {
		s.pos.Restore(win)
	}

	win.Show()

	// Started after Show, so the first reading lands on a window that
	// actually exists on screen. A no-op on backends with no native handle
	// to read - the fyne test driver included, so no test carries a poller.
	if s.remember {
		s.stopPoll = winpos.Poll(win, &s.pos, nil)
	}
}

// Window returns the open window, or nil when it's closed - the identity
// callers and tests use to tell "raised the same window" from "opened a
// second one".
func (s *Singleton) Window() fyne.Window {
	return s.win
}

// Open reports whether the window is currently open.
func (s *Singleton) Open() bool {
	return s.win != nil
}
