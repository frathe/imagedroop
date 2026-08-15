package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/imagedrop/internal/filepicker"
)

// openFileDialog opens the current OS's own file browser so users who never
// drag-and-drop can still load images - see internal/filepicker for the
// per-OS dispatch. It always runs on its own goroutine since every backing
// command blocks until the user closes the dialog.
func (v *viewer) openFileDialog() {
	// chooserDone mirrors clipboardDone/scanDone: closed once this pick's
	// goroutine has fully finished, error toast included, so a test can
	// wait for it rather than leave it running into the next one. That
	// matters more than it looks: reportChooserError renders a toast, and
	// under the fyne test driver this goroutine's fyne.Do runs inline
	// here rather than on the UI goroutine, so an unwaited-for failure
	// path measures text concurrently with whatever the test goroutine is
	// laying out - which races inside Fyne's own global font-metrics
	// cache (internal/cache's setAlive writes an expiry stamp unguarded).
	done := make(chan struct{})
	v.chooserDone = done

	go func() {
		defer close(done)

		v.runFileChooser()
	}()
}

// runFileChooser is split out from openFileDialog so tests can call it
// directly on the test goroutine - avoiding a data race on
// v.scanDone/v.loadDone, which handleDrop below would otherwise write from
// a background goroutine with nothing synchronizing that write against a
// test reading them, the same hazard documented on the zenity-specific
// tests this replaced. Production always reaches it through the goroutine
// in openFileDialog above.
func (v *viewer) runFileChooser() {
	out, err := filepicker.Choose()
	if err != nil {
		v.reportChooserError(err, runtime.GOOS)
		return
	}

	uris := filepicker.ParseFileList(out)
	if len(uris) == 0 {
		return
	}

	fyne.Do(func() {
		v.handleDrop(uris)
	})
}

// reportChooserError always logs a chooser failure - via fyne.LogError, so
// it's visible in a terminal or Console.app even though nothing else prints
// anything here - and, on macOS and Windows only, also shows it as a toast.
// Those two platforms' own choosers already swallow a plain user cancel
// internally (see internal/filepicker's darwin runModal-response check and
// Windows ShowDialog-result check), so any error reaching here on them is a
// genuine failure worth surfacing, not a normal cancel. zenity signals a
// plain cancel and a real failure with the same non-zero exit code,
// indistinguishable from here, so Linux stays log-only to avoid a toast on
// every ordinary cancel. goos is threaded through as a parameter (rather
// than reading runtime.GOOS directly in here) purely so tests can exercise
// all three branches from a single machine.
func (v *viewer) reportChooserError(err error, goos string) {
	detail := chooserErrorDetail(err)
	fyne.LogError("file chooser failed", errors.New(detail))

	if goos != "darwin" && goos != "windows" {
		return
	}

	fyne.Do(func() {
		v.ShowToast(fmt.Sprintf(lang.L("could not open the file browser: %v"), detail))
	})
}

// chooserErrorDetail pulls the most useful message out of err: an
// *exec.ExitError's own Error() is just "exit status N", but Output()
// populates its Stderr field with whatever the failed command printed,
// which is almost always the actually useful part (an AppleScript error
// message, a missing-binary complaint, and so on). Shared with
// reportClipboardError in clipboard.go, the same kind of chooser-adjacent
// shell-out failure.
func chooserErrorDetail(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
			return msg
		}
	}
	return err.Error()
}
