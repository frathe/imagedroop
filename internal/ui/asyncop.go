// asyncOpUI: the progress-UI bookkeeping shared by the folder scan
// (drop.go) and the background reorder (sort.go).

package ui

import (
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// asyncOpUI is the shape the folder scan (drop.go) and the background
// reorder (sort.go) share: one cancellable lifecycle, a flag saying whether
// that lifecycle's request is still meaningfully pending, a per-request done
// channel the test suite waits on, and the progress widgets shown for as
// long as it runs. Two instances of one type rather than two parallel sets
// of fields, so the flag-versus-token bookkeeping lives in one place instead
// of spread across drop.go, sort.go and keys.go.
//
// Deliberately viewer-independent: what to *do* about a cancelled operation
// - put the drop zone back, repaint, toast - differs between the two and
// stays at the call sites.
//
// A value field on viewer, never copied: it holds a lifecycle mutex.
type asyncOpUI struct {
	lifecycle requestLifecycle
	active    bool
	done      chan struct{}
	art       *canvas.Image // the scan's Trane-digging art; nil for the sort
	spinner   *widget.ProgressBarInfinite
	label     *widget.Label
}

// begin supersedes any request already in flight, marks the operation
// active, and installs a fresh done channel. The channel is returned as well
// as stored so the caller can capture it: a superseded request must still
// close its own channel without touching the field a newer one now owns.
func (o *asyncOpUI) begin() (requestToken, chan struct{}) {
	token := o.lifecycle.begin()
	o.active = true

	done := make(chan struct{})
	o.done = done

	return token, done
}

// show reveals the progress widgets. Separate from begin because the scan
// sets its label's text first. Nil-guarded: the sort instance has no art.
func (o *asyncOpUI) show() {
	if o.art != nil {
		o.art.Show()
	}
	o.spinner.Show()
	o.label.Show()
}

// finish clears the active flag and hides the progress widgets. Called by
// the completion step of whichever token is still current - never by a
// stale one, which must not report "nothing in flight" while a newer
// request is still running.
func (o *asyncOpUI) finish() {
	o.active = false
	if o.art != nil {
		o.art.Hide()
	}
	o.spinner.Hide()
	o.label.Hide()
}

// invalidate supersedes and cancels the current request, finishing the UI
// only if this operation was actually active. Returns the new revision.
func (o *asyncOpUI) invalidate() uint64 {
	revision := o.lifecycle.invalidate()
	if o.active {
		o.finish()
	}
	return revision
}

// cancel is invalidate guarded by the flag, reporting whether there was
// anything to cancel. The caller decides what the cancellation means.
func (o *asyncOpUI) cancel() bool {
	if !o.active {
		return false
	}
	o.invalidate()
	return true
}
