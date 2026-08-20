package help

import (
	"image/color"
	"math"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// span is a half-open rune range [start, end) into a haystack.
type span struct {
	start, end int
}

// findMatches returns non-overlapping, case-insensitive rune ranges of needle
// in haystack. An empty needle matches nothing.
func findMatches(haystack, needle string) []span {
	if needle == "" {
		return nil
	}

	h := []rune(haystack)
	n := []rune(needle)
	if len(n) == 0 || len(n) > len(h) {
		return nil
	}

	var out []span
	for i := 0; i <= len(h)-len(n); {
		if runesEqualFold(h[i:i+len(n)], n) {
			out = append(out, span{i, i + len(n)})
			i += len(n)
			continue
		}
		i++
	}

	return out
}

func runesEqualFold(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}

	return true
}

// searchState remembers the last submitted query so a repeated Enter walks
// to the next match instead of restarting at the first.
type searchState struct {
	query string
	index int
	used  bool
}

// nextIndex returns the match index to show for this Enter. A changed query
// (or the first search) starts at 0; the same query advances and wraps. -1
// when there are no matches.
func (s *searchState) nextIndex(query string, n int) int {
	if n <= 0 {
		s.query = query
		s.index = 0
		s.used = false

		return -1
	}

	if s.used && s.query == query {
		s.index = (s.index + 1) % n
	} else {
		s.query = query
		s.index = 0
		s.used = true
	}

	return s.index
}

func normalizeQuery(q string) string {
	return strings.TrimSpace(q)
}

// Hits stay *widget.TextSegment so Fyne's row layout can baseline-align them
// with the surrounding words. A custom chip (background rectangle) is not a
// *TextSegment, so the renderer treats it as a foreign inline object and
// shifts it down by the line's baseline — the "Wall" gap in the manual.
// Theme warning/error colors are loud against both light and dark body text
// without changing TextStyle (which would trip Fyne's test-theme font set).
func highlightSegments(segs []widget.RichTextSegment, needle string, current int) ([]widget.RichTextSegment, int, *widget.TextSegment) {
	if needle == "" {
		return segs, 0, nil
	}

	var (
		count int
		loc   *widget.TextSegment
	)
	out := replaceSlice(segs, needle, current, &count, &loc)

	return out, count, loc
}

func replaceSlice(segs []widget.RichTextSegment, needle string, current int, count *int, loc **widget.TextSegment) []widget.RichTextSegment {
	out := make([]widget.RichTextSegment, 0, len(segs))
	for _, s := range segs {
		switch t := s.(type) {
		case *widget.TextSegment:
			out = append(out, splitText(t, needle, current, count, loc)...)
		case *widget.ParagraphSegment:
			t.Texts = replaceSlice(t.Texts, needle, current, count, loc)
			out = append(out, t)
		case *widget.ListSegment:
			t.Items = replaceSlice(t.Items, needle, current, count, loc)
			out = append(out, t)
		default:
			out = append(out, t)
		}
	}

	return out
}

func splitText(t *widget.TextSegment, needle string, current int, count *int, loc **widget.TextSegment) []widget.RichTextSegment {
	matches := findMatches(t.Text, needle)
	if len(matches) == 0 {
		return []widget.RichTextSegment{t}
	}

	runes := []rune(t.Text)
	out := make([]widget.RichTextSegment, 0, len(matches)*2+1)
	last := 0

	for _, sp := range matches {
		if sp.start > last {
			pre := *t
			pre.Text = string(runes[last:sp.start])
			out = append(out, &pre)
		}

		hit := *t
		hit.Text = string(runes[sp.start:sp.end])
		if *count == current {
			hit.Style.ColorName = theme.ColorNameError
			*loc = &hit
		} else {
			hit.Style.ColorName = theme.ColorNameWarning
		}
		out = append(out, &hit)

		*count++
		last = sp.end
	}

	if last < len(runes) {
		rest := *t
		rest.Text = string(runes[last:])
		out = append(out, &rest)
	}

	return out
}

func countMatches(segs []widget.RichTextSegment, needle string) int {
	n := 0
	var walk func([]widget.RichTextSegment)
	walk = func(segs []widget.RichTextSegment) {
		for _, s := range segs {
			switch t := s.(type) {
			case *widget.TextSegment:
				n += len(findMatches(t.Text, needle))
			case *widget.ParagraphSegment:
				walk(t.Texts)
			case *widget.ListSegment:
				walk(t.Items)
			}
		}
	}
	walk(segs)

	return n
}

func currentHitY(rt *widget.RichText, needle string) (float32, bool) {
	if rt == nil {
		return 0, false
	}

	r := rt.CreateRenderer()
	r.Layout(rt.Size())
	return findColoredTextY(r.Objects(), needle, theme.Color(theme.ColorNameError))
}

// findColoredTextY returns the top-most Y of a canvas.Text whose color matches
// want. needle, when non-empty, also requires the glyphs to be that string or
// a wrapped prefix of it.
func findColoredTextY(objs []fyne.CanvasObject, needle string, want color.Color) (float32, bool) {
	best := float32(math.MaxFloat32)
	found := false

	var walk func(fyne.CanvasObject, fyne.Position)
	walk = func(o fyne.CanvasObject, origin fyne.Position) {
		pos := fyne.NewPos(origin.X+o.Position().X, origin.Y+o.Position().Y)
		switch t := o.(type) {
		case *canvas.Text:
			if !colorEq(t.Color, want) || t.Text == "" {
				return
			}
			if needle != "" && t.Text != needle && !strings.HasPrefix(needle, t.Text) {
				return
			}
			if pos.Y < best {
				best = pos.Y
				found = true
			}
		case *fyne.Container:
			for _, c := range t.Objects {
				walk(c, pos)
			}
		}
	}

	for _, o := range objs {
		walk(o, fyne.Position{})
	}

	return best, found
}

func colorEq(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
