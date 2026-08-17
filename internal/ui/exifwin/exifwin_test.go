package exifwin

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
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
