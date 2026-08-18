// The vector half of SVG support: a parsed document, retained so it can be
// rasterized again at a different size whenever the display scale moves.
// The size arithmetic and format detection it leans on are in svg.go.

package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"sync"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// ErrNotSVG and ErrNoSVGSize distinguish the two ways a document can fail
// to be a usable vector, because they say different things: the first is
// not an SVG at all, the second is one that declares no size this app can
// work out - neither a usable viewBox nor absolute width and height.
var (
	ErrNotSVG    = errors.New("not an SVG document")
	ErrNoSVGSize = errors.New("SVG declares no usable size")
)

// Vector is a parsed SVG kept alive so the app can rasterize it again as
// the zoom level or window size changes. Held on LoadedImage, and therefore
// shared through the image cache - see RasterAt for what that costs.
type Vector struct {
	// mu guards icon for the whole SetTarget-then-Draw sequence in
	// RasterAt; see there for why that is not optional.
	mu   sync.Mutex
	icon *oksvg.SvgIcon

	// logical is fixed at parse time, so it needs no lock.
	logical image.Rectangle

	// srcBytes is the encoded source length, kept only so
	// loadedImageBytes can charge the cache something honest for the
	// retained parse tree. Measuring the tree itself would mean walking
	// it; its size is proportional to this, and this is already bounded
	// by MaxEncodedBytes.
	srcBytes int
}

// ParseVector parses an SVG document. It does not rasterize - DecodeLoaded
// asks for the first raster separately, at Logical's size.
func ParseVector(data []byte) (*Vector, error) {
	viewBox, width, height, isSVG := svgRootAttrs(data)
	if !isSVG {
		return nil, ErrNotSVG
	}

	icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(trimSVGPrefix(data)), "#000000")
	if err != nil {
		return nil, fmt.Errorf("parse SVG: %w", err)
	}

	// oksvg reads the root element's attributes in document order and
	// abandons the whole element on the first it cannot parse - so
	// width="100%", which is what most web-exported SVGs carry, aborts it
	// before the viewBox that follows is ever read, leaving ViewBox at
	// zero and returning no error at all. svgSizeFrom re-reads the same
	// attributes with rules that tolerate that, so the file renders
	// instead of being rejected for having no size.
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		x, y, w, h, ok := svgSizeFrom(viewBox, width, height)
		if !ok {
			return nil, ErrNoSVGSize
		}

		icon.ViewBox.X, icon.ViewBox.Y = x, y
		icon.ViewBox.W, icon.ViewBox.H = w, h
	}

	logical := vectorLogical(icon.ViewBox.W, icon.ViewBox.H)
	if logical.Empty() {
		return nil, ErrNoSVGSize
	}

	return &Vector{icon: icon, logical: logical, srcBytes: len(data)}, nil
}

// Logical is the size the app treats this image as being, whatever size its
// current raster happens to be - see vectorLogical.
func (v *Vector) Logical() image.Rectangle {
	return v.logical
}

// RasterAt draws the vector at w by h pixels, clamped to the pixel ceiling.
//
// The lock is not optional: SetTarget writes icon.Transform and Draw reads
// it, and two of internal/ui/vector.go's rasterizeVector goroutines can be
// inside this method on the same *Vector at once - goroutine A can pass its
// own staleness check, and only then does a fresher scale change spawn
// goroutine B and bump the generation, so A is already past the guard and
// still rasterizing when B starts. TestRasterAtIsSafeForConcurrentUse
// covers exactly this. The recover sits inside the lock because oksvg
// panics outright on some inputs (a 60000-unit viewBox raises a
// slice-bounds panic) - letting that escape would both crash the app and
// leave the transform half written under a held mutex.
func (v *Vector) RasterAt(w, h int) (img image.Image, err error) {
	w, h = ClampVectorRaster(w, h)

	v.mu.Lock()
	defer v.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("rasterize SVG at %dx%d: %v", w, h, r)
		}
	}()

	x, y := vectorOffset(v.icon, h)
	v.icon.SetTarget(x, y, float64(w), float64(h))

	// Returned as *image.RGBA rather than copied to NRGBA the way Fyne's
	// own decoder does: its comment gives the reason as an x/image/draw
	// fast path for that copy, not a rendering requirement, and
	// imageBytes already weighs *image.RGBA correctly - so the copy would
	// cost a full extra frame allocation for nothing.
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())

	v.icon.Draw(rasterx.NewDasher(w, h, scanner), 1)

	return rgba, nil
}

// vectorOffset corrects for a viewBox whose origin sits above the canvas,
// mirroring the same adjustment Fyne's own SVG decoder makes: oksvg's
// SetTarget subtracts ViewBox.Y unconditionally, which pushes the drawing
// off the top of the raster when that value is negative.
func vectorOffset(icon *oksvg.SvgIcon, height int) (x, y float64) {
	if icon.ViewBox.Y < 0 {
		y = icon.ViewBox.Y + (-icon.ViewBox.Y/icon.ViewBox.H)*float64(height)
	}

	return 0, y
}
