// Package imaging reads, decodes, EXIF-orients, and caches the image files
// PicFetch displays: JPEG, PNG, GIF (including animated), WebP, BMP,
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
	"sync/atomic"
	"time"

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

	// HasEXIF reports whether ReadMetadata found anything in the raw bytes
	// this was decoded from - what the info overlay uses to decide whether
	// offering its "Show EXIF data" link means anything. Filled in by the
	// caller alongside FileSize rather than by DecodeLoaded itself, since
	// the thumbnail path decodes through here too and has no use for it.
	HasEXIF bool

	// AnimationTruncated reports that this was a multi-frame GIF whose
	// composited frames would have exceeded the animation budget, so only
	// its first frame was decoded. The image still displays - it just
	// doesn't move - which is a far better outcome for a valid file than
	// refusing it outright; internal/ui says so with a toast.
	AnimationTruncated bool
}

// maxImagePixels caps the pixel count a decoded image header is allowed to
// declare. It guards against decompression-bomb files that claim far more
// pixels than any real photo needs: fully decoding one - or even resizing
// the window to it - could exhaust memory well before any actual pixel data
// is touched. 200 megapixels comfortably covers real-world panoramas and
// professional camera output.
const maxImagePixels = 200_000_000

// DefaultImgCacheBytes is the shipped byte budget for NewImgCache, until
// the settings window (internal/ui/settingswin) changes it. Bounded in
// bytes rather than entries because a decoded image ranges over four orders
// of magnitude in size: 16 entries could mean 2 MB or 12 GB, which is no
// bound at all. 512 MB holds a good run of ordinary photos while staying
// well inside what a desktop can spare.
const DefaultImgCacheBytes = 512 << 20

// NewImgCache builds the byte-bounded cache callers use to hold recently
// decoded images, weighing each entry by the pixel memory all of its frames
// retain - see loadedImageBytes, and ByteCache for the eviction rule that
// keeps the image currently on screen resident even when it alone exceeds
// budget.
func NewImgCache(budget int64) *ByteCache[*LoadedImage] {
	return NewByteCache(budget, loadedImageBytes)
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

// DefaultMaxEncodedBytes is the shipped ceiling on a file's *encoded* size,
// until the settings window (internal/ui/settingswin) changes it. Sized off
// what real cameras actually produce rather than off an arbitrary small
// number: a 100-megapixel uncompressed TIFF lands near 600 MB, so 512 MB is
// deliberately generous while still stopping a file that would blow out
// memory before a single pixel is decoded. maxImagePixels above bounds the
// *decoded* side; this bounds the read that precedes it.
const DefaultMaxEncodedBytes = 512 << 20

// maxEncodedBytes is the live limit MaxEncodedBytes reports. Package-level
// rather than threaded through ReadAndProbe/LoadImage/CaptureDate/
// LoadThumbnail (and on into internal/filesort's Order) because it is a
// genuinely process-wide decode policy, not per-viewer state - the same
// reason clipboard.CopyImage and filepicker.Choose are package vars. Atomic
// because the settings window writes it on the UI goroutine while
// preloadOne's and the grid's background decodes read it.
var maxEncodedBytes atomic.Int64

// MaxEncodedBytes reports the current encoded-size ceiling. Zero means
// "never set", falling back to the shipped default - the same
// zero-means-unset sentinel internal/preferences uses for every numeric
// preference.
func MaxEncodedBytes() int64 {
	if n := maxEncodedBytes.Load(); n > 0 {
		return n
	}

	return DefaultMaxEncodedBytes
}

// SetMaxEncodedBytes changes the encoded-size ceiling - the settings
// window's binding, via internal/ui's SetMaxFileSizeMB. Applies to the next
// read; one already in flight finishes under the limit it started with.
func SetMaxEncodedBytes(n int64) {
	maxEncodedBytes.Store(n)
}

// InputTooLargeError reports that a file's encoded bytes exceed
// MaxEncodedBytes, so it was never read into memory in full. Distinct from
// InvalidDimensionsError because the two say different things to the user:
// that one means the file claims a size no real image has, this one means
// the file is real but larger than the limit they set.
type InputTooLargeError struct {
	limit int64
}

func (e *InputTooLargeError) Error() string {
	return fmt.Sprintf("file exceeds the %d-byte input limit", e.limit)
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

// readRawBytes reads u's contents into memory, up to MaxEncodedBytes - the
// first step shared by ReadAndProbe (which goes on to decode the header)
// and CaptureDate (which only needs the bytes to walk for Exif). ctx is
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
	defer func() { _ = rc.Close() }()

	// Read one byte past the limit rather than exactly up to it: io.ReadAll
	// on a LimitReader returns the same short slice whether the file ended
	// at the limit or was cut off there, so the extra byte is what
	// distinguishes "fits exactly" from "too large".
	limit := MaxEncodedBytes()

	data, err := io.ReadAll(io.LimitReader(ctxReader{ctx: ctx, r: rc}, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > limit {
		return nil, &InputTooLargeError{limit: limit}
	}

	return data, nil
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
func DecodeLoaded(ctx context.Context, data []byte, maxAnimBytes int64) (*LoadedImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	frames, delays, truncated := decodeAnimatedGIF(data, maxAnimBytes)

	if len(frames) > 1 {
		return &LoadedImage{Frames: frames, Delays: delays}, nil
	}

	// An animation refused for its size falls through to exactly the same
	// path a static image takes, since image.Decode on a GIF returns its
	// first frame - so the static fallback costs nothing beyond carrying
	// the flag out to the caller.
	decoded, _, err := image.Decode(bytes.NewReader(data))

	if err != nil {
		return nil, err
	}

	return &LoadedImage{
		Frames:             []image.Image{ApplyOrientation(decoded, readEXIFOrientation(data))},
		AnimationTruncated: truncated,
	}, nil
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
func LoadImage(u fyne.URI, maxAnimBytes int64) (*LoadedImage, error) {
	data, _, err := ReadAndProbe(context.Background(), u)

	if err != nil {
		return nil, err
	}

	return DecodeLoaded(context.Background(), data, maxAnimBytes)
}
