//go:build windows

package winpos

import (
	"syscall"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procClientToScreen = user32.NewProc("ClientToScreen")
	procShowWindow     = user32.NewProc("ShowWindow")
)

// swMaximize is SW_MAXIMIZE, the nCmdShow value ShowWindow uses to activate
// a window and size it to fill the work area - exactly what clicking the
// window's own maximize button does.
const swMaximize = 3

// swRestore is SW_RESTORE, the nCmdShow value that undoes swMaximize -
// activating the window and returning it to its pre-maximize size and
// position, exactly what clicking the window's own restore button does.
const swRestore = 9

type point struct {
	x, y int32
}

// platformPosition mirrors what Fyne's glfw driver itself does for its own
// position bookkeeping: ClientToScreen on the client area's (0,0) corner,
// not GetWindowRect, which would include the non-client window frame/border
// and drift from the coordinates RequestPosition/SetWindowPos actually
// place the window at.
func platformPosition(ctx any) (x, y int, ok bool) {
	win, isWin := ctx.(driver.WindowsWindowContext)
	if !isWin || win.HWND == 0 {
		return 0, 0, false
	}

	var pt point
	ret, _, _ := procClientToScreen.Call(win.HWND, uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		return 0, 0, false
	}
	return int(pt.x), int(pt.y), true
}

func platformMaximize(ctx any) {
	win, isWin := ctx.(driver.WindowsWindowContext)
	if !isWin || win.HWND == 0 {
		return
	}

	procShowWindow.Call(win.HWND, uintptr(swMaximize))
}

func platformUnmaximize(ctx any) {
	win, isWin := ctx.(driver.WindowsWindowContext)
	if !isWin || win.HWND == 0 {
		return
	}

	procShowWindow.Call(win.HWND, uintptr(swRestore))
}
