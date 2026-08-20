package spiral

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// settleTimeout bounds how long a test waits for the frame goroutine to be
// gone before calling it leaked. It should never actually be reached -
// Close cancels the goroutine outright rather than leaving it to notice on
// its next tick - so this only exists to turn a leak into a readable
// failure instead of a hung package.
const settleTimeout = 5 * time.Second

// newTestSpiral builds a Spiral on a fresh test app whose frame interval is
// long enough that the frame goroutine never actually ticks during the
// test, and closes it again on cleanup.
//
// The long interval is load-bearing, not tidiness. Fyne's test driver
// implements fyne.Do as a bare inline call on the *calling* goroutine
// (test/driver.go's DoFromGoroutine) rather than hopping to a UI thread, so
// a live 16ms frame goroutine would mutate the very Fyne objects these
// tests assert on, from a second goroutine, and -race would rightly fire.
// Every test here therefore drives frame() directly and synchronously and
// never lets the ticker fire at all.
func newTestSpiral(t *testing.T) *Spiral {
	t.Helper()

	a := test.NewApp()
	s := New(a)
	s.frameInterval = time.Minute

	t.Cleanup(func() {
		s.Close()
		waitSettled(t, s)
	})

	return s
}

// waitSettled fails the test unless s's frame goroutine is gone within
// settleTimeout. Settle itself blocks, so it runs on a goroutine of its own
// here purely so the timeout can report a leak; nothing waits on a duration
// to decide that things went *right*.
func waitSettled(t *testing.T, s *Spiral) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		s.Settle()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(settleTimeout):
		t.Errorf("Settle did not return within %s; the frame goroutine outlived Close", settleTimeout)
	}
}

// press sends a key through the window's own canvas rather than calling
// handleKey directly, so these tests cover the SetOnTypedKey wiring as well
// as the handler behind it.
func press(t *testing.T, s *Spiral, name fyne.KeyName) {
	t.Helper()

	if s.win == nil {
		t.Fatal("press: no window open")
	}
	handler := s.win.Canvas().OnTypedKey()
	if handler == nil {
		t.Fatal("canvas has no OnTypedKey handler; the spiral window never wired up its key handling")
	}
	handler(&fyne.KeyEvent{Name: name})
}

// assertUniform checks one shader uniform against want with a tolerance:
// the uniforms are float32 while the state behind them is float64, so
// demanding exact equality would be asserting on rounding rather than on
// behaviour.
func assertUniform(t *testing.T, s *Spiral, name string, want float32) {
	t.Helper()

	const tolerance = 1e-5

	got := s.shader.Uniforms[name]
	if diff := float64(got) - float64(want); diff > tolerance || diff < -tolerance {
		t.Errorf("Uniforms[%q] = %v; want %v", name, got, want)
	}
}

func TestShowOpensFullScreenShaderWindow(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	if !s.Open() {
		t.Fatal("Open() = false after Show(); want true")
	}
	if s.win == nil {
		t.Fatal("win is nil after Show(); want the open window")
	}

	content := s.win.Content()
	if _, ok := content.(*canvas.Shader); !ok {
		t.Errorf("window content = %T; want *canvas.Shader - the shader is set as content so Fyne resizes it to fill the window", content)
	}
	if content != fyne.CanvasObject(s.shader) {
		t.Error("window content is not the spiral's own shader")
	}
	if !s.win.FullScreen() {
		t.Error("FullScreen() = false; want true")
	}
	if !s.help.Visible() {
		t.Error("help overlay hidden on open; want it shown so the key list is the first thing seen")
	}
}

func TestShowTwiceRaisesTheSameWindow(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()
	first := s.win

	s.Show()

	if s.win != first {
		t.Error("second Show() replaced the window; want the already-open one raised instead")
	}
}

// TestEscapeClosesHelpThenWindow is the guard for this port's single most
// important deviation from the donor demo: the donor called app.Quit() on
// Escape because it was a standalone binary, and doing that here would take
// PicFetch down along with the easter egg. Escape must dismiss the help
// overlay first and then close nothing but this window.
func TestEscapeClosesHelpThenWindow(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()
	first := s.win

	if !s.help.Visible() {
		t.Fatal("help overlay not visible on open; the rest of this test assumes it is")
	}

	press(t, s, fyne.KeyEscape)

	if s.help.Visible() {
		t.Error("help overlay still visible after the first Escape; want it hidden")
	}
	if !s.Open() {
		t.Fatal("first Escape closed the window; want it to dismiss only the help overlay")
	}

	press(t, s, fyne.KeyEscape)

	if s.Open() {
		t.Fatal("Open() = true after the second Escape; want the window closed")
	}
	waitSettled(t, s)

	s.Show()

	if !s.Open() {
		t.Fatal("Open() = false after re-Show(); want a fresh window")
	}
	if s.win == first {
		t.Error("Show() after a close reused the closed window; want a freshly built one")
	}
}

// TestSettingsOverlayIsTopmost is the regression guard for the overlay
// ordering documented in Show: Fyne's hit-testing walks only
// Overlays().Top(), so the settings overlay - the one holding the
// full-window mouse tracker - has to be the last one added or mouse
// tracking silently dies.
func TestSettingsOverlayIsTopmost(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	overlays := s.win.Canvas().Overlays()
	if top := overlays.Top(); top != fyne.CanvasObject(s.panel.overlay) {
		t.Errorf("Overlays().Top() = %T; want the settings panel's overlay - it holds the mouse tracker and hit-testing only ever reaches the top overlay", top)
	}

	want := []fyne.CanvasObject{s.status, s.help, s.fps, s.panel.overlay}
	got := overlays.List()
	if len(got) != len(want) {
		t.Fatalf("overlay stack has %d entries; want %d (status, help, FPS, settings)", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("overlay %d is not the expected one; the stack order is load-bearing", i)
		}
	}
}

func TestKeyNTogglesPresetUniform(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	assertUniform(t, s, "preset", 0)

	press(t, s, fyne.KeyN)

	if !s.st.preset() {
		t.Error("preset() = false after N; want true")
	}
	assertUniform(t, s, "preset", 1)

	press(t, s, fyne.KeyN)

	if s.st.preset() {
		t.Error("preset() = true after a second N; want false")
	}
	assertUniform(t, s, "preset", 0)
}

func TestLeftRightAdjustAndClampSpeedUniform(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	assertUniform(t, s, "speed", float32(defaultSpeed))

	press(t, s, fyne.KeyRight)
	assertUniform(t, s, "speed", float32(defaultSpeed+speedStep))

	press(t, s, fyne.KeyLeft)
	assertUniform(t, s, "speed", float32(defaultSpeed))

	// Far more presses than the range allows: both the state and the
	// uniform have to stop at the ceiling rather than run away.
	for range 100 {
		press(t, s, fyne.KeyRight)
	}
	if got := s.st.speed(); got != maxSpeed {
		t.Errorf("speed() = %v after holding Right; want the %v ceiling", got, maxSpeed)
	}
	assertUniform(t, s, "speed", float32(maxSpeed))

	// And at the floor, which is negative: the spiral reverses rather than
	// stopping.
	for range 200 {
		press(t, s, fyne.KeyLeft)
	}
	if got := s.st.speed(); got != -maxSpeed {
		t.Errorf("speed() = %v after holding Left; want the %v floor", got, -maxSpeed)
	}
	assertUniform(t, s, "speed", float32(-maxSpeed))
}

func TestUpDownAdjustAndClampHueSpeedUniform(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	assertUniform(t, s, "hueSpeed", float32(defaultHueSpeed))

	press(t, s, fyne.KeyUp)
	assertUniform(t, s, "hueSpeed", float32(defaultHueSpeed+hueSpeedStep))

	press(t, s, fyne.KeyDown)
	assertUniform(t, s, "hueSpeed", float32(defaultHueSpeed))

	for range 100 {
		press(t, s, fyne.KeyUp)
	}
	if got := s.st.hueSpeed(); got != maxHueSpeed {
		t.Errorf("hueSpeed() = %v after holding Up; want the %v ceiling", got, maxHueSpeed)
	}
	assertUniform(t, s, "hueSpeed", float32(maxHueSpeed))

	// Colour speed, unlike turn speed, is not allowed to go negative.
	for range 100 {
		press(t, s, fyne.KeyDown)
	}
	if got := s.st.hueSpeed(); got != 0 {
		t.Errorf("hueSpeed() = %v after holding Down; want the 0 floor", got)
	}
	assertUniform(t, s, "hueSpeed", 0)
}

func TestOverlayToggleKeys(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	cases := []struct {
		name          string
		key           fyne.KeyName
		overlay       *fyne.Container
		startsVisible bool
	}{
		{"P toggles the FPS overlay", fyne.KeyP, s.fps, false},
		{"R toggles the status overlay", fyne.KeyR, s.status, false},
		{"F1 toggles the help overlay", fyne.KeyF1, s.help, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.overlay.Visible(); got != tc.startsVisible {
				t.Fatalf("Visible() = %t before any press; want %t", got, tc.startsVisible)
			}

			press(t, s, tc.key)
			if got := tc.overlay.Visible(); got == tc.startsVisible {
				t.Errorf("Visible() = %t after one press; want it flipped to %t", got, !tc.startsVisible)
			}

			press(t, s, tc.key)
			if got := tc.overlay.Visible(); got != tc.startsVisible {
				t.Errorf("Visible() = %t after two presses; want it back at %t", got, tc.startsVisible)
			}
		})
	}
}

func TestKeyFTogglesFollowMode(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()

	if s.st.follow() {
		t.Fatal("follow() = true on a fresh spiral; want false")
	}

	press(t, s, fyne.KeyF)
	if !s.st.follow() {
		t.Error("follow() = false after F; want true")
	}

	press(t, s, fyne.KeyF)
	if s.st.follow() {
		t.Error("follow() = true after a second F; want false")
	}
}

// TestFrameFollowModeTracksMouse drives one frame's worth of work directly,
// on the test's own goroutine, instead of waiting for the frame goroutine
// to tick - see newTestSpiral for why letting that goroutine run during a
// test is a race rather than an inconvenience.
func TestFrameFollowModeTracksMouse(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()
	s.win.Resize(fyne.NewSize(800, 600))

	size := s.win.Canvas().Size()
	if size.Width <= 0 || size.Height <= 0 {
		t.Fatalf("canvas size = %v after Resize; the follow-mode maths needs a real size", size)
	}

	// Right of centre and above it, expressed relative to the canvas so
	// this does not depend on the test driver honouring an exact size.
	rightAbove := fyne.NewPos(size.Width*0.75, size.Height*0.25)
	s.st.setMouse(float64(rightAbove.X), float64(rightAbove.Y))

	// Follow mode is off: a frame must leave the centre offset alone, so
	// the spiral stays wherever it was rather than chasing a cursor nobody
	// asked it to follow.
	s.frame(0.016)

	assertUniform(t, s, "centerOffsetX", 0)
	assertUniform(t, s, "centerOffsetY", 0)

	s.st.toggleFollow()
	s.frame(0.016)

	x, y := s.st.centerOffset()
	if x <= 0 {
		t.Errorf("centerOffset x = %v for a cursor right of centre; want > 0", x)
	}
	if y <= 0 {
		t.Errorf("centerOffset y = %v for a cursor above centre; want > 0 - the shader's y axis points up while Fyne's points down, so the port has to invert it", y)
	}
	assertUniform(t, s, "centerOffsetX", float32(x))
	assertUniform(t, s, "centerOffsetY", float32(y))

	// Left of centre and below it flips both signs.
	leftBelow := fyne.NewPos(size.Width*0.25, size.Height*0.75)
	s.st.setMouse(float64(leftBelow.X), float64(leftBelow.Y))
	s.frame(0.016)

	x, y = s.st.centerOffset()
	if x >= 0 {
		t.Errorf("centerOffset x = %v for a cursor left of centre; want < 0", x)
	}
	if y >= 0 {
		t.Errorf("centerOffset y = %v for a cursor below centre; want < 0", y)
	}
	assertUniform(t, s, "centerOffsetX", float32(x))
	assertUniform(t, s, "centerOffsetY", float32(y))
}

// TestShowThenCloseStopsTheFrameGoroutine is the leak check. The frame
// interval is a minute here (see newTestSpiral), so a Close that only
// invalidated the generation and left the goroutine asleep until its next
// tick would hang Settle for that minute instead of returning - which is
// exactly the bug this catches.
func TestShowThenCloseStopsTheFrameGoroutine(t *testing.T) {
	s := newTestSpiral(t)

	s.Show()
	if !s.Open() {
		t.Fatal("Open() = false after Show(); want true")
	}

	s.Close()

	if s.Open() {
		t.Error("Open() = true after Close(); want false")
	}
	waitSettled(t, s)
}

func TestCloseAndSettleOnUnopenedSpiralAreNoOps(t *testing.T) {
	a := test.NewApp()
	s := New(a)

	if s.Open() {
		t.Error("Open() = true on a freshly constructed Spiral; want false - New opens no window")
	}

	s.Close()
	s.Close() // twice, to make sure the second one has nothing left to trip over

	if s.Open() {
		t.Error("Open() = true after Close() on a never-opened Spiral; want false")
	}
	waitSettled(t, s)
}

// TestRunReturnsOnStaleGeneration checks the loop's own staleness guard
// rather than the cancel channel: a run whose generation has moved on -
// because a later session started, or because this one was torn down and
// the channel it captured was already drained - must retire itself on its
// next tick. No window is ever opened here, so even a loop that wrongly
// carried on has nothing to touch.
func TestRunReturnsOnStaleGeneration(t *testing.T) {
	a := test.NewApp()
	s := New(a)
	s.frameInterval = time.Millisecond
	s.gen.Store(7)

	done := make(chan struct{})
	go func() {
		s.run(1, make(chan struct{}))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(settleTimeout):
		t.Fatalf("run() with a stale generation did not return within %s", settleTimeout)
	}
}
