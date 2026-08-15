package slideshow

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/frathe/imagedrop/internal/winpos"
)

func TestMain(m *testing.M) {
	// The controller full-screens a real fyne.Window, so these need a
	// driver behind them. The fyne test driver's windows are not
	// driver.NativeWindow, so every winpos read fails and the tracker
	// stays empty - which is exactly the degraded path the app already
	// runs on any backend without a native handle.
	test.NewApp()
	os.Exit(m.Run())
}

// fakeHost stands in for the viewer: a file count, and a tally of the
// advances the slideshow asked for.
type fakeHost struct {
	files    int
	advances int
}

func (f *fakeHost) FileCount() int { return f.files }
func (f *fakeHost) Advance()       { f.advances++ }

// newController builds a controller over a real (test-driver) window with
// n files behind it, closing the window when the test ends.
func newController(t *testing.T, n int) (*Controller, *fakeHost) {
	t.Helper()

	host := &fakeHost{files: n}
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return New(host, win, &winpos.Tracker{}), host
}

// settle stops the slideshow and waits for its goroutine, failing rather
// than hanging if it never notices - a kick that doesn't wake the run loop
// would otherwise show up as a test that blocks for a whole interval.
func settle(t *testing.T, c *Controller) {
	t.Helper()

	c.Exit()

	done := make(chan struct{})
	go func() {
		c.Settle()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the slideshow goroutine to exit")
	}
}

// --- waitDuration ----------------------------------------------------------

func TestWaitDuration(t *testing.T) {
	cases := []struct {
		name         string
		interval     time.Duration
		animDuration time.Duration
		want         time.Duration
	}{
		{"static image uses the interval", 10 * time.Second, 0, 10 * time.Second},
		{"short gif loop doesn't shorten the interval", 10 * time.Second, 3 * time.Second, 10 * time.Second},
		{"long gif loop overrides a shorter interval", 10 * time.Second, 15 * time.Second, 15 * time.Second},
		{"equal duration and interval", 5 * time.Second, 5 * time.Second, 5 * time.Second},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := waitDuration(c.interval, c.animDuration); got != c.want {
				t.Errorf("waitDuration(%v, %v) = %v, want %v", c.interval, c.animDuration, got, c.want)
			}
		})
	}
}

// --- toggling / entering / exiting -----------------------------------------

func TestToggle_NoFilesIsNoop(t *testing.T) {
	c, _ := newController(t, 0)

	c.Toggle()
	t.Cleanup(func() { settle(t, c) })

	if c.Active() {
		t.Error("picture-frame mode should not turn on with nothing loaded")
	}
	if c.win.FullScreen() {
		t.Error("the window should not go full-screen with nothing loaded")
	}
}

func TestToggle_EntersAndExitsFullScreen(t *testing.T) {
	c, _ := newController(t, 2)
	// Long enough that the run goroutine cannot advance during the test.
	c.SetInterval(time.Hour)

	c.Toggle()
	t.Cleanup(func() { settle(t, c) })
	if !c.Active() {
		t.Error("picture-frame mode should be on after the first toggle")
	}
	if !c.win.FullScreen() {
		t.Error("the window should be full-screen after entering")
	}

	c.Toggle()
	if c.Active() {
		t.Error("picture-frame mode should be off after the second toggle")
	}
	if c.win.FullScreen() {
		t.Error("the window should leave full-screen after exiting")
	}
}

// Entering with a single file still works - a one-picture frame - it just
// has nothing to advance between (see advance).
func TestToggle_SingleFileStillEnters(t *testing.T) {
	c, _ := newController(t, 1)
	c.SetInterval(time.Hour)

	c.Toggle()
	t.Cleanup(func() { settle(t, c) })

	if !c.Active() || !c.win.FullScreen() {
		t.Error("picture-frame mode should turn on with a single file loaded")
	}
}

func TestExit_NoopWhenAlreadyOff(t *testing.T) {
	c, _ := newController(t, 2)

	// Must not panic (e.g. on the nil kick channel) when the mode was
	// never entered - the app calls Exit unconditionally when it clears
	// back to the drop zone.
	c.Exit()

	if c.Active() || c.win.FullScreen() {
		t.Error("Exit should be a no-op when picture-frame mode is already off")
	}
}

// Exit bumps the generation and kicks the run goroutine awake, so it stops
// right away rather than sleeping out the rest of its interval. With an
// hour-long interval, a goroutine that only noticed on timeout would hang
// this test instead of settling.
func TestExit_StopsTheRunGoroutineImmediately(t *testing.T) {
	c, _ := newController(t, 2)
	c.SetInterval(time.Hour)

	c.Toggle()

	settle(t, c)
}

// --- interval --------------------------------------------------------------

func TestEnter_SubstitutesTheDefaultIntervalOnlyWhenUnset(t *testing.T) {
	c, _ := newController(t, 2)

	if c.Interval() != 0 {
		t.Fatalf("a fresh controller's interval = %v, want 0 (nothing chosen yet)", c.Interval())
	}

	c.Toggle()
	t.Cleanup(func() { settle(t, c) })

	if c.Interval() != DefaultInterval {
		t.Errorf("interval after entering unset = %v, want the %v default", c.Interval(), DefaultInterval)
	}
}

func TestEnter_KeepsAnIntervalAlreadyChosen(t *testing.T) {
	c, _ := newController(t, 2)
	c.SetInterval(30 * time.Second)

	c.Toggle()
	t.Cleanup(func() { settle(t, c) })

	if got, want := c.Interval(), 30*time.Second; got != want {
		t.Errorf("interval after entering = %v, want the previously set %v", got, want)
	}
}

func TestAdjustInterval_ClampsToTheMinimum(t *testing.T) {
	c, _ := newController(t, 2)
	c.SetInterval(DefaultInterval)

	c.AdjustInterval(time.Second)
	if got, want := c.Interval(), DefaultInterval+time.Second; got != want {
		t.Errorf("interval after +1s = %v, want %v", got, want)
	}

	for range 30 {
		c.AdjustInterval(-time.Second)
	}
	if got := c.Interval(); got != MinInterval {
		t.Errorf("interval floor = %v, want the %v minimum", got, MinInterval)
	}
}

// --- shuffle -----------------------------------------------------------

func TestShuffle_DefaultsOff(t *testing.T) {
	c, _ := newController(t, 2)

	if c.Shuffle() {
		t.Error("a fresh controller's Shuffle() = true, want false")
	}
}

func TestToggleShuffle_Flips(t *testing.T) {
	c, _ := newController(t, 2)

	c.ToggleShuffle()
	if !c.Shuffle() {
		t.Error("Shuffle() = false after one ToggleShuffle, want true")
	}

	c.ToggleShuffle()
	if c.Shuffle() {
		t.Error("Shuffle() = true after two ToggleShuffle calls, want false")
	}
}

func TestSetShuffle_SetsOutright(t *testing.T) {
	c, _ := newController(t, 2)

	c.SetShuffle(true)
	if !c.Shuffle() {
		t.Error("Shuffle() = false after SetShuffle(true), want true")
	}

	c.SetShuffle(false)
	if c.Shuffle() {
		t.Error("Shuffle() = true after SetShuffle(false), want false")
	}
}

// --- Kick ------------------------------------------------------------------

func TestKick_NonBlockingAndCoalesces(t *testing.T) {
	c := &Controller{kick: make(chan struct{}, 1)}

	// Two kicks in a row must not block even though only one slot exists.
	c.Kick()
	c.Kick()

	select {
	case <-c.kick:
	default:
		t.Fatal("expected a pending kick after Kick")
	}

	select {
	case <-c.kick:
		t.Fatal("expected the second kick to have been coalesced, not queued")
	default:
	}
}

func TestKick_BeforeEnterIsANoop(t *testing.T) {
	c, _ := newController(t, 2)

	// The kick channel doesn't exist until the first enter; a send on a
	// nil channel must take the default branch rather than blocking
	// forever. AdjustInterval kicks too, and the app allows tuning the
	// interval before ever entering the mode.
	c.Kick()
}

// --- advance ---------------------------------------------------------------

func TestAdvance_StaleGenReportsStaleWithoutAdvancing(t *testing.T) {
	c, host := newController(t, 2)
	c.gen.Store(5)

	if stale := c.advance(4); !stale {
		t.Error("advance with a mismatched gen should report stale")
	}
	if host.advances != 0 {
		t.Errorf("advances = %d, want 0 - a stale gen must not advance", host.advances)
	}
}

func TestAdvance_SingleFileDoesNotAdvance(t *testing.T) {
	c, host := newController(t, 1)
	c.gen.Store(1)

	if stale := c.advance(1); stale {
		t.Error("advance with a matching gen should not report stale")
	}
	if host.advances != 0 {
		t.Errorf("advances = %d, want 0 - a single file has nothing to advance to", host.advances)
	}
}

func TestAdvance_AdvancesToTheNextFile(t *testing.T) {
	c, host := newController(t, 2)
	c.gen.Store(1)

	if stale := c.advance(1); stale {
		t.Error("advance with a matching gen should not report stale")
	}
	if host.advances != 1 {
		t.Errorf("advances = %d, want 1", host.advances)
	}
}

// --- animation duration ----------------------------------------------------

func TestSetAnimDuration_ExtendsTheWait(t *testing.T) {
	c, _ := newController(t, 2)
	c.SetInterval(5 * time.Second)

	c.SetAnimDuration(12 * time.Second)
	if got, want := waitDuration(c.Interval(), c.AnimDuration()), 12*time.Second; got != want {
		t.Errorf("wait with a 12s loop = %v, want %v", got, want)
	}

	// Cleared again by the next (static) image, so one image's loop can't
	// leak into the next one's wait.
	c.SetAnimDuration(0)
	if got, want := waitDuration(c.Interval(), c.AnimDuration()), 5*time.Second; got != want {
		t.Errorf("wait after clearing the loop = %v, want the %v interval", got, want)
	}
}
