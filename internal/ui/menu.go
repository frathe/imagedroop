// The window's menu bar: File (open, close, settings) composed with
// view.help's own Help menu.

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
	// AddShortcut call in build.go. This just shows the same accelerator
	// as a hint next to the menu item.
	open.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierShortcutDefault,
	}

	save := fyne.NewMenuItem(lang.L("Save Changes"), func() { view.saveRotation() })
	save.Disabled = true // updateSaveMenuState (save.go) enables it once there's a pending rotation to save
	save.Shortcut = &desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierShortcutDefault,
	}
	view.saveItem = save

	closeFiles := fyne.NewMenuItem(lang.L("Close Files"), func() { view.closeFiles() })
	settings := fyne.NewMenuItem(lang.L("Settings…"), func() { view.settings.Show() })

	fileMenu := fyne.NewMenu(lang.L("File"), open, save, closeFiles, fyne.NewMenuItemSeparator(), settings)

	return fyne.NewMainMenu(fileMenu, view.help.Menu())
}
