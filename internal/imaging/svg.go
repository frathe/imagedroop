// SVG is the one vector format this package reads. Unlike every raster
// format, it has no pixels of its own: what a caller gets back is a Vector
// (see vector.go) that can be rasterized again at any size, plus a first
// raster at the file's logical size. This file holds the size arithmetic
// and the format detection; the parsing and rasterizing live in vector.go.

package imaging

import (
	"bytes"
	"encoding/xml"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// MinVectorWidth and MinVectorHeight are the floor a vector's logical size
// is raised to when its own is smaller. Deliberately equal to internal/ui's
// startW/startH - the app's smallest window - so an icon-sized SVG opens
// filling that window rather than as a 24-pixel stamp in the corner of it.
// They cannot be imported from there (internal/imaging must not depend on
// any UI package), so internal/ui carries a test pinning the two together.
const (
	MinVectorWidth  = 520
	MinVectorHeight = 340
)

// DefaultMaxVectorRasterPixels caps a single rasterization until the
// settings window's image-cache budget derives a different cap (see
// internal/ui's SetMaxImageCacheMB). zoom's maxScale is 16, so an
// unclamped 1600x1200 logical image would ask for 492 megapixels - about
// 2 GB as RGBA - at full zoom. 32 million still covers the common case
// completely: a 340x340 icon at 16x is 5440x5440, or 29.6 million. Past
// the cap a vector goes soft exactly as every raster format already does,
// just very much later.
const DefaultMaxVectorRasterPixels = 32_000_000

// minVectorRasterPixels is the floor SetMaxVectorRasterPixels holds
// whatever budget the user configures, so a small memory setting can never
// make SVGs unusable: 8 MP still covers a fit-to-screen render on a 4K
// display, inside the re-render hysteresis band.
const minVectorRasterPixels = 8_000_000

// maxVectorRasterPx is the live cap MaxVectorRasterPixels reports.
// Package-level and atomic for the same reasons maxEncodedBytes is (see
// loader.go): genuinely process-wide decode policy, written by the
// settings window on the UI goroutine while re-render and thumbnail
// goroutines read it.
var maxVectorRasterPx atomic.Int64

// MaxVectorRasterPixels reports the current cap on one rasterization.
// Zero means "never set", falling back to the shipped default - the same
// sentinel MaxEncodedBytes uses.
func MaxVectorRasterPixels() int64 {
	if n := maxVectorRasterPx.Load(); n > 0 {
		return n
	}

	return DefaultMaxVectorRasterPixels
}

// SetMaxVectorRasterPixels changes that cap - internal/ui's
// SetMaxImageCacheMB derives it from the image-cache budget (a quarter of
// the budget's bytes, 4 of them per RGBA pixel), because the re-render
// raster is deliberately not charged to that cache and this is how the
// user's memory setting still reaches it. The clamp owns the domain rule
// the way SetMaxWindowWidth owns its floor: never below the usability
// floor, never above the shipped default's known-good ceiling.
func SetMaxVectorRasterPixels(n int64) {
	maxVectorRasterPx.Store(min(max(n, minVectorRasterPixels), DefaultMaxVectorRasterPixels))
}

// trimSVGPrefix strips a UTF-8 byte-order mark and any leading whitespace,
// neither of which encoding/xml nor oksvg tolerate at the head of a
// document. Both the detector and the parser run on the trimmed bytes so
// they can never disagree about whether a file is readable.
func trimSVGPrefix(data []byte) []byte {
	return bytes.TrimLeft(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), " \t\r\n")
}

// svgRootAttrs scans data far enough to read the document's first element
// and returns the three size attributes off it. ok is false unless that
// element is an <svg>, which makes this the authoritative format test as
// well: oksvg accepts a JSON document without complaint and reports a 0x0
// viewBox for it, so "did it parse" cannot be used to mean "is it an SVG".
//
// It stops at the first start element, so it costs a few hundred bytes of
// parsing however large the file is - which is why ReadAndProbe can use it
// for a header probe without paying for a full parse.
func svgRootAttrs(data []byte) (viewBox, width, height string, ok bool) {
	dec := xml.NewDecoder(bytes.NewReader(trimSVGPrefix(data)))

	// Accept any declared encoding without actually transcoding: the only
	// things read here are attribute names and numeric values, which are
	// ASCII in every encoding an SVG can declare. This costs nothing and
	// avoids depending on golang.org/x/net/html/charset just to reach the
	// root element of a file oksvg would go on to read anyway.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	for {
		tok, err := dec.Token()
		if err != nil {
			return "", "", "", false
		}

		// Skip the XML declaration, any DOCTYPE, comments and whitespace -
		// all of which legitimately precede the root element.
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}

		if !strings.EqualFold(se.Name.Local, "svg") {
			return "", "", "", false
		}

		for _, a := range se.Attr {
			switch a.Name.Local {
			case "viewBox":
				viewBox = a.Value
			case "width":
				width = a.Value
			case "height":
				height = a.Value
			}
		}

		return viewBox, width, height, true
	}
}

// svgViewBox parses the four numbers of a viewBox attribute.
func svgViewBox(s string) (x, y, w, h float64, ok bool) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) != 4 {
		return 0, 0, 0, 0, false
	}

	var v [4]float64
	for i, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		v[i] = n
	}

	if v[2] <= 0 || v[3] <= 0 {
		return 0, 0, 0, 0, false
	}

	return v[0], v[1], v[2], v[3], true
}

// svgLength parses a width or height attribute, tolerating an absolute unit
// suffix (px, pt, cm, mm, in, pc, em, ex). A percentage is deliberately
// rejected rather than guessed at: it is relative to a viewport this app
// does not have, and a document that uses one almost always carries a
// viewBox that svgSizeFrom will have preferred already.
func svgLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, "%") {
		return 0, false
	}

	s = strings.TrimRight(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0, false
	}

	return n, true
}

// svgSizeFrom derives an SVG's own coordinate box from its root attributes:
// the viewBox when it has a usable one, otherwise its width and height. Kept
// separate from svgIntrinsic so ParseVector can reuse it to repair the 0x0
// viewBox oksvg silently produces - see its own comment there.
func svgSizeFrom(viewBox, width, height string) (x, y, w, h float64, ok bool) {
	if x, y, w, h, ok = svgViewBox(viewBox); ok {
		return x, y, w, h, true
	}

	w, okW := svgLength(width)
	h, okH := svgLength(height)

	if !okW || !okH {
		return 0, 0, 0, 0, false
	}

	return 0, 0, w, h, true
}

// svgIntrinsic is svgRootAttrs and svgSizeFrom together: the size the SVG
// declares for itself, before the logical-size floor is applied.
func svgIntrinsic(data []byte) (x, y, w, h float64, ok bool) {
	viewBox, width, height, isSVG := svgRootAttrs(data)
	if !isSVG {
		return 0, 0, 0, 0, false
	}

	return svgSizeFrom(viewBox, width, height)
}

// isSVGData reports whether data is an SVG document. Content-based rather
// than extension-based because DecodeLoaded is handed bytes with no URI
// alongside them, and because both sides of the pipeline must agree.
func isSVGData(data []byte) bool {
	// Cheap rejection first, so the common case - a JPEG or PNG whose first
	// byte is not '<' - never builds an XML decoder at all.
	if t := trimSVGPrefix(data); len(t) == 0 || t[0] != '<' {
		return false
	}

	_, _, _, ok := svgRootAttrs(data)

	return ok
}

// vectorLogical applies the logical-size floor to an SVG's intrinsic size:
// an image already at least as large as the app's smallest window keeps its
// own size, and a smaller one is scaled up - preserving aspect ratio - until
// it fits that window. The result is what the window is sized to, what the
// title and info overlay report, and what "100%" means for this image; the
// raster behind it varies with the zoom level and is a separate concern.
// Returns the zero Rectangle for a non-positive input, which ReadAndProbe
// turns into an InvalidDimensionsError.
func vectorLogical(w, h float64) image.Rectangle {
	// maxImagePixels as a per-axis bound, checked while still a float: a
	// conversion of an out-of-int-range float is implementation-defined,
	// so the guard must run before the conversion, not after.
	if w <= 0 || h <= 0 || w > maxImagePixels || h > maxImagePixels {
		return image.Rectangle{}
	}

	if s := math.Min(MinVectorWidth/w, MinVectorHeight/h); s > 1 {
		w, h = w*s, h*s
	}

	return image.Rect(0, 0, int(w+0.5), int(h+0.5))
}

// ClampVectorRaster reduces a requested raster size to MaxVectorRasterPixels,
// scaling both axes together so the aspect ratio survives. Exported because
// internal/ui applies it to its own re-render target before comparing that
// target against the raster already on screen - without it, a request the
// cap would shrink anyway would look like a permanently unmet demand for a
// sharper image and re-render on every scale change forever.
func ClampVectorRaster(w, h int) (int, int) {
	limit := MaxVectorRasterPixels()

	// Floor at 1, then bound each axis by the cap on its own: an axis can
	// arrive large enough that the int64 product below wraps negative -
	// the same reason checkDimensions in loader.go bounds per axis.
	w, h = min(max(w, 1), int(limit)), min(max(h, 1), int(limit))

	if int64(w)*int64(h) <= limit {
		return w, h
	}

	s := math.Sqrt(float64(limit) / (float64(w) * float64(h)))

	w, h = max(int(float64(w)*s), 1), max(int(float64(h)*s), 1)

	// Flooring each axis at 1 can put the product back over the cap: when
	// the aspect ratio is extreme enough that one axis scales below a
	// single pixel, raising it to 1 leaves the other axis holding a value
	// derived from a scale that assumed otherwise, and it alone then
	// carries the whole budget. Trim the larger axis directly - the shared
	// scale no longer holds once a floor has been applied.
	if int64(w)*int64(h) > limit {
		if w >= h {
			w = max(int(limit/int64(h)), 1)
		} else {
			h = max(int(limit/int64(w)), 1)
		}
	}

	return w, h
}

// svgProbeBounds is the header probe for an SVG: its logical size, worked
// out from the root element alone. Deliberately not a full ParseVector -
// the probe only needs the size, and parsing twice per load would be pure
// waste. A document whose body is malformed therefore passes the probe and
// fails at DecodeLoaded, exactly as image.DecodeConfig can succeed on a
// truncated raster file whose Decode then fails.
func svgProbeBounds(data []byte) image.Rectangle {
	_, _, w, h, ok := svgIntrinsic(data)
	if !ok {
		return image.Rectangle{}
	}

	return vectorLogical(w, h)
}

// decodeVector is DecodeLoaded's SVG branch: parse, then take one raster at
// the logical size as the frame to display now. EXIF orientation is
// deliberately not applied - an SVG carries none, the same reason HEIC and
// AVIF are left alone (see readEXIFOrientation).
func decodeVector(data []byte) (*LoadedImage, error) {
	vec, err := ParseVector(data)
	if err != nil {
		return nil, err
	}

	b := vec.Logical()

	frame, err := vec.RasterAt(b.Dx(), b.Dy())
	if err != nil {
		return nil, err
	}

	return &LoadedImage{Frames: []image.Image{frame}, Vector: vec}, nil
}
