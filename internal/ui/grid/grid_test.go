package grid

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/uitest"
)

func TestMain(m *testing.M) {
	// The overlay is built from real widgets, so these need an app for the
	// theme and driver. Each test still gets its own window (see
	// newOverview) - New takes one to maximize on open - but the fyne test
	// driver's windows are not driver.NativeWindow, so winpos.Maximize
	// degrades to a no-op there, the same as every other winpos call this
	// app makes under test.
	test.NewApp()
	os.Exit(m.Run())
}

// fakeHost stands in for the viewer. It records the display actions the
// grid asks for, so a selection or a close can be observed without a real
// window behind it.
type fakeHost struct {
	files []fyne.URI
	index int
	gen   uint64

	// mods is what Modifiers reports - the keyboard state a tap is read
	// against, which a Fyne tap event carries none of. Set around a
	// wrap.Select call to stand in for holding a key while clicking (see
	// the click helper in selection_test.go).
	mods fyne.KeyModifier

	shown     []int
	repaints  int
	unfocused int

	// highlighted records every file index the grid reported for the ring
	// (-1 for "none"), so the window-title notification can be asserted in
	// order without a real title bar.
	highlighted []int
}

func (f *fakeHost) FileCount() int         { return len(f.files) }
func (f *fakeHost) FileAt(i int) fyne.URI  { return f.files[i] }
func (f *fakeHost) CurrentIndex() int      { return f.index }
func (f *fakeHost) Generation() uint64     { return f.gen }
func (f *fakeHost) ShowImage(i int)        { f.shown = append(f.shown, i) }
func (f *fakeHost) ForceRepaint()          { f.repaints++ }
func (f *fakeHost) Unfocus()               { f.unfocused++ }
func (f *fakeHost) HighlightChanged(i int) { f.highlighted = append(f.highlighted, i) }

func (f *fakeHost) Modifiers() fyne.KeyModifier { return f.mods }

// hostWith returns a host holding n small real JPEGs - real files because
// the decode path under test actually reads them.
func hostWith(t *testing.T, names ...string) *fakeHost {
	t.Helper()

	uris := make([]fyne.URI, 0, len(names))
	for _, name := range names {
		uris = append(uris, uitest.TempJPEGURI(t, name, 8, 8, color.White))
	}

	return &fakeHost{files: uris}
}

// newOverview builds an Overview over host and a real (test-driver)
// window, closing the window when the test ends - the fixture behind every
// New call in this file, now that New needs a window to maximize on open.
func newOverview(t *testing.T, host Host) *Overview {
	t.Helper()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return New(host, win)
}

// --- open / close ----------------------------------------------------------

func TestToggle_NoFilesIsNoop(t *testing.T) {
	g := newOverview(t, &fakeHost{})

	g.Toggle()

	if g.Visible() || g.Overlay().Visible() {
		t.Error("the grid should not open with nothing loaded")
	}
}

func TestToggle_OpensAndCloses(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	if !g.Visible() || !g.Overlay().Visible() {
		t.Fatal("the grid should be open after the first toggle")
	}

	g.Toggle()
	if g.Visible() || g.Overlay().Visible() {
		t.Error("the grid should be closed after the second toggle")
	}
}

func TestToggle_StartsHighlightOnCurrentImage(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg")
	host.index = 2
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()

	if g.Highlight() != 2 {
		t.Errorf("Highlight() = %d, want 2 (the image on screen when the grid opened)", g.Highlight())
	}
}

func TestClose_NoopWhenAlreadyClosed(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	g.Close()

	if g.Visible() || g.Overlay().Visible() {
		t.Error("Close should be a no-op when the grid isn't showing")
	}
	if host.unfocused != 0 {
		t.Error("Close on an already-closed grid should not touch focus")
	}
}

// TestClose_UnfocusesCanvas guards the bug where, after clicking a
// thumbnail, arrow-key navigation stopped working until the user clicked
// the image: Fyne's GridWrap grabs canvas focus on a real tap, and this app
// dispatches every key manually from the canvas's *unfocused* handler, so a
// focused GridWrap left behind swallows everything afterwards.
func TestClose_UnfocusesCanvas(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	g.Toggle()
	g.Close()

	if host.unfocused != 1 {
		t.Errorf("Unfocus calls = %d, want 1 - closing must hand the keyboard back", host.unfocused)
	}
}

// --- highlight notification ------------------------------------------------

// last is the most recent index the grid reported for the ring, or -2 when
// it never reported anything - distinct from the -1 that means "nothing
// highlighted".
func (f *fakeHost) last() int {
	if len(f.highlighted) == 0 {
		return -2
	}

	return f.highlighted[len(f.highlighted)-1]
}

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

// TestToggle_KeyboardCursorStartsOnTheCurrentImage: opening the grid puts
// the ring on the image on screen, and the arrow keys have to agree - they
// used to resume from cell 0 no matter where the ring was drawn.
func TestToggle_KeyboardCursorStartsOnTheCurrentImage(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")
	host.index = 2
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	if g.Highlight() != 3 {
		t.Errorf("Highlight() = %d, want 3 - Right should step on from the image the grid opened on", g.Highlight())
	}
}

// TestHandleRune_FilteringResetsTheKeyboardCursorToo: same reset as the
// ring's, since a cursor left past the end of the filtered set would send
// the first arrow key somewhere the user never was.
func TestHandleRune_FilteringResetsTheKeyboardCursorToo(t *testing.T) {
	host := hostWith(t, "moon.jpg", "a.jpg", "b.jpg", "c.jpg")
	host.index = 3
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	typeQuery(g, "moon")
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})

	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want it to stay at 0 - the filtered grid has a single cell", g.Highlight())
	}
}

// TestHandleRune_NoMatchesLeavesTheKeyboardCursorAlone: an empty grid has
// no cell to put a cursor on, and widening the query again must not have
// left one pointing into the set that was filtered away.
func TestHandleRune_NoMatchesLeavesTheKeyboardCursorAlone(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg", "b.jpg", "c.jpg"))
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	typeQuery(g, "zzz")
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 with nothing to highlight", g.Highlight())
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if g.Highlight() != 1 {
		t.Errorf("Highlight() = %d, want 1 - Right from the reset cursor once every cell is back", g.Highlight())
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

// --- search ----------------------------------------------------------------

// openGrid builds an overview over the named files, warms every thumbnail
// (so no cell spawns a background decode, see Warm) and opens it - the
// starting state for every search test below.
func openGrid(t *testing.T, names ...string) (*Overview, *fakeHost) {
	t.Helper()

	host := hostWith(t, names...)
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	return g, host
}

func TestHandleRune_SlashOpensSearch(t *testing.T) {
	g, _ := openGrid(t, "a.jpg")

	g.HandleRune('/')

	if !g.Searching() {
		t.Error("a typed / should open search mode")
	}
	if g.Query() != "" {
		t.Errorf("Query() = %q, want empty - the activating / must not land in the query itself", g.Query())
	}
}

// typeQuery opens search and types q into it, one rune at a time - the way
// the canvas's typed-rune callback delivers them.
func typeQuery(g *Overview, q string) {
	g.HandleRune('/')
	for _, r := range q {
		g.HandleRune(r)
	}
}

func TestHandleRune_QueryFiltersToMatchingNames(t *testing.T) {
	g, _ := openGrid(t, "sunset.jpg", "moon.jpg", "sunrise.jpg")

	typeQuery(g, "sun")

	if g.Query() != "sun" {
		t.Errorf("Query() = %q, want %q", g.Query(), "sun")
	}
	// Length is what GridWrap itself calls to size the grid, so this is the
	// cell count the user actually sees.
	if got := g.wrap.Length(); got != 2 {
		t.Errorf("grid length = %d, want 2 - only sunset.jpg and sunrise.jpg match %q", got, "sun")
	}
}

func TestHandleRune_MatchingIsCaseInsensitive(t *testing.T) {
	g, _ := openGrid(t, "Sunset.JPG", "moon.jpg")

	typeQuery(g, "sUnSeT")

	if got := g.wrap.Length(); got != 1 {
		t.Errorf("grid length = %d, want 1 - matching should ignore case on both sides", got)
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

// TestHandleRune_FilteringResetsHighlightToFirstMatch: the highlight is a
// display index, so a filter that shortens the grid under it would leave it
// pointing past the last cell.
func TestHandleRune_FilteringResetsHighlightToFirstMatch(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg", "moon.jpg")
	host.index = 2
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	if g.Highlight() != 2 {
		t.Fatalf("Highlight() = %d, want it to start on the current image (2)", g.Highlight())
	}

	typeQuery(g, "a")

	if g.Highlight() != 0 {
		t.Errorf("Highlight() = %d, want 0 - only one cell is left to highlight", g.Highlight())
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

func TestClose_ClearsTheSearch(t *testing.T) {
	g, _ := openGrid(t, "sunset.jpg", "moon.jpg")
	typeQuery(g, "sun")

	g.Close()
	g.Toggle()

	if g.Searching() || g.Query() != "" {
		t.Errorf("Searching() = %v, Query() = %q, want a reopened grid to start unfiltered", g.Searching(), g.Query())
	}
	if got := g.wrap.Length(); got != 2 {
		t.Errorf("grid length = %d, want the whole set back", got)
	}
}

func TestHandleRune_NoMatchesShowsAnEmptyGrid(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg")

	typeQuery(g, "zzz")

	if got := g.wrap.Length(); got != 0 {
		t.Errorf("grid length = %d, want 0 - nothing matches %q", got, "zzz")
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

func TestSearchBar_HiddenUntilSearchOpens(t *testing.T) {
	g, _ := openGrid(t, "a.jpg")

	if g.searchBar.Visible() {
		t.Error("the search bar should stay hidden until / opens it")
	}

	g.HandleRune('/')
	if !g.searchBar.Visible() {
		t.Error("/ should show the search bar")
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if g.searchBar.Visible() {
		t.Error("clearing the search should hide the bar again")
	}
}

func TestSearchBar_ShowsQueryAndMatchCount(t *testing.T) {
	g, _ := openGrid(t, "sunset.jpg", "moon.jpg", "sunrise.jpg")

	typeQuery(g, "sun")

	if want := fmt.Sprintf(lang.L("Search: %s"), "sun"); g.searchLabel.Text != want {
		t.Errorf("search label = %q, want %q", g.searchLabel.Text, want)
	}
	if want := fmt.Sprintf(lang.L("%d of %d"), 2, 3); g.countLabel.Text != want {
		t.Errorf("count label = %q, want %q", g.countLabel.Text, want)
	}
}

// TestSearchBar_EmptyNoticeOnlyWhenNothingMatches: an empty grid with no
// explanation reads as a bug, so the one state that draws no cells at all
// says why.
func TestSearchBar_EmptyNoticeOnlyWhenNothingMatches(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg")

	typeQuery(g, "a")
	if g.empty.Visible() {
		t.Error("the empty notice should stay hidden while something still matches")
	}

	g.HandleRune('z')
	if !g.empty.Visible() {
		t.Error("the empty notice should appear once nothing matches")
	}

	g.HandleKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if g.empty.Visible() {
		t.Error("the empty notice should go away again once a match comes back")
	}
}

// --- thumbnails ------------------------------------------------------------

// newCell returns a cell of the shape the grid's own CreateItem builds -
// the image plus its highlight ring - to hand to requestThumbnail
// directly.
func newCell() (*fyne.Container, *canvas.Image) {
	img := canvas.NewImageFromImage(nil)
	ring := canvas.NewRectangle(color.Transparent)

	return container.NewStack(img, ring), img
}

func TestRequestThumbnail_CacheHitAppliesSynchronously(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	g.requestThumbnail(cell, img, 0, host.gen)

	if img.Image == nil {
		t.Error("a cache hit should paint the cell synchronously, without waiting for a goroutine")
	}
}

func TestRequestThumbnail_DecodesInBackgroundAndCaches(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	g.requestThumbnail(cell, img, 0, host.gen)

	// Settle, not a poll of the cache: the cache write and the paint
	// happen in the same completion callback, so waiting on the decode
	// itself is what gives this goroutine a happens-before edge on both.
	g.Settle()

	if !g.Cached(host.files[0]) {
		t.Error("the decoded thumbnail should have been cached")
	}
	if img.Image == nil {
		t.Error("img.Image should be set once the background decode finishes")
	}
}

// TestSetCacheBytes_RetunesTheThumbnailBudget covers the one setter this
// package exposes - the settings window's route to the thumbnail cache, via
// internal/ui's SetMaxThumbCacheMB. An 8x8 JPEG stays 8x8 through scaleToFit
// (already inside ThumbnailSize) and decodes to a 4:2:0 *image.YCbCr, so each
// one weighs well under 200 bytes; a 100-byte budget therefore fits exactly
// one of them and Warm's second file has to evict its first.
func TestSetCacheBytes_RetunesTheThumbnailBudget(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	g.SetCacheBytes(100)

	if err := g.Warm(); err != nil {
		t.Fatalf("Warm() returned error: %v", err)
	}

	if g.thumbs.Len() != 1 {
		t.Errorf("cached thumbnails = %d, want 1 under a 100-byte budget", g.thumbs.Len())
	}
	if g.Cached(host.files[0]) {
		t.Error("the first thumbnail should have been evicted by the second")
	}
	if !g.Cached(host.files[1]) {
		t.Error("the most recently warmed thumbnail should still be cached")
	}

	// Raising the budget doesn't resurrect anything, but it does stop the
	// eviction: warming again now holds both.
	g.SetCacheBytes(imaging.DefaultThumbCacheBytes)

	if err := g.Warm(); err != nil {
		t.Fatalf("Warm() returned error: %v", err)
	}

	if g.thumbs.Len() != 2 {
		t.Errorf("cached thumbnails = %d after raising the budget, want 2", g.thumbs.Len())
	}
}

func TestRequestThumbnail_OutOfRangeIDIsNoop(t *testing.T) {
	host := hostWith(t, "a.jpg")
	g := newOverview(t, host)

	cell, img := newCell()

	g.requestThumbnail(cell, img, 5, host.gen) // only index 0 exists
	g.Settle()

	if img.Image != nil {
		t.Error("an out-of-range id should paint nothing")
	}
}

// TestClaimRelease drives the in-flight bookkeeping directly, the same way
// TestStillWanted drives the staleness predicate: these decisions guard
// against duplicate decode goroutines, which no amount of waiting on real
// ones could assert on.
func TestClaimRelease(t *testing.T) {
	g := newOverview(t, hostWith(t, "a.jpg"))
	cell, _ := newCell()

	if !g.claim(cell, 0) {
		t.Fatal("the first claim for a cell should allow a spawn")
	}
	if g.claim(cell, 0) {
		t.Error("an identical claim while one is in flight must not spawn a second decode")
	}
	if !g.claim(cell, 1) {
		t.Error("a claim for a different id should supersede the old one - the cell scrolled on")
	}

	g.release(cell, 0) // the superseded decode finishing late
	if g.claim(cell, 1) {
		t.Error("a stale release must not drop the newer claim")
	}

	g.release(cell, 1)
	if !g.claim(cell, 1) {
		t.Error("after its own release, a cell should be claimable again")
	}
}

// TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases pins the
// worker's pre-decode bail: a request whose cell is recycled while the
// request waits behind sem must neither paint the cell nor keep its claim.
// The workers are parked by filling sem from the test, so the recycle
// deterministically wins the race against the decode.
func TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	for range thumbConcurrency {
		g.sem <- struct{}{}
	}

	g.requestThumbnail(cell, img, 0, host.gen)
	g.cellIDs.Store(cell, 1) // the cell scrolls on before a worker picks this up

	for range thumbConcurrency {
		<-g.sem
	}
	g.Settle()

	if img.Image != nil {
		t.Error("a decode whose cell scrolled away must not paint it")
	}
	if !g.claim(cell, 0) {
		t.Error("the bailed decode should have released its claim")
	}
}

// TestRequestThumbnail_QueryChangeDiscardsInFlightDecode covers the
// staleness filtering adds on top of the two guards already here: the file
// set and the cell's own id can both still be current while the query
// underneath has renumbered the cells, so display cell 0 means a different
// file than the one this decode was started for. Same parking technique as
// the recycling test above - fill sem so the change deterministically
// beats the decode.
func TestRequestThumbnail_QueryChangeDiscardsInFlightDecode(t *testing.T) {
	host := hostWith(t, "a.jpg", "b.jpg")
	g := newOverview(t, host)

	cell, img := newCell()
	g.cellIDs.Store(cell, 0)

	for range thumbConcurrency {
		g.sem <- struct{}{}
	}

	g.requestThumbnail(cell, img, 0, host.gen)

	// Display cell 0 now means b.jpg; the decode in flight is for a.jpg.
	typeQuery(g, "b")

	for range thumbConcurrency {
		<-g.sem
	}
	g.Settle()

	if img.Image != nil {
		t.Error("a decode started under a different query must not paint a.jpg into a cell now showing b.jpg")
	}
}

func TestStillWanted(t *testing.T) {
	host := hostWith(t, "a.jpg")
	host.gen = 7
	g := newOverview(t, host)

	cell, _ := newCell()
	g.cellIDs.Store(cell, 3)

	fgen := g.filterGen.Load()

	if !g.stillWanted(cell, 3, 7, fgen) {
		t.Error("a decode for the cell's current id at the current generation is still wanted")
	}
	if g.stillWanted(cell, 4, 7, fgen) {
		t.Error("a decode for an id this cell has since been recycled away from is stale")
	}
	if g.stillWanted(cell, 3, 6, fgen) {
		t.Error("a decode from a superseded generation is stale")
	}
	if g.stillWanted(cell, 3, 7, fgen+1) {
		t.Error("a decode resolved under a superseded query is stale")
	}

	other, _ := newCell()
	if g.stillWanted(other, 3, 7, fgen) {
		t.Error("a cell the grid has never tracked is stale")
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
