package clipboard

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CopyFiles dispatches to the current OS's own way of putting a set of files
// onto the clipboard as file *references* - the thing a file manager's own
// Copy produces, so a Paste in Finder/Explorer/Nautilus creates copies of the
// files themselves. The opposite end of CopyImage, which puts decoded pixels
// there instead; the grid's batch copy wants references, since a dozen images
// can't meaningfully be one clipboard image.
//
// A var so callers' tests can stub the whole platform dispatch, exactly as
// CopyImage is. An empty list is a no-op: writing an empty selection would
// clear whatever the clipboard already held, which is not what "copy nothing"
// should mean.
var CopyFiles = func(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		return copyFilesDarwin(paths)
	case "windows":
		return copyFilesWindows(paths)
	default:
		return copyFilesLinux(paths)
	}
}

// uriList encodes paths as an RFC 2483 text/uri-list: percent-encoded file://
// URIs, CRLF-separated with a trailing CRLF. net/url does the escaping rather
// than a hand-rolled replacer, so a '#' (which would otherwise start a
// fragment) or a space survives into the receiving app intact.
func uriList(paths []string) []byte {
	var b strings.Builder
	for _, p := range paths {
		u := url.URL{Scheme: "file", Path: p}
		// strings.Builder's Write methods never return a non-nil error.
		_, _ = b.WriteString(u.String())
		_, _ = b.WriteString("\r\n")
	}

	return []byte(b.String())
}

// copyFilesLinux offers the selection as text/uri-list over the same
// xclip-then-wl-copy pair copyImageLinux uses, and over stdin the same way -
// no temp file needed.
//
// text/uri-list is the one target every file manager reads. Some also define
// a private target of their own that carries whether the drop was a copy or a
// cut (Nautilus's x-special/gnome-copied-files, for one); this deliberately
// publishes only the portable target, so a paste is always a copy.
func copyFilesLinux(paths []string) error {
	path, err := lookupXClip()
	args := []string{"-selection", "clipboard", "-t", "text/uri-list"}
	if err != nil {
		path, err = lookupWlCopy()
		if err != nil {
			return errors.New("neither xclip nor wl-copy is installed")
		}
		args = []string{"--type", "text/uri-list"}
	}

	cmd := exec.Command(path, args...)
	cmd.Stdin = bytes.NewReader(uriList(paths))
	_, err = runClipboardCommand(cmd)

	return err
}

// writeTempList writes paths one per line to a temp file, for
// copyFilesWindows to read back - the file-list twin of writeTempPNG.
func writeTempList(paths []string) (string, error) {
	f, err := os.CreateTemp("", "picfetch_clip_*.txt")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, p := range paths {
		// strings.Builder's Write methods never return a non-nil error.
		_, _ = b.WriteString(p)
		_, _ = b.WriteString("\r\n")
	}

	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())

		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())

		return "", err
	}

	return f.Name(), nil
}

// copyFilesWindows shells out to PowerShell's Set-Clipboard -LiteralPath,
// which puts a CF_HDROP file-drop list on the clipboard - the same thing
// Explorer's own Copy produces, and the reason copyImageWindows deliberately
// avoids it for image data.
//
// The paths reach PowerShell through a temp file named in the environment
// rather than interpolated into the script, for the reason copyImageWindows
// documents for its single path: a Windows file name may legally contain '$'
// or a backtick, both of which have meaning inside a PowerShell string. -STA
// and the try/catch are the same two fixes copyImageWindows carries - a
// clipboard write needs an STA thread, and an uncaught .NET exception would
// otherwise exit 0 and look like success.
func copyFilesWindows(paths []string) error {
	list, err := writeTempList(paths)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(list) }()

	const listEnv = "PICFETCH_CLIPBOARD_LIST"
	script := `try {
	$paths = Get-Content -LiteralPath $env:` + listEnv + `
	Set-Clipboard -LiteralPath $paths
} catch {
	[Console]::Error.WriteLine($_.Exception.Message)
	exit 1
}`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), listEnv+"="+list)
	hideConsoleWindow(cmd)
	_, err = runClipboardCommand(cmd)

	return err
}
