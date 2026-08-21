package favorites

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/ui/widgets"
)

// cancelChoice and confirmChoice are the two button indices every
// confirmation this package raises uses: Cancel first/left and so the
// default selection, the confirming action second/right - the same ordering
// rule deletion's cancelChoice/dangerChoice sets, and for the same reason: a
// prompt never opens, or reopens, with the action already under Return.
const (
	cancelChoice  = 0
	confirmChoice = 1
)

// confirmation describes one keyboard-driven two-choice prompt: a title, a
// message, the confirming button's own label and importance, and what runs
// on each way the prompt can go.
type confirmation struct {
	title      string
	message    string
	action     string            // the confirming button's label
	importance widget.Importance // widget.DangerImportance for a destructive action
	onConfirm  func()
	onCancel   func() // the Cancel choice and Escape both; nil for "just close"
	onClosed   func() // whichever way the dialog goes; nil for nothing
}

// showConfirm raises c as a two-choice confirmation and hands it the
// keyboard, returning the dialog for a caller that needs to hold onto it -
// removeFavorite's own manageDialog-style superseded-dialog guard is the
// pattern a future caller of this func would follow.
//
// Not dialog.NewConfirm: that focuses nothing inside itself, and Fyne
// resolves Canvas.Focused through the *top overlay's* focus manager only, so
// its confirmation left Focused() nil - which is exactly the state in which
// the glfw driver routes keys to the canvas's unfocused handler, this app's
// own dispatcher (internal/ui/keys.go). Escape then reset the session behind
// the prompt, and Return answered nothing at all, since a focused Fyne
// button reacts to Space and never to Return - that history is
// removeFavorite's origin, and is why every confirmation this package raises
// now goes through here instead.
//
// A widgets.ChoicePanel is a fyne.Focusable, so making it the dialog's
// content puts the keyboard where the user is looking, exactly as
// managePanel does for the dialog underneath it. The dialog carries no
// dismiss button of its own (NewCustomWithoutButtons): Cancel is already one
// of the two choices, and a Close beside it would be a second way to say the
// same thing.
//
// Callback ordering, load-bearing for a caller whose onCancel raises a
// dialog of its own: ChoicePanel dismisses itself - which is what fires
// onClosed, through this dialog's own Hide - before it runs the chosen
// action or onCancel (see ChoicePanel.runChoice, which calls onDismiss
// before OnChosen, and the Escape arm of its TypedKey, which calls onDismiss
// before onCancel). onClosed therefore always finishes running before
// onConfirm or onCancel even starts, so whatever onCancel raises of its own
// cannot be undone by this dialog's own teardown arriving late and
// unfocusing it.
func (f *Feature) showConfirm(c confirmation) dialog.Dialog {
	// Unwrapped, unlike Fyne's own text dialogs: they wrap the message and
	// then widen the dialog to fit it from a beforeShowHook, which a custom
	// dialog has no equivalent of, so a wrapping label here would collapse to
	// its own minimum and stack the sentence four words to a line.
	message := &widget.Label{
		Text:      c.message,
		Alignment: fyne.TextAlignCenter,
	}

	var confirm dialog.Dialog
	panel := widgets.NewChoicePanel(nil,
		// Cancel first/left and so the default selection, the confirming
		// action second/right - see cancelChoice/confirmChoice. Cancel's own
		// OnChosen is c.onCancel too, not left nil: Return (or a click) on
		// the default selection has to mean the same thing Escape does, and
		// a caller like the Replace prompt (which reopens the Add dialog
		// from onCancel) needs both paths there to agree.
		widgets.Choice{Label: lang.L("Cancel"), OnChosen: c.onCancel},
		widgets.Choice{
			Label:      c.action,
			Importance: c.importance,
			OnChosen:   c.onConfirm,
		},
	)
	// The panel dismisses before running any choice's OnChosen (which is
	// what fires onClosed - see this func's own doc comment on the ordering)
	// and before Escape's onCancel, so both close paths need nothing further
	// here beyond registering onCancel for Escape - SetOnCancel is what
	// TypedKey's KeyEscape arm runs, cancelChoice's own OnChosen above is
	// what a Return or a click on it runs.
	panel.SetOnDismiss(func() { confirm.Hide() })
	panel.SetOnCancel(c.onCancel)

	confirm = dialog.NewCustomWithoutButtons(c.title, container.NewVBox(message, panel), f.win)
	// dialog.SetOnClosed's own callback calls the func handed to it
	// unconditionally, with no nil check of its own - a bare
	// confirm.SetOnClosed(c.onClosed) would panic on Hide the first time a
	// caller leaves onClosed unset, so the nil check has to live here.
	confirm.SetOnClosed(func() {
		if c.onClosed != nil {
			c.onClosed()
		}
	})
	confirm.Show()
	// After Show, for the reason ShowManage focuses its own panel after Show:
	// Fyne can only focus an object that is already part of an overlay it can
	// walk to.
	f.win.Canvas().Focus(panel)

	return confirm
}
