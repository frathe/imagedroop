// Non-blocking toast notifications.

package ui

import (
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/frathe/picfetch/internal/ui/widgets"
)

// toastDuration is how long a toast stays on screen before auto-dismissing.
const toastDuration = 10 * time.Second

// toast is the self-dismissing notification card that replaces
// dialog.ShowError for recoverable problems: unlike a modal dialog it
// doesn't stop the event loop, so keyboard navigation and further drops
// keep working while it's up. It owns its widgets and its auto-hide
// lifecycle; the viewer composes one and exposes it through ShowToast.
type toast struct {
	text *canvas.Text
	card *fyne.Container

	// gen mirrors the staleness-guard pattern used for image loads: it
	// lets a newer toast's timer win, instead of an older one hiding the
	// card out from under a message that replaced it. Atomic - unlike the
	// pre-component plain counter - because the auto-hide goroutine reads
	// it while the UI goroutine may already be showing a newer toast.
	gen atomic.Uint64

	// duration is how long the card stays up before auto-hiding. A field
	// rather than a package var so tests inject their own: the test
	// constructor sets it to an hour, so a pending timer never fires
	// mid-suite - tests drive the hide synchronously via settleToast
	// (library_test.go) instead of waiting out real time, which is also
	// what keeps the auto-hide goroutine from ever touching widgets
	// concurrently with a test goroutine under the fyne test driver's
	// inline fyne.Do.
	duration time.Duration

	// stop cancels the pending auto-hide goroutine without hiding
	// anything (closed by the next show call, or by a test's settleToast);
	// done is closed when that goroutine exits, whichever way it went.
	// Both are per-show and only ever swapped on the UI goroutine.
	stop chan struct{}
	done chan struct{}

	// repaint forces the whole window to redraw after the card shows or
	// hides - the viewer's ForceRepaint, injected so this component
	// doesn't need the viewer itself.
	repaint func()
}

// newToast builds the toast card (hidden) with the production auto-hide
// duration. repaint is called after every visibility change.
func newToast(repaint func()) *toast {
	bg := canvas.NewRectangle(widgets.ToastBGColor)
	bg.CornerRadius = widgets.CardRadius
	text := canvas.NewText("", widgets.ToastTextColor)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	card := container.NewStack(bg, container.NewPadded(text))
	card.Hide()

	return &toast{
		text:     text,
		card:     card,
		duration: toastDuration,
		repaint:  repaint,
	}
}

// show displays msg and schedules the auto-hide. A previous toast's pending
// timer is cancelled outright - not just superseded via gen - so at most
// one auto-hide goroutine exists at a time and a replaced toast's timer
// never wakes up later just to discover it's stale.
func (t *toast) show(msg string) {
	gen := t.gen.Add(1)
	t.cancelAutoHide()

	stop := make(chan struct{})
	done := make(chan struct{})
	t.stop, t.done = stop, done

	t.text.Text = msg
	t.text.Refresh()
	t.card.Show()
	t.repaint()

	go func() {
		defer close(done)

		select {
		case <-time.After(t.duration):
		case <-stop:
			return
		}

		fyne.Do(func() {
			t.autoHide(gen)
		})
	}()
}

// cancelAutoHide stops the pending auto-hide goroutine, if any, without
// touching the card. Only ever called on the UI goroutine (show above,
// settleToast in tests), so the nil-out needs no synchronization.
func (t *toast) cancelAutoHide() {
	if t.stop != nil {
		close(t.stop)
		t.stop = nil
	}
}

// autoHide hides the card if gen still identifies the current toast - the
// staleness check that lets a newer toast's message survive an older
// timer. Split out from show's goroutine so tests can drive the exact
// hide path synchronously (see settleToast).
func (t *toast) autoHide(gen uint64) {
	if gen != t.gen.Load() {
		return
	}
	t.card.Hide()
	t.repaint()
}

// ShowToast displays a short, non-blocking notification - see the toast
// type above for the mechanics.
func (v *viewer) ShowToast(msg string) {
	v.toast.show(msg)
}
