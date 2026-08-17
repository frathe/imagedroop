// canExport/exportAs (export.go): the File > "Export as PNG…"/"Export as
// JPEG…" actions that write the frame on screen to a new file, in a format
// chosen by the menu item rather than by the source file.
//
// Per-OS save-panel dispatch (zenity/PowerShell/AppKit) is covered by
// internal/filepicker's own tests, and the encoders by internal/imaging's;
// what's here is the viewer's integration with both - the enable rules, the
// suggested name, the extension the destination ends up with, and the
// error/cancel paths.

package ui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/filepicker"
	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/uitest"
)

// --- canExport -----------------------------------------------------------

func TestCanExport_FalseWithNoImage(t *testing.T) {
	v := newTestViewer(t)

	if v.canExport() {
		t.Error("canExport should be false with nothing loaded")
	}
}

func TestCanExport_TrueForALoadedImage(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	if !v.canExport() {
		t.Error("canExport should be true once an image is loaded")
	}
}

// TestCanExport_TrueForAFormatWithNoEncoder is the gap the export action
// exists to close: a .webp can be displayed but never saved back, so Save
// Changes stays disabled for it while Export stays available. The file
// holds JPEG bytes under a .webp name, the same trick
// TestCanSaveRotation_FalseForUnsupportedFormat uses - image.Decode sniffs
// magic bytes, so it still displays, and the difference below comes purely
// from the extension.
func TestCanExport_TrueForAFormatWithNoEncoder(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.webp", uitest.EncodeJPEG(t, 4, 4, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	v.rotateBy(1)

	if v.canSaveRotation() {
		t.Fatal("canSaveRotation should be false for .webp - the premise of this test")
	}
	if !v.canExport() {
		t.Error("canExport should be true for a format with no encoder of its own")
	}
}

// TestCanExport_TrueForAnAnimatedImage is the other half of that gap: Save
// Changes refuses an animation because it would have to re-encode every
// frame, but exporting the frame on screen as a still is well-defined. Uses
// the 10s-per-frame delay trick (see TestCanSaveRotation_FalseForAnimatedImage)
// so animate's goroutine never fires during the test.
func TestCanExport_TrueForAnAnimatedImage(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{1000, 1000}))
	dropAndWait(t, v, storage.NewFileURI(path))

	if v.canSaveRotation() {
		t.Fatal("canSaveRotation should be false for an animation - the premise of this test")
	}
	if !v.canExport() {
		t.Error("canExport should be true for an animation - exporting the displayed frame is well-defined")
	}
}

// TestCanExport_FalseWhileLoading mirrors TestCanSaveRotation_FalseWhileLoading:
// mid-load CurrentFile() already names the file being navigated to while
// v.img.Image still holds the previous one's pixels, so an export started
// then would suggest the new file's name for the old file's image.
func TestCanExport_FalseWhileLoading(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	v.loading.Store(true)
	t.Cleanup(func() { v.loading.Store(false) })

	if v.canExport() {
		t.Error("canExport should be false while a load is in flight")
	}
}

// --- exportAs ------------------------------------------------------------

func TestExportAs_WritesTheDisplayedFrameToThePickedPath(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 2, color.White)) // asymmetric
	dropAndWait(t, v, storage.NewFileURI(path))

	v.rotateBy(1) // 4x2 -> 2x4; the export must carry the rotation on screen
	dest := filepath.Join(t.TempDir(), "copy.png")
	uitest.StubSaveChooser(t, func(string) ([]byte, error) { return []byte(dest + "\n"), nil })

	v.exportAs(".png")
	settleChooser(t, v)

	loaded, err := loadExported(t, dest)
	if err != nil {
		t.Fatalf("load the exported file: %v", err)
	}
	if b := loaded.Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("exported bounds = %v, want 2x4 (the rotation on screen carried into the file)", b)
	}

	// The source must be left exactly as it was: an export is a copy, not a
	// save, so the pending rotation is still pending afterwards.
	if v.rotation == 0 {
		t.Error("rotation = 0, want the pending rotation left untouched by an export")
	}
	src, err := loadExported(t, path)
	if err != nil {
		t.Fatalf("reload the source file: %v", err)
	}
	if b := src.Bounds(); b.Dx() != 4 || b.Dy() != 2 {
		t.Errorf("source bounds = %v, want the original 4x2 - an export must never write the source", b)
	}

	settleToast(t, v) // a successful export toasts
}

// TestExportAs_ExportsAFormatThatHasNoEncoderOfItsOwn is the end-to-end
// version of TestCanExport_TrueForAFormatWithNoEncoder: pixels this module
// can decode but never write back get out through the export path.
func TestExportAs_ExportsAFormatThatHasNoEncoderOfItsOwn(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.webp", uitest.EncodeJPEG(t, 6, 3, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	dest := filepath.Join(t.TempDir(), "copy.png")
	uitest.StubSaveChooser(t, func(string) ([]byte, error) { return []byte(dest + "\n"), nil })

	v.exportAs(".png")
	settleChooser(t, v)

	loaded, err := loadExported(t, dest)
	if err != nil {
		t.Fatalf("load the exported file: %v", err)
	}
	if b := loaded.Bounds(); b.Dx() != 6 || b.Dy() != 3 {
		t.Errorf("exported bounds = %v, want 6x3", b)
	}

	settleToast(t, v)
}

func TestExportAs_SuggestsTheSourceNameWithTheNewExtensionInItsOwnFolder(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "holiday.webp", uitest.EncodeJPEG(t, 4, 4, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	var suggested string
	uitest.StubSaveChooser(t, func(s string) ([]byte, error) {
		suggested = s
		return nil, nil // cancelled: this test only cares what the panel was offered
	})

	v.exportAs(".png")
	settleChooser(t, v)

	if want := filepath.Join(filepath.Dir(path), "holiday.png"); suggested != want {
		t.Errorf("suggested path = %q, want %q", suggested, want)
	}
}

// TestExportAs_CancelWritesNothing watches the *source* file's own folder,
// since that is where the suggested path points and so the one place a
// cancel mishandled as a valid empty pick could plausibly write. It covers
// the empty-output cancel macOS and Windows produce; zenity's own cancel is
// a non-zero exit indistinguishable from a real failure, and takes
// reportChooserError's path instead - see TestReportChooserError_TogglesToastByOS.
func TestExportAs_CancelWritesNothing(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.jpg", uitest.EncodeJPEG(t, 4, 4, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	uitest.StubSaveChooser(t, func(string) ([]byte, error) { return nil, nil })

	v.exportAs(".png")
	settleChooser(t, v)

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %v, want only the source file - a cancelled export must write nothing", entries)
	}
	if v.toast.card.Visible() {
		t.Error("a cancelled export should not toast")
	}
}

// TestExportAs_AppendsTheFormatExtensionWhenThePickedNameCannotBeEncoded
// covers the rule that keeps a file's bytes matching its name: whatever the
// user typed, the file ends up with an extension this module can actually
// encode.
func TestExportAs_AppendsTheFormatExtensionWhenThePickedNameCannotBeEncoded(t *testing.T) {
	tests := []struct {
		name   string
		picked string
		want   string
	}{
		{"no extension at all", "copy", "copy.png"},
		{"an extension with no encoder", "copy.webp", "copy.webp.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestViewer(t)
			dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

			dir := t.TempDir()
			uitest.StubSaveChooser(t, func(string) ([]byte, error) {
				return []byte(filepath.Join(dir, tt.picked) + "\n"), nil
			})

			v.exportAs(".png")
			settleChooser(t, v)

			if _, err := os.Stat(filepath.Join(dir, tt.want)); err != nil {
				entries, _ := os.ReadDir(dir)
				t.Errorf("expected %q to exist, got error %v; directory holds %v", tt.want, err, entries)
			}

			settleToast(t, v)
		})
	}
}

// TestExportAs_AppendsTheExtensionOfTheFormatActuallyPicked pins the
// appended extension to the menu item the user chose, which
// TestExportAs_AppendsTheFormatExtensionWhenThePickedNameCannotBeEncoded
// alone can't: it only ever exercises the PNG item, so a hardcoded ".png"
// in exportDestination would satisfy it. Checking the magic bytes as well
// as the name is what makes this about the format rather than the spelling.
func TestExportAs_AppendsTheExtensionOfTheFormatActuallyPicked(t *testing.T) {
	tests := []struct {
		ext   string
		want  string
		magic []byte
	}{
		{exportPNGExt, "copy.png", []byte("\x89PNG\r\n\x1a\n")},
		{exportJPEGExt, "copy.jpg", []byte{0xFF, 0xD8, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			v := newTestViewer(t)
			dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

			dir := t.TempDir()
			uitest.StubSaveChooser(t, func(string) ([]byte, error) {
				return []byte(filepath.Join(dir, "copy") + "\n"), nil // no extension typed
			})

			v.exportAs(tt.ext)
			settleChooser(t, v)

			data, err := os.ReadFile(filepath.Join(dir, tt.want))
			if err != nil {
				entries, _ := os.ReadDir(dir)
				t.Fatalf("expected %q to exist, got error %v; directory holds %v", tt.want, err, entries)
			}
			if !bytes.HasPrefix(data, tt.magic) {
				t.Errorf("%s does not start with its format's magic bytes: % x", tt.want, data[:min(8, len(data))])
			}

			settleToast(t, v)
		})
	}
}

// TestExportAs_HonorsAnEncodableExtensionTheUserTyped is the other side of
// that rule: a name the user typed that this module *can* encode wins over
// the menu item they picked, so the file never claims a format its bytes
// aren't in.
func TestExportAs_HonorsAnEncodableExtensionTheUserTyped(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	dest := filepath.Join(t.TempDir(), "copy.jpg")
	uitest.StubSaveChooser(t, func(string) ([]byte, error) { return []byte(dest + "\n"), nil })

	v.exportAs(".png") // the PNG menu item, overridden by the typed .jpg
	settleChooser(t, v)

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read the exported file: %v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		t.Errorf("exported file does not start with the JPEG magic bytes: % x", data[:min(4, len(data))])
	}

	settleToast(t, v)
}

func TestExportAs_ReportsAFailedWrite(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	// A directory that does not exist, so imaging.Export's own temp-file
	// creation fails - the same shape as any unwritable destination.
	dest := filepath.Join(t.TempDir(), "no-such-dir", "copy.png")
	uitest.StubSaveChooser(t, func(string) ([]byte, error) { return []byte(dest + "\n"), nil })

	v.exportAs(".png")
	settleChooser(t, v)

	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the failed export")
	}

	settleToast(t, v)
}

func TestExportAs_NoOpWithNothingLoaded(t *testing.T) {
	v := newTestViewer(t)

	called := false
	uitest.StubSaveChooser(t, func(string) ([]byte, error) {
		called = true
		return nil, nil
	})

	v.exportAs(".png")

	if called {
		t.Error("exportAs should never open a save panel with nothing loaded")
	}
}

func TestExportAs_RunsSavePanelInBackground(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	called := make(chan struct{})
	orig := filepicker.ChooseSave
	t.Cleanup(func() { filepicker.ChooseSave = orig })
	filepicker.ChooseSave = func(string) ([]byte, error) {
		close(called)
		return nil, errors.New("stub: not exercising the success path here")
	}

	v.exportAs(".png")

	select {
	case <-called:
	case <-time.After(testTimeout):
		t.Fatal("expected exportAs to invoke the native save panel")
	}

	settleChooser(t, v)
}

// --- Export menu items ---------------------------------------------------

func TestExportItems_DisabledInitiallyAndEnabledOnceAnImageLoads(t *testing.T) {
	v := newTestViewer(t)

	for i, item := range []*fyne.MenuItem{v.exportPNGItem, v.exportJPEGItem} {
		if !item.Disabled {
			t.Errorf("export item %d should start disabled, with nothing loaded", i)
		}
	}

	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	for i, item := range []*fyne.MenuItem{v.exportPNGItem, v.exportJPEGItem} {
		if item.Disabled {
			t.Errorf("export item %d should be enabled once an image is loaded", i)
		}
	}

	v.closeFiles()

	for i, item := range []*fyne.MenuItem{v.exportPNGItem, v.exportJPEGItem} {
		if !item.Disabled {
			t.Errorf("export item %d should be disabled again after Close Files", i)
		}
	}
}

func TestBuildMainMenu_ExportItemsExportTheirOwnFormat(t *testing.T) {
	tests := []struct {
		name  string
		index int
		want  string
	}{
		{"Export as PNG…", 2, ".png"},
		{"Export as JPEG…", 3, ".jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestViewer(t)
			dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

			var suggested string
			uitest.StubSaveChooser(t, func(s string) ([]byte, error) {
				suggested = s
				return nil, nil
			})

			menu := buildMainMenu(v)
			menu.Items[0].Items[tt.index].Action()
			settleChooser(t, v)

			if got := filepath.Ext(suggested); got != tt.want {
				t.Errorf("the %s action suggested %q (extension %q), want extension %q", tt.name, suggested, got, tt.want)
			}
		})
	}
}

// --- suggestedExportPath -------------------------------------------------

// TestSuggestedExportPath covers the name-building rules directly, since
// the panel-level test above can only reach the ordinary case.
func TestSuggestedExportPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		ext  string
		want string
	}{
		{"swaps the extension", "/photos/holiday.webp", ".png", "/photos/holiday.png"},
		{"keeps a name with no extension", "/photos/holiday", ".png", "/photos/holiday.png"},
		{"only the last dot is the extension", "/photos/holiday.2024.heic", ".jpg", "/photos/holiday.2024.jpg"},
		// A name that is nothing but an extension would otherwise suggest a
		// bare ".png", which the panel shows as an empty file-name field.
		{"falls back for a name that is only an extension", "/photos/.jpg", ".png", "/photos/image.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestedExportPath(storage.NewFileURI(tt.path), tt.ext); got != tt.want {
				t.Errorf("suggestedExportPath(%q, %q) = %q, want %q", tt.path, tt.ext, got, tt.want)
			}
		})
	}
}

// loadExported reads a file back through the real decode path, the same way
// save_test.go checks a written file.
func loadExported(t *testing.T, path string) (image.Image, error) {
	t.Helper()

	loaded, err := imaging.LoadImage(storage.NewFileURI(path), imaging.DefaultImgCacheBytes)
	if err != nil {
		return nil, err
	}
	return loaded.Frames[0], nil
}
