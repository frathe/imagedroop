//go:build !windows

package trash

import "os/exec"

// hideConsoleWindow's real implementation (windows.go) sets a Windows-only
// SysProcAttr field, so it needs this no-op twin for every other platform:
// moveWindows is compiled - and unit-tested - everywhere, even though Move
// only ever dispatches to it on Windows.
func hideConsoleWindow(*exec.Cmd) {}
