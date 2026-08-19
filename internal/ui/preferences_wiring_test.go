// preferences_wiring_test.go covers the build.go/run.go glue around
// internal/preferences: buildViewer applying a previously saved State to a
// fresh viewer/window, and windowSizeTracker keeping viewer.windowSize
// current so main() has something accurate to save at shutdown. The
// persistence logic itself (Save/Load round-tripping, zero-value guards) is
// covered directly in internal/preferences.
package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/preferences"
)

func TestBuildViewer_LoadsSavedPreferences(t *testing.T) {
	application := test.NewApp()
	preferences.Save(application, preferences.State{
		SortMode:          preferences.SortBySize,
		MergeMode:         true,
		SlideInterval:     7 * time.Second,
		SlideShuffle:      true,
		MaxImageCacheMB:   384,
		MaxThumbCacheMB:   192,
		MaxFileSizeMB:     256,
		WindowSize:        fyne.NewSize(700, 500),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
	})

	v, win := buildViewer(application)
	defer win.Close()
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	if v.state.SortMode() != filesort.BySize {
		t.Errorf("sortMode = %v, want filesort.BySize (from saved preferences)", v.state.SortMode())
	}
	if !v.state.MergeMode() {
		t.Error("mergeMode = false, want true (from saved preferences)")
	}
	if got, want := v.slides.Interval(), 7*time.Second; got != want {
		t.Errorf("slides.Interval() = %v, want %v", got, want)
	}
	if !v.slides.Shuffle() {
		t.Error("slides.Shuffle() = false, want true (from saved preferences)")
	}
	if got, want := win.Canvas().Size(), fyne.NewSize(700, 500); got != want {
		t.Errorf("initial window size = %v, want %v", got, want)
	}

	// The three memory limits reach three different places (memlimits.go):
	// the image cache's own budget, the grid's, and process-wide state in
	// internal/imaging - so each is checked where it actually landed rather
	// than only on the viewer's bookkeeping field.
	if got, want := v.MaxImageCacheMB(), 384; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want %d (from saved preferences)", got, want)
	}
	if got, want := v.imgCache.Budget(), int64(384*bytesPerMB); got != want {
		t.Errorf("imgCache.Budget() = %d, want %d", got, want)
	}
	if got, want := v.MaxThumbCacheMB(), 192; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want %d (from saved preferences)", got, want)
	}
	if got, want := v.MaxFileSizeMB(), 256; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want %d (from saved preferences)", got, want)
	}
	if got, want := imaging.MaxEncodedBytes(), int64(256*bytesPerMB); got != want {
		t.Errorf("imaging.MaxEncodedBytes() = %d, want %d", got, want)
	}

	x, y, ok := v.winPos.Get()
	if !ok {
		t.Fatal("winPos has nothing recorded, want the saved position seeded into it")
	}
	if x != 120 || y != 340 {
		t.Errorf("winPos = (%d, %d), want the saved (120, 340)", x, y)
	}
}

func TestBuildViewer_LoadsSavedSecondaryWindowGeometry(t *testing.T) {
	application := test.NewApp()
	preferences.Save(application, preferences.State{
		SettingsWindow: preferences.WindowGeometry{
			X: 210, Y: 220, PositionSet: true, Size: fyne.NewSize(600, 520),
		},
		ExifWindow: preferences.WindowGeometry{
			X: 310, Y: 320, PositionSet: true, Size: fyne.NewSize(430, 370),
		},
	})

	v, win := buildViewer(application)
	defer win.Close()
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	settings := v.settings.Geometry()
	if !settings.PositionSet || settings.X != 210 || settings.Y != 220 {
		t.Errorf("settings position = (%d, %d, set=%v), want the saved (210, 220, set=true)", settings.X, settings.Y, settings.PositionSet)
	}
	if got, want := settings.Size, fyne.NewSize(600, 520); got != want {
		t.Errorf("settings size = %v, want the saved %v", got, want)
	}

	exif := v.exif.Geometry()
	if !exif.PositionSet || exif.X != 310 || exif.Y != 320 {
		t.Errorf("exif position = (%d, %d, set=%v), want the saved (310, 320, set=true)", exif.X, exif.Y, exif.PositionSet)
	}
	if got, want := exif.Size, fyne.NewSize(430, 370); got != want {
		t.Errorf("exif size = %v, want the saved %v", got, want)
	}
}

// The shutdown save (Run's SetOnStopped) is what has to carry both windows'
// geometry back out again - a round trip that is only worth anything if
// what buildViewer seeded above survives to the State that gets written.
func TestCurrentPreferences_CarriesSecondaryWindowGeometry(t *testing.T) {
	application := test.NewApp()
	saved := preferences.WindowGeometry{X: 70, Y: 80, PositionSet: true, Size: fyne.NewSize(500, 400)}
	preferences.Save(application, preferences.State{SettingsWindow: saved, ExifWindow: saved})

	v, win := buildViewer(application)
	defer win.Close()
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	got := v.currentPreferences()
	if got.SettingsWindow != saved {
		t.Errorf("SettingsWindow = %+v, want %+v", got.SettingsWindow, saved)
	}
	if got.ExifWindow != saved {
		t.Errorf("ExifWindow = %+v, want %+v", got.ExifWindow, saved)
	}
}

func TestBuildViewer_NoSavedPreferencesUsesShippedDefaults(t *testing.T) {
	application := test.NewApp()

	v, win := buildViewer(application)
	defer win.Close()

	if v.state.SortMode() != filesort.ByName {
		t.Errorf("sortMode = %v, want filesort.ByName (the shipped default)", v.state.SortMode())
	}
	if v.state.MergeMode() {
		t.Error("mergeMode = true, want false (the shipped default)")
	}
	if got := v.slides.Interval(); got != 0 {
		t.Errorf("slides.Interval() = %v, want 0 (falls back to slideshow.DefaultInterval on first use)", got)
	}
	if v.slides.Shuffle() {
		t.Error("slides.Shuffle() = true, want false (the shipped default)")
	}
	if got, want := v.MaxImageCacheMB(), defaultMaxImageCacheMB; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want %d (the shipped default)", got, want)
	}
	if got, want := v.MaxThumbCacheMB(), defaultMaxThumbCacheMB; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want %d (the shipped default)", got, want)
	}
	if got, want := v.MaxFileSizeMB(), defaultMaxFileSizeMB; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want %d (the shipped default)", got, want)
	}
	if got, want := win.Canvas().Size(), fyne.NewSize(startW, startH); got != want {
		t.Errorf("initial window size = %v, want %v (startW/startH)", got, want)
	}
	if _, _, ok := v.winPos.Get(); ok {
		t.Error("winPos has a position recorded, want none with nothing saved")
	}
}

func TestWindowSizeTracker_RecordsResizes(t *testing.T) {
	application := test.NewApp()

	v, win := buildViewer(application)
	defer win.Close()

	win.Resize(fyne.NewSize(900, 650))

	if got, want := v.windowSize, fyne.NewSize(900, 650); got != want {
		t.Errorf("windowSize after resize = %v, want %v", got, want)
	}
}
