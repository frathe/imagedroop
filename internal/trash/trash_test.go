package trash

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestMoveLinux_PrefersGio(t *testing.T) {
	origGio, origTrashPut, origRun := lookupGio, lookupTrashPut, runTrashCommand
	t.Cleanup(func() { lookupGio, lookupTrashPut, runTrashCommand = origGio, origTrashPut, origRun })

	lookupGio = func() (string, error) { return "/usr/bin/gio", nil }
	lookupTrashPut = func() (string, error) {
		t.Fatal("trash-put should not be consulted when gio is present")
		return "", nil
	}

	var gotArgs []string
	runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotArgs = cmd.Args
		return nil, nil
	}

	if err := moveLinux("/tmp/photo.jpg"); err != nil {
		t.Fatalf("moveLinux() error = %v", err)
	}

	if !strings.Contains(strings.Join(gotArgs, " "), "trash /tmp/photo.jpg") {
		t.Errorf("gio args = %v, want them to run \"trash /tmp/photo.jpg\"", gotArgs)
	}
}

func TestMoveLinux_FallsBackToTrashPut(t *testing.T) {
	origGio, origTrashPut, origRun := lookupGio, lookupTrashPut, runTrashCommand
	t.Cleanup(func() { lookupGio, lookupTrashPut, runTrashCommand = origGio, origTrashPut, origRun })

	lookupGio = func() (string, error) { return "", errors.New("not found") }
	lookupTrashPut = func() (string, error) { return "/usr/bin/trash-put", nil }

	var gotPath string
	var gotArgs []string
	runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotPath = cmd.Path
		gotArgs = cmd.Args
		return nil, nil
	}

	if err := moveLinux("/tmp/photo.jpg"); err != nil {
		t.Fatalf("moveLinux() error = %v", err)
	}
	if !strings.Contains(gotPath, "trash-put") {
		t.Errorf("cmd.Path = %q, want it to run trash-put", gotPath)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "/tmp/photo.jpg") {
		t.Errorf("trash-put args = %v, want the path passed through", gotArgs)
	}
}

func TestMoveLinux_ReturnsErrorWhenNeitherToolInstalled(t *testing.T) {
	origGio, origTrashPut := lookupGio, lookupTrashPut
	t.Cleanup(func() { lookupGio, lookupTrashPut = origGio, origTrashPut })

	lookupGio = func() (string, error) { return "", errors.New("not found") }
	lookupTrashPut = func() (string, error) { return "", errors.New("not found") }

	if err := moveLinux("/tmp/photo.jpg"); err == nil {
		t.Error("expected an error when neither gio nor trash-put is installed")
	}
}

func TestMoveWindows_BuildsExpectedScript(t *testing.T) {
	origRun := runTrashCommand
	t.Cleanup(func() { runTrashCommand = origRun })

	var gotScript string
	runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) {
		for i, a := range cmd.Args {
			if a == "-Command" && i+1 < len(cmd.Args) {
				gotScript = cmd.Args[i+1]
			}
		}
		return nil, nil
	}

	if err := moveWindows(`C:\Users\me\photo.jpg`); err != nil {
		t.Fatalf("moveWindows() error = %v", err)
	}

	for _, want := range []string{
		"Microsoft.VisualBasic",
		"[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile",
		"SendToRecycleBin",
		"catch",
		"exit 1",
		`C:\Users\me\photo.jpg`,
	} {
		if !strings.Contains(gotScript, want) {
			t.Errorf("script does not contain %q:\n%s", want, gotScript)
		}
	}
}

// TestMoveWindows_EscapesPathMetacharacters guards a real difference from
// clipboard.go's copyImageWindows/filepicker.go's chooseFilesWindows: those
// embed only app-generated temp paths or a UI label, but this path is an
// arbitrary dropped file's path, which - unlike a Windows path with a
// literal " (illegal, so never a concern) - can legally contain ` and $,
// both special inside a PowerShell double-quoted string.
func TestMoveWindows_EscapesPathMetacharacters(t *testing.T) {
	origRun := runTrashCommand
	t.Cleanup(func() { runTrashCommand = origRun })

	var gotScript string
	runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) {
		for i, a := range cmd.Args {
			if a == "-Command" && i+1 < len(cmd.Args) {
				gotScript = cmd.Args[i+1]
			}
		}
		return nil, nil
	}

	if err := moveWindows("C:\\Users\\me\\$weird`file.jpg"); err != nil {
		t.Fatalf("moveWindows() error = %v", err)
	}

	if !strings.Contains(gotScript, "`$weird") {
		t.Errorf("script does not escape $ in the path:\n%s", gotScript)
	}
	if !strings.Contains(gotScript, "``file") {
		t.Errorf("script does not escape ` in the path:\n%s", gotScript)
	}
}

func TestEscapePowerShellPath(t *testing.T) {
	got := escapePowerShellPath("C:\\a$b`c")
	want := "C:\\a`$b``c"
	if got != want {
		t.Errorf("escapePowerShellPath() = %q, want %q", got, want)
	}
}
