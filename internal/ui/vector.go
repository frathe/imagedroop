// The vector re-render: how an SVG stays sharp when the display scale
// moves. internal/imaging owns the parsing and rasterizing; what lives here
// is the policy (is a new raster worth making?), the coalescing, and the
// hand-off back onto the UI goroutine.

package ui

import (
	"image"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/imaging"
)

const (
	// vectorSharpenRatio is how much denser a wanted raster must be before
	// it is worth producing. A little above 1 so a slow scroll, which
	// changes the scale a fraction of a percent at a time, doesn't
	// re-render on every frame.
	vectorSharpenRatio = 1.05

	// vectorReleaseRatio is the other side of the band: zooming out does
	// not hurt sharpness at all (a dense raster downscales cleanly), so a
	// re-render happens on the way down only to release memory the zoom
	// level no longer justifies.
	vectorReleaseRatio = 0.5

	// defaultVectorDebounce is long enough to swallow a scroll gesture's
	// burst of events and short enough not to feel laggy. Zero in tests -
	// see viewer.vector.debounce.
	defaultVectorDebounce = 90 * time.Millisecond
)

// vectorView is the whole state of the SVG re-render: the parsed document,
// the two sizes the policy compares, the lifecycle its rasterization runs
// under, and the three write-once seams tests replace. A value field on
// viewer, never copied - it holds a WaitGroup and a lifecycle mutex.
type vectorView struct {
	// svg is the parsed SVG behind the image on screen, nil for every
	// raster format. Non-nil is what makes a scale change mean anything.
	svg *imaging.Vector

	// logical is the size the app treats that vector as being - what
	// the window, the title and the info overlay are built on. Fixed for
	// the lifetime of the loaded image; the raster behind it is not.
	logical fyne.Size

	// raster is the pixel size of the raster currently on screen,
	// which requestVectorRender compares a new target against.
	raster image.Point

	// lifecycle owns debounce and rasterization for the latest SVG
	// render request. A newer scale, image change, clear, or shutdown cancels
	// the previous token and wakes it out of the debounce immediately.
	lifecycle requestLifecycle

	// pending is waited on by the test suite's drain, per the
	// module's concurrency invariant.
	pending sync.WaitGroup

	// debounce coalesces a burst of scroll-driven scale changes into
	// one rasterization. A per-viewer field rather than a package var
	// (concurrency invariant: the viewer has no mutable package state),
	// which is also the seam that lets tests set it to zero.
	debounce time.Duration

	// rasterize and after are RasterAt and time.After behind
	// per-viewer seams (the concurrency invariant forbids mutable package
	// state), so the coalescing test can count rasterizations and release
	// a parked burst deterministically. Production never overrides them.
	// Like debounce, they are write-once: set at construction, and
	// by a test only before its first drop.
	rasterize func(vec *imaging.Vector, w, h int) (image.Image, error)
	after     func(time.Duration) <-chan time.Time
}

// requestVectorRender is zoom's onScaleChanged handler: the effective
// display scale just moved, from a key, a scroll, or a window resize while
// fitting.
//
// It runs inside the zoom renderer's Layout, so it must not touch a widget
// synchronously - see zoom's package doc. It only reads viewer state and
// spawns; the pixels land later, through fyne.Do.
func (v *viewer) requestVectorRender(scale float32) {
	if v.vector.svg == nil || scale <= 0 || v.vector.logical.Width <= 0 {
		return
	}

	w := int(float64(v.vector.logical.Width)*float64(scale) + 0.5)
	h := int(float64(v.vector.logical.Height)*float64(scale) + 0.5)

	// No rotation adjustment here on purpose. The raster is produced in
	// unrotated space and redrawRotatedFrame turns it afterwards, and a
	// quarter turn preserves pixel count - so a raster of the unrotated
	// logical size times the scale rotates to exactly the size zoom lays
	// out. Swapping the axes would instead stretch the drawing, since
	// oksvg's SetTarget scales each axis independently.

	// Clamp before comparing, not after: an unclamped target the ceiling
	// would shrink anyway would look like a permanently unmet demand for a
	// sharper image and re-render forever.
	w, h = imaging.ClampVectorRaster(w, h)

	if !vectorNeedsRender(v.vector.raster, image.Pt(w, h)) {
		return
	}

	token := v.vector.lifecycle.begin()
	v.vector.pending.Add(1)

	go v.rasterizeVector(v.vector.svg, w, h, token)
}

// vectorNeedsRender is the hysteresis band described on the two ratio
// constants above. Comparing one axis is enough: ClampVectorRaster and the
// logical size both preserve aspect, so the two move together.
func vectorNeedsRender(have, want image.Point) bool {
	if have.X <= 0 {
		return true // nothing on screen yet - any raster beats none
	}

	// Unreachable from requestVectorRender (ClampVectorRaster floors at
	// 1), but the safe answer for a degenerate target is "no", not a
	// commission to rasterize at zero size.
	if want.X <= 0 {
		return false
	}

	ratio := float64(want.X) / float64(have.X)

	return ratio > vectorSharpenRatio || ratio < vectorReleaseRatio
}

// rasterizeVector waits out the debounce, checks it has not been
// superseded, rasterizes, and hands the result back to the UI goroutine.
// Every early return costs nothing: a burst of twenty scale changes spawns
// twenty of these and rasterizes once, because the other nineteen find the
// generation moved on before allocating anything.
func (v *viewer) rasterizeVector(vec *imaging.Vector, w, h int, token requestToken) {
	defer v.vector.pending.Done()
	defer token.cancelContext()

	if v.vector.debounce > 0 {
		select {
		case <-v.vector.after(v.vector.debounce):
		case <-token.context().Done():
			return
		}
	}

	if !token.current() {
		return
	}

	frame, err := v.vector.rasterize(vec, w, h)
	if err != nil {
		// Deliberately silent. There is always a valid, if softer, raster
		// already on screen, and toasting on a zoom step would be noise; a
		// failure during the initial decode is reported by the load path
		// instead.
		return
	}

	fyne.Do(func() {
		// Re-checked on this side too: the generation can move between the
		// check above and this callback running.
		if !token.current() || v.vector.svg != vec || len(v.displayFrames) == 0 {
			return
		}

		b := frame.Bounds()

		// Safe only because finishLoad gave a vector its own one-element
		// slice - see the comment there. Writing loaded.Frames would mutate
		// the cached LoadedImage and invalidate its ByteCache weight.
		v.displayFrames[0] = frame
		v.vector.raster = image.Pt(b.Dx(), b.Dy())

		// The one place that writes v.img.Image, which is what makes the
		// re-render compose with a pending rotation for free.
		v.redrawRotatedFrame()
	})
}

// clear drops the vector state and abandons any re-render in flight, so a
// rasterization started for the previous image can never land on the next.
func (vv *vectorView) clear() {
	vv.svg = nil
	vv.logical = fyne.Size{}
	vv.raster = image.Point{}
	vv.lifecycle.invalidate()
}

// clearVector drops the vector state and abandons any re-render in flight,
// so a rasterization started for the previous image can never land on the
// next one.
func (v *viewer) clearVector() {
	v.vector.clear()
	v.zoom.SetLogicalSize(fyne.Size{})
}
