// Package favorites owns the Favorites menu and its dialogs.
package favorites

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/favstore"
)

// ShortcutCount is how many sorted favorites can be opened by keyboard.
const ShortcutCount = 10

var shortcutKeys = [...]fyne.KeyName{
	fyne.Key1,
	fyne.Key2,
	fyne.Key3,
	fyne.Key4,
	fyne.Key5,
	fyne.Key6,
	fyne.Key7,
	fyne.Key8,
	fyne.Key9,
	fyne.Key0,
}

// Host is the viewer behavior used by the favorites feature.
type Host interface {
	FileCount() int
	FileAt(i int) fyne.URI
	OpenFiles(files []fyne.URI)
	ShowToast(msg string)

	// SyncFavoritePreviews brings the previews stored under favDir in line
	// with files, in the background. This feature knows nothing about
	// thumbnails or caches; it only reports that a favorite's file list is
	// now this, and leaves what that costs to the host.
	SyncFavoritePreviews(favDir string, files []fyne.URI)
}

// Feature owns the Favorites menu and its dialogs.
type Feature struct {
	host Host
	win  fyne.Window
	dir  string

	menu       *fyne.Menu
	addItem    *fyne.MenuItem
	manageItem *fyne.MenuItem
	names      []string

	// manageDialog and managePanel are the Manage Favorites dialog while it
	// is up, and nil whenever it is not - see manage.go, where a non-nil
	// manageDialog doubles as the guard against stacking a second one.
	manageDialog dialog.Dialog
	managePanel  *managePanel

	// addDialog and addPanel are the Add to Favorites dialog while it is up,
	// nil whenever it is not - see add.go, where a non-nil addDialog is the
	// same kind of guard against stacking a second one.
	addDialog dialog.Dialog
	addPanel  *addPanel

	pending sync.WaitGroup
}

// New builds the Favorites menu without reading from disk.
func New(host Host, win fyne.Window) *Feature {
	f := &Feature{host: host, win: win}
	f.addItem = fyne.NewMenuItem(lang.L("Add Current List to Favorites…"), f.addToFavorites)
	f.addItem.Disabled = true
	f.manageItem = fyne.NewMenuItem(lang.L("Manage Favorites…"), f.ShowManage)
	// Display-only, mirroring how internal/ui/menu.go sets Export image's
	// and Set as Wallpaper's Shortcut fields: the binding itself is
	// wireManageFavoritesShortcut's AddShortcut call
	// (internal/ui/shortcuts.go). This just shows the same accelerator next
	// to the menu item.
	f.manageItem.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyF,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}
	f.menu = fyne.NewMenu(lang.L("Favorites"),
		f.addItem, fyne.NewMenuItemSeparator(), f.manageItem)
	return f
}

// Menu returns the feature's top-level menu.
func (f *Feature) Menu() *fyne.Menu {
	return f.menu
}

// SetDir selects the storage directory and populates the menu from it.
func (f *Feature) SetDir(dir string) {
	f.dir = dir
	f.refreshMenu()
}

// SetHasFiles enables adding the current list when it is non-empty.
func (f *Feature) SetHasFiles(has bool) {
	f.addItem.Disabled = !has
	f.menu.Refresh()
}

// ShortcutForIndex returns the Cmd/Ctrl+digit accelerator for a zero-based
// favorite index: 1 through 9, then 0 for the tenth.
func ShortcutForIndex(index int) *desktop.CustomShortcut {
	if index < 0 || index >= len(shortcutKeys) {
		return nil
	}
	return &desktop.CustomShortcut{
		KeyName:  shortcutKeys[index],
		Modifier: fyne.KeyModifierShortcutDefault,
	}
}

// Open opens the favorite currently assigned to a zero-based shortcut slot.
func (f *Feature) Open(index int) {
	if index < 0 || index >= ShortcutCount || index >= len(f.names) {
		return
	}
	f.openFavorite(f.names[index])
}

func (f *Feature) refreshMenu() bool {
	names, err := favstore.List(f.dir)
	if err != nil {
		f.reportError(lang.L("could not list favorites: %v"), err)
		return false
	}

	items := []*fyne.MenuItem{f.addItem, fyne.NewMenuItemSeparator()}
	for i, name := range names {
		favoriteName := name
		item := fyne.NewMenuItem(f.menuLabel(favoriteName), func() {
			f.openFavorite(favoriteName)
		})
		if shortcut := ShortcutForIndex(i); shortcut != nil {
			item.Shortcut = shortcut
		}
		items = append(items, item)
	}
	if len(names) > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items, f.manageItem)
	f.names = names
	f.menu.Items = items
	f.menu.Refresh()
	return true
}

// menuLabel returns a favorite's Favorites-menu label: its name and its
// stored file count, sourced from favstore.Count so the number always
// matches what opening the favorite would try to load. A count that can't
// be read falls back to the bare name rather than a toast - the favorite
// still lists, still opens (through favoriteName in refreshMenu, never this
// label), and still holds its accelerator slot; reportError per favorite
// here would turn one broken file-list.json into a toast on every refresh.
func (f *Feature) menuLabel(name string) string {
	count, err := favstore.Count(f.dir, name)
	if err != nil {
		return name
	}
	return fmt.Sprintf(lang.L("%s (%d)"), name, count)
}

// addToFavorites is the Favorites menu's own "Add Current List to
// Favorites…" item - always a fresh, empty dialog; showAdd's initial
// parameter exists for Stage 5's Replace-Cancel, which reopens with the name
// that just clashed still in the field.
func (f *Feature) addToFavorites() {
	f.showAdd("")
}

func (f *Feature) saveFavorite(name string) {
	name = strings.TrimSpace(name)
	if !favstore.ValidName(name) {
		f.host.ShowToast(lang.L(`enter a name without / \ : * ? " < > |`))
		return
	}

	if favstore.Exists(f.dir, name) {
		// Plain importance, not widget.DangerImportance: replacing a
		// favorite is not trashing one, and this prompt looked the same
		// before it went through showConfirm. Cancel is still index 0 and
		// so the default selection either way, which is what keeps a bare
		// Return from replacing by itself.
		f.showConfirm(confirmation{
			title:     lang.L("Replace Favorite"),
			message:   fmt.Sprintf(lang.L("A favorite named %q already exists. Replace it?"), name),
			action:    lang.L("Replace"),
			onConfirm: func() { f.writeFavorite(name) },
			// Cancel and Escape both land here (showConfirm runs onCancel
			// for either), and both mean the same thing: go back to the
			// field that produced this name, with the name still in it, so
			// a clash costs one keystroke rather than the whole name. Safe
			// to reopen from inside onCancel specifically because
			// showConfirm's own onClosed - which unfocuses the canvas -
			// always finishes before onCancel starts (see confirm.go), so
			// this call's own Canvas().Focus(entry) at the end of showAdd
			// is the last thing to touch focus, not undone by the outgoing
			// dialog's teardown running late.
			onCancel: func() { f.showAdd(name) },
			onClosed: func() { f.win.Canvas().Unfocus() },
		})
		return
	}
	f.writeFavorite(name)
}

func (f *Feature) writeFavorite(name string) {
	count := f.host.FileCount()
	if count == 0 {
		f.host.ShowToast(lang.L("there are no open files to add to favorites"))
		return
	}

	files := make([]fyne.URI, count)
	for i := range files {
		files[i] = f.host.FileAt(i)
	}
	if err := favstore.Save(f.dir, name, files); err != nil {
		f.reportError(lang.L("could not save favorite %q: %v"), name, err)
		return
	}

	// Reported as soon as the list is on disk, so the host can act on it
	// while the favorite sits unopened rather than only when someone
	// eventually opens it. Placed above refreshMenu because the two are
	// independent: a menu that could not be rebuilt is no reason to leave
	// the favorite just written unprepared.
	f.host.SyncFavoritePreviews(favstore.Dir(f.dir, name), files)

	if !f.refreshMenu() {
		return
	}
	f.host.ShowToast(fmt.Sprintf(lang.L("saved favorite %q"), name))
}

func (f *Feature) openFavorite(name string) {
	files, err := favstore.Load(f.dir, name)
	if err != nil {
		f.reportError(lang.L("could not open favorite %q: %v"), name, err)
		return
	}

	// Reported before the files are handed over, so whatever the host does
	// with the list in the background starts alongside the scan this open
	// triggers rather than behind it.
	f.host.SyncFavoritePreviews(favstore.Dir(f.dir, name), files)
	f.host.OpenFiles(files)
}

func (f *Feature) reportError(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fyne.LogError("favorites operation failed", errors.New(message))
	f.host.ShowToast(message)
}
