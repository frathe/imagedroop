// Multi-select: which cells are picked, the gestures that pick them, and
// what the app acts on afterwards.
//
// The set holds *host* file indices, never the display indices the user
// actually clicked - a filter renumbers the cells under a selection, and
// narrowing the grid and widening it again must not silently change what was
// picked. Everything crossing that boundary goes through fileIndex, exactly
// as the search itself does.

package grid

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"github.com/frathe/imagedrop/internal/selection"
)

// Selection is the file indices currently picked, ascending. Empty when
// nothing is - see Targets for what a batch action should actually act on.
func (g *Overview) Selection() []int {
	return g.sel.Indices()
}

// SelectionCount is how many cells are picked.
func (g *Overview) SelectionCount() int {
	return g.sel.Len()
}

// Targets is what a batch action should run against: the selection when
// there is one, and otherwise the highlighted cell alone, so the batch
// shortcuts still mean something with nothing explicitly picked. Empty only
// when the grid has no cells at all.
func (g *Overview) Targets() []int {
	if g.sel.Len() > 0 {
		return g.sel.Indices()
	}

	if i := g.fileIndex(g.highlight); i >= 0 {
		return []int{i}
	}

	return nil
}

// ClearSelection drops the selection, leaving any active filter alone - the
// first stage of Escape (see escape).
func (g *Overview) ClearSelection() {
	if g.sel.Len() == 0 {
		return
	}

	g.sel.Clear()
	g.wrap.Refresh()
	g.syncTopBar()
	g.host.ForceRepaint()
}

// SelectAll picks every cell the grid is currently drawing - the *filtered*
// subset while a search narrows it, which is what makes '/' then Cmd/Ctrl+A
// then a batch action the point of the whole feature.
func (g *Overview) SelectAll() {
	all := make([]int, 0, g.count())
	for d := range g.count() {
		if i := g.fileIndex(d); i >= 0 {
			all = append(all, i)
		}
	}

	g.sel.Replace(all)
	g.wrap.Refresh()
	g.syncTopBar()
	g.host.ForceRepaint()
}

// FilesChanged resyncs the grid with a file set that has shrunk under it -
// what the app calls once a batch delete has actually removed the files.
//
// Everything the grid holds is an index into that set, so all of it has
// moved: the selection is dropped rather than remapped (the files it named
// are exactly the ones that just went to the Trash), and applyFilter
// recomputes the filter's display→host mapping against what is left and
// resets the highlight into range.
func (g *Overview) FilesChanged() {
	g.sel.Clear()
	g.applyFilter()
}

// toggleAt flips whether the cell at display index id is selected, and makes
// it the anchor a later Shift+click extends from.
func (g *Overview) toggleAt(id int) {
	i := g.fileIndex(id)
	if i < 0 {
		return
	}

	g.sel.Toggle(i)
	g.selectionChangedAt(id)
}

// extendTo selects every cell between the anchor and display index id
// inclusive - a Shift+click, or Shift+click again to re-extend, since the
// anchor deliberately doesn't move (Set.Add, not Set.Toggle).
//
// The range is walked in *display* space and each cell mapped back through
// fileIndex, so a range drawn across a filtered grid covers the cells
// actually between the two clicks rather than every file the host happens to
// hold between them.
//
// With no anchor to measure from - a Shift+click as the very first gesture,
// or one whose anchor has since been filtered out of the grid - this falls
// back to picking the clicked cell alone, which also sets the anchor for
// next time. Doing nothing would look like a dead key.
func (g *Overview) extendTo(id int) {
	target := g.fileIndex(id)
	if target < 0 {
		return
	}

	anchorFile, ok := g.sel.Anchor()
	anchorDisplay := -1
	if ok {
		anchorDisplay = g.displayIndex(anchorFile)
	}
	if anchorDisplay < 0 {
		g.sel.Toggle(target)
		g.selectionChangedAt(id)

		return
	}

	for _, d := range selection.Range(anchorDisplay, id) {
		if i := g.fileIndex(d); i >= 0 {
			g.sel.Add(i)
		}
	}

	g.wrap.Refresh()
	g.syncTopBar()
	g.host.ForceRepaint()
}

// displayIndex is fileIndex's inverse: where the host's file i is currently
// drawn, or -1 when the filter isn't drawing it at all. A linear scan of
// matches rather than a maintained reverse map - it runs once per Shift+click,
// against a slice applyFilter already rebuilds from scratch on every
// keystroke.
func (g *Overview) displayIndex(i int) int {
	if g.matches == nil {
		if i < 0 || i >= g.host.FileCount() {
			return -1
		}

		return i
	}

	for d, f := range g.matches {
		if f == i {
			return d
		}
	}

	return -1
}

// selectionChangedAt redraws the one cell whose selection just changed and
// resyncs the count in the top bar. Cheaper than the whole-grid Refresh a
// range extension needs, and the common case - most selections are built one
// cell at a time.
func (g *Overview) selectionChangedAt(id int) {
	g.wrap.RefreshItem(id)
	g.syncTopBar()
	g.host.ForceRepaint()
}

// isSelected reports whether the cell at display index id shows a selected
// file.
func (g *Overview) isSelected(id int) bool {
	i := g.fileIndex(id)

	return i >= 0 && g.sel.Contains(i)
}

// setCellSelected shows or hides a cell's selection tint - the twin of
// setCellHighlighted, and deliberately a separate visual: a cell can be both
// picked and the keyboard's current position at once.
func setCellSelected(tint *canvas.Rectangle, selected bool) {
	if selected {
		tint.Show()
	} else {
		tint.Hide()
	}
}

// pickModifier reports which of the two selection gestures mods names, if
// either: the shortcut modifier (Cmd on macOS, Ctrl elsewhere) toggles one
// cell, Shift extends a range. Holding neither is an ordinary click that
// opens the image; holding both is read as a toggle, since the caller checks
// them in that order - either reading is defensible, and picking one keeps a
// fumbled chord from doing the more destructive of the two (extending a
// range over cells the user never pointed at).
func pickModifier(mods fyne.KeyModifier) (toggle, extend bool) {
	return mods&fyne.KeyModifierShortcutDefault != 0, mods&fyne.KeyModifierShift != 0
}
