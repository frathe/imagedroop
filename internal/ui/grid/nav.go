// Keyboard and pointer navigation of the grid: the highlight ring, its
// relationship to GridWrap's own keyboard cursor, and the key dispatch that
// moves it.

package grid

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

// Highlight is the currently ringed cell.
func (g *Overview) Highlight() int {
	return g.highlight
}

// setHighlight moves the ring to display index id and keeps GridWrap's own
// keyboard cursor on the same cell.
//
// The two are separate positions: GridWrap advances its cursor only for the
// arrow keys it handles itself, so a mouse hover - or the grid opening on
// the file currently on screen - used to move the ring without it. The next
// arrow key then resumed from wherever the keyboard had last been, jumping
// the ring away from the cell the user was pointing at.
//
// wrap.Highlight re-enters OnHighlighted, which returns immediately because
// g.highlight is already set. GridWrap's own TypedKey does call RefreshItem
// on the old and new positions before this runs, but at that point
// g.highlight still holds the *old* value, so those calls redraw both cells
// as "still old" - the two RefreshItem calls here are the ones that actually
// apply the moved ring.
func (g *Overview) setHighlight(id int) {
	old := g.highlight
	g.highlight = id
	// Highlight is a no-op on an empty grid, which would leave the cursor
	// pointing into the set the filter just emptied.
	if g.count() > 0 {
		g.wrap.Highlight(id)
	}
	g.wrap.RefreshItem(old)
	g.wrap.RefreshItem(id)

	// Only while the grid owns the screen: with it closed the title belongs
	// to the image view, and setHighlight still runs from Toggle (which has
	// already set visible) and from a filter change.
	if g.visible {
		if g.count() == 0 {
			g.host.HighlightChanged(-1)
		} else {
			g.host.HighlightChanged(g.fileIndex(id))
		}
	}
}

// HandleKey handles a key press while the grid is up: Escape and G back out
// of it, Space picks the highlighted cell, Return commits it, arrow keys move
// the highlight, and Page Up/Page Down move it by one visible page. Every
// other key is deliberately swallowed by the caller.
//
// While a search is open the letter keys stop meaning anything here, since
// each of them is also arriving at HandleRune as a query character - G
// most visibly, which would otherwise close the grid on its way into the
// query. Space is left out of the search branch for exactly the same reason:
// a space typed into a query must not also toggle a cell.
//
// Escape stages rather than closing outright - see escape.
func (g *Overview) HandleKey(ev *fyne.KeyEvent) {
	if g.searching {
		switch ev.Name {
		case fyne.KeyEscape:
			g.escape()
		case fyne.KeyBackspace:
			g.backspace()
		case fyne.KeyReturn, fyne.KeyEnter:
			g.wrap.Select(g.highlight)
		case fyne.KeyPageUp:
			g.movePage(-1)
		case fyne.KeyPageDown:
			g.movePage(1)
		case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight,
			fyne.KeyHome, fyne.KeyEnd:
			// Listed rather than left to the default branch below: every
			// other key is a character being typed, and must not reach
			// GridWrap at all.
			g.wrap.TypedKey(ev)
		}

		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		g.escape()
	case fyne.KeyG:
		// Inert while a selection is pending, the same way it goes inert
		// while a search is open: closing the grid discards the selection,
		// and a user part-way through assembling one is far more likely to
		// have meant Escape's first stage. Escape is the way out either way.
		if g.sel.Len() == 0 {
			g.Close()
		}
	case fyne.KeySpace:
		g.toggleAt(g.highlight)
	case fyne.KeyReturn, fyne.KeyEnter:
		g.wrap.Select(g.highlight)
	case fyne.KeyPageUp:
		g.movePage(-1)
	case fyne.KeyPageDown:
		g.movePage(1)
	default:
		// GridWrap already knows how to move its own highlight across
		// rows and columns, including the row arithmetic - forward the
		// event rather than reimplementing it here.
		g.wrap.TypedKey(ev)
	}
}

// movePage moves the ring by one rendered grid page, clamped at either end.
// GridWrap handles arrows itself but deliberately has no Page Up/Page Down
// behavior, so keep this movement on the same setHighlight path that keeps
// its keyboard cursor, the ring, scrolling, and the host notification in
// sync. A grid that has not yet been laid out still advances by one row.
// Row count must mirror GridWrap.ColumnCount's own arithmetic (pitch is
// itemMin+padding, not itemMin) or the two disagree on where a row ends,
// and Page Down scrolls a partially visible edge row clean out of view.
func (g *Overview) movePage(direction int) {
	if g.count() == 0 {
		return
	}

	pad := g.wrap.Theme().Size(theme.SizeNamePadding)
	rows := max(1, int((g.wrap.Size().Height+pad)/(cellSize+pad)))
	step := g.wrap.ColumnCount() * rows
	target := g.highlight + direction*step
	target = max(0, min(target, g.count()-1))
	if target != g.highlight {
		g.setHighlight(target)
	}
}

// escape undoes one layer per press, smallest first: the selection, then the
// search, then the grid itself. Each of those took the user effort to build,
// so a single keystroke never throws away more than the one thing they were
// most likely aiming at.
func (g *Overview) escape() {
	switch {
	case g.sel.Len() > 0:
		g.ClearSelection()
	case g.searching:
		g.clearSearch()
	default:
		g.Close()
	}
}

// backspace drops the last character of the query. Rune-wise, not
// byte-wise: the query holds whatever the user typed, and a German file
// name's umlaut would otherwise be cut in half into invalid UTF-8.
func (g *Overview) backspace() {
	if g.query == "" {
		return
	}

	r := []rune(g.query)
	g.query = string(r[:len(r)-1])
	g.applyFilter()
}

// setCellHighlighted shows or hides a cell's highlight ring.
func setCellHighlighted(ring *canvas.Rectangle, highlighted bool) {
	if highlighted {
		ring.Show()
	} else {
		ring.Hide()
	}
}
