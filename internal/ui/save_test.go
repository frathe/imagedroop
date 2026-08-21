// canSaveRotation/saveRotation (save.go): the File > Save Changes action
// that persists rotate.go's view-only rotation back to disk, plus the menu
// item and Cmd/Ctrl+S shortcut that trigger it.

package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/uitest"
)

// --- canSaveRotation ---------------------------------------------------

func TestCanSaveRotation_FalseWithNoImage(t *testing.T) {
	v := newTestViewer(t)

	if v.canSaveRotation() {
		t.Error("canSaveRotation should be false with nothing loaded")
	}
}

func TestCanSaveRotation_FalseWithZeroRotation(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	if v.canSaveRotation() {
		t.Error("canSaveRotation should be false before any rotation")
	}
}

func TestCanSaveRotation_TrueAfterRotatingAnEncodableFormat(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.rotateBy(1)

	if !v.canSaveRotation() {
		t.Error("canSaveRotation should be true after rotating a JPEG")
	}
}

// TestCanSaveRotation_FalseForUnsupportedFormat names the file .webp while
// actually writing JPEG bytes to it: image.Decode sniffs the format from
// its magic bytes rather than the extension, so the viewer still displays
// it fine (proving the false result below comes from imaging.CanEncode's
// extension check, not a load failure).
func TestCanSaveRotation_FalseForUnsupportedFormat(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.webp", uitest.EncodeJPEG(t, 4, 4, color.White))
	dropAndWait(t, v, storage.NewFileURI(path))

	v.rotateBy(1)

	if v.rotation == 0 {
		t.Fatal("expected rotateBy to still rotate the view for an unsupported save format")
	}
	if v.canSaveRotation() {
		t.Error("canSaveRotation should be false for a format with no encoder (.webp)")
	}
}

// TestCanSaveRotation_FalseForAnimatedImage parks animate so its background
// goroutine never fires during the test - nothing here depends on the
// animation itself, only on displayFrames holding more than one frame.
func TestCanSaveRotation_FalseForAnimatedImage(t *testing.T) {
	v := newTestViewer(t)
	parkAnimate(v)
	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2}))
	dropAndWait(t, v, storage.NewFileURI(path))

	v.rotateBy(1)

	if v.canSaveRotation() {
		t.Error("canSaveRotation should be false for an animated image, even mid-rotation")
	}
}

func TestCanSaveRotation_FalseWhileLoading(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.rotateBy(1)
	if !v.canSaveRotation() {
		t.Fatal("expected canSaveRotation to be true before simulating an in-flight load")
	}

	v.loading.Store(true)
	t.Cleanup(func() { v.loading.Store(false) })

	if v.canSaveRotation() {
		t.Error("canSaveRotation should be false while a load is in flight - v.state.index may already point at a file whose pixels haven't finished decoding")
	}
}

// --- saveRotation --------------------------------------------------------

func TestSaveRotation_WritesRotatedPixelsAndResetsState(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 2, color.White)) // asymmetric
	u := storage.NewFileURI(path)
	dropAndWait(t, v, u)

	v.rotateBy(1) // 4x2 -> 2x4
	if v.rotation == 0 {
		t.Fatal("expected a nonzero rotation before saving")
	}
	wantBounds := v.img.Image.Bounds()

	v.saveRotation()

	if v.rotation != 0 {
		t.Errorf("rotation = %d, want reset to 0 after a successful save", v.rotation)
	}
	if got := v.img.Image.Bounds(); got != wantBounds {
		t.Errorf("bounds after save = %v, want unchanged at %v - saving should never itself change what's on screen", got, wantBounds)
	}
	if !v.saveItem.Disabled {
		t.Error("Save Changes menu item should be disabled again once there's nothing left to save")
	}

	loaded, err := imaging.LoadImage(u, imaging.DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("reload the saved file: %v", err)
	}
	if b := loaded.Frames[0].Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("saved file bounds = %v, want 2x4 (the rotated dimensions persisted to disk)", b)
	}

	settleToast(t, v) // saveRotation shows a "Saved" toast
}

func TestSaveRotation_NoOpWhenNothingToSave(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.saveRotation()

	if v.rotation != 0 {
		t.Error("saveRotation should no-op with nothing to save")
	}
	if v.toast.card.Visible() {
		t.Error("saveRotation should not show a toast when it's a no-op")
	}
}

// TestSaveRotation_FailedWriteLeavesRotationUnchanged forces SaveRotated's
// os.CreateTemp to fail by deleting the file's directory out from under it -
// deliberately not an unsupported-format file, since canSaveRotation's own
// imaging.CanEncode check would make saveRotation no-op before ever
// attempting one of those. This is the only other way saveRotation's error
// path can be reached.
func TestSaveRotation_FailedWriteLeavesRotationUnchanged(t *testing.T) {
	v := newTestViewer(t)
	path := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 4, color.White))
	u := storage.NewFileURI(path)
	dropAndWait(t, v, u)

	v.rotateBy(1)
	wantRotation := v.rotation

	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatalf("remove temp dir: %v", err)
	}

	v.saveRotation()

	if v.rotation != wantRotation {
		t.Errorf("rotation = %d, want unchanged at %d after a failed save", v.rotation, wantRotation)
	}

	settleToast(t, v) // saveRotation shows an error toast
}

// --- Save Changes menu item ----------------------------------------------

func TestSaveItem_DisabledInitially(t *testing.T) {
	v := newTestViewer(t)

	if !v.saveItem.Disabled {
		t.Error("Save Changes should start disabled, with nothing loaded")
	}
}

func TestSaveItem_EnabledAfterRotatingAndDisabledAfterNavigatingAway(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.Black)
	dropAndWait(t, v, a, b)

	v.rotateBy(1)
	if v.saveItem.Disabled {
		t.Fatal("Save Changes should be enabled after rotating")
	}

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	if !v.saveItem.Disabled {
		t.Error("Save Changes should be disabled again after navigating away - the rotation didn't carry over")
	}
}

func TestSaveItem_DisabledAfterCloseFiles(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.rotateBy(1)
	if v.saveItem.Disabled {
		t.Fatal("Save Changes should be enabled after rotating")
	}

	v.closeFiles()

	if !v.saveItem.Disabled {
		t.Error("Save Changes should be disabled after Close Files clears the loaded image")
	}
}

func TestBuildMainMenu_SaveChangesItemInvokesSaveRotation(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	v.rotateBy(1)

	menu := buildMainMenu(v)
	menu.Items[0].Items[1].Action()

	if v.rotation != 0 {
		t.Error("the Save Changes menu action should invoke saveRotation")
	}

	settleToast(t, v) // saveRotation shows a "Saved" toast
}

// --- Cmd/Ctrl+S shortcut --------------------------------------------------

// TestWireSaveShortcut_SavesTheCurrentRotation mirrors
// TestWireClipboardShortcuts_CopiesImageAndPath: S isn't one of the glfw
// driver's specially-cased bare shortcuts (see wireClipboardShortcuts' own
// comment for the ones that are), so a real Cmd/Ctrl+S reaches
// wireSaveShortcut's plain desktop.CustomShortcut registration the same way
// Cmd/Ctrl+O does.
func TestWireSaveShortcut_SavesTheCurrentRotation(t *testing.T) {
	v := newTestViewer(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)
	v.rotateBy(1)

	handler := &fyne.ShortcutHandler{}
	wireSaveShortcut(handler, v)

	handler.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault})

	if v.rotation != 0 {
		t.Error("expected Cmd/Ctrl+S to save the current rotation")
	}

	settleToast(t, v) // saveRotation shows a "Saved" toast
}
