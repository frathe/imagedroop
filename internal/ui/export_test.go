// canExport/exportAs (export.go): what runs once the File > "Export image"
// prompt's PNG or JPEG choice is made, writing the frame on screen to a new
// file in that format rather than the source file's own.
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
	"fyne.io/fyne/v2/driver/desktop"
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
// frame, but exporting the frame on screen as a still is well-defined.
// Parks animate so its goroutine never fires during the test.
func TestCanExport_TrueForAnAnimatedImage(t *testing.T) {
	v := newTestViewer(t)
	parkAnimate(v)
	path := uitest.WriteTempFile(t, "anim.gif", uitest.EncodeAnimatedGIF(t, 4, 4,
		[]color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}},
		[]int{2, 2}))
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

// --- Export menu item ------------------------------------------------------

func TestExportItem_DisabledInitiallyAndEnabledOnceAnImageLoads(t *testing.T) {
	v := newTestViewer(t)

	if !v.exportItem.Disabled {
		t.Error("export item should start disabled, with nothing loaded")
	}

	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	if v.exportItem.Disabled {
		t.Error("export item should be enabled once an image is loaded")
	}

	v.closeFiles()

	if !v.exportItem.Disabled {
		t.Error("export item should be disabled again after Close Files")
	}
}

// TestPromptExport_EachChoiceExportsItsOwnFormat is what the deleted
// TestBuildMainMenu_ExportItemsExportTheirOwnFormat used to pin, moved from
// the menu to the prompt: that the button labelled PNG is the one wired to
// exportPNGExt. widgets.ChoiceCard knows nothing about extensions - the
// mapping lives entirely in registerFeatures' two OnChosen closures
// (features.go), so without this, swapping them would leave the PNG button
// writing .jpg with the whole suite still green.
func TestPromptExport_EachChoiceExportsItsOwnFormat(t *testing.T) {
	tests := []struct {
		name   string
		choice int
		want   string
	}{
		{"PNG", pngChoice, ".png"},
		{"JPEG", jpegChoice, ".jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestViewer(t)
			dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

			var suggested string
			uitest.StubSaveChooser(t, func(s string) ([]byte, error) {
				suggested = s
				return nil, nil // cancelled: the suggested name is what names the format
			})

			v.promptExport()
			v.exportPrompt.Select(tt.choice)
			v.exportPrompt.Confirm()
			settleChooser(t, v)

			if got := filepath.Ext(suggested); got != tt.want {
				t.Errorf("the %s choice suggested %q (extension %q), want extension %q", tt.name, suggested, got, tt.want)
			}
		})
	}
}

// TestPromptExport_DoesNothingWhileTheDeleteCardIsUp covers the guard that
// keeps two modal cards from stacking: Cmd/Ctrl+E is a shortcut and arrives
// without passing handleKeyEvent, so without it the export prompt would
// paint over a delete confirmation that still owns the keyboard - see
// promptExport.
func TestPromptExport_DoesNothingWhileTheDeleteCardIsUp(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	v.requestDelete()
	if !v.deletion.Visible() {
		t.Fatal("the delete card should be up - the premise of this test")
	}

	v.promptExport()

	if v.exportPrompt.Visible() {
		t.Error("the export prompt must not open over a pending delete confirmation")
	}
}

// TestRequestDelete_DoesNothingWhileTheExportPromptIsUp is the mirror image:
// Shift+Delete is a shortcut too, so it can arrive mid-prompt and would
// otherwise raise the delete card underneath it.
func TestRequestDelete_DoesNothingWhileTheExportPromptIsUp(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	v.promptExport()
	if !v.exportPrompt.Visible() {
		t.Fatal("the export prompt should be up - the premise of this test")
	}

	v.requestDelete()

	if v.deletion.Visible() {
		t.Error("Shift+Delete must not raise the delete card under the export prompt")
	}
}

// TestPromptExport_ReopeningDoesNotResetAnAlreadyMadeChoice mirrors
// deletion's TestRequest_ReopeningDoesNotResetAnAlreadyMadeSelection: a
// second Cmd/Ctrl+E must not move the ring back to PNG under someone who has
// already moved it to JPEG.
func TestPromptExport_ReopeningDoesNotResetAnAlreadyMadeChoice(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	v.promptExport()
	v.exportPrompt.Select(jpegChoice)

	v.promptExport()

	if got := v.exportPrompt.Selected(); got != jpegChoice {
		t.Errorf("selection = %d after re-opening, want it left on jpegChoice (%d)", got, jpegChoice)
	}
}

// TestBuildMainMenu_ExportItemOpensThePrompt covers only that the menu
// reaches promptExport; which format each button then exports is
// TestPromptExport_EachChoiceExportsItsOwnFormat's job.
func TestBuildMainMenu_ExportItemOpensThePrompt(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))

	menu := buildMainMenu(v)
	menu.Items[0].Items[2].Action()

	if !v.exportPrompt.Visible() {
		t.Error("the Export image menu action should open the export-format prompt")
	}
}

// --- keyboard ownership ----------------------------------------------------
//
// The prompt's own selection/confirm/cancel state machine is
// widgets.ChoiceCard's job, covered in internal/ui/widgets against a fake
// repaint callback. What's here is the viewer's side, exactly the split
// delete_test.go documents for the delete confirmation: that the key
// dispatcher hands over to the prompt while it's up, and that Escape's usual
// meaning (reset the session, or close the window with nothing loaded) is
// suspended in favor of just dismissing it.

// TestHandleKeyEvent_ExportPromptSwallowsNavigationButRespondsToItsOwnKeys
// mirrors delete_test.go's
// TestHandleKeyEvent_DeleteConfirmSwallowsNavigationButRespondsToItsOwnKeys:
// while the prompt is up, every key that would otherwise navigate, zoom,
// sort, toggle merge mode, open the grid, toggle the info overlay, or open
// the EXIF panel must be swallowed - except Left/Right, which are the card's
// own selection keys (widgets.ChoiceCard.HandleKey), and Escape, which
// dismisses the prompt instead of falling through to its usual meaning.
func TestHandleKeyEvent_ExportPromptSwallowsNavigationButRespondsToItsOwnKeys(t *testing.T) {
	v, _, closed := newTestUI(t)
	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b, c)

	// Three files, opened on the middle one: from index 0, a Home that fell
	// all the way through to ShowImage(0) would land exactly where the test
	// started, so the assertion below would hold just as well with no guard
	// at all. Starting at 1 makes every one of Up/Down/Home/End a real jump.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})
	waitUntilLoaded(t, v)

	v.promptExport()
	if !v.exportPrompt.Visible() {
		t.Fatal("setup: the export prompt should be up after promptExport")
	}
	startIndex := v.state.index
	if startIndex != 1 {
		t.Fatalf("setup: index = %d, want the middle image (1) so Home and End both move", startIndex)
	}

	// Left/Right are the card's own: they move the selection ring, not the
	// image behind it.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyRight})
	if got := v.exportPrompt.Selected(); got != jpegChoice {
		t.Errorf("selection after Right = %d, want jpegChoice (%d)", got, jpegChoice)
	}
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if got := v.exportPrompt.Selected(); got != pngChoice {
		t.Errorf("selection after Left = %d, want pngChoice (%d)", got, pngChoice)
	}
	if v.state.index != startIndex {
		t.Error("Left/Right handled by the export prompt must not also navigate the image behind it")
	}

	// Up/Down/Home/End are the image view's own navigation keys - Left/Right
	// are covered separately above since the card claims those for itself.
	for _, ev := range []*fyne.KeyEvent{
		{Name: fyne.KeyUp}, {Name: fyne.KeyDown}, {Name: fyne.KeyHome}, {Name: fyne.KeyEnd},
	} {
		v.handleKeyEvent(ev)
		if v.state.index != startIndex {
			t.Errorf("%v changed the index to %d while the export prompt was up, want unchanged from %d", ev.Name, v.state.index, startIndex)
		}
	}

	// Zoom keys: 1 leaves "fit to window" for 100%, +/- step the percentage.
	fitting, percent := v.zoom.Fitting(), v.zoom.Percent()
	for _, ev := range []*fyne.KeyEvent{{Name: fyne.Key1}, {Name: fyne.KeyPlus}, {Name: fyne.KeyMinus}} {
		v.handleKeyEvent(ev)
	}
	if v.zoom.Fitting() != fitting || v.zoom.Percent() != percent {
		t.Errorf("zoom keys changed zoom state while the export prompt was up: fitting %v -> %v, percent %d -> %d",
			fitting, v.zoom.Fitting(), percent, v.zoom.Percent())
	}

	// S cycles the sort order.
	startSort := v.state.SortMode()
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyS})
	if v.state.SortMode() != startSort {
		t.Error("S (sort) should be swallowed while the export prompt is up")
	}

	// M toggles merge mode.
	startMerge := v.state.MergeMode()
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyM})
	if v.state.MergeMode() != startMerge {
		t.Error("M (merge mode) should be swallowed while the export prompt is up")
	}

	// G opens the grid.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyG})
	if v.grid.Visible() {
		t.Error("G (grid) should be swallowed while the export prompt is up")
	}

	// I toggles the persistent info overlay.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyI})
	if v.infoCard.Visible() {
		t.Error("I (info overlay) should be swallowed while the export prompt is up")
	}

	// Plain E would otherwise open the EXIF panel - only the modified
	// Cmd/Ctrl+E shortcut (which bypasses this dispatcher entirely) opens
	// the prompt itself; see handleKeyEvent's own comment.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyE})
	if v.exif.Open() {
		t.Error("plain E (EXIF panel) should be swallowed while the export prompt is up")
	}

	// Escape dismisses the prompt, not the session or the window.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if v.exportPrompt.Visible() {
		t.Error("Escape should dismiss the export prompt")
	}
	if len(v.state.files) != 3 {
		t.Error("Escape on the export prompt must not also reset the loaded file set")
	}
	if closed() {
		t.Error("Escape on the export prompt must not close the window")
	}
}

// --- Cmd/Ctrl+E / Cmd/Ctrl+Shift+E shortcuts --------------------------------

// TestWireExportShortcuts_OpensPromptAndSetsWallpaperWithoutColliding covers
// wireExportShortcuts (shortcuts.go): E isn't one of the glfw driver's
// specially-cased bare shortcuts, so both combos reach it as plain
// desktop.CustomShortcuts the way Cmd/Ctrl+S reaches wireSaveShortcut (see
// TestWireSaveShortcut_SavesTheCurrentRotation, save_test.go). Firing both
// through the same handler is what proves they don't collide: the plain
// combo must not touch the wallpaper and the Shift combo must not open the
// prompt.
func TestWireExportShortcuts_OpensPromptAndSetsWallpaperWithoutColliding(t *testing.T) {
	v := newTestViewer(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White))
	uitest.StubWallpaperSet(t, func(string) error { return nil })

	handler := &fyne.ShortcutHandler{}
	wireExportShortcuts(handler, v)

	handler.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierShortcutDefault})
	if !v.exportPrompt.Visible() {
		t.Fatal("expected Cmd/Ctrl+E to open the export prompt")
	}
	if v.wallpaper.Begun() {
		t.Error("Cmd/Ctrl+E must not also set the wallpaper")
	}
	v.exportPrompt.Hide()

	handler.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	settleWallpaper(t, v)
	if v.exportPrompt.Visible() {
		t.Error("Cmd/Ctrl+Shift+E must not open the export prompt")
	}
	if got := wallpaperFiles(t, v); len(got) != 1 {
		t.Errorf("wallpaper files = %v, want exactly one written by Cmd/Ctrl+Shift+E", got)
	}

	settleToast(t, v) // setAsWallpaper toasts on success
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
