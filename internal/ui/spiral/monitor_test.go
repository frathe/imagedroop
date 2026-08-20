package spiral

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestF32toStr(t *testing.T) {
	tests := []struct {
		v    float32
		want string
	}{
		{1.23, "1.23"},
		{1.0, "1.00"},
		{0.0, "0.00"},
		{123.456, "123.46"},
		{-1.5, "-1.50"},
		{-123.456, "-123.46"},
	}
	for _, tt := range tests {
		if got := f32toStr(tt.v); got != tt.want {
			t.Errorf("f32toStr(%f) = %s; want %s", tt.v, got, tt.want)
		}
	}
}

func TestMonitorInfoName(t *testing.T) {
	tests := []struct {
		x, y int
		want string
	}{
		{0, 0, "main"},
		{100, 0, "ext"},
		{0, 100, "ext"},
	}
	for _, tt := range tests {
		mi := monitorInfo{x: tt.x, y: tt.y}
		if got := mi.name(); got != tt.want {
			t.Errorf("monitorInfo{x:%d,y:%d}.name() = %s; want %s", tt.x, tt.y, got, tt.want)
		}
	}
}

// TestGetMonitorInfo checks getMonitorInfo against a real (windowless test
// driver) window resized to a known logical size: the reported logical
// dimensions must match the canvas size exactly, and the physical pixel
// dimensions must equal the logical size times the canvas scale, rounded -
// which is exactly what getMonitorInfo itself computes, so this mostly
// guards against the calculation drifting apart from the canvas it reads.
func TestGetMonitorInfo(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()

	w.Resize(fyne.NewSize(640, 480))

	mi := getMonitorInfo(w)

	wantLogicalW := w.Canvas().Size().Width
	wantLogicalH := w.Canvas().Size().Height
	if mi.logicalW != wantLogicalW {
		t.Errorf("logicalW = %f; want %f", mi.logicalW, wantLogicalW)
	}
	if mi.logicalH != wantLogicalH {
		t.Errorf("logicalH = %f; want %f", mi.logicalH, wantLogicalH)
	}

	scale := w.Canvas().Scale()
	if mi.scale != scale {
		t.Errorf("scale = %f; want %f", mi.scale, scale)
	}

	wantWidth := int(math.Round(float64(wantLogicalW) * float64(scale)))
	wantHeight := int(math.Round(float64(wantLogicalH) * float64(scale)))
	if mi.width != wantWidth {
		t.Errorf("width = %d; want %d", mi.width, wantWidth)
	}
	if mi.height != wantHeight {
		t.Errorf("height = %d; want %d", mi.height, wantHeight)
	}

	if mi.x != 0 || mi.y != 0 {
		t.Errorf("x, y = %d, %d; want 0, 0", mi.x, mi.y)
	}
}
