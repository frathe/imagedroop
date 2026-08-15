package ui

import (
	"errors"
	"image/color"
	"os/exec"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/imagedrop/internal/filepicker"
	"github.com/frathe/imagedrop/internal/ui/widgets"
	"github.com/frathe/imagedrop/internal/uitest"
)

// Per-OS dispatch (zenity/PowerShell/AppKit, parseFileList, powerShellEscape)
// is covered by internal/filepicker's own tests; the tests below exercise
// the viewer's integration with it - openFileDialog/runFileChooser wiring,
// error reporting, and the tappable drop-zone widget - which can't move
// since they depend on *viewer.

// TestRunFileChooser_LoadsSelectedImage exercises runFileChooser directly
// rather than through openFileDialog: the latter runs it on its own
// goroutine (see openfiles.go, mirroring how every native chooser is a
// real, blocking subprocess call that must never block the UI goroutine),
// but a background goroutine writing v.scanDone/v.loadDone with nothing
// synchronizing that write against this test goroutine reading them would
// itself be a data race in the test, distinct from anything production code
// does wrong. Calling the handler directly keeps this test on a single
// goroutine, the same way every other handleDrop-driven test in this
// package works.
func TestRunFileChooser_LoadsSelectedImage(t *testing.T) {
	v, _, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "picked.jpg", 20, 15, color.RGBA{R: 100, A: 255})
	uitest.StubChooser(t, []byte(jpegURI.Path()+"\n"), nil)

	v.runFileChooser()
	waitForScan(t, v)
	waitUntilLoaded(t, v)

	if !v.img.Visible() || v.img.Image == nil {
		t.Fatal("expected the chooser-selected image to load")
	}
}

func TestRunFileChooser_CancelLeavesStateUntouched(t *testing.T) {
	v, _, _ := newTestUI(t)

	// Every OS-specific chooser signals a cancel as a non-zero exit.
	uitest.StubChooser(t, nil, errors.New("exit status 1"))

	v.runFileChooser()

	if v.img.Visible() {
		t.Error("no image should be shown after a cancelled dialog")
	}
	if !v.welcomeArt.Visible() {
		t.Error("welcome art should still be showing after a cancelled dialog")
	}
}

func TestChooserErrorDetail_PrefersStderr(t *testing.T) {
	// A real failing command so err is an *exec.ExitError with Stderr
	// populated by Output(), the same as a failed osascript/zenity/
	// powershell call would produce.
	_, err := exec.Command("sh", "-c", "echo boom >&2; exit 1").Output()
	if err == nil {
		t.Fatal("expected the stub shell command to fail")
	}

	if got := chooserErrorDetail(err); got != "boom" {
		t.Errorf("chooserErrorDetail(err) = %q, want %q", got, "boom")
	}
}

func TestChooserErrorDetail_FallsBackToErrorString(t *testing.T) {
	err := errors.New("some other failure")

	if got := chooserErrorDetail(err); got != "some other failure" {
		t.Errorf("chooserErrorDetail(err) = %q, want %q", got, "some other failure")
	}
}

func TestReportChooserError_TogglesToastByOS(t *testing.T) {
	tests := []struct {
		goos      string
		wantToast bool
	}{
		{"darwin", true},
		{"windows", true},
		{"linux", false},
		{"freebsd", false},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			v, _, _ := newTestUI(t)

			v.reportChooserError(errors.New("boom"), tt.goos)

			if got := v.toast.card.Visible(); got != tt.wantToast {
				t.Errorf("toastCard.Visible() = %v, want %v for GOOS=%s", got, tt.wantToast, tt.goos)
			}
			if tt.wantToast {
				settleToast(t, v)
			}
		})
	}
}

// TestOpenFileDialog_RunsChooserInBackground checks that openFileDialog
// actually reaches the native chooser on a background goroutine. The stub
// returns an error immediately, so runFileChooser takes its
// report-and-return path rather than reaching v.handleDrop; settleChooser
// then waits for that goroutine to finish, since the error path still
// renders a toast after the stub has returned.
func TestOpenFileDialog_RunsChooserInBackground(t *testing.T) {
	v, _, _ := newTestUI(t)

	called := make(chan struct{})
	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) {
		close(called)
		return nil, errors.New("stub: not exercising the success path here")
	}

	v.openFileDialog()

	select {
	case <-called:
	case <-time.After(testTimeout):
		t.Fatal("expected openFileDialog to invoke the native chooser")
	}

	settleChooser(t, v)
}

// TestOpenShortcuts_InvokeFileDialog checks that wireOpenShortcuts (build.go)
// binds Cmd/Ctrl+O and Cmd/Ctrl+Shift+O to openFileDialog. It drives a bare
// *fyne.ShortcutHandler through the real wiring function rather than a full
// window/canvas: Fyne's test driver canvas (fyne.io/fyne/v2/test) embeds
// software.WindowlessCanvas by interface, which doesn't include
// TypedShortcut, so a real key-plus-modifier press can't be simulated
// through it - only the production glfw driver's canvas exposes that
// method (see wireOpenShortcuts's own comment). A bare ShortcutHandler is
// exactly what that driver's canvas embeds to do its own dispatch, so
// firing TypedShortcut on it exercises the same lookup-by-ShortcutName path
// a real press would.
func TestOpenShortcuts_InvokeFileDialog(t *testing.T) {
	tests := []struct {
		name     string
		modifier fyne.KeyModifier
	}{
		{"CmdOrCtrl+O", fyne.KeyModifierShortcutDefault},
		{"CmdOrCtrl+Shift+O", fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, _ := newTestUI(t)

			called := make(chan struct{})
			orig := filepicker.Choose
			t.Cleanup(func() { filepicker.Choose = orig })
			filepicker.Choose = func() ([]byte, error) {
				close(called)
				return nil, errors.New("stub: not exercising the success path here")
			}

			handler := &fyne.ShortcutHandler{}
			wireOpenShortcuts(handler, v)
			handler.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: tt.modifier})

			select {
			case <-called:
			case <-time.After(2 * time.Second):
				t.Fatalf("expected the %s shortcut to invoke the native chooser", tt.name)
			}

			settleChooser(t, v)
		})
	}
}

func TestTappableArea_TappedInvokesCallback(t *testing.T) {
	called := false
	rect := canvas.NewRectangle(color.Black)
	ta := widgets.NewTappableArea(rect, func() { called = true })

	test.Tap(ta)

	if !called {
		t.Error("expected onTapped to be invoked on tap")
	}
}

func TestTappableArea_HoverInvokesOnHoverCallback(t *testing.T) {
	rect := canvas.NewRectangle(color.Black)
	ta := widgets.NewTappableArea(rect, func() {})

	var got []bool
	ta.OnHover = func(hovering bool) { got = append(got, hovering) }

	ta.MouseIn(&desktop.MouseEvent{})
	ta.MouseOut()

	if len(got) != 2 || got[0] != true || got[1] != false {
		t.Errorf("onHover calls = %v, want [true false] for MouseIn then MouseOut", got)
	}
}

func TestTappableArea_HoverIsOptional(t *testing.T) {
	rect := canvas.NewRectangle(color.Black)
	ta := widgets.NewTappableArea(rect, func() {})

	// onHover is left nil - MouseIn/MouseOut must not panic when no caller
	// has opted into hover feedback.
	ta.MouseIn(&desktop.MouseEvent{})
	ta.MouseMoved(&desktop.MouseEvent{})
	ta.MouseOut()
}

func TestE2E_TappingDropzoneArtOpensFileDialog(t *testing.T) {
	v, _, _ := newTestUI(t)

	called := make(chan struct{})
	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) {
		close(called)
		return nil, errors.New("stub: not exercising the success path here")
	}

	test.Tap(v.dropzoneArt)

	select {
	case <-called:
	case <-time.After(testTimeout):
		t.Fatal("expected tapping the dropzone art to open the file dialog")
	}

	settleChooser(t, v)
}
