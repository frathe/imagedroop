package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"github.com/fyne-io/image/ico"
	"github.com/gen2brain/avif"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// TestMain registers the fyne test app so storage.NewFileURI's "file" scheme
// is resolvable; without it, every test that reads a temp file through a
// fyne.URI fails with "no repository registered for scheme 'file'".
func TestMain(m *testing.M) {
	test.NewApp()
	os.Exit(m.Run())
}

// fakeURI is a minimal fyne.URI so tests can control extension and MIME
// type independently, without touching the filesystem.
type fakeURI struct {
	name, ext, mime string
}

func (f fakeURI) Extension() string { return f.ext }
func (f fakeURI) Name() string      { return f.name }
func (f fakeURI) MimeType() string  { return f.mime }
func (f fakeURI) Scheme() string    { return "file" }
func (f fakeURI) Authority() string { return "" }
func (f fakeURI) Path() string      { return "/" + f.name }
func (f fakeURI) Query() string     { return "" }
func (f fakeURI) Fragment() string  { return "" }
func (f fakeURI) String() string    { return "file:///" + f.name }

func TestIsSupportedImage(t *testing.T) {
	cases := []struct {
		name string
		u    fakeURI
		want bool
	}{
		{"lowercase .jpg", fakeURI{name: "a.jpg", ext: ".jpg"}, true},
		{"uppercase .JPG", fakeURI{name: "a.JPG", ext: ".JPG"}, true},
		{".jpeg", fakeURI{name: "a.jpeg", ext: ".jpeg"}, true},
		{".jpe", fakeURI{name: "a.jpe", ext: ".jpe"}, true},
		{".jfif", fakeURI{name: "a.jfif", ext: ".jfif"}, true},
		{".png", fakeURI{name: "a.png", ext: ".png"}, true},
		{".gif", fakeURI{name: "a.gif", ext: ".gif"}, true},
		{".webp", fakeURI{name: "a.webp", ext: ".webp"}, true},
		{"uppercase .WEBP", fakeURI{name: "a.WEBP", ext: ".WEBP"}, true},
		{".bmp", fakeURI{name: "a.bmp", ext: ".bmp"}, true},
		{".tif", fakeURI{name: "a.tif", ext: ".tif"}, true},
		{".tiff", fakeURI{name: "a.tiff", ext: ".tiff"}, true},
		{".ico", fakeURI{name: "a.ico", ext: ".ico"}, true},
		{".xpm", fakeURI{name: "a.xpm", ext: ".xpm"}, true},
		{".heic", fakeURI{name: "a.heic", ext: ".heic"}, true},
		{".heif", fakeURI{name: "a.heif", ext: ".heif"}, true},
		{"uppercase .HEIC", fakeURI{name: "a.HEIC", ext: ".HEIC"}, true},
		{".avif", fakeURI{name: "a.avif", ext: ".avif"}, true},
		{"no extension, no mime", fakeURI{name: "a", ext: ""}, false},
		{"mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/jpeg"}, true},
		{"png mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/png"}, true},
		{"webp mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/webp"}, true},
		{"bmp mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/bmp"}, true},
		{"tiff mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/tiff"}, true},
		{"ico mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/x-icon"}, true},
		{"ms-icon mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/vnd.microsoft.icon"}, true},
		{"xpm mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/x-xpixmap"}, true},
		{"heic mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/heic"}, true},
		{"heif mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/heif"}, true},
		{"avif mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/avif"}, true},
		{"mime is case-insensitive", fakeURI{name: "a.bin", ext: ".bin", mime: "IMAGE/JPEG"}, true},
		{"wrong mime, wrong extension", fakeURI{name: "a.txt", ext: ".txt", mime: "text/plain"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSupportedImage(c.u); got != c.want {
				t.Errorf("IsSupportedImage(%+v) = %v, want %v", c.u, got, c.want)
			}
		})
	}
}

// --- LoadImage ----------------------------------------------------------

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	return path
}

func encodeJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return buf.Bytes()
}

func encodePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return buf.Bytes()
}

func encodeGIF(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	palette := color.Palette{color.White, c}
	img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}

	return buf.Bytes()
}

func encodeBMP(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		t.Fatalf("encode bmp: %v", err)
	}

	return buf.Bytes()
}

func encodeTIFF(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := tiff.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode tiff: %v", err)
	}

	return buf.Bytes()
}

func encodeICO(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := ico.Encode(&buf, img); err != nil {
		t.Fatalf("encode ico: %v", err)
	}

	return buf.Bytes()
}

// encodeXPM builds a minimal single-color XPM (1 color, 1 char per pixel),
// following the subset of the format internal/imaging's registered xpm
// decoder (github.com/fyne-io/image/xpm) parses: a header comment, a
// "width height ncolors chars-per-pixel" line, one "id c #RRGGBB" color
// line, then height rows of width repeated id characters.
func encodeXPM(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	r, g, b, _ := c.RGBA()
	// bytes.Buffer.Write/WriteString and fmt.Fprintf into it never return a
	// non-nil error, so every result below is ignored deliberately.
	var buf bytes.Buffer
	_, _ = buf.WriteString("/* XPM */\n")
	_, _ = buf.WriteString("static char * img_xpm[] = {\n")
	_, _ = fmt.Fprintf(&buf, "\"%d %d 1 1\",\n", w, h)
	_, _ = fmt.Fprintf(&buf, "\"X c #%02x%02x%02x\",\n", r>>8, g>>8, b>>8)
	for y := range h {
		_, _ = buf.WriteString("\"")
		_, _ = buf.WriteString(strings.Repeat("X", w))
		_, _ = buf.WriteString("\"")
		if y < h-1 {
			_, _ = buf.WriteString(",")
		}
		_, _ = buf.WriteString("\n")
	}
	_, _ = buf.WriteString("};\n")

	return buf.Bytes()
}

func encodeAVIF(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := avif.Encode(&buf, img); err != nil {
		t.Fatalf("encode avif: %v", err)
	}

	return buf.Bytes()
}

// encodeAnimatedGIF builds a multi-frame GIF, one solid-color w x h frame
// per entry in colors, with the matching delay (in 1/100ths of a second,
// gif.GIF's native unit) from delays.
func encodeAnimatedGIF(t *testing.T, w, h int, colors []color.Color, delays []int) []byte {
	t.Helper()

	g := &gif.GIF{}

	for i, c := range colors {
		palette := color.Palette{color.White, c}
		frame := image.NewPaletted(image.Rect(0, 0, w, h), palette)
		for y := range h {
			for x := range w {
				frame.Set(x, y, c)
			}
		}

		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, delays[i])
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encode animated gif: %v", err)
	}

	return buf.Bytes()
}

func TestLoadImage(t *testing.T) {
	t.Run("valid jpeg", func(t *testing.T) {
		path := writeTempFile(t, "photo.jpg", encodeJPEG(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid png", func(t *testing.T) {
		path := writeTempFile(t, "photo.png", encodePNG(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid single-frame gif", func(t *testing.T) {
		path := writeTempFile(t, "photo.gif", encodeGIF(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a single-frame gif", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid bmp", func(t *testing.T) {
		path := writeTempFile(t, "photo.bmp", encodeBMP(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid tiff", func(t *testing.T) {
		path := writeTempFile(t, "photo.tiff", encodeTIFF(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid ico", func(t *testing.T) {
		path := writeTempFile(t, "photo.ico", encodeICO(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid xpm", func(t *testing.T) {
		path := writeTempFile(t, "photo.xpm", encodeXPM(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid avif", func(t *testing.T) {
		path := writeTempFile(t, "photo.avif", encodeAVIF(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		b := loaded.Frames[0].Bounds()
		if b.Dx() != 12 || b.Dy() != 8 {
			t.Errorf("decoded size = %dx%d, want 12x8", b.Dx(), b.Dy())
		}
	})

	t.Run("valid heic, EXIF orientation already applied by the decoder", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join("testdata", "test_exif.heic"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		path := writeTempFile(t, "photo.heic", data)

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 1 {
			t.Fatalf("frames = %d, want 1 for a static image", len(loaded.Frames))
		}

		// The fixture carries Exif orientation 6 (a 90-degree rotation); the
		// heic decoder already applies it before returning pixels, and
		// readEXIFOrientation only recognizes the JPEG APP1 container, so it
		// correctly no-ops on a HEIC file's bytes. If LoadImage applied the
		// rotation a second time on top of the decoder's own correction,
		// these bounds would come out swapped.
		b := loaded.Frames[0].Bounds()
		if b.Dx() != 480 || b.Dy() != 640 {
			t.Errorf("decoded size = %dx%d, want 480x640 (EXIF-corrected once, not twice)", b.Dx(), b.Dy())
		}
	})

	t.Run("valid animated gif", func(t *testing.T) {
		path := writeTempFile(t, "anim.gif", encodeAnimatedGIF(t, 12, 8,
			[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
			[]int{5, 10}))

		loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("LoadImage returned error: %v", err)
		}

		if len(loaded.Frames) != 2 {
			t.Fatalf("frames = %d, want 2 for a 2-frame animated gif", len(loaded.Frames))
		}

		if got, want := loaded.Delays[0], 50*time.Millisecond; got != want {
			t.Errorf("delays[0] = %v, want %v", got, want)
		}
		if got, want := loaded.Delays[1], 100*time.Millisecond; got != want {
			t.Errorf("delays[1] = %v, want %v", got, want)
		}
	})

	t.Run("corrupt file", func(t *testing.T) {
		path := writeTempFile(t, "corrupt.jpg", []byte("this is not a jpeg"))

		if _, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes); err == nil {
			t.Fatal("expected an error decoding a corrupt file, got nil")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.jpg")

		if _, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes); err == nil {
			t.Fatal("expected an error reading a missing file, got nil")
		}
	})

	t.Run("rejects an absurd header-declared size without a full decode", func(t *testing.T) {
		path := writeTempFile(t, "bomb.png", truncatedPNGHeader(t, 60000, 60000))

		_, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
		if err == nil {
			t.Fatal("expected an error for a decompression-bomb-sized header, got nil")
		}

		// The file has no IDAT/IEND chunks, so it cannot be fully decoded -
		// any error other than InvalidDimensionsError would mean the header
		// check didn't catch it first and a full decode was attempted (and
		// failed) instead.
		var dimErr *InvalidDimensionsError
		if !errors.As(err, &dimErr) {
			t.Fatalf("err = %v, want an *InvalidDimensionsError", err)
		}
	})
}

// truncatedPNGHeader builds a PNG file containing only the 8-byte signature
// and a single, correctly-checksummed IHDR chunk declaring width x height -
// no IDAT/IEND, so it's useless for a full decode but perfectly readable by
// image.DecodeConfig, which for a non-paletted color type stops as soon as
// IHDR has been parsed. Used to prove ReadAndProbe/LoadImage reject an
// invalid declared size from the header alone, without needing the rest of
// the file.
func truncatedPNGHeader(t *testing.T, width, height uint32) []byte {
	t.Helper()

	// bytes.Buffer.Write/WriteString and hash.Hash.Write never return a
	// non-nil error, so every result below is ignored deliberately.
	var buf bytes.Buffer
	_, _ = buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})

	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8] = 8 // bit depth
	data[9] = 6 // color type: truecolor with alpha
	// data[10:13] (compression/filter/interlace methods) are left at 0.

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	_, _ = buf.Write(length[:])

	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte("IHDR"))
	_, _ = crc.Write(data)

	_, _ = buf.WriteString("IHDR")
	_, _ = buf.Write(data)

	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc.Sum32())
	_, _ = buf.Write(crcBytes[:])

	return buf.Bytes()
}

// halfRedHalfBlueJPEG encodes a w x h image with a red left half and a blue
// right half, then splices in an APP1 Exif segment declaring orientation, so
// LoadImage's correction can be checked against a real (lossy) JPEG file
// rather than just the in-memory transform functions.
func halfRedHalfBlueJPEG(t *testing.T, w, h int, orientation uint16) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if x < w/2 {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	data := buf.Bytes()

	exif := wrapAsAPP1(buildExifSegment(t, orientation, false))
	out := append([]byte{}, data[:2]...)
	out = append(out, exif...)
	out = append(out, data[2:]...)

	return out
}

func TestDecodeJPEG_AppliesEXIFOrientation(t *testing.T) {
	// A 20x10 image, red on the left, blue on the right. Orientation 6 asks
	// for a 90 degree clockwise rotation, which moves the (red) left edge to
	// the top: the corrected image should be 10x20 with red on top.
	path := writeTempFile(t, "rotated.jpg", halfRedHalfBlueJPEG(t, 20, 10, 6))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage returned error: %v", err)
	}

	img := loaded.Frames[0]
	b := img.Bounds()
	if b.Dx() != 10 || b.Dy() != 20 {
		t.Fatalf("decoded size = %dx%d, want 10x20 after a 90-degree correction", b.Dx(), b.Dy())
	}

	// Sample well away from the seam and the image edges to avoid JPEG
	// ringing artifacts.
	r, _, b2, _ := img.At(5, 5).RGBA()
	if r < b2 {
		t.Errorf("top of corrected image: R=%d B=%d, want red to dominate", r, b2)
	}

	r, _, b2, _ = img.At(5, 15).RGBA()
	if b2 < r {
		t.Errorf("bottom of corrected image: R=%d B=%d, want blue to dominate", r, b2)
	}
}

// TestReadAndProbe_AccountsForEXIFOrientation checks that ReadAndProbe's
// bounds - computed from the header alone - already reflect the swap a
// 90/270 degree Exif rotation applies, using the same file as
// TestDecodeJPEG_AppliesEXIFOrientation. A caller resizing the window from
// these bounds ahead of the full decode would otherwise size it for the raw
// 20x10 header and have to resize again once DecodeLoaded corrects it to
// 10x20.
func TestReadAndProbe_AccountsForEXIFOrientation(t *testing.T) {
	path := writeTempFile(t, "rotated.jpg", halfRedHalfBlueJPEG(t, 20, 10, 6))

	_, bounds, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("ReadAndProbe returned error: %v", err)
	}

	if bounds.Dx() != 10 || bounds.Dy() != 20 {
		t.Errorf("bounds = %dx%d, want 10x20 after accounting for the 90-degree orientation swap", bounds.Dx(), bounds.Dy())
	}
}

// --- the encoded-input limit -------------------------------------------------

// withMaxEncodedBytes points the process-wide limit at n for the duration of
// one test and puts it back afterward. Zero restores "never set", which
// MaxEncodedBytes reports as the shipped default - see its own comment.
func withMaxEncodedBytes(t *testing.T, n int64) {
	t.Helper()

	SetMaxEncodedBytes(n)
	t.Cleanup(func() { SetMaxEncodedBytes(0) })
}

func TestMaxEncodedBytes_UnsetReportsTheShippedDefault(t *testing.T) {
	if got := MaxEncodedBytes(); got != DefaultMaxEncodedBytes {
		t.Errorf("MaxEncodedBytes() = %d with nothing set, want the default %d", got, DefaultMaxEncodedBytes)
	}

	withMaxEncodedBytes(t, 1234)

	if got := MaxEncodedBytes(); got != 1234 {
		t.Errorf("MaxEncodedBytes() = %d after SetMaxEncodedBytes(1234), want 1234", got)
	}
}

func TestReadAndProbe_RejectsInputPastTheEncodedSizeLimit(t *testing.T) {
	data := encodeJPEG(t, 40, 40, color.RGBA{R: 200, G: 20, B: 20, A: 255})
	path := writeTempFile(t, "big.jpg", data)

	// One byte short of the file's real size, so the read is refused on
	// size alone - the file itself is a perfectly valid JPEG, which is what
	// distinguishes this from the InvalidDimensionsError case.
	withMaxEncodedBytes(t, int64(len(data))-1)

	_, _, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))

	var tooBig *InputTooLargeError
	if !errors.As(err, &tooBig) {
		t.Fatalf("ReadAndProbe() = %v, want an *InputTooLargeError", err)
	}
	if tooBig.limit != int64(len(data))-1 {
		t.Errorf("error's limit = %d, want %d", tooBig.limit, int64(len(data))-1)
	}
}

// TestReadAndProbe_AcceptsInputExactlyAtTheEncodedSizeLimit is the boundary
// readRawBytes' limit+1 LimitReader exists for: without the extra byte,
// io.ReadAll can't tell a file that ended at the limit from one truncated
// there, and a file of exactly the permitted size would be rejected.
func TestReadAndProbe_AcceptsInputExactlyAtTheEncodedSizeLimit(t *testing.T) {
	data := encodeJPEG(t, 40, 40, color.RGBA{R: 200, G: 20, B: 20, A: 255})
	path := writeTempFile(t, "exact.jpg", data)

	withMaxEncodedBytes(t, int64(len(data)))

	if _, _, err := ReadAndProbe(context.Background(), storage.NewFileURI(path)); err != nil {
		t.Errorf("ReadAndProbe() = %v for a file exactly at the limit, want no error", err)
	}
}

// CaptureDate reads through the same limited path, and the sort that calls it
// walks every file in the set - so an oversized file has to fall back to its
// mtime rather than stalling or blowing up the sort.
func TestCaptureDate_ReportsNotOKForInputPastTheEncodedSizeLimit(t *testing.T) {
	data := jpegWithDateTimeOriginal(t, "2021:07:04 09:10:11")
	path := writeTempFile(t, "dated.jpg", data)

	withMaxEncodedBytes(t, int64(len(data))-1)

	if _, ok := CaptureDate(storage.NewFileURI(path)); ok {
		t.Error("CaptureDate() reported ok for a file past the encoded-size limit, want false")
	}
}

// TestReadAndProbe_StopsEarlyWhenContextIsCancelled mirrors
// internal/filesort's TestOrder_StopsEarlyWhenContextIsCancelled: an
// already-cancelled ctx, checked deterministically rather than by racing a
// real cancellation against a real read. u is a perfectly valid, readable
// file, so a returned context.Canceled can only mean the ctx check itself
// stopped the read - any other error would mean it fell through to a real
// (and here, impossible) read failure instead.
func TestReadAndProbe_StopsEarlyWhenContextIsCancelled(t *testing.T) {
	path := writeTempFile(t, "photo.jpg", encodeJPEG(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := ReadAndProbe(ctx, storage.NewFileURI(path))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ReadAndProbe() with an already-cancelled ctx = %v, want context.Canceled", err)
	}
}

// TestDecodeLoaded_StopsEarlyWhenContextIsCancelled is
// TestReadAndProbe_StopsEarlyWhenContextIsCancelled for DecodeLoaded's own
// ctx check: data is a perfectly valid, already-read JPEG, so a returned
// context.Canceled can only mean DecodeLoaded checked ctx before spending
// any time decoding pixels for a result its caller has already discarded.
func TestDecodeLoaded_StopsEarlyWhenContextIsCancelled(t *testing.T) {
	data := encodeJPEG(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DecodeLoaded(ctx, data, DefaultImgCacheBytes)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("DecodeLoaded() with an already-cancelled ctx = %v, want context.Canceled", err)
	}
}

// countingReader wraps an io.Reader and counts how many times Read was
// actually called on it - ctxReader's job is to stop calling into this
// once its context is cancelled, so a reads count higher than expected
// would mean the cancellation was checked too late (or not at all).
// onRead, if set, runs after the underlying Read - here, used to cancel
// the context as a side effect of the first Read, standing in for a newer
// generation superseding this one midway through a real, larger file's
// transfer.
type countingReader struct {
	data   []byte
	pos    int
	reads  int
	onRead func()
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.onRead != nil {
		r.onRead()
	}
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// TestCtxReader_StopsMidStreamOnceContextIsCancelled proves ctxReader
// actually interrupts a read in progress instead of only checking ctx
// once up front - the difference that lets readRawBytes stop a large or
// slow (e.g. network-mounted) file's transfer partway through rather than
// only catching a cancellation before the next file starts.
func TestCtxReader_StopsMidStreamOnceContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	underlying := &countingReader{data: []byte("0123456789"), onRead: cancel}
	r := ctxReader{ctx: ctx, r: underlying}

	buf := make([]byte, 4)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("first Read returned error: %v", err)
	}
	if n != 4 {
		t.Fatalf("first Read returned n=%d, want 4", n)
	}

	if _, err := r.Read(buf); !errors.Is(err, context.Canceled) {
		t.Errorf("second Read after the context was cancelled = %v, want context.Canceled", err)
	}
	if underlying.reads != 1 {
		t.Errorf("underlying reader was Read %d times, want 1 - the second call should have stopped before reaching it", underlying.reads)
	}
}

// jpegWithDateTimeOriginal builds a JPEG carrying only a DateTimeOriginal
// tag - buildFullExifTIFF (exif_test.go) accepts a full fullExifFields, but
// CaptureDate only cares about this one.
func jpegWithDateTimeOriginal(t *testing.T, raw string) []byte {
	t.Helper()

	fullExifTIFF := buildFullExifTIFF(t, fullExifFields{dateTimeOriginal: raw})
	seg := wrapAsAPP1(append([]byte("Exif\x00\x00"), fullExifTIFF...))

	data := encodeJPEG(t, 4, 4, color.White)
	out := append([]byte{}, data[:2]...)
	out = append(out, seg...)
	out = append(out, data[2:]...)
	return out
}

func TestCaptureDate(t *testing.T) {
	t.Run("reads DateTimeOriginal", func(t *testing.T) {
		path := writeTempFile(t, "dated.jpg", jpegWithDateTimeOriginal(t, "2024:08:12 14:33:02"))

		got, ok := CaptureDate(storage.NewFileURI(path))
		if !ok {
			t.Fatal("ok = false, want true")
		}

		want := time.Date(2024, 8, 12, 14, 33, 2, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("CaptureDate() = %v, want %v", got, want)
		}
	})

	t.Run("no Exif data", func(t *testing.T) {
		path := writeTempFile(t, "plain.jpg", encodeJPEG(t, 4, 4, color.White))

		if _, ok := CaptureDate(storage.NewFileURI(path)); ok {
			t.Error("ok = true, want false for a file with no capture date")
		}
	})

	t.Run("unreadable file", func(t *testing.T) {
		missing := storage.NewFileURI(filepath.Join(t.TempDir(), "missing.jpg"))

		if _, ok := CaptureDate(missing); ok {
			t.Error("ok = true, want false for a file that can't be read")
		}
	})
}

// --- SVG ------------------------------------------------------------------

func TestIsSupportedImageAcceptsSVG(t *testing.T) {
	if !IsSupportedImage(storage.NewFileURI("/tmp/logo.svg")) {
		t.Fatal("*.svg must be a supported image")
	}
	if !IsSupportedImage(storage.NewFileURI("/tmp/LOGO.SVG")) {
		t.Fatal("extension match must be case-insensitive")
	}
}

func TestReadAndProbeSVGReportsLogicalBounds(t *testing.T) {
	path := writeTempFile(t, "icon.svg", svgDoc(`viewBox="0 0 24 24"`))

	_, bounds, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("ReadAndProbe: %v", err)
	}
	if bounds.Dx() != 340 || bounds.Dy() != 340 {
		t.Fatalf("bounds = %dx%d, want the 340x340 logical size", bounds.Dx(), bounds.Dy())
	}
}

func TestReadAndProbeRejectsGigapixelSVGBeforeDecoding(t *testing.T) {
	path := writeTempFile(t, "bomb.svg", svgDoc(`viewBox="0 0 60000 60000"`))

	_, _, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))

	var dimErr *InvalidDimensionsError
	if !errors.As(err, &dimErr) {
		t.Fatalf("err = %v, want InvalidDimensionsError - 3.6 gigapixels must be refused before allocating", err)
	}
}

// The guard the comment at ReadAndProbe's SVG arm already claims: a
// header-declared size is refused before anything is allocated. 4e9 x 4e9
// is the case a bare product test misses - an SVG's axes come from a text
// attribute, so each alone can be large enough that their int64 product
// wraps negative and slips past a "> maxImagePixels" comparison.
func TestReadAndProbeRejectsOverflowingSVGDimensions(t *testing.T) {
	path := writeTempFile(t, "wrap.svg", svgDoc(`viewBox="0 0 4000000000 4000000000"`))

	_, _, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))

	var dimErr *InvalidDimensionsError
	if !errors.As(err, &dimErr) {
		t.Fatalf("err = %v, want InvalidDimensionsError", err)
	}
}

func TestDecodeLoadedSVGCarriesVector(t *testing.T) {
	data := svgDoc(`viewBox="0 0 24 24"`)

	loaded, err := DecodeLoaded(context.Background(), data, 0)
	if err != nil {
		t.Fatalf("DecodeLoaded: %v", err)
	}
	if loaded.Vector == nil {
		t.Fatal("an SVG must carry its Vector so the app can re-rasterize it")
	}
	if len(loaded.Frames) != 1 {
		t.Fatalf("len(Frames) = %d, want 1", len(loaded.Frames))
	}
	if b := loaded.Frames[0].Bounds(); b.Dx() != 340 || b.Dy() != 340 {
		t.Fatalf("first frame = %dx%d, want the 340x340 logical size", b.Dx(), b.Dy())
	}
}

func TestDecodeLoadedRasterCarriesNoVector(t *testing.T) {
	loaded, err := DecodeLoaded(context.Background(), encodePNG(t, 8, 8, color.White), 0)
	if err != nil {
		t.Fatalf("DecodeLoaded: %v", err)
	}
	if loaded.Vector != nil {
		t.Fatal("a raster format must not carry a Vector")
	}
}

func TestCanEncodeRejectsSVG(t *testing.T) {
	if CanEncodeExt(".svg") {
		t.Fatal("SVG must not be encodable, so File > Save Changes stays disabled for one")
	}
}

func TestLoadedImageBytesChargesForVector(t *testing.T) {
	loaded, err := DecodeLoaded(context.Background(), svgDoc(`viewBox="0 0 24 24"`), 0)
	if err != nil {
		t.Fatalf("DecodeLoaded: %v", err)
	}

	frameOnly := imageBytes(loaded.Frames[0])
	if got := loadedImageBytes(loaded); got <= frameOnly {
		t.Fatalf("loadedImageBytes = %d, want more than the frame's own %d", got, frameOnly)
	}
}
