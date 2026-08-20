package favthumbs

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/uitest"
)

// TestMain registers the fyne test app so storage.NewFileURI's "file"
// scheme is resolvable. Sync is the first thing in this package that reads
// a source's bytes rather than just stat-ing it, and imaging.LoadThumbnail
// goes through storage.Reader to do so; without a repository registered
// every decode here fails with "no repository registered for scheme
// 'file'".
func TestMain(m *testing.M) {
	test.NewApp()
	os.Exit(m.Run())
}

// testSink records what Sync hands it. Sync calls Store from several
// workers at once, so every field is guarded - an unguarded map here would
// be a real data race, not a theoretical one, and -race would say so.
type testSink struct {
	mu     sync.Mutex
	cached map[string]image.Image
	stored map[string]image.Image
	calls  int
}

func newTestSink() *testSink {
	return &testSink{
		cached: make(map[string]image.Image),
		stored: make(map[string]image.Image),
	}
}

func (s *testSink) Cached(src fyne.URI) (image.Image, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	img, ok := s.cached[src.String()]
	return img, ok
}

func (s *testSink) Store(src fyne.URI, thumb image.Image) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stored[src.String()] = thumb
	s.calls++
}

// storedFor returns the thumbnail Store was given for src, if any.
func (s *testSink) storedFor(src fyne.URI) (image.Image, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	img, ok := s.stored[src.String()]
	return img, ok
}

// setCached seeds what Cached will report for src, standing in for a
// thumbnail the caller's in-memory cache already holds.
func (s *testSink) setCached(src fyne.URI, thumb image.Image) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cached[src.String()] = thumb
}

// storeCalls is the number of Store invocations, not the number of
// distinct files stored: repeated work on one file shows up here and
// nowhere else, since the stored map collapses it.
func (s *testSink) storeCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls
}

// previewPath returns where a preview for src is expected on disk. ext is
// explicit because the caller knows which encoding its fixture produces:
// the solid-color JPEG sources here always thumbnail to an opaque image.
func previewPath(t *testing.T, favDir string, src fyne.URI, ext string) string {
	t.Helper()

	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%v) reported false", src)
	}
	return filepath.Join(Dir(favDir), name+ext)
}

func TestSyncWritesPreviewForEveryFile(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	files := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255}),
		uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255}),
		uitest.TempJPEGURI(t, "c.jpg", 20, 20, color.RGBA{B: 200, A: 255}),
	}

	if err := Sync(context.Background(), favDir, files, newTestSink()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, f := range files {
		path := previewPath(t, favDir, f, ".jpg")
		if !fileExists(path) {
			t.Errorf("no preview at %q for %v", path, f)
		}
	}
}

func TestSyncStoresEveryFileInSink(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	files := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255}),
		uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255}),
	}
	sink := newTestSink()

	if err := Sync(context.Background(), favDir, files, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, f := range files {
		if _, ok := sink.storedFor(f); !ok {
			t.Errorf("Store was never called for %v", f)
		}
	}
}

// jpegTolerance is how far a per-channel sample may drift and still count
// as the same color. A preview makes two lossy round trips (the source JPEG
// and the stored preview), so exact equality is not on offer - but the
// tests that use this distinguish red from blue, which no amount of JPEG
// drift can blur together.
const jpegTolerance = 24

// assertNearColor fails unless img's center pixel is within jpegTolerance
// of want on every channel. The center is sampled rather than a corner
// because scaling filters can pull edge pixels toward whatever lies just
// outside the image.
func assertNearColor(t *testing.T, img image.Image, want color.RGBA, what string) {
	t.Helper()

	if img == nil {
		t.Fatalf("%s: nil image", what)
	}
	b := img.Bounds()
	got := sampleRGBA(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2))
	if absDiff(got.R, want.R) > jpegTolerance ||
		absDiff(got.G, want.G) > jpegTolerance ||
		absDiff(got.B, want.B) > jpegTolerance {
		t.Errorf("%s: center pixel %v, want within %d of %v", what, got, jpegTolerance, want)
	}
}

// TestSyncReusesPreviewFromDiskInsteadOfDecoding proves the disk hit is a
// real short circuit rather than a redundant lookup in front of a decode
// that happens anyway: the stored preview is red while the source file is
// blue, so the color of what reaches Store says which of the two Sync
// actually read.
func TestSyncReusesPreviewFromDiskInsteadOfDecoding(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	blue := color.RGBA{B: 255, A: 255}
	red := color.RGBA{R: 255, A: 255}
	src := uitest.TempJPEGURI(t, "a.jpg", 40, 30, blue)

	if err := Write(favDir, src, newOpaqueThumb(8, 8, red)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	sink := newTestSink()
	if err := Sync(context.Background(), favDir, []fyne.URI{src}, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, ok := sink.storedFor(src)
	if !ok {
		t.Fatal("Store was never called")
	}
	assertNearColor(t, got, red, "stored thumbnail")
}

// TestSyncCachedHitSkipsReadAndDecode covers the cheapest of the three
// paths. The caller already holds this thumbnail, so re-offering it through
// Store would be pure noise and decoding the source would be pure waste -
// but disk still has to end up with a copy, which is the one thing this
// path is for. Green in memory against a blue source makes it visible
// which image landed there.
func TestSyncCachedHitSkipsReadAndDecode(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	blue := color.RGBA{B: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	src := uitest.TempJPEGURI(t, "a.jpg", 40, 30, blue)

	sink := newTestSink()
	sink.setCached(src, newOpaqueThumb(8, 8, green))

	if err := Sync(context.Background(), favDir, []fyne.URI{src}, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if _, ok := sink.storedFor(src); ok {
		t.Error("Store was called for a file the sink already had cached")
	}

	stored, ok := Read(favDir, src)
	if !ok {
		t.Fatal("no preview on disk for a cached file")
	}
	assertNearColor(t, stored, green, "preview on disk")
}

// TestSyncCachedHitDoesNotRewriteExistingPreview is the second half of the
// cached path: when disk is already current there is nothing to do at all,
// and re-encoding a JPEG per file on every open of an unchanged favorite
// would be the most expensive way possible to produce no change. The
// preview's mod time is backdated first so an unwanted rewrite shows up as
// a jump to now rather than needing a sleep to become visible.
func TestSyncCachedHitDoesNotRewriteExistingPreview(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	green := color.RGBA{G: 255, A: 255}
	src := uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{B: 255, A: 255})

	thumb := newOpaqueThumb(8, 8, green)
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := previewPath(t, favDir, src, ".jpg")
	backdated := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	sink := newTestSink()
	sink.setCached(src, thumb)

	if err := Sync(context.Background(), favDir, []fyne.URI{src}, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("preview mod time moved from %v to %v, so it was rewritten",
			before.ModTime(), after.ModTime())
	}
}

// TestSyncSweepsStalePreview closes the loop on the pass: filling in what
// is missing only keeps a favorite's directory correct if the previews for
// files that left the list go away too, otherwise every edit of a favorite
// grows it permanently.
func TestSyncSweepsStalePreview(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	kept := uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255})
	dropped := uitest.TempJPEGURI(t, "b.jpg", 40, 30, color.RGBA{G: 200, A: 255})

	if err := Write(favDir, dropped, newOpaqueThumb(8, 8, color.RGBA{G: 255, A: 255})); err != nil {
		t.Fatalf("Write: %v", err)
	}
	stale := previewPath(t, favDir, dropped, ".jpg")

	if err := Sync(context.Background(), favDir, []fyne.URI{kept}, newTestSink()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if fileExists(stale) {
		t.Errorf("preview %q for a file no longer in the list survived the sweep", stale)
	}
	if path := previewPath(t, favDir, kept, ".jpg"); !fileExists(path) {
		t.Errorf("the sweep took the preview for a listed file at %q", path)
	}
}

// TestSyncCancelledContextReturnsErrorAndDoesNotSweep guards the more
// dangerous half of cancellation. Returning early is merely polite; not
// sweeping is correctness. A pass that was cut short never established
// which previews are garbage, so pruning on the strength of it would
// delete perfectly live previews and force a full re-decode of the
// favorite on its next open.
func TestSyncCancelledContextReturnsErrorAndDoesNotSweep(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	listed := uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255})
	dropped := uitest.TempJPEGURI(t, "b.jpg", 40, 30, color.RGBA{G: 200, A: 255})

	if err := Write(favDir, dropped, newOpaqueThumb(8, 8, color.RGBA{G: 255, A: 255})); err != nil {
		t.Fatalf("Write: %v", err)
	}
	stale := previewPath(t, favDir, dropped, ".jpg")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Sync(ctx, favDir, []fyne.URI{listed}, newTestSink()); err == nil {
		t.Error("Sync(cancelled ctx) = nil, want an error")
	}

	if !fileExists(stale) {
		t.Errorf("preview %q was swept by a cancelled pass", stale)
	}
}

// TestSyncBadFileDoesNotStopPeers keeps one broken file from costing the
// user every other preview in the favorite. A truncated download or a
// wrong-extension file is ordinary enough that aborting the pass on it
// would leave most favorites permanently half-cached.
func TestSyncBadFileDoesNotStopPeers(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	bad := storage.NewFileURI(uitest.WriteTempFile(t, "broken.jpg", []byte("not an image")))
	good := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255}),
		uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255}),
	}
	files := append([]fyne.URI{bad}, good...)

	sink := newTestSink()
	if err := Sync(context.Background(), favDir, files, sink); err == nil {
		t.Error("Sync = nil, want the undecodable file's error")
	}

	for _, f := range good {
		if path := previewPath(t, favDir, f, ".jpg"); !fileExists(path) {
			t.Errorf("no preview at %q - the broken file took its peers down with it", path)
		}
		if _, ok := sink.storedFor(f); !ok {
			t.Errorf("Store was never called for %v", f)
		}
	}
}

// countFiles returns how many regular files sit directly in dir.
func countFiles(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}

	n := 0
	for _, e := range entries {
		if e.Type().IsRegular() {
			n++
		}
	}
	return n
}

// TestSyncTwiceLeavesNoDuplicates pins down that a second pass is a no-op
// on disk. Sync runs on every favorite open, so anything it leaves behind
// per run - a second copy, a stray temp file - accumulates for as long as
// the favorite is used.
func TestSyncTwiceLeavesNoDuplicates(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	files := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255}),
		uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255}),
	}

	if err := Sync(context.Background(), favDir, files, newTestSink()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	first := countFiles(t, Dir(favDir))
	if first != len(files) {
		t.Fatalf("first pass left %d previews, want %d", first, len(files))
	}

	if err := Sync(context.Background(), favDir, files, newTestSink()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if second := countFiles(t, Dir(favDir)); second != first {
		t.Errorf("second pass left %d previews, want the same %d", second, first)
	}
}

// TestSyncNilSinkStillWritesPreviews covers the caller that wants previews
// on disk but has no in-memory cache to warm - a favorite saved while its
// files are not the ones on screen. Generating previews must not depend on
// there being somewhere to hand them afterwards.
func TestSyncNilSinkStillWritesPreviews(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	files := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255}),
		uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255}),
	}

	if err := Sync(context.Background(), favDir, files, nil); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, f := range files {
		if path := previewPath(t, favDir, f, ".jpg"); !fileExists(path) {
			t.Errorf("no preview at %q", path)
		}
	}
}

// TestSyncDeduplicatesRepeatedFiles guards against the app's merge mode,
// which loads one path at two indices whenever the same file arrives from
// two dropped folders. Left as-is, that path would be decoded twice and,
// worse, written twice concurrently to a single destination - so the pass
// deduplicates before it starts. Run under -race, where the double write
// would show up as more than a wasted decode.
func TestSyncDeduplicatesRepeatedFiles(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	dup := uitest.TempJPEGURI(t, "a.jpg", 40, 30, color.RGBA{R: 200, A: 255})
	other := uitest.TempJPEGURI(t, "b.jpg", 30, 40, color.RGBA{G: 200, A: 255})
	files := []fyne.URI{dup, other, dup}

	sink := newTestSink()
	if err := Sync(context.Background(), favDir, files, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := countFiles(t, Dir(favDir)); got != 2 {
		t.Errorf("thumbs dir holds %d previews, want 2", got)
	}
	if got := sink.storeCalls(); got != 2 {
		t.Errorf("Store was called %d times, want 2 - the repeated file was worked twice", got)
	}
}
