// canSetWallpaper/setAsWallpaper (wallpaper.go): the File > "Set as
// Wallpaper" action, which writes the frame on screen into the app's own
// cache directory and points the OS at that copy.
//
// Per-OS dispatch (gsettings/plasma-apply-wallpaperimage/PowerShell/AppKit)
// is covered by internal/wallpaper's own tests; what's here is the viewer's
// side - the enable rule, the copy it hands over, the sweep of the file it
// replaces, and the error path.

package ui

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

// settleWallpaper waits out the goroutine setAsWallpaper runs on - wallpaper
// is finished once that goroutine has fully run, toast included, so
// reading widget state afterwards is race-free.
func settleWallpaper(t *testing.T, v *viewer) {
	t.Helper()

	if !v.wallpaper.Begun() {
		t.Fatal("no wallpaper goroutine pending to settle")
	}

	waitFor(t, "the wallpaper change", &v.wallpaper)
}

// wallpaperFiles lists what setAsWallpaper has left in the viewer's cache
// directory, which is a t.TempDir() in every test here.
func wallpaperFiles(t *testing.T, v *viewer) []string {
	t.Helper()

	found, err := filepath.Glob(filepath.Join(v.wallpaperDir, "wallpaper-*.png"))
	if err != nil {
		t.Fatalf("glob the wallpaper directory: %v", err)
	}
	return found
}

// --- canSetWallpaper -----------------------------------------------------

func TestCanSetWallpaper_FalseWithNoImage(t *testing.T) {
	v := newTestViewer(t)

	if v.canSetWallpaper() {
		t.Error("canSetWallpaper should be false with nothing loaded")
	}
}

func TestCanSetWallpaper_TrueForALoadedImage(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	if !v.canSetWallpaper() {
		t.Error("canSetWallpaper should be true once an image is loaded")
	}
}

// TestCanSetWallpaper_TrueForAFormatWithNoEncoder mirrors canExport's own
// case, and for the same reason: the copy this writes is a PNG whatever the
// source was, so a decode-only format is no obstacle.
func TestCanSetWallpaper_TrueForAFormatWithNoEncoder(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.webp", uitest.EncodeJPEG(t, 4, 4, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	if v.canSaveRotation() {
		t.Fatal("canSaveRotation should be false for .webp - the premise of this test")
	}
	if !v.canSetWallpaper() {
		t.Error("canSetWallpaper should be true for a format with no encoder of its own")
	}
}

// TestCanSetWallpaper_FalseWhileLoading mirrors canExport's own: mid-load
// v.img.Image still holds the previous file's pixels, and those are what
// would land on the desktop.
func TestCanSetWallpaper_FalseWhileLoading(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	v.loading.Store(true)
	t.Cleanup(func() { v.loading.Store(false) })

	if v.canSetWallpaper() {
		t.Error("canSetWallpaper should be false while a load is in flight")
	}
}

// --- setAsWallpaper ------------------------------------------------------

// TestSetAsWallpaper_PointsTheOSAtACopyOfTheFrameOnScreen is the whole
// feature in one test: what the OS is handed is a PNG this app wrote, not
// the user's own file, and it holds the frame as displayed - rotation
// included, which is why the source here is asymmetric and rotated first.
func TestSetAsWallpaper_PointsTheOSAtACopyOfTheFrameOnScreen(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 2, color.White)) // asymmetric
	dropAndWait(t, v, storage.NewFileURI(path))
	v.rotateBy(1) // 4x2 -> 2x4

	var got string
	uitest.StubWallpaperSet(t, func(p string) error {
		got = p
		return nil
	})

	v.setAsWallpaper()
	settleWallpaper(t, v)

	if got == path {
		t.Fatal("the OS was pointed at the source file itself, want a copy this app owns")
	}
	if dir := filepath.Dir(got); dir != v.wallpaperDir {
		t.Errorf("wallpaper written to %q, want it inside the app's own cache directory %q", dir, v.wallpaperDir)
	}

	written, err := loadExported(t, got)
	if err != nil {
		t.Fatalf("load the written wallpaper: %v", err)
	}
	if b := written.Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("wallpaper bounds = %v, want 2x4 (the rotation on screen carried into the file)", b)
	}

	settleToast(t, v) // a successful change toasts
}

// TestSetAsWallpaper_SweepsTheFileItReplaced keeps the cache directory from
// growing one PNG per invocation. The name carries a timestamp rather than
// being fixed because macOS caches the desktop picture by path and can
// ignore a same-path content change, so the sweep is what bounds it.
func TestSetAsWallpaper_SweepsTheFileItReplaced(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))
	uitest.StubWallpaperSet(t, func(string) error { return nil })

	v.setAsWallpaper()
	settleWallpaper(t, v)
	settleToast(t, v)
	first := wallpaperFiles(t, v)
	if len(first) != 1 {
		t.Fatalf("wallpaper files after one call = %v, want exactly 1", first)
	}

	v.setAsWallpaper()
	settleWallpaper(t, v)
	settleToast(t, v)

	second := wallpaperFiles(t, v)
	if len(second) != 1 {
		t.Errorf("wallpaper files after two calls = %v, want exactly 1 - the previous one should be swept", second)
	}
	if second[0] == first[0] {
		t.Errorf("second call reused %q, want a fresh name", second[0])
	}
	if _, err := os.Stat(first[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) error = %v, want the replaced file to be gone", first[0], err)
	}
}

// TestSetAsWallpaper_ReportsAFailure covers the Linux "no tool installed"
// case and every other refusal the OS can hand back: the file is written
// either way, so without a toast the user would have no sign at all that
// their desktop didn't change.
func TestSetAsWallpaper_ReportsAFailure(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))
	uitest.StubWallpaperSet(t, func(string) error { return errors.New("no such schema") })

	v.setAsWallpaper()
	settleWallpaper(t, v)

	if !v.toast.card.Visible() {
		t.Fatal("no toast shown after a failed wallpaper change")
	}
	if msg := v.toast.text.Text; !strings.Contains(msg, "no such schema") {
		t.Errorf("toast = %q, want it to carry the reason the change failed", msg)
	}

	settleToast(t, v)
}

func TestSetAsWallpaper_DoesNothingWithoutAnImage(t *testing.T) {
	v := newTestViewer(t)
	uitest.StubWallpaperSet(t, func(string) error {
		t.Error("the wallpaper should not be changed with nothing loaded")
		return nil
	})

	v.setAsWallpaper()

	if v.wallpaper.Begun() {
		t.Error("setAsWallpaper started a goroutine with nothing loaded")
	}
}

// --- menu state ----------------------------------------------------------

func TestUpdateFileMenuState_TracksTheWallpaperItem(t *testing.T) {
	v := newTestViewer(t)

	if !v.wallpaperItem.Disabled {
		t.Error("the Set as Wallpaper item should start disabled")
	}

	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))
	if v.wallpaperItem.Disabled {
		t.Error("the Set as Wallpaper item should be enabled once an image is loaded")
	}

	v.closeFiles()
	if !v.wallpaperItem.Disabled {
		t.Error("the Set as Wallpaper item should be disabled again once the files are closed")
	}
}
