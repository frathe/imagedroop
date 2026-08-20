package favthumbs

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

// newOpaqueThumb returns a small, fully opaque, solid-color image.Image
// fixture suitable for exercising the JPEG path.
func newOpaqueThumb(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

// newSourceFile writes an arbitrary, non-empty file at dir/name and returns
// its URI. Store.go's Read/Write only ever os.Stat the source (via
// EntryName), so the file's contents are never inspected - only that it
// exists.
func newSourceFile(t *testing.T, dir, name string) fyne.URI {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return storage.NewFileURI(path)
}

// sampleRGBA converts c to 8-bit-per-channel RGBA for tolerance comparisons
// against JPEG's lossy round trip.
func sampleRGBA(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestWriteOpaqueUsesJPEGExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}

	wantPath := filepath.Join(Dir(favDir), name+".jpg")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected preview at %q, stat error: %v", wantPath, err)
	}
}

// newTransparentThumb returns a small image.NRGBA fixture with a real
// alpha hole in it: one pixel has alpha < 255, the rest are opaque. That
// mix - not full transparency - is what Write's extension choice actually
// has to detect.
func newTransparentThumb(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 128})
	return img
}

func TestWriteTransparentUsesPNGExtensionAndRoundTripsExactly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newTransparentThumb(3, 2)
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}

	wantPath := filepath.Join(Dir(favDir), name+".png")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected preview at %q, stat error: %v", wantPath, err)
	}
	if unwantedPath := filepath.Join(Dir(favDir), name+".jpg"); fileExists(unwantedPath) {
		t.Errorf("did not expect a .jpg preview at %q", unwantedPath)
	}

	got, ok := Read(favDir, src)
	if !ok {
		t.Fatalf("Read reported false after Write")
	}
	if got.Bounds() != thumb.Bounds() {
		t.Fatalf("Read bounds = %v, want %v", got.Bounds(), thumb.Bounds())
	}
	bounds := thumb.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			wantR, wantG, wantB, wantA := thumb.At(x, y).RGBA()
			gotR, gotG, gotB, gotA := got.At(x, y).RGBA()
			if wantR != gotR || wantG != gotG || wantB != gotB || wantA != gotA {
				t.Errorf("pixel (%d,%d) = %d,%d,%d,%d want %d,%d,%d,%d",
					x, y, gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
			}
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestWriteCreatesThumbsDirWhenMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	if _, err := os.Stat(Dir(favDir)); !os.IsNotExist(err) {
		t.Fatalf("thumbs dir already exists before Write: %v", err)
	}

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(Dir(favDir))
	if err != nil {
		t.Fatalf("thumbs dir missing after Write: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Dir(favDir) is not a directory")
	}
}

func TestReadMissesWhenNothingWritten(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	img, ok := Read(favDir, src)
	if ok {
		t.Fatalf("Read reported true with nothing written, image = %v", img)
	}
	if img != nil {
		t.Errorf("Read image = %v, want nil", img)
	}
}

func TestReadMissesWhenSourceModTimeChangesButOldFileSurvives(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 5, G: 6, B: 7, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	oldName, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	oldPath := filepath.Join(Dir(favDir), oldName+".jpg")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("preview missing right after Write: %v", err)
	}

	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(src.Path(), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	img, ok := Read(favDir, src)
	if ok {
		t.Fatalf("Read reported true after mtime changed, image = %v", img)
	}

	// Stale, not gone: deleting superseded previews is Stage 3's Sweep, not
	// this stage's Read.
	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("expected stale preview to remain on disk at %q: %v", oldPath, err)
	}
}

func TestReadMissesWhenStoredFileIsGarbage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	if err := os.MkdirAll(Dir(favDir), 0o755); err != nil {
		t.Fatal(err)
	}
	garbagePath := filepath.Join(Dir(favDir), name+".jpg")
	if err := os.WriteFile(garbagePath, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	img, ok := Read(favDir, src)
	if ok {
		t.Fatalf("Read reported true for garbage file, image = %v", img)
	}
}

func TestWriteErrorsWhenSourceCannotBeStated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := storage.NewFileURI(filepath.Join(dir, "does-not-exist.jpg"))
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 1, G: 1, B: 1, A: 255})
	if err := Write(favDir, missing, thumb); err == nil {
		t.Fatalf("Write with unstat-able source returned nil error")
	}
}

func TestWriteLeavesNoTempFilesBehind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(Dir(favDir))
	if err != nil {
		t.Fatalf("ReadDir(thumbs): %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("thumbs dir has %d entries after one Write, want 1: %v", len(entries), names)
	}
}

func TestWriteReadRoundTripOpaque(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(4, 3, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, ok := Read(favDir, src)
	if !ok {
		t.Fatalf("Read reported false after Write")
	}
	if got.Bounds() != thumb.Bounds() {
		t.Errorf("Read bounds = %v, want %v", got.Bounds(), thumb.Bounds())
	}

	want := sampleRGBA(thumb.At(1, 1))
	gotPixel := sampleRGBA(got.At(1, 1))
	const tol = 8
	if absDiff(want.R, gotPixel.R) > tol || absDiff(want.G, gotPixel.G) > tol || absDiff(want.B, gotPixel.B) > tol {
		t.Errorf("Read pixel = %+v, want within ±%d of %+v", gotPixel, tol, want)
	}
}

// TestWriteReplacesSiblingExtension pins down the one way the two-extension
// scheme could serve a stale preview forever. Both names derive from the
// same base, so a thumbnail that gains or loses transparency for an
// otherwise unchanged source writes the other extension and leaves the
// first behind - and since Read probes .jpg first, and Sweep considers the
// base expected either way, the superseded file would win every lookup from
// then on with nothing to ever clean it up.
func TestWriteReplacesSiblingExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	if err := Write(favDir, src, newOpaqueThumb(2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})); err != nil {
		t.Fatalf("Write opaque: %v", err)
	}
	if err := Write(favDir, src, newTransparentThumb(2, 2)); err != nil {
		t.Fatalf("Write transparent: %v", err)
	}

	entries, err := os.ReadDir(Dir(favDir))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("thumbs dir holds %d entries %v, want exactly 1", len(entries), names)
	}

	got, ok := Read(favDir, src)
	if !ok {
		t.Fatal("Read reported false after two writes")
	}
	if _, _, _, a := got.At(0, 0).RGBA(); a == 0xffff {
		t.Error("Read returned a fully opaque pixel, so it served the superseded JPEG")
	}
}

// TestWriteRejectsNilThumbnail guards a crash, not a misfeature: Write runs
// on a background goroutine, and a panic there takes the whole process down
// rather than failing one preview.
func TestWriteRejectsNilThumbnail(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")

	if err := Write(filepath.Join(dir, "Trip"), src, nil); err == nil {
		t.Error("Write(nil thumbnail) = nil, want an error")
	}
}

// TestHasCurrentPreviewTracksSourceVersion covers the cheap existence check
// Sync leans on when the caller already holds a thumbnail in memory: it has
// to answer "is this source's preview already on disk" without paying for a
// decode, and it has to answer it about the *current* version of the source
// rather than any preview that happens to bear its path hash.
func TestHasCurrentPreviewTracksSourceVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	if hasCurrentPreview(favDir, src) {
		t.Error("hasCurrentPreview = true before anything was written")
	}

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 9, G: 8, B: 7, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !hasCurrentPreview(favDir, src) {
		t.Error("hasCurrentPreview = false right after Write")
	}

	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(src.Path(), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if hasCurrentPreview(favDir, src) {
		t.Error("hasCurrentPreview = true for a source whose mod time moved on")
	}
}
