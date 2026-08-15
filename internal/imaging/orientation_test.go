package imaging

import (
	"image"
	"image/color"
	"testing"
)

// markedImage builds a w x h RGBA image where each pixel's color encodes its
// own coordinates, so transforms can be checked by looking up where a given
// source pixel ended up.
func markedImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}

	return img
}

func at(t *testing.T, img image.Image, x, y int) (int, int) {
	t.Helper()
	c := img.At(x, y).(color.RGBA)
	return int(c.R), int(c.G)
}

func TestApplyOrientation(t *testing.T) {
	const w, h = 3, 2 // asymmetric so transposed cases are unambiguous

	cases := []struct {
		name        string
		orientation int
		wantW       int
		wantH       int
		// check maps a source (x,y) to the (x,y) it should land at in the
		// corrected image.
		check func(x, y int) (int, int)
	}{
		{"1: identity", 1, w, h, func(x, y int) (int, int) { return x, y }},
		{"2: flip horizontal", 2, w, h, func(x, y int) (int, int) { return w - 1 - x, y }},
		{"3: rotate 180", 3, w, h, func(x, y int) (int, int) { return w - 1 - x, h - 1 - y }},
		{"4: flip vertical", 4, w, h, func(x, y int) (int, int) { return x, h - 1 - y }},
		{"6: rotate 90 CW", 6, h, w, func(x, y int) (int, int) { return h - 1 - y, x }},
		{"8: rotate 270 CW", 8, h, w, func(x, y int) (int, int) { return y, w - 1 - x }},
		{"unknown value: treated as identity", 99, w, h, func(x, y int) (int, int) { return x, y }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := markedImage(w, h)
			out := ApplyOrientation(src, c.orientation)

			b := out.Bounds()
			if b.Dx() != c.wantW || b.Dy() != c.wantH {
				t.Fatalf("bounds = %dx%d, want %dx%d", b.Dx(), b.Dy(), c.wantW, c.wantH)
			}

			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					wantX, wantY := c.check(x, y)
					gotX, gotY := at(t, out, wantX, wantY)
					if gotX != x || gotY != y {
						t.Errorf("source (%d,%d): pixel at (%d,%d) = (%d,%d), want (%d,%d)",
							x, y, wantX, wantY, gotX, gotY, x, y)
					}
				}
			}
		})
	}

	// 5 (transpose) and 7 (transverse) swap dimensions like 6 and 8, but
	// mirror across a diagonal rather than rotating; check them by
	// composition against the primitives they're defined in terms of.
	t.Run("5: transpose matches flipH+rotate270CW", func(t *testing.T) {
		src := markedImage(w, h)
		want := rotate270CW(flipH(src))
		got := ApplyOrientation(src, 5)

		if got.Bounds() != want.Bounds() {
			t.Fatalf("bounds = %v, want %v", got.Bounds(), want.Bounds())
		}

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				gx, gy := at(t, got, x, y)
				wx, wy := at(t, want, x, y)
				if gx != wx || gy != wy {
					t.Errorf("(%d,%d) = (%d,%d), want (%d,%d)", x, y, gx, gy, wx, wy)
				}
			}
		}
	})

	t.Run("7: transverse matches flipH+rotate90CW", func(t *testing.T) {
		src := markedImage(w, h)
		want := rotate90CW(flipH(src))
		got := ApplyOrientation(src, 7)

		if got.Bounds() != want.Bounds() {
			t.Fatalf("bounds = %v, want %v", got.Bounds(), want.Bounds())
		}

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				gx, gy := at(t, got, x, y)
				wx, wy := at(t, want, x, y)
				if gx != wx || gy != wy {
					t.Errorf("(%d,%d) = (%d,%d), want (%d,%d)", x, y, gx, gy, wx, wy)
				}
			}
		}
	})
}

func TestRotateSteps(t *testing.T) {
	const w, h = 3, 2 // asymmetric so transposed cases are unambiguous

	cases := []struct {
		name  string
		steps int
		wantW int
		wantH int
		check func(x, y int) (int, int)
	}{
		{"0: identity", 0, w, h, func(x, y int) (int, int) { return x, y }},
		{"1: rotate 90 CW", 1, h, w, func(x, y int) (int, int) { return h - 1 - y, x }},
		{"2: rotate 180", 2, w, h, func(x, y int) (int, int) { return w - 1 - x, h - 1 - y }},
		{"3: rotate 270 CW", 3, h, w, func(x, y int) (int, int) { return y, w - 1 - x }},
		{"4: wraps back to identity", 4, w, h, func(x, y int) (int, int) { return x, y }},
		{"-1: one turn counter-clockwise matches 3", -1, h, w, func(x, y int) (int, int) { return y, w - 1 - x }},
		{"-4: wraps back to identity", -4, w, h, func(x, y int) (int, int) { return x, y }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := markedImage(w, h)
			out := RotateSteps(src, c.steps)

			b := out.Bounds()
			if b.Dx() != c.wantW || b.Dy() != c.wantH {
				t.Fatalf("bounds = %dx%d, want %dx%d", b.Dx(), b.Dy(), c.wantW, c.wantH)
			}

			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					wantX, wantY := c.check(x, y)
					gotX, gotY := at(t, out, wantX, wantY)
					if gotX != x || gotY != y {
						t.Errorf("source (%d,%d): pixel at (%d,%d) = (%d,%d), want (%d,%d)",
							x, y, wantX, wantY, gotX, gotY, x, y)
					}
				}
			}
		})
	}

	t.Run("identity returns the same image, not a copy", func(t *testing.T) {
		src := markedImage(w, h)
		out := RotateSteps(src, 0)
		if out != image.Image(src) {
			t.Error("RotateSteps(img, 0) should return img unchanged, not a new buffer")
		}
	})

	t.Run("composing two 90 CW steps matches one 180 step", func(t *testing.T) {
		src := markedImage(w, h)
		want := RotateSteps(src, 2)
		got := RotateSteps(RotateSteps(src, 1), 1)

		if got.Bounds() != want.Bounds() {
			t.Fatalf("bounds = %v, want %v", got.Bounds(), want.Bounds())
		}

		for y := 0; y < want.Bounds().Dy(); y++ {
			for x := 0; x < want.Bounds().Dx(); x++ {
				gx, gy := at(t, got, x, y)
				wx, wy := at(t, want, x, y)
				if gx != wx || gy != wy {
					t.Errorf("(%d,%d) = (%d,%d), want (%d,%d)", x, y, gx, gy, wx, wy)
				}
			}
		}
	})
}
