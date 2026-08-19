package deletion

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/trash"
)

func TestMain(m *testing.M) {
	// The confirmation card is built from real widgets, so these need an
	// app for the theme and driver - but never a window: everything below
	// drives the component directly.
	test.NewApp()
	os.Exit(m.Run())
}

// fakeHost records what the confirmation asked the app to do. It is the
// whole point of Host being a consumer-side interface: the state machine
// can be driven, and every effect observed, without a viewer, a window, or
// a decode.
type fakeHost struct {
	files []fyne.URI
	index int
	gen   uint64

	// removed flattens every index RemoveFiles was asked for, removedBatch
	// is just the most recent call's list, and removeCalls counts the calls
	// themselves - a batch has to arrive as one call, or the indices in it
	// would shift out from under each other.
	removed      []int
	removedBatch []int
	removeCalls  int

	shown    []int
	toasts   []string
	emptied  []string
	repaints int
}

func (f *fakeHost) CurrentFile() (fyne.URI, int, bool) {
	if len(f.files) == 0 {
		return nil, 0, false
	}

	return f.files[f.index], f.index, true
}

// RemoveFiles drops every named index in one pass, descending, so an earlier
// removal can't shift a later index out from under the same call - the same
// thing the real viewer has to do.
func (f *fakeHost) RemoveFiles(indices []int) {
	f.removeCalls++
	f.removedBatch = slices.Clone(indices)
	f.removed = append(f.removed, indices...)

	for _, i := range slices.Backward(slices.Sorted(slices.Values(indices))) {
		f.files = append(f.files[:i], f.files[i+1:]...)
	}
	if f.index >= len(f.files) {
		f.index = 0
	}
}

func (f *fakeHost) ShowImage(i int)              { f.shown = append(f.shown, i) }
func (f *fakeHost) ShowToast(msg string)         { f.toasts = append(f.toasts, msg) }
func (f *fakeHost) ShowEmptyStateError(m string) { f.emptied = append(f.emptied, m) }
func (f *fakeHost) ForceRepaint()                { f.repaints++ }
func (f *fakeHost) Generation() uint64           { return f.gen }

// stubTrashMove makes trash.Move behave like a plain remove, so these tests
// exercise the confirmation flow's own logic without ever invoking the real
// per-OS trash mover - which, run for real in `go test`, would move the
// temp file into the machine's actual Trash.
func stubTrashMove(t *testing.T) {
	t.Helper()

	orig := trash.Move
	t.Cleanup(func() { trash.Move = orig })
	trash.Move = func(path string) error { return os.Remove(path) }
}

// tempFiles writes n real files (the delete path calls trash.Move, so they
// have to exist) and returns their URIs.
func tempFiles(t *testing.T, names ...string) []fyne.URI {
	t.Helper()

	dir := t.TempDir()

	uris := make([]fyne.URI, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("not really an image"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		uris = append(uris, storage.NewFileURI(path))
	}

	return uris
}

func TestRequest_ShowsCardWithMessageAndCancelSelectedByDefault(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "sunset.jpg")}
	c := New(host)

	c.Request()

	if !c.Visible() || !c.card.Overlay().Visible() {
		t.Fatal("the card should be visible after Request")
	}
	if !strings.Contains(c.card.Message().Text, "sunset.jpg") {
		t.Errorf("message = %q, want it to name the current file", c.card.Message().Text)
	}
	if c.dangerSelected() {
		t.Error("Cancel should be selected by default, not the danger button")
	}
	if !c.card.Ring(cancelChoice).Visible() || c.card.Ring(dangerChoice).Visible() {
		t.Error("the Cancel ring should be visible and the danger ring hidden by default")
	}
}

func TestRequest_NoOpWithNothingLoaded(t *testing.T) {
	c := New(&fakeHost{})

	c.Request()

	if c.Visible() || c.card.Overlay().Visible() {
		t.Error("Request should do nothing with no files loaded")
	}
}

func TestRequest_ReopeningDoesNotResetAnAlreadyMadeSelection(t *testing.T) {
	c := New(&fakeHost{files: tempFiles(t, "a.jpg")})

	c.Request()
	c.setSelection(true)

	c.Request() // re-triggering the shortcut mid-prompt

	if !c.dangerSelected() {
		t.Error("a second Request while already showing should not reset the selection back to Cancel")
	}
}

func TestSetSelection_TogglesRingVisibility(t *testing.T) {
	c := New(&fakeHost{})

	c.setSelection(true)
	if c.card.Ring(cancelChoice).Visible() || !c.card.Ring(dangerChoice).Visible() {
		t.Error("selecting the danger button should show its ring and hide Cancel's")
	}

	c.setSelection(false)
	if !c.card.Ring(cancelChoice).Visible() || c.card.Ring(dangerChoice).Visible() {
		t.Error("selecting Cancel should show its ring and hide the danger button's")
	}
}

func TestHandleKey_MovesSelectionAndCancels(t *testing.T) {
	c := New(&fakeHost{files: tempFiles(t, "a.jpg")})
	c.Request()

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if !c.dangerSelected() {
		t.Error("Right should move the selection to the danger button")
	}

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if c.dangerSelected() {
		t.Error("Left should move the selection back to Cancel")
	}

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if c.Visible() {
		t.Error("Escape should dismiss the card")
	}
}

func TestHandleKey_ReturnWithCancelSelectedJustHidesTheCard(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg")}
	c := New(host)
	c.Request()

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if c.Visible() {
		t.Error("Return with Cancel selected should dismiss the card")
	}
	if len(host.removed) != 0 {
		t.Error("Return with Cancel selected must not delete anything")
	}
	if _, err := os.Stat(host.files[0].Path()); err != nil {
		t.Errorf("the file should still exist on disk: %v", err)
	}
}

func TestPerformDelete_RemovesFileAndAdvances(t *testing.T) {
	stubTrashMove(t)
	files := tempFiles(t, "a.jpg", "b.jpg")
	host := &fakeHost{files: files}
	c := New(host)

	// Captured before the delete: RemoveFile shifts the survivor down over
	// the removed entry, and files here shares its backing array, so
	// files[0] would name b.jpg by the time we checked.
	deletedPath := files[0].Path()

	c.Request()

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight}) // select danger
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	c.Settle()

	if c.Visible() {
		t.Error("confirming should dismiss the card")
	}
	if _, err := os.Stat(deletedPath); !os.IsNotExist(err) {
		t.Error("the confirmed file should be gone from disk")
	}
	if want := []int{0}; len(host.removed) != 1 || host.removed[0] != want[0] {
		t.Errorf("RemoveFile calls = %v, want %v", host.removed, want)
	}
	if len(host.shown) != 1 || host.shown[0] != 0 {
		t.Errorf("ShowImage calls = %v, want the same index re-shown so the next file takes its place", host.shown)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "a.jpg") {
		t.Errorf("toasts = %v, want one naming the deleted file", host.toasts)
	}
}

func TestPerformDelete_LastFileFallsBackToEmptyState(t *testing.T) {
	stubTrashMove(t)
	files := tempFiles(t, "only.jpg")
	host := &fakeHost{files: files}
	c := New(host)
	c.Request()

	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if _, err := os.Stat(files[0].Path()); !os.IsNotExist(err) {
		t.Error("the confirmed file should be gone from disk")
	}
	if len(host.emptied) != 1 || !strings.Contains(host.emptied[0], "only.jpg") {
		t.Errorf("ShowEmptyStateError calls = %v, want one naming the deleted file", host.emptied)
	}
	if len(host.shown) != 0 {
		t.Error("nothing should be shown once the last file is gone")
	}
}

func TestPerformDelete_OSFailureKeepsTheFileAndToastsAnError(t *testing.T) {
	stubTrashMove(t)

	// A directory that isn't empty can't be removed, which is the simplest
	// portable way to make the stubbed trash.Move fail against a path that
	// still exists afterwards.
	dir := t.TempDir()
	stubborn := filepath.Join(dir, "notafile")
	if err := os.MkdirAll(filepath.Join(stubborn, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	host := &fakeHost{files: []fyne.URI{storage.NewFileURI(stubborn)}}
	c := New(host)
	c.Request()

	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if _, err := os.Stat(stubborn); err != nil {
		t.Errorf("the file should survive a failed delete: %v", err)
	}
	if len(host.removed) != 0 {
		t.Error("a failed trash move must not drop the file from the app's set")
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "could not move") {
		t.Errorf("toasts = %v, want one reporting the failure", host.toasts)
	}
}

// TestPerformDelete_SkipsFileSetMutationIfGenerationChangesDuringTheMove
// guards against a race the async redesign introduces: trash.Move runs on
// its own goroutine (see performDelete's doc comment for why it must), so
// something else - a fresh drop, a reset, another delete - can change the
// app's file set while it's still in flight, which would make the
// captured index stale by the time the move finishes. The move to Trash
// itself must still go through (the file shouldn't leak just because the
// bookkeeping got skipped), but RemoveFile/ShowImage/ShowEmptyStateError
// must not run against an index that may no longer mean what it did.
func TestPerformDelete_SkipsFileSetMutationIfGenerationChangesDuringTheMove(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg")}
	c := New(host)

	orig := trash.Move
	t.Cleanup(func() { trash.Move = orig })
	trash.Move = func(path string) error {
		// Stands in for a fresh drop (or any other event) landing while
		// the move to Trash is still in flight.
		host.gen++
		return os.Remove(path)
	}

	c.Request()
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if _, err := os.Stat(host.files[0].Path()); !os.IsNotExist(err) {
		t.Error("the file should still be moved to Trash even though the generation changed mid-flight")
	}
	if len(host.removed) != 0 {
		t.Error("RemoveFile must not run against a file set that moved on during the async move")
	}
	if len(host.shown) != 0 || len(host.emptied) != 0 {
		t.Error("neither ShowImage nor ShowEmptyStateError should run once the generation is stale")
	}
}

// TestShortcutHandler_RunsOnlyOnSecondaryCut: this package's whole job here
// is telling a real Shift+Delete apart from the Ctrl/Cmd+X that arrives as
// the same event type. What the key then confirms is the app's decision, so
// the handler runs a callback rather than reaching for Request itself.
func TestShortcutHandler_RunsOnlyOnSecondaryCut(t *testing.T) {
	requests := 0
	handle := ShortcutHandler(func() { requests++ })

	// A real Ctrl/Cmd+X: same ShortcutName, not ours.
	handle(&fyne.ShortcutCut{})
	if requests != 0 {
		t.Errorf("requests = %d after a plain cut shortcut, want 0", requests)
	}

	handle(&fyne.ShortcutCut{Secondary: true})
	if requests != 1 {
		t.Errorf("requests = %d after Shift+Delete, want 1", requests)
	}
}
