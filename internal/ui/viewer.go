package ui

import (
	"fmt"
	"image"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/ui/deletion"
	"github.com/frathe/picfetch/internal/ui/exifwin"
	"github.com/frathe/picfetch/internal/ui/favorites"
	"github.com/frathe/picfetch/internal/ui/grid"
	"github.com/frathe/picfetch/internal/ui/help"
	"github.com/frathe/picfetch/internal/ui/settingswin"
	"github.com/frathe/picfetch/internal/ui/slideshow"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/ui/zoom"
	"github.com/frathe/picfetch/internal/winpos"
)

// viewer bundles the UI elements and the navigation state so the drop
// handler and the key handler can share them without package-level globals.
type viewer struct {
	app        fyne.App
	win        fyne.Window
	img        *canvas.Image
	hint       *widget.Label
	dropzone   *fyne.Container
	loadingBar *widget.ProgressBarInfinite

	// windowSize is the window's current content size, kept up to date by
	// windowSizeTracker (windowtrack.go) on every layout of the root
	// content - Window itself exposes no size getter. Read once, at
	// shutdown, by Run's preferences.Save call so the window geometry a
	// user left the app at carries over to the next launch (see
	// internal/preferences).
	windowSize fyne.Size

	// dropzoneArt wraps the whole drop zone - the welcome/placeholder art,
	// the hint text, and restoreLink - in a tappable widget (see
	// openfiles.go) so clicking anywhere in the box opens a file dialog, for
	// users who never drag-and-drop.
	dropzoneArt *widgets.TappableArea

	// welcomeArt greets the user on first launch, in the same box as
	// emptyStateArt. handleDrop hides it on the first drop, so a later
	// error shows emptyStateArt there instead; reset (Escape) brings it
	// back, since that returns the viewer to its just-launched state.
	welcomeArt *canvas.Image

	// emptyStateArt is shown alongside the hint, on the right of the drop
	// zone, only while an error has left it with no images to display.
	emptyStateArt *canvas.Image

	// toast is the self-dismissing notification card behind ShowToast -
	// see the toast type (toast.go) for its widgets and auto-hide
	// lifecycle.
	toast *toast

	// help owns the manual and About windows and the Help menu - see
	// internal/ui/help, which needs nothing from the viewer at all.
	help *help.Help

	// favorites owns the Favorites menu and its add/open/remove dialogs.
	favorites *favorites.Feature

	// exif is the EXIF metadata panel - see internal/ui/exifwin, which
	// reaches back only through the "which file is on screen" accessor
	// registerFeatures hands it. finishLoad calls its Refresh so navigating
	// while it's open keeps it in sync.
	exif *exifwin.Window

	// settings is the Settings window (File menu) - see
	// internal/ui/settingswin, which reaches back through the Host
	// interface this viewer satisfies (SortMode/MergeMode/SlideShuffle/
	// SlideInterval/MaxScan/MaxWindowWidth/MaxWindowHeight and their
	// setters).
	settings *settingswin.Window

	// saveItem is the File menu's "Save Changes" item (save.go),
	// exportPNGItem/exportJPEGItem its two "Export as…" items (export.go),
	// wallpaperItem its "Set as Wallpaper" one (wallpaper.go), and
	// closeFilesItem its "Close Files" one (viewer.go's closeFiles) - kept
	// as their own fields, unlike the other menu items built in menu.go,
	// because their Disabled fields need updating from outside
	// buildMainMenu itself, at every site that can change whether there's
	// anything to save, export, set or close: rotate.go, load.go,
	// clearToDropzone, and save.go itself - see updateFileMenuState.
	saveItem       *fyne.MenuItem
	exportPNGItem  *fyne.MenuItem
	exportJPEGItem *fyne.MenuItem
	wallpaperItem  *fyne.MenuItem
	closeFilesItem *fyne.MenuItem

	// restoreLink offers to reload the file set saved when the window last
	// closed (see session.go). Shown only while welcomeArt is - and only
	// when savedSession is non-empty - and hidden together with it, since
	// past the first drop there's nothing left to "restore" into.
	restoreLink *widget.Hyperlink

	// savedSession is the file set loaded from the previous run's session
	// cache (session.go), consumed once by restoreSession. nil when there
	// is nothing to restore.
	savedSession []fyne.URI

	state appState

	// fileSetRevision is the identity of the index-to-URI mapping exposed to
	// feature packages. Grid thumbnail and deletion work capture it through
	// Generation and discard results after a drop, reorder, removal, or clear.
	// It is deliberately independent of loadLifecycle: navigation changes the
	// displayed index but not what any index means.
	fileSetRevision revision

	// loadLifecycle owns a logical navigation and all of its descendants:
	// probe/decode retries, neighbor preloads, and GIF animation. A newer
	// navigation, drop, clear, or shutdown cancels and supersedes the token.
	loadLifecycle requestLifecycle

	// scanLifecycle is independent of navigation, so browsing the existing
	// set during a merge-mode folder scan cannot strand scan UI state.
	scanLifecycle requestLifecycle

	// baseTitle is the window title without the "[merge] " prefix applyTitle
	// adds while merge mode is on, so toggling M can refresh the title
	// immediately without recomputing it.
	baseTitle string

	// gridTitle names the file under the grid overview's highlight while
	// the overview is up, and is empty otherwise. It *overrides* baseTitle
	// in applyTitle rather than replacing it, so whatever the image view
	// last set - or sets while the grid is open, e.g. a drop landing behind
	// it - is still there to fall back to when the grid closes, without
	// anyone having to save and restore a string.
	gridTitle string

	// loading is true while a decode/render is in flight. The key handler
	// checks it to ignore repeat events instead of piling up decodes for
	// images the user has already navigated past.
	loading atomic.Bool

	// winPos is the window's last known on-screen position. Unlike
	// windowSize, which windowSizeTracker captures for free off ordinary
	// layout passes, there is no Fyne hook that fires on a pure window
	// move (see internal/winpos) - startWindowPosPolling (windowtrack.go)
	// keeps the tracker current with a background poller instead, and the
	// slideshow captures and restores it directly around full-screen. A
	// value field, never copied: its state is atomic because the poller
	// reads and writes it off the UI goroutine.
	winPos winpos.Tracker

	// stopWinPosPoll stops startWindowPosPolling's background ticker
	// goroutine; initialized to noPollerStop by buildViewer, replaced by
	// Run after startup geometry is restored, and called from SetOnStopped
	// just before the final preferences save (winPos keeps its last reading,
	// so the save still has a value).
	stopWinPosPoll func()

	scanSpinner *widget.ProgressBarInfinite
	scanLabel   *widget.Label

	// scanning is true while a handleDrop scan (the current generation) is
	// in flight, so Escape can cancel it - see cancelScan. Only ever set on
	// the UI goroutine: true when handleDrop starts, false again once that
	// same generation's completion closure runs, whether it finished,
	// found nothing, or was cancelled. A scan superseded by a newer drop
	// (stale gen) never touches it, since the newer scan already owns the
	// flag by the time the stale one's closure would run.
	scanning bool

	// scanDone and loadDone are closed by handleDrop's and ShowImage's fyne.Do
	// completion blocks respectively, once that call's generation has
	// finished applying its result. Tests wait on them directly instead of
	// polling widget state, which otherwise races with the fyne test
	// driver's synchronous fyne.Do under -race. Each call replaces the
	// field with a fresh channel before starting its async work; a stale
	// request's own channel still gets closed, it just leaves the
	// shared state untouched.
	scanDone chan struct{}
	loadDone chan struct{}

	sortSpinner *widget.ProgressBarInfinite
	sortLabel   *widget.Label

	// sorting is true while the current sortLifecycle request is still
	// meaningfully pending, set by startSort and cleared by whichever of
	// invalidateSort (a newer sort, Escape via cancelSort, RemoveFile,
	// clearToDropzone) or finishSort landing that same token notices
	// first - see sort.go's invalidateSort, which every one of those but
	// finishSort itself goes through - so it never gets stuck true once
	// whatever it was tracking has been superseded, cancelled, or
	// discarded. Used for two things: gating cancelSort (nothing to cancel
	// if nothing's in flight) and handleKeyEvent's Escape case (keys.go) -
	// a first-ever drop clears v.scanning before startSort has actually
	// populated v.state.files, so without this Escape would see len(v.state.files) ==
	// 0 and quit the window instead of cancelling the still-computing
	// reorder.
	sorting bool

	// sortLifecycle owns the cancellable filesort.Order request. It stays
	// separate from loadLifecycle so reordering cannot stop an unrelated
	// decode, preload, or playing GIF.
	sortLifecycle requestLifecycle

	// sortDone is closed by finishSort once that request's reorder has
	// finished applying (or been discarded as stale), mirroring
	// scanDone/loadDone so tests can wait on it deterministically.
	sortDone chan struct{}

	// animFrame counts every write to v.img.Image - attemptLoad's initial
	// frame plus each one animate cycles to afterwards - and animStopped is
	// closed by animate once its load token is cancelled or stale and
	// returns. Both exist so tests can synchronize on frame changes and
	// animation shutdown via atomics and channel-close instead of reading
	// v.img.Image directly from another goroutine, which would race with
	// attemptLoad's/animate's writes under the fyne test driver: it runs
	// fyne.Do synchronously on the calling goroutine rather than marshaling
	// onto a single UI thread, so even a read sequenced after done/loadDone
	// closes has no happens-before edge against a concurrently running
	// animate call - only observing animFrame's new value does. Each
	// animate call gets its own captured animStopped (see attemptLoad), so
	// a superseded request's close can't be mistaken for a newer one's.
	animFrame   atomic.Uint64
	animStopped chan struct{}

	// displayFrames is the current image's decoded, EXIF-corrected frames
	// (loaded.Frames - unrotated), and displayFrameIdx which one of them is
	// currently on screen: index 0 for a static image, or whichever one
	// animate has most recently cycled to for an animated GIF. rotateBy
	// (rotate.go) needs both to redraw at a new rotation without waiting for
	// animate's next tick, since it can't otherwise tell which frame of an
	// in-progress animation is currently up. rotation is the view-only
	// clockwise quarter-turn count (0-3) composed with the EXIF orientation
	// already baked into those frames at render time - see
	// imaging.RotateSteps - and is never written back to disk. Reset to 0 by
	// every fresh navigation (finishLoad) and the 0 key, mirroring the way
	// the zoom view resets to fit.
	displayFrames   []image.Image
	displayFrameIdx int
	rotation        int

	// imgCache holds recently decoded frames keyed by URI string, so
	// navigating back to an image already seen this session - or one
	// preloadNeighbors decoded speculatively ahead of time - is a cache hit
	// instead of a fresh disk read plus decode. Bounded by a byte budget
	// (imgCacheMB below, the settings window's binding) rather than an
	// entry count, since a decoded image's size varies by four orders of
	// magnitude - see imaging.ByteCache, which is safe for concurrent use
	// on its own, so both attemptLoad's decode goroutine and preloadOne's
	// background goroutines can populate it without going through fyne.Do.
	imgCache *imaging.ByteCache[*imaging.LoadedImage]

	// preloading tracks URIs a preloadOne goroutine is currently decoding,
	// so rapid navigation doesn't pile up a second decode of the same
	// not-yet-cached neighbor while the first is still in flight.
	// preloadSem bounds how many of those decodes run at once, the same
	// small-worker-pool shape internal/ui/grid gives thumbnails - without
	// it, rapid navigation could stack an unbounded number of full-size
	// decode goroutines. preloadPending counts them, mirroring
	// thumbPending below: waitUntilLoaded (library_test.go) waits it out
	// after every load so a preload goroutine never outlives the test
	// whose navigation spawned it.
	preloading     sync.Map
	preloadSem     chan struct{}
	preloadPending sync.WaitGroup

	// zoom is the zoom/pan view of img (0/1/+/- and drag/scroll) - see
	// internal/ui/zoom, whose widget sits in the window's content Stack in
	// place of img itself, so it can override Stack's usual "fill the
	// container" layout. It needs no Host: the app and that package share
	// img on a single-writer-per-field contract (the app owns img.Image,
	// zoom owns img's size and position), and the only reach back is the
	// updateInfoOverlay callback registerFeatures hands it.
	zoom *zoom.Zoom

	// infoVisible is a standing preference toggled by I, mirroring
	// sortMode/mergeMode: it survives navigation and drops, but the card
	// itself (infoCard/infoText) is only ever actually shown while an image
	// is on screen - see syncInfoOverlayVisibility. currentFileSize is the
	// current file's raw byte count, carried on imaging.LoadedImage (populated in
	// attemptLoad, so a cache hit has it too) since nothing else in viewer
	// tracks the undecoded size.
	infoVisible bool
	infoText    *widget.Label
	// exifLink is the "Show EXIF data" link inside infoCard - see
	// components.go's construction and build.go's callback wiring. Kept as
	// its own field only so tests can trigger it directly (OnTapped) the
	// same way e2e_test.go does for restoreLink, without a real click through
	// the widget tree. It is only shown for a file that actually has metadata
	// to show (currentHasEXIF, carried the same way currentFileSize is): the
	// link is a promise, and offering it for a file with no Exif at all can
	// only ever open a panel saying so.
	exifLink        *widget.Hyperlink
	infoCard        *fyne.Container
	currentFileSize int64
	currentHasEXIF  bool

	// deletion is the Shift+Delete confirmation flow - see
	// internal/ui/deletion, which owns its own widgets and selection state
	// and reaches back through the Host interface this viewer satisfies.
	// handleKeyEvent checks its Visible() before anything else so every
	// other key is swallowed while a delete decision is pending.
	deletion *deletion.Confirmer

	// grid is the full-window thumbnail overview (G key) - see
	// internal/ui/grid, which owns the thumbnail cache and its decode
	// worker pool and reaches back through the Host interface this viewer
	// satisfies. handleKeyEvent checks its Visible() before its own
	// dispatch, the same way it does for the delete confirmation.
	grid *grid.Overview

	// slides is picture-frame mode (P key) - see internal/ui/slideshow,
	// which owns the full-screen switch, the auto-advance goroutine and
	// the interval behind it, and reaches back through a two-method Host
	// (FileCount/Advance) this viewer satisfies. The app's other
	// full-window mode is the grid above; neither knows about the other,
	// so keeping them from overlapping is this package's job - see
	// handleKeyEvent's G case and togglePictureFrameMode.
	slides *slideshow.Controller

	// fadeAnim is the crossfade in progress, if any, between the last
	// image on screen and the one replacing it - see load.go's
	// startFade/resetFade. Only ever non-nil while picture-frame mode is
	// active: ShowImage starts one fading the outgoing image out,
	// finishLoad starts the next fading the incoming one in, and every
	// path that ends picture-frame mode calls resetFade so the image is
	// never left invisible or half-faded once it's back in the normal,
	// instant-swap view.
	fadeAnim *fyne.Animation

	// clipboardDone is closed once copyImageToClipboard's background
	// shell-out goroutine has fully finished, error reporting included -
	// the same wait-channel discipline scanDone/loadDone give tests for
	// drops and loads. chooserDone is the same for openFileDialog's
	// native-dialog goroutine, and wallpaperDone for setAsWallpaper's.
	clipboardDone chan struct{}
	chooserDone   chan struct{}
	wallpaperDone chan struct{}

	// wallpaperDir is where setAsWallpaper (wallpaper.go) writes the PNG it
	// hands to the OS - defaultWallpaperDir in production, a t.TempDir() in
	// tests, which is why it is a field rather than a package-level
	// constant: this is the one action whose side effect outlives the
	// process, on the developer's own desktop.
	wallpaperDir string

	// maxScan caps how many images a single recursive folder scan will
	// gather - see handleDrop (drop.go). A field rather than the package
	// var it used to be, so tests shrink it per-viewer instead of
	// mutating a global.
	maxScan int

	// maxWinW/maxWinH cap how large the window is ever allowed to
	// auto-grow to fit a loaded image - see resizeToImage (load.go),
	// which never resizes past them. Fields rather than the constants
	// they used to be, so the settings window can change them per-viewer
	// and tests can shrink/grow them without touching a global.
	maxWinW, maxWinH float32

	// imgCacheMB/thumbCacheMB/maxFileMB are the app's memory budget, in
	// the megabytes the settings window shows - see memlimits.go, which
	// holds their getter/setter pairs and converts each to the byte budget
	// its consumer actually enforces.
	imgCacheMB, thumbCacheMB, maxFileMB int

	// keyModifiers reports the keyboard modifiers currently held -
	// defaultKeyModifiers (keys.go) in production, stubbed by tests (the
	// fyne test driver can't synthesize modifier state at all). Read by
	// handleKeyEvent's Shift+R, and by the zoom view's Shift+scroll pan
	// through the closure registerFeatures hands it.
	keyModifiers func() fyne.KeyModifier

	// vector is the parsed SVG behind the image on screen, nil for every
	// raster format. Non-nil is what makes a scale change mean anything.
	vector *imaging.Vector

	// vectorLogical is the size the app treats that vector as being - what
	// the window, the title and the info overlay are built on. Fixed for
	// the lifetime of the loaded image; the raster behind it is not.
	vectorLogical fyne.Size

	// vectorRaster is the pixel size of the raster currently on screen,
	// which requestVectorRender compares a new target against.
	vectorRaster image.Point

	// vectorLifecycle owns debounce and rasterization for the latest SVG
	// render request. A newer scale, image change, clear, or shutdown cancels
	// the previous token and wakes it out of the debounce immediately.
	vectorLifecycle requestLifecycle

	// vectorPending is waited on by the test suite's drain, per the
	// module's concurrency invariant.
	vectorPending sync.WaitGroup

	// vectorDebounce coalesces a burst of scroll-driven scale changes into
	// one rasterization. A per-viewer field rather than a package var
	// (concurrency invariant: the viewer has no mutable package state),
	// which is also the seam that lets tests set it to zero.
	vectorDebounce time.Duration

	// vectorRasterize and vectorAfter are RasterAt and time.After behind
	// per-viewer seams (the concurrency invariant forbids mutable package
	// state), so the coalescing test can count rasterizations and release
	// a parked burst deterministically. Production never overrides them.
	// Like vectorDebounce, they are write-once: set at construction, and
	// by a test only before its first drop.
	vectorRasterize func(vec *imaging.Vector, w, h int) (image.Image, error)
	vectorAfter     func(time.Duration) <-chan time.Time
}

// ForceRepaint refreshes the window's root content object, which has been
// part of the canvas (and so already registered with it) since startup.
// Fyne only registers an object with its canvas the first time it is
// painted while visible, so calling Show()/Refresh() on a widget that has
// spent its whole life hidden - like scanSpinner, scanLabel or loadingBar
// between uses - can't find a canvas to mark dirty and silently fails to
// schedule a repaint; it would otherwise only appear once some unrelated
// event (e.g. a window resize, which marks the canvas dirty directly)
// forces a full repaint. Refreshing an already-registered ancestor here
// triggers that repaint immediately instead.
func (v *viewer) ForceRepaint() {
	v.win.Content().Refresh()
}

// setTitle updates the window title to base, remembering it so a later
// mergeMode toggle can reapply the "[merge] " prefix (or drop it) without
// needing to recompute the title from scratch.
func (v *viewer) setTitle(base string) {
	v.baseTitle = base
	v.applyTitle()
}

// applyTitle (re)applies the current title to the window with a sort-mode
// prefix (see filesort.Label - empty, so invisible, for the default by-name
// sort) and the "[merge]"/"[shuffle]" prefixes when merge mode or the
// slideshow's shuffle order (Shift+P) are on, so the title always makes
// the active drop/sort/slideshow mode visible at a glance. The separating
// space is added here rather than baked into either prefix, so neither
// translation key carries trailing whitespace a translator could silently
// drop. While the grid overview is up gridTitle takes baseTitle's place -
// the image behind it isn't what the user is looking at.
func (v *viewer) applyTitle() {
	title := v.baseTitle
	if v.gridTitle != "" {
		title = v.gridTitle
	}
	if v.state.MergeMode() {
		title = lang.L("[merge]") + " " + title
	}
	if v.slides.Shuffle() {
		title = lang.L("[shuffle]") + " " + title
	}
	if p := filesort.Label(v.state.SortMode()); p != "" {
		title = p + " " + title
	}
	v.win.SetTitle(title)
}

// HighlightChanged names the grid overview's highlighted file in the window
// title (internal/ui/grid's Host): with the image view hidden behind the
// overlay, the title is the only place a thumbnail's file name is spelled
// out in full. i is -1 when nothing is highlighted - the grid closing, or a
// search matching no file - which hands the title back to the image view.
// The dimensions the image view shows alongside the name are deliberately
// absent: they'd cost a full decode of a file nobody has picked yet.
func (v *viewer) HighlightChanged(i int) {
	if i < 0 || i >= len(v.state.files) {
		v.gridTitle = ""
		v.applyTitle()

		return
	}

	title := v.state.files[i].Name()
	if n := len(v.state.files); n > 1 {
		title = fmt.Sprintf("%s  (%d/%d)", title, i+1, n)
	}

	v.gridTitle = title
	v.applyTitle()
}

// clearToDropzone drops the loaded file list and returns the viewer to an
// empty drop-zone state: no image, no files, window back to its start size
// and title. Callers pick which art (welcomeArt or emptyStateArt) belongs
// in the box afterward and are responsible for repainting.
func (v *viewer) clearToDropzone() {
	// A full-screen dropzone would look broken, and there's nothing left to
	// frame - safe to call even when picture-frame mode is already off.
	v.slides.Exit()
	v.resetFade()

	v.invalidateLoad() // invalidate any decode/preload or animation still in flight
	v.invalidateSort() // cancel a sort still in flight - see sortLifecycle's field comment
	v.scanLifecycle.invalidate()

	v.state.clearFiles()
	v.fileSetRevision.advance()

	// Purged, not left to age out: with no files open, every decode the
	// cache holds is of something unreachable, so keeping them just spends
	// the byte budget on nothing until the next drop happens to refill it.
	v.imgCache.Purge()

	v.img.Image = nil
	v.img.Hide()
	v.clearVector() // an in-flight rasterization must not land on whatever loads next

	// v.infoVisible itself is left alone - it's a standing preference like
	// sortMode/mergeMode, so the card comes back on the next load if it
	// was on, same as syncInfoOverlayVisibility already does from finishLoad.
	v.infoCard.Hide()

	v.loading.Store(false)
	v.loadingBar.Hide()

	v.hint.SetText(lang.L("Drop images here"))
	v.dropzone.Show()

	v.setTitle(appTitle)
	v.undoGridMaximize()
	v.win.Resize(fyne.NewSize(startW, startH))
	v.updateFileMenuState()
}

// undoGridMaximize restores the window from a grid-triggered maximize (see
// grid.Overview.ConsumeMaximized) before something is about to resize it
// for a reason of its own - a no-op unless the grid actually left it
// maximized. A plain Resize call alone can't shrink a window Maximize
// grew: on Linux and Windows the maximized state is tracked by the OS
// independently of window geometry, so a Resize made while it's still set
// is silently ignored - see winpos.Unmaximize. Restoring the last known
// position afterward matters for the same reason: the OS's own
// un-maximize placement rarely lands back where the window was before the
// grid took over.
func (v *viewer) undoGridMaximize() {
	if !v.grid.ConsumeMaximized() {
		return
	}
	winpos.Unmaximize(v.win)
	v.winPos.Restore(v.win)
}

// toggleMergeMode flips whether the next drop merges into the existing set
// instead of replacing it - see SetMergeMode below, which does the actual
// work.
func (v *viewer) toggleMergeMode() {
	v.SetMergeMode(!v.state.MergeMode())
}

// SetMergeMode sets merge mode directly - the settings window's binding for
// the toggle above - and immediately reflects it in the window title via
// the "[merge] " prefix so it doesn't wait for a drop to become visible.
func (v *viewer) SetMergeMode(on bool) {
	v.state.SetMergeMode(on)
	v.applyTitle()
}

// MergeMode reports whether merge mode is on - the settings window's
// getter.
func (v *viewer) MergeMode() bool {
	return v.state.MergeMode()
}

// showFileIfPresent looks up target in v.state.files by URI identity and shows it
// if found, reporting whether it was. Used to keep the same file in view
// across an operation - a sort toggle or a merge - that reorders or extends
// v.state.files without changing what's currently on screen.
func (v *viewer) showFileIfPresent(target fyne.URI) bool {
	for i, u := range v.state.files {
		if u.String() == target.String() {
			v.ShowImage(i)
			return true
		}
	}
	return false
}

// reset returns the viewer to the state it was in at launch, so Escape can
// act as "start over" instead of quitting whenever there's something to
// clear.
func (v *viewer) reset() {
	v.clearToDropzone()

	// Also cleared here, not just inside clearToDropzone: every path back to
	// the drop zone must independently abandon the vector, so none of them
	// can regress into leaving an in-flight rasterization able to land on
	// whatever loads next.
	v.clearVector()

	v.showWelcomeState()
	v.ForceRepaint()
}

// closeFiles is the File menu's "Close Files" item: it drops the currently
// loaded set and returns to the welcome drop zone, cancelling a scan still
// in progress first - unlike Escape (handleKeyEvent), it never closes the
// window, since File > Close is a distinct action from quitting the app.
func (v *viewer) closeFiles() {
	if v.scanning {
		v.cancelScan()
	}
	v.reset()
	v.clearVector() // see reset's own comment - each layer clears independently
}

// showWelcomeState restores the launch-time welcome look: welcome art in
// place of the empty-state error art, plus the restore-session link when
// there's a saved session to offer. Shared by reset and cancelScan, which
// differ only in how much else they put back.
func (v *viewer) showWelcomeState() {
	v.emptyStateArt.Hide()
	v.welcomeArt.Show()
	if len(v.savedSession) > 0 {
		v.restoreLink.Show()
	}
}

// ShowEmptyStateError clears back to an empty drop zone - so a previously
// displayed image never lingers behind an error - shows the error
// placeholder art, and raises a toast. Used whenever a drop, scan, or
// decode ends with nothing to display.
func (v *viewer) ShowEmptyStateError(msg string) {
	v.clearToDropzone()

	v.welcomeArt.Hide()
	v.restoreLink.Hide()
	v.emptyStateArt.Show()

	v.ForceRepaint()
	v.ShowToast(msg)
}

// The exported methods on this unexported type - CurrentFile, RemoveFile,
// ShowImage, ShowToast, ShowEmptyStateError, ForceRepaint - are the shared
// vocabulary the feature packages' own Host interfaces are written against
// (see internal/ui/deletion). One method satisfies every such interface, so
// the viewer never grows per-package adapters; and because the type itself
// stays unexported, none of it is reachable from outside internal/ui.

// CurrentFile returns the file currently displayed and its index, or
// ok=false when nothing is loaded.
func (v *viewer) CurrentFile() (u fyne.URI, index int, ok bool) {
	if len(v.state.files) == 0 {
		return nil, 0, false
	}

	return v.state.files[v.state.index], v.state.index, true
}

// displayedFile is CurrentFile narrowed to what the EXIF panel needs: a
// file that is not merely selected but actually decoded and on screen.
// The distinction matters during a failed or in-flight load, when v.state.files
// is non-empty but there is no image to describe.
func (v *viewer) displayedFile() (fyne.URI, bool) {
	if v.img.Image == nil {
		return nil, false
	}

	u, _, ok := v.CurrentFile()

	return u, ok
}

// RemoveFile drops the file at v.state.files[i] from both v.state.files and
// v.state.unsortedFiles, keeping them in sync so a later sort toggle doesn't
// resurrect a file that failed to load. v.state.files is trimmed by index rather
// than by URI match, since merge mode allows dropping the same file twice
// and a match would risk removing the wrong duplicate; unsortedFiles has
// no equivalent index to use, but any matching duplicate there is an
// equally valid one to drop.
func (v *viewer) RemoveFile(i int) {
	v.invalidateSort() // cancel a sort still in flight - see sortLifecycle's field comment

	target := v.state.removeFile(i)
	v.fileSetRevision.advance()
	v.imgCache.Remove(target.String())
}

// RemoveFiles drops every named index in one pass - what internal/ui/deletion
// calls once a batch of files has actually reached the Trash.
//
// Descending, so an earlier removal can't shift a later index out from under
// the same call, and sorted first because the caller's order is not something
// this should depend on. Duplicates are skipped rather than removing two
// different files for one index named twice.
//
// The grid is reconciled here, at the end, rather than by the caller: every
// index it holds - its selection, its filter's display→host mapping, its
// highlight - is an index into the set that just changed underneath it. It
// is also closed outright once nothing is left, since an open grid over an
// empty file set has no cells to draw and Toggle itself refuses to open in
// that state.
func (v *viewer) RemoveFiles(indices []int) {
	prev := -1
	for _, i := range slices.Backward(slices.Sorted(slices.Values(indices))) {
		if i == prev || i < 0 || i >= len(v.state.files) {
			continue
		}
		prev = i

		v.RemoveFile(i)
	}

	v.grid.FilesChanged()
	if len(v.state.files) == 0 {
		v.grid.Close()
	}
}

// Modifiers is which keyboard modifiers are held right now, for the feature
// packages that need to read a gesture rather than a key press -
// internal/ui/grid's Cmd/Ctrl+click and Shift+click. A method over the
// keyModifiers field rather than the field itself, so it satisfies grid.Host
// alongside the rest of the vocabulary below; internal/ui/zoom takes the same
// value as a bare func, which is why the field exists in the first place.
func (v *viewer) Modifiers() fyne.KeyModifier {
	return v.keyModifiers()
}

// FileCount, FileAt, OpenFiles, CurrentIndex, Generation, Unfocus, Modifiers,
// and Advance complete the exported vocabulary the feature packages' Host
// interfaces bind to (see the note above CurrentFile). internal/ui/grid uses
// the first six: the first three to draw the right cells, Generation to
// discard a decode whose file set has since been replaced, Unfocus to
// hand the keyboard back after a thumbnail tap, and Modifiers to tell its
// three click gestures apart. internal/ui/slideshow needs only FileCount and
// Advance.

// FileCount is how many files are currently loaded.
func (v *viewer) FileCount() int {
	return len(v.state.files)
}

// FileAt returns the file at index i.
func (v *viewer) FileAt(i int) fyne.URI {
	return v.state.files[i]
}

// OpenFiles sends a file list through the same scan, merge, sort, and display
// path as a drag-and-drop or the native file chooser.
func (v *viewer) OpenFiles(files []fyne.URI) {
	v.handleDrop(files)
}

// CurrentIndex is the index of the file on screen.
func (v *viewer) CurrentIndex() int {
	return v.state.index
}

// Generation is the current index-to-URI file-set revision. Navigation does
// not change it; replacement, reorder, removal, and clear operations do.
func (v *viewer) Generation() uint64 {
	return v.fileSetRevision.current()
}

// Unfocus releases Fyne's canvas focus.
func (v *viewer) Unfocus() {
	v.win.Canvas().Unfocus()
}

// Advance displays the next file, wrapping around at the end - attemptLoad
// folds the index back into range, so there is nothing to clamp here. It's
// the slideshow's auto-advance step. With shuffle off it's deliberately
// the same navigation the Right key performs rather than a private one;
// with shuffle on (Shift+P) it picks a random other file instead - see
// randomOtherIndex - still through the same ShowImage every navigation
// goes through, so the crossfade and everything else load.go does on a
// navigation applies here too.
func (v *viewer) Advance() {
	if v.slides.Shuffle() {
		v.ShowImage(randomOtherIndex(len(v.state.files), v.state.index))
		return
	}
	v.ShowImage(v.state.index + 1)
}
