// Application assembly and wiring: buildViewer composes app-owned
// components with the registered feature widgets, builds the root overlay
// stack, and installs the window's input handlers.
package ui

import (
	"fmt"
	"image"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
)

// buildViewer wires up every widget, the drop handler, and the key handler
// for a fresh window from inputs already loaded by loadStartupState. Tests
// call the same assembly path, so e2e coverage cannot drift from Run.
func buildViewer(application fyne.App, startup startupState) (*viewer, fyne.Window) {
	window := application.NewWindow(appTitle)
	savedSession := startup.savedSession
	prefs := startup.prefs

	// Declared ahead of the constructors below so their tap/click callbacks
	// can close over it: a callback only ever runs on a later interaction,
	// by which point view is assigned, but it needs to be referenceable
	// before the viewer itself can be constructed (which in turn needs the
	// widgets built below).
	var view *viewer

	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleSmooth
	img.Hide()

	dz := newDropzoneUI(
		func() { view.openFileDialog() },
		func() { view.restoreSession() },
	)
	scan := newScanUI()
	sortUIC := newSortUI()
	toastComp := newToast(func() { view.ForceRepaint() })
	info := newInfoOverlayUI(func() { view.exif.Show() })

	loadingBar := widget.NewProgressBarInfinite()
	loadingBar.Hide()

	view = &viewer{
		app:            application,
		win:            window,
		img:            img,
		hint:           dz.hint,
		dropzone:       dz.root,
		dropzoneArt:    dz.art,
		welcomeArt:     dz.welcomeArt,
		emptyStateArt:  dz.emptyStateArt,
		restoreLink:    dz.restoreLink,
		savedSession:   savedSession,
		loadingBar:     loadingBar,
		scanArt:        scan.art,
		scanSpinner:    scan.spinner,
		scanLabel:      scan.label,
		sortSpinner:    sortUIC.spinner,
		sortLabel:      sortUIC.label,
		toast:          toastComp,
		infoText:       info.text,
		infoCard:       info.card,
		exifLink:       info.exifLink,
		state:          newAppState(filesort.FromPref(prefs.SortMode), prefs.MergeMode),
		baseTitle:      appTitle,
		imgCache:       imaging.NewImgCache(int64(prefs.MaxImageCacheMB) * bytesPerMB),
		preloadSem:     make(chan struct{}, preloadConcurrency),
		maxScan:        prefs.MaxScanFiles,
		maxWinW:        prefs.MaxWindowWidth,
		maxWinH:        prefs.MaxWindowHeight,
		imgCacheMB:     prefs.MaxImageCacheMB,
		wallpaperDir:   defaultWallpaperDir(),
		keyModifiers:   defaultKeyModifiers,
		stopWinPosPoll: noPollerStop,
	}

	view.vector.debounce = defaultVectorDebounce
	view.vector.rasterize = func(vec *imaging.Vector, w, h int) (image.Image, error) { return vec.RasterAt(w, h) }
	view.vector.after = time.After

	// Seeded here for the same reason imgCache's budget is: buildViewer
	// applies the saved preference directly rather than through the
	// setter. Every later change goes through SetMaxImageCacheMB, which
	// keeps the two in step.
	imaging.SetMaxVectorRasterPixels(vectorRasterPixelsFor(prefs.MaxImageCacheMB))

	if n := len(savedSession); n > 0 {
		dz.restoreLink.SetText(fmt.Sprintf(lang.L("Restore last session (%d images)"), n))
		dz.restoreLink.Show()
	}

	registerFeatures(view, application, window, prefs)

	// The bar lives in its own overlay layer on top of the stack, pinned to
	// the top edge by the VBox layout, so showing/hiding it never resizes
	// or shifts the image underneath. VBoxLayout sizes each child to its
	// own MinSize height, so loadingBar is wrapped to force that height to
	// loadingBarHeight regardless of the widget's natural (themed) size.
	overlay := container.New(layout.NewVBoxLayout(), container.New(fixedHeightLayout{height: loadingBarHeight}, loadingBar))

	scanContainer := container.NewCenter(container.NewVBox(scan.art, scan.spinner, scan.label))
	sortContainer := container.NewCenter(container.NewVBox(sortUIC.spinner, sortUIC.label))

	// Pinned to the bottom edge, mirroring how loadingBar is pinned to the
	// top: a leading spacer eats all the slack space in the VBox, leaving
	// the card its natural size at the bottom.
	toastOverlay := container.New(layout.NewVBoxLayout(), layout.NewSpacer(), container.NewCenter(toastComp.card))

	// Pinned to the top-left corner: an HBox with a trailing spacer keeps
	// infoCard at its natural (unstretched) width instead of HBox's default
	// of filling the row, and nesting that inside a VBox (with nothing
	// below it) keeps it pinned to the top instead of vertically centered.
	infoOverlay := container.New(layout.NewVBoxLayout(), container.NewHBox(info.card, layout.NewSpacer()))

	// Order is paint order, back to front, and the tail of it is load-bearing.
	// The grid's backdrop is opaque and fills the window, so anything stacked
	// below it is simply invisible while it is open - which is fine for the
	// image view underneath, and wrong for the things that now have to
	// appear *over* an open grid: the batch delete confirmation and the
	// export-format prompt (both share widgets.ChoiceCard, whose own scrim
	// is translucent, so the grid dims through it rather than being hidden
	// by it) and the toast that reports what the batch did.
	window.SetContent(container.New(windowSizeTracker(view, window),
		view.zoom.Widget(), dz.root, scanContainer, sortContainer, overlay, infoOverlay,
		view.grid.Overlay(), view.deletion.Overlay(), view.exportPrompt.Overlay(), toastOverlay))
	window.SetMainMenu(buildMainMenu(view))

	window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		view.handleDrop(uris)
	})

	// F1 opens the manual, Escape clears back to the initial state (or quits
	// if it's already there), the arrow keys (plus Home/End) walk through
	// the dropped files, S toggles sort order, and M toggles merge mode.
	// See viewer.handleKeyEvent for the dispatch.
	window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		view.handleKeyEvent(ev)
	})

	// Typed characters, as opposed to key names: the grid's filename search
	// is the one feature that needs the actual character - case included,
	// and punctuation a fyne.KeyEvent has no name for. See
	// viewer.handleTypedRune.
	window.Canvas().SetOnTypedRune(func(r rune) {
		view.handleTypedRune(r)
	})

	wireGlobalShortcuts(window.Canvas(), view)

	return view, window
}
