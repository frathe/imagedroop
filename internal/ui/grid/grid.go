// Package grid is the full-window thumbnail overview (the G key): a
// virtualized grid of every loaded image, for jumping around a large drop
// by sight instead of arrowing through it one file at a time.
//
// It owns the thumbnail cache and the bounded worker pool that fills it,
// and reaches back into the app through Host. It knows nothing about the
// app's other full-window mode (the slideshow): the two don't compose, but
// that guard lives in the app's key dispatcher, not here.
package grid

import (
	"image"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/imagedrop/internal/imaging"
	"github.com/frathe/imagedrop/internal/ui/widgets"
	"github.com/frathe/imagedrop/internal/winpos"
)

// thumbConcurrency bounds how many thumbnail decodes run at once - a small
// worker-pool semaphore rather than one goroutine per request.
// widget.GridWrap is virtualized (it only ever builds/updates cells for the
// currently visible rows, unlike container.NewGridWrap), which already
// keeps the *number* of thumbnails requested at once bounded to roughly a
// screenful even for a several-thousand-file folder; this bounds how many
// of those run in parallel too, since a photo can still be tens of
// megapixels before scaling shrinks it down.
const thumbConcurrency = 4

// cellSize is the fixed width/height, in canvas points, each grid cell is
// laid out at.
const cellSize = 120

// Host is what the overview needs from the application: the file set to
// draw, the generation counter that tells a finished decode whether its
// file set is still current, and the display actions a selection triggers.
type Host interface {
	// FileCount is how many files are loaded.
	FileCount() int

	// FileAt returns the file at index i.
	FileAt(i int) fyne.URI

	// CurrentIndex is the file currently on screen - where the highlight
	// starts when the grid opens.
	CurrentIndex() int

	// Generation is the app's load generation. A decode captures it when
	// it starts and discards its result if it no longer matches, so a
	// fresh drop can't have a stale thumbnail painted into it.
	Generation() uint64

	// ShowImage displays the file at index i.
	ShowImage(i int)

	// ForceRepaint redraws the window after a visibility change.
	ForceRepaint()

	// Unfocus releases canvas focus - see Close for why that matters.
	Unfocus()
}

// Overview is the grid overlay and the state behind it.
type Overview struct {
	host Host
	win  fyne.Window

	visible bool
	wrap    *widget.GridWrap
	overlay *fyne.Container

	// maximized is true from the moment Toggle grows the window via
	// winpos.Maximize until ConsumeMaximized is next called. Left true
	// across a plain Close - see its own doc comment: closing the grid
	// deliberately leaves the window maximized - so a later resize the app
	// makes for a reason of its own (not just picking an image straight out
	// of the grid, but navigating on afterward too) still knows to undo it
	// first.
	maximized bool

	// highlight is which cell's ring is currently drawn, moved by the
	// arrow keys while the grid is up and committed with Return. Reset to
	// the host's current index every time the grid opens, so it starts on
	// whichever image was already on screen.
	highlight int

	// thumbs holds small, already-downsampled thumbnails keyed by URI
	// string - a separate LRU cache and budget from the app's full-size
	// image cache (imaging.NewThumbCache vs NewImgCache), since reusing
	// one for both would evict full-size decodes the normal viewing path
	// still needs, and vice versa.
	thumbs *lru.Cache[string, image.Image]

	// sem bounds concurrent decodes; pending counts them, so Settle can
	// wait for every spawned decode to finish.
	sem     chan struct{}
	pending sync.WaitGroup

	// cellIDs tracks which file id each recycled cell is currently
	// showing: GridWrap reuses a small, fixed pool of cell widgets as the
	// user scrolls rather than creating one per file, so an async decode
	// kicked off against an earlier id has to check, once it completes,
	// that its cell hasn't since been recycled to show a different file. A
	// *sync.Map, not a plain map: the update callback writes it on the UI
	// goroutine while a decode's fyne.Do callback reads it - under the
	// real GLFW driver both run on the same marshaled UI goroutine so a
	// plain map would never actually race, but the test driver runs
	// fyne.Do synchronously on the calling (background) goroutine instead,
	// which turned a plain map into a genuine, test-reproducible
	// concurrent read/write.
	cellIDs sync.Map

	// inflight records, per recycled cell, which id a spawned decode is
	// currently working toward - the claim/release pair around each decode
	// goroutine, so repeated update passes over a still-decoding cell
	// don't stack duplicate goroutines behind sem. A sync.Map for the same
	// reason as cellIDs.
	inflight sync.Map
}

// New builds the overview (hidden) around host. win is maximized (see
// Toggle) each time the overview opens - a bigger window means bigger, more
// legible thumbnails - the same reason slideshow.Controller is handed win
// directly rather than reaching for it some other way.
//
// Each cell is a small stack of the thumbnail image plus a highlight ring
// (mirroring the delete confirmation's own selection rings), rather than
// relying on GridWrap's own built-in highlight rendering - that ties into
// real Fyne canvas focus, which this app deliberately never hands to
// GridWrap (see Close's comment on why).
func New(host Host, win fyne.Window) *Overview {
	g := &Overview{
		host:   host,
		win:    win,
		thumbs: imaging.NewThumbCache(),
		sem:    make(chan struct{}, thumbConcurrency),
	}

	g.wrap = widget.NewGridWrap(
		host.FileCount,
		func() fyne.CanvasObject {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.ScaleMode = canvas.ImageScaleFastest
			img.SetMinSize(fyne.NewSize(cellSize, cellSize))

			ring := widgets.NewFocusRing(widgets.GridRingWidth, widgets.RingRadius)
			ring.Hide()

			return container.NewStack(img, ring)
		},
		func(id widget.GridWrapItemID, o fyne.CanvasObject) {
			cell := o.(*fyne.Container)
			img := cell.Objects[0].(*canvas.Image)
			ring := cell.Objects[1].(*canvas.Rectangle)

			g.cellIDs.Store(cell, id)
			setCellHighlighted(ring, id == g.highlight)

			// Refresh reaches this callback whether or not the overlay is
			// actually open: every ForceRepaint refreshes the whole widget
			// tree, hidden GridWrap included. Requesting thumbnails from
			// those refreshes would kick off background decodes for an
			// invisible grid on every navigation step - and, under the
			// fyne test driver (where fyne.Do runs a goroutine's callback
			// inline on the caller instead of marshaling it to the UI
			// goroutine), would let such a decode's completion paint race
			// a later refresh's own cell reset. Skip while hidden: Toggle
			// sets visible before its own refresh, so opening the grid
			// still populates every visible cell.
			if !g.visible {
				img.Image = nil
				img.Refresh()
				return
			}

			// While visible, blanking is requestThumbnail's job, and only
			// on a cache miss - clearing unconditionally here made every
			// scroll tick repaint every already-cached cell twice, with an
			// empty flash in between.
			g.requestThumbnail(cell, img, id, g.host.Generation())
		},
	)

	g.wrap.OnSelected = func(id widget.GridWrapItemID) {
		g.Close()
		if id >= 0 && id < g.host.FileCount() {
			g.host.ShowImage(id)
		}
		g.wrap.UnselectAll()
	}

	// Fired both by keyboard highlight movement (HandleKey forwards the
	// arrow keys to wrap.TypedKey, see below) and by mouse hover (GridWrap
	// wires its own onHovered to the same callback) - either way, move the
	// ring to match. GridWrap's own TypedKey already calls RefreshItem on
	// the old and new positions before this fires, but at that point
	// g.highlight still holds the *old* value, so those calls redraw both
	// cells as "still old" - these two RefreshItem calls are the ones that
	// actually apply the moved ring, now that g.highlight has been updated.
	g.wrap.OnHighlighted = func(id widget.GridWrapItemID) {
		old := g.highlight
		g.highlight = id
		g.wrap.RefreshItem(old)
		g.wrap.RefreshItem(id)
	}

	// An opaque backdrop, not a translucent scrim like the delete
	// confirmation's: the grid replaces the image view entirely rather
	// than dimming it behind a centered card, so it needs to fully hide
	// whatever's underneath.
	backdrop := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	g.overlay = container.NewStack(backdrop, container.NewPadded(g.wrap))
	g.overlay.Hide()

	return g
}

// Overlay is the full-window grid, for the app to place in its window
// stack.
func (g *Overview) Overlay() fyne.CanvasObject {
	return g.overlay
}

// Visible reports whether the grid is up - the app's key dispatcher checks
// this before its own handling.
func (g *Overview) Visible() bool {
	return g.visible
}

// Highlight is the currently ringed cell.
func (g *Overview) Highlight() int {
	return g.highlight
}

// ConsumeMaximized reports whether the window is still sitting maximized
// from an earlier Toggle and hasn't been undone since, clearing the flag
// either way - a one-shot check for whoever is about to resize the window
// for a reason of their own to know whether it first needs to undo the
// grid's maximize (see winpos.Unmaximize), without ever being told twice.
func (g *Overview) ConsumeMaximized() bool {
	m := g.maximized
	g.maximized = false
	return m
}

// Toggle flips the grid on or off. A no-op with nothing loaded. The
// caller is responsible for not opening it while another full-window mode
// owns the screen - see the app's key dispatcher.
func (g *Overview) Toggle() {
	if g.visible {
		g.Close()
		return
	}
	if g.host.FileCount() == 0 {
		return
	}

	g.visible = true

	// Maximize, not full-screen (see winpos.Maximize) - more room for more,
	// bigger thumbnails at once, without picture-frame mode's chrome-free
	// look. Deliberately one-way: closing the grid does not shrink the
	// window back down, the same way clicking a real maximize button
	// doesn't un-maximize when you switch to another app and back. A no-op
	// wherever winpos can't reach a native window (the fyne test driver,
	// Wayland, mobile, wasm), so the grid still opens there, just without
	// the resize.
	winpos.Maximize(g.win)
	g.maximized = true

	// Start the highlight on whichever image is currently on screen, and
	// scroll it into view - ScrollTo also refreshes the grid, which is
	// what actually paints the ring now that highlight is set.
	g.highlight = g.host.CurrentIndex()
	g.wrap.ScrollTo(g.highlight)
	g.overlay.Show()
	g.host.ForceRepaint()
}

// Close dismisses the grid, restoring the normal image view. A no-op when
// it isn't showing, so the app can call it defensively (on every drop, and
// when entering its other full-window mode) without checking Visible
// first.
//
// Unfocuses the canvas on the way out: tapping a thumbnail is a real Fyne
// widget tap, and Fyne's own GridWrap unconditionally grabs canvas focus
// on tap before calling OnSelected. This app otherwise never uses Fyne's
// widget-focus system - every other key binding is dispatched manually
// from the canvas's default (unfocused) key handler - so a focused
// GridWrap left behind after closing would silently swallow every key
// press afterward (arrow keys included) until something else happened to
// steal focus back.
func (g *Overview) Close() {
	if !g.visible {
		return
	}

	g.visible = false
	g.overlay.Hide()
	g.host.Unfocus()
	g.host.ForceRepaint()
}

// HandleKey handles a key press while the grid is up: Escape and G close
// it, Return commits the highlighted cell, and the arrow keys move the
// highlight. Every other key is deliberately swallowed by the caller.
func (g *Overview) HandleKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyEscape, fyne.KeyG:
		g.Close()
	case fyne.KeyReturn, fyne.KeyEnter:
		g.wrap.Select(g.highlight)
	default:
		// GridWrap already knows how to move its own highlight across
		// rows and columns, including the row arithmetic - forward the
		// event rather than reimplementing it here.
		g.wrap.TypedKey(ev)
	}
}

// Warm decodes thumbnails for the host's current file set into the cache,
// synchronously, so a subsequent open paints every cell from the cache
// instead of spawning background decodes.
//
// The app itself decodes lazily, as cells scroll into view; this exists
// for callers that need the cache populated up front. In practice that is
// the test suite: under the fyne test driver a decode's completion runs
// inline on the decoding goroutine, so a lazily-filled grid can paint a
// cell while the update pass that spawned the decode is still walking
// cells - a race no amount of waiting afterwards can undo, only avoided by
// having nothing to spawn.
func (g *Overview) Warm() error {
	for i := 0; i < g.host.FileCount(); i++ {
		u := g.host.FileAt(i)
		if _, ok := g.thumbs.Get(u.String()); ok {
			continue
		}

		thumb, err := imaging.LoadThumbnail(u)
		if err != nil {
			return err
		}
		g.thumbs.Add(u.String(), thumb)
	}

	return nil
}

// Settle waits for every thumbnail decode spawned so far to finish -
// including its completion paint, which runs before the wait returns. The
// app never needs this; tests do, to keep a decode goroutine from touching
// widgets after the test that started it has moved on.
func (g *Overview) Settle() {
	g.pending.Wait()
}

// Cached reports whether u's thumbnail is in the cache.
func (g *Overview) Cached(u fyne.URI) bool {
	_, ok := g.thumbs.Get(u.String())

	return ok
}

// setCellHighlighted shows or hides a cell's highlight ring.
func setCellHighlighted(ring *canvas.Rectangle, highlighted bool) {
	if highlighted {
		ring.Show()
	} else {
		ring.Hide()
	}
}

// requestThumbnail fills img with the thumbnail for the file at id, from
// the cache if present (painted synchronously) or freshly decoded and
// scaled otherwise. key identifies which cell this request was made for
// (see the cellIDs field) - it's the stable per-slot container, not img
// itself, only because that's what cellIDs and inflight are keyed by; img
// is where the result actually gets painted. gen is the host's generation
// at request time: if a new drop supersedes the current file set before
// the decode finishes, the result must not be painted - a now-meaningless
// index into a cell. cellIDs[key] guards the paint for the same reason at
// a finer grain - the file set can still be current while this particular
// cell has scrolled on to show a different id in the meantime.
func (g *Overview) requestThumbnail(key *fyne.Container, img *canvas.Image, id int, gen uint64) {
	if id < 0 || id >= g.host.FileCount() {
		return
	}

	u := g.host.FileAt(id)
	cacheKey := u.String()

	if thumb, ok := g.thumbs.Get(cacheKey); ok {
		img.Image = thumb
		img.Refresh()

		return
	}

	// Nothing to show while the decode is in flight; whatever the recycled
	// cell held last belongs to a different file. Skipped when already
	// blank so the repaints that arrive during a slow decode don't each
	// redraw an empty cell.
	if img.Image != nil {
		img.Image = nil
		img.Refresh()
	}

	if !g.claim(key, id) {
		return
	}

	// pending lets Settle wait for every spawned decode to fully finish -
	// Done fires only after the completion fyne.Do below has returned, so
	// a Wait that comes back guarantees no decode goroutine will touch a
	// widget afterwards.
	g.pending.Go(func() {

		g.sem <- struct{}{}
		defer func() { <-g.sem }()

		// Bail *before* decoding, not just after: during a fast scroll
		// through a large set, most queued requests are for cells recycled
		// long ago to other files, and this predicate is exactly what the
		// completion below would discard their results with anyway.
		// Checking it here (safe off the UI goroutine: a sync.Map load and
		// an atomic generation read) drains that dead backlog at lookup
		// speed - without it, the workers grind through a full decode per
		// scrolled-past cell while the cells actually on screen sit blank
		// at the back of the queue.
		if !g.stillWanted(key, id, gen) {
			g.release(key, id)

			// That check raced the UI goroutine's cell updates in one
			// narrow window: the cell scrolled away and back to id between
			// the update pass (which saw the old claim and didn't spawn)
			// and here. Re-check on the UI goroutine, where updates are
			// serialized, and re-request rather than leave the cell blank
			// until something else happens to refresh it.
			fyne.Do(func() {
				if g.stillWanted(key, id, gen) {
					g.requestThumbnail(key, img, id, gen)
				}
			})

			return
		}

		// A second look at the cache: merge mode can load one path at two
		// indices, so a peer worker may have finished this exact file
		// while this request sat behind sem.
		thumb, ok := g.thumbs.Get(cacheKey)
		if !ok {
			var err error
			if thumb, err = imaging.LoadThumbnail(u); err != nil {
				// No retry here: release lets the cell's next update pass
				// claim and try again, and the normal viewing path is
				// where the file's actual error surfaces to the user.
				g.release(key, id)
				return
			}

			// Cached unconditionally, not gated on stillWanted like the
			// paint below: the thumbnail is keyed by URI, not index, so it
			// stays valid however far the cell has scrolled on. Discarding
			// it would mean decoding the same file again the moment the
			// user scrolls back.
			g.thumbs.Add(cacheKey, thumb)
		}

		g.release(key, id)

		fyne.Do(func() {
			if g.stillWanted(key, id, gen) {
				img.Image = thumb
				img.Refresh()
			}
		})
	})
}

// claim records that a decode goroutine is being spawned to fill the cell
// key with id's thumbnail - false means an identical decode is already in
// flight, so don't spawn another. Every repaint re-runs GridWrap's update
// callback for every visible cell, and one multi-megapixel decode easily
// outlives several repaints; without this gate each of those passes would
// stack another goroutine behind sem for work already underway. A claim
// for a different id overwrites the old entry - the cell scrolled on, and
// the superseded decode's release is compare-and-delete precisely so its
// late finish can't clobber the newer claim.
func (g *Overview) claim(key *fyne.Container, id int) bool {
	if cur, ok := g.inflight.Load(key); ok && cur == id {
		return false
	}
	g.inflight.Store(key, id)

	return true
}

// release clears key's claim, but only if it still belongs to id - see
// claim on why a finished decode must not drop a newer claim made over it.
func (g *Overview) release(key *fyne.Container, id int) {
	g.inflight.CompareAndDelete(key, id)
}

// stillWanted reports whether a decode for id (kicked off at generation
// gen) is still worth anything to the cell identified by key - checked by
// the worker before it decodes and by the completion before it paints, and
// split out so the generation and cell-recycling logic can be driven
// directly and synchronously from a test instead of racing a real decode
// goroutine. Safe from any goroutine (cellIDs is a sync.Map, Generation an
// atomic read). False whenever a newer drop superseded the file set gen
// was captured against, or this cell has since been recycled to show a
// different id.
func (g *Overview) stillWanted(key *fyne.Container, id int, gen uint64) bool {
	current, ok := g.cellIDs.Load(key)

	return ok && gen == g.host.Generation() && current == id
}
