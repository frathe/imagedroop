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
func buildGIF(t testing.TB, canvasW, canvasH int, frames []*image.Paletted, delays []int, disposal []byte) []byte {
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

	frames, delays, _ := decodeAnimatedGIF(data, DefaultImgCacheBytes)

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

	frames, _, _ := decodeAnimatedGIF(data, DefaultImgCacheBytes)

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

	_, delays, _ := decodeAnimatedGIF(data, DefaultImgCacheBytes)

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

	frames, delays, _ := decodeAnimatedGIF(data, DefaultImgCacheBytes)

	if frames != nil || delays != nil {
		t.Errorf("expected nil, nil for a single-frame GIF, got %d frames, %d delays", len(frames), len(delays))
	}
}

func TestDecodeAnimatedGIF_NotAGIFReturnsNil(t *testing.T) {
	frames, delays, _ := decodeAnimatedGIF([]byte("not a gif"), DefaultImgCacheBytes)

	if frames != nil || delays != nil {
		t.Errorf("expected nil, nil for non-GIF data, got %d frames, %d delays", len(frames), len(delays))
	}
}

// --- the animation budget ----------------------------------------------------

// TestDecodeAnimatedGIF_RefusesAnAnimationPastTheBudget covers the reason the
// budget exists: every frame is retained as a full composited RGBA canvas, so
// an animation's cost is canvas size times frame count - unbounded before
// this check, and unrelated to the per-image pixel cap, which only ever saw
// one canvas.
func TestDecodeAnimatedGIF_RefusesAnAnimationPastTheBudget(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}

	frames := make([]*image.Paletted, 4)
	delays := make([]int, 4)
	disposal := make([]byte, 4)
	for i := range frames {
		frames[i] = solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1])
		delays[i] = 5
		disposal[i] = gif.DisposalNone
	}

	data := buildGIF(t, 10, 10, frames, delays, disposal)

	// 10x10x4 bytes per frame across 4 frames is 1600; a budget one byte
	// short of that has to refuse the whole animation rather than
	// compositing up to the limit.
	got, gotDelays, truncated := decodeAnimatedGIF(data, 1599)

	if got != nil || gotDelays != nil {
		t.Errorf("expected nil, nil for an over-budget animation, got %d frames, %d delays", len(got), len(gotDelays))
	}
	if !truncated {
		t.Error("truncated = false, want true so the caller can tell the user why the GIF isn't moving")
	}

	// Exactly at the budget still plays.
	got, _, truncated = decodeAnimatedGIF(data, 1600)

	if len(got) != 4 {
		t.Errorf("frames = %d at exactly the budget, want 4", len(got))
	}
	if truncated {
		t.Error("truncated = true for an animation that fits, want false")
	}
}

// A zero budget is the thumbnail path's way of saying "never composite an
// animation" - it is not a refusal, so truncated stays false and the caller
// gets no toast about a limit it never asked to be near.
func TestDecodeAnimatedGIF_ZeroBudgetSkipsCompositingWithoutReportingTruncation(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frame1 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])

	data := buildGIF(t, 4, 4,
		[]*image.Paletted{frame1, frame2},
		[]int{5, 5},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	frames, delays, truncated := decodeAnimatedGIF(data, 0)

	if frames != nil || delays != nil {
		t.Errorf("expected nil, nil for a zero budget, got %d frames, %d delays", len(frames), len(delays))
	}
	if truncated {
		t.Error("truncated = true for a caller that asked for no animation at all, want false")
	}
}

// TestDecodeLoaded_FallsBackToAStaticFrameForAnOverBudgetAnimation is the
// user-visible half: the image still displays, it just doesn't move, and the
// flag is what internal/ui turns into a toast.
func TestDecodeLoaded_FallsBackToAStaticFrameForAnOverBudgetAnimation(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	frame1 := solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 10, 10), palette, palette[2])

	data := buildGIF(t, 10, 10,
		[]*image.Paletted{frame1, frame2},
		[]int{5, 5},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	loaded, err := DecodeLoaded(t.Context(), data, 1)
	if err != nil {
		t.Fatalf("DecodeLoaded returned error: %v", err)
	}

	if len(loaded.Frames) != 1 {
		t.Errorf("Frames = %d, want 1 - an over-budget animation still shows its first frame", len(loaded.Frames))
	}
	if !loaded.AnimationTruncated {
		t.Error("AnimationTruncated = false, want true")
	}

	if r, _, _, _ := loaded.Frames[0].At(5, 5).RGBA(); r == 0 {
		t.Error("the retained frame should be the animation's first (red) frame")
	}
}

func TestDecodeLoaded_LeavesAnimationTruncatedUnsetForAnimationsThatFit(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frame1 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])
	frame2 := solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])

	data := buildGIF(t, 4, 4,
		[]*image.Paletted{frame1, frame2},
		[]int{5, 5},
		[]byte{gif.DisposalNone, gif.DisposalNone})

	loaded, err := DecodeLoaded(t.Context(), data, DefaultImgCacheBytes)
	if err != nil {
		t.Fatalf("DecodeLoaded returned error: %v", err)
	}

	if len(loaded.Frames) != 2 {
		t.Fatalf("Frames = %d, want 2", len(loaded.Frames))
	}
	if loaded.AnimationTruncated {
		t.Error("AnimationTruncated = true for an animation that fits the budget, want false")
	}
}

// --- probeGIF: the pre-decode structural walk --------------------------------

// probeGIF's whole reason for existing is to answer "how many frames, and how
// big is the canvas" without decoding any pixels, so the budget can be checked
// before gif.DecodeAll allocates a paletted image per frame. The load-bearing
// property is therefore that it agrees with gif.DecodeAll on every GIF
// gif.DecodeAll can read - if it ever under-counts, an over-budget animation
// slips through the gate; if it ever fails on a readable GIF, that GIF stops
// animating.
func TestProbeGIF_AgreesWithDecodeAll(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	other := color.Palette{color.Black, color.RGBA{G: 255, A: 255}}

	fullFrame := func(n int) []*image.Paletted {
		frames := make([]*image.Paletted, n)
		for i := range frames {
			frames[i] = solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1+i%2])
		}
		return frames
	}

	cases := []struct {
		name   string
		w, h   int
		frames []*image.Paletted
	}{
		{"single frame", 10, 10, fullFrame(1)},
		{"two full-canvas frames", 10, 10, fullFrame(2)},
		{"many frames", 10, 10, fullFrame(9)},
		{
			// Partial-update frames: each image descriptor declares a
			// sub-rectangle, so the descriptor's own width/height differ from
			// the logical screen's and must not be mistaken for it.
			"frames smaller than the canvas", 10, 10,
			[]*image.Paletted{
				solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1]),
				solidFrame(image.Rect(3, 3, 7, 7), palette, palette[2]),
				solidFrame(image.Rect(1, 6, 4, 9), palette, palette[1]),
			},
		},
		{
			// A frame whose palette differs from the global one forces the
			// encoder to emit a local color table, which sits between the
			// image descriptor and the LZW data and has to be skipped by its
			// declared size.
			"local color tables", 8, 8,
			[]*image.Paletted{
				solidFrame(image.Rect(0, 0, 8, 8), palette, palette[1]),
				solidFrame(image.Rect(0, 0, 8, 8), other, other[1]),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delays := make([]int, len(tc.frames))
			disposal := make([]byte, len(tc.frames))
			for i := range tc.frames {
				delays[i] = 5
				disposal[i] = gif.DisposalNone
			}

			data := buildGIF(t, tc.w, tc.h, tc.frames, delays, disposal)

			g, err := gif.DecodeAll(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("gif.DecodeAll rejected the fixture: %v", err)
			}

			n, w, h, ok := probeGIF(data)

			if !ok {
				t.Fatal("ok = false for a GIF gif.DecodeAll reads fine")
			}
			if n != len(g.Image) {
				t.Errorf("frames = %d, want %d (gif.DecodeAll's count)", n, len(g.Image))
			}
			if w != g.Config.Width || h != g.Config.Height {
				t.Errorf("canvas = %dx%d, want %dx%d", w, h, g.Config.Width, g.Config.Height)
			}
		})
	}
}

// Comment and application extensions carry no pixels but do sit in the block
// stream between frames, so the walk has to skip them by their sub-block
// lengths rather than treating the first byte it doesn't recognize as the end.
func TestProbeGIF_SkipsExtensionBlocks(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1]),
		solidFrame(image.Rect(0, 0, 4, 4), palette, palette[0]),
	}

	data := buildGIF(t, 4, 4, frames, []int{5, 5}, []byte{gif.DisposalNone, gif.DisposalNone})

	// Splice the extensions in immediately before the trailer, the one
	// position locatable without re-implementing the parser under test.
	if data[len(data)-1] != 0x3B {
		t.Fatalf("fixture does not end in a trailer byte, got 0x%02x", data[len(data)-1])
	}

	comment := []byte{0x21, 0xFE, 0x05, 'h', 'e', 'l', 'l', 'o', 0x00}
	application := append([]byte{0x21, 0xFF, 0x0B}, []byte("NETSCAPE2.0")...)
	application = append(application, 0x03, 0x01, 0x00, 0x00, 0x00)

	spliced := append([]byte{}, data[:len(data)-1]...)
	spliced = append(spliced, comment...)
	spliced = append(spliced, application...)
	spliced = append(spliced, 0x3B)

	if _, err := gif.DecodeAll(bytes.NewReader(spliced)); err != nil {
		t.Fatalf("gif.DecodeAll rejected the spliced fixture, so it cannot pin the walk: %v", err)
	}

	n, _, _, ok := probeGIF(spliced)

	if !ok {
		t.Fatal("ok = false for a GIF carrying comment and application extensions")
	}
	if n != 2 {
		t.Errorf("frames = %d, want 2 - the extensions must not be counted or terminate the walk", n)
	}
}

// A file the walk cannot make sense of reports ok = false, which puts
// decodeAnimatedGIF on exactly the static-image path it already takes today
// when gif.DecodeAll returns an error.
func TestProbeGIF_RejectsMalformedInput(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1]),
		solidFrame(image.Rect(0, 0, 4, 4), palette, palette[0]),
	}
	good := buildGIF(t, 4, 4, frames, []int{5, 5}, []byte{gif.DisposalNone, gif.DisposalNone})

	// The trailer replaced by a byte that is neither extension introducer,
	// image descriptor, nor trailer.
	unknownBlock := append([]byte{}, good[:len(good)-1]...)
	unknownBlock = append(unknownBlock, 0xAB)

	cases := []struct {
		name string
		data []byte
	}{
		{"not a GIF at all", []byte("not a gif")},
		{"empty", nil},
		{"header only", good[:6]},
		{"truncated screen descriptor", good[:10]},
		{"truncated mid-stream", good[:len(good)-4]},
		{"unknown block type", unknownBlock},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, ok := probeGIF(tc.data); ok {
				t.Error("ok = true, want false for input the walk cannot trust")
			}
		})
	}
}

// FuzzProbeGIFAgreesWithDecodeAll pins the one property the budget gate rests
// on, over inputs no hand-written fixture would think to produce: whenever
// gif.DecodeAll can read a file, probeGIF must agree with it about the frame
// count and the canvas. Under-counting would let an over-budget animation
// through the gate; failing outright would stop a readable GIF from animating.
// The reverse direction is deliberately not asserted - probeGIF accepting a
// file gif.DecodeAll rejects is the safe divergence, and is documented on
// probeGIF itself.
func FuzzProbeGIFAgreesWithDecodeAll(f *testing.F) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	other := color.Palette{color.Black, color.RGBA{G: 255, A: 255}}

	f.Add(buildGIF(f, 4, 4,
		[]*image.Paletted{solidFrame(image.Rect(0, 0, 4, 4), palette, palette[1])},
		[]int{5}, []byte{gif.DisposalNone}))

	f.Add(buildGIF(f, 10, 10,
		[]*image.Paletted{
			solidFrame(image.Rect(0, 0, 10, 10), palette, palette[1]),
			solidFrame(image.Rect(3, 3, 7, 7), palette, palette[2]),
		},
		[]int{5, 5}, []byte{gif.DisposalNone, gif.DisposalBackground}))

	f.Add(buildGIF(f, 8, 8,
		[]*image.Paletted{
			solidFrame(image.Rect(0, 0, 8, 8), palette, palette[1]),
			solidFrame(image.Rect(0, 0, 8, 8), other, other[1]),
			solidFrame(image.Rect(2, 2, 6, 6), palette, palette[2]),
		},
		[]int{0, 7, 250}, []byte{gif.DisposalNone, gif.DisposalPrevious, gif.DisposalNone}))

	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return // nothing to agree about
		}

		n, w, h, ok := probeGIF(data)

		if !ok {
			t.Fatalf("ok = false for a GIF gif.DecodeAll read as %d frame(s)", len(g.Image))
		}
		if n != len(g.Image) {
			t.Errorf("frames = %d, want %d", n, len(g.Image))
		}
		if w != g.Config.Width || h != g.Config.Height {
			t.Errorf("canvas = %dx%d, want %dx%d", w, h, g.Config.Width, g.Config.Height)
		}
	})
}

// The point of the whole change: an animation past the budget must be refused
// without gif.DecodeAll ever running, since that call allocates a paletted
// image per frame before any budget check could see it. Allocation count is
// the observable that distinguishes "walked the blocks and declined" from
// "decoded everything, then declined" - the walk indexes into the caller's
// slice and allocates nothing, while decoding 24 frames allocates hundreds of
// objects.
func TestDecodeAnimatedGIF_RefusesOverBudgetWithoutDecodingFrames(t *testing.T) {
	palette := color.Palette{color.White, color.RGBA{R: 255, A: 255}}

	const count = 24
	frames := make([]*image.Paletted, count)
	delays := make([]int, count)
	disposal := make([]byte, count)
	for i := range frames {
		frames[i] = solidFrame(image.Rect(0, 0, 32, 32), palette, palette[i%2])
		delays[i] = 5
		disposal[i] = gif.DisposalNone
	}

	data := buildGIF(t, 32, 32, frames, delays, disposal)

	allocs := testing.AllocsPerRun(3, func() {
		if got, _, _ := decodeAnimatedGIF(data, 1); got != nil {
			t.Fatal("expected the animation to be refused")
		}
	})

	if allocs > 8 {
		t.Errorf("over-budget refusal allocated %.0f objects, want <= 8 - gif.DecodeAll appears to still be running before the budget check", allocs)
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

	_, delays, _ := decodeAnimatedGIF(data, DefaultImgCacheBytes)

	if got, want := delays[0], 70*time.Millisecond; got != want {
		t.Errorf("delays[0] = %v, want %v", got, want)
	}
	if got, want := delays[1], 2500*time.Millisecond; got != want {
		t.Errorf("delays[1] = %v, want %v", got, want)
	}
}
