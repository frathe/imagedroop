// Package filepicker opens the current OS's own file/folder browser and
// returns the paths the user picked. Fyne's built-in dialog is never used
// here: it can select neither a folder nor more than one file, so it can't
// stand in for any of the three OS-specific choosers this package
// dispatches to (Linux and Windows below; darwin is cgo/AppKit and lives in
// darwin.go).
package filepicker

import (
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"
)

// Choose produces the raw, newline-separated path list the current OS's own
// file browser returned. A var so callers' tests can stub the whole
// platform dispatch without needing any of these tools - or a display - to
// actually be present.
var Choose = func() ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return chooseFilesDarwin()
	case "windows":
		return chooseFilesWindows()
	default:
		return chooseFilesLinux()
	}
}

// lookupZenity finds the zenity binary; a var so tests can force either
// outcome of chooseFilesLinux deterministically instead of depending on
// whether the machine running the test happens to have zenity installed.
var lookupZenity = exec.LookPath

// runZenityCommand runs the already-built zenity command and returns its
// stdout; a var so tests can stub the process out entirely.
var runZenityCommand = func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// chooseFilesLinux shells out to zenity, the closest thing Linux desktops
// have to one standard native file dialog. GTK's file chooser (the widget
// zenity itself wraps) lets a user select folders as well as files while in
// multi-select mode - highlighting a folder selects it instead of
// navigating into it - so one dialog covers both without a mode switch. If
// zenity isn't installed, the caller treats the returned error the same as
// a cancel - see the viewer's runFileChooser.
func chooseFilesLinux() ([]byte, error) {
	path, err := lookupZenity("zenity")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(path, "--file-selection", "--multiple",
		"--separator=\n", "--title="+lang.L("Open images"))
	return runZenityCommand(cmd)
}

// chooseFilesWindows shells out to a WinForms OpenFileDialog via
// PowerShell - the closest thing to a native Fyne-dialog replacement on
// Windows. Files only: Windows' own shell dialogs have no mode that
// combines folder selection with multi-file selection (IFileOpenDialog's
// FOS_PICKFOLDERS flag switches the whole dialog to folders-only, mutually
// exclusive with picking files; highlighted folders are silently dropped
// from an OpenFileDialog result). This used to fake folder support with a
// sentinel-filename trick - pick a localized "Select this folder." name to
// mean "the folder I'm currently in" - but that was confusing enough in
// practice to be worse than not having it, so it was removed; Ctrl+O now
// only ever returns files here. Folders are still fully supported, just via
// drag-and-drop instead (handleDrop's recursive expansion, same as every
// other platform).
func chooseFilesWindows() ([]byte, error) {
	return buildPowerShellCmd().Output()
}

func buildPowerShellCmd() *exec.Cmd {
	script := `Add-Type -AssemblyName System.Windows.Forms
$dlg = New-Object System.Windows.Forms.OpenFileDialog
$dlg.Multiselect = $true
$dlg.Title = "` + powerShellEscape(lang.L("Open images")) + `"
if ($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	$dlg.FileNames | ForEach-Object { Write-Output $_ }
}`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	hideConsoleWindow(cmd)
	return cmd
}

// powerShellEscape escapes s for embedding inside a double-quoted
// PowerShell string literal.
func powerShellEscape(s string) string {
	s = strings.ReplaceAll(s, "`", "``")
	return strings.ReplaceAll(s, `"`, "`\"")
}

// ParseFileList splits a native file chooser's newline-separated stdout
// into file URIs. Shared by all three OS-specific choosers, since each is
// built to emit one absolute path per line. A trailing separator, blank
// lines, and a trailing \r per line (PowerShell's Write-Output uses CRLF
// line endings on Windows) are all tolerated.
func ParseFileList(out []byte) []fyne.URI {
	trimmed := strings.TrimRight(string(out), "\r\n")
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	uris := make([]fyne.URI, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		uris = append(uris, storage.NewFileURI(line))
	}
	return uris
}
