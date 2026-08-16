// Package slideshow is picture-frame mode (the P key): the window goes
// full-screen and, with more than one file loaded, advances to the next
// image on its own every few seconds - a photo frame rather than a viewer.
//
// It owns the auto-advance goroutine, the interval the user tunes with
// Up/Down, the shuffle order Shift+P toggles (see the shuffle field - it
// only changes which file Advance picks next, so it lives here even though
// the picking itself happens on the viewer's side of Host), and the
// window-position capture/restore that makes leaving full-screen put the
// window back where the user had actually left it (see winpos.Tracker). It
// reaches back into the app through Host, which is deliberately tiny: the
// two things a photo frame does are ask how many pictures there are and
// move to the next one. The crossfade between images belongs to that next
// one too - it's the app's own internal/ui/load.go, since this package
// never touches a pixel.
//
// It knows nothing about the app's other full-window mode (the grid): the
// two don't compose, but that guard lives in the app's key dispatcher, not
// here.
package slideshow

import (
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/winpos"
)

const (
	// DefaultInterval is how often picture-frame mode advances the first
	// time it's ever entered; MinInterval is the floor AdjustInterval
	// clamps to.
	DefaultInterval = 10 * time.Second
	MinInterval     = 1 * time.Second
)

// Host is what the slideshow needs from the application: how much there is
// to show, and how to show the next thing.
type Host interface {
	// FileCount is how many files are loaded. Zero means there is nothing
	// to frame - Toggle refuses to enter - and one means there is nothing
	// to advance between, so the mode still works (full-screen, single
	// picture) but the auto-advance stays quiet.
	FileCount() int

	// Advance displays the next file, wrapping around at the end.
	Advance()
}

// Controller is picture-frame mode: the state behind it and the goroutine
// that drives it.
type Controller struct {
	host Host
	win  fyne.Window

	// pos is where the window's manually-placed position is remembered
	// across a full-screen round trip - see enter/Exit.
	pos *winpos.Tracker

	// active is the mode flag. Atomic because the app's window-position
	// poller reads it off the UI goroutine (to skip readings taken while
	// full-screen); every other reader and writer is UI-goroutine code.
	// The run goroutine itself never touches it - it stops on gen instead,
	// so that "is the mode on" and "should this session keep running" stay
	// two separate questions.
	active atomic.Bool

	// gen identifies the current picture-frame session. run captures it at
	// start and stops as soon as it no longer matches, which is how Exit
	// (and a rapid off/on toggle) retires the previous goroutine without
	// having to track it.
	gen atomic.Uint64

	// kick restarts the countdown in progress - see Kick. Per-session and
	// captured by run, so a kick can never reach a goroutine other than
	// the one it was meant for.
	kick chan struct{}

	// running counts the live run goroutine, so Settle can wait it out.
	// Only ever incremented on the UI goroutine, in enter.
	running sync.WaitGroup

	// intervalNS is the auto-advance interval, and animDurNS the current
	// image's full animation loop (0 for a static one). Both atomic: run
	// reads them on its own goroutine while the key dispatcher and the
	// load path write them on the UI one. intervalNS is 0 until something
	// sets it - a fresh install with no saved preference - and enter
	// substitutes DefaultInterval at that point rather than at
	// construction, so "never chosen" stays distinguishable from a real
	// value for as long as possible.
	intervalNS atomic.Int64
	animDurNS  atomic.Int64

	// shuffle is a standing preference like intervalNS: on, it makes
	// Advance (the Host method the run goroutine below calls) pick a
	// random other file instead of the next one in order. It lives here
	// rather than on the app's viewer because it's part of the same
	// "how picture-frame mode paces itself" state as the interval, and
	// Toggle/enter/Exit don't touch it at all - unlike the interval, a
	// shuffle order has nothing to reset or substitute a default for.
	// Atomic for the same reason as the rest of this struct, though in
	// practice both its reader (the viewer's Advance) and its writers
	// (Shift+P, and the app seeding it from a saved preference) only ever
	// run on the UI goroutine.
	shuffle atomic.Bool
}

// New builds the controller for win, idle. host supplies the file set, and
// pos is the shared window-position tracker the app also keeps current
// with its own poller.
func New(host Host, win fyne.Window, pos *winpos.Tracker) *Controller {
	return &Controller{host: host, win: win, pos: pos}
}

// Toggle flips picture-frame mode on or off. There's nothing to frame with
// zero files loaded, so that case is a no-op rather than full-screening an
// empty drop zone.
func (c *Controller) Toggle() {
	if c.active.Load() {
		c.Exit()

		return
	}
	if c.host.FileCount() == 0 {
		return
	}
	c.enter()
}

// enter full-screens the window and starts a fresh run goroutine for this
// session. The interval keeps whatever value Up/Down left it at from a
// previous session, so the chosen pace is a standing preference across
// toggles the same way the app's sort and merge modes are.
func (c *Controller) enter() {
	// Captured synchronously, right before full-screening, so the position
	// Exit restores below is exactly where the user last left the window
	// rather than whatever the app's background poller happened to have on
	// its last tick, up to a poll interval stale.
	c.pos.Capture(c.win)

	c.active.Store(true)
	if c.intervalNS.Load() == 0 {
		c.intervalNS.Store(int64(DefaultInterval))
	}
	c.win.SetFullScreen(true)

	gen := c.gen.Add(1)
	kick := make(chan struct{}, 1)
	c.kick = kick

	c.running.Go(func() {
		c.run(gen, kick)
	})
}

// Exit leaves full-screen and stops the auto-advance goroutine by bumping
// the generation and kicking it awake, so it notices right away instead of
// lingering asleep for up to the current wait duration. Safe to call when
// picture-frame mode is already off - the app calls it unconditionally
// when clearing back to the drop zone, so a reset or a load error never
// leaves a full-screen drop zone behind.
func (c *Controller) Exit() {
	if !c.active.Load() {
		return
	}
	c.active.Store(false)
	c.gen.Add(1)
	c.Kick()
	c.win.SetFullScreen(false)

	// Put the window back where the user manually left it rather than
	// wherever the OS chose to un-full-screen it to - see enter's capture.
	c.pos.Restore(c.win)
}

// Active reports whether picture-frame mode is on.
func (c *Controller) Active() bool {
	return c.active.Load()
}

// Interval is the current auto-advance interval, or 0 if none has ever
// been chosen (enter substitutes DefaultInterval at that point).
func (c *Controller) Interval() time.Duration {
	return time.Duration(c.intervalNS.Load())
}

// SetInterval sets the auto-advance interval outright - how the app seeds
// it from the saved preference at startup.
func (c *Controller) SetInterval(d time.Duration) {
	c.intervalNS.Store(int64(d))
}

// AdjustInterval changes the interval by delta (±1s, from Up/Down while in
// picture-frame mode), clamped to MinInterval, and kicks the slideshow so
// the new pace applies to the countdown already in progress instead of
// waiting for it to finish first.
func (c *Controller) AdjustInterval(delta time.Duration) {
	next := max(c.Interval()+delta, MinInterval)
	c.SetInterval(next)
	c.Kick()
}

// Shuffle reports whether auto-advance picks a random other file instead
// of the next one in order - see the shuffle field.
func (c *Controller) Shuffle() bool {
	return c.shuffle.Load()
}

// SetShuffle sets shuffle mode outright - how the app seeds it from the
// saved preference at startup.
func (c *Controller) SetShuffle(on bool) {
	c.shuffle.Store(on)
}

// ToggleShuffle flips shuffle mode - Shift+P.
func (c *Controller) ToggleShuffle() {
	c.shuffle.Store(!c.shuffle.Load())
}

// AnimDuration is the current image's full animation loop, 0 for a static
// one - see SetAnimDuration.
func (c *Controller) AnimDuration() time.Duration {
	return time.Duration(c.animDurNS.Load())
}

// SetAnimDuration tells the slideshow how long the image now on screen
// takes to play through once (0 for a static image), so an animated GIF
// gets to finish at least one loop before the slideshow moves on - see
// waitDuration. The app calls it on every load, unconditionally, so one
// image's duration can never leak into the next.
func (c *Controller) SetAnimDuration(d time.Duration) {
	c.animDurNS.Store(int64(d))
}

// Kick restarts the current countdown early instead of waiting for it to
// time out on its own. Used after a manual navigation and after an
// interval change, so both take effect immediately. The channel is
// buffered by 1 and the send is non-blocking: a kick arriving while
// another is still pending is simply dropped, since run is about to
// restart its wait anyway. A no-op before the first enter, when there is
// no channel yet (a send on a nil channel takes the default branch).
func (c *Controller) Kick() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// Settle waits for the auto-advance goroutine to finish. Exit only asks it
// to stop; this is how the app's test suite makes sure it is actually gone
// before the test that started it ends - otherwise it sleeps out its
// interval and then wakes to advance a slide, doing full UI work, in the
// middle of whatever test is running by then.
func (c *Controller) Settle() {
	c.running.Wait()
}

// waitDuration returns how long to wait before advancing: the longer of
// the configured interval and, for an animated GIF, its full loop duration
// - so a GIF always gets to finish playing at least once before the
// slideshow moves on. A free function so it's trivially unit-testable
// without any goroutine or real-time waiting involved.
func waitDuration(interval, animDuration time.Duration) time.Duration {
	if animDuration > interval {
		return animDuration
	}

	return interval
}

// advance moves to the next file if gen is still current and there's more
// than one file to advance between, and reports whether gen turned out to
// be stale (in which case the caller, run, stops). Split out from run so
// the advance/staleness logic can be driven directly and synchronously
// from a test instead of through a real timer.
func (c *Controller) advance(gen uint64) (stale bool) {
	if c.gen.Load() != gen {
		return true
	}
	if c.host.FileCount() > 1 {
		c.host.Advance()
	}

	return false
}

// run drives the auto-advance on its own goroutine. gen is this session's
// generation and kick this session's channel, both captured at enter
// rather than read back off the controller, so this goroutine touches no
// mutable state outside the atomics and never advances on behalf of a
// session that has already ended.
func (c *Controller) run(gen uint64, kick chan struct{}) {
	for {
		wait := waitDuration(c.Interval(), c.AnimDuration())

		select {
		case <-time.After(wait):
			// Timed out - fall through and advance below.
		case <-kick:
			// A manual navigation or an interval change: restart the
			// countdown with fresh values, but check staleness first so
			// Exit's own kick makes this goroutine stop right away
			// instead of looping once more.
			if c.gen.Load() != gen {
				return
			}

			continue
		}

		stale := false
		fyne.Do(func() { stale = c.advance(gen) })
		if stale {
			return
		}
	}
}
