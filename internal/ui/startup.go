// Startup assembly loads persisted inputs once, constructs the complete
// viewer, and only then restores geometry. Runtime side effects remain in
// run.go and start after this sequence returns.
package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/preferences"
	"github.com/frathe/picfetch/internal/session"
)

// startupState is the persisted input snapshot consumed by buildViewer and
// geometry restoration.
type startupState struct {
	savedSession []fyne.URI
	prefs        preferences.State
}

// loadStartupState reads persistence and fills only preference defaults that
// have no distinct zero-value meaning.
func loadStartupState(application fyne.App) startupState {
	return startupState{
		savedSession: session.Load(application),
		prefs:        normalizePreferenceDefaults(preferences.Load(application)),
	}
}

// buildStartupViewer is the shared load, construct, then restore entry point.
// It leaves noPollerStop installed for startViewerRuntime to replace.
func buildStartupViewer(application fyne.App) (*viewer, fyne.Window) {
	startup := loadStartupState(application)
	view, window := buildViewer(application, startup)
	restoreStartupGeometry(view, window, startup)
	return view, window
}

// normalizePreferenceDefaults fills only caps. The other zero values remain
// meaningful: an unset slideshow interval is chosen on first use, geometry
// flags distinguish unsaved positions, and zero secondary geometry uses each
// window's built-in placement and size.
func normalizePreferenceDefaults(prefs preferences.State) preferences.State {
	if prefs.MaxScanFiles <= 0 {
		prefs.MaxScanFiles = defaultMaxScannedFiles
	}
	if prefs.MaxWindowWidth <= 0 {
		prefs.MaxWindowWidth = defaultMaxWindowWidth
	}
	if prefs.MaxWindowHeight <= 0 {
		prefs.MaxWindowHeight = defaultMaxWindowHeight
	}
	if prefs.MaxImageCacheMB <= 0 {
		prefs.MaxImageCacheMB = defaultMaxImageCacheMB
	}
	if prefs.MaxThumbCacheMB <= 0 {
		prefs.MaxThumbCacheMB = defaultMaxThumbCacheMB
	}
	if prefs.MaxFileSizeMB <= 0 {
		prefs.MaxFileSizeMB = defaultMaxFileSizeMB
	}

	return prefs
}

// restoreStartupGeometry runs after feature construction so the settings and
// EXIF windows exist before their remembered geometry is applied.
func restoreStartupGeometry(view *viewer, window fyne.Window, startup startupState) {
	prefs := startup.prefs

	view.settingsWin.RestoreGeometry(widgetGeometry(prefs.SettingsWindow))
	view.exif.RestoreGeometry(widgetGeometry(prefs.ExifWindow))

	initialSize := fyne.NewSize(startW, startH)
	if prefs.WindowSize.Width > 0 && prefs.WindowSize.Height > 0 {
		initialSize = prefs.WindowSize
	}
	window.Resize(initialSize)

	if prefs.WindowPositionSet {
		view.winPos.Store(prefs.WindowPosX, prefs.WindowPosY)
		view.winPos.Restore(window)
	}
}
