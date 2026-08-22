package help

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// mascotArtSize is the square box each Trane picture occupies in the
// manual: large enough to break up the text, small enough for the 720-wide
// window.
const mascotArtSize = 220

//go:embed TaneWithFrame.webp
var mascotFrameWebP []byte

//go:embed trane_digging.webp
var mascotDiggingWebP []byte

//go:embed trane_wags.webp
var mascotWagsWebP []byte

func mascotResource(name string) (fyne.Resource, bool) {
	var data []byte
	switch name {
	case "TaneWithFrame.webp":
		data = mascotFrameWebP
	case "trane_digging.webp":
		data = mascotDiggingWebP
	case "trane_wags.webp":
		data = mascotWagsWebP
	default:
		return nil, false
	}
	if len(data) == 0 {
		return nil, false
	}

	return fyne.NewStaticResource(name, data), true
}

// bindManualImages rewrites markdown image segments whose filename matches
// an embedded mascot to a resource-backed mascotSegment. Fyne's markdown
// parser only produces file-URI ImageSegments, which a packaged app cannot
// load; GitHub still renders the same files next to the markdown. Called
// after every parse, including the search bar's re-parse.
func bindManualImages(rt *widget.RichText) {
	if rt == nil {
		return
	}

	rt.Segments = replaceManualImages(rt.Segments)
}

func replaceManualImages(segs []widget.RichTextSegment) []widget.RichTextSegment {
	out := make([]widget.RichTextSegment, 0, len(segs))
	for _, s := range segs {
		switch t := s.(type) {
		case *widget.ImageSegment:
			if t.Source != nil {
				if res, ok := mascotResource(t.Source.Name()); ok {
					out = append(out, &mascotSegment{res: res})
					continue
				}
			}
			out = append(out, t)
		case *widget.ParagraphSegment:
			t.Texts = replaceManualImages(t.Texts)
			out = append(out, t)
		case *widget.ListSegment:
			t.Items = replaceManualImages(t.Items)
			out = append(out, t)
		default:
			out = append(out, t)
		}
	}

	return out
}

// mascotSegment is a RichText image backed by an embedded resource rather
// than a file URI, sized to sit between manual sections without dominating
// the 720-wide window. Fyne's own ImageSegment cannot do this.
type mascotSegment struct {
	res fyne.Resource
}

func (m *mascotSegment) Inline() bool { return false }

func (m *mascotSegment) Textual() string { return "" }

func (m *mascotSegment) Visual() fyne.CanvasObject {
	img := canvas.NewImageFromResource(m.res)
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleSmooth
	img.SetMinSize(fyne.NewSize(mascotArtSize, mascotArtSize))

	return img
}

func (m *mascotSegment) Update(o fyne.CanvasObject) {
	img, ok := o.(*canvas.Image)
	if !ok {
		return
	}

	img.Resource = m.res
	img.Refresh()
}

func (m *mascotSegment) Select(fyne.Position, fyne.Position) {}

func (m *mascotSegment) SelectedText() string { return "" }

func (m *mascotSegment) Unselect() {}
