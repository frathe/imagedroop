package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/filesort"
)

// toggleSort is the S key: it cycles the state sort mode to the next mode - see
// SetSortMode below, which does the actual work.
func (v *viewer) toggleSort() {
	v.SetSortMode(v.state.SortMode().Next())
}

// SetSortMode sets the sort order directly - the settings window's binding
// for the cycle above. Re-derives v.state.files from v.state.unsortedFiles under the
// new mode in the background (see filesort.Order's own doc comment: the
// capture-date/modified/size modes each stat or Exif-read every file, which
// visibly pauses a large recursive folder scan if done inline on the UI
// goroutine), keeping whichever file is currently on screen in view across
// the switch instead of jumping to wherever position 0 lands. Safe to call
// before any files are ever loaded, unlike toggleSort's own S-key call
// site, which is gated behind handleKeyEvent's len(v.state.files)<2 guard.
func (v *viewer) SetSortMode(m filesort.Mode) {
	if len(v.state.files) == 0 {
		v.state.SetSortMode(m)
		v.applyTitle()

		return
	}

	current := v.state.files[v.state.index]

	// Defensively copied rather than aliased: v.state.unsortedFiles's backing
	// array can be mutated in place by RemoveFile (a failed-decode retry
	// dropping a file, or a Shift+Delete) while this snapshot is still
	// being read by startSort's background goroutine - a concurrent
	// read/write on the same backing array that filesort.Order's own copy
	// of its argument doesn't protect against, since that copy only happens
	// after this handoff.
	unsorted := append([]fyne.URI(nil), v.state.unsortedFiles...)

	// The title's sort-mode prefix updates immediately, even before the
	// reorder itself finishes - there's no reason to make the user wait for
	// a large sort just to see that their choice registered.
	v.state.SetSortMode(m)
	v.applyTitle()

	v.startSort(m, unsorted, func(ordered []fyne.URI) {
		v.state.files = ordered
		v.ForceRepaint()
		v.showFileIfPresent(current)
	})
}

// invalidateSort advances sortOp.lifecycle and, if a reorder is currently in
// flight, cancels its context so filesort.Order's per-file stat/Exif loop
// notices and stops promptly instead of running to completion for a result
// that's already guaranteed to be discarded - see sortOp's field comment for
// every caller (a newer sort superseding an older one, Escape via
// cancelSort, RemoveFile, clearToDropzone). It returns the new revision for
// tests and diagnostics.
func (v *viewer) invalidateSort() uint64 {
	return v.sortOp.invalidate()
}

// startSort reorders unsorted under mode in the background, showing the sort
// spinner/label while it computes - shared by SetSortMode and
// applyScannedFiles (drop.go), the two places that call filesort.Order over a
// potentially large file set: its capture-date/modified/size modes stat or
// Exif-read every file, which freezes the UI for as long as that takes if
// done inline on the UI goroutine (see filesort.Order's own doc comment).
// Any sort already in flight is cancelled by sortOp.begin, rather
// than left to keep computing a result this call already supersedes - so
// pressing S repeatedly cycles straight through modes instead of queuing up
// wasted background work behind whichever one happened to be slowest.
// onDone runs once, and only if this call's token is still current once
// the reorder finishes - see sortOp's field comment for every way it can be
// superseded.
func (v *viewer) startSort(mode filesort.Mode, unsorted []fyne.URI, onDone func(ordered []fyne.URI)) {
	token, sortDone := v.sortOp.begin()

	v.sortOp.show()
	// A widget hidden since construction has never been painted, so it has
	// no canvas of its own to mark dirty on Show/Refresh - see
	// ForceRepaint's own doc comment.
	v.ForceRepaint()

	go func() {
		ordered := filesort.Order(token.context(), mode, unsorted)
		fyne.Do(func() {
			v.finishSort(token, ordered, sortDone, onDone)
		})
	}()
}

// finishSort is startSort's completion step, shaped like drop.go's
// applyScanResult: it must run on the UI goroutine (startSort's goroutine
// wraps it in fyne.Do), always closes sortDone (honoring that channel's
// contract even when a newer request has made this result stale), and always
// releases this invocation's own token context.
func (v *viewer) finishSort(token requestToken, ordered []fyne.URI, sortDone chan struct{}, onDone func([]fyne.URI)) {
	defer close(sortDone)
	defer token.cancelContext()

	// Superseded either by a newer sort or by something else that changed
	// v.state.files/v.state.unsortedFiles while this one was still computing
	// (Shift+Delete, or Escape/File>Close - see those call sites' own
	// invalidateSort call). Applying ordered in either case would silently
	// clobber newer state, so just drop it.
	if !token.current() {
		return
	}

	// v.sortOp.active and the progress widgets are finalized here, inside
	// the staleness check: if two sorts overlap (a
	// second large first-drop landing before the first one's reorder
	// finishes, say), the earlier, stale one's finishSort must not report
	// "no sort in flight" while the current one is still computing - that
	// would reopen the Escape-quits-mid-reorder bug v.sortOp.active exists
	// to close, just for a narrower window. Only the token that's still
	// current when it finishes gets to clear it.
	v.sortOp.finish()
	v.fileSetRevision.advance()

	onDone(ordered)
}

// cancelSort aborts a reorder in progress (Escape while v.sortOp.active is
// true), mirroring cancelScan (drop.go) for the analogous scan-gathering
// phase. invalidateSort's context cancellation makes filesort.Order's
// per-file stat/Exif loop notice and stop promptly instead of running to
// completion in the background for a result nobody will see.
//
// Unlike cancelScan, there's nothing to put back: v.state.files/v.state.unsortedFiles
// are never touched until a reorder's own onDone callback runs (see
// applyScannedFiles's and SetSortMode's own comments on why the pairing is
// atomic), so cancelling before that lands leaves them exactly as they
// already were - the untouched pre-sort file set, still fully intact and on
// screen, if there was one; nothing, still showing the dropzone, for a
// first-ever drop's cancelled reorder.
func (v *viewer) cancelSort() {
	if !v.sortOp.cancel() {
		return
	}

	if len(v.state.files) == 0 {
		v.showWelcomeState()
		v.dropzone.Show()
	}

	v.ForceRepaint()
	v.ShowToast(lang.L("cancelled sorting"))
}

// SortMode reports the current sort order - the settings window's getter.
func (v *viewer) SortMode() filesort.Mode {
	return v.state.SortMode()
}
