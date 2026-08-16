// Package wallpaper sets the current OS's desktop wallpaper to an image
// file, for the File > "Set as Wallpaper" action. Each platform needs its
// own approach: NSWorkspace on macOS (darwin.go, cgo/AppKit - not an
// AppleScript "tell application \"System Events\"" shell-out, which would
// trigger the one-time Automation permission prompt a direct framework call
// avoids entirely, the same reasoning internal/trash's darwin.go is built
// on); SystemParametersInfo via PowerShell on Windows; gsettings, or
// plasma-apply-wallpaperimage on KDE, on Linux.
//
// The path handed to Set must stay readable for as long as it is the
// wallpaper: every platform here stores a reference to the file rather than
// a copy of its pixels. internal/ui/wallpaper.go is what guarantees that, by
// writing its own PNG into the user's cache directory rather than pointing
// any of this at a file the user can move or delete from the app itself.
package wallpaper

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Set dispatches to the current OS's own way of making path the desktop
// wallpaper. A var so callers' tests can stub the whole platform dispatch.
var Set = func(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return setDarwin(path)
	case "windows":
		return setWindows(path)
	default:
		return setLinux(path)
	}
}

// lookupGsettings/lookupPlasmaApply find their respective binaries; vars so
// tests can force either outcome deterministically instead of depending on
// what's installed on the machine running the test.
var lookupGsettings = func() (string, error) { return exec.LookPath("gsettings") }
var lookupPlasmaApply = func() (string, error) { return exec.LookPath("plasma-apply-wallpaperimage") }

// runWallpaperCommand runs the already-built command; a var so tests can
// stub the process out entirely.
var runWallpaperCommand = func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// gnomeBackgroundSchema is the GSettings schema GNOME and the desktops built
// on its session (Cinnamon, Budgie, Unity) read the wallpaper from.
const gnomeBackgroundSchema = "org.gnome.desktop.background"

// setLinux prefers plasma-apply-wallpaperimage on KDE and gsettings
// everywhere else. The desktop check comes first because it is the one case
// where the fallback would silently do nothing: glib - and so gsettings - is
// installed on most KDE systems, so a lookup-order fallback would find it,
// write the GNOME schema successfully, and leave the user's actual desktop
// unchanged while the app reported success.
func setLinux(path string) error {
	if isKDE() {
		if bin, err := lookupPlasmaApply(); err == nil {
			_, err := runWallpaperCommand(exec.Command(bin, path))
			return err
		}
		// Plasma before 5.24 ships no plasma-apply-wallpaperimage. gsettings
		// is a long shot there, but it beats refusing outright.
	}

	bin, err := lookupGsettings()
	if err != nil {
		return errors.New("neither gsettings nor plasma-apply-wallpaperimage is installed")
	}

	uri := fileURI(path)
	if _, err := runWallpaperCommand(exec.Command(bin, "set", gnomeBackgroundSchema, "picture-uri", uri)); err != nil {
		return err
	}

	// picture-uri-dark is GNOME 42 and later, where the background is a
	// light/dark pair: writing only the light key leaves a user in dark mode
	// looking at an unchanged desktop. Its failure is ignored rather than
	// reported, because the one thing that makes it fail is running on an
	// older GNOME that has no such key - where the write above was already
	// the whole job.
	_, _ = runWallpaperCommand(exec.Command(bin, "set", gnomeBackgroundSchema, "picture-uri-dark", uri))

	return nil
}

// isKDE reports whether this is a KDE Plasma session. XDG_CURRENT_DESKTOP is
// colon-separated and may name several desktops ("KDE", but also
// "KDE:Plasma"), so this is a substring test rather than an equality one.
func isKDE() bool {
	return strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE")
}

// fileURI converts a filesystem path to the file:// URI gsettings stores.
// Built through net/url rather than by concatenation so the characters that
// are legal in a file name but structural in a URI - '#', which would
// otherwise start a fragment and truncate the path, '?', and spaces - are
// percent-encoded.
func fileURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

// setWindows shells out to SystemParametersInfo, the Win32 call the
// Personalization settings page itself ends up making. PowerShell has no
// wallpaper cmdlet, so this goes through Add-Type: a P/Invoke declaration
// compiled on the fly, which is the standard way to reach a user32 entry
// point from a script. spiSetDeskWallpaper (20) is the action;
// SPIF_UPDATEINIFILE|SPIF_SENDWININICHANGE (3) is what makes the change
// persist across a reboot and reach the running shell, rather than living
// only in this session's memory.
func setWindows(path string) error {
	script := `Add-Type @"
using System;
using System.Runtime.InteropServices;
public class ImageDropWallpaper {
	[DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
	public static extern int SystemParametersInfo(int uAction, int uParam, string lpvParam, int fuWinIni);
}
"@
if ([ImageDropWallpaper]::SystemParametersInfo(20, 0, "` + escapePowerShellPath(path) + `", 3) -eq 0) {
	[Console]::Error.WriteLine("SystemParametersInfo could not set the wallpaper")
	exit 1
}`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	hideConsoleWindow(cmd)
	_, err := runWallpaperCommand(cmd)
	return err
}

// escapePowerShellPath escapes path for embedding inside a double-quoted
// PowerShell string literal, exactly as internal/trash's own copy does:
// Windows paths can't contain a literal " (an illegal filename character
// there), so only PowerShell's own metacharacters need guarding - ` (its
// escape character) and $ (variable interpolation), both legal in a Windows
// filename.
func escapePowerShellPath(path string) string {
	path = strings.ReplaceAll(path, "`", "``")
	return strings.ReplaceAll(path, "$", "`$")
}
