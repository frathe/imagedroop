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

	// The two export items carry no accelerator: they sit next to Save
	// Changes' own Cmd/Ctrl+S, and a "which of these two formats did I just
	// bind?" shortcut is worse than none at all.
	exportPNG := fyne.NewMenuItem(lang.L("Export as PNG…"), func() { view.exportAs(exportPNGExt) })
	exportPNG.Disabled = true // updateFileMenuState (save.go) enables both once an image is loaded
	view.exportPNGItem = exportPNG

	exportJPEG := fyne.NewMenuItem(lang.L("Export as JPEG…"), func() { view.exportAs(exportJPEGExt) })
	exportJPEG.Disabled = true
	view.exportJPEGItem = exportJPEG

	// Carries no accelerator either, for the same reason the export items
	// don't: it sits among them, and nothing about the action suggests one
	// key over another.
	setWallpaper := fyne.NewMenuItem(lang.L("Set as Wallpaper"), func() { view.setAsWallpaper() })
	setWallpaper.Disabled = true // updateFileMenuState (save.go) enables it once an image is loaded
	view.wallpaperItem = setWallpaper

	closeFiles := fyne.NewMenuItem(lang.L("Close Files"), func() { view.closeFiles() })
	closeFiles.Disabled = true // updateFileMenuState (save.go) enables it once a file is loaded
	view.closeFilesItem = closeFiles
	settings := fyne.NewMenuItem(lang.L("Settings…"), func() { view.settings.Show() })

	fileMenu := fyne.NewMenu(lang.L("File"),
		open, save, exportPNG, exportJPEG, setWallpaper, closeFiles, fyne.NewMenuItemSeparator(), settings)

	return fyne.NewMainMenu(fileMenu, view.favorites.Menu(), view.help.Menu())
}
