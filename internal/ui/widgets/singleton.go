package widgets

import (
	"fyne.io/fyne/v2"
)

// Singleton is a secondary window that should exist at most once - the
// manual, the About box, the EXIF panel. Its zero value is a closed window,
// so it can be embedded as a plain field and used without construction.
type Singleton struct {
	win fyne.Window
}

// Show raises the window if it's already open, or builds a fresh one from
// build and shows it.
//
// The new window is resized *before* content is set - a window starts at a
// zero size, and laying a wrapped RichText out at zero width panics in this
// Fyne version (widget/richtext.go's row-bounds computation isn't zero-size
// safe). Escape closes just this window - it must not reset or quit the app
// the way it does in the image window. Closing forgets the window so the
// next call opens a fresh one rather than trying to raise a closed one;
// onClosed, if non-nil, runs then too for any extra teardown.
func (s *Singleton) Show(app fyne.App, title string, size fyne.Size, build func() fyne.CanvasObject, onClosed func()) {
	if s.win != nil {
		s.win.Show()
		s.win.RequestFocus()

		return
	}

	win := app.NewWindow(title)
	win.Resize(size)
	win.SetContent(build())

	win.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			win.Close()
		}
	})

	win.SetOnClosed(func() {
		s.win = nil
		if onClosed != nil {
			onClosed()
		}
	})

	s.win = win

	win.Show()
}

// Window returns the open window, or nil when it's closed - the identity
// callers and tests use to tell "raised the same window" from "opened a
// second one".
func (s *Singleton) Window() fyne.Window {
	return s.win
}

// Open reports whether the window is currently open.
func (s *Singleton) Open() bool {
	return s.win != nil
}
