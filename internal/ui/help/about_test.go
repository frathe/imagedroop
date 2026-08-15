package help

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// Unlike ShowManual (see TestManualIsEmbedded's neighbours below, and the
// comment above TestE2E_EscapeQuitsWhenNothingLoaded in internal/ui), the
// About window's RichText content is a single plain heading - not
// manual.md's mix of styles - so it doesn't hit the test theme's limited
// font-combination coverage, and can be exercised directly.
func TestShowAbout_OpensAndRaisesSameWindow(t *testing.T) {
	h := New(test.NewApp(), "Image Drop", nil)

	h.ShowAbout()

	win := h.aboutWin.Window()
	if win == nil {
		t.Fatal("ShowAbout did not open a window")
	}

	h.ShowAbout()

	if h.aboutWin.Window() != win {
		t.Error("a second ShowAbout call should raise the existing window, not open a new one")
	}

	win.Close()

	if h.aboutWin.Open() {
		t.Error("closing the About window should leave the singleton closed")
	}
}
