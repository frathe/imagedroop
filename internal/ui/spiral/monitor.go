package spiral

import (
	"fmt"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

// monitorInfo holds the physical pixel dimensions and scale of the monitor
// that the fullscreen window is displayed on.
type monitorInfo struct {
	x, y     int     // top-left position (in logical points)
	width    int     // physical pixel width
	height   int     // physical pixel height
	scale    float32 // DPI scale factor
	logicalW float32 // logical (point) width
	logicalH float32 // logical (point) height
}

// getMonitorInfo computes the physical pixel dimensions from the canvas size
// and scale. On macOS the scale factor already accounts for Retina, so
// Size() * Scale() gives the true framebuffer resolution.
func getMonitorInfo(w fyne.Window) monitorInfo {
	mi := monitorInfo{
		scale:    w.Canvas().Scale(),
		logicalW: w.Canvas().Size().Width,
		logicalH: w.Canvas().Size().Height,
	}

	// Convert logical points to physical pixels.
	mi.width = int(math.Round(float64(mi.logicalW) * float64(mi.scale)))
	mi.height = int(math.Round(float64(mi.logicalH) * float64(mi.scale)))

	mi.x = 0
	mi.y = 0

	return mi
}

// name identifies the monitor for the status overlay: picfetch, like the
// donor demo, has no way to name a real display, so this just distinguishes
// the window's own (0, 0) origin from a window that has been dragged onto
// a second monitor.
func (mi monitorInfo) name() string {
	if mi.x == 0 && mi.y == 0 {
		return lang.L("main")
	}
	return lang.L("ext")
}

// f32toStr formats a float32 to two decimal places, for the status overlay.
func f32toStr(f float32) string {
	return fmt.Sprintf("%.2f", f)
}
