package favthumbs

import (
	"context"
	"image"
	"sync"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/imaging"
)

// syncConcurrency bounds how many files a single pass works on at once - a
// small worker-pool semaphore rather than one goroutine per file, since a
// favorite can hold thousands and each miss costs a full-resolution decode.
//
// This is deliberately its own budget rather than a share of grid's
// thumbConcurrency. The two pools run in different situations: the grid's
// serves decodes the user is waiting to see, while this one runs in the
// background of whatever the user is doing now. Making them draw on one
// budget would let a background pass over a large favorite starve the
// thumbnails actually on screen, which is the exact opposite of what a
// preview cache is for.
const syncConcurrency = 4

// Sink is the consumer-side view of the caller's in-memory thumbnail cache.
//
// Both methods are called from Sync's worker goroutines, several at a time
// and never on the UI goroutine, so an implementation has to be safe for
// concurrent use. Binding this to a mutex-guarded cache (imaging.ByteCache
// is one) satisfies that; a bare map does not.
type Sink interface {
	// Cached returns a thumbnail the caller already holds in memory.
	Cached(src fyne.URI) (image.Image, bool)

	// Store offers a thumbnail to the caller's cache. The caller decides
	// whether to keep it; Sync does not care whether it was kept.
	Store(src fyne.URI, thumb image.Image)
}

// Sync brings favDir's previews in line with files: every file ends up with
// a current preview on disk, the caller's in-memory cache is offered
// whatever the pass produced or found, and previews that no file maps to
// any more are pruned.
//
// Each file takes the cheapest route that gets it there. A thumbnail the
// caller already holds needs neither a decode nor a read, only a write if
// disk is behind. A preview already on disk needs no decode. Only a file
// that is in neither place pays for imaging.LoadThumbnail.
//
// One file's failure does not abort its peers - a truncated download in a
// favorite of two thousand should cost one preview, not all of them - so
// the pass runs to the end and returns the first error it saw. A cancelled
// ctx returns ctx.Err().
//
// sink may be nil, which reads as "nothing is cached, and storing is a
// no-op": the pass still fills the on-disk cache for a later opener.
func Sync(ctx context.Context, favDir string, files []fyne.URI, sink Sink) error {
	// The app's merge mode loads one path at two indices whenever the same
	// file arrives from two dropped folders. Two workers on that path would
	// duplicate a full decode and then race each other to write a single
	// destination, so the repeats come out before any work is handed out.
	// Sweep below still gets the original slice: it builds its own
	// expected-name set, which dedupes internally.
	work := dedupe(files)

	// sem bounds concurrent decodes; wg lets the sweep below wait for every
	// worker to be completely done, which is what makes the sweep's view of
	// the directory a settled one rather than a snapshot mid-pass.
	sem := make(chan struct{}, syncConcurrency)
	var wg sync.WaitGroup

	// Several workers can fail at once, so the shared first error needs a
	// lock of its own; it is not derivable from anything the workers
	// otherwise share.
	var mu sync.Mutex
	var firstErr error
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()

		if firstErr == nil {
			firstErr = err
		}
	}

loop:
	for _, u := range work {
		// Checked before queueing as well as inside the worker so a
		// cancelled pass stops handing out goroutines at all, rather than
		// spawning one per remaining file just to have each discover the
		// cancellation for itself.
		if ctx.Err() != nil {
			break
		}

		// Acquired here rather than inside the worker so the number of
		// live goroutines is the pool size, not the size of the favorite:
		// taking the slot first means a two-thousand-file list parks the
		// loop on this send instead of parking two thousand goroutines on
		// it. Cancellation gets a case of its own because this send blocks,
		// and a superseded pass should stop queueing immediately rather
		// than wait out a decode for a favorite nobody is looking at.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}

		wg.Go(func() {
			defer func() { <-sem }()

			// Bail *before* decoding, not just after: a large favorite
			// leaves most of its files queued behind sem, and by the time
			// they get their turn the pass may well have been superseded
			// by the user opening a different favorite. Checking here
			// drains that backlog at check speed instead of grinding
			// through a full-resolution decode per file whose result
			// nobody is waiting for any more.
			if ctx.Err() != nil {
				return
			}

			if err := syncFile(favDir, u, sink); err != nil {
				fail(err)
			}
		})
	}

	wg.Wait()

	// A cancelled pass never established which previews are garbage: files
	// it never reached have no preview yet through no fault of their own.
	// Sweeping on that basis would delete previews that are perfectly live
	// and force a full re-decode on the next open, so a cut-short pass
	// leaves the directory alone and the next complete one prunes it.
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := Sweep(favDir, files); err != nil {
		fail(err)
	}

	mu.Lock()
	defer mu.Unlock()

	return firstErr
}

// syncFile brings one file's preview up to date, taking the cheapest of the
// three routes that applies. It is the body of a worker goroutine, so it
// touches nothing shared beyond sink, which the caller owns and guards.
func syncFile(favDir string, u fyne.URI, sink Sink) error {
	if sink != nil {
		if thumb, ok := sink.Cached(u); ok {
			// Deliberately no Store here: the sink is where this
			// thumbnail just came from, so offering it back is pure
			// churn. Disk still has to catch up if it is behind, and
			// hasCurrentPreview answers that with a stat rather than the
			// decode Read would cost to reach the same conclusion.
			if hasCurrentPreview(favDir, u) {
				return nil
			}
			return Write(favDir, u, thumb)
		}
	}

	if thumb, ok := Read(favDir, u); ok {
		if sink != nil {
			sink.Store(u, thumb)
		}
		return nil
	}

	thumb, err := imaging.LoadThumbnail(u)
	if err != nil {
		return err
	}

	// The sink is offered the thumbnail even when the write fails: a full
	// disk or a read-only volume is no reason to make the caller decode
	// this file again for the display it is about to paint.
	err = Write(favDir, u, thumb)
	if sink != nil {
		sink.Store(u, thumb)
	}
	return err
}

// dedupe returns files with repeats of the same path removed, preserving
// first-appearance order. Nil entries drop out with them, since there is no
// file for the pass to work on.
func dedupe(files []fyne.URI) []fyne.URI {
	seen := make(map[string]bool, len(files))
	out := make([]fyne.URI, 0, len(files))
	for _, u := range files {
		if u == nil {
			continue
		}

		path := u.Path()
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, u)
	}
	return out
}
