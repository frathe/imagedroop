// The File menu (menu.go): its structure and each item's wiring, plus
// closeFiles - the viewer.go action its "Close Files" item runs.

package ui

import (
	"errors"
	"image/color"
	"testing"
	"time"

	"github.com/frathe/imagedrop/internal/filepicker"
	"github.com/frathe/imagedrop/internal/uitest"
)

// TestBuildMainMenu_Structure checks the bar's shape: File (Open Files…,
// Save Changes, Export as PNG…, Export as JPEG…, Set as Wallpaper, Close
// Files, a separator, Settings…) followed by view.help's own Help menu -
// mirroring help's own
// TestHelpMenu (manual_test.go), which covers the Help submenu's own
// contents.
func TestBuildMainMenu_Structure(t *testing.T) {
	v := newTestViewer(t)

	menu := buildMainMenu(v)

	if len(menu.Items) != 2 {
		t.Fatalf("top-level menus = %d, want 2 (File, Help)", len(menu.Items))
	}

	file := menu.Items[0]
	if file.Label != "File" {
		t.Errorf("first menu label = %q, want %q", file.Label, "File")
	}
	if len(file.Items) != 8 {
		t.Fatalf("File menu items = %d, want 8 (Open Files…, Save Changes, Export as PNG…, Export as JPEG…, Set as Wallpaper, Close Files, separator, Settings…)", len(file.Items))
	}

	if got := file.Items[0]; got.Label != "Open Files…" || got.Action == nil {
		t.Errorf("File menu item 0 = %+v, want %q with an action", got, "Open Files…")
	}
	if got := file.Items[1]; got.Label != "Save Changes" || got.Action == nil || !got.Disabled {
		t.Errorf("File menu item 1 = %+v, want %q with an action, starting disabled", got, "Save Changes")
	}
	if got := file.Items[2]; got.Label != "Export as PNG…" || got.Action == nil || !got.Disabled {
		t.Errorf("File menu item 2 = %+v, want %q with an action, starting disabled", got, "Export as PNG…")
	}
	if got := file.Items[3]; got.Label != "Export as JPEG…" || got.Action == nil || !got.Disabled {
		t.Errorf("File menu item 3 = %+v, want %q with an action, starting disabled", got, "Export as JPEG…")
	}
	if got := file.Items[4]; got.Label != "Set as Wallpaper" || got.Action == nil || !got.Disabled {
		t.Errorf("File menu item 4 = %+v, want %q with an action, starting disabled", got, "Set as Wallpaper")
	}
	if got := file.Items[5]; got.Label != "Close Files" || got.Action == nil {
		t.Errorf("File menu item 5 = %+v, want %q with an action", got, "Close Files")
	}
	if !file.Items[6].IsSeparator {
		t.Error("expected a separator between Close Files and Settings…")
	}
	if got := file.Items[7]; got.Label != "Settings…" || got.Action == nil {
		t.Errorf("File menu item 7 = %+v, want %q with an action", got, "Settings…")
	}

	if got := menu.Items[1]; got.Label != "Help" {
		t.Errorf("second menu label = %q, want %q", got.Label, "Help")
	}
}

// TestBuildMainMenu_OpenFilesItemInvokesTheNativeChooser mirrors
// TestOpenFileDialog_RunsChooserInBackground (openfiles_test.go): the menu
// item must reach the same openFileDialog/runFileChooser path Cmd/Ctrl+O
// and the drop-zone tap already do.
func TestBuildMainMenu_OpenFilesItemInvokesTheNativeChooser(t *testing.T) {
	v := newTestViewer(t)
	menu := buildMainMenu(v)

	called := make(chan struct{})
	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) {
		close(called)
		return nil, errors.New("stub: not exercising the success path here")
	}

	menu.Items[0].Items[0].Action()

	select {
	case <-called:
	case <-time.After(testTimeout):
		t.Fatal("expected the Open Files… action to invoke the native chooser")
	}

	settleChooser(t, v)
}

func TestBuildMainMenu_CloseFilesItemResetsToWelcomeState(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	menu := buildMainMenu(v)
	menu.Items[0].Items[5].Action()

	if v.files != nil {
		t.Errorf("files = %v, want nil after the Close Files action", v.files)
	}
	if !v.welcomeArt.Visible() {
		t.Error("expected the welcome drop zone back after the Close Files action")
	}
}

func TestBuildMainMenu_SettingsItemOpensTheSettingsWindow(t *testing.T) {
	v := newTestViewer(t)
	menu := buildMainMenu(v)

	if v.settings.Open() {
		t.Fatal("settings window should not be open yet")
	}

	menu.Items[0].Items[7].Action()

	if !v.settings.Open() {
		t.Error("the Settings… action should open the settings window")
	}
}

// --- closeFiles ----------------------------------------------------------

func TestCloseFiles_ResetsLoadedFilesToWelcomeState(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.closeFiles()

	if v.files != nil {
		t.Errorf("files = %v, want nil after closeFiles", v.files)
	}
	if !v.welcomeArt.Visible() || !v.dropzone.Visible() {
		t.Error("expected the welcome drop zone back after closeFiles")
	}
}

// TestCloseFiles_NeverClosesTheWindow is the one behavior that sets
// closeFiles apart from Escape's own reset branch (handleKeyEvent): with
// nothing loaded, Escape closes the window, but File > Close Files must
// not - it is a distinct action from quitting the app.
func TestCloseFiles_NeverClosesTheWindow(t *testing.T) {
	v, _, closed := newTestUI(t)

	v.closeFiles()

	if closed() {
		t.Error("closeFiles must never close the window, unlike Escape with nothing loaded")
	}
}

// TestCloseFiles_CancelsScanInProgress mirrors
// TestCancelScan_CancelsInFlightScanWithNoFilesYet (library_test.go): it
// drives cancelScan's target state directly rather than racing handleDrop's
// own background goroutine.
func TestCloseFiles_CancelsScanInProgress(t *testing.T) {
	v := newTestViewer(t)

	v.gen.Add(1)
	v.scanning = true
	v.scanSpinner.Show()
	v.scanLabel.Show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()

	v.closeFiles()

	if v.scanning {
		t.Error("closeFiles should cancel a scan in progress")
	}
	if v.scanSpinner.Visible() || v.scanLabel.Visible() {
		t.Error("scan spinner/label should be hidden after closeFiles cancels a scan")
	}
	if !v.dropzone.Visible() || !v.welcomeArt.Visible() {
		t.Error("expected the welcome drop zone back after closeFiles cancels a scan")
	}

	settleToast(t, v) // cancelScan raises a "cancelled scanning" toast
}
