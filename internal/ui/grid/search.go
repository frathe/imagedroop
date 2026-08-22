// The filename search ('/'): the query, the display-index-to-host-index
// mapping a filter creates, and the top bar that reports both it and the
// selection.

package grid

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/lang"
)

// clearSearch closes the search bar and restores the unfiltered grid.
func (g *Overview) clearSearch() {
	if !g.searching {
		return
	}

	g.searching = false
	g.query = ""
	g.applyFilter()
}

// Searching reports whether the search bar is up.
func (g *Overview) Searching() bool {
	return g.searching
}

// Query is the filter text typed so far.
func (g *Overview) Query() string {
	return g.query
}

// HandleRune handles a character typed while the grid is up: '/' opens the
// search bar, and every character after that extends the query.
//
// Runes rather than the key names HandleKey sees, because a fyne.KeyEvent
// carries neither case nor the punctuation filenames are full of - there is
// no key name for '_' at all. Taking the canvas's typed-rune callback also
// keeps Fyne's widget focus out of it, so the arrow keys, Return and Escape
// still reach HandleKey exactly as they do with no search open (an
// approach a focused widget.Entry would have taken away).
//
// Search is opened from the rune rather than from HandleKey's KeySlash
// because a key press delivers both callbacks: activating on the key event
// would open the bar and then immediately type the '/' into it.
func (g *Overview) HandleRune(r rune) {
	if !g.searching {
		if r == '/' {
			g.searching = true
			g.applyFilter()
		}

		return
	}

	g.query += string(r)
	g.applyFilter()
}

// count is how many cells the grid shows - the filtered subset while a
// search narrows it, the whole file set otherwise. This is GridWrap's own
// length function.
func (g *Overview) count() int {
	if g.matches != nil {
		return len(g.matches)
	}

	return g.host.FileCount()
}

// fileIndex maps a display index to the host's file index, or -1 when the
// display index addresses no cell. The two numberings differ only while a
// filter is active; the bounds check is the one OnSelected and
// requestThumbnail did against FileCount before filtering existed.
func (g *Overview) fileIndex(id int) int {
	if id < 0 || id >= g.count() {
		return -1
	}
	if g.matches == nil {
		return id
	}

	return g.matches[id]
}

// applyFilter recomputes the visible subset from the current query and
// redraws the grid around it. An empty query - which is what an
// just-opened search bar has - matches everything, so opening search
// changes nothing on screen until a character is typed.
//
// The whole set is rescanned per keystroke rather than narrowed from the
// previous result: Backspace widens the match set again, and a
// strings.Contains over a few thousand names is not worth a cache.
func (g *Overview) applyFilter() {
	g.matches = nil

	if g.searching && g.query != "" {
		needle := strings.ToLower(g.query)
		g.matches = make([]int, 0, g.host.FileCount())

		for i := range g.host.FileCount() {
			if strings.Contains(strings.ToLower(g.host.FileAt(i).Name()), needle) {
				g.matches = append(g.matches, i)
			}
		}
	}

	g.filterGen.Add(1)

	g.wrap.Refresh()
	// The highlight is a display index, so a filter that shortens the grid
	// under it would leave it pointing past the last cell. After the
	// refresh, so GridWrap's cursor is moved against the new length.
	g.setHighlight(0)
	if g.count() > 0 {
		g.wrap.ScrollTo(0)
	}

	g.syncTopBar()
}

// syncTopBar redraws the bar from the current query, match count and
// selection size. The bar earns its space whenever either of the two is
// active, and each half appears on its own: a selection built without ever
// opening the search shows only its count, and vice versa.
func (g *Overview) syncTopBar() {
	if g.searching {
		g.searchLabel.SetText(fmt.Sprintf(lang.L("Search: %s"), g.query))
		g.countLabel.SetText(fmt.Sprintf(lang.L("%d of %d"), g.count(), g.host.FileCount()))
		g.searchLabel.Show()
		g.countLabel.Show()
	} else {
		g.searchLabel.Hide()
		g.countLabel.Hide()
	}

	if n := g.sel.Len(); n > 0 {
		g.selLabel.SetText(fmt.Sprintf(lang.L("%d selected"), n))
		g.selLabel.Show()
	} else {
		g.selLabel.Hide()
	}

	if !g.searching && g.sel.Len() == 0 {
		g.searchBar.Hide()
		g.empty.Hide()

		return
	}

	g.searchBar.Show()

	// Only when the query itself emptied the grid: with no files loaded at
	// all there is no search to be in (Toggle refuses to open), so this
	// can't misfire on an empty set.
	if g.searching && g.count() == 0 {
		g.empty.Show()
	} else {
		g.empty.Hide()
	}
}
