//go:build windows

package filepicker

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW, which the syscall package doesn't
// name: the child is a console application run without a console window.
const createNoWindow = 0x08000000

// hideConsoleWindow keeps the powershell child from opening a console
// window of its own: picfetch is a GUI-subsystem binary with no console
// to inherit, so a console-subsystem child gets a fresh visible console
// unless CREATE_NO_WINDOW says otherwise. Deliberately not SysProcAttr's
// HideWindow: that sets STARTUPINFO's wShowWindow to SW_HIDE, a hint the
// child's first ShowWindow(SW_SHOWDEFAULT) call would honor - risking the
// file dialog itself - while CREATE_NO_WINDOW suppresses exactly the
// console and nothing else.
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
