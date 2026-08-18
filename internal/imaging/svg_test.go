package imaging

import (
	"testing"
)

func svgDoc(attrs string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" ` + attrs + `><rect width="1" height="1"/></svg>`)
}

func TestVectorLogicalAppliesFloor(t *testing.T) {
	for _, tc := range []struct {
		name         string
		w, h         float64
		wantW, wantH int
	}{
		{"icon upscaled to the floor", 24, 24, 340, 340},
		{"already larger than the floor", 800, 600, 800, 600},
		{"already taller than the floor", 100, 400, 100, 400},
		{"width-limited by the floor", 100, 50, 520, 260},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := vectorLogical(tc.w, tc.h)
			if got.Dx() != tc.wantW || got.Dy() != tc.wantH {
				t.Fatalf("vectorLogical(%v, %v) = %dx%d, want %dx%d",
					tc.w, tc.h, got.Dx(), got.Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}

func TestVectorLogicalRejectsNonPositive(t *testing.T) {
	if got := vectorLogical(0, 10); !got.Empty() {
		t.Fatalf("vectorLogical(0, 10) = %v, want empty", got)
	}
}

func TestVectorLogicalRejectsAbsurdMagnitudes(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h float64
	}{
		{"wrapping product", 4e9, 4e9},
		{"beyond float-to-int range", 1e300, 1e300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := vectorLogical(tc.w, tc.h); !got.Empty() {
				t.Fatalf("vectorLogical(%v, %v) = %v, want empty", tc.w, tc.h, got)
			}
		})
	}
}

func TestClampVectorRasterSurvivesOverflowingAxes(t *testing.T) {
	w, h := ClampVectorRaster(4_000_000_000, 4_000_000_000)
	if n := int64(w) * int64(h); n > MaxVectorRasterPixels() {
		t.Fatalf("clamped to %dx%d = %d px, over the cap", w, h, n)
	}
}

func TestSVGIntrinsicPrefersViewBox(t *testing.T) {
	_, _, w, h, ok := svgIntrinsic(svgDoc(`width="800" height="600" viewBox="0 0 24 24"`))
	if !ok || w != 24 || h != 24 {
		t.Fatalf("got %vx%v ok=%v, want 24x24 ok=true", w, h, ok)
	}
}

// The failure mode that motivates svgSizeFrom: oksvg abandons its root
// handler on width="100%" and silently reports a 0x0 viewBox.
func TestSVGIntrinsicRecoversFromPercentageWidth(t *testing.T) {
	_, _, w, h, ok := svgIntrinsic(svgDoc(`width="100%" height="100%" viewBox="0 0 24 24"`))
	if !ok || w != 24 || h != 24 {
		t.Fatalf("got %vx%v ok=%v, want 24x24 ok=true", w, h, ok)
	}
}

func TestSVGIntrinsicFallsBackToWidthHeightWithUnits(t *testing.T) {
	_, _, w, h, ok := svgIntrinsic(svgDoc(`width="10cm" height="5cm"`))
	if !ok || w != 10 || h != 5 {
		t.Fatalf("got %vx%v ok=%v, want 10x5 ok=true", w, h, ok)
	}
}

func TestSVGIntrinsicRejectsSizelessDocument(t *testing.T) {
	if _, _, _, _, ok := svgIntrinsic(svgDoc("")); ok {
		t.Fatal("a document with neither viewBox nor width/height must be rejected")
	}
}

func TestIsSVGData(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{"plain svg", svgDoc(`viewBox="0 0 24 24"`), true},
		{"xml declaration and doctype", []byte("<?xml version=\"1.0\"?>\n<!DOCTYPE svg>\n" + string(svgDoc(`viewBox="0 0 48 32"`))), true},
		{"utf-8 bom", append([]byte{0xEF, 0xBB, 0xBF}, svgDoc(`viewBox="0 0 8 8"`)...), true},
		{"leading whitespace", append([]byte("\n\n  "), svgDoc(`viewBox="0 0 8 8"`)...), true},
		{"json is not an svg", []byte(`{"hello":"world"}`), false},
		{"png bytes are not an svg", []byte{0x89, 'P', 'N', 'G'}, false},
		{"html is not an svg", []byte(`<html><body/></html>`), false},
		{"empty", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSVGData(tc.data); got != tc.want {
				t.Fatalf("isSVGData = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClampVectorRasterPreservesAspect(t *testing.T) {
	w, h := ClampVectorRaster(40000, 30000) // 1.2e9 px, far over the cap
	if int64(w)*int64(h) > MaxVectorRasterPixels() {
		t.Fatalf("clamped to %dx%d = %d px, over the %d cap", w, h, int64(w)*int64(h), MaxVectorRasterPixels())
	}
	if ratio := float64(w) / float64(h); ratio < 1.32 || ratio > 1.34 {
		t.Fatalf("aspect %v, want ~1.333", ratio)
	}
}

func TestClampVectorRasterLeavesSmallRastersAlone(t *testing.T) {
	// A 340x340 icon at zoom's maxScale of 16 must fit inside the cap.
	if w, h := ClampVectorRaster(5440, 5440); w != 5440 || h != 5440 {
		t.Fatalf("got %dx%d, want 5440x5440 untouched", w, h)
	}
}

func TestClampVectorRasterFloorsAtOne(t *testing.T) {
	if w, h := ClampVectorRaster(0, -5); w != 1 || h != 1 {
		t.Fatalf("got %dx%d, want 1x1", w, h)
	}
}

func TestSetMaxVectorRasterPixelsClampsToItsRange(t *testing.T) {
	t.Cleanup(func() { SetMaxVectorRasterPixels(DefaultMaxVectorRasterPixels) })

	for _, tc := range []struct {
		name string
		set  int64
		want int64
	}{
		{"below the floor", 1_000_000, minVectorRasterPixels},
		{"inside the range", 16_000_000, 16_000_000},
		{"above the ceiling", 500_000_000, DefaultMaxVectorRasterPixels},
	} {
		t.Run(tc.name, func(t *testing.T) {
			SetMaxVectorRasterPixels(tc.set)
			if got := MaxVectorRasterPixels(); got != tc.want {
				t.Fatalf("MaxVectorRasterPixels = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClampVectorRasterHonorsALoweredCeiling(t *testing.T) {
	t.Cleanup(func() { SetMaxVectorRasterPixels(DefaultMaxVectorRasterPixels) })
	SetMaxVectorRasterPixels(minVectorRasterPixels) // 8 MP

	w, h := ClampVectorRaster(5440, 5440) // 29.6 MP: inside the default, over the lowered cap
	if n := int64(w) * int64(h); n > minVectorRasterPixels {
		t.Fatalf("clamped to %dx%d = %d px, over the lowered %d cap", w, h, n, int64(minVectorRasterPixels))
	}
}

func TestClampVectorRasterHoldsCapAtExtremeAspectRatios(t *testing.T) {
	// A shared scale factor stops being enough once either axis floors at
	// 1: the other axis is then free to carry the whole budget alone.
	//
	// The 1e11 rows are int literals that need a 64-bit int - fine for the
	// Makefile's amd64/arm64 targets, a compile error on a future 32-bit
	// one. Deliberate: shrinking them would stop testing what an SVG's
	// text-attribute axes can actually claim, and a compile error is loud.
	for _, tc := range []struct{ w, h int }{
		{100_000_000_000, 1},
		{1, 100_000_000_000},
		{200_000_000, 1},
		{DefaultMaxVectorRasterPixels * 4, 3},
	} {
		w, h := ClampVectorRaster(tc.w, tc.h)
		if n := int64(w) * int64(h); n > MaxVectorRasterPixels() {
			t.Errorf("ClampVectorRaster(%d, %d) = %dx%d = %d px, over the %d cap",
				tc.w, tc.h, w, h, n, MaxVectorRasterPixels())
		}
		if w < 1 || h < 1 {
			t.Errorf("ClampVectorRaster(%d, %d) = %dx%d, both axes must stay >= 1", tc.w, tc.h, w, h)
		}
	}
}
