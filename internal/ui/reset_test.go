package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file owns reset itself: viewer.go's Escape-triggered "start over",
// which with files loaded wipes the session back to exactly the state the
// viewer launched in - no files, no image, welcome art and the drop zone
// showing again, window back to its start size and title. reset is not
// clearToDropzone - it calls clearToDropzone and then goes further,
// clearing any in-flight vector rasterization, restoring the launch-time
// welcome look, and forcing a repaint - see both functions in viewer.go.
//
// What a reset does to one particular feature's own state lives with that
// feature instead of here: TestViewerReset_ReshowsRestoreLinkWhenSessionUnconsumed
// and TestViewerReset_DoesNotReshowRestoreLinkOnceConsumed are in
// session_test.go, TestClearToDropzone_HidesInfoCardButKeepsThePreference is
// in info_test.go, and TestClearToDropzone_PurgesTheImageCache is in
// imgcache_test.go. What stays here is the reset itself.

func TestViewerReset(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	dropAndWait(t, v, jpegURI)

	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded before reset")
	}

	v.reset()

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after reset", v.state.files)
	}
	if v.state.index != 0 {
		t.Errorf("index = %d, want 0 after reset", v.state.index)
	}
	if v.img.Image != nil {
		t.Error("image should be cleared after reset")
	}
	if v.img.Visible() {
		t.Error("image should be hidden after reset")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again after reset")
	}
	if !v.welcomeArt.Visible() {
		t.Error("welcomeArt should be visible again after reset, matching the just-launched state")
	}
	if v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be hidden after reset")
	}
	if got, want := v.hint.Text, lang.L("Drop images here"); got != want {
		t.Errorf("hint text = %q, want %q after reset", got, want)
	}
	if size := v.win.Canvas().Size(); !uitest.ApproxEqual(size.Width, startW) || !uitest.ApproxEqual(size.Height, startH) {
		t.Errorf("window size = %v, want %vx%v after reset", size, startW, startH)
	}
}
