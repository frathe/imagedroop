package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Choice is one button on a ChoicePanel - and so on the ChoiceCard built
// around one: its label, the Fyne button importance it renders with (the zero
// value, widget.MediumImportance, for a plain choice - deletion's "Move to
// Trash" is the one so far that wants widget.DangerImportance instead), and
// what Confirm runs when it is the selected one.
type Choice struct {
	Label      string
	Importance widget.Importance
	OnChosen   func()
}

// ChoicePanel is a row of choice buttons and every rule about picking one:
// a focus ring on the selected button, Left/Right moving it (clamping, not
// wrapping), Return/Enter running the ringed choice, Escape cancelling.
// ChoiceCard wraps one into the app's modal prompt by adding a scrim, a
// message and its own visibility; on its own the panel is a fyne.Focusable
// widget, so it can also be a dialog's content and own the keyboard while
// that dialog is up - Fyne resolves Canvas.Focus/Focused through the *top
// overlay's* focus manager, so a dialog whose content focuses nothing leaves
// the canvas's unfocused key handler (this app's own dispatcher,
// internal/ui/keys.go) answering keys aimed at the dialog.
//
// The ring is drawn from the panel's own selected index either way, never
// from Fyne's widget-focus state: inside the card the panel is not focused at
// all (HandleKey feeds it the keys the app dispatcher already owns), and
// inside a dialog it is a single focus stop whose selection still has to move
// under the arrow keys. Same manual model internal/ui/favorites' managePanel
// uses over its two axes.
type ChoicePanel struct {
	widget.BaseWidget

	// repaint is called after every selection change - the app has no
	// automatic redraw loop, so a hidden window has to be told to paint again
	// itself (see viewer.ForceRepaint, which every caller so far passes in
	// directly). Optional: content inside a dialog Fyne draws for itself has
	// nothing to add here.
	repaint func()

	// onDismiss takes whatever contains the panel off screen - see
	// SetOnDismiss. Nil for a container that stays up regardless.
	onDismiss func()

	// onCancel is what Escape runs once the panel is dismissed, beyond
	// dismissing it - see SetOnCancel. Nil is a valid, common choice: a panel
	// whose index-0 choice already does nothing more than dismiss (deletion's
	// Cancel) has nothing left for Escape to do.
	onCancel func()

	// onBack is what Up runs - see SetOnBack. Nil for every panel that lives
	// inside a ChoiceCard (deletion, the export prompt): those are fed keys by
	// the app's own dispatcher, where Up already means something else, and
	// leaving the field nil is what keeps this panel from stepping on it.
	onBack func()

	choices  []Choice
	selected int

	content fyne.CanvasObject
	buttons []*widget.Button
	rings   []*canvas.Rectangle
}

// NewChoicePanel builds the button row over the given choices, left to right.
// Index 0 is the leftmost button and the default selection.
func NewChoicePanel(repaint func(), choices ...Choice) *ChoicePanel {
	p := &ChoicePanel{repaint: repaint, choices: choices}
	p.ExtendBaseWidget(p)

	cells := make([]fyne.CanvasObject, len(choices))
	p.buttons = make([]*widget.Button, len(choices))
	p.rings = make([]*canvas.Rectangle, len(choices))
	for i, choice := range choices {
		btn := widget.NewButton(choice.Label, p.runChoice(i))
		btn.Importance = choice.Importance

		ring := NewFocusRing(ButtonRingWidth, RingRadius)
		if i != 0 {
			ring.Hide()
		}
		p.buttons[i] = btn
		p.rings[i] = ring
		cells[i] = Ringed(ring, btn)
	}
	// One column per choice, except for the choiceless panel Select and
	// runChoice already tolerate: a zero-column grid divides by its own
	// column count while laying out, so it gets a single empty column
	// instead.
	cols := max(len(choices), 1)
	p.content = container.NewGridWithColumns(cols, cells...)

	return p
}

func (p *ChoicePanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

// Selected is the index Left/Right (or Select) last moved the ring to.
func (p *ChoicePanel) Selected() int {
	return p.selected
}

// Ring is the selection ring drawn behind choice i, or nil for an
// out-of-range index.
func (p *ChoicePanel) Ring(i int) *canvas.Rectangle {
	if i < 0 || i >= len(p.rings) {
		return nil
	}

	return p.rings[i]
}

// SetOnDismiss registers what takes the panel off screen: ChoiceCard hides
// the card, a dialog closes itself. It runs before any choice's OnChosen and
// before onCancel, so an action that raises something of its own doesn't have
// to dismiss this prompt first. Optional, for a container that stays up
// regardless.
func (p *ChoicePanel) SetOnDismiss(onDismiss func()) {
	p.onDismiss = onDismiss
}

// SetOnCancel registers what Escape runs once the panel is dismissed, in
// addition to dismissing it. Optional: a panel whose index-0 choice already
// does nothing beyond dismissing has nothing more for Escape to do.
func (p *ChoicePanel) SetOnCancel(onCancel func()) {
	p.onCancel = onCancel
}

// SetOnBack registers what Up runs: the panel is one stop in a larger
// keyboard story and Up is how the user leaves it upwards (internal/ui/
// favorites' Add dialog, whose name field is the stop above the panel of
// Cancel/Add buttons below it). Optional - nil leaves Up ignored, which is
// what every panel inside a ChoiceCard wants, since the app dispatcher feeds
// those keys and Up means something else out there.
func (p *ChoicePanel) SetOnBack(onBack func()) {
	p.onBack = onBack
}

// SetChoiceEnabled enables or disables choice i. A disabled choice runs
// nothing and dismisses nothing, whether it is clicked or confirmed from the
// keyboard, and renders greyed - the same deal a disabled button in a Fyne
// dialog.FormDialog offers, which is what the Add dialog this exists for
// replaces. There is deliberately no parallel []bool tracking this: the
// button itself (Enable/Disable/Disabled) is the one place enabled state
// lives, so the greyed rendering and ChoiceEnabled's answer can never
// disagree with each other. Out-of-range indices are a no-op.
func (p *ChoicePanel) SetChoiceEnabled(i int, enabled bool) {
	if i < 0 || i >= len(p.buttons) {
		return
	}

	if enabled {
		p.buttons[i].Enable()
	} else {
		p.buttons[i].Disable()
	}
}

// ChoiceEnabled reports whether choice i can currently be run. False for an
// out-of-range index - there is no button there to be either.
func (p *ChoicePanel) ChoiceEnabled(i int) bool {
	if i < 0 || i >= len(p.buttons) {
		return false
	}

	return !p.buttons[i].Disabled()
}

// Select moves the selection to index i, clamping to the choice range rather
// than wrapping, and redraws whichever ring now marks it.
//
// The high end is clamped before the low end, not after, so that a panel
// built with no choices at all still lands on 0 rather than on len-1 == -1 -
// a negative index that would then panic in runChoice. A choiceless panel is
// a caller's mistake either way, but an inert panel is a far easier mistake
// to find than an index-out-of-range on whatever key press happens next.
//
// Select deliberately still moves the ring onto a disabled choice rather than
// skipping over it: a greyed button under the ring is the app telling the
// user why Return just did nothing (the Add dialog's Add, before a valid
// name is typed), and Fyne's own dialog.FormDialog leaves a disabled Submit
// focusable too, rather than jumping focus away from it. Do not "fix" this
// into a skip - a caller that wants Left/Right to bypass a disabled choice
// has to say so itself, this panel has no opinion on it.
func (p *ChoicePanel) Select(i int) {
	if last := len(p.choices) - 1; i > last {
		i = last
	}
	if i < 0 {
		i = 0
	}

	p.selected = i
	for idx, ring := range p.rings {
		if idx == i {
			ring.Show()
		} else {
			ring.Hide()
		}
	}
	// Refreshing the panel rather than trusting each ring's own Show/Hide:
	// Fyne only registers an object with its canvas the first time it is
	// painted while visible, so a ring that has been hidden since the panel
	// went up has no canvas to mark dirty and would silently fail to appear
	// (see viewer.ForceRepaint for the same trap).
	p.Refresh()

	if p.repaint != nil {
		p.repaint()
	}
}

// Confirm runs whichever choice is currently selected - Return/Enter while
// the panel has the keys, or a caller's own test seam (deletion's
// confirmSelection) calling it directly.
func (p *ChoicePanel) Confirm() {
	p.runChoice(p.selected)()
}

// runChoice is choice i's button OnTapped: a click always runs that specific
// button's action, regardless of what Left/Right currently has selected - the
// same as Confirm, but by index rather than by whatever TypedKey last moved
// the ring to.
//
// A disabled choice is checked first and returns before onDismiss runs at
// all: a disabled choice must not take the prompt down any more than it runs
// OnChosen, so Return on a greyed Add in the Add dialog does nothing
// whatsoever rather than closing the dialog with nothing saved. That check is
// deliberately not ChoiceEnabled(i), which reports false for an out-of-range
// index too - here an out-of-range i names no button to be disabled, so it
// has to fall through to the dismiss-then-range-check pair below rather than
// being swallowed by this guard, or the choiceless panel would stop
// dismissing on Return. test.Tap never reaches this func for a disabled
// button - widget.Button.Tapped checks Disabled() itself - but Confirm (the
// keyboard path) goes straight through runChoice, so the guard has to live
// here to cover both.
//
// The panel is otherwise dismissed first, before the action and before the
// range check below, so an action that shows something of its own doesn't
// have to take this prompt down first. The range check covers the one index
// that can reach here without naming a button: Select's clamp on a panel
// built with no choices at all. That panel still dismisses, so even that
// mistake dismisses rather than wedging.
func (p *ChoicePanel) runChoice(i int) func() {
	return func() {
		if i >= 0 && i < len(p.buttons) && p.buttons[i].Disabled() {
			return
		}

		if p.onDismiss != nil {
			p.onDismiss()
		}

		if i < 0 || i >= len(p.choices) {
			return
		}

		if fn := p.choices[i].OnChosen; fn != nil {
			fn()
		}
	}
}

// TypedKey handles a key press while the panel holds the keyboard - or one
// handed to it by a caller that owns the keyboard itself
// (ChoiceCard.HandleKey): Left/Right move the selection (clamping at either
// end), Return/Enter runs whichever is selected, Escape dismisses the panel
// and runs onCancel if one is registered, Up runs onBack if one is
// registered. Every other key is deliberately left alone, so a caller can
// still make its own use of it.
func (p *ChoicePanel) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyLeft:
		p.Select(p.selected - 1)
	case fyne.KeyRight:
		p.Select(p.selected + 1)
	case fyne.KeyReturn, fyne.KeyEnter:
		p.Confirm()
	case fyne.KeyEscape:
		if p.onDismiss != nil {
			p.onDismiss()
		}
		if p.onCancel != nil {
			p.onCancel()
		}
	case fyne.KeyUp:
		// Deliberately not p.Select(...) of anything - Up leaves the panel
		// rather than moving within it (see onBack's field comment), so the
		// selection must be exactly where it was found when the caller comes
		// back down with Down.
		if p.onBack != nil {
			p.onBack()
		}
	}
}

// TypedRune ignores every rune: the panel has no type-ahead, and a stray
// character must not disturb the selection.
func (p *ChoicePanel) TypedRune(rune) {}

// FocusGained and FocusLost have nothing to do: the ring is drawn from the
// panel's own selected index rather than from Fyne's focus state (see the
// type comment), and losing the keyboard to something transient on top is no
// reason to stop showing where the selection stands.
func (p *ChoicePanel) FocusGained() {}

func (p *ChoicePanel) FocusLost() {}
