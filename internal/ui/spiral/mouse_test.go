package spiral

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func mouseEventAt(x, y float32) *desktop.MouseEvent {
	return &desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y)}}
}

func TestNewMouseTrackerMouseMovedStoresPositionAndFiresActivity(t *testing.T) {
	st := newState()
	activity := 0
	tr := newMouseTracker(st, func() { activity++ })

	tr.MouseMoved(mouseEventAt(12.5, 34.5))

	if x, y := st.mouse(); x != 12.5 || y != 34.5 {
		t.Errorf("mouse() = (%f, %f); want (12.5, 34.5)", x, y)
	}
	if activity != 1 {
		t.Errorf("activity = %d; want 1", activity)
	}
}

func TestNewMouseTrackerMouseInStoresPositionAndFiresActivity(t *testing.T) {
	st := newState()
	activity := 0
	tr := newMouseTracker(st, func() { activity++ })

	tr.MouseIn(mouseEventAt(1, 2))

	if x, y := st.mouse(); x != 1 || y != 2 {
		t.Errorf("mouse() = (%f, %f); want (1, 2)", x, y)
	}
	if activity != 1 {
		t.Errorf("activity = %d; want 1", activity)
	}
}

func TestNewMouseTrackerMouseOutDoesNothing(t *testing.T) {
	st := newState()
	activity := 0
	tr := newMouseTracker(st, func() { activity++ })

	// Seed a known position first so MouseOut leaving it untouched is a
	// meaningful assertion rather than coincidentally matching the zero
	// value newState() already starts with.
	st.setMouse(5, 6)

	tr.MouseOut()

	if x, y := st.mouse(); x != 5 || y != 6 {
		t.Errorf("mouse() = (%f, %f); want (5, 6) unchanged - MouseOut must not touch state", x, y)
	}
	if activity != 0 {
		t.Errorf("activity = %d; want 0 - MouseOut must not fire the activity callback", activity)
	}
}

// TestNewMouseTrackerSizeExceedsAnyPlausibleWindow guards the donor's
// reasoning for the tracker's huge fixed size: it must stay under the
// cursor without ever being resized, so it has to dwarf any window a real
// desktop would actually open.
func TestNewMouseTrackerSizeExceedsAnyPlausibleWindow(t *testing.T) {
	st := newState()
	tr := newMouseTracker(st, func() {})

	const plausibleMaxWindowDimension = 100000 // far beyond any real display
	size := tr.Size()
	if size.Width <= plausibleMaxWindowDimension || size.Height <= plausibleMaxWindowDimension {
		t.Errorf("tracker size = %v; want both dimensions > %d", size, plausibleMaxWindowDimension)
	}
}
