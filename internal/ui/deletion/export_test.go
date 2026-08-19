// Test-only seams onto the card behind Confirmer.
//
// All three were production methods before the card moved out into
// widgets.ChoiceCard, and all three lost their last production caller in the
// same move: HandleKey now hands Left/Right/Return straight to the card.
// They live here rather than in deletion.go so that file carries only what
// the app itself runs, while this package's tests keep driving the flow in
// the terms it is written in ("select the danger button, then confirm")
// instead of restating the card's indices at every call site.

package deletion

// dangerSelected reports whether the red "Move to Trash" button is the one
// Return would run. Derived from the card on every call rather than tracked
// alongside it: a click on either button reaches the card directly, without
// passing through this package at all, so any copy kept here would go stale
// exactly then.
func (c *Confirmer) dangerSelected() bool {
	return c.card.Selected() == dangerChoice
}

// setSelection moves the card's Left/Right selection between Cancel (false)
// and the red "Move to Trash" button (true).
func (c *Confirmer) setSelection(dangerSelected bool) {
	if dangerSelected {
		c.card.Select(dangerChoice)
		return
	}

	c.card.Select(cancelChoice)
}

// confirmSelection is what Return/Enter reaches through HandleKey: it runs
// whichever action Left/Right last selected, so Return always agrees with
// what's visibly highlighted rather than always meaning one specific thing.
func (c *Confirmer) confirmSelection() {
	c.card.Confirm()
}
