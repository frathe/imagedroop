// Construction and wiring: buildViewer and the per-feature widget
// constructors it composes, plus the keyboard-shortcut wiring. Each
// new*UI constructor builds one feature's widget cluster and returns it as
// a small struct - the widgets themselves still land in the viewer's flat
// fields for now.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/session"
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
)

// fixedHeightLayout wraps a single object, forcing its MinSize height to a
// fixed value instead of the object's natural (themed) size, while the
// object still fills whatever size it's ultimately resized to.
type fixedHeightLayout struct {
	height float32
}

func (f fixedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w float32
	for _, o := range objects {
		w = fyne.Max(w, o.MinSize().Width)
	}
	return fyne.NewSize(w, f.height)
}

func (f fixedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

// dropzoneUI is the empty-state drop zone: the rounded border box, the
// "Drop images here" hint, the restore-session link, the welcome and
// empty-state art, all inside one tappable area (root) that doubles as an
// "open files" button.
type dropzoneUI struct {
	hint          *widget.Label
	restoreLink   *widget.Hyperlink
	welcomeArt    *canvas.Image
	emptyStateArt *canvas.Image
	art           *widgets.TappableArea
	root          *fyne.Container
}

// newDropzoneUI builds the drop zone. onOpen runs when the zone is tapped
// (the "open files" fallback for users who never drag-and-drop - see
// openFileDialog in openfiles.go); onRestore when the restore-session link
// is. Both callbacks are invoked only ever on a later tap, so buildViewer
// can hand in closures over a viewer variable that isn't assigned yet.
func newDropzoneUI(onOpen, onRestore func()) dropzoneUI {
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = widgets.DropzoneBorderColor
	border.StrokeWidth = widgets.DropzoneBorderWidth
	border.CornerRadius = widgets.DropzoneBorderRadius

	hint := widget.NewLabelWithStyle(lang.L("Drop images here"),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// restoreLink offers to reload the file set saved when the window last
	// closed (see session.go). Shown only if a saved session actually
	// exists - buildViewer sets its text and visibility once savedSession
	// is known.
	restoreLink := widget.NewHyperlink("", nil)
	restoreLink.Hide()
	restoreLink.OnTapped = onRestore

	// welcomeArt greets the user on first launch; handleDrop hides it for
	// good the moment the first drop happens. emptyStateArt is shown only
	// once an error subsequently leaves the drop zone empty (see ShowToast
	// call sites in drop.go/load.go). Both share one min size so they occupy
	// the exact same box on the right of the drop zone, and ImageFillContain
	// scales their (much larger) source art down to fit inside it.
	welcomeArt := canvas.NewImageFromResource(fyne.NewStaticResource("welcome.webp", assets.WelcomeWebP))
	welcomeArt.FillMode = canvas.ImageFillContain
	welcomeArt.ScaleMode = canvas.ImageScaleSmooth
	welcomeArt.SetMinSize(fyne.NewSize(widgets.WelcomeArtSize, widgets.WelcomeArtSize))

	emptyStateArt := canvas.NewImageFromResource(fyne.NewStaticResource("placeholder.webp", assets.PlaceholderWebP))
	emptyStateArt.FillMode = canvas.ImageFillContain
	emptyStateArt.ScaleMode = canvas.ImageScaleSmooth
	emptyStateArt.SetMinSize(fyne.NewSize(widgets.WelcomeArtSize, widgets.WelcomeArtSize))
	emptyStateArt.Hide()

	// Tappable so the whole drop zone - not just the art - doubles as an
	// "open files" button. restoreLink still gets its own taps: Fyne
	// resolves a tap to the deepest matching Tappable under the pointer, so
	// tapping restoreLink itself reaches its own OnTapped rather than this
	// wrapper's, even though it's nested inside it.
	art := widgets.NewTappableArea(container.NewBorder(nil, nil, nil,
		container.NewStack(welcomeArt, emptyStateArt),
		container.NewCenter(container.NewVBox(hint, restoreLink))), onOpen)
	art.OnHover = func(hovering bool) {
		if hovering {
			border.StrokeColor = widgets.DropzoneHoverColor
		} else {
			border.StrokeColor = widgets.DropzoneBorderColor
		}
		border.Refresh()
	}

	return dropzoneUI{
		hint:          hint,
		restoreLink:   restoreLink,
		welcomeArt:    welcomeArt,
		emptyStateArt: emptyStateArt,
		art:           art,
		root:          container.NewStack(border, art),
	}
}

// scanUI is the folder-scan progress indicator: an infinite spinner over a
// "Scanning... N images" counter, both hidden until handleDrop shows them.
type scanUI struct {
	spinner *widget.ProgressBarInfinite
	label   *widget.Label
}

func newScanUI() scanUI {
	spinner := widget.NewProgressBarInfinite()
	label := widget.NewLabelWithStyle(lang.L("Scanning... 0 images"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	spinner.Hide()
	label.Hide()

	return scanUI{spinner: spinner, label: label}
}

// sortUI is the background-reorder progress indicator: an infinite spinner
// over a static "Sorting..." label, both hidden until startSort (sort.go)
// shows them - for a sort-mode change or for the reorder a finished drop
// hands over. A dedicated pair rather than reusing scanUI's - a background
// scan (a merge-mode drop) can still be in flight when a sort-mode change is
// requested, since handleKeyEvent's S-key guard only checks
// len(v.state.files)<2/v.loading, not v.scanning, and the two would otherwise
// fight over one pair of widgets. Unlike scanUI's label, this one's text
// never changes: the ask is only to show that a sort is running, not to
// track its progress the way the scan counter does.
type sortUI struct {
	spinner *widget.ProgressBarInfinite
	label   *widget.Label
}

func newSortUI() sortUI {
	spinner := widget.NewProgressBarInfinite()
	label := widget.NewLabelWithStyle(lang.L("Sorting..."), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	spinner.Hide()
	label.Hide()

	return sortUI{spinner: spinner, label: label}
}

// infoUI is the persistent info overlay (I key, see toggleInfoOverlay in
// info.go) - unlike the toast it never auto-hides itself, and it's several
// distinct lines rather than one centered message, so it uses the theme's
// own overlay-background/foreground pairing (the same one dialogs use)
// instead of the toast's fixed, deliberately loud warning colors - legible
// in both light and dark themes without hardcoding either.
type infoUI struct {
	text     *widget.Label
	exifLink *widget.Hyperlink
	card     *fyne.Container
}

// newInfoOverlayUI builds the info card. onShowExif backs the "Show EXIF
// data" link right below the card's own text (the click equivalent of the E
// key, see internal/ui/exifwin); like newDropzoneUI's callbacks it
// only ever runs on a later tap, so it may close over a not-yet-assigned
// viewer variable.
func newInfoOverlayUI(onShowExif func()) infoUI {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	bg.CornerRadius = widgets.CardRadius
	text := widget.NewLabel("")
	text.Alignment = fyne.TextAlignLeading

	exifLink := widget.NewHyperlink(lang.L("Show EXIF data"), nil)
	exifLink.OnTapped = onShowExif

	card := container.NewStack(bg, container.NewPadded(container.NewVBox(text, exifLink)))
	card.Hide()

	return infoUI{text: text, exifLink: exifLink, card: card}
}

// buildViewer wires up every widget, the drop handler, and the key handler
// for a fresh window - exactly what main() runs live. Tests call it the
// same way, so e2e coverage exercises the real construction path instead
// of a hand-copied replica that can drift out of sync with it.
func buildViewer(application fyne.App) (*viewer, fyne.Window) {
	window := application.NewWindow(appTitle)

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

	// Loaded now, ahead of the struct literal below, so savedSession is
	// ready the moment view exists - restoreLink's own visibility is set
	// right after, once view.restoreSession has somewhere to close over.
	savedSession := session.Load(application)

	// Loaded alongside savedSession so sortMode/mergeMode start from the
	// previous run's standing preferences instead of the shipped defaults -
	// see internal/preferences. prefs.SlideInterval and prefs.WindowSize are
	// applied further below, once view/window exist to apply them to.
	prefs := preferences.Load(application)

	// maxScan falls back to the shipped default when nothing was ever
	// saved (prefs.MaxScanFiles's zero value - see preferences.State's own
	// comment on that field), the same zero-means-unset pattern
	// prefs.SlideInterval already uses below. maxWinW/maxWinH do the same
	// for the window-size cap resizeToImage (load.go) enforces.
	maxScan := defaultMaxScannedFiles
	if prefs.MaxScanFiles > 0 {
		maxScan = prefs.MaxScanFiles
	}
	maxWinW := float32(defaultMaxWindowWidth)
	if prefs.MaxWindowWidth > 0 {
		maxWinW = prefs.MaxWindowWidth
	}
	maxWinH := float32(defaultMaxWindowHeight)
	if prefs.MaxWindowHeight > 0 {
		maxWinH = prefs.MaxWindowHeight
	}

	// The three memory limits (memlimits.go) fall back the same way. The
	// image cache's budget is applied in the literal below, since the cache
	// is built there; the other two need view/grid to exist first and are
	// applied through their setters further down.
	imgCacheMB := defaultMaxImageCacheMB
	if prefs.MaxImageCacheMB > 0 {
		imgCacheMB = prefs.MaxImageCacheMB
	}
	thumbCacheMB := defaultMaxThumbCacheMB
	if prefs.MaxThumbCacheMB > 0 {
		thumbCacheMB = prefs.MaxThumbCacheMB
	}
	maxFileMB := defaultMaxFileSizeMB
	if prefs.MaxFileSizeMB > 0 {
		maxFileMB = prefs.MaxFileSizeMB
	}

	view = &viewer{
		app:           application,
		win:           window,
		img:           img,
		hint:          dz.hint,
		dropzone:      dz.root,
		dropzoneArt:   dz.art,
		welcomeArt:    dz.welcomeArt,
		emptyStateArt: dz.emptyStateArt,
		restoreLink:   dz.restoreLink,
		savedSession:  savedSession,
		loadingBar:    loadingBar,
		scanSpinner:   scan.spinner,
		scanLabel:     scan.label,
		sortSpinner:   sortUIC.spinner,
		sortLabel:     sortUIC.label,
		toast:         toastComp,
		infoText:      info.text,
		infoCard:      info.card,
		exifLink:      info.exifLink,
		state:         newAppState(filesort.FromPref(prefs.SortMode), prefs.MergeMode),
		baseTitle:     appTitle,
		help:          help.New(application, appTitle, assets.WelcomeWebP),
		exif:          exifwin.New(application, func() (fyne.URI, bool) { return view.displayedFile() }),
		imgCache:      imaging.NewImgCache(int64(imgCacheMB) * bytesPerMB),
		preloadSem:    make(chan struct{}, preloadConcurrency),
		maxScan:       maxScan,
		maxWinW:       maxWinW,
		maxWinH:       maxWinH,
		imgCacheMB:    imgCacheMB,
		wallpaperDir:  defaultWallpaperDir(),
		keyModifiers:  defaultKeyModifiers,
	}

	view.vectorDebounce = defaultVectorDebounce
	view.vectorRasterize = func(vec *imaging.Vector, w, h int) (image.Image, error) { return vec.RasterAt(w, h) }
	view.vectorAfter = time.After

	// Seeded here for the same reason imgCache's budget is: buildViewer
	// applies the saved preference directly rather than through the
	// setter. Every later change goes through SetMaxImageCacheMB, which
	// keeps the two in step.
	imaging.SetMaxVectorRasterPixels(vectorRasterPixelsFor(imgCacheMB))

	if n := len(savedSession); n > 0 {
		dz.restoreLink.SetText(fmt.Sprintf(lang.L("Restore last session (%d images)"), n))
		dz.restoreLink.Show()
	}

	// The zoom/pan view: its widget goes into the content Stack below in
	// place of img itself, so it can override Stack's usual fill-container
	// layout and so dragging the image pans it. Both funcs are wrapped in
	// closures rather than passed as method values, so they resolve
	// against the viewer at call time - which is what lets tests swap
	// keyModifiers on an already-built viewer and have the scroll handler
	// see the new one.
	view.zoom = zoom.New(img,
		func() { view.updateInfoOverlay() },
		func() fyne.KeyModifier { return view.keyModifiers() },
		view.requestVectorRender)

	// The full-window thumbnail grid (G key), built now for the same
	// reason the zoom view is: grid.New takes the viewer as its Host. Also
	// takes window directly, to maximize it on open - see grid.New.
	view.grid = grid.New(view, window)

	// The thumbnail cache's budget and the encoded-input ceiling, applied
	// through the same setters the settings window uses (memlimits.go) -
	// the first needs view.grid to exist, and the second writes
	// process-wide state in internal/imaging rather than a viewer field.
	view.SetMaxThumbCacheMB(thumbCacheMB)
	view.SetMaxFileSizeMB(maxFileMB)

	// The delete-confirmation flow, same reason again: deletion.New takes
	// the viewer as its Host, so it can only be built once view exists.
	view.deletion = deletion.New(view)

	// Picture-frame mode (P key), same reason once more - plus the window
	// and the position tracker it captures and restores around
	// full-screen, which is the same tracker startWindowPosPolling below
	// keeps current the rest of the time. Built before the poller starts,
	// since the poller reads its Active() on every tick.
	view.slides = slideshow.New(view, window, &view.winPos)
	if prefs.SlideInterval > 0 {
		view.slides.SetInterval(prefs.SlideInterval)
	}
	view.slides.SetShuffle(prefs.SlideShuffle)

	// The settings window (File > Settings…), same reason once more:
	// settingswin.New takes the viewer as its Host, so it can only be built
	// once view exists.
	view.settings = settingswin.New(application, view)
	view.favorites = favorites.New(view, window)

	// Both secondary windows remember where the user last put them and how
	// large they left them, the same way the main window does below - see
	// widgets.Singleton, which owns the mechanism, and windowtrack.go for
	// the translation. Seeded before either can be opened; Run's
	// SetOnStopped reads the current values back out at shutdown.
	view.settings.RestoreGeometry(widgetGeometry(prefs.SettingsWindow))
	view.exif.RestoreGeometry(widgetGeometry(prefs.ExifWindow))

	// The bar lives in its own overlay layer on top of the stack, pinned to
	// the top edge by the VBox layout, so showing/hiding it never resizes
	// or shifts the image underneath. VBoxLayout sizes each child to its
	// own MinSize height, so loadingBar is wrapped to force that height to
	// loadingBarHeight regardless of the widget's natural (themed) size.
	overlay := container.New(layout.NewVBoxLayout(), container.New(fixedHeightLayout{height: loadingBarHeight}, loadingBar))

	scanContainer := container.NewCenter(container.NewVBox(scan.spinner, scan.label))
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
	// image view underneath, and wrong for the two things that now have to
	// appear *over* an open grid: the batch delete confirmation (whose own
	// scrim is translucent, so the grid dims through it) and the toast that
	// reports what the batch did.
	window.SetContent(container.New(windowSizeTracker(view, window),
		view.zoom.Widget(), dz.root, scanContainer, sortContainer, overlay, infoOverlay,
		view.grid.Overlay(), view.deletion.Overlay(), toastOverlay))
	window.SetMainMenu(buildMainMenu(view))

	// The saved window size (see internal/preferences) is only ever the
	// empty-dropzone size in practice: as soon as a file loads,
	// resizeToImage takes over fitting the window to each image, the same
	// as it always has.
	initialSize := fyne.NewSize(startW, startH)
	if prefs.WindowSize.Width > 0 && prefs.WindowSize.Height > 0 {
		initialSize = prefs.WindowSize
	}
	window.Resize(initialSize)

	// The saved window position (see internal/preferences/internal/winpos)
	// is applied the same way the saved size is above: RequestPosition
	// before the window is actually shown just primes the coordinates the
	// glfw driver's own window-creation path applies once it does run.
	// Stored into the tracker first, and not only applied, so a shutdown
	// before startWindowPosPolling's background poller (below) ever gets a
	// fresh reading still has last launch's good value to hand
	// preferences.Save, rather than losing it to a zero reading.
	if prefs.WindowPositionSet {
		view.winPos.Store(prefs.WindowPosX, prefs.WindowPosY)
		view.winPos.Restore(window)
	}
	view.stopWinPosPoll = startWindowPosPolling(view, window)

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

	wireOpenShortcuts(window.Canvas(), view)
	wireFavoriteShortcuts(window.Canvas(), view.favorites.Open)
	wireClipboardShortcuts(window.Canvas(), view)
	wireDeleteShortcut(window.Canvas(), view)
	wireSelectAllShortcut(window.Canvas(), view)
	wireSaveShortcut(window.Canvas(), view)

	return view, window
}

// shortcutAdder is the one method the shortcut wiring needs from fyne.Canvas,
// narrow enough that a bare *fyne.ShortcutHandler satisfies it too -
// so tests can drive the exact same wiring against that handler directly
// and then fire it via its own TypedShortcut, instead of going through a
// full canvas. That detour is load-bearing, not a style choice: Fyne's test
// driver canvas (fyne.io/fyne/v2/test) embeds software.WindowlessCanvas by
// interface, which doesn't include TypedShortcut, so a real Ctrl+O key
// event can never be simulated through it - only the production glfw
// driver's canvas has that method reachable at all (see
// internal/driver/glfw/window.go's triggersShortcut, which is what turns a
// real key-plus-modifier press into this call).
type shortcutAdder interface {
	AddShortcut(shortcut fyne.Shortcut, handler func(shortcut fyne.Shortcut))
}

// wireOpenShortcuts binds Cmd/Ctrl+O and Cmd/Ctrl+Shift+O to the same
// native file/folder browser tapping the drop zone already opens
// (openFileDialog in openfiles.go). There's only one such dialog - it
// combines files and folders in one go, see internal/filepicker - so the
// second, modified binding isn't a second dialog, just a second way to
// reach the first one. Modified key combos never reach handleKeyEvent's
// SetOnTypedKey dispatch at all: Fyne's desktop driver intercepts them as
// shortcuts before TypedKey ever fires, which is why this needs
// AddShortcut instead of another case there.
func wireOpenShortcuts(c shortcutAdder, view *viewer) {
	openShortcut := func(fyne.Shortcut) { view.openFileDialog() }
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault,
	}, openShortcut)
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}, openShortcut)
}

// wireFavoriteShortcuts binds the first ten sorted favorites to Cmd/Ctrl+1
// through 9, then Cmd/Ctrl+0. The handlers stay registered while Feature.Open
// resolves each slot against the latest menu refresh after an add or removal.
func wireFavoriteShortcuts(c shortcutAdder, open func(index int)) {
	for i := 0; i < favorites.ShortcutCount; i++ {
		index := i
		c.AddShortcut(favorites.ShortcutForIndex(index), func(fyne.Shortcut) {
			open(index)
		})
	}
}

// wireClipboardShortcuts binds Cmd/Ctrl+C to copy the current image and
// Cmd/Ctrl+Shift+C to copy its file path (clipboard.go). Both need
// AddShortcut rather than handleKeyEvent's plain SetOnTypedKey dispatch, for
// the same reason wireOpenShortcuts does: modified key combos never reach
// TypedKey at all. Deliberately not gated behind handleKeyEvent's
// len(v.state.files)<2 navigation guard - both work fine with a single file
// loaded, and copyImageToClipboard/copyPathToClipboard already no-op safely
// when nothing is loaded yet.
//
// The plain Cmd/Ctrl+C binding is *not* a desktop.CustomShortcut, unlike
// every other shortcut in this file - that was the bug that shipped
// initially. Fyne's glfw driver special-cases the bare default-modifier
// forms of Z/Y/V/C/Insert/X/A (undo/redo/paste/copy/.../cut/select-all) into
// its own built-in fyne.Shortcut types *before* it ever considers building a
// desktop.CustomShortcut - see triggersShortcut in
// internal/driver/glfw/window.go, where that switch runs first and only
// falls through to a CustomShortcut when it didn't match. So a
// CustomShortcut registered for {KeyC, KeyModifierShortcutDefault} is
// simply never reachable by a real Cmd/Ctrl+C press; the driver dispatches a
// &fyne.ShortcutCopy{} instead, which needs its own AddShortcut entry to be
// caught. Shift+Cmd/Ctrl+C isn't one of the driver's special-cased combos,
// so it still becomes a CustomShortcut and needs no such treatment.
func wireClipboardShortcuts(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) { view.copySelection() })
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}, func(fyne.Shortcut) { view.copyPathToClipboard() })
}

// wireSelectAllShortcut binds Cmd/Ctrl+A to the grid's select-all
// (batch.go's selectAllInGrid). A third instance of the same driver quirk
// wireClipboardShortcuts documents: A is one of the bare combos
// triggersShortcut special-cases into a built-in fyne.Shortcut type
// (&fyne.ShortcutSelectAll{}) before it would ever build a
// desktop.CustomShortcut, so a CustomShortcut for {KeyA,
// KeyModifierShortcutDefault} could never be reached by a real key press.
func wireSelectAllShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutSelectAll{}, func(fyne.Shortcut) { view.selectAllInGrid() })
}

// wireDeleteShortcut binds Shift+Delete to open the permanent-delete
// confirmation card (deletion.Confirmer.Request). Same bug shape as
// Cmd/Ctrl+C above, different special case: triggersShortcut special-cases
// bare Shift+Delete into &fyne.ShortcutCut{Secondary: true} (its "alternative
// cut" binding, mirroring Shift+Insert for paste) *before* it would ever
// consider a desktop.CustomShortcut - and unlike the Ctrl+key cases, that
// function's CustomShortcut fallback explicitly skips building one whenever
// the modifier is bare Shift at all, so a CustomShortcut{KeyDelete,
// KeyModifierShift} registration wouldn't just be shadowed here, it could
// never be reached by any bare-Shift combo. So this needs an AddShortcut
// entry for &fyne.ShortcutCut{} instead - see deletion.ShortcutHandler
// (deletion.go) for how it tells a real Shift+Delete apart from a genuine
// Ctrl/Cmd+X (which reaches the same handler, Secondary false, and is
// correctly ignored: this app has no cut action).
//
// What it runs is batch.go's requestDelete rather than Confirmer.Request
// directly: the same key means the grid's selection while the overview is up
// and the file on screen otherwise, and deciding that is this package's job,
// not either feature package's. It used to be gated behind a `blocked` check
// that dropped the shortcut entirely while the grid was showing - there was
// nothing then for it to act on there, and the card would have opened hidden
// behind the grid's backdrop. Both of those are now handled instead of
// avoided (see the window stack in buildViewer).
func wireDeleteShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutCut{}, deletion.ShortcutHandler(view.requestDelete))
}

// wireSaveShortcut binds Cmd/Ctrl+S to saveRotation (save.go). S isn't one
// of the driver's specially-cased bare shortcuts (only Z/Y/V/C/Insert/X/A
// are - see wireClipboardShortcuts' comment), so a plain desktop.
// CustomShortcut reaches it the same way Cmd/Ctrl+O reaches
// wireOpenShortcuts.
func wireSaveShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}, func(fyne.Shortcut) { view.saveRotation() })
}
