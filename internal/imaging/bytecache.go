package imaging

import (
	"container/list"
	"image"
	"sync"
)

// ByteCache is an LRU cache bounded by the estimated byte weight of what it
// holds, rather than by a count of entries. Decoded image memory varies by
// four orders of magnitude between a 16x16 icon and a 200-megapixel
// panorama, so an entry count says nothing useful about how much memory the
// cache is actually using - which is the whole reason this type exists
// instead of a plain count-bounded LRU.
//
// It is safe for concurrent use on its own: internal/ui's attemptLoad
// decode goroutine and its preloadOne background goroutines both populate
// the image cache without going through fyne.Do, and internal/ui/grid's
// worker pool does the same for thumbnails.
type ByteCache[V any] struct {
	mu     sync.Mutex
	budget int64
	used   int64
	weigh  func(V) int64

	// ll orders entries most- to least-recently used (front is newest);
	// items indexes into it so a lookup doesn't walk the list.
	ll    *list.List
	items map[string]*list.Element
}

// cacheEntry is what ll's elements hold. The weight is stored rather than
// recomputed on eviction because the value may have been mutated in place
// since it was added (internal/ui/save.go swaps a rotated frame into a
// LoadedImage it then evicts) - the running total has to be unwound by
// exactly what was added to it, or it drifts.
type cacheEntry[V any] struct {
	key    string
	val    V
	weight int64
}

// NewByteCache returns a cache holding at most budget bytes, as measured by
// weigh. A budget below 1 is raised to 1 rather than rejected: the eviction
// rule below keeps the most recently added entry regardless of budget, so
// even an absurdly small one still behaves correctly - it just degenerates
// to holding a single entry.
func NewByteCache[V any](budget int64, weigh func(V) int64) *ByteCache[V] {
	if budget < 1 {
		budget = 1
	}

	return &ByteCache[V]{
		budget: budget,
		weigh:  weigh,
		ll:     list.New(),
		items:  make(map[string]*list.Element),
	}
}

// Get returns the value stored under key and promotes it to most-recently
// used. Use Contains instead for a presence test that shouldn't reorder
// anything - see its own comment.
func (c *ByteCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	c.ll.MoveToFront(el)

	return el.Value.(*cacheEntry[V]).val, true
}

// Contains reports whether key is cached, without promoting it. This is the
// right call for a "have we already got this?" check on a speculative path
// (internal/ui's preloadOne, internal/ui/grid's Cached): promoting a
// neighbor the user isn't looking at can make it outlive the image that's
// actually on screen once the budget is tight.
func (c *ByteCache[V]) Contains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.items[key]

	return ok
}

// Add stores v under key as the most recently used entry, evicting older
// ones until the budget is met. An entry that on its own exceeds the whole
// budget is still stored: see evict for why that's deliberate.
func (c *ByteCache[V]) Add(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.add(key, v)
}

// AddIfFits stores v only if it fits in the budget by itself, reporting
// whether it did. This is what speculative writers use (internal/ui's
// preloadOne): Add's never-evict-the-newest rule exists to protect the
// image being displayed, and a preloaded neighbor big enough to trigger it
// would evict that image instead - the opposite of the point.
func (c *ByteCache[V]) AddIfFits(key string, v V) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.weigh(v) > c.budget {
		return false
	}

	c.add(key, v)

	return true
}

// add is the shared body of Add/AddIfFits. Caller holds c.mu.
func (c *ByteCache[V]) add(key string, v V) {
	w := c.weigh(v)

	// Re-adding an existing key replaces its value, so the running total
	// has to swap the old weight out rather than add on top of it.
	if el, ok := c.items[key]; ok {
		e := el.Value.(*cacheEntry[V])
		c.used += w - e.weight
		e.val, e.weight = v, w
		c.ll.MoveToFront(el)
	} else {
		c.items[key] = c.ll.PushFront(&cacheEntry[V]{key: key, val: v, weight: w})
		c.used += w
	}

	c.evict()
}

// evict drops least-recently-used entries until the budget is met - but
// never the last one standing, even when that single entry exceeds the
// budget on its own. That exception is what lets the viewer display an
// image larger than the whole cache budget and still find it cached on the
// way back: Add puts the image being displayed at the front, so it is the
// entry that survives. Without it, a budget smaller than one photo would
// make the cache permanently empty.
func (c *ByteCache[V]) evict() {
	for c.used > c.budget && c.ll.Len() > 1 {
		c.removeElement(c.ll.Back())
	}
}

// Remove drops key's entry if present.
func (c *ByteCache[V]) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// Purge drops every entry, releasing the memory they held. internal/ui's
// clearToDropzone calls this: once the file set is closed, holding decodes
// of files no longer open just spends the budget on nothing.
func (c *ByteCache[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.used = 0
}

// SetBudget changes the byte budget and immediately evicts down to it, so a
// user lowering the limit in the settings window frees the memory now
// rather than at some later Add.
func (c *ByteCache[V]) SetBudget(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if n < 1 {
		n = 1
	}

	c.budget = n
	c.evict()
}

// Budget reports the current byte budget - what a caller deciding whether a
// decode is worth starting compares an estimate against.
func (c *ByteCache[V]) Budget() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.budget
}

// Bytes reports the total weight currently held.
func (c *ByteCache[V]) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.used
}

// Len reports how many entries are held. Unlike the count-bounded cache
// this replaced, it is a diagnostic rather than the thing being bounded.
func (c *ByteCache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ll.Len()
}

// removeElement unlinks el and unwinds its weight. Caller holds c.mu.
func (c *ByteCache[V]) removeElement(el *list.Element) {
	e := el.Value.(*cacheEntry[V])

	c.ll.Remove(el)
	delete(c.items, e.key)
	c.used -= e.weight
}

// imageBytes estimates the memory m's pixel data retains. Type-switched on
// the concrete types Go's decoders actually produce rather than assuming
// four bytes per pixel throughout: a JPEG decodes to *image.YCbCr, which at
// 4:2:0 subsampling is about 1.5 bytes per pixel, and charging it 2.7x what
// it costs would make the megabyte figure in the settings window mean
// nothing to the user who set it.
func imageBytes(m image.Image) int64 {
	switch t := m.(type) {
	case nil:
		return 0
	case *image.RGBA:
		return int64(len(t.Pix))
	case *image.RGBA64:
		return int64(len(t.Pix))
	case *image.NRGBA:
		return int64(len(t.Pix))
	case *image.NRGBA64:
		return int64(len(t.Pix))
	case *image.Alpha:
		return int64(len(t.Pix))
	case *image.Alpha16:
		return int64(len(t.Pix))
	case *image.Gray:
		return int64(len(t.Pix))
	case *image.Gray16:
		return int64(len(t.Pix))
	case *image.CMYK:
		return int64(len(t.Pix))
	case *image.Paletted:
		return int64(len(t.Pix))
	case *image.YCbCr:
		return int64(len(t.Y) + len(t.Cb) + len(t.Cr))
	case *image.NYCbCrA:
		return int64(len(t.Y) + len(t.Cb) + len(t.Cr) + len(t.A))
	}

	// Anything else (a decoder's own image type, or a wrapper) falls back
	// to the four-bytes-per-pixel ceiling - an over-estimate is the safe
	// direction for a budget.
	return EstimateDecodedBytes(m.Bounds())
}

// loadedImageBytes weighs a whole LoadedImage: every frame, since an
// animated GIF retains a full composited canvas for each one and that -
// not the single-frame case - is what the budget has to survive. The
// encoded FileSize isn't counted: those bytes are released once the decode
// returns, and only the frames are still referenced by the time an entry
// reaches the cache.
func loadedImageBytes(l *LoadedImage) int64 {
	if l == nil {
		return 0
	}

	var total int64
	for _, f := range l.Frames {
		total += imageBytes(f)
	}

	// A retained Vector's parse tree is real memory the cache would
	// otherwise hold for free. Its true footprint cannot be measured
	// without walking it, but it is proportional to the source, and the
	// source length is already bounded by MaxEncodedBytes.
	if l.Vector != nil {
		total += int64(l.Vector.srcBytes)
	}

	return total
}

// EstimateDecodedBytes is the worst-case decoded size of an image whose
// header declares these bounds - for callers deciding whether a decode is
// worth starting at all, before there is any concrete image type to
// measure. Deliberately the four-bytes-per-pixel ceiling: guessing low
// here would let exactly the images this budget exists to bound slip
// through the check.
func EstimateDecodedBytes(b image.Rectangle) int64 {
	w, h := int64(b.Dx()), int64(b.Dy())
	if w <= 0 || h <= 0 {
		return 0
	}

	return w * h * 4
}
