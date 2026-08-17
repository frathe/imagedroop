// Persisting rotate.go's view-only rotation back to disk.

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/imaging"
)

// canSaveRotation reports whether the File > Save Changes item (also
// Cmd/Ctrl+S) should be enabled: an image is loaded, its view rotation
// (rotate.go) is non-zero, it isn't mid-load, it's a single frame, and its
// format has a registered encoder. Checked here rather than only inside
// saveRotation so the item is never offered for an action guaranteed to
// fail or do nothing.
//
//   - !v.loading.Load(): attemptLoad sets v.index to the file being
//     navigated to before that file's pixels have finished decoding, so
//     mid-load, CurrentFile() already names the new file while
//     v.displayFrames/v.img.Image still hold the old one's - saving then
//     would write the wrong file's pixels into the new file's path.
//   - len(v.displayFrames) == 1: an animated GIF's frames would all need
//     re-rotating and re-encoding as a fresh animation, which SaveRotated
//     doesn't attempt.
//   - imaging.CanEncode: WebP/HEIC/ICO/XPM have no encoder in this module's
//     dependencies (see save.go's own doc comment in internal/imaging).
func (v *viewer) canSaveRotation() bool {
	u, _, ok := v.CurrentFile()
	if !ok {
		return false
	}

	return v.rotation != 0 && !v.loading.Load() && len(v.displayFrames) == 1 && imaging.CanEncode(u)
}

// saveRotation is the File menu's "Save Changes" action (also Cmd/Ctrl+S,
// see wireSaveShortcut in build.go): it writes the currently displayed,
// already-rotated frame back to the file it came from, in that file's own
// format. A no-op unless canSaveRotation() is currently true - re-checked
// here rather than trusted from the menu item's Disabled state, since the
// shortcut bypasses the menu entirely.
func (v *viewer) saveRotation() {
	if !v.canSaveRotation() {
		return
	}

	u, _, _ := v.CurrentFile()

	if err := imaging.SaveRotated(u, v.img.Image); err != nil {
		fyne.LogError("failed to save rotation", err)
		v.ShowToast(fmt.Sprintf(lang.L("could not save %q: %v"), u.Name(), err))
		return
	}

	// The file on disk now holds exactly what v.img.Image already shows, so
	// folding that into displayFrames and zeroing rotation changes nothing
	// on screen - it just makes "unrotated" mean the file's new orientation
	// instead of the one it was decoded at. Without this, the next redraw
	// (an animate tick, or the 0 key) would revert to the old in-memory
	// pixels, which the file on disk no longer matches.
	v.displayFrames[v.displayFrameIdx] = v.img.Image
	v.rotation = 0

	// Evicted rather than mutated in place: attemptLoad is the only writer
	// of this exact key (preloadOne only ever adds neighbors), and it never
	// runs concurrently with this call - canSaveRotation's !v.loading.Load()
	// check rules that out - so a plain Remove is enough; the next visit to
	// this file just costs one re-decode instead of a stale cache hit.
	v.imgCache.Remove(u.String())

	v.updateFileMenuState()
	v.ShowToast(lang.L("Saved"))
}

// updateFileMenuState keeps the File menu's five file-dependent items in
// sync with what's currently loaded: "Save Changes" with canSaveRotation
// above, both "Export as…" items plus "Set as Wallpaper" with canExport
// (export.go, and canSetWallpaper in wallpaper.go, which is the same
// condition for the same reasons), and "Close Files" with whether there's
// anything open at all. Called
// wherever v.rotation, v.displayFrames, v.loading, or the current file can
// change: rotateBy/resetRotation (rotate.go), ShowImage/finishLoad
// (load.go), clearToDropzone (viewer.go), and after a successful
// saveRotation above. One function rather than one per item because every
// one of those sites can move more than one of these conditions at once.
func (v *viewer) updateFileMenuState() {
	v.saveItem.Disabled = !v.canSaveRotation()

	noExport := !v.canExport()
	v.exportPNGItem.Disabled = noExport
	v.exportJPEGItem.Disabled = noExport
	v.wallpaperItem.Disabled = !v.canSetWallpaper()

	v.closeFilesItem.Disabled = v.FileCount() == 0

	v.win.MainMenu().Refresh()
}
