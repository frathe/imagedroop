// Package imaging reads, decodes, EXIF-orients, and caches the image files
// Image Drop displays: JPEG, PNG, GIF (including animated), WebP, BMP,
// TIFF, ICO, XPM, HEIC, and AVIF.
package imaging

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // registers JPEG with image.Decode
	_ "image/png"  // registers PNG with image.Decode
	"io"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	_ "github.com/fyne-io/image/ico" // registers ICO with image.Decode
	_ "github.com/fyne-io/image/xpm" // registers XPM with image.Decode
	_ "github.com/gen2brain/avif"    // registers AVIF with image.Decode (WASM/wazero, no cgo)
	_ "github.com/gen2brain/heic"    // registers HEIC with image.Decode (WASM/wazero, no cgo)
	_ "golang.org/x/image/bmp"       // registers BMP with image.Decode
	_ "golang.org/x/image/tiff"      // registers TIFF with image.Decode
	_ "golang.org/x/image/webp"      // registers WebP with image.Decode
)

func IsSupportedImage(u fyne.URI) bool {
	// Extension is checked first because it's a pure string comparison.
	// MimeType(), by contrast, opens and content-sniffs the resource
	// whenever the extension isn't in Go's built-in MIME table (true for
	// every directory, since they have no extension, and for common
	// non-image clutter such as .DS_Store) - checking it first turned a
	// recursive folder scan into thousands of needless file opens.
	switch strings.ToLower(u.Extension()) {
	case ".jpg", ".jpeg", ".jpe", ".jfif", ".png", ".gif", ".webp", ".bmp", ".tif", ".tiff", ".ico", ".xpm",
		".heic", ".heif", ".avif":
		return true
	}

	switch strings.ToLower(u.MimeType()) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff",
		"image/x-icon", "image/vnd.microsoft.icon", "image/x-xpixmap",
		"image/heic", "image/heif", "image/avif":
		return true
	}

	return false
}

// LoadedImage holds one or more display-ready frames. Static images (JPEG,
// PNG, WebP, single-frame GIF) carry exactly one frame; animated GIFs carry
// every frame, each already composited to the GIF's full canvas per its
// disposal method, paired with that frame's display delay.
type LoadedImage struct {
	Frames   []image.Image
	Delays   []time.Duration // parallel to Frames; unused when len(Frames) == 1
	FileSize int64           // raw byte count read by ReadAndProbe, for the info overlay
}

// maxImagePixels caps the pixel count a decoded image header is allowed to
// declare. It guards against decompression-bomb files that claim far more
// pixels than any real photo needs: fully decoding one - or even resizing
// the window to it - could exhaust memory well before any actual pixel data
// is touched. 200 megapixels comfortably covers real-world panoramas and
// professional camera output.
const maxImagePixels = 200_000_000

// imgCacheSize bounds how many decoded images NewImgCache keeps in memory at
// once - each entry holds full pixel data, so this is a small multiple of
// "the current image plus its immediate neighbors", not the whole set.
const imgCacheSize = 16

// NewImgCache builds the LRU cache callers use to hold recently decoded
// images. lru.New only errors for a non-positive size, which imgCacheSize
// never is, so a failure here would be a programmer error worth crashing on
// immediately rather than limping along with a nil cache.
func NewImgCache() *lru.Cache[string, *LoadedImage] {
	c, err := lru.New[string, *LoadedImage](imgCacheSize)
	if err != nil {
		panic(err)
	}
	return c
}

// InvalidDimensionsError reports that an image's header declared dimensions
// ReadAndProbe rejects: zero, negative, or large enough to be a
// decompression-bomb risk.
type InvalidDimensionsError struct {
	w, h int
}

func (e *InvalidDimensionsError) Error() string {
	return fmt.Sprintf("invalid image dimensions %dx%d", e.w, e.h)
}

// ctxReader wraps r so a Read call fails with ctx's error once ctx is
// done, instead of running r's Read to completion for a result a
// cancelled load has already discarded. readRawBytes's io.ReadAll loop
// calls Read repeatedly for anything bigger than one chunk, so this stops
// a large or slow (e.g. network-mounted) file's read partway through
// rather than only catching the cancellation before the next file starts.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}

// readRawBytes reads u's entire contents into memory - the first step
// shared by ReadAndProbe (which goes on to decode the header) and
// CaptureDate (which only needs the bytes to walk for Exif). ctx is
// checked once up front, before even opening u - cheap enough there's no
// reason not to, mirroring internal/filesort's Order - and then on every
// Read the io.ReadAll loop makes, via ctxReader, so a load abandoned
// partway through a large read stops doing I/O for it instead of finishing
// unseen.
func readRawBytes(ctx context.Context, u fyne.URI) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rc, err := storage.Reader(u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(ctxReader{ctx: ctx, r: rc})
}

// CaptureDate reads u's raw bytes and returns its Exif capture date (see
// Metadata.DateTakenTime), without decoding pixels or building the rest of
// Metadata - the one field internal/filesort's capture-date sort mode
// actually needs. ok is false if u can't be read or carries no recognizable
// capture date, mirroring ReadMetadata's tolerant-failure style; callers
// are expected to fall back to the file's mtime in that case. Uses
// context.Background() rather than taking a ctx of its own: filesort.Order
// already checks its own ctx once per file before calling this (see its
// own doc comment), which is the granularity that sort needs; the read
// itself is small enough not to need a second, finer-grained cancellation
// point on top of that.
func CaptureDate(u fyne.URI) (time.Time, bool) {
	data, err := readRawBytes(context.Background(), u)
	if err != nil {
		return time.Time{}, false
	}

	t := ReadMetadata(data).DateTakenTime
	return t, !t.IsZero()
}

// ReadAndProbe reads u's raw bytes and decodes just its header - via
// image.DecodeConfig, so no pixel data is touched - to learn its final
// display size and reject a zero or absurdly large one instantly, without
// paying for a full decode that was only going to be thrown away. bounds
// already accounts for any Exif orientation swap (a 90/270 degree rotation
// exchanges width and height), so a caller can resize the window to it
// ahead of the full pixel decode in DecodeLoaded. This is also the natural
// hook for a future downsampling pass on huge-but-valid images.
//
// ctx is threaded through to readRawBytes, which is where the actual I/O
// happens - see its own comment. A caller (internal/ui's attemptLoad/
// preloadOne) whose generation has been superseded by a newer navigation
// or drop cancels ctx instead of just discarding the result once it comes
// back, so an abandoned load stops doing I/O instead of finishing unseen.
func ReadAndProbe(ctx context.Context, u fyne.URI) (data []byte, bounds image.Rectangle, err error) {
	data, err = readRawBytes(ctx, u)

	if err != nil {
		return nil, image.Rectangle{}, err
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))

	if err != nil {
		return nil, image.Rectangle{}, err
	}

	w, h := cfg.Width, cfg.Height

	if w <= 0 || h <= 0 || int64(w)*int64(h) > maxImagePixels {
		return nil, image.Rectangle{}, &InvalidDimensionsError{w: w, h: h}
	}

	if o := readEXIFOrientation(data); o >= 5 && o <= 8 {
		w, h = h, w
	}

	return data, image.Rect(0, 0, w, h), nil
}

// DecodeLoaded finishes decoding data - already read and header-validated by
// ReadAndProbe - applying EXIF orientation correction where present. Only
// JPEG files carry an Exif orientation tag in practice; readEXIFOrientation
// returns 1 (no correction) for anything else. Animated GIFs are decoded to
// every frame instead of just the first.
//
// ctx is checked once, up front, rather than threaded into the decode
// itself: unlike ReadAndProbe's file read, decoding already-in-memory
// bytes doesn't block on external I/O, so there's no slow operation to
// interrupt mid-flight - only a possibly-wasted one to skip entirely if
// ctx is already done by the time this runs (e.g. a generation that went
// stale while queued behind preloadOne's semaphore).
func DecodeLoaded(ctx context.Context, data []byte) (*LoadedImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if frames, delays := decodeAnimatedGIF(data); len(frames) > 1 {
		return &LoadedImage{Frames: frames, Delays: delays}, nil
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))

	if err != nil {
		return nil, err
	}

	return &LoadedImage{Frames: []image.Image{ApplyOrientation(decoded, readEXIFOrientation(data))}}, nil
}

// LoadImage reads and decodes an image file of any format registered with
// the image package (JPEG, PNG, GIF, WebP, BMP, TIFF, ICO, XPM, HEIC, AVIF)
// - see ReadAndProbe and
// DecodeLoaded, which callers wanting to resize a window ahead of the full
// pixel decode call separately instead. Uses context.Background() rather
// than taking a ctx of its own: its only caller, LoadThumbnail, is read by
// internal/ui/grid's own bounded worker pool, which has its own staleness
// guard (a generation the caller checks against once a thumbnail comes
// back) rather than the cancellable-context one internal/ui's main decode
// path (ShowImage/attemptLoad/preloadOne) uses.
func LoadImage(u fyne.URI) (*LoadedImage, error) {
	data, _, err := ReadAndProbe(context.Background(), u)

	if err != nil {
		return nil, err
	}

	return DecodeLoaded(context.Background(), data)
}
