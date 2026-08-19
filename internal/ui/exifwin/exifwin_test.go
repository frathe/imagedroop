package exifwin

import (
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/uitest"
)

func TestFormatExifMetadata(t *testing.T) {
	t.Run("every field set", func(t *testing.T) {
		m := imaging.Metadata{
			Make: "Canon", Model: "EOS 90D",
			LensModel:    "EF50mm f/1.8",
			ExposureTime: "1/200 s",
			FNumber:      "f/2.8",
			ISO:          "ISO 400",
			FocalLength:  "50 mm",
			DateTaken:    "2024-08-12 14:33:02",
		}

		want := "Camera: Canon EOS 90D\n" +
			"Lens: EF50mm f/1.8\n" +
			"Exposure: 1/200 s\n" +
			"Aperture: f/2.8\n" +
			"ISO: ISO 400\n" +
			"Focal length: 50 mm\n" +
			"Date taken: 2024-08-12 14:33:02"

		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("only some fields set", func(t *testing.T) {
		m := imaging.Metadata{Make: "Canon", ISO: "ISO 400"}

		want := "Camera: Canon\nISO: ISO 400"
		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("nothing set", func(t *testing.T) {
		want := "No EXIF metadata found in this file."
		if got := formatExifMetadata(imaging.Metadata{}); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("position set", func(t *testing.T) {
		m := imaging.Metadata{Make: "Canon", Latitude: 48.858222, Longitude: 2.2945, HasGPS: true}

		want := "Camera: Canon\nLatitude: 48.858222°\nLongitude: 2.294500°"
		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("southern and western hemispheres keep their sign", func(t *testing.T) {
		m := imaging.Metadata{Latitude: -33.856784, Longitude: -70.664247, HasGPS: true}

		want := "Latitude: -33.856784°\nLongitude: -70.664247°"
		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	// Null Island is a real position, and the only one a zero-valued
	// Metadata could be mistaken for - HasGPS is what tells them apart.
	t.Run("a zero position is still shown when it is a position", func(t *testing.T) {
		want := "Latitude: 0.000000°\nLongitude: 0.000000°"
		if got := formatExifMetadata(imaging.Metadata{HasGPS: true}); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("coordinates are left out without GPS", func(t *testing.T) {
		m := imaging.Metadata{Make: "Canon", Latitude: 48.858222, Longitude: 2.2945}

		want := "Camera: Canon"
		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})
}

// The panel needs a file to read before it will open at all (Show is a
// no-op with nothing displayed), so every geometry test below hands it one.
func testApp(t *testing.T) (fyne.App, func() (fyne.URI, bool)) {
	t.Helper()
	app := test.NewApp()
	u := uitest.TempJPEGURI(t, "exif.jpg", 8, 8, color.White)

	return app, func() (fyne.URI, bool) { return u, true }
}

func TestRestoreGeometry_OpensAtTheSavedGeometry(t *testing.T) {
	app, current := testApp(t)
	w := New(app, current)
	w.RestoreGeometry(widgets.Geometry{X: 310, Y: 320, PositionSet: true, Size: fyne.NewSize(520, 480)})

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	if got, want := w.Window().Canvas().Size(), fyne.NewSize(520, 480); got != want {
		t.Errorf("window size = %v, want the saved %v", got, want)
	}

	got := w.Geometry()
	if !got.PositionSet || got.X != 310 || got.Y != 320 {
		t.Errorf("Geometry() position = (%d, %d, set=%v), want the saved (310, 320, set=true)", got.X, got.Y, got.PositionSet)
	}
}

func TestGeometry_TracksAResizeAndOutlivesTheWindow(t *testing.T) {
	app, current := testApp(t)
	w := New(app, current)
	w.RestoreGeometry(widgets.Geometry{})

	w.Show()
	w.Window().Resize(fyne.NewSize(560, 500))
	w.Window().Close()

	if got, want := w.Geometry().Size, fyne.NewSize(560, 500); got != want {
		t.Errorf("Geometry().Size after closing = %v, want the last tracked %v", got, want)
	}
}

func TestStopTracking_IsSafeWithNoWindowOpen(t *testing.T) {
	app, current := testApp(t)

	New(app, current).StopTracking()
}

func TestShow_WithoutRestoreGeometryUsesTheBuiltInSize(t *testing.T) {
	app, current := testApp(t)
	w := New(app, current)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	if got, want := w.Window().Canvas().Size(), fyne.NewSize(exifW, exifH); got != want {
		t.Errorf("window size = %v, want the built-in %v", got, want)
	}
}

// gpsApp is testApp with a photo that carries GPS tags, for the map
// section's tests. The coordinates are the Eiffel Tower's.
func gpsApp(t *testing.T) (fyne.App, func() (fyne.URI, bool)) {
	t.Helper()
	app := test.NewApp()
	u := uitest.TempGPSJPEGURI(t, "gps.jpg", 8, 8, 48.858222, 2.2945)

	return app, func() (fyne.URI, bool) { return u, true }
}

func TestShow_LocationSectionIsShownCollapsedForAPhotoWithGPS(t *testing.T) {
	app, current := gpsApp(t)
	w := New(app, current)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	loc := w.Location()
	if loc == nil {
		t.Fatal("Location() = nil while the window is open")
	}

	if !loc.Visible() {
		t.Error("location section is hidden for a photo that has GPS tags, want shown")
	}

	if w.LocationExpanded() {
		t.Error("location section starts expanded, want collapsed until the user opens it")
	}

	if w.body.Visible() {
		t.Error("map is visible while the section is collapsed, want hidden")
	}
}

func TestShow_LocationSectionIsHiddenWithoutGPS(t *testing.T) {
	app, current := testApp(t) // a plain JPEG, no Exif at all
	w := New(app, current)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	if w.Location().Visible() {
		t.Error("location section is shown for a photo with no GPS tags, want hidden")
	}
}

func TestRefresh_LocationSectionFollowsTheCurrentImage(t *testing.T) {
	app := test.NewApp()

	withGPS := uitest.TempGPSJPEGURI(t, "gps.jpg", 8, 8, 48.858222, 2.2945)
	without := uitest.TempJPEGURI(t, "plain.jpg", 8, 8, color.White)

	shown := withGPS
	w := New(app, func() (fyne.URI, bool) { return shown, true })

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	if !w.Location().Visible() {
		t.Fatal("location section is hidden for the GPS photo, want shown")
	}

	shown = without
	w.Refresh()

	if w.Location().Visible() {
		t.Error("location section stayed visible after navigating to a photo with no GPS, want hidden")
	}

	shown = withGPS
	w.Refresh()

	if !w.Location().Visible() {
		t.Error("location section stayed hidden after navigating back to the GPS photo, want shown")
	}
}

func TestRefresh_LocationSectionIsHiddenForAnUnreadableFile(t *testing.T) {
	app := test.NewApp()

	missing := storage.NewFileURI(filepath.Join(t.TempDir(), "gone.jpg"))
	shown := uitest.TempGPSJPEGURI(t, "gps.jpg", 8, 8, 48.858222, 2.2945)

	w := New(app, func() (fyne.URI, bool) { return shown, true })

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	shown = missing
	w.Refresh()

	if w.Location().Visible() {
		t.Error("location section stayed visible for an unreadable file, want hidden")
	}

	if got, want := w.Text().Text, "Could not read this file's metadata."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// waitForWarm blocks until the prefetch the last expand started has
// finished, so a test can assert on the loading indicator without racing
// it. Deliberately a channel wait rather than polling widget state: the
// Fyne test driver runs fyne.Do inline, so widget state is written from the
// fetching goroutine.
func waitForWarm(t *testing.T, w *Window) {
	t.Helper()

	if w.warmDone == nil {
		t.Fatal("no prefetch has been started")
	}

	select {
	case <-w.warmDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the map prefetch")
	}
}

// The tile server is held for every stretch in which the test's own
// goroutine touches widgets: the Fyne test driver runs fyne.Do inline on
// the calling goroutine, so a tile landing mid-assertion would have a
// background goroutine repainting the map while this one reads it.
func TestToggleLocation_ShowsAndHidesTheMap(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)
	release := server.hold()

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.ToggleLocation()

	if !w.LocationExpanded() || !w.body.Visible() {
		t.Fatal("map is still hidden after expanding the section, want shown")
	}

	release()
	waitForWarm(t, w)

	w.ToggleLocation()

	if w.LocationExpanded() || w.body.Visible() {
		t.Error("map is still shown after collapsing the section, want hidden")
	}
}

func TestRefresh_IsANoOpWhileTheWindowIsClosed(t *testing.T) {
	app, current := gpsApp(t)
	w := New(app, current)

	w.Refresh() // must not panic on the nil label and nil map

	if w.Location() != nil {
		t.Error("Location() is non-nil with no window open")
	}
}

func TestToggleLocation_ShowsTheLoadingIndicatorUntilTheTilesAreIn(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)
	release := server.hold()
	t.Cleanup(release)

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	if w.loading.Visible() {
		t.Error("loading indicator is up before the section was ever expanded, want hidden")
	}

	w.ToggleLocation()

	if !w.loading.Visible() {
		t.Error("loading indicator is hidden while tiles are still downloading, want shown")
	}

	if w.locationMap.Visible() {
		t.Error("map is drawn while its tiles are still downloading, want it held back until they are in")
	}

	release()
	waitForWarm(t, w)

	if w.loading.Visible() {
		t.Error("loading indicator stayed up after the tiles arrived, want hidden")
	}

	if !w.locationMap.Visible() {
		t.Error("map is still hidden after its tiles arrived, want shown")
	}
}

func TestToggleLocation_FetchesNothingUntilTheSectionIsExpanded(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)
	release := server.hold()

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.Refresh()

	if got := server.count(); got != 0 {
		t.Fatalf("server saw %d requests with the section collapsed, want none", got)
	}

	w.ToggleLocation()
	release()
	waitForWarm(t, w)

	if server.count() == 0 {
		t.Error("server saw no requests after expanding the section, want the prefetch")
	}
}

func TestRefresh_ExpandedSectionRefetchesForANewPosition(t *testing.T) {
	app := test.NewApp()

	server := newTileServer(t)

	paris := uitest.TempGPSJPEGURI(t, "paris.jpg", 8, 8, 48.858222, 2.2945)
	sydney := uitest.TempGPSJPEGURI(t, "sydney.jpg", 8, 8, -33.856785, 151.215194)

	release := server.hold()

	shown := paris
	w := New(app, func() (fyne.URI, bool) { return shown, true })
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.ToggleLocation()
	release()
	waitForWarm(t, w)

	first := server.count()

	release = server.hold()

	shown = sydney
	w.Refresh()
	release()
	waitForWarm(t, w)

	if server.count() <= first {
		t.Error("navigating to a photo on the other side of the world fetched no new tiles")
	}

	if w.loading.Visible() {
		t.Error("loading indicator stayed up after the second prefetch, want hidden")
	}
}

func TestClose_StopsTheFetcherFromTouchingDeadWidgets(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)
	release := server.hold()

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	w.ToggleLocation()
	w.Window().Close()

	// The tiles land after the window is gone: the fetcher must find no
	// callback and the prefetch must find no map, rather than panicking on
	// either.
	release()
	waitForWarm(t, w)

	if w.Location() != nil {
		t.Error("Location() is non-nil after the window closed")
	}
}

func TestPaint_DoesNotBlockOnSlowTiles(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.Window().Resize(fyne.NewSize(exifW, exifH))
	w.ToggleLocation()
	waitForWarm(t, w)

	if w.locationMap.Size().IsZero() {
		t.Fatal("expanded map has no size, so this test would not be painting it at all")
	}

	// Panning off the prefetched block is the case that still reaches the
	// network from inside the widget's raster draw - which runs on the UI
	// goroutine, so before this package's tile plumbing existed a frame
	// like this froze the whole app for as long as the server took.
	release := server.hold()
	t.Cleanup(release)

	w.locationMap.PanEast()
	w.locationMap.PanEast()
	w.locationMap.PanEast()

	start := time.Now()
	w.Window().Canvas().Capture()
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("painting the map took %v while the tile server was hanging, want a prompt frame", elapsed)
	}
}

func TestToggleLocation_ExpandedMapGetsRealSpace(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.Window().Resize(fyne.NewSize(exifW, exifH))
	w.ToggleLocation()
	waitForWarm(t, w)

	// Revealing a child does not re-run its parent's layout by itself, and
	// a hidden child is given no space: without an explicit refresh the
	// map is "visible" at zero height and never drawn at all.
	if got := w.locationMap.Size(); got.Height < mapH {
		t.Errorf("expanded map size = %v, want at least %v tall", got, mapH)
	}
}

func TestToggleLocation_MapGrowsWithTheWindow(t *testing.T) {
	app, current := gpsApp(t)

	server := newTileServer(t)

	w := New(app, current)
	w.tiles = fetcherFor(server)

	w.Show()
	t.Cleanup(func() { w.Window().Close() })

	w.Window().Resize(fyne.NewSize(exifW, exifH))
	w.ToggleLocation()
	waitForWarm(t, w)

	before := w.locationMap.Size().Height

	w.Window().Resize(fyne.NewSize(exifW, exifH+300))

	// The map fills what the metadata above it leaves, so a taller window
	// is a taller map - the whole extra height, since nothing else in the
	// panel grows.
	if got := w.locationMap.Size().Height; got < before+300 {
		t.Errorf("map height after growing the window by 300 = %v, want at least %v", got, before+300)
	}
}
