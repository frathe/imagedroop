package help

import (
	"testing"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Fyne's markdown ImageSegment loads a file URI, which a packaged app does
// not ship. These tests pin that the three mascot filenames in the manuals
// are rewritten to the bytes go:embed'd beside this package.

func TestNewManualView_BindsEmbeddedMascots(t *testing.T) {
	v := newManualView("before\n\n![Trane](TaneWithFrame.webp)\n\nafter\n", nil)
	if !hasEmbeddedMascot(v.text.Segments, "TaneWithFrame.webp") {
		t.Fatal("expected TaneWithFrame.webp to render from the embedded resource")
	}
}

func TestNewManualView_RebindsMascotsAfterSearch(t *testing.T) {
	v := newManualView("hello there\n\n![Trane](trane_wags.webp)\n\nunique-tail\n", nil)
	v.submit("unique-tail")
	if !hasEmbeddedMascot(v.text.Segments, "trane_wags.webp") {
		t.Fatal("search re-parse dropped the embedded mascot")
	}
}

func TestBindManualImages_LeavesUnknownFiles(t *testing.T) {
	rt := widget.NewRichTextFromMarkdown("![nope](missing.webp)\n")
	bindManualImages(rt)
	if _, ok := rt.Segments[0].(*widget.ImageSegment); !ok {
		t.Fatalf("unknown image became %T, want *widget.ImageSegment", rt.Segments[0])
	}
}

func hasEmbeddedMascot(segs []widget.RichTextSegment, name string) bool {
	for _, s := range segs {
		img, ok := s.Visual().(*canvas.Image)
		if !ok || img.Resource == nil {
			continue
		}
		if img.Resource.Name() == name {
			return true
		}
	}
	return false
}

func TestMascotsAreEmbedded(t *testing.T) {
	for _, name := range []string{"TaneWithFrame.webp", "trane_digging.webp", "trane_wags.webp"} {
		res, ok := mascotResource(name)
		if !ok || res == nil || len(res.Content()) == 0 {
			t.Errorf("%s was not embedded", name)
		}
	}
}
