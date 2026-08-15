package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
	var buf bytes.Buffer
	buf.WriteString("/* XPM */\n")
	buf.WriteString("static char * img_xpm[] = {\n")
	fmt.Fprintf(&buf, "\"%d %d 1 1\",\n", w, h)
	fmt.Fprintf(&buf, "\"X c #%02x%02x%02x\",\n", r>>8, g>>8, b>>8)
	for y := 0; y < h; y++ {
		buf.WriteString("\"")
		buf.WriteString(strings.Repeat("X", w))
		buf.WriteString("\"")
		if y < h-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("};\n")

	return buf.Bytes()
}

func encodeAVIF(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		loaded, err := LoadImage(storage.NewFileURI(path))
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

		if _, err := LoadImage(storage.NewFileURI(path)); err == nil {
			t.Fatal("expected an error decoding a corrupt file, got nil")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.jpg")

		if _, err := LoadImage(storage.NewFileURI(path)); err == nil {
			t.Fatal("expected an error reading a missing file, got nil")
		}
	})

	t.Run("rejects an absurd header-declared size without a full decode", func(t *testing.T) {
		path := writeTempFile(t, "bomb.png", truncatedPNGHeader(t, 60000, 60000))

		_, err := LoadImage(storage.NewFileURI(path))
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

	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})

	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8] = 8 // bit depth
	data[9] = 6 // color type: truecolor with alpha
	// data[10:13] (compression/filter/interlace methods) are left at 0.

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	buf.Write(length[:])

	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(data)

	buf.WriteString("IHDR")
	buf.Write(data)

	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc.Sum32())
	buf.Write(crcBytes[:])

	return buf.Bytes()
}

// halfRedHalfBlueJPEG encodes a w x h image with a red left half and a blue
// right half, then splices in an APP1 Exif segment declaring orientation, so
// LoadImage's correction can be checked against a real (lossy) JPEG file
// rather than just the in-memory transform functions.
func halfRedHalfBlueJPEG(t *testing.T, w, h int, orientation uint16) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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

	loaded, err := LoadImage(storage.NewFileURI(path))
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

	_, bounds, err := ReadAndProbe(storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("ReadAndProbe returned error: %v", err)
	}

	if bounds.Dx() != 10 || bounds.Dy() != 20 {
		t.Errorf("bounds = %dx%d, want 10x20 after accounting for the 90-degree orientation swap", bounds.Dx(), bounds.Dy())
	}
}

// jpegWithDateTimeOriginal builds a JPEG carrying only a DateTimeOriginal
// tag - buildFullExifTIFF (exif_test.go) accepts a full fullExifFields, but
// CaptureDate only cares about this one.
func jpegWithDateTimeOriginal(t *testing.T, raw string) []byte {
	t.Helper()

	tiff := buildFullExifTIFF(t, fullExifFields{dateTimeOriginal: raw})
	seg := wrapAsAPP1(append([]byte("Exif\x00\x00"), tiff...))

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
