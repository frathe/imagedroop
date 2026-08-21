package ui

import (
	"os"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// This file is the shared harness every other test file in this package
// builds on: newTestUI/newTestViewer construct a viewer through the same
// startup path production uses, and the wait/settle/assert helpers below let
// tests synchronize with its background work instead of guessing at timing.
// newTestViewer alone is used by 17 of the package's other test files,
// dropAndWait by 16, waitUntilLoaded by 15 - a new helper shared across more
// than one feature belongs here, not bolted onto whichever feature file
// happens to need it first.
//
// ShowImage and handleDrop decode and scan off the main goroutine and apply
// their results via fyne.Do, closing loadDone/scanOp.done as the last thing
// their completion block does. Waiting on those channels - rather than
// polling v.loading or a widget's visibility - gives the receive a proper
// happens-before relationship with everything the producer goroutine wrote,
// which is what makes these tests race-free under the test driver's
// fyne.Do: unlike the real app drivers, it runs synchronously on the calling
// goroutine instead of marshaling onto a single GUI goroutine. Never sleep
// to guess completion.
//
// AGENTS.md states the rule this file exists to enforce: every goroutine
// needs cancellation/staleness handling plus an observable stop/done signal,
// and any new background work must be added to newTestUI's drain cleanup,
// below.
var testApp fyne.App

func TestMain(m *testing.M) {
	testApp = test.NewApp()

	// No global tweaks needed here anymore: the toast auto-hide duration,
	// the folder-scan cap, and the key-modifier reader - all package vars
	// once, all mutated from tests - are per-viewer state now, overridden
	// where each viewer is built (newTestUI, or the individual test).
	os.Exit(m.Run())
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
	// them still pending when a test asserts on v.vector.raster/v.vector.pending
	// moments later - zeroed here the same way the toast's duration is.
	v.vector.debounce = 0

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
// has a nil scanOp.done - but the set is exhaustive on purpose: it is the
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
	v.scanOp.lifecycle.invalidate()
	v.sortOp.lifecycle.invalidate()
	v.vector.lifecycle.invalidate()
	v.favThumbLifecycle.invalidate()
	v.slides.Exit()

	// Vector re-renders: spawned by any effective-scale change, so a test
	// that zoomed or resized may still have one in flight. Must stay below
	// invalidateLoad and slides.Exit above: only once no superseded decode
	// can still land in finishLoad (whose resize triggers a scale change)
	// and no slideshow advance can start a load is this Wait racing no
	// further Add.
	v.vector.pending.Wait()

	for _, c := range []struct {
		name string
		ch   chan struct{}
	}{
		{"scan", v.scanOp.done},
		{"sort", v.sortOp.done},
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

// --- waiting for async work -------------------------------------------------

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
// returns before ever reaching startSort in that case, so v.sortOp.done is left
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
	case <-v.scanOp.done:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for scan to finish")
	}
}

func waitForSort(t *testing.T, v *viewer) {
	t.Helper()

	select {
	case <-v.sortOp.done:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for sort to finish")
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

// waitForCached polls imgCache - populated from preloadOne's background
// goroutines, which run independently of loadDone/scanOp.done - until it holds
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

// --- priming and settling background work -----------------------------------

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

// --- file-set assertions ----------------------------------------------------

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
