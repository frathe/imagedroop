package widgets

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func newSingletonContent() fyne.CanvasObject { return widget.NewLabel("content") }

// A Singleton nobody called Remember on must behave exactly as it always
// has - that is what keeps the manual and the About box (internal/ui/help),
// which have no geometry to remember, out of all of this.
func TestSingleton_WithoutRememberOpensAtTheGivenSize(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Show(app, "plain", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	if got, want := s.Window().Canvas().Size(), fyne.NewSize(300, 200); got != want {
		t.Errorf("window size = %v, want %v (the caller's own default)", got, want)
	}
	if got := s.Geometry(); got != (Geometry{}) {
		t.Errorf("Geometry() = %+v, want the zero value with nothing being remembered", got)
	}
}

func TestSingleton_RememberedSizeWinsOverTheDefault(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Remember(Geometry{Size: fyne.NewSize(500, 450)})
	s.Show(app, "sized", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	if got, want := s.Window().Canvas().Size(), fyne.NewSize(500, 450); got != want {
		t.Errorf("window size = %v, want the remembered %v", got, want)
	}
}

func TestSingleton_UnrememberedSizeFallsBackToTheDefault(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Remember(Geometry{X: 10, Y: 20, PositionSet: true})
	s.Show(app, "unsized", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	if got, want := s.Window().Canvas().Size(), fyne.NewSize(300, 200); got != want {
		t.Errorf("window size = %v, want the caller's default %v when no size was saved", got, want)
	}
}

func TestSingleton_GeometryTracksResizes(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Remember(Geometry{})
	s.Show(app, "resized", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	s.Window().Resize(fyne.NewSize(640, 480))

	if got, want := s.Geometry().Size, fyne.NewSize(640, 480); got != want {
		t.Errorf("Geometry().Size = %v, want %v", got, want)
	}
}

// The seeded position has to survive until a live reading replaces it -
// the same rule the main window's own tracker follows (internal/ui's
// startup restoration stores the saved position before Run starts the
// poller), so a launch that never gets a fresh reading still saves last
// launch's good value instead of losing it to a zero.
func TestSingleton_RememberedPositionSurvivesWithoutALiveReading(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Remember(Geometry{X: 120, Y: 340, PositionSet: true, Size: fyne.NewSize(500, 450)})
	s.Show(app, "placed", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	got := s.Geometry()
	if !got.PositionSet || got.X != 120 || got.Y != 340 {
		t.Errorf("Geometry() position = (%d, %d, set=%v), want the seeded (120, 340, set=true)", got.X, got.Y, got.PositionSet)
	}
}

// Geometry outlives the window: the app saves it at shutdown, long after
// the user closed the panel it belongs to.
func TestSingleton_GeometrySurvivesTheWindowClosing(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.Remember(Geometry{X: 70, Y: 80, PositionSet: true})
	s.Show(app, "closed", fyne.NewSize(300, 200), newSingletonContent, nil)
	s.Window().Resize(fyne.NewSize(640, 480))
	s.Window().Close()

	if s.Open() {
		t.Fatal("Open() = true after Close")
	}

	got := s.Geometry()
	if got.Size != fyne.NewSize(640, 480) {
		t.Errorf("Geometry().Size = %v, want the last tracked 640x480", got.Size)
	}
	if !got.PositionSet || got.X != 70 || got.Y != 80 {
		t.Errorf("Geometry() position = (%d, %d, set=%v), want (70, 80, set=true)", got.X, got.Y, got.PositionSet)
	}
}

// The fyne test driver's windows are not desktop.Windows, so the always-on-
// top request has nothing to reach - it must degrade to a no-op rather than
// panicking on a failed type assertion, and leave the window otherwise
// untouched.
func TestSingleton_KeepOnTopOnANonDesktopWindow(t *testing.T) {
	app := test.NewApp()

	var s Singleton
	s.KeepOnTop()
	s.Show(app, "ontop", fyne.NewSize(300, 200), newSingletonContent, nil)
	defer s.Window().Close()

	if s.Window() == nil {
		t.Fatal("window should be open after Show")
	}
	if got, want := s.Window().Canvas().Size(), fyne.NewSize(300, 200); got != want {
		t.Errorf("window size = %v, want %v", got, want)
	}
}

func TestSingleton_StopTrackingIsSafeWhenNothingIsTracking(t *testing.T) {
	var s Singleton

	s.StopTracking()
	s.StopTracking()
}
