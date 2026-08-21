package ui

import (
	"image"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/uitest"
)

// resizeToImage: the window growing to fit a newly loaded image, and the
// user-configurable cap - MaxWindowWidth/MaxWindowHeight - that bounds how
// far it is allowed to grow, together with the settings window's getters and
// setters and the floor that keeps a too-small custom cap from silently
// doing nothing.
//
// resizeToImage itself is shared: finishLoad calls it when an image arrives,
// syncWindowToZoom when the user changes zoom, and applyRotationLayout when
// the frame turns. Only the geometry it computes is pinned here. The other
// callers are tested against their own reasons for calling it, in
// zoom_test.go and rotate_test.go.

func TestResizeToImage(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		maxW, maxH   float32
		wantW, wantH float32
	}{
		{"small image is floored to the drop-zone min size", 400, 300, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, startH},
		{"image already exactly at the cap", int(defaultMaxWindowWidth), int(defaultMaxWindowHeight), defaultMaxWindowWidth, defaultMaxWindowHeight, defaultMaxWindowWidth, defaultMaxWindowHeight},
		{"wide image is capped by width", 3000, 950, defaultMaxWindowWidth, defaultMaxWindowHeight, 1500, 475},
		{"tall image is capped by height, then floored to the min width", 950, 3000, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, 950},
		{"large image is capped by whichever dimension needs it most", 3000, 2000, defaultMaxWindowWidth, defaultMaxWindowHeight, 1425, 950},
		{"tiny image is floored to the drop-zone size, not left ungrabbable", 50, 50, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, startH},
		{"a custom, smaller cap is honored instead of the shipped default", 3000, 2000, 900, 700, 900, 600},
		{"a custom, larger cap is honored instead of the shipped default", 3000, 2000, 2800, 2000, 2800, 1866.6667},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			win := test.NewWindow(nil)
			defer win.Close()

			resizeToImage(win, image.Rect(0, 0, c.w, c.h), c.maxW, c.maxH)

			size := win.Canvas().Size()
			if !uitest.ApproxEqual(size.Width, c.wantW) || !uitest.ApproxEqual(size.Height, c.wantH) {
				t.Errorf("resizeToImage(%dx%d, max %vx%v) -> %vx%v, want %vx%v", c.w, c.h, c.maxW, c.maxH, size.Width, size.Height, c.wantW, c.wantH)
			}
		})
	}
}

// TestMaxWindowSizeGetterSetter is MaxWindowWidth/SetMaxWindowWidth and
// MaxWindowHeight/SetMaxWindowHeight - the settings window's binding for
// the cap resizeToImage enforces (tested directly above).
func TestMaxWindowSizeGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if got := v.MaxWindowWidth(); got != defaultMaxWindowWidth {
		t.Errorf("MaxWindowWidth() = %v, want the shipped default %v", got, defaultMaxWindowWidth)
	}
	if got := v.MaxWindowHeight(); got != defaultMaxWindowHeight {
		t.Errorf("MaxWindowHeight() = %v, want the shipped default %v", got, defaultMaxWindowHeight)
	}

	v.SetMaxWindowWidth(1800)
	v.SetMaxWindowHeight(1100)
	if got := v.MaxWindowWidth(); got != 1800 {
		t.Errorf("MaxWindowWidth() = %v, want 1800 after SetMaxWindowWidth(1800)", got)
	}
	if got := v.MaxWindowHeight(); got != 1100 {
		t.Errorf("MaxWindowHeight() = %v, want 1100 after SetMaxWindowHeight(1100)", got)
	}
}

// TestSetMaxWindowSize_FloorsAtTheDropZoneSize guards against a cap below
// startW/startH: resizeToImage never shrinks the window past that regardless
// (see its own "never shrink below the drop-zone size" comment), so a lower
// value would silently have no effect - the setters floor instead, so what
// the settings window shows always matches what the window actually does.
func TestSetMaxWindowSize_FloorsAtTheDropZoneSize(t *testing.T) {
	v := newTestViewer(t)

	v.SetMaxWindowWidth(100)
	v.SetMaxWindowHeight(50)

	if got := v.MaxWindowWidth(); got != startW {
		t.Errorf("MaxWindowWidth() = %v, want it floored to startW (%v)", got, float32(startW))
	}
	if got := v.MaxWindowHeight(); got != startH {
		t.Errorf("MaxWindowHeight() = %v, want it floored to startH (%v)", got, float32(startH))
	}
}
