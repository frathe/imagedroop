package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"time"
)

// minFrameDelay is substituted for a zero (or negative) frame delay, which
// most GIF encoders use to mean "as fast as possible" but which would
// otherwise spin the UI thread pointlessly.
const minFrameDelay = 100 * time.Millisecond

// decodeAnimatedGIF decodes every frame of an animated GIF, compositing each
// one onto the GIF's full canvas per its disposal method so every returned
// frame is a complete, ready-to-display image rather than just the
// (typically partial) region that frame updates. It returns a nil slice —
// not an error — for anything that isn't a multi-frame GIF, so callers fall
// back to decoding it as a static image.
//
// budget caps the total bytes the composited frames may retain. An
// animation over budget takes the same nil-slice path, with truncated set
// so the caller can tell the user why a GIF isn't moving; a budget of zero
// or less means "never composite an animation at all", which is what the
// thumbnail path passes since it keeps only the first frame anyway.
//
// One limit worth knowing: gif.DecodeAll below has already decoded every
// frame as a paletted image (about one byte per pixel) before the budget
// can be consulted, because the standard library exposes no way to learn a
// GIF's frame count without decoding it. What the check bounds is the four-
// bytes-per-pixel composited copies, which are both the dominant cost and
// the only part that stays referenced after this returns; the paletted
// decode is transient and bounded by MaxEncodedBytes and maxImagePixels.
func decodeAnimatedGIF(data []byte, budget int64) ([]image.Image, []time.Duration, bool) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(g.Image) <= 1 {
		return nil, nil, false
	}

	// Checked before the loop, not inside it, so an animation that can't
	// fit allocates nothing at all rather than filling up to the limit and
	// then throwing the work away.
	perFrame := int64(g.Config.Width) * int64(g.Config.Height) * 4

	if budget <= 0 || perFrame*int64(len(g.Image)) > budget {
		// Not "truncated" when the caller asked for no animation in the
		// first place - only when one was genuinely refused.
		return nil, nil, budget > 0
	}

	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvasImg := image.NewRGBA(bounds)

	frames := make([]image.Image, 0, len(g.Image))
	delays := make([]time.Duration, 0, len(g.Image))

	var beforeFrame *image.RGBA

	for i, frame := range g.Image {
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		// DisposalPrevious means "after this frame, restore the canvas to
		// how it looked before this frame was drawn", so snapshot now.
		if disposal == gif.DisposalPrevious {
			beforeFrame = copyRGBA(canvasImg)
		}

		draw.Draw(canvasImg, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		frames = append(frames, copyRGBA(canvasImg))

		delay := time.Duration(g.Delay[i]) * 10 * time.Millisecond
		if delay <= 0 {
			delay = minFrameDelay
		}
		delays = append(delays, delay)

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvasImg, frame.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvasImg = beforeFrame
		}
	}

	return frames, delays, false
}

func copyRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}
