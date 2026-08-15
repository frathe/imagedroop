// Drop handling and the recursive folder scan.

package ui

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/imagedrop/internal/imaging"
)

// cancelScan aborts a scan in progress (Escape while v.scanning is true).
// It bumps gen the same way clearToDropzone/ShowImage already do for loads, so
// the background goroutine in handleDrop notices via the gen check in its
// directory-walk loop and stops touching the filesystem instead of racing a
// large tree to completion for a result nobody will see.
//
// Unlike reset, it never touches v.files or v.unsortedFiles: a merge-mode
// scan can be cancelled mid-way through without losing images that were
// already loaded before it started. Only a scan that had nothing loaded yet
// (the first-ever drop) needs the drop zone put back the way handleDrop
// found it.
func (v *viewer) cancelScan() {
	if !v.scanning {
		return
	}

	v.gen.Add(1)
	v.stopAnimation()
	v.scanning = false

	v.scanSpinner.Hide()
	v.scanLabel.Hide()

	if len(v.files) == 0 {
		v.showWelcomeState()
		v.dropzone.Show()
	}

	v.ForceRepaint()
	v.ShowToast(lang.L("cancelled scanning"))
}

// realPathOf resolves u's filesystem path through any symlinks, falling
// back to the URI's own path if that fails (a broken symlink, or a
// filesystem race between the scan and something else touching the same
// path) so callers always have something to key a visited-set on.
func realPathOf(u fyne.URI) string {
	real, err := filepath.EvalSymlinks(u.Path())
	if err != nil {
		return u.Path()
	}
	return real
}

// defaultMaxScannedFiles caps how many images a single recursive folder
// scan will gather (the viewer's maxScan field, which tests shrink
// per-viewer instead of creating hundreds of thousands of temp files to
// exercise the cap). It's a safety valve for pathological trees (a runaway
// symlink cycle EvalSymlinks doesn't resolve to a repeat, or a genuinely
// enormous archive) - past this, stat-ing and holding URIs would stall the
// scan goroutine and bloat v.files well past anything the viewer or its
// sort/preload paths are meant to handle.
const defaultMaxScannedFiles = 200_000

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
	merging := v.mergeMode && len(v.files) > 0

	gen := v.gen.Add(1)
	v.stopAnimation()
	v.scanning = true

	scanDone := make(chan struct{})
	v.scanDone = scanDone

	v.scanLabel.SetText(lang.L("Scanning... 0 images"))
	v.scanSpinner.Show()
	v.scanLabel.Show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()
	v.restoreLink.Hide()
	v.emptyStateArt.Hide()
	v.ForceRepaint()

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
		var images []fyne.URI
		seen := make(map[string]bool)
		for _, u := range uris {
			if !imaging.IsSupportedImage(u) {
				continue
			}
			real := realPathOf(u)
			if seen[real] {
				continue
			}
			seen[real] = true
			images = append(images, u)
		}
		fyne.Do(func() {
			v.applyScanResult(gen, merging, uris, images, false, scanDone)
		})
		return
	}

	go func() {
		var images []fyne.URI
		dirs := make([]fyne.URI, 0, len(uris))
		count := 0
		truncated := false

		// visitedDirs guards against symlink cycles (e.g. a symlink inside a
		// dropped folder pointing back at one of its own ancestors), which
		// would otherwise send this scan into an unbounded loop: each visit
		// resolves the directory to its real, symlink-free path and only
		// descends into a given real path once. A plain map of dropped URIs
		// wouldn't catch this - a cycle keeps producing new, ever-longer
		// URIs (a/link, a/link/link, ...) that all resolve to the same real
		// directory.
		visitedDirs := make(map[string]bool)
		visitDir := func(u fyne.URI) bool {
			real := realPathOf(u)
			if visitedDirs[real] {
				return false
			}
			visitedDirs[real] = true
			return true
		}

		// seenFiles dedupes images within this one scan, keyed the same way
		// as visitedDirs: dropping a folder together with one of its own
		// subfolders, or a symlinked file reachable via two different
		// directory paths, would otherwise add the same picture to v.files
		// twice. This is scoped to a single handleDrop call, not across
		// drops - merge mode has always allowed re-merging a file that's
		// already loaded (see RemoveFile's comment on why it removes by
		// index rather than URI match), and that's left alone here.
		seenFiles := make(map[string]bool)

		process := func(u fyne.URI) {
			if truncated {
				return
			}

			// Checked before IsSupportedImage so directories - which have no
			// extension and would otherwise fall through to MimeType()'s
			// open-and-sniff fallback - are recognized via a cheap stat
			// instead of a wasted file open.
			if canList, err := storage.CanList(u); err == nil && canList {
				if visitDir(u) {
					dirs = append(dirs, u)
				}
				return
			}

			if imaging.IsSupportedImage(u) {
				real := realPathOf(u)
				if seenFiles[real] {
					return
				}
				seenFiles[real] = true

				images = append(images, u)
				count++
				n := count
				if n >= v.maxScan {
					truncated = true
				}
				// update counter periodically to avoid flooding the UI thread
				if n == 1 || n%10 == 0 || truncated {
					fyne.Do(func() {
						if v.gen.Load() != gen {
							return
						}
						v.scanLabel.SetText(fmt.Sprintf(lang.L("Scanning... %d images"), n))
					})
				}
			}
		}

		for _, u := range uris {
			process(u)
		}

		for len(dirs) > 0 && !truncated {
			// A newer drop (or an explicit cancel - see cancelScan) bumped
			// gen out from under this scan: stop walking the tree instead of
			// racing storage.List calls to completion for a result nobody
			// will see. The trailing fyne.Do below re-checks gen and would
			// discard the result anyway; bailing here just stops the wasted
			// I/O sooner. scanDone is still closed directly, skipping
			// fyne.Do, to honor its documented contract of always closing
			// for a stale generation - even though nothing currently waits
			// on this particular (already-overwritten) channel value.
			if v.gen.Load() != gen {
				close(scanDone)
				return
			}
			d := dirs[len(dirs)-1]
			dirs = dirs[:len(dirs)-1]
			children, err := storage.List(d)
			if err != nil {
				continue
			}
			for _, child := range children {
				process(child)
			}
		}

		fyne.Do(func() {
			v.applyScanResult(gen, merging, uris, images, truncated, scanDone)
		})
	}()
}

// applyScanResult is the shared completion step for both of handleDrop's
// paths - the synchronous no-directories fast path and the folder-scan
// goroutine. It must run on the UI goroutine (both callers wrap it in
// fyne.Do) and always closes scanDone, honoring that channel's contract
// even when a newer generation has made this result stale.
func (v *viewer) applyScanResult(gen uint64, merging bool, uris, images []fyne.URI, truncated bool, scanDone chan struct{}) {
	defer close(scanDone)

	if v.gen.Load() != gen {
		return
	}
	v.scanning = false
	v.scanSpinner.Hide()
	v.scanLabel.Hide()

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
		v.ShowToast(fmt.Sprintf(lang.L("stopped scanning after %d images - the dropped folder tree is very large"), v.maxScan))
	}

	// Deliberately last: ShowImage kicks off an async decode chain whose
	// completion work (finishLoad/resizeToImage) runs inline on the decode
	// goroutine under the fyne test driver, so nothing on this goroutine
	// may touch the UI after it starts - the truncation toast above raced
	// exactly that way before this ordering was fixed. Under the real
	// driver the fyne.Do queue serializes both orders identically.
	if merging {
		v.unsortedFiles = append(v.unsortedFiles, images...)
		v.files = v.orderedFiles(v.unsortedFiles)
		if !v.showFileIfPresent(images[0]) {
			v.ShowImage(0)
		}
	} else {
		v.unsortedFiles = images
		v.files = v.orderedFiles(images)
		v.ShowImage(0)
	}
}
