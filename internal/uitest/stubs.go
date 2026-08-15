package uitest

import (
	"testing"

	"github.com/frathe/imagedrop/internal/clipboard"
	"github.com/frathe/imagedrop/internal/filepicker"
)

// The two stubs below swap the exported dispatcher vars that stand in front
// of this app's OS-level shell-outs (a native file dialog, a clipboard
// copy). Those vars exist precisely so tests never actually open a dialog
// or touch the system clipboard; both restore the real implementation on
// cleanup, so a test that stubs them can't affect the ones that follow.

// StubChooser makes filepicker.Choose return out/err instead of opening the
// OS file browser.
func StubChooser(t *testing.T, out []byte, err error) {
	t.Helper()

	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) { return out, err }
}

// StubClipboardCopy makes clipboard.CopyImage call fn instead of shelling
// out to the OS clipboard.
func StubClipboardCopy(t *testing.T, fn func(data []byte) error) {
	t.Helper()

	orig := clipboard.CopyImage
	t.Cleanup(func() { clipboard.CopyImage = orig })
	clipboard.CopyImage = fn
}
