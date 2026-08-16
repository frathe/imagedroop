package imaging

import (
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"github.com/gen2brain/avif"
)

// jpegSaveQuality is used instead of image/jpeg's own default (75, quite
// lossy) since SaveRotated is re-encoding a photo that was very likely
// already JPEG-compressed once; keeping it high limits how much additional
// generation loss one rotate-and-save round trip adds.
const jpegSaveQuality = 95

// encoders maps a lowercased file extension to the function that encodes an
// image.Image in that format. Every entry's decoder is already linked into
// the binary for IsSupportedImage's sake (see the package doc's import
// block), so adding the matching Encode call here costs nothing extra.
// WebP and HEIC are decode-only in the libraries this module depends on
// (golang.org/x/image/webp and github.com/gen2brain/heic expose no Encode),
// and ICO/XPM aren't meaningful save targets for a rotated photo, so none of
// the four appear here - CanEncode reports false for them, and SaveRotated
// refuses before touching the file.
var encoders = map[string]func(io.Writer, image.Image) error{
	".jpg":  encodeJPEGForSave,
	".jpeg": encodeJPEGForSave,
	".jpe":  encodeJPEGForSave,
	".jfif": encodeJPEGForSave,
	".png":  png.Encode,
	".gif":  func(w io.Writer, img image.Image) error { return gif.Encode(w, img, nil) },
	".bmp":  bmp.Encode,
	".tif":  func(w io.Writer, img image.Image) error { return tiff.Encode(w, img, nil) },
	".tiff": func(w io.Writer, img image.Image) error { return tiff.Encode(w, img, nil) },
	".avif": func(w io.Writer, img image.Image) error { return avif.Encode(w, img) },
}

func encodeJPEGForSave(w io.Writer, img image.Image) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: jpegSaveQuality})
}

// CanEncode reports whether SaveRotated has an encoder for u's format, so a
// caller (internal/ui's canSaveRotation) can decide whether to offer saving
// at all instead of finding out only after attempting it.
func CanEncode(u fyne.URI) bool {
	ext := u.Extension()
	if path, err := filepath.EvalSymlinks(u.Path()); err == nil {
		ext = filepath.Ext(path)
	}
	_, ok := encoders[strings.ToLower(ext)]
	return ok
}

// SaveRotated writes img - a caller's already-rotated, already-oriented
// frame, typically internal/ui's v.img.Image - back to u, re-encoded in the
// target file's format, replacing the file's previous contents.
// Encoding a fresh raster this way (rather than patching the original
// bytes) does not preserve any Exif metadata the original file carried -
// SaveRotated is a plain pixel round-trip, not a metadata-preserving edit.
//
// It resolves a symlink before writing, so saving an image opened through a
// link updates the target instead of replacing the link itself. It writes to
// a temp file in the target's directory first and renames it over the target
// only once the encode has fully succeeded, so a failed or
// interrupted encode can never leave the original file truncated or
// corrupted. The replacement keeps the original file's permission bits.
func SaveRotated(u fyne.URI, img image.Image) error {
	path, err := filepath.EvalSymlinks(u.Path())
	if err != nil {
		return err
	}

	ext := filepath.Ext(path)
	encode, ok := encoders[strings.ToLower(ext)]
	if !ok {
		return &UnsupportedSaveFormatError{ext: ext}
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".imagedrop-save-*"+ext)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// A no-op once the rename below has already moved tmpPath to path; left
	// unchecked deliberately, since its only job left by then is to clean up
	// on an error return above.
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := encode(tmp, img); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// UnsupportedSaveFormatError reports that SaveRotated has no encoder for a
// file's extension.
type UnsupportedSaveFormatError struct {
	ext string
}

func (e *UnsupportedSaveFormatError) Error() string {
	return "saving " + e.ext + " images isn't supported"
}
