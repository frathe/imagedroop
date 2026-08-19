// Package deletion is the Shift+Delete confirmation flow: a dimmed scrim
// behind a centered card asking whether to move the file on screen to the
// Trash, and the trash.Move that follows if the user says yes.
//
// It owns which files a pending confirmation is about, and reaches back
// into the app only through Host - the first of the per-feature interfaces
// this app's package split is built on. Host is declared here, by the
// consumer, and lists exactly what this feature needs and nothing else; the
// viewer satisfies it incidentally, without knowing this package exists.
// The card itself - scrim, message, buttons, focus rings, the selection
// behind them, and the Left/Right/Return/Escape handling - is
// internal/ui/widgets.ChoiceCard, shared with the export-format prompt.
package deletion

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/trash"
	"github.com/frathe/picfetch/internal/ui/widgets"
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

// cancelChoice and dangerChoice are the card's two button indices - Cancel
// first/left (selected by default), the red "Move to Trash" button
// second/right, so the Right arrow key - which moves selection toward the
// higher index - points toward where it actually sits.
const (
	cancelChoice = 0
	dangerChoice = 1
)

// Confirmer is the confirmation card and the state behind it.
type Confirmer struct {
	host Host

	// targets is what confirming would move to the Trash: one file for the
	// Shift+Delete on the image being viewed, or the grid's whole selection
	// for a batch. Captured by Request/RequestFiles rather than read back
	// when the danger button is pressed, so a card that is already up asks
	// about exactly the files it named.
	targets []Target

	// pending tracks the background goroutine performDelete starts for its
	// trash.Move call - see performDelete's doc comment for why that call
	// can't run on the UI goroutine. Settle waits on it, the same way
	// slideshow.Controller's own pending WaitGroup lets tests wait out its
	// background goroutine before asserting on state it's still touching.
	pending sync.WaitGroup

	// card is the prompt itself: scrim, message, the two buttons and their
	// focus rings, and the Left/Right/Return/Escape handling over them. It
	// owns the selection outright - this type keeps no copy of it, so a
	// click on a button (which reaches the card directly, never this
	// package) can't leave the two disagreeing.
	card *widgets.ChoiceCard
}

// New builds the confirmation card (hidden) around host.
//
// The card is a dimmed scrim behind a centered box, with Cancel first/left
// (selected by default) and the red "Move to Trash" button second/right -
// see cancelChoice/dangerChoice. The Cancel choice runs nothing of its own:
// widgets.ChoiceCard already hides itself before running either choice, and
// that is everything Cancel/Escape have ever needed to do here, so there is
// nothing left for SetOnCancel to add.
func New(host Host) *Confirmer {
	c := &Confirmer{host: host}

	c.card = widgets.NewChoiceCard(host.ForceRepaint,
		widgets.Choice{Label: lang.L("Cancel")},
		widgets.Choice{
			Label:      lang.L("Move to Trash"),
			Importance: widget.DangerImportance,
			OnChosen:   c.performDelete,
		},
	)

	return c
}

// Overlay is the card, for the app to place in its window stack.
func (c *Confirmer) Overlay() fyne.CanvasObject {
	return c.card.Overlay()
}

// Visible reports whether the card is up - the app's key dispatcher checks
// this before its own handling.
func (c *Confirmer) Visible() bool {
	return c.card.Visible()
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
	if len(targets) == 0 || c.card.Visible() {
		return
	}

	c.targets = targets

	var msg string
	if len(targets) == 1 {
		msg = fmt.Sprintf(lang.L("Move %q to the Trash?"), targets[0].URI.Name())
	} else {
		msg = fmt.Sprintf(lang.L("Move %d files to the Trash?"), len(targets))
	}

	// Show resets the selection to cancelChoice, so a card never opens with
	// the destructive button already under Return.
	c.card.Show(msg)
}

// Cancel dismisses the card without touching any file - Escape while it's
// up, clicking Cancel, or a fresh drop arriving mid-prompt. A no-op when
// the card isn't showing, so callers that call it defensively (the app, on
// every drop) don't need to check Visible themselves first.
func (c *Confirmer) Cancel() {
	if !c.card.Visible() {
		return
	}

	c.card.Hide()
}

// HandleKey handles a key press while the card is up: Left/Right move the
// selection, Return runs whichever action is selected, Escape cancels.
// Every other key is deliberately swallowed by the caller.
func (c *Confirmer) HandleKey(ev *fyne.KeyEvent) {
	c.card.HandleKey(ev)
}

// performDelete is the danger button's action (or Return with it
// selected): it moves the current file to the OS trash/recycle bin via
// trash.Move rather than removing it outright, so Shift+Delete is
// recoverable the same way a delete from Finder/Explorer/a Linux file
// manager already is. It runs as the danger choice's OnChosen, so
// widgets.ChoiceCard has already hidden the card by the time this starts.
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
