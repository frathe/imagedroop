package favorites

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/ui/widgets"
)

// openCol and removeCol are the two columns of the panel's (row, col) ring
// position - Open first/left, the red Remove second/right, so the Right
// arrow key, which moves toward the higher index, points at where the
// destructive button actually sits. The same ordering rule deletion's
// cancelChoice/dangerChoice sets, and for the same reason: a panel never
// opens, or reopens after a removal, with Remove already under Return.
const (
	openCol = iota
	removeCol
	columnCount
)

// cancelChoice and removeChoice are the removal confirmation's two button
// indices, and they follow the same rule one level down: Cancel first/left
// and so the default selection, the red Remove second/right. Named the way
// deletion's cancelChoice/dangerChoice are, because it is the same rule.
const (
	cancelChoice = 0
	removeChoice = 1
)

// manageEntry is one favorite as the panel needs it: the label to show and
// what its two buttons do. The feature builds these; the panel knows
// nothing about favorites, names, or storage.
type manageEntry struct {
	label  string
	open   func()
	remove func()
}

// manageRow is one built row: its label, the buttons and the rings behind
// them by column, the actions Return runs, and the row container itself,
// which scrollIntoView needs to know where the row sits in the scrolled
// content.
type manageRow struct {
	box     fyne.CanvasObject
	label   *widget.Label
	buttons [columnCount]*widget.Button
	rings   [columnCount]*canvas.Rectangle
	actions [columnCount]func()
}

// managePanel is the Manage Favorites dialog's content: a scrolled list of
// favorites with a focus ring that moves over rows and over each row's two
// buttons. It is a fyne.Focusable - one of only two things this app ever
// focuses, the other being the widgets.ChoicePanel removeFavorite gives its
// confirmation - because Fyne resolves Canvas.Focus through the *top
// overlay's* focus manager, the dialog's, so a focusable content widget is
// what makes the dialog's keys reach this package rather than falling
// through to the app's own dispatcher (internal/ui/keys.go).
type managePanel struct {
	widget.BaseWidget

	// canvas is the window's, kept so the panel can take the keyboard back
	// itself after a click: Fyne buttons are focusable, so tapping one
	// takes canvas focus off the panel before its OnTapped even runs.
	canvas fyne.Canvas

	content fyne.CanvasObject

	// scroll is nil for the empty state, which has no list to scroll.
	scroll *container.Scroll

	rows []manageRow

	// row and col are where the ring sits, (-1, -1) when there are no rows
	// to mark at all.
	row, col int

	onEscape func()
}

// newManagePanel builds the panel (unshown) over entries, top to bottom.
// onEscape is what the Escape key runs - closing the dialog, which the
// panel itself knows nothing about.
func newManagePanel(c fyne.Canvas, entries []manageEntry, onEscape func()) *managePanel {
	p := &managePanel{canvas: c, onEscape: onEscape, row: -1, col: -1}
	p.ExtendBaseWidget(p)

	if len(entries) == 0 {
		p.content = widget.NewLabel(lang.L("No favorites yet"))
		return p
	}

	buttonLabels := [columnCount]string{lang.L("Open"), lang.L("Remove")}
	boxes := make([]fyne.CanvasObject, len(entries))
	p.rows = make([]manageRow, len(entries))
	for row, entry := range entries {
		p.rows[row].label = widget.NewLabel(entry.label)
		p.rows[row].actions = [columnCount]func(){entry.open, entry.remove}

		cells := make([]fyne.CanvasObject, columnCount)
		for col, label := range buttonLabels {
			btn := widget.NewButton(label, func() { p.run(row, col) })
			if col == removeCol {
				btn.Importance = widget.DangerImportance
			}
			ring := widgets.NewFocusRing(widgets.ButtonRingWidth, widgets.RingRadius)
			ring.Hide()

			p.rows[row].buttons[col] = btn
			p.rows[row].rings[col] = ring
			cells[col] = widgets.Ringed(ring, btn)
		}

		p.rows[row].box = container.NewBorder(nil, nil, nil,
			container.NewHBox(cells...), p.rows[row].label)
		boxes[row] = p.rows[row].box
	}

	p.scroll = container.NewVScroll(container.NewVBox(boxes...))
	p.scroll.SetMinSize(fyne.NewSize(420, 240))
	p.content = p.scroll
	p.moveTo(0, openCol)

	return p
}

func (p *managePanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

// takeFocus gives the panel the keyboard. Called once the dialog is up (a
// widget can only be focused while it is part of an overlay Fyne can walk),
// and again whenever something transient took the keyboard away.
func (p *managePanel) takeFocus() {
	p.canvas.Focus(p)
}

// FocusGained and FocusLost have nothing to do: the ring is drawn from the
// panel's own (row, col), not from Fyne's focus state, the same manual
// selection model widgets.ChoicePanel uses. Losing the keyboard here is
// always transient - only the removal confirmation takes it, and it hands
// it straight back - so dimming the ring for it would flicker rather than
// inform.
func (p *managePanel) FocusGained() {}

func (p *managePanel) FocusLost() {}

// TypedRune ignores every rune: this panel has no type-ahead, and a stray
// character must not disturb the ring.
func (p *managePanel) TypedRune(rune) {}

// TypedKey moves the ring (clamping at every edge rather than wrapping, the
// rule widgets.ChoicePanel.Select sets for its one axis), runs the ringed
// button, or closes the dialog. Every other key is deliberately ignored.
func (p *managePanel) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyUp:
		p.moveTo(p.row-1, p.col)
	case fyne.KeyDown:
		p.moveTo(p.row+1, p.col)
	case fyne.KeyLeft:
		p.moveTo(p.row, p.col-1)
	case fyne.KeyRight:
		p.moveTo(p.row, p.col+1)
	case fyne.KeyReturn, fyne.KeyEnter:
		p.run(p.row, p.col)
	case fyne.KeyEscape:
		if p.onEscape != nil {
			p.onEscape()
		}
	}
}

// moveTo puts the ring on (row, col), clamped into the list rather than
// wrapped around it, and scrolls that row back into view. A no-op with no
// rows: the empty state keeps its (-1, -1) "no ring at all".
func (p *managePanel) moveTo(row, col int) {
	if len(p.rows) == 0 {
		return
	}

	p.row = min(max(row, 0), len(p.rows)-1)
	p.col = min(max(col, openCol), removeCol)

	for r := range p.rows {
		for c, ring := range p.rows[r].rings {
			if r == p.row && c == p.col {
				ring.Show()
			} else {
				ring.Hide()
			}
		}
	}
	// Refreshing the panel rather than trusting each ring's own Show/Hide:
	// Fyne only registers an object with its canvas the first time it is
	// painted while visible, so a ring that has been hidden since the
	// dialog opened has no canvas to mark dirty and would silently fail to
	// appear (see viewer.ForceRepaint for the same trap).
	p.Refresh()

	p.scrollIntoView(p.row)
}

// run activates the button at (row, col) and leaves the ring on it. Both a
// click and Return come through here, so a click always runs the button it
// landed on wherever the ring happened to sit (widgets.ChoicePanel.runChoice's
// rule) and the ring follows, leaving Return to mean what was just clicked.
func (p *managePanel) run(row, col int) {
	if row < 0 || row >= len(p.rows) || col < openCol || col > removeCol {
		return
	}

	// Before the action, not after: a tap has already moved canvas focus
	// off the panel, and the action may put another overlay on top that
	// should keep the keyboard while it is up.
	p.takeFocus()
	p.moveTo(row, col)

	if fn := p.rows[row].actions[col]; fn != nil {
		fn()
	}
}

// scrollIntoView scrolls the row back inside the viewport when the ring has
// moved off either edge of it - a ring the user cannot see is worse than no
// ring. A no-op before the panel has been laid out, when there is no
// viewport to be outside of yet.
func (p *managePanel) scrollIntoView(row int) {
	if p.scroll == nil || row < 0 || row >= len(p.rows) {
		return
	}

	view := p.scroll.Size().Height
	if view <= 0 {
		return
	}

	box := p.rows[row].box
	top := box.Position().Y
	bottom := top + box.Size().Height
	switch {
	case top < p.scroll.Offset.Y:
		p.scroll.ScrollToOffset(fyne.NewPos(p.scroll.Offset.X, top))
	case bottom > p.scroll.Offset.Y+view:
		p.scroll.ScrollToOffset(fyne.NewPos(p.scroll.Offset.X, bottom-view))
	}
}

// ShowManage raises the Manage Favorites dialog over the favorites as they
// stand now (the Favorites menu item, also Cmd/Ctrl+Shift+F - see
// wireManageFavoritesShortcut in internal/ui/shortcuts.go). A no-op while one
// is already up, the guard deletion.RequestFiles and promptExport each make
// for themselves: a second dialog would stack over the first and take the
// keyboard from it.
func (f *Feature) ShowManage() {
	if f.manageDialog != nil {
		return
	}

	names, err := favstore.List(f.dir)
	if err != nil {
		f.reportError(lang.L("could not list favorites: %v"), err)
		return
	}

	entries := make([]manageEntry, len(names))
	for i, name := range names {
		favoriteName := name
		entries[i] = manageEntry{
			// The menu's own label, so the dialog and the menu can never
			// disagree about how many files a favorite holds.
			label: f.menuLabel(favoriteName),
			open: func() {
				f.hideManage()
				f.openFavorite(favoriteName)
			},
			remove: func() { f.removeFavorite(favoriteName) },
		}
	}

	panel := newManagePanel(f.win.Canvas(), entries, f.hideManage)
	d := dialog.NewCustom(lang.L("Manage Favorites"), lang.L("Close"), panel, f.win)
	d.SetOnClosed(func() {
		// Only for the dialog still on screen: a removal rebuilds by hiding
		// this one and showing the next, and this callback fires inside
		// that Hide. Without the check it would run against whatever
		// replaced it and leave the feature holding a dialog it believes is
		// closed.
		if f.manageDialog != d {
			return
		}

		f.manageDialog, f.managePanel = nil, nil
		// The release grid.Overview.Close performs, for the same reason:
		// every other key binding in this app is dispatched from the
		// canvas's own unfocused handler, so a focus left behind would
		// swallow key presses afterwards.
		f.win.Canvas().Unfocus()
	})

	f.manageDialog, f.managePanel = d, panel
	d.Show()
	// After Show: Fyne can only focus an object it can walk to, and the
	// panel is only part of an overlay once the dialog is up.
	panel.takeFocus()
}

// hideManage closes the dialog if it is up - Escape, or the Open button on
// its way to the image view.
func (f *Feature) hideManage() {
	if f.manageDialog != nil {
		f.manageDialog.Hide()
	}
}

// focusManage hands the keyboard back to whichever panel is current. Not
// the panel a caller captured earlier: a confirmed removal rebuilds the
// dialog, so by the time the confirmation closes the panel underneath may
// already be a different one.
func (f *Feature) focusManage() {
	if f.managePanel != nil {
		f.managePanel.takeFocus()
	}
}

// removeFavorite asks before trashing a favorite's folder. Not
// dialog.NewConfirm: that focuses nothing inside itself, and Fyne resolves
// Canvas.Focused through the *top overlay's* focus manager only, so its
// confirmation left Focused() nil - which is exactly the state in which the
// glfw driver routes keys to the canvas's unfocused handler, this app's own
// dispatcher (internal/ui/keys.go). Escape then reset the session behind the
// prompt, and Return answered nothing at all, since a focused Fyne button
// reacts to Space and never to Return.
//
// A widgets.ChoicePanel is a fyne.Focusable, so making it the dialog's
// content puts the keyboard where the user is looking, exactly as
// managePanel does for the dialog underneath. The dialog carries no dismiss
// button of its own (NewCustomWithoutButtons): Cancel is already one of the
// two choices, and a Close beside it would be a second way to say the same
// thing.
func (f *Feature) removeFavorite(name string) {
	// Unwrapped, unlike Fyne's own text dialogs: they wrap the message and
	// then widen the dialog to fit it from a beforeShowHook, which a custom
	// dialog has no equivalent of, so a wrapping label here would collapse to
	// its own minimum and stack the sentence four words to a line.
	message := &widget.Label{
		Text:      fmt.Sprintf(lang.L("Remove %q from favorites? Its folder will be moved to the Trash."), name),
		Alignment: fyne.TextAlignCenter,
	}

	var confirm dialog.Dialog
	panel := widgets.NewChoicePanel(nil,
		// Cancel first/left and so the default selection, the red Remove
		// second/right - see cancelChoice/removeChoice.
		widgets.Choice{Label: lang.L("Cancel")},
		widgets.Choice{
			Label:      lang.L("Remove"),
			Importance: widget.DangerImportance,
			OnChosen:   func() { f.performRemove(name) },
		},
	)
	// Cancel and Escape need nothing beyond this: the panel dismisses before
	// running any choice, and Cancel's choice runs nothing of its own.
	panel.SetOnDismiss(func() { confirm.Hide() })

	confirm = dialog.NewCustomWithoutButtons(lang.L("Remove Favorite"),
		container.NewVBox(message, panel), f.win)
	// The confirmation is a second overlay and owns the keyboard while it is
	// up; whichever way it goes, the panel underneath has to get it back.
	// Fyne happens to hand it back on its own, because removing the top
	// overlay drops only that overlay's focus manager and the dialog's below
	// it still has the panel focused - but a dialog left unable to answer
	// Escape is a dead end for the user, so this does not lean on it.
	confirm.SetOnClosed(f.focusManage)
	confirm.Show()
	// After Show, for the reason ShowManage focuses its own panel after Show:
	// Fyne can only focus an object that is already part of an overlay it can
	// walk to.
	f.win.Canvas().Focus(panel)
}

func (f *Feature) performRemove(name string) {
	f.pending.Add(1)
	go func() {
		err := favstore.Remove(f.dir, name)
		fyne.Do(func() {
			defer f.pending.Done()

			if err != nil {
				f.reportError(lang.L("could not remove favorite %q: %v"), name, err)
				return
			}

			f.refreshMenu()
			f.host.ShowToast(fmt.Sprintf(lang.L("removed favorite %q"), name))
			if f.managePanel != nil {
				f.rebuildManage()
			}
		})
	}()
}

// rebuildManage reopens the dialog on the list as it now stands, keeping
// the ring on the row index the removed favorite held - which is now
// whichever favorite moved up into its place, or the new last row when the
// removed one was last. Back on Open rather than the Remove the user just
// used, so the ring never lands on a second destructive button by itself.
func (f *Feature) rebuildManage() {
	row := f.managePanel.row

	f.hideManage()
	f.ShowManage()
	// Not unconditional: a rebuild whose favstore.List failed reported that
	// and left no dialog, and so nothing to put a ring on.
	if f.managePanel != nil {
		f.managePanel.moveTo(row, openCol)
	}
}
