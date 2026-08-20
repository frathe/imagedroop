package help

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestFindMatches_EmptyNeedleReturnsNone(t *testing.T) {
	if got := findMatches("hello", ""); len(got) != 0 {
		t.Errorf("findMatches empty needle = %v, want none", got)
	}
}

func TestFindMatches_CaseInsensitiveNonOverlapping(t *testing.T) {
	got := findMatches("Hello hello HELLO", "heLLo")
	want := []span{{0, 5}, {6, 11}, {12, 17}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findMatches = %v, want %v", got, want)
	}
}

func TestFindMatches_GermanRunes(t *testing.T) {
	got := findMatches("Größe größer", "größe")
	want := []span{{0, 5}, {6, 11}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findMatches umlauts = %v, want %v", got, want)
	}
}

func TestFindMatches_NonOverlappingAAA(t *testing.T) {
	got := findMatches("aaa", "aa")
	want := []span{{0, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findMatches overlapping = %v, want %v", got, want)
	}
}

func TestNextMatchIndex_WrapsAndResetsOnQueryChange(t *testing.T) {
	s := &searchState{}

	if got := s.nextIndex("zoom", 3); got != 0 {
		t.Errorf("first search index = %d, want 0", got)
	}
	if got := s.nextIndex("zoom", 3); got != 1 {
		t.Errorf("second Enter = %d, want 1", got)
	}
	if got := s.nextIndex("zoom", 3); got != 2 {
		t.Errorf("third Enter = %d, want 2", got)
	}
	if got := s.nextIndex("zoom", 3); got != 0 {
		t.Errorf("wrap = %d, want 0", got)
	}
	if got := s.nextIndex("pan", 2); got != 0 {
		t.Errorf("new query = %d, want 0", got)
	}
}

func TestNextMatchIndex_ZeroMatches(t *testing.T) {
	s := &searchState{}
	if got := s.nextIndex("missing", 0); got != -1 {
		t.Errorf("no matches index = %d, want -1", got)
	}
}

func TestHighlightSegments_SplitsAndColorsMatches(t *testing.T) {
	rt := widget.NewRichTextFromMarkdown("hello there hello")
	var count int
	var current *widget.TextSegment
	rt.Segments, count, current = highlightSegments(rt.Segments, "HELLO", 1)
	if count != 2 {
		t.Fatalf("match count = %d, want 2", count)
	}
	if current == nil {
		t.Fatal("current match segment is nil")
	}
	if current.Text != "hello" {
		t.Errorf("current match text = %q, want hello", current.Text)
	}

	highlighted := hitTexts(rt.Segments)
	if want := []string{"hello", "hello"}; !reflect.DeepEqual(highlighted, want) {
		t.Errorf("highlighted texts = %v, want %v", highlighted, want)
	}
}

func TestHighlightSegments_WalksParagraphsAndLists(t *testing.T) {
	rt := widget.NewRichTextFromMarkdown("# zoom\n\n- zoom left\n- pan right")
	var count int
	rt.Segments, count, _ = highlightSegments(rt.Segments, "zoom", 0)
	if count != 2 {
		t.Fatalf("match count = %d, want 2 (heading and list)", count)
	}

	highlighted := hitTexts(rt.Segments)
	if want := []string{"zoom", "zoom"}; !reflect.DeepEqual(highlighted, want) {
		t.Errorf("highlighted texts = %v, want %v", highlighted, want)
	}
}

func TestHighlightSegments_ParseMarkdownClearsHighlights(t *testing.T) {
	rt := widget.NewRichTextFromMarkdown("hello there")
	rt.Segments, _, _ = highlightSegments(rt.Segments, "hello", 0)
	rt.ParseMarkdown("hello there")
	if got := hitTexts(rt.Segments); len(got) != 0 {
		t.Errorf("after ParseMarkdown highlighted = %v, want none", got)
	}
}

func TestHighlightSegments_RenderedObjectsUseHighlightColor(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	rt := widget.NewRichTextFromMarkdown("hello there hello")
	rt.Wrapping = fyne.TextWrapWord
	w := a.NewWindow("")
	w.SetContent(rt)
	w.Resize(fyne.NewSize(400, 200))
	w.Show()

	rt.Segments, _, _ = highlightSegments(rt.Segments, "hello", 0)
	rt.Refresh()

	hits := 0
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch t := o.(type) {
		case *canvas.Text:
			if colorEq(t.Color, theme.Color(theme.ColorNameWarning)) ||
				colorEq(t.Color, theme.Color(theme.ColorNameError)) {
				hits++
			}
		case *fyne.Container:
			for _, c := range t.Objects {
				walk(c)
			}
		}
	}
	for _, o := range test.WidgetRenderer(rt).Objects() {
		walk(o)
	}
	if hits < 2 {
		t.Fatalf("rendered highlight-colored runs = %d, want >= 2", hits)
	}
}

func TestHighlightSegments_MatchStaysOnTheSameRow(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	rt := widget.NewRichTextFromMarkdown("Set as Wallpaper now")
	rt.Wrapping = fyne.TextWrapOff
	w := a.NewWindow("")
	w.SetContent(rt)
	w.Resize(fyne.NewSize(700, 120))
	w.Show()

	rt.Segments, _, _ = highlightSegments(rt.Segments, "Wall", 0)
	rt.Refresh()

	var ys []float32
	for _, o := range test.WidgetRenderer(rt).Objects() {
		ys = append(ys, o.Position().Y)
	}
	if len(ys) < 2 {
		t.Fatalf("want at least two text runs after splitting, got %v", ys)
	}

	base := ys[0]
	for _, y := range ys {
		if d := y - base; d > 2 || d < -2 {
			t.Fatalf("highlighted run left the row: Ys=%v", ys)
		}
	}
}

func hitTexts(segs []widget.RichTextSegment) []string {
	var out []string
	var walk func([]widget.RichTextSegment)
	walk = func(segs []widget.RichTextSegment) {
		for _, s := range segs {
			switch t := s.(type) {
			case *widget.TextSegment:
				if t.Style.ColorName == theme.ColorNameWarning || t.Style.ColorName == theme.ColorNameError {
					out = append(out, t.Text)
				}
			case *widget.ParagraphSegment:
				walk(t.Texts)
			case *widget.ListSegment:
				walk(t.Items)
			}
		}
	}
	walk(segs)
	return out
}
