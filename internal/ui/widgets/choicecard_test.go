package widgets

import (
	"testing"

	"fyne.io/fyne/v2"
)

// threeChoices gives clamping something real to hit at both ends - a
// two-choice card (deletion's own) can't tell "clamped" apart from "always
// jumps to the other end", since with two positions they're the same thing.
func threeChoices(chosen *[]int) []Choice {
	return []Choice{
		{Label: "A", OnChosen: func() { *chosen = append(*chosen, 0) }},
		{Label: "B", OnChosen: func() { *chosen = append(*chosen, 1) }},
		{Label: "C", OnChosen: func() { *chosen = append(*chosen, 2) }},
	}
}

func TestChoiceCard_SelectClampsAtTheLowEnd(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	c.Select(1)
	c.Select(-5)

	if got := c.Selected(); got != 0 {
		t.Errorf("Selected() = %d, want clamped to 0", got)
	}
	if !c.Ring(0).Visible() || c.Ring(1).Visible() || c.Ring(2).Visible() {
		t.Error("only index 0's ring should be visible after clamping low")
	}
}

func TestChoiceCard_SelectClampsAtTheHighEnd(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	c.Select(50)

	if got := c.Selected(); got != 2 {
		t.Errorf("Selected() = %d, want clamped to the last index (2)", got)
	}
	if c.Ring(0).Visible() || c.Ring(1).Visible() || !c.Ring(2).Visible() {
		t.Error("only the last index's ring should be visible after clamping high")
	}
}

func TestChoiceCard_HandleKeyLeftRightClampRatherThanWrap(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if got := c.Selected(); got != 0 {
		t.Errorf("Selected() = %d after Left at index 0, want 0 (no wrap)", got)
	}

	c.Select(2)
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if got := c.Selected(); got != 2 {
		t.Errorf("Selected() = %d after Right at the last index, want 2 (no wrap)", got)
	}
}

func TestChoiceCard_ShowResetsSelectionToIndexZero(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	c.Select(2)
	c.Show("pick one")

	if got := c.Selected(); got != 0 {
		t.Errorf("Selected() after Show = %d, want reset to 0", got)
	}
	if !c.Ring(0).Visible() || c.Ring(1).Visible() || c.Ring(2).Visible() {
		t.Error("only index 0's ring should be visible after Show resets the selection")
	}
	if got := c.Message().Text; got != "pick one" {
		t.Errorf("Message().Text = %q, want %q", got, "pick one")
	}
}

func TestChoiceCard_ReturnRunsTheSelectedChoice(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	c.Select(1)
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if want := []int{1}; len(chosen) != 1 || chosen[0] != want[0] {
		t.Errorf("chosen = %v, want %v (index 1's OnChosen)", chosen, want)
	}
	if c.Visible() {
		t.Error("the card should hide once a choice is confirmed")
	}
}

// TestChoiceCard_ReturnHidesBeforeRunningTheChoice guards the ordering
// performDelete's move to Trash depends on: whatever the chosen action does
// (deletion's own trash.Move, a future export dialog) must not find the
// card still reporting itself visible.
func TestChoiceCard_ReturnHidesBeforeRunningTheChoice(t *testing.T) {
	var sawVisible bool
	var c *ChoiceCard
	c = NewChoiceCard(nil, Choice{
		Label:    "go",
		OnChosen: func() { sawVisible = c.Visible() },
	})
	c.Show("go?")

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if sawVisible {
		t.Error("OnChosen ran while the card still reported itself visible")
	}
}

func TestChoiceCard_EnterAlsoConfirms(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEnter})

	if want := []int{0}; len(chosen) != 1 || chosen[0] != want[0] {
		t.Errorf("chosen = %v, want %v (the default index 0)", chosen, want)
	}
}

func TestChoiceCard_EscapeCancelsWithoutRunningAnyChoice(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	c.Select(2)
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if c.Visible() {
		t.Error("Escape should hide the card")
	}
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - Escape must not run any choice", chosen)
	}
}

func TestChoiceCard_EscapeRunsTheRegisteredOnCancel(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	cancelled := 0
	c.SetOnCancel(func() { cancelled++ })

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if cancelled != 1 {
		t.Errorf("onCancel ran %d times, want exactly 1", cancelled)
	}
}

func TestChoiceCard_EscapeWithoutOnCancelJustHides(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) // no SetOnCancel call - must not panic

	if c.Visible() {
		t.Error("Escape should still hide the card with no onCancel registered")
	}
}

func TestChoiceCard_VisibleTransitions(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	if c.Visible() {
		t.Error("a fresh card should start hidden")
	}

	c.Show("pick one")
	if !c.Visible() {
		t.Error("Visible() should be true after Show")
	}

	c.Hide()
	if c.Visible() {
		t.Error("Visible() should be false after Hide")
	}
}

func TestChoiceCard_RepaintFiresOnShowSelectAndHide(t *testing.T) {
	var chosen []int
	repaints := 0
	c := NewChoiceCard(func() { repaints++ }, threeChoices(&chosen)...)

	c.Show("pick one")
	if repaints == 0 {
		t.Error("Show should trigger at least one repaint")
	}

	before := repaints
	c.Select(1)
	if repaints <= before {
		t.Error("Select should trigger a repaint")
	}

	before = repaints
	c.Hide()
	if repaints <= before {
		t.Error("Hide should trigger a repaint")
	}
}

// TestChoiceCard_RepaintCallbackIsOptional: nil is what a caller with no
// repaint hook passes (this package's own tests do, above and below), and
// every mutating method has to tolerate it.
func TestChoiceCard_RepaintCallbackIsOptional(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	c.Show("pick one")
	c.Select(1)
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
}

func TestChoiceCard_ClickRunsThatButtonRegardlessOfSelection(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)
	c.Show("pick one")

	c.Select(2) // keyboard selection points at index 2

	c.runChoice(0)() // a click on button 0 always runs button 0's action

	if want := []int{0}; len(chosen) != 1 || chosen[0] != want[0] {
		t.Errorf("chosen = %v, want %v - a click ignores the keyboard selection", chosen, want)
	}
}

// TestChoiceCard_ConfirmingAChoiceWithNoActionJustHides covers the shape
// deletion actually ships: its Cancel choice carries no OnChosen at all,
// because hiding is everything Cancel has ever had to do. Every fixture
// above hands each choice a function, so without this the nil branch in
// runChoice would only ever be exercised from another package's suite - and
// a regression there is a nil-func panic on the safest button on the card.
func TestChoiceCard_ConfirmingAChoiceWithNoActionJustHides(t *testing.T) {
	c := NewChoiceCard(nil, Choice{Label: "cancel"})
	c.Show("cancel?")

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if c.Visible() {
		t.Error("confirming an action-less choice should still hide the card")
	}
}

// TestChoiceCard_NoChoicesIsInertRatherThanPanicking pins what a card built
// with no choices does, now that this is an exported widget any caller can
// reach: Select's clamp lands on 0 rather than on -1, and runChoice's range
// check keeps Return from indexing an empty slice. A choiceless card is
// still a caller's mistake - it just surfaces as a card that does nothing
// instead of a panic on the next key press.
func TestChoiceCard_NoChoicesIsInertRatherThanPanicking(t *testing.T) {
	c := NewChoiceCard(nil)

	c.Show("nothing to pick")
	if got := c.Selected(); got < 0 {
		t.Errorf("Selected() = %d, want a non-negative index", got)
	}

	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	c.HandleKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if c.Visible() {
		t.Error("Return should still hide the card with nothing to run")
	}
}

func TestChoiceCard_RingReturnsNilOutOfRange(t *testing.T) {
	var chosen []int
	c := NewChoiceCard(nil, threeChoices(&chosen)...)

	if c.Ring(-1) != nil {
		t.Error("Ring(-1) should be nil")
	}
	if c.Ring(3) != nil {
		t.Error("Ring(len(choices)) should be nil")
	}
}
