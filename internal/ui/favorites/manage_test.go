package favorites

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/uitest"
)

// panelFixture is a managePanel inside a shown dialog, the way ShowManage
// assembles one, but with entries the test writes itself - the ring
// mechanics below are about rows and columns, not about favorites on disk.
type panelFixture struct {
	panel   *managePanel
	win     fyne.Window
	escapes int
}

func newPanelFixture(t *testing.T, entries ...manageEntry) *panelFixture {
	t.Helper()

	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("manage panel test")
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(600, 480))

	fx := &panelFixture{win: win}
	fx.panel = newManagePanel(win.Canvas(), entries, func() { fx.escapes++ })
	// In a dialog, as ShowManage assembles it: the panel can only be
	// focused while it is part of an overlay Fyne can walk to.
	dialog.NewCustom("Manage Favorites", "Close", fx.panel, win).Show()
	fx.panel.takeFocus()

	return fx
}

// recordingEntries builds one entry per label whose buttons only report
// that they ran, so a test can tell exactly which cell was activated.
func recordingEntries(log *[]string, labels ...string) []manageEntry {
	entries := make([]manageEntry, len(labels))
	for i, label := range labels {
		entries[i] = manageEntry{
			label:  label,
			open:   func() { *log = append(*log, "open "+label) },
			remove: func() { *log = append(*log, "remove "+label) },
		}
	}

	return entries
}

// typeKey sends a key to whatever the canvas currently reports as focused,
// rather than to the panel directly: every keyboard test then also proves
// the panel is the thing holding the keyboard at that moment.
func typeKey(t *testing.T, win fyne.Window, name fyne.KeyName) {
	t.Helper()

	focused := win.Canvas().Focused()
	if focused == nil {
		t.Fatalf("no focused object to send %s to", name)
	}
	focused.TypedKey(&fyne.KeyEvent{Name: name})
}

// ringedCell reports where the panel actually draws its ring, asserting the
// invariant that exactly one is ever visible. (-1, -1) means no ring at all,
// which is what an empty panel must report.
func ringedCell(t *testing.T, p *managePanel) (int, int) {
	t.Helper()

	row, col := -1, -1
	for r := range p.rows {
		for c, ring := range p.rows[r].rings {
			if !ring.Visible() {
				continue
			}
			if row != -1 {
				t.Fatalf("two rings visible at once: (%d,%d) and (%d,%d)", row, col, r, c)
			}
			row, col = r, c
		}
	}

	return row, col
}

// assertRing checks the panel's ring position and what it renders together,
// so a state change that never reaches the screen still fails.
func assertRing(t *testing.T, p *managePanel, wantRow, wantCol int) {
	t.Helper()

	if p.row != wantRow || p.col != wantCol {
		t.Errorf("ring position = (%d,%d), want (%d,%d)", p.row, p.col, wantRow, wantCol)
	}
	if row, col := ringedCell(t, p); row != wantRow || col != wantCol {
		t.Errorf("ring drawn at (%d,%d), want (%d,%d)", row, col, wantRow, wantCol)
	}
}

func TestManagePanelStartsOnTheFirstRowsOpenButton(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha", "beta", "zebra")...)

	assertRing(t, fx.panel, 0, openCol)
	if fx.win.Canvas().Focused() != fx.panel {
		t.Errorf("focused = %v, want the panel to hold the keyboard once shown", fx.win.Canvas().Focused())
	}
}

func TestManagePanelArrowKeysMoveTheRingInBothAxes(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha", "beta", "zebra")...)

	typeKey(t, fx.win, fyne.KeyDown)
	assertRing(t, fx.panel, 1, openCol)

	typeKey(t, fx.win, fyne.KeyRight)
	assertRing(t, fx.panel, 1, removeCol)

	typeKey(t, fx.win, fyne.KeyDown)
	assertRing(t, fx.panel, 2, removeCol)

	typeKey(t, fx.win, fyne.KeyUp)
	assertRing(t, fx.panel, 1, removeCol)

	typeKey(t, fx.win, fyne.KeyLeft)
	assertRing(t, fx.panel, 1, openCol)
}

// TestManagePanelClampsAtEveryEdge pins the rule ChoicePanel.Select already
// sets for one axis and this panel extends to two: movement stops at the
// edge, it never wraps around to the far side.
func TestManagePanelClampsAtEveryEdge(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha", "beta")...)

	typeKey(t, fx.win, fyne.KeyUp)
	assertRing(t, fx.panel, 0, openCol)

	typeKey(t, fx.win, fyne.KeyLeft)
	assertRing(t, fx.panel, 0, openCol)

	typeKey(t, fx.win, fyne.KeyRight)
	typeKey(t, fx.win, fyne.KeyRight)
	assertRing(t, fx.panel, 0, removeCol)

	typeKey(t, fx.win, fyne.KeyDown)
	typeKey(t, fx.win, fyne.KeyDown)
	assertRing(t, fx.panel, 1, removeCol)
}

// TestManagePanelClampsOnASingleRow is the degenerate list: the only row is
// both the first and the last, so every vertical move has to leave the ring
// exactly where it is.
func TestManagePanelClampsOnASingleRow(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha")...)

	typeKey(t, fx.win, fyne.KeyDown)
	assertRing(t, fx.panel, 0, openCol)

	typeKey(t, fx.win, fyne.KeyUp)
	assertRing(t, fx.panel, 0, openCol)
}

// TestManagePanelMarksRemoveAsDestructive matches the delete card's own
// red button: the one row action that cannot be undone from here looks
// different from the one that can.
func TestManagePanelMarksRemoveAsDestructive(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha")...)

	if got := fx.panel.rows[0].buttons[removeCol].Importance; got != widget.DangerImportance {
		t.Errorf("Remove importance = %v, want widget.DangerImportance", got)
	}
	if got := fx.panel.rows[0].buttons[openCol].Importance; got != widget.MediumImportance {
		t.Errorf("Open importance = %v, want the plain default", got)
	}
}

func TestManagePanelReturnRunsTheRingedButton(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha", "beta")...)

	typeKey(t, fx.win, fyne.KeyReturn)
	typeKey(t, fx.win, fyne.KeyDown)
	typeKey(t, fx.win, fyne.KeyRight)
	typeKey(t, fx.win, fyne.KeyEnter)

	if want := []string{"open Alpha", "remove beta"}; !slices.Equal(log, want) {
		t.Errorf("actions run = %v, want %v", log, want)
	}
}

func TestManagePanelEscapeCloses(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha")...)

	typeKey(t, fx.win, fyne.KeyEscape)

	if fx.escapes != 1 {
		t.Errorf("escape ran %d times, want 1", fx.escapes)
	}
	if len(log) != 0 {
		t.Errorf("escape ran a row action: %v", log)
	}
}

// TestManagePanelEmptyStateStillTakesTheKeyboard covers the list with
// nothing in it: no rows to ring, but the panel is still what holds the
// keyboard, so Escape still closes the dialog.
func TestManagePanelEmptyStateStillTakesTheKeyboard(t *testing.T) {
	fx := newPanelFixture(t)

	assertRing(t, fx.panel, -1, -1)
	if len(fx.panel.rows) != 0 {
		t.Fatalf("empty panel built %d rows", len(fx.panel.rows))
	}

	typeKey(t, fx.win, fyne.KeyDown)
	typeKey(t, fx.win, fyne.KeyRight)
	typeKey(t, fx.win, fyne.KeyReturn)
	assertRing(t, fx.panel, -1, -1)

	typeKey(t, fx.win, fyne.KeyEscape)
	if fx.escapes != 1 {
		t.Errorf("escape ran %d times on the empty panel, want 1", fx.escapes)
	}
}

func TestManagePanelIgnoresTypedRunes(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha")...)

	fx.panel.TypedRune('j')
	fx.panel.TypedRune(' ')

	assertRing(t, fx.panel, 0, openCol)
	if len(log) != 0 {
		t.Errorf("typing ran a row action: %v", log)
	}
}

// TestManagePanelClickRunsItsOwnButtonAndMovesTheRing is ChoicePanel's rule
// for a second axis: a click activates the button it landed on wherever the
// ring happens to sit, and the ring follows so Return can't then mean
// something else. The keyboard has to come back too - Fyne buttons are
// focusable, so the tap takes canvas focus off the panel on its way in.
func TestManagePanelClickRunsItsOwnButtonAndMovesTheRing(t *testing.T) {
	var log []string
	fx := newPanelFixture(t, recordingEntries(&log, "Alpha", "beta", "zebra")...)

	test.Tap(fx.panel.rows[2].buttons[removeCol])

	if want := []string{"remove zebra"}; !slices.Equal(log, want) {
		t.Errorf("actions run = %v, want %v", log, want)
	}
	assertRing(t, fx.panel, 2, removeCol)
	if fx.win.Canvas().Focused() != fx.panel {
		t.Errorf("focused = %v, want the panel to keep the keyboard after a click", fx.win.Canvas().Focused())
	}

	typeKey(t, fx.win, fyne.KeyReturn)
	if want := []string{"remove zebra", "remove zebra"}; !slices.Equal(log, want) {
		t.Errorf("actions run = %v, want the ring to follow the click", log)
	}
}

// TestManagePanelScrollsTheRingBackIntoView guards the rule that a ring the
// user cannot see is worse than no ring: the viewport is deliberately far
// smaller than the list, so moving down has to scroll and moving back to the
// top has to scroll back.
func TestManagePanelScrollsTheRingBackIntoView(t *testing.T) {
	var log []string
	labels := make([]string, 12)
	for i := range labels {
		labels[i] = fmt.Sprintf("Favorite %02d", i)
	}
	fx := newPanelFixture(t, recordingEntries(&log, labels...)...)
	fx.panel.Resize(fyne.NewSize(420, 120))

	for range len(labels) - 1 {
		typeKey(t, fx.win, fyne.KeyDown)
	}

	last := fx.panel.rows[len(labels)-1].box
	offset := fx.panel.scroll.Offset.Y
	view := fx.panel.scroll.Size().Height
	if offset <= 0 {
		t.Fatalf("scroll offset = %v, want the last row scrolled into view", offset)
	}
	if top, bottom := last.Position().Y, last.Position().Y+last.Size().Height; top < offset || bottom > offset+view {
		t.Errorf("row spans %v..%v, outside the viewport %v..%v", top, bottom, offset, offset+view)
	}

	for range len(labels) - 1 {
		typeKey(t, fx.win, fyne.KeyUp)
	}
	if fx.panel.scroll.Offset.Y != 0 {
		t.Errorf("scroll offset = %v after returning to the first row, want 0", fx.panel.scroll.Offset.Y)
	}
}

func saveFavorites(t *testing.T, f *Feature, names ...string) {
	t.Helper()

	for _, name := range names {
		files := []fyne.URI{storage.NewFileURI(fmt.Sprintf("/photos/%s.jpg", name))}
		if err := favstore.Save(f.dir, name, files); err != nil {
			t.Fatal(err)
		}
	}
	f.SetDir(f.dir)
}

func TestShowManageBuildsEmptyAndPopulatedDialogs(t *testing.T) {
	f := newFeature(t, &fakeHost{})

	f.ShowManage()
	if f.manageDialog == nil {
		t.Fatal("showManage did not build an empty dialog")
	}
	f.manageDialog.Hide()

	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	f.ShowManage()
	if f.manageDialog == nil {
		t.Fatal("showManage did not build a populated dialog")
	}
	f.manageDialog.Hide()
}

// TestShowManageRowsCarryTheMenuLabel keeps the dialog and the menu on one
// source for a favorite's count: both go through menuLabel, so neither can
// claim a number the other doesn't.
func TestShowManageRowsCarryTheMenuLabel(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha", "beta")

	f.ShowManage()

	if len(f.managePanel.rows) != 2 {
		t.Fatalf("panel built %d rows, want 2", len(f.managePanel.rows))
	}
	for i, name := range f.names {
		if got, want := f.managePanel.rows[i].label.Text, f.menuLabel(name); got != want {
			t.Errorf("row %d label = %q, want %q", i, got, want)
		}
	}
}

// TestShowManageFocusesThePanelAndReleasesOnClose mirrors what
// grid.Overview.Close does on the way out: this app dispatches keys from the
// canvas's own unfocused handler everywhere else, so a focus left behind
// would swallow every key press afterwards.
func TestShowManageFocusesThePanelAndReleasesOnClose(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha")

	f.ShowManage()
	if f.win.Canvas().Focused() != f.managePanel {
		t.Fatalf("focused = %v, want the panel", f.win.Canvas().Focused())
	}

	typeKey(t, f.win, fyne.KeyEscape)

	if f.manageDialog != nil || f.managePanel != nil {
		t.Error("closing the dialog left it registered on the feature")
	}
	if got := f.win.Canvas().Focused(); got != nil {
		t.Errorf("focused = %v, want the canvas released", got)
	}
}

// TestShowManageTwiceDoesNotStackDialogs is the guard deletion.RequestFiles
// and promptExport each make for themselves: a second request while the
// first is still up is a no-op, not a second overlay over the first.
func TestShowManageTwiceDoesNotStackDialogs(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha")

	f.ShowManage()
	first, panel := f.manageDialog, f.managePanel
	f.ShowManage()

	if f.manageDialog != first || f.managePanel != panel {
		t.Error("a second showManage replaced the dialog that was already up")
	}
	if n := len(f.win.Canvas().Overlays().List()); n != 1 {
		t.Errorf("overlay count = %d, want the one dialog", n)
	}

	f.manageDialog.Hide()
	f.ShowManage()
	if f.manageDialog == nil {
		t.Error("showManage refused to reopen after the dialog was closed")
	}
}

// TestManageIgnoresACloseFromASupersededDialog covers the reentrancy the
// rebuild after a removal creates: hiding the old dialog fires its own
// SetOnClosed, and by then the feature may already hold its replacement. A
// stale close must not deregister the dialog that is actually on screen and
// leave it unable to answer anything.
func TestManageIgnoresACloseFromASupersededDialog(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha")

	f.ShowManage()
	superseded := f.manageDialog
	f.manageDialog.Hide()
	f.ShowManage()
	current, panel := f.manageDialog, f.managePanel

	superseded.Hide()

	if f.manageDialog != current || f.managePanel != panel {
		t.Fatal("a superseded dialog's close deregistered the live one")
	}
	if f.win.Canvas().Focused() != panel {
		t.Errorf("focused = %v, want the live panel to keep the keyboard", f.win.Canvas().Focused())
	}
}

func TestManageOpenHidesTheDialogAndOpensThatFavorite(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha", "beta", "zebra")

	f.ShowManage()
	typeKey(t, f.win, fyne.KeyDown)
	typeKey(t, f.win, fyne.KeyReturn)

	if len(host.opened) != 1 || host.opened[0].Path() != "/photos/beta.jpg" {
		t.Errorf("opened = %v, want beta's stored file", host.opened)
	}
	if want := []string{"sync", "open"}; !slices.Equal(host.calls, want) {
		t.Errorf("call order = %v, want %v: Open still goes through openFavorite", host.calls, want)
	}
	if f.manageDialog != nil {
		t.Error("Open left the dialog up")
	}
}

func TestManageRemoveRaisesTheConfirmation(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha")

	f.ShowManage()
	typeKey(t, f.win, fyne.KeyRight)
	typeKey(t, f.win, fyne.KeyReturn)

	overlays := f.win.Canvas().Overlays().List()
	if len(overlays) != 2 {
		t.Fatalf("overlay count = %d, want the confirmation over the dialog", len(overlays))
	}
	if !confirmVisible(f.win, "Alpha") {
		t.Error("the top overlay does not name the favorite it is about to remove")
	}
	if !favstore.Exists(f.dir, "Alpha") {
		t.Error("the favorite went before the confirmation was answered")
	}
}

// confirmVisible reports whether the topmost overlay is a confirmation
// naming want.
func confirmVisible(win fyne.Window, want string) bool {
	top := win.Canvas().Overlays().Top()
	if top == nil {
		return false
	}
	for _, obj := range test.LaidOutObjects(top) {
		if label, ok := obj.(*widget.Label); ok && strings.Contains(label.Text, want) {
			return true
		}
	}

	return false
}

// dismissConfirm taps the confirmation's dismiss button: the one this
// package did not mark destructive. Matching on importance rather than on
// the label, which is Fyne's own default and not something this app sets.
func dismissConfirm(t *testing.T, win fyne.Window) {
	t.Helper()

	for _, obj := range test.LaidOutObjects(win.Canvas().Overlays().Top()) {
		if btn, ok := obj.(*widget.Button); ok && btn.Importance != widget.DangerImportance {
			test.Tap(btn)
			return
		}
	}
	t.Fatal("no dismiss button in the top overlay")
}

// TestManageConfirmationHandsTheKeyboardBack covers the second overlay: it
// owns the keyboard while it is up, so the panel underneath has to get it
// back when the confirmation goes away without removing anything.
func TestManageConfirmationHandsTheKeyboardBack(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha")

	f.ShowManage()
	test.Tap(f.managePanel.rows[0].buttons[removeCol])
	dismissConfirm(t, f.win)

	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the panel back once the confirmation closed", f.win.Canvas().Focused())
	}
	if !favstore.Exists(f.dir, "Alpha") {
		t.Error("dismissing the confirmation removed the favorite anyway")
	}

	typeKey(t, f.win, fyne.KeyEscape)
	if f.manageDialog != nil {
		t.Error("the panel stopped answering Escape after the confirmation closed")
	}
}

// confirmPanel is the removal confirmation's own choice panel, taken from
// what the canvas reports as focused - which is the whole point of the
// shape: a dialog.NewConfirm focuses nothing inside itself, so Focused()
// came back nil and every key aimed at the confirmation fell through to the
// app's dispatcher (internal/ui/keys.go) instead.
func confirmPanel(t *testing.T, win fyne.Window) *widgets.ChoicePanel {
	t.Helper()

	panel, ok := win.Canvas().Focused().(*widgets.ChoicePanel)
	if !ok {
		t.Fatalf("focused = %v, want the confirmation's choice panel", win.Canvas().Focused())
	}

	return panel
}

// raiseConfirm opens the Manage Favorites dialog and drives its keyboard to
// the removal confirmation for the first row's favorite.
func raiseConfirm(t *testing.T, f *Feature) *widgets.ChoicePanel {
	t.Helper()

	f.ShowManage()
	typeKey(t, f.win, fyne.KeyRight)
	typeKey(t, f.win, fyne.KeyReturn)

	return confirmPanel(t, f.win)
}

// TestManageConfirmationTakesTheKeyboardAndStartsOnCancel is the bug this
// dialog shape exists for, pinned from both ends: the confirmation is what
// the canvas reports as focused, and the ring is on Cancel, never on the
// destructive answer.
func TestManageConfirmationTakesTheKeyboardAndStartsOnCancel(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha", "beta")

	panel := raiseConfirm(t, f)

	if got := panel.Selected(); got != cancelChoice {
		t.Errorf("selected = %d, want Cancel (%d): a prompt never opens with Remove under Return", got, cancelChoice)
	}
	if !panel.Ring(cancelChoice).Visible() || panel.Ring(confirmChoice).Visible() {
		t.Error("the ring is not drawn on Cancel")
	}
}

// TestManageConfirmationOffersExactlyTwoButtons keeps the dialog's own
// dismiss button out: with Cancel already in the content, a Close beside it
// would be a second way to say the same thing.
func TestManageConfirmationOffersExactlyTwoButtons(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha", "beta")

	raiseConfirm(t, f)

	var buttons []*widget.Button
	for _, obj := range test.LaidOutObjects(f.win.Canvas().Overlays().Top()) {
		if btn, ok := obj.(*widget.Button); ok {
			buttons = append(buttons, btn)
		}
	}
	if len(buttons) != 2 {
		t.Fatalf("confirmation has %d buttons, want just Cancel and Remove", len(buttons))
	}
	if got := buttons[cancelChoice].Importance; got != widget.MediumImportance {
		t.Errorf("Cancel importance = %v, want the plain default", got)
	}
	if got := buttons[confirmChoice].Importance; got != widget.DangerImportance {
		t.Errorf("Remove importance = %v, want widget.DangerImportance", got)
	}
}

// TestManageConfirmationReturnOnCancelClosesWithoutRemoving is the key path
// that did not work at all before: a focused Fyne button answers Space, never
// Return, so Return on dialog.NewConfirm's Cancel did nothing.
func TestManageConfirmationReturnOnCancelClosesWithoutRemoving(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha", "beta")
	panel := raiseConfirm(t, f)

	typeKey(t, f.win, fyne.KeyReturn)

	if f.win.Canvas().Focused() == panel {
		t.Fatal("Return on Cancel left the confirmation up")
	}
	if !favstore.Exists(f.dir, "Alpha") {
		t.Error("Return on Cancel removed the favorite anyway")
	}
	if n := len(f.win.Canvas().Overlays().List()); n != 1 {
		t.Errorf("overlay count = %d, want the manage dialog alone", n)
	}
	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the manage panel back", f.win.Canvas().Focused())
	}
}

// TestManageConfirmationEscapeClosesWithoutRemoving is the other half of the
// same bug: Escape used to reach the app's dispatcher and reset the session
// behind the confirmation instead of answering it.
func TestManageConfirmationEscapeClosesWithoutRemoving(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	saveFavorites(t, f, "Alpha", "beta")
	raiseConfirm(t, f)

	typeKey(t, f.win, fyne.KeyEscape)

	if !favstore.Exists(f.dir, "Alpha") {
		t.Error("Escape removed the favorite")
	}
	if n := len(f.win.Canvas().Overlays().List()); n != 1 {
		t.Errorf("overlay count = %d, want the manage dialog alone", n)
	}
	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the manage panel back", f.win.Canvas().Focused())
	}

	// And the panel underneath is answering again, not just holding focus.
	typeKey(t, f.win, fyne.KeyEscape)
	if f.manageDialog != nil {
		t.Error("the manage panel stopped answering Escape after the confirmation closed")
	}
}

// TestManageConfirmationRightThenReturnRemoves is the confirmed path: the
// ring has to be moved onto Remove deliberately before Return means it.
func TestManageConfirmationRightThenReturnRemoves(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha", "beta")
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })
	panel := raiseConfirm(t, f)

	typeKey(t, f.win, fyne.KeyRight)
	if got := panel.Selected(); got != confirmChoice {
		t.Fatalf("selected = %d, want Remove (%d)", got, confirmChoice)
	}
	typeKey(t, f.win, fyne.KeyReturn)
	f.pending.Wait()

	if favstore.Exists(f.dir, "Alpha") {
		t.Error("the confirmed removal left the favorite in place")
	}
	if n := len(f.win.Canvas().Overlays().List()); n != 1 {
		t.Errorf("overlay count = %d, want the rebuilt manage dialog alone", n)
	}
	if f.managePanel == nil || len(f.managePanel.rows) != 1 {
		t.Fatalf("manage panel = %v, want it rebuilt around the one remaining favorite", f.managePanel)
	}
	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the rebuilt manage panel", f.win.Canvas().Focused())
	}
}

// TestPerformRemoveKeepsTheRingOnTheSameRow rebuilds the dialog around the
// shorter list: the ring stays on the row index the removed favorite held,
// which is now the favorite that moved up into its place.
func TestPerformRemoveKeepsTheRingOnTheSameRow(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha", "beta", "zebra")
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })

	f.ShowManage()
	f.managePanel.moveTo(1, removeCol)

	f.performRemove("beta")
	f.pending.Wait()

	if len(f.managePanel.rows) != 2 {
		t.Fatalf("rebuilt panel has %d rows, want 2", len(f.managePanel.rows))
	}
	if got := f.managePanel.rows[1].label.Text; !strings.HasPrefix(got, "zebra") {
		t.Errorf("row 1 label = %q, want zebra to have moved up into it", got)
	}
	// Back on Open, never on the destructive button the user just used.
	assertRing(t, f.managePanel, 1, openCol)
	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the rebuilt panel", f.win.Canvas().Focused())
	}
}

func TestPerformRemoveClampsTheRingWhenTheLastRowGoes(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha", "beta", "zebra")
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })

	f.ShowManage()
	f.managePanel.moveTo(2, removeCol)

	f.performRemove("zebra")
	f.pending.Wait()

	if len(f.managePanel.rows) != 2 {
		t.Fatalf("rebuilt panel has %d rows, want 2", len(f.managePanel.rows))
	}
	assertRing(t, f.managePanel, 1, openCol)
}

func TestPerformRemoveLeavesNoRingOnTheEmptyList(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha")
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })

	f.ShowManage()
	f.performRemove("Alpha")
	f.pending.Wait()

	if len(f.managePanel.rows) != 0 {
		t.Fatalf("rebuilt panel has %d rows, want the empty state", len(f.managePanel.rows))
	}
	assertRing(t, f.managePanel, -1, -1)

	typeKey(t, f.win, fyne.KeyEscape)
	if f.manageDialog != nil {
		t.Error("the empty panel stopped answering Escape")
	}
}

func TestPerformRemoveTrashesDirectoryAndRefreshesMenu(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	f.SetDir(f.dir)
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })

	f.performRemove("Trip")
	f.pending.Wait()

	if favstore.Exists(f.dir, "Trip") {
		t.Error("favorite still exists after removal")
	}
	if len(f.menu.Items) != 3 {
		t.Errorf("menu item count = %d, want static 3 after removal", len(f.menu.Items))
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Trip") {
		t.Errorf("toasts = %v", host.toasts)
	}
}

func TestPerformRemoveReportsTrashError(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("trash unavailable")
	uitest.StubTrashMove(t, func(string) error { return wantErr })

	f.performRemove("Trip")
	f.pending.Wait()

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("favorite disappeared after failed removal")
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], wantErr.Error()) {
		t.Errorf("toasts = %v", host.toasts)
	}
}

// TestPerformRemoveFailureLeavesTheDialogAsItWas checks the error path
// doesn't rebuild: the list on screen still matches the list on disk, and
// the panel still holds the keyboard.
func TestPerformRemoveFailureLeavesTheDialogAsItWas(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	saveFavorites(t, f, "Alpha", "beta")
	uitest.StubTrashMove(t, func(string) error { return errors.New("trash unavailable") })

	f.ShowManage()
	panel := f.managePanel
	f.managePanel.moveTo(1, removeCol)

	f.performRemove("beta")
	f.pending.Wait()

	if f.managePanel != panel {
		t.Error("a failed removal rebuilt the dialog")
	}
	assertRing(t, f.managePanel, 1, removeCol)
	if f.win.Canvas().Focused() != f.managePanel {
		t.Errorf("focused = %v, want the panel", f.win.Canvas().Focused())
	}
}
