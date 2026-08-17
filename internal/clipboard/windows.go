//go:build windows

package clipboard

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
// child's first ShowWindow(SW_SHOWDEFAULT) call would honor rather than a
// guarantee, where CREATE_NO_WINDOW suppresses exactly the console and
// nothing else - the copy-to-clipboard shell-out has no window of its own
// that a hint could misfire against, but a flashing console for a keyboard
// shortcut would still be a jarring, needless glitch.
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
