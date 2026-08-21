package ui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file owns the info overlay that the I key toggles
// (internal/ui/info.go): its content - filename and position, pixel
// dimensions, file size, zoom level - that it stays current across a
// navigation instead of freezing on whatever the first image showed, and
// that the zoom line tracks every zoom mutator (ActualSize, In,
// FitToWindow), not just the value at load time. The I preference itself
// is a standing one, like naturalSort/mergeMode:
// TestClearToDropzone_HidesInfoCardButKeepsThePreference checks it
// survives a reset even though the card itself is one of the things that
// reset hides. TestFormatFileSize is the pure byte-count formatting
// helper underneath the card's text.

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1500, "1.5 KiB"},
		{1_048_576, "1.0 MiB"},
		{1_500_000, "1.4 MiB"},
		{1_073_741_824, "1.0 GiB"},
	}

	for _, c := range cases {
		if got := formatFileSize(c.n); got != c.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded guards against I turning
// the card on before there's anything for it to describe: pressed before
// the first drop (allowed, like M/P - see handleKeyEvent), the preference
// should be recorded but the card must stay hidden until an image actually
// loads.
func TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded(t *testing.T) {
	v := newTestViewer(t)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyI})
	if !v.infoVisible {
		t.Fatal("infoVisible should flip on right away, even with nothing loaded")
	}
	if v.infoCard.Visible() {
		t.Fatal("infoCard should stay hidden until an image is actually on screen")
	}

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	if !v.infoCard.Visible() {
		t.Error("infoCard should appear once the first image loads, since the toggle was already on")
	}
}

// TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation covers the
// card's actual content (filename+position, pixel dimensions, file size,
// zoom) and that it keeps itself current across a navigation instead of
// freezing on whatever the first image showed.
func TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 80, 10, color.White)
	dropAndWait(t, v, a, b)

	v.toggleInfoOverlay()
	if !v.infoCard.Visible() {
		t.Fatal("infoCard should be visible right after toggling on with an image already loaded")
	}

	aInfo, err := os.Stat(a.Path())
	if err != nil {
		t.Fatalf("stat a.jpg: %v", err)
	}
	// The zoom line's own value is whatever fit scale the test window's
	// size works out to, so it's read back rather than pinned: what this
	// test is about is the card's content and that it stays current. The
	// fit math itself is internal/ui/zoom's to test, against a viewport it
	// can actually control.
	want := fmt.Sprintf("a.jpg  (1/2)\n40 x 20\n%s\nZoom: %d%%", formatFileSize(aInfo.Size()), v.zoom.Percent())
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText = %q, want %q", got, want)
	}

	// Step to the second file: the card must refresh, not keep showing a's
	// info.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)
	v.updateInfoOverlay()

	bInfo, err := os.Stat(b.Path())
	if err != nil {
		t.Fatalf("stat b.jpg: %v", err)
	}
	want = fmt.Sprintf("b.jpg  (2/2)\n80 x 10\n%s\nZoom: %d%%", formatFileSize(bInfo.Size()), v.zoom.Percent())
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText after navigating = %q, want %q", got, want)
	}

	// Toggling off hides it; toggling back on immediately re-shows current info.
	v.toggleInfoOverlay()
	if v.infoCard.Visible() {
		t.Fatal("infoCard should hide once toggled off")
	}
	v.toggleInfoOverlay()
	if !v.infoCard.Visible() {
		t.Fatal("infoCard should reappear once toggled back on")
	}
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText after re-enabling = %q, want %q (still on b.jpg)", got, want)
	}
}

// TestToggleInfoOverlay_ZoomLineTracksZoomChanges checks the last line
// updates with every zoom mutator (ActualSize, In, FitToWindow), not just
// at load time.
func TestToggleInfoOverlay_ZoomLineTracksZoomChanges(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 400, 200, color.White)
	dropAndWait(t, v, a)

	// The fit percentage depends on the test window's size, so it's read
	// back once here and used as the anchor the last step returns to.
	// Actual size (100%) is the one value that's the same everywhere.
	fitPct := fmt.Sprintf("Zoom: %d%%", v.zoom.Percent())

	v.toggleInfoOverlay()

	if !strings.HasSuffix(v.infoText.Text, fitPct) {
		t.Errorf("infoText = %q, want it to end with the %q fit scale", v.infoText.Text, fitPct)
	}

	v.zoom.ActualSize()
	if !strings.HasSuffix(v.infoText.Text, "Zoom: 100%") {
		t.Errorf("infoText after ActualSize = %q, want it to end with 100%%", v.infoText.Text)
	}

	v.zoom.In()
	if v.zoom.Percent() <= 100 {
		t.Fatalf("setup: zoom percent after In = %d, want more than 100", v.zoom.Percent())
	}
	want := fmt.Sprintf("Zoom: %d%%", v.zoom.Percent())
	if !strings.HasSuffix(v.infoText.Text, want) {
		t.Errorf("infoText after In = %q, want it to end with %q", v.infoText.Text, want)
	}

	v.zoom.FitToWindow()
	if !strings.HasSuffix(v.infoText.Text, fitPct) {
		t.Errorf("infoText after FitToWindow = %q, want back to %q", v.infoText.Text, fitPct)
	}
}

// TestClearToDropzone_HidesInfoCardButKeepsThePreference guards the reset
// (Escape) path: the card must disappear along with the image, but the I
// preference itself is a standing one - like naturalSort/mergeMode - so a
// fresh drop afterward should bring the card straight back.
func TestClearToDropzone_HidesInfoCardButKeepsThePreference(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)
	v.toggleInfoOverlay()

	v.reset()

	if !v.infoVisible {
		t.Error("infoVisible preference should survive a reset")
	}
	if v.infoCard.Visible() {
		t.Error("infoCard should be hidden once reset back to the empty drop zone")
	}

	b := uitest.TempJPEGURI(t, "b.jpg", 40, 20, color.White)
	dropAndWait(t, v, b)

	if !v.infoCard.Visible() {
		t.Error("infoCard should reappear on the next load since the preference was still on")
	}
}
