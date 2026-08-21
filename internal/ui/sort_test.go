package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/uitest"
)

// This file covers the sort order the app applies to a file set: the
// default natural sort a first drop applies, the S key cycling through
// every filesort.Mode and back to name, the settings window's SetSortMode
// jumping straight to one mode instead of cycling, and cancellation.
//
// Cancelling a sort is not one behavior but two, and conflating them was a
// real bug: cancelling a first-ever drop's reorder has nothing loaded yet
// to lose, but cancelling a resort of files already on screen must leave
// that set and the displayed image alone. v.sorting is what tells the two
// states apart - during a first drop's reorder, v.state.files reads
// exactly like the empty "nothing to reset" state that Escape otherwise
// closes the window on.
//
// Distinct file orders per mode are deliberately not asserted here - see
// internal/ui/sort.go and internal/filesort/filesort_test.go's own
// TestOrderedFiles_SortsByCaptureDate/SortsByModTime/SortsBySize, which
// cover that with controlled inputs.

func TestHandleDrop_NaturalSortsByDefault(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	img2 := uitest.TempJPEGURI(t, "IMG_2.jpg", 4, 4, color.White)

	// Dropped out of numeric order, the way an unsorted OS listing might
	// hand them over.
	dropAndWait(t, v, img10, img1, img2)

	var got []string
	for _, u := range v.state.files {
		got = append(got, u.Name())
	}
	want := []string{"IMG_1.jpg", "IMG_2.jpg", "IMG_10.jpg"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want natural-sorted %v", got, want)
	}
}

// TestToggleSort_CyclesThroughAllModesAndBackToName exercises S cycling
// through every sortMode and back, plus the title-bar label each one shows.
// It deliberately doesn't try to assert a distinct file order for each
// mode - see TestOrderedFiles_SortsByCaptureDate/SortsByModTime/SortsBySize
// for that, with controlled inputs. Instead it leans on all three images
// being pixel-identical (same size/color, no Exif) and created in this
// exact order (img10, then img1, then img2): every non-name mode ties
// across all three files under its own key (capture date falls back to an
// equal mtime; mtime and size are both literally equal), so
// sort.SliceStable's tie-break puts them all back in that same creation
// order - letting one test cover mode cycling, "keep the current file in
// view", and the title label together without a real filesystem race.
func TestToggleSort_CyclesThroughAllModesAndBackToName(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	img2 := uitest.TempJPEGURI(t, "IMG_2.jpg", 4, 4, color.White)

	dropOrder := []fyne.URI{img10, img1, img2}
	v.handleDrop(dropOrder)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	namesOf := func(files []fyne.URI) []string {
		names := make([]string, len(files))
		for i, u := range files {
			names[i] = u.Name()
		}
		return names
	}

	natural := []string{"IMG_1.jpg", "IMG_2.jpg", "IMG_10.jpg"}
	scanOrder := namesOf(dropOrder) // IMG_10.jpg, IMG_1.jpg, IMG_2.jpg

	if got := namesOf(v.state.files); !slices.Equal(got, natural) {
		t.Fatalf("files = %v, want natural-sorted %v before any toggle", got, natural)
	}
	if v.state.SortMode() != filesort.ByName {
		t.Fatalf("sortMode = %v, want filesort.ByName before any toggle", v.state.SortMode())
	}

	// Step onto IMG_2.jpg (index 1 in natural order, i.e. position 2/3)
	// before cycling, so "keep the current file in view" is exercised
	// throughout.
	v.ShowImage(1)
	waitUntilLoaded(t, v)
	if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
		t.Fatalf("displayed file = %q, want IMG_2.jpg before cycling", got)
	}
	if title := v.win.Title(); !strings.Contains(title, "(2/3)") {
		t.Fatalf("title = %q, want it to contain (2/3) before cycling", title)
	}

	// Every mode below resolves to scanOrder (see the comment above), which
	// puts IMG_2.jpg last - position 3/3.
	steps := []struct {
		mode  filesort.Mode
		label string
	}{
		{filesort.ByCaptureDate, "[sort: date]"},
		{filesort.ByModTime, "[sort: modified]"},
		{filesort.BySize, "[sort: size]"},
		{filesort.ByDropOrder, "[unsorted]"},
	}

	for _, step := range steps {
		v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyS})
		waitForSort(t, v)
		waitUntilLoaded(t, v)

		if v.state.SortMode() != step.mode {
			t.Fatalf("sortMode = %v, want %v", v.state.SortMode(), step.mode)
		}
		if got := namesOf(v.state.files); !slices.Equal(got, scanOrder) {
			t.Errorf("[mode %v] files = %v, want %v", step.mode, got, scanOrder)
		}
		if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
			t.Errorf("[mode %v] displayed file = %q, want IMG_2.jpg to stay in view", step.mode, got)
		}

		title := v.win.Title()
		if !strings.HasPrefix(title, step.label+" ") {
			t.Errorf("[mode %v] title = %q, want it prefixed with %q", step.mode, title, step.label)
		}
		if !strings.Contains(title, "(3/3)") {
			t.Errorf("[mode %v] title = %q, want it to contain (3/3)", step.mode, title)
		}
	}

	// One more S wraps back around to natural (by-name) sort.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyS})
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if v.state.SortMode() != filesort.ByName {
		t.Fatalf("sortMode = %v, want filesort.ByName after wrapping around", v.state.SortMode())
	}
	if got := namesOf(v.state.files); !slices.Equal(got, natural) {
		t.Errorf("files = %v, want natural-sorted %v after wrapping around", got, natural)
	}
	if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
		t.Errorf("displayed file = %q, want IMG_2.jpg to stay in view", got)
	}

	title := v.win.Title()
	if strings.Contains(title, "[sort:") || strings.Contains(title, "[unsorted]") {
		t.Errorf("title = %q, want no sort-mode prefix for the default natural sort", title)
	}
	if !strings.Contains(title, "(2/3)") {
		t.Errorf("title = %q, want it to contain (2/3) after wrapping back to natural order", title)
	}
}

// TestSetSortMode_JumpsDirectlyRatherThanCycling is SetSortMode - the
// settings window's binding - as opposed to toggleSort's own S-key cycle
// already covered above: it should reach any mode in one call and still
// keep the current file in view across the switch.
func TestSetSortMode_JumpsDirectlyRatherThanCycling(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	dropAndWait(t, v, img10, img1) // natural sort: IMG_1.jpg, IMG_10.jpg

	current := v.state.files[v.state.index].Name()

	v.SetSortMode(filesort.ByDropOrder)

	if v.state.SortMode() != filesort.ByDropOrder {
		t.Errorf("sortMode = %v, want ByDropOrder straight after one SetSortMode call", v.state.SortMode())
	}

	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if got := v.state.files[v.state.index].Name(); got != current {
		t.Errorf("displayed file = %q, want it to stay on %q across the sort-mode change", got, current)
	}
}

// TestSetSortMode_SafeWithNoFilesLoaded guards the settings window's own
// call site: unlike toggleSort's S key (gated behind handleKeyEvent's
// len(v.state.files)<2 guard), the settings window can change the sort order
// before anything has ever been dropped.
func TestSetSortMode_SafeWithNoFilesLoaded(t *testing.T) {
	v := newTestViewer(t)

	v.SetSortMode(filesort.BySize)

	if v.state.SortMode() != filesort.BySize {
		t.Errorf("sortMode = %v, want BySize", v.state.SortMode())
	}
}

func TestInvalidateSortCancelsAndFinalizesCurrentProgress(t *testing.T) {
	v := newTestViewer(t)

	token := v.sortLifecycle.begin()
	v.sorting = true
	v.sortSpinner.Show()
	v.sortLabel.Show()

	v.invalidateSort()

	if token.current() || token.context().Err() == nil {
		t.Fatal("invalidateSort should cancel and supersede the current sort token")
	}
	if v.sorting || v.sortSpinner.Visible() || v.sortLabel.Visible() {
		t.Fatal("invalidateSort should synchronously finalize the current sort progress UI")
	}
}

// TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles is a -race regression
// test for the snapshot SetSortMode hands to startSort's goroutine: a plain
// slice-header copy of v.state.unsortedFiles aliases its backing array, which
// RemoveFile (a failed-decode retry, a Shift+Delete) then shifts *in place*
// on the UI goroutine while filesort.Order is still copying it - an
// unsynchronized read/write on the same memory. Nothing here asserts: the
// race detector is the assertion, and the test is only meaningful under
// -race.
//
// The file set is built from FakeURIs and assigned directly rather than
// dropped, since what matters is a set large enough for filesort.Order's
// initial copy to still be in flight when the removals start - not that the
// files exist (ByModTime just gets a zero time for each failed stat, in the
// same loop it would otherwise stat a real one).
func TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles(t *testing.T) {
	v := newTestViewer(t)

	const (
		files    = 4000
		removals = 200
	)

	unsorted := make([]fyne.URI, 0, files)
	for i := range files {
		unsorted = append(unsorted, uitest.FakeURI{FileName: fmt.Sprintf("img_%05d.jpg", i), Ext: ".jpg"})
	}

	v.state.files = append([]fyne.URI(nil), unsorted...)
	v.state.unsortedFiles = unsorted

	v.SetSortMode(filesort.ByModTime)

	for range removals {
		v.RemoveFile(0)
	}

	waitForSort(t, v)
}

// TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow guards
// keys.go's Escape branch: a first-ever drop's scan clears v.scanning back
// to false before applyScannedFiles's startSort (drop.go/sort.go) has
// actually populated v.state.files, so for as long as that reorder is still
// computing, v.state.files reads exactly like the "nothing left to reset" state
// Escape otherwise closes the window on. v.sorting is what tells the two
// apart. Drives the in-flight state directly - v.sorting true, v.state.files
// still empty, and sortLifecycle armed - rather than racing a real drop's
// background goroutine to reproduce that window, the same approach
// TestCancelScan_CancelsInFlightScanWithNoFilesYet uses for the gathering
// phase (drop_test.go).
func TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow(t *testing.T) {
	v, _, closed := newTestUI(t)

	v.sortLifecycle.begin()
	v.sorting = true

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if closed() {
		t.Error("Escape should not close the window while a first-drop reorder is still in flight")
	}
	if v.sorting {
		t.Error("Escape's cancelSort should clear v.sorting")
	}
}

// TestHandleKeyEvent_EscapeDuringResortOfExistingFilesDoesNotClearThem is a
// regression test: keys.go's Escape branch used to fall through to a plain
// v.reset() whenever v.sorting was true, which is correct for a first-ever
// drop's cancelled reorder (there's nothing loaded yet to lose) but wrong
// for cancelling a resort of files that were already loaded and on
// screen - v.reset() wipes the whole session, not just the pending sort.
// cancelSort must leave the existing set and the displayed image alone,
// mirroring how cancelScan leaves a merge-mode scan's pre-existing files
// alone.
func TestHandleKeyEvent_EscapeDuringResortOfExistingFilesDoesNotClearThem(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	filesBefore := append([]fyne.URI(nil), v.state.files...)
	indexBefore := v.state.index

	v.sortLifecycle.begin()
	v.sorting = true

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if len(v.state.files) != len(filesBefore) {
		t.Fatalf("files = %v, want unchanged %v after cancelling a resort", v.state.files, filesBefore)
	}
	for i, u := range v.state.files {
		if u.String() != filesBefore[i].String() {
			t.Errorf("files[%d] = %q, want unchanged %q after cancelling a resort", i, u, filesBefore[i])
		}
	}
	if v.state.index != indexBefore {
		t.Errorf("index = %d, want unchanged %d after cancelling a resort", v.state.index, indexBefore)
	}
	if v.img.Image == nil {
		t.Error("the displayed image should not be cleared by cancelling a resort")
	}
	if v.sorting {
		t.Error("Escape's cancelSort should clear v.sorting")
	}
}
