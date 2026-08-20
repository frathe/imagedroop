package spiral

import "testing"

func TestClampFloat(t *testing.T) {
	tests := []struct {
		v, lo, hi, want float64
	}{
		{10.0, 0.0, 20.0, 10.0},
		{-5.0, 0.0, 20.0, 0.0},
		{25.0, 0.0, 20.0, 20.0},
	}
	for _, tt := range tests {
		if got := clampFloat(tt.v, tt.lo, tt.hi); got != tt.want {
			t.Errorf("clampFloat(%f, %f, %f) = %f; want %f", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestNewStateSeedsDefaults(t *testing.T) {
	s := newState()

	if got := s.speed(); got != defaultSpeed {
		t.Errorf("speed() = %f; want %f", got, defaultSpeed)
	}
	if got := s.hueSpeed(); got != defaultHueSpeed {
		t.Errorf("hueSpeed() = %f; want %f", got, defaultHueSpeed)
	}
	if s.follow() {
		t.Error("follow() = true; want false")
	}
	if s.preset() {
		t.Error("preset() = true; want false")
	}
	if got := s.arms; got != defaultArms {
		t.Errorf("arms = %f; want %f", got, defaultArms)
	}
	if got := s.twist; got != defaultTwistBase {
		t.Errorf("twist = %f; want %f", got, defaultTwistBase)
	}
	if got := s.density; got != defaultDensity {
		t.Errorf("density = %f; want %f", got, defaultDensity)
	}
	if x, y := s.mouse(); x != 0 || y != 0 {
		t.Errorf("mouse() = (%f, %f); want (0, 0)", x, y)
	}
	if x, y := s.centerOffset(); x != 0 || y != 0 {
		t.Errorf("centerOffset() = (%f, %f); want (0, 0)", x, y)
	}
}

func TestSpeedAdjustment(t *testing.T) {
	s := newState()

	// Reset to a known baseline of 0 before exercising deltas, mirroring
	// the donor's own test approach of using adjustSpeed itself to zero
	// out the seeded default rather than reaching into internals.
	s.adjustSpeed(-s.speed())
	s.adjustSpeed(2.0)
	if got := s.speed(); got != 2.0 {
		t.Errorf("speed() = %f; want 2.0", got)
	}

	s.adjustSpeed(10.0) // should clamp to maxSpeed (8.0)
	if got := s.speed(); got != maxSpeed {
		t.Errorf("speed() = %f; want %f", got, maxSpeed)
	}

	s.adjustSpeed(-20.0) // should clamp to -maxSpeed (-8.0); negative reverses direction
	if got := s.speed(); got != -maxSpeed {
		t.Errorf("speed() = %f; want %f", got, -maxSpeed)
	}
}

func TestHueSpeedAdjustment(t *testing.T) {
	s := newState()

	s.adjustHueSpeed(-s.hueSpeed())
	s.adjustHueSpeed(0.1)
	if got := s.hueSpeed(); got != 0.1 {
		t.Errorf("hueSpeed() = %f; want 0.1", got)
	}

	s.adjustHueSpeed(1.0) // should clamp to maxHueSpeed (0.5)
	if got := s.hueSpeed(); got != maxHueSpeed {
		t.Errorf("hueSpeed() = %f; want %f", got, maxHueSpeed)
	}

	s.adjustHueSpeed(-1.0) // should clamp to 0, never negative
	if got := s.hueSpeed(); got != 0 {
		t.Errorf("hueSpeed() = %f; want 0", got)
	}
}

func TestPresetName(t *testing.T) {
	s := newState()

	if got := s.presetName(); got != "Ripple" {
		t.Errorf("presetName() = %q; want %q", got, "Ripple")
	}

	s.togglePreset()
	if got := s.presetName(); got != "Nautilus" {
		t.Errorf("presetName() = %q; want %q", got, "Nautilus")
	}

	s.togglePreset()
	if got := s.presetName(); got != "Ripple" {
		t.Errorf("presetName() = %q; want %q", got, "Ripple")
	}
}

func TestTogglePreset(t *testing.T) {
	s := newState()

	if s.preset() {
		t.Fatal("preset() = true; want false before any toggle")
	}
	s.togglePreset()
	if !s.preset() {
		t.Error("preset() = false; want true after one toggle")
	}
	s.togglePreset()
	if s.preset() {
		t.Error("preset() = true; want false after two toggles")
	}
}

func TestToggleFollow(t *testing.T) {
	s := newState()

	if s.follow() {
		t.Fatal("follow() = true; want false before any toggle")
	}
	s.toggleFollow()
	if !s.follow() {
		t.Error("follow() = false; want true after one toggle")
	}
	s.toggleFollow()
	if s.follow() {
		t.Error("follow() = true; want false after two toggles")
	}
}

func TestMouseRoundTrip(t *testing.T) {
	s := newState()

	s.setMouse(12.5, -34.75)
	if x, y := s.mouse(); x != 12.5 || y != -34.75 {
		t.Errorf("mouse() = (%f, %f); want (12.5, -34.75)", x, y)
	}
}

func TestCenterOffsetRoundTrip(t *testing.T) {
	s := newState()

	s.setCenterOffset(-100.25, 200.5)
	if x, y := s.centerOffset(); x != -100.25 || y != 200.5 {
		t.Errorf("centerOffset() = (%f, %f); want (-100.25, 200.5)", x, y)
	}
}
