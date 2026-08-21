// Package spiral is the Hypno Spiral easter egg: a full-screen, GPU-shader
// spiral in a window of its own, ported from a standalone Fyne demo into
// picfetch's package layout.
//
// Nothing in the app's menus, shortcuts, or settings leads here. The only
// way in is the manual's search box (internal/ui/help): typing the magic
// word there opens it, which is the whole point of an easter egg - you have
// to already know it is there. That also means this package is reached from
// exactly one call site, and nothing else in the app depends on it.
//
// Escape closes this window and only this window. The donor demo called
// app.Quit() on Escape because it was a standalone binary; doing the same
// here would take PicFetch down along with the easter egg, losing whatever
// the user had open. That is the single most important deviation in the
// port, and TestEscapeClosesHelpThenWindow is its guard.
//
// The rest of the keys are handled entirely inside this window (see
// handleKey): F1 for the help overlay, F for follow mode, N for the spiral
// pattern, P and R for the FPS and resolution overlays, and the arrow keys
// for turn and colour speed.
//
// The package splits up as: this file owns the window, the key bindings,
// and the per-frame goroutine; state.go carries the demo's package-level
// globals over as fields on a struct - this repo forbids mutable
// package-level state; shader.go holds the two GLSL sources and the uniform
// seeding; overlays.go the status/help/FPS text panels; settings.go the
// auto-hiding slider panel; mouse.go the full-window hover tracker follow
// mode reads; and monitor.go the small helpers that describe the monitor a
// window is showing on.
package spiral

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/lang"
)

// defaultFrameInterval is how often the frame goroutine does a round of
// per-frame UI work - roughly 60 fps, the donor demo's 16ms. It only paces
// follow mode, the FPS readout, and the settings panel's auto-hide; the
// spiral's own motion comes from canvas.NewShaderAnimation, which Fyne
// drives on its own render clock.
const defaultFrameInterval = 16 * time.Millisecond

// followEpsilon is how far, in physical pixels, the cursor has to be from
// the current centre offset before follow mode bothers rewriting the
// uniforms. Sub-pixel jitter is invisible and a Refresh per frame for it is
// pure waste.
const followEpsilon = 0.5

// Spiral is the easter egg's window and the state behind it. New builds one
// without opening anything, so the app can construct it at startup and pay
// nothing until someone actually finds it.
type Spiral struct {
	app fyne.App

	// win is the open window, nil whenever it is closed. Read and written
	// only on Fyne's UI goroutine (Show, Close, the SetOnClosed callback,
	// and frame, which run marshals there) - it is the identity callers and
	// tests use to tell "raised the same window" from "opened a second one",
	// the same way widgets.Singleton does.
	win fyne.Window

	// st survives across open/close cycles, so a spiral reopened later
	// comes back at the speed, pattern, and slider values it was left at
	// rather than snapping back to defaults - see newShader, which seeds
	// its uniforms from st rather than from the package constants.
	st *state

	// Everything below is rebuilt per window by Show and is only ever
	// touched on the UI goroutine. They are fields rather than closure
	// captures so the key handler, frame, and the tests can all reach them
	// without Show having to hand out a bundle.
	shader *canvas.Shader
	status *fyne.Container
	help   *fyne.Container
	fps    *fyne.Container
	panel  *settingsPanel
	anim   *fyne.Animation

	// frameInterval paces the frame goroutine. A field rather than a
	// package-level var because AGENTS.md forbids mutable package-level
	// test seams: tests stretch it to a minute so the goroutine provably
	// never ticks while they are asserting (Fyne's test driver runs fyne.Do
	// inline on the calling goroutine, so a live frame goroutine would be a
	// real data race, not a theoretical one).
	frameInterval time.Duration

	// gen identifies the current spiral session. run captures it at start
	// and stops as soon as it no longer matches, which is how a close - or
	// a rapid close/reopen - retires the previous goroutine without having
	// to track it.
	gen atomic.Uint64

	// cancel wakes the frame goroutine immediately instead of leaving it
	// asleep until its next tick, the job slideshow's kick channel does
	// there. Closed by stop and then nilled; captured by run per session so
	// a cancellation can never reach a goroutine other than the one it was
	// meant for. Deliberately independent of the window: the goroutine has
	// to be stoppable whether the window is closing, already gone, or
	// never existed.
	cancel chan struct{}

	// running counts the live frame goroutine so Settle can wait it out.
	// Only ever incremented on the UI goroutine, in Show.
	running sync.WaitGroup
}

// New builds the easter egg without opening anything. Cheap enough to
// construct unconditionally at startup: no window, no goroutine, no shader
// - just the state the first Show will seed its uniforms from.
func New(app fyne.App) *Spiral {
	return &Spiral{
		app:           app,
		st:            newState(),
		frameInterval: defaultFrameInterval,
	}
}

// Open reports whether the spiral window is currently open.
func (s *Spiral) Open() bool {
	return s.win != nil
}

// Show raises the window if it is already open, or builds and shows a fresh
// full-screen one. Mirrors widgets.Singleton.Show's raise behaviour - the
// easter egg is a single window, and finding it a second time should bring
// the one already up to the front rather than stack another on top of it.
// ShowForGesture opens the spiral on the pattern the user's gesture asked
// for, and is the window-drag gesture's way in (internal/wingesture, wired
// up in internal/ui/gesture.go): swirling the window clockwise brings up the
// Nautilus, counter-clockwise the Ripple. The manual's secret phrase goes on
// using plain Show, which opens whichever pattern the spiral was last left
// on - the same as the N key's own toggle.
//
// Which direction maps to which pattern is arbitrary and lives here rather
// than at the call site, since knowing what presets exist is this package's
// business and not internal/ui/help's.
//
// The uniform has to be written as well as the state whenever a window is
// already open: newShader seeds the uniforms from the state once, when the
// shader is built, so on an already-open spiral Show alone would raise the
// old window and change nothing.
func (s *Spiral) ShowForGesture(clockwise bool) {
	s.st.setPreset(clockwise)

	if s.win != nil {
		preset := float32(0)
		if clockwise {
			preset = 1
		}
		s.setUniform(s.win, "preset", preset)
	}

	s.Show()
}

func (s *Spiral) Show() {
	if s.win != nil {
		s.win.Show()
		s.win.RequestFocus()

		return
	}

	s.shader = newShader(s.st)
	s.status = newStatusOverlay()
	s.help = newHelpOverlay()
	updateHelpText(s.help)
	s.fps = newFPSOverlay()
	s.panel = newSettingsPanel(s.st, s.shader)

	win := s.app.NewWindow(lang.L("Hypno Spiral"))
	win.SetFullScreen(true)
	win.SetPadded(false)

	// The shader is set as the window's content, not as another overlay, so
	// Fyne resizes it to fill the window on its own.
	win.SetContent(s.shader)

	overlays := win.Canvas().Overlays()
	overlays.Add(s.status)
	overlays.Add(s.help)
	overlays.Add(s.fps)

	// The settings overlay must go on last, and this ordering is
	// load-bearing rather than cosmetic.
	//
	// Fyne has no window- or canvas-level "mouse moved" hook - hover events
	// are only ever dispatched to individual CanvasObjects implementing
	// desktop.Hoverable - and its hit-testing
	// (FindObjectAtPositionMatching) walks *only* canvas.Overlays().Top()
	// whenever any overlay is present. Every other overlay, and the canvas
	// content underneath, is skipped entirely, not merely shadowed. So the
	// full-window mouse tracker follow mode depends on lives inside the
	// settings overlay (see newSettingsPanel), and that overlay has to be
	// the topmost one and is never itself hidden - the auto-hide timer
	// hides the visible box inside it instead. Add anything after this, or
	// hide this, and mouse tracking dies silently: no panic, no error, the
	// spiral just stops following the cursor and the settings panel stops
	// reappearing. TestSettingsOverlayIsTopmost guards the order.
	overlays.Add(s.panel.overlay)

	// The key list is the first thing on screen, since nothing else in the
	// app advertises these bindings.
	s.help.Show()

	win.Canvas().SetOnTypedKey(s.handleKey)

	// Covers the user closing the window themselves (the window manager's
	// close button, or Cmd+W); Close comes at the same teardown from the
	// other side. stop is idempotent, so whichever path runs first wins and
	// the other finds nothing left to do.
	win.SetOnClosed(s.stop)

	s.win = win
	win.Show()

	// Started straight after Show, with none of the donor's half-second
	// sleep before it: a bare sleeping goroutine would keep running - and
	// then touch the shader - across a close and reopen. Should real
	// hardware ever turn out to need a delay here, it belongs in a
	// time.AfterFunc whose callback re-checks the generation first.
	s.anim = canvas.NewShaderAnimation(s.shader)
	s.anim.Start()

	gen := s.gen.Add(1)
	cancel := make(chan struct{})
	s.cancel = cancel
	s.running.Go(func() {
		s.run(gen, cancel)
	})
}

// Close closes the window if one is open, and is a no-op otherwise.
func (s *Spiral) Close() {
	win := s.win
	if win == nil {
		return
	}

	// Torn down before the window is asked to go away, rather than relying
	// on SetOnClosed to get there first: that keeps Settle's answer
	// independent of when a given driver gets around to running the closed
	// callback, so a shutdown path can Close and then immediately Settle.
	s.stop()
	win.Close()
}

// Settle waits for the frame goroutine to finish. Close only asks it to
// stop; this is how a test - or the app's own shutdown - makes sure it is
// actually gone rather than about to wake up and do UI work in the middle
// of whatever is running by then.
func (s *Spiral) Settle() {
	s.running.Wait()
}

// stop retires the current session: it invalidates the frame goroutine's
// generation, wakes it so it notices right now instead of on its next tick,
// freezes the shader animation, and forgets the window so the next Show
// builds a fresh one. Idempotent - both Close and the window's own closed
// callback route through it.
func (s *Spiral) stop() {
	if s.win == nil {
		return
	}

	s.gen.Add(1)
	if s.cancel != nil {
		close(s.cancel)
		s.cancel = nil
	}
	if s.anim != nil {
		s.anim.Stop()
		s.anim = nil
	}
	s.win = nil
}

// run drives the per-frame work on its own goroutine. gen is this session's
// generation and cancel this session's stop channel, both captured at Show
// rather than read back off the Spiral, so this goroutine touches no
// mutable field of its own and can never do a frame's work on behalf of a
// session that has already ended.
func (s *Spiral) run(gen uint64, cancel chan struct{}) {
	ticker := time.NewTicker(s.frameInterval)
	defer ticker.Stop()

	last := time.Now()
	for {
		select {
		case <-ticker.C:
			if s.gen.Load() != gen {
				return
			}

			now := time.Now()
			dt := now.Sub(last).Seconds()
			last = now

			// One hop per tick, wrapping the whole frame: everything frame
			// touches is a Fyne object, and splitting it across several
			// fyne.Do calls would let a close land between them.
			fyne.Do(func() { s.frame(dt) })
		case <-cancel:
			return
		}
	}
}

// frame does one frame's worth of UI work. It must run on Fyne's UI
// goroutine; run() is what marshals it there.
//
// Split out of run so it can be driven directly and synchronously from a
// test - no goroutine, no ticker, no sleeping - which matters more here
// than usual: Fyne's test driver runs fyne.Do inline on the calling
// goroutine, so a live frame goroutine during a test is a genuine data
// race rather than merely awkward timing.
func (s *Spiral) frame(dt float64) {
	// A hop queued just before the window closed can still arrive after
	// stop has cleared it.
	win := s.win
	if win == nil {
		return
	}

	updateFollowMode(win, s.st, s.shader)

	// Skipped while hidden: rebuilding the readout's text and backdrop
	// every frame for an overlay nobody is looking at is the one piece of
	// per-frame work here that is worth avoiding.
	if s.fps.Visible() {
		updateFPS(win, s.fps, dt)
	}

	s.panel.tick(win)
}

// handleKey is the window's key dispatcher, wired to the canvas in Show.
// A separate method rather than a closure so tests can reach it the way the
// rest of this repo's key handling is tested.
func (s *Spiral) handleKey(ev *fyne.KeyEvent) {
	win := s.win
	if win == nil {
		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		// Escape closes the help overlay if it is up, and otherwise closes
		// this window - never the app. See the package comment.
		if s.help.Visible() {
			s.help.Hide()

			return
		}
		s.Close()
	case fyne.KeyF1:
		toggleOverlay(s.help)
	case fyne.KeyF:
		// Follow mode: the centre chases the cursor. Leaving it does not
		// recentre the spiral - see toggleFollow.
		s.st.toggleFollow()
	case fyne.KeyN:
		s.st.togglePreset()
		preset := float32(0)
		if s.st.preset() {
			preset = 1
		}
		s.setUniform(win, "preset", preset)
	case fyne.KeyP:
		toggleOverlay(s.fps)
	case fyne.KeyR:
		// Unlike the other two overlays this one has content that goes
		// stale, so revealing it refreshes it first.
		if s.status.Visible() {
			s.status.Hide()

			return
		}
		s.status.Show()
		updateStatus(win, s.st, s.status)
	case fyne.KeyUp:
		s.st.adjustHueSpeed(hueSpeedStep)
		s.setUniform(win, "hueSpeed", float32(s.st.hueSpeed()))
	case fyne.KeyDown:
		s.st.adjustHueSpeed(-hueSpeedStep)
		s.setUniform(win, "hueSpeed", float32(s.st.hueSpeed()))
	case fyne.KeyRight:
		s.st.adjustSpeed(speedStep)
		s.setUniform(win, "speed", float32(s.st.speed()))
	case fyne.KeyLeft:
		s.st.adjustSpeed(-speedStep)
		s.setUniform(win, "speed", float32(s.st.speed()))
	}
}

// setUniform writes one shader uniform, repaints the shader, and brings the
// status overlay up to date if it happens to be showing - the three things
// every uniform-changing key does, in that order.
func (s *Spiral) setUniform(w fyne.Window, name string, value float32) {
	s.shader.Uniforms[name] = value
	s.shader.Refresh()
	refreshStatus(w, s.st, s.status)
}

// toggleOverlay flips one overlay's visibility, the shape the plain
// show/hide key bindings share.
func toggleOverlay(o *fyne.Container) {
	if o.Visible() {
		o.Hide()

		return
	}
	o.Show()
}

// updateFollowMode moves the shader's centre offset toward the mouse cursor
// while follow mode is on. With it off it does nothing, leaving the offset
// wherever it last was so the spiral keeps its position instead of
// recentring.
//
// Unlike the donor demo's version this wraps nothing in fyne.Do: it runs
// from frame, which run has already marshalled onto the UI goroutine, so
// the mutations below are on the right goroutine as they stand and a nested
// hop would only be a way to smear one frame's work across two of them.
func updateFollowMode(w fyne.Window, st *state, shader *canvas.Shader) {
	if !st.follow() {
		return
	}

	size := w.Canvas().Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}

	mx, my := st.mouse()

	// PixelCoordinateForPosition converts logical points into the same
	// physical pixel space the shader's `frame` uniform is expressed in.
	// canvas.Scale() alone is not enough: on macOS it is always 1 and
	// Retina/HiDPI scaling is folded into a private texture-scale factor
	// instead, so multiplying by Scale() directly undershoots the offset on
	// HiDPI displays and the spiral centre lags behind the cursor.
	px, py := w.Canvas().PixelCoordinateForPosition(fyne.NewPos(float32(mx), float32(my)))
	frameW, frameH := w.Canvas().PixelCoordinateForPosition(fyne.NewPos(size.Width, size.Height))

	// Y is inverted: Fyne's mouse coordinates have their origin top-left
	// (y grows down), while gl_FragCoord in the shader has its origin
	// bottom-left (y grows up).
	wantX := float64(px) - float64(frameW)/2
	wantY := float64(frameH)/2 - float64(py)

	haveX, haveY := st.centerOffset()
	if math.Abs(wantX-haveX) <= followEpsilon && math.Abs(wantY-haveY) <= followEpsilon {
		return
	}

	st.setCenterOffset(wantX, wantY)
	shader.Uniforms["centerOffsetX"] = float32(wantX)
	shader.Uniforms["centerOffsetY"] = float32(wantY)
	shader.Refresh()
}
