package grid

import (
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/imagedrop/internal/imaging"
	"github.com/frathe/imagedrop/internal/uitest"
)

func TestMain(m *testing.M) {
	// The overlay is built from real widgets, so these need an app for the
	// theme and driver. Each test still gets its own window (see
	// newOverview) - New takes one to maximize on open - but the fyne test
	// driver's windows are not driver.NativeWindow, so winpos.Maximize
	// degrades to a no-op there, the same as every other winpos call this
	// app makes under test.
	test.NewApp()
	os.Exit(m.Run())
}

// fakeHost stands in for the viewer. It records the display actions the
// grid asks for, so a selection or a close can be observed without a real
// window behind it.
type fakeHost struct {
	files []fyne.URI
	index int
	gen   uint64

	shown     []int
	repaints  int
	unfocused int
}

func (f *fakeHost) FileCount() int        { return len(f.files) }
func (f *fakeHost) FileAt(i int) fyne.URI { return f.files[i] }
func (f *fakeHost) CurrentIndex() int     { return f.index }
func (f *fakeHost) Generation() uint64    { return f.gen }
func (f *fakeHost) ShowImage(i int)       { f.shown = append(f.shown, i) }
func (f *fakeHost) ForceRepaint()         { f.repaints++ }
func (f *fakeHost) Unfocus()              { f.unfocused++ }

// hostWith returns a host holding n small real JPEGs - real files because
// the decode path under test actually reads them.
func hostWith(t *testing.T, names ...string) *fakeHost {
	t.Helper()

	uris := make([]fyne.URI, 0, len(names))
	for _, name := range names {
		uris = append(uris, uitest.TempJPEGURI(t, name, 8, 8, color.White))
	}

	return &fakeHost{files: uris}
}

// newOverview builds an Overview over host and a real (test-driver)
// window, closing the window when the test ends - the fixture behind every
// New call in this file, now that New needs a window to maximize on open.
func newOverview(t *testing.T, host Host) *Overview {
	t.Helper()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return New(host, win)
}

// --- open / close ----------------------------------------------------------

func TestToggle_NoFilesIsNoop(t *testing.T) {
	g := newOverview(t, &fakeHost{})

	g.Toggle()

	if g.Visible() || g.Overlay().Visible() {
		t.Error("the grid should not open with nothing loaded")
	}
}

func TestToggle_OpensAndCloses(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	if !g.Visible() || !g.Overlay().Visible() {
		t.Fatal("the grid should be open after the first toggle")
	}

	g.Toggle()
	if g.Visible() || g.Overlay().Visible() {
		t.Error("the grid should be closed after the second toggle")
	}
}

func TestToggle_StartsHighlightOnCurrentImage(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	host.index = 2
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()

	if g.Highlight() != 2 {
		t.Errorf("Highlight() = %d, want 2 (the image on screen when the grid opened)", g.Highlight())
	}
}

func TestClose_NoopWhenAlreadyClosed(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	g.Close()

	if g.Visible() || g.Overlay().Visible() {
		t.Error("Close should be a no-op when the grid isn't showing")
	}
	if host.unfocused != 0 {
		t.Error("Close on an already-closed grid should not touch focus")
	}
}

// TestClose_UnfocusesCanvas guards the bug where, after clicking a
// thumbnail, arrow-key navigation stopped working until the user clicked
// the image: Fyne's GridWrap grabs canvas focus on a real tap, and this app
// dispatches every key manually from the canvas's *unfocused* handler, so a
// focused GridWrap left behind swallows everything afterwards.
func TestClose_UnfocusesCanvas(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	g.Close()

	if host.unfocused != 1 {
		t.Errorf("Unfocus calls = %d, want 1 - closing must hand the keyboard back", host.unfocused)
	}
}

// --- key handling ----------------------------------------------------------

func TestHandleKey_EscapeAndGClose(t *testing.T) {
	for _, key := range []fyne.KeyName{fyne.KeyEscape, fyne.KeyG} {
		t.Run(string(key), func(t *testing.T) {
			g := newOverview(t, hostWith(t, "a.jpg"))
			if err := g.Warm(); err != nil {
				t.Fatalf("Warm: %v", err)
			}
			g.Toggle()

			g.HandleKey(&fyne.KeyEvent{Name: key})

			if g.Visible() {
				t.Errorf("%s should close the grid", key)
			}
		})
	}
}

func TestHandleKey_ArrowMovesHighlight(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg", "c.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	if g.Highlight() != 0 {
		t.Fatalf("Highlight() = %d, want 0 at the start", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if g.Highlight() != 1 {
		t.Errorf("Highlight() = %d, want 1 after Right", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 after Left", g.Highlight())
	}
}

func TestHandleKey_LeftAtStartIsNoop(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})

	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want it to stay at 0 - there's nothing before the first cell", g.Highlight())
	}
}

func TestHandleKey_ReturnOpensHighlightedAndCloses(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if len(host.shown) != 1 || host.shown[0] != 1 {
		t.Errorf("ShowImage calls = %v, want just the highlighted cell (1)", host.shown)
	}
	if g.Visible() {
		t.Error("committing a cell should close the grid")
	}
}

// --- thumbnails ------------------------------------------------------------

// newCell returns a cell of the shape the grid's own CreateItem builds -
// the image plus its highlight ring - to hand to requestThumbnail
// directly.
func newCell() (*fyne.Container, *canvas.Image) {
	img := canvas.NewImageFromImage(nil)
	ring := canvas.NewRectangle(color.Transparent)

	return container.NewStack(img, ring), img
}

func TestRequestThumbnail_CacheHitAppliesSynchronously(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	g.requestThumbnail(cell, img, 0, host.gen)

	if img.Image == nil {
		t.Error("a cache hit should paint the cell synchronously, without waiting for a goroutine")
	}
}

func TestRequestThumbnail_DecodesInBackgroundAndCaches(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	g.requestThumbnail(cell, img, 0, host.gen)

	// Settle, not a poll of the cache: the cache write and the paint
	// happen in the same completion callback, so waiting on the decode
	// itself is what gives this goroutine a happens-before edge on both.
	g.Settle()

	if !g.Cached(host.files[0]) {
		t.Error("the decoded thumbnail should have been cached")
	}
	if img.Image == nil {
		t.Error("img.Image should be set once the background decode finishes")
	}
}

// TestSetCacheBytes_RetunesTheThumbnailBudget covers the one setter this
// package exposes - the settings window's route to the thumbnail cache, via
// internal/ui's SetMaxThumbCacheMB. An 8x8 JPEG stays 8x8 through scaleToFit
// (already inside ThumbnailSize) and decodes to a 4:2:0 *image.YCbCr, so each
// one weighs well under 200 bytes; a 100-byte budget therefore fits exactly
// one of them and Warm's second file has to evict its first.
func TestSetCacheBytes_RetunesTheThumbnailBudget(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	g.SetCacheBytes(100)

	if err := g.Warm(); err != nil {
		t.Fatalf("Warm() returned error: %v", err)
	}

	if g.thumbs.Len() != 1 {
		t.Errorf("cached thumbnails = %d, want 1 under a 100-byte budget", g.thumbs.Len())
	}
	if g.Cached(host.files[0]) {
		t.Error("the first thumbnail should have been evicted by the second")
	}
	if !g.Cached(host.files[1]) {
		t.Error("the most recently warmed thumbnail should still be cached")
	}

	// Raising the budget doesn't resurrect anything, but it does stop the
	// eviction: warming again now holds both.
	g.SetCacheBytes(imaging.DefaultThumbCacheBytes)

	if err := g.Warm(); err != nil {
		t.Fatalf("Warm() returned error: %v", err)
	}

	if g.thumbs.Len() != 2 {
		t.Errorf("cached thumbnails = %d after raising the budget, want 2", g.thumbs.Len())
	}
}

func TestRequestThumbnail_OutOfRangeIDIsNoop(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	cell, img := newCell()

	g.requestThumbnail(cell, img, 5, host.gen) // only index 0 exists
	g.Settle()

	if img.Image != nil {
		t.Error("an out-of-range id should paint nothing")
	}
}

// TestClaimRelease drives the in-flight bookkeeping directly, the same way
// TestStillWanted drives the staleness predicate: these decisions guard
// against duplicate decode goroutines, which no amount of waiting on real
// ones could assert on.
func TestClaimRelease(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg"))
	cell, _ := newCell()

	if !g.claim(cell, 0) {
		t.Fatal("the first claim for a cell should allow a spawn")
	}
	if g.claim(cell, 0) {
		t.Error("an identical claim while one is in flight must not spawn a second decode")
	}
	if !g.claim(cell, 1) {
		t.Error("a claim for a different id should supersede the old one - the cell scrolled on")
	}

	g.release(cell, 0) // the superseded decode finishing late
	if g.claim(cell, 1) {
		t.Error("a stale release must not drop the newer claim")
	}

	g.release(cell, 1)
	if !g.claim(cell, 1) {
		t.Error("after its own release, a cell should be claimable again")
	}
}

// TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases pins the
// worker's pre-decode bail: a request whose cell is recycled while the
// request waits behind sem must neither paint the cell nor keep its claim.
// The workers are parked by filling sem from the test, so the recycle
// deterministically wins the race against the decode.
func TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	for range thumbConcurrency {
		g.sem <- struct{}{}
	}

	g.requestThumbnail(cell, img, 0, host.gen)
	g.cellIDs.Store(cell, 1) // the cell scrolls on before a worker picks this up

	for range thumbConcurrency {
		<-g.sem
	}
	g.Settle()

	if img.Image != nil {
		t.Error("a decode whose cell scrolled away must not paint it")
	}
	if !g.claim(cell, 0) {
		t.Error("the bailed decode should have released its claim")
	}
}

func TestStillWanted(t *testing.T) {
	host := hostWith(t, "a.jpg")
	host.gen = 7
	g := newOverview(t, host)

	cell, _ := newCell()
	g.cellIDs.Store(cell, 3)

	if !g.stillWanted(cell, 3, 7) {
		t.Error("a decode for the cell's current id at the current generation is still wanted")
	}
	if g.stillWanted(cell, 4, 7) {
		t.Error("a decode for an id this cell has since been recycled away from is stale")
	}
	if g.stillWanted(cell, 3, 6) {
		t.Error("a decode from a superseded generation is stale")
	}

	other, _ := newCell()
	if g.stillWanted(other, 3, 7) {
		t.Error("a cell the grid has never tracked is stale")
	}
}

func TestSetCellHighlighted(t *testing.T) {
	ring := canvas.NewRectangle(color.Transparent)

	setCellHighlighted(ring, true)
	if !ring.Visible() {
		t.Error("highlighting should show the ring")
	}

	setCellHighlighted(ring, false)
	if ring.Visible() {
		t.Error("un-highlighting should hide the ring")
	}
}
