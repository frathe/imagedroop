package uitest

import (
	"testing"

	"github.com/frathe/imagedrop/internal/clipboard"
	"github.com/frathe/imagedrop/internal/filepicker"
	"github.com/frathe/imagedrop/internal/trash"
)

// The stubs below swap the exported dispatcher vars that stand in front of
// this app's OS-level shell-outs (a native file dialog, a clipboard copy, a
// trash move). Those vars exist precisely so tests never actually open a
// dialog, touch the system clipboard, or move a file to the real Trash;
// each restores the real implementation on cleanup, so a test that stubs
// one can't affect the ones that follow.

// StubChooser makes filepicker.Choose return out/err instead of opening the
// OS file browser.
func StubChooser(t *testing.T, out []byte, err error) {
	t.Helper()

	orig := filepicker.Choose
	t.Cleanup(func() { filepicker.Choose = orig })
	filepicker.Choose = func() ([]byte, error) { return out, err }
}

// StubSaveChooser makes filepicker.ChooseSave call fn instead of opening
// the OS save panel. It takes a function rather than a fixed result the way
// StubChooser does, since a caller usually wants to assert on the suggested
// path it was offered as well as control what comes back.
func StubSaveChooser(t *testing.T, fn func(suggestedPath string) ([]byte, error)) {
	t.Helper()

	orig := filepicker.ChooseSave
	t.Cleanup(func() { filepicker.ChooseSave = orig })
	filepicker.ChooseSave = fn
}

// StubClipboardCopy makes clipboard.CopyImage call fn instead of shelling
// out to the OS clipboard.
func StubClipboardCopy(t *testing.T, fn func(data []byte) error) {
	t.Helper()

	orig := clipboard.CopyImage
	t.Cleanup(func() { clipboard.CopyImage = orig })
	clipboard.CopyImage = fn
}

// StubClipboardCopyFiles makes clipboard.CopyFiles call fn instead of
// shelling out to the OS clipboard - the file-reference twin of
// StubClipboardCopy, for the grid's batch copy.
func StubClipboardCopyFiles(t *testing.T, fn func(paths []string) error) {
	t.Helper()

	orig := clipboard.CopyFiles
	t.Cleanup(func() { clipboard.CopyFiles = orig })
	clipboard.CopyFiles = fn
}

// StubTrashMove makes trash.Move call fn instead of shelling out to the
// OS's real trash/recycle-bin mover.
func StubTrashMove(t *testing.T, fn func(path string) error) {
	t.Helper()

	orig := trash.Move
	t.Cleanup(func() { trash.Move = orig })
	trash.Move = fn
}
