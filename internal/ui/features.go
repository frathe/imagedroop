package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/ui/assets"
	"github.com/frathe/picfetch/internal/ui/deletion"
	"github.com/frathe/picfetch/internal/ui/exifwin"
	"github.com/frathe/picfetch/internal/ui/favorites"
	"github.com/frathe/picfetch/internal/ui/grid"
	"github.com/frathe/picfetch/internal/ui/help"
	"github.com/frathe/picfetch/internal/ui/settingswin"
	"github.com/frathe/picfetch/internal/ui/slideshow"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/ui/zoom"
	"github.com/frathe/picfetch/internal/wingesture"
)

// registerFeatures constructs every feature in dependency order. It only
// assigns the viewer's feature fields; build.go still decides how their
// widgets compose, and menu.go still decides how their menus compose.
func registerFeatures(view *viewer, application fyne.App, window fyne.Window, prefs preferences.State) {
	view.help = help.New(application, appTitle, assets.WelcomeWebP)

	// The window-drag easter egg (gesture.go): the detector is fed by the
	// position poller, and a recognised spiral goes to the same Help that
	// owns the manual's secret-phrase door, so both raise one window.
	view.spiralDrag = wingesture.New(wingesture.Config{})
	view.spiralGesture = view.help.OpenSpiral
	view.exif = exifwin.New(application, func() (fyne.URI, bool) {
		return view.displayedFile()
	})

	// Resolve these callbacks against the viewer at call time so tests can
	// replace keyModifiers after construction.
	view.zoom = zoom.New(
		view.img,
		func() {
			view.syncWindowToZoom()
			view.updateInfoOverlay()
		},
		func() fyne.KeyModifier { return view.keyModifiers() },
		view.requestVectorRender,
	)

	// The thumbnail-cache setter reaches into the grid, so the grid must be
	// registered before saved cache limits are applied.
	view.grid = grid.New(view, window)
	view.SetMaxThumbCacheMB(prefs.MaxThumbCacheMB)
	view.SetMaxFileSizeMB(prefs.MaxFileSizeMB)

	view.SetFavoritePreviewCache(prefs.FavoritePreviewCache)

	view.deletion = deletion.New(view)

	// The export-format prompt (promptExport, export.go) is a bare
	// widgets.ChoiceCard, unlike deletion's own wrapping Confirmer: each
	// choice's OnChosen already re-checks canExport/CurrentFile through
	// exportAs at the moment it runs, so there's no per-request state (the
	// way deletion's targets are) for a wrapper type to hold.
	//
	// Filled by index rather than listed positionally so that export.go's
	// pngChoice/jpegChoice constants are what actually place the buttons -
	// the same constants the prompt is selected and asserted through
	// elsewhere. Written as two positional literals, the constants would
	// only describe the order, and a swap of the two lines would leave the
	// PNG button quietly writing JPEGs.
	choices := make([]widgets.Choice, 2)
	choices[pngChoice] = widgets.Choice{Label: lang.L("PNG"), OnChosen: func() { view.exportAs(exportPNGExt) }}
	choices[jpegChoice] = widgets.Choice{Label: lang.L("JPEG"), OnChosen: func() { view.exportAs(exportJPEGExt) }}
	view.exportPrompt = widgets.NewChoiceCard(view.ForceRepaint, choices...)

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
