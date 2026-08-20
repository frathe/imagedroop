// Package help is the app's documentation UI: the embedded end-user
// manual, the About box, and the Help menu that opens either.
//
// It's the one feature package that needs nothing from the viewer - no
// host interface, no callbacks back into the app. Everything it draws
// comes from its constructor arguments (the app, for windows and metadata;
// the app title; the artwork), which is why it was the first extraction of
// the per-feature split. It also dissolves a mutual dependency that used
// to exist between the About window and the manual window, since the About
// box links to the manual: both are methods on one type here.
package help

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/ui/widgets"
)

// Help owns the two documentation windows. Each is a widgets.Singleton, so
// a second request raises the window that's already open instead of
// stacking up duplicates.
type Help struct {
	app   fyne.App
	title string

	// art is the welcome image the About box shows beside the app name -
	// passed in rather than imported so this package doesn't depend on
	// where the app keeps its assets.
	art []byte

	manualWin widgets.Singleton
	aboutWin  widgets.Singleton
	manual    *manualView
}

// New returns the help UI for application, showing title as the app's name
// and art as the About box's illustration.
func New(application fyne.App, title string, art []byte) *Help {
	return &Help{app: application, title: title, art: art}
}

// Menu is the app's Help menu: the manual, and an About screen below a
// separator (the usual place for it in a Help menu). Returns the *fyne.Menu
// itself rather than a whole *fyne.MainMenu, so internal/ui can combine it
// with its own File menu into one bar - composing menus is the app's job,
// not this package's, the same "internal/ui decides how features compose"
// rule the grid/slideshow full-window-mode guard follows (see
// ARCHITECTURE.md).
func (h *Help) Menu() *fyne.Menu {
	manual := fyne.NewMenuItem(lang.L("Manual"), h.ShowManual)
	// Display-only: F1 itself is handleKeyEvent in internal/ui. This is the
	// same menu-hint pattern File uses for Open/Save/Export.
	manual.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyF1}
	about := fyne.NewMenuItem(lang.L("About"), h.ShowAbout)

	return fyne.NewMenu(lang.L("Help"), manual, fyne.NewMenuItemSeparator(), about)
}
