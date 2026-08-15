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

	"github.com/frathe/imagedrop/internal/filesort"
	"github.com/frathe/imagedrop/internal/preferences"
)

func TestBuildViewer_LoadsSavedPreferences(t *testing.T) {
	application := test.NewApp()
	preferences.Save(application, preferences.State{
		SortMode:          preferences.SortBySize,
		MergeMode:         true,
		SlideInterval:     7 * time.Second,
		SlideShuffle:      true,
		WindowSize:        fyne.NewSize(700, 500),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
	})

	v, win := buildViewer(application)
	defer win.Close()

	if v.sortMode != filesort.BySize {
		t.Errorf("sortMode = %v, want filesort.BySize (from saved preferences)", v.sortMode)
	}
	if !v.mergeMode {
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

	x, y, ok := v.winPos.Get()
	if !ok {
		t.Fatal("winPos has nothing recorded, want the saved position seeded into it")
	}
	if x != 120 || y != 340 {
		t.Errorf("winPos = (%d, %d), want the saved (120, 340)", x, y)
	}
}

func TestBuildViewer_NoSavedPreferencesUsesShippedDefaults(t *testing.T) {
	application := test.NewApp()

	v, win := buildViewer(application)
	defer win.Close()

	if v.sortMode != filesort.ByName {
		t.Errorf("sortMode = %v, want filesort.ByName (the shipped default)", v.sortMode)
	}
	if v.mergeMode {
		t.Error("mergeMode = true, want false (the shipped default)")
	}
	if got := v.slides.Interval(); got != 0 {
		t.Errorf("slides.Interval() = %v, want 0 (falls back to slideshow.DefaultInterval on first use)", got)
	}
	if v.slides.Shuffle() {
		t.Error("slides.Shuffle() = true, want false (the shipped default)")
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
