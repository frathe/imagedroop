// Package clipboard puts image data onto the system clipboard as a real
// image - not a file reference - so it can be pasted into another app
// (Slack, an image editor, ...) as an image, the way a browser's own "Copy
// Image" works. fyne.Clipboard is text-only, so each platform needs its own
// shell-out: AppleScript on macOS (pbcopy has no image support at all);
// xclip/wl-copy on Linux; PowerShell's System.Windows.Forms.Clipboard on
// Windows.
package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// CopyImage dispatches to the current OS's own way of putting PNG-encoded
// image data onto the clipboard. A var so callers' tests can stub the whole
// platform dispatch.
var CopyImage = func(data []byte) error {
	switch runtime.GOOS {
	case "darwin":
		return copyImageDarwin(data)
	case "windows":
		return copyImageWindows(data)
	default:
		return copyImageLinux(data)
	}
}

// writeTempPNG writes data to a temp file for the shell-outs below that need
// a real path - osascript's "read ... as «class PNGf»" and PowerShell's
// Image.FromFile both require one, neither accepts piped bytes.
func writeTempPNG(data []byte) (string, error) {
	f, err := os.CreateTemp("", "image_drop_clip_*.png")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		err := os.Remove(f.Name())
		if err != nil {
			return "", err
		}
		return "", err
	}
	if err := f.Close(); err != nil {
		err := os.Remove(f.Name())
		if err != nil {
			return "", err
		}
		return "", err
	}
	return f.Name(), nil
}

// runClipboardCommand runs the already-built clipboard command; a var so
// tests can stub the process out entirely.
var runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// copyImageDarwin shells out to osascript: pbcopy is text-only, but
// AppleScript's "read ... as «class PNGf»" reads a PNG file straight onto
// the clipboard as an image.
func copyImageDarwin(data []byte) error {
	path, err := writeTempPNG(data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()

	script := fmt.Sprintf(`set the clipboard to (read (POSIX file %q) as «class PNGf»)`, path)
	_, err = runClipboardCommand(exec.Command("osascript", "-e", script))
	return err
}

// lookupXClip/lookupWlCopy find their respective binaries; vars so tests can
// force either outcome deterministically instead of depending on what's
// installed on the machine running the test.
var lookupXClip = func() (string, error) { return exec.LookPath("xclip") }
var lookupWlCopy = func() (string, error) { return exec.LookPath("wl-copy") }

// copyImageLinux prefers xclip, the more widely available of the two (X11
// and XWayland both work with it), falling back to wl-copy for a
// Wayland-native session with no XWayland. Unlike the macOS/Windows shell-
// outs, both take the PNG straight over stdin - no temp file needed.
func copyImageLinux(data []byte) error {
	path, err := lookupXClip()
	args := []string{"-selection", "clipboard", "-t", "image/png"}
	if err != nil {
		path, err = lookupWlCopy()
		if err != nil {
			return errors.New("neither xclip nor wl-copy is installed")
		}
		args = []string{"--type", "image/png"}
	}

	cmd := exec.Command(path, args...)
	cmd.Stdin = bytes.NewReader(data)
	_, err = runClipboardCommand(cmd)
	return err
}

// copyImageWindows shells out to PowerShell's
// System.Windows.Forms.Clipboard - the same API Explorer's own "Copy" uses
// on an image file - rather than Set-Clipboard -Path, which would copy a
// file reference instead of decoded image data.
//
// Two things beyond a plain SetImage call, both needed to fix a Windows bug
// report where Ctrl+C silently left the clipboard untouched (a paste kept
// showing whatever was copied there before) with nothing to explain why:
//
//   - SetDataObject($img, $true, 10, 100), not SetImage: SetImage opens the
//     clipboard exactly once, and Windows 10/11's own Clipboard History
//     (Win+V) routinely holds it open for a moment after every copy
//     elsewhere on the system, which fails that single attempt with
//     CLIPBRD_E_CANT_OPEN. SetDataObject's trailing (retries, msDelay)
//     overload retries automatically instead.
//   - -Command failures don't reach us as a Go error by default: an
//     uncaught .NET exception here prints to PowerShell's error stream but
//     exits 0 unless the script says otherwise, so the shell-out looks
//     like it succeeded either way. The try/catch below writes the real
//     .NET exception message to stderr and exits 1 on failure, so a
//     genuine failure now surfaces as the toast reportClipboardError shows,
//     instead of a silent no-op.
func copyImageWindows(data []byte) error {
	path, err := writeTempPNG(data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()

	// Pass the path through the environment instead of interpolating it into
	// PowerShell source. A Windows temp directory may legally contain `$` or
	// a backtick, both of which have meaning inside a double-quoted script.
	const pathEnv = "IMAGEDROP_CLIPBOARD_PNG"
	script := `Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
try {
	$img = [System.Drawing.Image]::FromFile($env:` + pathEnv + `)
	[System.Windows.Forms.Clipboard]::SetDataObject($img, $true, 10, 100)
	$img.Dispose()
} catch {
	[Console]::Error.WriteLine($_.Exception.Message)
	exit 1
}`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), pathEnv+"="+path)
	hideConsoleWindow(cmd)
	_, err = runClipboardCommand(cmd)
	return err
}
