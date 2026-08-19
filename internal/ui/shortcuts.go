// Application-wide shortcut registration. The individual wiring functions
// remain separate so focused tests can exercise the production bindings
// directly through Fyne's shortcut handler.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/frathe/picfetch/internal/ui/deletion"
	"github.com/frathe/picfetch/internal/ui/favorites"
)

// shortcutAdder is the one method the shortcut wiring needs from fyne.Canvas,
// narrow enough that a bare *fyne.ShortcutHandler satisfies it too -
// so tests can drive the exact same wiring against that handler directly
// and then fire it via its own TypedShortcut, instead of going through a
// full canvas. That detour is load-bearing, not a style choice: Fyne's test
// driver canvas (fyne.io/fyne/v2/test) embeds software.WindowlessCanvas by
// interface, which doesn't include TypedShortcut, so a real Ctrl+O key
// event can never be simulated through it - only the production glfw
// driver's canvas has that method reachable at all (see
// internal/driver/glfw/window.go's triggersShortcut, which is what turns a
// real key-plus-modifier press into this call).
type shortcutAdder interface {
	AddShortcut(shortcut fyne.Shortcut, handler func(shortcut fyne.Shortcut))
}

// wireGlobalShortcuts keeps the application-wide shortcut groups composed in
// one visible sequence. This is the same order buildViewer used before the
// registration moved out of the top-level assembly.
func wireGlobalShortcuts(c shortcutAdder, view *viewer) {
	wireOpenShortcuts(c, view)
	wireFavoriteShortcuts(c, view.favorites.Open)
	wireClipboardShortcuts(c, view)
	wireDeleteShortcut(c, view)
	wireSelectAllShortcut(c, view)
	wireSaveShortcut(c, view)
}

// wireOpenShortcuts binds Cmd/Ctrl+O and Cmd/Ctrl+Shift+O to the same
// native file/folder browser tapping the drop zone already opens
// (openFileDialog in openfiles.go). There's only one such dialog - it
// combines files and folders in one go, see internal/filepicker - so the
// second, modified binding isn't a second dialog, just a second way to
// reach the first one. Modified key combos never reach handleKeyEvent's
// SetOnTypedKey dispatch at all: Fyne's desktop driver intercepts them as
// shortcuts before TypedKey ever fires, which is why this needs
// AddShortcut instead of another case there.
func wireOpenShortcuts(c shortcutAdder, view *viewer) {
	openShortcut := func(fyne.Shortcut) { view.openFileDialog() }
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault,
	}, openShortcut)
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}, openShortcut)
}

// wireFavoriteShortcuts binds the first ten sorted favorites to Cmd/Ctrl+1
// through 9, then Cmd/Ctrl+0. The handlers stay registered while Feature.Open
// resolves each slot against the latest menu refresh after an add or removal.
func wireFavoriteShortcuts(c shortcutAdder, open func(index int)) {
	for i := 0; i < favorites.ShortcutCount; i++ {
		index := i
		c.AddShortcut(favorites.ShortcutForIndex(index), func(fyne.Shortcut) {
			open(index)
		})
	}
}

// wireClipboardShortcuts binds Cmd/Ctrl+C to copy the current image and
// Cmd/Ctrl+Shift+C to copy its file path (clipboard.go). Both need
// AddShortcut rather than handleKeyEvent's plain SetOnTypedKey dispatch, for
// the same reason wireOpenShortcuts does: modified key combos never reach
// TypedKey at all. Deliberately not gated behind handleKeyEvent's
// len(v.state.files)<2 navigation guard - both work fine with a single file
// loaded, and copyImageToClipboard/copyPathToClipboard already no-op safely
// when nothing is loaded yet.
//
// The plain Cmd/Ctrl+C binding is *not* a desktop.CustomShortcut, unlike
// every other shortcut in this file - that was the bug that shipped
// initially. Fyne's glfw driver special-cases the bare default-modifier
// forms of Z/Y/V/C/Insert/X/A (undo/redo/paste/copy/.../cut/select-all) into
// its own built-in fyne.Shortcut types *before* it ever considers building a
// desktop.CustomShortcut - see triggersShortcut in
// internal/driver/glfw/window.go, where that switch runs first and only
// falls through to a CustomShortcut when it didn't match. So a
// CustomShortcut registered for {KeyC, KeyModifierShortcutDefault} is
// simply never reachable by a real Cmd/Ctrl+C press; the driver dispatches a
// &fyne.ShortcutCopy{} instead, which needs its own AddShortcut entry to be
// caught. Shift+Cmd/Ctrl+C isn't one of the driver's special-cased combos,
// so it still becomes a CustomShortcut and needs no such treatment.
func wireClipboardShortcuts(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) { view.copySelection() })
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}, func(fyne.Shortcut) { view.copyPathToClipboard() })
}

// wireSelectAllShortcut binds Cmd/Ctrl+A to the grid's select-all
// (batch.go's selectAllInGrid). A third instance of the same driver quirk
// wireClipboardShortcuts documents: A is one of the bare combos
// triggersShortcut special-cases into a built-in fyne.Shortcut type
// (&fyne.ShortcutSelectAll{}) before it would ever build a
// desktop.CustomShortcut, so a CustomShortcut for {KeyA,
// KeyModifierShortcutDefault} could never be reached by a real key press.
func wireSelectAllShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutSelectAll{}, func(fyne.Shortcut) { view.selectAllInGrid() })
}

// wireDeleteShortcut binds Shift+Delete to open the permanent-delete
// confirmation card (deletion.Confirmer.Request). Same bug shape as
// Cmd/Ctrl+C above, different special case: triggersShortcut special-cases
// bare Shift+Delete into &fyne.ShortcutCut{Secondary: true} (its "alternative
// cut" binding, mirroring Shift+Insert for paste) *before* it would ever
// consider a desktop.CustomShortcut - and unlike the Ctrl+key cases, that
// function's CustomShortcut fallback explicitly skips building one whenever
// the modifier is bare Shift at all, so a CustomShortcut{KeyDelete,
// KeyModifierShift} registration wouldn't just be shadowed here, it could
// never be reached by any bare-Shift combo. So this needs an AddShortcut
// entry for &fyne.ShortcutCut{} instead - see deletion.ShortcutHandler
// (deletion.go) for how it tells a real Shift+Delete apart from a genuine
// Ctrl/Cmd+X (which reaches the same handler, Secondary false, and is
// correctly ignored: this app has no cut action).
//
// What it runs is batch.go's requestDelete rather than Confirmer.Request
// directly: the same key means the grid's selection while the overview is up
// and the file on screen otherwise, and deciding that is this package's job,
// not either feature package's. It used to be gated behind a `blocked` check
// that dropped the shortcut entirely while the grid was showing - there was
// nothing then for it to act on there, and the card would have opened hidden
// behind the grid's backdrop. Both of those are now handled instead of
// avoided (see the window stack in buildViewer).
func wireDeleteShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&fyne.ShortcutCut{}, deletion.ShortcutHandler(view.requestDelete))
}

// wireSaveShortcut binds Cmd/Ctrl+S to saveRotation (save.go). S isn't one
// of the driver's specially-cased bare shortcuts (only Z/Y/V/C/Insert/X/A
// are - see wireClipboardShortcuts' comment), so a plain desktop.
// CustomShortcut reaches it the same way Cmd/Ctrl+O reaches
// wireOpenShortcuts.
func wireSaveShortcut(c shortcutAdder, view *viewer) {
	c.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}, func(fyne.Shortcut) { view.saveRotation() })
}
