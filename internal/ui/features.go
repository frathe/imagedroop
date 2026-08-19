package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/ui/assets"
	"github.com/frathe/picfetch/internal/ui/deletion"
	"github.com/frathe/picfetch/internal/ui/exifwin"
	"github.com/frathe/picfetch/internal/ui/favorites"
	"github.com/frathe/picfetch/internal/ui/grid"
	"github.com/frathe/picfetch/internal/ui/help"
	"github.com/frathe/picfetch/internal/ui/settingswin"
	"github.com/frathe/picfetch/internal/ui/slideshow"
	"github.com/frathe/picfetch/internal/ui/zoom"
)

// registerFeatures constructs every feature in dependency order. It only
// assigns the viewer's feature fields; build.go still decides how their
// widgets compose, and menu.go still decides how their menus compose.
func registerFeatures(view *viewer, application fyne.App, window fyne.Window, prefs preferences.State) {
	view.help = help.New(application, appTitle, assets.WelcomeWebP)
	view.exif = exifwin.New(application, func() (fyne.URI, bool) {
		return view.displayedFile()
	})

	// Resolve these callbacks against the viewer at call time so tests can
	// replace keyModifiers after construction.
	view.zoom = zoom.New(
		view.img,
		func() { view.updateInfoOverlay() },
		func() fyne.KeyModifier { return view.keyModifiers() },
		view.requestVectorRender,
	)

	// The thumbnail-cache setter reaches into the grid, so the grid must be
	// registered before saved cache limits are applied.
	view.grid = grid.New(view, window)
	view.SetMaxThumbCacheMB(prefs.MaxThumbCacheMB)
	view.SetMaxFileSizeMB(prefs.MaxFileSizeMB)

	view.deletion = deletion.New(view)

	// Run starts the position poller only after buildViewer returns. Register
	// the slideshow first because the poller's skip callback reads Active.
	view.slides = slideshow.New(view, window, &view.winPos)
	if prefs.SlideInterval > 0 {
		view.slides.SetInterval(prefs.SlideInterval)
	}
	view.slides.SetShuffle(prefs.SlideShuffle)

	view.settings = settingswin.New(application, view)
	view.favorites = favorites.New(view, window)
}
