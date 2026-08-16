// Package deletion is the Shift+Delete confirmation flow: a dimmed scrim
// behind a centered card asking whether to move the file on screen to the
// Trash, and the trash.Move that follows if the user says yes.
//
// It owns its own widgets and its own selection state, and reaches back
// into the app only through Host - the first of the per-feature interfaces
// this app's package split is built on. Host is declared here, by the
// consumer, and lists exactly what this feature needs and nothing else;
// the viewer satisfies it incidentally, without knowing this package
// exists.
package deletion

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/imagedrop/internal/trash"
	"github.com/frathe/imagedrop/internal/ui/widgets"
)

// Host is what the confirmation flow needs from the application: which file
// is on screen, and the handful of display actions that follow a delete.
type Host interface {
	// CurrentFile returns the displayed file and its index, ok=false when
	// nothing is loaded.
	CurrentFile() (u fyne.URI, index int, ok bool)

	// RemoveFile drops the file at index i from the app's file set.
	RemoveFile(i int)

	// ShowImage displays the file at index i, wrapping at both ends.
	ShowImage(i int)

	// ShowEmptyStateError clears to the empty drop zone with msg - used
	// when the deleted file was the last one.
	ShowEmptyStateError(msg string)

	// ShowToast raises a short, non-blocking notification.
	ShowToast(msg string)

	// ForceRepaint redraws the window after a visibility change.
	ForceRepaint()

	// Generation is the app's current load generation - see performDelete:
	// it's how a trash.Move that finishes after the file set has moved on
	// (a fresh drop, a reset, another navigation) notices and skips acting
	// on a now-stale index.
	Generation() uint64
}

// Confirmer is the confirmation card and the state behind it.
type Confirmer struct {
	host Host

	// visible is true while the card is up. The app's key dispatcher
	// checks it before its own handling, so every other key is swallowed
	// rather than acted on while a decision about moving a file to the
	// Trash is pending.
	visible bool

	// dangerSelected tracks which button Left/Right has selected: false
	// (Cancel, the default) or true (the red "permanently delete"
	// button) - reset to false every time Request opens the card, never
	// carried over from a previous prompt.
	dangerSelected bool

	// pending tracks the background goroutine performDelete starts for its
	// trash.Move call - see performDelete's doc comment for why that call
	// can't run on the UI goroutine. Settle waits on it, the same way
	// slideshow.Controller's own pending WaitGroup lets tests wait out its
	// background goroutine before asserting on state it's still touching.
	pending sync.WaitGroup

	overlay    *fyne.Container
	message    *widget.Label
	cancelRing *canvas.Rectangle
	dangerRing *canvas.Rectangle
}

// New builds the confirmation card (hidden) around host.
//
// The card is a dimmed scrim behind a centered box, with Cancel first/left
// (selected by default) and the red "permanently delete" button
// second/right, so the Right arrow key - which moves selection there, see
// HandleKey - points toward where it actually sits. Left/Right/Return/
// Escape are handled by HandleKey while it's up, not Fyne's own
// widget-focus system (this app never uses that - see the app's
// wireOpenShortcuts comment on why modified combos already have to bypass
// it), so the selection rings are drawn manually to match, rather than
// relying on Button's own FocusGained highlight.
func New(host Host) *Confirmer {
	c := &Confirmer{host: host}

	scrim := canvas.NewRectangle(widgets.ScrimColor)

	c.message = widget.NewLabel("")
	c.message.Alignment = fyne.TextAlignCenter
	c.message.Wrapping = fyne.TextWrapWord

	cancelBtn := widget.NewButton(lang.L("Cancel"), c.Cancel)
	dangerBtn := widget.NewButton(lang.L("Move to Trash"), c.performDelete)
	dangerBtn.Importance = widget.DangerImportance

	c.cancelRing = widgets.NewFocusRing(widgets.ButtonRingWidth, widgets.RingRadius)

	c.dangerRing = widgets.NewFocusRing(widgets.ButtonRingWidth, widgets.RingRadius)
	c.dangerRing.Hide()

	buttons := container.NewGridWithColumns(2,
		ringed(c.cancelRing, cancelBtn),
		ringed(c.dangerRing, dangerBtn),
	)

	cardBG := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	cardBG.CornerRadius = widgets.CardRadius
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

// Overlay is the card, for the app to place in its window stack.
func (c *Confirmer) Overlay() fyne.CanvasObject {
	return c.overlay
}

// Visible reports whether the card is up - the app's key dispatcher checks
// this before its own handling.
func (c *Confirmer) Visible() bool {
	return c.visible
}

// Request opens the confirmation card for the file currently on screen. A
// no-op with nothing loaded, or if the card is already up: re-triggering
// the shortcut mid-prompt shouldn't reset the selection out from under a
// user who's already moved it onto the danger button and is reaching for
// Return.
func (c *Confirmer) Request() {
	u, _, ok := c.host.CurrentFile()
	if !ok || c.visible {
		return
	}

	c.message.SetText(fmt.Sprintf(lang.L("Move %q to the Trash?"), u.Name()))

	c.dangerSelected = false
	c.updateSelectionVisual()

	c.visible = true
	c.overlay.Show()
	c.host.ForceRepaint()
}

// Cancel dismisses the card without touching any file - Escape while it's
// up, clicking Cancel, or a fresh drop arriving mid-prompt. A no-op when
// the card isn't showing, so callers that call it defensively (the app, on
// every drop) don't need to check Visible themselves first.
func (c *Confirmer) Cancel() {
	if !c.visible {
		return
	}

	c.visible = false
	c.overlay.Hide()
	c.host.ForceRepaint()
}

// HandleKey handles a key press while the card is up: Left/Right move the
// selection, Return runs whichever action is selected, Escape cancels.
// Every other key is deliberately swallowed by the caller.
func (c *Confirmer) HandleKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyLeft:
		c.setSelection(false)
	case fyne.KeyRight:
		c.setSelection(true)
	case fyne.KeyReturn, fyne.KeyEnter:
		c.confirmSelection()
	case fyne.KeyEscape:
		c.Cancel()
	}
}

// setSelection moves the card's Left/Right selection between Cancel
// (false) and the red "permanently delete" button (true), redrawing
// whichever ring highlights the currently selected one.
func (c *Confirmer) setSelection(dangerSelected bool) {
	c.dangerSelected = dangerSelected
	c.updateSelectionVisual()
}

// updateSelectionVisual shows the focus ring around whichever button
// setSelection last selected and hides it on the other one.
func (c *Confirmer) updateSelectionVisual() {
	if c.dangerSelected {
		c.cancelRing.Hide()
		c.dangerRing.Show()
	} else {
		c.cancelRing.Show()
		c.dangerRing.Hide()
	}
	c.host.ForceRepaint()
}

// confirmSelection is Return/Enter while the card is up: it runs whichever
// action Left/Right last selected, so Return always agrees with what's
// visibly highlighted rather than always meaning one specific thing.
func (c *Confirmer) confirmSelection() {
	if c.dangerSelected {
		c.performDelete()
	} else {
		c.Cancel()
	}
}

// performDelete is the danger button's action (or Return with it
// selected): it moves the current file to the OS trash/recycle bin via
// trash.Move rather than removing it outright, so Shift+Delete is
// recoverable the same way a delete from Finder/Explorer/a Linux file
// manager already is.
//
// It runs on its own goroutine, mirroring openFileDialog/copyImageToClipboard
// - and here that's not just consistency with them, it's load-bearing:
// trash.Move's darwin implementation waits on NSWorkspace's completion
// handler via a semaphore, and that handler is delivered back through
// Cocoa's own machinery, which needs this app's UI goroutine free to keep
// running for the delivery to ever happen. Call trash.Move synchronously
// from the UI goroutine - as os.Remove safely was - and that wait
// deadlocks the whole app on every confirmed delete on macOS; a background
// goroutine (confirmed against a real NSWorkspace call, not just reasoned
// about) keeps the UI goroutine free the whole time.
//
// The index and generation are captured up front, before the goroutine
// starts: something else (a fresh drop, a reset, another navigation) can
// change the app's file set while trash.Move is in flight, which would
// make the captured index stale by the time it returns. The generation
// check below catches that - if it no longer matches, the move to Trash
// still happened (nothing is lost), but Host.RemoveFile/ShowImage aren't
// called against an index that may no longer mean what it did; the
// now-missing entry, if it's even still in the set, fails to decode the
// ordinary way the next time it's navigated to, the same fallback a
// duplicate merge-mode path already relies on.
func (c *Confirmer) performDelete() {
	c.visible = false
	c.overlay.Hide()

	target, i, ok := c.host.CurrentFile()
	if !ok {
		return
	}
	name := target.Name()
	path := target.Path()
	gen := c.host.Generation()

	c.pending.Add(1)
	go func() {
		defer c.pending.Done()

		err := trash.Move(path)

		fyne.Do(func() {
			if err != nil {
				c.host.ShowToast(fmt.Sprintf(lang.L("could not move %q to the Trash: %v"), name, err))
				return
			}

			if c.host.Generation() != gen {
				return
			}

			c.host.RemoveFile(i)

			if _, _, stillLoaded := c.host.CurrentFile(); !stillLoaded {
				c.host.ShowEmptyStateError(fmt.Sprintf(lang.L("moved %q to the Trash"), name))
				return
			}

			c.host.ShowToast(fmt.Sprintf(lang.L("moved %q to the Trash"), name))
			c.host.ShowImage(i)
		})
	}()
}

// Settle waits for any in-flight trash-move goroutine performDelete started
// to finish. Mirrors slideshow.Controller's Settle: the app's test suite
// uses this so a test doesn't end - or assert on Host state - while that
// goroutine's fyne.Do callback is still about to run.
func (c *Confirmer) Settle() {
	c.pending.Wait()
}

// ShortcutHandler is registered against &fyne.ShortcutCut{} rather than a
// desktop.CustomShortcut - see the app's wireDeleteShortcut for why a
// CustomShortcut for Shift+Delete can never actually fire. shortcut is
// whatever ShortcutName() == "Cut" event the driver produced; only the
// Secondary one (Shift+Delete) is ours, so a real Ctrl/Cmd+X - which this
// app has no cut action for - is correctly ignored rather than opening a
// delete prompt.
//
// blocked reports whether something else is currently claiming the screen
// (the grid overview): Shift+Delete is a global shortcut, not gated by the
// app's own key dispatch, so without that check it would open a
// confirmation card hidden behind the grid and capture the keyboard out
// from under it.
func ShortcutHandler(c *Confirmer, blocked func() bool) func(fyne.Shortcut) {
	return func(shortcut fyne.Shortcut) {
		if cut, ok := shortcut.(*fyne.ShortcutCut); ok && cut.Secondary && !blocked() {
			c.Request()
		}
	}
}
