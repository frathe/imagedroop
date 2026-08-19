package deletion

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/trash"
)

// targetsFor turns a host's whole file set into the target list the grid
// would hand over for a select-all.
func targetsFor(host *fakeHost, indices ...int) []Target {
	ts := make([]Target, 0, len(indices))
	for _, i := range indices {
		ts = append(ts, Target{URI: host.files[i], Index: i})
	}

	return ts
}

func TestRequestFiles_NamesTheCountRatherThanEachFile(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg", "c.jpg")}
	c := New(host)

	c.RequestFiles(targetsFor(host, 0, 1, 2))

	if !c.Visible() {
		t.Fatal("the card should be visible after RequestFiles")
	}
	if !strings.Contains(c.card.Message().Text, "3") {
		t.Errorf("message = %q, want it to say how many files are about to go", c.card.Message().Text)
	}
}

// TestRequestFiles_OneTargetReadsExactlyLikeTheSingleFilePrompt keeps the
// existing wording - and the golden masters that render it - intact: a batch
// of one is the same question Shift+Delete has always asked.
func TestRequestFiles_OneTargetReadsExactlyLikeTheSingleFilePrompt(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "sunset.jpg")}
	c := New(host)

	c.RequestFiles(targetsFor(host, 0))
	batch := c.card.Message().Text

	c.Cancel()
	c.Request()

	if batch != c.card.Message().Text {
		t.Errorf("RequestFiles message = %q, Request message = %q, want them identical for one file", batch, c.card.Message().Text)
	}
}

func TestRequestFiles_NoOpWithNoTargets(t *testing.T) {
	c := New(&fakeHost{files: tempFiles(t, "a.jpg")})

	c.RequestFiles(nil)

	if c.Visible() {
		t.Error("RequestFiles should do nothing with an empty target list")
	}
}

func TestPerformDelete_BatchRemovesEveryTarget(t *testing.T) {
	stubTrashMove(t)
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")}
	c := New(host)

	paths := []string{host.files[0].Path(), host.files[2].Path()}
	keptPath := host.files[1].Path()

	c.RequestFiles(targetsFor(host, 0, 2))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone from disk", p)
		}
	}
	if _, err := os.Stat(keptPath); err != nil {
		t.Errorf("%s was not selected and should still exist: %v", keptPath, err)
	}
	if want := []int{0, 2}; !slices.Equal(host.removedBatch, want) {
		t.Errorf("RemoveFiles call = %v, want %v", host.removedBatch, want)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "2") {
		t.Errorf("toasts = %v, want one reporting how many moved", host.toasts)
	}
}

// TestPerformDelete_BatchRemovesInOneCall is what keeps the indices valid:
// removing them one at a time would shift every later index out from under
// the list already captured.
func TestPerformDelete_BatchRemovesInOneCall(t *testing.T) {
	stubTrashMove(t)
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg", "c.jpg")}
	c := New(host)

	c.RequestFiles(targetsFor(host, 0, 1, 2))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if host.removeCalls != 1 {
		t.Errorf("RemoveFiles was called %d times, want exactly 1 for one batch", host.removeCalls)
	}
}

func TestPerformDelete_BatchOfEverythingFallsBackToEmptyState(t *testing.T) {
	stubTrashMove(t)
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg")}
	c := New(host)

	c.RequestFiles(targetsFor(host, 0, 1))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if len(host.emptied) != 1 {
		t.Errorf("ShowEmptyStateError calls = %v, want one - nothing is left to show", host.emptied)
	}
	if len(host.shown) != 0 {
		t.Errorf("ShowImage calls = %v, want none with an empty file set", host.shown)
	}
}

// TestPerformDelete_PartialFailureKeepsTheFilesItCouldNotMove: one bad file
// must not cost the user the rest of the batch, and the toast has to say so
// rather than claiming a clean run.
func TestPerformDelete_PartialFailureKeepsTheFilesItCouldNotMove(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "good.jpg", "bad.jpg", "alsogood.jpg")}
	c := New(host)

	badPath := host.files[1].Path()
	stubTrashMoveExcept(t, badPath)

	c.RequestFiles(targetsFor(host, 0, 1, 2))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if want := []int{0, 2}; !slices.Equal(host.removedBatch, want) {
		t.Errorf("RemoveFiles call = %v, want %v - only what actually moved", host.removedBatch, want)
	}
	if _, err := os.Stat(badPath); err != nil {
		t.Errorf("the file that failed to move should still be on disk: %v", err)
	}
	if len(host.toasts) != 1 {
		t.Fatalf("toasts = %v, want exactly one", host.toasts)
	}
	if !strings.Contains(host.toasts[0], "2") || !strings.Contains(host.toasts[0], "3") {
		t.Errorf("toast = %q, want it to report 2 of 3", host.toasts[0])
	}
}

// TestPerformDelete_TotalFailureRemovesNothing: every move failed, so the
// app's file set must be left exactly as it was.
func TestPerformDelete_TotalFailureRemovesNothing(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg")}
	c := New(host)
	stubTrashMoveExcept(t, host.files[0].Path(), host.files[1].Path())

	c.RequestFiles(targetsFor(host, 0, 1))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if host.removeCalls != 0 {
		t.Errorf("RemoveFiles was called %d times, want 0 when nothing moved", host.removeCalls)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "could not") {
		t.Errorf("toasts = %v, want one reporting the failure", host.toasts)
	}
}

// TestPerformDelete_BatchSkipsBookkeepingWhenTheFileSetMovedOn mirrors the
// single-file guard: the files still went to the Trash, but the captured
// indices no longer mean anything.
func TestPerformDelete_BatchSkipsBookkeepingWhenTheFileSetMovedOn(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg"), gen: 1}
	c := New(host)

	orig := trash.Move
	t.Cleanup(func() { trash.Move = orig })
	trash.Move = func(path string) error {
		// Stands in for a fresh drop landing while the batch is still in
		// flight - the generation is captured when the goroutine starts, so
		// it has to move after that to be the race this guards.
		host.gen = 2

		return os.Remove(path)
	}

	c.RequestFiles(targetsFor(host, 0, 1))
	c.setSelection(true)
	c.confirmSelection()
	c.Settle()

	if host.removeCalls != 0 {
		t.Error("RemoveFiles must not run against a file set that moved on during the async move")
	}
	if len(host.shown) != 0 || len(host.emptied) != 0 {
		t.Errorf("ShowImage = %v / ShowEmptyStateError = %v, want neither", host.shown, host.emptied)
	}
}

// TestHandleKey_EscapeCancelsABatch: the batch prompt is the same card, so
// backing out of it works the same way.
func TestHandleKey_EscapeCancelsABatch(t *testing.T) {
	host := &fakeHost{files: tempFiles(t, "a.jpg", "b.jpg")}
	c := New(host)

	c.RequestFiles(targetsFor(host, 0, 1))
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if c.Visible() {
		t.Error("Escape should dismiss the batch prompt")
	}
	if host.removeCalls != 0 {
		t.Error("Escape must not delete anything")
	}
	if _, err := os.Stat(host.files[0].Path()); err != nil {
		t.Errorf("the files should still exist on disk: %v", err)
	}
}

// stubTrashMove above makes every move succeed; this one fails for the named
// paths and removes the rest, so a partial failure can be driven exactly.
func stubTrashMoveExcept(t *testing.T, failing ...string) {
	t.Helper()

	orig := trash.Move
	t.Cleanup(func() { trash.Move = orig })

	trash.Move = func(path string) error {
		if slices.Contains(failing, path) {
			return errors.New("permission denied")
		}

		return os.Remove(path)
	}
}
