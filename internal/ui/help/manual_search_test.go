package help

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

const searchFixture = "alpha one\n\nbeta two\n\nalpha three"

func TestNewManualView_SearchBarPlaceholder(t *testing.T) {
	v := newManualView(searchFixture, nil)
	if v.entry.PlaceHolder != "Search for..." {
		t.Errorf("placeholder = %q, want %q", v.entry.PlaceHolder, "Search for...")
	}
}

func TestNewManualView_EnterHighlightsAndAdvances(t *testing.T) {
	v := newManualView(searchFixture, nil)

	v.entry.SetText("alpha")
	v.submit(v.entry.Text)

	if got := hitTexts(v.text.Segments); len(got) != 2 {
		t.Fatalf("first Enter highlights = %v, want two alphas", got)
	}
	if v.current == nil || v.current.Text != "alpha" {
		t.Fatalf("first current = %+v, want alpha", v.current)
	}
	first := v.current

	v.submit(v.entry.Text)
	if v.current == nil || v.current == first {
		t.Fatal("second Enter should move to a different current match")
	}
	if v.current.Text != "alpha" {
		t.Errorf("second current text = %q, want alpha", v.current.Text)
	}
}

func TestNewManualView_EmptyQueryClearsHighlights(t *testing.T) {
	v := newManualView(searchFixture, nil)
	v.entry.SetText("alpha")
	v.submit(v.entry.Text)

	v.entry.SetText("  ")
	v.submit(v.entry.Text)

	if got := hitTexts(v.text.Segments); len(got) != 0 {
		t.Errorf("empty query left highlights %v", got)
	}
}

func TestNewManualView_ChangedQueryRestartsAtFirst(t *testing.T) {
	v := newManualView(searchFixture, nil)
	v.entry.SetText("alpha")
	v.submit(v.entry.Text)
	v.submit(v.entry.Text)

	v.entry.SetText("beta")
	v.submit(v.entry.Text)

	if got := hitTexts(v.text.Segments); len(got) != 1 || got[0] != "beta" {
		t.Errorf("changed query highlights = %v, want [beta]", got)
	}
}

func TestNewManualView_ScrollPutsCurrentMatchInViewport(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	body := strings.Repeat("padding paragraph goes here\n\n", 50) + "unique-tail"
	v := newManualView(body, nil)
	w := a.NewWindow("")
	w.SetContent(v.content())
	w.Resize(fyne.NewSize(manualW, 280))
	w.Show()

	v.entry.SetText("unique-tail")
	v.submit(v.entry.Text)

	matchY, ok := currentHitY(v.text, "unique-tail")
	if !ok {
		t.Fatal("could not find the current (error-colored) hit on screen")
	}

	viewH := v.scroll.Size().Height
	top := v.scroll.Offset.Y
	bot := top + viewH
	if matchY < top || matchY > bot {
		t.Errorf("current hit Y=%.1f is outside the viewport [%.1f, %.1f] (offset=%.1f)", matchY, top, bot, v.scroll.Offset.Y)
	}
}

func TestNewManualView_ScrollMovesOnLaterMatch(t *testing.T) {
	body := "first-hit\n\n" + strings.Repeat("padding paragraph\n\n", 40) + "second-hit"
	v := newManualView(body, nil)
	v.scroll.Resize(fyne.NewSize(manualW, 200))
	v.text.Resize(v.text.MinSize())

	v.entry.SetText("second-hit")
	v.submit(v.entry.Text)

	if v.scroll.Offset.Y == 0 {
		t.Error("expected scroll offset to move for a match near the end")
	}
}

func TestShowManual_OpensSearchBarAndFocusesIt(t *testing.T) {
	h := New(test.NewApp(), "PicFetch", nil)
	orig := currentManual
	t.Cleanup(func() { currentManual = orig })
	currentManual = func() string { return "# Head\n\nplain text" }

	h.ShowManual()

	win := h.manualWin.Window()
	if win == nil {
		t.Fatal("ShowManual did not open a window")
	}

	if h.manual == nil || h.manual.entry == nil {
		t.Fatal("manual view has no search entry")
	}

	if win.Canvas().Focused() != h.manual.entry {
		t.Errorf("focused = %T, want the search entry", win.Canvas().Focused())
	}

	h.ShowManual()
	if h.manualWin.Window() != win {
		t.Error("a second ShowManual call should raise the existing window")
	}

	win.Close()
}

func TestShowManual_DoesNotUseFullManualInThisTest(t *testing.T) {
	// Guard: the fixture must stay free of bold-inside-code, which panics
	// Fyne's test theme when the real manual.md is rendered.
	h := New(test.NewApp(), "PicFetch", nil)
	orig := currentManual
	t.Cleanup(func() { currentManual = orig })
	currentManual = func() string { return "just words" }

	h.ShowManual()
	defer h.manualWin.Window().Close()

	if _, ok := h.manual.text.Segments[0].(*widget.TextSegment); !ok {
		if _, ok := h.manual.text.Segments[0].(*widget.ParagraphSegment); !ok {
			t.Fatalf("unexpected first segment %T", h.manual.text.Segments[0])
		}
	}
}
