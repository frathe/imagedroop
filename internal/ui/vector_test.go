package ui

import (
	"image"
	"image/color"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/uitest"
)

// The logical-size floor for an SVG is the app's smallest window, but
// internal/imaging cannot import internal/ui to say so. This is what stops
// the two copies of that value from drifting apart silently.
func TestVectorFloorMatchesStartWindowSize(t *testing.T) {
	// Both sides are untyped constants, so comparing them directly lets the
	// compiler (and the IDE) constant-fold the condition to a literal
	// false/true - defeating the point of a test meant to catch a future
	// drift. Assigning to a var first forces a real runtime comparison.
	gotW, wantW := startW, float64(imaging.MinVectorWidth)
	if gotW != wantW {
		t.Fatalf("startW = %v, imaging.MinVectorWidth = %v", startW, imaging.MinVectorWidth)
	}
	gotH, wantH := startH, float64(imaging.MinVectorHeight)
	if gotH != wantH {
		t.Fatalf("startH = %v, imaging.MinVectorHeight = %v", startH, imaging.MinVectorHeight)
	}
}

func TestSVGDisplaysAtLogicalSize(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	if v.vector.svg == nil {
		t.Fatal("a loaded SVG must leave a Vector on the viewer")
	}
	if got := v.vector.logical; got.Width != 340 || got.Height != 340 {
		t.Fatalf("logical size = %v, want 340x340", got)
	}
	if b := v.img.Image.Bounds(); b.Dx() != 340 || b.Dy() != 340 {
		t.Fatalf("first raster = %dx%d, want 340x340", b.Dx(), b.Dy())
	}
}

func TestSVGReRendersAtHigherDensityOnZoom(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	before := v.vector.raster

	for range 6 { // 1.25^6 ~= 3.8x
		v.zoom.In()
	}
	v.vector.pending.Wait()

	if v.vector.raster.X <= before.X {
		t.Fatalf("raster stayed at %v after zooming in from %v", v.vector.raster, before)
	}
	if b := v.img.Image.Bounds(); b.Dx() != v.vector.raster.X {
		t.Fatalf("displayed image is %dx%d, out of step with vector.raster %v", b.Dx(), b.Dy(), v.vector.raster)
	}

	// The logical size must not move - it is what the window, the title and
	// the info overlay are all built on.
	if got := v.vector.logical; got.Width != 340 || got.Height != 340 {
		t.Fatalf("logical size drifted to %v", got)
	}
}

func TestSVGReRenderNeverMutatesTheCachedEntry(t *testing.T) {
	v, _, _ := newTestUI(t)
	uri := uitest.TempSVGURI(t, "icon.svg", 24, 24)
	dropAndWait(t, v, uri)

	cached, ok := v.imgCache.Get(uri.String())
	if !ok {
		t.Fatal("the loaded SVG should be in the image cache")
	}
	cachedBefore := cached.Frames[0].Bounds()

	for range 6 {
		v.zoom.In()
	}
	v.vector.pending.Wait()

	if got := cached.Frames[0].Bounds(); got != cachedBefore {
		t.Fatalf("re-render mutated the cached frame: %v -> %v", cachedBefore, got)
	}
	if v.displayFrames[0] == cached.Frames[0] {
		t.Fatal("displayFrames must not share the cached entry's backing array")
	}
}

// TestZoomKeysDriveVectorRerenders pins the zoom-to-request wiring and the
// frame/vector.raster consistency after a run of key presses. It does NOT
// pin coalescing - a rasterization per press would also pass here; that
// guarantee is TestRasterizeVectorCoalescesABurst's job below.
func TestZoomKeysDriveVectorRerenders(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	// Every one of these bumps the generation; only the last-spawned
	// goroutine survives its own gen check and actually rasterizes - the
	// rest bail before touching anything. Stays under 13 presses
	// deliberately: zoom's maxScale is 16, and 1.25^13 first exceeds it, so
	// a 13th press would repeat the same clamped scale zoom.Zoom.notifyScale
	// already reported, which it then skips - apply's read of img.Image
	// still happens, but requestVectorRender is never called for it, so
	// that read is never followed by a matching spawn.
	//
	// That matters because the invariant this test relies on is: no
	// foreground read of img.Image may follow the last surviving spawn
	// before vector.pending.Wait() is called. Every press up to and
	// including the 12th here is itself the trigger for a spawn (or is
	// superseded by one later in the loop), so nothing reads img.Image
	// after the winning goroutine exists; that goroutine's eventual write,
	// synchronized against this test goroutine only via vector.pending's
	// Done-then-Wait edge, lands while the test is safely blocked inside
	// Wait(). A 13th press would read img.Image (in zoom.apply, for its own
	// layout math) without spawning anything - an unsynchronized read that
	// could run concurrently with the earlier goroutine's write under the
	// fake test driver, which (unlike production) never marshals fyne.Do
	// onto a single goroutine, so nothing here would order the two.
	before := v.vector.lifecycle.currentRevision()
	for range 12 {
		v.zoom.In()
	}
	v.vector.pending.Wait()

	if v.vector.lifecycle.currentRevision() <= before {
		t.Fatal("zooming must request re-renders")
	}
	if b := v.img.Image.Bounds(); b.Dx() != v.vector.raster.X {
		t.Fatalf("final raster %dx%d out of step with %v", b.Dx(), b.Dy(), v.vector.raster)
	}
}

// TestRasterizeVectorCoalescesABurst pins the guarantee the generation
// counter and debounce exist for: a burst of scale changes produces
// exactly ONE rasterization. Deterministic where a real timer could not
// be: every spawned goroutine parks on the same channel, so the whole
// burst is in flight - generations all bumped - before any of them gets
// to its staleness check; closing the channel releases them together and
// only the final generation may reach RasterAt. Delete rasterizeVector's
// generation check and this fails with 5.
func TestRasterizeVectorCoalescesABurst(t *testing.T) {
	v, _, _ := newTestUI(t)

	release := make(chan time.Time)
	var rasterized atomic.Int32

	// All three writes happen before the drop - the write-once rule for
	// fields a background goroutine reads (see viewer.vector.debounce).
	v.vector.debounce = time.Hour // >0 routes through vector.after; the duration itself is never waited
	v.vector.after = func(time.Duration) <-chan time.Time { return release }
	inner := v.vector.rasterize
	v.vector.rasterize = func(vec *imaging.Vector, w, h int) (image.Image, error) {
		rasterized.Add(1)
		return inner(vec, w, h)
	}

	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	for scale := 2; scale <= 6; scale++ {
		v.requestVectorRender(float32(scale)) // each spawns and supersedes its predecessor
	}

	close(release)
	v.vector.pending.Wait()

	if got := rasterized.Load(); got != 1 {
		t.Fatalf("a burst of 5 scale changes rasterized %d times, want exactly 1", got)
	}
	if b := v.img.Image.Bounds(); b.Dx() != v.vector.raster.X {
		t.Fatalf("frame %dx%d out of step with vector.raster %v", b.Dx(), b.Dy(), v.vector.raster)
	}
}

// TestRasterizeVectorStopsOnShutdownSignal exercises the lifecycle-context arm
// of rasterizeVector's debounce select: shutdown invalidation cuts a parked
// goroutine out of its wait. Most tests here leave
// vector.debounce at the zero newTestUI sets it to and skip the select
// entirely; TestRasterizeVectorCoalescesABurst enters it too, but through
// the vector.after seam rather than a real timer, and never touches the
// stop arm.
func TestRasterizeVectorStopsOnShutdownSignal(t *testing.T) {
	v, _, _ := newTestUI(t)

	// Only this test wants a real wait: it exists to exercise the debounce
	// select itself, which every other test here zeroes out (see
	// newTestUI) to stay fast. Set before the drop, per the write-once
	// rule: the drop currently spawns nothing (a fresh SVG's fit scale is
	// <= 1), but a write after it would have no happens-before edge to any
	// goroutine the drop could spawn.
	v.vector.debounce = 20 * time.Millisecond

	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	before := v.vector.raster

	v.requestVectorRender(2) // comfortably past vectorSharpenRatio, so it spawns

	// Closed while the spawned goroutine is still parked in its debounce
	// select - the same shutdown signal run.go's SetOnStopped sends in
	// production. vector.pending.Wait(), not a sleep, is what proves the
	// goroutine actually exited: a sleep here would itself be racing the
	// 20ms debounce instead of deterministically observing the goroutine's
	// own exit.
	v.vector.lifecycle.invalidate()
	v.vector.pending.Wait()

	if v.vector.raster != before {
		t.Fatal("invalidating vector.lifecycle must not let a rasterization land")
	}
}

func TestRasterFormatKeepsNoVectorState(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempJPEGURI(t, "photo.jpg", 40, 30, color.White))

	if v.vector.svg != nil {
		t.Fatal("a JPEG must leave no Vector behind")
	}
	if got := v.vector.logical; got != (fyne.Size{}) {
		t.Fatalf("vector.logical = %v, want zero", got)
	}
}

func TestSVGThenRasterClearsVectorState(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))
	dropAndWait(t, v, uitest.TempJPEGURI(t, "photo.jpg", 40, 30, color.White))

	if v.vector.svg != nil {
		t.Fatal("navigating from an SVG to a JPEG must clear the Vector")
	}
}

func TestSVGRotationSwapsLogicalAxes(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "wide.svg", 200, 100)) // logical 520x260

	v.rotateBy(1)
	v.vector.pending.Wait()

	// The source is wider than it is tall, so a quarter turn must leave the
	// displayed frame taller than it is wide.
	if b := v.img.Image.Bounds(); b.Dy() <= b.Dx() {
		t.Fatalf("after a quarter turn the frame is %dx%d, want taller than wide", b.Dx(), b.Dy())
	}
}

func TestSVGRotationSwapsTheLogicalSizeZoomMeasuresAgainst(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "wide.svg", 200, 100)) // logical 520x260

	if got := v.zoom.LogicalSize(); got.Width != 520 || got.Height != 260 {
		t.Fatalf("before rotation zoom measures against %v, want 520x260", got)
	}

	v.rotateBy(1)
	v.vector.pending.Wait()

	// The quarter turn has to reach zoom, or fit scale is computed against
	// the wrong axis. Nothing downstream can reveal this: requestVectorRender
	// scales both axes by the same factor, so a wrong scale changes the
	// raster's magnitude but never its aspect ratio.
	if got := v.zoom.LogicalSize(); got.Width != 260 || got.Height != 520 {
		t.Fatalf("after a quarter turn zoom measures against %v, want 260x520", got)
	}

	// ...while the field itself must stay unrotated, since the raster target
	// is built from it in unrotated space.
	if got := v.vector.logical; got.Width != 520 || got.Height != 260 {
		t.Fatalf("vector.logical moved to %v, must stay the unrotated 520x260", got)
	}

	v.rotateBy(1) // 180 degrees: back to the original axes

	if got := v.zoom.LogicalSize(); got.Width != 520 || got.Height != 260 {
		t.Fatalf("at 180 degrees zoom measures against %v, want 520x260", got)
	}
}

// The regression guard for the transposed-target bug: requestVectorRender
// must NOT swap its raster target's axes on a quarter turn. Rotation
// preserves pixel count, so the unrotated raster stays in unrotated
// proportions; swapping instead hands RasterAt a transposed target, and
// oksvg's SetTarget scales each axis independently, stretching the drawing
// rather than turning it. Every other SVG fixture in this suite is square,
// where that bug is invisible - this one must not be.
func TestRotatedNonSquareSVGKeepsItsAspectRatio(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "wide.svg", 200, 100)) // logical 520x260, 2:1

	v.rotateBy(1)
	for range 6 {
		v.zoom.In()
	}
	v.vector.pending.Wait()

	// displayFrames[0] is the raster before rotation is applied, so it must
	// still be twice as wide as it is tall.
	b := v.displayFrames[0].Bounds()
	if got, want := float64(b.Dx())/float64(b.Dy()), 2.0; got < want*0.98 || got > want*1.02 {
		t.Fatalf("unrotated raster is %dx%d (aspect %.3f), want aspect ~2.0 - a swapped target stretches the drawing",
			b.Dx(), b.Dy(), got)
	}
}

func TestSVGReRendersAfterRotation(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	v.rotateBy(1)
	for range 6 {
		v.zoom.In()
	}
	v.vector.pending.Wait()

	if b := v.img.Image.Bounds(); b.Dx() != v.vector.raster.X {
		t.Fatalf("rotated frame %dx%d out of step with vector.raster %v", b.Dx(), b.Dy(), v.vector.raster)
	}
}

// TestInfoOverlayReportsLogicalSizeNotLiveRaster is the regression guard
// for a bug where updateInfoOverlay read v.img.Image.Bounds() directly:
// since rasterizeVector replaces that raster with a denser one on every
// landed re-render, the overlay's reported dimensions would climb as the
// user zoomed in, leaking an implementation detail (how sharp the current
// raster happens to be) into a field meant to describe the image's fixed
// size. Zooming an SVG must never move the reported dimensions, and a
// rotation must swap them exactly the way applyRotationLayout already
// swaps zoom's own logical size.
//
// Asserts against displayedDimensions() - the value updateInfoOverlay
// renders - rather than v.infoText itself while zooming: updateInfoOverlay's
// own nil-check reads v.img.Image, and toggling the overlay on during the
// zoom loop below would let that read race an in-flight rasterizeVector
// goroutine's write to the same field under the fake test driver, which -
// unlike production, where fyne.Do genuinely marshals onto one UI goroutine
// - runs a fyne.Do callback inline on whichever goroutine calls it (see
// ARCHITECTURE.md's concurrency invariant). The final check, once
// vector.pending.Wait() has drained every goroutine spawned by the rotation
// too, has nothing left to race and does go through the real v.infoText.
func TestInfoOverlayReportsLogicalSizeNotLiveRaster(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "wide.svg", 200, 100)) // logical 520x260

	if w, h := v.displayedDimensions(); w != 520 || h != 260 {
		t.Fatalf("dimensions = %dx%d, want 520x260", w, h)
	}

	for range 6 {
		v.zoom.In()
	}
	v.vector.pending.Wait()

	// Confirm the raster actually grew, or the assertion below would pass
	// vacuously no matter which way the bug went.
	if b := v.img.Image.Bounds(); b.Dx() <= 520 {
		t.Fatalf("test setup: raster did not grow after zooming in, got %dx%d", b.Dx(), b.Dy())
	}
	if w, h := v.displayedDimensions(); w != 520 || h != 260 {
		t.Fatalf("dimensions after zooming in = %dx%d, want unchanged 520x260 (raster is now %v)", w, h, v.img.Image.Bounds())
	}

	v.rotateBy(1)
	v.vector.pending.Wait()

	w, h := v.displayedDimensions()
	if w != 260 || h != 520 {
		t.Fatalf("dimensions after a quarter turn = %dx%d, want swapped 260x520", w, h)
	}

	// Nothing is in flight now, so it's safe to also confirm the value
	// reaches the overlay's actual rendered text, not just the helper.
	v.toggleInfoOverlay()
	if got := strings.Split(v.infoText.Text, "\n")[1]; got != "260 x 520" {
		t.Fatalf("infoText dimensions = %q, want %q", got, "260 x 520")
	}
}

func TestCloseFilesClearsVector(t *testing.T) {
	v, _, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "icon.svg", 24, 24))

	v.closeFiles()

	if v.vector.svg != nil {
		t.Fatal("closing files must clear the vector state")
	}
}

func TestVectorNeedsRender(t *testing.T) {
	for _, tc := range []struct {
		name       string
		have, want image.Point
		expect     bool
	}{
		{"nothing rendered yet", image.Point{}, image.Pt(340, 340), true},
		{"meaningfully sharper wanted", image.Pt(340, 340), image.Pt(680, 680), true},
		{"within the hysteresis band", image.Pt(340, 340), image.Pt(350, 350), false},
		{"slightly smaller wanted", image.Pt(340, 340), image.Pt(300, 300), false},
		{"grossly oversized, release it", image.Pt(2720, 2720), image.Pt(340, 340), true},
		{"degenerate target is never worth producing", image.Pt(100, 100), image.Pt(0, 0), false},
		{"exactly the sharpen boundary holds still", image.Pt(100, 100), image.Pt(105, 105), false},
		{"just past the sharpen boundary re-renders", image.Pt(100, 100), image.Pt(106, 106), true},
		{"exactly the release boundary holds still", image.Pt(100, 100), image.Pt(50, 50), false},
		{"just past the release boundary re-renders", image.Pt(100, 100), image.Pt(49, 49), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := vectorNeedsRender(tc.have, tc.want); got != tc.expect {
				t.Fatalf("vectorNeedsRender(%v, %v) = %v, want %v", tc.have, tc.want, got, tc.expect)
			}
		})
	}
}

// The window must be sized from the image, not from how sharp its current
// raster happens to be. Rotating a vector used to resize the window to
// v.img.Image.Bounds(), which for an SVG is whatever density the zoom level
// had driven the re-render to - so rotating a zoomed-in SVG threw the window
// to a size that had nothing to do with the picture. This pins the invariant
// that makes that impossible: the window a rotation produces does not depend
// on the zoom level it was rotated at.
func TestRotatingAZoomedSVGSizesTheWindowFromItsLogicalSize(t *testing.T) {
	v, win, _ := newTestUI(t)
	dropAndWait(t, v, uitest.TempSVGURI(t, "wide.svg", 200, 100)) // logical 520x260

	v.rotateBy(1)
	v.vector.pending.Wait()

	unzoomed := win.Canvas().Size()

	v.resetRotation()

	for range 6 { // ~3.8x, well inside zoom's maxScale clamp
		v.zoom.In()
	}
	v.vector.pending.Wait()

	if v.vector.raster.X <= int(v.vector.logical.Width) {
		t.Fatalf("raster is %v, expected zooming to have made it denser than the logical %v - "+
			"the rest of this test proves nothing otherwise", v.vector.raster, v.vector.logical)
	}

	v.rotateBy(1)
	v.vector.pending.Wait()

	if got := win.Canvas().Size(); got != unzoomed {
		t.Fatalf("rotating a zoomed SVG sized the window to %v, want %v - the same window "+
			"an unzoomed rotation produces", got, unzoomed)
	}
}

// The grid reaches an SVG through a wholly separate path from the display:
// LoadThumbnail runs its own ReadAndProbe/DecodeLoaded and builds an
// ephemeral Vector rather than sharing the cached one, then downsamples.
// Nothing else in this suite exercises that path, so a break in it would
// only show up as blank cells in the overview.
func TestSVGThumbnailsInTheGridOverview(t *testing.T) {
	v := newTestViewer(t)

	svg := uitest.TempSVGURI(t, "icon.svg", 24, 24) // logical 340x340
	dropAndWait(t, v, svg, uitest.TempJPEGURI(t, "photo.jpg", 8, 8, color.White))

	warmThumbs(t, v)

	v.grid.Toggle()
	if !v.grid.Visible() {
		t.Fatal("the grid should be open")
	}

	// Warm only reports that decoding did not error. Assert the vector
	// actually produced a downsampled thumbnail rather than nothing, or a
	// full-size raster the grid would then have to scale on every paint.
	thumb, err := imaging.LoadThumbnail(svg)
	if err != nil {
		t.Fatalf("LoadThumbnail on an SVG: %v", err)
	}

	b := thumb.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("SVG thumbnail is %dx%d, want real pixels", b.Dx(), b.Dy())
	}
	if b.Dx() >= imaging.MinVectorWidth {
		t.Fatalf("SVG thumbnail is %dx%d, want it downsampled well below the %d logical size",
			b.Dx(), b.Dy(), imaging.MinVectorWidth)
	}
	if b.Dx() != b.Dy() {
		t.Fatalf("SVG thumbnail is %dx%d, want the source's square aspect preserved", b.Dx(), b.Dy())
	}
}
