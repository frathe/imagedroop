package imaging

import (
	"image"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/image/draw"

	"fyne.io/fyne/v2"
)

// ThumbnailSize is the maximum length, in pixels, of a generated
// thumbnail's longer edge; the shorter edge is scaled to preserve the
// source image's aspect ratio.
const ThumbnailSize = 200

// thumbCacheSize bounds how many thumbnails NewThumbCache keeps in memory
// at once. Each entry is capped to ThumbnailSize on its long edge - a small
// fraction of a full decode's pixel count - so this budget can comfortably
// cover a large drop's entire file set at once, unlike imgCacheSize, which
// only ever needs to hold the current image plus its immediate neighbors.
const thumbCacheSize = 4096

// NewThumbCache builds the LRU cache callers use to hold generated
// thumbnails, keyed by URI string - a separate cache and budget from
// NewImgCache's full-size decodes, so populating it (e.g. from the grid
// overview) can never evict a full decode the normal viewing path still
// needs, or vice versa.
func NewThumbCache() *lru.Cache[string, image.Image] {
	c, err := lru.New[string, image.Image](thumbCacheSize)
	if err != nil {
		panic(err)
	}
	return c
}

// LoadThumbnail reads and decodes u exactly like LoadImage - full EXIF
// orientation correction included - then downsamples the first frame
// (animated GIFs show only their first frame here, same as every other
// still context in this app) to fit within ThumbnailSize on its longer
// edge.
func LoadThumbnail(u fyne.URI) (image.Image, error) {
	loaded, err := LoadImage(u)
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
