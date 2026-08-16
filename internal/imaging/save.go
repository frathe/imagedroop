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
// at all instead of finding out only after attempting it. It resolves a
// symlink first, matching SaveRotated's own behavior: what governs there is
// the format of the file that will actually be written.
func CanEncode(u fyne.URI) bool {
	ext := u.Extension()
	if path, err := filepath.EvalSymlinks(u.Path()); err == nil {
		ext = filepath.Ext(path)
	}
	return CanEncodeExt(ext)
}

// CanEncodeExt reports whether ext (a leading-dot file extension, as
// filepath.Ext and fyne.URI.Extension both produce, in any case) has an
// encoder. It is the check the export path wants - internal/ui asks it
// about a destination the user just named, which may not exist yet and so
// has no symlink for CanEncode above to resolve.
func CanEncodeExt(ext string) bool {
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
// link updates the target instead of replacing the link itself, and the
// replacement keeps the original file's permission bits. See writeEncoded
// for the atomic write both this and Export go through.
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

	return writeEncoded(path, info.Mode().Perm(), encode, img)
}

// defaultExportPerm is what a file Export creates from scratch gets, the
// same 0644-before-umask a plain os.WriteFile would produce - os.CreateTemp
// opens at 0600, so writeEncoded has to be told the mode either way.
const defaultExportPerm = 0o644

// Export writes img to u as a new file, encoded in *u's* format rather than
// the source image's - the File > "Export as…" actions, and the one way to
// get pixels out of a file this module can decode but not encode (WebP,
// HEIC) or out of a single frame of an animation. Like SaveRotated it is a
// plain pixel round-trip that carries no Exif metadata across.
//
// The destination's extension alone picks the encoder: unlike SaveRotated,
// no symlink is resolved first, since u is a destination the user just
// named rather than a file already open in the viewer, and the format they
// typed is the format they asked for. An existing destination is replaced
// (keeping its own permission bits), atomically, by the same
// temp-file-then-rename writeEncoded gives SaveRotated - so an export over
// a previous copy cannot damage it if the encode fails partway.
func Export(u fyne.URI, img image.Image) error {
	ext := u.Extension()
	encode, ok := encoders[strings.ToLower(ext)]
	if !ok {
		return &UnsupportedSaveFormatError{ext: ext}
	}
	path := u.Path()
	perm := os.FileMode(defaultExportPerm)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	return writeEncoded(path, perm, encode, img)
}

// writeEncoded encodes img into a temp file in path's own directory and
// renames it over path only once the encode has fully succeeded, so a
// failed or interrupted encode can never leave the destination truncated or
// corrupted - and, since the rename is within one directory, never leaves a
// half-written file where the caller asked for a whole one either. Shared
// by SaveRotated (overwriting the file on screen) and Export (writing a
// copy elsewhere), which differ only in how they arrive at path, perm, and
// encode.
func writeEncoded(path string, perm os.FileMode, encode func(io.Writer, image.Image) error, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".imagedrop-save-*"+filepath.Ext(path))
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// A no-op once the rename below has already moved tmpPath to path; left
	// unchecked deliberately, since its only job left by then is to clean up
	// on an error return above.
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(perm); err != nil {
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
