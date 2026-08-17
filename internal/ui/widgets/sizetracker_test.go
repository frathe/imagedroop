package widgets

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSizeTracker_RecordsTheWindowSizeOnEveryLayout(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	var size fyne.Size
	win.SetContent(container.New(NewSizeTracker(win, &size), widget.NewLabel("tracked")))

	win.Resize(fyne.NewSize(400, 300))

	// Deliberately the full window size rather than the laid-out content
	// size this Layout call is handed: the content sits inside the window's
	// padding, so recording it would shrink the persisted geometry by that
	// padding on every launch/save cycle.
	if want := fyne.NewSize(400, 300); size != want {
		t.Errorf("tracked size = %v, want %v (the full window size, not the padded content size)", size, want)
	}
	if got := win.Canvas().Size(); size != got {
		t.Errorf("tracked size = %v, want %v (what Window.Resize deals in)", size, got)
	}
}

func TestSizeTracker_LaysOutItsContentLikeAStack(t *testing.T) {
	win := test.NewWindow(nil)
	defer win.Close()

	label := widget.NewLabel("tracked")
	var size fyne.Size
	win.SetContent(container.New(NewSizeTracker(win, &size), label))

	win.Resize(fyne.NewSize(400, 300))

	if got := label.Size(); got.Width <= 0 || got.Height <= 0 {
		t.Errorf("content size = %v, want it laid out to fill the container", got)
	}
}
