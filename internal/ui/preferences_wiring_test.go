// preferences_wiring_test.go covers the startup/build/run glue around
// internal/preferences: loading and normalizing a previously saved State,
// applying it to a fresh viewer/window, and keeping geometry current for
// shutdown. Persistence round trips and zero-value write guards are covered
// directly in internal/preferences.
package ui

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/preferences"
)

func TestStartup_LoadsSavedPreferencesIntoViewer(t *testing.T) {
	application := test.NewApp()
	preferences.Save(application, preferences.State{
		SortMode:          preferences.SortBySize,
		MergeMode:         true,
		SlideInterval:     7 * time.Second,
		SlideShuffle:      true,
		MaxScanFiles:      5000,
		MaxWindowWidth:    1800,
		MaxWindowHeight:   1100,
		MaxImageCacheMB:   384,
		MaxThumbCacheMB:   192,
		MaxFileSizeMB:     256,
		WindowSize:        fyne.NewSize(700, 500),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
	})

	v, win := buildStartupViewer(application)
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
	if got, want := v.MaxScan(), 5000; got != want {
		t.Errorf("MaxScan() = %d, want %d (from saved preferences)", got, want)
	}
	if got, want := v.MaxWindowWidth(), float32(1800); got != want {
		t.Errorf("MaxWindowWidth() = %v, want %v (from saved preferences)", got, want)
	}
	if got, want := v.MaxWindowHeight(), float32(1100); got != want {
		t.Errorf("MaxWindowHeight() = %v, want %v (from saved preferences)", got, want)
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

func TestStartup_LoadsSavedSecondaryWindowGeometry(t *testing.T) {
	application := test.NewApp()
	preferences.Save(application, preferences.State{
		SettingsWindow: preferences.WindowGeometry{
			X: 210, Y: 220, PositionSet: true, Size: fyne.NewSize(600, 520),
		},
		ExifWindow: preferences.WindowGeometry{
			X: 310, Y: 320, PositionSet: true, Size: fyne.NewSize(430, 370),
		},
	})

	v, win := buildStartupViewer(application)
	defer win.Close()
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	settings := v.settingsWin.Geometry()
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
// what startup restoration seeded above survives to the State that gets
// written.
func TestCurrentPreferences_CarriesSecondaryWindowGeometry(t *testing.T) {
	application := test.NewApp()
	saved := preferences.WindowGeometry{X: 70, Y: 80, PositionSet: true, Size: fyne.NewSize(500, 400)}
	preferences.Save(application, preferences.State{SettingsWindow: saved, ExifWindow: saved})

	v, win := buildStartupViewer(application)
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

func TestStartup_OmittedPreferencesUseShippedDefaults(t *testing.T) {
	application := test.NewApp()

	v, win := buildStartupViewer(application)
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
	if got, want := v.MaxScan(), defaultMaxScannedFiles; got != want {
		t.Errorf("MaxScan() = %d, want %d (the shipped default)", got, want)
	}
	if got, want := v.MaxWindowWidth(), float32(defaultMaxWindowWidth); got != want {
		t.Errorf("MaxWindowWidth() = %v, want %v (the shipped default)", got, want)
	}
	if got, want := v.MaxWindowHeight(), float32(defaultMaxWindowHeight); got != want {
		t.Errorf("MaxWindowHeight() = %v, want %v (the shipped default)", got, want)
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

func TestNormalizePreferenceDefaults(t *testing.T) {
	defaults := preferences.State{
		MaxScanFiles:    defaultMaxScannedFiles,
		MaxWindowWidth:  defaultMaxWindowWidth,
		MaxWindowHeight: defaultMaxWindowHeight,
		MaxImageCacheMB: defaultMaxImageCacheMB,
		MaxThumbCacheMB: defaultMaxThumbCacheMB,
		MaxFileSizeMB:   defaultMaxFileSizeMB,
	}
	custom := preferences.State{
		MaxScanFiles:    1,
		MaxWindowWidth:  1,
		MaxWindowHeight: 1,
		MaxImageCacheMB: 1,
		MaxThumbCacheMB: 1,
		MaxFileSizeMB:   1,
	}
	negative := preferences.State{
		MaxScanFiles:    -1,
		MaxWindowWidth:  -1,
		MaxWindowHeight: -1,
		MaxImageCacheMB: -1,
		MaxThumbCacheMB: -1,
		MaxFileSizeMB:   -1,
	}
	sentinels := preferences.State{
		WindowSize:        fyne.NewSize(0, 500),
		WindowPosX:        17,
		WindowPosY:        -23,
		WindowPositionSet: false,
		SettingsWindow: preferences.WindowGeometry{
			PositionSet: true,
		},
		ExifWindow: preferences.WindowGeometry{
			X: 31, Y: 32, Size: fyne.NewSize(430, 0),
		},
	}
	sentinelsWithDefaults := sentinels
	sentinelsWithDefaults.MaxScanFiles = defaultMaxScannedFiles
	sentinelsWithDefaults.MaxWindowWidth = defaultMaxWindowWidth
	sentinelsWithDefaults.MaxWindowHeight = defaultMaxWindowHeight
	sentinelsWithDefaults.MaxImageCacheMB = defaultMaxImageCacheMB
	sentinelsWithDefaults.MaxThumbCacheMB = defaultMaxThumbCacheMB
	sentinelsWithDefaults.MaxFileSizeMB = defaultMaxFileSizeMB

	for _, tc := range []struct {
		name  string
		input preferences.State
		want  preferences.State
	}{
		{name: "zero caps use defaults", want: defaults},
		{name: "negative caps use defaults", input: negative, want: defaults},
		{name: "positive caps survive", input: custom, want: custom},
		{name: "non-cap sentinels survive", input: sentinels, want: sentinelsWithDefaults},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizePreferenceDefaults(tc.input); got != tc.want {
				t.Errorf("normalizePreferenceDefaults() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func funcName(f func()) string {
	if f == nil {
		return "<nil>"
	}
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

func TestStartViewerRuntime_ReplacesConstructionStopAfterGeometryRestoration(t *testing.T) {
	application := test.NewApp()
	settingsGeometry := preferences.WindowGeometry{
		X: 210, Y: 220, PositionSet: true, Size: fyne.NewSize(600, 520),
	}
	exifGeometry := preferences.WindowGeometry{
		X: 310, Y: 320, PositionSet: true, Size: fyne.NewSize(430, 370),
	}
	preferences.Save(application, preferences.State{
		WindowSize:        fyne.NewSize(700, 500),
		WindowPosX:        120,
		WindowPosY:        340,
		WindowPositionSet: true,
		SettingsWindow:    settingsGeometry,
		ExifWindow:        exifGeometry,
	})

	favoritesDir := t.TempDir()
	if err := favstore.Save(favoritesDir, "Runtime Favorite", nil); err != nil {
		t.Fatalf("save temporary favorite: %v", err)
	}

	v, win := buildStartupViewer(application)
	t.Cleanup(win.Close)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	if v.stopWinPosPoll == nil {
		t.Fatal("buildStartupViewer left stopWinPosPoll nil")
	}
	noPollerStopName := funcName(noPollerStop)
	if got := funcName(v.stopWinPosPoll); got != noPollerStopName {
		t.Fatalf("buildStartupViewer stop callback = %s, want noPollerStop %s", got, noPollerStopName)
	}

	if got, want := win.Canvas().Size(), fyne.NewSize(700, 500); got != want {
		t.Errorf("main window size = %v, want restored %v", got, want)
	}
	x, y, positionSet := v.winPos.Get()
	if !positionSet || x != 120 || y != 340 {
		t.Errorf("main window position = (%d, %d, set=%v), want restored (120, 340, set=true)", x, y, positionSet)
	}
	if got := prefGeometry(v.settingsWin.Geometry()); got != settingsGeometry {
		t.Errorf("settings geometry = %+v, want restored %+v", got, settingsGeometry)
	}
	if got := prefGeometry(v.exif.Geometry()); got != exifGeometry {
		t.Errorf("EXIF geometry = %+v, want restored %+v", got, exifGeometry)
	}

	startViewerRuntime(v, win, favoritesDir)
	runtimeStop := v.stopWinPosPoll
	if runtimeStop == nil {
		t.Fatal("startViewerRuntime left stopWinPosPoll nil")
	}
	t.Cleanup(runtimeStop)
	if got := funcName(runtimeStop); got == noPollerStopName {
		t.Fatalf("startViewerRuntime left noPollerStop installed (%s)", got)
	}

	// A prefix rather than the whole label: this test is about the runtime
	// directory reaching the menu at all, and the favorites package owns how
	// an item is worded (it carries the favorite's stored file count).
	items := v.favorites.Menu().Items
	if len(items) != 5 || !strings.HasPrefix(items[2].Label, "Runtime Favorite") {
		t.Errorf("favorites menu items = %+v, want the temporary favorite", items)
	}
}

func TestWindowSizeTracker_RecordsResizes(t *testing.T) {
	application := test.NewApp()

	v, win := buildStartupViewer(application)
	defer win.Close()

	win.Resize(fyne.NewSize(900, 650))

	if got, want := v.windowSize, fyne.NewSize(900, 650); got != want {
		t.Errorf("windowSize after resize = %v, want %v", got, want)
	}
}

// TestFavoritePreviewCache_DefaultsToTrueOnStartup checks the startup-restore
// path: preferences.Load already defaults FavoritePreviewCache to true (Stage
// 6a), so a viewer built with nothing ever saved must read the same default
// once features.go wires it through.
func TestFavoritePreviewCache_DefaultsToTrueOnStartup(t *testing.T) {
	v := newTestViewer(t)

	if !v.FavoritePreviewCache() {
		t.Error("FavoritePreviewCache() = false, want true (the shipped default)")
	}
}

// TestCurrentPreferences_DefaultFavoritePreviewCacheIsTrue guards the trap
// documented in favorites_disk_thumbnail_cache.md's Stage 6b: Save writes
// FavoritePreviewCache unconditionally, so any currentPreferences() literal
// that forgets to set it from the viewer would carry the zero value, false,
// straight to disk on every shutdown.
func TestCurrentPreferences_DefaultFavoritePreviewCacheIsTrue(t *testing.T) {
	v := newTestViewer(t)

	if !v.currentPreferences().FavoritePreviewCache {
		t.Error("currentPreferences().FavoritePreviewCache = false, want true - run.go's literal must set it from the viewer")
	}
}

// TestSetFavoritePreviewCache_UpdatesGetterAndCurrentPreferences checks the
// settings window's binding end to end: a user turning the checkbox off must
// be reflected both immediately (FavoritePreviewCache) and at the next
// shutdown save (currentPreferences).
func TestSetFavoritePreviewCache_UpdatesGetterAndCurrentPreferences(t *testing.T) {
	v := newTestViewer(t)

	v.SetFavoritePreviewCache(false)

	if v.FavoritePreviewCache() {
		t.Error("FavoritePreviewCache() = true after SetFavoritePreviewCache(false)")
	}
	if v.currentPreferences().FavoritePreviewCache {
		t.Error("currentPreferences().FavoritePreviewCache = true after SetFavoritePreviewCache(false)")
	}
}
