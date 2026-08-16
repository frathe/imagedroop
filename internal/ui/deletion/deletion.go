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

	// RemoveFiles drops every named index from the app's file set, in one
	// call. One call rather than one per file on purpose: removing them
	// one at a time would shift every later index out from under the list
	// already captured here.
	RemoveFiles(indices []int)

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

// Target is one file a pending confirmation would move to the Trash: the
// URI to move, and the index to drop from the app's file set afterwards.
// Both are captured when the card opens - see performDelete on why the index
// can't simply be looked up again once the move returns.
type Target struct {
	URI   fyne.URI
	Index int
}

// Confirmer is the confirmation card and the state behind it.
type Confirmer struct {
	host Host

	// targets is what confirming would move to the Trash: one file for the
	// Shift+Delete on the image being viewed, or the grid's whole selection
	// for a batch. Captured by Request/RequestFiles rather than read back
	// when the danger button is pressed, so a card that is already up asks
	// about exactly the files it named.
	targets []Target

	// visible is true while the card is up. The app's key dispatcher
	// checks it before its own handling, so every other key is swallowed
	// rather than acted on while a decision about moving a file to the
	// Trash is pending.
	visible bool

	// dangerSelected tracks which button Left/Right has selected: false
	// (Cancel, the default) or true (the red "Move to Trash"
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
// (selected by default) and the red "Move to Trash" button
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

// Request opens the confirmation card for the file currently on screen - the
// plain Shift+Delete on the image being viewed. A no-op with nothing loaded.
func (c *Confirmer) Request() {
	u, i, ok := c.host.CurrentFile()
	if !ok {
		return
	}

	c.RequestFiles([]Target{{URI: u, Index: i}})
}

// RequestFiles opens the confirmation card for a whole set of files - the
// grid's selection, via the app's batch glue. A no-op with no targets, or if
// the card is already up: re-triggering the shortcut mid-prompt shouldn't
// reset the selection out from under a user who's already moved it onto the
// danger button and is reaching for Return.
//
// A single target is worded exactly as the single-file prompt always was,
// naming the file; anything more names the count instead, since a card
// listing forty file names would be unreadable and unbounded in height.
func (c *Confirmer) RequestFiles(targets []Target) {
	if len(targets) == 0 || c.visible {
		return
	}

	c.targets = targets
	if len(targets) == 1 {
		c.message.SetText(fmt.Sprintf(lang.L("Move %q to the Trash?"), targets[0].URI.Name()))
	} else {
		c.message.SetText(fmt.Sprintf(lang.L("Move %d files to the Trash?"), len(targets)))
	}

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
// (false) and the red "Move to Trash" button (true), redrawing
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
// The targets and the generation are captured up front, before the goroutine
// starts: something else (a fresh drop, a reset, another navigation) can
// change the app's file set while trash.Move is in flight, which would
// make the captured indices stale by the time it returns. The generation
// check below catches that - if it no longer matches, the moves to Trash
// still happened (nothing is lost), but Host.RemoveFiles/ShowImage aren't
// called against indices that may no longer mean what they did; the
// now-missing entries, if they're even still in the set, fail to decode the
// ordinary way the next time they're navigated to, the same fallback a
// duplicate merge-mode path already relies on.
//
// The moves run one after another on that single goroutine rather than in
// parallel: trash.Move's darwin implementation already blocks on a
// completion handler per call, and a selection is tens of files, not
// thousands. Failures are collected instead of aborting the batch - one file
// the OS refuses to move shouldn't cost the user the rest of it - and only
// what actually moved is removed from the file set, so anything left behind
// on disk is also still in the app.
func (c *Confirmer) performDelete() {
	c.visible = false
	c.overlay.Hide()

	targets := c.targets
	if len(targets) == 0 {
		return
	}
	gen := c.host.Generation()

	c.pending.Go(func() {

		moved := make([]int, 0, len(targets))
		var firstErr error
		var firstFailed string

		for _, t := range targets {
			if err := trash.Move(t.URI.Path()); err != nil {
				if firstErr == nil {
					firstErr, firstFailed = err, t.URI.Name()
				}

				continue
			}
			moved = append(moved, t.Index)
		}

		fyne.Do(func() {
			if len(moved) == 0 {
				c.host.ShowToast(fmt.Sprintf(lang.L("could not move %q to the Trash: %v"), firstFailed, firstErr))
				return
			}

			if c.host.Generation() != gen {
				return
			}

			c.host.RemoveFiles(moved)

			msg := c.movedMessage(targets, moved, firstFailed, firstErr)

			if _, i, stillLoaded := c.host.CurrentFile(); stillLoaded {
				c.host.ShowToast(msg)
				c.host.ShowImage(i)
			} else {
				c.host.ShowEmptyStateError(msg)
			}
		})
	})
}

// movedMessage is what the toast (or the empty-state notice) says once the
// moves are done: the single-file wording when one file was asked for, a
// count when more were, and a count of both when some of them failed - a
// batch that silently reported success for files still sitting on disk would
// be the worst of the three.
func (c *Confirmer) movedMessage(targets []Target, moved []int, failedName string, failedErr error) string {
	switch {
	case len(moved) < len(targets):
		return fmt.Sprintf(lang.L("moved %d of %d files to the Trash; %q failed: %v"),
			len(moved), len(targets), failedName, failedErr)
	case len(targets) == 1:
		return fmt.Sprintf(lang.L("moved %q to the Trash"), targets[0].URI.Name())
	default:
		return fmt.Sprintf(lang.L("moved %d files to the Trash"), len(moved))
	}
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
// request is what a real Shift+Delete runs. A callback rather than this
// package calling Request itself, because what the key should confirm
// depends on state this package deliberately knows nothing about - the grid
// overview's selection, when that is up. Telling the two apart is the app's
// job (see internal/ui's requestDelete); recognising the key is this one's.
func ShortcutHandler(request func()) func(fyne.Shortcut) {
	return func(shortcut fyne.Shortcut) {
		if cut, ok := shortcut.(*fyne.ShortcutCut); ok && cut.Secondary {
			request()
		}
	}
}
