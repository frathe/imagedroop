package trash

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// TestMoveLinux_OverridesConfinedXDGDataHome guards against a real bug: a
// snap-confined launcher (VS Code's own snap, notably - this app is
// routinely built and run from its integrated terminal) sets XDG_DATA_HOME
// to a private per-app directory without touching HOME. gio/trash-put
// resolve the trash directory as $XDG_DATA_HOME/Trash, so inheriting that
// override moves the file somewhere the desktop's file manager never shows
// - indistinguishable from a silent permanent delete. moveLinux must force
// XDG_DATA_HOME back to $HOME/.local/share for the child process regardless
// of what the parent process's environment says.
func TestMoveLinux_OverridesConfinedXDGDataHome(t *testing.T) {
	origGio, origTrashPut, origRun := lookupGio, lookupTrashPut, runTrashCommand
	t.Cleanup(func() { lookupGio, lookupTrashPut, runTrashCommand = origGio, origTrashPut, origRun })

	t.Setenv("XDG_DATA_HOME", "/home/me/snap/code/257/.local/share")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	wantDataHome := "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share")

	lookupGio = func() (string, error) { return "/usr/bin/gio", nil }
	lookupTrashPut = func() (string, error) { return "", errors.New("not found") }

	var gotEnv []string
	runTrashCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotEnv = cmd.Env
		return nil, nil
	}

	if err := moveLinux("/tmp/photo.jpg"); err != nil {
		t.Fatalf("moveLinux() error = %v", err)
	}

	found := false
	for _, kv := range gotEnv {
		if strings.HasPrefix(kv, "XDG_DATA_HOME=") {
			if kv != wantDataHome {
				t.Errorf("XDG_DATA_HOME = %q, want %q", kv, wantDataHome)
			}
			found = true
		}
	}
	if !found {
		t.Error("cmd.Env has no XDG_DATA_HOME entry")
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
	tests := []struct {
		name       string
		makeTarget func(*testing.T) string
		method     string
		notMethod  string
	}{
		{
			name: "file",
			makeTarget: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "photo.jpg")
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			method:    "DeleteFile",
			notMethod: "DeleteDirectory",
		},
		{
			name:       "directory",
			makeTarget: func(t *testing.T) string { return t.TempDir() },
			method:     "DeleteDirectory",
			notMethod:  "DeleteFile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			path := tt.makeTarget(t)
			if err := moveWindows(path); err != nil {
				t.Fatalf("moveWindows() error = %v", err)
			}

			for _, want := range []string{
				"Microsoft.VisualBasic",
				"[Microsoft.VisualBasic.FileIO.FileSystem]::" + tt.method,
				"SendToRecycleBin",
				"catch",
				"exit 1",
				path,
			} {
				if !strings.Contains(gotScript, want) {
					t.Errorf("script does not contain %q:\n%s", want, gotScript)
				}
			}
			if strings.Contains(gotScript, "[Microsoft.VisualBasic.FileIO.FileSystem]::"+tt.notMethod) {
				t.Errorf("script unexpectedly contains %s:\n%s", tt.notMethod, gotScript)
			}
		})
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

	path := filepath.Join(t.TempDir(), "$weird`file.jpg")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveWindows(path); err != nil {
		t.Fatalf("moveWindows() error = %v", err)
	}

	if !strings.Contains(gotScript, "`$weird") {
		t.Errorf("script does not escape $ in the path:\n%s", gotScript)
	}
	if !strings.Contains(gotScript, "``file") {
		t.Errorf("script does not escape ` in the path:\n%s", gotScript)
	}
}

func TestMoveWindows_ReturnsStatErrorBeforeRunningCommand(t *testing.T) {
	origRun := runTrashCommand
	t.Cleanup(func() { runTrashCommand = origRun })
	runTrashCommand = func(*exec.Cmd) ([]byte, error) {
		t.Fatal("command should not run for a missing target")
		return nil, nil
	}

	if err := moveWindows(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("moveWindows error = %v, want os.ErrNotExist", err)
	}
}

func TestMoveWindows_ReturnsCommandError(t *testing.T) {
	origRun := runTrashCommand
	t.Cleanup(func() { runTrashCommand = origRun })
	wantErr := errors.New("powershell failed")
	runTrashCommand = func(*exec.Cmd) ([]byte, error) { return nil, wantErr }

	if err := moveWindows(t.TempDir()); !errors.Is(err, wantErr) {
		t.Errorf("moveWindows error = %v, want %v", err, wantErr)
	}
}

func TestEscapePowerShellPath(t *testing.T) {
	got := escapePowerShellPath("C:\\a$b`c")
	want := "C:\\a`$b``c"
	if got != want {
		t.Errorf("escapePowerShellPath() = %q, want %q", got, want)
	}
}
