package spiral

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
)

// hoverRect is a desktop.Hoverable canvas.Rectangle that forwards
// MouseIn/MouseMoved to a callback and ignores MouseOut. newMouseTracker
// below is its only user in this file, but the type stays general enough
// for any other invisible hover-catching rectangle this package needs.
type hoverRect struct {
	*canvas.Rectangle
	onMove func(*desktop.MouseEvent)
}

func newHoverRect(c color.Color, onMove func(*desktop.MouseEvent)) *hoverRect {
	return &hoverRect{canvas.NewRectangle(c), onMove}
}

func (h *hoverRect) MouseIn(e *desktop.MouseEvent)    { h.onMove(e) }
func (h *hoverRect) MouseMoved(e *desktop.MouseEvent) { h.onMove(e) }
func (h *hoverRect) MouseOut()                        {}

// newMouseTracker is an invisible CanvasObject whose sole purpose is to
// receive desktop.Hoverable events so st's mouse position stays up to date
// for follow mode and the status overlay. It is sized far larger than any
// real window so it never needs resizing to stay under the cursor.
// onActivity runs on every move; the caller decides what "the user is still
// here" means to it (e.g. the settings panel deciding whether to reveal
// itself) - this file only reports the event, it doesn't act on it.
func newMouseTracker(st *state, onActivity func()) *hoverRect {
	t := newHoverRect(color.Transparent, func(e *desktop.MouseEvent) {
		st.setMouse(float64(e.Position.X), float64(e.Position.Y))
		onActivity()
	})
	t.Resize(fyne.NewSize(1<<20, 1<<20))
	return t
}
