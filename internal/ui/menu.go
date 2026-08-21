// The window's menu bar: File (open, close, settings), Favorites, and Help.

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
)

// buildMainMenu assembles the menu bar. Composed here rather than inside
// either feature package, per the "internal/ui decides how features
// compose" rule (see ARCHITECTURE.md) - help.Menu and settingswin.Window
// both stay ignorant of where they sit in the bar.
func buildMainMenu(view *viewer) *fyne.MainMenu {
	open := fyne.NewMenuItem(lang.L("Open Files…"), func() { view.openFileDialog() })
	// Display-only: the Cmd/Ctrl+O binding itself is wireOpenShortcuts's
	// AddShortcut call in shortcuts.go. This just shows the same accelerator
	// as a hint next to the menu item.
	open.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault,
	}

	save := fyne.NewMenuItem(lang.L("Save Changes"), func() { view.saveRotation() })
	save.Disabled = true // updateFileMenuState (save.go) enables it once there's a pending rotation to save
	save.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}
	view.saveItem = save

	export := fyne.NewMenuItem(lang.L("Export image"), func() { view.promptExport() })
	export.Disabled = true // updateFileMenuState (save.go) enables it once an image is loaded
	// Display-only, like Open's above: the binding itself is
	// wireExportShortcuts's AddShortcut call in shortcuts.go.
	export.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyE,
		Modifier: fyne.KeyModifierShortcutDefault,
	}
	view.exportItem = export

	setWallpaper := fyne.NewMenuItem(lang.L("Set as Wallpaper"), func() { view.setAsWallpaper() })
	setWallpaper.Disabled = true // updateFileMenuState (save.go) enables it once an image is loaded
	// Also display-only, also bound in wireExportShortcuts - Shift added to
	// Export image's own Cmd/Ctrl+E, since the two sit right next to each
	// other in the menu.
	setWallpaper.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyE,
		Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift,
	}
	view.wallpaperItem = setWallpaper

	closeFiles := fyne.NewMenuItem(lang.L("Close Files"), func() { view.closeFiles() })
	closeFiles.Disabled = true // updateFileMenuState (save.go) enables it once a file is loaded
	view.closeFilesItem = closeFiles
	settings := fyne.NewMenuItem(lang.L("Settings…"), func() { view.settingsWin.Show() })

	fileMenu := fyne.NewMenu(lang.L("File"),
		open, save, export, setWallpaper, closeFiles, fyne.NewMenuItemSeparator(), settings)

	return fyne.NewMainMenu(fileMenu, view.favorites.Menu(), view.help.Menu())
}
