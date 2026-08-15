package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/imagedrop/internal/filesort"
)

// toggleSort is the S key: it cycles v.sortMode to the next mode and
// re-derives v.files from v.unsortedFiles under it, keeping whichever file
// is currently on screen in view across the switch instead of jumping to
// wherever position 0 lands.
func (v *viewer) toggleSort() {
	current := v.files[v.index]

	v.sortMode = v.sortMode.Next()
	v.files = v.orderedFiles(v.unsortedFiles)
	v.applyTitle()

	v.showFileIfPresent(current)
}

// orderedFiles returns raw arranged according to the current sort mode -
// the viewer's one-line binding to internal/filesort, which owns the
// orderings themselves.
func (v *viewer) orderedFiles(raw []fyne.URI) []fyne.URI {
	return filesort.Order(v.sortMode, raw)
}
