// Package favorites owns the Favorites menu and its dialogs.
package favorites

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

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

	manageDialog dialog.Dialog
	pending      sync.WaitGroup
}

// New builds the Favorites menu without reading from disk.
func New(host Host, win fyne.Window) *Feature {
	f := &Feature{host: host, win: win}
	f.addItem = fyne.NewMenuItem(lang.L("Add Current List to Favorites…"), f.addToFavorites)
	f.addItem.Disabled = true
	f.manageItem = fyne.NewMenuItem(lang.L("Manage Favorites…"), f.showManage)
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
		item := fyne.NewMenuItem(favoriteName, func() {
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

func (f *Feature) addToFavorites() {
	form, _ := f.newAddDialog()
	form.Show()
}

func (f *Feature) newAddDialog() (*dialog.FormDialog, *widget.Entry) {
	entry := widget.NewEntry()
	reason := lang.L(`enter a name without / \ : * ? " < > |`)
	entry.Validator = validation.NewAllStrings(
		validation.NewRegexp(`^[^/\\:*?"<>|]+$`, reason),
		func(name string) error {
			if !favstore.ValidName(strings.TrimSpace(name)) {
				return errors.New(reason)
			}
			return nil
		},
	)

	form := dialog.NewForm(
		lang.L("Add to Favorites"),
		lang.L("Add"),
		lang.L("Cancel"),
		[]*widget.FormItem{widget.NewFormItem(lang.L("Name"), entry)},
		func(confirmed bool) {
			if confirmed {
				f.saveFavorite(entry.Text)
			}
		},
		f.win,
	)
	return form, entry
}

func (f *Feature) saveFavorite(name string) {
	name = strings.TrimSpace(name)
	if !favstore.ValidName(name) {
		f.host.ShowToast(lang.L(`enter a name without / \ : * ? " < > |`))
		return
	}

	if favstore.Exists(f.dir, name) {
		confirm := dialog.NewConfirm(
			lang.L("Replace Favorite"),
			fmt.Sprintf(lang.L("A favorite named %q already exists. Replace it?"), name),
			func(replace bool) {
				if replace {
					f.writeFavorite(name)
				}
			},
			f.win,
		)
		confirm.SetConfirmText(lang.L("Replace"))
		confirm.Show()
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

func (f *Feature) showManage() {
	names, err := favstore.List(f.dir)
	if err != nil {
		f.reportError(lang.L("could not list favorites: %v"), err)
		return
	}

	var content fyne.CanvasObject
	if len(names) == 0 {
		content = widget.NewLabel(lang.L("No favorites yet"))
	} else {
		rows := make([]fyne.CanvasObject, 0, len(names))
		for _, name := range names {
			favoriteName := name
			remove := widget.NewButton(lang.L("Remove"), func() {
				f.removeFavorite(favoriteName)
			})
			rows = append(rows, container.NewBorder(nil, nil, nil, remove,
				widget.NewLabel(favoriteName)))
		}
		scroll := container.NewVScroll(container.NewVBox(rows...))
		scroll.SetMinSize(fyne.NewSize(420, 240))
		content = scroll
	}

	f.manageDialog = dialog.NewCustom(
		lang.L("Manage Favorites"),
		lang.L("Close"),
		content,
		f.win,
	)
	f.manageDialog.Show()
}

func (f *Feature) removeFavorite(name string) {
	confirm := dialog.NewConfirm(
		lang.L("Remove Favorite"),
		fmt.Sprintf(lang.L("Remove %q from favorites? Its folder will be moved to the Trash."), name),
		func(remove bool) {
			if remove {
				f.performRemove(name)
			}
		},
		f.win,
	)
	confirm.SetConfirmText(lang.L("Remove"))
	confirm.SetConfirmImportance(widget.DangerImportance)
	confirm.Show()
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
			if f.manageDialog != nil {
				f.manageDialog.Hide()
				f.showManage()
			}
		})
	}()
}

func (f *Feature) reportError(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fyne.LogError("favorites operation failed", errors.New(message))
	f.host.ShowToast(message)
}
