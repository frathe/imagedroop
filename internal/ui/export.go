// Writing the frame on screen out as a new file, in a format of the user's
// choosing rather than the source file's - the File > "Export as…" actions.

package ui

import (
	"fmt"
	"image"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/imagedrop/internal/filepicker"
	"github.com/frathe/imagedrop/internal/imaging"
)

// exportPNGExt/exportJPEGExt are the two formats the File menu offers to
// export to: the universally readable lossless one and the universally
// readable lossy one. internal/imaging can encode four more (GIF, BMP,
// TIFF, AVIF) and honors any of them if the user types that extension into
// the save panel themselves - see exportDestination - but offering all six
// as menu items would be a long list for a choice that is really only ever
// these two.
const (
	exportPNGExt  = ".png"
	exportJPEGExt = ".jpg"
)

// canExport reports whether the File > "Export as…" items should be
// enabled. Deliberately a much weaker condition than canSaveRotation
// (save.go), because an export answers a different question - "write these
// pixels somewhere new", not "write them back where they came from":
//
//   - The source format doesn't matter. imaging.CanEncode gates saving
//     because SaveRotated must re-encode the file in its own format; an
//     export picks the destination's format instead, which is exactly how a
//     WebP or HEIC (decode-only in this module's dependencies) gets out.
//   - An animation doesn't matter. Save Changes refuses one because it would
//     have to re-rotate and re-encode every frame; exporting the single
//     frame on screen as a still is well-defined.
//   - A pending rotation doesn't matter. There's nothing to persist - an
//     export writes whatever is on screen, rotated or not.
//
// What does still matter is !v.loading.Load(), for the same reason it does
// in canSaveRotation: mid-load, CurrentFile() already names the file being
// navigated to while v.img.Image still holds the previous one's pixels, so
// an export started then would offer the new file's name for the old file's
// image.
func (v *viewer) canExport() bool {
	_, _, ok := v.CurrentFile()

	return ok && !v.loading.Load() && v.img.Image != nil
}

// exportAs is the File menu's "Export as PNG…"/"Export as JPEG…" action for
// the format ext: it opens the OS's own save panel and writes the frame on
// screen to whatever the user names there. A no-op unless canExport() is
// currently true.
//
// The current file and frame are captured here, on the UI goroutine, before
// the panel opens - mirroring copyImageToClipboard's own capture, and for
// the same reason: the goroutine below outlives this call by however long
// the user spends in a modal dialog, and v.img.Image belongs to the load
// path.
func (v *viewer) exportAs(ext string) {
	if !v.canExport() {
		return
	}

	src, _, _ := v.CurrentFile()
	img := v.img.Image

	// chooserDone is shared with openFileDialog's own goroutine rather than
	// given a twin of its own: it means "the native file dialog goroutine",
	// and these two are never in flight at once - both panels are app-modal,
	// so neither can be reached while the other is up.
	done := make(chan struct{})
	v.chooserDone = done

	go func() {
		defer close(done)

		v.runExport(src, img, ext)
	}()
}

// runExport is split out from exportAs the way runFileChooser is from
// openFileDialog, so tests can drive the whole panel-to-file path on a
// single goroutine. src and img are passed in rather than read from the
// viewer here, since this runs off the UI goroutine.
func (v *viewer) runExport(src fyne.URI, img image.Image, ext string) {
	out, err := filepicker.ChooseSave(suggestedExportPath(src, ext))
	if err != nil {
		v.reportChooserError(err, runtime.GOOS)
		return
	}

	picked := filepicker.ParseFileList(out)
	if len(picked) == 0 {
		return // cancelled
	}
	dest := exportDestination(picked[0], ext)

	if err := imaging.Export(dest, img); err != nil {
		fyne.LogError("failed to export image", err)
		fyne.Do(func() {
			v.ShowToast(fmt.Sprintf(lang.L("could not export %q: %v"), dest.Name(), err))
		})
		return
	}

	fyne.Do(func() {
		v.ShowToast(fmt.Sprintf(lang.L("Exported %q"), dest.Name()))
	})
}

// suggestedExportPath is what the save panel opens pre-filled with: the
// source file's own name carrying the export format's extension, in the
// source file's own folder. A full path rather than a bare name, since that
// is what every panel in internal/filepicker needs to open somewhere more
// useful than the working directory.
func suggestedExportPath(src fyne.URI, ext string) string {
	name := src.Name()
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		// A name that is nothing but an extension (".jpg"). Rare enough to
		// be worth one line rather than a suggestion of "" that the panel
		// would show as an empty file-name field.
		base = "image"
	}

	return filepath.Join(filepath.Dir(src.Path()), base+ext)
}

// exportDestination decides what the file the user named in the save panel
// is actually called. The rule is that a file's bytes must always match its
// extension: if the name they typed already carries a format this module
// can encode, that wins over the menu item they picked (typing "copy.jpg"
// means they want JPEG, whichever "Export as…" item got them here);
// otherwise the menu item's extension is appended, so "copy" becomes
// "copy.png" and "copy.webp" - a format with no encoder here - becomes
// "copy.webp.png" rather than a PNG masquerading as a WebP.
func exportDestination(picked fyne.URI, ext string) fyne.URI {
	if imaging.CanEncodeExt(picked.Extension()) {
		return picked
	}

	return storage.NewFileURI(picked.Path() + ext)
}
