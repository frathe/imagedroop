package ui

import (
	"image"
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/imagedrop/internal/filesort"
	"github.com/frathe/imagedrop/internal/imaging"
	"github.com/frathe/imagedrop/internal/ui/deletion"
	"github.com/frathe/imagedrop/internal/ui/exifwin"
	"github.com/frathe/imagedrop/internal/ui/grid"
	"github.com/frathe/imagedrop/internal/ui/help"
	"github.com/frathe/imagedrop/internal/ui/settingswin"
	"github.com/frathe/imagedrop/internal/ui/slideshow"
	"github.com/frathe/imagedrop/internal/ui/widgets"
	"github.com/frathe/imagedrop/internal/ui/zoom"
	"github.com/frathe/imagedrop/internal/winpos"
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

	// exif is the EXIF metadata panel - see internal/ui/exifwin, which
	// reaches back only through the "which file is on screen" accessor
	// buildViewer hands it. finishLoad calls its Refresh so navigating
	// while it's open keeps it in sync.
	exif *exifwin.Window

	// settings is the Settings window (File menu) - see
	// internal/ui/settingswin, which reaches back through the Host
	// interface this viewer satisfies (SortMode/MergeMode/SlideShuffle/
	// SlideInterval/MaxScan/MaxWindowWidth/MaxWindowHeight and their
	// setters).
	settings *settingswin.Window

	// restoreLink offers to reload the file set saved when the window last
	// closed (see session.go). Shown only while welcomeArt is - and only
	// when savedSession is non-empty - and hidden together with it, since
	// past the first drop there's nothing left to "restore" into.
	restoreLink *widget.Hyperlink

	// savedSession is the file set loaded from the previous run's session
	// cache (session.go), consumed once by restoreSession. nil when there
	// is nothing to restore.
	savedSession []fyne.URI

	files []fyne.URI
	index int

	// gen is the load generation: it guards against out-of-order async
	// loads. It's an atomic rather than a plain uint64 because animate's
	// background goroutine reads it outside of fyne.Do's synchronization -
	// under the test driver, fyne.Do runs its closure synchronously on the
	// calling goroutine instead of handing it off to the UI goroutine, so
	// that read would otherwise race ShowImage()'s write from a different
	// goroutine.
	gen atomic.Uint64

	// unsortedFiles is the raw scan/drop order, kept alongside files so the
	// S key can cycle back to it without rescanning. sortMode picks which
	// ordering files currently holds (see sort.go); it persists across
	// drops instead of resetting, since it's a standing display preference.
	unsortedFiles []fyne.URI
	sortMode      filesort.Mode

	// mergeMode is a standing preference, toggled by M, that makes
	// handleDrop merge newly dropped files into the existing set instead
	// of replacing it; it defaults to false (replace) and persists across
	// drops like sortMode. baseTitle is the window title without the
	// "[merge] " prefix applyTitle adds while mergeMode is on, so toggling
	// M can refresh the title immediately without recomputing it.
	mergeMode bool
	baseTitle string

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
	// goroutine; wired by buildViewer and called from Run's SetOnStopped
	// just before the final preferences save (winPos keeps its last
	// reading, so the save still has a value). Always non-nil after
	// buildViewer - a no-op func when the window isn't a
	// driver.NativeWindow and no poller ever started.
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
	// generation's own channel still gets closed, it just leaves the
	// shared state untouched.
	scanDone chan struct{}
	loadDone chan struct{}

	// animFrame counts every write to v.img.Image - attemptLoad's initial
	// frame plus each one animate cycles to afterwards - and animStopped is
	// closed by animate once it notices its generation is stale and
	// returns. Both exist so tests can synchronize on frame changes and
	// animation shutdown via atomics and channel-close instead of reading
	// v.img.Image directly from another goroutine, which would race with
	// attemptLoad's/animate's writes under the fyne test driver: it runs
	// fyne.Do synchronously on the calling goroutine rather than marshaling
	// onto a single UI thread, so even a read sequenced after done/loadDone
	// closes has no happens-before edge against a concurrently running
	// animate call - only observing animFrame's new value does. Each
	// animate call gets its own captured animStopped (see attemptLoad), so
	// a superseded generation's close can't be mistaken for a newer one's.
	// animStop is the other direction: closing it (stopAnimation, called
	// wherever gen bumps with an animation possibly running) wakes animate
	// out of its frame-delay sleep so it exits immediately instead of up
	// to one full frame delay later; the gen check stays as belt and
	// braces. Only ever swapped on the UI goroutine.
	animFrame   atomic.Uint64
	animStopped chan struct{}
	animStop    chan struct{}

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
	// instead of a fresh disk read plus decode. *lru.Cache is safe for
	// concurrent use on its own, so both attemptLoad's decode goroutine and
	// preloadOne's background goroutines can populate it without going
	// through fyne.Do.
	imgCache *lru.Cache[string, *imaging.LoadedImage]

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
	// updateInfoOverlay callback buildViewer hands it.
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
	// exifLink is the "Show EXIF data" link inside infoCard - see build.go's
	// wiring. Kept as its own field only so tests can trigger it directly
	// (OnTapped) the same way e2e_test.go does for restoreLink, without a
	// real click through the widget tree.
	exifLink        *widget.Hyperlink
	infoCard        *fyne.Container
	currentFileSize int64

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
	// native-dialog goroutine.
	clipboardDone chan struct{}
	chooserDone   chan struct{}

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

	// keyModifiers reports the keyboard modifiers currently held -
	// defaultKeyModifiers (keys.go) in production, stubbed by tests (the
	// fyne test driver can't synthesize modifier state at all). Read by
	// handleKeyEvent's Shift+R, and by the zoom view's Shift+scroll pan
	// through the closure buildViewer hands it.
	keyModifiers func() fyne.KeyModifier
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

// applyTitle (re)applies baseTitle to the window with a sort-mode prefix
// (see filesort.Label - empty, so invisible, for the default by-name sort)
// and the "[merge]"/"[shuffle]" prefixes when merge mode or the
// slideshow's shuffle order (Shift+P) are on, so the title always makes
// the active drop/sort/slideshow mode visible at a glance. The separating
// space is added here rather than baked into either prefix, so neither
// translation key carries trailing whitespace a translator could silently
// drop.
func (v *viewer) applyTitle() {
	title := v.baseTitle
	if v.mergeMode {
		title = lang.L("[merge]") + " " + title
	}
	if v.slides.Shuffle() {
		title = lang.L("[shuffle]") + " " + title
	}
	if p := filesort.Label(v.sortMode); p != "" {
		title = p + " " + title
	}
	v.win.SetTitle(title)
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

	v.gen.Add(1) // invalidate any decode or animation still in flight
	v.stopAnimation()

	v.files = nil
	v.unsortedFiles = nil
	v.index = 0

	v.img.Image = nil
	v.img.Hide()

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
	v.SetMergeMode(!v.mergeMode)
}

// SetMergeMode sets merge mode directly - the settings window's binding for
// the toggle above - and immediately reflects it in the window title via
// the "[merge] " prefix so it doesn't wait for a drop to become visible.
func (v *viewer) SetMergeMode(on bool) {
	v.mergeMode = on
	v.applyTitle()
}

// MergeMode reports whether merge mode is on - the settings window's
// getter.
func (v *viewer) MergeMode() bool {
	return v.mergeMode
}

// showFileIfPresent looks up target in v.files by URI identity and shows it
// if found, reporting whether it was. Used to keep the same file in view
// across an operation - a sort toggle or a merge - that reorders or extends
// v.files without changing what's currently on screen.
func (v *viewer) showFileIfPresent(target fyne.URI) bool {
	for i, u := range v.files {
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
	if len(v.files) == 0 {
		return nil, 0, false
	}

	return v.files[v.index], v.index, true
}

// displayedFile is CurrentFile narrowed to what the EXIF panel needs: a
// file that is not merely selected but actually decoded and on screen.
// The distinction matters during a failed or in-flight load, when v.files
// is non-empty but there is no image to describe.
func (v *viewer) displayedFile() (fyne.URI, bool) {
	if v.img.Image == nil {
		return nil, false
	}

	u, _, ok := v.CurrentFile()

	return u, ok
}

// RemoveFile drops the file at v.files[i] from both v.files and
// v.unsortedFiles, keeping them in sync so a later sort toggle doesn't
// resurrect a file that failed to load. v.files is trimmed by index rather
// than by URI match, since merge mode allows dropping the same file twice
// and a match would risk removing the wrong duplicate; unsortedFiles has
// no equivalent index to use, but any matching duplicate there is an
// equally valid one to drop.
func (v *viewer) RemoveFile(i int) {
	target := v.files[i]
	v.files = append(v.files[:i], v.files[i+1:]...)
	v.imgCache.Remove(target.String())

	for j, u := range v.unsortedFiles {
		if u.String() == target.String() {
			v.unsortedFiles = append(v.unsortedFiles[:j], v.unsortedFiles[j+1:]...)
			break
		}
	}
}

// FileCount, FileAt, CurrentIndex, Generation, Unfocus, and Advance
// complete the exported vocabulary the feature packages' Host interfaces
// bind to (see the note above CurrentFile). internal/ui/grid uses the
// first five: the first three to draw the right cells, Generation to
// discard a decode whose file set has since been replaced, and Unfocus to
// hand the keyboard back after a thumbnail tap. internal/ui/slideshow
// needs only FileCount and Advance.

// FileCount is how many files are currently loaded.
func (v *viewer) FileCount() int {
	return len(v.files)
}

// FileAt returns the file at index i.
func (v *viewer) FileAt(i int) fyne.URI {
	return v.files[i]
}

// CurrentIndex is the index of the file on screen.
func (v *viewer) CurrentIndex() int {
	return v.index
}

// Generation is the current load generation - see the gen field.
func (v *viewer) Generation() uint64 {
	return v.gen.Load()
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
		v.ShowImage(randomOtherIndex(len(v.files), v.index))
		return
	}
	v.ShowImage(v.index + 1)
}
