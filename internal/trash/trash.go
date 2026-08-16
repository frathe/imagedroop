// Package trash moves a file to the current OS's trash/recycle bin rather
// than deleting it permanently, so Shift+Delete in the app is as
// recoverable as a delete from Finder/Explorer/a Linux file manager already
// is. Each platform needs its own approach: NSWorkspace on macOS (darwin.go,
// cgo/AppKit - an AppleScript "tell application \"Finder\"" shell-out would
// trigger a one-time Automation permission prompt this avoids entirely);
// Microsoft.VisualBasic.FileIO's recycle-bin delete via PowerShell on
// Windows; gio trash, falling back to trash-cli's trash-put, on Linux -
// both already implement the freedesktop.org trash spec correctly, rather
// than hand-rolling it here.
package trash

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// Move dispatches to the current OS's own way of moving path to the trash.
// A var so callers' tests can stub the whole platform dispatch.
var Move = func(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return moveDarwin(path)
	case "windows":
		return moveWindows(path)
	default:
		return moveLinux(path)
	}
}

// lookupGio/lookupTrashPut find their respective binaries; vars so tests
// can force either outcome deterministically instead of depending on what's
// installed on the machine running the test.
var lookupGio = func() (string, error) { return exec.LookPath("gio") }
var lookupTrashPut = func() (string, error) { return exec.LookPath("trash-put") }

// runTrashCommand runs the already-built trash command; a var so tests can
// stub the process out entirely.
var runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// moveLinux prefers gio trash - part of GLib/GVFS and preinstalled on every
// GNOME-based desktop - which implements the freedesktop.org trash spec
// correctly, including per-mount trash directories for files outside the
// home partition, falling back to trash-cli's trash-put for desktops
// without GVFS.
func moveLinux(path string) error {
	bin, err := lookupGio()
	args := []string{"trash", path}
	if err != nil {
		bin, err = lookupTrashPut()
		if err != nil {
			return errors.New("neither gio nor trash-put is installed")
		}
		args = []string{path}
	}

	_, err = runTrashCommand(exec.Command(bin, args...))
	return err
}

// moveWindows shells out to Microsoft.VisualBasic.FileIO.FileSystem's
// DeleteFile with RecycleOption.SendToRecycleBin - the same recycle-bin
// delete Explorer's own Delete key uses.
func moveWindows(path string) error {
	script := `Add-Type -AssemblyName Microsoft.VisualBasic
try {
	[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile("` + escapePowerShellPath(path) + `", 'OnlyErrorDialogs', 'SendToRecycleBin')
} catch {
	[Console]::Error.WriteLine($_.Exception.Message)
	exit 1
}`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	hideConsoleWindow(cmd)
	_, err := runTrashCommand(cmd)
	return err
}

// escapePowerShellPath escapes path for embedding inside a double-quoted
// PowerShell string literal. Windows paths can't contain a literal " (an
// illegal filename character there), so unlike filepicker's
// powerShellEscape this only needs to guard PowerShell's own
// metacharacters: ` (its escape character) and $ (variable interpolation) -
// both of which are legal in a Windows filename, unlike the app-generated
// temp paths clipboard.go/filepicker.go embed the same way.
func escapePowerShellPath(path string) string {
	path = strings.ReplaceAll(path, "`", "``")
	return strings.ReplaceAll(path, "$", "`$")
}
