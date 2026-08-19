package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Choice is one button on a ChoiceCard: its label, the Fyne button
// importance it renders with (the zero value, widget.MediumImportance, for
// a plain choice - deletion's "Move to Trash" is the one so far that wants
// widget.DangerImportance instead), and what Confirm runs when it is the
// selected one.
type Choice struct {
	Label      string
	Importance widget.Importance
	OnChosen   func()
}

// ChoiceCard is a dimmed scrim behind a centered message-and-buttons card,
// with manual Left/Right selection and Return/Escape handling - the modal
// prompt shape deletion's Shift+Delete confirmation originated. Selection is
// drawn with a focus ring per button rather than relying on Fyne's own
// widget-focus system, because this app never uses that (modified key
// combos already have to bypass it - see the app's wireOpenShortcuts
// comment), so HandleKey drives the rings manually while the card is up.
type ChoiceCard struct {
	// repaint is called after every visibility or selection change - the
	// app has no automatic redraw loop, so a hidden window has to be told
	// to paint again itself (see viewer.ForceRepaint, which every caller so
	// far passes in directly).
	repaint func()

	// onCancel is what Escape runs once the card is hidden, beyond hiding
	// it - see SetOnCancel. Nil is a valid, common choice: a card whose
	// index-0 choice already does nothing more than dismiss (deletion's
	// Cancel) has nothing left for Escape to do.
	onCancel func()

	choices  []Choice
	selected int

	visible bool
	overlay *fyne.Container
	message *widget.Label
	rings   []*canvas.Rectangle
}

// NewChoiceCard builds the card (hidden) with the given choices, left to
// right. Index 0 is the leftmost button and the default selection.
func NewChoiceCard(repaint func(), choices ...Choice) *ChoiceCard {
	c := &ChoiceCard{repaint: repaint, choices: choices}

	scrim := canvas.NewRectangle(ScrimColor)

	c.message = widget.NewLabel("")
	c.message.Alignment = fyne.TextAlignCenter
	c.message.Wrapping = fyne.TextWrapWord

	cells := make([]fyne.CanvasObject, len(choices))
	c.rings = make([]*canvas.Rectangle, len(choices))
	for i, choice := range choices {
		btn := widget.NewButton(choice.Label, c.runChoice(i))
		btn.Importance = choice.Importance

		ring := NewFocusRing(ButtonRingWidth, RingRadius)
		if i != 0 {
			ring.Hide()
		}
		c.rings[i] = ring
		cells[i] = ringed(ring, btn)
	}
	// One column per choice, except for the choiceless card Select and
	// runChoice already tolerate: a zero-column grid divides by its own
	// column count while laying out, so it gets a single empty column
	// instead.
	cols := max(len(choices), 1)
	buttons := container.NewGridWithColumns(cols, cells...)

	cardBG := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	cardBG.CornerRadius = CardRadius
	card := container.NewStack(cardBG, container.NewPadded(container.NewVBox(c.message, buttons)))

	c.overlay = container.NewStack(scrim, container.NewCenter(card))
	c.overlay.Hide()

	return c
}

// ringed pairs a button with its selection ring: the ring fills the cell,
// the button is inset by one padding step inside it, so the ring's stroke
// lands in that gap instead of underneath the button. Stacking the two at
// the same size hides the ring entirely - a Fyne button paints an opaque
// background across its whole area, including the DangerImportance red -
// and the card then looks identical whichever button is selected. Behind
// rather than on top so the ring can never sit between the pointer and the
// button it marks.
func ringed(ring *canvas.Rectangle, btn *widget.Button) *fyne.Container {
	return container.NewStack(ring, container.NewPadded(btn))
}

// runChoice is choice i's button OnTapped: a click always runs that
// specific button's action, regardless of what Left/Right currently has
// selected - the same as Confirm, but by index rather than by whatever
// HandleKey last moved the ring to.
//
// The range check covers the one index that can reach here without naming a
// button: Select's clamp on a card built with no choices at all. The card
// still hides, so even that mistake dismisses rather than wedging.
func (c *ChoiceCard) runChoice(i int) func() {
	return func() {
		c.Hide()

		if i < 0 || i >= len(c.choices) {
			return
		}

		if fn := c.choices[i].OnChosen; fn != nil {
			fn()
		}
	}
}

// Overlay is the card, for the caller to place in its window stack.
func (c *ChoiceCard) Overlay() fyne.CanvasObject {
	return c.overlay
}

// Visible reports whether the card is up.
func (c *ChoiceCard) Visible() bool {
	return c.visible
}

// Selected is the index Left/Right (or Select) last moved the ring to - a
// test seam, mirrored by Message and Ring below for consumers built
// directly on the card that need to assert on its rendered state.
func (c *ChoiceCard) Selected() int {
	return c.selected
}

// Message is the card's headline label.
func (c *ChoiceCard) Message() *widget.Label {
	return c.message
}

// Ring is the selection ring drawn behind choice i, or nil for an
// out-of-range index.
func (c *ChoiceCard) Ring(i int) *canvas.Rectangle {
	if i < 0 || i >= len(c.rings) {
		return nil
	}

	return c.rings[i]
}

// SetOnCancel registers what Escape runs once the card is hidden, in
// addition to hiding it. Optional: a card whose index-0 choice already does
// nothing beyond dismissing has nothing more for Escape to do.
func (c *ChoiceCard) SetOnCancel(onCancel func()) {
	c.onCancel = onCancel
}

// Show raises the card with the given message, resetting the selection to
// index 0 - never carried over from whatever a previous prompt left it at.
func (c *ChoiceCard) Show(message string) {
	c.message.SetText(message)
	c.Select(0)

	c.visible = true
	c.overlay.Show()
	if c.repaint != nil {
		c.repaint()
	}
}

// Hide dismisses the card without running any choice - Escape, a caller's
// own guarded dismissal (deletion.Confirmer.Cancel), or Confirm/runChoice
// hiding it before the chosen action runs. Always repaints, even when the
// card is already hidden: a caller that just hid it through some other path
// still wants the window redrawn.
func (c *ChoiceCard) Hide() {
	c.visible = false
	c.overlay.Hide()
	if c.repaint != nil {
		c.repaint()
	}
}

// Select moves the selection to index i, clamping to the choice range
// rather than wrapping, and redraws whichever ring now marks it.
//
// The high end is clamped before the low end, not after, so that a card
// built with no choices at all still lands on 0 rather than on len-1 == -1 -
// a negative index that would then panic in runChoice. A choiceless card is
// a caller's mistake either way, but an inert card is a far easier mistake
// to find than an index-out-of-range on whatever key press happens next.
func (c *ChoiceCard) Select(i int) {
	if last := len(c.choices) - 1; i > last {
		i = last
	}
	if i < 0 {
		i = 0
	}

	c.selected = i
	for idx, ring := range c.rings {
		if idx == i {
			ring.Show()
		} else {
			ring.Hide()
		}
	}

	if c.repaint != nil {
		c.repaint()
	}
}

// Confirm runs whichever choice is currently selected - Return/Enter while
// the card is up, or deletion's own confirmSelection test seam calling it
// directly. The card hides before the choice's OnChosen runs, so an action
// that shows something else of its own doesn't have to hide this card
// first.
func (c *ChoiceCard) Confirm() {
	c.runChoice(c.selected)()
}

// HandleKey handles a key press while the card is up: Left/Right move the
// selection (clamping at either end), Return/Enter runs whichever is
// selected, Escape hides the card and runs onCancel if one is registered.
// Every other key is deliberately left to the caller.
func (c *ChoiceCard) HandleKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyLeft:
		c.Select(c.selected - 1)
	case fyne.KeyRight:
		c.Select(c.selected + 1)
	case fyne.KeyReturn, fyne.KeyEnter:
		c.Confirm()
	case fyne.KeyEscape:
		c.Hide()
		if c.onCancel != nil {
			c.onCancel()
		}
	}
}
