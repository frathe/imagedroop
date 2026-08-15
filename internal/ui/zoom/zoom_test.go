package zoom

import (
	"image"
	"math"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/imagedrop/internal/uitest"
)

func TestMain(m *testing.M) {
	// The widget is a real fyne widget and the image a real canvas.Image,
	// so both want a driver behind them. No window: everything below drives
	// the component directly.
	test.NewApp()
	os.Exit(m.Run())
}

// newZoom builds a Zoom over an image of the given native pixel size (0x0
// for "nothing loaded yet") laid out in a viewport of the given size, and
// reports how many times the onChanged callback fired.
func newZoom(t *testing.T, native image.Rectangle, viewport fyne.Size) (*Zoom, *int) {
	t.Helper()

	img := canvas.NewImageFromImage(nil)
	if !native.Empty() {
		img.Image = image.NewRGBA(native)
	}

	changes := 0
	z := New(img, func() { changes++ }, nil)

	// Through the widget rather than by setting z.viewport, since that is
	// the only path production has: the renderer's Layout is what caches
	// the viewport, and it applies the layout on the way through.
	z.Widget().Resize(viewport)

	return z, &changes
}

// pointUnderCursor returns the native-pixel coordinate currently displayed
// at the given cursor position, using the same layout math as apply. Tests
// use it to check that a scroll-zoom keeps the point under the cursor fixed
// on screen, without duplicating at's own formula.
func pointUnderCursor(z *Zoom, cursor fyne.Position) (float32, float32) {
	b := z.img.Image.Bounds()
	scale := z.scale
	if z.fit {
		scale = z.fitScale()
	}
	scaled := fyne.NewSize(float32(b.Dx())*scale, float32(b.Dy())*scale)
	pos := z.originFor(scaled, z.pan)

	return (cursor.X - pos.X) / scale, (cursor.Y - pos.Y) / scale
}

// --- clampPanAxis -----------------------------------------------------------

func TestClampPanAxis(t *testing.T) {
	cases := []struct {
		name                     string
		offset, scaled, viewport float32
		want                     float32
	}{
		{"image smaller than viewport pins to center", 50, 100, 200, 0},
		{"image exactly viewport size pins to center", 999, 200, 200, 0},
		{"image within panSlack of viewport size still pins to center", 999, 200.2, 200, 0},
		{"offset within range is untouched", 10, 400, 200, 10},
		{"offset beyond the positive limit is clamped", 999, 400, 200, 100},
		{"offset beyond the negative limit is clamped", -999, 400, 200, -100},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clampPanAxis(c.offset, c.scaled, c.viewport); got != c.want {
				t.Errorf("clampPanAxis(%v, %v, %v) = %v, want %v", c.offset, c.scaled, c.viewport, got, c.want)
			}
		})
	}
}

// --- apply -------------------------------------------------------------------

func TestApply_FitFillsViewportAndResetsPan(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 600))
	z.pan = fyne.NewPos(50, -30) // should be wiped by fit mode

	z.apply()

	if got := z.img.Size(); !uitest.ApproxEqual(got.Width, 800) || !uitest.ApproxEqual(got.Height, 600) {
		t.Errorf("size = %v, want the full 800x600 viewport while fitting", got)
	}
	if got := z.img.Position(); got != fyne.NewPos(0, 0) {
		t.Errorf("position = %v, want (0,0) while fitting", got)
	}
	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want zero while fitting", z.pan)
	}
}

func TestApply_NoImageBehavesLikeFit(t *testing.T) {
	z, _ := newZoom(t, image.Rectangle{}, fyne.NewSize(800, 600))
	z.fit = false // even set to manual zoom, nothing to zoom into yet
	z.scale = 3

	z.apply()

	if got := z.img.Size(); !uitest.ApproxEqual(got.Width, 800) || !uitest.ApproxEqual(got.Height, 600) {
		t.Errorf("size = %v, want the full viewport with no image loaded", got)
	}
}

func TestApply_ManualScaleCentersImage(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 600))
	z.fit = false
	z.scale = 2 // -> 800x400, exactly as wide as the viewport, shorter

	z.apply()

	if got := z.img.Size(); !uitest.ApproxEqual(got.Width, 800) || !uitest.ApproxEqual(got.Height, 400) {
		t.Errorf("size = %v, want 800x400", got)
	}
	if got := z.img.Position(); !uitest.ApproxEqual(got.X, 0) || !uitest.ApproxEqual(got.Y, 100) {
		t.Errorf("position = %v, want (0,100) - vertically centered", got)
	}
}

func TestApply_ClampsPanWithinViewport(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.fit = false
	z.scale = 4 // -> 400x400, bigger than the viewport on both axes
	z.pan = fyne.NewPos(10000, -10000)

	z.apply()

	const limit = float32(100) // (400-200)/2
	if z.pan.X != limit || z.pan.Y != -limit {
		t.Errorf("pan = %v, want clamped to (%v, %v)", z.pan, limit, -limit)
	}
}

// --- panBy -------------------------------------------------------------------

func TestPanBy_UpdatesOffsetAndReflowsImage(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.fit = false
	z.scale = 4 // -> 400x400 scaled, room to pan on both axes

	z.panBy(fyne.NewDelta(20, -5))

	if z.pan.X != 20 || z.pan.Y != -5 {
		t.Errorf("pan = %v, want (20,-5)", z.pan)
	}

	wantX, wantY := float32(-80), float32(-105) // (200-400)/2 +/- the drag
	if pos := z.img.Position(); !uitest.ApproxEqual(pos.X, wantX) || !uitest.ApproxEqual(pos.Y, wantY) {
		t.Errorf("img position = %v, want (%v, %v)", pos, wantX, wantY)
	}
}

func TestPanBy_NoopAtFitScale(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))

	z.panBy(fyne.NewDelta(50, 50))

	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want zero - dragging at fit scale should do nothing", z.pan)
	}
}

// --- In / Out / ActualSize / FitToWindow / ResetToFit -----------------------

func TestIn_StartsFromCurrentFitScale(t *testing.T) {
	z, changes := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400)) // fit scale = min(2,2) = 2

	z.In()

	if z.Fitting() {
		t.Error("In should switch out of fit mode")
	}
	if want := float32(2) * step; !uitest.ApproxEqual(z.scale, want) {
		t.Errorf("scale = %v, want %v (fit scale x step)", z.scale, want)
	}
	if *changes != 1 {
		t.Errorf("onChanged fired %d times, want 1", *changes)
	}
}

func TestInOut_ClampToMinAndMax(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))
	z.fit = false

	z.scale = maxScale
	z.In()
	if z.scale != maxScale {
		t.Errorf("scale = %v, want clamped to max %v", z.scale, maxScale)
	}

	z.scale = minScale
	z.Out()
	if z.scale != minScale {
		t.Errorf("scale = %v, want clamped to min %v", z.scale, minScale)
	}
}

// TestInThenOutLeavesNothingToPan guards against float32 rounding noise: In
// followed by Out multiplies then divides by the same value, which doesn't
// always land back on exactly the original float32 bit pattern (a/b*b != a
// in general for floats). Without panSlack absorbing that, a
// zoom-in-then-out could leave the image a fraction of a point larger than
// the viewport and wrongly register as still pannable - which is exactly
// what was reported: zoom in once, zoom out once, and the grab cursor
// stayed on even though the image was back to filling the window exactly.
func TestInThenOutLeavesNothingToPan(t *testing.T) {
	natives := [][2]int{{400, 200}, {377, 10}, {2837, 953}, {4000, 3}, {33, 4001}, {1, 1}}
	viewports := []fyne.Size{
		fyne.NewSize(800, 600),
		fyne.NewSize(1500, 950),
		fyne.NewSize(340, 340),
		fyne.NewSize(1481.8, 340),
		fyne.NewSize(520, 340),
	}

	for _, n := range natives {
		for _, vp := range viewports {
			z, _ := newZoom(t, image.Rect(0, 0, n[0], n[1]), vp)

			z.In()
			z.Out()

			if z.CanPan() {
				t.Errorf("native=%v viewport=%v: CanPan() = true after zoom in then out, want back at fit "+
					"scale with nothing to pan (scale=%v, fitScale=%v)", n, vp, z.scale, z.fitScale())
			}
		}
	}
}

func TestActualSize(t *testing.T) {
	z, changes := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))
	z.pan = fyne.NewPos(30, 30)

	z.ActualSize()

	if z.Fitting() {
		t.Error("ActualSize should leave fit mode")
	}
	if z.scale != 1 {
		t.Errorf("scale = %v, want 1", z.scale)
	}
	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want reset to zero", z.pan)
	}
	if *changes != 1 {
		t.Errorf("onChanged fired %d times, want 1", *changes)
	}
}

func TestFitToWindow_ReturnsToFitAndNotifies(t *testing.T) {
	z, changes := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))
	z.fit = false
	z.scale = 5
	z.pan = fyne.NewPos(30, 30)

	z.FitToWindow()

	if !z.Fitting() {
		t.Error("FitToWindow should turn fit mode back on")
	}
	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want reset to zero", z.pan)
	}
	if *changes != 1 {
		t.Errorf("onChanged fired %d times, want 1", *changes)
	}
}

// ResetToFit is FitToWindow without the notification: it exists for callers
// already mid-update (a fresh load, a rotation) that refresh the display
// themselves, and a second notification from here would be redundant.
func TestResetToFit_DoesNotNotify(t *testing.T) {
	z, changes := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))
	z.fit = false
	z.scale = 5
	z.pan = fyne.NewPos(30, 30)

	z.ResetToFit()

	if !z.Fitting() {
		t.Error("ResetToFit should turn fit mode back on")
	}
	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want reset to zero", z.pan)
	}
	if *changes != 0 {
		t.Errorf("onChanged fired %d times, want 0 - ResetToFit is the silent form", *changes)
	}
}

// --- Percent -----------------------------------------------------------------

func TestPercent(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400)) // fit scale = 2

	if got := z.Percent(); got != 200 {
		t.Errorf("Percent() while fitting = %d, want 200 (the fit scale)", got)
	}

	z.ActualSize()
	if got := z.Percent(); got != 100 {
		t.Errorf("Percent() at actual size = %d, want 100", got)
	}

	// A variable, not the constant expression: converting a non-integral
	// constant to int is a compile-time error in Go.
	stepped := step
	z.In()
	if got, want := z.Percent(), int(stepped*100+0.5); got != want {
		t.Errorf("Percent() after one step in = %d, want %d", got, want)
	}
}

// --- at (scroll zoom) --------------------------------------------------------

func TestAt_AnchorsCursorPoint(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))

	cursor := fyne.NewPos(150, 150) // off-center: 75% across the image
	wantX, wantY := pointUnderCursor(z, cursor)

	z.at(200, cursor) // large positive dy: zoom in well past fit, no clamping at this cursor/scale

	if z.Fitting() {
		t.Fatal("at should switch out of fit mode")
	}
	if z.scale <= 2 {
		t.Fatalf("scale = %v, want > 2 (fit scale) after a positive-dy scroll", z.scale)
	}

	gotX, gotY := pointUnderCursor(z, cursor)
	if !uitest.ApproxEqual(gotX, wantX) || !uitest.ApproxEqual(gotY, wantY) {
		t.Errorf("native point under cursor moved from (%v,%v) to (%v,%v), want it pinned in place", wantX, wantY, gotX, gotY)
	}
}

func TestAt_NegativeDYZoomsOut(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.fit = false
	z.scale = 4

	z.at(-100, fyne.NewPos(100, 100)) // centered cursor, negative dy zooms out

	if z.scale >= 4 {
		t.Errorf("scale = %v, want less than 4 after a negative-dy scroll", z.scale)
	}
}

func TestAt_StartsFromCurrentFitScale(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400)) // fit scale = min(2,2) = 2

	z.at(1, fyne.NewPos(400, 200)) // centered cursor, small dy

	if z.Fitting() {
		t.Error("at should switch out of fit mode")
	}
	want := float32(2) * float32(math.Exp(1*float64(scrollSensitivity)))
	if !uitest.ApproxEqual(z.scale, want) {
		t.Errorf("scale = %v, want %v (fit scale x scroll factor)", z.scale, want)
	}
}

func TestAt_ClampsToMinAndMax(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))
	z.fit = false

	z.scale = maxScale
	z.at(1000, fyne.NewPos(400, 200))
	if z.scale != maxScale {
		t.Errorf("scale = %v, want clamped to max %v", z.scale, maxScale)
	}

	z.scale = minScale
	z.at(-1000, fyne.NewPos(400, 200))
	if z.scale != minScale {
		t.Errorf("scale = %v, want clamped to min %v", z.scale, minScale)
	}
}

func TestAt_NoImageIsNoop(t *testing.T) {
	z, changes := newZoom(t, image.Rectangle{}, fyne.NewSize(800, 400))
	z.scale = 3

	z.at(100, fyne.NewPos(400, 200))

	if !z.Fitting() || z.scale != 3 {
		t.Errorf("at with no image loaded should be a no-op, got fit=%v scale=%v", z.Fitting(), z.scale)
	}
	if *changes != 0 {
		t.Errorf("onChanged fired %d times, want 0 - nothing changed", *changes)
	}
}

// --- the widget's own event handling ----------------------------------------

// scroll fires a scroll event at the widget the way Fyne's driver would.
func scroll(z *Zoom, at fyne.Position, delta fyne.Delta) {
	z.widget.Scrolled(&fyne.ScrollEvent{
		PointEvent: fyne.PointEvent{Position: at},
		Scrolled:   delta,
	})
}

func TestScrolled_ZoomsAtCursor(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))

	scroll(z, fyne.NewPos(400, 200), fyne.NewDelta(0, 50))

	if z.Fitting() {
		t.Error("scrolling should switch out of fit mode")
	}
	if z.scale <= 2 {
		t.Errorf("scale = %v, want > 2 (fit scale) after a positive scroll", z.scale)
	}
}

func TestScrolled_IgnoresHorizontalOnlyScroll(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))

	scroll(z, fyne.NewPos(400, 200), fyne.NewDelta(50, 0))

	if !z.Fitting() {
		t.Error("a horizontal-only scroll (DY=0) should not affect zoom")
	}
}

func TestScrolled_ShiftPansInsteadOfZooming(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.modifiers = func() fyne.KeyModifier { return fyne.KeyModifierShift }
	z.fit = false
	z.scale = 4 // -> 400x400 scaled, room to pan on both axes

	scroll(z, fyne.NewPos(100, 100), fyne.NewDelta(20, -5))

	if z.scale != 4 {
		t.Errorf("scale = %v, want unchanged at 4 - Shift+scroll should pan, not zoom", z.scale)
	}
	if z.pan.X != 20 || z.pan.Y != -5 {
		t.Errorf("pan = %v, want (20,-5) from the scroll delta", z.pan)
	}
}

func TestScrolled_ShiftWithNothingToPanIsNoop(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.modifiers = func() fyne.KeyModifier { return fyne.KeyModifierShift }

	scroll(z, fyne.NewPos(100, 100), fyne.NewDelta(20, 20))

	if !z.Fitting() {
		t.Error("Shift+scroll should never switch out of fit mode")
	}
	if z.pan != (fyne.Position{}) {
		t.Errorf("pan = %v, want zero - nothing to pan while fitting", z.pan)
	}
}

// A nil modifiers func reads as "nothing held" rather than panicking - the
// documented contract for a caller that has no way to ask.
func TestScrolled_NilModifiersZoomsRatherThanPanning(t *testing.T) {
	img := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 400, 200)))
	z := New(img, nil, nil)
	z.Widget().Resize(fyne.NewSize(800, 400))

	scroll(z, fyne.NewPos(400, 200), fyne.NewDelta(0, 50))

	if z.Fitting() {
		t.Error("with no modifiers func, a scroll should zoom as usual")
	}
}

func TestDragged_Pans(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 100, 100), fyne.NewSize(200, 200))
	z.fit = false
	z.scale = 4

	z.widget.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(15, 25)})

	if z.pan.X != 15 || z.pan.Y != 25 {
		t.Errorf("pan = %v, want (15,25) from the drag delta", z.pan)
	}
}

// --- CanPan / Cursor ---------------------------------------------------------

func TestCanPan(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 600))

	z.scale = 10 // ignored while fitting
	if z.CanPan() {
		t.Error("CanPan should be false while fitting")
	}

	z.fit = false
	z.scale = 1 // -> 400x200, well within the 800x600 viewport
	if z.CanPan() {
		t.Error("CanPan should be false when the zoomed image still fits entirely")
	}

	z.scale = 2 // -> 800x400, exactly viewport width, still not over
	if z.CanPan() {
		t.Error("CanPan should be false when the zoomed image exactly matches the viewport")
	}

	z.scale = 2.0002 // -> a hair over 800 wide, within panSlack of the viewport
	if z.CanPan() {
		t.Error("CanPan should be false when the overflow is within panSlack (float32 zoom round-trip noise)")
	}

	z.scale = 3 // -> 1200x600, overflows on width
	if !z.CanPan() {
		t.Error("CanPan should be true once the zoomed image overflows the viewport on any axis")
	}

	z.img.Image = nil
	if z.CanPan() {
		t.Error("CanPan should be false with no image loaded")
	}
}

func TestCursor_SignalsWhetherTheImageIsDraggable(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 600))

	if got := z.Cursor(); got != desktop.DefaultCursor {
		t.Errorf("cursor while fitting = %v, want the default arrow", got)
	}

	// Zoomed in, but the image still fits entirely within the viewport -
	// nothing to pan.
	z.fit = false
	z.scale = 1
	if got := z.Cursor(); got != desktop.DefaultCursor {
		t.Errorf("cursor when the zoomed image still fits = %v, want the default arrow", got)
	}

	// Zoomed past the viewport on at least one axis - now it can be dragged.
	z.scale = 4
	if got := z.Cursor(); got != desktop.PointerCursor {
		t.Errorf("cursor when the zoomed image overflows the viewport = %v, want the grab-hand stand-in", got)
	}

	// The widget is what Fyne actually asks, and it must agree.
	if got := z.widget.Cursor(); got != desktop.PointerCursor {
		t.Errorf("widget cursor = %v, want the same shape Zoom reports", got)
	}
}

// --- the renderer's layout hand-off ------------------------------------------

// Resizing the widget is what caches the viewport and re-lays the image
// out against it - the path a window resize takes in production.
func TestLayout_CachesViewportAndReflows(t *testing.T) {
	z, _ := newZoom(t, image.Rect(0, 0, 400, 200), fyne.NewSize(800, 400))

	if z.Percent() != 200 {
		t.Fatalf("setup: Percent() = %d, want the 200%% fit scale for an 800x400 viewport", z.Percent())
	}

	z.Widget().Resize(fyne.NewSize(400, 200))

	if got := z.Percent(); got != 100 {
		t.Errorf("Percent() after halving the viewport = %d, want 100", got)
	}
	if got := z.img.Size(); !uitest.ApproxEqual(got.Width, 400) || !uitest.ApproxEqual(got.Height, 200) {
		t.Errorf("img size = %v, want the new 400x200 viewport while fitting", got)
	}
}
