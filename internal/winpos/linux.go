//go:build linux

package winpos

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <string.h>

// translateToRoot resolves win's own (0,0) corner to root-window (i.e.
// screen) coordinates, the same call GLFW's own x11_window.c makes for its
// own position bookkeeping, so a value read here and later handed to Set
// lands back in the same place.
static int translateToRoot(Display *display, Window win, int *x, int *y) {
	Window root = DefaultRootWindow(display);
	Window child;
	return XTranslateCoordinates(display, win, root, 0, 0, x, y, &child);
}

// maximizeWindow asks the window manager to maximize win via the EWMH
// _NET_WM_STATE protocol: a ClientMessage to the root window (not to win
// itself - a maximize request has to be *redirected* through the WM the
// way SubstructureRedirectMask names it, the same channel a window manager
// grants a client no direct access to) asking to add both the horizontal
// and vertical maximized states. This is what every EWMH-compliant window
// manager (GNOME, KDE, XFCE...) implements behind its own maximize button;
// a bare X11/Xlib connection has no simpler verb for it than Windows'
// single ShowWindow(SW_MAXIMIZE) call or Cocoa's -zoom:.
static void maximizeWindow(Display *display, Window win) {
	Atom wmState = XInternAtom(display, "_NET_WM_STATE", False);
	Atom maxVert = XInternAtom(display, "_NET_WM_STATE_MAXIMIZED_VERT", False);
	Atom maxHorz = XInternAtom(display, "_NET_WM_STATE_MAXIMIZED_HORZ", False);

	XEvent xev;
	memset(&xev, 0, sizeof(xev));
	xev.type = ClientMessage;
	xev.xclient.window = win;
	xev.xclient.message_type = wmState;
	xev.xclient.format = 32;
	xev.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
	xev.xclient.data.l[1] = (long)maxVert;
	xev.xclient.data.l[2] = (long)maxHorz;
	xev.xclient.data.l[3] = 1; // source indication: a normal application
	xev.xclient.data.l[4] = 0;

	Window root = DefaultRootWindow(display);
	XSendEvent(display, root, False,
		SubstructureRedirectMask | SubstructureNotifyMask, &xev);
	XFlush(display);
}

// unmaximizeWindow is maximizeWindow's inverse: the identical EWMH
// ClientMessage, but with data.l[0] asking the window manager to remove
// (_NET_WM_STATE_REMOVE) both maximized states instead of adding them.
static void unmaximizeWindow(Display *display, Window win) {
	Atom wmState = XInternAtom(display, "_NET_WM_STATE", False);
	Atom maxVert = XInternAtom(display, "_NET_WM_STATE_MAXIMIZED_VERT", False);
	Atom maxHorz = XInternAtom(display, "_NET_WM_STATE_MAXIMIZED_HORZ", False);

	XEvent xev;
	memset(&xev, 0, sizeof(xev));
	xev.type = ClientMessage;
	xev.xclient.window = win;
	xev.xclient.message_type = wmState;
	xev.xclient.format = 32;
	xev.xclient.data.l[0] = 0; // _NET_WM_STATE_REMOVE
	xev.xclient.data.l[1] = (long)maxVert;
	xev.xclient.data.l[2] = (long)maxHorz;
	xev.xclient.data.l[3] = 1; // source indication: a normal application
	xev.xclient.data.l[4] = 0;

	Window root = DefaultRootWindow(display);
	XSendEvent(display, root, False,
		SubstructureRedirectMask | SubstructureNotifyMask, &xev);
	XFlush(display);
}
*/
import "C"

import (
	"fyne.io/fyne/v2/driver"
)

// platformPosition only ever succeeds under X11: ctx is a
// driver.WaylandWindowContext instead on a Wayland session, which the type
// assertion below simply doesn't match, matching RequestPosition's own
// documented "may be ignored" stance there - Wayland has no protocol for a
// client to ask the compositor where its own window sits.
func platformPosition(ctx any) (x, y int, ok bool) {
	x11, isX11 := ctx.(driver.X11WindowContext)
	if !isX11 || x11.WindowHandle == 0 {
		return 0, 0, false
	}

	display := C.XOpenDisplay(nil)
	if display == nil {
		return 0, 0, false
	}
	defer C.XCloseDisplay(display)

	var cx, cy C.int
	if C.translateToRoot(display, C.Window(x11.WindowHandle), &cx, &cy) == 0 {
		return 0, 0, false
	}
	return int(cx), int(cy), true
}

// platformMaximize only ever succeeds under X11, the same as
// platformPosition above: ctx is a driver.WaylandWindowContext instead on a
// Wayland session, which the type assertion below simply doesn't match. There
// is no equivalent to send there - maximizing is an xdg-shell toplevel
// request tied to the surface's own protocol object, not something a
// separate, unrelated connection can ask for from the outside the way X11's
// client-message convention allows.
func platformMaximize(ctx any) {
	x11, isX11 := ctx.(driver.X11WindowContext)
	if !isX11 || x11.WindowHandle == 0 {
		return
	}

	display := C.XOpenDisplay(nil)
	if display == nil {
		return
	}
	defer C.XCloseDisplay(display)

	C.maximizeWindow(display, C.Window(x11.WindowHandle))
}

// platformUnmaximize only ever succeeds under X11, the same as
// platformMaximize above - see its comment for why Wayland has no
// equivalent to send.
func platformUnmaximize(ctx any) {
	x11, isX11 := ctx.(driver.X11WindowContext)
	if !isX11 || x11.WindowHandle == 0 {
		return
	}

	display := C.XOpenDisplay(nil)
	if display == nil {
		return
	}
	defer C.XCloseDisplay(display)

	C.unmaximizeWindow(display, C.Window(x11.WindowHandle))
}
