package grid

import (
	"context"
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/frathe/picfetch/internal/imaging"
)

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

// --- CachedThumb / StoreThumb / ThumbCacheFull -----------------------------

func TestCachedThumb_MissesForUnstoredURI(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	if _, ok := g.CachedThumb(host.files[0]); ok {
		t.Error("CachedThumb should miss for a URI that was never stored")
	}
}

func TestStoreThumb_ThenCachedThumb_ReturnsWhatWasStored(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	thumb := image.NewRGBA(image.Rect(0, 0, 4, 4))

	if ok := g.StoreThumb(host.files[0], thumb); !ok {
		t.Fatal("StoreThumb should report true for a thumbnail well within budget")
	}

	got, ok := g.CachedThumb(host.files[0])
	if !ok {
		t.Fatal("CachedThumb should hit after StoreThumb")
	}
	if got != image.Image(thumb) {
		t.Error("CachedThumb should return the same image that was stored")
	}
}

// TestStoreThumb_TooBigForBudgetIsRefused covers AddIfFits's
// never-evict-for-a-speculative-write rule (see its own comment in
// internal/imaging/bytecache.go): an entry that alone outweighs the whole
// budget is refused outright rather than stored and left to evict
// everything else in the cache.
func TestStoreThumb_TooBigForBudgetIsRefused(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	g.SetCacheBytes(100)

	// A 100x100 RGBA thumbnail weighs 100*100*4 = 40000 bytes, comfortably
	// over the 100-byte budget.
	big := image.NewRGBA(image.Rect(0, 0, 100, 100))

	if ok := g.StoreThumb(host.files[0], big); ok {
		t.Error("StoreThumb should refuse a thumbnail that alone exceeds the whole budget")
	}
	if _, ok := g.CachedThumb(host.files[0]); ok {
		t.Error("CachedThumb should still miss after a refused StoreThumb")
	}
}

func TestThumbCacheFull_FalseUntilBudgetReached(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	g.SetCacheBytes(100)

	if g.ThumbCacheFull() {
		t.Error("a fresh overview's thumbnail cache should not report full")
	}

	// A 5x5 RGBA thumbnail weighs 5*5*4 = 100 bytes: exactly the budget.
	full := image.NewRGBA(image.Rect(0, 0, 5, 5))
	if ok := g.StoreThumb(host.files[0], full); !ok {
		t.Fatal("StoreThumb should accept a thumbnail exactly at budget")
	}

	if !g.ThumbCacheFull() {
		t.Error("the cache should report full once stored bytes reach the budget")
	}
}

// TestStoreThumb_AloneDoesNotProtectTheHeadOfTheList is the eviction-churn
// behavior ThumbCacheFull's doc comment warns about: a pre-warm pass that
// only ever calls StoreThumb, with no ThumbCacheFull check between offers,
// will happily evict the entries it stored first to make room for the
// ones it stores last. A caller pre-warming a favorite's disk previews in
// file-list order needs the *first* files warm when the grid opens at
// index 0, not the last ones - which is exactly what this test shows
// StoreThumb alone does not guarantee.
func TestStoreThumb_AloneDoesNotProtectTheHeadOfTheList(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	// A 5x5 RGBA thumbnail weighs 5*5*4 = 100 bytes, so a 100-byte budget
	// fits exactly one.
	g.SetCacheBytes(100)

	first := image.NewRGBA(image.Rect(0, 0, 5, 5))
	second := image.NewRGBA(image.Rect(0, 0, 5, 5))

	if ok := g.StoreThumb(host.files[0], first); !ok {
		t.Fatal("StoreThumb should accept the first thumbnail, which alone fits the budget")
	}
	if ok := g.StoreThumb(host.files[1], second); !ok {
		t.Fatal("StoreThumb should accept the second thumbnail, which alone fits the budget")
	}

	if _, ok := g.CachedThumb(host.files[0]); ok {
		t.Error("the first thumbnail should have been evicted by the second - StoreThumb alone does not protect the head of the list")
	}
	if _, ok := g.CachedThumb(host.files[1]); !ok {
		t.Error("the second (most recently stored) thumbnail should still be cached")
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

	if !g.decodes.Claim(cell, 0) {
		t.Fatal("the first claim for a cell should allow a spawn")
	}
	if g.decodes.Claim(cell, 0) {
		t.Error("an identical claim while one is in flight must not spawn a second decode")
	}
	if !g.decodes.Claim(cell, 1) {
		t.Error("a claim for a different id should supersede the old one - the cell scrolled on")
	}

	g.decodes.Release(cell, 0) // the superseded decode finishing late
	if g.decodes.Claim(cell, 1) {
		t.Error("a stale release must not drop the newer claim")
	}

	g.decodes.Release(cell, 1)
	if !g.decodes.Claim(cell, 1) {
		t.Error("after its own release, a cell should be claimable again")
	}
}

// TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases pins the
// worker's pre-decode bail: a request whose cell is recycled while the
// request waits for a slot must neither paint the cell nor keep its claim.
// The workers are parked by filling the pool from the test, so the recycle
// deterministically wins the race against the decode.
func TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	// The pool waits for its slot on the spawned goroutine, so each parker
	// has to report that it really holds one before the request below can
	// be sure of queueing behind them.
	holding := make(chan struct{}, thumbConcurrency)
	parked := make(chan struct{})
	for range thumbConcurrency {
		g.decodes.Go(context.Background(), func(bool) {
			holding <- struct{}{}
			<-parked
		})
	}
	for range thumbConcurrency {
		<-holding
	}

	g.requestThumbnail(cell, img, 0, host.gen)
	g.cellIDs.Store(cell, 1) // the cell scrolls on before a worker picks this up

	close(parked)
	g.Settle()

	if img.Image != nil {
		t.Error("a decode whose cell scrolled away must not paint it")
	}
	if !g.decodes.Claim(cell, 0) {
		t.Error("the bailed decode should have released its claim")
	}
}

// TestRequestThumbnail_QueryChangeDiscardsInFlightDecode covers the
// staleness filtering adds on top of the two guards already here: the file
// set and the cell's own id can both still be current while the query
// underneath has renumbered the cells, so display cell 0 means a different
// file than the one this decode was started for. Same parking technique as
// the recycling test above - fill the pool so the change deterministically
// beats the decode.
func TestRequestThumbnail_QueryChangeDiscardsInFlightDecode(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	holding := make(chan struct{}, thumbConcurrency)
	parked := make(chan struct{})
	for range thumbConcurrency {
		g.decodes.Go(context.Background(), func(bool) {
			holding <- struct{}{}
			<-parked
		})
	}
	for range thumbConcurrency {
		<-holding
	}

	g.requestThumbnail(cell, img, 0, host.gen)

	// Display cell 0 now means b.jpg; the decode in flight is for a.jpg.
	typeQuery(g, "b")

	close(parked)
	g.Settle()

	if img.Image != nil {
		t.Error("a decode started under a different query must not paint a.jpg into a cell now showing b.jpg")
	}
}

func TestStillWanted(t *testing.T) {
	host := hostWith(t, "a.jpg")
	host.gen = 7
	g := newOverview(t, host)

	cell, _ := newCell()
	g.cellIDs.Store(cell, 3)

	fgen := g.filterGen.Load()

	if !g.stillWanted(cell, 3, 7, fgen) {
		t.Error("a decode for the cell's current id at the current generation is still wanted")
	}
	if g.stillWanted(cell, 4, 7, fgen) {
		t.Error("a decode for an id this cell has since been recycled away from is stale")
	}
	if g.stillWanted(cell, 3, 6, fgen) {
		t.Error("a decode from a superseded generation is stale")
	}
	if g.stillWanted(cell, 3, 7, fgen+1) {
		t.Error("a decode resolved under a superseded query is stale")
	}

	other, _ := newCell()
	if g.stillWanted(other, 3, 7, fgen) {
		t.Error("a cell the grid has never tracked is stale")
	}
}
