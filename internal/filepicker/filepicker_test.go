package filepicker

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestParseFileList(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want int
	}{
		{"single path", "/tmp/a.jpg\n", 1},
		{"multiple paths", "/tmp/a.jpg\n/tmp/b.png\n", 2},
		{"no trailing newline", "/tmp/a.jpg", 1},
		{"blank lines skipped", "/tmp/a.jpg\n\n/tmp/b.png\n", 2},
		{"empty output", "", 0},
		{"only a newline", "\n", 0},
		{"CRLF line endings (PowerShell)", "/tmp/a.jpg\r\n/tmp/b.png\r\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uris := ParseFileList([]byte(tt.out))
			if len(uris) != tt.want {
				t.Errorf("len(uris) = %d, want %d", len(uris), tt.want)
			}
		})
	}
}

func TestParseFileList_PreservesOrderAndStripsCR(t *testing.T) {
	uris := ParseFileList([]byte("/tmp/a.jpg\r\n/tmp/b.png\r\n"))

	if len(uris) != 2 {
		t.Fatalf("len(uris) = %d, want 2", len(uris))
	}
	if uris[0].Path() != "/tmp/a.jpg" {
		t.Errorf("uris[0].Path() = %q, want /tmp/a.jpg", uris[0].Path())
	}
	if uris[1].Path() != "/tmp/b.png" {
		t.Errorf("uris[1].Path() = %q, want /tmp/b.png", uris[1].Path())
	}
}

func TestPowerShellEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Open images", "Open images"},
		{`say "hi"`, "say `\"hi`\""},
		{"back`tick", "back``tick"},
		{"costs $5", "costs `$5"},
		{"$HOME`, really", "`$HOME``, really"},
	}
	for _, tt := range tests {
		if got := powerShellEscape(tt.in); got != tt.want {
			t.Errorf("powerShellEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The darwin chooser (darwin.go) deliberately has no unit test: it is an
// in-process cgo/AppKit modal panel, so exercising it means opening a real
// dialog and clicking it - the package compiling on darwin is the only
// automatic check it gets. Its two subprocess-based predecessors both
// passed every non-interactive check (osacompile syntax pinning, a
// self-cancelling NSTimer modal-loop smoke test) and still failed in
// users' hands, which is exactly why it became in-process cgo - see
// darwin.go's chooseFilesDarwin comment for the two failed generations.
func TestBuildPowerShellCmd(t *testing.T) {
	cmd := buildPowerShellCmd()

	if got := cmd.Args[0]; !strings.HasSuffix(got, "powershell") {
		t.Errorf("cmd.Args[0] = %q, want it to name powershell", got)
	}

	var script string
	for i, a := range cmd.Args {
		if a == "-Command" && i+1 < len(cmd.Args) {
			script = cmd.Args[i+1]
		}
	}
	if script == "" {
		t.Fatalf("cmd.Args = %v, expected a -Command argument", cmd.Args)
	}

	for _, want := range []string{
		"OpenFileDialog",
		"$dlg.Multiselect = $true",
		"Open images",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script does not contain %q:\n%s", want, script)
		}
	}

	for _, notWant := range []string{
		"folderSentinel",
		"Select this folder.",
		"GetDirectoryName",
		"ValidateNames",
	} {
		if strings.Contains(script, notWant) {
			t.Errorf("script contains %q, want the folder-sentinel workaround gone:\n%s", notWant, script)
		}
	}
}

func TestChooseFilesLinux_ReturnsErrorWhenZenityMissing(t *testing.T) {
	origLookup := lookupZenity
	t.Cleanup(func() { lookupZenity = origLookup })
	lookupZenity = func(string) (string, error) { return "", errors.New("zenity not found") }

	if _, err := chooseFilesLinux(); err == nil {
		t.Error("expected an error when zenity isn't installed")
	}
}

func TestChooseFilesLinux_RunsZenityWithMultiSelect(t *testing.T) {
	origLookup := lookupZenity
	origRun := runZenityCommand
	t.Cleanup(func() {
		lookupZenity = origLookup
		runZenityCommand = origRun
	})
	lookupZenity = func(string) (string, error) { return "/usr/bin/zenity", nil }

	var gotArgs []string
	runZenityCommand = func(cmd *exec.Cmd) ([]byte, error) {
		gotArgs = cmd.Args
		return []byte("/tmp/a.jpg\n"), nil
	}

	out, err := chooseFilesLinux()
	if err != nil {
		t.Fatalf("chooseFilesLinux() error = %v", err)
	}
	if string(out) != "/tmp/a.jpg\n" {
		t.Errorf("out = %q, want the stubbed zenity output", out)
	}

	found := false
	for _, a := range gotArgs {
		if a == "--multiple" {
			found = true
		}
	}
	if !found {
		t.Errorf("zenity args = %v, want --multiple present", gotArgs)
	}
}
