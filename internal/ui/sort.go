package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/filesort"
)

// toggleSort is the S key: it cycles v.sortMode to the next mode - see
// SetSortMode below, which does the actual work.
func (v *viewer) toggleSort() {
	v.SetSortMode(v.sortMode.Next())
}

// SetSortMode sets the sort order directly - the settings window's binding
// for the cycle above. Re-derives v.files from v.unsortedFiles under the
// new mode in the background (see filesort.Order's own doc comment: the
// capture-date/modified/size modes each stat or Exif-read every file, which
// visibly pauses a large recursive folder scan if done inline on the UI
// goroutine), keeping whichever file is currently on screen in view across
// the switch instead of jumping to wherever position 0 lands. Safe to call
// before any files are ever loaded, unlike toggleSort's own S-key call
// site, which is gated behind handleKeyEvent's len(v.files)<2 guard.
func (v *viewer) SetSortMode(m filesort.Mode) {
	if len(v.files) == 0 {
		v.sortMode = m
		v.applyTitle()

		return
	}

	current := v.files[v.index]

	// Defensively copied rather than aliased: v.unsortedFiles's backing
	// array can be mutated in place by RemoveFile (a failed-decode retry
	// dropping a file, or a Shift+Delete) while this snapshot is still
	// being read by startSort's background goroutine - a concurrent
	// read/write on the same backing array that filesort.Order's own copy
	// of its argument doesn't protect against, since that copy only happens
	// after this handoff.
	unsorted := append([]fyne.URI(nil), v.unsortedFiles...)

	// The title's sort-mode prefix updates immediately, even before the
	// reorder itself finishes - there's no reason to make the user wait for
	// a large sort just to see that their choice registered.
	v.sortMode = m
	v.applyTitle()

	v.startSort(m, unsorted, func(ordered []fyne.URI) {
		v.files = ordered
		v.ForceRepaint()
		v.showFileIfPresent(current)
	})
}

// invalidateSort bumps sortGen and, if a reorder is currently in flight,
// cancels its context so filesort.Order's per-file stat/Exif loop notices
// and stops promptly instead of running to completion for a result that's
// already guaranteed to be discarded - see sortGen's own field comment for
// every caller (a newer sort superseding an older one, Escape via
// cancelSort, RemoveFile, clearToDropzone). Returns the new generation, for
// startSort's own use as the gen its freshly-started sort should report
// under - nothing else could have bumped sortGen again between this call
// and that one, since both run synchronously on the UI goroutine.
func (v *viewer) invalidateSort() uint64 {
	gen := v.sortGen.Add(1)
	if v.sorting {
		v.sorting = false
		v.sortCancel()
	}
	return gen
}

// startSort reorders unsorted under mode in the background, showing the sort
// spinner/label while it computes - shared by SetSortMode and
// applyScannedFiles (drop.go), the two places that call filesort.Order over a
// potentially large file set: its capture-date/modified/size modes stat or
// Exif-read every file, which freezes the UI for as long as that takes if
// done inline on the UI goroutine (see filesort.Order's own doc comment).
// Any sort already in flight is cancelled first via invalidateSort, rather
// than left to keep computing a result this call already supersedes - so
// pressing S repeatedly cycles straight through modes instead of queuing up
// wasted background work behind whichever one happened to be slowest.
// onDone runs once, and only if this call's generation is still current once
// the reorder finishes - see sortGen's field comment for every way it can be
// superseded.
func (v *viewer) startSort(mode filesort.Mode, unsorted []fyne.URI, onDone func(ordered []fyne.URI)) {
	gen := v.invalidateSort()
	v.sorting = true

	ctx, cancel := context.WithCancel(context.Background())
	v.sortCancel = cancel

	sortDone := make(chan struct{})
	v.sortDone = sortDone

	v.sortSpinner.Show()
	v.sortLabel.Show()
	// A widget hidden since construction has never been painted, so it has
	// no canvas of its own to mark dirty on Show/Refresh - see
	// ForceRepaint's own doc comment.
	v.ForceRepaint()

	go func() {
		ordered := filesort.Order(ctx, mode, unsorted)
		fyne.Do(func() {
			v.finishSort(gen, ordered, sortDone, cancel, onDone)
		})
	}()
}

// finishSort is startSort's completion step, shaped like drop.go's
// applyScanResult: it must run on the UI goroutine (startSort's goroutine
// wraps it in fyne.Do), always closes sortDone (honoring that channel's
// contract even when a newer generation has made this result stale), and
// always releases cancel - the context.CancelFunc for *this* generation's
// own ctx, captured by the goroutine that's calling in, not read back
// through v.sortCancel (which may already point at a newer generation's
// cancel func by the time this runs).
func (v *viewer) finishSort(gen uint64, ordered []fyne.URI, sortDone chan struct{}, cancel context.CancelFunc, onDone func([]fyne.URI)) {
	defer close(sortDone)
	defer cancel()

	v.sortSpinner.Hide()
	v.sortLabel.Hide()

	// Superseded either by a newer sort (another startSort call bumped
	// sortGen again) or by something else that changed
	// v.files/v.unsortedFiles while this one was still computing
	// (Shift+Delete, or Escape/File>Close - see those call sites' own
	// invalidateSort call). Applying ordered in either case would silently
	// clobber newer state, so just drop it.
	if gen != v.sortGen.Load() {
		return
	}

	// v.sorting is cleared here, inside the staleness check, rather than
	// unconditionally like the spinner/label above: if two sorts overlap (a
	// second large first-drop landing before the first one's reorder
	// finishes, say), the earlier, stale one's finishSort must not report
	// "no sort in flight" while the current one is still computing - that
	// would reopen the Escape-quits-mid-reorder bug v.sorting exists to
	// close, just for a narrower window. Only the generation that's still
	// current when it finishes gets to clear it.
	v.sorting = false

	onDone(ordered)
}

// cancelSort aborts a reorder in progress (Escape while v.sorting is true),
// mirroring cancelScan (drop.go) for the analogous scan-gathering phase.
// invalidateSort's context cancellation makes filesort.Order's per-file
// stat/Exif loop notice and stop promptly instead of running to completion
// in the background for a result nobody will see.
//
// Unlike cancelScan, there's nothing to put back: v.files/v.unsortedFiles
// are never touched until a reorder's own onDone callback runs (see
// applyScannedFiles's and SetSortMode's own comments on why the pairing is
// atomic), so cancelling before that lands leaves them exactly as they
// already were - the untouched pre-sort file set, still fully intact and on
// screen, if there was one; nothing, still showing the dropzone, for a
// first-ever drop's cancelled reorder.
func (v *viewer) cancelSort() {
	if !v.sorting {
		return
	}

	v.invalidateSort()

	v.sortSpinner.Hide()
	v.sortLabel.Hide()

	if len(v.files) == 0 {
		v.showWelcomeState()
		v.dropzone.Show()
	}

	v.ForceRepaint()
	v.ShowToast(lang.L("cancelled sorting"))
}

// SortMode reports the current sort order - the settings window's getter.
func (v *viewer) SortMode() filesort.Mode {
	return v.sortMode
}
