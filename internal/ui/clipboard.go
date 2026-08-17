package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/clipboard"
)

// copyPathToClipboard puts the current file's absolute path on the system
// text clipboard. No shell-out needed here, unlike copyImageToClipboard
// below - fyne.Clipboard already handles text on every platform.
func (v *viewer) copyPathToClipboard() {
	if len(v.files) == 0 {
		return
	}
	v.app.Clipboard().SetContent(v.files[v.index].Path())
}

// copyImageToClipboard puts the currently displayed frame onto the system
// clipboard as real image data, via internal/clipboard's per-OS shell-out -
// the same kind openfiles.go already established for the file/folder
// dialog. Always runs on its own goroutine, mirroring openFileDialog: every
// backing command blocks on external I/O.
func (v *viewer) copyImageToClipboard() {
	img := v.img.Image
	if img == nil {
		return
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		fyne.LogError("failed to encode current image for clipboard copy", err)
		return
	}
	data := buf.Bytes()

	// clipboardDone mirrors scanDone/loadDone: closed once this copy's
	// goroutine has fully finished, error reporting included, so a test
	// can wait for the whole operation instead of polling widget state
	// the goroutine may still be writing.
	done := make(chan struct{})
	v.clipboardDone = done

	go func() {
		defer close(done)

		if err := clipboard.CopyImage(data); err != nil {
			v.reportClipboardError(err)
		}
	}()
}

// reportClipboardError always logs a clipboard-copy failure and shows it as
// a toast, on every platform - unlike reportChooserError in openfiles.go,
// which stays log-only on Linux because zenity signals a plain user cancel
// and a real failure with the same exit code. There's no such ambiguity
// here: an image-clipboard copy either succeeds or genuinely failed, on
// every OS, so it's always worth surfacing.
func (v *viewer) reportClipboardError(err error) {
	detail := chooserErrorDetail(err)
	fyne.LogError("clipboard image copy failed", errors.New(detail))

	fyne.Do(func() {
		v.ShowToast(fmt.Sprintf(lang.L("could not copy the image: %v"), detail))
	})
}
