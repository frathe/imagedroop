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
// The three funcs New takes are the only other coupling. onChanged is how
// the app hears that the zoom level moved (it redraws its info overlay);
// modifiers reports which keyboard modifiers are held, which Fyne only
// exposes through the desktop driver, so the app injects it and tests stub
// it.
//
// One wrinkle that contract creates: an SVG's raster is re-rendered at a
// different pixel count as the scale moves, so the number of pixels the
// image has is no longer the size it should be drawn at. Everything here
// therefore measures against a *logical* size (see native and
// SetLogicalSize) rather than against img.Image.Bounds() directly. Raster
// formats leave it unset and behave exactly as before.
//
// onScaleChanged is the third and last callback. It fires from apply -
// which runs inside the renderer's Layout - so its handler must not touch
// a widget synchronously; it may only record state and spawn.
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

	// onScaleChanged reports that the effective display scale moved, from
	// any cause - a key, a scroll, or a window resize while fitting. The
	// app uses it to re-rasterize a vector at the new density. Fires from
	// apply, i.e. possibly mid-layout: see the package doc. May be nil.
	onScaleChanged func(scale float32)

	// lastScale is the value onScaleChanged was last called with, so a
	// layout pass that changes nothing stays silent.
	lastScale float32

	// logical is the size this package's math treats the image as being,
	// independent of how many pixels actually back it. Zero means "use the
	// raster's own bounds", which is every format except SVG.
	logical fyne.Size
}

// New builds the zoom view over img. onChanged and modifiers may both be
// nil (no notification; no modifiers held). It starts out fitting, which
// is the state every freshly loaded image is shown in.
func New(img *canvas.Image, onChanged func(), modifiers func() fyne.KeyModifier, onScaleChanged func(scale float32)) *Zoom {
	z := &Zoom{
		img:            img,
		onChanged:      onChanged,
		modifiers:      modifiers,
		onScaleChanged: onScaleChanged,
		fit:            true,
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
	return int(z.Scale()*100 + 0.5)
}

// Scale is the display scale currently in effect - whichever of the fit
// scale or the manual scale actually applies. Percent is this rounded to a
// whole percentage for the info overlay; the app's vector re-render needs
// the unrounded value.
func (z *Zoom) Scale() float32 {
	if z.fit {
		return z.fitScale()
	}

	return z.scale
}

// SetLogicalSize sets the size the zoom math measures against, replacing
// the raster's own bounds. A zero Size clears it. Deliberately does not
// re-lay out: every caller (finishLoad, applyRotationLayout) is mid-update
// and calls ResetToFit immediately afterwards, which does.
func (z *Zoom) SetLogicalSize(s fyne.Size) {
	z.logical = s

	// Re-arm the scale notification. lastScale exists to suppress a repeat
	// notification for a scale that hasn't moved, but "hasn't moved" is only
	// meaningful within one image: the next image can legitimately land on
	// the very same number, and the app still has to hear about it, because
	// what it does in response - rasterizing a vector to that density - has
	// to happen again for the new image. Without this, a folder of
	// same-sized SVGs shown at a fixed viewport (picture-frame mode, which
	// skips the per-image window resize) notifies for the first one only,
	// and every one after it stays at its load-time raster: blown up and
	// blurry, in the one mode that has no zoom control to recover with.
	//
	// Zero rather than a flag because no real scale is ever zero, and this
	// is reached on every load - clearVector calls it for raster formats
	// too, where the extra notification costs a single early return.
	z.lastScale = 0
}

// LogicalSize reports the size the zoom math is currently measuring
// against - the logical size when one is set, zero otherwise. Like
// Fitting, nothing in production branches on it: it exists so the app's
// own tests can assert the contract that a quarter turn swaps the axes
// zoom fits against, which is otherwise only observable as a magnitude
// difference in a raster several steps downstream.
func (z *Zoom) LogicalSize() fyne.Size {
	return z.logical
}

// native is the size everything here measures against: the logical size
// when one is set, otherwise the raster's own bounds.
func (z *Zoom) native() fyne.Size {
	if z.logical.Width > 0 && z.logical.Height > 0 {
		return z.logical
	}

	if z.img.Image == nil {
		return fyne.Size{}
	}

	b := z.img.Image.Bounds()

	return fyne.NewSize(float32(b.Dx()), float32(b.Dy()))
}

// notifyScale reports an effective-scale change, and only a real one - a
// layout pass that leaves the scale where it was stays silent. Exact
// float comparison is deliberate: this filters out no-ops, and the app
// applies its own hysteresis on top before deciding to re-render anything.
func (z *Zoom) notifyScale() {
	if z.onScaleChanged == nil || z.img.Image == nil {
		return
	}

	s := z.Scale()
	if s == z.lastScale {
		return
	}

	z.lastScale = s
	z.onScaleChanged(s)
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

	native := z.native()

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

	defer z.notifyScale()

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
	n := z.native()

	return fyne.NewSize(n.Width*z.scale, n.Height*z.scale)
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

	n := z.native()
	if n.Width == 0 || n.Height == 0 {
		return 1
	}

	return min(z.viewport.Width/n.Width, z.viewport.Height/n.Height)
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
