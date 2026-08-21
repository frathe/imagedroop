package favorites

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/ui/widgets"
)

// TestShowConfirmGivesTheKeyboardToItsPanelStartingOnCancel is removeFavorite's
// own bug fix (see confirm.go), pinned directly against showConfirm rather
// than only through the caller that happens to exercise it today.
func TestShowConfirmGivesTheKeyboardToItsPanelStartingOnCancel(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showConfirm(confirmation{title: "Title", message: "Message", action: "Confirm"})

	panel, ok := f.win.Canvas().Focused().(*widgets.ChoicePanel)
	if !ok {
		t.Fatalf("focused = %v, want the confirmation's choice panel", f.win.Canvas().Focused())
	}
	if got := panel.Selected(); got != cancelChoice {
		t.Errorf("selected = %d, want Cancel (%d): a prompt never opens with the action already under Return", got, cancelChoice)
	}
}

// TestShowConfirmCancelChoiceRunsOnCancelAfterTheDialogCloses covers the API
// surface removeFavorite itself never exercises, since it passes onCancel:
// nil - Return on the default Cancel selection has to run onCancel only once
// the confirmation is already off screen, not while it is still the top
// overlay, since Stage 5's Replace-Cancel is going to raise a dialog of its
// own from inside onCancel.
func TestShowConfirmCancelChoiceRunsOnCancelAfterTheDialogCloses(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	var ran int
	f.showConfirm(confirmation{
		title:   "Title",
		message: "Message",
		action:  "Confirm",
		onCancel: func() {
			ran++
			if n := len(f.win.Canvas().Overlays().List()); n != 0 {
				t.Errorf("overlay count = %d while onCancel ran, want the confirmation already gone", n)
			}
		},
	})

	typeKey(t, f.win, fyne.KeyReturn)

	if ran != 1 {
		t.Errorf("onCancel ran %d times, want 1", ran)
	}
}

// TestShowConfirmEscapeRunsOnCancelAfterTheDialogCloses is the other way to
// say Cancel, and has to leave onCancel looking at the same already-closed
// state Return does.
func TestShowConfirmEscapeRunsOnCancelAfterTheDialogCloses(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	var ran int
	f.showConfirm(confirmation{
		title:   "Title",
		message: "Message",
		action:  "Confirm",
		onCancel: func() {
			ran++
			if n := len(f.win.Canvas().Overlays().List()); n != 0 {
				t.Errorf("overlay count = %d while onCancel ran, want the confirmation already gone", n)
			}
		},
	})

	typeKey(t, f.win, fyne.KeyEscape)

	if ran != 1 {
		t.Errorf("onCancel ran %d times, want 1", ran)
	}
}

// TestShowConfirmOnClosedRunsBeforeOnConfirm pins the ordering showConfirm's
// doc comment documents and Stage 5 is built on: a real test rather than
// just trusting the comment.
func TestShowConfirmOnClosedRunsBeforeOnConfirm(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	var order []string
	f.showConfirm(confirmation{
		title:     "Title",
		message:   "Message",
		action:    "Confirm",
		onConfirm: func() { order = append(order, "confirm") },
		onClosed:  func() { order = append(order, "closed") },
	})

	typeKey(t, f.win, fyne.KeyRight)
	typeKey(t, f.win, fyne.KeyReturn)

	if want := []string{"closed", "confirm"}; !slices.Equal(order, want) {
		t.Errorf("callback order = %v, want %v", order, want)
	}
}

// TestShowConfirmOnClosedRunsBeforeOnCancel is the same pin for the other
// exit: Escape's onDismiss-then-onCancel order in ChoicePanel.TypedKey.
func TestShowConfirmOnClosedRunsBeforeOnCancel(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	var order []string
	f.showConfirm(confirmation{
		title:    "Title",
		message:  "Message",
		action:   "Confirm",
		onCancel: func() { order = append(order, "cancel") },
		onClosed: func() { order = append(order, "closed") },
	})

	typeKey(t, f.win, fyne.KeyEscape)

	if want := []string{"closed", "cancel"}; !slices.Equal(order, want) {
		t.Errorf("callback order = %v, want %v", order, want)
	}
}

// TestShowConfirmToleratesNoOnClosed guards the case removeFavorite's own
// call never hits: dialog.SetOnClosed's own callback runs unconditionally,
// so a nil confirmation.onClosed has to be handled by showConfirm itself, not
// left to panic the first time some future caller leaves it unset.
func TestShowConfirmToleratesNoOnClosed(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showConfirm(confirmation{title: "Title", message: "Message", action: "Confirm"})

	typeKey(t, f.win, fyne.KeyReturn)
}
