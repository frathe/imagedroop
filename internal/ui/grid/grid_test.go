package grid

import (
	"testing"

	"fyne.io/fyne/v2"
)

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
