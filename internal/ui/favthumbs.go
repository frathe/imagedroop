// favthumbs.go is the viewer-side wiring for the disk-backed favorite
// preview cache (internal/favthumbs): the "Cache favorite previews on disk"
// preference the settings window binds to, and the background pass the
// favorites feature asks for whenever a favorite's file list changes.
package ui

import (
	"context"
	"errors"
	"image"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/favthumbs"
	"github.com/frathe/picfetch/internal/ui/grid"
)

// FavoritePreviewCache/SetFavoritePreviewCache are the settings window's
// getter/setter pair for the preference, the same shape memlimits.go uses
// for the three memory limits.
func (v *viewer) FavoritePreviewCache() bool { return v.settings.favPreviewCache }

// SetFavoritePreviewCache applies the preference, and switching it off also
// abandons any pass still in flight. Guarding only the *start* of a pass
// would leave the checkbox lying about itself: a favorite big enough to be
// worth caching is also big enough that its pass runs for a while, and
// during that while the app would keep decoding and keep writing preview
// files to disk after the user asked it to stop doing exactly that.
func (v *viewer) SetFavoritePreviewCache(on bool) {
	v.settings.favPreviewCache = on

	if !on {
		v.favThumbLifecycle.invalidate()
	}
}

// SyncFavoritePreviews brings the previews stored under favDir in line with
// files, in the background - the favorites feature's report that a favorite
// now holds this list, arriving both when one is saved and when one is
// opened. This is where that report turns into thumbnail work: favorites
// itself has no idea previews exist, the same way batch.go rather than the
// grid decides what a selection means.
//
// Deliberately not skipped for an empty files slice: a favorite the user
// emptied should have its previews swept, and that is exactly what a Sync
// over no files does.
func (v *viewer) SyncFavoritePreviews(favDir string, files []fyne.URI) {
	// favDir is empty when favstore.Dir was handed a name it rejects, which
	// leaves nothing to write previews into or sweep.
	if !v.settings.favPreviewCache || favDir == "" {
		return
	}

	// A new pass supersedes and cancels any pass still running, so opening
	// favorite B while A is still being walked stops A rather than leaving
	// two passes competing for decodes - and for the thumbnail cache, where
	// the loser would be evicting the winner's entries.
	token := v.favThumbLifecycle.begin()

	done := v.favThumb.Begin()

	go func() {
		defer done()

		if err := favthumbs.Sync(token.context(), favDir, files, gridSink{v.grid}); err != nil {
			// A superseded pass returns context.Canceled, which is this
			// design working rather than anything failing.
			if errors.Is(err, context.Canceled) {
				return
			}
			fyne.LogError("failed to cache favorite previews", err)
		}
	}()
}

// gridSink adapts the grid overview to favthumbs.Sink, so a preview pass
// can skip decoding what the grid already holds and hand back what it
// produced. This is the pre-warm: by the time the user presses G, the
// thumbnails for the favorite they just opened are already in memory.
//
// Both methods are called from several of Sync's worker goroutines at once,
// and neither wraps its work in fyne.Do - unlike almost everything else
// this package does off the UI goroutine. That is safe *because* of how
// little they reach: CachedThumb and StoreThumb bottom out in the grid's
// imaging.ByteCache, which guards itself with a mutex, and touch no widget,
// no canvas, and no viewer field. Anything added here that does touch a
// widget needs fyne.Do again.
type gridSink struct {
	grid *grid.Overview
}

func (s gridSink) Cached(src fyne.URI) (image.Image, bool) {
	return s.grid.CachedThumb(src)
}

func (s gridSink) Store(src fyne.URI, thumb image.Image) {
	// The check that makes the pre-warm worth doing, and the reason
	// ThumbCacheFull exists at all (see its comment in internal/ui/grid):
	// StoreThumb's AddIfFits refuses only a thumbnail too big for the whole
	// budget, and once the cache is merely full it evicts least-recently-used
	// entries and stores anyway. Offering unconditionally over a favorite
	// larger than the budget would therefore evict this pass's own earliest
	// entries as it walked the list, leaving only the tail cached - while
	// the grid opens at the head. Stopping at the budget keeps the head
	// warm and lets the tail decode on demand, exactly as it does without
	// any of this.
	if s.grid.ThumbCacheFull() {
		return
	}

	s.grid.StoreThumb(src, thumb)
}
