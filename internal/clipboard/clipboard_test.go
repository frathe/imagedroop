package clipboard

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCopyImageLinux_PrefersXClip(t *testing.T) {
	origXClip, origWlCopy, origRun := lookupXClip, lookupWlCopy, runClipboardCommand
	t.Cleanup(func() { lookupXClip, lookupWlCopy, runClipboardCommand = origXClip, origWlCopy, origRun })

	lookupXClip = func() (string, error) { return "/usr/bin/xclip", nil }
	lookupWlCopy = func() (string, error) {
		t.Fatal("wl-copy should not be consulted when xclip is present")
		return "", nil
	}

	var gotArgs []string
	var gotStdin []byte
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotArgs = cmd.Args
		gotStdin, _ = io.ReadAll(cmd.Stdin)
		return nil, nil
	}

	if err := copyImageLinux([]byte("png-bytes")); err != nil {
		t.Fatalf("copyImageLinux() error = %v", err)
	}

	if !strings.Contains(strings.Join(gotArgs, " "), "image/png") {
		t.Errorf("xclip args = %v, want image/png mime type", gotArgs)
	}
	if string(gotStdin) != "png-bytes" {
		t.Errorf("stdin = %q, want the PNG bytes piped straight through", gotStdin)
	}
}

func TestCopyImageLinux_FallsBackToWlCopy(t *testing.T) {
	origXClip, origWlCopy, origRun := lookupXClip, lookupWlCopy, runClipboardCommand
	t.Cleanup(func() { lookupXClip, lookupWlCopy, runClipboardCommand = origXClip, origWlCopy, origRun })

	lookupXClip = func() (string, error) { return "", errors.New("not found") }
	lookupWlCopy = func() (string, error) { return "/usr/bin/wl-copy", nil }

	var gotPath string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotPath = cmd.Path
		return nil, nil
	}

	if err := copyImageLinux([]byte("png-bytes")); err != nil {
		t.Fatalf("copyImageLinux() error = %v", err)
	}
	if !strings.Contains(gotPath, "wl-copy") {
		t.Errorf("cmd.Path = %q, want it to run wl-copy", gotPath)
	}
}

func TestCopyImageLinux_ReturnsErrorWhenNeitherToolInstalled(t *testing.T) {
	origXClip, origWlCopy := lookupXClip, lookupWlCopy
	t.Cleanup(func() { lookupXClip, lookupWlCopy = origXClip, origWlCopy })

	lookupXClip = func() (string, error) { return "", errors.New("not found") }
	lookupWlCopy = func() (string, error) { return "", errors.New("not found") }

	if err := copyImageLinux([]byte("png-bytes")); err == nil {
		t.Error("expected an error when neither xclip nor wl-copy is installed")
	}
}

func TestCopyImageDarwin_RunsOsascriptAgainstATempPNGFile(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	var gotScript, gotPath string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotPath = cmd.Path
		for i, a := range cmd.Args {
			if a == "-e" && i+1 < len(cmd.Args) {
				gotScript = cmd.Args[i+1]
			}
		}
		return nil, nil
	}

	if err := copyImageDarwin([]byte("png-bytes")); err != nil {
		t.Fatalf("copyImageDarwin() error = %v", err)
	}

	if !strings.Contains(gotPath, "osascript") {
		t.Errorf("cmd.Path = %q, want it to run osascript", gotPath)
	}
	for _, want := range []string{"set the clipboard to", "PNGf", "POSIX file"} {
		if !strings.Contains(gotScript, want) {
			t.Errorf("script does not contain %q:\n%s", want, gotScript)
		}
	}
}

func TestCopyImageWindows_BuildsExpectedScript(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	var gotScript string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		for i, a := range cmd.Args {
			if a == "-Command" && i+1 < len(cmd.Args) {
				gotScript = cmd.Args[i+1]
			}
		}
		return nil, nil
	}

	if err := copyImageWindows([]byte("png-bytes")); err != nil {
		t.Fatalf("copyImageWindows() error = %v", err)
	}

	for _, want := range []string{
		"System.Windows.Forms",
		"System.Drawing",
		"[System.Drawing.Image]::FromFile",
		"[System.Windows.Forms.Clipboard]::SetDataObject",
		"catch",
		"exit 1",
		"$env:IMAGEDROP_CLIPBOARD_PNG",
	} {
		if !strings.Contains(gotScript, want) {
			t.Errorf("script does not contain %q:\n%s", want, gotScript)
		}
	}
	if strings.Contains(gotScript, os.TempDir()) {
		t.Errorf("script embeds the temporary path instead of reading it from the environment:\n%s", gotScript)
	}
}

// TestCopyImageWindows_RunsInSTA guards the fix for a Windows bug where
// Ctrl+C silently left the clipboard untouched: System.Windows.Forms
// clipboard access needs an STA thread, so the shell-out passes -STA
// explicitly rather than trusting powershell.exe's default apartment state.
func TestCopyImageWindows_RunsInSTA(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	var gotArgs []string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotArgs = cmd.Args
		return nil, nil
	}

	if err := copyImageWindows([]byte("png-bytes")); err != nil {
		t.Fatalf("copyImageWindows() error = %v", err)
	}

	found := false
	for _, a := range gotArgs {
		if a == "-STA" {
			found = true
		}
	}
	if !found {
		t.Errorf("cmd.Args = %v, want -STA present", gotArgs)
	}
}

func TestWriteTempPNG_WritesAndCleansUp(t *testing.T) {
	path, err := writeTempPNG([]byte("hello"))
	if err != nil {
		t.Fatalf("writeTempPNG() error = %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", data, "hello")
	}
}
