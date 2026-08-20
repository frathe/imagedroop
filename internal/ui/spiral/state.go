package spiral

import (
	"math"
	"sync/atomic"

	"fyne.io/fyne/v2/lang"
)

const (
	// defaultSpeed / defaultHueSpeed are the initial turn speed (rotation)
	// and colour-change speed. Both can be adjusted at runtime with the
	// arrow keys.
	defaultSpeed    = 2.2
	defaultHueSpeed = 0.06

	// speedStep / hueSpeedStep are how much the left/right and up/down
	// arrow keys change the turn speed and colour speed per key press.
	speedStep    = 0.2
	hueSpeedStep = 0.02

	// maxSpeed bounds the turn speed in either direction (negative values
	// reverse the spiral's direction of rotation). maxHueSpeed bounds the
	// colour-change speed, which is not allowed to go negative.
	maxSpeed    = 8.0
	maxHueSpeed = 0.5

	// defaultArms controls how many spiral arms are drawn.
	defaultArms = 4.0

	// defaultTwistBase is the base twist constant. A larger value makes
	// the spiral more tightly wound; smaller values spread the arms
	// apart.
	defaultTwistBase = 30.0

	// defaultDensity is the fraction of the monitor's physical pixels we
	// want to use for rendering. 1.0 = native resolution; lower values
	// trade quality for speed.
	defaultDensity = 1.0
)

// state holds everything about a running spiral that used to live in the
// donor demo's package-level vars. It splits into two groups by who
// touches each field:
//
//   - speed, hue speed, follow mode, active preset, mouse position, and
//     center offset are all written from one goroutine and read from
//     another: the key handler and mouse tracker run on Fyne's UI
//     goroutine, while the per-frame render/follow ticker runs on its own
//     goroutine. They are atomics (float64s stored as bit patterns via
//     math.Float64bits, as the donor did) so -race has nothing to catch
//     and callers never need their own locking.
//   - arms, twist, and density are only ever written by the settings
//     panel's slider callbacks and read by the status overlay, both of
//     which run on the Fyne UI goroutine exclusively. Plain float64 fields
//     are enough; making them atomic too would just be noise.
type state struct {
	speedBits    atomic.Uint64
	hueSpeedBits atomic.Uint64

	followMode   atomic.Bool
	activePreset atomic.Bool

	mouseXBits atomic.Uint64
	mouseYBits atomic.Uint64

	centerOffsetXBits atomic.Uint64
	centerOffsetYBits atomic.Uint64

	arms    float64
	twist   float64
	density float64
}

// newState builds a state seeded with the same defaults the donor demo
// initialised its package-level vars with.
func newState() *state {
	s := &state{
		arms:    defaultArms,
		twist:   defaultTwistBase,
		density: defaultDensity,
	}
	s.speedBits.Store(math.Float64bits(defaultSpeed))
	s.hueSpeedBits.Store(math.Float64bits(defaultHueSpeed))
	return s
}

func (s *state) speed() float64 {
	return math.Float64frombits(s.speedBits.Load())
}

// adjustSpeed changes the turn speed by delta, clamping to
// [-maxSpeed, maxSpeed]. Negative values reverse the spiral's direction.
func (s *state) adjustSpeed(delta float64) {
	v := clampFloat(s.speed()+delta, -maxSpeed, maxSpeed)
	s.speedBits.Store(math.Float64bits(v))
}

func (s *state) hueSpeed() float64 {
	return math.Float64frombits(s.hueSpeedBits.Load())
}

// adjustHueSpeed changes the colour-change speed by delta, clamping to
// [0, maxHueSpeed].
func (s *state) adjustHueSpeed(delta float64) {
	v := clampFloat(s.hueSpeed()+delta, 0, maxHueSpeed)
	s.hueSpeedBits.Store(math.Float64bits(v))
}

// preset reports which spiral pattern is active: false is the original
// ripple spiral, true is the layered nautilus spiral.
func (s *state) preset() bool {
	return s.activePreset.Load()
}

// togglePreset switches between spiral presets, bound to the N key.
func (s *state) togglePreset() {
	s.activePreset.Store(!s.activePreset.Load())
}

// presetName returns the display name of the currently active preset, for
// the status/help overlays.
func (s *state) presetName() string {
	if s.activePreset.Load() {
		return lang.L("Nautilus")
	}
	return lang.L("Ripple")
}

// follow reports whether the spiral center currently follows the mouse
// cursor.
func (s *state) follow() bool {
	return s.followMode.Load()
}

// toggleFollow flips follow mode, bound to the F key. On exit, the center
// offset is left as-is so the spiral keeps its last position instead of
// snapping back to screen centre.
func (s *state) toggleFollow() {
	s.followMode.Store(!s.followMode.Load())
}

// setMouse records the mouse cursor's last-seen position, fed by the
// full-window mouse tracker.
func (s *state) setMouse(x, y float64) {
	s.mouseXBits.Store(math.Float64bits(x))
	s.mouseYBits.Store(math.Float64bits(y))
}

// mouse returns the mouse cursor's last-seen position.
func (s *state) mouse() (x, y float64) {
	return math.Float64frombits(s.mouseXBits.Load()), math.Float64frombits(s.mouseYBits.Load())
}

// setCenterOffset records the shader's current center offset, updated by
// follow mode as it chases the cursor.
func (s *state) setCenterOffset(x, y float64) {
	s.centerOffsetXBits.Store(math.Float64bits(x))
	s.centerOffsetYBits.Store(math.Float64bits(y))
}

// centerOffset returns the shader's current center offset.
func (s *state) centerOffset() (x, y float64) {
	return math.Float64frombits(s.centerOffsetXBits.Load()), math.Float64frombits(s.centerOffsetYBits.Load())
}

// clampFloat restricts v to the closed interval [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
