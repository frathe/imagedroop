package ui

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/completion"
	"github.com/frathe/picfetch/internal/filescan"
	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/uitest"
)

// This file holds viewer-level invariants over the file set: that files and
// unsortedFiles stay equivalent as multisets across scan/sort/remove
// transitions, that the index stays in range, that sort mode and merge mode
// apply whether they are set before or after files load, that a stale async
// completion cannot overwrite newer state, and that the generation counter
// tracks file-set identity - it changes on a removal, not on navigation -
// which is the contract internal/ui/grid and internal/ui/deletion rely on to
// keep indices meaningful.
//
// internal/ui/state_test.go draws the boundary against this file: it tests
// appState - newAppState, replaceFiles, removeFile, clearFiles - as a plain
// struct with no viewer and no Fyne app. This file tests what the viewer
// must hold true across real transitions.

func TestViewerFileStateSlicesRemainEquivalentAcrossTransitions(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.White)

	dropAndWait(t, v, a, c)
	assertEquivalentFileSlices(t, v)

	v.SetSortMode(filesort.ByDropOrder)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	assertEquivalentFileSlices(t, v)

	v.SetMergeMode(true)
	dropAndWait(t, v, b)
	assertEquivalentFileSlices(t, v)

	v.RemoveFile(v.state.index)
	assertEquivalentFileSlices(t, v)

	v.SetMergeMode(false)
	dropAndWait(t, v, a)
	assertEquivalentFileSlices(t, v)
}

func TestViewerIndexStaysValidAcrossFileStateTransitions(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.White)

	assertValidFileIndex(t, v)

	dropAndWait(t, v, a, c)
	assertValidFileIndex(t, v)

	v.ShowImage(len(v.state.files) - 1)
	waitUntilLoaded(t, v)
	assertValidFileIndex(t, v)

	v.SetSortMode(filesort.ByDropOrder)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	assertValidFileIndex(t, v)

	v.SetMergeMode(true)
	dropAndWait(t, v, b)
	assertValidFileIndex(t, v)

	v.RemoveFile(v.state.index)
	assertValidFileIndex(t, v)

	v.SetMergeMode(false)
	dropAndWait(t, v, a)
	assertValidFileIndex(t, v)

	v.reset()
	assertValidFileIndex(t, v)
}

func TestViewerModesApplyBeforeAndAfterLoadingFiles(t *testing.T) {
	v := newTestViewer(t)

	v.SetSortMode(filesort.ByDropOrder)
	v.SetMergeMode(true)

	if v.SortMode() != filesort.ByDropOrder || !v.MergeMode() {
		t.Fatal("modes set before a drop were not retained")
	}

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	dropAndWait(t, v, b)

	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"2.jpg", "1.jpg"}) {
		t.Errorf("files = %v, want merge mode and drop-order mode applied", got)
	}

	v.SetSortMode(filesort.ByName)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	v.SetMergeMode(false)

	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"1.jpg", "2.jpg"}) {
		t.Errorf("files = %v, want name sort applied after loading", got)
	}
	if v.MergeMode() {
		t.Error("merge mode should be disabled after loading")
	}
}

func TestStaleFileStateCompletionsDoNotOverwriteNewerState(t *testing.T) {
	v := newTestViewer(t)

	current := []fyne.URI{
		uitest.FakeURI{FileName: "current.jpg", Ext: ".jpg"},
	}
	stale := []fyne.URI{
		uitest.FakeURI{FileName: "stale.jpg", Ext: ".jpg"},
	}
	v.state.files = append([]fyne.URI(nil), current...)
	v.state.unsortedFiles = append([]fyne.URI(nil), current...)

	staleScanToken := v.scanOp.lifecycle.begin()
	v.scanOp.lifecycle.begin()
	var scanSignal completion.Signal
	v.applyScanResult(staleScanToken, false, stale, stale, false, filescan.DefaultMax, scanSignal.Begin())
	waitFor(t, "the stale scan completion", &scanSignal)
	assertEquivalentFileSlices(t, v)
	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"current.jpg"}) {
		t.Errorf("files = %v, want newer scan state retained", got)
	}

	staleSortToken := v.sortOp.lifecycle.begin()
	newSortToken := v.sortOp.lifecycle.begin()
	defer newSortToken.cancelContext()
	v.sortOp.active = true
	v.sortOp.spinner.Show()
	v.sortOp.label.Show()
	var sortSignal completion.Signal
	called := false
	v.finishSort(staleSortToken, stale, sortSignal.Begin(), func([]fyne.URI) {
		called = true
	})
	waitFor(t, "the stale sort completion", &sortSignal)

	if called {
		t.Error("stale sort completion should not invoke its state-writing callback")
	}
	if !v.sortOp.active {
		t.Error("stale sort completion should not clear a newer sort's in-flight state")
	}
	if !v.sortOp.spinner.Visible() || !v.sortOp.label.Visible() {
		t.Error("stale sort completion should not hide the newer sort's progress UI")
	}
	assertEquivalentFileSlices(t, v)
	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"current.jpg"}) {
		t.Errorf("files = %v, want newer sort state retained", got)
	}
}

// TestGenerationTracksFileSetIdentityNotNavigation protects the contract used
// by grid and deletion: indices retain their meaning across navigation but not
// across a removal.
func TestGenerationTracksFileSetIdentityNotNavigation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	beforeNavigation := v.Generation()
	v.ShowImage(1)
	waitUntilLoaded(t, v)
	if got := v.Generation(); got != beforeNavigation {
		t.Fatalf("Generation changed from %d to %d on navigation", beforeNavigation, got)
	}

	v.RemoveFile(0)
	if got := v.Generation(); got <= beforeNavigation {
		t.Fatalf("Generation = %d after removal, want greater than %d", got, beforeNavigation)
	}
}
