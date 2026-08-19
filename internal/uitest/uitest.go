// Package uitest provides test fixtures shared by this module's test
// suites: synthetic image files in every format the viewer reads, the temp
// files and URIs to hand them over by, and swap-in stubs for the OS-level
// seams (file chooser, image clipboard).
//
// It exists because Go can't share unexported test helpers across packages,
// and the previous answer to that - copying `writeTempFile`/`encodeJPEG`
// into each package that needed them - doesn't scale to the per-feature
// package split this module is working through. Everything here is
// deliberately viewer-free: fixtures build bytes, files, and URIs, and know
// nothing about the app's own types.
//
// What stays behind, in each package's own _test.go files, is anything that
// reads unexported state: the viewer's channel/WaitGroup wait helpers
// (waitUntilLoaded, settleToast, settleThumbs, ...) synchronize on private
// fields, and keeping them private is what stops those sync primitives from
// leaking into an exported API.
//
// Test-only code in a non-test file is intentional (the same shape as the
// standard library's net/http/httptest): only _test.go files import this,
// so it never reaches a production binary.
package uitest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

// FakeURI is a minimal fyne.URI so tests can control extension and MIME
// type independently, without touching the filesystem.
type FakeURI struct {
	FileName, Ext, Mime string
}

func (f FakeURI) Extension() string { return f.Ext }
func (f FakeURI) Name() string      { return f.FileName }
func (f FakeURI) MimeType() string  { return f.Mime }
func (f FakeURI) Scheme() string    { return "file" }
func (f FakeURI) Authority() string { return "" }
func (f FakeURI) Path() string      { return "/" + f.FileName }
func (f FakeURI) Query() string     { return "" }
func (f FakeURI) Fragment() string  { return "" }
func (f FakeURI) String() string    { return "file:///" + f.FileName }

// WriteTempFile writes data to a uniquely-named file in the test's own temp
// directory and returns its path. The directory is cleaned up by testing.
func WriteTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	return path
}

// TempJPEGURI writes a solid-color w x h JPEG to a temp file and returns its
// file URI - the one-liner behind most of this suite's "give me an image to
// drop" setup.
func TempJPEGURI(t *testing.T, name string, w, h int, c color.Color) fyne.URI {
	t.Helper()

	return storage.NewFileURI(WriteTempFile(t, name, EncodeJPEG(t, w, h, c)))
}

// EncodeJPEG returns a solid-color w x h JPEG.
func EncodeJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solidRGBA(w, h, c), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return buf.Bytes()
}

// SVGBytes builds a synthetic SVG with the given viewBox and a filled rect
// covering it, so a rasterization of it has visibly non-zero pixels.
func SVGBytes(w, h int) []byte {
	return []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`+
			`<rect width="%d" height="%d" fill="#cc0000"/></svg>`,
		w, h, w, h))
}

// TempSVGURI writes SVGBytes to a temp file and returns its URI, mirroring
// TempJPEGURI.
func TempSVGURI(t *testing.T, name string, w, h int) fyne.URI {
	t.Helper()

	return storage.NewFileURI(WriteTempFile(t, name, SVGBytes(w, h)))
}

// EncodePNG returns a solid-color w x h PNG.
func EncodePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, solidRGBA(w, h, c)); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return buf.Bytes()
}

// EncodeGIF returns a single-frame solid-color w x h GIF.
func EncodeGIF(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := gif.Encode(&buf, solidPaletted(w, h, c), nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}

	return buf.Bytes()
}

// EncodeAnimatedGIF builds a multi-frame GIF, one solid-color w x h frame
// per entry in colors, with the matching delay (in 1/100ths of a second,
// gif.GIF's native unit) from delays.
func EncodeAnimatedGIF(t *testing.T, w, h int, colors []color.Color, delays []int) []byte {
	t.Helper()

	g := &gif.GIF{}
	for i, c := range colors {
		g.Image = append(g.Image, solidPaletted(w, h, c))
		g.Delay = append(g.Delay, delays[i])
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encode animated gif: %v", err)
	}

	return buf.Bytes()
}

// CaptureDateJPEG builds a minimal encoded JPEG carrying a single Exif
// DateTime tag (0x0132) set to raw ("YYYY:MM:DD HH:MM:SS") - just enough for
// imaging.CaptureDate, which the capture-date sort mode relies on, to read a
// timestamp back, without needing a real camera-shot fixture.
func CaptureDateJPEG(t *testing.T, w, h int, raw string) []byte {
	t.Helper()

	data := EncodeJPEG(t, w, h, color.White)
	dateBytes := append([]byte(raw), 0) // NUL-terminated ASCII

	const (
		headerSize   = 8 // "II" + magic(2) + IFD0 offset(4)
		ifd0EntryCnt = 1
		ifd0Size     = 2 + ifd0EntryCnt*12 + 4
	)
	valueOffset := uint32(headerSize + ifd0Size)

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }

	var tiff bytes.Buffer
	tiff.WriteString("II")
	tiff.Write(u16(0x002A))
	tiff.Write(u32(headerSize))

	tiff.Write(u16(ifd0EntryCnt))
	tiff.Write(u16(0x0132)) // DateTime
	tiff.Write(u16(2))      // ASCII
	tiff.Write(u32(uint32(len(dateBytes))))
	tiff.Write(u32(valueOffset))
	tiff.Write(u32(0)) // next IFD offset

	tiff.Write(dateBytes)

	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	length := len(seg) + 2
	app1 := append([]byte{0xFF, 0xE1, byte(length >> 8), byte(length)}, seg...)

	out := append([]byte{}, data[:2]...)
	out = append(out, app1...)
	out = append(out, data[2:]...)

	return out
}

// GPSJPEG builds a minimal encoded JPEG carrying an Exif GPS sub-IFD (the
// 0x8825 pointer in IFD0, then the latitude/longitude reference and
// degrees/minutes/seconds tags) for the given signed decimal degrees -
// enough for imaging.ReadMetadata to read a position back, and so for the
// EXIF window's map section to have somewhere to point.
func GPSJPEG(t *testing.T, w, h int, lat, lon float64) []byte {
	t.Helper()

	data := EncodeJPEG(t, w, h, color.White)

	const (
		headerSize  = 8 // "II" + magic(2) + IFD0 offset(4)
		ifd0Size    = 2 + 1*12 + 4
		gpsEntryCnt = 4
		gpsSize     = 2 + gpsEntryCnt*12 + 4
	)
	gpsOffset := uint32(headerSize + ifd0Size)
	valueOffset := gpsOffset + gpsSize

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }

	// Exif carries the hemisphere in its own tag, so the coordinate itself
	// is written unsigned.
	ref := func(v float64, positive, negative string) []byte {
		b := make([]byte, 4)
		if v < 0 {
			copy(b, negative)
		} else {
			copy(b, positive)
		}
		return b
	}

	buf := new(bytes.Buffer)
	buf.WriteString("II")
	buf.Write(u16(0x002A))
	buf.Write(u32(headerSize))

	buf.Write(u16(1))
	buf.Write(u16(0x8825)) // GPSIFDPointer
	buf.Write(u16(4))      // LONG
	buf.Write(u32(1))
	buf.Write(u32(gpsOffset))
	buf.Write(u32(0)) // next IFD offset

	buf.Write(u16(gpsEntryCnt))

	buf.Write(u16(0x0001)) // GPSLatitudeRef
	buf.Write(u16(2))      // ASCII
	buf.Write(u32(2))
	buf.Write(ref(lat, "N", "S"))

	buf.Write(u16(0x0002)) // GPSLatitude
	buf.Write(u16(5))      // RATIONAL
	buf.Write(u32(3))
	buf.Write(u32(valueOffset))

	buf.Write(u16(0x0003)) // GPSLongitudeRef
	buf.Write(u16(2))
	buf.Write(u32(2))
	buf.Write(ref(lon, "E", "W"))

	buf.Write(u16(0x0004)) // GPSLongitude
	buf.Write(u16(5))
	buf.Write(u32(3))
	buf.Write(u32(valueOffset + 24)) // three rationals past the latitude

	buf.Write(u32(0)) // next IFD offset

	buf.Write(dmsRationals(lat))
	buf.Write(dmsRationals(lon))

	seg := append([]byte("Exif\x00\x00"), buf.Bytes()...)
	length := len(seg) + 2
	app1 := append([]byte{0xFF, 0xE1, byte(length >> 8), byte(length)}, seg...)

	out := append([]byte{}, data[:2]...)
	out = append(out, app1...)
	out = append(out, data[2:]...)

	return out
}

// dmsRationals encodes the magnitude of a decimal-degree coordinate as the
// three little-endian unsigned rationals Exif stores it in: whole degrees,
// whole minutes, and seconds to four decimal places (a ten-thousandth of a
// second is well under a millimeter, so nothing meaningful is lost).
func dmsRationals(deg float64) []byte {
	deg = math.Abs(deg)

	d := math.Floor(deg)
	m := math.Floor((deg - d) * 60)
	s := ((deg-d)*60 - m) * 60

	b := make([]byte, 0, 24)
	b = binary.LittleEndian.AppendUint32(b, uint32(d))
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = binary.LittleEndian.AppendUint32(b, uint32(m))
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = binary.LittleEndian.AppendUint32(b, uint32(math.Round(s*10000)))
	b = binary.LittleEndian.AppendUint32(b, 10000)

	return b
}

// TempGPSJPEGURI writes GPSJPEG's output to a temp file and returns its
// URI, mirroring TempJPEGURI.
func TempGPSJPEGURI(t *testing.T, name string, w, h int, lat, lon float64) fyne.URI {
	t.Helper()

	return storage.NewFileURI(WriteTempFile(t, name, GPSJPEG(t, w, h, lat, lon)))
}

// TruncatedPNGHeader builds a PNG file containing only the 8-byte signature
// and a single, correctly-checksummed IHDR chunk declaring width x height -
// no IDAT/IEND, so it's useless for a full decode but perfectly readable by
// image.DecodeConfig, which for a non-paletted color type stops as soon as
// IHDR has been parsed. Used to prove imaging.ReadAndProbe/LoadImage reject
// an invalid declared size from the header alone, without needing the rest
// of the file, and to exercise the viewer's end-to-end handling of that
// same rejection.
func TruncatedPNGHeader(t *testing.T, width, height uint32) []byte {
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

// ApproxEqual reports whether two float32s are within 0.01 of each other -
// the tolerance layout assertions need, since Fyne's sizing math accumulates
// rounding noise well below one canvas point.
func ApproxEqual(a, b float32) bool {
	const eps = 0.01

	d := a - b

	return d > -eps && d < eps
}

func solidRGBA(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}

	return img
}

func solidPaletted(w, h int, c color.Color) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.White, c})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}

	return img
}
