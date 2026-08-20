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
	if got.MaxScanFiles != 0 {
		t.Errorf("MaxScanFiles = %d, want 0", got.MaxScanFiles)
	}
	if got.MaxWindowWidth != 0 {
		t.Errorf("MaxWindowWidth = %v, want 0", got.MaxWindowWidth)
	}
	if got.MaxWindowHeight != 0 {
		t.Errorf("MaxWindowHeight = %v, want 0", got.MaxWindowHeight)
	}
	if got.MaxImageCacheMB != 0 {
		t.Errorf("MaxImageCacheMB = %d, want 0", got.MaxImageCacheMB)
	}
	if got.MaxThumbCacheMB != 0 {
		t.Errorf("MaxThumbCacheMB = %d, want 0", got.MaxThumbCacheMB)
	}
	if got.MaxFileSizeMB != 0 {
		t.Errorf("MaxFileSizeMB = %d, want 0", got.MaxFileSizeMB)
	}
	if got.WindowSize != (fyne.Size{}) {
		t.Errorf("WindowSize = %v, want zero value", got.WindowSize)
	}
	if got.WindowPositionSet {
		t.Error("WindowPositionSet = true, want false")
	}
	if got.SettingsWindow != (WindowGeometry{}) {
		t.Errorf("SettingsWindow = %+v, want zero value", got.SettingsWindow)
	}
	if got.ExifWindow != (WindowGeometry{}) {
		t.Errorf("ExifWindow = %+v, want zero value", got.ExifWindow)
	}
	if !got.FavoritePreviewCache {
		t.Error("FavoritePreviewCache = false, want true (default is on)")
	}
}

func TestSavePreferences_RoundTrip(t *testing.T) {
	app := test.NewApp()

	want := State{
		SortMode:          SortBySize,
		MergeMode:         true,
		SlideInterval:     7 * time.Second,
		SlideShuffle:      true,
		MaxScanFiles:      5000,
		MaxWindowWidth:    1800,
		MaxWindowHeight:   1100,
		MaxImageCacheMB:   384,
		MaxThumbCacheMB:   192,
		MaxFileSizeMB:     256,
		WindowSize:        fyne.NewSize(640, 480),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
		SettingsWindow: WindowGeometry{
			X: 200, Y: 210, PositionSet: true, Size: fyne.NewSize(470, 440),
		},
		ExifWindow: WindowGeometry{
			X: 300, Y: 310, PositionSet: true, Size: fyne.NewSize(430, 370),
		},
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

func TestSavePreferences_UnsetSecondaryWindowGeometryDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	saved := WindowGeometry{X: 70, Y: 80, PositionSet: true, Size: fyne.NewSize(500, 400)}
	Save(app, State{SettingsWindow: saved, ExifWindow: saved})
	Save(app, State{})

	got := Load(app)
	if got.SettingsWindow != saved {
		t.Errorf("SettingsWindow after an unset Save = %+v, want %+v to survive", got.SettingsWindow, saved)
	}
	if got.ExifWindow != saved {
		t.Errorf("ExifWindow after an unset Save = %+v, want %+v to survive", got.ExifWindow, saved)
	}
}

func TestSavePreferences_SecondaryWindowGeometryRoundTripsAtOrigin(t *testing.T) {
	app := test.NewApp()

	Save(app, State{ExifWindow: WindowGeometry{X: 0, Y: 0, PositionSet: true}})

	if got := Load(app).ExifWindow; got.X != 0 || got.Y != 0 || !got.PositionSet {
		t.Errorf("ExifWindow position = (%d, %d, set=%v), want (0, 0, set=true)", got.X, got.Y, got.PositionSet)
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

func TestSavePreferences_ZeroMaxWindowSizeDoesNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{MaxWindowWidth: 1600, MaxWindowHeight: 1000})
	Save(app, State{MaxWindowWidth: 0, MaxWindowHeight: 0})

	got := Load(app)
	if got.MaxWindowWidth != 1600 || got.MaxWindowHeight != 1000 {
		t.Errorf("MaxWindowWidth/Height = %v/%v, want 1600/1000 (should survive a zero-value Save)", got.MaxWindowWidth, got.MaxWindowHeight)
	}
}

func TestSavePreferences_ZeroMemoryLimitsDoNotOverwritePreviouslySaved(t *testing.T) {
	app := test.NewApp()

	Save(app, State{MaxImageCacheMB: 384, MaxThumbCacheMB: 192, MaxFileSizeMB: 256})
	Save(app, State{MaxImageCacheMB: 0, MaxThumbCacheMB: 0, MaxFileSizeMB: 0})

	got := Load(app)
	if got.MaxImageCacheMB != 384 || got.MaxThumbCacheMB != 192 || got.MaxFileSizeMB != 256 {
		t.Errorf("memory limits = %d/%d/%d, want 384/192/256 (should survive a zero-value Save)",
			got.MaxImageCacheMB, got.MaxThumbCacheMB, got.MaxFileSizeMB)
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

func TestSavePreferences_FavoritePreviewCacheFalseRoundTrips(t *testing.T) {
	app := test.NewApp()

	Save(app, State{FavoritePreviewCache: false})

	if got := Load(app).FavoritePreviewCache; got {
		t.Error("FavoritePreviewCache = true, want false to round-trip (must be written unconditionally, unlike the zero-sentinel fields)")
	}
}

func TestSavePreferences_FavoritePreviewCacheTrueRoundTrips(t *testing.T) {
	app := test.NewApp()

	Save(app, State{FavoritePreviewCache: true})

	if got := Load(app).FavoritePreviewCache; !got {
		t.Error("FavoritePreviewCache = false, want true to round-trip")
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
