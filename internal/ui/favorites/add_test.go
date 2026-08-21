package favorites

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/favstore"
)

// newNameEntryFixture builds a nameEntry inside a shown window and gives it
// the keyboard, the way newAddDialog does once the entry sits inside a real
// dialog's content. typeKey (manage_test.go) then sends keys
// through the canvas's focused object rather than straight to the entry, so
// every keyboard test here also proves the entry is what actually holds the
// keyboard, not just that its TypedKey works when called directly.
func newNameEntryFixture(t *testing.T, onEscape, onDown func()) (*nameEntry, fyne.Window) {
	t.Helper()

	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("name entry test")
	t.Cleanup(win.Close)

	entry := newNameEntry(onEscape, onDown)
	// Focus can only land on an object the canvas can walk to from its
	// content (internal/app.FocusManager.Focus returns false, silently,
	// for anything outside that tree) - the same reason managePanel's own
	// fixture focuses only after the dialog holding it is shown.
	win.SetContent(entry)
	win.Canvas().Focus(entry)

	return entry, win
}

func TestNameEntryEscapeRunsOnEscapeAndLeavesTextUntouched(t *testing.T) {
	var escaped int
	entry, win := newNameEntryFixture(t, func() { escaped++ }, nil)
	entry.SetText("Trip")

	typeKey(t, win, fyne.KeyEscape)

	if escaped != 1 {
		t.Errorf("onEscape ran %d times, want 1", escaped)
	}
	if entry.Text != "Trip" {
		t.Errorf("text = %q, want unchanged %q", entry.Text, "Trip")
	}
}

func TestNameEntryDownRunsOnDown(t *testing.T) {
	var moved int
	_, win := newNameEntryFixture(t, nil, func() { moved++ })

	typeKey(t, win, fyne.KeyDown)

	if moved != 1 {
		t.Errorf("onDown ran %d times, want 1", moved)
	}
}

// TestNameEntryEscapeAndDownAreInertWithNilHooks pins that a nil onEscape/
// onDown is a legitimate state for the bare widget - newAddDialog always
// wires both, but this fixture builds the field on its own, with nowhere yet
// for the Down key to hand the keyboard to - both keys must still be
// consumed rather than fall through to Entry.TypedKey (Escape) or reach some
// other stray meaning, and the field must not panic on either.
func TestNameEntryEscapeAndDownAreInertWithNilHooks(t *testing.T) {
	entry, win := newNameEntryFixture(t, nil, nil)
	entry.SetText("Trip")

	typeKey(t, win, fyne.KeyEscape)
	typeKey(t, win, fyne.KeyDown)

	if entry.Text != "Trip" {
		t.Errorf("text = %q, want unchanged %q", entry.Text, "Trip")
	}
}

// TestNameEntryOtherKeysReachTheEmbeddedEntry proves every key nameEntry
// does not name in its own TypedKey still falls through to Entry.TypedKey.
// Typing "abc" (rather than SetText, which leaves the cursor wherever it
// already was - 0, for a fresh entry) is what actually puts the cursor at
// the end, the same way a user would arrive there, so the Backspace below
// removes the last character rather than being a no-op at the front.
func TestNameEntryOtherKeysReachTheEmbeddedEntry(t *testing.T) {
	entry, win := newNameEntryFixture(t, nil, nil)
	test.Type(entry, "abc")

	typeKey(t, win, fyne.KeyBackspace)

	if entry.Text != "ab" {
		t.Errorf("text = %q, want %q", entry.Text, "ab")
	}
}

func TestNameEntryTypedRunesLandInText(t *testing.T) {
	entry, _ := newNameEntryFixture(t, nil, nil)

	test.Type(entry, "Trip")

	if entry.Text != "Trip" {
		t.Errorf("text = %q, want %q", entry.Text, "Trip")
	}
}

// TestNameEntryReturnFiresOnSubmittedWithCurrentText pins that Return is
// deliberately left to Entry.TypedKey rather than intercepted here: a
// single-line widget.Entry already calls OnSubmitted(text) for it
// (typedKeyReturn in Fyne's widget/entry.go), which is where newAddDialog
// hangs direct submission from the field.
func TestNameEntryReturnFiresOnSubmittedWithCurrentText(t *testing.T) {
	entry, win := newNameEntryFixture(t, nil, nil)
	test.Type(entry, "Trip")

	var submitted string
	entry.OnSubmitted = func(text string) { submitted = text }

	typeKey(t, win, fyne.KeyReturn)

	if submitted != "Trip" {
		t.Errorf("OnSubmitted got %q, want %q", submitted, "Trip")
	}
}

// TestNameEntryAClickFocusesTheOuterEntry guards newNameEntry's
// ExtendBaseWidget call along the one path the tests above cannot reach.
// They all focus the entry themselves, so the override would keep firing for
// them even if that call went away; a mouse click into the field is what
// would silently stop working.
//
// The click path is MouseDown, not Tapped - widget.Entry.Tapped only tidies
// up a mobile selection - and MouseDown focuses through requestFocus, which
// focuses e.super(): the impl pointer ExtendBaseWidget sets.
// Entry.CreateRenderer sets that pointer too, to the *embedded* *Entry, and
// ExtendBaseWidget keeps the first one written (it returns early when impl
// is already set). newNameEntry therefore wins that race only by running
// first: drop its call and the renderer claims impl instead, a click lands
// the keyboard on the embedded Entry rather than on *nameEntry, and Escape
// and Down go back to doing nothing - the exact dead field this type exists
// to fix, reached by the most ordinary thing a user does to a name field.
func TestNameEntryAClickFocusesTheOuterEntry(t *testing.T) {
	entry, win := newNameEntryFixture(t, nil, nil)

	escapes := 0
	entry.onEscape = func() { escapes++ }

	// Unfocused first, so it is the click that puts the keyboard back rather
	// than the fixture's own Focus still standing.
	win.Canvas().Unfocus()
	entry.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})

	if focused := win.Canvas().Focused(); focused != fyne.Focusable(entry) {
		t.Fatalf("focused = %T, want *nameEntry - a click focused the embedded Entry, so TypedKey is bypassed", focused)
	}

	typeKey(t, win, fyne.KeyEscape)
	if escapes != 1 {
		t.Errorf("onEscape ran %d times after a click-then-Escape, want 1", escapes)
	}
}

// --- Stage 4: the Add to Favorites dialog itself (showAdd/newAddDialog) ---

func TestShowAddFocusesTheNameField(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("")

	if got := f.win.Canvas().Focused(); got != fyne.Focusable(f.addPanel.entry) {
		t.Errorf("focused = %T, want the name field", got)
	}
}

func TestShowAddDownMovesToChoicesUpMovesBackToTheField(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("")

	typeKey(t, f.win, fyne.KeyDown)
	if got := f.win.Canvas().Focused(); got != fyne.Focusable(f.addPanel.choices) {
		t.Fatalf("focused after Down = %T, want the choice panel", got)
	}

	typeKey(t, f.win, fyne.KeyUp)
	if got := f.win.Canvas().Focused(); got != fyne.Focusable(f.addPanel.entry) {
		t.Errorf("focused after Up = %T, want the name field again", got)
	}
}

// TestShowAddChoiceEnabledTracksValidationLive pins the wiring's real
// mechanism, not the one the plan first reached for. widget.Entry's own
// setValidationError (Fyne v2.8.0, widget/entry_validation.go) suppresses
// onValidationChanged for any transition *into* an error state while the
// entry is still focused - and the field stays focused for its entire
// useful lifetime, since Down (not a blur) is the only way to leave it while
// the dialog stays open. Confirmed with a standalone probe against
// widget.Entry before writing this: SetOnValidationChanged alone never
// re-fires going from a valid name back to an invalid one while the field
// keeps focus - only FocusLost forces a fresh validate() - so wiring it
// straight to SetOnValidationChanged would leave Add stuck enabled after the
// user typed past a name and back into nonsense, and Return would sail
// through it into saveFavorite's toast-only rejection instead of being
// stopped at the ring. entry.Validate()'s return value is not subject to
// that suppression (only the internal validationError/inline-message/
// callback triple is), so newAddDialog instead hooks entry.OnChanged and
// reads entry.Validate() itself on every keystroke - this test is what
// would fail first if that wiring ever regressed back to
// SetOnValidationChanged.
func TestShowAddChoiceEnabledTracksValidationLive(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("")

	if f.addPanel.choices.ChoiceEnabled(confirmChoice) {
		t.Error("Add starts enabled on an empty field, want disabled")
	}

	test.Type(f.addPanel.entry, "Trip")
	if !f.addPanel.choices.ChoiceEnabled(confirmChoice) {
		t.Error("Add stayed disabled after a valid name, want enabled")
	}

	f.addPanel.entry.SetText("a/b")
	if f.addPanel.choices.ChoiceEnabled(confirmChoice) {
		t.Error("Add stayed enabled after an invalid name typed while the field kept focus, want disabled")
	}
}

func TestShowAddReturnWithAValidNameSavesAndClosesTheDialog(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")

	typeKey(t, f.win, fyne.KeyReturn)

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("Return with a valid name did not save the favorite")
	}
	if f.addDialog != nil {
		t.Error("dialog still open after Return saved a valid name")
	}
}

// TestShowAddReturnWithAnInvalidNameSavesNothingAndLeavesTheRingOnCancel
// pins entry.OnSubmitted's own guard: the ChoiceEnabled check runs before
// anything else, so an invalid Return changes nothing at all, not even the
// ring - it must never reach choices.Select, which would have moved the
// ring onto Add regardless of what running it then did.
func TestShowAddReturnWithAnInvalidNameSavesNothingAndLeavesTheRingOnCancel(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	f.addPanel.entry.SetText("a/b")

	typeKey(t, f.win, fyne.KeyReturn)

	if favstore.Exists(f.dir, "a/b") || favstore.Exists(f.dir, "a") {
		t.Error("Return with an invalid name saved something")
	}
	if f.addDialog == nil {
		t.Fatal("dialog closed after Return on an invalid name, want it to stay up")
	}
	if got := f.addPanel.choices.Selected(); got != cancelChoice {
		t.Errorf("selected = %d, want Cancel (%d): an invalid Return must not move the ring", got, cancelChoice)
	}
	if len(host.toasts) != 0 {
		t.Errorf("toasts = %v, want none: saveFavorite must never even run", host.toasts)
	}
}

func TestShowAddDownRightReturnSaves(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")

	typeKey(t, f.win, fyne.KeyDown)
	typeKey(t, f.win, fyne.KeyRight)
	typeKey(t, f.win, fyne.KeyReturn)

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("Down, Right, Return on Add did not save")
	}
	if f.addDialog != nil {
		t.Error("dialog still open after Down, Right, Return saved")
	}
}

func TestShowAddDownReturnOnCancelClosesWithoutSaving(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")

	typeKey(t, f.win, fyne.KeyDown)
	typeKey(t, f.win, fyne.KeyReturn)

	if favstore.Exists(f.dir, "Trip") {
		t.Error("Down, Return on Cancel saved the typed name")
	}
	if f.addDialog != nil {
		t.Error("dialog still open after Cancel")
	}
}

func TestShowAddEscapeClosesWithoutSavingFromTheField(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")

	typeKey(t, f.win, fyne.KeyEscape)

	if favstore.Exists(f.dir, "Trip") {
		t.Error("Escape from the field saved the typed name")
	}
	if f.addDialog != nil {
		t.Error("dialog still open after Escape from the field")
	}
}

func TestShowAddEscapeClosesWithoutSavingFromTheChoices(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")
	typeKey(t, f.win, fyne.KeyDown)

	typeKey(t, f.win, fyne.KeyEscape)

	if favstore.Exists(f.dir, "Trip") {
		t.Error("Escape from the choices saved the typed name")
	}
	if f.addDialog != nil {
		t.Error("dialog still open after Escape from the choices")
	}
}

func TestShowAddTrimsTheName(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	f.showAdd("")
	f.addPanel.entry.SetText("  Trip  ")

	typeKey(t, f.win, fyne.KeyReturn)

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("Return did not save the trimmed name")
	}
	if favstore.Exists(f.dir, "  Trip  ") {
		t.Error("the untrimmed name was saved verbatim")
	}
}

// TestShowAddASecondCallWhileOneIsUpDoesNotStackAnotherOverlay mirrors
// ShowManage's own guard test: the Favorites menu stays clickable while a
// Fyne dialog is up (a canvas overlay, not an OS-modal window), so a second
// click has to be a no-op rather than stacking a second dialog over the
// first and stranding its keyboard.
func TestShowAddASecondCallWhileOneIsUpDoesNotStackAnotherOverlay(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("")
	first := f.addDialog

	f.showAdd("Other")

	if f.addDialog != first {
		t.Error("a second showAdd replaced the dialog while one was already up")
	}
	if got := len(f.win.Canvas().Overlays().List()); got != 1 {
		t.Errorf("overlay count = %d, want 1", got)
	}
	if f.addPanel.entry.Text != "" {
		t.Errorf("entry text = %q, want the first dialog's untouched text", f.addPanel.entry.Text)
	}
}

func TestShowAddClosingReleasesTheKeyboard(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("")

	typeKey(t, f.win, fyne.KeyEscape)

	if got := f.win.Canvas().Focused(); got != nil {
		t.Errorf("focused = %v, want nil after the dialog closed", got)
	}
}

// TestShowAddOpensWithInitialNameAndAddAlreadyEnabled is the seeding case
// Stage 5 depends on: reopening the Add dialog from the Replace prompt's
// Cancel hands showAdd a name that is already known-valid (it was just
// rejected only because it already exists, not because it is malformed), so
// Add must not make the user retype it to re-enable the button.
func TestShowAddOpensWithInitialNameAndAddAlreadyEnabled(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	f.showAdd("Holiday")

	if f.addPanel.entry.Text != "Holiday" {
		t.Errorf("entry text = %q, want %q", f.addPanel.entry.Text, "Holiday")
	}
	if !f.addPanel.choices.ChoiceEnabled(confirmChoice) {
		t.Error("Add starts disabled on a valid prefilled name, want enabled")
	}
}

// addGuardDuringSave wraps fakeHost to capture f.addDialog at the moment
// SyncFavoritePreviews runs - deep inside Add's OnChosen (f.saveFavorite ->
// writeFavorite), which runs entirely synchronously with no goroutine of its
// own. It exists to pin the ordering ChoicePanel.runChoice guarantees and
// Stage 5 needs: the panel dismisses the dialog (which clears f.addDialog,
// through this dialog's own SetOnClosed) before it ever runs a choice's
// OnChosen, so by the time saveFavorite's own side effects run, the guard
// against stacking a second Add dialog is already clear rather than still
// pointing at the one on its way down.
type addGuardDuringSave struct {
	*fakeHost
	f      *Feature
	guard  dialog.Dialog
	called bool
}

func (h *addGuardDuringSave) SyncFavoritePreviews(dir string, files []fyne.URI) {
	h.called = true
	h.guard = h.f.addDialog
	h.fakeHost.SyncFavoritePreviews(dir, files)
}

func TestShowAddDismissesBeforeOnChosenRuns(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("favorites test")
	t.Cleanup(win.Close)

	host := &addGuardDuringSave{fakeHost: &fakeHost{
		files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")},
	}}
	f := New(host, win)
	f.SetDir(t.TempDir())
	host.f = f

	f.showAdd("")
	test.Type(f.addPanel.entry, "Trip")
	typeKey(t, f.win, fyne.KeyReturn)

	if !host.called {
		t.Fatal("SyncFavoritePreviews never ran - the save did not happen")
	}
	if host.guard != nil {
		t.Errorf("f.addDialog = %v while OnChosen was still running, want nil", host.guard)
	}
}
