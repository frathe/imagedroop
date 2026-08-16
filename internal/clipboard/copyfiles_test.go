package clipboard

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestCopyFiles_IgnoresAnEmptyList: an empty selection has nothing to put on
// the clipboard, and writing an empty uri-list would wipe whatever was there
// instead - so nothing runs at all.
func TestCopyFiles_IgnoresAnEmptyList(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		t.Fatal("no clipboard command should run for an empty file list")
		return nil, nil
	}

	if err := CopyFiles(nil); err != nil {
		t.Errorf("CopyFiles(nil) error = %v, want nil", err)
	}
}

func TestCopyFilesLinux_SendsAUriListOverXClip(t *testing.T) {
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

	if err := copyFilesLinux([]string{"/pics/a.jpg", "/pics/b.png"}); err != nil {
		t.Fatalf("copyFilesLinux() error = %v", err)
	}

	if !strings.Contains(strings.Join(gotArgs, " "), "text/uri-list") {
		t.Errorf("xclip args = %v, want the text/uri-list mime type", gotArgs)
	}
	if want := "file:///pics/a.jpg\r\nfile:///pics/b.png\r\n"; string(gotStdin) != want {
		t.Errorf("stdin = %q, want %q", gotStdin, want)
	}
}

// TestCopyFilesLinux_PercentEncodesThePaths: a uri-list carries URIs, not
// paths, so a space or a '#' in a file name has to survive the round trip.
func TestCopyFilesLinux_PercentEncodesThePaths(t *testing.T) {
	origXClip, origRun := lookupXClip, runClipboardCommand
	t.Cleanup(func() { lookupXClip, runClipboardCommand = origXClip, origRun })

	lookupXClip = func() (string, error) { return "/usr/bin/xclip", nil }

	var gotStdin []byte
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotStdin, _ = io.ReadAll(cmd.Stdin)
		return nil, nil
	}

	if err := copyFilesLinux([]string{"/pics/holiday #1.jpg"}); err != nil {
		t.Fatalf("copyFilesLinux() error = %v", err)
	}

	if want := "file:///pics/holiday%20%231.jpg\r\n"; string(gotStdin) != want {
		t.Errorf("stdin = %q, want %q", gotStdin, want)
	}
}

func TestCopyFilesLinux_FallsBackToWlCopy(t *testing.T) {
	origXClip, origWlCopy, origRun := lookupXClip, lookupWlCopy, runClipboardCommand
	t.Cleanup(func() { lookupXClip, lookupWlCopy, runClipboardCommand = origXClip, origWlCopy, origRun })

	lookupXClip = func() (string, error) { return "", errors.New("not found") }
	lookupWlCopy = func() (string, error) { return "/usr/bin/wl-copy", nil }

	var gotPath string
	var gotArgs []string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotPath, gotArgs = cmd.Path, cmd.Args
		return nil, nil
	}

	if err := copyFilesLinux([]string{"/pics/a.jpg"}); err != nil {
		t.Fatalf("copyFilesLinux() error = %v", err)
	}

	if !strings.Contains(gotPath, "wl-copy") {
		t.Errorf("cmd.Path = %q, want it to run wl-copy", gotPath)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "text/uri-list") {
		t.Errorf("wl-copy args = %v, want the text/uri-list mime type", gotArgs)
	}
}

func TestCopyFilesLinux_ReturnsErrorWhenNeitherToolInstalled(t *testing.T) {
	origXClip, origWlCopy := lookupXClip, lookupWlCopy
	t.Cleanup(func() { lookupXClip, lookupWlCopy = origXClip, origWlCopy })

	lookupXClip = func() (string, error) { return "", errors.New("not found") }
	lookupWlCopy = func() (string, error) { return "", errors.New("not found") }

	if err := copyFilesLinux([]string{"/pics/a.jpg"}); err == nil {
		t.Error("expected an error when neither xclip nor wl-copy is installed")
	}
}

// TestCopyFilesWindows_ReadsThePathListFromAFile pins the same choice
// copyImageWindows makes for its single path: the list never reaches
// PowerShell as source text, so a '$' or a backtick in a file name can't be
// interpreted as script.
func TestCopyFilesWindows_ReadsThePathListFromAFile(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	var gotScript, gotListPath, gotList string
	var gotArgs []string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotArgs = cmd.Args
		for i, a := range cmd.Args {
			if a == "-Command" && i+1 < len(cmd.Args) {
				gotScript = cmd.Args[i+1]
			}
		}
		for _, e := range cmd.Env {
			if name, value, _ := strings.Cut(e, "="); name == "IMAGEDROP_CLIPBOARD_LIST" {
				gotListPath = value
			}
		}
		data, err := os.ReadFile(gotListPath)
		if err != nil {
			t.Fatalf("reading the path list at %q: %v", gotListPath, err)
		}
		gotList = string(data)

		return nil, nil
	}

	if err := copyFilesWindows([]string{`C:\pics\a.jpg`, `C:\pics\b $one.png`}); err != nil {
		t.Fatalf("copyFilesWindows() error = %v", err)
	}

	for _, want := range []string{"Set-Clipboard", "-LiteralPath", "$env:IMAGEDROP_CLIPBOARD_LIST", "catch", "exit 1"} {
		if !strings.Contains(gotScript, want) {
			t.Errorf("script does not contain %q:\n%s", want, gotScript)
		}
	}
	if strings.Contains(gotScript, `b $one.png`) {
		t.Errorf("script embeds the file names instead of reading them from a file:\n%s", gotScript)
	}
	if want := "C:\\pics\\a.jpg\r\nC:\\pics\\b $one.png\r\n"; gotList != want {
		t.Errorf("path list file = %q, want %q", gotList, want)
	}
	if !slices.Contains(gotArgs, "-STA") {
		t.Errorf("cmd.Args = %v, want -STA present (same apartment-state fix copyImageWindows carries)", gotArgs)
	}
}

// TestCopyFilesWindows_RemovesTheListFile: the temp file holds file names,
// not image data, but it is still litter if the shell-out leaves it behind.
func TestCopyFilesWindows_RemovesTheListFile(t *testing.T) {
	origRun := runClipboardCommand
	t.Cleanup(func() { runClipboardCommand = origRun })

	var listPath string
	runClipboardCommand = func(cmd *exec.Cmd) ([]byte, error) {
		for _, e := range cmd.Env {
			if name, value, _ := strings.Cut(e, "="); name == "IMAGEDROP_CLIPBOARD_LIST" {
				listPath = value
			}
		}
		return nil, nil
	}

	if err := copyFilesWindows([]string{`C:\pics\a.jpg`}); err != nil {
		t.Fatalf("copyFilesWindows() error = %v", err)
	}

	if listPath == "" {
		t.Fatal("no IMAGEDROP_CLIPBOARD_LIST in the command environment")
	}
	if _, err := os.Stat(listPath); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) err = %v, want the list file to have been removed", listPath, err)
	}
}
