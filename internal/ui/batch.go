// The grid's batch actions: what Shift+Delete and Cmd/Ctrl+C mean while the
// thumbnail overview is up.
//
// This file is the only thing in the module that knows both sides exist.
// internal/ui/grid owns a selection and will tell anyone who asks what is in
// it; internal/ui/deletion moves a set of files to the Trash; neither imports
// the other, and neither knows the grid's Targets are what the confirmation's
// Targets get built from. That is the same cross-feature composition rule the
// grid/slideshow guard follows - features expose state and actions, and this
// package decides how they compose.

package ui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/clipboard"
	"github.com/frathe/picfetch/internal/ui/deletion"
)

// requestDelete is what Shift+Delete runs (see wireDeleteShortcut). It routes
// to the grid's selection while the overview is up and to the file on screen
// otherwise, so one shortcut means the same thing - "get rid of what I'm
// looking at" - in both places.
func (v *viewer) requestDelete() {
	if v.grid.Visible() {
		v.deleteGridSelection()
		return
	}

	v.deletion.Request()
}

// deleteGridSelection opens the confirmation card for whatever the grid has
// picked - or, with nothing explicitly picked, the highlighted cell alone
// (grid.Targets). The card is raised over the grid rather than closing it:
// the whole point of a batch is working through a large set, and closing the
// overview after every one would throw away the user's place in it.
func (v *viewer) deleteGridSelection() {
	targets := v.grid.Targets()
	if len(targets) == 0 {
		return
	}

	ts := make([]deletion.Target, 0, len(targets))
	for _, i := range targets {
		if i < 0 || i >= len(v.files) {
			continue
		}
		ts = append(ts, deletion.Target{URI: v.files[i], Index: i})
	}

	v.deletion.RequestFiles(ts)
}

// copySelection is what Cmd/Ctrl+C runs (see wireClipboardShortcuts): the
// grid's selection as file references while the overview is up, and the
// displayed frame as image data otherwise. Two different things on one
// shortcut, because they are the same intent applied to two different
// subjects - a dozen selected images cannot meaningfully become one clipboard
// image, and a single image being viewed is more useful as pixels than as a
// path.
func (v *viewer) copySelection() {
	if v.grid.Visible() {
		v.copyGridSelection()
		return
	}

	v.copyImageToClipboard()
}

// copyGridSelection puts the selected files on the clipboard as file
// references, so a paste in Finder/Explorer/a Linux file manager creates
// copies of the files themselves.
//
// Runs on its own goroutine and reports through clipboardDone for the same
// reasons copyImageToClipboard does: every backing command blocks on external
// I/O, and a test needs one thing to wait on rather than polling widgets the
// goroutine may still be writing.
func (v *viewer) copyGridSelection() {
	targets := v.grid.Targets()

	paths := make([]string, 0, len(targets))
	for _, i := range targets {
		if i >= 0 && i < len(v.files) {
			paths = append(paths, v.files[i].Path())
		}
	}
	if len(paths) == 0 {
		return
	}

	done := make(chan struct{})
	v.clipboardDone = done

	go func() {
		defer close(done)

		if err := clipboard.CopyFiles(paths); err != nil {
			v.reportFileCopyError(err)

			return
		}

		fyne.Do(func() {
			if len(paths) == 1 {
				v.ShowToast(lang.L("copied 1 file"))
				return
			}

			v.ShowToast(fmt.Sprintf(lang.L("copied %d files"), len(paths)))
		})
	}()
}

// reportFileCopyError logs a failed file-reference copy and shows it as a
// toast, the same way reportClipboardError does for image data and for the
// same reason: this either worked or genuinely failed, on every OS, with none
// of the cancel-vs-failure ambiguity that keeps reportChooserError quiet on
// Linux.
func (v *viewer) reportFileCopyError(err error) {
	detail := chooserErrorDetail(err)
	fyne.LogError("clipboard file copy failed", errors.New(detail))

	fyne.Do(func() {
		v.ShowToast(fmt.Sprintf(lang.L("could not copy the files: %v"), detail))
	})
}

// selectAllInGrid is Cmd/Ctrl+A, and does nothing outside the grid: there is
// nothing to select in the normal image view, and quietly building a
// selection there would make it appear out of nowhere the next time the
// overview opened.
func (v *viewer) selectAllInGrid() {
	if !v.grid.Visible() {
		return
	}

	v.grid.SelectAll()
}
