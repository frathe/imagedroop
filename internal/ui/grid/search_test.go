package grid

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

// --- search ----------------------------------------------------------------

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

func TestHandleRune_NoMatchesShowsAnEmptyGrid(t *testing.T) {
	g, _ := openGrid(t, "a.jpg", "b.jpg")

	typeQuery(g, "zzz")

	if got := g.wrap.Length(); got != 0 {
		t.Errorf("grid length = %d, want 0 - nothing matches %q", got, "zzz")
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
