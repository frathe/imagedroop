package widgets

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/test"
)

// focusedPanel is a panel inside a shown dialog, the way a caller that wants
// the keyboard assembles one: a widget can only be focused while it is part
// of an overlay Fyne can walk to.
func focusedPanel(t *testing.T, choices ...Choice) (*ChoicePanel, fyne.Window) {
	t.Helper()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(400, 300))

	p := NewChoicePanel(nil, choices...)
	dialog.NewCustom("pick one", "Close", p, win).Show()
	win.Canvas().Focus(p)

	return p, win
}

// typeKey sends a key to whatever the canvas currently reports as focused,
// rather than to the panel directly: every keyboard test then also proves the
// panel is the thing holding the keyboard at that moment.
func typeKey(t *testing.T, win fyne.Window, name fyne.KeyName) {
	t.Helper()

	focused := win.Canvas().Focused()
	if focused == nil {
		t.Fatalf("no focused object to send %s to", name)
	}
	focused.TypedKey(&fyne.KeyEvent{Name: name})
}

// ringedIndex reports where the panel actually draws its ring, asserting the
// invariant that exactly one is ever visible. -1 means no ring at all, which
// is what a choiceless panel must report.
func ringedIndex(t *testing.T, p *ChoicePanel) int {
	t.Helper()

	found := -1
	for i, ring := range p.rings {
		if !ring.Visible() {
			continue
		}
		if found >= 0 {
			t.Fatalf("rings %d and %d are both visible; exactly one may be", found, i)
		}
		found = i
	}

	return found
}

func assertSelection(t *testing.T, p *ChoicePanel, want int) {
	t.Helper()

	if got := p.Selected(); got != want {
		t.Errorf("Selected() = %d, want %d", got, want)
	}
	if got := ringedIndex(t, p); got != want {
		t.Errorf("ring drawn at %d, want %d", got, want)
	}
}

func TestChoicePanel_SelectClampsAtTheLowEnd(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(1)
	p.Select(-1)
	assertSelection(t, p, 0)

	p.Select(-50)
	assertSelection(t, p, 0)
}

func TestChoicePanel_SelectClampsAtTheHighEnd(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(2) // the last real index, not yet clamped
	assertSelection(t, p, 2)

	p.Select(3)
	assertSelection(t, p, 2)

	p.Select(50)
	assertSelection(t, p, 2)
}

func TestChoicePanel_StartsOnIndexZero(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	assertSelection(t, p, 0)
}

func TestChoicePanel_ArrowKeysClampRatherThanWrap(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	assertSelection(t, p, 0)

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	assertSelection(t, p, 2)

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	assertSelection(t, p, 2)
}

func TestChoicePanel_ReturnRunsTheSelectedChoice(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(1)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if want := []int{1}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v (index 1's OnChosen)", chosen, want)
	}
}

func TestChoicePanel_EnterAlsoConfirms(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})

	if want := []int{0}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v (the default index 0)", chosen, want)
	}
}

// TestChoicePanel_ConfirmingAChoiceWithNoActionIsSafe covers the shape
// deletion ships: its Cancel choice carries no OnChosen at all, because
// dismissing is everything Cancel has ever had to do.
func TestChoicePanel_ConfirmingAChoiceWithNoActionIsSafe(t *testing.T) {
	dismissed := 0
	p := NewChoicePanel(nil, Choice{Label: "cancel"})
	p.SetOnDismiss(func() { dismissed++ })

	p.Confirm()

	if dismissed != 1 {
		t.Errorf("onDismiss ran %d times, want exactly 1 - an action-less choice still dismisses", dismissed)
	}
}

// TestChoicePanel_DismissRunsBeforeTheChosenAction is the ordering
// ChoiceCard's callers depend on (deletion's move to Trash, the export
// prompt's file dialog): whatever the action raises must not find the prompt
// that started it still on screen.
func TestChoicePanel_DismissRunsBeforeTheChosenAction(t *testing.T) {
	var order []string
	p := NewChoicePanel(nil, Choice{
		Label:    "go",
		OnChosen: func() { order = append(order, "chosen") },
	})
	p.SetOnDismiss(func() { order = append(order, "dismiss") })

	p.Confirm()

	if want := []string{"dismiss", "chosen"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestChoicePanel_EscapeDismissesAndCancelsWithoutRunningAnyChoice(t *testing.T) {
	var chosen []int
	var order []string
	p := NewChoicePanel(nil, threeChoices(&chosen)...)
	p.SetOnDismiss(func() { order = append(order, "dismiss") })
	p.SetOnCancel(func() { order = append(order, "cancel") })

	p.Select(2)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if want := []string{"dismiss", "cancel"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - Escape must not run any choice", chosen)
	}
}

// TestChoicePanel_EscapeWithoutHooksIsInert: both hooks are optional, and a
// panel whose container needs neither (ChoiceCard registers only the
// dismissal) must not panic on Escape.
func TestChoicePanel_EscapeWithoutHooksIsInert(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none", chosen)
	}
}

// TestChoicePanel_ClickRunsItsOwnChoiceRegardlessOfSelection taps the real
// button, so the wiring from Choice through the button's OnTapped is what is
// under test, not runChoice on its own. The selection stays where the
// keyboard left it: the click names its own index.
func TestChoicePanel_ClickRunsItsOwnChoiceRegardlessOfSelection(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(2)
	test.Tap(p.buttons[0])

	if want := []int{0}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v - a click ignores the keyboard selection", chosen, want)
	}
	assertSelection(t, p, 2)
}

// TestChoicePanel_NoChoicesIsInertRatherThanPanicking pins what a panel built
// with no choices does: Select's clamp lands on 0 rather than on -1, and
// runChoice's range check keeps Return from indexing an empty slice. A
// choiceless panel is a caller's mistake - it just surfaces as a panel that
// does nothing instead of a panic on the next key press. It still dismisses,
// so even that mistake can be got rid of.
func TestChoicePanel_NoChoicesIsInertRatherThanPanicking(t *testing.T) {
	dismissed := 0
	p := NewChoicePanel(nil)
	p.SetOnDismiss(func() { dismissed++ })

	p.Select(-1)
	p.Select(7)
	if got := p.Selected(); got != 0 {
		t.Errorf("Selected() = %d, want 0 on a panel with nothing to select", got)
	}
	if got := ringedIndex(t, p); got != -1 {
		t.Errorf("ring drawn at %d, want no ring at all", got)
	}

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if dismissed != 1 {
		t.Errorf("onDismiss ran %d times, want exactly 1 - Return still dismisses with nothing to run", dismissed)
	}
}

func TestChoicePanel_RingReturnsNilOutOfRange(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	if p.Ring(-1) != nil {
		t.Error("Ring(-1) should be nil")
	}
	if p.Ring(3) != nil {
		t.Error("Ring(len(choices)) should be nil")
	}
	if p.Ring(0) == nil {
		t.Error("Ring(0) should be the first choice's ring")
	}
}

func TestChoicePanel_RepaintFiresOnEverySelectionChange(t *testing.T) {
	var chosen []int
	repaints := 0
	p := NewChoicePanel(func() { repaints++ }, threeChoices(&chosen)...)

	p.Select(1)
	if repaints != 1 {
		t.Errorf("repaints = %d after Select, want 1", repaints)
	}

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if repaints != 2 {
		t.Errorf("repaints = %d after Right, want 2", repaints)
	}

	// Clamped at the end, so nothing moves - but the caller asked for a
	// selection change and the ring state was rewritten, so it still repaints
	// rather than the panel second-guessing which changes are worth drawing.
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if repaints != 3 {
		t.Errorf("repaints = %d after a clamped Right, want 3", repaints)
	}
}

// TestChoicePanel_TypedRuneLeavesTheSelectionAlone: the panel has no
// type-ahead, so a stray character (a caller's own shortcut letter reaching
// the focused panel) must not move the ring or run anything.
func TestChoicePanel_TypedRuneLeavesTheSelectionAlone(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(1)
	p.TypedRune('c')
	p.TypedRune('\n')

	assertSelection(t, p, 1)
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - runes run nothing", chosen)
	}
}

// TestChoicePanel_FocusedInADialogReceivesKeys is the whole reason the panel
// is a widget: Fyne resolves Canvas.Focused through the top overlay's focus
// manager, so keys only reach a dialog's content if that content is
// focusable. Every key here goes through the canvas rather than straight to
// the panel, so the focus wiring is what is under test.
func TestChoicePanel_FocusedInADialogReceivesKeys(t *testing.T) {
	var chosen []int
	p, win := focusedPanel(t, threeChoices(&chosen)...)

	if got := win.Canvas().Focused(); got != p {
		t.Fatalf("focused = %v, want the panel to hold the keyboard inside the dialog", got)
	}

	typeKey(t, win, fyne.KeyRight)
	assertSelection(t, p, 1)

	typeKey(t, win, fyne.KeyReturn)
	if want := []int{1}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v (the ringed choice, through the canvas)", chosen, want)
	}
}

func TestChoicePanel_FocusedInADialogAnswersEscape(t *testing.T) {
	var chosen []int
	p, win := focusedPanel(t, threeChoices(&chosen)...)

	cancelled := 0
	p.SetOnCancel(func() { cancelled++ })

	typeKey(t, win, fyne.KeyEscape)

	if cancelled != 1 {
		t.Errorf("onCancel ran %d times, want exactly 1", cancelled)
	}
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none", chosen)
	}
}

// TestChoicePanel_FocusChangesLeaveTheRingWhereItIs: the ring is drawn from
// the panel's own selected index, not from Fyne's focus state, so a panel
// that loses the keyboard to something transient on top keeps showing where
// the selection stands.
func TestChoicePanel_FocusChangesLeaveTheRingWhereItIs(t *testing.T) {
	var chosen []int
	p, win := focusedPanel(t, threeChoices(&chosen)...)

	typeKey(t, win, fyne.KeyRight)
	assertSelection(t, p, 1)

	win.Canvas().Unfocus()
	assertSelection(t, p, 1)

	win.Canvas().Focus(p)
	assertSelection(t, p, 1)

	typeKey(t, win, fyne.KeyReturn)
	if want := []int{1}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v - the selection survived the focus round trip", chosen, want)
	}
}

// TestChoicePanel_UpWithNoOnBackIsInert: onBack is optional, and every panel
// inside a ChoiceCard (deletion, the export prompt) never sets it, since the
// app dispatcher owns Up out there. Up must not stand in for anything else
// - in particular it must not move the selection - when nothing is
// registered.
func TestChoicePanel_UpWithNoOnBackIsInert(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.Select(1)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})

	assertSelection(t, p, 1)
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - Up must not run anything", chosen)
	}
}

// TestChoicePanel_UpRunsOnBackWithoutMovingSelection is the Add dialog's
// shape one stage over: Up leaves the field's stop and comes back to it,
// which only works if the ring is exactly where it was found.
func TestChoicePanel_UpRunsOnBackWithoutMovingSelection(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	backs := 0
	p.SetOnBack(func() { backs++ })

	p.Select(2)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})

	if backs != 1 {
		t.Errorf("onBack ran %d times, want exactly 1", backs)
	}
	assertSelection(t, p, 2)
	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - Up must not run a choice", chosen)
	}

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if backs != 2 {
		t.Errorf("onBack ran %d times after a second Up, want 2", backs)
	}
}

// TestChoicePanel_SetChoiceEnabledTogglesTheButtonAndTheReport pins the
// single source of truth: enabled state lives on the Fyne button itself
// (Enable/Disable/Disabled), not in a parallel bool the panel could get out
// of sync with the greyed rendering.
func TestChoicePanel_SetChoiceEnabledTogglesTheButtonAndTheReport(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	if !p.ChoiceEnabled(1) {
		t.Fatal("ChoiceEnabled(1) = false, want every choice to start enabled")
	}

	p.SetChoiceEnabled(1, false)
	if p.ChoiceEnabled(1) {
		t.Error("ChoiceEnabled(1) = true after SetChoiceEnabled(1, false)")
	}
	if !p.buttons[1].Disabled() {
		t.Error("buttons[1].Disabled() = false, want the button itself disabled")
	}

	p.SetChoiceEnabled(1, true)
	if !p.ChoiceEnabled(1) {
		t.Error("ChoiceEnabled(1) = false after re-enabling")
	}
	if p.buttons[1].Disabled() {
		t.Error("buttons[1].Disabled() = true, want the button re-enabled")
	}
}

// TestChoicePanel_ConfirmOnADisabledChoiceRunsNothing is what makes a
// disabled choice under the ring mean something: Return/Enter (Confirm)
// must not run OnChosen, and must not dismiss the prompt either - a
// disabled choice is a dead end, not a shortcut around the guard a click
// already gets from widget.Button.Tapped.
func TestChoicePanel_ConfirmOnADisabledChoiceRunsNothing(t *testing.T) {
	var chosen []int
	dismissed := 0
	p := NewChoicePanel(nil, threeChoices(&chosen)...)
	p.SetOnDismiss(func() { dismissed++ })

	p.Select(1)
	p.SetChoiceEnabled(1, false)

	p.Confirm()

	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - a disabled choice must not run", chosen)
	}
	if dismissed != 0 {
		t.Errorf("onDismiss ran %d times, want 0 - a disabled choice must not dismiss either", dismissed)
	}
}

// TestChoicePanel_TapOnADisabledChoiceRunsNothing proves the keyboard guard
// in TestChoicePanel_ConfirmOnADisabledChoiceRunsNothing isn't needed to
// cover the click path too - widget.Button.Tapped already refuses a
// disabled button - but pins it anyway, since it's the same behaviour a
// caller relies on regardless of which path reaches it.
func TestChoicePanel_TapOnADisabledChoiceRunsNothing(t *testing.T) {
	var chosen []int
	dismissed := 0
	p := NewChoicePanel(nil, threeChoices(&chosen)...)
	p.SetOnDismiss(func() { dismissed++ })

	p.SetChoiceEnabled(0, false)
	test.Tap(p.buttons[0])

	if len(chosen) != 0 {
		t.Errorf("chosen = %v, want none - a disabled choice must not run", chosen)
	}
	if dismissed != 0 {
		t.Errorf("onDismiss ran %d times, want 0", dismissed)
	}
}

// TestChoicePanel_ChoiceEnabledOutOfRangeIsFalse: an index that names no
// button can't be "enabled" - false is the only answer that doesn't invent
// state for a button that doesn't exist.
func TestChoicePanel_ChoiceEnabledOutOfRangeIsFalse(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	if p.ChoiceEnabled(-1) {
		t.Error("ChoiceEnabled(-1) = true, want false")
	}
	if p.ChoiceEnabled(3) {
		t.Error("ChoiceEnabled(len(choices)) = true, want false")
	}
}

// TestChoicePanel_SetChoiceEnabledOutOfRangeIsANoOp: nothing to disable at
// an out-of-range index, and nothing should panic trying.
func TestChoicePanel_SetChoiceEnabledOutOfRangeIsANoOp(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)

	p.SetChoiceEnabled(-1, false)
	p.SetChoiceEnabled(3, false)

	for i, btn := range p.buttons {
		if btn.Disabled() {
			t.Errorf("buttons[%d].Disabled() = true after an out-of-range SetChoiceEnabled, want untouched", i)
		}
	}
}

// TestChoicePanel_SelectStillLandsOnADisabledChoice pins the deliberate
// choice *not* to skip a disabled button: a greyed button under the ring is
// the app telling the user why Return just did nothing, the same way
// Fyne's own dialog.FormDialog leaves a disabled Submit focusable rather
// than jumping focus away from it. Select must therefore still move the
// ring onto choice 1 even though it is disabled - a later "fix" that skips
// disabled choices would take that explanation away.
func TestChoicePanel_SelectStillLandsOnADisabledChoice(t *testing.T) {
	var chosen []int
	p := NewChoicePanel(nil, threeChoices(&chosen)...)
	p.SetChoiceEnabled(1, false)

	p.Select(1)

	assertSelection(t, p, 1)
}

// TestChoicePanel_RegressionOldStyleCallerStillConfirmsAndDismisses is the
// safety net for every existing ChoiceCard caller (deletion, the export
// prompt): a panel built exactly the old way, never touching SetOnBack or
// SetChoiceEnabled, must keep working byte-for-byte - Up stays ignored and
// every choice starts, and stays, enabled.
func TestChoicePanel_RegressionOldStyleCallerStillConfirmsAndDismisses(t *testing.T) {
	var chosen []int
	dismissed := 0
	p := NewChoicePanel(nil, threeChoices(&chosen)...)
	p.SetOnDismiss(func() { dismissed++ })

	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	assertSelection(t, p, 0)
	if len(chosen) != 0 || dismissed != 0 {
		t.Fatalf("Up with no onBack ran something: chosen=%v dismissed=%d", chosen, dismissed)
	}

	p.Select(2)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})

	if want := []int{2}; !slices.Equal(chosen, want) {
		t.Errorf("chosen = %v, want %v", chosen, want)
	}
	if dismissed != 1 {
		t.Errorf("onDismiss ran %d times, want exactly 1", dismissed)
	}
}
