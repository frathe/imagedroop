package imaging

import (
	"image"
	"image/color"
	"sync"
	"testing"
)

// weighInt is the weight function every ByteCache test below uses: the
// value *is* its own weight, so each test's numbers are the byte totals
// under test rather than something derived from an image's dimensions.
// imageBytes gets its own tests further down.
func weighInt(n int64) int64 { return n }

func newTestByteCache(budget int64) *ByteCache[int64] {
	return NewByteCache(budget, weighInt)
}

// --- ByteCache eviction ------------------------------------------------------

// TestByteCache_EvictsByWeightNotCount is the whole point of the type: three
// entries is a trivially small count, but at 40 bytes each they overrun a
// 100-byte budget, so the oldest has to go. A count-bounded cache would
// have kept all three.
func TestByteCache_EvictsByWeightNotCount(t *testing.T) {
	c := newTestByteCache(100)

	c.Add("a", 40)
	c.Add("b", 40)
	c.Add("c", 40)

	if c.Len() != 2 {
		t.Errorf("Len() = %d, want 2 (120 bytes of entries can't fit a 100-byte budget)", c.Len())
	}
	if got := c.Bytes(); got != 80 {
		t.Errorf("Bytes() = %d, want 80", got)
	}
	if c.Contains("a") {
		t.Error("the least recently used entry should have been evicted, but \"a\" is still cached")
	}
	if !c.Contains("b") || !c.Contains("c") {
		t.Error("the two most recently used entries should have survived")
	}
}

// TestByteCache_KeepsTheNewestEntryEvenWhenItAloneExceedsBudget covers the
// exception the viewer depends on: internal/ui's attemptLoad adds the image
// it is about to display, so that entry must survive even when the user has
// set a budget smaller than a single photo. Without this the cache would sit
// permanently empty and every navigation back would re-decode.
func TestByteCache_KeepsTheNewestEntryEvenWhenItAloneExceedsBudget(t *testing.T) {
	c := newTestByteCache(100)

	c.Add("small", 10)
	c.Add("huge", 500)

	if !c.Contains("huge") {
		t.Error("the most recently added entry must never be evicted, even over budget")
	}
	if c.Contains("small") {
		t.Error("an over-budget newcomer should still evict everything older than it")
	}
	if got := c.Bytes(); got != 500 {
		t.Errorf("Bytes() = %d, want 500 (the running total must reflect what's actually held)", got)
	}
}

// TestByteCache_AddIfFitsRefusesAnOverBudgetValue is the other half of that
// rule: preloadOne uses AddIfFits precisely so a speculative neighbor can't
// invoke the never-evict-the-newest exception and push out the image on
// screen.
func TestByteCache_AddIfFitsRefusesAnOverBudgetValue(t *testing.T) {
	c := newTestByteCache(100)

	c.Add("current", 60)

	if c.AddIfFits("huge", 500) {
		t.Error("AddIfFits() = true for a value larger than the whole budget, want false")
	}
	if c.Contains("huge") {
		t.Error("a refused value must not be cached")
	}
	if !c.Contains("current") {
		t.Error("a refused AddIfFits must not evict anything either")
	}

	// Exactly at the budget is a fit, not an overflow.
	if !c.AddIfFits("exact", 100) {
		t.Error("AddIfFits() = false for a value exactly at the budget, want true")
	}
}

func TestByteCache_ReAddingAKeyReplacesItsWeight(t *testing.T) {
	c := newTestByteCache(1000)

	c.Add("k", 10)
	c.Add("k", 30)

	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (re-adding a key replaces it rather than duplicating)", c.Len())
	}
	if got := c.Bytes(); got != 30 {
		t.Errorf("Bytes() = %d, want 30 (the old weight must be subtracted, not added on top)", got)
	}
}

func TestByteCache_SetBudgetEvictsImmediately(t *testing.T) {
	c := newTestByteCache(200)

	c.Add("a", 40)
	c.Add("b", 40)
	c.Add("c", 40)

	if got := c.Bytes(); got != 120 {
		t.Fatalf("Bytes() = %d, want 120 before the budget is lowered", got)
	}

	// Lowering the limit in the settings window has to free the memory now,
	// not at whatever later Add happens to trip the check.
	c.SetBudget(100)

	if got := c.Budget(); got != 100 {
		t.Errorf("Budget() = %d, want 100", got)
	}
	if got := c.Bytes(); got > 100 {
		t.Errorf("Bytes() = %d, want <= 100 immediately after SetBudget", got)
	}
	if c.Contains("a") {
		t.Error("SetBudget should have evicted the least recently used entry")
	}
}

func TestByteCache_RemoveAndPurgeRestoreTheByteTotal(t *testing.T) {
	c := newTestByteCache(1000)

	c.Add("a", 40)
	c.Add("b", 60)

	c.Remove("a")

	if got := c.Bytes(); got != 60 {
		t.Errorf("Bytes() = %d after Remove, want 60", got)
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d after Remove, want 1", c.Len())
	}

	// Removing a key that was never there must not disturb the total.
	c.Remove("nonexistent")
	if got := c.Bytes(); got != 60 {
		t.Errorf("Bytes() = %d after removing an absent key, want 60", got)
	}

	c.Purge()

	if got := c.Bytes(); got != 0 {
		t.Errorf("Bytes() = %d after Purge, want 0", got)
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d after Purge, want 0", c.Len())
	}
}

// --- ByteCache recency ordering ----------------------------------------------

// TestByteCache_ContainsDoesNotPromote pairs with the Get test below: the
// two differ only in which lookup is used, and that difference decides which
// entry survives. It matters because preloadOne and grid.Cached ask "is this
// already here?" about images nobody is looking at.
func TestByteCache_ContainsDoesNotPromote(t *testing.T) {
	c := newTestByteCache(100)

	c.Add("a", 40)
	c.Add("b", 40)

	if !c.Contains("a") {
		t.Fatal("Contains(\"a\") = false, want true before anything is evicted")
	}

	c.Add("c", 40)

	if c.Contains("a") {
		t.Error("Contains should not have promoted \"a\", so it should be the entry evicted")
	}
}

func TestByteCache_GetPromotes(t *testing.T) {
	c := newTestByteCache(100)

	c.Add("a", 40)
	c.Add("b", 40)

	if _, ok := c.Get("a"); !ok {
		t.Fatal("Get(\"a\") = false, want true before anything is evicted")
	}

	c.Add("c", 40)

	if !c.Contains("a") {
		t.Error("Get should have promoted \"a\" past \"b\", leaving \"b\" as the eviction target")
	}
	if c.Contains("b") {
		t.Error("\"b\" should have been evicted as the least recently used entry")
	}
}

func TestByteCache_GetReportsMissesWithoutMutating(t *testing.T) {
	c := newTestByteCache(100)

	v, ok := c.Get("absent")

	if ok {
		t.Error("Get() on an empty cache reported a hit")
	}
	if v != 0 {
		t.Errorf("Get() miss returned %d, want the zero value", v)
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d after a miss, want 0", c.Len())
	}
}

// --- ByteCache construction and concurrency ----------------------------------

func TestNewByteCache_RaisesANonPositiveBudget(t *testing.T) {
	// A zero budget isn't a "no limit" any of this is written to
	// understand, so it's raised rather than accepted - the cache then
	// degenerates to holding exactly its newest entry.
	for _, budget := range []int64{0, -1} {
		c := newTestByteCache(budget)

		if got := c.Budget(); got != 1 {
			t.Errorf("Budget() = %d for a budget of %d, want 1", got, budget)
		}

		c.Add("k", 10)
		if !c.Contains("k") {
			t.Errorf("a cache built with a budget of %d should still hold its newest entry", budget)
		}
	}
}

// TestByteCache_ConcurrentUseIsRaceFree exercises the contract the viewer
// relies on: attemptLoad's decode goroutine and preloadOne's background
// goroutines both write this cache with no fyne.Do hop between them. Only
// meaningful under -race, which is how the suite runs (see the Makefile).
func TestByteCache_ConcurrentUseIsRaceFree(t *testing.T) {
	c := newTestByteCache(500)

	keys := []string{"a", "b", "c", "d", "e"}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			for j, k := range keys {
				switch (i + j) % 5 {
				case 0:
					c.Add(k, int64(10*(j+1)))
				case 1:
					_ = c.AddIfFits(k, int64(10*(j+1)))
				case 2:
					c.Get(k)
				case 3:
					c.Contains(k)
				case 4:
					c.Remove(k)
				}
			}

			c.SetBudget(int64(200 + 50*i))
		}(i)
	}
	wg.Wait()

	// Nothing to assert about the contents after that; what matters is that
	// the running total stayed consistent with what's actually held.
	if got := c.Bytes(); got < 0 {
		t.Errorf("Bytes() = %d, want a non-negative total - the running count has drifted", got)
	}
}

// --- imageBytes ---------------------------------------------------------------

// unknownImage is an image.Image none of imageBytes' cases name, standing in
// for a decoder that returns its own type - it must fall back to the
// four-bytes-per-pixel ceiling rather than reporting zero.
type unknownImage struct{ b image.Rectangle }

func (u unknownImage) ColorModel() color.Model { return color.RGBAModel }
func (u unknownImage) Bounds() image.Rectangle { return u.b }
func (u unknownImage) At(int, int) color.Color { return color.RGBA{} }

func TestImageBytes(t *testing.T) {
	palette := color.Palette{color.Black, color.White}

	cases := []struct {
		name string
		img  image.Image
		want int64
	}{
		{"nil", nil, 0},
		{"RGBA", image.NewRGBA(image.Rect(0, 0, 4, 4)), 4 * 4 * 4},
		{"NRGBA", image.NewNRGBA(image.Rect(0, 0, 2, 2)), 2 * 2 * 4},
		{"RGBA64", image.NewRGBA64(image.Rect(0, 0, 2, 2)), 2 * 2 * 8},
		{"Gray", image.NewGray(image.Rect(0, 0, 4, 4)), 4 * 4},
		{"Gray16", image.NewGray16(image.Rect(0, 0, 4, 4)), 4 * 4 * 2},
		{"CMYK", image.NewCMYK(image.Rect(0, 0, 2, 2)), 2 * 2 * 4},
		{"Paletted", image.NewPaletted(image.Rect(0, 0, 4, 4), palette), 4 * 4},
		// 4:2:0 stores one Y sample per pixel and one Cb/Cr pair per 2x2
		// block: 16 + 4 + 4. This is the case the type switch exists for -
		// charging a JPEG 4 bytes per pixel would over-report it by 2.7x.
		{"YCbCr 4:2:0", image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420), 16 + 4 + 4},
		{"unknown type falls back to 4 bytes per pixel", unknownImage{image.Rect(0, 0, 4, 4)}, 4 * 4 * 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := imageBytes(c.img); got != c.want {
				t.Errorf("imageBytes() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestLoadedImageBytes_SumsEveryFrame(t *testing.T) {
	if got := loadedImageBytes(nil); got != 0 {
		t.Errorf("loadedImageBytes(nil) = %d, want 0", got)
	}

	// An animation's cost is all of its frames, not just the one on screen -
	// that is exactly what the entry-count cache used to miss.
	l := &LoadedImage{Frames: []image.Image{
		image.NewRGBA(image.Rect(0, 0, 4, 4)),
		image.NewRGBA(image.Rect(0, 0, 4, 4)),
		image.NewRGBA(image.Rect(0, 0, 4, 4)),
	}}

	if got, want := loadedImageBytes(l), int64(3*4*4*4); got != want {
		t.Errorf("loadedImageBytes() = %d, want %d", got, want)
	}
}

func TestEstimateDecodedBytes(t *testing.T) {
	cases := []struct {
		name   string
		bounds image.Rectangle
		want   int64
	}{
		{"ordinary bounds", image.Rect(0, 0, 100, 50), 100 * 50 * 4},
		{"empty bounds", image.Rect(0, 0, 0, 0), 0},
		// Built as a literal rather than with image.Rect, which
		// canonicalizes swapped corners - a negative Dx/Dy has to come out
		// as 0 rather than as a negative byte count.
		{"inverted bounds", image.Rectangle{Min: image.Pt(10, 10), Max: image.Pt(0, 0)}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EstimateDecodedBytes(c.bounds); got != c.want {
				t.Errorf("EstimateDecodedBytes(%v) = %d, want %d", c.bounds, got, c.want)
			}
		})
	}
}

// --- NewImgCache / NewThumbCache ---------------------------------------------

func TestNewImgCache_WeighsByDecodedFrames(t *testing.T) {
	c := NewImgCache(DefaultImgCacheBytes)

	c.Add("k", &LoadedImage{Frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 10, 10))}})

	if got, want := c.Bytes(), int64(10*10*4); got != want {
		t.Errorf("Bytes() = %d, want %d - the image cache must weigh entries by their pixels", got, want)
	}
}

func TestNewThumbCache_EvictsByBytes(t *testing.T) {
	// Two 100x100 RGBA thumbnails are 40,000 bytes each, so a 50,000-byte
	// budget fits exactly one of them.
	c := NewThumbCache(50_000)

	c.Add("a", image.NewRGBA(image.Rect(0, 0, 100, 100)))
	c.Add("b", image.NewRGBA(image.Rect(0, 0, 100, 100)))

	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (only one 40,000-byte thumbnail fits a 50,000-byte budget)", c.Len())
	}
	if c.Contains("a") {
		t.Error("the older thumbnail should have been evicted")
	}
}
