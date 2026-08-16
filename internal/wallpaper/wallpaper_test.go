package wallpaper

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// captureCommands swaps runWallpaperCommand for a recorder, returning a
// pointer to the argument lists of every command the code under test ran, in
// order. fail, when non-nil, decides which of them report an error - the
// Linux path deliberately treats two of its three possible commands
// differently on failure, and that distinction is most of what there is to
// test here.
func captureCommands(t *testing.T, fail func(args []string) error) *[][]string {
	t.Helper()

	orig := runWallpaperCommand
	t.Cleanup(func() { runWallpaperCommand = orig })

	var got [][]string
	runWallpaperCommand = func(cmd *exec.Cmd) ([]byte, error) {
		got = append(got, cmd.Args)
		if fail == nil {
			return nil, nil
		}
		return nil, fail(cmd.Args)
	}

	return &got
}

// stubLookups forces both binary lookups to a fixed outcome, so a test never
// depends on what happens to be installed on the machine running it.
func stubLookups(t *testing.T, gsettings, plasma string) {
	t.Helper()

	origG, origP := lookupGsettings, lookupPlasmaApply
	t.Cleanup(func() { lookupGsettings, lookupPlasmaApply = origG, origP })

	lookupGsettings = func() (string, error) {
		if gsettings == "" {
			return "", errors.New("not found")
		}
		return gsettings, nil
	}
	lookupPlasmaApply = func() (string, error) {
		if plasma == "" {
			return "", errors.New("not found")
		}
		return plasma, nil
	}
}

// TestSetLinux_SetsBothGnomeBackgroundKeys pins the reason this writes two
// keys rather than one: GNOME 42 split the background into a light and a
// dark picture-uri, and a user in dark mode sees nothing at all change when
// only the light one is written.
func TestSetLinux_SetsBothGnomeBackgroundKeys(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	stubLookups(t, "/usr/bin/gsettings", "")
	got := captureCommands(t, nil)

	if err := setLinux("/home/me/photo.png"); err != nil {
		t.Fatalf("setLinux() error = %v", err)
	}

	if len(*got) != 2 {
		t.Fatalf("ran %d commands, want 2 (picture-uri and picture-uri-dark): %v", len(*got), *got)
	}
	for i, key := range []string{"picture-uri", "picture-uri-dark"} {
		args := (*got)[i]
		if !slices.Contains(args, key) {
			t.Errorf("command %d = %v, want it to set %q", i, args, key)
		}
		if !slices.Contains(args, "org.gnome.desktop.background") {
			t.Errorf("command %d = %v, want it to address the background schema", i, args)
		}
		if !slices.Contains(args, "file:///home/me/photo.png") {
			t.Errorf("command %d = %v, want it to carry the file:// URI", i, args)
		}
	}
}

// TestSetLinux_IgnoresAMissingDarkKey covers GNOME before 42, where
// picture-uri-dark does not exist and gsettings fails on it. The light key
// is the whole job there, so that failure must not be reported as one.
func TestSetLinux_IgnoresAMissingDarkKey(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	stubLookups(t, "/usr/bin/gsettings", "")
	captureCommands(t, func(args []string) error {
		if slices.Contains(args, "picture-uri-dark") {
			return errors.New("No such key 'picture-uri-dark'")
		}
		return nil
	})

	if err := setLinux("/home/me/photo.png"); err != nil {
		t.Errorf("setLinux() error = %v, want nil - a missing dark key is not a failure", err)
	}
}

func TestSetLinux_ReturnsErrorWhenTheLightKeyIsRejected(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	stubLookups(t, "/usr/bin/gsettings", "")
	captureCommands(t, func(args []string) error {
		if slices.Contains(args, "picture-uri") {
			return errors.New("no schema")
		}
		return nil
	})

	if err := setLinux("/home/me/photo.png"); err == nil {
		t.Error("expected an error when gsettings rejects picture-uri")
	}
}

// TestSetLinux_PercentEncodesThePath guards the difference between a path
// and a URI: a '#' in a file name would otherwise start a fragment and
// truncate the URI, leaving gsettings pointed at a file that isn't there.
func TestSetLinux_PercentEncodesThePath(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	stubLookups(t, "/usr/bin/gsettings", "")
	got := captureCommands(t, nil)

	if err := setLinux("/home/me/my #1 photo.png"); err != nil {
		t.Fatalf("setLinux() error = %v", err)
	}

	want := "file:///home/me/my%20%231%20photo.png"
	if !slices.Contains((*got)[0], want) {
		t.Errorf("command = %v, want the URI %q", (*got)[0], want)
	}
}

// TestSetLinux_PrefersPlasmaOnKDE covers the one case where gsettings would
// succeed and change nothing: KDE, where glib is usually installed (so the
// lookup succeeds) but the desktop reads its wallpaper from somewhere else
// entirely, so the user would be told it worked and see no change.
func TestSetLinux_PrefersPlasmaOnKDE(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	stubLookups(t, "/usr/bin/gsettings", "/usr/bin/plasma-apply-wallpaperimage")
	got := captureCommands(t, nil)

	if err := setLinux("/home/me/photo.png"); err != nil {
		t.Fatalf("setLinux() error = %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("ran %d commands, want 1: %v", len(*got), *got)
	}
	args := (*got)[0]
	if !strings.Contains(args[0], "plasma-apply-wallpaperimage") {
		t.Errorf("ran %v, want plasma-apply-wallpaperimage", args)
	}
	if !slices.Contains(args, "/home/me/photo.png") {
		t.Errorf("args = %v, want the plain path passed through", args)
	}
}

// TestSetLinux_FallsBackToGsettingsOnKDEWithoutPlasmaApply covers Plasma
// before 5.24, which ships no plasma-apply-wallpaperimage: gsettings is a
// long shot there, but it is strictly better than refusing outright.
func TestSetLinux_FallsBackToGsettingsOnKDEWithoutPlasmaApply(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	stubLookups(t, "/usr/bin/gsettings", "")
	got := captureCommands(t, nil)

	if err := setLinux("/home/me/photo.png"); err != nil {
		t.Fatalf("setLinux() error = %v", err)
	}

	if len(*got) == 0 || !strings.Contains((*got)[0][0], "gsettings") {
		t.Errorf("ran %v, want a gsettings fallback", *got)
	}
}

func TestSetLinux_ReturnsErrorWhenNeitherToolIsInstalled(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	stubLookups(t, "", "")

	if err := setLinux("/home/me/photo.png"); err == nil {
		t.Error("expected an error when neither gsettings nor plasma-apply-wallpaperimage is installed")
	}
}

// scriptOf pulls the -Command argument out of a recorded PowerShell
// invocation, mirroring internal/trash's own windows-path tests.
func scriptOf(t *testing.T, args []string) string {
	t.Helper()

	for i, a := range args {
		if a == "-Command" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no -Command argument in %v", args)
	return ""
}

func TestSetWindows_BuildsExpectedScript(t *testing.T) {
	got := captureCommands(t, nil)

	if err := setWindows(`C:\Users\me\photo.png`); err != nil {
		t.Fatalf("setWindows() error = %v", err)
	}

	script := scriptOf(t, (*got)[0])
	for _, want := range []string{
		"Add-Type",
		"user32.dll",
		"SystemParametersInfo",
		// SPI_SETDESKWALLPAPER with SPIF_UPDATEINIFILE|SPIF_SENDWININICHANGE:
		// the flags are what make the change persist across a reboot and
		// reach the running shell rather than only this session's memory.
		`SystemParametersInfo(20, 0, "C:\Users\me\photo.png", 3)`,
		"exit 1",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script does not contain %q:\n%s", want, script)
		}
	}
}

// TestSetWindows_EscapesPathMetacharacters mirrors internal/trash's own: the
// path here is derived from a user's file name, and ` and $ are both legal
// in a Windows filename and both special inside a double-quoted PowerShell
// string.
func TestSetWindows_EscapesPathMetacharacters(t *testing.T) {
	got := captureCommands(t, nil)

	if err := setWindows("C:\\Users\\me\\$weird`file.png"); err != nil {
		t.Fatalf("setWindows() error = %v", err)
	}

	script := scriptOf(t, (*got)[0])
	if !strings.Contains(script, "`$weird") {
		t.Errorf("script does not escape $ in the path:\n%s", script)
	}
	if !strings.Contains(script, "``file") {
		t.Errorf("script does not escape ` in the path:\n%s", script)
	}
}

func TestEscapePowerShellPath(t *testing.T) {
	got := escapePowerShellPath("C:\\a$b`c")
	want := "C:\\a`$b``c"
	if got != want {
		t.Errorf("escapePowerShellPath() = %q, want %q", got, want)
	}
}
