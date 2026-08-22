package ui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/frathe/picfetch/internal/uitest"
)

// Per-platform shell-out behavior (osascript/xclip/wl-copy/PowerShell
// dispatch, writeTempPNG) is covered by internal/clipboard's own tests; the
// tests below exercise the viewer's integration with it - encoding the
// current frame, dispatching on its own goroutine, and reporting failures -
// which can't move since they depend on *viewer.

func TestCopyPathToClipboard_SetsFilePath(t *testing.T) {
	v, _, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "picked.jpg", 4, 4, color.RGBA{R: 100, A: 255})
	v.state.files = []fyne.URI{jpegURI}
	v.state.index = 0

	v.copyPathToClipboard()

	if got := v.app.Clipboard().Content(); got != jpegURI.Path() {
		t.Errorf("clipboard content = %q, want %q", got, jpegURI.Path())
	}
}

func TestCopyPathToClipboard_NoFilesIsNoop(t *testing.T) {
	v, _, _ := newTestUI(t)

	v.app.Clipboard().SetContent("untouched")
	v.copyPathToClipboard()

	if got := v.app.Clipboard().Content(); got != "untouched" {
		t.Errorf("clipboard content = %q, want it left untouched", got)
	}
}

func TestCopyImageToClipboard_NoImageIsNoop(t *testing.T) {
	v, _, _ := newTestUI(t)

	called := false
	uitest.StubClipboardCopy(t, func([]byte) error { called = true; return nil })

	v.copyImageToClipboard()

	if called {
		t.Error("clipboard.CopyImage should not run with no image loaded")
	}
}

func TestCopyImageToClipboard_DispatchesEncodedPNG(t *testing.T) {
	v, _, _ := newTestUI(t)

	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.Set(1, 1, color.RGBA{R: 200, G: 10, B: 10, A: 255})
	v.img.Image = src

	received := make(chan []byte, 1)
	uitest.StubClipboardCopy(t, func(data []byte) error {
		received <- data
		return nil
	})

	v.copyImageToClipboard()

	select {
	case data := <-received:
		decoded, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("png.Decode() error = %v, want the encoded frame to decode cleanly", err)
		}
		if decoded.Bounds() != src.Bounds() {
			t.Errorf("decoded bounds = %v, want %v", decoded.Bounds(), src.Bounds())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected clipboard.CopyImage to be invoked")
	}
}

func TestCopyImageToClipboard_DispatchFailureShowsToast(t *testing.T) {
	v, _, _ := newTestUI(t)

	v.img.Image = image.NewRGBA(image.Rect(0, 0, 2, 2))
	uitest.StubClipboardCopy(t, func([]byte) error { return errors.New("boom") })

	v.copyImageToClipboard()

	// copyImageToClipboard reports failures from a background goroutine
	// via fyne.Do; waiting on v.clipboard (finished after that goroutine
	// has fully run, error toast included) is what makes reading the
	// toast widgets afterward race-free - polling them would read state
	// the goroutine may still be writing.
	waitFor(t, "the clipboard copy", &v.clipboard)

	if !v.toast.card.Visible() {
		t.Error("expected a toast for a clipboard copy failure")
	}
	if !strings.Contains(v.toast.text.Text, "boom") {
		t.Errorf("toast text = %q, want it to mention the underlying error", v.toast.text.Text)
	}
	settleToast(t, v)
}

func TestReportClipboardError_ShowsToast(t *testing.T) {
	v, _, _ := newTestUI(t)

	v.reportClipboardError(errors.New("boom"))

	if !v.toast.card.Visible() {
		t.Error("expected a toast for a clipboard copy failure")
	}
	if !strings.Contains(v.toast.text.Text, "boom") {
		t.Errorf("toast text = %q, want it to mention the underlying error", v.toast.text.Text)
	}
	settleToast(t, v)
}

func TestWireClipboardShortcuts_CopiesImageAndPath(t *testing.T) {
	v, _, _ := newTestUI(t)

	jpegURI := uitest.TempJPEGURI(t, "picked.jpg", 4, 4, color.RGBA{R: 100, A: 255})
	v.state.files = []fyne.URI{jpegURI}
	v.state.index = 0
	v.img.Image = image.NewRGBA(image.Rect(0, 0, 2, 2))

	called := make(chan struct{}, 1)
	uitest.StubClipboardCopy(t, func([]byte) error {
		select {
		case called <- struct{}{}:
		default:
		}
		return nil
	})

	handler := &fyne.ShortcutHandler{}
	wireClipboardShortcuts(handler, v)

	// A plain Cmd/Ctrl+C press never reaches TypedShortcut as a
	// desktop.CustomShortcut - the real glfw driver special-cases it into
	// this built-in type first (see wireClipboardShortcuts's own comment).
	// Firing a CustomShortcut here instead would pass even if
	// wireClipboardShortcuts bound the wrong shortcut type, the way it
	// originally did.
	handler.TypedShortcut(&fyne.ShortcutCopy{})
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("expected Cmd/Ctrl+C to copy the image")
	}

	handler.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if got := v.app.Clipboard().Content(); got != jpegURI.Path() {
		t.Errorf("clipboard content = %q, want %q after Cmd/Ctrl+Shift+C", got, jpegURI.Path())
	}
}
