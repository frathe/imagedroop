package imaging

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2/storage"
)

// --- scaleToFit -----------------------------------------------------------

func TestScaleToFit_DownscalesLandscapePreservingAspect(t *testing.T) {
	img := imageOfSize(t, 4000, 2000)

	got := scaleToFit(img, ThumbnailSize)

	b := got.Bounds()
	if b.Dx() != ThumbnailSize {
		t.Errorf("width = %d, want %d (the longer edge)", b.Dx(), ThumbnailSize)
	}
	if b.Dy() != ThumbnailSize/2 {
		t.Errorf("height = %d, want %d (aspect preserved)", b.Dy(), ThumbnailSize/2)
	}
}

func TestScaleToFit_DownscalesPortraitPreservingAspect(t *testing.T) {
	img := imageOfSize(t, 1000, 3000)

	got := scaleToFit(img, ThumbnailSize)

	b := got.Bounds()
	if b.Dy() != ThumbnailSize {
		t.Errorf("height = %d, want %d (the longer edge)", b.Dy(), ThumbnailSize)
	}
	if b.Dx() != ThumbnailSize/3 {
		t.Errorf("width = %d, want %d (aspect preserved)", b.Dx(), ThumbnailSize/3)
	}
}

func TestScaleToFit_LeavesAlreadySmallImageUnscaled(t *testing.T) {
	img := imageOfSize(t, 50, 30)

	got := scaleToFit(img, ThumbnailSize)

	if got != img {
		t.Errorf("scaleToFit changed an already-small image instead of returning it unchanged")
	}
}

func imageOfSize(t *testing.T, w, h int) *image.RGBA {
	t.Helper()
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

// --- NewThumbCache ----------------------------------------------------------

func TestNewThumbCache_IsEmptyAndUsable(t *testing.T) {
	c := NewThumbCache(DefaultThumbCacheBytes)

	if c.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 for a fresh cache", c.Len())
	}

	c.Add("key", imageOfSize(t, 1, 1))
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after Add", c.Len())
	}
}

// --- LoadThumbnail ----------------------------------------------------------

func TestLoadThumbnail_DecodesAndDownsamples(t *testing.T) {
	path := writeTempFile(t, "photo.jpg", encodeJPEG(t, 800, 400, color.RGBA{R: 200, G: 20, B: 20, A: 255}))

	thumb, err := LoadThumbnail(storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("LoadThumbnail returned error: %v", err)
	}

	b := thumb.Bounds()
	if b.Dx() != ThumbnailSize || b.Dy() != ThumbnailSize/2 {
		t.Errorf("thumbnail size = %dx%d, want %dx%d", b.Dx(), b.Dy(), ThumbnailSize, ThumbnailSize/2)
	}
}

// TestLoadThumbnail_DoesNotCompositeEveryAnimationFrame guards the zero
// animation budget LoadThumbnail passes. The two paths are told apart by the
// concrete type rather than by pixels, since both would end up showing frame
// 0's content: compositing runs every frame through copyRGBA and so yields an
// *image.RGBA, while the static path hands back what gif.Decode produced - a
// paletted first frame, untouched, because ApplyOrientation is a no-op for
// orientation 1 and the image is already inside ThumbnailSize so scaleToFit
// returns it unchanged. Before the budget existed, a long animation composited
// every frame to a full canvas here just to keep one and discard the rest.
func TestLoadThumbnail_DoesNotCompositeEveryAnimationFrame(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}

	path := writeTempFile(t, "anim.gif",
		encodeAnimatedGIF(t, 20, 20, []color.Color{red, blue, blue}, []int{5, 5, 5}))

	thumb, err := LoadThumbnail(storage.NewFileURI(path))
	if err != nil {
		t.Fatalf("LoadThumbnail returned error: %v", err)
	}

	if _, composited := thumb.(*image.RGBA); composited {
		t.Error("thumbnail is an *image.RGBA, which means every frame was composited before one was kept")
	}

	// Still the first frame's content, which is what the grid should show.
	if r, _, _, _ := thumb.At(10, 10).RGBA(); r == 0 {
		t.Error("thumbnail should be the animation's first (red) frame")
	}
}

func TestLoadThumbnail_PropagatesDecodeError(t *testing.T) {
	path := writeTempFile(t, "broken.jpg", []byte("not an image"))

	if _, err := LoadThumbnail(storage.NewFileURI(path)); err == nil {
		t.Error("LoadThumbnail returned nil error for an unparseable file, want an error")
	}
}
