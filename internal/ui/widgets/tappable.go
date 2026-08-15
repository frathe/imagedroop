package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// TappableArea wraps a CanvasObject to make it respond to taps and hovers,
// without changing anything about how the wrapped object itself is shown,
// laid out, or hidden - callers keep reaching through to the wrapped object
// (e.g. welcomeArt/emptyStateArt) directly for that. Used to make the drop
// zone double as an "open files" button for users who never drag-and-drop.
type TappableArea struct {
	widget.BaseWidget

	content  fyne.CanvasObject
	onTapped func()

	// OnHover, if set, is driven by MouseIn/MouseOut below so a caller can
	// give the tap target a visual cue - Cursor's pointer swap on its own is
	// easy to miss on a target this large, and doesn't exist at all on a
	// touch or trackpad-only setup.
	OnHover func(hovering bool)
}

// NewTappableArea returns content wrapped so taps on it call onTapped.
func NewTappableArea(content fyne.CanvasObject, onTapped func()) *TappableArea {
	t := &TappableArea{content: content, onTapped: onTapped}
	t.ExtendBaseWidget(t)

	return t
}

func (t *TappableArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

func (t *TappableArea) Tapped(*fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *TappableArea) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

// MouseIn, MouseMoved, and MouseOut implement desktop.Hoverable.
func (t *TappableArea) MouseIn(*desktop.MouseEvent) {
	if t.OnHover != nil {
		t.OnHover(true)
	}
}

func (t *TappableArea) MouseMoved(*desktop.MouseEvent) {}

func (t *TappableArea) MouseOut() {
	if t.OnHover != nil {
		t.OnHover(false)
	}
}
