// The Fyne widget half of the package: the canvas object that goes into
// the window's content, and the renderer that hands layout off to the zoom
// state rather than filling the container with the image.

package zoom

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// imageWidget wraps the app's canvas.Image so it can (a) override Stack's
// default "resize the child to fill the container" layout with zoom/pan-
// aware sizing (see imageRenderer.Layout and Zoom.apply), and (b) respond
// to click-and-drag panning and scroll-to-zoom. It holds no zoom/pan state
// itself - the Zoom it points at is the single source of truth - so
// there's never a second copy to keep in sync with the keyboard shortcuts.
type imageWidget struct {
	widget.BaseWidget

	z *Zoom
}

func newImageWidget(z *Zoom) *imageWidget {
	w := &imageWidget{z: z}
	w.ExtendBaseWidget(w)

	return w
}

func (w *imageWidget) CreateRenderer() fyne.WidgetRenderer {
	return &imageRenderer{z: w.z}
}

// Dragged pans the image while zoomed in. At fit scale there's nothing to
// pan - apply pins the offset to zero in that case regardless of what this
// reports - so dragging an unzoomed image is a harmless no-op.
func (w *imageWidget) Dragged(ev *fyne.DragEvent) {
	w.z.panBy(ev.Dragged)
}

func (w *imageWidget) DragEnd() {}

// Scrolled implements fyne.Scrollable: a mouse-wheel or trackpad gesture
// over the image zooms in/out anchored at the pointer, unlike the +/- keys
// (In/Out) which always zoom around the image centre. ev.Position is
// already relative to this widget, the same coordinate space the viewport
// and Zoom.apply's layout math use, so it can be passed straight through
// to at. Horizontal-only gestures (DY==0) are left alone rather than
// treated as a zero-factor zoom, so a sideways trackpad swipe over the
// image doesn't silently knock it out of fit mode.
//
// Holding Shift while scrolling pans instead of zooming. That's a
// deliberate stand-in for two-finger trackpad pan, not a literal detection
// of it: Fyne's desktop driver is backed by GLFW, which only ever forwards
// scroll-wheel events - its Cocoa/Win32/X11 backends have no magnify or
// gesture callback, so a real pinch or a two-finger pan is
// indistinguishable from an ordinary scroll by the time it reaches this
// widget. Shift+scroll plays the same role here that it does in browsers
// remapping the wheel to a second axis.
func (w *imageWidget) Scrolled(ev *fyne.ScrollEvent) {
	if w.z.modifiers != nil && w.z.modifiers()&fyne.KeyModifierShift != 0 {
		w.z.panBy(ev.Scrolled)

		return
	}

	if ev.Scrolled.DY == 0 {
		return
	}

	w.z.at(ev.Scrolled.DY, ev.Position)
}

// Cursor makes the widget desktop.Cursorable; the shape itself is Zoom's
// call, since it depends on the zoom state.
func (w *imageWidget) Cursor() desktop.Cursor {
	return w.z.Cursor()
}

// imageRenderer renders the app's image directly - unlike
// widget.SimpleRenderer, its Layout does not just resize the wrapped
// object to fill whatever size it's given; it hands off to Zoom.apply,
// which sizes and positions the image according to the current zoom/pan
// state instead.
type imageRenderer struct {
	z *Zoom
}

func (r *imageRenderer) Destroy() {}

func (r *imageRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.z.img}
}

func (r *imageRenderer) Refresh() {
	r.z.img.Refresh()
}

func (r *imageRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}

// Layout runs on every resize of the widget - which fills the window's
// content area, so in practice that means the initial layout pass and
// every subsequent window resize. The size is cached on the Zoom so a
// keyboard zoom change, which doesn't resize anything itself, can lay out
// against the same viewport later.
func (r *imageRenderer) Layout(size fyne.Size) {
	r.z.viewport = size
	r.z.apply()
}
