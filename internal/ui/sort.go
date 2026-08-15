package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/filesort"
)

// toggleSort is the S key: it cycles v.sortMode to the next mode - see
// SetSortMode below, which does the actual work.
func (v *viewer) toggleSort() {
	v.SetSortMode(v.sortMode.Next())
}

// SetSortMode sets the sort order directly - the settings window's binding
// for the cycle above. Re-derives v.files from v.unsortedFiles under the
// new mode, keeping whichever file is currently on screen in view across
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

	v.sortMode = m
	v.files = v.orderedFiles(v.unsortedFiles)
	v.applyTitle()

	v.showFileIfPresent(current)
}

// SortMode reports the current sort order - the settings window's getter.
func (v *viewer) SortMode() filesort.Mode {
	return v.sortMode
}

// orderedFiles returns raw arranged according to the current sort mode -
// the viewer's one-line binding to internal/filesort, which owns the
// orderings themselves.
func (v *viewer) orderedFiles(raw []fyne.URI) []fyne.URI {
	return filesort.Order(v.sortMode, raw)
}
