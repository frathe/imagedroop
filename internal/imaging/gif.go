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
func decodeAnimatedGIF(data []byte) ([]image.Image, []time.Duration) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(g.Image) <= 1 {
		return nil, nil
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

	return frames, delays
}

func copyRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}
