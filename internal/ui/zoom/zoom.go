// Package zoom is the zoom/pan view of the displayed image: the 0/1/+/-
// keys, click-and-drag panning, and scroll-to-zoom anchored at the pointer.
//
// It owns the widget the image is laid out by - deliberately replacing
// Stack's "resize the child to fill the container" with zoom/pan-aware
// sizing - and all the state behind it: fit versus manual, the scale, the
// pan offset, and the viewport it was last laid out against.
//
// It takes no Host interface, because it needs no callback into the app's
// state. What it shares with the app is one *canvas.Image, on a strict
// single-writer-per-field contract:
//
//	the app owns img.Image  - the pixels: it decodes, rotates and animates them
//	this package owns img's size and position - Resize and Move
//
// Neither writes the other's side. That is what lets the two coexist
// without a lock or a callback: a new frame arriving from the app never
// disturbs the layout, and a zoom never disturbs the pixels.
//
// The two funcs New takes are the only other coupling. onChanged is how
// the app hears that the zoom level moved (it redraws its info overlay);
// modifiers reports which keyboard modifiers are held, which Fyne only
// exposes through the desktop driver, so the app injects it and tests stub
// it.
package zoom

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	// step is the multiplier In/Out apply to the scale per press.
	step = float32(1.25)

	minScale = float32(0.05)
	maxScale = float32(16)

	// panSlack absorbs the float32 rounding error that creeps into scale
	// when In/Out round-trips it through fitScale (viewport/native) and
	// back (native*scale, in apply/canPan): a zoom-in followed by a
	// matching zoom-out can land a small fraction of a point away from the
	// original fit size instead of exactly on it. Half a point is
	// imperceptible but comfortably clears the drift seen in practice
	// (well under a thousandth of a point), so an image back at
	// effectively fit size doesn't register as overflowing the viewport -
	// see clampPanAxis and canPan below.
	panSlack = float32(0.5)

	// scrollSensitivity converts a fyne.ScrollEvent.Scrolled.DY into a
	// zoom factor via exp(dy*sensitivity) rather than scaling linearly: a
	// discrete mouse-wheel notch (DY in the tens, per Fyne's glfw driver)
	// and a single trackpad tick (DY of a handful of units) both compose
	// sensibly this way, repeated small trackpad deltas multiply out the
	// same as fewer larger wheel notches, and the result is always
	// positive so it can never flip the zoom direction on a big fling.
	scrollSensitivity = float32(0.01)
)

// Zoom is the zoom/pan state and the widget that renders it.
type Zoom struct {
	// img is the app's image - see the package doc for who writes what.
	img *canvas.Image

	// onChanged is called after every change to the zoom level made
	// through this package's own key/scroll entry points, so the app can
	// refresh anything that displays it. Not called by ResetToFit, whose
	// callers are mid-update and refresh for themselves. May be nil.
	onChanged func()

	// modifiers reports the keyboard modifiers currently held, for the
	// Shift+scroll pan (see imageWidget.Scrolled). May be nil, read as
	// "nothing held".
	modifiers func() fyne.KeyModifier

	widget *imageWidget

	// fit is true while the image is scaled to fit the viewport, which is
	// exactly the behaviour the app had before zoom existed
	// (ImageFillContain within a window sized to the image). The 1/+/-
	// keys and a scroll switch to manual zoom; the 0 key and every fresh
	// navigation switch back.
	fit bool

	// scale is the display scale used while fit is false: 1 means one
	// image pixel per canvas point ("100%", set by ActualSize) - the same
	// pixel/point convention the app's window sizing uses. In/Out
	// multiply and divide it by step, clamped to [minScale, maxScale].
	scale float32

	// pan shifts the zoomed image away from center, in canvas points;
	// dragging updates it, and apply clamps it so the image can never be
	// dragged fully out of view. Pinned to zero whenever fit is on.
	pan fyne.Position

	// viewport is the size apply last laid the image out against, cached
	// by the renderer's Layout so a keyboard zoom change - which doesn't
	// itself trigger a resize - has a size to lay out against without
	// waiting for the next one.
	viewport fyne.Size
}

// New builds the zoom view over img. onChanged and modifiers may both be
// nil (no notification; no modifiers held). It starts out fitting, which
// is the state every freshly loaded image is shown in.
func New(img *canvas.Image, onChanged func(), modifiers func() fyne.KeyModifier) *Zoom {
	z := &Zoom{
		img:       img,
		onChanged: onChanged,
		modifiers: modifiers,
		fit:       true,
	}
	z.widget = newImageWidget(z)

	return z
}

// Widget is the canvas object to place in the window's content, in place
// of the image itself.
func (z *Zoom) Widget() fyne.CanvasObject {
	return z.widget
}

// Fitting reports whether the image is scaled to fit the viewport rather
// than held at a manual zoom level. It is the component's primary state,
// and the app's own tests assert the "every fresh navigation and every
// rotation goes back to fit" contract through it - nothing in production
// branches on it, since Percent covers display and CanPan folds it in
// already.
func (z *Zoom) Fitting() bool {
	return z.fit
}

// Percent is the display scale currently in effect - whichever of the fit
// scale or the manual scale actually applies - as a rounded percentage,
// for the app's info overlay.
func (z *Zoom) Percent() int {
	scale := z.scale
	if z.fit {
		scale = z.fitScale()
	}

	return int(scale*100 + 0.5)
}

// CanPan reports whether the image, at its current scale, overflows the
// viewport on at least one axis - i.e. whether there's actually anything
// for a drag to pan around. False while fitting (the viewport is what it's
// fit to, by definition), with no image loaded yet, or when a manual zoom
// level still leaves the whole image visible.
func (z *Zoom) CanPan() bool {
	if z.fit || z.img.Image == nil {
		return false
	}

	return overflows(z.scaledSize(), z.viewport)
}

// Cursor is the pointer shape over the image: a grab hand whenever the
// image actually overflows the viewport, so hovering it signals it can be
// dragged. Fyne has no dedicated "grab" cursor, so PointerCursor (the same
// hand used for links) is the closest built-in stand-in. A manual zoom
// that still fits entirely within the window (e.g. 100% on a small image)
// has nothing to pan, so it keeps the plain arrow, same as fitting.
func (z *Zoom) Cursor() desktop.Cursor {
	if z.CanPan() {
		return desktop.PointerCursor
	}

	return desktop.DefaultCursor
}

// ResetToFit returns to fit-to-window silently - without the onChanged
// notification. For callers already in the middle of a bigger update that
// will refresh the display themselves: loading a new image, and rotating
// the current one. FitToWindow is the same thing as a user action.
func (z *Zoom) ResetToFit() {
	z.fit = true
	z.apply()
}

// FitToWindow is the 0 key: back to the default fit-to-window display.
func (z *Zoom) FitToWindow() {
	z.ResetToFit()
	z.changed()
}

// ActualSize is the 1 key: 100%, one image pixel per canvas point,
// centered.
func (z *Zoom) ActualSize() {
	z.fit = false
	z.scale = 1
	z.pan = fyne.NewPos(0, 0)
	z.apply()
	z.changed()
}

// In and Out are the +/- keys: one step of zoom around the image centre.
// The step multiplier stays in this package rather than being handed to
// the caller, so the key dispatcher binds keys to intentions ("zoom in")
// instead of to arithmetic.
func (z *Zoom) In() {
	z.by(step)
}

// Out is In's counterpart - see it.
func (z *Zoom) Out() {
	z.by(1 / step)
}

// by multiplies the current scale by factor (or 1/factor to zoom out),
// clamped to [minScale, maxScale]. The first press out of fit mode starts
// from whatever scale fit is currently showing, via fitScale, so zooming
// in and out feels continuous instead of jumping straight to 100%.
func (z *Zoom) by(factor float32) {
	if z.fit {
		z.fit = false
		z.scale = z.fitScale()
	}

	z.scale = min(max(z.scale*factor, minScale), maxScale)
	z.apply()
	z.changed()
}

// at is the mouse-wheel/trackpad handler behind imageWidget.Scrolled: like
// by it turns a scroll delta into a multiplicative change to the scale
// (starting from fitScale on the first scroll out of fit mode, same as
// by), but where by always zooms around the image centre, at solves for
// the pan offset that leaves the native-pixel point under the cursor
// exactly where it was on screen, so the point the user is pointing at is
// the point that stays still. Positive dy (a wheel notch away from the
// user, or a trackpad swipe up) zooms in, matching Preview.app, Google
// Maps, and most other scroll-to-zoom UIs. cursor is in the same
// coordinate space as viewport (see imageWidget.Scrolled).
func (z *Zoom) at(dy float32, cursor fyne.Position) {
	if z.img.Image == nil {
		return
	}

	oldScale := z.scale
	if z.fit {
		oldScale = z.fitScale()
	}

	factor := float32(math.Exp(float64(dy * scrollSensitivity)))
	newScale := min(max(oldScale*factor, minScale), maxScale)

	b := z.img.Image.Bounds()
	native := fyne.NewSize(float32(b.Dx()), float32(b.Dy()))

	// pan is guaranteed zero here whenever fit is true (see apply), so
	// oldPos is exactly the position apply would have laid the image out
	// at, fit or not.
	oldScaled := fyne.NewSize(native.Width*oldScale, native.Height*oldScale)
	oldPos := z.originFor(oldScaled, z.pan)

	// Native-pixel coordinates of the point under the cursor, so it can be
	// re-anchored at the same screen position once newScale takes effect.
	imgX := (cursor.X - oldPos.X) / oldScale
	imgY := (cursor.Y - oldPos.Y) / oldScale

	newScaled := fyne.NewSize(native.Width*newScale, native.Height*newScale)
	z.fit = false
	z.scale = newScale
	z.pan = fyne.NewPos(
		cursor.X-imgX*newScale-(z.viewport.Width-newScaled.Width)/2,
		cursor.Y-imgY*newScale-(z.viewport.Height-newScaled.Height)/2,
	)
	// apply re-clamps pan against the new scale, so a zoom that would
	// otherwise pull the image's edge into view stays pinned instead.
	z.apply()
	z.changed()
}

// panBy is the drag handler: it nudges the pan offset by the delta and
// re-lays out immediately, so the image visibly tracks the pointer.
func (z *Zoom) panBy(d fyne.Delta) {
	z.pan = z.pan.Add(d)
	z.apply()
}

// apply sizes and positions the image within the viewport according to the
// current zoom/pan state. Called from the renderer's Layout on every
// resize, and directly by every mutator above, since those don't
// themselves trigger a resize to hang it off.
func (z *Zoom) apply() {
	if z.viewport.Width <= 0 || z.viewport.Height <= 0 {
		return
	}

	// No image yet, or fitting: exactly the pre-zoom behaviour - fill the
	// viewport with ImageFillContain, at (0, 0), no pan.
	if z.img.Image == nil || z.fit {
		z.pan = fyne.NewPos(0, 0)
		z.img.Resize(z.viewport)
		z.img.Move(fyne.NewPos(0, 0))

		return
	}

	scaled := z.scaledSize()
	z.pan = clampPan(z.pan, scaled, z.viewport)

	z.img.Resize(scaled)
	z.img.Move(z.originFor(scaled, z.pan))
}

// changed notifies the app that the zoom level moved.
func (z *Zoom) changed() {
	if z.onChanged != nil {
		z.onChanged()
	}
}

// scaledSize is the image's size at the current manual scale.
func (z *Zoom) scaledSize() fyne.Size {
	b := z.img.Image.Bounds()

	return fyne.NewSize(float32(b.Dx())*z.scale, float32(b.Dy())*z.scale)
}

// originFor is where an image of size scaled sits in the viewport:
// centered, then shifted by the pan offset.
func (z *Zoom) originFor(scaled fyne.Size, pan fyne.Position) fyne.Position {
	return fyne.NewPos(
		(z.viewport.Width-scaled.Width)/2+pan.X,
		(z.viewport.Height-scaled.Height)/2+pan.Y,
	)
}

// fitScale is the scale fit mode is currently displaying the image at, as
// a multiple of its native pixel size - the same "shrink or grow to fit,
// preserving aspect ratio" math ImageFillContain applies, worked out here
// as a plain number so by and at have a starting point to zoom from.
func (z *Zoom) fitScale() float32 {
	if z.img.Image == nil || z.viewport.Width <= 0 || z.viewport.Height <= 0 {
		return 1
	}

	b := z.img.Image.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return 1
	}

	return min(z.viewport.Width/float32(b.Dx()), z.viewport.Height/float32(b.Dy()))
}

// overflows reports whether scaled sticks out past viewport on either
// axis, by more than the float32 noise panSlack absorbs.
func overflows(scaled, viewport fyne.Size) bool {
	return scaled.Width > viewport.Width+panSlack || scaled.Height > viewport.Height+panSlack
}

// clampPan keeps a zoomed image from being dragged out of view: on an axis
// where the scaled image is smaller than the viewport it's pinned to
// centered (an offset of 0); on one where it's larger, the offset is
// clamped so the image's own edge never crosses into the viewport - i.e.
// the viewport stays fully covered by the image on that axis.
func clampPan(offset fyne.Position, scaled, viewport fyne.Size) fyne.Position {
	return fyne.NewPos(
		clampPanAxis(offset.X, scaled.Width, viewport.Width),
		clampPanAxis(offset.Y, scaled.Height, viewport.Height),
	)
}

func clampPanAxis(offset, scaled, viewport float32) float32 {
	if scaled <= viewport+panSlack {
		return 0
	}

	limit := (scaled - viewport) / 2

	return min(max(offset, -limit), limit)
}
