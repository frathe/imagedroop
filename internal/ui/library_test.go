package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/filesort"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/uitest"
)

// testApp is the one fyne test app every viewer in this suite is built on
// (see newTestUI). Shared rather than created per test because
// test.NewApp() clears process-global caches - painter.ClearFontCache and
// cache.ResetThemeCaches, see fyne's test/app.go - so one app per test
// meant ~180 font-cache rebuilds per run, and every text render after each
// one re-shaping its glyphs from scratch: it cost ~3x on the whole suite.
//
// Sharing is safe because nothing in the viewer writes persistent state
// during a test: preferences.Save and session.Save are only ever called
// from main()'s SetOnStopped. The tests that do assert on persistence
// build their own app (test.NewApp) and run the startup/build path with it -
// keep doing that rather than saving into this one, which every other test
// expects to find empty.
var testApp fyne.App

func TestMain(m *testing.M) {
	testApp = test.NewApp()

	// No global tweaks needed here anymore: the toast auto-hide duration,
	// the folder-scan cap, and the key-modifier reader - all package vars
	// once, all mutated from tests - are per-viewer state now, overridden
	// where each viewer is built (newTestUI, or the individual test).
	os.Exit(m.Run())
}

func TestResizeToImage(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		maxW, maxH   float32
		wantW, wantH float32
	}{
		{"small image is floored to the drop-zone min size", 400, 300, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, startH},
		{"image already exactly at the cap", int(defaultMaxWindowWidth), int(defaultMaxWindowHeight), defaultMaxWindowWidth, defaultMaxWindowHeight, defaultMaxWindowWidth, defaultMaxWindowHeight},
		{"wide image is capped by width", 3000, 950, defaultMaxWindowWidth, defaultMaxWindowHeight, 1500, 475},
		{"tall image is capped by height, then floored to the min width", 950, 3000, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, 950},
		{"large image is capped by whichever dimension needs it most", 3000, 2000, defaultMaxWindowWidth, defaultMaxWindowHeight, 1425, 950},
		{"tiny image is floored to the drop-zone size, not left ungrabbable", 50, 50, defaultMaxWindowWidth, defaultMaxWindowHeight, startW, startH},
		{"a custom, smaller cap is honored instead of the shipped default", 3000, 2000, 900, 700, 900, 600},
		{"a custom, larger cap is honored instead of the shipped default", 3000, 2000, 2800, 2000, 2800, 1866.6667},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			win := test.NewWindow(nil)
			defer win.Close()

			resizeToImage(win, image.Rect(0, 0, c.w, c.h), c.maxW, c.maxH)

			size := win.Canvas().Size()
			if !uitest.ApproxEqual(size.Width, c.wantW) || !uitest.ApproxEqual(size.Height, c.wantH) {
				t.Errorf("resizeToImage(%dx%d, max %vx%v) -> %vx%v, want %vx%v", c.w, c.h, c.maxW, c.maxH, size.Width, size.Height, c.wantW, c.wantH)
			}
		})
	}
}

// TestMaxWindowSizeGetterSetter is MaxWindowWidth/SetMaxWindowWidth and
// MaxWindowHeight/SetMaxWindowHeight - the settings window's binding for
// the cap resizeToImage enforces (tested directly above).
func TestMaxWindowSizeGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if got := v.MaxWindowWidth(); got != defaultMaxWindowWidth {
		t.Errorf("MaxWindowWidth() = %v, want the shipped default %v", got, defaultMaxWindowWidth)
	}
	if got := v.MaxWindowHeight(); got != defaultMaxWindowHeight {
		t.Errorf("MaxWindowHeight() = %v, want the shipped default %v", got, defaultMaxWindowHeight)
	}

	v.SetMaxWindowWidth(1800)
	v.SetMaxWindowHeight(1100)
	if got := v.MaxWindowWidth(); got != 1800 {
		t.Errorf("MaxWindowWidth() = %v, want 1800 after SetMaxWindowWidth(1800)", got)
	}
	if got := v.MaxWindowHeight(); got != 1100 {
		t.Errorf("MaxWindowHeight() = %v, want 1100 after SetMaxWindowHeight(1100)", got)
	}
}

// TestSetMaxWindowSize_FloorsAtTheDropZoneSize guards against a cap below
// startW/startH: resizeToImage never shrinks the window past that regardless
// (see its own "never shrink below the drop-zone size" comment), so a lower
// value would silently have no effect - the setters floor instead, so what
// the settings window shows always matches what the window actually does.
func TestSetMaxWindowSize_FloorsAtTheDropZoneSize(t *testing.T) {
	v := newTestViewer(t)

	v.SetMaxWindowWidth(100)
	v.SetMaxWindowHeight(50)

	if got := v.MaxWindowWidth(); got != startW {
		t.Errorf("MaxWindowWidth() = %v, want it floored to startW (%v)", got, float32(startW))
	}
	if got := v.MaxWindowHeight(); got != startH {
		t.Errorf("MaxWindowHeight() = %v, want it floored to startH (%v)", got, float32(startH))
	}
}

// --- test viewer construction ----------------------------------------------

// newTestUI builds a fresh app and window through the same startup load,
// assembly, and geometry restoration Run uses, without starting runtime
// polling or touching production favorites storage.
//
// It tracks whether the window has already been closed (e.g. by Escape,
// mid-test) so cleanup never closes it a second time: Fyne's test driver's
// removeWindow (test/driver.go) unlocks its windows-list mutex without a
// defer, so a second Close panics partway through and leaves that mutex
// permanently locked - wedging every later test in the package that touches
// a window, not just this one.
func newTestUI(t *testing.T) (v *viewer, win fyne.Window, closed func() bool) {
	t.Helper()

	// Reassert the shared app as the current one before building: the
	// persistence tests construct their own app, and Fyne makes whichever
	// was built last the process-wide current app - which widget internals
	// (theme, driver) read directly. Without this, a test running after one
	// of those would build its widgets against a different app than the one
	// buildViewer was handed. Cheap, unlike test.NewApp - see testApp.
	fyne.SetCurrentApp(testApp)

	v, win = buildStartupViewer(testApp)

	// The auto-hide timer must never fire on its own mid-suite: its inline
	// fyne.Do (under the test driver) would write widgets concurrently with
	// whatever the test goroutine is doing by then. Tests drive the hide
	// synchronously via settleToast instead, so the production duration is
	// irrelevant here - an hour just guarantees a leaked timer sleeps
	// harmlessly until the process exits.
	v.toast.duration = time.Hour

	// Vector re-renders fire from every effective-scale change (a key, a
	// scroll, or a window resize), and the production debounce would leave
	// them still pending when a test asserts on vectorRaster/vectorPending
	// moments later - zeroed here the same way the toast's duration is.
	v.vectorDebounce = 0

	// setAsWallpaper writes a PNG it then hands to the OS, and unlike every
	// other file this suite produces that one is meant to outlive the
	// process - so it is redirected out of the user's real cache directory
	// here, the same way the toast's duration is neutralized above.
	// wallpaper.Set itself is stubbed per-test (uitest.StubWallpaperSet), so
	// nothing here ever reaches the desktop.
	v.wallpaperDir = t.TempDir()

	var isClosed bool
	win.SetOnClosed(func() { isClosed = true })

	t.Cleanup(func() {
		if !isClosed {
			win.Close()
		}
	})

	// Registered after the close above so it runs *before* it (t.Cleanup is
	// LIFO): drain whatever this test left in flight while its window is
	// still alive. Not every test waits for the work it starts - asserting
	// that a key is a no-op, say, needs no load to finish - and a decode
	// goroutine outliving its test goes on to run finishLoad/ForceRepaint
	// (inline, under the test driver's fyne.Do) while the *next* test is
	// building its own viewer, which is a genuine race between two tests
	// rather than anything production does wrong.
	t.Cleanup(func() { drain(t, v) })

	return v, win, func() bool { return isClosed }
}

// drain waits out every background operation this viewer may still have in
// flight. Each wait is individually optional - a viewer that never scanned
// has a nil scanDone - but the set is exhaustive on purpose: it is the
// backstop that keeps one test's goroutines out of the next one, whatever
// that test happened to exercise.
func drain(t *testing.T, v *viewer) {
	t.Helper()

	// Supersede any in-flight decode/retry chain first, so a load that was
	// deliberately abandoned mid-test (a broken-file retry loop, say) stops
	// re-entering rather than being waited out step by step - invalidateLoad
	// also cancels its context, so an abandoned decode/preload actually
	// stops doing I/O instead of just being ignored once it finishes. The
	// slideshow is asked to stop for the same reason, on this goroutine,
	// since leaving picture-frame mode touches the window.
	v.invalidateLoad()
	v.scanLifecycle.invalidate()
	v.sortLifecycle.invalidate()
	v.vectorLifecycle.invalidate()
	v.favThumbLifecycle.invalidate()
	v.slides.Exit()

	// Vector re-renders: spawned by any effective-scale change, so a test
	// that zoomed or resized may still have one in flight. Must stay below
	// invalidateLoad and slides.Exit above: only once no superseded decode
	// can still land in finishLoad (whose resize triggers a scale change)
	// and no slideshow advance can start a load is this Wait racing no
	// further Add.
	v.vectorPending.Wait()

	for _, c := range []struct {
		name string
		ch   chan struct{}
	}{
		{"scan", v.scanDone},
		{"sort", v.sortDone},
		{"load", v.loadDone},
		{"clipboard copy", v.clipboardDone},
		{"file chooser", v.chooserDone},
		{"wallpaper", v.wallpaperDone},
		{"favorite previews", v.favThumbDone},
	} {
		if c.ch == nil {
			continue
		}
		select {
		case <-c.ch:
		case <-time.After(testTimeout):
			t.Fatalf("timed out draining the %s goroutine at cleanup", c.name)
		}
	}

	settled := make(chan struct{})
	go func() {
		v.preloadPending.Wait()
		v.grid.Settle()
		v.slides.Settle()
		close(settled)
	}()

	select {
	case <-settled:
	case <-time.After(testTimeout):
		t.Fatal("timed out draining preload/thumbnail/slideshow goroutines at cleanup")
	}
}

// newTestViewer is newTestUI for the majority of tests, which drive the
// viewer directly and never need the window handle or the closed-reporter.
func newTestViewer(t *testing.T) *viewer {
	t.Helper()

	v, _, _ := newTestUI(t)

	return v
}

// --- viewer.handleDrop (synchronous behaviour) -----------------------------

func TestHandleDrop_EmptyDrop(t *testing.T) {
	v := newTestViewer(t)

	v.handleDrop(nil)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after an empty drop", v.state.files)
	}

	if n := len(v.win.Canvas().Overlays().List()); n != 0 {
		t.Errorf("overlays = %d, want 0 after an empty drop", n)
	}
}

func TestHandleDrop_NoSupportedImages(t *testing.T) {
	v := newTestViewer(t)

	v.handleDrop([]fyne.URI{
		uitest.FakeURI{FileName: "a.txt", Ext: ".txt"},
		uitest.FakeURI{FileName: "b.pdf", Ext: ".pdf"},
	})
	waitForScan(t, v)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil when nothing dropped is a supported image", v.state.files)
	}

	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown when nothing dropped is a supported image")
	}

	if !v.dropzone.Visible() {
		t.Error("dropzone (\"Drop images here\") should be restored once the scan finds nothing to show")
	}
	settleToast(t, v)
}

func TestHandleDrop_ErrorAfterImagesClearsDisplay(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	dropAndWait(t, v, jpegURI)

	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded before the second, bad drop")
	}

	// A second drop that yields nothing displayable must not leave the
	// previous image sitting behind the error toast and placeholder art.
	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after a drop with nothing supported", v.state.files)
	}
	if v.img.Image != nil {
		t.Error("the previous image should be cleared, not left showing behind the error")
	}
	if v.img.Visible() {
		t.Error("the previous image should be hidden, not left showing behind the error")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again after the error")
	}
	if !v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be shown in place of the cleared image")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown for the bad drop")
	}
	settleToast(t, v)
}

func TestHandleDrop_FiltersUnsupportedFiles(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "keep.jpg", 4, 4, color.White)

	v.handleDrop([]fyne.URI{
		jpegURI,
		uitest.FakeURI{FileName: "skip.txt", Ext: ".txt"},
	})
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 1 || v.state.files[0].Name() != jpegURI.Name() {
		t.Errorf("files = %v, want only %q kept", v.state.files, jpegURI.Name())
	}
}

func TestHandleDrop_AcceptsPNGAndGIF(t *testing.T) {
	v := newTestViewer(t)

	pngPath := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 4, color.White))
	gifPath := uitest.WriteTempFile(t, "b.gif", uitest.EncodeGIF(t, 4, 4, color.White))

	dropAndWait(t, v, storage.NewFileURI(pngPath), storage.NewFileURI(gifPath))

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want both the PNG and the GIF kept", v.state.files)
	}
}

// --- handleDrop merge vs. replace (M toggles merge mode) --------------------

func TestHandleDrop_SecondDropWithoutMergeModeReplaces(t *testing.T) {
	v := newTestViewer(t)

	first := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, first)

	second := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	v.handleDrop([]fyne.URI{second}) // mergeMode defaults to false
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 1 || v.state.files[0].Name() != "b.jpg" {
		t.Errorf("files = %v, want only %q - the second drop should replace the first", v.state.files, "b.jpg")
	}
}

func TestHandleDrop_MergeModeMergesIntoExistingSet(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	v.state.SetMergeMode(true)
	dropAndWait(t, v, b)

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want both a.jpg and b.jpg after a merge-mode drop", v.state.files)
	}

	// The merge should have jumped to the file just added, not stayed on a.jpg.
	if got := v.state.files[v.state.index].Name(); got != "b.jpg" {
		t.Errorf("displayed file = %q, want b.jpg (the just-merged file) in view", got)
	}
}

func TestHandleDrop_MergeModeDropWithNothingSupportedKeepsExistingSet(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.state.SetMergeMode(true)
	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if len(v.state.files) != 1 || v.state.files[0].Name() != "a.jpg" {
		t.Errorf("files = %v, want the existing a.jpg untouched by a merge-mode drop with nothing supported", v.state.files)
	}
	if v.img.Image == nil {
		t.Error("the existing image should stay displayed, not cleared, when a merge-mode drop finds nothing new")
	}
	if v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should not appear - this isn't an empty-state error, just nothing to add")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast explaining nothing supported was found")
	}
	settleToast(t, v)
}

func TestToggleMergeMode_PrefixesTitleAndPersistsAcrossDrops(t *testing.T) {
	v := newTestViewer(t)

	if title := v.win.Title(); strings.Contains(title, "[merge]") {
		t.Fatalf("title = %q, should not start prefixed before M is ever pressed", title)
	}

	// M works even with nothing loaded yet, and takes effect immediately.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyM})
	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Fatalf("title = %q, want it prefixed with [merge] right after toggling M", title)
	}

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Errorf("title = %q, want the [merge] prefix to persist once files are loaded", title)
	}

	// M again turns it back off.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyM})
	if title := v.win.Title(); strings.Contains(title, "[merge]") {
		t.Errorf("title = %q, want the [merge] prefix gone after toggling M again", title)
	}
}

// TestMergeModeGetterSetter is MergeMode/SetMergeMode - the settings
// window's binding - as opposed to toggleMergeMode's own M-key flip already
// covered above.
func TestMergeModeGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if v.MergeMode() {
		t.Fatal("MergeMode() = true, want false by default")
	}

	v.SetMergeMode(true)
	if !v.MergeMode() {
		t.Error("MergeMode() = false, want true after SetMergeMode(true)")
	}
	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Errorf("title = %q, want it prefixed right after SetMergeMode(true)", title)
	}

	v.SetMergeMode(false)
	if v.MergeMode() {
		t.Error("MergeMode() = true, want false after SetMergeMode(false)")
	}
}

// --- info overlay (I) -------------------------------------------------

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1500, "1.5 KiB"},
		{1_048_576, "1.0 MiB"},
		{1_500_000, "1.4 MiB"},
		{1_073_741_824, "1.0 GiB"},
	}

	for _, c := range cases {
		if got := formatFileSize(c.n); got != c.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded guards against I turning
// the card on before there's anything for it to describe: pressed before
// the first drop (allowed, like M/P - see handleKeyEvent), the preference
// should be recorded but the card must stay hidden until an image actually
// loads.
func TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded(t *testing.T) {
	v := newTestViewer(t)

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyI})
	if !v.infoVisible {
		t.Fatal("infoVisible should flip on right away, even with nothing loaded")
	}
	if v.infoCard.Visible() {
		t.Fatal("infoCard should stay hidden until an image is actually on screen")
	}

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)

	if !v.infoCard.Visible() {
		t.Error("infoCard should appear once the first image loads, since the toggle was already on")
	}
}

// TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation covers the
// card's actual content (filename+position, pixel dimensions, file size,
// zoom) and that it keeps itself current across a navigation instead of
// freezing on whatever the first image showed.
func TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 80, 10, color.White)
	dropAndWait(t, v, a, b)

	v.toggleInfoOverlay()
	if !v.infoCard.Visible() {
		t.Fatal("infoCard should be visible right after toggling on with an image already loaded")
	}

	aInfo, err := os.Stat(a.Path())
	if err != nil {
		t.Fatalf("stat a.jpg: %v", err)
	}
	// The zoom line's own value is whatever fit scale the test window's
	// size works out to, so it's read back rather than pinned: what this
	// test is about is the card's content and that it stays current. The
	// fit math itself is internal/ui/zoom's to test, against a viewport it
	// can actually control.
	want := fmt.Sprintf("a.jpg  (1/2)\n40 x 20\n%s\nZoom: %d%%", formatFileSize(aInfo.Size()), v.zoom.Percent())
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText = %q, want %q", got, want)
	}

	// Step to the second file: the card must refresh, not keep showing a's
	// info.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)
	v.updateInfoOverlay()

	bInfo, err := os.Stat(b.Path())
	if err != nil {
		t.Fatalf("stat b.jpg: %v", err)
	}
	want = fmt.Sprintf("b.jpg  (2/2)\n80 x 10\n%s\nZoom: %d%%", formatFileSize(bInfo.Size()), v.zoom.Percent())
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText after navigating = %q, want %q", got, want)
	}

	// Toggling off hides it; toggling back on immediately re-shows current info.
	v.toggleInfoOverlay()
	if v.infoCard.Visible() {
		t.Fatal("infoCard should hide once toggled off")
	}
	v.toggleInfoOverlay()
	if !v.infoCard.Visible() {
		t.Fatal("infoCard should reappear once toggled back on")
	}
	if got := v.infoText.Text; got != want {
		t.Errorf("infoText after re-enabling = %q, want %q (still on b.jpg)", got, want)
	}
}

// TestToggleInfoOverlay_ZoomLineTracksZoomChanges checks the last line
// updates with every zoom mutator (ActualSize, In, FitToWindow), not just
// at load time.
func TestToggleInfoOverlay_ZoomLineTracksZoomChanges(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 400, 200, color.White)
	dropAndWait(t, v, a)

	// The fit percentage depends on the test window's size, so it's read
	// back once here and used as the anchor the last step returns to.
	// Actual size (100%) is the one value that's the same everywhere.
	fitPct := fmt.Sprintf("Zoom: %d%%", v.zoom.Percent())

	v.toggleInfoOverlay()

	if !strings.HasSuffix(v.infoText.Text, fitPct) {
		t.Errorf("infoText = %q, want it to end with the %q fit scale", v.infoText.Text, fitPct)
	}

	v.zoom.ActualSize()
	if !strings.HasSuffix(v.infoText.Text, "Zoom: 100%") {
		t.Errorf("infoText after ActualSize = %q, want it to end with 100%%", v.infoText.Text)
	}

	v.zoom.In()
	if v.zoom.Percent() <= 100 {
		t.Fatalf("setup: zoom percent after In = %d, want more than 100", v.zoom.Percent())
	}
	want := fmt.Sprintf("Zoom: %d%%", v.zoom.Percent())
	if !strings.HasSuffix(v.infoText.Text, want) {
		t.Errorf("infoText after In = %q, want it to end with %q", v.infoText.Text, want)
	}

	v.zoom.FitToWindow()
	if !strings.HasSuffix(v.infoText.Text, fitPct) {
		t.Errorf("infoText after FitToWindow = %q, want back to %q", v.infoText.Text, fitPct)
	}
}

// TestClearToDropzone_HidesInfoCardButKeepsThePreference guards the reset
// (Escape) path: the card must disappear along with the image, but the I
// preference itself is a standing one - like naturalSort/mergeMode - so a
// fresh drop afterward should bring the card straight back.
func TestClearToDropzone_HidesInfoCardButKeepsThePreference(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 40, 20, color.White)
	dropAndWait(t, v, a)
	v.toggleInfoOverlay()

	v.reset()

	if !v.infoVisible {
		t.Error("infoVisible preference should survive a reset")
	}
	if v.infoCard.Visible() {
		t.Error("infoCard should be hidden once reset back to the empty drop zone")
	}

	b := uitest.TempJPEGURI(t, "b.jpg", 40, 20, color.White)
	dropAndWait(t, v, b)

	if !v.infoCard.Visible() {
		t.Error("infoCard should reappear on the next load since the preference was still on")
	}
}

func TestHandleDrop_RecursesIntoNestedDirectories(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	for i := range 3 {
		dir := filepath.Join(root, fmt.Sprintf("sub%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Non-image clutter that a real photo folder always contains -
		// directories and files like these have no recognized extension,
		// so imaging.IsSupportedImage must not open them to find out.
		if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("photo%d.jpg", i)), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 3 {
		t.Fatalf("files = %v, want the 3 nested photos, none of the .DS_Store junk", v.state.files)
	}

	if v.dropzone.Visible() {
		t.Error("dropzone (\"Drop images here\") should be hidden once an image is showing")
	}
}

// TestHandleDrop_SymlinkCycleDoesNotHang guards the visitedDirs check in
// handleDrop: a symlink back to an ancestor directory turns the recursive
// expansion into a cycle (listing root/loop lists root again, including
// root/loop, forever) unless each directory's real, symlink-resolved path is
// tracked and a repeat visit is skipped.
func TestHandleDrop_SymlinkCycleDoesNotHang(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 1 {
		t.Fatalf("files = %v, want the 1 real photo, not one entry per pass through the symlink cycle", v.state.files)
	}
}

// TestHandleDrop_CapsFileCountForLargeTrees exercises maxScan, the safety
// valve that stops a recursive scan once it's gathered enough images -
// shrunk here so the test doesn't need to create tens of thousands of temp
// files to hit it. A per-viewer field, so no global to save and restore.
func TestHandleDrop_CapsFileCountForLargeTrees(t *testing.T) {
	v := newTestViewer(t)
	v.maxScan = 3

	root := t.TempDir()
	for i := range 5 {
		name := filepath.Join(root, fmt.Sprintf("photo%d.jpg", i))
		if err := os.WriteFile(name, uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 3 {
		t.Fatalf("files = %d, want the scan to stop at maxScan (3)", len(v.state.files))
	}

	if !v.toast.card.Visible() {
		t.Fatal("want a toast warning that the scan was truncated")
	}
	if !strings.Contains(v.toast.text.Text, "3") {
		t.Errorf("toast text = %q, want it to mention the cap (3)", v.toast.text.Text)
	}
	settleToast(t, v)
}

// TestMaxScanGetterSetter is MaxScan/SetMaxScan - the settings window's
// binding for the same v.maxScan field TestHandleDrop_CapsFileCountForLargeTrees
// above exercises by writing it directly.
func TestMaxScanGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if got := v.MaxScan(); got != defaultMaxScannedFiles {
		t.Errorf("MaxScan() = %d, want the shipped default %d", got, defaultMaxScannedFiles)
	}

	v.SetMaxScan(5)
	if got := v.MaxScan(); got != 5 {
		t.Errorf("MaxScan() = %d, want 5 after SetMaxScan(5)", got)
	}
}

// TestSetMaxScan_FloorsAtOne guards the scan path's own n >= v.maxScan
// check (drop.go): a 0 or negative cap would stop a scan before it ever
// gathered anything, which isn't what a settings-window typo should do.
func TestSetMaxScan_FloorsAtOne(t *testing.T) {
	v := newTestViewer(t)

	v.SetMaxScan(0)
	if got := v.MaxScan(); got != 1 {
		t.Errorf("MaxScan() = %d, want it floored to 1 for a 0 input", got)
	}

	v.SetMaxScan(-5)
	if got := v.MaxScan(); got != 1 {
		t.Errorf("MaxScan() = %d, want it floored to 1 for a negative input", got)
	}
}

// --- invalidateLoad (load-request cancellation) ----------------------------

// TestInvalidateLoad_CancelsPriorLoadContext checks invalidateLoad's own
// contract: advance the lifecycle and cancel the previous request token.
func TestInvalidateLoad_CancelsPriorLoadContext(t *testing.T) {
	v := newTestViewer(t)

	token := v.loadLifecycle.begin()

	got := v.invalidateLoad()

	if token.context().Err() == nil {
		t.Error("invalidateLoad should cancel the previous generation's load context")
	}
	if got != token.revision+1 {
		t.Errorf("invalidateLoad() = %d, want %d", got, token.revision+1)
	}
	if v.loadLifecycle.currentRevision() != got {
		t.Errorf("load revision = %d, want %d", v.loadLifecycle.currentRevision(), got)
	}
}

// TestInvalidateLoad_ZeroValueIsSafe covers the state before any image has
// ever been shown.
func TestInvalidateLoad_ZeroValueIsSafe(t *testing.T) {
	v := newTestViewer(t)

	v.invalidateLoad() // must not panic
}

// TestShowImage_StartsLoadLifecycle checks that navigation owns a cancellable
// lifecycle request rather than relying only on a revision comparison.
func TestShowImage_StartsLoadLifecycle(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.loadLifecycle.mu.Lock()
	hasCancel := v.loadLifecycle.cancel != nil
	v.loadLifecycle.mu.Unlock()
	if !hasCancel {
		t.Error("ShowImage should leave a cancellable load request for preloads and animation")
	}
}

// TestCancelScan_NoOpWhenNotScanning covers the guard at the top of
// cancelScan: calling it with no scan in flight (the common case - Escape's
// other two branches, close/reset, are what normally run) must do nothing,
// not bump gen or raise a spurious "cancelled scanning" toast.
func TestCancelScan_NoOpWhenNotScanning(t *testing.T) {
	v := newTestViewer(t)
	revisionBefore := v.scanLifecycle.currentRevision()

	v.cancelScan()

	if v.scanLifecycle.currentRevision() != revisionBefore {
		t.Error("cancelScan should not invalidate the scan lifecycle when nothing is scanning")
	}
	if v.toast.card.Visible() {
		t.Error("cancelScan should not raise a toast when nothing is scanning")
	}
}

// TestCancelScan_CancelsInFlightScanWithNoFilesYet drives cancelScan
// directly against the UI state handleDrop leaves in place while its scan
// is still in flight (token started, spinner/counter shown, drop zone hidden),
// without racing handleDrop's own background goroutine to reproduce that
// state - see the note on TestHandleDrop_SupersededScanGoroutineExits below
// for why the goroutine itself is exercised separately instead.
func TestCancelScan_CancelsInFlightScanWithNoFilesYet(t *testing.T) {
	v := newTestViewer(t)

	token := v.scanLifecycle.begin()
	v.scanning = true
	v.scanSpinner.Show()
	v.scanLabel.Show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()

	v.cancelScan()

	if v.scanning {
		t.Error("scanning should be false after cancelScan")
	}
	if v.scanSpinner.Visible() || v.scanLabel.Visible() {
		t.Error("scan spinner/label should be hidden after cancelScan")
	}
	if !v.dropzone.Visible() || !v.welcomeArt.Visible() {
		t.Error("drop zone/welcome art should be restored after cancelling a scan that had no files loaded yet")
	}
	if token.current() || token.context().Err() == nil {
		t.Error("cancelScan should cancel and supersede the in-flight scan token")
	}
	if !v.toast.card.Visible() {
		t.Error("want a toast confirming the scan was cancelled")
	}
	if !strings.Contains(v.toast.text.Text, "cancelled") {
		t.Errorf("toast text = %q, want it to mention the cancellation", v.toast.text.Text)
	}
	settleToast(t, v)
}

// TestCancelScan_PreservesExistingFilesInMergeMode checks that cancelling a
// merge-mode scan never touches files already loaded before that scan
// started - unlike reset (Escape with no scan running), which always clears
// back to the drop zone.
func TestCancelScan_PreservesExistingFilesInMergeMode(t *testing.T) {
	v := newTestViewer(t)

	existing := uitest.TempJPEGURI(t, "existing.jpg", 4, 4, color.White)
	v.state.files = []fyne.URI{existing}
	v.state.unsortedFiles = []fyne.URI{existing}
	v.dropzone.Hide()

	v.scanning = true
	v.scanSpinner.Show()
	v.scanLabel.Show()

	v.cancelScan()

	if len(v.state.files) != 1 || v.state.files[0].String() != existing.String() {
		t.Errorf("files = %v, want the pre-existing file untouched by cancelling a merge-mode scan", v.state.files)
	}
	if v.dropzone.Visible() {
		t.Error("drop zone should stay hidden - an image was already loaded before the cancelled scan started")
	}
	if v.scanning {
		t.Error("scanning should be false after cancelScan")
	}
	settleToast(t, v)
}

// TestHandleDrop_SupersededScanGoroutineExits drops a folder large enough to
// force several storage.List round trips, then immediately drops a second,
// unrelated file before the first scan can finish. gen is bumped
// synchronously by the second handleDrop call, on this same goroutine,
// before the first scan's background goroutine has any chance to run -
// so by the time that goroutine makes its first post-bump gen check
// (whichever of the several in handleDrop it reaches first), it's
// already stale. This exercises the gen check inside the directory-walk
// loop (added so a superseded scan stops touching the filesystem instead
// of racing a large tree to completion for a discarded result) without
// depending on real-time scheduling to land the cancellation mid-scan.
func TestHandleDrop_SupersededScanGoroutineExits(t *testing.T) {
	v := newTestViewer(t)

	rootA := t.TempDir()
	for i := range 20 {
		sub := filepath.Join(rootA, fmt.Sprintf("d%d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "photo.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	v.handleDrop([]fyne.URI{storage.NewFileURI(rootA)})
	scanDoneA := v.scanDone

	jpegB := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, jpegB)

	select {
	case <-scanDoneA:
	case <-time.After(5 * time.Second):
		t.Fatal("superseded scan's goroutine never exited - scanDone was never closed")
	}

	if len(v.state.files) != 1 || v.state.files[0].String() != jpegB.String() {
		t.Errorf("files = %v, want only the second drop's file applied", v.state.files)
	}
}

// TestNavigationDoesNotInvalidateScan pins the lifecycle split: a user may
// browse an existing set while a merge-mode directory scan is in flight, and
// that navigation must not silently strand scanning=true or discard the scan.
func TestNavigationDoesNotInvalidateScan(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	scanToken := v.scanLifecycle.begin()
	v.scanning = true

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	if !scanToken.current() {
		t.Fatal("navigation invalidated an unrelated in-flight scan")
	}
	if !v.scanning {
		t.Fatal("navigation cleared scanning before the scan completed")
	}

	v.cancelScan()
	settleToast(t, v)
}

// TestHandleDrop_DedupesOverlappingDirectories drops a folder together with
// one of its own subfolders in the same call - a folder tree reached via two
// different dropped paths - and checks the subfolder's photo isn't counted
// twice in the resulting set.
func TestHandleDrop_DedupesOverlappingDirectories(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}

	dropAndWait(t, v, storage.NewFileURI(root), storage.NewFileURI(sub))

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want top.jpg and nested.jpg once each, not nested.jpg twice from the overlapping drop", v.state.files)
	}
}

// TestHandleDrop_DedupesDuplicateURIsInDirectDrop covers the fast (no
// directories) path in handleDrop: passing the same file twice in one drop -
// which os.Args launch or a native chooser's output could in principle
// produce - should not add it to v.state.files twice.
func TestHandleDrop_DedupesDuplicateURIsInDirectDrop(t *testing.T) {
	v := newTestViewer(t)

	photo := uitest.TempJPEGURI(t, "photo.jpg", 4, 4, color.White)

	dropAndWait(t, v, photo, photo)

	if len(v.state.files) != 1 {
		t.Fatalf("files = %v, want the duplicate URI collapsed to a single entry", v.state.files)
	}
}

// --- viewer.ShowImage (async decode path) -----------------------------------
//
// ShowImage() and handleDrop() decode/scan off the main goroutine and apply
// their result via fyne.Do, closing loadDone/scanDone as the last thing that
// completion block does. Waiting on those channels - rather than polling
// v.loading or widget visibility - gives the receive a proper
// happens-before relationship with everything the producer goroutine wrote,
// so these tests are race-free under the test driver's fyne.Do, which (unlike
// the real app drivers) runs synchronously on the calling goroutine instead
// of marshaling onto a single GUI goroutine.

// testTimeout is the deadline every wait helper below gives its operation.
// One value for all of them, rather than a per-call argument: a timeout
// here is a failure deadline, not a delay - a passing test returns as soon
// as its channel closes and never waits this long - so a single generous
// value costs nothing and keeps the call sites free of a number that
// suggested a tuning knob nobody was actually turning.
const testTimeout = 5 * time.Second

// dropAndWait drops uris and waits for the resulting scan, reorder and load
// to finish - the opening lines of nearly every test in this suite. The sort
// step is part of the chain because applyScanResult hands the scanned files
// to startSort, which only shows the first image once the reorder lands.
// Use dropAndWaitScan instead when the drop is expected to load nothing
// (no supported images), since neither sortDone nor loadDone is touched in
// that case.
func dropAndWait(t *testing.T, v *viewer, uris ...fyne.URI) {
	t.Helper()

	v.handleDrop(uris)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
}

// dropAndWaitScan drops uris and waits only for the scan, for drops that
// end with nothing displayable - an unsupported file, an empty folder, a
// merge that adds nothing. Deliberately no waitForSort: applyScanResult
// returns before ever reaching startSort in that case, so v.sortDone is left
// holding whatever channel some earlier call put there.
func dropAndWaitScan(t *testing.T, v *viewer, uris ...fyne.URI) {
	t.Helper()

	v.handleDrop(uris)
	waitForScan(t, v)
}

func waitUntilLoaded(t *testing.T, v *viewer) {
	t.Helper()

	select {
	case <-v.loadDone:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the image to finish loading")
	}

	// Also wait out the neighbor preloads finishLoad kicked off (they're
	// registered with preloadPending before loadDone closes): a preload
	// goroutine that outlives its test keeps reading files - and shared
	// library state like the MIME map - under whatever test runs next,
	// which -race rightly reports. "Loaded" here deliberately means
	// "loaded, and everything that load spawned has settled".
	settled := make(chan struct{})
	go func() {
		v.preloadPending.Wait()
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for neighbor preloads to settle")
	}
}

func waitForScan(t *testing.T, v *viewer) {
	t.Helper()

	select {
	case <-v.scanDone:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for scan to finish")
	}
}

func waitForSort(t *testing.T, v *viewer) {
	t.Helper()

	select {
	case <-v.sortDone:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for sort to finish")
	}
}

// settleToast finishes the current toast deterministically: it cancels the
// pending auto-hide timer, waits for that goroutine to exit, then runs the
// hide synchronously through the same autoHide path the timer would have
// taken. Any test that triggers a toast should call this before returning.
// It replaces the old real-time wait-for-auto-hide design, which both kept
// ~2s of wall-clock per toast test (a shortened global duration) and let
// the timer's inline fyne.Do perform widget writes concurrently with the
// test goroutine's own - the suite's dominant source of -race failures
// before stage 2 (concurrent access to Fyne/harfbuzz's shared text-shaping
// state included).
func settleToast(t *testing.T, v *viewer) {
	t.Helper()

	if v.toast.stop == nil {
		t.Fatal("no toast auto-hide pending to settle - was a toast actually shown?")
	}

	v.toast.cancelAutoHide()

	select {
	case <-v.toast.done:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the toast's auto-hide goroutine to exit")
	}

	v.toast.autoHide(v.toast.gen.Load())
}

// settleChooser waits for openFileDialog's background goroutine to finish.
// Signalling from inside a filepicker.Choose stub is not enough: the stub
// returns first, and the error path renders a toast afterwards - so a test
// that only waited on its own stub channel left that rendering running
// concurrently with whatever came next.
func settleChooser(t *testing.T, v *viewer) {
	t.Helper()

	if v.chooserDone == nil {
		t.Fatal("no file-chooser goroutine pending to settle")
	}

	select {
	case <-v.chooserDone:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the file-chooser goroutine to finish")
	}
}

// settleSlideshow leaves picture-frame mode (a no-op when it's already
// off) and waits for the session's auto-advance goroutine to exit. Every
// test that enters picture-frame mode registers this as a cleanup:
// without it the goroutine outlives the test, sleeps out its interval
// (10s default), and then wakes to advance a slide - full inline-fyne.Do
// UI work - in the middle of whatever test is running by then.
//
// Exit runs on this goroutine, since it un-full-screens the window; only
// the wait is handed off, so a goroutine that never notices fails the test
// instead of hanging it.
func settleSlideshow(t *testing.T, v *viewer) {
	t.Helper()

	v.slides.Exit()

	settled := make(chan struct{})
	go func() {
		v.slides.Settle()
		close(settled)
	}()

	select {
	case <-settled:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the slideshow goroutine to exit")
	}
}

// warmThumbs decodes every current file's thumbnail synchronously into the
// grid's cache, so opening the grid afterwards populates each cell from
// the cache without spawning decode goroutines. That matters under the
// fyne test driver: a spawned decode's completion paint runs inline on the
// decode goroutine and can interleave with the very cell-refresh walk that
// spawned it - a race that is already over before any post-hoc wait could
// begin, so it can only be prevented, not waited out. The async decode path
// itself is still covered by TestRequestThumbnail_DecodesInBackgroundAndCaches,
// which drives requestThumbnail directly while the main goroutine stays
// quiescent.
func warmThumbs(t *testing.T, v *viewer) {
	t.Helper()

	if err := v.grid.Warm(); err != nil {
		t.Fatalf("warming thumbnails: %v", err)
	}
}

// waitForAnimFrame polls v.animFrame - an atomic counter animate bumps after
// every frame write - until it reaches at least n. Polling the atomic is
// race-free, unlike reading v.img.Image directly from the test goroutine
// while animate's own goroutine writes it.
func waitForAnimFrame(t *testing.T, v *viewer, n uint64) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for v.animFrame.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for animFrame to reach %d", n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitForAnimStopped waits for the current animate call to close
// v.animStopped, which it does right before returning once it notices its
// generation is stale.
func waitForAnimStopped(t *testing.T, v *viewer) {
	t.Helper()

	select {
	case <-v.animStopped:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for animation to stop")
	}
}

func TestViewerShow_LoadsAndNavigates(t *testing.T) {
	v := newTestViewer(t)

	// Named so natural sort (the default) keeps them in drop order - this
	// test is about ShowImage's load/navigate/wraparound behavior, not
	// sorting, which gets its own test below.
	first := uitest.TempJPEGURI(t, "1.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	second := uitest.TempJPEGURI(t, "2.jpg", 20, 10, color.RGBA{G: 255, A: 255})
	third := uitest.TempJPEGURI(t, "3.jpg", 15, 25, color.RGBA{B: 255, A: 255})

	dropAndWait(t, v, first, second, third)

	if v.state.index != 0 {
		t.Fatalf("index = %d, want 0 after the initial drop", v.state.index)
	}
	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded")
	}
	if b := v.img.Image.Bounds(); b.Dx() != 10 || b.Dy() != 10 {
		t.Errorf("loaded image size = %dx%d, want 10x10", b.Dx(), b.Dy())
	}
	if v.dropzone.Visible() {
		t.Error("dropzone should be hidden once an image is showing")
	}

	// Step forward to the second image.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.state.index != 1 {
		t.Fatalf("index = %d, want 1 after stepping forward", v.state.index)
	}
	if b := v.img.Image.Bounds(); b.Dx() != 20 || b.Dy() != 10 {
		t.Errorf("loaded image size = %dx%d, want 20x10", b.Dx(), b.Dy())
	}

	// Right at the end wraps around to the first image.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.state.index != 0 {
		t.Fatalf("index = %d, want wraparound to 0", v.state.index)
	}

	// Left from the first image wraps around to the last one.
	v.ShowImage(v.state.index - 1)
	waitUntilLoaded(t, v)

	if v.state.index != 2 {
		t.Fatalf("index = %d, want wraparound to the last index (2)", v.state.index)
	}
	if b := v.img.Image.Bounds(); b.Dx() != 15 || b.Dy() != 25 {
		t.Errorf("loaded image size = %dx%d, want 15x25", b.Dx(), b.Dy())
	}
}

// --- sorting ----------------------------------------------------------

func TestHandleDrop_NaturalSortsByDefault(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	img2 := uitest.TempJPEGURI(t, "IMG_2.jpg", 4, 4, color.White)

	// Dropped out of numeric order, the way an unsorted OS listing might
	// hand them over.
	dropAndWait(t, v, img10, img1, img2)

	var got []string
	for _, u := range v.state.files {
		got = append(got, u.Name())
	}
	want := []string{"IMG_1.jpg", "IMG_2.jpg", "IMG_10.jpg"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want natural-sorted %v", got, want)
	}
}

// TestToggleSort_CyclesThroughAllModesAndBackToName exercises S cycling
// through every sortMode and back, plus the title-bar label each one shows.
// It deliberately doesn't try to assert a distinct file order for each
// mode - see TestOrderedFiles_SortsByCaptureDate/SortsByModTime/SortsBySize
// for that, with controlled inputs. Instead it leans on all three images
// being pixel-identical (same size/color, no Exif) and created in this
// exact order (img10, then img1, then img2): every non-name mode ties
// across all three files under its own key (capture date falls back to an
// equal mtime; mtime and size are both literally equal), so
// sort.SliceStable's tie-break puts them all back in that same creation
// order - letting one test cover mode cycling, "keep the current file in
// view", and the title label together without a real filesystem race.
func TestToggleSort_CyclesThroughAllModesAndBackToName(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	img2 := uitest.TempJPEGURI(t, "IMG_2.jpg", 4, 4, color.White)

	dropOrder := []fyne.URI{img10, img1, img2}
	v.handleDrop(dropOrder)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	namesOf := func(files []fyne.URI) []string {
		names := make([]string, len(files))
		for i, u := range files {
			names[i] = u.Name()
		}
		return names
	}

	natural := []string{"IMG_1.jpg", "IMG_2.jpg", "IMG_10.jpg"}
	scanOrder := namesOf(dropOrder) // IMG_10.jpg, IMG_1.jpg, IMG_2.jpg

	if got := namesOf(v.state.files); !slices.Equal(got, natural) {
		t.Fatalf("files = %v, want natural-sorted %v before any toggle", got, natural)
	}
	if v.state.SortMode() != filesort.ByName {
		t.Fatalf("sortMode = %v, want filesort.ByName before any toggle", v.state.SortMode())
	}

	// Step onto IMG_2.jpg (index 1 in natural order, i.e. position 2/3)
	// before cycling, so "keep the current file in view" is exercised
	// throughout.
	v.ShowImage(1)
	waitUntilLoaded(t, v)
	if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
		t.Fatalf("displayed file = %q, want IMG_2.jpg before cycling", got)
	}
	if title := v.win.Title(); !strings.Contains(title, "(2/3)") {
		t.Fatalf("title = %q, want it to contain (2/3) before cycling", title)
	}

	// Every mode below resolves to scanOrder (see the comment above), which
	// puts IMG_2.jpg last - position 3/3.
	steps := []struct {
		mode  filesort.Mode
		label string
	}{
		{filesort.ByCaptureDate, "[sort: date]"},
		{filesort.ByModTime, "[sort: modified]"},
		{filesort.BySize, "[sort: size]"},
		{filesort.ByDropOrder, "[unsorted]"},
	}

	for _, step := range steps {
		v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyS})
		waitForSort(t, v)
		waitUntilLoaded(t, v)

		if v.state.SortMode() != step.mode {
			t.Fatalf("sortMode = %v, want %v", v.state.SortMode(), step.mode)
		}
		if got := namesOf(v.state.files); !slices.Equal(got, scanOrder) {
			t.Errorf("[mode %v] files = %v, want %v", step.mode, got, scanOrder)
		}
		if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
			t.Errorf("[mode %v] displayed file = %q, want IMG_2.jpg to stay in view", step.mode, got)
		}

		title := v.win.Title()
		if !strings.HasPrefix(title, step.label+" ") {
			t.Errorf("[mode %v] title = %q, want it prefixed with %q", step.mode, title, step.label)
		}
		if !strings.Contains(title, "(3/3)") {
			t.Errorf("[mode %v] title = %q, want it to contain (3/3)", step.mode, title)
		}
	}

	// One more S wraps back around to natural (by-name) sort.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyS})
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if v.state.SortMode() != filesort.ByName {
		t.Fatalf("sortMode = %v, want filesort.ByName after wrapping around", v.state.SortMode())
	}
	if got := namesOf(v.state.files); !slices.Equal(got, natural) {
		t.Errorf("files = %v, want natural-sorted %v after wrapping around", got, natural)
	}
	if got := v.state.files[v.state.index].Name(); got != "IMG_2.jpg" {
		t.Errorf("displayed file = %q, want IMG_2.jpg to stay in view", got)
	}

	title := v.win.Title()
	if strings.Contains(title, "[sort:") || strings.Contains(title, "[unsorted]") {
		t.Errorf("title = %q, want no sort-mode prefix for the default natural sort", title)
	}
	if !strings.Contains(title, "(2/3)") {
		t.Errorf("title = %q, want it to contain (2/3) after wrapping back to natural order", title)
	}
}

// TestSetSortMode_JumpsDirectlyRatherThanCycling is SetSortMode - the
// settings window's binding - as opposed to toggleSort's own S-key cycle
// already covered above: it should reach any mode in one call and still
// keep the current file in view across the switch.
func TestSetSortMode_JumpsDirectlyRatherThanCycling(t *testing.T) {
	v := newTestViewer(t)

	img10 := uitest.TempJPEGURI(t, "IMG_10.jpg", 4, 4, color.White)
	img1 := uitest.TempJPEGURI(t, "IMG_1.jpg", 4, 4, color.White)
	dropAndWait(t, v, img10, img1) // natural sort: IMG_1.jpg, IMG_10.jpg

	current := v.state.files[v.state.index].Name()

	v.SetSortMode(filesort.ByDropOrder)

	if v.state.SortMode() != filesort.ByDropOrder {
		t.Errorf("sortMode = %v, want ByDropOrder straight after one SetSortMode call", v.state.SortMode())
	}

	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if got := v.state.files[v.state.index].Name(); got != current {
		t.Errorf("displayed file = %q, want it to stay on %q across the sort-mode change", got, current)
	}
}

// TestSetSortMode_SafeWithNoFilesLoaded guards the settings window's own
// call site: unlike toggleSort's S key (gated behind handleKeyEvent's
// len(v.state.files)<2 guard), the settings window can change the sort order
// before anything has ever been dropped.
func TestSetSortMode_SafeWithNoFilesLoaded(t *testing.T) {
	v := newTestViewer(t)

	v.SetSortMode(filesort.BySize)

	if v.state.SortMode() != filesort.BySize {
		t.Errorf("sortMode = %v, want BySize", v.state.SortMode())
	}
}

func TestViewerFileStateSlicesRemainEquivalentAcrossTransitions(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.White)

	dropAndWait(t, v, a, c)
	assertEquivalentFileSlices(t, v)

	v.SetSortMode(filesort.ByDropOrder)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	assertEquivalentFileSlices(t, v)

	v.SetMergeMode(true)
	dropAndWait(t, v, b)
	assertEquivalentFileSlices(t, v)

	v.RemoveFile(v.state.index)
	assertEquivalentFileSlices(t, v)

	v.SetMergeMode(false)
	dropAndWait(t, v, a)
	assertEquivalentFileSlices(t, v)
}

func TestViewerIndexStaysValidAcrossFileStateTransitions(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.White)

	assertValidFileIndex(t, v)

	dropAndWait(t, v, a, c)
	assertValidFileIndex(t, v)

	v.ShowImage(len(v.state.files) - 1)
	waitUntilLoaded(t, v)
	assertValidFileIndex(t, v)

	v.SetSortMode(filesort.ByDropOrder)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	assertValidFileIndex(t, v)

	v.SetMergeMode(true)
	dropAndWait(t, v, b)
	assertValidFileIndex(t, v)

	v.RemoveFile(v.state.index)
	assertValidFileIndex(t, v)

	v.SetMergeMode(false)
	dropAndWait(t, v, a)
	assertValidFileIndex(t, v)

	v.reset()
	assertValidFileIndex(t, v)
}

func TestViewerModesApplyBeforeAndAfterLoadingFiles(t *testing.T) {
	v := newTestViewer(t)

	v.SetSortMode(filesort.ByDropOrder)
	v.SetMergeMode(true)

	if v.SortMode() != filesort.ByDropOrder || !v.MergeMode() {
		t.Fatal("modes set before a drop were not retained")
	}

	a := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	dropAndWait(t, v, b)

	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"2.jpg", "1.jpg"}) {
		t.Errorf("files = %v, want merge mode and drop-order mode applied", got)
	}

	v.SetSortMode(filesort.ByName)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	v.SetMergeMode(false)

	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"1.jpg", "2.jpg"}) {
		t.Errorf("files = %v, want name sort applied after loading", got)
	}
	if v.MergeMode() {
		t.Error("merge mode should be disabled after loading")
	}
}

func TestStaleFileStateCompletionsDoNotOverwriteNewerState(t *testing.T) {
	v := newTestViewer(t)

	current := []fyne.URI{
		uitest.FakeURI{FileName: "current.jpg", Ext: ".jpg"},
	}
	stale := []fyne.URI{
		uitest.FakeURI{FileName: "stale.jpg", Ext: ".jpg"},
	}
	v.state.files = append([]fyne.URI(nil), current...)
	v.state.unsortedFiles = append([]fyne.URI(nil), current...)

	staleScanToken := v.scanLifecycle.begin()
	v.scanLifecycle.begin()
	scanDone := make(chan struct{})
	v.applyScanResult(staleScanToken, false, stale, stale, false, scanDone)
	<-scanDone
	assertEquivalentFileSlices(t, v)
	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"current.jpg"}) {
		t.Errorf("files = %v, want newer scan state retained", got)
	}

	staleSortToken := v.sortLifecycle.begin()
	newSortToken := v.sortLifecycle.begin()
	defer newSortToken.cancelContext()
	v.sorting = true
	v.sortSpinner.Show()
	v.sortLabel.Show()
	sortDone := make(chan struct{})
	called := false
	v.finishSort(staleSortToken, stale, sortDone, func([]fyne.URI) {
		called = true
	})
	<-sortDone

	if called {
		t.Error("stale sort completion should not invoke its state-writing callback")
	}
	if !v.sorting {
		t.Error("stale sort completion should not clear a newer sort's in-flight state")
	}
	if !v.sortSpinner.Visible() || !v.sortLabel.Visible() {
		t.Error("stale sort completion should not hide the newer sort's progress UI")
	}
	assertEquivalentFileSlices(t, v)
	if got := namesOfURIs(v.state.files); !slices.Equal(got, []string{"current.jpg"}) {
		t.Errorf("files = %v, want newer sort state retained", got)
	}
}

func TestInvalidateSortCancelsAndFinalizesCurrentProgress(t *testing.T) {
	v := newTestViewer(t)

	token := v.sortLifecycle.begin()
	v.sorting = true
	v.sortSpinner.Show()
	v.sortLabel.Show()

	v.invalidateSort()

	if token.current() || token.context().Err() == nil {
		t.Fatal("invalidateSort should cancel and supersede the current sort token")
	}
	if v.sorting || v.sortSpinner.Visible() || v.sortLabel.Visible() {
		t.Fatal("invalidateSort should synchronously finalize the current sort progress UI")
	}
}

// TestGenerationTracksFileSetIdentityNotNavigation protects the contract used
// by grid and deletion: indices retain their meaning across navigation but not
// across a removal.
func TestGenerationTracksFileSetIdentityNotNavigation(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	beforeNavigation := v.Generation()
	v.ShowImage(1)
	waitUntilLoaded(t, v)
	if got := v.Generation(); got != beforeNavigation {
		t.Fatalf("Generation changed from %d to %d on navigation", beforeNavigation, got)
	}

	v.RemoveFile(0)
	if got := v.Generation(); got <= beforeNavigation {
		t.Fatalf("Generation = %d after removal, want greater than %d", got, beforeNavigation)
	}
}

func assertEquivalentFileSlices(t *testing.T, v *viewer) {
	t.Helper()

	files := namesOfURIs(v.state.files)
	unsorted := namesOfURIs(v.state.unsortedFiles)
	slices.Sort(files)
	slices.Sort(unsorted)
	if !slices.Equal(files, unsorted) {
		t.Errorf("files = %v and unsortedFiles = %v do not contain the same URIs", v.state.files, v.state.unsortedFiles)
	}
}

func assertValidFileIndex(t *testing.T, v *viewer) {
	t.Helper()

	if len(v.state.files) == 0 {
		if v.state.index != 0 {
			t.Errorf("index = %d, want 0 with no files", v.state.index)
		}
		return
	}
	if v.state.index < 0 || v.state.index >= len(v.state.files) {
		t.Errorf("index = %d, want a value in [0, %d)", v.state.index, len(v.state.files))
	}
}

func namesOfURIs(files []fyne.URI) []string {
	names := make([]string, len(files))
	for i, u := range files {
		names[i] = u.Name()
	}
	return names
}

// TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles is a -race regression
// test for the snapshot SetSortMode hands to startSort's goroutine: a plain
// slice-header copy of v.state.unsortedFiles aliases its backing array, which
// RemoveFile (a failed-decode retry, a Shift+Delete) then shifts *in place*
// on the UI goroutine while filesort.Order is still copying it - an
// unsynchronized read/write on the same memory. Nothing here asserts: the
// race detector is the assertion, and the test is only meaningful under
// -race.
//
// The file set is built from FakeURIs and assigned directly rather than
// dropped, since what matters is a set large enough for filesort.Order's
// initial copy to still be in flight when the removals start - not that the
// files exist (ByModTime just gets a zero time for each failed stat, in the
// same loop it would otherwise stat a real one).
func TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles(t *testing.T) {
	v := newTestViewer(t)

	const (
		files    = 4000
		removals = 200
	)

	unsorted := make([]fyne.URI, 0, files)
	for i := range files {
		unsorted = append(unsorted, uitest.FakeURI{FileName: fmt.Sprintf("img_%05d.jpg", i), Ext: ".jpg"})
	}

	v.state.files = append([]fyne.URI(nil), unsorted...)
	v.state.unsortedFiles = unsorted

	v.SetSortMode(filesort.ByModTime)

	for range removals {
		v.RemoveFile(0)
	}

	waitForSort(t, v)
}

// TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow guards
// keys.go's Escape branch: a first-ever drop's scan clears v.scanning back
// to false before applyScannedFiles's startSort (drop.go/sort.go) has
// actually populated v.state.files, so for as long as that reorder is still
// computing, v.state.files reads exactly like the "nothing left to reset" state
// Escape otherwise closes the window on. v.sorting is what tells the two
// apart. Drives the in-flight state directly - v.sorting true, v.state.files
// still empty, and sortLifecycle armed - rather than racing a real drop's
// background goroutine to reproduce that window, the same approach
// TestCancelScan_CancelsInFlightScanWithNoFilesYet uses for the gathering
// phase.
func TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow(t *testing.T) {
	v, _, closed := newTestUI(t)

	v.sortLifecycle.begin()
	v.sorting = true

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if closed() {
		t.Error("Escape should not close the window while a first-drop reorder is still in flight")
	}
	if v.sorting {
		t.Error("Escape's cancelSort should clear v.sorting")
	}
}

// TestHandleKeyEvent_EscapeDuringResortOfExistingFilesDoesNotClearThem is a
// regression test: keys.go's Escape branch used to fall through to a plain
// v.reset() whenever v.sorting was true, which is correct for a first-ever
// drop's cancelled reorder (there's nothing loaded yet to lose) but wrong
// for cancelling a resort of files that were already loaded and on
// screen - v.reset() wipes the whole session, not just the pending sort.
// cancelSort must leave the existing set and the displayed image alone,
// mirroring how cancelScan leaves a merge-mode scan's pre-existing files
// alone.
func TestHandleKeyEvent_EscapeDuringResortOfExistingFilesDoesNotClearThem(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	filesBefore := append([]fyne.URI(nil), v.state.files...)
	indexBefore := v.state.index

	v.sortLifecycle.begin()
	v.sorting = true

	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if len(v.state.files) != len(filesBefore) {
		t.Fatalf("files = %v, want unchanged %v after cancelling a resort", v.state.files, filesBefore)
	}
	for i, u := range v.state.files {
		if u.String() != filesBefore[i].String() {
			t.Errorf("files[%d] = %q, want unchanged %q after cancelling a resort", i, u, filesBefore[i])
		}
	}
	if v.state.index != indexBefore {
		t.Errorf("index = %d, want unchanged %d after cancelling a resort", v.state.index, indexBefore)
	}
	if v.img.Image == nil {
		t.Error("the displayed image should not be cleared by cancelling a resort")
	}
	if v.sorting {
		t.Error("Escape's cancelSort should clear v.sorting")
	}
}

func TestViewerReset(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	dropAndWait(t, v, jpegURI)

	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded before reset")
	}

	v.reset()

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after reset", v.state.files)
	}
	if v.state.index != 0 {
		t.Errorf("index = %d, want 0 after reset", v.state.index)
	}
	if v.img.Image != nil {
		t.Error("image should be cleared after reset")
	}
	if v.img.Visible() {
		t.Error("image should be hidden after reset")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again after reset")
	}
	if !v.welcomeArt.Visible() {
		t.Error("welcomeArt should be visible again after reset, matching the just-launched state")
	}
	if v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be hidden after reset")
	}
	if got, want := v.hint.Text, lang.L("Drop images here"); got != want {
		t.Errorf("hint text = %q, want %q after reset", got, want)
	}
	if size := v.win.Canvas().Size(); !uitest.ApproxEqual(size.Width, startW) || !uitest.ApproxEqual(size.Height, startH) {
		t.Errorf("window size = %v, want %vx%v after reset", size, startW, startH)
	}
}

func TestViewerShow_DecodeErrorKeepsHint(t *testing.T) {
	v := newTestViewer(t)

	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "corrupt.jpg", []byte("not a jpeg")))

	dropAndWait(t, v, corrupt)

	if v.img.Image != nil {
		t.Error("no image should be loaded after a decode error")
	}
	if got, want := v.hint.Text, lang.L("Drop images here"); got != want {
		t.Errorf("hint text = %q, want %q after a decode error on first load", got, want)
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown after a decode error")
	}
	settleToast(t, v)
}

// TestViewerShow_RejectsAbsurdHeaderDimensions checks the end-to-end wiring
// from a decompression-bomb-sized header, through attemptLoad's
// errors.As(*imaging.InvalidDimensionsError) branch, to the same
// "invalid image dimensions" toast the old post-decode zero-dimension check
// used to produce - now reached via the cheap header probe instead of a
// full (and here, impossible - the file has no IDAT/IEND) decode.
func TestViewerShow_RejectsAbsurdHeaderDimensions(t *testing.T) {
	v := newTestViewer(t)

	bomb := storage.NewFileURI(uitest.WriteTempFile(t, "bomb.png", uitest.TruncatedPNGHeader(t, 60000, 60000)))

	dropAndWait(t, v, bomb)

	if v.img.Image != nil {
		t.Error("no image should be loaded after a rejected header")
	}
	if !v.toast.card.Visible() {
		t.Fatal("expected a toast to be shown after a rejected header")
	}
	if got, want := v.toast.text.Text, fmt.Sprintf(lang.L("invalid image dimensions for %q"), "bomb.png"); got != want {
		t.Errorf("toast text = %q, want %q", got, want)
	}
	settleToast(t, v)
}

func TestViewerShow_AutoAdvancesPastBrokenFileDuringNavigation(t *testing.T) {
	v := newTestViewer(t)

	// Named so natural sort (the default) keeps them in this order.
	first := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "2.jpg", []byte("not a jpeg")))
	third := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.RGBA{B: 255, A: 255})

	dropAndWait(t, v, first, corrupt, third)

	if len(v.state.files) != 3 {
		t.Fatalf("files = %v, want all 3 dropped files kept until navigation actually reaches the broken one", v.state.files)
	}

	// Step onto the broken file.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want the broken file dropped from the set", v.state.files)
	}
	for _, u := range v.state.files {
		if u.Name() == "2.jpg" {
			t.Errorf("files = %v, the broken file should have been removed", v.state.files)
		}
	}
	if got := v.state.files[v.state.index].Name(); got != "3.jpg" {
		t.Errorf("displayed file = %q, want auto-advance to land on 3.jpg", got)
	}
	if v.img.Image == nil {
		t.Fatal("expected the auto-advanced-to image to be loaded, not left blank")
	}
	if got, want := v.win.Title(), "3.jpg"; !strings.Contains(got, want) {
		t.Errorf("title = %q, want it to reflect the auto-advanced-to file %q, not the stale broken one", got, want)
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the broken file was skipped")
	}
	settleToast(t, v)
}

func TestViewerShow_AutoAdvancesPastBrokenFirstFile(t *testing.T) {
	v := newTestViewer(t)

	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "1.jpg", []byte("not a jpeg")))
	second := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, corrupt, second)

	if len(v.state.files) != 1 || v.state.files[0].Name() != "2.jpg" {
		t.Fatalf("files = %v, want only 2.jpg left after the broken first file was auto-skipped", v.state.files)
	}
	if v.img.Image == nil {
		t.Fatal("expected the app to auto-advance to the one good image instead of giving up on the first failure")
	}
	if v.dropzone.Visible() {
		t.Error("dropzone should be hidden once the auto-advanced-to image is showing")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the broken file was skipped")
	}
	settleToast(t, v)
}

func TestViewerShow_AllFilesBrokenFallsBackToEmptyState(t *testing.T) {
	v := newTestViewer(t)

	corrupt1 := storage.NewFileURI(uitest.WriteTempFile(t, "1.jpg", []byte("not a jpeg")))
	corrupt2 := storage.NewFileURI(uitest.WriteTempFile(t, "2.jpg", []byte("also not a jpeg")))

	dropAndWait(t, v, corrupt1, corrupt2)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil once every dropped file has failed to decode", v.state.files)
	}
	if v.img.Image != nil {
		t.Error("no image should be displayed once every file has failed")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again once every file has failed")
	}
	if !v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be shown once every file has failed")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the failure")
	}
	settleToast(t, v)
}

func TestViewerShow_AnimatesGIF(t *testing.T) {
	v := newTestViewer(t)

	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})) // 20ms per frame, fast enough to keep the test quick

	dropAndWait(t, v, storage.NewFileURI(path))

	// animate() writes v.img.Image from its own goroutine for as long as
	// its load token stays current, which the fyne test driver never marshals onto
	// this one - so reading v.img.Image from here at any point before that
	// goroutine has fully stopped would race with those writes, even right
	// after waitForAnimFrame observes a given count: animate is free to
	// keep writing further frames in between that observation and the next
	// statement. animFrame reaching 2 (1 for attemptLoad's own first frame,
	// 1 more for animate's first cycle) is proof the animation loop ran at
	// all; invalidating loadLifecycle and waiting for animStopped then guarantees no
	// further write can happen, at which point animFrame's final value is
	// stable and it's finally safe to read v.img.Image.
	waitForAnimFrame(t, v, 2)

	v.loadLifecycle.invalidate()
	waitForAnimStopped(t, v)

	// Frame 0 (red) is written on odd counts (attemptLoad's initial write
	// is count 1), frame 1 (blue) on even ones - whichever count animate
	// happened to stop on, this checks the frame it left on screen actually
	// matches the data for that count instead of stale or corrupted pixels.
	n := v.animFrame.Load()
	wantBlue := n%2 == 0

	r, _, b, _ := v.img.Image.At(0, 0).RGBA()
	if wantBlue && b == 0 {
		t.Fatalf("expected the blue frame at animFrame=%d, got r=%d b=%d", n, r, b)
	}
	if !wantBlue && r == 0 {
		t.Fatalf("expected the red frame at animFrame=%d, got r=%d b=%d", n, r, b)
	}
}

func TestViewerShow_NavigatingAwayStopsAnimation(t *testing.T) {
	v := newTestViewer(t)

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2})))
	staticURI := uitest.TempJPEGURI(t, "static.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, animURI, staticURI)

	// Capture the first image's animate goroutine before superseding it -
	// once ShowImage(1) bumps gen, that goroutine's close(stopped) is the
	// only signal that it has actually noticed and returned, rather than
	// still being asleep between frames.
	oldAnimStopped := v.animStopped

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	// Wait for the superseded animation goroutine to actually stop instead
	// of sleeping a fixed duration and hoping: it writes v.img.Image from
	// its own goroutine, so reading the field before it's confirmed done
	// would race with that write even though the staleness check means it
	// would never actually overwrite the static image once gen has moved
	// on.
	select {
	case <-oldAnimStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the superseded animation to stop")
	}

	// JPEG is lossy, so a "solid green" square won't decode back to an exact
	// R=0, but green should still clearly dominate; an animation frame
	// bleeding through would show red or blue dominating instead.
	r, g, b, _ := v.img.Image.At(0, 0).RGBA()
	if g <= r || g <= b {
		t.Errorf("expected the static green image to remain displayed, got r=%d g=%d b=%d", r, g, b)
	}
}

// --- imgCache / speculative preloading --------------------------------

// waitForCached polls imgCache - populated from preloadOne's background
// goroutines, which run independently of loadDone/scanDone - until it holds
// an entry for u, the same polling-with-timeout style waitForAnimFrame uses
// for animate's background writes.
func waitForCached(t *testing.T, v *viewer, u fyne.URI) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for !v.imgCache.Contains(u.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q to be preloaded into imgCache", u.Name())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestFinishLoad_PreloadsBothNeighbors(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b, c)

	v.ShowImage(1) // b, with a genuine neighbor on each side
	waitUntilLoaded(t, v)

	waitForCached(t, v, a)
	waitForCached(t, v, c)
}

// TestAttemptLoad_CacheHitServesFileRemovedFromDisk proves a cache hit
// really does skip the disk read: b's file is deleted from disk right after
// it's preloaded, so a real (non-cached) load of it would fail and trigger
// retryAfterLoadFailure, dropping it from v.state.files. Navigating to it
// succeeding instead demonstrates the display came from imgCache.
func TestAttemptLoad_CacheHitServesFileRemovedFromDisk(t *testing.T) {
	v := newTestViewer(t)

	aPath := uitest.WriteTempFile(t, "a.jpg", uitest.EncodeJPEG(t, 4, 4, color.White))
	bPath := uitest.WriteTempFile(t, "b.jpg", uitest.EncodeJPEG(t, 4, 4, color.White))
	a := storage.NewFileURI(aPath)
	b := storage.NewFileURI(bPath)

	dropAndWait(t, v, a, b)

	waitForCached(t, v, b)

	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	if v.state.index != 1 {
		t.Fatalf("index = %d, want 1 - a cache hit must not fall through to retryAfterLoadFailure", v.state.index)
	}
	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want b still present - a cache hit must not treat it as broken", v.state.files)
	}
}

func TestRemoveFile_PurgesCacheEntry(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	v.state.files = []fyne.URI{a, b}
	v.state.unsortedFiles = []fyne.URI{a, b}
	v.imgCache.Add(a.String(), &imaging.LoadedImage{Frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 1, 1))}})

	v.RemoveFile(0)

	if v.imgCache.Contains(a.String()) {
		t.Error("RemoveFile should purge the removed file's imgCache entry")
	}
}

// --- byte-bounded image memory ---------------------------------------------

// TestAttemptLoad_DisplaysAnImageLargerThanTheWholeCacheBudget is the
// completion criterion the byte budget had to be designed around: a budget
// smaller than a single decode must not make the app unable to show that
// image. ByteCache never evicts its most recently added entry, and
// attemptLoad adds the image it is about to display, so the one on screen is
// always the survivor.
func TestAttemptLoad_DisplaysAnImageLargerThanTheWholeCacheBudget(t *testing.T) {
	v := newTestViewer(t)

	// One byte - past anything the settings window allows (it floors at
	// 1 MB), so this is the extreme the cache itself still has to handle.
	v.imgCache.SetBudget(1)

	u := uitest.TempJPEGURI(t, "big.jpg", 64, 64, color.White)
	dropAndWait(t, v, u)

	if v.img.Image == nil {
		t.Fatal("no image displayed - an image larger than the whole cache budget must still show")
	}
	if len(v.displayFrames) != 1 {
		t.Errorf("displayFrames = %d, want 1", len(v.displayFrames))
	}
	if !v.imgCache.Contains(u.String()) {
		t.Error("the displayed image should still be cached - ByteCache never evicts its newest entry")
	}
}

// TestPreloadOne_SkipsANeighborTooLargeForTheBudget covers the other
// completion criterion: neighbor preloading must not multiply one oversized
// image into several retained decodes. The bail happens on the header alone,
// before the decode, so an over-large neighbor costs nothing but the probe.
func TestPreloadOne_SkipsANeighborTooLargeForTheBudget(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 64, 64, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 64, 64, color.White)

	// 64x64 estimates at 16,384 decoded bytes (4 per pixel). A 16 KiB budget
	// puts that exactly at the budget and so past the half-budget line
	// preloadOne bails at - the point where the current image and one
	// neighbor stop both fitting.
	v.imgCache.SetBudget(16 * 1024)

	dropAndWait(t, v, a, b)

	if !v.imgCache.Contains(a.String()) {
		t.Error("the displayed image should be cached")
	}
	if v.imgCache.Contains(b.String()) {
		t.Error("a neighbor whose decode would evict the current image should not have been preloaded")
	}
}

// TestAttemptLoad_ReportsAFileTooLargeToOpen wires imaging's
// *InputTooLargeError through attemptLoad's errors.As branch to its own
// message - distinct from the "invalid image dimensions" one, since the file
// here is a perfectly valid JPEG that is merely bigger than the limit.
func TestAttemptLoad_ReportsAFileTooLargeToOpen(t *testing.T) {
	v := newTestViewer(t)

	u := uitest.TempJPEGURI(t, "big.jpg", 64, 64, color.White)

	imaging.SetMaxEncodedBytes(1)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) })

	dropAndWait(t, v, u)

	if v.img.Image != nil {
		t.Error("no image should be loaded after a file is refused for its size")
	}
	if len(v.state.files) != 0 {
		t.Errorf("files = %v, want the refused file dropped from the set", v.state.files)
	}
	if !v.toast.card.Visible() {
		t.Fatal("expected a toast after a file was refused for its size")
	}
	if got, want := v.toast.text.Text, fmt.Sprintf(lang.L("%q is too large to open"), "big.jpg"); got != want {
		t.Errorf("toast text = %q, want %q", got, want)
	}
	settleToast(t, v)
}

// TestAttemptLoad_ToastsAndFallsBackToAStaticFrameForAnOversizedAnimation
// covers the deliberate choice not to refuse an over-budget animation the way
// an oversized file is refused: the image is valid, so it stays in the set and
// on screen, and only the motion is given up.
func TestAttemptLoad_ToastsAndFallsBackToAStaticFrameForAnOversizedAnimation(t *testing.T) {
	v := newTestViewer(t)

	anim := storage.NewFileURI(uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 20, 20,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{50, 50})))

	// One frame of this GIF is 20*20*4 = 1600 bytes, so a 1000-byte budget
	// can't hold even one - let alone both composited frames.
	v.imgCache.SetBudget(1000)

	dropAndWait(t, v, anim)

	if v.img.Image == nil {
		t.Fatal("an over-budget animation must still display its first frame")
	}
	if len(v.displayFrames) != 1 {
		t.Errorf("displayFrames = %d, want 1 - the animation should not have been composited", len(v.displayFrames))
	}
	if v.animStopped != nil {
		t.Error("animStopped is armed, want no animation goroutine for a refused animation")
	}
	if len(v.state.files) != 1 {
		t.Errorf("files = %v, want the file kept - it is valid, just too big to animate", v.state.files)
	}
	if !v.toast.card.Visible() {
		t.Fatal("expected a toast explaining why the animation isn't playing")
	}
	if got, want := v.toast.text.Text, fmt.Sprintf(lang.L("animation in %q is too large to play"), "anim.gif"); got != want {
		t.Errorf("toast text = %q, want %q", got, want)
	}
	settleToast(t, v)
}

func TestClearToDropzone_PurgesTheImageCache(t *testing.T) {
	v := newTestViewer(t)

	u := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, u)

	if !v.imgCache.Contains(u.String()) {
		t.Fatal("the displayed image should be cached before the file set is closed")
	}

	v.closeFiles()

	if v.imgCache.Bytes() != 0 {
		t.Errorf("imgCache holds %d bytes after closing the file set, want 0", v.imgCache.Bytes())
	}
}

// --- the memory-limit getter/setter pairs ------------------------------------

func TestMemoryLimitGettersAndSetters(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	if got, want := v.MaxImageCacheMB(), defaultMaxImageCacheMB; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want the shipped default %d", got, want)
	}
	if got, want := v.MaxThumbCacheMB(), defaultMaxThumbCacheMB; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want the shipped default %d", got, want)
	}
	if got, want := v.MaxFileSizeMB(), defaultMaxFileSizeMB; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want the shipped default %d", got, want)
	}

	// Each setter has to reach past the viewer's own bookkeeping field to
	// the thing that actually enforces the limit.
	v.SetMaxImageCacheMB(64)
	if got, want := v.MaxImageCacheMB(), 64; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want %d", got, want)
	}
	if got, want := v.imgCache.Budget(), int64(64*bytesPerMB); got != want {
		t.Errorf("imgCache.Budget() = %d, want %d", got, want)
	}

	v.SetMaxThumbCacheMB(32)
	if got, want := v.MaxThumbCacheMB(), 32; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want %d", got, want)
	}

	v.SetMaxFileSizeMB(16)
	if got, want := v.MaxFileSizeMB(), 16; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want %d", got, want)
	}
	if got, want := imaging.MaxEncodedBytes(), int64(16*bytesPerMB); got != want {
		t.Errorf("imaging.MaxEncodedBytes() = %d, want %d", got, want)
	}
}

// A zero or negative megabyte figure isn't a "no limit" any of this is
// written to understand, so every setter floors at 1 - the same guard
// SetMaxScan makes for the scan cap.
func TestMemoryLimitSetters_FloorAtOne(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) })

	for _, n := range []int{0, -5} {
		v.SetMaxImageCacheMB(n)
		v.SetMaxThumbCacheMB(n)
		v.SetMaxFileSizeMB(n)

		if got := v.MaxImageCacheMB(); got != 1 {
			t.Errorf("MaxImageCacheMB() = %d after SetMaxImageCacheMB(%d), want 1", got, n)
		}
		if got := v.MaxThumbCacheMB(); got != 1 {
			t.Errorf("MaxThumbCacheMB() = %d after SetMaxThumbCacheMB(%d), want 1", got, n)
		}
		if got := v.MaxFileSizeMB(); got != 1 {
			t.Errorf("MaxFileSizeMB() = %d after SetMaxFileSizeMB(%d), want 1", got, n)
		}
	}
}

// The SVG re-render raster is deliberately never charged to imgCache (it is
// live display state, not a cache entry), so honoring the user's memory
// setting means deriving its ceiling from the budget instead: a quarter of
// the budget's bytes at 4 B per RGBA pixel, clamped by imaging's own
// floor and default ceiling.
func TestSetMaxImageCacheMBRetunesTheVectorRasterCeiling(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxVectorRasterPixels(imaging.DefaultMaxVectorRasterPixels) })

	// 256 MB / 4 (a quarter of the budget) / 4 B per RGBA px = 16,777,216.
	v.SetMaxImageCacheMB(256)
	if got := imaging.MaxVectorRasterPixels(); got != 16_777_216 {
		t.Errorf("after 256 MB: ceiling = %d, want 16777216", got)
	}

	// A tiny budget lands on the floor rather than making SVGs unusable...
	v.SetMaxImageCacheMB(64)
	if got := imaging.MaxVectorRasterPixels(); got != 8_000_000 {
		t.Errorf("after 64 MB: ceiling = %d, want the 8000000 floor", got)
	}

	// ...and a huge one never exceeds the shipped 32 MP behavior.
	v.SetMaxImageCacheMB(4096)
	if got := imaging.MaxVectorRasterPixels(); got != imaging.DefaultMaxVectorRasterPixels {
		t.Errorf("after 4096 MB: ceiling = %d, want the %d default ceiling", got, int64(imaging.DefaultMaxVectorRasterPixels))
	}
}

// --- stage-2 stop signals --------------------------------------------------

// TestInvalidateLoad_WakesAnimateImmediately parks animate in a frame-delay
// sleep far longer than the test and checks lifecycle cancellation wakes it
// immediately rather than waiting for the next frame tick.
func TestInvalidateLoad_WakesAnimateImmediately(t *testing.T) {
	v := newTestViewer(t)

	animURI := storage.NewFileURI(uitest.WriteTempFile(t, "slow.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{1000, 1000}))) // 10s per frame, in centiseconds

	dropAndWait(t, v, animURI)

	if v.animStopped == nil {
		t.Fatal("loading an animated GIF should arm animStopped")
	}

	v.loadLifecycle.invalidate()

	waitForAnimStopped(t, v)
	v.loadLifecycle.invalidate() // repeated invalidation must remain safe
}

// TestStartWindowPosPolling_TestDriverGetsNoopStop pins the stop-func
// contract: under the fyne test driver the window is never a
// driver.NativeWindow, so no poller goroutine starts - but the returned
// stop func must still be non-nil and safe to call, since main()'s
// SetOnStopped calls it unconditionally. (The goroutine path itself can't
// run under the test driver at all - see startWindowPosPolling's comment.)
func TestStartWindowPosPolling_TestDriverGetsNoopStop(t *testing.T) {
	v := newTestViewer(t)

	if v.stopWinPosPoll == nil {
		t.Fatal("buildStartupViewer should initialize stopWinPosPoll")
	}
	v.stopWinPosPoll() // safe before Run replaces it with the live poller's stop

	stop := startWindowPosPolling(v, v.win)
	if stop == nil {
		t.Fatal("startWindowPosPolling should never return a nil stop func")
	}
	stop() // must not panic or block
}

func TestStartWindowPosPolling_PanicsWithoutConstructedSlideshow(t *testing.T) {
	const want = "ui: startWindowPosPolling called before slideshow construction"

	defer func() {
		if got := recover(); got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()

	startWindowPosPolling(&viewer{}, nil)
}
