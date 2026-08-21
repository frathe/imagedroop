package favorites

import (
	"errors"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/ui/widgets"
)

// nameEntry is the Add to Favorites dialog's name field, and the first of
// its two keyboard stops - the second is the widgets.ChoicePanel below it,
// wired up in newAddDialog. A plain widget.Entry cannot fill that role on
// its own: its TypedKey switch (widget/entry.go) has no fyne.KeyEscape case
// at all, so Escape aimed at the dialog dies silently in the field instead
// of closing it, and its Down key moves a cursor *row* - meaningless on a
// single-line entry, but still consumed - leaving no key that means "I am
// done typing, take me to the buttons below". nameEntry adds exactly those
// two meanings and nothing else; every other key it leaves to the embedded
// Entry.
type nameEntry struct {
	widget.Entry

	// onEscape and onDown are both optional: nil leaves the corresponding
	// key inert (still consumed, just with nothing to run) rather than
	// falling through to Entry.TypedKey, which is what a caller building the
	// field before it has anywhere to send those keys wants.
	onEscape func()
	onDown   func()
}

// newNameEntry builds an unfocused nameEntry ready to be placed in a dialog.
//
// ExtendBaseWidget(e) is not optional decoration here: without it, Fyne's
// focus and render machinery operates on the embedded widget.Entry (the
// BaseWidget it finds first walking the struct), not on *nameEntry, and this
// type's own TypedKey override is silently never called - the field would
// render and accept typed runes exactly as before, so nothing would look
// wrong until Escape or Down were pressed and turned out to do nothing. A
// later reader chasing that has to know this line is where it would show up.
func newNameEntry(onEscape, onDown func()) *nameEntry {
	e := &nameEntry{onEscape: onEscape, onDown: onDown}
	e.ExtendBaseWidget(e)
	return e
}

// TypedKey handles the two keys that make this field a keyboard stop rather
// than a dead end, and leaves every other key - including Return/Enter,
// deliberately - to the embedded Entry. A single-line widget.Entry already
// calls OnSubmitted(text) for Return (typedKeyReturn in Fyne's own
// widget/entry.go), which is exactly the hook a caller wants for "save on
// Return in the field", so intercepting it here would only get in the way.
// Delegating everything else (not just a named list of keys) is what keeps
// Left/Right, Home, End, Backspace, Delete and selection all working - this
// type only adds meaning, it does not take any of the field's own away.
func (e *nameEntry) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyEscape:
		// Consumed either way, hook or no hook: an Escape that reached
		// Entry.TypedKey would do nothing (it has no case for it) and then
		// fall through to whatever the app's own dispatcher does with an
		// unhandled key - exactly the dead field this type exists to fix.
		if e.onEscape != nil {
			e.onEscape()
		}
	case fyne.KeyDown:
		// Same reasoning as Escape: Entry's own Down case moves a cursor row
		// a single-line field does not have, so letting it through would be
		// a silent no-op rather than "leave the field", which is what this
		// key is repurposed to mean here.
		if e.onDown != nil {
			e.onDown()
		}
	default:
		e.Entry.TypedKey(ev)
	}
}

// addDialogWidth is the min width of the transparent canvas.Rectangle
// newAddDialog stacks behind its content. An entry's own minimum width is
// only a few characters wide, and a widgets.ChoicePanel is no wider than its
// two button labels, so a buttonless dialog.NewCustomWithoutButtons sized to
// that content alone would open as a sliver - managePanel's scroll
// (manage.go) sets a floor on its own minimum size for exactly the same
// reason.
const addDialogWidth = 360

// addPanel is the Add to Favorites dialog's content and both of its
// keyboard stops: the name field, which holds the keyboard on open, and the
// widgets.ChoicePanel below it, which Down hands the keyboard to and Up
// hands it back from.
type addPanel struct {
	entry   *nameEntry
	choices *widgets.ChoicePanel
	content fyne.CanvasObject
}

// newAddDialog builds the Add to Favorites dialog (unshown) with initial
// already sitting in its name field - "" for a fresh "Add Current List to
// Favorites…", a name Stage 5's Replace-Cancel is handing back after a
// clash so the user does not have to retype it.
//
// d and choices are declared before either is built, the same forward
// reference showConfirm's own "var confirm dialog.Dialog" uses: the entry's
// hooks need to reach the dialog and the panel, and the panel needs to reach
// the dialog, before any of the three exist yet. By the time a key press
// actually runs one of these closures, all three are assigned - only the
// order they are *declared* in requires this.
func (f *Feature) newAddDialog(initial string) (dialog.Dialog, *addPanel) {
	var d dialog.Dialog
	var choices *widgets.ChoicePanel

	entry := newNameEntry(
		func() { d.Hide() },
		func() { f.win.Canvas().Focus(choices) },
	)
	entry.SetText(initial)

	// Unchanged from the dialog.NewForm this dialog replaces: the regexp
	// rejects the characters a filesystem path cannot carry, and the
	// trimmed-name check catches everything else favstore.ValidName refuses
	// (bare "..", an all-whitespace name) so the field's own inline feedback
	// agrees with what saveFavorite would do anyway.
	reason := lang.L(`enter a name without / \ : * ? " < > |`)
	entry.Validator = validation.NewAllStrings(
		validation.NewRegexp(`^[^/\\:*?"<>|]+$`, reason),
		func(name string) error {
			if !favstore.ValidName(strings.TrimSpace(name)) {
				return errors.New(reason)
			}
			return nil
		},
	)

	choices = widgets.NewChoicePanel(nil,
		// repaint is nil, as showConfirm's panel is: Fyne redraws a dialog's
		// content for itself, there is no hidden window here that needs to be
		// told to paint again.
		widgets.Choice{Label: lang.L("Cancel")},
		widgets.Choice{Label: lang.L("Add"), OnChosen: func() { f.saveFavorite(entry.Text) }},
	)
	choices.SetOnBack(func() { f.win.Canvas().Focus(entry) })
	choices.SetOnDismiss(func() { d.Hide() })

	// entry.OnChanged, not entry.SetOnValidationChanged - the callback the
	// data/validation package's own doc comment points a reader at first.
	// widget.Entry.setValidationError (Fyne v2.8.0,
	// widget/entry_validation.go) suppresses onValidationChanged for any
	// transition *into* an error state while the entry still has focus, only
	// forcing a fresh validate() on FocusLost - and this field keeps focus
	// for its entire useful lifetime, since Down (not a blur) is the only way
	// to leave it while the dialog stays open. Wired the obvious way, typing
	// a valid name and then a stray "/" would leave Add's enabled state stuck
	// on "valid", so Return would sail straight past entry.OnSubmitted's own
	// guard into saveFavorite's toast-only rejection instead of being stopped
	// at the ring - confirmed against a standalone probe of widget.Entry
	// before settling on this. entry.Validate()'s return value is not itself
	// subject to that suppression (only the internal validationError/inline
	// message/callback triple is), so calling it ourselves on every keystroke
	// keeps SetChoiceEnabled accurate live while leaving Fyne's own inline
	// feedback - which still goes through the same SetValidationError, and so
	// still enjoys the same not-mid-word suppression - exactly as it always
	// behaves.
	entry.OnChanged = func(string) {
		choices.SetChoiceEnabled(confirmChoice, entry.Validate() == nil)
	}
	// The seed above is not optional decoration: OnChanged only fires on a
	// text *change*, so it never runs for this dialog's own construction -
	// not for a fresh empty field (Add must still start disabled) and not
	// for entry.SetText(initial) a few lines up, which ran before Validator
	// even existed to validate against. Both cases are settled here, once,
	// explicitly.
	choices.SetChoiceEnabled(confirmChoice, entry.Validate() == nil)

	entry.OnSubmitted = func(string) {
		// Checked first, before anything else runs: an invalid Return must
		// change nothing at all, not even the ring - if this ran
		// choices.Select first the ring would land on Add regardless of
		// what running it then did.
		if !choices.ChoiceEnabled(confirmChoice) {
			return
		}
		// Through the panel's own path rather than calling f.saveFavorite
		// directly: this keeps exactly one dismiss-then-run ordering for
		// both ways of saying Add, Return in the field and a click on the
		// button, rather than the field's shortcut skipping the dismiss the
		// button's path goes through.
		choices.Select(confirmChoice)
		choices.Confirm()
	}

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(addDialogWidth, 0))
	body := container.NewVBox(widget.NewLabel(lang.L("Name")), entry, choices)
	content := container.NewStack(rect, body)

	d = dialog.NewCustomWithoutButtons(lang.L("Add to Favorites"), content, f.win)
	d.SetOnClosed(func() {
		// The same superseded-dialog guard ShowManage makes for itself: this
		// callback fires from inside d.Hide(), and a caller that reopens the
		// dialog (Stage 5's Replace-Cancel) has already Hide-then-shown a new
		// one by the time this one's own teardown runs, so it must not clear
		// fields that now belong to that new dialog.
		if f.addDialog != d {
			return
		}

		f.addDialog, f.addPanel = nil, nil
		// The release grid.Overview.Close and ShowManage's own dialog both
		// perform, for the same reason: every other key binding in this app
		// is dispatched from the canvas's own unfocused handler, so a focus
		// left behind here would swallow key presses afterwards.
		f.win.Canvas().Unfocus()
	})

	return d, &addPanel{entry: entry, choices: choices, content: content}
}

// showAdd raises the Add to Favorites dialog with initial already in its
// name field - "" from the Favorites menu's own "Add Current List to
// Favorites…" item, a typed name from Stage 5's Replace-Cancel. A no-op
// while one is already up: the menu bar stays live while a Fyne dialog is up
// (they are canvas overlays, not OS-modal windows), so the menu item can be
// chosen twice - the same guard ShowManage makes for the same reason.
func (f *Feature) showAdd(initial string) {
	if f.addDialog != nil {
		return
	}

	d, panel := f.newAddDialog(initial)
	f.addDialog, f.addPanel = d, panel
	d.Show()
	// After Show, not before: Fyne can only focus an object that is already
	// part of an overlay it can walk to, and the field is only part of one
	// once the dialog holding it is up.
	f.win.Canvas().Focus(panel.entry)
}
