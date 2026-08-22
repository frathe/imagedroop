// The thumbnail decode pipeline: the byte-budgeted cache, its accessors, and
// the bounded worker pool plus the three staleness guards that decide whether
// a finished decode still belongs on the cell that asked for it.

package grid

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"github.com/frathe/picfetch/internal/imaging"
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

// Cached reports whether u's thumbnail is in the cache. Contains rather
// than Get, so asking the question doesn't reorder the cache's own idea of
// what was used least recently.
func (g *Overview) Cached(u fyne.URI) bool {
	return g.thumbs.Contains(u.String())
}

// CachedThumb returns u's cached thumbnail, if any. Get rather than
// Contains - unlike Cached above, this is a real read on behalf of a
// caller that's about to use the image (favthumbs' pre-warm pass skipping
// its own decode), so the entry should be promoted to most-recently-used
// like any other read.
func (g *Overview) CachedThumb(u fyne.URI) (image.Image, bool) {
	return g.thumbs.Get(u.String())
}

// StoreThumb offers thumb to the cache under u, reporting whether it was
// actually stored. AddIfFits, not Add: this is the same speculative-write
// situation preloadOne is in (see AddIfFits's own comment), so a preview
// too big to fit the budget at all is refused outright rather than stored
// and left to evict everything else.
//
// That refusal is the only bound this offers. Once the cache is merely
// full, AddIfFits still evicts to make room - see ThumbCacheFull below for
// why a pre-warm pass has to check that separately rather than read a
// false return here as "the cache is full".
func (g *Overview) StoreThumb(u fyne.URI, thumb image.Image) bool {
	return g.thumbs.AddIfFits(u.String(), thumb)
}

// ThumbCacheFull reports whether the thumbnail cache has reached its byte
// budget.
//
// This exists so a background pass can pre-warm the cache from
// disk-persisted previews (internal/favthumbs) before the grid ever opens,
// and StoreThumb's AddIfFits alone cannot bound that pass. AddIfFits only
// refuses an entry that outweighs the *whole* budget by itself; once the
// cache is merely full it evicts least-recently-used entries and stores
// anyway (see evict's comment in internal/imaging/bytecache.go). So a
// pre-warm that just called StoreThumb in list order over a favorite
// bigger than the budget would evict its own earliest entries as it went,
// finishing with only the *last* N thumbnails cached - while the grid
// opens at the *first* file. Checking ThumbCacheFull between offers lets
// the caller stop pre-warming at the budget instead, keeping the head of
// the list warm and letting the tail decode on demand exactly as it does
// today.
func (g *Overview) ThumbCacheFull() bool {
	return g.thumbs.Bytes() >= g.thumbs.Budget()
}

// SetCacheBytes retunes the thumbnail cache's byte budget and evicts down
// to it right away - the settings window's binding, reached through
// internal/ui's SetMaxThumbCacheMB. A setter rather than a New parameter
// for the same reason slideshow.Controller.SetInterval is one: the value
// changes while the app runs, and the overview is built once and lives in
// the window's content stack for the process's lifetime.
func (g *Overview) SetCacheBytes(n int64) {
	g.thumbs.SetBudget(n)
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
	i := g.fileIndex(id)
	if i < 0 {
		return
	}

	// Captured here, on the UI goroutine, for the same reason gen is
	// passed in: it pins which query this request's id was resolved
	// under, so the completion can tell whether that is still the query
	// on screen.
	fgen := g.filterGen.Load()

	u := g.host.FileAt(i)
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
		if !g.stillWanted(key, id, gen, fgen) {
			g.release(key, id)

			// That check raced the UI goroutine's cell updates in one
			// narrow window: the cell scrolled away and back to id between
			// the update pass (which saw the old claim and didn't spawn)
			// and here. Re-check on the UI goroutine, where updates are
			// serialized, and re-request rather than leave the cell blank
			// until something else happens to refresh it.
			fyne.Do(func() {
				if g.stillWanted(key, id, gen, fgen) {
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
			if g.stillWanted(key, id, gen, fgen) {
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
// gen, under filter generation fgen) is still worth anything to the cell
// identified by key - checked by the worker before it decodes and by the
// completion before it paints, and split out so the generation and
// cell-recycling logic can be driven directly and synchronously from a test
// instead of racing a real decode goroutine. Safe from any goroutine
// (cellIDs is a sync.Map, Generation and filterGen atomic reads).
//
// False whenever a newer drop superseded the file set gen was captured
// against, this cell has since been recycled to show a different id, or a
// keystroke has since renumbered the cells under it - the three ways the
// file this decode is carrying can stop being the file this cell shows.
func (g *Overview) stillWanted(key *fyne.Container, id int, gen, fgen uint64) bool {
	current, ok := g.cellIDs.Load(key)

	return ok && gen == g.host.Generation() && fgen == g.filterGen.Load() && current == id
}
