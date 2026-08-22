package ui

import (
	"errors"
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/uitest"
)

// These drive the app's own composition of the grid's selection with the
// delete confirmation and the clipboard - the glue in batch.go that is the
// only thing in the module knowing both sides exist.

// openGridWith drops the named files, warms every thumbnail and opens the
// grid, leaving the viewer in the state every test below starts from.
func openGridWith(t *testing.T, names ...string) *viewer {
	t.Helper()

	v := newTestViewer(t)

	uris := make([]fyne.URI, 0, len(names))
	for _, name := range names {
		uris = append(uris, uitest.TempJPEGURI(t, name, 4, 4, color.White))
	}
	dropAndWait(t, v, uris...)

	warmThumbs(t, v)
	v.grid.Toggle()
	if !v.grid.Visible() {
		t.Fatal("setup: the grid should be open")
	}

	return v
}

// waitForClipboard waits out the goroutine a clipboard copy runs on -
// v.clipboard is finished once that goroutine has fully run, error toast
// included, so reading widget state afterwards is race-free.
func waitForClipboard(t *testing.T, v *viewer) {
	t.Helper()

	waitFor(t, "the clipboard copy", &v.clipboard)
}

// --- delete ----------------------------------------------------------------

// TestShiftDelete_WhileGridVisiblePromptsForTheSelection replaces the old
// "ignored while the grid is up" behaviour: the shortcut now targets what the
// grid has picked, and the card is raised over the still-open grid.
func TestShiftDelete_WhileGridVisiblePromptsForTheSelection(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")

	v.grid.SelectAll()

	handler := &fyne.ShortcutHandler{}
	wireDeleteShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCut{Secondary: true})

	if !v.deletion.Visible() {
		t.Fatal("Shift+Delete with a selection should open the confirmation")
	}
	if !v.grid.Visible() {
		t.Error("the grid should stay open behind the confirmation card")
	}
}

// TestShiftDelete_WhileGridVisibleFallsBackToTheHighlightedCell: with
// nothing explicitly picked, the keyboard cursor is a selection of one, so
// the shortcut is never a dead key.
func TestShiftDelete_WhileGridVisibleFallsBackToTheHighlightedCell(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg")

	handler := &fyne.ShortcutHandler{}
	wireDeleteShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCut{Secondary: true})

	if !v.deletion.Visible() {
		t.Error("Shift+Delete with nothing picked should still prompt for the highlighted cell")
	}
}

// TestShiftDelete_OutsideTheGridStillPromptsForTheCurrentFile is the
// regression guard on the flow that existed before any of this.
func TestShiftDelete_OutsideTheGridStillPromptsForTheCurrentFile(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	handler := &fyne.ShortcutHandler{}
	wireDeleteShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCut{Secondary: true})

	if !v.deletion.Visible() {
		t.Error("Shift+Delete outside the grid should open the confirmation as it always has")
	}
}

// TestShiftDelete_IgnoredWithNothingLoaded: no files, nothing to confirm.
func TestShiftDelete_IgnoredWithNothingLoaded(t *testing.T) {
	v := newTestViewer(t)

	handler := &fyne.ShortcutHandler{}
	wireDeleteShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCut{Secondary: true})

	if v.deletion.Visible() {
		t.Error("Shift+Delete should do nothing with no files loaded")
	}
}

func TestBatchDelete_RemovesEverySelectedFileAndLeavesTheGridOpen(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")

	var moved []string
	uitest.StubTrashMove(t, func(path string) error {
		moved = append(moved, path)
		return nil
	})

	kept := v.state.files[1].Path()
	v.grid.ClearSelection()
	v.grid.SelectAll()
	// Deselect the middle one, so this is a real subset rather than "all".
	v.grid.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	v.grid.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace})

	v.deleteGridSelection()
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	v.deletion.Settle()

	if len(moved) != 2 {
		t.Errorf("trash.Move calls = %v, want the two selected files", moved)
	}
	if slices.Contains(moved, kept) {
		t.Errorf("the deselected file %q was moved to the Trash", kept)
	}
	if len(v.state.files) != 1 {
		t.Errorf("len(v.state.files) = %d, want 1 left", len(v.state.files))
	}
	if !v.grid.Visible() {
		t.Error("the grid should stay open after a batch delete, so the user keeps their place")
	}
	if v.grid.SelectionCount() != 0 {
		t.Errorf("SelectionCount() = %d after the delete, want the selection cleared", v.grid.SelectionCount())
	}
}

// TestBatchDelete_ClosesTheGridWhenNothingIsLeft: an open grid over an empty
// file set has no cells to draw and no way out but Escape - Toggle itself
// refuses to open in that state, so it must not be left in it either.
func TestBatchDelete_ClosesTheGridWhenNothingIsLeft(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg")
	uitest.StubTrashMove(t, func(string) error { return nil })

	v.grid.SelectAll()
	v.deleteGridSelection()
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	v.deletion.Settle()

	if len(v.state.files) != 0 {
		t.Fatalf("len(v.state.files) = %d, want every file gone", len(v.state.files))
	}
	if v.grid.Visible() {
		t.Error("the grid should close once its last file is deleted")
	}
}

// TestBatchDelete_LeavesTheWindowMaximized: the grid maximizes the window on
// open and undoes that only when something resizes it for a reason of its
// own. A batch delete re-shows whatever now takes the deleted file's place,
// and that load's resize would otherwise shrink the window back to one
// image's size with the grid still filling it. Nothing could reach ShowImage
// with the grid open before this feature, so the guard is new.
func TestBatchDelete_LeavesTheWindowMaximized(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")
	uitest.StubTrashMove(t, func(string) error { return nil })

	v.grid.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace}) // pick the first cell
	v.deleteGridSelection()
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	v.deletion.Settle()
	waitUntilLoaded(t, v)

	if !v.grid.Visible() {
		t.Fatal("setup: the grid should still be open with files left")
	}
	if !v.grid.ConsumeMaximized() {
		t.Error("the batch delete's re-show undid the grid's maximize, shrinking the window under the open grid")
	}
}

// TestBatchDelete_LeavesTheWindowMaximizedOnAColdReload covers the same
// invariant as the test above on the other load path. attemptLoad resizes
// twice - once from the header, as soon as a probe knows the dimensions, and
// again in finishLoad - but the first is skipped entirely on a cache hit, so
// a test whose survivors are all cached exercises only the second. Purging
// the cache first forces the decode, and with it the probe-time resize.
func TestBatchDelete_LeavesTheWindowMaximizedOnAColdReload(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")
	uitest.StubTrashMove(t, func(string) error { return nil })

	v.grid.HandleKey(&fyne.KeyEvent{Name: fyne.KeySpace}) // pick the first cell
	v.deleteGridSelection()
	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	// Emptied after the prompt is up but before it is confirmed, so the
	// re-show that follows has to go back to disk.
	v.imgCache.Purge()

	v.deletion.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	v.deletion.Settle()
	waitUntilLoaded(t, v)

	if !v.grid.Visible() {
		t.Fatal("setup: the grid should still be open with files left")
	}
	if !v.grid.ConsumeMaximized() {
		t.Error("the cold re-show's probe-time resize undid the grid's maximize, shrinking the window under the open grid")
	}
}

// --- copy ------------------------------------------------------------------

func TestCopy_WhileGridVisibleCopiesTheSelectionAsFileReferences(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")

	var got []string
	uitest.StubClipboardCopyFiles(t, func(paths []string) error {
		got = paths
		return nil
	})
	uitest.StubClipboardCopy(t, func([]byte) error {
		t.Error("the grid's copy should put file references on the clipboard, not image data")
		return nil
	})

	v.grid.SelectAll()

	handler := &fyne.ShortcutHandler{}
	wireClipboardShortcuts(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCopy{})
	waitForClipboard(t, v)

	want := []string{v.state.files[0].Path(), v.state.files[1].Path(), v.state.files[2].Path()}
	if !slices.Equal(got, want) {
		t.Errorf("CopyFiles paths = %v, want %v", got, want)
	}
}

func TestCopy_WhileGridVisibleFallsBackToTheHighlightedCell(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg")

	var got []string
	uitest.StubClipboardCopyFiles(t, func(paths []string) error {
		got = paths
		return nil
	})

	v.grid.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	handler := &fyne.ShortcutHandler{}
	wireClipboardShortcuts(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCopy{})
	waitForClipboard(t, v)

	if want := []string{v.state.files[1].Path()}; !slices.Equal(got, want) {
		t.Errorf("CopyFiles paths = %v, want %v", got, want)
	}
}

// TestCopy_OutsideTheGridStillCopiesTheImage is the regression guard: Cmd+C
// on the image view means the pixels, exactly as before.
func TestCopy_OutsideTheGridStillCopiesTheImage(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	copied := false
	uitest.StubClipboardCopy(t, func([]byte) error {
		copied = true
		return nil
	})
	uitest.StubClipboardCopyFiles(t, func([]string) error {
		t.Error("outside the grid Cmd+C should copy image data, not file references")
		return nil
	})

	handler := &fyne.ShortcutHandler{}
	wireClipboardShortcuts(handler, v)
	handler.TypedShortcut(&fyne.ShortcutCopy{})
	waitForClipboard(t, v)

	if !copied {
		t.Error("Cmd+C outside the grid should copy the image")
	}
}

func TestCopy_ReportsAFailedFileCopy(t *testing.T) {
	v := openGridWith(t, "a.jpg")

	uitest.StubClipboardCopyFiles(t, func([]string) error { return errors.New("no clipboard tool") })

	v.grid.SelectAll()
	v.copyGridSelection()
	waitForClipboard(t, v)

	if !v.toast.card.Visible() {
		t.Error("a failed file copy should raise a toast")
	}
}

// --- select all ------------------------------------------------------------

func TestSelectAllShortcut_PicksEveryCellWhileTheGridIsUp(t *testing.T) {
	v := openGridWith(t, "a.jpg", "b.jpg", "c.jpg")

	handler := &fyne.ShortcutHandler{}
	wireSelectAllShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutSelectAll{})

	if want := []int{0, 1, 2}; !slices.Equal(v.grid.Selection(), want) {
		t.Errorf("Selection() = %v, want %v", v.grid.Selection(), want)
	}
}

// TestSelectAllShortcut_IgnoredOutsideTheGrid: there is nothing to select in
// the normal image view, and the shortcut must not quietly build a selection
// that appears the next time the grid opens.
func TestSelectAllShortcut_IgnoredOutsideTheGrid(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	handler := &fyne.ShortcutHandler{}
	wireSelectAllShortcut(handler, v)
	handler.TypedShortcut(&fyne.ShortcutSelectAll{})

	if v.grid.SelectionCount() != 0 {
		t.Errorf("SelectionCount() = %d outside the grid, want 0", v.grid.SelectionCount())
	}
}

// --- RemoveFiles -----------------------------------------------------------

func TestRemoveFiles_DropsEveryIndexAndPurgesTheirCacheEntries(t *testing.T) {
	v := newTestViewer(t)
	uris := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White),
		uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White),
		uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.White),
	}
	dropAndWait(t, v, uris...)

	gone := []string{v.state.files[0].String(), v.state.files[2].String()}
	kept := v.state.files[1].String()

	v.RemoveFiles([]int{0, 2})

	if len(v.state.files) != 1 || v.state.files[0].String() != kept {
		t.Errorf("v.state.files = %v, want only %s left", v.state.files, kept)
	}
	for _, key := range gone {
		if v.imgCache.Contains(key) {
			t.Errorf("RemoveFiles should purge %s from the image cache", key)
		}
	}
}

// TestRemoveFiles_HandlesUnsortedIndices: the grid hands over ascending
// indices today, but removing them in that order would shift each later one
// out from under the loop, so the order must not be load-bearing.
func TestRemoveFiles_HandlesUnsortedIndices(t *testing.T) {
	v := newTestViewer(t)
	uris := []fyne.URI{
		uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White),
		uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White),
		uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.White),
	}
	dropAndWait(t, v, uris...)

	kept := v.state.files[1].String()

	v.RemoveFiles([]int{2, 0})

	if len(v.state.files) != 1 || v.state.files[0].String() != kept {
		t.Errorf("v.state.files = %v, want only %s left", v.state.files, kept)
	}
}

// --- overlay stacking ------------------------------------------------------

// TestOverlayOrder_ConfirmationAndToastSitAboveTheGrid: the grid's backdrop
// is opaque and fills the window, so anything stacked under it while it is
// open is simply invisible. Both the batch confirmation and the toast that
// reports the result have to be raised over it.
func TestOverlayOrder_ConfirmationAndToastSitAboveTheGrid(t *testing.T) {
	v := newTestViewer(t)

	objects := v.win.Content().(*fyne.Container).Objects

	// By subtree rather than by identity: the toast's card is wrapped in a
	// layout container built inline by buildViewer, so what sits in the
	// window's stack is not the card itself.
	indexOf := func(want fyne.CanvasObject) int {
		for i, o := range objects {
			if containsObject(o, want) {
				return i
			}
		}

		return -1
	}

	grid := indexOf(v.grid.Overlay())
	confirm := indexOf(v.deletion.Overlay())
	toast := indexOf(v.toast.card)

	if grid < 0 || confirm < 0 || toast < 0 {
		t.Fatalf("overlay not found in the window content: grid=%d confirm=%d toast=%d", grid, confirm, toast)
	}
	if confirm < grid {
		t.Error("the delete confirmation must stack above the grid, or its card renders behind the grid's backdrop")
	}
	if toast < grid || toast < confirm {
		t.Error("the toast must stack above both, or a batch result is reported where nobody can see it")
	}
}

// containsObject reports whether want is root or anywhere beneath it.
func containsObject(root, want fyne.CanvasObject) bool {
	if root == want {
		return true
	}

	c, ok := root.(*fyne.Container)
	if !ok {
		return false
	}
	for _, o := range c.Objects {
		if containsObject(o, want) {
			return true
		}
	}

	return false
}
