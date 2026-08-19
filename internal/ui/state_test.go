package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/filesort"
)

func TestAppStateModelPreferences(t *testing.T) {
	state := newAppState(filesort.ByName, false)

	if got := state.SortMode(); got != filesort.ByName {
		t.Errorf("SortMode() = %v, want ByName", got)
	}
	if state.MergeMode() {
		t.Error("MergeMode() = true, want false")
	}

	state.SetSortMode(filesort.BySize)
	state.SetMergeMode(true)

	if got := state.SortMode(); got != filesort.BySize {
		t.Errorf("SortMode() = %v, want BySize after SetSortMode", got)
	}
	if !state.MergeMode() {
		t.Error("MergeMode() = false, want true after SetMergeMode")
	}
}

func TestAppStateReplaceFilesCopiesAndResetsIndex(t *testing.T) {
	unsorted := []fyne.URI{storage.NewFileURI("/images/b.jpg"), storage.NewFileURI("/images/a.jpg")}
	ordered := []fyne.URI{unsorted[1], unsorted[0]}
	state := appState{index: 1}

	state.replaceFiles(unsorted, ordered)
	unsorted[0] = storage.NewFileURI("/images/changed.jpg")
	ordered[0] = storage.NewFileURI("/images/changed.jpg")

	if state.index != 0 {
		t.Errorf("index = %d, want 0 after replacement", state.index)
	}
	if got, want := state.unsortedFiles[0].Name(), "b.jpg"; got != want {
		t.Errorf("unsortedFiles[0] = %q, want %q", got, want)
	}
	if got, want := state.files[0].Name(), "a.jpg"; got != want {
		t.Errorf("files[0] = %q, want %q", got, want)
	}
}

func TestAppStateRemoveFileRemovesOneMatchingUnsortedDuplicate(t *testing.T) {
	a := storage.NewFileURI("/images/a.jpg")
	b := storage.NewFileURI("/images/b.jpg")
	state := appState{
		files:         []fyne.URI{a, b, a},
		unsortedFiles: []fyne.URI{a, a, b},
		index:         2,
	}

	removed := state.removeFile(2)

	if removed.String() != a.String() {
		t.Errorf("removed = %q, want %q", removed, a)
	}
	if got, want := state.files, []fyne.URI{a, b}; !slices.EqualFunc(got, want, sameURI) {
		t.Errorf("files = %v, want %v", got, want)
	}
	if got, want := state.unsortedFiles, []fyne.URI{a, b}; !slices.EqualFunc(got, want, sameURI) {
		t.Errorf("unsortedFiles = %v, want %v", got, want)
	}
	if state.index != 1 {
		t.Errorf("index = %d, want 1", state.index)
	}
}

func TestAppStateClearFilesResetsFileState(t *testing.T) {
	state := appState{
		files:         []fyne.URI{storage.NewFileURI("/images/a.jpg")},
		unsortedFiles: []fyne.URI{storage.NewFileURI("/images/a.jpg")},
		index:         4,
	}

	state.clearFiles()

	if state.files != nil || state.unsortedFiles != nil {
		t.Errorf("file slices = %v, %v, want nil", state.files, state.unsortedFiles)
	}
	if state.index != 0 {
		t.Errorf("index = %d, want 0", state.index)
	}
}

func sameURI(a, b fyne.URI) bool {
	return a.String() == b.String()
}
