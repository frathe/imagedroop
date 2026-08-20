package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ChoiceCard is a dimmed scrim behind a centered message-and-buttons card -
// the modal prompt shape deletion's Shift+Delete confirmation originated and
// now shares with the export-format prompt. The buttons and everything about
// selection are the ChoicePanel underneath; the card adds the scrim, the
// message above them, and its own visibility.
//
// The card never gives that panel Fyne's keyboard focus, unlike a dialog
// would: this app dispatches every key from the canvas's unfocused handler
// (modified key combos already have to bypass widget focus - see the app's
// wireOpenShortcuts comment), so the app's dispatcher hands the card the keys
// through HandleKey instead while it is up.
type ChoiceCard struct {
	// panel owns the buttons, their focus rings, the selected index and the
	// key rules over them. The card keeps no copy of any of that, so a click
	// on a button - which reaches the panel directly, never this type - can't
	// leave the two disagreeing.
	panel *ChoicePanel

	// repaint is called after every visibility change - the app has no
	// automatic redraw loop, so a hidden window has to be told to paint again
	// itself (see viewer.ForceRepaint, which every caller so far passes in
	// directly). The panel gets the same hook for its selection changes.
	repaint func()

	visible bool
	overlay *fyne.Container
	message *widget.Label
}

// NewChoiceCard builds the card (hidden) with the given choices, left to
// right. Index 0 is the leftmost button and the default selection.
func NewChoiceCard(repaint func(), choices ...Choice) *ChoiceCard {
	c := &ChoiceCard{repaint: repaint}

	c.panel = NewChoicePanel(repaint, choices...)
	// The card is what a confirmed or cancelled prompt has to take off
	// screen, and hiding it is all there is to that - see the panel's
	// SetOnDismiss for the ordering this buys.
	c.panel.SetOnDismiss(c.Hide)

	scrim := canvas.NewRectangle(ScrimColor)

	c.message = widget.NewLabel("")
	c.message.Alignment = fyne.TextAlignCenter
	c.message.Wrapping = fyne.TextWrapWord

	cardBG := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	cardBG.CornerRadius = CardRadius
	card := container.NewStack(cardBG, container.NewPadded(container.NewVBox(c.message, c.panel)))

	c.overlay = container.NewStack(scrim, container.NewCenter(card))
	c.overlay.Hide()

	return c
}

// runChoice is the card's name for the panel's click path (see
// ChoicePanel.runChoice), so a caller holding a card can run choice i the way
// a click on its button does - card hidden first, keyboard selection ignored.
func (c *ChoiceCard) runChoice(i int) func() {
	return c.panel.runChoice(i)
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
// test seam, mirrored by Message and Ring below for consumers built directly
// on the card that need to assert on its rendered state.
func (c *ChoiceCard) Selected() int {
	return c.panel.Selected()
}

// Message is the card's headline label.
func (c *ChoiceCard) Message() *widget.Label {
	return c.message
}

// Ring is the selection ring drawn behind choice i, or nil for an
// out-of-range index.
func (c *ChoiceCard) Ring(i int) *canvas.Rectangle {
	return c.panel.Ring(i)
}

// SetOnCancel registers what Escape runs once the card is hidden, in
// addition to hiding it. Optional: a card whose index-0 choice already does
// nothing beyond dismissing has nothing more for Escape to do.
func (c *ChoiceCard) SetOnCancel(onCancel func()) {
	c.panel.SetOnCancel(onCancel)
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
// own guarded dismissal (deletion.Confirmer.Cancel), or the panel taking the
// card down before a chosen action runs. Always repaints, even when the card
// is already hidden: a caller that just hid it through some other path still
// wants the window redrawn.
func (c *ChoiceCard) Hide() {
	c.visible = false
	c.overlay.Hide()
	if c.repaint != nil {
		c.repaint()
	}
}

// Select moves the selection to index i, clamping to the choice range rather
// than wrapping - see ChoicePanel.Select, which owns that rule.
func (c *ChoiceCard) Select(i int) {
	c.panel.Select(i)
}

// Confirm runs whichever choice is currently selected - Return/Enter while
// the card is up, or deletion's own confirmSelection test seam calling it
// directly. The card hides before the choice's OnChosen runs, so an action
// that shows something else of its own doesn't have to hide this card first.
func (c *ChoiceCard) Confirm() {
	c.panel.Confirm()
}

// HandleKey handles a key press while the card is up: Left/Right move the
// selection (clamping at either end), Return/Enter runs whichever is
// selected, Escape hides the card and runs onCancel if one is registered.
// Every other key is deliberately left to the caller.
//
// The app's key dispatcher calls this rather than Fyne delivering it to the
// panel, because nothing on this card ever holds widget focus - see the type
// comment.
func (c *ChoiceCard) HandleKey(ev *fyne.KeyEvent) {
	c.panel.TypedKey(ev)
}
