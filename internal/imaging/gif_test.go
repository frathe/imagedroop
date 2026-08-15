package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

// buildGIF assembles a raw animated GIF from frames that may be smaller than
// the overall canvas (the GIF format lets each frame update only part of the
// image), so disposal-method compositing can be exercised directly.
func buildGIF(t *testing.T, canvasW, canvasH int, frames []*image.Paletted, delays []int, disposal []byte) []byte {
	t.Helper()

	g := &gif.GIF{
		Image:    frames,
		Delay:    delays,
		Disposal: disposal,
		Config: image.Config{
			ColorModel: frames[0].Palette,
			Width:      canvasW,
			Height:     canvasH,
		},
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encode gif: %v", err)
	}

	return buf.Bytes()
}

func solidFrame(bounds image.Rectangle, palette color.Palette, c color.Color) *image.Paletted {
	frame := image.NewPaletted(bounds, palette)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			frame.Set(x, y, c)
		}
	}
	return frame
}

func TestDecodeAnimatedGIF_DisposalNoneRetainsUntouchedRegion(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}

	// Frame 1 fills the whole 10x10 canvas red. Frame 2 only updates a 4x4
	// blue square in the corner, with DisposalNone, so the rest of frame 2
	// should still show frame 1's red.
	frame1 := solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1])
	frame2 := solidFrame(image.Rect(3, 3, 7, 7), palette, palette[2])

	data := buildGIF(t, 10, 10,
		[]*image.Paletted{frame1, frame2},
		[]int{5, 5},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	frames, delays := decodeAnimatedGIF(data)

	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if len(delays) != 2 {
		t.Fatalf("delays = %d, want 2", len(delays))
	}

	second := frames[1]

	if r, _, _, _ := second.At(0, 0).RGBA(); r == 0 {
		t.Errorf("frame 2 at (0,0) should still be red (untouched by frame 2), got r=%d", r)
	}
	if _, _, b, _ := second.At(5, 5).RGBA(); b == 0 {
		t.Errorf("frame 2 at (5,5) should be blue (inside frame 2's updated region), got b=%d", b)
	}
}

func TestDecodeAnimatedGIF_DisposalBackgroundClearsRegion(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}

	// Frame 1 fills the whole canvas red and disposes to background
	// (cleared/transparent) before frame 2 is drawn. Frame 2 only draws a
	// small blue square elsewhere, so frame 1's red should NOT bleed through
	// into frame 3 where frame 1's region is now showing again.
	frame1 := solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 2, 2), palette, palette[2])

	data := buildGIF(t, 10, 10,
		[]*image.Paletted{frame1, frame2},
		[]int{5, 5},
		[]byte{gif.DisposalBackground, gif.DisposalNone})

	frames, _ := decodeAnimatedGIF(data)

	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}

	second := frames[1]

	// (5,5) was red in frame 1 but frame 1 disposes to background before
	// frame 2 draws, and frame 2 doesn't touch (5,5), so it should now be
	// transparent rather than still showing frame 1's red.
	_, _, _, a := second.At(5, 5).RGBA()
	if a != 0 {
		t.Errorf("frame 2 at (5,5) should be cleared to transparent after frame 1's background disposal, got alpha=%d", a)
	}
}

func TestDecodeAnimatedGIF_ZeroDelayFloorsToMinimum(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frame1 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])

	data := buildGIF(t, 4, 4,
		[]*image.Paletted{frame1, frame2},
		[]int{0, 0},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	_, delays := decodeAnimatedGIF(data)

	for i, d := range delays {
		if d != minFrameDelay {
			t.Errorf("delays[%d] = %v, want the floor of %v for a zero-delay frame", i, d, minFrameDelay)
		}
	}
}

func TestDecodeAnimatedGIF_SingleFrameReturnsNil(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frame := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])

	data := buildGIF(t, 4, 4, []*image.Paletted{frame}, []int{10}, []byte{gif.DisposalNone})

	frames, delays := decodeAnimatedGIF(data)

	if frames != nil || delays != nil {
		t.Errorf("expected nil, nil for a single-frame GIF, got %d frames, %d delays", len(frames), len(delays))
	}
}

func TestDecodeAnimatedGIF_NotAGIFReturnsNil(t *testing.T) {
	frames, delays := decodeAnimatedGIF([]byte("not a gif"))

	if frames != nil || delays != nil {
		t.Errorf("expected nil, nil for non-GIF data, got %d frames, %d delays", len(frames), len(delays))
	}
}

// sanity check that the time conversion matches the GIF spec's 1/100s unit
func TestDecodeAnimatedGIF_DelayUnitConversion(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frame1 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])

	data := buildGIF(t, 4, 4,
		[]*image.Paletted{frame1, frame2},
		[]int{7, 250},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	_, delays := decodeAnimatedGIF(data)

	if got, want := delays[0], 70*time.Millisecond; got != want {
		t.Errorf("delays[0] = %v, want %v", got, want)
	}
	if got, want := delays[1], 2500*time.Millisecond; got != want {
		t.Errorf("delays[1] = %v, want %v", got, want)
	}
}
