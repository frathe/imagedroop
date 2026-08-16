package imaging

import (
	"image"

	"golang.org/x/image/draw"

	"fyne.io/fyne/v2"
)

// ThumbnailSize is the maximum length, in pixels, of a generated
// thumbnail's longer edge; the shorter edge is scaled to preserve the
// source image's aspect ratio.
const ThumbnailSize = 200

// DefaultThumbCacheBytes is the shipped byte budget for NewThumbCache,
// until the settings window (internal/ui/settingswin) changes it. Each
// entry is capped to ThumbnailSize on its long edge, so 256 MB covers on
// the order of 1600 thumbnails - enough for a large drop's whole file set
// to stay warm while scrolling the grid, without the 625 MB an
// entry-bounded 4096 could quietly reach.
const DefaultThumbCacheBytes = 256 << 20

// NewThumbCache builds the byte-bounded cache callers use to hold generated
// thumbnails, keyed by URI string - a separate cache and budget from
// NewImgCache's full-size decodes, so populating it (e.g. from the grid
// overview) can never evict a full decode the normal viewing path still
// needs, or vice versa.
func NewThumbCache(budget int64) *ByteCache[image.Image] {
	return NewByteCache(budget, imageBytes)
}

// LoadThumbnail reads and decodes u exactly like LoadImage - full EXIF
// orientation correction included - then downsamples the first frame
// (animated GIFs show only their first frame here, same as every other
// still context in this app) to fit within ThumbnailSize on its longer
// edge.
//
// The zero animation budget is what makes that "first frame only" literal:
// without it a long animation composited every one of its frames to a full
// RGBA canvas so this could keep one and discard the rest, which for a
// large GIF meant gigabytes of allocation per grid cell.
func LoadThumbnail(u fyne.URI) (image.Image, error) {
	loaded, err := LoadImage(u, 0)
	if err != nil {
		return nil, err
	}
	return scaleToFit(loaded.Frames[0], ThumbnailSize), nil
}

// scaleToFit downsamples src to fit within maxEdge x maxEdge, preserving
// aspect ratio; src is returned unchanged if it's already within bounds,
// rather than upscaled. Uses ApproxBiLinear rather than the higher-quality
// (but much slower on a large downscale) BiLinear kernel - draw.Interpolator's
// own doc recommends it as the speed/quality tradeoff for exactly this case,
// and a small preview doesn't need kernel-grade sharpness.
func scaleToFit(src image.Image, maxEdge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= maxEdge && h <= maxEdge) {
		return src
	}

	dw, dh := maxEdge, h*maxEdge/w
	if h > w {
		dw, dh = w*maxEdge/h, maxEdge
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}
