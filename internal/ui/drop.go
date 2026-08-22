// Drop handling: the recursive folder scan itself lives in internal/filescan.

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/filescan"
)

// cancelScan aborts a scan in progress (Escape while v.scanOp.active is true).
// It invalidates the scan's own lifecycle, so the background goroutine in
// handleDrop stops touching the filesystem without interrupting navigation,
// preloading, or animation for an already-loaded merge-mode file set.
//
// Unlike reset, it never touches v.state.files or v.state.unsortedFiles: a merge-mode
// scan can be cancelled mid-way through without losing images that were
// already loaded before it started. Only a scan that had nothing loaded yet
// (the first-ever drop) needs the drop zone put back the way handleDrop
// found it.
func (v *viewer) cancelScan() {
	if !v.scanOp.cancel() {
		return
	}

	if len(v.state.files) == 0 {
		v.showWelcomeState()
		v.dropzone.Show()
	}

	v.ForceRepaint()
	v.ShowToast(lang.L("cancelled scanning"))
}

// MaxScan is the current recursive-folder-scan cap - the settings window's
// getter for SetMaxScan below.
func (v *viewer) MaxScan() int {
	return v.settings.maxScan
}

// SetMaxScan sets the recursive-folder-scan cap directly - the settings
// window's binding. Floored at 1 rather than 0, since a 0 cap would stop a
// scan before it gathered anything at all - not a "no limit" filescan.Images
// is written to understand (it floors again itself, defence in depth rather
// than a replacement for this floor). Applies to the next scan; one already
// in flight keeps running under whatever cap it started with - handleDrop
// snapshots v.settings.maxScan once, before starting the scan, so this
// setter can't retroactively change a scan that's already running.
func (v *viewer) SetMaxScan(n int) {
	if n < 1 {
		n = 1
	}

	v.settings.maxScan = n
}

// handleDrop starts an asynchronous scan for images, recursing into dropped
// folders and updating a spinner + counter while gathering. The first image
// is shown once the scan finishes. A plain drop replaces the current set,
// same as always; with mergeMode on (toggled by M) the newly scanned images
// are merged into it instead, keeping the sort order applied and jumping to
// the first image just added.
func (v *viewer) handleDrop(uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}

	// A drag-and-drop is a separate OS-level event, not gated by the
	// keyboard the way handleKeyEvent's own guard blocks everything else
	// while a delete confirmation is up - so it's still possible to drop
	// new files mid-prompt. Dismiss the prompt rather than let it linger
	// over a file list a replace-mode drop is about to wipe out from under
	// it. Same reasoning for the grid overview: it shows the file set that's
	// about to be replaced (or, in merge mode, about to change), so a drop
	// arriving while it's open closes it back to the normal view instead of
	// leaving it showing stale thumbnails.
	v.deletion.Cancel()
	v.grid.Close()

	// Snapshotted now, not read back inside the completion closure below:
	// a folder scan can take seconds, and toggling M while one is still
	// running shouldn't retroactively change how this already-in-flight
	// drop gets applied.
	merging := v.state.MergeMode() && len(v.state.files) > 0

	v.invalidateLoad()
	token, scanDone := v.scanOp.begin()

	v.scanOp.label.SetText(lang.L("Scanning... 0 images"))
	v.scanOp.show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()
	v.restoreLink.Hide()
	v.emptyStateArt.Hide()
	v.ForceRepaint()

	// Snapshotted once, here, rather than read live from v.settings.maxScan
	// by whichever path runs below: the settings window can write that field
	// while a scan is in flight, and both handleDrop's goroutine and its
	// synchronous fast path need a stable cap for the lifetime of this one
	// scan. See SetMaxScan's doc comment.
	maxScan := v.settings.maxScan

	// Fast path for drops that contain no directories – keep tests synchronous
	// and avoid spawning a goroutine for simple file drops.
	hasDirs := false
	for _, u := range uris {
		if canList, err := storage.CanList(u); err == nil && canList {
			hasDirs = true
			break
		}
	}
	if !hasDirs {
		// nil progress: this path is synchronous and instantaneous, so
		// there's nothing to show, and it avoids calling fyne.Do from the
		// UI goroutine.
		images, truncated := filescan.Images(token.context(), uris, maxScan, nil)
		fyne.Do(func() {
			v.applyScanResult(token, merging, uris, images, truncated, maxScan, scanDone)
		})
		return
	}

	go func() {
		// token.context() is what lets a superseded scan (a newer drop, or
		// an explicit cancel - see cancelScan) stop walking the tree instead
		// of racing storage.List calls to completion for a result nobody
		// will see; the trailing fyne.Do below re-checks the token and would
		// discard the result anyway.
		images, truncated := filescan.Images(token.context(), uris, maxScan, func(n int) {
			fyne.Do(func() {
				if !token.current() {
					return
				}
				v.scanOp.label.SetText(fmt.Sprintf(lang.L("Scanning... %d images"), n))
			})
		})

		fyne.Do(func() {
			v.applyScanResult(token, merging, uris, images, truncated, maxScan, scanDone)
		})
	}()
}

// applyScanResult is the shared completion step for both of handleDrop's
// paths - the synchronous no-directories fast path and the folder-scan
// goroutine. It must run on the UI goroutine (both callers wrap it in
// fyne.Do) and always closes scanDone, honoring that channel's contract
// even when a newer generation has made this result stale. maxScan is the
// cap the scan actually ran under (handleDrop's snapshot), so the
// truncation toast below reports it accurately even if the settings window
// has since changed v.settings.maxScan.
func (v *viewer) applyScanResult(token requestToken, merging bool, uris, images []fyne.URI, truncated bool, maxScan int, scanDone chan struct{}) {
	defer close(scanDone)
	defer token.cancelContext()

	if !token.current() {
		return
	}
	v.scanOp.finish()

	if len(images) == 0 {
		msg := fmt.Sprintf(lang.L("none of the %d dropped files is a supported image"), len(uris))
		if len(uris) == 1 {
			msg = fmt.Sprintf(lang.L("%q is not a supported image file"), uris[0].Name())
		}
		if merging {
			// Nothing to add - leave the existing set exactly as it
			// was instead of wiping it out from under the user.
			v.ShowToast(msg)
		} else {
			v.ShowEmptyStateError(msg)
		}
		return
	}

	v.ForceRepaint()

	// Both a Cmd/Ctrl+O pick and an OS-level drag-and-drop can land while
	// the window itself isn't focused (the file dialog owned focus; a drop
	// from Finder/Explorer never gives it in the first place). Without this
	// the freshly loaded image sits there unresponsive to keyboard input
	// until the user clicks the window once just to focus it.
	v.win.RequestFocus()

	if truncated {
		v.ShowToast(fmt.Sprintf(lang.L("stopped scanning after %d images - the dropped folder tree is very large"), maxScan))
	}

	// Deliberately last: applyScannedFiles hands the reorder to a background
	// goroutine that goes on to call ShowImage, which itself kicks off an
	// async decode chain - and under the fyne test driver both that
	// goroutine and the decode's completion work (finishLoad/resizeToImage)
	// run inline rather than being marshaled onto one UI goroutine, so
	// nothing here may touch the UI once it starts. The truncation toast
	// above raced exactly that way before this ordering was fixed. Under the
	// real driver the fyne.Do queue serializes both orders identically.
	v.applyScannedFiles(merging, images)
}

// applyScannedFiles merges or replaces the file set with images, then
// reorders v.state.unsortedFiles/v.state.files under the current sort mode in the
// background via startSort (sort.go) - same reason SetSortMode does: the
// capture-date/modified/size modes stat or Exif-read every file, which would
// otherwise freeze the UI for as long as this scan just took to gather them,
// right as it finishes.
//
// v.state.unsortedFiles and v.state.files are deliberately only ever written together,
// once the reorder lands - never one without the other. A replacement also
// resets index in that same callback. This
// matters because RemoveFile's own comment documents them as required to
// always hold the same set of files (just possibly different order) so a
// later sort toggle doesn't resurrect a removed file; updating
// v.state.unsortedFiles synchronously here but leaving v.state.files to catch up later
// would violate that invariant for as long as the background reorder is
// still running, and could leave v.state.index pointing past the end of a v.state.files
// a *different*, later-landing reorder (a concurrent SetSortMode call, say)
// has already replaced out from under it. Keeping both deferred to the same
// onDone callback means that can't happen: whichever reorder's generation is
// current when it finishes is the one and only writer of both fields for
// that landing.
func (v *viewer) applyScannedFiles(merging bool, images []fyne.URI) {
	var unsorted []fyne.URI
	if merging {
		// Copied rather than appended onto v.state.unsortedFiles directly - same
		// reason SetSortMode's own snapshot is a copy: this slice is about to
		// be read by a background goroutine, and appending onto
		// v.state.unsortedFiles's existing backing array (when it has spare
		// capacity) would let a concurrent RemoveFile mutate the same memory
		// the goroutine is reading.
		unsorted = append(append([]fyne.URI(nil), v.state.unsortedFiles...), images...)
	} else {
		unsorted = images
	}

	v.startSort(v.state.SortMode(), unsorted, func(ordered []fyne.URI) {
		if !merging {
			v.state.replaceFiles(unsorted, ordered)
		} else {
			v.state.setFiles(unsorted, ordered)
		}
		v.ForceRepaint()

		if merging {
			if !v.showFileIfPresent(images[0]) {
				v.ShowImage(0)
			}
		} else {
			v.ShowImage(0)
		}
	})
}
