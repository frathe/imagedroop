package preferences

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestLoadPreferences_NothingSavedReturnsDefaults(t *testing.T) {
	app := test.NewApp()

	got := Load(app)

	if got.SortMode != SortByName {
		t.Errorf("SortMode = %q, want %q (the shipped default)", got.SortMode, SortByName)
	}
	if got.MergeMode {
		t.Error("MergeMode = true, want false")
	}
	if got.SlideInterval != 0 {
		t.Errorf("SlideInterval = %v, want 0", got.SlideInterval)
	}
	if got.SlideShuffle {
		t.Error("SlideShuffle = true, want false")
	}
	if got.InfoVisible {
		t.Error("InfoVisible = true, want false")
	}
	if got.MaxScanFiles != 0 {
		t.Errorf("MaxScanFiles = %d, want 0", got.MaxScanFiles)
	}
	if got.WindowSize != (fyne.Size{}) {
		t.Errorf("WindowSize = %v, want zero value", got.WindowSize)
	}
	if got.WindowPositionSet {
		t.Error("WindowPositionSet = true, want false")
	}
}

func TestSavePreferences_RoundTrip(t *testing.T) {
	app := test.NewApp()

	want := State{
		SortMode:          SortBySize,
		MergeMode:         true,
		SlideInterval:     7 * time.Second,
		SlideShuffle:      true,
		InfoVisible:       true,
		MaxScanFiles:      5000,
		WindowSize:        fyne.NewSize(640, 480),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
	}
	Save(app, want)

	got := Load(app)
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestSavePreferences_RoundTripAtOrigin(t *testing.T) {
	app := test.NewApp()

	want := State{WindowPosX: 0, WindowPosY: 0, WindowPositionSet: true}
	Save(app, want)

	got := Load(app)
	if got.WindowPosX != 0 || got.WindowPosY != 0 || !got.WindowPositionSet {
		t.Errorf("Load() position = (%d, %d, set=%v), want (0, 0, set=true)", got.WindowPosX, got.WindowPosY, got.WindowPositionSet)
	}
}

func TestSavePreferences_ZeroSlideIntervalDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{SlideInterval: 5 * time.Second})
	Save(app, State{SlideInterval: 0})

	if got := Load(app).SlideInterval; got != 5*time.Second {
		t.Errorf("SlideInterval = %v, want 5s (should survive a zero-value Save)", got)
	}
}

func TestSavePreferences_ZeroMaxScanFilesDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{MaxScanFiles: 500})
	Save(app, State{MaxScanFiles: 0})

	if got := Load(app).MaxScanFiles; got != 500 {
		t.Errorf("MaxScanFiles = %d, want 500 (should survive a zero-value Save)", got)
	}
}

func TestSavePreferences_ZeroWindowSizeDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{WindowSize: fyne.NewSize(800, 600)})
	Save(app, State{WindowSize: fyne.Size{}})

	if got := Load(app).WindowSize; got != fyne.NewSize(800, 600) {
		t.Errorf("WindowSize = %v, want 800x600 (should survive a zero-value Save)", got)
	}
}

func TestSavePreferences_UnsetWindowPositionDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{WindowPosX: 50, WindowPosY: 60, WindowPositionSet: true})
	Save(app, State{WindowPositionSet: false})

	got := Load(app)
	if got.WindowPosX != 50 || got.WindowPosY != 60 || !got.WindowPositionSet {
		t.Errorf("position after an unset Save = (%d, %d, set=%v), want (50, 60, set=true) to survive", got.WindowPosX, got.WindowPosY, got.WindowPositionSet)
	}
}
