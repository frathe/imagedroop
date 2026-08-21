package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/color"
	"image/jpeg"
	"testing"

	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

func TestIsSupportedImageAcceptsRAW(t *testing.T) {
	cases := []struct {
		name string
		u    fakeURI
		want bool
	}{
		{".cr2", fakeURI{name: "a.cr2", ext: ".cr2"}, true},
		{"uppercase .CR2", fakeURI{name: "a.CR2", ext: ".CR2"}, true},
		{".cr3", fakeURI{name: "a.cr3", ext: ".cr3"}, true},
		{".nef", fakeURI{name: "a.nef", ext: ".nef"}, true},
		{".nrw", fakeURI{name: "a.nrw", ext: ".nrw"}, true},
		{".arw", fakeURI{name: "a.arw", ext: ".arw"}, true},
		{".dng", fakeURI{name: "a.dng", ext: ".dng"}, true},
		{".orf", fakeURI{name: "a.orf", ext: ".orf"}, true},
		{".rw2", fakeURI{name: "a.rw2", ext: ".rw2"}, true},
		{".raf", fakeURI{name: "a.raf", ext: ".raf"}, true},
		{".pef", fakeURI{name: "a.pef", ext: ".pef"}, true},
		{".srw", fakeURI{name: "a.srw", ext: ".srw"}, true},
		{".raw", fakeURI{name: "a.raw", ext: ".raw"}, true},
		{"dng mime overrides odd extension", fakeURI{name: "a.bin", ext: ".bin", mime: "image/x-adobe-dng"}, true},
		{"canon cr2 mime", fakeURI{name: "a.bin", ext: ".bin", mime: "image/x-canon-cr2"}, true},
		{"unrelated mime still rejected", fakeURI{name: "a.bin", ext: ".bin", mime: "application/octet-stream"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSupportedImage(c.u); got != c.want {
				t.Errorf("IsSupportedImage(%+v) = %v, want %v", c.u, got, c.want)
			}
		})
	}
}

func TestLoadImage_RAWPreview(t *testing.T) {
	path := writeTempFile(t, "photo.cr2", uitest.EncodeRAWPreview(t, uitest.RAWPreview{
		Width: 20, Height: 10, Color: color.RGBA{R: 200, A: 255},
	}))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if !loaded.Preview {
		t.Error("Preview = false, want true for an extracted RAW preview")
	}
	if len(loaded.Frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(loaded.Frames))
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 20 || b.Dy() != 10 {
		t.Errorf("decoded size = %dx%d, want 20x10 (the embedded JPEG)", b.Dx(), b.Dy())
	}
}

func TestLoadImage_PlainTIFFIsNotAPreview(t *testing.T) {
	path := writeTempFile(t, "photo.tiff", encodeTIFF(t, 12, 8, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if loaded.Preview {
		t.Error("Preview = true, want false for a displayable TIFF")
	}
}

func TestLoadImage_JPEGIsNotAPreview(t *testing.T) {
	path := writeTempFile(t, "photo.jpg", encodeJPEG(t, 8, 8, color.White))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if loaded.Preview {
		t.Error("Preview = true, want false for a JPEG")
	}
}

func TestLoadImage_RAWPicksLargestEmbeddedJPEG(t *testing.T) {
	small := encodeJPEG(t, 8, 8, color.White)
	large := encodeJPEG(t, 40, 20, color.RGBA{G: 200, A: 255})
	data := tiffWithTwoJPEGIFDs(t, large, small)
	path := writeTempFile(t, "photo.nef", data)

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 40 || b.Dy() != 20 {
		t.Errorf("decoded size = %dx%d, want 40x20 (the larger IFD's JPEG)", b.Dx(), b.Dy())
	}
}

func TestLoadImage_RAWJPEGInSubIFD(t *testing.T) {
	jpegBytes := encodeJPEG(t, 16, 12, color.White)
	path := writeTempFile(t, "photo.dng", tiffWithJPEGInSubIFD(t, jpegBytes))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 16 || b.Dy() != 12 {
		t.Errorf("decoded size = %dx%d, want 16x12", b.Dx(), b.Dy())
	}
	if !loaded.Preview {
		t.Error("Preview = false, want true")
	}
}

func TestLoadImage_CR3EmbeddedJPEG(t *testing.T) {
	jpegBytes := encodeJPEG(t, 24, 16, color.RGBA{B: 200, A: 255})
	path := writeTempFile(t, "photo.cr3", cr3WithJPEG(jpegBytes))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 24 || b.Dy() != 16 {
		t.Errorf("decoded size = %dx%d, want 24x16", b.Dx(), b.Dy())
	}
	if !loaded.Preview {
		t.Error("Preview = false, want true")
	}
}

func TestLoadImage_RAFEmbeddedJPEG(t *testing.T) {
	jpegBytes := encodeJPEG(t, 18, 12, color.White)
	path := writeTempFile(t, "photo.raf", rafWithJPEG(jpegBytes))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 18 || b.Dy() != 12 {
		t.Errorf("decoded size = %dx%d, want 18x12", b.Dx(), b.Dy())
	}
}

func TestLoadImage_RAWWithoutPreviewFails(t *testing.T) {
	// A TIFF-shaped file with compression 6 but no JPEG bytes anywhere.
	data := tiffJPEGIFDWithoutPayload(t)
	path := writeTempFile(t, "empty.cr2", data)

	if _, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes); err == nil {
		t.Fatal("LoadImage succeeded, want an error when no preview can be extracted")
	}
}

func TestReadAndProbe_RAWUsesPreviewDimensions(t *testing.T) {
	path := writeTempFile(t, "photo.arw", uitest.EncodeRAWPreview(t, uitest.RAWPreview{
		Width: 30, Height: 10, Color: color.White,
	}))

	_, bounds, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("ReadAndProbe: %v", err)
	}
	if bounds.Dx() != 30 || bounds.Dy() != 10 {
		t.Errorf("bounds = %dx%d, want 30x10", bounds.Dx(), bounds.Dy())
	}
}

func TestReadAndProbe_RAWOrientationSwapsBounds(t *testing.T) {
	path := writeTempFile(t, "photo.cr2", uitest.EncodeRAWPreview(t, uitest.RAWPreview{
		Width: 20, Height: 10, Color: color.White, Orientation: 6,
	}))

	_, bounds, err := ReadAndProbe(context.Background(), storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("ReadAndProbe: %v", err)
	}
	if bounds.Dx() != 10 || bounds.Dy() != 20 {
		t.Errorf("bounds = %dx%d, want 10x20 after orientation 6", bounds.Dx(), bounds.Dy())
	}

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 10 || b.Dy() != 20 {
		t.Errorf("decoded size = %dx%d, want 10x20 after orientation 6", b.Dx(), b.Dy())
	}
}

func TestReadMetadata_TIFFContainer(t *testing.T) {
	data := uitest.EncodeRAWPreview(t, uitest.RAWPreview{
		Width: 8, Height: 8, Color: color.White,
		Make: "Canon", Model: "EOS R5", DateTime: "2024:03:15 09:30:00",
	})

	m := ReadMetadata(data)
	if m.Make != "Canon" || m.Model != "EOS R5" {
		t.Errorf("Make/Model = %q/%q, want Canon/EOS R5", m.Make, m.Model)
	}
	if m.DateTaken != "2024-03-15 09:30:00" {
		t.Errorf("DateTaken = %q, want 2024-03-15 09:30:00", m.DateTaken)
	}
}

func TestCaptureDate_RAWTIFF(t *testing.T) {
	path := writeTempFile(t, "photo.nef", uitest.EncodeRAWPreview(t, uitest.RAWPreview{
		Width: 8, Height: 8, Color: color.White, DateTime: "2021:07:04 12:00:00",
	}))

	got, ok := CaptureDate(storage.NewFileURI(path))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got.Year() != 2021 || got.Month() != 7 || got.Day() != 4 {
		t.Errorf("CaptureDate() = %v, want 2021-07-04", got)
	}
}

func TestCanEncode_RAWIsFalse(t *testing.T) {
	for _, ext := range []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".raf"} {
		if CanEncodeExt(ext) {
			t.Errorf("CanEncodeExt(%q) = true, want false (preview-only, no RAW write-back)", ext)
		}
	}
}

func TestLoadImage_RAWStripOffsetsJPEG(t *testing.T) {
	jpegBytes := encodeJPEG(t, 14, 10, color.White)
	path := writeTempFile(t, "photo.arw", tiffWithJPEGStrips(t, jpegBytes))

	loaded, err := LoadImage(storage.NewFileURI(path), DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	b := loaded.Frames[0].Bounds()
	if b.Dx() != 14 || b.Dy() != 10 {
		t.Errorf("decoded size = %dx%d, want 14x10", b.Dx(), b.Dy())
	}
}

// tiffWithTwoJPEGIFDs writes a compression-6 TIFF whose IFD0 holds large
// and IFD1 holds small, so the extractor has to pick by pixel count rather
// than by "first JPEG found".
func tiffWithTwoJPEGIFDs(t *testing.T, ifd0JPEG, ifd1JPEG []byte) []byte {
	t.Helper()

	cfg0, err := jpeg.DecodeConfig(bytes.NewReader(ifd0JPEG))
	if err != nil {
		t.Fatalf("ifd0 jpeg: %v", err)
	}
	cfg1, err := jpeg.DecodeConfig(bytes.NewReader(ifd1JPEG))
	if err != nil {
		t.Fatalf("ifd1 jpeg: %v", err)
	}

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }
	writeJPEGIFD := func(buf *bytes.Buffer, w, h int, jpegOff, jpegLen, next uint32) {
		_, _ = buf.Write(u16(5))
		writeShort := func(tag, v uint16) {
			_, _ = buf.Write(u16(tag))
			_, _ = buf.Write(u16(3))
			_, _ = buf.Write(u32(1))
			mar := make([]byte, 4)
			le.PutUint16(mar, v)
			_, _ = buf.Write(mar)
		}
		writeLong := func(tag uint16, v uint32) {
			_, _ = buf.Write(u16(tag))
			_, _ = buf.Write(u16(4))
			_, _ = buf.Write(u32(1))
			_, _ = buf.Write(u32(v))
		}
		writeShort(0x0100, uint16(w))
		writeShort(0x0101, uint16(h))
		writeShort(0x0103, 6)
		writeLong(0x0201, jpegOff)
		writeLong(0x0202, jpegLen)
		_, _ = buf.Write(u32(next))
	}

	const headerSize = 8
	ifd0Size := 2 + 5*12 + 4
	ifd1Size := ifd0Size
	ifd0Off := uint32(headerSize)
	ifd1Off := ifd0Off + uint32(ifd0Size)
	jpeg0Off := ifd1Off + uint32(ifd1Size)
	jpeg1Off := jpeg0Off + uint32(len(ifd0JPEG))

	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("II")
	_, _ = buf.Write(u16(0x002A))
	_, _ = buf.Write(u32(ifd0Off))
	writeJPEGIFD(buf, cfg0.Width, cfg0.Height, jpeg0Off, uint32(len(ifd0JPEG)), ifd1Off)
	writeJPEGIFD(buf, cfg1.Width, cfg1.Height, jpeg1Off, uint32(len(ifd1JPEG)), 0)
	_, _ = buf.Write(ifd0JPEG)
	_, _ = buf.Write(ifd1JPEG)
	return buf.Bytes()
}

func tiffWithJPEGInSubIFD(t *testing.T, jpegBytes []byte) []byte {
	t.Helper()

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes))
	if err != nil {
		t.Fatalf("jpeg: %v", err)
	}

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }

	const headerSize = 8
	ifd0Size := 2 + 1*12 + 4 // SubIFD pointer only
	subSize := 2 + 5*12 + 4
	ifd0Off := uint32(headerSize)
	subOff := ifd0Off + uint32(ifd0Size)
	jpegOff := subOff + uint32(subSize)

	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("II")
	_, _ = buf.Write(u16(0x002A))
	_, _ = buf.Write(u32(ifd0Off))

	_, _ = buf.Write(u16(1))
	_, _ = buf.Write(u16(0x014A)) // SubIFDs
	_, _ = buf.Write(u16(4))
	_, _ = buf.Write(u32(1))
	_, _ = buf.Write(u32(subOff))
	_, _ = buf.Write(u32(0))

	_, _ = buf.Write(u16(5))
	writeShort := func(tag, v uint16) {
		_, _ = buf.Write(u16(tag))
		_, _ = buf.Write(u16(3))
		_, _ = buf.Write(u32(1))
		mar := make([]byte, 4)
		le.PutUint16(mar, v)
		_, _ = buf.Write(mar)
	}
	writeLong := func(tag uint16, v uint32) {
		_, _ = buf.Write(u16(tag))
		_, _ = buf.Write(u16(4))
		_, _ = buf.Write(u32(1))
		_, _ = buf.Write(u32(v))
	}
	writeShort(0x0100, uint16(cfg.Width))
	writeShort(0x0101, uint16(cfg.Height))
	writeShort(0x0103, 6)
	writeLong(0x0201, jpegOff)
	writeLong(0x0202, uint32(len(jpegBytes)))
	_, _ = buf.Write(u32(0))
	_, _ = buf.Write(jpegBytes)
	return buf.Bytes()
}

func tiffWithJPEGStrips(t *testing.T, jpegBytes []byte) []byte {
	t.Helper()

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes))
	if err != nil {
		t.Fatalf("jpeg: %v", err)
	}

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }

	const headerSize = 8
	ifdSize := 2 + 5*12 + 4
	ifdOff := uint32(headerSize)
	jpegOff := ifdOff + uint32(ifdSize)

	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("II")
	_, _ = buf.Write(u16(0x002A))
	_, _ = buf.Write(u32(ifdOff))

	_, _ = buf.Write(u16(5))
	writeShort := func(tag, v uint16) {
		_, _ = buf.Write(u16(tag))
		_, _ = buf.Write(u16(3))
		_, _ = buf.Write(u32(1))
		mar := make([]byte, 4)
		le.PutUint16(mar, v)
		_, _ = buf.Write(mar)
	}
	writeLong := func(tag uint16, v uint32) {
		_, _ = buf.Write(u16(tag))
		_, _ = buf.Write(u16(4))
		_, _ = buf.Write(u32(1))
		_, _ = buf.Write(u32(v))
	}
	writeShort(0x0100, uint16(cfg.Width))
	writeShort(0x0101, uint16(cfg.Height))
	writeShort(0x0103, 7) // JPEG (new)
	writeLong(0x0111, jpegOff)
	writeLong(0x0117, uint32(len(jpegBytes)))
	_, _ = buf.Write(u32(0))
	_, _ = buf.Write(jpegBytes)
	return buf.Bytes()
}

func tiffJPEGIFDWithoutPayload(t *testing.T) []byte {
	t.Helper()

	le := binary.LittleEndian
	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }

	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("II")
	_, _ = buf.Write(u16(0x002A))
	_, _ = buf.Write(u32(8))
	_, _ = buf.Write(u16(3))
	writeShort := func(tag, v uint16) {
		_, _ = buf.Write(u16(tag))
		_, _ = buf.Write(u16(3))
		_, _ = buf.Write(u32(1))
		mar := make([]byte, 4)
		le.PutUint16(mar, v)
		_, _ = buf.Write(mar)
	}
	writeShort(0x0100, 8)
	writeShort(0x0101, 8)
	writeShort(0x0103, 6)
	_, _ = buf.Write(u32(0))
	return buf.Bytes()
}

func cr3WithJPEG(jpegBytes []byte) []byte {
	var buf bytes.Buffer
	// size includes the 8-byte header; ftyp with major brand crx  and no
	// extra compatible brands is 16 bytes.
	_, _ = buf.Write([]byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'c', 'r', 'x', ' ', 0, 0, 0, 0})
	mdatSize := uint32(8 + len(jpegBytes))
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], mdatSize)
	_, _ = buf.Write(sz[:])
	_, _ = buf.WriteString("mdat")
	_, _ = buf.Write(jpegBytes)
	return buf.Bytes()
}

func rafWithJPEG(jpegBytes []byte) []byte {
	head := make([]byte, 256)
	copy(head, []byte("FUJIFILMCCD-RAW "))
	return append(head, jpegBytes...)
}
