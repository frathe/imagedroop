package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/layout"
)

// sizeTracker lays out a window's root content exactly like
// layout.NewStackLayout() while also recording that window's current full
// size into into on every layout pass. This is the only way to get it back:
// fyne.Window has no size getter, and SetOnClosed is a single callback slot
// already spoken for elsewhere.
//
// Deliberately reads win.Canvas().Size() rather than using the size this
// Layout call was handed: that size is the root content's own laid-out size,
// which sits inside the window's padding and so is consistently smaller than
// the window size Resize/Canvas().Size() deal in - recording it directly
// would shrink the persisted geometry by that padding on every launch/save
// cycle.
type sizeTracker struct {
	win  fyne.Window
	into *fyne.Size
}

// NewSizeTracker returns a layout that keeps *into current with win's size.
// Wrap a window's root content in it (container.New) to make that size
// available at shutdown - see internal/preferences, and Singleton.Remember
// for the secondary-window case.
func NewSizeTracker(win fyne.Window, into *fyne.Size) fyne.Layout {
	return sizeTracker{win: win, into: into}
}

func (t sizeTracker) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return layout.NewStackLayout().MinSize(objects)
}

func (t sizeTracker) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	*t.into = t.win.Canvas().Size()
	layout.NewStackLayout().Layout(objects, size)
}
