package grid

import (
	"fmt"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// --- highlight notification ------------------------------------------------

// TestHighlightChanged_ReportsTheFileUnderTheRing covers the whole life of
// the notification the window title is drawn from: which file the grid
// opens on, every move of the ring, and the handover back to the image
// view on close.
func TestHighlightChanged_ReportsTheFileUnderTheRing(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	host.index = 1
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	if got := host.last(); got != 1 {
		t.Fatalf("reported index on open = %d, want 1 (the image already on screen)", got)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if got := host.last(); got != 2 {
		t.Errorf("reported index after Right = %d, want 2", got)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if got := host.last(); got != 1 {
		t.Errorf("reported index after Left = %d, want 1", got)
	}

	g.Close()
	if got := host.last(); got != -1 {
		t.Errorf("reported index after Close = %d, want -1 - the title goes back to the image view", got)
	}
}

// TestHighlightChanged_ReportsTheHostIndexOfAFilteredCell: with a filter
// on, the ring's display index and the file's own index are different
// numbers, and it's the file the title has to name.
func TestHighlightChanged_ReportsTheHostIndexOfAFilteredCell(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	g.HandleRune('/')
	g.HandleRune('c')

	if got := host.last(); got != 2 {
		t.Errorf("reported index for the only match = %d, want 2 (its host index, not display index 0)", got)
	}
}

// TestHighlightChanged_ReportsNoneWhenNothingMatches: an empty grid has no
// cell under the ring, so there is no file name to show either.
func TestHighlightChanged_ReportsNoneWhenNothingMatches(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	g.HandleRune('/')
	g.HandleRune('z')

	if got := host.last(); got != -1 {
		t.Errorf("reported index with no matches = %d, want -1", got)
	}
}

// TestHighlightChanged_SilentWhileClosed: setHighlight also runs from a
// closed grid's reconciliation after a batch delete, and the image view
// owns the title then.
func TestHighlightChanged_SilentWhileClosed(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	g := newOverview(t, host)

	host.files = host.files[:1]
	g.FilesChanged()

	if len(host.highlighted) != 0 {
		t.Errorf("reported %v while the grid was closed, want nothing", host.highlighted)
	}
}

// --- key handling ----------------------------------------------------------

func TestHandleKey_EscapeAndGClose(t *testing.T) {
	for _, key := range []fyne.KeyName{fyne.KeyEscape, fyne.KeyG} {
		t.Run(string(key), func(t *testing.T) {
			g := newOverview(t, hostWith(t, "a.jpg"))
			if err := g.Warm(); err != nil {
				t.Fatalf("Warm: %v", err)
			}
			g.Toggle()

			g.HandleKey(&fyne.KeyEvent{Name: key})

			if g.Visible() {
				t.Errorf("%s should close the grid", key)
			}
		})
	}
}

func TestHandleKey_ArrowMovesHighlight(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg", "c.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	if g.Highlight() != 0 {
		t.Fatalf("Highlight() = %d, want 0 at the start", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if g.Highlight() != 1 {
		t.Errorf("Highlight() = %d, want 1 after Right", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 after Left", g.Highlight())
	}
}

func TestHandleKey_PageMovesHighlightByOneVisiblePage(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("image-%02d.jpg", i)
	}
	g, _ := openGrid(t, names...)
	g.wrap.Resize(fyne.NewSize(cellSize*4, cellSize*3))

	// GridWrap lays out rows and columns at a pitch of itemMin+padding, not
	// itemMin, matching ColumnCount's own arithmetic (see movePage). At the
	// default 4pt padding and a 480x360 wrap: cols = floor((480+4)/124) = 3,
	// rows = floor((360+4)/124) = 2, so one page is 3*2 = 6 cells. This is
	// hardcoded rather than mirroring movePage's formula so the test still
	// catches a regression to the old, inconsistent Height/cellSize row count
	// (which gives 3 rows here, an undetected off-by-one page).
	const step = 6
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyPageUp})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 - Page Up at the first page must stay put", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	if want := step; g.Highlight() != want {
		t.Errorf("Highlight() = %d, want %d after Page Down", g.Highlight(), want)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyPageUp})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 after Page Up", g.Highlight())
	}

	g.setHighlight(len(names) - 2)
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	if want := len(names) - 1; g.Highlight() != want {
		t.Errorf("Highlight() = %d, want %d - Page Down must clamp at the last cell", g.Highlight(), want)
	}
}

func TestHandleKey_PageMovesHighlightWhileSearching(t *testing.T) {
	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("match-%02d.jpg", i)
	}
	g, _ := openGrid(t, names...)
	g.wrap.Resize(fyne.NewSize(cellSize*4, cellSize*3))
	typeQuery(g, "match")

	// Same 480x360 geometry as TestHandleKey_PageMovesHighlightByOneVisiblePage:
	// 3 columns * 2 rows = 6 cells per page.
	const step = 6
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})

	if want := min(step, len(names)-1); g.Highlight() != want {
		t.Errorf("Highlight() = %d, want %d after Page Down in search", g.Highlight(), want)
	}
	if !g.Searching() || g.Query() != "match" {
		t.Errorf("page navigation changed search state: Searching() = %v, Query() = %q", g.Searching(), g.Query())
	}
}

// hover stands in for the pointer entering the cell at display index id.
// Fyne's GridWrap gives its items an onHovered that does exactly this call
// and nothing else, so driving the callback is the whole of a hover as far
// as the grid can observe it - the test driver has no pointer to move.
func hover(g *Overview, id int) {
	g.wrap.OnHighlighted(id)
}

// TestHover_MovesTheRingAndTheKeyboardCursor: the ring and GridWrap's own
// keyboard cursor are separate positions, and a hover only ever moved the
// first - so the next arrow key resumed from wherever the keyboard had last
// been rather than from the cell under the pointer.
func TestHover_MovesTheRingAndTheKeyboardCursor(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	hover(g, 2)
	if g.Highlight() != 2 {
		t.Fatalf("Highlight() = %d, want 2 right after hovering that cell", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if g.Highlight() != 3 {
		t.Errorf("Highlight() = %d, want 3 - Right should step on from the hovered cell", g.Highlight())
	}

	hover(g, 0)
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want it to stay at 0 - Left from the hovered first cell has nowhere to go", g.Highlight())
	}
}

// TestHover_OnTheHighlightedCellIsANoop covers the re-entry guard: moving
// the keyboard cursor fires the same callback a hover does, so an
// unguarded handler would recurse until the stack ran out.
func TestHover_OnTheHighlightedCellIsANoop(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	hover(g, 0)
	hover(g, 0)

	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0", g.Highlight())
	}
}

func TestHandleKey_LeftAtStartIsNoop(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})

	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want it to stay at 0 - there's nothing before the first cell", g.Highlight())
	}
}

func TestHandleKey_ReturnOpensHighlightedAndCloses(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if len(host.shown) != 1 || host.shown[0] != 1 {
		t.Errorf("ShowImage calls = %v, want just the highlighted cell (1)", host.shown)
	}
	if g.Visible() {
		t.Error("committing a cell should close the grid")
	}
}

// TestHandleKey_ReturnOpensHostIndexOfFilteredCell is the mapping this
// whole feature turns on: a filtered grid renumbers its cells from zero,
// but ShowImage takes the app's own file index, so opening the only match
// for "sunr" must show file 2 and not cell 0.
func TestHandleKey_ReturnOpensHostIndexOfFilteredCell(t *testing.T) {
	g, host := openGrid(t, "sunset.jpg", "moon.jpg", "sunrise.jpg")

	typeQuery(g, "sunr")
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if len(host.shown) != 1 || host.shown[0] != 2 {
		t.Errorf("ShowImage calls = %v, want [2] - sunrise.jpg is display cell 0 but host file 2", host.shown)
	}
}

// TestHandleKey_EscapeClearsSearchBeforeClosingTheGrid pins the staging the
// user asked for: Escape means "undo the filter" while one is up, and only
// falls back to its usual "leave the grid" once there is nothing to undo.
func TestHandleKey_EscapeClearsSearchBeforeClosingTheGrid(t *testing.T) {
	g, _ := openGrid(t, "sunset.jpg", "moon.jpg")
	typeQuery(g, "sun")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if !g.Visible() {
		t.Error("the first Escape should clear the search, not close the grid")
	}
	if g.Searching() || g.Query() != "" {
		t.Errorf("Searching() = %v, Query() = %q, want the search gone", g.Searching(), g.Query())
	}
	if got := g.wrap.Length(); got != 2 {
		t.Errorf("grid length = %d, want all 2 files shown again", got)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if g.Visible() {
		t.Error("a second Escape, with no search left to clear, should close the grid")
	}
}

func TestHandleKey_BackspaceShortensTheQuery(t *testing.T) {
	g, _ := openGrid(t, "sunset.jpg", "sunrise.jpg", "moon.jpg")
	typeQuery(g, "sunr")

	if got := g.wrap.Length(); got != 1 {
		t.Fatalf("grid length = %d, want 1 before the backspace", got)
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})

	if g.Query() != "sun" {
		t.Errorf("Query() = %q, want %q", g.Query(), "sun")
	}
	if got := g.wrap.Length(); got != 2 {
		t.Errorf("grid length = %d, want 2 - deleting a character widens the match set again", got)
	}
}

// TestHandleKey_BackspaceDeletesAWholeRune: the app ships a German
// translation and reads whatever files the user drops, so the query holds
// multi-byte characters - and cutting one in half leaves invalid UTF-8 that
// matches nothing.
func TestHandleKey_BackspaceDeletesAWholeRune(t *testing.T) {
	g, _ := openGrid(t, "Grüße.jpg", "moon.jpg")
	typeQuery(g, "grüß")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})

	if g.Query() != "grü" {
		t.Errorf("Query() = %q, want %q - backspace must delete a rune, not a byte", g.Query(), "grü")
	}
	if got := g.wrap.Length(); got != 1 {
		t.Errorf("grid length = %d, want 1 - %q should still match Grüße.jpg", got, g.Query())
	}
}

func TestHandleKey_BackspaceOnEmptyQueryStaysInSearch(t *testing.T) {
	g, _ := openGrid(t, "a.jpg")
	g.HandleRune('/')

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})

	if !g.Searching() {
		t.Error("backspacing an already-empty query should leave search open, not exit it")
	}
	if g.Query() != "" {
		t.Errorf("Query() = %q, want it to stay empty", g.Query())
	}
}

// TestHandleKey_GDoesNotCloseWhileSearching guards the collision the rune
// input creates: a letter key delivers both a rune and a key event, and G
// is the grid's own close shortcut. While searching it has to be a query
// character in one path and nothing at all in the other.
func TestHandleKey_GDoesNotCloseWhileSearching(t *testing.T) {
	g, _ := openGrid(t, "gold.jpg", "moon.jpg")
	typeQuery(g, "g")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyG})

	if !g.Visible() {
		t.Error("G should be a query character while searching, not a close")
	}
	if g.Query() != "g" {
		t.Errorf("Query() = %q, want %q - the key event must not also edit the query", g.Query(), "g")
	}
}

func TestHandleKey_ReturnWithNoMatchesOpensNothing(t *testing.T) {
	g, host := openGrid(t, "a.jpg", "b.jpg")
	typeQuery(g, "zzz")

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if len(host.shown) != 0 {
		t.Errorf("ShowImage calls = %v, want none - there is no match to open", host.shown)
	}
	if !g.Visible() {
		t.Error("a Return with nothing to open should leave the grid up")
	}
}

func TestSetCellHighlighted(t *testing.T) {
	ring := canvas.NewRectangle(color.Transparent)

	setCellHighlighted(ring, true)
	if !ring.Visible() {
		t.Error("highlighting should show the ring")
	}

	setCellHighlighted(ring, false)
	if ring.Visible() {
		t.Error("un-highlighting should hide the ring")
	}
}
